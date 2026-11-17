package sitemgmt

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/dougong/dougong/internal/adapter/acme"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storesitemgmt "github.com/dougong/dougong/internal/store/sitemgmt"
)

// Service 是网站管理门面（18 接口的业务承载；handler 薄壳，不感知 HTTP）。
// site 实时投影零持久化（I-07）；certificate 持久化经 store/sitemgmt。
type Service struct {
	gw         NginxGateway
	phpfpm     PhpFpmGateway
	acme       AcmeGateway
	store      *storesitemgmt.Store
	audit      AuditRecorder
	dnsVault   *DnsCredentialVault // v2.0（⚠️A20）：dns-cred.key 凭据保险库（可空=凭证功能不可用）
	dataDir    string              // 面板数据目录（备份/托管证书/acme 账户与 webroot 派生基点）
	backupRoot string              // {dataDir}/conf.backup（R-06）
	certsDir   string              // {dataDir}/certs（A-SM11）
	webrootDir string              // {dataDir}/acme-webroot（A-SM10）
	// defaultAccessLog/defaultErrorLog：nginx 默认日志路径回退（D-07 ⚠️A15），
	// 可经 Options 覆写——测试环境无关性（CI runner 可能装有 nginx）。
	defaultAccessLog string
	defaultErrorLog  string
	clock            func() time.Time
}

// Options 是路径派生覆盖项（缺省由 DataDir 派生；测试注入用）。
type Options struct {
	DataDir          string
	BackupRoot       string
	CertsDir         string
	WebrootDir       string
	DefaultAccessLog string
	DefaultErrorLog  string
	DnsVault         *DnsCredentialVault // v2.0：nil 时 DNS 凭证功能返回错误（防御）
}

// New 构造服务；clock 为 nil 用 time.Now（白盒测试可覆写）。
func New(gw NginxGateway, phpfpmGw PhpFpmGateway, acmeGw AcmeGateway,
	store *storesitemgmt.Store, auditRec AuditRecorder, opts Options) *Service {
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = "."
	}
	s := &Service{
		gw: gw, phpfpm: phpfpmGw, acme: acmeGw, store: store, audit: auditRec,
		dataDir: dataDir, clock: time.Now, dnsVault: opts.DnsVault,
	}
	s.backupRoot = orDefault(opts.BackupRoot, filepath.Join(dataDir, BackupDirName))
	s.defaultAccessLog = orDefault(opts.DefaultAccessLog, NginxDefaultAccessLog)
	s.defaultErrorLog = orDefault(opts.DefaultErrorLog, NginxDefaultErrorLog)
	s.certsDir = orDefault(opts.CertsDir, filepath.Join(dataDir, "certs"))
	s.webrootDir = orDefault(opts.WebrootDir, acme.DefaultWebrootDir) // v2.4 公共可读
	return s
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// panelWebroot 面板 http-01 webroot（A-SM10 ②注入 location 的 root 值）。
func (s *Service) panelWebroot() string { return s.webrootDir }

// auditAction 记录一条操作审计（R-03；文件系统侧动作成功记一条——A-SM7；
// D-12 写入失败重试一次，仍失败记面板运行日志兜底，不影响操作结果返回）。
func (s *Service) auditAction(ctx context.Context, actor *authsvc.Identity,
	action, objectType, objectID string, detail any, success bool) {
	result := audit.ResultSuccess
	if !success {
		result = audit.ResultFailure
	}
	entry := audit.Entry{
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
		entry.ActorID = &uid
		entry.ActorName = actor.Username
	} else { // 防御：AuthRequired 链路下不可达
		entry.ActorName = "unknown"
	}
	if err := s.audit.Record(ctx, entry); err != nil {
		if err := s.audit.Record(ctx, entry); err != nil { // 重试一次（D-12）
			slog.WarnContext(ctx, "site-mgmt: audit record degraded to runtime log",
				"action", action, "object", objectID, "result", result, "err", err)
		}
	}
}
