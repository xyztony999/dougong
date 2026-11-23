// create_test.go 是 v1.1 四个创建端点的 HTTP 全栈用例（鉴权边界 + 请求体→服务→
// 通道的逐字段贯通 + 落盘结果）。此前这四个端点只有接口 stub、零 HTTP 用例。
package dockermgmt

import (
	"bytes"
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

var createEndpoints = []struct {
	path string
	body map[string]any
}{
	{"/docker/container/create", map[string]any{"name": "web", "image": "nginx:1"}},
	{"/docker/volume/create", map[string]any{"name": "data"}},
	{"/docker/network/create", map[string]any{"name": "app-net"}},
	{"/docker/compose/create", map[string]any{"projectName": "demo", "composeYaml": "services: {}\n"}},
}

// TestCreateEndpointsGuard 四个创建端点：未登录 AUTH_001 / readonly AUTH_008 /
// 缺 CSRF AUTH_003，且零写触达、零审计。
func TestCreateEndpointsGuard(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	readonly := loginReadonly(t, app, admin)

	for _, e := range createEndpoints {
		wantCode(t, doPost(t, app, e.path, nil, e.body), "AUTH_001")
		wantCode(t, doPost(t, app, e.path, &readonly, e.body), "AUTH_008")
	}

	b, _ := json.Marshal(createEndpoints[0].body)
	req, _ := http.NewRequest("POST", app.ts.URL+createEndpoints[0].path, bytes.NewReader(b))
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
	if len(app.gw.created()) != 0 || app.gw.createSpec.Name != "" || runs != 0 || app.rec.count() != 0 {
		t.Fatalf("越界/缺 CSRF 应零触达零审计：created=%v spec=%+v runs=%d audit=%d",
			app.gw.created(), app.gw.createSpec, runs, app.rec.count())
	}
}

// TestCreateEndpointsAdminChain admin 全链路：请求体逐字段贯通到通道参数；校验
// 失败按语义归码；Compose 项目落盘内容含服务级归属标签并以动词 up 启动。
func TestCreateEndpointsAdminChain(t *testing.T) {
	root := t.TempDir()
	app := newTestApp(t, dockersvc.WithComposeRootDir(root))
	admin := initWizardAndLogin(t, app)
	app.gw.state["cid-new"] = "running"

	env := doPost(t, app, "/docker/container/create", &admin, map[string]any{
		"name":  "web",
		"image": "nginx:1",
		"ports": []map[string]any{{"hostPort": "8080", "containerPort": "80"},
			{"hostPort": "5353", "containerPort": "53", "protocol": "udp"}},
		"env":           []map[string]any{{"key": "TZ", "value": "Asia/Shanghai"}},
		"mounts":        []map[string]any{{"source": "data", "target": "/data", "mode": "ro"}},
		"restartPolicy": "on-failure",
		"network":       "bridge", // 基座网络的既有名
	})
	wantCode(t, env, "0")
	var container struct {
		ContainerID string `json:"containerId"`
		Name        string `json:"name"`
		State       string `json:"state"`
	}
	if err := json.Unmarshal(env.Data, &container); err != nil ||
		container.ContainerID != "cid-new" || container.Name != "web" || container.State != "running" {
		t.Fatalf("container create data = %+v (err=%v)", container, err)
	}
	spec := app.gw.createSpec
	if spec.Name != "web" || spec.Image != "nginx:1" || spec.NetworkName != "bridge" ||
		spec.RestartPolicy != "on-failure" || len(spec.PortMappings) != 2 ||
		spec.PortMappings[0].Protocol != "tcp" || spec.PortMappings[1].Protocol != "udp" ||
		len(spec.Env) != 1 || spec.Env[0] != "TZ=Asia/Shanghai" ||
		len(spec.Mounts) != 1 || spec.Mounts[0].Mode != "ro" {
		t.Fatalf("请求体未逐字段贯通到通道参数：spec=%+v", spec)
	}

	env = doPost(t, app, "/docker/container/create", &admin, map[string]any{"name": "bad name", "image": "nginx:1"})
	wantCode(t, env, "DK_019")
	env = doPost(t, app, "/docker/container/create", &admin,
		map[string]any{"name": "web2", "image": "nginx:1", "network": "ghost"})
	wantCode(t, env, "DK_009")

	env = doPost(t, app, "/docker/volume/create", &admin, map[string]any{"name": "data"})
	wantCode(t, env, "0")
	if !strings.Contains(string(env.Data), `"driver":"local"`) {
		t.Fatalf("卷驱动缺省 local：data=%s", env.Data)
	}
	env = doPost(t, app, "/docker/network/create", &admin, map[string]any{"name": "app-net"})
	wantCode(t, env, "0")
	if !strings.Contains(string(env.Data), `"driver":"bridge"`) {
		t.Fatalf("网络驱动缺省 bridge：data=%s", env.Data)
	}
	if got := app.gw.created(); len(got) != 2 || got[0] != "volume:data:local" || got[1] != "network:app-net:bridge" {
		t.Fatalf("卷/网络创建通道调用 = %v", got)
	}

	composeYaml := "services:\n  web:\n    image: nginx:1\n    ports:\n      - \"8080:80\"\n"
	env = doPost(t, app, "/docker/compose/create", &admin,
		map[string]any{"projectName": "demo", "composeYaml": composeYaml})
	wantCode(t, env, "0")
	var project struct {
		ProjectName string `json:"projectName"`
		WorkingDir  string `json:"workingDir"`
		ConfigFile  string `json:"configFile"`
		Started     bool   `json:"started"`
	}
	if err := json.Unmarshal(env.Data, &project); err != nil || !project.Started ||
		project.ProjectName != "demo" || project.ConfigFile != filepath.Join(root, "demo", "docker-compose.yml") {
		t.Fatalf("compose create data = %+v (err=%v)", project, err)
	}
	app.comp.mu.Lock()
	runs := append([]string(nil), app.comp.runs...)
	app.comp.mu.Unlock()
	if len(runs) != 1 || runs[0] != "demo:"+dockerx.VerbUp {
		t.Fatalf("compose 启动动词 = %v, want [demo:up]", runs)
	}
	raw, err := os.ReadFile(project.ConfigFile)
	if err != nil {
		t.Fatalf("compose.yml 未落盘: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("落盘文件不是合法 YAML: %v", err)
	}
	if _, topLabels := doc["labels"]; topLabels {
		t.Fatalf("顶层不得出现 labels 键（CLI 拒绝）:\n%s", raw)
	}
	svc, _ := doc["services"].(map[string]any)["web"].(map[string]any)
	labels, _ := svc["labels"].([]any)
	if len(labels) != 1 || labels[0] != "dougong.app=demo" {
		t.Fatalf("服务级归属标签缺失：labels=%v\n%s", labels, raw)
	}

	// YAML 错误 → DK_026 + data.reason；重名 → DK_027。
	env = doPost(t, app, "/docker/compose/create", &admin,
		map[string]any{"projectName": "demo2", "composeYaml": "services:\n  web:\n    image: [x\n"})
	wantCode(t, env, "DK_026")
	if !strings.Contains(string(env.Data), "YAML 语法错误") {
		t.Fatalf("DK_026 data 应带原因：data=%s", env.Data)
	}
	if _, err := os.Stat(filepath.Join(root, "demo2")); !os.IsNotExist(err) {
		t.Fatalf("校验失败不应留目录：stat err=%v", err)
	}
	env = doPost(t, app, "/docker/compose/create", &admin,
		map[string]any{"projectName": "demo", "composeYaml": composeYaml})
	wantCode(t, env, "DK_027")

	// 审计恰 8 条：容器 ok / 容器名 fail / 网络 fail / 卷 ok / 网络 ok /
	// compose ok / YAML fail / 重名 fail。
	if n := app.rec.count(); n != 8 {
		t.Fatalf("审计 = %d, want 8", n)
	}
}
