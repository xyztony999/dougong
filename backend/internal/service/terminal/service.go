package terminal

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
)

// 审计类别/结果/来源常量透传（audit OHS 定稿值，account-auth §3.4）。
const (
	CategorySecurity = audit.CategorySecurity
	ResultSuccess    = audit.ResultSuccess
	SourceWeb        = audit.SourceWeb
	SourceSystem     = audit.SourceSystem
)

// Limits 是会话限流与调度参数（ADR-023 编译期常量为默认值；生产零配置——
// PRD OUT-6，字段化仅为同包测试注入，不透出配置面）。
type Limits struct {
	MaxConcurrent   int
	IdleTimeout     time.Duration
	HardLimit       time.Duration
	PingInterval    time.Duration
	PongWait        time.Duration
	RecheckInterval time.Duration
}

func defaultLimits() Limits {
	return Limits{
		MaxConcurrent:   MaxConcurrentPerUser,
		IdleTimeout:     IdleTimeout,
		HardLimit:       HardLimit,
		PingInterval:    PingInterval,
		PongWait:        PongWait,
		RecheckInterval: PanelRecheckInterval,
	}
}

// WsActor 是 WS 握手身份（TerminalHandshakeGuard 产出、Admit 消费；
// RawToken=panelSessionRef 复核凭据，D-07）。
type WsActor struct {
	UserID   int64
	Username string // 空时经 PanelDirectory 解析（A-T6）
	Role     string
	RawToken string
}

// Service 是终端门面：候选清单查询、握手准入编排（TerminalAdmissionService，
// I-01）、附着交互与统一清理（TerminalCleanupService 单点，I-02/I-07）。
type Service struct {
	pty      PtyGateway
	users    SysUserDirectory
	audit    AuditRecorder
	verifier PanelSessionVerifier
	dir      PanelDirectory
	registry *Registry
	limits   Limits
	clock    func() time.Time
}

// New 构造终端服务（gateways/adapters 由 main 装配注入；clock 为 nil 用
// time.Now）。零持久化：无 store 依赖（I-04）。
func New(pty PtyGateway, users SysUserDirectory, rec AuditRecorder,
	verifier PanelSessionVerifier, dir PanelDirectory, clock func() time.Time) *Service {
	limits := defaultLimits()
	reg := NewRegistry(limits.MaxConcurrent)
	if clock == nil {
		clock = time.Now
	}
	return &Service{pty: pty, users: users, audit: rec, verifier: verifier,
		dir: dir, registry: reg, limits: limits, clock: clock}
}

// ListUserOptions GET /terminal/user/options：系统用户实况直读（D-02）→
// 非交互 shell 过滤与排序在网关 → data={items, generatedAt}。
// 实况读取失败=TERM_007（不返回虚构数据）。
func (s *Service) ListUserOptions(ctx context.Context) (*UserOptionsData, error) {
	users, err := s.users.ListLoginShellUsers(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "terminal user options: read system users", "err", err)
		return nil, errcode.ErrSystemUserDirUnavailable
	}
	items := make([]UserOptionItem, 0, len(users))
	for _, u := range users {
		items = append(items, UserOptionItem{Name: u.Name, UID: u.UID, Shell: u.Shell})
	}
	return &UserOptionsData{Items: items, GeneratedAt: formatTS(s.clock())}, nil
}

// systemUsernamePattern 系统用户名形态（POSIX 便携子集：小写字母/下划线起，
// 含 - _ 数字，可选尾 $；R-14 TERM_005 形态校验）。
var systemUsernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}\$?$`)

// validateConnect 握手参数校验（I-01 ④：identitySource 枚举值/username 形态）。
func validateConnect(identitySource, username string) error {
	var fields []errcode.FieldError
	switch identitySource {
	case IdentitySourcePanel:
		if username != "" { // panel 分支不接受 username
			fields = append(fields, errcode.FieldError{Field: "username",
				Message: "panel_run_user 身份来源无需指定 username"})
		}
	case IdentitySourceSpecified:
		if username == "" {
			fields = append(fields, errcode.FieldError{Field: "username", Message: "必填"})
		} else if !systemUsernamePattern.MatchString(username) {
			fields = append(fields, errcode.FieldError{Field: "username", Message: "系统用户名形态不合法"})
		}
	default:
		fields = append(fields, errcode.FieldError{Field: "identitySource",
			Message: "必须为 panel_run_user 或 specified_system_user"})
	}
	if len(fields) > 0 {
		return errcode.ErrTerminalValidation.WithFields(fields...)
	}
	return nil
}

// Admit 是握手准入编排（I-01 ④~⑧，升级前执行——拒绝仍是 HTTP 语义 §3.1）：
// 参数校验（TERM_005）→ 并发预检（TERM_001）→ 身份校验（TERM_002/003）→
// PTY spawn（TERM_004，参数数组 R-08）→ 建立审计（失败=Kill PTY→TERM_006，
// D-06 从严拒建）。返回未附着会话（Conn=nil，等待升级后 Attach 登记）。
func (s *Service) Admit(ctx context.Context, actor WsActor, identitySource, username,
	sourceIP, userAgent string) (*Session, error) {
	// ④ 参数校验。
	if err := validateConnect(identitySource, username); err != nil {
		return nil, err
	}
	// ⑤ 并发预检（权威判定在 Attach→Registry.Create 原子 CAS，I-05）。
	if current := s.registry.CountByUser(actor.UserID); current >= s.limits.MaxConcurrent {
		return nil, errcode.ErrConcurrentLimit.WithData(map[string]int{
			"limit": s.limits.MaxConcurrent, "current": current})
	}
	// ⑥ 身份校验与运行身份解析。
	identity := RunIdentity{Source: identitySource}
	runUser := ""
	switch identitySource {
	case IdentitySourceSpecified:
		u, ok := s.users.Get(ctx, username)
		if !ok {
			return nil, errcode.ErrSystemUserNotFound
		}
		if !IsInteractiveShell(u.Shell) {
			return nil, errcode.ErrNoLoginShell
		}
		identity.SystemUsername = username
		runUser = username
	default: // panel_run_user
		cur, err := s.pty.CurrentUser(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "terminal admit: resolve panel run user", "err", err)
			return nil, fmt.Errorf("terminal resolve run user: %w", err)
		}
		runUser = cur
	}
	// ⑦ PTY spawn（参数数组，R-08；失败不产生会话对象、不占并发配额）。
	// 子进程生命周期锚定会话作用域 ctx（background 派生、统一清理时取消）：
	// 不随握手请求 ctx 取消（hijack 后取消时机不定），由 close() 单点收敛。
	sessCtx, cancel := context.WithCancel(context.Background())
	handle, err := s.pty.Spawn(sessCtx, identity)
	if err != nil {
		cancel()
		slog.ErrorContext(ctx, "terminal admit: pty spawn", "identity", identity.Source, "err", err)
		return nil, errcode.ErrPtySpawnFailed
	}
	// 会话对象构建（未登记：⑨ 在 Attach 升级成功后原子登记）。
	sessionID, err := newSessionID()
	if err != nil {
		cancel()
		_ = handle.Kill()
		_ = handle.Wait()
		return nil, fmt.Errorf("terminal admit: %w", err)
	}
	actorName := actor.Username
	if actorName == "" && s.dir != nil {
		if resolved, ok := s.dir.UsernameByID(ctx, actor.UserID); ok {
			actorName = resolved
		} else {
			actorName = fmt.Sprintf("user#%d", actor.UserID) // 审计四要素不落空（A-T6）
		}
	}
	now := s.clock()
	sess := &Session{
		ID:              sessionID,
		UserID:          actor.UserID,
		Username:        actorName,
		PanelSessionRef: actor.RawToken,
		Identity:        identity,
		RunUser:         runUser,
		SourceIP:        sourceIP,
		UserAgent:       userAgent,
		CreatedAt:       now,
		LastActiveAt:    now,
		PTY:             handle,
		ring:            newOutputRing(OutputRingSize),
		svc:             s,
		sessCtx:         sessCtx,
		cancelCtx:       cancel,
		HardDeadlineAt:  hardDeadline(now, s.limits.HardLimit),
	}
	// ⑧ 建立审计 terminal_session_opened（security/web；失败→回收已 spawn
	// 的 PTY→TERM_006，D-06——root shell 不容无痕开）。
	if err := s.audit.Record(ctx, AuditEntry{
		ActorID:    &actor.UserID,
		ActorName:  actorName,
		Category:   CategorySecurity,
		Action:     ActionSessionOpened,
		ObjectType: ObjectTypeTerminalSession,
		ObjectID:   sess.ID,
		Detail:     map[string]string{"identitySource": identitySource, "runUser": runUser, "sourceIp": sourceIP},
		Result:     ResultSuccess,
		Source:     SourceWeb,
	}); err != nil {
		slog.ErrorContext(ctx, "terminal admit: open audit failed, rejecting (TERM_006)", "err", err)
		cancel()
		_ = handle.Kill()
		_ = handle.Wait()
		return nil, errcode.ErrAuditUnavailable
	}
	return sess, nil
}

// Attach 完成 ⑨：升级成功后附着 WS 通道——原子登记（配额 CAS，I-05）→
// ready 首帧（会话元信息权威值）→ 启动 PTY 读泵/WS 写泵（连接作用域，
// A-T7）→ 读循环（当前 goroutine 阻塞至会话终结，统一清理已收敛单点）。
func (s *Service) Attach(pending *Session, conn WSConn) error {
	pending.Conn = conn
	if !s.registry.Create(pending) { // 配额竞态（预检与登记间的并发窗口）
		pending.close(ReasonSelfClose, "concurrent limit race on attach")
		return errcode.ErrConcurrentLimit.WithData(map[string]int{
			"limit": s.limits.MaxConcurrent, "current": s.limits.MaxConcurrent})
	}
	// ready 帧：sessionId/identity/limits（前端权威值，D-01）。
	payload, err := marshalFrame(wsServerFrame{Type: FrameReady, Data: ReadyPayload{
		SessionID: pending.ID,
		Identity:  IdentityPayload{Source: pending.Identity.Source, RunUser: pending.RunUser},
		Limits: LimitsPayload{
			MaxConcurrent:      s.limits.MaxConcurrent,
			IdleTimeoutSeconds: int(s.limits.IdleTimeout.Seconds()),
			HardLimitSeconds:   int(s.limits.HardLimit.Seconds()),
		},
	}})
	if err != nil {
		pending.close(ReasonNetworkError, "ready encode")
		return nil
	}
	if err := conn.WriteFrame(payload); err != nil {
		pending.close(ReasonNetworkError, "ready write")
		return nil
	}
	go pending.ptyPump()
	go pending.wsPump()
	pending.serve()
	return nil
}

// Abort 丢弃未附着的已准入会话（升级失败路径，A-T5）：Kill PTY + closed
// 审计（I-07 恰好一对开合审计；conn 未附着不发帧）。
func (s *Service) Abort(pending *Session, reason string) {
	if pending != nil {
		pending.close(reason, "aborted before attach")
	}
}
