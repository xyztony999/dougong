// db-tools 测试替身（全注入零 runner 实况依赖——npm/php/systemd/nginx/网络
// 均不参与测试，CI 环境耦合预防沿 envpack/dbmgmt 先例）。
package dbtools

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/adapter/phpfpm"
	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storedbt "github.com/dougong/dougong/internal/store/dbtools"
)

// ---- fakeDeploy（内存文件系统 + 操作日志，白名单断言数据源）----

type fakeDeploy struct {
	files        map[string][]byte
	modes        map[string]fs.FileMode
	calls        []string
	downloadBody []byte
	downloadErr  error
	shaOK        bool
	shaErr       error
	extractErr   error
	npmErr       error
	npmRemoveErr error
	globalRoot   string
	phpExts      []string
	phpErr       error
}

func newFakeDeploy() *fakeDeploy {
	return &fakeDeploy{
		files: map[string][]byte{}, modes: map[string]fs.FileMode{},
		shaOK: true, globalRoot: "/usr/lib/node_modules",
		phpExts:      []string{"mysqli", "mbstring", "zip", "gd", "json", "curl", "openssl"},
		downloadBody: []byte("fake-tarball"),
	}
}

func (f *fakeDeploy) log(s string) { f.calls = append(f.calls, s) }

func (f *fakeDeploy) DownloadTarball(_ context.Context, url, mirror string, _ time.Duration) (string, error) {
	f.log("download " + url + " mirror=" + mirror)
	if f.downloadErr != nil {
		return "", f.downloadErr
	}
	p := "/tmp/fake-download.tar.gz"
	f.files[p] = f.downloadBody
	return p, nil
}

func (f *fakeDeploy) VerifySha256(localPath, expected string) (bool, error) {
	f.log("sha256 " + localPath)
	return f.shaOK, f.shaErr
}

func (f *fakeDeploy) ExtractTar(localPath, destDir string) error {
	f.log("extract " + localPath + " → " + destDir)
	if f.extractErr != nil {
		return f.extractErr
	}
	// 模拟 phpMyAdmin tarball 解压产物（含应被移除的 setup/）。
	f.files[filepath.Join(destDir, "index.php")] = []byte("<?php // pma")
	f.files[filepath.Join(destDir, "setup", "index.php")] = []byte("<?php // setup")
	f.files[filepath.Join(destDir, "libraries", "Common.php")] = []byte("<?php // lib")
	return nil
}

func (f *fakeDeploy) NpmInstall(_ context.Context, pkg, version, registry string, _ time.Duration) (string, error) {
	f.log("npm install " + pkg + "@" + version + " registry=" + registry)
	return "npm install ok", f.npmErr
}

func (f *fakeDeploy) NpmRemove(_ context.Context, pkg string, _ time.Duration) (string, error) {
	f.log("npm remove " + pkg)
	return "npm remove ok", f.npmRemoveErr
}

func (f *fakeDeploy) NpmGlobalRoot(_ context.Context) (string, error) {
	f.log("npm root -g")
	return f.globalRoot, nil
}

func (f *fakeDeploy) PhpExtensions(_ context.Context) ([]string, error) {
	f.log("php -m")
	return f.phpExts, f.phpErr
}

func (f *fakeDeploy) WriteSecretFile(path string, content []byte) error {
	f.log("write-secret " + path)
	f.files[path] = content
	f.modes[path] = 0o600
	return nil
}

func (f *fakeDeploy) WriteFile(path string, content []byte, mode fs.FileMode) error {
	f.log("write " + path)
	f.files[path] = content
	f.modes[path] = mode
	return nil
}

func (f *fakeDeploy) ReadFile(path string) ([]byte, error) {
	if b, ok := f.files[path]; ok {
		return b, nil
	}
	return nil, &os.PathError{Op: "read", Path: path, Err: os.ErrNotExist}
}

func (f *fakeDeploy) MkdirAll(path string, _ fs.FileMode) error {
	f.log("mkdir " + path)
	return nil
}

func (f *fakeDeploy) Remove(path string) error {
	f.log("remove " + path)
	delete(f.files, path)
	return nil
}

func (f *fakeDeploy) RemoveAll(path string) error {
	f.log("removeall " + path)
	for p := range f.files {
		if p == path || strings.HasPrefix(p, path+string(os.PathSeparator)) || strings.HasPrefix(p, path+"/") {
			delete(f.files, p)
		}
	}
	return nil
}

func (f *fakeDeploy) Rename(old, new string) error {
	f.log("rename " + old + " → " + new)
	for p, b := range f.files {
		if p == old || strings.HasPrefix(p, old+"/") {
			delete(f.files, p)
			f.files[new+strings.TrimPrefix(p, old)] = b
		}
	}
	return nil
}

// ---- 其余网关替身 ----

type fakeCreds struct {
	ready map[int64]ReadyInfo
	creds map[int64]ToolCreds
	err   error
}

func (f *fakeCreds) InstanceReadyForTool(_ context.Context, id int64) (ReadyInfo, error) {
	if f.err != nil {
		return ReadyInfo{}, f.err
	}
	if r, ok := f.ready[id]; ok {
		return r, nil
	}
	return ReadyInfo{Ready: false, Reason: "实例未纳管"}, nil
}

func (f *fakeCreds) InstanceCredentialsForTool(_ context.Context, id int64) (ToolCreds, error) {
	if f.err != nil {
		return ToolCreds{}, f.err
	}
	if c, ok := f.creds[id]; ok {
		return c, nil
	}
	return ToolCreds{}, errors.New("creds unavailable")
}

type fakePipeline struct {
	files    map[string]string
	applies  []string // file 序列
	applyErr error
}

func (f *fakePipeline) ConfigApply(_ context.Context, file, newContent string, _ bool) error {
	f.applies = append(f.applies, file)
	if f.applyErr != nil {
		return f.applyErr
	}
	f.files[file] = newContent
	return nil
}

func (f *fakePipeline) ConfigRemove(_ context.Context, file string) error {
	f.applies = append(f.applies, "remove:"+file)
	return nil
}

type fakeEnv struct{ status map[string][3]any } // installed bool, version string, ok bool

func (f *fakeEnv) EnvStatus(_ context.Context, envType string) (bool, string, bool) {
	if v, ok := f.status[envType]; ok {
		return v[0].(bool), v[1].(string), v[2].(bool)
	}
	return false, "", true
}

type fakeSysd struct {
	units      map[string]sysd.UnitDetail
	calls      []string
	startErr   error
	restartErr error
	jobOK      bool
}

func (f *fakeSysd) GetUnit(_ context.Context, name string) (sysd.UnitDetail, error) {
	if d, ok := f.units[name]; ok {
		return d, nil
	}
	return sysd.UnitDetail{Name: name, LoadState: "not-found"}, nil
}

func (f *fakeSysd) StartUnit(_ context.Context, name string) (sysd.JobResult, error) {
	f.calls = append(f.calls, "start "+name)
	if f.startErr != nil {
		return sysd.JobResult{}, f.startErr
	}
	f.units[name] = sysd.UnitDetail{Name: name, LoadState: "loaded", ActiveState: "active"}
	return sysd.JobResult{Succeeded: f.jobOK, Reason: "done", ActiveState: "active"}, nil
}

func (f *fakeSysd) StopUnit(_ context.Context, name string) (sysd.JobResult, error) {
	f.calls = append(f.calls, "stop "+name)
	f.units[name] = sysd.UnitDetail{Name: name, LoadState: "loaded", ActiveState: "inactive"}
	return sysd.JobResult{Succeeded: true, Reason: "done"}, nil
}

func (f *fakeSysd) RestartUnit(_ context.Context, name string) (sysd.JobResult, error) {
	f.calls = append(f.calls, "restart "+name)
	if f.restartErr != nil {
		return sysd.JobResult{}, f.restartErr
	}
	return sysd.JobResult{Succeeded: f.jobOK, Reason: "done", ActiveState: "active"}, nil
}

func (f *fakeSysd) EnableUnit(_ context.Context, name string) error {
	f.calls = append(f.calls, "enable "+name)
	return nil
}

func (f *fakeSysd) DisableUnit(_ context.Context, name string) error {
	f.calls = append(f.calls, "disable "+name)
	return nil
}

func (f *fakeSysd) DaemonReload(_ context.Context) error {
	f.calls = append(f.calls, "daemon-reload")
	return nil
}

type fakePhpFpm struct {
	pools []phpfpm.Pool
	err   error
}

func (f *fakePhpFpm) ListPools(_ context.Context) ([]phpfpm.Pool, error) {
	return f.pools, f.err
}

type fakeNginx struct{ files map[string]string }

func (f *fakeNginx) ScanConfDirs() ([]nginxx.FileSnapshot, error) {
	keys := make([]string, 0, len(f.files))
	for k := range f.files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]nginxx.FileSnapshot, 0, len(keys))
	for _, k := range keys {
		out = append(out, nginxx.FileSnapshot{Path: k, Content: f.files[k]})
	}
	return out, nil
}

func (f *fakeNginx) ReadFile(path string) (string, time.Time, error) {
	if c, ok := f.files[path]; ok {
		return c, time.Now(), nil
	}
	return "", time.Time{}, &os.PathError{Op: "read", Path: path, Err: os.ErrNotExist}
}

type fakePanel struct {
	pa  PanelAccess
	err error
}

func (f *fakePanel) PanelAccess(_ context.Context) (PanelAccess, error) { return f.pa, f.err }

type fakeSession struct{ sessions map[string]string } // rawToken → role

func (f *fakeSession) VerifyWs(_ context.Context, rawToken string) (int64, string, error) {
	if role, ok := f.sessions[rawToken]; ok {
		return 1, role, nil
	}
	return 0, "", errcode.ErrUnauthenticated
}

type fakeAudit struct{ entries []audit.Entry }

func (f *fakeAudit) Record(_ context.Context, e audit.Entry) error {
	f.entries = append(f.entries, e)
	return nil
}

// auditConditionAction 审计动作筛选条件（事务审计经 store 查询断言用）。
func auditConditionAction(action string) storeauth.AuditCondition {
	return storeauth.AuditCondition{Action: action}
}

// detailOf 审计 detail 文本（nil 安全）。
func detailOf(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ---- 测试装配 ----

type testFakes struct {
	deploy   *fakeDeploy
	creds    *fakeCreds
	pipeline *fakePipeline
	env      *fakeEnv
	sysd     *fakeSysd
	phpfpm   *fakePhpFpm
	nginx    *fakeNginx
	panel    *fakePanel
	session  *fakeSession
	audit    *fakeAudit
}

const testPanelConf = "/etc/nginx/conf.d/panel.conf"

const testPanelConfBody = `server {
    listen 80;
    server_name panel.example.test;
    location / {
        proxy_pass http://127.0.0.1:8687;
    }
}
`

func defaultFakes() *testFakes {
	// nginx 读取与管线写入共享同一文件实况（注入→撤除往返断言依赖）。
	confFiles := map[string]string{testPanelConf: testPanelConfBody}
	return &testFakes{
		deploy: newFakeDeploy(),
		creds: &fakeCreds{
			ready: map[int64]ReadyInfo{7: {Ready: true, Engine: "mongodb"}, 9: {Ready: true, Engine: "mysql"}},
			creds: map[int64]ToolCreds{
				7: {Username: "root", Password: "p@ss:w/ord", Host: "127.0.0.1", Port: 27017, Engine: "mongodb"},
				9: {Username: "root", Password: "mysql-pass", Host: "127.0.0.1", Port: 3306, Engine: "mysql"},
			},
		},
		pipeline: &fakePipeline{files: confFiles},
		env: &fakeEnv{status: map[string][3]any{
			"nginx": {true, "1.24.0", true}, "php_fpm": {true, "8.1", true}, "node": {true, "22.11.0", true},
		}},
		sysd:   &fakeSysd{units: map[string]sysd.UnitDetail{}, jobOK: true},
		phpfpm: &fakePhpFpm{pools: []phpfpm.Pool{{Name: "www", Socket: "/run/php/php8.1-fpm.sock"}}},
		nginx:  &fakeNginx{files: confFiles},
		panel: &fakePanel{pa: PanelAccess{
			BindDomain: "panel.example.test", ListenHost: "127.0.0.1", ListenPort: 8687,
		}},
		session: &fakeSession{sessions: map[string]string{
			"admin-token": "admin", "readonly-token": "readonly",
		}},
		audit: &fakeAudit{},
	}
}

func openTestDB(t testing.TB) *sql.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate（含 013 两表）: %v", err)
	}
	return db
}

func newTestService(t testing.TB, f *testFakes) (*Service, *storedbt.Store) {
	t.Helper()
	db := openTestDB(t)
	store := storedbt.New(db)
	vault, err := OpenToolVault(t.TempDir())
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	svc := New(store, vault, Deps{
		Deploy: f.deploy, Creds: f.creds, Pipeline: f.pipeline, Env: f.env,
		Sysd: f.sysd, PhpFpm: f.phpfpm, Nginx: f.nginx, Panel: f.panel,
		Session: f.session, Audit: f.audit,
	}, Options{DataDir: t.TempDir(), DisableDispatch: true})
	return svc, store
}
