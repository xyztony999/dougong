// v1.3 Node 增量测试（prd FR-08/⚠️A16/A17；api-db-tools D-18 联动）：
// NodeSource 三系代配 worker 链（次序：代配先于索引刷新）、运行时型无服务
// 单元（服务操作拒绝/服务态 null）、版本经 node --version、EnvStatus 读契约。
// envpack 探测/执行/代配 IO 全注入——零 runner 实况依赖（CI 环境耦合预防）。
package envmgmt

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/dougong/dougong/internal/adapter/envpack"
	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storeenv "github.com/dougong/dougong/internal/store/envmgmt"
)

const nodeArmoredKey = "-----BEGIN PGP PUBLIC KEY BLOCK-----\nfake\n-----END PGP PUBLIC KEY BLOCK-----\n"

// nodeFakeIO 代配 IO 替身（os-release 预置 + 写入捕获 + 公钥下载伪化）。
type nodeFakeIO struct {
	files   map[string]string
	written map[string]string
	events  *[]string
}

func (f *nodeFakeIO) vendorIO() *envpack.VendorIO {
	return &envpack.VendorIO{
		ReadFile: func(p string) ([]byte, error) {
			if s, ok := f.files[p]; ok {
				return []byte(s), nil
			}
			return nil, errors.New("not found")
		},
		WriteFile: func(p string, b []byte, _ fs.FileMode) error {
			f.written[p] = string(b)
			f.files[p] = string(b)
			*f.events = append(*f.events, "write:"+p)
			return nil
		},
		MkdirAll: func(string, fs.FileMode) error { return nil },
		HTTPGet: func(_ context.Context, url string) ([]byte, error) {
			*f.events = append(*f.events, "http:"+url)
			return []byte(nodeArmoredKey), nil
		},
	}
}

func newNodeTestService(t *testing.T,
	run func(ctx context.Context, env map[string]string, name string, args []string) (string, error),
	vio *envpack.VendorIO) *Service {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate（含 014 八值 CHECK）: %v", err)
	}
	return New(
		envpack.NewGatewayWithVendorIO(func(string) (string, error) { return "/usr/bin/apt", nil }, run, vio),
		sysd.NewGateway(0), storeenv.New(db), Options{})
}

func nodeActor() *authsvc.Identity {
	return &authsvc.Identity{UserID: 1, Username: "admin", Role: authsvc.RoleAdmin}
}

// TestNodeInstallWorkerChain 安装链：NodeSource 代配先于索引刷新（FR-08①
// 次序）→ apt-get install nodejs → succeeded；env_repo_configured 审计在位。
func TestNodeInstallWorkerChain(t *testing.T) {
	var events []string
	fio := &nodeFakeIO{
		files:   map[string]string{"/etc/os-release": "ID=ubuntu\nVERSION_CODENAME=noble\n"},
		written: map[string]string{}, events: &events,
	}
	run := func(_ context.Context, _ map[string]string, name string, args []string) (string, error) {
		events = append(events, "cmd:"+name+" "+strings.Join(args, " "))
		switch {
		case name == "dpkg-query":
			return "dpkg-query: no packages found matching nodejs", errors.New("exit status 1")
		case name == "apt-get" && args[0] == "update":
			return "Hit:1 nodesource nodistro InRelease", nil
		case name == "apt-get" && strings.Contains(strings.Join(args, " "), " install "):
			if args[len(args)-1] != "nodejs" {
				t.Errorf("install pkg = %v, want nodejs（三系包名同）", args)
			}
			return "Setting up nodejs (22.11.0-1nodesource1)", nil
		}
		return "", errors.New("unexpected cmd " + name)
	}
	s := newNodeTestService(t, run, fio.vendorIO())
	// createTask + runTask 同步执行（确定性断言，不依赖 dispatchWorker goroutine 时序）。
	taskID, err := s.createTask(context.Background(), nodeActor(), storeenv.EnvNode,
		storeenv.ActionInstall, AuditActionEnvInstallStarted, map[string]any{"envType": storeenv.EnvNode})
	if err != nil {
		t.Fatalf("createTask: %v", err)
	}
	s.runTask(taskID)

	task, err := s.store.Task().GetByID(context.Background(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != storeenv.TaskSucceeded {
		t.Fatalf("status = %s err=%v tail=%s", task.Status, task.Error, task.OutputTail)
	}
	// 次序断言：源 list 写入先于 apt-get update（新源须参与本次索引刷新）。
	wi, ui := -1, -1
	for i, e := range events {
		if e == "write:/etc/apt/sources.list.d/nodesource.list" && wi < 0 {
			wi = i
		}
		if strings.HasPrefix(e, "cmd:apt-get update") && ui < 0 {
			ui = i
		}
	}
	if wi < 0 || ui < 0 || wi > ui {
		t.Fatalf("代配先于索引刷新次序违反: events=%v", events)
	}
	if !strings.Contains(fio.written["/etc/apt/sources.list.d/nodesource.list"], "node_22.x nodistro main") {
		t.Fatalf("nodesource list 内容异常: %v", fio.written)
	}
	// 审计：env_repo_configured + env_install_started + env_task_finished。
	for _, action := range []string{AuditActionEnvRepoConfigured, AuditActionEnvInstallStarted, AuditActionEnvTaskFinished} {
		recs, _, serr := s.store.Audit().Search(context.Background(),
			storeauth.AuditCondition{Action: action}, 0, 10)
		if serr != nil || len(recs) == 0 {
			t.Fatalf("audit %s missing (err=%v)", action, serr)
		}
	}
	// Node 非数据库类——无引导步骤（FR-08④）。
	if task.BootstrapStatus != nil {
		t.Fatalf("node bootstrap = %v, want nil（装完即用无引导）", *task.BootstrapStatus)
	}
}

// TestNodeUninstallWorkerChain 卸载链：移除命中候选 nodejs，不代配不刷网络源。
func TestNodeUninstallWorkerChain(t *testing.T) {
	var events []string
	fio := &nodeFakeIO{files: map[string]string{}, written: map[string]string{}, events: &events}
	run := func(_ context.Context, _ map[string]string, name string, args []string) (string, error) {
		events = append(events, "cmd:"+name+" "+strings.Join(args, " "))
		switch {
		case name == "dpkg-query":
			return "install ok installed 22.11.0-1nodesource1", nil
		case name == "apt-get" && args[0] == "remove":
			if args[len(args)-1] != "nodejs" {
				t.Errorf("remove pkg = %v, want nodejs", args)
			}
			return "Removing nodejs", nil
		case name == "apt-get" && args[0] == "update":
			return "ok", nil
		}
		return "", errors.New("unexpected cmd " + name)
	}
	s := newNodeTestService(t, run, fio.vendorIO())
	taskID, err := s.createTask(context.Background(), nodeActor(), storeenv.EnvNode,
		storeenv.ActionUninstall, AuditActionEnvUninstallStarted, map[string]any{"envType": storeenv.EnvNode})
	if err != nil {
		t.Fatalf("createTask: %v", err)
	}
	s.runTask(taskID)
	task, err := s.store.Task().GetByID(context.Background(), taskID)
	if err != nil || task.Status != storeenv.TaskSucceeded {
		t.Fatalf("task = %+v err=%v", task, err)
	}
	if len(fio.written) != 0 {
		t.Fatalf("卸载不应代配源文件: %v", fio.written)
	}
}

// TestNodeServiceOpRejected ⚠️A17：运行时型无服务单元——服务操作如实拒绝
// ENV_006（不在操作对象集），不发起 systemd 调用。
func TestNodeServiceOpRejected(t *testing.T) {
	s := newNodeTestService(t, func(_ context.Context, _ map[string]string, _ string, _ []string) (string, error) {
		return "", errors.New("no cmd expected")
	}, nil)
	for _, op := range []string{"start", "stop", "restart", "enable", "disable"} {
		_, err := s.ServiceOp(context.Background(), nodeActor(), storeenv.EnvNode, op)
		var ec *errcode.Error
		if !errors.As(err, &ec) || ec.Symbol() != "ENV_006" {
			t.Fatalf("op %s: err = %v, want ENV_006", op, err)
		}
	}
}

// TestNodeEnvListVersionFromBinary FR-08②：已装判定经包数据库、版本经
// node --version（v 前缀剥离）；单元名空/服务态 null（⚠️A17）。
func TestNodeEnvListVersionFromBinary(t *testing.T) {
	run := func(_ context.Context, _ map[string]string, name string, args []string) (string, error) {
		switch {
		case name == "dpkg-query" && args[len(args)-1] == "nodejs":
			return "install ok installed 22.11.0-1nodesource1", nil
		case name == "dpkg-query":
			return "dpkg-query: no packages found matching " + args[len(args)-1], errors.New("exit status 1")
		case name == "node" && len(args) == 1 && args[0] == "--version":
			return "v22.11.0\n", nil
		}
		return "", errors.New("unexpected cmd " + name)
	}
	s := newNodeTestService(t, run, nil)
	data, err := s.EnvList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var node *EnvItem
	for i := range data.Items {
		if data.Items[i].EnvType == storeenv.EnvNode {
			node = &data.Items[i]
		}
	}
	if node == nil {
		t.Fatal("node 条目缺席")
	}
	if node.Installed == nil || !*node.Installed {
		t.Fatalf("node installed = %v, want true", node.Installed)
	}
	if node.Version == nil || *node.Version != "22.11.0" {
		t.Fatalf("node version = %v, want 22.11.0（node --version 去 v 前缀，非包版本）", node.Version)
	}
	if node.UnitName != "" || node.ServiceRunning != nil || node.ServiceEnabled != nil {
		t.Fatalf("node 运行时型应 unit 空/服务态 null: %+v", node)
	}
	// EnvStatus 读契约（db-tools 依赖就绪度消费）。
	inst, ver, ok := s.EnvStatus(context.Background(), storeenv.EnvNode)
	if !inst || ver != "22.11.0" || !ok {
		t.Fatalf("EnvStatus(node) = %v/%q/%v", inst, ver, ok)
	}
	inst, _, ok = s.EnvStatus(context.Background(), storeenv.EnvPHPFPM)
	if inst || !ok {
		t.Fatalf("EnvStatus(php_fpm) = %v/%v, want false/true（未装如实）", inst, ok)
	}
	if _, _, ok := s.EnvStatus(context.Background(), "nope"); ok {
		t.Fatal("EnvStatus 未知类型应 probeOK=false")
	}
}

// TestNodeVersionBinaryAbsent 包数据库已装但 node 二进制缺席 → version=null
// 如实不虚构（installed 从包数据库判定，FR-08②）。
func TestNodeVersionBinaryAbsent(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	run := func(_ context.Context, _ map[string]string, name string, args []string) (string, error) {
		if name == "dpkg-query" && args[len(args)-1] == "nodejs" {
			return "install ok installed 22.11.0-1nodesource1", nil
		}
		return "dpkg-query: no packages found matching " + args[len(args)-1], errors.New("exit status 1")
	}
	s := New(
		envpack.NewGatewayWithRunners(func(bin string) (string, error) {
			if bin == "node" {
				return "", errors.New("not found") // 二进制缺席
			}
			return "/usr/bin/apt", nil
		}, run),
		sysd.NewGateway(0), storeenv.New(db), Options{})
	inst, ver, ok := s.EnvStatus(context.Background(), storeenv.EnvNode)
	if !inst || ver != "" || !ok {
		t.Fatalf("EnvStatus = %v/%q/%v, want true/\"\"/true（版本不可知如实）", inst, ver, ok)
	}
}
