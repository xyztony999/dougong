// Package panel 是面板基座（panel-core）应用服务（api-panel-core.md §4.1）：
// panel_config 单例读写与生效分类（D-P02 RestartClassifier）、安全入口路径
// （EntryPathGuard 数据源，D-P03）、面板自身证书自签/导入（D-P04~P06）、
// 菜单树按角色下发（D-P07 ModuleMenuRegistry）、构建期版本信息（FR-03）
// 与双重门控更新检查（D-P09，R-09 零外呼承诺）。
package panel

import (
	"context"
	cryptorand "crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/dougong/dougong/internal/adapter/acme"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storepanel "github.com/dougong/dougong/internal/store/panel"
)

// SessionPolicy 是 internal/config 会话两键的服务端快照（D-P08：只读透传，
// 载体=配置文件，本模块无修改通道）。
type SessionPolicy struct {
	AbsoluteTimeout time.Duration
	IdleTimeout     time.Duration
}

// ListenBinding 是进程启动监听基线（D-P02：启动时从 panel_config 读绑定；
// pendingRestart = 落库值 ≠ 基线，重启后消失，不落库）。
type ListenBinding struct {
	Scope   string
	Address string
	Port    int
}

// Service 是 panel-core 应用服务。
type Service struct {
	st       Store
	clock    func() time.Time
	rand     io.Reader
	sessions SessionPolicy
	startup  ListenBinding
	menus    *MenuRegistry
	source   VersionSource
	// v2.0 面板证书 ACME 通道（ADR-029）：未装配时 cert/request·retry-renewal
	// 触发即 PANEL_008（通道不可用 fail-closed，与版本源 A-P8 同法）。
	acme       AcmeIssuer
	nginx      NginxChannel
	webrootDir string // 面板 http-01 挑战目录基点（{dataDir}/acme-webroot，与 site-mgmt 同源）
	backupRoot string // 挑战注入写前备份目录（{dataDir}/conf.backup/panel-acme/，R-06）
}

// Option 是 Service 可选注入（测试时钟/随机源；main 注入会话策略与版本源）。
type Option func(*Service)

// WithClock 注入时钟（默认 time.Now）。
func WithClock(clock func() time.Time) Option {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

// WithRand 注入随机源（默认 crypto/rand；测试确定性用）。
func WithRand(r io.Reader) Option {
	return func(s *Service) {
		if r != nil {
			s.rand = r
		}
	}
}

// WithSessionPolicy 注入会话策略快照（main 从 internal/config 加载后传入）。
func WithSessionPolicy(p SessionPolicy) Option {
	return func(s *Service) {
		if p.AbsoluteTimeout > 0 && p.IdleTimeout > 0 {
			s.sessions = p
		}
	}
}

// WithVersionSource 注入版本源适配器（D-P09；nil=未装配，触发即 PANEL_005）。
func WithVersionSource(src VersionSource) Option {
	return func(s *Service) {
		s.source = src
	}
}

// WithAcmeChannel 注入面板证书 ACME 通道（v2.0，ADR-029/D-P13：AcmeIssuer=
// site-mgmt 同一 adapter/acme.Gateway；nginx=同一 adapter/nginxx.Gateway；
// dataDir 为面板数据目录——挑战目录与写前备份由此派生，与 site-mgmt 同源）。
func WithAcmeChannel(acmeGw AcmeIssuer, nginxGw NginxChannel, dataDir string) Option {
	return func(s *Service) {
		if acmeGw != nil && nginxGw != nil && dataDir != "" {
			s.acme = acmeGw
			s.nginx = nginxGw
			s.webrootDir = acme.DefaultWebrootDir // v2.4：公共可读挑战目录（nginx worker 可读）
			s.backupRoot = filepath.Join(dataDir, "conf.backup", "panel-acme")
		}
	}
}

// New 构造服务（会话策略零值时 12h/2h 兜底，与 config 默认一致）。
func New(st Store, opts ...Option) *Service {
	s := &Service{
		st:       st,
		clock:    time.Now,
		rand:     cryptorand.Reader,
		sessions: SessionPolicy{AbsoluteTimeout: 12 * time.Hour, IdleTimeout: 2 * time.Hour},
		menus:    NewMenuRegistry(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Menus 返回构建期菜单登记表（D-P07：main 装配阶段登记，运行期只读）。
func (s *Service) Menus() *MenuRegistry { return s.menus }

// now 取当前时刻（统一入口，测试可控）。
func (s *Service) now() time.Time { return s.clock().UTC() }

// withTx 事务编排助手：fn 返回错误即回滚，成功提交（I-P07 原子 +
// I-05 同事务审计）。
func (s *Service) withTx(ctx context.Context, fn func(tx *storepanel.TxStore) error) error {
	tx, err := s.st.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("panel: begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		if rerr := tx.Rollback(); rerr != nil {
			return fmt.Errorf("panel: rollback: %w (tx error: %v)", rerr, err)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("panel: commit: %w", err)
	}
	return nil
}

// Bootstrap 启动自检（main 装配期调用）：确保 panel_config 单例存在
// （I-P01；缺失时以 crypto/rand 随机高位端口插入——安装期「随机高位端口」
// 语义的运行库首启兜底，tasks.md 裁定 A-P6），读取当前值并记录进程启动
// 监听基线（D-P02 pendingRestart 判定依据 + main 绑定数据源）。
func (s *Service) Bootstrap(ctx context.Context) (storepanel.Config, error) {
	port, err := randomListenPort(s.rand)
	if err != nil {
		return storepanel.Config{}, fmt.Errorf("panel: random listen port: %w", err)
	}
	if err := s.st.Config().EnsureSingleton(ctx, port, s.now()); err != nil {
		return storepanel.Config{}, fmt.Errorf("panel: ensure singleton: %w", err)
	}
	cfg, err := s.st.Config().Get(ctx)
	if err != nil {
		return storepanel.Config{}, fmt.Errorf("panel: bootstrap get: %w", err)
	}
	s.startup = ListenBinding{Scope: cfg.ListenScope, Address: derefString(cfg.ListenAddress), Port: cfg.ListenPort}
	return cfg, nil
}

// Detail 组装配置单例全量视图（GET /panel/config/detail）。
func (s *Service) Detail(ctx context.Context) (*DetailView, error) {
	cfg, err := s.st.Config().Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("panel: get config: %w", err)
	}
	return s.buildDetail(ctx, cfg)
}

// buildDetail 由行数据构造视图（update 复用，响应结构同 detail）。
func (s *Service) buildDetail(ctx context.Context, cfg storepanel.Config) (*DetailView, error) {
	var certSummary *CertSummary
	cert, err := s.st.Cert().Get(ctx)
	if err == nil {
		certSummary = &CertSummary{
			Source:    cert.Source,
			Issuer:    cert.Issuer,
			NotBefore: cert.NotBefore,
			NotAfter:  cert.NotAfter,
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("panel: get cert: %w", err)
	}
	return &DetailView{
		Listen: ListenView{
			Scope:      cfg.ListenScope,
			Address:    cfg.ListenAddress,
			Port:       cfg.ListenPort,
			EffectMode: EffectRestartRequired, // RestartClassifier：仅 listen.* 需重启
		},
		EntryPath: EntryPathView{
			Enabled:    cfg.EntryPathEnabled,
			Path:       cfg.EntryPath,
			EffectMode: EffectImmediate,
		},
		BindDomain: BindDomainView{
			Value:      cfg.BindDomain,
			EffectMode: EffectImmediate, // HostGuard 下一请求即用（D-P12/D-P16）
		},
		HTTPS: HTTPSView{
			Mode:       cfg.HTTPSMode,
			Cert:       certSummary,
			EffectMode: EffectImmediate, // 证书材料热读取即时生效（D-P06）
		},
		UpdateCheck: UpdateCheckView{Enabled: cfg.UpdateCheck, EffectMode: EffectImmediate},
		Lang:        LangView{Value: cfg.Lang, Supported: SupportedLangs, EffectMode: EffectImmediate},
		Session: SessionView{
			AbsoluteTimeout: int64(s.sessions.AbsoluteTimeout / time.Second),
			IdleTimeout:     int64(s.sessions.IdleTimeout / time.Second),
		},
		PendingRestart: s.pendingRestart(cfg),
	}, nil
}

// pendingRestart 派生「待重启」标识：listen 组落库值 ≠ 进程启动绑定值
// （D-P02；基线在 Bootstrap 记录，重启后随基线刷新消失，不落库）。
func (s *Service) pendingRestart(cfg storepanel.Config) bool {
	current := ListenBinding{Scope: cfg.ListenScope, Address: derefString(cfg.ListenAddress), Port: cfg.ListenPort}
	return current != s.startup
}

// EntryPathState 供 EntryPathGuard 每请求读取当前安全入口开关与路径值
// （api §5.1 被依赖契约；修改即时生效——下一请求即用新值）。
func (s *Service) EntryPathState(ctx context.Context) (bool, string, error) {
	cfg, err := s.st.Config().Get(ctx)
	if err != nil {
		return false, "", fmt.Errorf("panel: entry path state: %w", err)
	}
	return cfg.EntryPathEnabled, derefString(cfg.EntryPath), nil
}

// BindDomainState 供 HostGuard 每请求读取当前绑定域名（v2.0 api §5.1 被依赖
// 契约，D-P16/ADR-028；绑定/解绑即时生效——下一请求即用新值；读态失败由
// HostGuard fail-closed 拦截）。
func (s *Service) BindDomainState(ctx context.Context) (string, error) {
	cfg, err := s.st.Config().Get(ctx)
	if err != nil {
		return "", fmt.Errorf("panel: bind domain state: %w", err)
	}
	return derefString(cfg.BindDomain), nil
}

// CertMaterial 供 TLS GetCertificate 回调按连接热读取证书材料（D-P06；
// 仅进程内消费——key_pem 永不经任何接口下发，I-P06）。
func (s *Service) CertMaterial(ctx context.Context) (certPEM, keyPEM string, err error) {
	cert, err := s.st.Cert().Get(ctx)
	if err != nil {
		return "", "", fmt.Errorf("panel: cert material: %w", err)
	}
	return cert.CertPEM, cert.KeyPEM, nil
}

// ---- 内部助手 ----

// auditEntry 构造本模块审计四要素基底（category=change，source=web，
// api §3.4 业务常量；手动更新检查触发不落审计——PRD FR-16 口径）。
func auditEntry(actor *authsvc.Identity, action, objectType string, detail any) audit.Entry {
	return audit.Entry{
		ActorID:    &actor.UserID,
		ActorName:  actor.Username,
		Category:   audit.CategoryChange,
		Action:     action,
		ObjectType: objectType,
		ObjectID:   SingletonObjectID,
		Detail:     detail,
		Result:     audit.ResultSuccess,
		Source:     audit.SourceWeb,
	}
}

// invalid 构造 AUTH_012 逐字段校验错误。
func invalid(fields ...errcode.FieldError) *errcode.Error {
	return errcode.ErrValidation.WithFields(fields...)
}

// auditSystemEntry 构造 source=system 的审计基底（4.1.3 自动续期后台动作，
// D-P13/R-03；actor 缺省面板自身）。
func auditSystemEntry(action, objectType string, detail any) audit.Entry {
	return audit.Entry{
		ActorName:  "system",
		Category:   audit.CategoryChange,
		Action:     action,
		ObjectType: objectType,
		ObjectID:   SingletonObjectID,
		Detail:     detail,
		Result:     audit.ResultSuccess,
		Source:     audit.SourceSystem,
	}
}

// fieldDelta 是 panel_config_changed 审计 detail 的单项（api §3.4：
// detail 携带 {field, before, after}[]）。
type fieldDelta struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// maskEntryPath 审计脱敏（api 4.1.4：路径值以「前 2 字符+长度」掩码落
// 审计——审计页面对 readonly 可见，明文路径不得入审计）。
func maskEntryPath(p string) string {
	if p == "" {
		return ""
	}
	head := p
	if len(head) > 2 {
		head = head[:2]
	}
	return fmt.Sprintf("%s****(len=%d)", head, len(p))
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// randomListenPort 生成随机高位端口 [1024, 65535]（安装期随机高位端口语义）。
func randomListenPort(r io.Reader) (int, error) {
	n, err := cryptorand.Int(r, big.NewInt(ListenPortMax-ListenPortMin+1))
	if err != nil {
		return 0, fmt.Errorf("rand int: %w", err)
	}
	return ListenPortMin + int(n.Int64()), nil
}

// validHost 校验合法 IP（v4/v6）或主机名（RFC 1123 标签规则，I-P02/R-14）。
func validHost(addr string) bool {
	if net.ParseIP(addr) != nil {
		return true
	}
	if len(addr) == 0 || len(addr) > 253 {
		return false
	}
	for _, label := range strings.Split(addr, ".") {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		for i, ch := range label {
			switch {
			case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
			case ch == '-':
				if i == 0 || i == len(label)-1 {
					return false // 标签首尾不得为连字符
				}
			default:
				return false
			}
		}
	}
	return true
}
