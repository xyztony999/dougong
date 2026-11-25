package envpack

import (
	"context"
	"strings"
	"testing"
)

func TestParseAptMadison(t *testing.T) {
	out := `
 nginx | 1.24.0-2ubuntu7.5 | http://archive.ubuntu.com/ubuntu noble-updates/main amd64 Packages
 nginx | 1.24.0-2ubuntu7.5 | http://security.ubuntu.com/ubuntu noble-security/main amd64 Packages
 nginx | 1.24.0-2ubuntu7 | http://archive.ubuntu.com/ubuntu noble/main amd64 Packages
`
	got := parseAptMadison(out)
	want := []string{"1.24.0-2ubuntu7.5", "1.24.0-2ubuntu7"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestParseRpmList(t *testing.T) {
	out := `Available Packages
nginx.x86_64                    1:1.24.0-1.el9                  appstream
nginx.x86_64                    1.22.1-1.el9                    appstream
httpd.x86_64                    2.4.57-1.el9                    appstream
`
	got := parseRpmList(out, "nginx")
	want := []string{"1.24.0-1.el9", "1.22.1-1.el9"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestValidPkgVersion(t *testing.T) {
	ok := []string{"1.24.0-2ubuntu7.5", "8.0.36", "1:not", "22.11.0"}
	ok[2] = "1.24.0+incompatible"
	for _, v := range []string{"1.24.0-2ubuntu7.5", "8.0.36", "1.24.0+incompatible", "22.11.0"} {
		if !ValidPkgVersion(v) {
			t.Fatalf("%q should be valid", v)
		}
	}
	for _, v := range []string{"", "1.0;rm -rf", "foo bar", "../etc"} {
		if ValidPkgVersion(v) {
			t.Fatalf("%q should be invalid", v)
		}
	}
	_ = ok
}

func TestInstallAtVersionAptArgv(t *testing.T) {
	var sawName string
	var sawArgs []string
	g := NewGatewayWithRunners(nil, func(_ context.Context, _ map[string]string, name string, args []string) (string, error) {
		sawName, sawArgs = name, append([]string(nil), args...)
		return "ok", nil
	})
	if _, err := g.InstallAtVersion(context.Background(), "apt", "nginx", "1.24.0-1"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if sawName != "apt-get" {
		t.Fatalf("bin = %s", sawName)
	}
	joined := strings.Join(sawArgs, " ")
	if !strings.Contains(joined, "install nginx=1.24.0-1") {
		t.Fatalf("argv = %v", sawArgs)
	}
	if _, err := g.InstallAtVersion(context.Background(), "apt", "nginx", "1.0;bad"); err == nil {
		t.Fatal("invalid version must be rejected")
	}
}
