// EnsureVendorRepo（D-06 v1.1 代配补真）测试：os-release/文件写入/HTTP 全
// 注入，断言支持矩阵判定与源文件内容——不触达真实 /etc 与网络（环境无关）。
package envpack

import (
	"context"
	"errors"
	"io/fs"
	"runtime"
	"strings"
	"testing"
)

const armoredKey = "-----BEGIN PGP PUBLIC KEY BLOCK-----\nmQINBF...\n-----END PGP PUBLIC KEY BLOCK-----\n"

type vendorDeps struct {
	files    map[string]string // 预置文件（os-release / 已存在公钥）
	written  map[string]string // 写入捕获
	httpHits []string          // 下载 URL 捕获
	httpBody string
	httpErr  error
}

func newVendorGW(osRelease string) (*Gateway, *vendorDeps) {
	d := &vendorDeps{files: map[string]string{"/etc/os-release": osRelease},
		written: map[string]string{}, httpBody: armoredKey}
	g := newGateway()
	g.readFile = func(p string) ([]byte, error) {
		if s, ok := d.files[p]; ok {
			return []byte(s), nil
		}
		return nil, errors.New("not found")
	}
	g.writeFile = func(p string, b []byte, _ fs.FileMode) error {
		d.written[p] = string(b)
		d.files[p] = string(b) // 模拟真实文件系统：写入后 readFile 可见（幂等断言依赖）
		return nil
	}
	g.mkdirAll = func(string, fs.FileMode) error { return nil }
	g.httpGet = func(_ context.Context, url string) ([]byte, error) {
		d.httpHits = append(d.httpHits, url)
		if d.httpErr != nil {
			return nil, d.httpErr
		}
		return []byte(d.httpBody), nil
	}
	return g, d
}

func TestEnsureVendorRepoAptUbuntu(t *testing.T) {
	g, d := newVendorGW(`ID=ubuntu
VERSION_ID="22.04"
VERSION_CODENAME=jammy
ID_LIKE=debian
`)
	s, err := g.EnsureVendorRepo(context.Background(), "mongodb", "apt")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	list, ok := d.written[mongodbAptList]
	if !ok {
		t.Fatalf("list not written: %v (summary=%s)", d.written, s)
	}
	archOpt := "signed-by=/usr/share/keyrings/mongodb-server-8.0.asc"
	switch runtime.GOARCH { // 与实现同源：仅 amd64/arm64 限定 arch，其余不限定
	case "amd64", "arm64":
		// v0.1.3b：多选项空格分隔（逗号会把 signed-by 并入 arch 值被 apt 静默丢弃）
		archOpt = "arch=" + runtime.GOARCH + " " + archOpt
	}
	for _, want := range []string{
		"deb [" + archOpt + "]",
		"https://repo.mongodb.org/apt/ubuntu jammy/mongodb-org/8.0 multiverse",
	} {
		if !strings.Contains(list, want) {
			t.Fatalf("list missing %q: %s", want, list)
		}
	}
	if d.written[mongodbKeyPath] != armoredKey {
		t.Fatalf("key not written")
	}
	if len(d.httpHits) != 1 || !strings.HasPrefix(d.httpHits[0], "https://www.mongodb.org/") {
		t.Fatalf("http hits = %v", d.httpHits)
	}
}

func TestEnsureVendorRepoAptDebian(t *testing.T) {
	g, d := newVendorGW(`ID=debian
VERSION_ID="12"
VERSION_CODENAME=bookworm
`)
	if _, err := g.EnsureVendorRepo(context.Background(), "mongodb", "apt"); err != nil {
		t.Fatalf("err: %v", err)
	}
	list := d.written[mongodbAptList]
	if !strings.Contains(list, "https://repo.mongodb.org/apt/debian bookworm/mongodb-org/8.0 main\n") {
		t.Fatalf("debian line wrong: %s", list)
	}
	if strings.Contains(list, "multiverse") {
		t.Fatalf("debian must not use multiverse component: %s", list)
	}
	if strings.Contains(list, "main main") {
		t.Fatalf("debian must not append a second main component: %s", list)
	}
}

func TestEnsureVendorRepoUnsupported(t *testing.T) {
	cases := []struct{ name, rel string }{
		{"ubuntu 不支持代际", "ID=ubuntu\nVERSION_CODENAME=bionic\n"},
		{"debian 不支持代际", "ID=debian\nVERSION_CODENAME=trixie\n"},
		{"未知发行版", "ID=alpine\nVERSION_ID=3.19\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, d := newVendorGW(tc.rel)
			if _, err := g.EnsureVendorRepo(context.Background(), "mongodb", "apt"); err == nil {
				t.Fatalf("want unsupported error")
			}
			if len(d.written) != 0 {
				t.Fatalf("unsupported must write nothing, wrote %v", d.written)
			}
		})
	}
	// RHEL 系支持矩阵外
	for _, rel := range []string{
		"ID=fedora\nVERSION_ID=41\n",
		"ID=rocky\nVERSION_ID=10.0\n",
	} {
		g, d := newVendorGW(rel)
		if _, err := g.EnsureVendorRepo(context.Background(), "mongodb", "dnf"); err == nil {
			t.Fatalf("want unsupported error for %s", rel)
		}
		if _, err := g.EnsureVendorRepo(context.Background(), "docker", "yum"); err == nil {
			t.Fatalf("want unsupported docker error for %s", rel)
		}
		if len(d.written) != 0 {
			t.Fatalf("unsupported must write nothing")
		}
	}
}

func TestEnsureVendorRepoYumMongoAndDocker(t *testing.T) {
	g, d := newVendorGW(`NAME="Rocky Linux"
ID="rocky"
VERSION_ID="9.4"
ID_LIKE="rhel centos"
`)
	s, err := g.EnsureVendorRepo(context.Background(), "mongodb", "dnf")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	repo := d.written[mongodbYumRepo]
	for _, want := range []string{
		"[mongodb-org-8.0]",
		"baseurl=https://repo.mongodb.org/yum/redhat/$releasever/mongodb-org/8.0/$basearch/",
		"gpgkey=https://www.mongodb.org/static/pgp/server-8.0.asc",
	} {
		if !strings.Contains(repo, want) {
			t.Fatalf("mongo repo missing %q: %s", want, repo)
		}
	}
	if strings.Contains(s, "失败") {
		t.Fatalf("unexpected failure summary: %s", s)
	}
	if _, err := g.EnsureVendorRepo(context.Background(), "docker", "yum"); err != nil {
		t.Fatalf("docker err: %v", err)
	}
	drepo := d.written[dockerCEYumRepo]
	if !strings.Contains(drepo, "baseurl=https://download.docker.com/linux/centos/$releasever/$basearch/stable") {
		t.Fatalf("docker repo wrong: %s", drepo)
	}
	if len(d.httpHits) != 0 {
		t.Fatalf("yum 路径不应下载公钥（gpgkey 内联）: %v", d.httpHits)
	}
}

func TestEnsureVendorRepoIdempotent(t *testing.T) {
	g, d := newVendorGW("ID=ubuntu\nVERSION_CODENAME=noble\n")
	if _, err := g.EnsureVendorRepo(context.Background(), "mongodb", "apt"); err != nil {
		t.Fatal(err)
	}
	// 二次执行：公钥已存在复用（不重复下载），list 内容一致跳过。
	firstHits := len(d.httpHits)
	s, err := g.EnsureVendorRepo(context.Background(), "mongodb", "apt")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.httpHits) != firstHits {
		t.Fatalf("key re-downloaded on second run")
	}
	if !strings.Contains(s, "已存在") {
		t.Fatalf("idempotent summary wrong: %s", s)
	}
}

func TestEnsureVendorRepoKeyValidation(t *testing.T) {
	g, d := newVendorGW("ID=ubuntu\nVERSION_CODENAME=jammy\n")
	d.httpBody = "<html>404-ish page</html>" // 错页防落盘
	if _, err := g.EnsureVendorRepo(context.Background(), "mongodb", "apt"); err == nil {
		t.Fatalf("want key validation error")
	}
	if len(d.written) != 0 {
		t.Fatalf("invalid key must write nothing, wrote %v", d.written)
	}
}

func TestEnsureVendorRepoNoop(t *testing.T) {
	g, _ := newVendorGW("ID=ubuntu\nVERSION_CODENAME=jammy\n")
	s, err := g.EnsureVendorRepo(context.Background(), "nginx", "apt")
	if err != nil || !strings.Contains(s, "无需第三方源") {
		t.Fatalf("noop: s=%q err=%v", s, err)
	}
}
