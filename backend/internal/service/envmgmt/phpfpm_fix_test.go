// v0.1.15 实机热修回归（用户服务器 v0.1.14 反馈）：
// ① unitFor 字面名 not-found「假命中」跳过版本化扫描 → apt php-fpm.service
//
//	StartUnit unit not found（真实单元 phpX.Y-fpm.service）；
//
// ② PHP-FPM 安装仅装 php-fpm 缺扩展 → phpMyAdmin DBT_001（mysqli/mbstring/zip/gd/curl）。
// 纯函数 resolveUnitName 表驱动 + worker 安装链全注入（零 runner 实况依赖）。
package envmgmt

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dougong/dougong/internal/adapter/sysd"
	storeenv "github.com/dougong/dougong/internal/store/envmgmt"
)

// TestResolveUnitNameVersionedFallback 锁 Fix1：not-found/空装载态字面名须排除，
// 落版本化扫描；loaded 字面名直用（RHEL php-fpm.service 真实存在形态）。
func TestResolveUnitNameVersionedFallback(t *testing.T) {
	pattern := `^php[0-9][0-9.]*-fpm\.service$`
	units := []sysd.UnitSnapshot{
		{Name: "nginx.service", LoadState: "loaded"},
		{Name: "php8.3-fpm.service", LoadState: "loaded"},
	}
	cases := []struct {
		name    string
		lit     string
		detail  sysd.UnitDetail
		litErr  bool
		pattern string
		units   []sysd.UnitSnapshot
		want    string
		wantOK  bool
	}{
		{"not-found 字面名落版本化扫描", "php-fpm.service", sysd.UnitDetail{LoadState: "not-found"}, false, pattern, units, "php8.3-fpm.service", true},
		{"空装载态同样排除", "php-fpm.service", sysd.UnitDetail{LoadState: ""}, false, pattern, units, "php8.3-fpm.service", true},
		{"GetUnit 出错落扫描", "php-fpm.service", sysd.UnitDetail{}, true, pattern, units, "php8.3-fpm.service", true},
		{"loaded 字面名直用", "php-fpm.service", sysd.UnitDetail{LoadState: "loaded"}, false, "", nil, "php-fpm.service", true},
		{"无匹配返回不命中", "php-fpm.service", sysd.UnitDetail{LoadState: "not-found"}, false, pattern, []sysd.UnitSnapshot{{Name: "nginx.service"}}, "php-fpm.service", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := resolveUnitName(c.lit, c.detail, c.litErr, c.pattern, c.units)
			if ok != c.wantOK || got != c.want {
				t.Fatalf("resolveUnitName = %q/%v, want %q/%v", got, ok, c.want, c.wantOK)
			}
		})
	}
}

// TestPHPFPMInstallIncludesExtensions 锁 Fix2：PHP-FPM 安装链主包 php-fpm 之后
// best-effort 安装五必需扩展（phpMyAdmin 依赖）；任务 succeeded。
func TestPHPFPMInstallIncludesExtensions(t *testing.T) {
	var installed []string
	run := func(_ context.Context, _ map[string]string, name string, args []string) (string, error) {
		switch {
		case name == "dpkg-query":
			return "dpkg-query: no packages found matching php-fpm", errors.New("exit status 1")
		case name == "apt-get" && args[0] == "update":
			return "Hit:1 ubuntu noble InRelease", nil
		case name == "apt-get" && strings.Contains(strings.Join(args, " "), " install "):
			pkg := args[len(args)-1]
			installed = append(installed, pkg)
			return "Setting up " + pkg, nil
		}
		return "", errors.New("unexpected cmd " + name)
	}
	s := newNodeTestService(t, run, nil)
	ctx := context.Background()
	taskID, err := s.createTask(ctx, nodeActor(), storeenv.EnvPHPFPM,
		storeenv.ActionInstall, AuditActionEnvInstallStarted, map[string]any{"envType": storeenv.EnvPHPFPM})
	if err != nil {
		t.Fatalf("createTask: %v", err)
	}
	s.runTask(taskID)
	task, err := s.store.Task().GetByID(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != storeenv.TaskSucceeded {
		t.Fatalf("status=%s err=%v tail=%s", task.Status, task.Error, task.OutputTail)
	}
	// 主包 + 五扩展均经 apt-get install（顺序：主包先）。
	want := []string{"php-fpm", "php-mysqli", "php-mbstring", "php-zip", "php-gd", "php-curl"}
	if len(installed) < len(want) || installed[0] != "php-fpm" {
		t.Fatalf("installed=%v, want 主包 php-fpm 先 + 五扩展", installed)
	}
	for _, w := range want {
		found := false
		for _, p := range installed {
			if p == w {
				found = true
			}
		}
		if !found {
			t.Fatalf("扩展 %s 未安装：installed=%v", w, installed)
		}
	}
}
