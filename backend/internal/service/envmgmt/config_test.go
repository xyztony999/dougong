package envmgmt

import (
	"context"
	"io/fs"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/dougong/dougong/internal/adapter/envpack"
	"github.com/dougong/dougong/internal/adapter/sysd"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storeenv "github.com/dougong/dougong/internal/store/envmgmt"
)

func TestCaptureValue(t *testing.T) {
	src := "user www-data;\nworker_processes auto;\nevents {\n    worker_connections 768;\n}\n"
	re := regexp.MustCompile(`(?m)^(\s*)worker_processes\s+\S+;`)
	if got := captureValue(re, src); got != "auto" {
		t.Fatalf("capture worker_processes = %q", got)
	}
	re2 := regexp.MustCompile(`(?m)^(\s*)worker_connections\s+\S+;`)
	if got := captureValue(re2, src); got != "768" {
		t.Fatalf("capture worker_connections = %q", got)
	}
}

func TestCaptureApacheKeepAlive(t *testing.T) {
	src := "Timeout 300\nKeepAlive On\nMaxKeepAliveRequests 100\nKeepAliveTimeout 5\n"
	re := regexp.MustCompile(`(?m)^(\s*)KeepAlive\s+\S+`)
	if got := captureValue(re, src); got != "On" {
		t.Fatalf("KeepAlive = %q", got)
	}
}

func TestSanitizeSettingValue(t *testing.T) {
	if _, err := sanitizeSettingValue("64m"); err != nil {
		t.Fatal(err)
	}
	if _, err := sanitizeSettingValue("0.0.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := sanitizeSettingValue("auto"); err != nil {
		t.Fatal(err)
	}
	if _, err := sanitizeSettingValue("1; rm"); err == nil {
		t.Fatal("must reject")
	}
}

func TestSettingsSaveNginxRoundTrip(t *testing.T) {
	files := map[string]string{
		"/etc/nginx/nginx.conf": "user www-data;\nworker_processes auto;\nevents {\n    worker_connections 768;\n}\nhttp {\n    gzip off;\n}\n",
	}
	var mu sync.Mutex
	var ran []string
	cfs := &ConfigFS{
		Exists: func(p string) bool {
			mu.Lock()
			defer mu.Unlock()
			_, ok := files[p]
			return ok
		},
		ReadFile: func(p string) ([]byte, error) {
			mu.Lock()
			defer mu.Unlock()
			return []byte(files[p]), nil
		},
		WriteFile: func(p string, b []byte, _ fs.FileMode) error {
			mu.Lock()
			defer mu.Unlock()
			files[p] = string(b)
			return nil
		},
		LookPath: func(name string) (string, error) { return "/usr/sbin/" + name, nil },
		Run: func(_ context.Context, name string, args []string) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			ran = append(ran, name+" "+strings.Join(args, " "))
			return "ok", nil
		},
	}
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	run := fakeDpkg(map[string]string{"nginx": "install ok installed 1.24.0-1"})
	svc := New(
		envpack.NewGatewayWithRunners(func(string) (string, error) { return "/usr/bin/apt", nil }, run),
		sysd.NewGateway(0), storeenv.New(db), Options{ConfigFS: cfs})
	actor := &authsvc.Identity{UserID: 1, Username: "admin", Role: storeauth.RoleAdmin}
	_, err = svc.SettingsSave(context.Background(), actor, storeenv.EnvNginx, map[string]string{
		"worker_processes": "4",
		"gzip":             "on",
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	got := files["/etc/nginx/nginx.conf"]
	if !strings.Contains(got, "worker_processes 4;") {
		t.Fatalf("worker_processes not applied:\n%s", got)
	}
	if !strings.Contains(got, "gzip on;") {
		t.Fatalf("gzip not applied:\n%s", got)
	}
	joined := strings.Join(ran, ";")
	if !strings.Contains(joined, "nginx -t") {
		t.Fatalf("expected nginx -t, ran %v", ran)
	}
}

func TestConfigFileSaveRollsBackOnValidateFail(t *testing.T) {
	orig := "worker_processes auto;\n"
	files := map[string]string{"/etc/nginx/nginx.conf": orig}
	cfs := &ConfigFS{
		Exists:    func(p string) bool { return p == "/etc/nginx/nginx.conf" },
		ReadFile:  func(p string) ([]byte, error) { return []byte(files[p]), nil },
		WriteFile: func(p string, b []byte, _ fs.FileMode) error { files[p] = string(b); return nil },
		LookPath:  func(name string) (string, error) { return "/usr/sbin/" + name, nil },
		Run: func(_ context.Context, name string, args []string) (string, error) {
			if len(args) > 0 && args[0] == "-t" {
				return "nginx: [emerg] unexpected }", errValidate
			}
			return "", nil
		},
	}
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	run := fakeDpkg(map[string]string{"nginx": "install ok installed 1.24.0-1"})
	svc := New(
		envpack.NewGatewayWithRunners(func(string) (string, error) { return "/usr/bin/apt", nil }, run),
		sysd.NewGateway(0), storeenv.New(db), Options{ConfigFS: cfs})
	actor := &authsvc.Identity{UserID: 1, Username: "admin", Role: storeauth.RoleAdmin}
	_, err = svc.ConfigFileSave(context.Background(), actor, storeenv.EnvNginx, "main", "this is bad")
	if err == nil {
		t.Fatal("want validate fail")
	}
	if files["/etc/nginx/nginx.conf"] != orig {
		t.Fatalf("must roll back, got %q", files["/etc/nginx/nginx.conf"])
	}
}

var errValidate = errString("nginx -t failed")

type errString string

func (e errString) Error() string { return string(e) }
