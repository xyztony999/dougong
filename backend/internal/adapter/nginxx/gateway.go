package nginxx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrUnavailable 表示 nginx 通道不可用（二进制未检出/无法执行/执行超时，
// A-SM18：区别于 `nginx -t` 退出码非 0 的配置错误）。
var ErrUnavailable = errors.New("nginxx: nginx channel unavailable")

// IsUnavailable 报告错误是否为（或包装了）通道不可用哨兵（service 层分派
// SITE_012/SITE_004 语义用）。
func IsUnavailable(err error) bool { return errors.Is(err, ErrUnavailable) }

// commandTimeout 单次 nginx 子命令（-t/-s reload）的执行上限（通道级防御；
// ACME 的 120s 上限独立于本值，D-09）。
const commandTimeout = 30 * time.Second

// Gateway 是 NginxGateway 契约的 nginxx 适配器实现（api-site-mgmt.md §4.2.5）：
// 配置目录读取（D-01）+ nginx 二进制通道（-t 校验 R-06 第三步 / -s reload
// D-05）+ 受管配置域内文件系统原语。全部外部命令经 exec.CommandContext
// 字面量参数数组执行（R-08：无 shell 无拼接），二进制路径以 exec.LookPath
// 解析、不信任外部传入可执行路径（backend-03 §4）。
type Gateway struct {
	binName   string
	confRoot  string
	dirNames  []string // 惯例目录名（优先级降序，D-03）
	binPath   string
	lookupErr error
}

// NewGateway 构造网关：bin 通常为 "nginx"（LookPath 解析；含路径分隔符时
// 视为显式路径直接校验）；confRoot 为 nginx 配置根（如 /etc/nginx）；
// dirNames 为惯例目录名优先级序列（sites-enabled > conf.d > vhost.d）。
func NewGateway(bin, confRoot string, dirNames []string) *Gateway {
	g := &Gateway{binName: bin, confRoot: confRoot, dirNames: dirNames}
	if strings.ContainsRune(bin, filepath.Separator) || strings.ContainsRune(bin, '/') {
		st, err := os.Stat(bin)
		switch {
		case err != nil:
			g.lookupErr = fmt.Errorf("nginxx: nginx binary %q: %w", bin, err)
		case st.IsDir() || st.Mode()&0o111 == 0:
			g.lookupErr = fmt.Errorf("nginxx: nginx binary %q is not executable", bin)
		default:
			g.binPath = bin
		}
		return g
	}
	p, err := exec.LookPath(bin)
	if err != nil {
		g.lookupErr = fmt.Errorf("nginxx: nginx binary %q not found in PATH: %w", bin, err)
		return g // lookupErr 承载不可用原因，Available 呈现（SITE_012）
	}
	g.binPath = p
	return g
}

// FileSnapshot 是一个配置文件的读取快照（D-01：文件清单+原文）。
type FileSnapshot struct {
	Path    string
	Dir     string // 所属惯例目录绝对路径（优先级判定用）
	Name    string // 文件名
	Content string
	ModTime time.Time
}

// ConfRoot 返回配置根目录。
func (g *Gateway) ConfRoot() string { return g.confRoot }

// PriorityDirs 返回惯例目录绝对路径（优先级降序；可能不存在，按需探测）。
func (g *Gateway) PriorityDirs() []string {
	out := make([]string, 0, len(g.dirNames))
	for _, d := range g.dirNames {
		out = append(out, filepath.Join(g.confRoot, d))
	}
	return out
}

// Available 返回 nginx 通道可用性；不可用时错误携带原因（SITE_012 data.reason）。
func (g *Gateway) Available() error { return g.lookupErr }

// ScanConfDirs 读取全部惯例目录中的 .conf 文件（D-01 实时扫描；目录不存在
// 跳过——非全部发行版三目录齐备；文件按目录优先级内文件名升序稳定排序）。
// 纯文件读取不经 nginx 二进制（GET 读路径不前置通道可用性，I-12——nginx
// 缺席时站点投影仍可由文件实况推导）。
func (g *Gateway) ScanConfDirs() ([]FileSnapshot, error) {
	var out []FileSnapshot
	for _, dir := range g.PriorityDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("nginxx: scan %s: %w", dir, err)
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") {
				continue
			}
			names = append(names, e.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			p := filepath.Join(dir, name)
			b, err := os.ReadFile(p)
			if err != nil {
				return nil, fmt.Errorf("nginxx: read %s: %w", p, err)
			}
			st, err := os.Stat(p)
			if err != nil {
				return nil, fmt.Errorf("nginxx: stat %s: %w", p, err)
			}
			out = append(out, FileSnapshot{
				Path: p, Dir: dir, Name: name,
				Content: string(b), ModTime: st.ModTime(),
			})
		}
	}
	return out, nil
}

// ReadFile 读取受管配置文件内容与 mtime（源码编辑乐观锁基线，D-06）。
func (g *Gateway) ReadFile(path string) (string, time.Time, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("nginxx: read %s: %w", path, err)
	}
	st, err := os.Stat(path)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("nginxx: stat %s: %w", path, err)
	}
	return string(b), st.ModTime(), nil
}

// WriteFileAtomic 原子写回：同目录临时文件 + rename（R-05 临时文件+rename
// 同法；观察者要么见旧内容要么见新内容，无半写状态）。
func (g *Gateway) WriteFileAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".dougong-write-*")
	if err != nil {
		return fmt.Errorf("nginxx: temp create in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		if rmErr := os.Remove(tmpName); rmErr != nil && !os.IsNotExist(rmErr) {
			return // 清理失败只能放弃（rename 前提下不影响主流程错误语义）
		}
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("nginxx: temp write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("nginxx: temp close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		cleanup()
		return fmt.Errorf("nginxx: temp chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("nginxx: rename %s → %s: %w", tmpName, path, err)
	}
	return nil
}

// WriteSecretFileAtomic 原子写敏感文件（ACME 账户密钥/证书私钥等，0600）——
// 与 WriteFileAtomic 同法，权限收紧（D-09：密钥文件 0600）。
func (g *Gateway) WriteSecretFileAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".dougong-write-*")
	if err != nil {
		return fmt.Errorf("nginxx: temp create in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		if rmErr := os.Remove(tmpName); rmErr != nil && !os.IsNotExist(rmErr) {
			return // 清理失败只能放弃（rename 前提下不影响主流程错误语义）
		}
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("nginxx: temp write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("nginxx: temp close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		cleanup()
		return fmt.Errorf("nginxx: temp chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("nginxx: rename %s → %s: %w", tmpName, path, err)
	}
	return nil
}

// RemoveFile 删除文件（不存在视为成功——删除语义幂等）。
func (g *Gateway) RemoveFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("nginxx: remove %s: %w", path, err)
	}
	return nil
}

// CopyFile 复制文件（快照备份用；目标以 0644 创建）。
func (g *Gateway) CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("nginxx: open %s: %w", src, err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("nginxx: mkdir %s: %w", filepath.Dir(dst), err)
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("nginxx: create %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("nginxx: copy %s → %s: %w", src, dst, err)
	}
	return out.Close()
}

// MkdirAll 递归建目录。
func (g *Gateway) MkdirAll(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("nginxx: mkdir %s: %w", dir, err)
	}
	return nil
}

// Exists 报告路径是否存在（文件或目录）。
func (g *Gateway) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DirExists 报告目录存在且是目录。
func (g *Gateway) DirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// FirstWritableDir 返回首个「存在且可写」的惯例目录（D-03 新建落点；
// 全部不可用时返回错误——无法落盘即无法建站）。
func (g *Gateway) FirstWritableDir() (string, error) {
	for _, dir := range g.PriorityDirs() {
		if !g.DirExists(dir) || !g.dirWritable(dir) {
			continue
		}
		return dir, nil
	}
	return "", fmt.Errorf("nginxx: no writable config dir under %s", g.confRoot)
}

// dirWritable 以创建临时文件探测目录可写性（探测文件即建即删）。
func (g *Gateway) dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".dougong-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

// TestResult 是 `nginx -t` 的结果：ok=false 表示配置错误（errors 为输出行）；
// err 非 nil 表示通道不可用（A-SM18）。
type TestResult struct {
	OK     bool
	Output []string
}

// TestConfig 执行 `nginx -t`（R-06 第三步；字面量参数数组 R-08）。退出码
// 非 0=配置错误（ok=false + 输出行）；进程无法执行/超时=通道不可用
// （ErrUnavailable，SITE_012 语义）。
func (g *Gateway) TestConfig(ctx context.Context) (TestResult, error) {
	if g.lookupErr != nil {
		return TestResult{}, g.lookupErr
	}
	cctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, g.binPath, "-t") // 字面量参数数组，无拼接
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if cctx.Err() == nil && errors.As(err, &ee) { // 已执行且退出码非 0：配置错误
			return TestResult{OK: false, Output: splitLines(buf.String())}, nil
		}
		// spawn 失败/被超时杀死：通道不可用
		return TestResult{}, fmt.Errorf("%w: %s -t: %v", ErrUnavailable, g.binName, err)
	}
	return TestResult{OK: true, Output: splitLines(buf.String())}, nil
}

// Reload 执行 `nginx -s reload`（D-05 平滑重载）。返回输出文本；进程退出码
// 非 0 或无法执行均为错误（调用方按 SITE_004/SITE_012 语义分派——reload
// 失败时以输出报错、配置保留）。
func (g *Gateway) Reload(ctx context.Context) (string, error) {
	if g.lookupErr != nil {
		return "", g.lookupErr
	}
	cctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, g.binPath, "-s", "reload") // 字面量参数数组
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if cctx.Err() == nil && errors.As(err, &ee) { // 命令本身失败（D-05：配置保留 + 输出报错）
			out := buf.String()
			// v2.7 存量坑回退：pid 文件空/缺失使 -s reload 失败而 master 仍存活
			// （实机：/run/nginx.pid 空文件，systemd ExecReload 同败）——解析
			// master PID 直接 SIGHUP（同平滑重载语义）；解析/发信号失败则保留原错误。
			if isPidFileErr(out) {
				if pid, perr := masterPid(cctx); perr == nil {
					if kerr := signalHup(pid); kerr == nil {
						return out + "\n(nginx -s reload 因 pid 文件无效失败；已回退 SIGHUP master pid " +
							strconv.Itoa(pid) + ")", nil
					}
				}
			}
			return out, fmt.Errorf("nginxx: reload exit: %w: %s", err, out)
		}
		return "", fmt.Errorf("%w: %s -s reload: %v", ErrUnavailable, g.binName, err)
	}
	return buf.String(), nil
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\r\n")
	if s == "" {
		return nil
	}
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}
