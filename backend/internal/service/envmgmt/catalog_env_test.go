// W1 实机冒烟缺陷修复回归（api-env-mgmt.md v1.1）：
//   - 目录机械断言（D-13 ⚠️A8）：七条目 UNIT 名规范全名——W1 缺陷 1
//     （enable nginx 报 DBus InvalidArgs）根因为无后缀名；
//   - EnvList 三态回归（D-14 ⚠️A9）：W1 缺陷 2（未装误显「探测不可用」）
//     根因为 dpkg「no packages found matching」被误归通道错误。
//
// envpack 经 NewGatewayWithRunners 全注入（探测恒 apt、dpkg-query 输出伪化），
// CI runner 包数据库/systemd 状态不参与测试（第 5 起环境耦合预防）。
package envmgmt

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dougong/dougong/internal/adapter/envpack"
	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/store"
	storeenv "github.com/dougong/dougong/internal/store/envmgmt"
)

func TestCatalogUnitNamesFullyQualified(t *testing.T) {
	if len(Catalog) != 7 {
		t.Fatalf("catalog entries = %d, want 7（v1.1 +MongoDB）", len(Catalog))
	}
	families := []string{"apt", "dnf", "yum"}
	for _, e := range Catalog {
		for _, f := range families {
			u := e.UnitByFamily[f]
			if u == "" {
				t.Fatalf("%s/%s unit empty", e.Type, f)
			}
			if !strings.HasSuffix(u, ".service") {
				t.Fatalf("%s/%s unit %q 无 .service 后缀（D-13：EnableUnitFiles 无后缀补全→InvalidArgs）", e.Type, f, u)
			}
			if len(e.PkgByFamily[f]) == 0 {
				t.Fatalf("%s/%s candidates empty", e.Type, f)
			}
		}
	}
	mongo := CatalogByType(storeenv.EnvMongoDB)
	if mongo == nil {
		t.Fatal("mongodb 不在目录（⚠️A1 七环境）")
	}
	for _, f := range families {
		if !mongo.VendorRepo[f] {
			t.Fatalf("mongodb/%s VendorRepo 应为 true（⚠️A10 三系均需第三方源）", f)
		}
	}
	if CatalogByType("nope") != nil {
		t.Fatal("未知 envType 应返回 nil")
	}
}

// fakeDpkg 按包名返回 dpkg-query -W 输出（apt 族）；键不存在=未装错误分支。
func fakeDpkg(out map[string]string) func(ctx context.Context, env map[string]string, name string, args []string) (string, error) {
	return func(_ context.Context, env map[string]string, name string, args []string) (string, error) {
		if env["LC_ALL"] != "C" {
			return "", errors.New("fakeDpkg: LC_ALL=C not injected")
		}
		pkg := args[len(args)-1]
		if o, ok := out[pkg]; ok {
			return o, nil
		}
		return "dpkg-query: no packages found matching " + pkg, errors.New("exit status 1")
	}
}

func newTestService(t *testing.T, run func(ctx context.Context, env map[string]string, name string, args []string) (string, error)) *Service {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate（含 010 七值 CHECK）: %v", err)
	}
	return New(
		envpack.NewGatewayWithRunners(func(string) (string, error) { return "/usr/bin/apt", nil }, run),
		sysd.NewGateway(0), storeenv.New(db), Options{})
}

// TestEnvListThreeStates 三态回归（W1 缺陷 2）：
//   - 未装（含包名不在 dpkg 数据库）→ installed=false「未安装」非「探测不可用」；
//   - 多候选第二命中（docker-ce 自装形态）→ installed=true；
//   - 探测真错误（php-fpm 查询超时）→ installed=null 条目级「探测不可用」。
func TestEnvListThreeStates(t *testing.T) {
	s := newTestService(t, fakeDpkg(map[string]string{
		"nginx":                  "install ok installed 1.24.0-1",   // 已装
		"mysql-community-server": "install ok installed 8.0.36",     // 第二候选命中
		"redis-server":           "deinstall ok config-files 5.0.7", // 配置残留=未装
		// docker.io/docker-ce、apache2、mongodb-org、php-fpm 无键 → no packages found
		// php-fpm 在 fakeDpkg 中需走「真错误」分支——改用独立 runner（见下）。
	}))
	data, err := s.EnvList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Items) != 7 {
		t.Fatalf("items = %d, want 7", len(data.Items))
	}
	if !data.Channel.Available || data.Channel.Family != "apt" {
		t.Fatalf("channel = %+v", data.Channel)
	}
	type itemState struct {
		inst *bool
		ver  *string
		unit string
	}
	got := map[string]itemState{}
	for _, it := range data.Items {
		got[it.EnvType] = itemState{it.Installed, it.Version, it.UnitName}
	}
	// 未装（W1 缺陷 2 核心回归）：docker 双候选均不在数据库 → false 而非 null。
	if got["docker"].inst == nil || *got["docker"].inst {
		t.Fatalf("docker installed = %v, want false（no packages found=未装 D-14）", got["docker"].inst)
	}
	if got["docker"].unit != "docker.service" {
		t.Fatalf("docker unit = %q, want docker.service（全名 D-13）", got["docker"].unit)
	}
	// 已装 + 版本。
	if got["nginx"].inst == nil || !*got["nginx"].inst || got["nginx"].ver == nil || *got["nginx"].ver != "1.24.0-1" {
		t.Fatalf("nginx = %+v", got["nginx"])
	}
	// 多候选第二命中。
	if got["mysql"].inst == nil || !*got["mysql"].inst || *got["mysql"].ver != "8.0.36" {
		t.Fatalf("mysql = %+v（want 第二候选 mysql-community-server 命中）", got["mysql"])
	}
	// 配置残留=未装。
	if got["redis"].inst == nil || *got["redis"].inst {
		t.Fatalf("redis = %+v（deinstall ok config-files=未装）", got["redis"])
	}
	// mongodb 已入目录且未装。
	if got["mongodb"].inst == nil || *got["mongodb"].inst || got["mongodb"].unit != "mongod.service" {
		t.Fatalf("mongodb = %+v", got["mongodb"])
	}
	// apache 未装。
	if got["apache"].inst == nil || *got["apache"].inst {
		t.Fatalf("apache = %+v", got["apache"])
	}
}

// TestEnvListProbeErrorNullItem 探测真错误 → 条目级 null（探测不可用），
// 不与「未装」混同（D-01 三态）。
func TestEnvListProbeErrorNullItem(t *testing.T) {
	s := newTestService(t, func(_ context.Context, _ map[string]string, name string, args []string) (string, error) {
		if name == "dpkg-query" && args[len(args)-1] == "php-fpm" {
			return "", errors.New("context deadline exceeded") // 真错误（超时）
		}
		return "dpkg-query: no packages found matching " + args[len(args)-1], errors.New("exit status 1")
	})
	data, err := s.EnvList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range data.Items {
		switch it.EnvType {
		case "php_fpm":
			if it.Installed != nil {
				t.Fatalf("php_fpm installed = %v, want nil（探测真错误=条目级不可用）", *it.Installed)
			}
		default:
			if it.Installed == nil || *it.Installed {
				t.Fatalf("%s installed 应为 false（未装），got %v", it.EnvType, it.Installed)
			}
		}
	}
}
