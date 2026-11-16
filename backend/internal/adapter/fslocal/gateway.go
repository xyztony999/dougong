// Package fslocal 是本机文件系统适配器（FSGateway 契约实现，api-file-manager
// §4.2.5）：全部入参为 PathScopeGuard 校验后的真实路径（R-04 出口收敛），
// 纯 Go 标准库零外部命令（R-08/R-09）。平台差异（uid/gid 属主映射、Chown、
// EXDEV 判定）经 build tag 分离（A-F7）：unix 完整 / windows 退化。
package fslocal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	filemansvc "github.com/dougong/dougong/internal/service/fileman"
)

// Gateway 实现 fileman.FSGateway（无状态；每次调用直达文件系统实况）。
type Gateway struct{}

// New 构造本机文件系统网关。
func New() *Gateway { return &Gateway{} }

var _ filemansvc.FSGateway = (*Gateway)(nil)

// classifyEntry mode → file_type 枚举（其他特殊类型按 file 归类，§3.4 三值域）。
func classifyEntry(m fs.FileMode) string {
	switch {
	case m.IsDir():
		return filemansvc.TypeDirectory
	case m&fs.ModeSymlink != 0:
		return filemansvc.TypeSymlink
	default:
		return filemansvc.TypeFile
	}
}

// snapshot 由 Lstat 信息组装条目快照（owner 经平台映射；目录 size=0 不计子树）。
func (g *Gateway) snapshot(info fs.FileInfo, owners ownerMaps) filemansvc.EntrySnapshot {
	size := info.Size()
	if info.IsDir() {
		size = 0 // 目录为 0（§4.2.1）
	}
	var uid, gid int
	if st, ok := statOwner(info); ok {
		uid, gid = st.uid, st.gid
	}
	return filemansvc.EntrySnapshot{
		Name:       info.Name(),
		EntryType:  classifyEntry(info.Mode()),
		Size:       size,
		Mode:       fmt.Sprintf("%04o", info.Mode().Perm()),
		OwnerUser:  owners.user[uid],
		OwnerGroup: owners.group[gid],
		Mtime:      info.ModTime(),
	}
}

// ListDir 目录投影（I-06 当次快照；含隐藏条目 I-10；Lstat 语义符号链接按
// 链接自身呈现）。
func (g *Gateway) ListDir(ctx context.Context, realPath string) ([]filemansvc.EntrySnapshot, error) {
	entries, err := os.ReadDir(realPath)
	if err != nil {
		return nil, mapStatErr(err)
	}
	owners := loadOwners() // 每次调用实时读取（系统实况，D-14 同口径）
	out := make([]filemansvc.EntrySnapshot, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // 竞态消失的条目跳过（投影一致性以当次为准）
			}
			return nil, mapStatErr(err)
		}
		out = append(out, g.snapshot(info, owners))
	}
	return out, nil
}

// Stat 单点属性快照（Lstat 语义；入参经守卫解析，真实路径非符号链接）。
func (g *Gateway) Stat(ctx context.Context, realPath string) (filemansvc.EntrySnapshot, error) {
	info, err := os.Lstat(realPath)
	if err != nil {
		return filemansvc.EntrySnapshot{}, mapStatErr(err)
	}
	snap := g.snapshot(info, loadOwners())
	snap.Name = filepath.Base(realPath)
	return snap, nil
}

// ReadFile 读取文件内容（limit>0 至多 limit 字节；≤0 全量——zip 预检 A-F18）。
func (g *Gateway) ReadFile(ctx context.Context, realPath string, limit int64) ([]byte, error) {
	f, err := os.Open(realPath) //nolint:gosec // 入参经守卫校验（R-04）
	if err != nil {
		return nil, mapStatErr(err)
	}
	defer func() { _ = f.Close() }()
	if limit > 0 {
		buf := make([]byte, limit)
		n, err := io.ReadFull(f, buf)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("fslocal read %s: %w", realPath, err)
		}
		return buf[:n], nil
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("fslocal read %s: %w", realPath, err)
	}
	return data, nil
}

// WriteFileAtomic 临时文件+rename 原子写（I-09；失败清理临时文件不留半写）。
func (g *Gateway) WriteFileAtomic(ctx context.Context, realPath string, data []byte) error {
	return g.writeAtomic(ctx, realPath, func(w io.Writer) error {
		if _, err := w.Write(data); err != nil {
			return err
		}
		return nil
	})
}

// WriteFileAtomicStreaming 流式原子写（A-F5；大体量载荷免全量进内存）。
func (g *Gateway) WriteFileAtomicStreaming(ctx context.Context, realPath string, fn func(io.Writer) error) error {
	return g.writeAtomic(ctx, realPath, fn)
}

func (g *Gateway) writeAtomic(ctx context.Context, realPath string, fn func(io.Writer) error) (err error) {
	dir := filepath.Dir(realPath)
	tmp, err := os.CreateTemp(dir, ".dougong-tmp-*")
	if err != nil {
		return fmt.Errorf("fslocal temp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		// 失败或 panic 展栈时统一清理（I-09 不留半写文件与句柄）。
		if !committed {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()
	if err = fn(tmp); err != nil {
		return fmt.Errorf("fslocal write temp: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("fslocal sync temp: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("fslocal close temp: %w", err)
	}
	if err = os.Chmod(tmpName, existingOrDefaultMode(realPath)); err != nil {
		return fmt.Errorf("fslocal chmod temp: %w", err)
	}
	if err = os.Rename(tmpName, realPath); err != nil {
		return fmt.Errorf("fslocal rename into %s: %w", realPath, err)
	}
	committed = true
	return nil
}

// existingOrDefaultMode 覆盖场景保留既有权限位，新建默认 0644。
func existingOrDefaultMode(realPath string) fs.FileMode {
	if info, err := os.Lstat(realPath); err == nil {
		return info.Mode().Perm()
	}
	return 0o644
}

// Mkdir 新建空目录。
func (g *Gateway) Mkdir(ctx context.Context, realPath string) error {
	if err := os.Mkdir(realPath, 0o755); err != nil {
		return mapStatErr(err)
	}
	return nil
}

// CreateFile 新建空文件（O_EXCL：同名即错，D-13 语义由 service 预检兜底）。
func (g *Gateway) CreateFile(ctx context.Context, realPath string) error {
	f, err := os.OpenFile(realPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return mapStatErr(err)
	}
	return f.Close()
}

// Rename 重命名/移动（EXDEV 上抛触发调用方降级，D-15）。
func (g *Gateway) Rename(ctx context.Context, from, to string) error {
	if err := os.Rename(from, to); err != nil {
		if isCrossDevice(err) {
			return filemansvc.ErrCrossDeviceRename
		}
		return mapStatErr(err)
	}
	return nil
}

// CopyThenRemove EXDEV 降级 copy+delete（D-15；目录递归、符号链接按链接复制）。
func (g *Gateway) CopyThenRemove(ctx context.Context, from, to string) error {
	if err := copyTree(from, to); err != nil {
		return err
	}
	if err := os.RemoveAll(from); err != nil {
		return fmt.Errorf("fslocal remove after copy %s: %w", from, err)
	}
	return nil
}

func copyTree(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return mapStatErr(err)
	}
	switch {
	case info.IsDir():
		if err := os.MkdirAll(to, info.Mode().Perm()); err != nil {
			return fmt.Errorf("fslocal mkdir %s: %w", to, err)
		}
		entries, err := os.ReadDir(from)
		if err != nil {
			return mapStatErr(err)
		}
		for _, e := range entries {
			if err := copyTree(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
				return err
			}
		}
		return nil
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(from)
		if err != nil {
			return fmt.Errorf("fslocal readlink %s: %w", from, err)
		}
		if err := os.Symlink(target, to); err != nil {
			return fmt.Errorf("fslocal symlink %s: %w", to, err)
		}
		return nil
	default:
		in, err := os.Open(from) //nolint:gosec // 入参经守卫校验
		if err != nil {
			return mapStatErr(err)
		}
		defer func() { _ = in.Close() }()
		out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return fmt.Errorf("fslocal create %s: %w", to, err)
		}
		defer func() { _ = out.Close() }()
		if _, err := io.Copy(out, in); err != nil {
			return fmt.Errorf("fslocal copy %s→%s: %w", from, to, err)
		}
		return nil
	}
}

// RemoveTree 递归删除（不存在视为成功——purge/清理幂等语义）。
func (g *Gateway) RemoveTree(ctx context.Context, realPath string) error {
	if err := os.RemoveAll(realPath); err != nil {
		return fmt.Errorf("fslocal remove tree %s: %w", realPath, err)
	}
	return nil
}

// Chmod 权限位修改（mode 为服务层已校验的 0~0777 八进制串）。
func (g *Gateway) Chmod(ctx context.Context, realPath string, mode string) error {
	v, err := strconv.ParseUint(strings.TrimSpace(mode), 8, 32)
	if err != nil {
		return fmt.Errorf("fslocal chmod parse %q: %w", mode, err)
	}
	if err := os.Chmod(realPath, fs.FileMode(v)); err != nil {
		return mapStatErr(err)
	}
	return nil
}

// OpenReadStream 打开只读流（下载：校验后开流，D-16）。
func (g *Gateway) OpenReadStream(ctx context.Context, realPath string) (io.ReadCloser, error) {
	f, err := os.Open(realPath) //nolint:gosec // 入参经守卫校验（R-04）
	if err != nil {
		return nil, mapStatErr(err)
	}
	return f, nil
}

// EnsureDir MkdirAll 语义（trash 目录/解压目录惰性建立）。
func (g *Gateway) EnsureDir(ctx context.Context, realPath string) error {
	if err := os.MkdirAll(realPath, 0o755); err != nil {
		return fmt.Errorf("fslocal ensure dir %s: %w", realPath, err)
	}
	return nil
}

// SizeOf 累计大小（文件=自身；目录=子树累计——trash 入站登记 size 列语义）。
func (g *Gateway) SizeOf(ctx context.Context, realPath string) (int64, error) {
	info, err := os.Lstat(realPath)
	if err != nil {
		return 0, mapStatErr(err)
	}
	if !info.IsDir() {
		return info.Size(), nil
	}
	var total int64
	err = filepath.WalkDir(realPath, func(_ string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.Type().IsRegular() {
			if di, err := d.Info(); err == nil {
				total += di.Size()
			}
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("fslocal size of %s: %w", realPath, err)
	}
	return total, nil
}

// Exists 存在性探测（Lstat 语义；冲突预检用）。
func (g *Gateway) Exists(ctx context.Context, realPath string) (bool, error) {
	_, err := os.Lstat(realPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, mapStatErr(err)
	}
	return true, nil
}

// RemoveSingle 删除单个条目（不递归；文件/空目录/符号链接）。
func (g *Gateway) RemoveSingle(ctx context.Context, realPath string) error {
	if err := os.Remove(realPath); err != nil {
		return mapStatErr(err)
	}
	return nil
}

// mapStatErr os 错误 → 通道哨兵（service 再映射 FILE 码）。
func mapStatErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return filemansvc.ErrEntryNotExist
	}
	if isNotADirectory(err) { // 平台判定（syscall.ENOTDIR 解链，无字符串匹配）
		return filemansvc.ErrNotADirectory
	}
	// fs.ErrPermission 与其余错误原样包装上抛（service 归一 FILE_013/权限 reason）。
	return fmt.Errorf("fslocal: %w", err)
}
