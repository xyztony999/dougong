// Package appx 是应用管理（app-mgmt）的本机资源适配层（api-app-mgmt.md
// §4.4 网关契约：AppPyGateway/AppUnitWriter/LogGateway 承载）。全部外部命令
// 字面量参数数组（R-08）；路径类入参须经 service 层规范化校验后传入（防逃逸
// 前置在 service，D-03）。Windows/无 systemd 环境命令如实失败（install.sh
// 支持矩阵外，不短路不虚构）。
package appx

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	// UnitDir 服务单元落位目录（D-06 编译期常量）。
	UnitDir = "/etc/systemd/system"
	// LogTimeout 单条日志查询超时（D-11）。
	LogTimeout = 10 * time.Second
	// verifyTimeout systemd-analyze verify 超时（D-06 安全写入管线）。
	verifyTimeout = 30 * time.Second
)

// Gateway 聚合 venv/pip/单元/日志通道（零状态；并发安全）。
type Gateway struct{}

// New 构造适配器。
func New() *Gateway { return &Gateway{} }

// run 执行外部命令（R-08 参数数组；dir 为工作目录；输出合并捕获）。
func run(ctx context.Context, dir string, timeout time.Duration, name string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return buf.String(), fmt.Errorf("appx: %s %v: %w", name, args, err)
	}
	return buf.String(), nil
}

// ---- AppPyGateway（D-05：venv 与依赖）----

// CreateVenv 在应用目录创建 .venv（python3 -m venv .venv；PY_OP_TIMEOUT 由
// service 传入）。目录不存在由 service 前置校验（APP_003）。
func (g *Gateway) CreateVenv(ctx context.Context, dir string, timeout time.Duration) (string, error) {
	return run(ctx, dir, timeout, "python3", "-m", "venv", ".venv")
}

// PipInstall 在应用目录 venv 内安装 requirements.txt（无清单时调用方跳过；
// 清单缺失/venv 缺失如实报错不虚构成功）。
func (g *Gateway) PipInstall(ctx context.Context, dir string, timeout time.Duration) (string, error) {
	return run(ctx, dir, timeout, filepath.ToSlash(filepath.Join(".venv", "bin", "pip")),
		"install", "-r", "requirements.txt")
}

// VenvExists 判定应用目录 venv 存在性（D-08：.venv/ 目录判定）。
func (g *Gateway) VenvExists(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, ".venv"))
	return err == nil && fi.IsDir()
}

// DepsProbe 依赖安装态只读计数（D-01：venv 内 pip list 行数；venv 缺失=错误如实）。
func (g *Gateway) DepsProbe(ctx context.Context, dir string) (int, error) {
	out, err := run(ctx, dir, LogTimeout, filepath.ToSlash(filepath.Join(".venv", "bin", "pip")),
		"list", "--format=freeze")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, line := range splitLines(out) {
		if line != "" {
			n++
		}
	}
	return n, nil
}

// RemoveDir 删除应用目录（D-07 purgeDir=true 显式路径；拒绝空/根/相对路径——
// 防逃逸最后一道防线，service 层规范化校验前置）。
func (g *Gateway) RemoveDir(dir string) error {
	if !filepath.IsAbs(dir) || dir == string(filepath.Separator) || filepath.Clean(dir) == string(filepath.Separator) {
		return fmt.Errorf("appx: refuse to remove non-absolute or root path %q", dir)
	}
	return os.RemoveAll(dir)
}

// ---- AppUnitWriter（D-06：单元安全写入管线——备份→原子写→校验→失败回滚）----

// WriteUnit 写入服务单元文件：已存在文件先备份（.dougong-bak 后缀）→ 原子写
// （临时文件+rename）→ systemd-analyze verify 校验 → 校验失败回滚原字节。
// 返回回滚函数（调用方在后续步骤失败时恢复原状）。
func (g *Gateway) WriteUnit(ctx context.Context, unitName, content string) (rollback func(), err error) {
	path := filepath.Join(UnitDir, unitName)
	backup := path + ".dougong-bak"
	hadBackup := false
	if _, err := os.Stat(path); err == nil {
		prev, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil, fmt.Errorf("appx: read existing unit %s: %w", unitName, rerr)
		}
		if werr := os.WriteFile(backup, prev, 0o644); werr != nil {
			return nil, fmt.Errorf("appx: backup unit %s: %w", unitName, werr)
		}
		hadBackup = true
	}
	restore := func() {
		if hadBackup {
			_, _ = copyFile(backup, path)
		} else {
			_ = os.Remove(path)
		}
		_ = g.DaemonReload(context.Background())
	}
	tmp := path + ".dougong-tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return restore, fmt.Errorf("appx: write unit tmp %s: %w", unitName, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return restore, fmt.Errorf("appx: rename unit %s: %w", unitName, err)
	}
	if err := g.VerifyUnit(ctx, path); err != nil {
		restore()
		return nil, fmt.Errorf("appx: unit verify failed (rolled back): %w", err)
	}
	if err := g.DaemonReload(ctx); err != nil {
		restore()
		return nil, fmt.Errorf("appx: daemon-reload failed (rolled back): %w", err)
	}
	_ = os.Remove(backup)
	return func() {}, nil
}

// RemoveUnit 移除服务单元（D-07；先移入备份位，daemon-reload 后清理；
// 单元不存在=幂等无错）。
func (g *Gateway) RemoveUnit(ctx context.Context, unitName string) error {
	path := filepath.Join(UnitDir, unitName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	backup := path + ".dougong-bak"
	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("appx: stash unit %s: %w", unitName, err)
	}
	if err := g.DaemonReload(ctx); err != nil {
		// 回滚：恢复单元文件（保守——撤除失败不静默）。
		_, _ = copyFile(backup, path)
		_ = os.Remove(backup)
		return fmt.Errorf("appx: daemon-reload after unit removal: %w", err)
	}
	return os.Remove(backup)
}

// VerifyUnit systemd-analyze verify 校验单元文件（D-06 第三步）。
func (g *Gateway) VerifyUnit(ctx context.Context, path string) error {
	_, err := run(ctx, "", verifyTimeout, "systemd-analyze", "verify", path)
	return err
}

// DaemonReload systemctl daemon-reload（D-06/D-07 单元变更后生效）。
func (g *Gateway) DaemonReload(ctx context.Context) error {
	_, err := run(ctx, "", verifyTimeout, "systemctl", "daemon-reload")
	return err
}

// ---- LogGateway（D-11：应用日志尾部）----

// Tail 查询服务单元日志尾部（journalctl -u {unit} -n {lines} --no-pager；
// 档位由 service 层白名单校验后传入）。
func (g *Gateway) Tail(ctx context.Context, unitName string, lines int) (string, error) {
	return run(ctx, "", LogTimeout, "journalctl",
		"-u", unitName, "-n", fmt.Sprintf("%d", lines), "--no-pager")
}

// ---- 内部辅助 ----

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, trimCR(s[start:i]))
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, trimCR(s[start:]))
	}
	return out
}

func trimCR(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\r' {
		return s[:len(s)-1]
	}
	return s
}

func copyFile(src, dst string) (int64, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return 0, err
	}
	return int64(len(data)), nil
}
