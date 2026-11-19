// Package toolx 是 db-tools 工具部署网关适配器（api-db-tools.md §4.1.5
// ToolDeployGateway 契约实现）：tarball 下载（Go net/http + 镜像拼接式前缀
// install.sh:38-45 同构）+ sha256 逐字节校验（D-05 强制，失配不安装）+
// tar 解压（stdlib archive/tar 逐条目 zip-slip 路径校验——零外部命令，
// R-08 by-absence，tasks.md A-DBT3）+ npm 全局安装/移除/根查询（R-08 字面量
// 参数数组；凭据不经 npm argv）+ php -m 扩展检测（D-06）+ 秘密文件 0600
// 原子写（nginxx WriteSecretFileAtomic 同构）。
//
// 全部外部依赖（探测/执行/下载/文件 IO）经注入点提供——测试环境无关
// （CI runner 不依赖 npm/php/网络实况，沿 envpack 全注入先例）。
package toolx

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Deps 是网关依赖注入组（nil 字段回退生产实现；测试全伪化）。
type Deps struct {
	LookPath   func(string) (string, error)
	RunCmd     func(ctx context.Context, env map[string]string, name string, args []string) (string, error)
	Fetch      func(ctx context.Context, url string) (io.ReadCloser, error)
	WriteFile  func(path string, data []byte, mode fs.FileMode) error
	MkdirAll   func(path string, mode fs.FileMode) error
	Remove     func(path string) error
	RemoveAll  func(path string) error
	Rename     func(old, new string) error
	CreateTemp func(dir, pattern string) (*os.File, error)
	ProbePort  func(port int) (bool, error)
}

// Gateway 是 ToolDeployGateway 适配器（并发安全：注入函数只读）。
type Gateway struct {
	lookPath   func(string) (string, error)
	runCmd     func(ctx context.Context, env map[string]string, name string, args []string) (string, error)
	fetch      func(ctx context.Context, url string) (io.ReadCloser, error)
	writeFile  func(string, []byte, fs.FileMode) error
	mkdirAll   func(string, fs.FileMode) error
	remove     func(string) error
	removeAll  func(string) error
	rename     func(old, new string) error
	createTemp func(dir, pattern string) (*os.File, error)
	probePort  func(port int) (bool, error)
}

// NewGateway 构造生产网关。
func NewGateway() *Gateway { return NewGatewayWithDeps(Deps{}) }

// NewGatewayWithDeps 以注入依赖构造（nil 字段回退生产实现）。
func NewGatewayWithDeps(d Deps) *Gateway {
	g := &Gateway{
		lookPath:   exec.LookPath,
		runCmd:     runCapture,
		fetch:      httpFetch,
		writeFile:  os.WriteFile,
		mkdirAll:   os.MkdirAll,
		remove:     os.Remove,
		removeAll:  os.RemoveAll,
		rename:     os.Rename,
		createTemp: os.CreateTemp,
		probePort:  probePortAvailable,
	}
	if d.LookPath != nil {
		g.lookPath = d.LookPath
	}
	if d.RunCmd != nil {
		g.runCmd = d.RunCmd
	}
	if d.Fetch != nil {
		g.fetch = d.Fetch
	}
	if d.WriteFile != nil {
		g.writeFile = d.WriteFile
	}
	if d.MkdirAll != nil {
		g.mkdirAll = d.MkdirAll
	}
	if d.Remove != nil {
		g.remove = d.Remove
	}
	if d.RemoveAll != nil {
		g.removeAll = d.RemoveAll
	}
	if d.Rename != nil {
		g.rename = d.Rename
	}
	if d.CreateTemp != nil {
		g.createTemp = d.CreateTemp
	}
	if d.ProbePort != nil {
		g.probePort = d.ProbePort
	}
	return g
}

// MirrorURL 镜像拼接式前缀（DOUGONG_MIRROR 同构，install.sh:38-45 先例：
// mirror 非空时 finalURL = mirror + "/" + 官方 URL；空=官方源直连）。
func MirrorURL(mirror, official string) string {
	m := strings.TrimRight(strings.TrimSpace(mirror), "/")
	if m == "" {
		return official
	}
	return m + "/" + official
}

// DownloadTarball 下载 tarball 到临时文件（D-05：DOWNLOAD_TIMEOUT 调用方
// 注入；流式落盘不全量进内存）。返回本地路径（调用方用毕清理）；失败清理
// 半成品文件。下载为用户主动发起 R-09 例外（与 ACME 同判）。
func (g *Gateway) DownloadTarball(ctx context.Context, url, mirror string, timeout time.Duration) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	finalURL := MirrorURL(mirror, url)
	body, err := g.fetch(cctx, finalURL)
	if err != nil {
		return "", fmt.Errorf("toolx: download %s: %w", finalURL, err)
	}
	defer func() { _ = body.Close() }() //nolint:errcheck // teardown 弃错：读取完成后关闭（Gate 2 I5）
	f, err := g.createTemp("", "dougong-tool-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("toolx: download temp create: %w", err)
	}
	tmpPath := f.Name()
	if _, err := io.Copy(f, body); err != nil {
		_ = f.Close()         //nolint:errcheck // 失败路径清理
		_ = g.remove(tmpPath) //nolint:errcheck // 半成品清理（D-05）
		return "", fmt.Errorf("toolx: download write %s: %w", tmpPath, err)
	}
	if err := f.Close(); err != nil {
		_ = g.remove(tmpPath) //nolint:errcheck // 半成品清理
		return "", fmt.Errorf("toolx: download close %s: %w", tmpPath, err)
	}
	return tmpPath, nil
}

// VerifySha256 逐字节 sha256 校验（D-05 强制：失配=任务 failed
// sha256_mismatch，不降级不跳过不安装）。expected 为 hex 小写/大写均可。
func (g *Gateway) VerifySha256(localPath, expected string) (bool, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return false, fmt.Errorf("toolx: sha256 open %s: %w", localPath, err)
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // teardown 弃错（Gate 2 I5）
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, fmt.Errorf("toolx: sha256 read %s: %w", localPath, err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	want := strings.ToLower(strings.TrimSpace(expected))
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1, nil
}

// ExtractTar 解压 .tar.gz 到 destDir（A-DBT3：stdlib 逐条目 + zip-slip 防线
// 沿 FILE_009 语义——绝对路径/.. 分量/前缀逃逸一律拒绝；链接条目拒绝〔钉死
// tarball 仅含常规文件/目录〕）。全部条目共享单一顶层目录时剥离之
// （phpMyAdmin-5.2.2-all-languages/ → destDir 直落）。
func (g *Gateway) ExtractTar(localPath, destDir string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("toolx: extract open %s: %w", localPath, err)
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // teardown 弃错（Gate 2 I5）
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("toolx: extract gzip %s: %w", localPath, err)
	}
	defer func() { _ = gz.Close() }() //nolint:errcheck // teardown 弃错（Gate 2 I5）
	// 第一遍：判定共享顶层目录（剥离决策）。
	names, err := tarEntryNames(gz)
	if err != nil {
		return err
	}
	strip := commonRoot(names)
	// 第二遍：重新打开逐条目解压（gzip 流不可 rewind）。
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("toolx: extract rewind: %w", err)
	}
	gz2, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("toolx: extract gzip reopen: %w", err)
	}
	defer func() { _ = gz2.Close() }() //nolint:errcheck // teardown 弃错（Gate 2 I5）
	if err := g.mkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("toolx: extract mkdir %s: %w", destDir, err)
	}
	root := filepath.Clean(destDir)
	tr := tar.NewReader(gz2)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("toolx: extract read: %w", err)
		}
		rel, err := safeRelPath(hdr.Name, strip)
		if err != nil {
			return err
		}
		if rel == "" {
			continue // 顶层目录自身条目
		}
		target := filepath.Join(root, rel)
		// zip-slip 前缀白名单（Clean 后仍须落在 destDir 子树）。
		if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return fmt.Errorf("toolx: extract path escape rejected: %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := g.mkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("toolx: extract mkdir %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := g.mkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("toolx: extract mkdir parent %s: %w", target, err)
			}
			if err := writeTarFile(target, tr); err != nil {
				return err
			}
		default:
			// 链接/设备/非常规条目拒绝（钉死官方 tarball 不应包含；保守失败）。
			return fmt.Errorf("toolx: extract unsupported entry type %d: %q", hdr.Typeflag, hdr.Name)
		}
	}
	return nil
}

// tarEntryNames 收集全部条目名（第一遍扫描）。
func tarEntryNames(gz io.Reader) ([]string, error) {
	tr := tar.NewReader(gz)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return names, nil
		}
		if err != nil {
			return nil, fmt.Errorf("toolx: extract scan: %w", err)
		}
		names = append(names, hdr.Name)
	}
}

// commonRoot 全部条目共享单一顶层目录时返回该目录名（否则 ""=不剥离）。
func commonRoot(names []string) string {
	root := ""
	for _, n := range names {
		n = strings.TrimPrefix(filepath.ToSlash(n), "./")
		first, _, _ := strings.Cut(n, "/")
		if first == "" || first == "." {
			return ""
		}
		if root == "" {
			root = first
		} else if first != root {
			return ""
		}
	}
	return root
}

// safeRelPath 条目名 → 安全相对路径（zip-slip 防线：绝对路径/.. 分量拒绝；
// 剥离共享顶层目录；Clean 归一）。返回 ""=顶层目录自身（跳过）。
func safeRelPath(name, strip string) (string, error) {
	slashed := filepath.ToSlash(name)
	if strings.HasPrefix(slashed, "/") {
		return "", fmt.Errorf("toolx: extract absolute path rejected: %q", name)
	}
	slashed = strings.TrimPrefix(slashed, "./")
	if strip != "" {
		if slashed == strip || slashed == strip+"/" {
			return "", nil
		}
		if !strings.HasPrefix(slashed, strip+"/") {
			return "", fmt.Errorf("toolx: extract entry outside common root: %q", name)
		}
		slashed = strings.TrimPrefix(slashed, strip+"/")
	}
	for _, part := range strings.Split(slashed, "/") {
		if part == ".." {
			return "", fmt.Errorf("toolx: extract path traversal rejected: %q", name)
		}
	}
	cleaned := filepath.Clean(filepath.FromSlash(slashed))
	if cleaned == "." || cleaned == "" {
		return "", nil
	}
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("toolx: extract path escape rejected: %q", name)
	}
	return cleaned, nil
}

// writeTarFile 单文件落盘（0644；内容已经 sha256 校验链信任）。
func writeTarFile(target string, r io.Reader) error {
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("toolx: extract create %s: %w", target, err)
	}
	if _, err := io.Copy(out, r); err != nil {
		_ = out.Close() //nolint:errcheck // 失败路径清理
		return fmt.Errorf("toolx: extract write %s: %w", target, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("toolx: extract close %s: %w", target, err)
	}
	return nil
}

// NpmInstall npm 全局精确版本安装（D-05/D-07：`npm install --global
// pkg@version` 参数数组 R-08；registry 镜像选项 --registry 同构；凭据不经
// argv——Mongo 凭据经 systemd Environment 注入，D-07）。
func (g *Gateway) NpmInstall(ctx context.Context, pkg, version, registry string, timeout time.Duration) (string, error) {
	if _, err := g.lookPath("npm"); err != nil {
		return "", fmt.Errorf("toolx: npm not found: %w", err)
	}
	args := []string{"install", "--global", pkg + "@" + version}
	if registry != "" {
		args = append(args, "--registry", registry)
	}
	return g.run(ctx, timeout, "npm", args)
}

// NpmRemove npm 全局移除（卸载执行链，D-15）。
func (g *Gateway) NpmRemove(ctx context.Context, pkg string, timeout time.Duration) (string, error) {
	if _, err := g.lookPath("npm"); err != nil {
		return "", fmt.Errorf("toolx: npm not found: %w", err)
	}
	return g.run(ctx, timeout, "npm", []string{"uninstall", "--global", pkg})
}

// NpmGlobalRoot npm 全局根查询（systemd 单元 ExecStart 路径构造，D-07：
// `npm root -g` 参数数组）。
func (g *Gateway) NpmGlobalRoot(ctx context.Context) (string, error) {
	if _, err := g.lookPath("npm"); err != nil {
		return "", fmt.Errorf("toolx: npm not found: %w", err)
	}
	out, err := g.run(ctx, 30*time.Second, "npm", []string{"root", "-g"})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// PhpExtensions php -m 扩展清单（D-06 必需扩展比对数据源；R-08 参数数组；
// 输出小写归一，段标题/空行剔除）。php 二进制缺席=错误（调用方按缺失处理）。
func (g *Gateway) PhpExtensions(ctx context.Context) ([]string, error) {
	if _, err := g.lookPath("php"); err != nil {
		return nil, fmt.Errorf("toolx: php not found: %w", err)
	}
	out, err := g.run(ctx, 30*time.Second, "php", []string{"-m"})
	if err != nil {
		return nil, err
	}
	var exts []string
	for _, line := range strings.Split(out, "\n") {
		l := strings.ToLower(strings.TrimSpace(line))
		if l == "" || strings.HasPrefix(l, "[") {
			continue
		}
		exts = append(exts, l)
	}
	return exts, nil
}

// WriteSecretFile 原子写敏感文件（0600：同目录临时文件 + chmod + rename，
// nginxx gateway WriteSecretFileAtomic / dbmgmt vault 同构机制）。
func (g *Gateway) WriteSecretFile(path string, content []byte) error {
	return g.writeAtomic(path, content, 0o600)
}

// WriteFile 原子写常规文件（指定权限——config.inc.php/fastcgi.conf 0644
// PHP-FPM worker 可读，tasks.md A-DBT5 分级）。
func (g *Gateway) WriteFile(path string, content []byte, mode fs.FileMode) error {
	return g.writeAtomic(path, content, mode)
}

func (g *Gateway) writeAtomic(path string, content []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := g.mkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("toolx: mkdir %s: %w", dir, err)
	}
	tmp, err := g.createTemp(dir, ".dougong-tool-*")
	if err != nil {
		return fmt.Errorf("toolx: temp create in %s: %w", dir, err)
	}
	name := tmp.Name()
	defer func() { _ = g.remove(name) }() //nolint:errcheck // rename 成功后 no-op；失败兜底清理
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close() //nolint:errcheck // 失败路径清理
		return fmt.Errorf("toolx: write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("toolx: close %s: %w", name, err)
	}
	if err := os.Chmod(name, mode); err != nil {
		return fmt.Errorf("toolx: chmod %s: %w", name, err)
	}
	if err := g.rename(name, path); err != nil {
		return fmt.Errorf("toolx: rename %s → %s: %w", name, path, err)
	}
	return nil
}

// ReadFile 读取文件内容（轮换回滚基线/秘密文件读取；不存在如实报错）。
func (g *Gateway) ReadFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("toolx: read %s: %w", path, err)
	}
	return b, nil
}

// MkdirAll 递归建目录（工具管理目录创建）。
func (g *Gateway) MkdirAll(path string, mode fs.FileMode) error {
	if err := g.mkdirAll(path, mode); err != nil {
		return fmt.Errorf("toolx: mkdir %s: %w", path, err)
	}
	return nil
}

// Remove 删除单文件（秘密文件撤除/下载清理）。
func (g *Gateway) Remove(path string) error {
	if err := g.remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("toolx: remove %s: %w", path, err)
	}
	return nil
}

// RemoveAll 删除目录树（卸载程序目录——调用方白名单限定工具目录，I-C03）。
func (g *Gateway) RemoveAll(path string) error {
	if err := g.removeAll(path); err != nil {
		return fmt.Errorf("toolx: remove all %s: %w", path, err)
	}
	return nil
}

// Rename 目录改名（升级 staging 替换，D-14）。
func (g *Gateway) Rename(old, new string) error {
	if err := g.rename(old, new); err != nil {
		return fmt.Errorf("toolx: rename %s → %s: %w", old, new, err)
	}
	return nil
}

// ProbePortAvailable 端口占用探针（v1.3，D-19③：net.Listen("tcp", :port)
// 即探即放——占用→false 如实；探针通过不保证 nginx 绑定成功，nginx -t/
// reload 失败仍沿 R-06 回滚兜底）。
func (g *Gateway) ProbePortAvailable(port int) (bool, error) {
	return g.probePort(port)
}

// probePortAvailable 生产探针实现（绑定失败=占用或不可绑，均按不可用如实；
// 成功即关闭释放——不持有端口）。
func probePortAvailable(port int) (bool, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port)))
	if err != nil {
		return false, nil
	}
	if cerr := ln.Close(); cerr != nil {
		return false, fmt.Errorf("toolx: probe port close: %w", cerr)
	}
	return true, nil
}

// run 执行外部命令（字面量参数数组 R-08；超时调用方注入；stdout+stderr 合并）。
func (g *Gateway) run(ctx context.Context, timeout time.Duration, name string, args []string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := g.runCmd(cctx, nil, name, args)
	if err != nil {
		return out, fmt.Errorf("toolx: %s: %w", name, err)
	}
	return out, nil
}

func runCapture(ctx context.Context, env map[string]string, name string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) // R-08：参数数组，不经 shell
	var buf strings.Builder
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if len(env) > 0 {
		e := cmd.Environ()
		for k, v := range env {
			e = append(e, k+"="+v)
		}
		cmd.Env = e
	}
	err := cmd.Run()
	out := buf.String()
	if err != nil {
		if ctx.Err() != nil {
			return out, fmt.Errorf("toolx: %s: timeout: %w (cause: %v)", name, ctx.Err(), err)
		}
		return out, fmt.Errorf("toolx: %s: %w", name, err)
	}
	return out, nil
}

// httpFetch 生产下载通道（http.NewRequestWithContext；状态码非 200 如实报错）。
func httpFetch(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("toolx: request %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("toolx: fetch %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close() //nolint:errcheck // 错误路径关闭
		return nil, fmt.Errorf("toolx: fetch %s: HTTP %d", url, resp.StatusCode)
	}
	return resp.Body, nil
}
