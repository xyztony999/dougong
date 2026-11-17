package dbmgmt

import (
	"context"
	"errors"
	"log/slog"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/adapter/dbmongo"
	"github.com/dougong/dougong/internal/adapter/dbmysql"
	"github.com/dougong/dougong/internal/adapter/dbredis"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// InstanceProber 是探测纳管领域服务（D-07）：DISCOVERY_PROBE_SET 探测 →
// identity_key 归并登记（新 identity INSERT 无凭据档案 / 既有档案
// conn_state 整值更新 + version 随可达连接实况刷新，I-01/I-03/A-DB4）。
//
// reachableFn 为传输层探测注入点（测试替身；生产 targetReachable——A-DB4：
// socket 存在/TCP 拨通）。
type InstanceProber struct {
	store     StoreAPI
	mysql     MySQLGateway
	redis     RedisGateway
	mongo     MongoGateway
	vault     *CredentialVault
	reachable func(dbconn.Target) bool
}

// NewProber 构造探测服务（vault 供有凭据档案的引擎实况刷新）。
func NewProber(store StoreAPI, mysql MySQLGateway, redis RedisGateway, mongo MongoGateway,
	vault *CredentialVault, reachable func(dbconn.Target) bool) *InstanceProber {
	if reachable == nil {
		reachable = targetReachable
	}
	return &InstanceProber{store: store, mysql: mysql, redis: redis, mongo: mongo, vault: vault, reachable: reachable}
}

// ProbeAll 执行一轮完整探测（DISCOVERY_PROBE_SET），返回 {discovered, updated}
// 统计（2.3 探测纳管编排）。单目标失败不中断整体（探测为系统机制，尽力而为，
// 错误记日志——D-07）。
func (p *InstanceProber) ProbeAll(ctx context.Context) (discovered, updated int, err error) {
	for _, pt := range DiscoveryProbeSet {
		if cerr := ctx.Err(); cerr != nil {
			return discovered, updated, cerr
		}
		disc, handled, perr := p.probeOnce(ctx, pt)
		if perr != nil {
			slog.WarnContext(ctx, "dbmgmt: probe target failed",
				"engine", pt.Engine, "connMode", pt.ConnMode, "err", perr)
			continue
		}
		if !handled {
			continue // 传输层不可达且无档案：未发现，不纳管（如实）
		}
		if disc {
			discovered++
		} else {
			updated++
		}
	}
	return discovered, updated, nil
}

// probeOnce 对单个探测目标执行归并登记。返回 (discovered, handled, err)。
func (p *InstanceProber) probeOnce(ctx context.Context, pt ProbeTarget) (discovered, handled bool, err error) {
	t := dbconn.Target{Engine: pt.Engine, ConnMode: pt.ConnMode,
		SocketPath: pt.SocketPath, Host: pt.Host, Port: pt.Port}
	key := BuildIdentityKey(pt.Engine, pt.ConnMode, pt.SocketPath, pt.Host, pt.Port)
	reachable := p.reachable(t)

	inst := storedb.Instance{IdentityKey: key, Engine: pt.Engine, ConnMode: pt.ConnMode}
	if pt.SocketPath != "" {
		inst.SocketPath = &pt.SocketPath
	}
	if pt.Host != "" {
		host := pt.Host
		inst.Host = &host
	}
	if pt.Port != 0 {
		port := pt.Port
		inst.Port = &port
	}

	existing, err := p.store.Instance().GetByIdentity(ctx, key)
	if err != nil {
		return false, false, err
	}
	if !reachable && existing == nil {
		return false, false, nil
	}

	// 既有档案 + 传输层可达 + 已配置凭据：升级为引擎实况判定（Ping/version，
	// A-DB4）；其余形态（新档案 / 既有无凭据 / 不可达）走整值 upsert。
	if existing != nil && reachable && existing.CredentialsSet() {
		connState, version := p.engineProbe(ctx, existing)
		tx, terr := p.store.BeginTx(ctx)
		if terr != nil {
			return false, false, terr
		}
		defer func() { _ = tx.Rollback() }() // Commit 后为 no-op
		if uerr := tx.Instance().UpdateConnState(ctx, existing.ID, connState, version); uerr != nil {
			return false, false, uerr
		}
		if cerr := tx.Commit(); cerr != nil {
			return false, false, cerr
		}
		return false, true, nil
	}

	state := storedb.ConnStateUnreachable
	if reachable {
		state = storedb.ConnStateReachable
	}
	tx, terr := p.store.BeginTx(ctx)
	if terr != nil {
		return false, false, terr
	}
	defer func() { _ = tx.Rollback() }()
	discovered, uerr := tx.Instance().UpsertDiscovered(ctx, inst, state, nil)
	if uerr != nil {
		return false, false, uerr
	}
	if cerr := tx.Commit(); cerr != nil {
		return false, false, cerr
	}
	return discovered, true, nil
}

// engineProbe 以档案凭据实连引擎：Ping 成功 → reachable + version 实况刷新
// （D-04：version 仅 credentials/rescan 落库刷新）；网络层失败 → unreachable；
// 认证失败 → reachable（实例活着，凭据问题由治理链 DBM_005 表达，A-DB4）。
func (p *InstanceProber) engineProbe(ctx context.Context, inst *storedb.Instance) (string, *string) {
	creds, err := decryptCreds(p.vault, inst)
	if err != nil {
		return storedb.ConnStateReachable, nil // 解密失败：凭据问题归治理链
	}
	t := targetOf(inst)
	if storedb.IsMySQLEngine(inst.Engine) {
		if p.mysql == nil {
			return storedb.ConnStateReachable, nil
		}
		if err := p.mysql.Ping(ctx, t, creds); err != nil {
			if errors.Is(err, dbmysql.ErrConnFailed) {
				return storedb.ConnStateUnreachable, nil
			}
			return storedb.ConnStateReachable, nil
		}
		if v, verr := p.mysql.ServerVersion(ctx, t, creds); verr == nil {
			return storedb.ConnStateReachable, &v
		}
		return storedb.ConnStateReachable, nil
	}
	if storedb.IsMongoEngine(inst.Engine) {
		if p.mongo == nil {
			return storedb.ConnStateReachable, nil
		}
		if err := p.mongo.Ping(ctx, t, creds); err != nil {
			if errors.Is(err, dbmongo.ErrConnFailed) {
				return storedb.ConnStateUnreachable, nil
			}
			return storedb.ConnStateReachable, nil
		}
		if v, verr := p.mongo.ServerVersion(ctx, t, creds); verr == nil {
			return storedb.ConnStateReachable, &v
		}
		return storedb.ConnStateReachable, nil
	}
	if p.redis == nil {
		return storedb.ConnStateReachable, nil
	}
	if err := p.redis.Ping(ctx, t, creds); err != nil {
		if errors.Is(err, dbredis.ErrConnFailed) {
			return storedb.ConnStateUnreachable, nil
		}
		return storedb.ConnStateReachable, nil
	}
	if v, verr := p.redis.ServerVersion(ctx, t, creds); verr == nil {
		return storedb.ConnStateReachable, &v
	}
	return storedb.ConnStateReachable, nil
}
