package dbmgmt

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/adapter/dbmongo"
	"github.com/dougong/dougong/internal/adapter/dbmysql"
	"github.com/dougong/dougong/internal/adapter/dbredis"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// InstanceProber 是探测纳管领域服务（D-07/D-21）：DISCOVERY_PROBE_SET
// TCP 优先两阶段探测 → identity_key 归并登记（新 identity INSERT 无凭据
// 档案 / 既有档案 conn_state 整值更新 + version 随可达连接实况刷新，
// I-01/I-03/A-DB4）+ rescan 保守清理（MergeRedundantSockets，D-21）。
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

// ProbeResult 是一轮探测统计（v2.3 D-21）：Discovered/Updated 沿既有口径；
// TCPReachable 记录各 engine 本机 TCP 本轮传输层可达性（阶段 1 产物——
// 阶段 2 socket 跳过登记判据与 rescan 保守清理判据的数据源）。
type ProbeResult struct {
	Discovered   int
	Updated      int
	TCPReachable map[string]bool
}

// ProbeAll 执行一轮完整探测（DISCOVERY_PROBE_SET，TCP 优先两阶段 D-21）：
// 阶段 1 探全部 TCP 目标并记录各 engine 本机 TCP 可达性（登记/更新沿既有
// 归并口径）；阶段 2 探 socket 目标——该 engine 本机 TCP 可达 → 跳过登记
// （视为同一服务的冗余通道，不 INSERT 新档案；既有 socket 档案 conn_state
// 仍整值刷新如实），仅该 engine 无任何本机 TCP 可达时 socket 兜底登记
// （覆盖 skip-networking）。单目标失败不中断整体（探测为系统机制，尽力
// 而为，错误记日志——D-07）。保守清理不在本方法内（仅 rescan 显式调用
// MergeRedundantSockets，自动探测不写审计不清理，D-07/D-21）。
func (p *InstanceProber) ProbeAll(ctx context.Context) (ProbeResult, error) {
	res := ProbeResult{TCPReachable: map[string]bool{}}
	// 阶段 1：TCP 目标（登记 + 本机 TCP 可达性记录）。
	for _, pt := range DiscoveryProbeSet {
		if pt.ConnMode != storedb.ConnModeTCP {
			continue
		}
		if cerr := ctx.Err(); cerr != nil {
			return res, cerr
		}
		t := dbconn.Target{Engine: pt.Engine, ConnMode: pt.ConnMode, Host: pt.Host, Port: pt.Port}
		reachable := p.reachable(t)
		if reachable {
			res.TCPReachable[pt.Engine] = true
		}
		disc, handled, perr := p.probeOnce(ctx, pt, reachable, true)
		if perr != nil {
			slog.WarnContext(ctx, "dbmgmt: probe target failed",
				"engine", pt.Engine, "connMode", pt.ConnMode, "err", perr)
			continue
		}
		if !handled {
			continue // 传输层不可达且无档案：未发现，不纳管（如实）
		}
		if disc {
			res.Discovered++
		} else {
			res.Updated++
		}
	}
	// 阶段 2：socket 目标（TCP 可达 → 跳过登记仅刷新既有档案；否则兜底）。
	for _, pt := range DiscoveryProbeSet {
		if pt.ConnMode != storedb.ConnModeSocket {
			continue
		}
		if cerr := ctx.Err(); cerr != nil {
			return res, cerr
		}
		t := dbconn.Target{Engine: pt.Engine, ConnMode: pt.ConnMode, SocketPath: pt.SocketPath}
		reachable := p.reachable(t)
		allowInsert := !res.TCPReachable[pt.Engine]
		disc, handled, perr := p.probeOnce(ctx, pt, reachable, allowInsert)
		if perr != nil {
			slog.WarnContext(ctx, "dbmgmt: probe target failed",
				"engine", pt.Engine, "connMode", pt.ConnMode, "err", perr)
			continue
		}
		if !handled {
			continue // 跳过登记且无既有档案 / 不可达且无档案：不计数
		}
		if disc {
			res.Discovered++
		} else {
			res.Updated++
		}
	}
	return res, nil
}

// probeOnce 对单个探测目标执行归并登记。返回 (discovered, handled, err)。
// reachable 由调用方传入（两阶段编排已拨号，免二次探测）；allowInsert=false
// 时（D-21 阶段 2 跳过登记形态）仅刷新既有档案 conn_state，不 INSERT 新行。
func (p *InstanceProber) probeOnce(ctx context.Context, pt ProbeTarget, reachable, allowInsert bool) (discovered, handled bool, err error) {
	key := BuildIdentityKey(pt.Engine, pt.ConnMode, pt.SocketPath, pt.Host, pt.Port)
	existing, err := p.store.Instance().GetByIdentity(ctx, key)
	if err != nil {
		return false, false, err
	}
	if existing == nil && (!reachable || !allowInsert) {
		return false, false, nil // 未发现不纳管 / TCP 优先跳过登记（D-21）
	}

	// 既有档案 + 传输层可达 + 已配置凭据：升级为引擎实况判定（Ping/version，
	// A-DB4）；其余形态（新档案 / 既有无凭据 / 不可达）走整值刷新。
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
	if existing != nil {
		// 既有档案整值刷新（version 维持——无凭据探测仅达传输层，D-04）。
		if uerr := tx.Instance().UpdateConnState(ctx, existing.ID, state, nil); uerr != nil {
			return false, false, uerr
		}
		if cerr := tx.Commit(); cerr != nil {
			return false, false, cerr
		}
		return false, true, nil
	}
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
	discovered, uerr := tx.Instance().UpsertDiscovered(ctx, inst, state, nil)
	if uerr != nil {
		return false, false, uerr
	}
	if cerr := tx.Commit(); cerr != nil {
		return false, false, cerr
	}
	return discovered, true, nil
}

// MergeRedundantSockets 是 rescan 保守清理（D-21，仅 Rescan 调用）：对
// 「conn_mode=socket 且同 engine 本机 TCP 本轮可达 且 !CredentialsSet()」
// 的既有档案行执行 DELETE（解除纳管）+ 审计 instance_merged 同事务（R-03）；
// 「已配凭据」的 socket 行保留（避免凭据丢失——凭据无法从实况推导，D-02
// 哲学；instance/list redundantSocket 标注引导用户改配 TCP 后手动 remove）。
// 不做凭据自动迁移。远程 TCP 实例不受影响（tcpReachable 仅含本机探测集判定）。
func (p *InstanceProber) MergeRedundantSockets(ctx context.Context, tcpReachable map[string]bool,
	actor *authsvc.Identity) (int, error) {
	rows, err := p.store.Instance().List(ctx, "")
	if err != nil {
		return 0, err
	}
	merged := 0
	for i := range rows {
		inst := &rows[i]
		if inst.ConnMode != storedb.ConnModeSocket || inst.CredentialsSet() || !tcpReachable[inst.Engine] {
			continue
		}
		if cerr := ctx.Err(); cerr != nil {
			return merged, cerr
		}
		tx, terr := p.store.BeginTx(ctx)
		if terr != nil {
			return merged, terr
		}
		if derr := tx.Instance().Delete(ctx, inst.ID); derr != nil {
			_ = tx.Rollback()
			return merged, derr
		}
		entry := auditEntry(actor, ActionInstanceMerged, ObjectTypeInstance, instanceObjectID(inst.ID),
			map[string]string{"engine": inst.Engine, "identityKey": inst.IdentityKey,
				"reason": "redundant_socket_tcp_reachable"}, true)
		if aerr := tx.Audit().Append(ctx, entry, time.Now()); aerr != nil {
			_ = tx.Rollback()
			return merged, aerr
		}
		if cerr := tx.Commit(); cerr != nil {
			return merged, cerr
		}
		merged++
	}
	return merged, nil
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
