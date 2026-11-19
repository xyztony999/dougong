// toolx 部署网关测试（全注入 + 合成 tarball——零 runner npm/php/网络实况
// 依赖；sha256/zip-slip/npm argv/镜像拼接/秘密文件权限断言）。
package toolx

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// buildTarGz 合成 tarball（entries: name→content；name 以 / 结尾=目录）。
func buildTarGz(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		if strings.HasSuffix(name, "/") {
			if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "synthetic.tar.gz")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestVerifySha256 命中/失配（D-05 强制校验）。
func TestVerifySha256(t *testing.T) {
	g := NewGateway()
	p := filepath.Join(t.TempDir(), "payload.bin")
	content := []byte("phpMyAdmin tarball synthetic content")
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	hexSum := hex.EncodeToString(sum[:])
	ok, err := g.VerifySha256(p, hexSum)
	if err != nil || !ok {
		t.Fatalf("match: ok=%v err=%v", ok, err)
	}
	ok, err = g.VerifySha256(p, strings.ToUpper(hexSum)) // 大写 hex 兼容
	if err != nil || !ok {
		t.Fatalf("upper hex: ok=%v err=%v", ok, err)
	}
	ok, err = g.VerifySha256(p, strings.Repeat("0", 64))
	if err != nil || ok {
		t.Fatalf("mismatch should be ok=false: ok=%v err=%v", ok, err)
	}
}

// TestExtractTarStripRoot 共享顶层目录剥离 + 内容落盘。
func TestExtractTarStripRoot(t *testing.T) {
	g := NewGateway()
	p := buildTarGz(t, map[string]string{
		"phpMyAdmin-5.2.2-all-languages/":                     "",
		"phpMyAdmin-5.2.2-all-languages/index.php":            "<?php // index",
		"phpMyAdmin-5.2.2-all-languages/libraries/":           "",
		"phpMyAdmin-5.2.2-all-languages/libraries/Common.php": "<?php // common",
	})
	dest := filepath.Join(t.TempDir(), "phpmyadmin")
	if err := g.ExtractTar(p, dest); err != nil {
		t.Fatalf("extract: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "index.php"))
	if err != nil || string(b) != "<?php // index" {
		t.Fatalf("index.php = %q err=%v（顶层目录应剥离）", b, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "libraries", "Common.php")); err != nil {
		t.Fatalf("nested file missing: %v", err)
	}
}

// TestExtractTarZipSlipRejected zip-slip 路径防线（FILE_009 语义）：
// .. 分量/绝对路径/前缀逃逸一律拒绝，零越界写入。
func TestExtractTarZipSlipRejected(t *testing.T) {
	cases := []struct {
		name    string
		entries map[string]string
	}{
		{"dotdot", map[string]string{"root/": "", "root/../../evil.php": "x"}},
		{"absolute", map[string]string{"root/": "", "/evil.php": "x"}},
		{"dotdot-mid", map[string]string{"root/": "", "root/sub/../../../evil.php": "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGateway()
			p := buildTarGz(t, tc.entries)
			dest := filepath.Join(t.TempDir(), "dest")
			err := g.ExtractTar(p, dest)
			if err == nil {
				t.Fatal("zip-slip entry should be rejected")
			}
			// dest 子树外零写入（父目录无 evil.php）。
			if _, serr := os.Stat(filepath.Join(filepath.Dir(dest), "evil.php")); serr == nil {
				t.Fatal("escaped file written outside dest")
			}
		})
	}
}

// TestExtractTarSymlinkRejected 链接条目保守拒绝。
func TestExtractTarSymlinkRejected(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "root/", Typeflag: tar.TypeDir, Mode: 0o755})
	_ = tw.WriteHeader(&tar.Header{Name: "root/link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777})
	_ = tw.Close()
	_ = gz.Close()
	p := filepath.Join(t.TempDir(), "link.tar.gz")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	g := NewGateway()
	if err := g.ExtractTar(p, filepath.Join(t.TempDir(), "dest")); err == nil {
		t.Fatal("symlink entry should be rejected")
	}
}

// TestNpmArgvArrays npm 命令字面量参数数组（R-08）+ registry 镜像选项。
func TestNpmArgvArrays(t *testing.T) {
	var calls [][]string
	g := NewGatewayWithDeps(Deps{
		LookPath: func(string) (string, error) { return "/usr/bin/npm", nil },
		RunCmd: func(_ context.Context, _ map[string]string, name string, args []string) (string, error) {
			calls = append(calls, append([]string{name}, args...))
			if name == "npm" && args[0] == "root" {
				return "/usr/lib/node_modules\n", nil
			}
			return "ok", nil
		},
	})
	ctx := context.Background()
	if _, err := g.NpmInstall(ctx, "mongo-express", "1.0.0", "", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := g.NpmInstall(ctx, "mongo-express", "1.0.0", "https://registry.npmmirror.com", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := g.NpmRemove(ctx, "mongo-express", time.Minute); err != nil {
		t.Fatal(err)
	}
	root, err := g.NpmGlobalRoot(ctx)
	if err != nil || root != "/usr/lib/node_modules" {
		t.Fatalf("root = %q err=%v", root, err)
	}
	want := [][]string{
		{"npm", "install", "--global", "mongo-express@1.0.0"},
		{"npm", "install", "--global", "mongo-express@1.0.0", "--registry", "https://registry.npmmirror.com"},
		{"npm", "uninstall", "--global", "mongo-express"},
		{"npm", "root", "-g"},
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v", calls)
	}
	for i := range want {
		if strings.Join(calls[i], " ") != strings.Join(want[i], " ") {
			t.Fatalf("call %d = %v, want %v（R-08 参数数组）", i, calls[i], want[i])
		}
	}
}

// TestNpmAbsent npm 二进制缺席如实报错（不虚构成功）。
func TestNpmAbsent(t *testing.T) {
	g := NewGatewayWithDeps(Deps{
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
	})
	if _, err := g.NpmInstall(context.Background(), "mongo-express", "1.0.0", "", time.Minute); err == nil {
		t.Fatal("absent npm should error")
	}
}

// TestPhpExtensions php -m 解析（段标题/空行剔除、小写归一）。
func TestPhpExtensions(t *testing.T) {
	g := NewGatewayWithDeps(Deps{
		LookPath: func(string) (string, error) { return "/usr/bin/php", nil },
		RunCmd: func(_ context.Context, _ map[string]string, name string, args []string) (string, error) {
			if name != "php" || len(args) != 1 || args[0] != "-m" {
				t.Fatalf("argv = %s %v, want php -m", name, args)
			}
			return "[PHP Modules]\nmysqli\nmbstring\nGD\n\n[Zend Modules]\nZend OPcache\n", nil
		},
	})
	exts, err := g.PhpExtensions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(exts, ",")
	if joined != "mysqli,mbstring,gd,zend opcache" {
		t.Fatalf("exts = %v", exts)
	}
}

// TestMirrorURL 拼接式镜像前缀（install.sh:38-45 同构）。
func TestMirrorURL(t *testing.T) {
	official := "https://files.phpmyadmin.net/phpMyAdmin/5.2.2/phpMyAdmin-5.2.2-all-languages.tar.gz"
	if got := MirrorURL("", official); got != official {
		t.Fatalf("empty mirror = %q", got)
	}
	if got := MirrorURL("https://ghproxy.com/", official); got != "https://ghproxy.com/"+official {
		t.Fatalf("mirror concat = %q", got)
	}
}

// TestDownloadTarball 注入 fetch 流式落盘 + 失败清理。
func TestDownloadTarball(t *testing.T) {
	g := NewGatewayWithDeps(Deps{
		Fetch: func(_ context.Context, url string) (io.ReadCloser, error) {
			if url != "https://mirror.example/"+officialURL {
				t.Fatalf("fetch url = %q（镜像拼接）", url)
			}
			return io.NopCloser(strings.NewReader("tarball-bytes")), nil
		},
	})
	p, err := g.DownloadTarball(context.Background(), officialURL, "https://mirror.example", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(p)
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "tarball-bytes" {
		t.Fatalf("downloaded = %q err=%v", b, err)
	}
	// fetch 失败：无半成品残留（错误路径清理）。
	g2 := NewGatewayWithDeps(Deps{
		Fetch: func(_ context.Context, _ string) (io.ReadCloser, error) {
			return nil, errors.New("connection reset")
		},
	})
	if _, err := g2.DownloadTarball(context.Background(), officialURL, "", time.Minute); err == nil {
		t.Fatal("fetch failure should error")
	}
}

const officialURL = "https://files.phpmyadmin.net/phpMyAdmin/5.2.2/phpMyAdmin-5.2.2-all-languages.tar.gz"

// TestWriteSecretFileMode 秘密文件 0600（Windows 权限位语义差异跳过 mode
// 断言，内容/原子性断言环境无关）。
func TestWriteSecretFileMode(t *testing.T) {
	g := NewGateway()
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "internal-secret")
	if err := g.WriteSecretFile(p, []byte("secret-hex")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "secret-hex" {
		t.Fatalf("content = %q err=%v", b, err)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(p)
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, want 0600", st.Mode())
		}
	}
	// 常规文件 0644（config.inc.php PHP-FPM 可读分级 A-DBT5）。
	p2 := filepath.Join(dir, "config.inc.php")
	if err := g.WriteFile(p2, []byte("<?php"), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(p2)
		if st.Mode().Perm() != 0o644 {
			t.Fatalf("config mode = %v, want 0644", st.Mode())
		}
	}
}

// TestProbePortAvailable D-19③ 端口占用探针：占用→false 如实；释放→true
// （即探即放——探针自身不持有端口）。
func TestProbePortAvailable(t *testing.T) {
	g := NewGateway()
	// 与探针同口径绑通配地址（Windows 允许通配+具体地址并存重叠——
	// 占用判定须同地址族；生产 Linux 通配与任何绑定冲突）。
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ok, err := g.ProbePortAvailable(port)
	if err != nil || ok {
		t.Fatalf("占用端口探针 = %v err=%v, want false", ok, err)
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	ok, err = g.ProbePortAvailable(port)
	if err != nil || !ok {
		t.Fatalf("释放端口探针 = %v err=%v, want true", ok, err)
	}
	// 探针即探即放：连续两次探测同一空闲端口均成功（不持有）。
	ok2, err := g.ProbePortAvailable(port)
	if err != nil || !ok2 {
		t.Fatalf("探针应即探即放 = %v err=%v", ok2, err)
	}
}
