// 部署链路测试（D-04/D-05/D-14/D-15/I-C01~C03）：安装/升级/卸载 worker 链、
// sha256 强制、互斥、终态档案同事务、卸载数据保全白名单回归。
package dbtools

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storedbt "github.com/dougong/dougong/internal/store/dbtools"
)

func testActor() *authsvc.Identity {
	return &authsvc.Identity{UserID: 1, Username: "admin", Role: "admin", SessionDigest: "abcd1234"}
}

func symbolOf(err error) string {
	var ec *errcode.Error
	if errors.As(err, &ec) {
		return ec.Symbol()
	}
	return ""
}

// TestInstallValidation 同步校验链（DBT_002/001/006/004/003）。
func TestInstallValidation(t *testing.T) {
	f := defaultFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	inst := int64(7)

	if _, err := svc.Install(ctx, testActor(), "nope", nil); symbolOf(err) != "DBT_002" {
		t.Fatalf("invalid type: %v", err)
	}
	// 依赖缺失（node 未装）→ DBT_001 data.missing。
	f.env.status["node"] = [3]any{false, "", true}
	_, err := svc.Install(ctx, testActor(), storedbt.ToolMongoExpress, &inst)
	if symbolOf(err) != "DBT_001" {
		t.Fatalf("deps missing: %v", err)
	}
	var ec *errcode.Error
	errors.As(err, &ec)
	missing, ok := ec.Data().(map[string]any)["missing"].([]DepMissing)
	if !ok || len(missing) == 0 || missing[0].Name != "node" {
		t.Fatalf("DBT_001 data.missing = %v", ec.Data())
	}
	f.env.status["node"] = [3]any{true, "22.11.0", true}
	// 实例未就绪 → DBT_006。
	f.creds.ready[7] = ReadyInfo{Ready: false, Reason: "凭据未配置"}
	if _, err := svc.Install(ctx, testActor(), storedbt.ToolMongoExpress, &inst); symbolOf(err) != "DBT_006" {
		t.Fatalf("instance not ready: %v", err)
	}
	f.creds.ready[7] = ReadyInfo{Ready: true, Engine: "mongodb"}
	// mongo_express 缺 instanceId → AUTH_012。
	if _, err := svc.Install(ctx, testActor(), storedbt.ToolMongoExpress, nil); symbolOf(err) != "AUTH_012" {
		t.Fatalf("missing instanceId: %v", err)
	}
	// 已装 → DBT_004（语义分流升级）。
	if _, err := store.Tool().Create(ctx, storedbt.Tool{ToolType: storedbt.ToolMongoExpress,
		InstalledVersion: "1.0.0", DeployPath: "x", SiteRef: "{}", CredCipher: "c"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Install(ctx, testActor(), storedbt.ToolMongoExpress, &inst); symbolOf(err) != "DBT_004" {
		t.Fatalf("already installed: %v", err)
	}
	// 在途互斥 → DBT_003（phpmyadmin 独立任务）。
	if _, err := store.Task().Create(ctx, storedbt.Task{ToolType: storedbt.ToolPHPMyAdmin,
		Action: storedbt.ActionInstall, RequestedBy: "admin"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Install(ctx, testActor(), storedbt.ToolPHPMyAdmin, nil); symbolOf(err) != "DBT_003" {
		t.Fatalf("in-flight mutex: %v", err)
	}
}

// TestInstallPHPMyAdminChain 安装全链（D-05/D-06）：sha256→解压→config
// signon→注入→档案密文同事务；零遥测配置断言。
func TestInstallPHPMyAdminChain(t *testing.T) {
	f := defaultFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	created, err := svc.Install(ctx, testActor(), storedbt.ToolPHPMyAdmin, nil)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	svc.runTask(created.TaskID) // 同步执行（确定性断言）

	task, err := store.Task().GetByID(ctx, created.TaskID)
	if err != nil || task.Status != storedbt.TaskSucceeded {
		t.Fatalf("task = %+v err=%v tail=%s", task, err, task.OutputTail)
	}
	a, err := store.Tool().GetByType(ctx, storedbt.ToolPHPMyAdmin)
	if err != nil || a == nil {
		t.Fatalf("archive missing (I-C01): %v", err)
	}
	if a.InstalledVersion != PHPMyAdminVersion || a.UnitName != nil || a.InstanceID != nil {
		t.Fatalf("archive = %+v", a)
	}
	// 凭据密文可解且含 32B signon 密钥（I-02：档案仅密文）。
	var cj toolCredJSON
	if err := svc.decryptToolCreds(a.CredCipher, &cj); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if key, err := hex.DecodeString(cj.SignonKey); err != nil || len(key) != 32 {
		t.Fatalf("signon key = %q err=%v", cj.SignonKey, err)
	}
	if cj.BlowfishSecret == "" {
		t.Fatal("blowfish secret missing")
	}
	// config.inc.php：signon 桥接 + SignonURL 指向 reopen + 零遥测（R-09）。
	cfg := string(f.deploy.files[filepath.Join(svc.toolDir, "phpmyadmin", "config.inc.php")])
	for _, want := range []string{
		"'auth_type'] = 'signon'", "SignonScript", "/dbtool/access/reopen?instanceId=",
		"dg_tool_instance", "$cfg['VersionCheck'] = false", "$cfg['SendErrorReports'] = 'never'",
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("config.inc.php missing %q", want)
		}
	}
	if !phpSingleQuoteBalance(cfg) {
		t.Fatalf("config.inc.php 单引号字符串未配平（v0.6.9 结构引号回归锁）:\n%s", cfg)
	}
	if f.deploy.modes[filepath.Join(svc.toolDir, "phpmyadmin", "config.inc.php")] != 0o644 {
		t.Fatal("config.inc.php should be 0644 (PHP-FPM worker readable, A-DBT5)")
	}
	// signon.php：loopback verify URL + 内部秘密头。
	signon := string(f.deploy.files[filepath.Join(svc.toolDir, "phpmyadmin", "signon.php")])
	if !strings.Contains(signon, "http://127.0.0.1:8687/dbtool/signon/verify") ||
		!strings.Contains(signon, InternalSecretHeader) ||
		// v0.6.9：SignonScript 契约——PMA include 后调用 get_login_credentials
		//（AuthenticationSignon.php:156 单参调用，$dgPass 须带默认值）。
		!strings.Contains(signon, "function get_login_credentials($dgUser, $dgPass = '')") ||
		!strings.Contains(signon, "$_SESSION['PMA_single_signon_user'] = $dgData['data']['user']") ||
		!strings.Contains(signon, "return array($_SESSION['PMA_single_signon_user'], $_SESSION['PMA_single_signon_password'])") ||
		!strings.Contains(signon, "PMA_single_signon_host") {
		t.Fatalf("signon.php content invalid")
	}
	if !phpSingleQuoteBalance(signon) {
		t.Fatalf("signon.php 单引号字符串未配平:\n%s", signon)
	}
	// setup/ 移除（加固）。
	if _, ok := f.deploy.files[filepath.Join(svc.toolDir, "phpmyadmin", "setup", "index.php")]; ok {
		t.Fatal("setup/ should be removed")
	}
	// 秘密文件 0600。
	secretPath := filepath.Join(svc.toolDir, InternalSecretName)
	if f.deploy.modes[secretPath] != 0o600 || len(f.deploy.files[secretPath]) != 64 {
		t.Fatalf("internal-secret mode/len = %v/%d", f.deploy.modes[secretPath], len(f.deploy.files[secretPath]))
	}
	// nginx 注入：gate + 工具 location + auth_request + FastCGI pool。
	conf := f.pipeline.files[testPanelConf]
	for _, want := range []string{
		"# BEGIN dougong-dbtool gate", "location = /dbtool-gate-internal", "internal;",
		"proxy_pass http://127.0.0.1:8687/dbtool/gate/verify",
		"# BEGIN dougong-dbtool phpmyadmin", "auth_request /dbtool-gate-internal",
		"fastcgi_pass unix:/run/php/php8.1-fpm.sock", "SCRIPT_FILENAME",
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("nginx conf missing %q", want)
		}
	}
	// 任务创建/终态审计走 store 同事务（TestTerminalAuditSameTx 断言）。
}

// TestInstallSha256Mismatch D-05 强制：失配=任务 failed sha256_mismatch，
// 不安装不留档案，下载文件清理。
func TestInstallSha256Mismatch(t *testing.T) {
	f := defaultFakes()
	f.deploy.shaOK = false
	svc, store := newTestService(t, f)
	ctx := context.Background()
	created, err := svc.Install(ctx, testActor(), storedbt.ToolPHPMyAdmin, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.runTask(created.TaskID)
	task, _ := store.Task().GetByID(ctx, created.TaskID)
	if task.Status != storedbt.TaskFailed || task.Error == nil || !strings.Contains(*task.Error, "sha256_mismatch") {
		t.Fatalf("task = %+v", task)
	}
	if a, _ := store.Tool().GetByType(ctx, storedbt.ToolPHPMyAdmin); a != nil {
		t.Fatal("failed install must not leave archive (I-C01)")
	}
	if _, ok := f.deploy.files["/tmp/fake-download.tar.gz"]; ok {
		t.Fatal("downloaded file should be cleaned on mismatch")
	}
	// 未发生解压/注入。
	for _, c := range f.deploy.calls {
		if strings.HasPrefix(c, "extract") {
			t.Fatal("extract must not run after sha256 mismatch")
		}
	}
	if len(f.pipeline.applies) != 0 {
		t.Fatal("nginx injection must not run after sha256 mismatch")
	}
}

// TestInstallMongoExpressChain D-07：npm 精确版本 + systemd env 注入
// （凭据不进 argv）+ bind 127.0.0.1 + baseUrl + basic auth 秘密文件。
func TestInstallMongoExpressChain(t *testing.T) {
	f := defaultFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	inst := int64(7)
	created, err := svc.Install(ctx, testActor(), storedbt.ToolMongoExpress, &inst)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	svc.runTask(created.TaskID)
	task, _ := store.Task().GetByID(ctx, created.TaskID)
	if task.Status != storedbt.TaskSucceeded {
		t.Fatalf("task = %+v tail=%s", task, task.OutputTail)
	}
	a, _ := store.Tool().GetByType(ctx, storedbt.ToolMongoExpress)
	if a == nil || a.InstanceID == nil || *a.InstanceID != 7 || a.UnitName == nil || *a.UnitName != MongoExpressUnitName {
		t.Fatalf("archive = %+v", a)
	}
	// npm argv 精确版本（R-08 参数数组；凭据不在其中）。
	var npmCall string
	for _, c := range f.deploy.calls {
		if strings.HasPrefix(c, "npm install") {
			npmCall = c
		}
	}
	if npmCall != "npm install mongo-express@1.0.0 registry=" {
		t.Fatalf("npm call = %q", npmCall)
	}
	for _, c := range f.deploy.calls {
		if strings.Contains(c, "p@ss") || strings.Contains(c, "root:") {
			t.Fatalf("credentials leaked into command argv: %q", c)
		}
	}
	// 单元文件：env 注入 + localhost 绑定 + baseUrl + 0600。bind host 不经
	// env（mongo-express 1.0.0 无 host env——写什么都不生效）。
	unit := string(f.deploy.files[MongoExpressUnitPath])
	for _, want := range []string{
		"ExecStart=/usr/bin/node /usr/lib/node_modules/mongo-express/app.js",
		`ME_CONFIG_MONGODB_URL="mongodb://root:p%40ss%3Aw%2Ford@127.0.0.1:27017/?authSource=admin"`,
		`ME_CONFIG_BASICAUTH_USERNAME="dougong"`,
		`ME_CONFIG_SITE_BASEURL="/dbtool/mongo-express/"`,
		`ME_CONFIG_SITE_PORT="8081"`,
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit missing %q:\n%s", want, unit)
		}
	}
	if strings.Contains(unit, "BINDADDRESS") || strings.Contains(unit, "BIND_ADDRESS") {
		t.Fatalf("unit 不应携带 bind env（无 env 支持，写坏变量名会静默回落 localhost）:\n%s", unit)
	}
	if f.deploy.modes[MongoExpressUnitPath] != 0o600 {
		t.Fatal("unit file must be 0600")
	}
	// v0.6.9：包根 config.js 覆盖文件钉死 127.0.0.1 回环绑定（config.default
	// 的 localhost 被 Node 17+ 解析到 ::1 单栈 IPv6 回环——nginx 127.0.0.1
	// proxy_pass 502，实机取证）。
	cfgjs := string(f.deploy.files[f.deploy.globalRoot+"/mongo-express/"+MongoExpressConfigFileName])
	for _, want := range []string{
		"require('./config.default')",
		"config.site.host = '127.0.0.1'",
		"config.site.port = 8081",
	} {
		if !strings.Contains(cfgjs, want) {
			t.Fatalf("config.js missing %q:\n%s", want, cfgjs)
		}
	}
	if f.deploy.modes[f.deploy.globalRoot+"/mongo-express/"+MongoExpressConfigFileName] != 0o644 {
		t.Fatal("config.js must be 0644 (php-free node read)")
	}
	// systemd 编排：daemon-reload → enable → start。
	joined := strings.Join(f.sysd.calls, ";")
	if !strings.Contains(joined, "daemon-reload") || !strings.Contains(joined, "enable "+MongoExpressUnitName) ||
		!strings.Contains(joined, "start "+MongoExpressUnitName) {
		t.Fatalf("sysd calls = %v", f.sysd.calls)
	}
	// basic auth 秘密文件：Basic base64(dougong:口令)，0600。
	authConf := string(f.deploy.files[filepath.Join(svc.toolDir, MongoExpressAuthConfName)])
	var cj toolCredJSON
	if err := svc.decryptToolCreds(a.CredCipher, &cj); err != nil {
		t.Fatal(err)
	}
	wantHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("dougong:"+cj.BasicAuthPassword))
	if !strings.Contains(authConf, wantHeader) || f.deploy.modes[filepath.Join(svc.toolDir, MongoExpressAuthConfName)] != 0o600 {
		t.Fatalf("auth conf invalid (mode=%v)", f.deploy.modes[filepath.Join(svc.toolDir, MongoExpressAuthConfName)])
	}
	// nginx 注入：proxy_pass 127.0.0.1:8081 + auth_request + include auth conf。
	conf := f.pipeline.files[testPanelConf]
	for _, want := range []string{
		"# BEGIN dougong-dbtool mongo_express", "proxy_pass http://127.0.0.1:8081/dbtool/mongo-express/",
		"auth_request /dbtool-gate-internal", "mongo-express-auth.conf",
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("nginx conf missing %q", want)
		}
	}
}

// TestUninstallDataPreservation 卸载数据保全回归（I-C03/D-15 安全底线）：
// 执行链白名单仅工具目录/单元/注入配置——引擎数据目录与 db_instance 零触及。
func TestUninstallDataPreservation(t *testing.T) {
	for _, toolType := range []string{storedbt.ToolPHPMyAdmin, storedbt.ToolMongoExpress} {
		t.Run(toolType, func(t *testing.T) {
			f := defaultFakes()
			svc, store := newTestService(t, f)
			ctx := context.Background()
			// 先安装（建立部署落位）。
			var inst *int64
			if toolType == storedbt.ToolMongoExpress {
				v := int64(7)
				inst = &v
			}
			created, err := svc.Install(ctx, testActor(), toolType, inst)
			if err != nil {
				t.Fatal(err)
			}
			svc.runTask(created.TaskID)
			f.deploy.calls = nil // 清零日志，仅观测卸载链
			f.sysd.calls = nil

			un, err := svc.Uninstall(ctx, testActor(), toolType)
			if err != nil {
				t.Fatal(err)
			}
			svc.runTask(un.TaskID)
			task, _ := store.Task().GetByID(ctx, un.TaskID)
			if task.Status != storedbt.TaskSucceeded {
				t.Fatalf("uninstall task = %+v", task)
			}
			// 档案删除（同事务）。
			if a, _ := store.Tool().GetByType(ctx, toolType); a != nil {
				t.Fatal("archive should be deleted on uninstall success")
			}
			// 白名单断言：全部删除操作仅落在工具目录/systemd 单元路径。
			for _, c := range f.deploy.calls {
				if strings.HasPrefix(c, "remove ") || strings.HasPrefix(c, "removeall ") {
					path := strings.TrimPrefix(strings.TrimPrefix(c, "removeall "), "remove ")
					inToolDir := strings.HasPrefix(path, svc.toolDir)
					isUnit := path == MongoExpressUnitPath
					if !inToolDir && !isUnit {
						t.Fatalf("uninstall touched non-whitelisted path: %q (I-C03)", c)
					}
					for _, forbidden := range []string{"/var/lib/mysql", "/var/lib/mongodb", "/var/lib/docker", "db_instance"} {
						if strings.Contains(path, forbidden) {
							t.Fatalf("uninstall touched engine data: %q", c)
						}
					}
				}
			}
			// nginx 注入撤除恢复原字节（byte 级回归）。
			if got := f.pipeline.files[testPanelConf]; got != testPanelConfBody {
				t.Fatalf("nginx conf not restored to original bytes:\n%q\nwant:\n%q", got, testPanelConfBody)
			}
			// mongo-express：停服+禁用+移除单元+daemon-reload 在 npm 移除前。
			if toolType == storedbt.ToolMongoExpress {
				joined := strings.Join(f.sysd.calls, ";")
				if !strings.Contains(joined, "stop "+MongoExpressUnitName) ||
					!strings.Contains(joined, "disable "+MongoExpressUnitName) {
					t.Fatalf("sysd uninstall calls = %v", f.sysd.calls)
				}
				var npmRemoved bool
				for _, c := range f.deploy.calls {
					if c == "npm remove mongo-express" {
						npmRemoved = true
					}
				}
				if !npmRemoved {
					t.Fatal("npm package should be removed")
				}
			}
		})
	}
}

// TestUpgradeValidationAndChain DBT_004/011 前置 + phpMyAdmin 升级保留配置。
func TestUpgradeValidationAndChain(t *testing.T) {
	f := defaultFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	if _, err := svc.Upgrade(ctx, testActor(), storedbt.ToolPHPMyAdmin); symbolOf(err) != "DBT_004" {
		t.Fatalf("upgrade not installed: %v", err)
	}
	// 安装后同版本升级 → DBT_011。
	created, _ := svc.Install(ctx, testActor(), storedbt.ToolPHPMyAdmin, nil)
	svc.runTask(created.TaskID)
	if _, err := svc.Upgrade(ctx, testActor(), storedbt.ToolPHPMyAdmin); symbolOf(err) != "DBT_011" {
		t.Fatalf("same version upgrade: %v", err)
	}
	// 模拟旧版本档案 → 升级链执行（配置保留）。
	if err := store.Tool().UpdateVersion(ctx, storedbt.ToolPHPMyAdmin, "5.2.1"); err != nil {
		t.Fatal(err)
	}
	oldCfg := f.deploy.files[filepath.Join(svc.toolDir, "phpmyadmin", "config.inc.php")]
	up, err := svc.Upgrade(ctx, testActor(), storedbt.ToolPHPMyAdmin)
	if err != nil {
		t.Fatal(err)
	}
	svc.runTask(up.TaskID)
	task, _ := store.Task().GetByID(ctx, up.TaskID)
	if task.Status != storedbt.TaskSucceeded {
		t.Fatalf("upgrade task = %+v tail=%s", task, task.OutputTail)
	}
	a, _ := store.Tool().GetByType(ctx, storedbt.ToolPHPMyAdmin)
	if a.InstalledVersion != PHPMyAdminVersion {
		t.Fatalf("version not updated: %s", a.InstalledVersion)
	}
	newCfg := f.deploy.files[filepath.Join(svc.toolDir, "phpmyadmin", "config.inc.php")]
	if string(newCfg) != string(oldCfg) {
		t.Fatal("config.inc.php must be preserved on upgrade (D-14)")
	}
}

// TestRotateChainAndRollback D-08/I-C02：成功→密文替换+审计脱敏；重启失败
// →文件回滚旧口令保持有效+DBT_012。
func TestRotateChainAndRollback(t *testing.T) {
	f := defaultFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	inst := int64(7)
	created, _ := svc.Install(ctx, testActor(), storedbt.ToolMongoExpress, &inst)
	svc.runTask(created.TaskID)
	a, _ := store.Tool().GetByType(ctx, storedbt.ToolMongoExpress)
	oldCipher := a.CredCipher
	authConfPath := filepath.Join(svc.toolDir, MongoExpressAuthConfName)
	oldAuthConf := string(f.deploy.files[authConfPath])
	oldUnit := string(f.deploy.files[MongoExpressUnitPath])

	// phpmyadmin 轮换 → DBT_009。
	if _, err := svc.Rotate(ctx, testActor(), storedbt.ToolPHPMyAdmin); symbolOf(err) != "DBT_009" {
		t.Fatalf("rotate phpmyadmin: %v", err)
	}
	// 重启失败 → 回滚。
	f.sysd.jobOK = false
	if _, err := svc.Rotate(ctx, testActor(), storedbt.ToolMongoExpress); symbolOf(err) != "DBT_012" {
		t.Fatalf("rotate with failed restart: %v", err)
	}
	if string(f.deploy.files[authConfPath]) != oldAuthConf {
		t.Fatal("auth.conf should be rolled back (I-C02)")
	}
	if string(f.deploy.files[MongoExpressUnitPath]) != oldUnit {
		t.Fatal("unit file should be rolled back (I-C02)")
	}
	a, _ = store.Tool().GetByType(ctx, storedbt.ToolMongoExpress)
	if a.CredCipher != oldCipher {
		t.Fatal("cred_cipher must not change on failed rotate (I-C02)")
	}
	// 成功路径。
	f.sysd.jobOK = true
	data, err := svc.Rotate(ctx, testActor(), storedbt.ToolMongoExpress)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if data.RotatedAt == "" {
		t.Fatal("rotatedAt missing")
	}
	a, _ = store.Tool().GetByType(ctx, storedbt.ToolMongoExpress)
	if a.CredCipher == oldCipher {
		t.Fatal("cred_cipher should be replaced on success")
	}
	if string(f.deploy.files[authConfPath]) == oldAuthConf {
		t.Fatal("auth.conf should carry new password")
	}
	// 审计脱敏：dbt_credential_rotated detail 不含口令（事务审计经 store）。
	recs, _, err := store.Audit().Search(ctx, auditConditionAction(AuditActionCredentialRotated), 0, 10)
	if err != nil || len(recs) != 1 {
		t.Fatalf("rotate audit: %d err=%v", len(recs), err)
	}
	var cj toolCredJSON
	_ = svc.decryptToolCreds(a.CredCipher, &cj)
	if strings.Contains(detailOf(recs[0].Detail), cj.BasicAuthPassword) || strings.Contains(detailOf(recs[0].Detail), "basicAuthPassword") {
		t.Fatalf("rotate audit detail leaks password: %s", detailOf(recs[0].Detail))
	}
}

// TestRecoverInFlight I-09：running/pending 启动恢复 failed 不续跑。
func TestRecoverInFlight(t *testing.T) {
	f := defaultFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	id1, _ := store.Task().Create(ctx, storedbt.Task{ToolType: storedbt.ToolPHPMyAdmin, Action: storedbt.ActionInstall, RequestedBy: "admin"})
	_ = store.Task().MarkRunning(ctx, id1)
	id2, _ := store.Task().Create(ctx, storedbt.Task{ToolType: storedbt.ToolMongoExpress, Action: storedbt.ActionInstall, RequestedBy: "admin"})
	svc.RecoverInFlight(ctx)
	for _, id := range []int64{id1, id2} {
		task, _ := store.Task().GetByID(ctx, id)
		if task.Status != storedbt.TaskFailed || task.Error == nil || *task.Error != "面板重启中断" {
			t.Fatalf("recovered task %d = %+v", id, task)
		}
	}
}

// TestTerminalAuditSameTx 任务创建/终态审计与台账同事务（I-08/I-C01）。
func TestTerminalAuditSameTx(t *testing.T) {
	f := defaultFakes()
	svc, store := newTestService(t, f)
	ctx := context.Background()
	created, err := svc.Install(ctx, testActor(), storedbt.ToolPHPMyAdmin, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 创建审计已在事务内（pending 态即可查）。
	recs, _, _ := store.Audit().Search(ctx, auditConditionAction(AuditActionInstallStarted), 0, 10)
	if len(recs) != 1 {
		t.Fatalf("install_started audit = %d", len(recs))
	}
	svc.runTask(created.TaskID)
	recs, _, _ = store.Audit().Search(ctx, auditConditionAction(AuditActionTaskFinished), 0, 10)
	if len(recs) != 1 {
		t.Fatalf("task_finished audit = %d", len(recs))
	}
	if !strings.Contains(detailOf(recs[0].Detail), `"status":"succeeded"`) {
		t.Fatalf("finished detail = %s", detailOf(recs[0].Detail))
	}
}
