// ToolCredentialGateway OHS 测试（api-database-mgmt §4.9 / api-db-tools D-10）：
// 就绪判定（存在/凭据/引擎/TCP 形态）+ 凭据取出（vault 解密仅服务端 I-02）。
package dbmgmt

import (
	"context"
	"testing"

	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// TestInstanceReadyForTool 就绪四形态。
func TestInstanceReadyForTool(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	mysql := e.seedInstance(t, storedb.EngineMySQL)
	info, err := e.svc.InstanceReadyForTool(ctx, mysql.ID)
	if err != nil || !info.Ready || info.Engine != storedb.EngineMySQL {
		t.Fatalf("mysql ready = %+v err=%v", info, err)
	}
	// mariadb 同族就绪。
	maria := e.seedInstance(t, storedb.EngineMariaDB)
	if info, err := e.svc.InstanceReadyForTool(ctx, maria.ID); err != nil || !info.Ready {
		t.Fatalf("mariadb ready = %+v err=%v", info, err)
	}
	// mongodb TCP 就绪。
	mongo := e.seedInstance(t, storedb.EngineMongoDB)
	if info, err := e.svc.InstanceReadyForTool(ctx, mongo.ID); err != nil || !info.Ready {
		t.Fatalf("mongodb ready = %+v err=%v", info, err)
	}
	// redis 无对应工具。
	redis := e.seedInstance(t, storedb.EngineRedis)
	info, err = e.svc.InstanceReadyForTool(ctx, redis.ID)
	if err != nil || info.Ready {
		t.Fatalf("redis should not be ready: %+v err=%v", info, err)
	}
	// 未纳管。
	info, err = e.svc.InstanceReadyForTool(ctx, 99999)
	if err != nil || info.Ready {
		t.Fatalf("absent instance: %+v err=%v", info, err)
	}
}

// TestInstanceReadyCredentialsNotSet 凭据未配置 → 未就绪（引导 database-mgmt）。
func TestInstanceReadyCredentialsNotSet(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inst := storedb.Instance{
		IdentityKey: BuildIdentityKey(storedb.EngineMySQL, storedb.ConnModeTCP, "", "127.0.0.1", 3307),
		Engine:      storedb.EngineMySQL, ConnMode: storedb.ConnModeTCP,
		Host: strPtrOf("127.0.0.1"), Port: intPtrOf(3307),
	}
	tx, err := e.svc.store.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Instance().UpsertDiscovered(ctx, inst, storedb.ConnStateReachable, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	rows, _ := e.svc.store.Instance().GetByIdentity(ctx, inst.IdentityKey)
	info, err := e.svc.InstanceReadyForTool(ctx, rows.ID)
	if err != nil || info.Ready {
		t.Fatalf("no credentials should not be ready: %+v err=%v", info, err)
	}
	// 凭据取出 likewise 拒绝。
	if _, err := e.svc.InstanceCredentialsForTool(ctx, rows.ID); err == nil {
		t.Fatal("credentials fetch should fail when not set")
	}
}

// TestInstanceReadyMongoSocket mongo-express 须 TCP（socket 模式拒绝）。
func TestInstanceReadyMongoSocket(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	sock := "/var/run/mongod.sock"
	inst := storedb.Instance{
		IdentityKey: BuildIdentityKey(storedb.EngineMongoDB, storedb.ConnModeSocket, sock, "", 0),
		Engine:      storedb.EngineMongoDB, ConnMode: storedb.ConnModeSocket,
		SocketPath: &sock,
	}
	tx, _ := e.svc.store.BeginTx(ctx)
	if _, err := tx.Instance().UpsertDiscovered(ctx, inst, storedb.ConnStateReachable, nil); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit()
	rows, _ := e.svc.store.Instance().GetByIdentity(ctx, inst.IdentityKey)
	cipher, _ := e.vault.Encrypt("mongo-pass")
	tx, _ = e.svc.store.BeginTx(ctx)
	_ = tx.Instance().UpdateCredentials(ctx, rows.ID, "root", cipher, nil)
	_ = tx.Commit()
	info, err := e.svc.InstanceReadyForTool(ctx, rows.ID)
	if err != nil || info.Ready {
		t.Fatalf("mongo socket should not be ready: %+v err=%v", info, err)
	}
}

// TestInstanceCredentialsForTool 凭据取出（vault 解密仅服务端；socket 模式
// Host=localhost/Port=0 mysqli 语义）。
func TestInstanceCredentialsForTool(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	mysql := e.seedInstance(t, storedb.EngineMySQL)
	creds, err := e.svc.InstanceCredentialsForTool(ctx, mysql.ID)
	if err != nil {
		t.Fatal(err)
	}
	if creds.Username != "root" || creds.Password != "secret-old" ||
		creds.Host != "localhost" || creds.Port != 3306 || creds.Engine != storedb.EngineMySQL {
		t.Fatalf("creds = %+v", creds)
	}
	// 未纳管 → 错误（不虚构凭据）。
	if _, err := e.svc.InstanceCredentialsForTool(ctx, 99999); err == nil {
		t.Fatal("absent instance should error")
	}
}
