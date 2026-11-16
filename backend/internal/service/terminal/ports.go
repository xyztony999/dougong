package terminal

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/dougong/dougong/internal/service/audit"
)

// ErrUnsupportedPlatform 平台不支持哨兵（Windows 开发机退化实现；fslocal
// A-F7 同口径——目标环境 Linux 功能完整）。
var ErrUnsupportedPlatform = errors.New("terminal: unsupported platform")

// RunIdentity 是会话运行身份（§4.1.5 值对象；会话期内不可变——换身份=新建会话）。
type RunIdentity struct {
	// Source 取 IdentitySourcePanel / IdentitySourceSpecified。
	Source string
	// SystemUsername specified 分支的目标系统用户名（panel 分支为空）。
	SystemUsername string
}

// PtyHandle 是已 spawn 的 PTY 运行时载体（进程组，R-08）。
type PtyHandle interface {
	// Read 读取 PTY 输出（EOF=子进程退出）。
	Read(p []byte) (n int, err error)
	// Write 写入键入字节。
	Write(p []byte) error
	// Resize 同步终端尺寸（cols/rows）。
	Resize(cols, rows uint16) error
	// Kill 终止进程组并关闭 master（I-02：不遗留孤儿）。
	Kill() error
	// Wait 收割进程（防孤儿，D-03）。
	Wait() error
}

// PtyGateway 是本机 PTY 网关契约（§4.1.5；实现 internal/adapter/ptyx，
// main 装配注入）。spawn 一律参数数组（R-08 禁 shell 拼接）：
// panel_run_user=以面板运行用户直启默认 shell；specified_system_user=
// su 语义代启指定用户登录 shell（不代输密码，D1 落定）。
type PtyGateway interface {
	Spawn(ctx context.Context, identity RunIdentity) (PtyHandle, error)
	// CurrentUser 解析面板运行用户名（A-T3：ready 帧 runUser 权威值）。
	CurrentUser(ctx context.Context) (string, error)
}

// SystemUser 是系统用户实况条目（/etc/passwd 语义，D-02）。
type SystemUser struct {
	Name  string
	UID   uint32
	Shell string
}

// SysUserDirectory 是系统用户实况只读网关（§4.1.5；实现
// internal/adapter/sysuser）。ListLoginShellUsers 含非交互 shell 过滤
// （§3.4 排除集）；Get 返回原始条目（TERM_002/003 判定在 service）。
type SysUserDirectory interface {
	ListLoginShellUsers(ctx context.Context) ([]SystemUser, error)
	Get(ctx context.Context, name string) (SystemUser, bool)
}

// AuditRecorder 是审计写入契约（account-auth OHS：AuditService.Record，
// D-06 两段口径——建立失败拒建 TERM_006 / 断开失败日志兜底）。
type AuditRecorder interface {
	Record(ctx context.Context, e AuditEntry) error
}

// AuditEntry 是审计写入载荷别名（仅依赖本包声明端口；本体为
// internal/service/audit.Entry——与 servicemgmt 同款 OHS 消费形态）。
type AuditEntry = audit.Entry

// PanelSessionVerifier 是面板会话复核契约（SessionAuthService.VerifyWs OHS，
// R-18 只读不续期；握手守卫与 30s 存续期复核共用，D-07）。
type PanelSessionVerifier interface {
	VerifyWs(ctx context.Context, rawToken string) (userID int64, role string, err error)
}

// PanelDirectory 是面板用户目录端口（A-T6：审计 ActorName 解析；
// api/terminal 以 account-auth Service 适配注入）。
type PanelDirectory interface {
	UsernameByID(ctx context.Context, userID int64) (string, bool)
}

// WSConn 是 WS 帧通道抽象（传输层由 api 装配注入 gorilla 适配；service
// 不感知具体 WS 库）。WriteFrame 实现方保证串行化（输出/ready/closed），
// Ping 为协议层控制帧、可与数据帧并发。
type WSConn interface {
	// ReadFrame 阻塞读一条文本帧（返回原始 JSON 字节）。
	ReadFrame() ([]byte, error)
	// WriteFrame 写一条文本帧。
	WriteFrame(payload []byte) error
	// Ping 发送协议层 ping 控制帧。
	Ping() error
	// SetPongHandler 注册 pong 回调（用于读截止续期）。
	SetPongHandler(h func())
	// SetReadDeadline 设置读截止（心跳失联判定，A-T8）。
	SetReadDeadline(t time.Time) error
	// Close 关闭底层连接。
	Close() error
}

// wsReadTimeout 判定读侧超时（读截止到点=连续 2 次未收 pong 的等价实现，
// A-T8；net 超时错误链上 os.ErrDeadlineExceeded）。
func wsReadTimeout(err error) bool {
	return errors.Is(err, os.ErrDeadlineExceeded)
}
