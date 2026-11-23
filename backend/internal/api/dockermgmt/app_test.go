// app_test.go 是 v1.2 面板应用与 Compose 生命周期能力的 HTTP 全栈用例
// （ADR-033）：鉴权边界（未登录 / readonly / 缺 CSRF）+ 管理员两条链路——
// 「创建应用 → 清单 → 停止 → 日志 → 删除」与「创建项目 → 零容器仍可见 →
// 启动 → 删除」，并断言落盘结果与通道调用。
package dockermgmt

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dougong/dougong/internal/adapter/dockerx"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	dockersvc "github.com/dougong/dougong/internal/service/dockermgmt"
	"gopkg.in/yaml.v3"
)

var appWriteEndpoints = []struct {
	path string
	body map[string]any
}{
	{"/docker/app/create", map[string]any{"name": "x", "image": "nginx:alpine", "hostPort": 18110, "containerPort": 80}},
	{"/docker/app/service", map[string]any{"name": "x", "op": "stop"}},
	{"/docker/app/delete", map[string]any{"name": "x"}},
	{"/docker/app/site", map[string]any{"name": "x", "domains": []string{"a.example.com"}}},
	{"/docker/compose/delete", map[string]any{"project": "x"}},
}

// TestAppEndpointsGuard 五个写端点：未登录 AUTH_001 / readonly AUTH_008 / 缺 CSRF
// AUTH_003，且零写触达、零审计。
func TestAppEndpointsGuard(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	readonly := loginReadonly(t, app, admin)

	for _, e := range appWriteEndpoints {
		wantCode(t, doPost(t, app, e.path, nil, e.body), "AUTH_001")
		wantCode(t, doPost(t, app, e.path, &readonly, e.body), "AUTH_008")
	}
	b, _ := json.Marshal(appWriteEndpoints[0].body)
	req, _ := http.NewRequest("POST", app.ts.URL+appWriteEndpoints[0].path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	req.Header.Set("Origin", app.ts.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post without csrf: %v", err)
	}
	wantCode(t, decodeEnv(t, resp), "AUTH_003")

	app.comp.mu.Lock()
	runs := len(app.comp.runs)
	app.comp.mu.Unlock()
	if runs != 0 || app.rec.count() != 0 {
		t.Fatalf("越界/缺 CSRF 应零触达零审计：runs=%d audit=%d", runs, app.rec.count())
	}
	// 观测类端点两角色可看
	wantCode(t, doGet(t, app, "/docker/app/list", &readonly), "0")
}

// lastRun 最近一次 Compose 通道调用（project:verb）。
func lastRun(t *testing.T, app *testApp) string {
	t.Helper()
	app.comp.mu.Lock()
	defer app.comp.mu.Unlock()
	if len(app.comp.runs) == 0 {
		t.Fatal("期望至少一次通道调用，实际为零")
	}
	return app.comp.runs[len(app.comp.runs)-1]
}

func TestAppEndpointsAdminChain(t *testing.T) {
	root := t.TempDir()
	app := newTestApp(t, dockersvc.WithComposeRootDir(root))
	admin := initWizardAndLogin(t, app)

	// 创建应用：落盘编排（归属标签 + 只绑回环的端口映射）并以 up 启动。
	env := doPost(t, app, "/docker/app/create", &admin, map[string]any{
		"name": "myapp", "image": "nginx:alpine", "hostPort": 18111, "containerPort": 8080,
		"env": []map[string]any{{"key": "GREETING", "value": "hello"}},
	})
	wantCode(t, env, "0")
	var created struct {
		Name       string `json:"name"`
		WorkingDir string `json:"workingDir"`
		ConfigFile string `json:"configFile"`
		HostPort   int    `json:"hostPort"`
		State      string `json:"state"`
	}
	if err := json.Unmarshal(env.Data, &created); err != nil || created.Name != "myapp" ||
		created.HostPort != 18111 || created.ConfigFile != filepath.Join(root, "myapp", "docker-compose.yml") {
		t.Fatalf("app create data = %s (err=%v)", env.Data, err)
	}
	if got := lastRun(t, app); got != "myapp:"+dockerx.VerbUp {
		t.Fatalf("创建后应以 up 启动: %s", got)
	}
	raw, err := os.ReadFile(created.ConfigFile)
	if err != nil {
		t.Fatalf("应用编排未落盘: %v", err)
	}
	var doc struct {
		Services map[string]struct {
			Ports  []string          `yaml:"ports"`
			Env    map[string]string `yaml:"environment"`
			Labels map[string]string `yaml:"labels"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("应用编排不是合法 YAML: %v\n%s", err, raw)
	}
	svcDef := doc.Services["app"]
	if len(doc.Services) != 1 || len(svcDef.Ports) != 1 || svcDef.Ports[0] != "127.0.0.1:18111:8080" ||
		svcDef.Labels["dougong.app"] != "myapp" || svcDef.Env["GREETING"] != "hello" {
		t.Fatalf("应用编排内容错误: %+v\n%s", svcDef, raw)
	}

	// 清单：应用页签可见；Compose 列表里不出现（归「应用」管理）。
	env = doGet(t, app, "/docker/app/list", &admin)
	wantCode(t, env, "0")
	var apps struct {
		Items []struct {
			Name     string `json:"name"`
			HostPort int    `json:"hostPort"`
			State    string `json:"state"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &apps); err != nil || apps.Total != 1 || apps.Items[0].Name != "myapp" ||
		apps.Items[0].HostPort != 18111 {
		t.Fatalf("app list = %s (err=%v)", env.Data, err)
	}
	env = doGet(t, app, "/docker/compose/list", &admin)
	wantCode(t, env, "0")
	if strings.Contains(string(env.Data), `"project":"myapp"`) {
		t.Fatalf("应用不应出现在 Compose 列表: %s", env.Data)
	}

	// 停止（保留容器）/ 日志 / 重启。
	wantCode(t, doPost(t, app, "/docker/app/service", &admin, map[string]any{"name": "myapp", "op": "stop"}), "0")
	if got := lastRun(t, app); got != "myapp:"+dockerx.VerbStop {
		t.Fatalf("应用停止应用 stop 动词: %s", got)
	}
	wantCode(t, doGet(t, app, "/docker/app/logs?name=myapp&tail=200", &admin), "0")
	wantCode(t, doPost(t, app, "/docker/app/service", &admin, map[string]any{"name": "myapp", "op": "pause"}), "DK_001")
	// 应用不能经 Compose 删除入口删除；建站通道未装配时如实拒绝。
	wantCode(t, doPost(t, app, "/docker/compose/delete", &admin, map[string]any{"project": "myapp"}), "DK_014")
	wantCode(t, doPost(t, app, "/docker/app/site", &admin, map[string]any{"name": "myapp"}), "DK_030")

	// 删除应用：down + 目录移除；再次删除 DK_028。
	wantCode(t, doPost(t, app, "/docker/app/delete", &admin, map[string]any{"name": "myapp"}), "0")
	if got := lastRun(t, app); got != "myapp:"+dockerx.VerbDown {
		t.Fatalf("删除前应 down: %s", got)
	}
	if _, err := os.Stat(created.WorkingDir); !os.IsNotExist(err) {
		t.Fatalf("应用目录应被删除（stat err=%v）", err)
	}
	wantCode(t, doPost(t, app, "/docker/app/delete", &admin, map[string]any{"name": "myapp"}), "DK_028")
}

// TestComposeLifecycleOverHTTP Compose 项目生命周期（D0 Q4 三项）：autoUp=false
// 创建 → 零容器项目在列表可见（此前不可见）→ 可再次启动 → 可删除。
func TestComposeLifecycleOverHTTP(t *testing.T) {
	root := t.TempDir()
	app := newTestApp(t, dockersvc.WithComposeRootDir(root))
	admin := initWizardAndLogin(t, app)

	wantCode(t, doPost(t, app, "/docker/compose/create", &admin, map[string]any{
		"projectName": "ghost", "composeYaml": "services:\n  web:\n    image: busybox\n", "autoUp": false}), "0")
	env := doGet(t, app, "/docker/compose/list", &admin)
	wantCode(t, env, "0")
	var list struct {
		Items []struct {
			Project      string `json:"project"`
			Locatable    bool   `json:"locatable"`
			PanelManaged bool   `json:"panelManaged"`
			Containers   []any  `json:"containers"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &list); err != nil {
		t.Fatalf("compose list = %s (err=%v)", env.Data, err)
	}
	// 夹具自带一个预置项目 web；这里只看新建的 ghost（零容器：此前在列表中不可见）。
	var ghost *struct {
		Project      string `json:"project"`
		Locatable    bool   `json:"locatable"`
		PanelManaged bool   `json:"panelManaged"`
		Containers   []any  `json:"containers"`
	}
	for i := range list.Items {
		if list.Items[i].Project == "ghost" {
			ghost = &list.Items[i]
		}
	}
	if ghost == nil {
		t.Fatalf("零容器的面板项目应出现在列表: %s", env.Data)
	}
	if g := ghost; !g.Locatable || !g.PanelManaged || len(g.Containers) != 0 {
		t.Fatalf("零容器的面板项目应可定位且标记为面板创建: %+v", g)
	}
	wantCode(t, doPost(t, app, "/docker/compose/up", &admin, map[string]any{"project": "ghost"}), "0")
	if got := lastRun(t, app); got != "ghost:"+dockerx.VerbUp {
		t.Fatalf("零容器项目应可再次启动: %s", got)
	}
	wantCode(t, doPost(t, app, "/docker/compose/down", &admin, map[string]any{"project": "ghost"}), "0")
	env = doPost(t, app, "/docker/compose/delete", &admin, map[string]any{"project": "ghost"})
	wantCode(t, env, "0")
	if !strings.Contains(string(env.Data), `"deletedDir":true`) {
		t.Fatalf("面板项目的删除响应应标注目录已删除: %s", env.Data)
	}
	if _, err := os.Stat(filepath.Join(root, "ghost")); !os.IsNotExist(err) {
		t.Fatalf("项目目录应被删除（stat err=%v）", err)
	}
	wantCode(t, doPost(t, app, "/docker/compose/delete", &admin, map[string]any{"project": "ghost"}), "DK_014")
}
