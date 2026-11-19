// 落点选择/注入撤除/全景投影测试（D-03/D-02/I-11；A-DBT8 byte 级往返）。
package dbtools

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/errcode"
	storedbt "github.com/dougong/dougong/internal/store/dbtools"
)

// TestLandingSelection 落点选择形态（bind_domain 匹配/proxy 回退/多候选/
// 0 匹配自动建块〔v1.3 D-19〕/https scheme）。
func TestLandingSelection(t *testing.T) {
	ctx := context.Background()
	t.Run("bind-domain-match", func(t *testing.T) {
		f := defaultFakes()
		svc, _ := newTestService(t, f)
		ref, err := svc.selectLanding(ctx, f.panel.pa)
		if err != nil {
			t.Fatal(err)
		}
		if ref.File != testPanelConf || ref.ServerName != "panel.example.test" || ref.Scheme != "http" {
			t.Fatalf("ref = %+v", ref)
		}
	})
	t.Run("proxy-fallback", func(t *testing.T) {
		f := defaultFakes()
		f.panel.pa.BindDomain = "" // 未绑定 → proxy_pass 指向面板监听地址
		f.nginx.files[testPanelConf] = `server {
    listen 80;
    server_name site.example.test;
    location / {
        proxy_pass http://localhost:8687/;
    }
}
`
		f.pipeline.files[testPanelConf] = f.nginx.files[testPanelConf]
		svc, _ := newTestService(t, f)
		ref, err := svc.selectLanding(ctx, f.panel.pa)
		if err != nil {
			t.Fatal(err)
		}
		if ref.ServerName != "site.example.test" {
			t.Fatalf("ref = %+v", ref)
		}
	})
	t.Run("multi-candidate", func(t *testing.T) {
		f := defaultFakes()
		f.panel.pa.BindDomain = ""
		extra := "/etc/nginx/conf.d/second.conf"
		body := `server {
    listen 80;
    server_name another.example.test;
    location / {
        proxy_pass http://127.0.0.1:8687;
    }
}
`
		f.nginx.files[extra] = body
		f.pipeline.files[extra] = body
		svc, _ := newTestService(t, f)
		_, err := svc.selectLanding(ctx, f.panel.pa)
		if symbolOf(err) != "DBT_012" {
			t.Fatalf("multi candidate: %v", err)
		}
		var ec *errcode.Error
		errors.As(err, &ec)
		reason, _ := ec.Data().(map[string]any)["reason"].(string)
		if !strings.Contains(reason, "second.conf") {
			t.Fatalf("reason should list candidates: %s", reason)
		}
	})
	t.Run("no-candidate-auto-create", func(t *testing.T) {
		// v1.3 D-19：0 匹配不再 DBT_012 拒绝——自动创建专用工具落地块。
		f := defaultFakes()
		f.panel.pa.BindDomain = "absent.example.test"
		svc, _ := newTestService(t, f)
		ref, err := svc.selectLanding(ctx, f.panel.pa)
		if err != nil {
			t.Fatalf("0 匹配应自动建块: %v", err)
		}
		if !ref.AutoCreated || ref.Port != ToolLandingPort || ref.Scheme != "http" ||
			ref.ServerName != "absent.example.test" || filepath.Base(ref.File) != ToolLandingConfName {
			t.Fatalf("auto landing ref = %+v", ref)
		}
	})
	t.Run("https-scheme", func(t *testing.T) {
		f := defaultFakes()
		f.nginx.files[testPanelConf] = strings.Replace(testPanelConfBody, "listen 80;", "listen 443 ssl;", 1)
		f.pipeline.files[testPanelConf] = f.nginx.files[testPanelConf]
		svc, _ := newTestService(t, f)
		ref, err := svc.selectLanding(ctx, f.panel.pa)
		if err != nil || ref.Scheme != "https" {
			t.Fatalf("ref = %+v err=%v", ref, err)
		}
	})
}

// TestInjectionRoundTripBothTools 两工具共享 gate：注入→逐个撤除 byte 级
// 还原（A-DBT8）；gate 随最后一个工具撤除。
func TestInjectionRoundTripBothTools(t *testing.T) {
	f := defaultFakes()
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	pa := f.panel.pa
	secret, _ := svc.ensureInternalSecret()
	ref, err := svc.selectLanding(ctx, pa)
	if err != nil {
		t.Fatal(err)
	}
	// 注入 phpmyadmin（含 gate）。
	if err := svc.injectToolLocations(ctx, storedbt.ToolPHPMyAdmin, ref, pa, secret, "/run/php/php8.1-fpm.sock"); err != nil {
		t.Fatal(err)
	}
	afterPMA := f.pipeline.files[testPanelConf]
	if !hasMarker(afterPMA, markerKeyGate) || !hasMarker(afterPMA, storedbt.ToolPHPMyAdmin) {
		t.Fatal("pma+gate markers missing")
	}
	// 注入态语法自检（括号配平——闭合 '}' 不被 END 注释吞没，A-DBT8）。
	if issues := nginxx.ValidateSyntax(afterPMA); len(issues) != 0 {
		t.Fatalf("injected conf syntax issues: %v", issues)
	}
	// 注入 mongo_express（gate 幂等不重复）。
	if err := svc.injectToolLocations(ctx, storedbt.ToolMongoExpress, ref, pa, secret, ""); err != nil {
		t.Fatal(err)
	}
	both := f.pipeline.files[testPanelConf]
	if strings.Count(both, "# BEGIN dougong-dbtool gate") != 1 {
		t.Fatal("gate location duplicated")
	}
	if issues := nginxx.ValidateSyntax(both); len(issues) != 0 {
		t.Fatalf("both-tools conf syntax issues: %v", issues)
	}
	// 撤除 phpmyadmin：gate 保留（mongo 仍在）。
	if err := svc.removeToolInjection(ctx, storedbt.ToolPHPMyAdmin, ref); err != nil {
		t.Fatal(err)
	}
	afterRemovePMA := f.pipeline.files[testPanelConf]
	if hasMarker(afterRemovePMA, storedbt.ToolPHPMyAdmin) || !hasMarker(afterRemovePMA, markerKeyGate) {
		t.Fatal("pma removed but gate should remain")
	}
	// 撤除 mongo_express：gate 一并撤除 → 原字节。
	if err := svc.removeToolInjection(ctx, storedbt.ToolMongoExpress, ref); err != nil {
		t.Fatal(err)
	}
	if got := f.pipeline.files[testPanelConf]; got != testPanelConfBody {
		t.Fatalf("not byte-restored:\n%q", got)
	}
	// 幂等：再次撤除无副作用。
	if err := svc.removeToolInjection(ctx, storedbt.ToolMongoExpress, ref); err != nil {
		t.Fatal(err)
	}
	if got := f.pipeline.files[testPanelConf]; got != testPanelConfBody {
		t.Fatal("idempotent removal changed content")
	}
}

// TestStripMarkerCorrupted BEGIN 无 END → 如实报错（不静默残留）。
func TestStripMarkerCorrupted(t *testing.T) {
	content := "server {\n    # BEGIN dougong-dbtool phpmyadmin\n    location x {}\n}\n"
	if _, _, err := stripMarker(content, storedbt.ToolPHPMyAdmin); err == nil {
		t.Fatal("corrupted marker should error")
	}
	// 无标记 → 原样返回 removed=false。
	out, removed, err := stripMarker(testPanelConfBody, storedbt.ToolPHPMyAdmin)
	if err != nil || removed || out != testPanelConfBody {
		t.Fatalf("no marker: removed=%v err=%v", removed, err)
	}
}

// TestToolListProjection 全景投影（D-02/I-11）：档案存在性+运行态+依赖
// 就绪度+在途标记；可空字段 null 纪律。
func TestToolListProjection(t *testing.T) {
	f := defaultFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	// 空档案：两工具未装，依赖就绪（env 全装+扩展齐全）。
	data, err := svc.ToolList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Items) != 2 || data.GeneratedAt == "" {
		t.Fatalf("items = %d", len(data.Items))
	}
	for _, it := range data.Items {
		if it.Installed || it.InstalledVersion != nil || it.Running != nil || it.BoundInstanceID != nil {
			t.Fatalf("uninstalled item should carry nulls: %+v", it)
		}
		if !it.DepsReady || len(it.DepsMissing) != 0 {
			t.Fatalf("deps should be ready: %+v", it)
		}
	}
	// 安装 mongo_express → installed/running/boundInstanceId 投影。
	inst := int64(7)
	created, _ := svc.Install(ctx, testActor(), storedbt.ToolMongoExpress, &inst)
	svc.runTask(created.TaskID)
	data, _ = svc.ToolList(ctx)
	var mongo *ToolItem
	for i := range data.Items {
		if data.Items[i].ToolType == storedbt.ToolMongoExpress {
			mongo = &data.Items[i]
		}
	}
	if mongo == nil || !mongo.Installed || mongo.InstalledVersion == nil || *mongo.InstalledVersion != "1.0.0" ||
		mongo.BoundInstanceID == nil || *mongo.BoundInstanceID != 7 || mongo.Running == nil || !*mongo.Running {
		t.Fatalf("mongo item = %+v", mongo)
	}
	// 依赖缺失投影：node 未装 + php 扩展缺 gd。
	f.env.status["node"] = [3]any{false, "", true}
	f.deploy.phpExts = []string{"mysqli", "mbstring", "zip", "json", "curl", "openssl"}
	data, _ = svc.ToolList(ctx)
	for _, it := range data.Items {
		if it.ToolType == storedbt.ToolMongoExpress {
			if it.DepsReady || len(it.DepsMissing) != 1 || it.DepsMissing[0].Name != "node" {
				t.Fatalf("mongo deps = %+v", it)
			}
		}
		if it.ToolType == storedbt.ToolPHPMyAdmin {
			if it.DepsReady || len(it.DepsMissing) != 1 || it.DepsMissing[0].Kind != "extension" || it.DepsMissing[0].Name != "gd" {
				t.Fatalf("pma deps = %+v", it)
			}
		}
	}
	// 在途任务标记。
	id, _ := store.Task().Create(ctx, storedbt.Task{ToolType: storedbt.ToolPHPMyAdmin,
		Action: storedbt.ActionInstall, RequestedBy: "admin"})
	data, _ = svc.ToolList(ctx)
	for _, it := range data.Items {
		if it.ToolType == storedbt.ToolPHPMyAdmin && (it.RunningTaskID == nil || *it.RunningTaskID != id) {
			t.Fatalf("running task marker missing: %+v", it)
		}
	}
}

// TestToolStatusForEngine OHS（5.1：database-mgmt 高级入口状态源）。
func TestToolStatusForEngine(t *testing.T) {
	f := defaultFakes()
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	// 未装。
	info := svc.ToolStatusForEngine(ctx, "mongodb")
	if info.ToolType != storedbt.ToolMongoExpress || info.Installed || info.Openable {
		t.Fatalf("uninstalled status = %+v", info)
	}
	if svc.ToolStatusForEngine(ctx, "redis").ToolType != "" {
		t.Fatal("redis should map no tool")
	}
	// 安装后运行中 → openable。
	inst := int64(7)
	created, _ := svc.Install(ctx, testActor(), storedbt.ToolMongoExpress, &inst)
	svc.runTask(created.TaskID)
	info = svc.ToolStatusForEngine(ctx, "mongodb")
	if !info.Installed || !info.Running || !info.Openable || info.BoundInstanceID == nil || *info.BoundInstanceID != 7 {
		t.Fatalf("installed status = %+v", info)
	}
	// 单元停止 → running=false/openable=false。
	f.sysd.units[MongoExpressUnitName] = sysd.UnitDetail{LoadState: "loaded", ActiveState: "inactive"}
	info = svc.ToolStatusForEngine(ctx, "mongodb")
	if info.Running || info.Openable {
		t.Fatalf("stopped status = %+v", info)
	}
	// mysql/mariadb → phpmyadmin 映射。
	if svc.ToolStatusForEngine(ctx, "mysql").ToolType != storedbt.ToolPHPMyAdmin ||
		svc.ToolStatusForEngine(ctx, "mariadb").ToolType != storedbt.ToolPHPMyAdmin {
		t.Fatal("mysql/mariadb should map phpmyadmin")
	}
}
