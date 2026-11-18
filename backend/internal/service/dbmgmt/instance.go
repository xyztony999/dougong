package dbmgmt

import (
	"context"
	"log/slog"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// ListInstances GET /db/instance/list：进程内首次调用触发自动探测（D-07：
// 系统机制不写审计，可被 readonly 触发）→ 读表按 discovered_at 排序 →
// engine 筛选（D-04 读路径零写副作用）。
func (s *Service) ListInstances(ctx context.Context, engine string) (*InstanceListData, error) {
	if engine != "" && engine != storedb.EngineMySQL && engine != storedb.EngineMariaDB &&
		engine != storedb.EngineRedis && engine != storedb.EngineMongoDB {
		return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "engine", Message: "engine 须为 mysql|mariadb|redis|mongodb"})
	}
	s.discoverOnce.Do(func() {
		if _, err := s.prober.ProbeAll(ctx); err != nil {
			slogWarn(ctx, "dbmgmt: initial auto discovery failed", err)
		}
	})
	rows, err := s.store.Instance().List(ctx, engine)
	if err != nil {
		return nil, err
	}
	// redundantSocket 派生（D-21）：同 engine 本机 TCP 档案存在且可达 →
	// socket 行标注冗余（已配凭据行保留+标注口径；读路径零写副作用 I-04）。
	localTCPReachable := map[string]bool{}
	for i := range rows {
		r := &rows[i]
		if r.ConnMode == storedb.ConnModeTCP && r.ConnState == storedb.ConnStateReachable &&
			r.Host != nil && isLocalTCPHost(*r.Host) {
			localTCPReachable[r.Engine] = true
		}
	}
	items := make([]InstanceItem, 0, len(rows))
	for i := range rows {
		item := projectInstance(&rows[i])
		item.RedundantSocket = rows[i].ConnMode == storedb.ConnModeSocket && localTCPReachable[rows[i].Engine]
		items = append(items, item)
	}
	return &InstanceListData{Items: items, GeneratedAt: s.clock().UTC()}, nil
}

// isLocalTCPHost 本机 TCP 主机判据（redundantSocket 派生用——远程 TCP 档案
// 不触发冗余标注，D-21「远程 TCP 实例不受影响」）。
func isLocalTCPHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// projectInstance 档案行 → 响应条目（cred_cipher 零下发 I-02；版本/凭据
// 时间可空如实呈现）。
func projectInstance(inst *storedb.Instance) InstanceItem {
	item := InstanceItem{
		ID:             inst.ID,
		Engine:         inst.Engine,
		ConnMode:       inst.ConnMode,
		SocketPath:     inst.SocketPath,
		Host:           inst.Host,
		Port:           inst.Port,
		Version:        inst.Version,
		ConnState:      inst.ConnState,
		CredentialsSet: inst.CredentialsSet(),
		CredUsername:   inst.CredUsername,
		CredUpdatedAt:  inst.CredUpdatedAt,
		DiscoveredAt:   inst.DiscoveredAt.UTC(),
	}
	return item
}

// Rescan POST /db/instance/rescan：手动探测编排（2.3，v2.3 TCP 优先两阶段
// D-21）+ 保守清理（同 engine 本机 TCP 本轮可达且无凭据的 socket 档案
// DELETE，审计 instance_merged 同事务；已配凭据保留）+ 审计
// instance_rescanned（admin 显式动作写审计，detail 含 merged 统计，D-07）。
func (s *Service) Rescan(ctx context.Context, actor *authsvc.Identity) (*RescanData, error) {
	res, err := s.prober.ProbeAll(ctx)
	if err != nil {
		return nil, err
	}
	merged, err := s.prober.MergeRedundantSockets(ctx, res.TCPReachable, actor)
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, auditEntry(actor, ActionInstanceRescanned, ObjectTypeInstance, "",
		map[string]int{"discovered": res.Discovered, "updated": res.Updated, "merged": merged}, true))
	return &RescanData{Discovered: res.Discovered, Updated: res.Updated, Merged: merged}, nil
}

// SetCredentials POST /db/instance/credentials：凭据接管编排（D-03 先验证
// 后落库——验证失败不落库不覆盖既有有效凭据）。
func (s *Service) SetCredentials(ctx context.Context, actor *authsvc.Identity, req CredentialsRequest) (*CredentialsData, error) {
	if req.InstanceID <= 0 {
		return nil, errcode.ErrDBMInvalidInstanceID
	}
	inst, err := s.loadInstance(ctx, req.InstanceID)
	if err != nil {
		return nil, err
	}
	if req.Username == "" {
		return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "username", Message: "username 必填"})
	}
	if err := validatePassword("password", req.Password); err != nil {
		return nil, err
	}

	// 连接目标纠正（D-03：可选变更 → identity_key 重算 → 冲突 DBM_020）。
	spec := ConnTargetSpec{ConnMode: req.ConnMode, SocketPath: req.SocketPath, Host: req.Host, Port: req.Port}
	target, identityKey, err := spec.resolve(inst)
	if err != nil {
		return nil, err
	}
	if identityKey != inst.IdentityKey {
		other, gerr := s.store.Instance().GetByIdentity(ctx, identityKey)
		if gerr != nil {
			return nil, gerr
		}
		if other != nil && other.ID != inst.ID {
			return nil, errcode.ErrDBMIdentityConflict
		}
	}

	// 连接验证（DBM_005/006/021/022 不落库；translatePingErr——未分类失败归
	// DBM_006，不裸 INTERNAL）。
	creds := dbconn.Creds{Username: req.Username, Password: req.Password}
	switch {
	case storedb.IsMySQLEngine(inst.Engine):
		if err := s.mysql.Ping(ctx, target, creds); err != nil {
			return nil, translatePingErr(err)
		}
	case storedb.IsMongoEngine(inst.Engine):
		if err := s.mongo.Ping(ctx, target, creds); err != nil {
			return nil, translatePingErr(err)
		}
	default:
		if err := s.redis.Ping(ctx, target, creds); err != nil {
			return nil, translatePingErr(err)
		}
	}

	// 版本实况（D-04：credentials 刷新）。
	version := ""
	switch {
	case storedb.IsMySQLEngine(inst.Engine):
		if v, verr := s.mysql.ServerVersion(ctx, target, creds); verr == nil {
			version = v
		}
	case storedb.IsMongoEngine(inst.Engine):
		if v, verr := s.mongo.ServerVersion(ctx, target, creds); verr == nil {
			version = v
		}
	default:
		if v, verr := s.redis.ServerVersion(ctx, target, creds); verr == nil {
			version = v
		}
	}

	// 加密 + 同事务落库与审计（detail 不含密码，D-13）。
	cipher, err := s.vault.Encrypt(req.Password)
	if err != nil {
		return nil, err
	}
	username := req.Username
	if !storedb.IsMySQLEngine(inst.Engine) && !storedb.IsMongoEngine(inst.Engine) {
		username = RedisDefaultUsername // redis 凭据用户名固定（A-DB6）；mongodb 按请求实名
	}
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var versionPtr *string
	if version != "" {
		versionPtr = &version
	}
	if identityKey != inst.IdentityKey {
		var socketPtr, hostPtr *string
		var portPtr *int
		if target.ConnMode == storedb.ConnModeSocket {
			socketPtr = &target.SocketPath
		} else {
			hostPtr = &target.Host
			portPtr = &target.Port
		}
		err = tx.Instance().UpdateTargetAndCredentials(ctx, inst.ID,
			identityKey, target.ConnMode, socketPtr, hostPtr, portPtr,
			username, cipher, versionPtr)
	} else {
		err = tx.Instance().UpdateCredentials(ctx, inst.ID, username, cipher, versionPtr)
	}
	if err != nil {
		return nil, err
	}
	if err := s.txAudit(ctx, tx, auditEntry(actor, ActionInstanceCredentials, ObjectTypeInstance,
		instanceObjectID(inst.ID),
		map[string]string{"engine": inst.Engine, "connMode": target.ConnMode}, true)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &CredentialsData{InstanceID: inst.ID, ConnState: storedb.ConnStateReachable, Version: version}, nil
}

// RemoveInstance POST /db/instance/remove：移除纳管（⚠️A20：仅删档案行
// 连带凭据密文；在途守卫 DBM_016；同事务审计）。
func (s *Service) RemoveInstance(ctx context.Context, actor *authsvc.Identity, req InstanceIDRequest) (*RemoveData, error) {
	if req.InstanceID <= 0 {
		return nil, errcode.ErrDBMInvalidInstanceID
	}
	inst, err := s.loadInstance(ctx, req.InstanceID)
	if err != nil {
		return nil, err
	}
	if s.busy.Busy(inst.ID) {
		return nil, errcode.ErrDBMInstanceBusy
	}
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.Instance().Delete(ctx, inst.ID); err != nil {
		return nil, err
	}
	if err := s.txAudit(ctx, tx, auditEntry(actor, ActionInstanceRemoved, ObjectTypeInstance,
		instanceObjectID(inst.ID),
		map[string]string{"engine": inst.Engine, "identityKey": inst.IdentityKey}, true)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &RemoveData{InstanceID: inst.ID}, nil
}

// slogWarn 结构化警告（探测等系统机制降级日志；集中定义避免散落）。
func slogWarn(ctx context.Context, msg string, err error) {
	slog.WarnContext(ctx, msg, "err", err)
}
