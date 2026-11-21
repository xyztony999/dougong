package envmgmt

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dougong/dougong/internal/adapter/envpack"
	"github.com/dougong/dougong/internal/adapter/sysd"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/dougong/dougong/internal/store"
	storeenv "github.com/dougong/dougong/internal/store/envmgmt"
)

// v1.4a Python 环境缺陷修复回归（实机 RHEL：python3 预装→「已装」态 pip 永远
// 缺位）：①目录条目 extras 承载 venv/pip 支撑包（apt python3-venv/python3-pip、
// dnf/yum python3-pip）；②worker 安装链在主包后补装 extras（「已装但不完整」
// 经升级动作补齐——沿 phpMyAdmin PHP 扩展 ExtraPkgsByFamily 先例）。

func TestPythonEnvExtrasByFamily(t *testing.T) {
	e := CatalogByType(storeenv.EnvPython)
	if e == nil {
		t.Fatal("python 不在目录（九环境）")
	}
	// 探测候选：apt 三包（python3 首选为主包）；dnf/yum 单候选 python3。
	if got := strings.Join(e.PkgByFamily["apt"], ","); got != "python3" {
		t.Fatalf("apt candidates = %q, want python3（主包；venv/pip 移入 extras）", got)
	}
	if got := strings.Join(e.PkgByFamily["dnf"], ","); got != "python3" {
		t.Fatalf("dnf candidates = %q, want python3", got)
	}
	// extras：venv/pip 支撑包（实机缺陷 1 根因修复——此前无 extras 致 pip 永远缺位）。
	if got := strings.Join(e.ExtraPkgsByFamily["apt"], ","); got != "python3-venv,python3-pip" {
		t.Fatalf("apt extras = %q, want python3-venv,python3-pip", got)
	}
	if got := strings.Join(e.ExtraPkgsByFamily["dnf"], ","); got != "python3-pip" {
		t.Fatalf("dnf extras = %q, want python3-pip", got)
	}
	if got := strings.Join(e.ExtraPkgsByFamily["yum"], ","); got != "python3-pip" {
		t.Fatalf("yum extras = %q, want python3-pip", got)
	}
	for _, f := range []string{"apt", "dnf", "yum"} {
		if e.VendorRepo[f] {
			t.Fatalf("python/%s VendorRepo 应为 false（发行版仓库包 ⚠️A18）", f)
		}
		if u := e.UnitByFamily[f]; u != "" {
			t.Fatalf("python/%s unit = %q, want empty（运行时型 ⚠️A17 同构）", f, u)
		}
	}
}

// TestPythonInstallChainInstallsPipExtras worker 安装链：主包 python3 → extras
// python3-venv → python3-pip（次序断言；apt 形态；零 runner 实况依赖）。
func TestPythonInstallChainInstallsPipExtras(t *testing.T) {
	var events []string
	run := func(_ context.Context, _ map[string]string, name string, args []string) (string, error) {
		joined := strings.Join(args, " ")
		events = append(events, "cmd:"+name+" "+joined)
		switch {
		case name == "dpkg-query":
			// python3 未装（新装语义；预装形态走 upgrade 同链 extras 照装）。
			return "dpkg-query: no packages found matching python3", errors.New("exit status 1")
		case name == "apt-get" && args[0] == "update":
			return "Hit:1 archive.ubuntu.com noble InRelease", nil
		case name == "apt-get" && strings.Contains(joined, " install "):
			return "Setting up " + args[len(args)-1], nil
		}
		return "", errors.New("unexpected cmd " + name + " " + joined)
	}
	s := newPythonTestService(t, run)
	taskID, err := s.createTask(context.Background(), pythonActor(), storeenv.EnvPython,
		storeenv.ActionInstall, AuditActionEnvInstallStarted, map[string]any{"envType": storeenv.EnvPython})
	if err != nil {
		t.Fatalf("createTask: %v", err)
	}
	s.runTask(taskID)

	task, err := s.store.Task().GetByID(context.Background(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != storeenv.TaskSucceeded {
		t.Fatalf("status = %s (%v)", task.Status, derefEnv(task.Error))
	}
	joined := strings.Join(events, "\n")
	if !strings.Contains(joined, "apt-get update") {
		t.Fatalf("install chain missing apt-get update\n%s", joined)
	}
	// 逐事件后缀匹配（apt install 前缀含 -y/-o conffile 保守参数 D-05）+ 保序：
	// 主包 python3 → extras python3-venv → python3-pip（worker extras 环节次序）。
	idx := map[string]int{}
	for i, ev := range events {
		for _, pkg := range []string{"python3", "python3-venv", "python3-pip"} {
			if strings.HasSuffix(ev, "install "+pkg) {
				if _, seen := idx[pkg]; !seen {
					idx[pkg] = i
				}
			}
		}
	}
	prev := -1
	for _, pkg := range []string{"python3", "python3-venv", "python3-pip"} {
		i, ok := idx[pkg]
		if !ok {
			t.Fatalf("install chain missing %s\n%s", pkg, joined)
		}
		if i <= prev {
			t.Fatalf("install order broken: %s after previous\n%s", pkg, joined)
		}
		prev = i
	}
}

func newPythonTestService(t *testing.T,
	run func(ctx context.Context, env map[string]string, name string, args []string) (string, error),
) *Service {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(envpack.NewGatewayWithVendorIO(func(string) (string, error) { return "/usr/bin/apt", nil }, run, &envpack.VendorIO{}),
		sysd.NewGateway(0), storeenv.New(db), Options{})
}

func pythonActor() *authsvc.Identity {
	return &authsvc.Identity{UserID: 1, Username: "admin", Role: authsvc.RoleAdmin}
}

func derefEnv(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
