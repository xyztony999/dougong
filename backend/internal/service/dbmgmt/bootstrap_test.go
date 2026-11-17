package dbmgmt

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/dbmysql"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// 引导窄接口替身（零 runner 实况引擎依赖，CI 第 7 起预防；治理 Ping/version
// 复用 dbmgmt_test.go 的 fakeMySQL/fakeRedis/fakeMongo）。

type fakeMySQLBoot struct {
	exists bool
	err    error
	calls  int
	socket string
	user   string
	pwd    string
}

func (f *fakeMySQLBoot) BootstrapAdminUser(ctx context.Context, socketPath, username, password string) (bool, error) {
	f.calls++
	f.socket, f.user, f.pwd = socketPath, username, password
	return f.exists, f.err
}

type fakeRedisBoot struct {
	alreadySet bool
	loopback   bool
	err        error
	calls      int
	pwd        string
}

func (f *fakeRedisBoot) BootstrapRequirepass(ctx context.Context, host string, port int, newPassword string) (bool, bool, error) {
	f.calls++
	f.pwd = newPassword
	return f.alreadySet, f.loopback, f.err
}

type fakeMongoBoot struct {
	exists bool
	err    error
	calls  int
	user   string
	pwd    string
}

func (f *fakeMongoBoot) BootstrapAdminUser(ctx context.Context, host string, port int, username, password string) (bool, error) {
	f.calls++
	f.user, f.pwd = username, password
	return f.exists, f.err
}

// bootEnv 构造装配引导替身的测试服务。
func bootEnv(t *testing.T, mb MySQLBootstrap, rb RedisBootstrap, mg MongoBootstrap) *testEnv {
	t.Helper()
	e := newTestEnv(t)
	e.svc.SetBootstrapAdapters(mb, rb, mg)
	e.svc.bootReadyTimeout = 50 * time.Millisecond // 缩短重试窗口（测试免等 60s）
	return e
}

func TestBootstrapMySQLSucceeded(t *testing.T) {
	mb := &fakeMySQLBoot{exists: false}
	e := bootEnv(t, mb, &fakeRedisBoot{loopback: true}, &fakeMongoBoot{})
	ctx := context.Background()
	out, err := e.svc.BootstrapAfterInstall(ctx, storedb.EngineMySQL, "admin")
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if out.Status != BootstrapStatusSucceeded || out.InstanceID == nil {
		t.Fatalf("want succeeded with instanceId, got %+v", out)
	}
	if mb.calls == 0 || mb.user != BootstrapAccountName {
		t.Fatalf("引导通道未建 dougong 账户: %+v", mb)
	}
	// 密码强随机且零回显（Note 不含密码）。
	if len(mb.pwd) < 24 || strings.Contains(out.Note, mb.pwd) {
		t.Fatalf("密码强度/零回显违例: pwdLen=%d note=%q", len(mb.pwd), out.Note)
	}
	// 档案注册：cred_username=dougong + 密文可解 + reachable + version。
	key := BuildIdentityKey(storedb.EngineMySQL, storedb.ConnModeTCP, "", "localhost", 3306)
	inst, err := e.svc.store.Instance().GetByIdentity(ctx, key)
	if err != nil || inst == nil {
		t.Fatalf("档案未注册: %v", err)
	}
	if inst.CredUsername == nil || *inst.CredUsername != BootstrapAccountName || !inst.CredentialsSet() {
		t.Fatalf("凭据未落库: %+v", inst)
	}
	if inst.ConnState != storedb.ConnStateReachable {
		t.Fatalf("conn_state 应 reachable: %s", inst.ConnState)
	}
	plain, err := e.vault.Decrypt(*inst.CredCipher)
	if err != nil || plain != mb.pwd {
		t.Fatalf("密文解密不一致: %v", err)
	}
	if !e.hasAudit(ctx, ActionInstanceBootstrapped) {
		t.Fatal("缺 instance_bootstrapped 审计")
	}
}

func TestBootstrapSkippedWhenCredentialsSet(t *testing.T) {
	mb := &fakeMySQLBoot{}
	e := bootEnv(t, mb, &fakeRedisBoot{loopback: true}, &fakeMongoBoot{})
	ctx := context.Background()
	e.seedInstance(t, storedb.EngineMySQL) // 已配置凭据
	out, err := e.svc.BootstrapAfterInstall(ctx, storedb.EngineMySQL, "admin")
	if err != nil || out.Status != BootstrapStatusSkipped {
		t.Fatalf("已配置凭据应 skipped: %+v err=%v", out, err)
	}
	if mb.calls != 0 {
		t.Fatalf("已配置凭据不应触发引导通道: calls=%d", mb.calls)
	}
}

func TestBootstrapMySQLAccountExists(t *testing.T) {
	mb := &fakeMySQLBoot{exists: true}
	e := bootEnv(t, mb, &fakeRedisBoot{loopback: true}, &fakeMongoBoot{})
	out, err := e.svc.BootstrapAfterInstall(context.Background(), storedb.EngineMySQL, "admin")
	if err != nil || out.Status != BootstrapStatusSkipped {
		t.Fatalf("账户已存在应 skipped: %+v err=%v", out, err)
	}
}

func TestBootstrapMySQLVerifyFailed(t *testing.T) {
	mb := &fakeMySQLBoot{exists: false}
	e := bootEnv(t, mb, &fakeRedisBoot{loopback: true}, &fakeMongoBoot{})
	e.mysql.pingErr = dbmysql.ErrAuthRejected // 治理通道验证失败
	out, err := e.svc.BootstrapAfterInstall(context.Background(), storedb.EngineMySQL, "admin")
	if err != nil || out.Status != BootstrapStatusFailed {
		t.Fatalf("验证失败应 failed: %+v err=%v", out, err)
	}
	// 不写未验证凭据（D-03 哲学）。
	key := BuildIdentityKey(storedb.EngineMySQL, storedb.ConnModeTCP, "", "localhost", 3306)
	inst, _ := e.svc.store.Instance().GetByIdentity(context.Background(), key)
	if inst != nil && inst.CredentialsSet() {
		t.Fatal("验证失败不应落库凭据")
	}
}

func TestBootstrapMySQLConnRetryThenFail(t *testing.T) {
	mb := &fakeMySQLBoot{err: dbmysql.ErrConnFailed} // 连接类错误触发有界重试
	e := bootEnv(t, mb, &fakeRedisBoot{loopback: true}, &fakeMongoBoot{})
	out, err := e.svc.BootstrapAfterInstall(context.Background(), storedb.EngineMySQL, "admin")
	if err != nil || out.Status != BootstrapStatusFailed {
		t.Fatalf("连接失败重试后应 failed: %+v err=%v", out, err)
	}
	if mb.calls < 2 {
		t.Fatalf("连接类错误应有界重试（calls>=2）: calls=%d", mb.calls)
	}
}

func TestBootstrapRedisSucceeded(t *testing.T) {
	rb := &fakeRedisBoot{alreadySet: false, loopback: true}
	e := bootEnv(t, &fakeMySQLBoot{}, rb, &fakeMongoBoot{})
	out, err := e.svc.BootstrapAfterInstall(context.Background(), storedb.EngineRedis, "admin")
	if err != nil || out.Status != BootstrapStatusSucceeded {
		t.Fatalf("redis 引导应 succeeded: %+v err=%v", out, err)
	}
	key := BuildIdentityKey(storedb.EngineRedis, storedb.ConnModeTCP, "", "localhost", 6379)
	inst, _ := e.svc.store.Instance().GetByIdentity(context.Background(), key)
	if inst == nil || inst.CredUsername == nil || *inst.CredUsername != RedisDefaultUsername {
		t.Fatalf("redis 凭据用户名应 default: %+v", inst)
	}
	if strings.Contains(out.Note, rb.pwd) {
		t.Fatal("密码零回显违例")
	}
}

func TestBootstrapRedisAlreadySet(t *testing.T) {
	rb := &fakeRedisBoot{alreadySet: true, loopback: true}
	e := bootEnv(t, &fakeMySQLBoot{}, rb, &fakeMongoBoot{})
	out, err := e.svc.BootstrapAfterInstall(context.Background(), storedb.EngineRedis, "admin")
	if err != nil || out.Status != BootstrapStatusSkipped {
		t.Fatalf("requirepass 已设应 skipped: %+v err=%v", out, err)
	}
}

func TestBootstrapMongoSucceeded(t *testing.T) {
	mg := &fakeMongoBoot{exists: false}
	e := bootEnv(t, &fakeMySQLBoot{}, &fakeRedisBoot{loopback: true}, mg)
	out, err := e.svc.BootstrapAfterInstall(context.Background(), storedb.EngineMongoDB, "admin")
	if err != nil || out.Status != BootstrapStatusSucceeded {
		t.Fatalf("mongo 引导应 succeeded: %+v err=%v", out, err)
	}
	if mg.user != BootstrapAccountName {
		t.Fatalf("mongo 账户应 dougong: %s", mg.user)
	}
	key := BuildIdentityKey(storedb.EngineMongoDB, storedb.ConnModeTCP, "", "localhost", 27017)
	inst, _ := e.svc.store.Instance().GetByIdentity(context.Background(), key)
	if inst == nil || !inst.CredentialsSet() {
		t.Fatalf("mongo 凭据未落库: %+v", inst)
	}
}

func TestBootstrapMongoExists(t *testing.T) {
	mg := &fakeMongoBoot{exists: true}
	e := bootEnv(t, &fakeMySQLBoot{}, &fakeRedisBoot{loopback: true}, mg)
	out, err := e.svc.BootstrapAfterInstall(context.Background(), storedb.EngineMongoDB, "admin")
	if err != nil || out.Status != BootstrapStatusSkipped {
		t.Fatalf("mongo 已有用户应 skipped: %+v err=%v", out, err)
	}
}

func TestBootstrapUnsupportedEngine(t *testing.T) {
	e := bootEnv(t, &fakeMySQLBoot{}, &fakeRedisBoot{loopback: true}, &fakeMongoBoot{})
	if _, err := e.svc.BootstrapAfterInstall(context.Background(), "nginx", "admin"); err == nil {
		t.Fatal("非数据库引擎应返回错误")
	}
}

func TestBootstrapAdaptersNotConfigured(t *testing.T) {
	e := newTestEnv(t) // 未 SetBootstrapAdapters
	out, err := e.svc.BootstrapAfterInstall(context.Background(), storedb.EngineMySQL, "admin")
	if err != nil || out.Status != BootstrapStatusFailed {
		t.Fatalf("未装配引导适配器应 failed: %+v err=%v", out, err)
	}
}

func TestGenBootstrapPassword(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		p, err := genBootstrapPassword()
		if err != nil {
			t.Fatalf("gen: %v", err)
		}
		if len(p) != 32 { // 24 字节 base64url = 32 字符
			t.Fatalf("密码长度应 32: %d", len(p))
		}
		if seen[p] {
			t.Fatal("随机密码重复（弱随机）")
		}
		seen[p] = true
	}
}

// 确保替身满足窄接口（编译期断言）。
var (
	_ MySQLBootstrap = (*fakeMySQLBoot)(nil)
	_ RedisBootstrap = (*fakeRedisBoot)(nil)
	_ MongoBootstrap = (*fakeMongoBoot)(nil)
)
