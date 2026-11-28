// v1.3 自动 nginx 落地回归（api-db-tools D-19~D-23，d0-clarification-
// dbtools-auto-landing-2026-11-19.md 验收口径）：自动建块内容/域名联动/
// 端口占用/幂等复用/既有块优先/卸载清块/toolBaseHost Port 分支/reason 可见。
package dbtools

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/errcode"
	storedbt "github.com/dougong/dougong/internal/store/dbtools"
)

// testLandingConf 与 landingConfPath 同口径（filepath.Join——Windows 测试
// 环境路径分隔符一致；生产 Linux 为 /etc/nginx/conf.d/dougong-tools.conf）。
var testLandingConf = filepath.Join("/etc/nginx/conf.d", ToolLandingConfName)

// autoLandingFakes 0 匹配形态替身（nginx 无任何 server 块——IP 直连未绑
// 域名实机根因形态；读取与管线共享同一空文件实况）。
func autoLandingFakes() *testFakes {
	f := defaultFakes()
	empty := map[string]string{}
	f.nginx.files = empty
	f.pipeline.files = empty
	f.panel.pa.BindDomain = ""
	return f
}

// tlsFakes HTTPS 面板+证书可用形态替身（v1.4 D-24）：自动落地块追加
// listen 2087 ssl（面板证书落盘引用）。
func tlsFakes() *testFakes {
	f := autoLandingFakes()
	f.panel.pa.HTTPSEnabled = true
	f.panel.certPEM = "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"
	f.panel.keyPEM = "-----BEGIN PRIVATE KEY-----\nBBBB\n-----END PRIVATE KEY-----\n"
	return f
}

// stripTLSLines 模拟旧版本 http-only 块（从 conf 剥离 TLS 行与指纹注释行）。
func stripTLSLines(conf string) string {
	var kept []string
	for _, ln := range strings.Split(conf, "\n") {
		switch {
		case strings.Contains(ln, "2087 ssl"),
			strings.Contains(ln, "ssl_certificate"),
			strings.Contains(ln, "Landing cert fingerprint"):
			continue
		}
		kept = append(kept, ln)
	}
	return strings.Join(kept, "\n")
}

// reasonOf 提取 errcode data.reason（D-22 断言用）。
func reasonOf(err error) string {
	var ec *errcode.Error
	if errors.As(err, &ec) {
		if m, ok := ec.Data().(map[string]any); ok {
			if r, ok := m["reason"].(string); ok {
				return r
			}
		}
	}
	return ""
}

// installAutoTool 自动块形态安装（同步 worker 确定性断言）。
func installAutoTool(t *testing.T, svc *Service, toolType string, instanceID *int64) {
	t.Helper()
	created, err := svc.Install(context.Background(), testActor(), toolType, instanceID)
	if err != nil {
		t.Fatalf("install %s: %v", toolType, err)
	}
	svc.runTask(created.TaskID)
	task, err := svc.store.Task().GetByID(context.Background(), created.TaskID)
	if err != nil || task.Status != storedbt.TaskSucceeded {
		t.Fatalf("install %s task = %+v err=%v tail=%s", toolType, task, err, task.OutputTail)
	}
}

// TestAutoLandingInstallUnbound 0 匹配自动建块（D-19）：listen 18090/
// server_name _（未绑）/gate+工具 location/每请求 auth_request；siteRef
// port/autoCreated；安装成功放行提示（D-19⑦）；SignonURL 面板直连端口
// 口径（A-DBT11）。
func TestAutoLandingInstallUnbound(t *testing.T) {
	f := autoLandingFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	installAutoTool(t, svc, storedbt.ToolPHPMyAdmin, nil)

	conf, ok := f.pipeline.files[testLandingConf]
	if !ok {
		t.Fatalf("自动落地块未创建（files=%v）", f.pipeline.files)
	}
	for _, want := range []string{
		"listen 18090;", "server_name _;",
		"# BEGIN dougong-dbtool landing", "# END dougong-dbtool landing",
		"# BEGIN dougong-dbtool gate", "location = /dbtool-gate-internal", "internal;",
		"proxy_pass http://127.0.0.1:8687/dbtool/gate/verify",
		"# BEGIN dougong-dbtool phpmyadmin", "auth_request /dbtool-gate-internal",
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("自动块缺少 %q:\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "if ($host") {
		t.Fatal("未绑域名不应有 if 非匹配 Host 404 行")
	}
	if issues := nginxx.ValidateSyntax(conf); len(issues) != 0 {
		t.Fatalf("自动块语法问题: %v", issues)
	}
	// siteRef 投影（D-19：port=18090/autoCreated=true）。
	a, _ := store.Tool().GetByType(ctx, storedbt.ToolPHPMyAdmin)
	ref, err := parseSiteRef(a.SiteRef)
	if err != nil || ref.Port != ToolLandingPort || !ref.AutoCreated || ref.Scheme != "http" || ref.ServerName != "" {
		t.Fatalf("siteRef = %+v err=%v", ref, err)
	}
	// 安装成功输出尾部放行提示（D-19⑦ 仅提示不自动放行）。
	task, _ := store.Task().GetByID(ctx, 1)
	if !strings.Contains(task.OutputTail, "请放行 TCP 18090 端口（防火墙/云安全组）") {
		t.Fatalf("outputTail 缺放行提示: %s", task.OutputTail)
	}
	// SignonURL：未绑安装期无请求语境→动态 HTTP_HOST 去端口+面板端口
	//（A-DBT11：reopen 为面板后端 API，非工具落地端口 18090）。
	cfg := string(f.deploy.files[filepath.Join(svc.toolDir, "phpmyadmin", "config.inc.php")])
	if !strings.Contains(cfg, "preg_replace('/:\\d+$/', '', $_SERVER['HTTP_HOST'])") ||
		!strings.Contains(cfg, ":8687") || !strings.Contains(cfg, "/dbtool/access/reopen?instanceId=") {
		t.Fatalf("SignonURL 动态面板端口表达式缺失:\n%s", cfg)
	}
	if strings.Contains(cfg, ":18090") {
		t.Fatal("SignonURL 不得指向工具落地端口（reopen 为面板后端 API）")
	}
}

// TestAutoLandingInstallBound 已绑域名 0 匹配自动建块（D-20）：
// server_name=域名 + if 非匹配 Host 404（正则元字符转义，镜像 HostGuard）；
// Open URL 域名:18090（D-23）。
func TestAutoLandingInstallBound(t *testing.T) {
	f := autoLandingFakes()
	f.panel.pa.BindDomain = "tools.example.test"
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	installAutoTool(t, svc, storedbt.ToolPHPMyAdmin, nil)

	conf := f.pipeline.files[testLandingConf]
	for _, want := range []string{
		"server_name tools.example.test;",
		`if ($host !~* "^tools\.example\.test$") { return 404; }`,
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("自动块缺少 %q:\n%s", want, conf)
		}
	}
	if issues := nginxx.ValidateSyntax(conf); len(issues) != 0 {
		t.Fatalf("自动块语法问题: %v", issues)
	}
	// SignonURL 静态烘焙：面板直连基址含监听端口（A-DBT11）。
	cfg := string(f.deploy.files[filepath.Join(svc.toolDir, "phpmyadmin", "config.inc.php")])
	if !strings.Contains(cfg, "http://tools.example.test:8687") {
		t.Fatalf("SignonURL 静态面板基址缺失:\n%s", cfg)
	}
	// Open URL 端口分支（D-23：BindDomain 优先）。
	inst := int64(9)
	data, err := svc.Open(ctx, testActor(), storedbt.ToolPHPMyAdmin, &inst, "10.0.0.8:8687")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !strings.HasPrefix(data.URL, "http://tools.example.test:18090/dbtool/phpmyadmin/?dg_signon=") {
		t.Fatalf("open url = %q", data.URL)
	}
}

// TestAutoLandingPortOccupied 端口占用（D-19③）：DBT_012 reason 如实
// 「工具端口 18090 被占用，请释放后重试」——不静默换端口；reason 经 worker
// 错误文本可见（D-22）；不建块不留档案。
func TestAutoLandingPortOccupied(t *testing.T) {
	f := autoLandingFakes()
	f.deploy.portOK = false
	svc, store := newTestService(t, f)
	ctx := context.Background()
	created, err := svc.Install(ctx, testActor(), storedbt.ToolPHPMyAdmin, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.runTask(created.TaskID)
	task, _ := store.Task().GetByID(ctx, created.TaskID)
	if task.Status != storedbt.TaskFailed || task.Error == nil {
		t.Fatalf("task = %+v", task)
	}
	want := "DBT_012: 部署操作失败: 工具端口 18090 被占用，请释放后重试"
	if *task.Error != want {
		t.Fatalf("task.Error = %q, want %q（D-22 reason 可见）", *task.Error, want)
	}
	if a, _ := store.Tool().GetByType(ctx, storedbt.ToolPHPMyAdmin); a != nil {
		t.Fatal("失败安装不留档案（I-C01）")
	}
	if _, ok := f.pipeline.files[testLandingConf]; ok {
		t.Fatal("端口占用不得建块")
	}
	if f.deploy.probeCalls != 1 {
		t.Fatalf("probe calls = %d, want 1", f.deploy.probeCalls)
	}
}

// TestAutoLandingWriteFailureReason 自动建块写入失败（ConfigApply R-06
// 回滚后）→DBT_012 reason「自动落地块写入失败」经 task.error 可见（D-22）。
func TestAutoLandingWriteFailureReason(t *testing.T) {
	f := autoLandingFakes()
	f.pipeline.applyErr = errors.New("nginx -t failed (fake)")
	svc, store := newTestService(t, f)
	ctx := context.Background()
	created, _ := svc.Install(ctx, testActor(), storedbt.ToolMongoExpress, ptrInt64(7))
	svc.runTask(created.TaskID)
	task, _ := store.Task().GetByID(ctx, created.TaskID)
	if task.Status != storedbt.TaskFailed || task.Error == nil ||
		!strings.Contains(*task.Error, "DBT_012: 部署操作失败: 自动落地块写入失败: nginx -t failed (fake)") {
		t.Fatalf("task = %+v", task)
	}
}

// TestAutoLandingNoConfDir nginx 配置目录全不可用→DBT_012 reason
// 「nginx 配置目录不可用」（D-19①）。
func TestAutoLandingNoConfDir(t *testing.T) {
	f := autoLandingFakes()
	f.nginx.dirs = nil
	svc, _ := newTestService(t, f)
	_, err := svc.selectLanding(context.Background(), f.panel.pa)
	if symbolOf(err) != "DBT_012" || !strings.Contains(reasonOf(err), "nginx 配置目录不可用") {
		t.Fatalf("no conf dir: %v", err)
	}
}

// TestAutoLandingIdempotentReuse 幂等复用（D-19④）：两工具共用一块，
// gate 去重；复用先于端口探针（nginx 自身已占 18090 为常态——第二工具
// 安装不再探针）。
func TestAutoLandingIdempotentReuse(t *testing.T) {
	f := autoLandingFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	installAutoTool(t, svc, storedbt.ToolPHPMyAdmin, nil)
	if f.deploy.probeCalls != 1 {
		t.Fatalf("首装探针 = %d, want 1", f.deploy.probeCalls)
	}
	installAutoTool(t, svc, storedbt.ToolMongoExpress, ptrInt64(7))
	if f.deploy.probeCalls != 1 {
		t.Fatalf("复用不应再探针（nginx 已占 18090）: %d", f.deploy.probeCalls)
	}
	conf := f.pipeline.files[testLandingConf]
	if strings.Count(conf, "# BEGIN dougong-dbtool gate") != 1 {
		t.Fatal("gate location 重复注入")
	}
	if !hasMarker(conf, storedbt.ToolPHPMyAdmin) || !hasMarker(conf, storedbt.ToolMongoExpress) {
		t.Fatal("两工具 location 应共用一块")
	}
	if issues := nginxx.ValidateSyntax(conf); len(issues) != 0 {
		t.Fatalf("共用块语法问题: %v", issues)
	}
	// 两档案引用同一自动块。
	for _, tt := range []string{storedbt.ToolPHPMyAdmin, storedbt.ToolMongoExpress} {
		a, _ := store.Tool().GetByType(ctx, tt)
		ref, err := parseSiteRef(a.SiteRef)
		if err != nil || ref.File != testLandingConf || ref.Port != ToolLandingPort || !ref.AutoCreated {
			t.Fatalf("%s siteRef = %+v err=%v", tt, ref, err)
		}
	}
}

// TestAutoLandingExistingBlockPriority 既有匹配块优先（D-19：不新建）；
// 既有块 siteRef port=0/autoCreated=false（旧形态缺省语义）。
func TestAutoLandingExistingBlockPriority(t *testing.T) {
	f := defaultFakes() // panel.conf server_name 匹配 BindDomain
	svc, store := newTestService(t, f)
	ctx := context.Background()
	installAutoTool(t, svc, storedbt.ToolPHPMyAdmin, nil)
	if _, ok := f.pipeline.files[testLandingConf]; ok {
		t.Fatal("既有匹配块优先——不得新建自动块")
	}
	if f.deploy.probeCalls != 0 {
		t.Fatal("既有块路径不应探针")
	}
	a, _ := store.Tool().GetByType(ctx, storedbt.ToolPHPMyAdmin)
	ref, _ := parseSiteRef(a.SiteRef)
	if ref.File != testPanelConf || ref.Port != 0 || ref.AutoCreated {
		t.Fatalf("既有块 siteRef = %+v", ref)
	}
}

// TestAutoLandingCleanupOnUninstall 卸载清块（D-19⑥/I-C04）：另一工具
// 仍在装→共享块保留；两工具均卸载→ConfigRemove 整块移除；非自动块绝不
// 动（既有块形态卸载无 remove 调用）。
func TestAutoLandingCleanupOnUninstall(t *testing.T) {
	f := autoLandingFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	installAutoTool(t, svc, storedbt.ToolPHPMyAdmin, nil)
	installAutoTool(t, svc, storedbt.ToolMongoExpress, ptrInt64(7))

	un, _ := svc.Uninstall(ctx, testActor(), storedbt.ToolPHPMyAdmin)
	svc.runTask(un.TaskID)
	if task, _ := store.Task().GetByID(ctx, un.TaskID); task.Status != storedbt.TaskSucceeded {
		t.Fatalf("uninstall pma = %+v", task)
	}
	if _, ok := f.pipeline.files[testLandingConf]; !ok {
		t.Fatal("mongo 仍在装——共享块应保留")
	}
	for _, c := range f.pipeline.applies {
		if strings.HasPrefix(c, "remove:") {
			t.Fatal("另一工具仍在装不得清块")
		}
	}
	un2, _ := svc.Uninstall(ctx, testActor(), storedbt.ToolMongoExpress)
	svc.runTask(un2.TaskID)
	if task, _ := store.Task().GetByID(ctx, un2.TaskID); task.Status != storedbt.TaskSucceeded {
		t.Fatalf("uninstall mongo = %+v", task)
	}
	if _, ok := f.pipeline.files[testLandingConf]; ok {
		t.Fatal("两工具均卸载——自动块应移除")
	}
	var removed bool
	for _, c := range f.pipeline.applies {
		if c == "remove:"+testLandingConf {
			removed = true
		}
	}
	if !removed {
		t.Fatalf("清块应经 ConfigRemove: %v", f.pipeline.applies)
	}
	// 非自动块形态：卸载不触及整块（byte 级还原沿既有 span 机制）。
	f2 := defaultFakes()
	svc2, _ := newTestService(t, f2)
	installAutoTool(t, svc2, storedbt.ToolPHPMyAdmin, nil)
	un3, _ := svc2.Uninstall(ctx, testActor(), storedbt.ToolPHPMyAdmin)
	svc2.runTask(un3.TaskID)
	for _, c := range f2.pipeline.applies {
		if strings.HasPrefix(c, "remove:") {
			t.Fatalf("非自动块绝不动（I-C04）: %v", f2.pipeline.applies)
		}
	}
	if got := f2.pipeline.files[testPanelConf]; got != testPanelConfBody {
		t.Fatal("既有块应 byte 级还原")
	}
}

// TestAutoLandingCleanupFailure 清块失败→任务 failed DBT_012 reason
// 「清块失败」档案不变（I-C01/I-C04）。
func TestAutoLandingCleanupFailure(t *testing.T) {
	f := autoLandingFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	installAutoTool(t, svc, storedbt.ToolPHPMyAdmin, nil)
	f.pipeline.removeErr = errors.New("reload failed (fake)")
	un, _ := svc.Uninstall(ctx, testActor(), storedbt.ToolPHPMyAdmin)
	svc.runTask(un.TaskID)
	task, _ := store.Task().GetByID(ctx, un.TaskID)
	if task.Status != storedbt.TaskFailed || task.Error == nil ||
		!strings.Contains(*task.Error, "DBT_012: 部署操作失败: 清块失败: reload failed (fake)") {
		t.Fatalf("task = %+v", task)
	}
	if a, _ := store.Tool().GetByType(ctx, storedbt.ToolPHPMyAdmin); a == nil {
		t.Fatal("清块失败档案不变（I-C01）")
	}
}

// TestDomainHookRefresh 域名联动钩子主通道（D-20③）：绑定→server_name
// +if 404 行；一致零写入（幂等）；解绑→server_name _ + if 移除；无自动块
// 零写入。
func TestDomainHookRefresh(t *testing.T) {
	f := autoLandingFakes()
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	installAutoTool(t, svc, storedbt.ToolPHPMyAdmin, nil)

	svc.OnBindDomainChanged(ctx, "tools.example.test")
	conf := f.pipeline.files[testLandingConf]
	if !strings.Contains(conf, "server_name tools.example.test;") ||
		!strings.Contains(conf, `if ($host !~* "^tools\.example\.test$") { return 404; }`) {
		t.Fatalf("绑定刷新缺失:\n%s", conf)
	}
	if issues := nginxx.ValidateSyntax(conf); len(issues) != 0 {
		t.Fatalf("刷新后语法问题: %v", issues)
	}
	// 幂等：同值重复触发零写入。
	n := len(f.pipeline.applies)
	svc.OnBindDomainChanged(ctx, "tools.example.test")
	if len(f.pipeline.applies) != n {
		t.Fatal("server_name 一致应零写入（幂等）")
	}
	// 换绑：if 行随域名更新。
	svc.OnBindDomainChanged(ctx, "other.example.test")
	conf = f.pipeline.files[testLandingConf]
	if !strings.Contains(conf, "server_name other.example.test;") ||
		!strings.Contains(conf, `if ($host !~* "^other\.example\.test$") { return 404; }`) ||
		strings.Contains(conf, "tools.example.test") {
		t.Fatalf("换绑刷新缺失:\n%s", conf)
	}
	// 解绑：server_name _ + if 行移除。
	svc.OnBindDomainChanged(ctx, "")
	conf = f.pipeline.files[testLandingConf]
	if !strings.Contains(conf, "server_name _;") || strings.Contains(conf, "if ($host") {
		t.Fatalf("解绑刷新缺失:\n%s", conf)
	}
	if issues := nginxx.ValidateSyntax(conf); len(issues) != 0 {
		t.Fatalf("解绑后语法问题: %v", issues)
	}
	// 工具 location 注入字节不受域名刷新影响（仅改 server_name/if）。
	if !hasMarker(conf, storedbt.ToolPHPMyAdmin) || !hasMarker(conf, markerKeyGate) {
		t.Fatal("域名刷新不得触及工具 location")
	}
	// 无自动块（既有面板域形态）→钩子零写入。
	f2 := defaultFakes()
	svc2, _ := newTestService(t, f2)
	n2 := len(f2.pipeline.applies)
	svc2.OnBindDomainChanged(ctx, "whatever.example.test")
	if len(f2.pipeline.applies) != n2 {
		t.Fatal("无自动块钩子应零写入")
	}
}

// TestLazyRefreshOnOpen 惰性校正兜底（D-20④）：钩子失败窗口（块
// server_name 陈旧）→open 时按当前 BindDomain 自愈刷新；刷新失败不阻断
// 打开（URL 仍按当前 BindDomain 生成）。
func TestLazyRefreshOnOpen(t *testing.T) {
	f := autoLandingFakes()
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	installAutoTool(t, svc, storedbt.ToolPHPMyAdmin, nil)
	// 模拟钩子失败窗口：外部置陈旧 server_name（未绑形态安装后绑域名，
	// 钩子未生效）。
	f.nginx.files[testLandingConf] = strings.Replace(
		f.pipeline.files[testLandingConf], "server_name _;", "server_name stale.example.test;", 1)
	f.panel.pa.BindDomain = "new.example.test"
	inst := int64(9)
	data, err := svc.Open(ctx, testActor(), storedbt.ToolPHPMyAdmin, &inst, "10.0.0.8:8687")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	conf := f.pipeline.files[testLandingConf]
	if !strings.Contains(conf, "server_name new.example.test;") ||
		!strings.Contains(conf, `if ($host !~* "^new\.example\.test$") { return 404; }`) {
		t.Fatalf("惰性校正未刷新:\n%s", conf)
	}
	if !strings.HasPrefix(data.URL, "http://new.example.test:18090/dbtool/phpmyadmin/?dg_signon=") {
		t.Fatalf("open url = %q", data.URL)
	}
	// 刷新失败不阻断打开（降级口径：slog+URL 仍按当前 BindDomain）——
	// 再置陈旧 server_name 并令 ConfigApply 失败，刷新尝试失败仍放行。
	f.pipeline.applyErr = errors.New("nginx down (fake)")
	f.nginx.files[testLandingConf] = strings.Replace(
		f.pipeline.files[testLandingConf], "server_name new.example.test;", "server_name _;", 1)
	data, err = svc.Open(ctx, testActor(), storedbt.ToolPHPMyAdmin, &inst, "10.0.0.8:8687")
	if err != nil {
		t.Fatalf("刷新失败不应阻断打开: %v", err)
	}
	if !strings.HasPrefix(data.URL, "http://new.example.test:18090/") {
		t.Fatalf("open url = %q", data.URL)
	}
}

// TestToolBaseHostPortBranch D-23 端口分支表驱动：port≠0→scheme://
// hostname:port（BindDomain 优先否则 requestHost 去端口）；tlsPort≠0→
// https://hostname:tlsPort 覆盖（D-24，仅自动块）；port=0 沿旧形态。
func TestToolBaseHostPortBranch(t *testing.T) {
	f := defaultFakes()
	svc, _ := newTestService(t, f)
	auto := siteRef{File: testLandingConf, Scheme: "http", Port: ToolLandingPort, AutoCreated: true}
	cases := []struct {
		name        string
		ref         siteRef
		pa          PanelAccess
		requestHost string
		tlsPort     int
		wantBase    string
		wantDynamic bool
	}{
		{"自动块+绑域名", auto, PanelAccess{BindDomain: "tools.example.test"}, "10.0.0.8:8687", 0, "http://tools.example.test:18090", false},
		{"自动块+未绑+请求Host去端口", auto, PanelAccess{}, "10.0.0.8:8687", 0, "http://10.0.0.8:18090", false},
		{"自动块+未绑+IPv6请求Host", auto, PanelAccess{}, "[::1]:8687", 0, "http://[::1]:18090", false},
		{"自动块+未绑+无请求语境→动态", auto, PanelAccess{}, "", 0, "", true},
		{"自动块+TLS+绑域名→https:2087", auto, PanelAccess{BindDomain: "tools.example.test"}, "10.0.0.8:8687", ToolLandingTLSPort, "https://tools.example.test:2087", false},
		{"自动块+TLS+未绑+请求Host去端口", auto, PanelAccess{}, "10.0.0.8:8687", ToolLandingTLSPort, "https://10.0.0.8:2087", false},
		{"既有块+tlsPort 不适用（非自动块）", siteRef{ServerName: "panel.example.test", Scheme: "https"}, PanelAccess{}, "", ToolLandingTLSPort, "https://panel.example.test", false},
		{"既有块port=0旧形态", siteRef{ServerName: "panel.example.test", Scheme: "https"}, PanelAccess{}, "", 0, "https://panel.example.test", false},
		{"既有块port=0回退BindDomain", siteRef{Scheme: "http"}, PanelAccess{BindDomain: "panel.example.test"}, "", 0, "http://panel.example.test", false},
		{"既有块port=0请求Host原样", siteRef{Scheme: "http"}, PanelAccess{}, "10.0.0.8:8687", 0, "http://10.0.0.8:8687", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base, dynamic := svc.toolBaseHost(tc.ref, tc.pa, tc.requestHost, tc.tlsPort)
			if base != tc.wantBase || dynamic != tc.wantDynamic {
				t.Fatalf("toolBaseHost = %q dynamic=%v, want %q dynamic=%v", base, dynamic, tc.wantBase, tc.wantDynamic)
			}
		})
	}
}

// TestToolListLandingFields tool/list +landingPort/autoCreated（additive
// 可空）：未装=null/null；自动块=18090/true；既有块=0/false（旧档案缺省
// 语义不虚构非零端口）。
func TestToolListLandingFields(t *testing.T) {
	f := autoLandingFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	data, _ := svc.ToolList(ctx)
	for _, it := range data.Items {
		if it.LandingPort != nil || it.AutoCreated != nil {
			t.Fatalf("未装应 null/null: %+v", it)
		}
	}
	installAutoTool(t, svc, storedbt.ToolPHPMyAdmin, nil)
	data, _ = svc.ToolList(ctx)
	for _, it := range data.Items {
		if it.ToolType != storedbt.ToolPHPMyAdmin {
			continue
		}
		if it.LandingPort == nil || *it.LandingPort != ToolLandingPort || it.AutoCreated == nil || !*it.AutoCreated {
			t.Fatalf("自动块投影 = %+v", it)
		}
	}
	// 旧格式档案（无 port/autoCreated 字段）→缺省 0/false（parseSiteRef
	// 零值向后兼容，不虚构非零端口）。
	if _, err := store.Tool().Create(ctx, storedbt.Tool{
		ToolType: storedbt.ToolMongoExpress, InstalledVersion: "1.0.0", DeployPath: "x",
		SiteRef:    `{"file":"/etc/nginx/conf.d/panel.conf","index":0,"serverName":"panel.example.test","scheme":"http"}`,
		CredCipher: "c",
	}); err != nil {
		t.Fatal(err)
	}
	data, _ = svc.ToolList(ctx)
	for _, it := range data.Items {
		if it.ToolType == storedbt.ToolMongoExpress {
			if it.LandingPort == nil || *it.LandingPort != 0 || it.AutoCreated == nil || *it.AutoCreated {
				t.Fatalf("旧档案缺省投影 = %+v", it)
			}
		}
	}
}

// TestOpenReopenAutoURL 自动块形态 open/reopen URL 同口径（D-23）：
// 未绑→请求 Host 去端口:18090；reopen 重签令牌 302 Location 同口径。
func TestOpenReopenAutoURL(t *testing.T) {
	f := autoLandingFakes()
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	installAutoTool(t, svc, storedbt.ToolPHPMyAdmin, nil)
	inst := int64(9)
	data, err := svc.Open(ctx, testActor(), storedbt.ToolPHPMyAdmin, &inst, "10.0.0.8:8687")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !strings.HasPrefix(data.URL, "http://10.0.0.8:18090/dbtool/phpmyadmin/?dg_signon=") {
		t.Fatalf("open url = %q", data.URL)
	}
	url, err := svc.Reopen(ctx, testActor(), inst, "10.0.0.8:8687")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if !strings.HasPrefix(url, "http://10.0.0.8:18090/dbtool/phpmyadmin/?dg_signon=") {
		t.Fatalf("reopen url = %q", url)
	}
	// mongo_express 自动块 URL 同口径（无令牌——basic auth 服务端注入）。
	installAutoTool(t, svc, storedbt.ToolMongoExpress, ptrInt64(7))
	mdata, err := svc.Open(ctx, testActor(), storedbt.ToolMongoExpress, nil, "10.0.0.8:8687")
	if err != nil {
		t.Fatalf("open mongo: %v", err)
	}
	if mdata.URL != "http://10.0.0.8:18090/dbtool/mongo-express/" || mdata.TokenExpiresAt != nil {
		t.Fatalf("mongo open data = %+v", mdata)
	}
}

func ptrInt64(v int64) *int64 { return &v }

// TestAutoLandingTLSInstall HTTPS 面板+证书可用形态安装（v1.4 D-24）：
// 自动块追加 listen 2087 ssl+证书落盘行+指纹注释；证书/私钥落盘 0600；
// 双端口探针；Open URL https://域名:2087；安装提示含 2087；SignonURL 仍
// 指向面板基址（不受工具 TLS 影响，A-DBT11 口径不变）。
func TestAutoLandingTLSInstall(t *testing.T) {
	f := tlsFakes()
	f.panel.pa.BindDomain = "tools.example.test"
	svc, store := newTestService(t, f)
	ctx := context.Background()
	installAutoTool(t, svc, storedbt.ToolPHPMyAdmin, nil)

	certPath := filepath.Join(svc.toolDir, LandingCertFileName)
	keyPath := filepath.Join(svc.toolDir, LandingKeyFileName)
	conf := f.pipeline.files[testLandingConf]
	for _, want := range []string{
		"listen 18090;",
		fmt.Sprintf("listen %d ssl;", ToolLandingTLSPort),
		fmt.Sprintf("ssl_certificate %q;", certPath),
		fmt.Sprintf("ssl_certificate_key %q;", keyPath),
		"# Landing cert fingerprint: ",
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("TLS 落地块缺少 %q:\n%s", want, conf)
		}
	}
	if b, ok := f.deploy.files[certPath]; !ok || !strings.Contains(string(b), "BEGIN CERTIFICATE") {
		t.Fatalf("证书未落盘: %v", f.deploy.files)
	}
	if f.deploy.modes[certPath] != 0o600 || f.deploy.modes[keyPath] != 0o600 {
		t.Fatalf("证书/私钥权限非 0600: %v %v", f.deploy.modes[certPath], f.deploy.modes[keyPath])
	}
	if f.deploy.probeCalls != 2 {
		t.Fatalf("probe calls = %d, want 2（18090+2087）", f.deploy.probeCalls)
	}
	if issues := nginxx.ValidateSyntax(conf); len(issues) != 0 {
		t.Fatalf("TLS 自动块语法问题: %v", issues)
	}
	inst := int64(9)
	data, err := svc.Open(ctx, testActor(), storedbt.ToolPHPMyAdmin, &inst, "10.0.0.8:8687")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !strings.HasPrefix(data.URL, "https://tools.example.test:2087/dbtool/phpmyadmin/?dg_signon=") {
		t.Fatalf("open url = %q", data.URL)
	}
	task, _ := store.Task().GetByID(ctx, 1)
	if !strings.Contains(task.OutputTail, "请放行 TCP 18090、2087 端口（防火墙/云安全组）") {
		t.Fatalf("outputTail 缺 2087 放行提示: %s", task.OutputTail)
	}
	cfg := string(f.deploy.files[filepath.Join(svc.toolDir, "phpmyadmin", "config.inc.php")])
	if !strings.Contains(cfg, "https://tools.example.test:8687") {
		t.Fatalf("SignonURL 面板基址缺失:\n%s", cfg)
	}
	// TLS 面板回环形态（v0.6.9 实机修复）：gate 子请求 https+proxy_ssl_verify
	// off；signon.php https+curl 校验豁免（Go TLS 监听对明文回环回 400）。
	if !strings.Contains(conf, "proxy_pass https://127.0.0.1:8687/dbtool/gate/verify;") ||
		!strings.Contains(conf, "proxy_ssl_verify off;") {
		t.Fatalf("TLS 面板 gate 回环应走 https+verify off:\n%s", conf)
	}
	signon := string(f.deploy.files[filepath.Join(svc.toolDir, "phpmyadmin", "signon.php")])
	if !strings.Contains(signon, "curl_init('https://127.0.0.1:8687/dbtool/signon/verify')") ||
		!strings.Contains(signon, "CURLOPT_SSL_VERIFYPEER => false") {
		t.Fatalf("signon.php TLS 回调形态缺失:\n%s", signon)
	}
}

// TestPanelLoopbackBaseScheme 回环基址 scheme 跟随面板 TLS 态（v0.6.9：
// TLS 面板 Go 监听只讲 TLS，明文回环请求被 Go 以 400 拒答——实机 500 根因）。
func TestPanelLoopbackBaseScheme(t *testing.T) {
	if got := panelLoopbackBase(PanelAccess{ListenPort: 8687}); got != "http://127.0.0.1:8687" {
		t.Fatalf("非 TLS 面板回环 = %q", got)
	}
	if got := panelLoopbackBase(PanelAccess{ListenPort: 2083, HTTPSEnabled: true, EntryPath: "/ent"}); got != "https://127.0.0.1:2083/ent" {
		t.Fatalf("TLS 面板回环 = %q", got)
	}
}

// TestLandingTLSUpgradeRefresh 升级自愈（v1.4 D-24）：旧版本 http-only 块
// +面板切 HTTPS/证书后装第二工具→幂等复用路径刷新补 TLS 行（工具 location
// 字节不动）；一致后 open 零写入（幂等）；证书轮换→指纹行变更→conf 重写；
// HTTPS 关闭→TLS 行+指纹注释移除、URL 回落 http。
func TestLandingTLSUpgradeRefresh(t *testing.T) {
	f := autoLandingFakes()
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	installAutoTool(t, svc, storedbt.ToolPHPMyAdmin, nil)
	if strings.Contains(f.pipeline.files[testLandingConf], "listen 2087") {
		t.Fatal("非 HTTPS 面板不应有 TLS 监听")
	}
	// 模拟升级：面板 HTTPS+证书可用+绑域名（旧 conf 不动），装第二工具。
	f.panel.pa.HTTPSEnabled = true
	f.panel.pa.BindDomain = "tools.example.test"
	f.panel.certPEM = "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"
	f.panel.keyPEM = "-----BEGIN PRIVATE KEY-----\nBBBB\n-----END PRIVATE KEY-----\n"
	installAutoTool(t, svc, storedbt.ToolMongoExpress, ptrInt64(7))
	conf := f.pipeline.files[testLandingConf]
	for _, want := range []string{"listen 2087 ssl;", "ssl_certificate ", "# Landing cert fingerprint: ",
		"server_name tools.example.test;"} {
		if !strings.Contains(conf, want) {
			t.Fatalf("升级刷新未补齐 %q:\n%s", want, conf)
		}
	}
	if issues := nginxx.ValidateSyntax(conf); len(issues) != 0 {
		t.Fatalf("升级后语法问题: %v", issues)
	}
	if !hasMarker(conf, storedbt.ToolPHPMyAdmin) || !hasMarker(conf, storedbt.ToolMongoExpress) ||
		!hasMarker(conf, markerKeyGate) {
		t.Fatal("刷新不得触及工具 location")
	}
	// 幂等：全一致 open 零写入。
	n := len(f.pipeline.applies)
	inst := int64(9)
	data, err := svc.Open(ctx, testActor(), storedbt.ToolPHPMyAdmin, &inst, "10.0.0.8:8687")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(f.pipeline.applies) != n {
		t.Fatal("TLS/server_name 全一致应零写入（幂等）")
	}
	if !strings.HasPrefix(data.URL, "https://tools.example.test:2087/") {
		t.Fatalf("open url = %q", data.URL)
	}
	// 证书轮换：指纹行变更→conf 重写（reload 载入新证书）。
	n = len(f.pipeline.applies)
	f.panel.certPEM = "-----BEGIN CERTIFICATE-----\nCCCC\n-----END CERTIFICATE-----\n"
	if _, err := svc.Open(ctx, testActor(), storedbt.ToolPHPMyAdmin, &inst, "10.0.0.8:8687"); err != nil {
		t.Fatalf("open after rotation: %v", err)
	}
	if len(f.pipeline.applies) != n+1 {
		t.Fatal("证书轮换应触发 conf 重写（指纹行变更）")
	}
	rotated := f.pipeline.files[testLandingConf]
	if issues := nginxx.ValidateSyntax(rotated); strings.Count(rotated, "Landing cert fingerprint") != 1 || len(issues) != 0 {
		t.Fatalf("轮换后指纹行应恰好一条且语法完好 issues=%v:\n%s", issues, rotated)
	}
	// HTTPS 关闭：TLS 行+指纹注释移除，URL 回落 http。
	f.panel.pa.HTTPSEnabled = false
	data, err = svc.Open(ctx, testActor(), storedbt.ToolPHPMyAdmin, &inst, "10.0.0.8:8687")
	if err != nil {
		t.Fatalf("open after https off: %v", err)
	}
	off := f.pipeline.files[testLandingConf]
	if strings.Contains(off, "2087") || strings.Contains(off, "ssl_certificate") ||
		strings.Contains(off, "Landing cert fingerprint") {
		t.Fatalf("HTTPS 关闭应移除 TLS 行与指纹注释:\n%s", off)
	}
	if issues := nginxx.ValidateSyntax(off); len(issues) != 0 {
		t.Fatalf("移除后语法问题: %v", issues)
	}
	if !strings.HasPrefix(data.URL, "http://tools.example.test:18090/") {
		t.Fatalf("open url = %q", data.URL)
	}
}

// TestOpenTLSFallbackOnRefreshFailure 刷新失败窗口 URL 以磁盘实况为准（D-24）：
// conf 已具 TLS+刷新失败→URL 仍 https；conf 无 TLS（旧块）+刷新失败→URL
// http 兜底（不虚设 https——虚设将致打开 404/拒连）。
func TestOpenTLSFallbackOnRefreshFailure(t *testing.T) {
	f := tlsFakes()
	f.panel.pa.BindDomain = "tools.example.test"
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	installAutoTool(t, svc, storedbt.ToolPHPMyAdmin, nil)
	inst := int64(9)
	f.pipeline.applyErr = errors.New("nginx down (fake)")
	data, err := svc.Open(ctx, testActor(), storedbt.ToolPHPMyAdmin, &inst, "10.0.0.8:8687")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !strings.HasPrefix(data.URL, "https://tools.example.test:2087/") {
		t.Fatalf("磁盘具 TLS+刷新失败应沿 https: %q", data.URL)
	}
	// 磁盘回落 http-only（模拟旧块）+刷新失败→URL http。
	f.nginx.files[testLandingConf] = stripTLSLines(f.pipeline.files[testLandingConf])
	data, err = svc.Open(ctx, testActor(), storedbt.ToolPHPMyAdmin, &inst, "10.0.0.8:8687")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !strings.HasPrefix(data.URL, "http://tools.example.test:18090/") {
		t.Fatalf("磁盘无 TLS+刷新失败应沿 http 兜底: %q", data.URL)
	}
}

// TestToolListTlsField tool/list +landingTlsPort（v1.4 D-24 additive 可空）：
// HTTPS 面板+证书可用+自动块=2087；非 HTTPS 面板=null；未装=null。
func TestToolListTlsField(t *testing.T) {
	f := tlsFakes()
	f.panel.pa.BindDomain = "tools.example.test"
	svc, _ := newTestService(t, f)
	ctx := context.Background()
	installAutoTool(t, svc, storedbt.ToolPHPMyAdmin, nil)
	data, _ := svc.ToolList(ctx)
	for _, it := range data.Items {
		if it.ToolType != storedbt.ToolPHPMyAdmin {
			continue
		}
		if it.LandingTLSPort == nil || *it.LandingTLSPort != ToolLandingTLSPort {
			t.Fatalf("TLS 投影 = %+v", it)
		}
	}
	// 非 HTTPS 面板 → null。
	f2 := autoLandingFakes()
	svc2, _ := newTestService(t, f2)
	installAutoTool(t, svc2, storedbt.ToolPHPMyAdmin, nil)
	data2, _ := svc2.ToolList(ctx)
	for _, it := range data2.Items {
		if it.ToolType != storedbt.ToolPHPMyAdmin {
			continue
		}
		if it.LandingTLSPort != nil {
			t.Fatalf("非 HTTPS 应 null: %+v", it)
		}
	}
	// 未装 → null。
	f3 := tlsFakes()
	svc3, _ := newTestService(t, f3)
	data3, _ := svc3.ToolList(context.Background())
	for _, it := range data3.Items {
		if it.LandingTLSPort != nil {
			t.Fatalf("未装应 null: %+v", it)
		}
	}
}
