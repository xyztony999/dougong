package dbmgmt

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/adapter/dbmysql"
	"github.com/dougong/dougong/internal/adapter/dbredis"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// Service 是数据库管理应用服务（门面：instance/mysql/database/dbuser/
// backup/redis/key 全部接口编排；分层纪律——不感知 HTTP）。
type Service struct {
	mysql  MySQLGateway
	redis  RedisGateway
	store  StoreAPI
	audit  AuditRecorder
	vault  *CredentialVault
	prober *InstanceProber
	busy   *BusyTable
	clock  clockNow

	discoverOnce sync.Once // 进程内首次探测触发器（D-07：任一角色首次 list）
}

// New 构造服务（装配：store/mysql/redis/vault/audit；探测与 busy 表内建）。
func New(store StoreAPI, mysql MySQLGateway, redis RedisGateway,
	vault *CredentialVault, auditRec AuditRecorder) *Service {
	s := &Service{
		mysql: mysql, redis: redis, store: store,
		audit: auditRec, vault: vault,
		busy:  NewBusyTable(),
		clock: time.Now,
	}
	s.prober = NewProber(store, mysql, redis, vault, nil)
	return s
}

// ---- I-05 统一前置链 ----

// parseInstanceID instanceId 形态校验（DBM_001，R-14）。
func parseInstanceID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errcode.ErrDBMInvalidInstanceID
	}
	return id, nil
}

// loadInstance 档案存在性（DBM_002）。
func (s *Service) loadInstance(ctx context.Context, id int64) (*storedb.Instance, error) {
	inst, err := s.store.Instance().GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, errcode.ErrDBMInstanceNotFound
	}
	return inst, nil
}

// checkEngine 引擎匹配（DBM_018：mysql 族接口要求 engine∈mysql/mariadb；
// redis 族接口要求 engine=redis）。
func checkEngine(inst *storedb.Instance, mysqlFamily bool) error {
	if mysqlFamily != storedb.IsMySQLEngine(inst.Engine) {
		return errcode.ErrDBMEngineMismatch
	}
	return nil
}

// governTarget 治理/观测统一前置（I-05 全链，query string 形态入口——GET
// 接口用；DBM_001 形态判定在此发生）。
func (s *Service) governTarget(ctx context.Context, instanceID string, mysqlFamily bool) (
	*storedb.Instance, dbconn.Target, dbconn.Creds, error) {
	id, err := parseInstanceID(instanceID)
	if err != nil {
		return nil, dbconn.Target{}, dbconn.Creds{}, err
	}
	return s.governTargetByID(ctx, id, mysqlFamily)
}

// governTargetByID 同前置链（POST 体数值形态入口）。
func (s *Service) governTargetByID(ctx context.Context, id int64, mysqlFamily bool) (
	*storedb.Instance, dbconn.Target, dbconn.Creds, error) {
	inst, err := s.loadInstance(ctx, id)
	if err != nil {
		return nil, dbconn.Target{}, dbconn.Creds{}, err
	}
	if err := checkEngine(inst, mysqlFamily); err != nil {
		return nil, dbconn.Target{}, dbconn.Creds{}, err
	}
	if inst.ConnState != storedb.ConnStateReachable {
		return nil, dbconn.Target{}, dbconn.Creds{},
			errcode.ErrDBMInstanceUnreachable.WithData(map[string]string{
				"reason": "连接状态 unreachable；实例启停归服务管理模块"})
	}
	if !inst.CredentialsSet() {
		return nil, dbconn.Target{}, dbconn.Creds{},
			errcode.ErrDBMCredentialsNotSet.WithData(map[string]string{"hint": "credentials"})
	}
	creds, err := decryptCreds(s.vault, inst)
	if err != nil {
		return nil, dbconn.Target{}, dbconn.Creds{}, err
	}
	return inst, targetOf(inst), creds, nil
}

// decryptCreds 解密档案凭据（I-02：仅建连路径；redis username 固定 default，
// A-DB6；解密失败按内部错误脱敏——密文/密钥不外泄）。
func decryptCreds(vault *CredentialVault, inst *storedb.Instance) (dbconn.Creds, error) {
	if vault == nil || inst.CredCipher == nil || inst.CredUsername == nil {
		return dbconn.Creds{}, errors.New("dbmgmt: credentials unavailable")
	}
	password, err := vault.Decrypt(*inst.CredCipher)
	if err != nil {
		slog.ErrorContext(context.Background(), "dbmgmt: decrypt credentials failed",
			"instanceId", inst.ID, "err", err) // 细节进日志（不含密文），响应脱敏
		return dbconn.Creds{}, errcode.ErrInternal
	}
	return dbconn.Creds{Username: *inst.CredUsername, Password: password}, nil
}

// ---- 网关错误翻译（引擎方言 → DBM 语义，§4.1.5 约束）----

// reasonOf 提取哨兵错误的原因文本（"prefix: reason" 的 reason 段）。
func reasonOf(err error) string {
	msg := err.Error()
	for i := 0; i < len(msg); i++ {
		if msg[i] == ':' {
			return msg[i+2:]
		}
	}
	return msg
}

// translateGatewayErr 将 MySQL/Redis 网关哨兵翻译为 DBM 错误码；errcode
// 原样透传，其余 wrap 原样（response 兜底 INTERNAL）。
func translateGatewayErr(err error) error {
	if err == nil {
		return nil
	}
	var ec *errcode.Error
	if errors.As(err, &ec) {
		return err
	}
	switch {
	case errors.Is(err, dbmysql.ErrAuthRejected), errors.Is(err, dbredis.ErrAuthRejected):
		return errcode.ErrDBMCredentialsInvalid.WithData(map[string]string{"reason": reasonOf(err)})
	case errors.Is(err, dbmysql.ErrConnFailed), errors.Is(err, dbredis.ErrConnFailed):
		return errcode.ErrDBMConnTestFailed.WithData(map[string]string{"reason": reasonOf(err)})
	case errors.Is(err, dbmysql.ErrDatabaseExists):
		return errcode.ErrDBMDatabaseExists
	case errors.Is(err, dbmysql.ErrDatabaseNotFound):
		return errcode.ErrDBMDatabaseNotFound
	case errors.Is(err, dbmysql.ErrUserExists):
		return errcode.ErrDBMDBUserExists
	case errors.Is(err, dbmysql.ErrUserNotFound):
		return errcode.ErrDBMDBUserNotFound
	case errors.Is(err, dbmysql.ErrGrantMissing):
		return errcode.ErrDBMGrantNotFound
	case errors.Is(err, dbredis.ErrKeyNotFound):
		return errcode.ErrDBMKeyNotFound
	case errors.Is(err, dbmysql.ErrDumpFailed):
		return errcode.ErrDBMDumpFailed.WithData(map[string]string{"reason": reasonOf(err)})
	case errors.Is(err, dbmysql.ErrImportFailed):
		return errcode.ErrDBMImportFailed.WithData(map[string]string{"reason": reasonOf(err)})
	default:
		return err
	}
}

// ---- 审计（D-13：13 动作码 category=change；detail 不含密码/密文/密钥）----

// auditEntry 组装四要素审计载荷（actor 来自会话身份；R-03）。
func auditEntry(actor *authsvc.Identity, action, objectType, objectID string, detail any, success bool) audit.Entry {
	result := audit.ResultSuccess
	if !success {
		result = audit.ResultFailure
	}
	e := audit.Entry{
		Category:   audit.CategoryChange,
		Action:     action,
		ObjectType: objectType,
		ObjectID:   objectID,
		Detail:     detail,
		Result:     result,
		Source:     audit.SourceWeb,
	}
	if actor != nil {
		uid := actor.UserID
		e.ActorID = &uid
		e.ActorName = actor.Username
	} else { // 防御：AuthRequired 链路下不可达
		e.ActorName = "unknown"
	}
	return e
}

// recordAudit 独立事务审计写入（引擎侧动作场景；失败重试一次，仍失败降级
// 运行日志，不影响操作结果——sitemgmt auditAction 同法）。
func (s *Service) recordAudit(ctx context.Context, e audit.Entry) {
	if err := s.audit.Record(ctx, e); err != nil {
		if err := s.audit.Record(ctx, e); err != nil {
			slog.WarnContext(ctx, "dbmgmt: audit record degraded to runtime log",
				"action", e.Action, "object", e.ObjectID, "result", e.Result, "err", err)
		}
	}
}

// txAudit 同事务审计写入（档案变更场景：credentials/remove/rescan；
// R-03 同事务原子性——失败整体回滚）。
func (s *Service) txAudit(ctx context.Context, tx TxAPI, e audit.Entry) error {
	if err := tx.Audit().Append(ctx, e, s.clock()); err != nil {
		return err
	}
	return nil
}

// instanceObjectID 审计对象标识（instanceId 字符串形态）。
func instanceObjectID(id int64) string { return strconv.FormatInt(id, 10) }
