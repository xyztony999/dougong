package dbmgmt

import (
	"context"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/adapter/dbmongo"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// MongoDB 治理编排（api-database-mgmt.md §4.5；v0.1.4 反馈第 3 项增量）。
// 前置链 I-05 同构：mongo 族接口要求 engine=mongodb（DBM_018）+ reachable/
// 凭据前置（DBM_003/004）；库/用户为实况实时投影零落库（I-04，D-18/D-19）。

// mongoTarget 统一前置链（governTargetByID + engine=mongodb）。
func (s *Service) mongoTarget(ctx context.Context, id int64) (dbconn.Target, dbconn.Creds, error) {
	_, t, c, err := s.governTargetByID(ctx, id, familyMongo)
	if err != nil {
		return dbconn.Target{}, dbconn.Creds{}, err
	}
	return t, c, nil
}

// MongoStatus GET /db/mongo/status：serverStatus 白名单摘要（D-08 同构）。
func (s *Service) MongoStatus(ctx context.Context, instanceID string) (*MongoStatusData, error) {
	id, err := parseInstanceID(instanceID)
	if err != nil {
		return nil, err
	}
	t, c, err := s.mongoTarget(ctx, id)
	if err != nil {
		return nil, err
	}
	sum, err := s.mongo.StatusSummary(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	return &MongoStatusData{
		Version: sum.Version, UptimeSec: sum.UptimeSec,
		ConnectionsCurrent: sum.ConnectionsCurrent, ConnectionsAvailable: sum.ConnectionsAvailable,
		MemResidentMb: sum.MemResidentMb,
		OpInsert:      sum.OpInsert, OpQuery: sum.OpQuery, OpUpdate: sum.OpUpdate, OpDelete: sum.OpDelete,
	}, nil
}

// MongoListDatabases GET /db/mdb/list：库清单只读投影（D-18；系统库如实返回）。
func (s *Service) MongoListDatabases(ctx context.Context, instanceID string) (*MongoDatabaseListData, error) {
	id, err := parseInstanceID(instanceID)
	if err != nil {
		return nil, err
	}
	t, c, err := s.mongoTarget(ctx, id)
	if err != nil {
		return nil, err
	}
	dbs, err := s.mongo.ListDatabases(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	items := make([]MongoDatabaseItem, 0, len(dbs))
	for _, d := range dbs {
		items = append(items, MongoDatabaseItem{Name: d.Name, SizeBytes: d.SizeBytes, Collections: d.Collections})
	}
	return &MongoDatabaseListData{Items: items, GeneratedAt: s.clock().UTC()}, nil
}

// MongoListUsers GET /db/muser/list：用户清单实况投影（密码零出口 I-02）。
func (s *Service) MongoListUsers(ctx context.Context, instanceID string) (*MongoUserListData, error) {
	id, err := parseInstanceID(instanceID)
	if err != nil {
		return nil, err
	}
	t, c, err := s.mongoTarget(ctx, id)
	if err != nil {
		return nil, err
	}
	users, err := s.mongo.ListUsers(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	items := make([]MongoUserItem, 0, len(users))
	for _, u := range users {
		item := MongoUserItem{Username: u.Username, Mechanisms: u.Mechanisms, Roles: []MongoUserRole{}}
		if item.Mechanisms == nil {
			item.Mechanisms = []string{}
		}
		for _, r := range u.Roles {
			item.Roles = append(item.Roles, MongoUserRole{DB: r.DB, Role: r.Role})
		}
		items = append(items, item)
	}
	return &MongoUserListData{Items: items, GeneratedAt: s.clock().UTC()}, nil
}

// MongoCreateUser POST /db/muser/create：建用户（admin 库 + 首库首角色，D-19；
// 密码止步网关 I-02；审计 detail 不含密码 D-13）。
func (s *Service) MongoCreateUser(ctx context.Context, actor *authsvc.Identity, req MongoUserCreateRequest) (*MongoUserActionResult, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMongo)
	if err != nil {
		return nil, err
	}
	if err := validateUsername(req.Username); err != nil {
		return nil, err
	}
	if err := validatePassword("password", req.Password); err != nil {
		return nil, err
	}
	if err := validateDatabaseName(req.DB); err != nil {
		return nil, err
	}
	if !dbmongo.MongoRoleValid(req.Role) {
		return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "role", Message: "role 须为 read|readWrite|dbAdmin"})
	}
	if err := s.mongo.CreateUser(ctx, t, c, req.Username, req.Password, req.DB, req.Role); err != nil {
		return nil, translateGatewayErr(err)
	}
	s.recordAudit(ctx, auditEntry(actor, ActionMongoUserCreated, ObjectTypeDBUser,
		instanceObjectID(inst.ID),
		map[string]string{"username": req.Username, "db": req.DB, "role": req.Role}, true))
	return &MongoUserActionResult{InstanceID: inst.ID, Username: req.Username}, nil
}

// MongoDeleteUser POST /db/muser/delete：删用户（连带回收全部角色，引擎语义）。
func (s *Service) MongoDeleteUser(ctx context.Context, actor *authsvc.Identity, req MongoUserRequest) (*MongoUserActionResult, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMongo)
	if err != nil {
		return nil, err
	}
	if err := validateUsername(req.Username); err != nil {
		return nil, err
	}
	if err := s.mongo.DropUser(ctx, t, c, req.Username); err != nil {
		return nil, translateGatewayErr(err)
	}
	s.recordAudit(ctx, auditEntry(actor, ActionMongoUserDeleted, ObjectTypeDBUser,
		instanceObjectID(inst.ID), map[string]string{"username": req.Username}, true))
	return &MongoUserActionResult{InstanceID: inst.ID, Username: req.Username}, nil
}

// MongoGrantRole POST /db/muser/grant：授予库级角色（幂等——重复授予结果一致）。
func (s *Service) MongoGrantRole(ctx context.Context, actor *authsvc.Identity, req MongoUserRoleRequest) (*MongoRoleActionResult, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMongo)
	if err != nil {
		return nil, err
	}
	if err := s.validateMongoRoleTarget(req); err != nil {
		return nil, err
	}
	if err := s.mongoUserExists(ctx, t, c, req.Username); err != nil {
		return nil, err
	}
	if err := s.mongo.GrantRole(ctx, t, c, req.Username, req.DB, req.Role); err != nil {
		return nil, translateGatewayErr(err)
	}
	s.recordAudit(ctx, auditEntry(actor, ActionMongoUserGranted, ObjectTypeDBUser,
		instanceObjectID(inst.ID),
		map[string]string{"username": req.Username, "db": req.DB, "role": req.Role}, true))
	return &MongoRoleActionResult{InstanceID: inst.ID, Username: req.Username, DB: req.DB, Role: req.Role}, nil
}

// MongoRevokeRole POST /db/muser/revoke：回收库级角色。DBM_011 前置实况校验
// （revokeRolesFromUser 对未持有角色不必然报错——service 层 ListUsers 判据
// 承载「授权关系不存在」语义，§4.5.2）。
func (s *Service) MongoRevokeRole(ctx context.Context, actor *authsvc.Identity, req MongoUserRoleRequest) (*MongoRoleActionResult, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMongo)
	if err != nil {
		return nil, err
	}
	if err := s.validateMongoRoleTarget(req); err != nil {
		return nil, err
	}
	users, err := s.mongo.ListUsers(ctx, t, c)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	var held bool
	found := false
	for _, u := range users {
		if u.Username != req.Username {
			continue
		}
		found = true
		for _, r := range u.Roles {
			if r.DB == req.DB && r.Role == req.Role {
				held = true
			}
		}
	}
	if !found {
		return nil, errcode.ErrDBMDBUserNotFound.WithData(map[string]string{"username": req.Username})
	}
	if !held {
		return nil, errcode.ErrDBMGrantNotFound.WithData(map[string]string{
			"username": req.Username, "db": req.DB, "role": req.Role})
	}
	if err := s.mongo.RevokeRole(ctx, t, c, req.Username, req.DB, req.Role); err != nil {
		return nil, translateGatewayErr(err)
	}
	s.recordAudit(ctx, auditEntry(actor, ActionMongoUserRevoked, ObjectTypeDBUser,
		instanceObjectID(inst.ID),
		map[string]string{"username": req.Username, "db": req.DB, "role": req.Role}, true))
	return &MongoRoleActionResult{InstanceID: inst.ID, Username: req.Username, DB: req.DB, Role: req.Role}, nil
}

// validateMongoRoleTarget 角色/库/用户名校验（R-14 白名单；role 取值域 D-19）。
func (s *Service) validateMongoRoleTarget(req MongoUserRoleRequest) error {
	if err := validateUsername(req.Username); err != nil {
		return err
	}
	if err := validateDatabaseName(req.DB); err != nil {
		return err
	}
	if !dbmongo.MongoRoleValid(req.Role) {
		return errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "role", Message: "role 须为 read|readWrite|dbAdmin"})
	}
	return nil
}

// mongoUserExists 用户存在性实况校验（DBM_009——grantRolesToUser 对不存在
// 用户报 UserNotFound（code 11）由引擎侧分类兜底，此处前置给稳定语义）。
func (s *Service) mongoUserExists(ctx context.Context, t dbconn.Target, c dbconn.Creds, username string) error {
	users, err := s.mongo.ListUsers(ctx, t, c)
	if err != nil {
		return translateGatewayErr(err)
	}
	for _, u := range users {
		if u.Username == username {
			return nil
		}
	}
	return errcode.ErrDBMDBUserNotFound.WithData(map[string]string{"username": username})
}
