package dbmgmt

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/adapter/dbmongo"
	"github.com/dougong/dougong/internal/adapter/dbmysql"
	"github.com/dougong/dougong/internal/adapter/dbredis"
	"github.com/dougong/dougong/internal/service/audit"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// 本文件是 database-mgmt 对 env-mgmt 的进程内 OHS（api-database-mgmt.md §4.6/
// D-20，安装后配置就绪增量）：数据库类环境安装成功后，env-mgmt worker 消费
// BootstrapAfterInstall 完成「建管理账户/设随机强密码 + 注册实例与凭据」，使面板
// 装完即可治理。单向依赖 env-mgmt→db-mgmt（与 cron-backup 消费 DumpStream 同构）。
//
// 引导通道区别于 TCP+密码治理通道：MySQL unix socket root auth_socket 免密 /
// Redis 免认证 / MongoDB localhost exception 免认证。幂等保守不覆盖用户已有配置
// （⚠️A13）；治理通道 TCP 验证后落库（D-03 先验证后落库哲学）；密码 crypto/rand
// 随机强密码 AES-256-GCM 密文落库零回显（I-02，BootstrapOutcome.Note/审计/日志
// 均不含密码）。服务就绪（start/等待）归 env-mgmt，本 OHS 不操作进程。

// ---- 引导常量（D-20）----

const (
	// BootstrapAccountName 引导创建的管理账户名（MySQL/MongoDB；Q1 拍板 dougong）。
	BootstrapAccountName = "dougong"
	// BootstrapPasswordBytes 随机强密码随机字节数（crypto/rand → base64url 32 字符）。
	BootstrapPasswordBytes = 24
	// BootstrapReadyTimeout 等待引擎就绪上限（D-19；间隔 BootstrapRetryInterval 重试）。
	BootstrapReadyTimeout = 60 * time.Second
	// BootstrapRetryInterval 引擎就绪重试间隔。
	BootstrapRetryInterval = 2 * time.Second
)

// 引导结果状态（与 env-mgmt bootstrap_status 同值域，api §4.6）。
const (
	BootstrapStatusSucceeded = "succeeded"
	BootstrapStatusFailed    = "failed"
	BootstrapStatusSkipped   = "skipped"
)

// ActionInstanceBootstrapped 引导注册审计动作码（D-20；detail 不含密码，脱敏）。
const ActionInstanceBootstrapped = "instance_bootstrapped"

// BootstrapOutcome 是引导结果（回传 env-mgmt 任务台账 bootstrap_status/note）。
// Note 为人类可读摘要/原因，**不含密码/密文/密钥**（I-02 零出口）。
type BootstrapOutcome struct {
	Status     string // succeeded | failed | skipped
	Note       string
	InstanceID *int64 // 注册成功（succeeded）或已有档案（skipped）时指向 db_instance.id
}

// ---- 引擎引导窄接口（适配器实现，main 注入；与治理接口分离——既有治理测试
// 不受影响，引导测试用替身，零 runner 实况依赖，CI 第 7 起预防）----

// MySQLBootstrap 经 unix socket root auth_socket 免密建管理账户（§4.6）。
type MySQLBootstrap interface {
	BootstrapAdminUser(ctx context.Context, socketPath, username, password string) (exists bool, err error)
}

// RedisBootstrap 免认证设 requirepass + CONFIG REWRITE（§4.6）。
type RedisBootstrap interface {
	BootstrapRequirepass(ctx context.Context, host string, port int, newPassword string) (alreadySet bool, bindLoopback bool, err error)
}

// MongoBootstrap 经 localhost exception 建 admin root 用户（§4.6）。
type MongoBootstrap interface {
	BootstrapAdminUser(ctx context.Context, host string, port int, username, password string) (exists bool, err error)
}

// SetBootstrapAdapters 注入引擎引导窄接口（main 装配：与治理网关同一适配器实例；
// 测试注入替身）。未注入时 BootstrapAfterInstall 返回 failed（通道未装配）。
func (s *Service) SetBootstrapAdapters(mb MySQLBootstrap, rb RedisBootstrap, mg MongoBootstrap) {
	s.mysqlBoot, s.redisBoot, s.mongoBoot = mb, rb, mg
}

// bootstrapTarget 本机默认治理目标（TCP；注册 identity 与治理通道一致，D-20）。
func bootstrapTarget(engine string) (dbconn.Target, bool) {
	switch engine {
	case storedb.EngineMySQL, storedb.EngineMariaDB:
		return dbconn.Target{Engine: storedb.EngineMySQL, ConnMode: storedb.ConnModeTCP, Host: "localhost", Port: 3306}, true
	case storedb.EngineRedis:
		return dbconn.Target{Engine: storedb.EngineRedis, ConnMode: storedb.ConnModeTCP, Host: "localhost", Port: 6379}, true
	case storedb.EngineMongoDB:
		return dbconn.Target{Engine: storedb.EngineMongoDB, ConnMode: storedb.ConnModeTCP, Host: "localhost", Port: 27017}, true
	}
	return dbconn.Target{}, false
}

// BootstrapAfterInstall 是安装后配置就绪 OHS（§4.6/D-20）。编排：①幂等前置
// （本机默认 identity 已 credentialsSet → skipped）；②引导通道建账户/设密码
// （引擎窄接口，有界重试等待就绪）；③治理通道 TCP Ping 验证新凭据（失败 →
// failed，不写未验证凭据）；④加密 UPSERT db_instance + reachable + version +
// 审计 instance_bootstrapped（同事务脱敏）。密码零回显。
func (s *Service) BootstrapAfterInstall(ctx context.Context, engine, actorName string) (BootstrapOutcome, error) {
	target, ok := bootstrapTarget(engine)
	if !ok {
		return BootstrapOutcome{}, fmt.Errorf("dbmgmt: bootstrap unsupported engine %q", engine)
	}
	key := BuildIdentityKey(target.Engine, target.ConnMode, "", target.Host, target.Port)

	// ① 幂等前置：已配置凭据 → 保守跳过（不覆盖用户已有凭据，⚠️A13）。
	existing, err := s.store.Instance().GetByIdentity(ctx, key)
	if err != nil {
		return bootstrapFailed("读取实例档案失败: " + err.Error()), nil
	}
	if existing != nil && existing.CredentialsSet() {
		id := existing.ID
		return BootstrapOutcome{Status: BootstrapStatusSkipped, Note: "实例已配置凭据，保守跳过（不覆盖）", InstanceID: &id}, nil
	}

	// ② 引导通道建账户/设密码（随机强密码；有界重试等待引擎就绪）。
	password, err := genBootstrapPassword()
	if err != nil {
		return bootstrapFailed("生成随机密码失败: " + err.Error()), nil
	}
	username := BootstrapAccountName
	if engine == storedb.EngineRedis {
		username = RedisDefaultUsername
	}
	skipped, note, err := s.bootstrapEngine(ctx, engine, target, username, password)
	if err != nil {
		return bootstrapFailed(note), nil
	}
	if skipped {
		out := BootstrapOutcome{Status: BootstrapStatusSkipped, Note: note}
		if existing != nil {
			id := existing.ID
			out.InstanceID = &id
		}
		return out, nil
	}

	// ③ 治理通道 TCP Ping 验证新凭据（装完即可管；失败不写未验证凭据，D-03）。
	creds := dbconn.Creds{Username: username, Password: password}
	if err := s.bootstrapPing(ctx, engine, target, creds); err != nil {
		return bootstrapFailed("管理账户已创建，但面板治理通道验证失败: " + reasonOf(err) +
			"（MySQL 请确认 skip-name-resolve 未开启使 localhost 匹配 TCP 来源）"), nil
	}
	version := s.bootstrapVersion(ctx, engine, target, creds)

	// ④ 加密 + 同事务 UPSERT 档案与凭据 + 审计（脱敏）。
	cipher, err := s.vault.Encrypt(password)
	if err != nil {
		return bootstrapFailed("凭据加密失败: " + err.Error()), nil
	}
	id, err := s.registerBootstrapped(ctx, target, key, username, cipher, version, actorName)
	if err != nil {
		return bootstrapFailed("注册实例凭据失败: " + err.Error()), nil
	}
	return BootstrapOutcome{
		Status:     BootstrapStatusSucceeded,
		Note:       "已创建管理账户并注册到数据库管理，可直接治理",
		InstanceID: &id,
	}, nil
}

// bootstrapFailed 构造 failed 结果（Note 不含密码）。
func bootstrapFailed(note string) BootstrapOutcome {
	return BootstrapOutcome{Status: BootstrapStatusFailed, Note: note}
}

// genBootstrapPassword 生成随机强密码（crypto/rand BootstrapPasswordBytes 字节 →
// base64url 32 字符，不弱默认；D-16）。
func genBootstrapPassword() (string, error) {
	b := make([]byte, BootstrapPasswordBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("dbmgmt: bootstrap rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// bootstrapEngine 引擎引导通道建账户/设密码（有界重试等待就绪，D-19）。返回
// (skipped, note, err)：skipped=用户已有配置保守跳过；err=引导失败（note 含原因）。
func (s *Service) bootstrapEngine(ctx context.Context, engine string, target dbconn.Target, username, password string) (bool, string, error) {
	timeout := s.bootReadyTimeout
	if timeout <= 0 {
		timeout = BootstrapReadyTimeout
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		skipped, note, err := s.bootstrapEngineOnce(ctx, engine, target, username, password)
		if err == nil {
			return skipped, note, nil
		}
		lastErr = err
		if !isConnError(engine, err) || time.Now().After(deadline) {
			return false, "引导通道建连/配置失败: " + reasonOf(err), lastErr
		}
		select {
		case <-ctx.Done():
			return false, "引导取消: " + ctx.Err().Error(), ctx.Err()
		case <-time.After(BootstrapRetryInterval):
		}
	}
}

// bootstrapEngineOnce 单轮引擎引导（MySQL 逐 socket 候选尝试）。
func (s *Service) bootstrapEngineOnce(ctx context.Context, engine string, target dbconn.Target, username, password string) (bool, string, error) {
	switch engine {
	case storedb.EngineMySQL, storedb.EngineMariaDB:
		if s.mysqlBoot == nil {
			return false, "", errors.New("dbmgmt: mysql bootstrap adapter not configured")
		}
		var lastErr error
		for _, pt := range DiscoveryProbeSet {
			if pt.Engine != storedb.EngineMySQL || pt.ConnMode != storedb.ConnModeSocket {
				continue
			}
			exists, err := s.mysqlBoot.BootstrapAdminUser(ctx, pt.SocketPath, username, password)
			if err == nil {
				if exists {
					return true, "管理账户 dougong@localhost 已存在，保守跳过（不重置密码）", nil
				}
				return false, "", nil
			}
			lastErr = err
			if !isConnError(engine, err) {
				return false, "", err // 非连接类错误（如 CREATE 被拒）不重试其它 socket
			}
		}
		return false, "", lastErr
	case storedb.EngineRedis:
		if s.redisBoot == nil {
			return false, "", errors.New("dbmgmt: redis bootstrap adapter not configured")
		}
		alreadySet, loopback, err := s.redisBoot.BootstrapRequirepass(ctx, target.Host, target.Port, password)
		if err != nil {
			return false, "", err
		}
		if alreadySet {
			return true, "Redis 已设置 requirepass，保守跳过（不覆盖）", nil
		}
		if !loopback {
			return false, "requirepass 已设；注意 bind 非纯本机（fresh install 默认 127.0.0.1，请人工核查）", nil
		}
		return false, "", nil
	case storedb.EngineMongoDB:
		if s.mongoBoot == nil {
			return false, "", errors.New("dbmgmt: mongo bootstrap adapter not configured")
		}
		exists, err := s.mongoBoot.BootstrapAdminUser(ctx, target.Host, target.Port, username, password)
		if err != nil {
			return false, "", err
		}
		if exists {
			return true, "MongoDB 已有用户或已开鉴权，保守跳过（localhost exception 不适用）", nil
		}
		return false, "", nil
	}
	return false, "", fmt.Errorf("dbmgmt: bootstrap unsupported engine %q", engine)
}

// isConnError 判定引擎错误是否连接类（可重试等待就绪；D-19）。
func isConnError(engine string, err error) bool {
	switch engine {
	case storedb.EngineMySQL, storedb.EngineMariaDB:
		return errors.Is(err, dbmysql.ErrConnFailed) || errors.Is(err, dbmysql.ErrUnavailable)
	case storedb.EngineRedis:
		return errors.Is(err, dbredis.ErrConnFailed) || errors.Is(err, dbredis.ErrUnavailable)
	case storedb.EngineMongoDB:
		return errors.Is(err, dbmongo.ErrConnFailed)
	}
	return false
}

// bootstrapPing 治理通道 TCP 验证新凭据（复用既有网关 Ping）。
func (s *Service) bootstrapPing(ctx context.Context, engine string, target dbconn.Target, creds dbconn.Creds) error {
	switch engine {
	case storedb.EngineMySQL, storedb.EngineMariaDB:
		return s.mysql.Ping(ctx, target, creds)
	case storedb.EngineRedis:
		return s.redis.Ping(ctx, target, creds)
	case storedb.EngineMongoDB:
		return s.mongo.Ping(ctx, target, creds)
	}
	return fmt.Errorf("dbmgmt: bootstrap unsupported engine %q", engine)
}

// bootstrapVersion 治理通道实况版本（尽力刷新，失败不阻断注册）。
func (s *Service) bootstrapVersion(ctx context.Context, engine string, target dbconn.Target, creds dbconn.Creds) string {
	var v string
	var err error
	switch engine {
	case storedb.EngineMySQL, storedb.EngineMariaDB:
		v, err = s.mysql.ServerVersion(ctx, target, creds)
	case storedb.EngineRedis:
		v, err = s.redis.ServerVersion(ctx, target, creds)
	case storedb.EngineMongoDB:
		v, err = s.mongo.ServerVersion(ctx, target, creds)
	}
	if err != nil {
		return ""
	}
	return v
}

// registerBootstrapped 同事务 UPSERT 档案 + 凭据密文 + reachable + version +
// 审计 instance_bootstrapped（脱敏，R-03）。返回档案 id。
func (s *Service) registerBootstrapped(ctx context.Context, target dbconn.Target, key, username, cipher, version, actorName string) (int64, error) {
	host := target.Host
	port := target.Port
	inst := storedb.Instance{IdentityKey: key, Engine: target.Engine, ConnMode: target.ConnMode, Host: &host, Port: &port}
	var versionPtr *string
	if version != "" {
		versionPtr = &version
	}
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Instance().UpsertDiscovered(ctx, inst, storedb.ConnStateReachable, versionPtr); err != nil {
		return 0, err
	}
	row, err := tx.Instance().GetByIdentity(ctx, key)
	if err != nil {
		return 0, err
	}
	if row == nil {
		return 0, errors.New("dbmgmt: bootstrap register identity missing after upsert")
	}
	if err := tx.Instance().UpdateCredentials(ctx, row.ID, username, cipher, versionPtr); err != nil {
		return 0, err
	}
	e := audit.Entry{
		ActorName:  actorName,
		Category:   audit.CategoryChange,
		Action:     ActionInstanceBootstrapped,
		ObjectType: ObjectTypeInstance,
		ObjectID:   instanceObjectID(row.ID),
		Detail:     map[string]string{"engine": target.Engine, "connMode": target.ConnMode, "status": BootstrapStatusSucceeded},
		Result:     audit.ResultSuccess,
		Source:     audit.SourceWeb,
	}
	if err := s.txAudit(ctx, tx, e); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return row.ID, nil
}
