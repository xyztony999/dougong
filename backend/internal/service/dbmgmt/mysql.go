package dbmgmt

import (
	"context"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// MySQLStatus GET /db/mysql/status：状态概览（D-08 白名单 ≤10 项；实时读取
// 不落库不缓存 D-04/I-04）。
func (s *Service) MySQLStatus(ctx context.Context, instanceID string) (*MySQLStatusData, error) {
	_, t, c, err := s.governTarget(ctx, instanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	version, vars, err := s.mysql.StatusVariables(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	filtered := make(map[string]string, len(MySQLStatusFields))
	for _, k := range MySQLStatusFields {
		if v, ok := vars[k]; ok {
			filtered[k] = v
		}
	}
	return &MySQLStatusData{Version: version, Variables: filtered}, nil
}

// MySQLProcessList GET /db/mysql/processlist：进程列表实时快照（全量返回，
// 连接数量级小 D-09）。
func (s *Service) MySQLProcessList(ctx context.Context, instanceID string) (*ProcessListData, error) {
	_, t, c, err := s.governTarget(ctx, instanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	rows, err := s.mysql.ProcessList(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	items := make([]ProcessItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, ProcessItem{
			ID: r.ID, User: r.User, Host: r.Host, DB: r.DB,
			Command: r.Command, TimeSec: r.TimeSec, State: r.State, Info: r.Info,
		})
	}
	return &ProcessListData{Items: items, GeneratedAt: s.clock().UTC()}, nil
}

// ChangeRootPassword POST /db/mysql/root-password：改密两步同步编排（D-05）：
// 引擎侧改密（ALTER USER USER()，A-DB8）→ 同请求 UPSERT 面板凭据密文 →
// 落库失败 DBM_019（引擎侧已改不回滚，data.recoverHint=credentials）。
func (s *Service) ChangeRootPassword(ctx context.Context, actor *authsvc.Identity, req PasswordRequest) (*PasswordChangeData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	if err := validatePassword("newPassword", req.NewPassword); err != nil {
		return nil, err
	}
	return s.changePassword(ctx, actor, inst, t, c, req.NewPassword, ActionRootPasswordChanged)
}

// changePassword 改密同步共用编排（mysql root-password / redis password，
// D-05；审计 detail 不含新旧密码 D-13）。
func (s *Service) changePassword(ctx context.Context, actor *authsvc.Identity,
	inst *storedb.Instance, t dbconn.Target, c dbconn.Creds,
	newPassword, action string) (*PasswordChangeData, error) {
	// 第一步：引擎侧改密。
	var engineErr error
	if s.isMySQL(inst) {
		engineErr = s.mysql.AlterRootPassword(ctx, t, c, newPassword)
	} else {
		engineErr = s.redis.SetRequirepass(ctx, t, c, c.Password, newPassword)
	}
	if engineErr != nil {
		err := translateGatewayErr(engineErr)
		s.recordAudit(ctx, auditEntry(actor, action, ObjectTypeInstance,
			instanceObjectID(inst.ID), nil, false))
		return nil, err
	}

	// 第二步：面板凭据密文 UPSERT（同请求；失败 DBM_019 不回滚引擎侧）。
	cipher, err := s.vault.Encrypt(newPassword)
	if err == nil {
		var tx TxAPI
		tx, err = s.store.BeginTx(ctx)
		if err == nil {
			defer func() { _ = tx.Rollback() }()
			username := *inst.CredUsername
			err = tx.Instance().UpdateCredentials(ctx, inst.ID, username, cipher, nil)
			if err == nil {
				if aerr := s.txAudit(ctx, tx, auditEntry(actor, action, ObjectTypeInstance,
					instanceObjectID(inst.ID), nil, true)); aerr != nil {
					err = aerr
				}
			}
			if err == nil {
				err = tx.Commit()
			}
		}
	}
	if err != nil {
		s.recordAudit(ctx, auditEntry(actor, action, ObjectTypeInstance,
			instanceObjectID(inst.ID), nil, false))
		return nil, errcode.ErrDBMCredSyncFailed.WithData(map[string]string{
			"recoverHint": "credentials"})
	}
	return &PasswordChangeData{InstanceID: inst.ID, Synced: true}, nil
}

// isMySQL 引擎族判定（changePassword 编排用）。
func (s *Service) isMySQL(inst *storedb.Instance) bool { return storedb.IsMySQLEngine(inst.Engine) }

// ListDatabases GET /db/database/list：库清单实时投影（D-04）。
func (s *Service) ListDatabases(ctx context.Context, instanceID string) (*DatabaseListData, error) {
	_, t, c, err := s.governTarget(ctx, instanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	dbs, err := s.mysql.ListDatabases(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	items := make([]DatabaseItem, 0, len(dbs))
	for _, d := range dbs {
		items = append(items, DatabaseItem{Name: d.Name, Charset: d.Charset})
	}
	return &DatabaseListData{Items: items, GeneratedAt: s.clock().UTC()}, nil
}

// ListCharsets GET /db/database/charsets：字符集实况候选（D-11 开放集合）。
func (s *Service) ListCharsets(ctx context.Context, instanceID string) (*CharsetsData, error) {
	_, t, c, err := s.governTarget(ctx, instanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	charsets, err := s.mysql.ShowCharsets(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	if charsets == nil {
		charsets = []string{}
	}
	return &CharsetsData{Items: charsets}, nil
}

// CreateDatabase POST /db/database/create：建库（D-11 默认 utf8mb4；charset
// ∈ 实况候选 AUTH_012；同名实况校验 DBM_008）。
func (s *Service) CreateDatabase(ctx context.Context, actor *authsvc.Identity, req DatabaseCreateRequest) (*DatabaseCreateData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	if err := validateDatabaseName(req.Name); err != nil {
		return nil, err
	}
	charset := req.Charset
	if charset == "" {
		charset = DefaultCharset
	}
	charsets, err := s.mysql.ShowCharsets(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	if !containsString(charsets, charset) {
		return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "charset", Message: "charset 不在实例实况候选内"})
	}
	exists, err := s.mysql.DatabaseExists(ctx, t, c, req.Name)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	if exists {
		return nil, errcode.ErrDBMDatabaseExists
	}
	if err := s.mysql.CreateDatabase(ctx, t, c, req.Name, charset); err != nil {
		err = translateGatewayErr(err)
		s.recordAudit(ctx, auditEntry(actor, ActionDatabaseCreated, ObjectTypeDatabase,
			dbObjectID(inst, req.Name), nil, false))
		return nil, err
	}
	s.recordAudit(ctx, auditEntry(actor, ActionDatabaseCreated, ObjectTypeDatabase,
		dbObjectID(inst, req.Name),
		map[string]string{"charset": charset}, true))
	return &DatabaseCreateData{InstanceID: inst.ID, Name: req.Name, Charset: charset}, nil
}

// DeleteDatabase POST /db/database/delete：删库（不可逆；库存在 DBM_007
// 不静默成功；交互层二次确认前置）。
func (s *Service) DeleteDatabase(ctx context.Context, actor *authsvc.Identity, req DatabaseDeleteRequest) (*DatabaseDeleteData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	if err := validateDatabaseName(req.Name); err != nil {
		return nil, err
	}
	exists, err := s.mysql.DatabaseExists(ctx, t, c, req.Name)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	if !exists {
		return nil, errcode.ErrDBMDatabaseNotFound
	}
	if err := s.mysql.DropDatabase(ctx, t, c, req.Name); err != nil {
		err = translateGatewayErr(err)
		s.recordAudit(ctx, auditEntry(actor, ActionDatabaseDropped, ObjectTypeDatabase,
			dbObjectID(inst, req.Name), nil, false))
		return nil, err
	}
	s.recordAudit(ctx, auditEntry(actor, ActionDatabaseDropped, ObjectTypeDatabase,
		dbObjectID(inst, req.Name), nil, true))
	return &DatabaseDeleteData{InstanceID: inst.ID, Name: req.Name}, nil
}

// ---- dbuser（§4.3.4）----

// ListDBUsers GET /db/dbuser/list：用户清单（raw 权限经 D-10 映射表归并三档
// 呈现；无任何密码字段）。
func (s *Service) ListDBUsers(ctx context.Context, instanceID string) (*DBUserListData, error) {
	_, t, c, err := s.governTarget(ctx, instanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	users, err := s.mysql.ListUsers(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	return &DBUserListData{Items: mapUserGrants(users), GeneratedAt: s.clock().UTC()}, nil
}

// CreateDBUser POST /db/dbuser/create：建用户（密码只写不读 I-02；同名同
// host 实况校验 DBM_010；审计 detail 不含密码）。
func (s *Service) CreateDBUser(ctx context.Context, actor *authsvc.Identity, req DBUserCreateRequest) (*DBUserData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	if err := validateUsername(req.Username); err != nil {
		return nil, err
	}
	if err := validateUserHost(req.Host); err != nil {
		return nil, err
	}
	if err := validatePassword("password", req.Password); err != nil {
		return nil, err
	}
	users, err := s.mysql.ListUsers(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	for _, u := range users {
		if u.Username == req.Username && u.Host == req.Host {
			return nil, errcode.ErrDBMDBUserExists
		}
	}
	if err := s.mysql.CreateUser(ctx, t, c, req.Username, req.Host, req.Password); err != nil {
		err = translateGatewayErr(err)
		s.recordAudit(ctx, auditEntry(actor, ActionDBUserCreated, ObjectTypeDBUser,
			userObjectID(inst, req.Username, req.Host), nil, false))
		return nil, err
	}
	s.recordAudit(ctx, auditEntry(actor, ActionDBUserCreated, ObjectTypeDBUser,
		userObjectID(inst, req.Username, req.Host),
		map[string]string{"host": req.Host}, true))
	return &DBUserData{InstanceID: inst.ID, Username: req.Username, Host: req.Host}, nil
}

// DeleteDBUser POST /db/dbuser/delete：删用户（引擎语义连带回收授权）。
func (s *Service) DeleteDBUser(ctx context.Context, actor *authsvc.Identity, req DBUserRevokeRequest) (*DBUserData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	if err := validateUsername(req.Username); err != nil {
		return nil, err
	}
	if err := validateUserHost(req.Host); err != nil {
		return nil, err
	}
	users, err := s.mysql.ListUsers(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	found := false
	for _, u := range users {
		if u.Username == req.Username && u.Host == req.Host {
			found = true
			break
		}
	}
	if !found {
		return nil, errcode.ErrDBMDBUserNotFound
	}
	if err := s.mysql.DropUser(ctx, t, c, req.Username, req.Host); err != nil {
		err = translateGatewayErr(err)
		s.recordAudit(ctx, auditEntry(actor, ActionDBUserDeleted, ObjectTypeDBUser,
			userObjectID(inst, req.Username, req.Host), nil, false))
		return nil, err
	}
	s.recordAudit(ctx, auditEntry(actor, ActionDBUserDeleted, ObjectTypeDBUser,
		userObjectID(inst, req.Username, req.Host), nil, true))
	return &DBUserData{InstanceID: inst.ID, Username: req.Username, Host: req.Host}, nil
}

// GrantDBUser POST /db/dbuser/grant：库级三档授权（D-10；幂等；用户 DBM_009/
// 库 DBM_007/level 枚举 AUTH_012）。
func (s *Service) GrantDBUser(ctx context.Context, actor *authsvc.Identity, req DBUserGrantRequest) (*DBUserGrantData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	if err := validateUsername(req.Username); err != nil {
		return nil, err
	}
	if err := validateUserHost(req.Host); err != nil {
		return nil, err
	}
	if err := validateDatabaseName(req.Database); err != nil {
		return nil, err
	}
	if req.Level != LevelReadonly && req.Level != LevelWrite && req.Level != LevelAll {
		return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "level", Message: "level 须为 readonly|write|all"})
	}
	if _, err := s.mysql.ShowGrants(ctx, t, c, req.Username, req.Host); err != nil {
		return nil, translateGatewayErr(err) // 含 DBM_009（用户不存在）
	}
	exists, err := s.mysql.DatabaseExists(ctx, t, c, req.Database)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	if !exists {
		return nil, errcode.ErrDBMDatabaseNotFound
	}
	if err := s.mysql.Grant(ctx, t, c, req.Username, req.Host, req.Database, req.Level); err != nil {
		err = translateGatewayErr(err)
		s.recordAudit(ctx, auditEntry(actor, ActionDBUserGranted, ObjectTypeDBUser,
			userObjectID(inst, req.Username, req.Host),
			map[string]string{"database": req.Database}, false))
		return nil, err
	}
	s.recordAudit(ctx, auditEntry(actor, ActionDBUserGranted, ObjectTypeDBUser,
		userObjectID(inst, req.Username, req.Host),
		map[string]string{"database": req.Database, "level": req.Level}, true))
	return &DBUserGrantData{InstanceID: inst.ID, Username: req.Username, Host: req.Host,
		Database: req.Database, Level: req.Level}, nil
}

// RevokeDBUser POST /db/dbuser/revoke：按库回收（授权关系不存在 DBM_011
// 如实报错）。
func (s *Service) RevokeDBUser(ctx context.Context, actor *authsvc.Identity, req DBUserRevokeRequest) (*DBUserRevokeData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	if err := validateUsername(req.Username); err != nil {
		return nil, err
	}
	if err := validateUserHost(req.Host); err != nil {
		return nil, err
	}
	if err := validateDatabaseName(req.Database); err != nil {
		return nil, err
	}
	grants, err := s.mysql.ShowGrants(ctx, t, c, req.Username, req.Host)
	if err != nil {
		return nil, translateGatewayErr(err) // 含 DBM_009（用户不存在）
	}
	if !hasDBGrant(grants, req.Database) {
		return nil, errcode.ErrDBMGrantNotFound
	}
	if err := s.mysql.Revoke(ctx, t, c, req.Username, req.Host, req.Database); err != nil {
		err = translateGatewayErr(err)
		s.recordAudit(ctx, auditEntry(actor, ActionDBUserRevoked, ObjectTypeDBUser,
			userObjectID(inst, req.Username, req.Host),
			map[string]string{"database": req.Database}, false))
		return nil, err
	}
	s.recordAudit(ctx, auditEntry(actor, ActionDBUserRevoked, ObjectTypeDBUser,
		userObjectID(inst, req.Username, req.Host),
		map[string]string{"database": req.Database}, true))
	return &DBUserRevokeData{InstanceID: inst.ID, Username: req.Username, Host: req.Host,
		Database: req.Database}, nil
}

// ---- 审计对象标识 ----

func dbObjectID(inst *storedb.Instance, name string) string {
	return instanceObjectID(inst.ID) + ":" + name
}

func userObjectID(inst *storedb.Instance, username, host string) string {
	return instanceObjectID(inst.ID) + ":" + username + "@" + host
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
