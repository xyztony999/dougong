package envmgmt

import (
	"context"
	"errors"
	"strings"
	"testing"

	dbmgmtsvc "github.com/dougong/dougong/internal/service/dbmgmt"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storeenv "github.com/dougong/dougong/internal/store/envmgmt"
)

// fakeBoot 是 db-mgmt BootstrapAfterInstall OHS 替身（零 runner 实况依赖）。
type fakeBoot struct {
	out    dbmgmtsvc.BootstrapOutcome
	err    error
	calls  int
	engine string
	actor  string
}

func (f *fakeBoot) BootstrapAfterInstall(ctx context.Context, engine, actorName string) (dbmgmtsvc.BootstrapOutcome, error) {
	f.calls++
	f.engine, f.actor = engine, actorName
	return f.out, f.err
}

func bootTask(envType string) storeenv.Task {
	return storeenv.Task{EnvType: envType, Action: storeenv.ActionInstall, RequestedBy: "admin"}
}

func TestRunBootstrapMySQLSucceeded(t *testing.T) {
	s := newTestService(t, fakeDpkg(map[string]string{}))
	fb := &fakeBoot{out: dbmgmtsvc.BootstrapOutcome{Status: "succeeded", Note: "已创建管理账户并注册"}}
	s.bootstrap = fb
	ring := newOutputRing()
	status, note := s.runBootstrap(context.Background(), bootTask(storeenv.EnvMySQL), ring)
	if status == nil || *status != storeenv.BootstrapSucceeded {
		t.Fatalf("引导成功应定格 succeeded: %v", status)
	}
	if note == nil || !strings.Contains(*note, "已创建管理账户") {
		t.Fatalf("引导摘要缺失: %v", note)
	}
	if fb.calls != 1 || fb.engine != storeenv.EnvMySQL || fb.actor != "admin" {
		t.Fatalf("OHS 消费参数异常: %+v", fb)
	}
	if !strings.Contains(ring.String(), "[bootstrap]") {
		t.Fatalf("输出尾部缺 [bootstrap] 行: %s", ring.String())
	}
	rows, total, err := s.store.Audit().Search(context.Background(), storeauth.AuditCondition{Action: AuditActionEnvBootstrap}, 0, 10)
	if err != nil || total == 0 {
		t.Fatalf("缺 env_bootstrap 审计: total=%d err=%v", total, err)
	}
	_ = rows
}

func TestRunBootstrapNonDBNoStep(t *testing.T) {
	s := newTestService(t, fakeDpkg(map[string]string{}))
	fb := &fakeBoot{out: dbmgmtsvc.BootstrapOutcome{Status: "succeeded"}}
	s.bootstrap = fb
	ring := newOutputRing()
	status, note := s.runBootstrap(context.Background(), bootTask(storeenv.EnvNginx), ring)
	if status != nil || note != nil {
		t.Fatalf("非数据库环境无引导步骤（应 nil）: %v %v", status, note)
	}
	if fb.calls != 0 {
		t.Fatalf("非数据库不应消费 OHS: calls=%d", fb.calls)
	}
}

func TestRunBootstrapFailedIndependent(t *testing.T) {
	s := newTestService(t, fakeDpkg(map[string]string{}))
	fb := &fakeBoot{out: dbmgmtsvc.BootstrapOutcome{Status: "failed", Note: "引导通道建连失败"}}
	s.bootstrap = fb
	ring := newOutputRing()
	status, note := s.runBootstrap(context.Background(), bootTask(storeenv.EnvRedis), ring)
	if status == nil || *status != storeenv.BootstrapFailed {
		t.Fatalf("引导失败应定格 failed: %v", status)
	}
	if note == nil || !strings.Contains(*note, "建连失败") {
		t.Fatalf("失败原因缺失: %v", note)
	}
}

func TestRunBootstrapNilGateway(t *testing.T) {
	s := newTestService(t, fakeDpkg(map[string]string{}))
	s.bootstrap = nil
	ring := newOutputRing()
	status, _ := s.runBootstrap(context.Background(), bootTask(storeenv.EnvMySQL), ring)
	if status == nil || *status != storeenv.BootstrapFailed {
		t.Fatalf("未装配 OHS 应 failed: %v", status)
	}
}

func TestRunBootstrapOHSError(t *testing.T) {
	s := newTestService(t, fakeDpkg(map[string]string{}))
	fb := &fakeBoot{err: errors.New("ohs unavailable")}
	s.bootstrap = fb
	ring := newOutputRing()
	status, note := s.runBootstrap(context.Background(), bootTask(storeenv.EnvMongoDB), ring)
	if status == nil || *status != storeenv.BootstrapFailed {
		t.Fatalf("OHS 错误应 failed: %v", status)
	}
	if note == nil || !strings.Contains(*note, "ohs unavailable") {
		t.Fatalf("错误原因缺失: %v", note)
	}
}

var _ BootstrapGateway = (*fakeBoot)(nil)
