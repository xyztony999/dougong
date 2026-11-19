// NodeSource 三系代配测试（v1.3 Node 增量，prd FR-08①/⚠️A16；tasks.md
// A-DBT2③ 取证锚点）：os-release/文件写入/HTTP 全注入——源文件内容与支持
// 矩阵断言环境无关（不触达真实 /etc 与网络）。
package envpack

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
)

// TestEnsureNodeRepoApt apt 系 nodistro 统一代际源（官方 setup_22.x 取证形态）。
func TestEnsureNodeRepoApt(t *testing.T) {
	for _, rel := range []string{
		"ID=ubuntu\nVERSION_ID=\"24.04\"\nVERSION_CODENAME=noble\nID_LIKE=debian\n",
		"ID=debian\nVERSION_ID=\"12\"\nVERSION_CODENAME=bookworm\n",
	} {
		g, d := newVendorGW(rel)
		s, err := g.EnsureVendorRepo(context.Background(), "node", "apt")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		list, ok := d.written[nodeAptList]
		if !ok {
			t.Fatalf("list not written: %v (summary=%s)", d.written, s)
		}
		archOpt := "signed-by=" + nodeKeyPath
		switch runtime.GOARCH {
		case "amd64", "arm64":
			archOpt = "arch=" + runtime.GOARCH + " " + archOpt
		}
		for _, want := range []string{
			"deb [" + archOpt + "]",
			"https://deb.nodesource.com/node_22.x nodistro main",
		} {
			if !strings.Contains(list, want) {
				t.Fatalf("list missing %q: %s", want, list)
			}
		}
		if d.written[nodeKeyPath] != armoredKey {
			t.Fatal("nodesource key not written")
		}
		if len(d.httpHits) != 1 || d.httpHits[0] != nodeKeyURL {
			t.Fatalf("http hits = %v, want [%s]", d.httpHits, nodeKeyURL)
		}
		// 幂等：二次调用公钥复用不重复下载、源文件一致跳过。
		d.httpHits = nil
		if _, err := g.EnsureVendorRepo(context.Background(), "node", "apt"); err != nil {
			t.Fatalf("second call: %v", err)
		}
		if len(d.httpHits) != 0 {
			t.Fatalf("key re-downloaded on idempotent call: %v", d.httpHits)
		}
	}
}

// TestEnsureNodeRepoYum RHEL 系源（官方 setup_22.x 取证：nodistro/nodejs/
// $basearch + ns-operations-public.key + module_hotfixes）。
func TestEnsureNodeRepoYum(t *testing.T) {
	for _, family := range []string{"dnf", "yum"} {
		g, d := newVendorGW("ID=rocky\nVERSION_ID=\"9.4\"\nID_LIKE=\"rhel centos fedora\"\n")
		if _, err := g.EnsureVendorRepo(context.Background(), "node", family); err != nil {
			t.Fatalf("err: %v", err)
		}
		repo, ok := d.written[nodeYumRepo]
		if !ok {
			t.Fatalf("repo not written: %v", d.written)
		}
		for _, want := range []string{
			"[nodesource-nodejs]",
			"baseurl=https://rpm.nodesource.com/pub_22.x/nodistro/nodejs/$basearch",
			"gpgkey=https://rpm.nodesource.com/gpgkey/ns-operations-public.key",
			"gpgcheck=1",
			"module_hotfixes=1",
		} {
			if !strings.Contains(repo, want) {
				t.Fatalf("repo missing %q: %s", want, repo)
			}
		}
	}
}

// TestEnsureNodeRepoUnsupported 支持矩阵外组合：明确错误零文件写入。
func TestEnsureNodeRepoUnsupported(t *testing.T) {
	cases := []struct {
		name, osRelease, family string
	}{
		{"arch-linux", "ID=arch\n", "apt"},
		{"suse", "ID=opensuse-leap\nVERSION_ID=\"15.5\"\n", "dnf"},
		{"rhel7", "ID=centos\nVERSION_ID=\"7\"\nID_LIKE=\"rhel fedora\"\n", "yum"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, d := newVendorGW(tc.osRelease)
			if _, err := g.EnsureVendorRepo(context.Background(), "node", tc.family); err == nil {
				t.Fatal("want unsupported error")
			}
			if len(d.written) != 0 {
				t.Fatalf("unsupported combo wrote files: %v", d.written)
			}
		})
	}
}

// TestEnsureNodeRepoKeyInvalid 公钥内容非 armored（错页）→ 错误不落盘。
func TestEnsureNodeRepoKeyInvalid(t *testing.T) {
	g, d := newVendorGW("ID=ubuntu\nVERSION_CODENAME=noble\n")
	d.httpBody = "<html>error page</html>"
	if _, err := g.EnsureVendorRepo(context.Background(), "node", "apt"); err == nil {
		t.Fatal("want invalid key error")
	}
	if _, ok := d.written[nodeKeyPath]; ok {
		t.Fatal("invalid key written to disk")
	}
	if _, ok := d.written[nodeAptList]; ok {
		t.Fatal("list written despite key failure")
	}
}

// TestBinaryVersion node --version 参数数组（R-08）与二进制缺席如实。
func TestBinaryVersion(t *testing.T) {
	var gotArgs []string
	g := NewGatewayWithRunners(
		func(bin string) (string, error) {
			if bin == "node" {
				return "/usr/bin/node", nil
			}
			return "", errors.New("not found")
		},
		func(_ context.Context, _ map[string]string, name string, args []string) (string, error) {
			gotArgs = append([]string{name}, args...)
			return "v22.11.0\n", nil
		})
	v, err := g.BinaryVersion(context.Background(), "node")
	if err != nil || v != "v22.11.0" {
		t.Fatalf("v=%q err=%v", v, err)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "node" || gotArgs[1] != "--version" {
		t.Fatalf("argv = %v, want [node --version]（R-08 参数数组）", gotArgs)
	}
	if _, err := g.BinaryVersion(context.Background(), "absent-bin"); err == nil {
		t.Fatal("absent binary should error (version=null 如实)")
	}
}
