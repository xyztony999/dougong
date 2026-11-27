package auth

import (
	"context"
	"time"

	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// Store 是仓储聚合契约：连接池视图 + 事务开启（I-05 跨仓储原子写）。
// 各仓储行为契约见 internal/store/auth 各 Repo 文档注释（仓储接口不在此
// 重复定义——测试采用真实 :memory: SQLite，不 mock 数据库，backend-03 §6；
// 可替换的算法型领域服务仍以接口注入，见 PasswordHasher/TotpVerifier）。
type Store interface {
	Users() *storeauth.UserRepo
	Sessions() *storeauth.SessionRepo
	Totp() *storeauth.TotpRepo
	Webauthn() *storeauth.WebauthnRepo
	Recovery() *storeauth.RecoveryRepo
	Devices() *storeauth.DeviceRepo
	Init() *storeauth.InitRepo
	Audit() *storeauth.AuditRepo
	BeginTx(ctx context.Context) (*storeauth.TxStore, error)
}

// PasswordHasher 是密码慢哈希领域服务（§3.2：bcrypt 类 cost≥10，常量时间比较）。
type PasswordHasher interface {
	Hash(plain string) (string, error)
	Verify(hash, plain string) bool
}

// TotpVerifier 是 TOTP 领域服务（RFC 6238：30 秒窗口 ±1，6 位数字码）。
type TotpVerifier interface {
	Verify(secret, code string, now time.Time) bool
	GenerateSecret() (string, error)
	OtpauthURI(secret, account string) string
}

// WebAuthnProvider 是 WebAuthn/passkey 协议领域服务（adapter/webauthnx 实现，
// 测试可注入 fake）。begin 返回 options JSON + 一次性 token；finish 凭 token 取回
// 服务端 session data 校验。凭据集由 service 层从仓储加载传入（断言匹配用）。
type WebAuthnProvider interface {
	BeginRegistration(ctx context.Context, userID int64, username string, existing []storeauth.WebauthnCredential) (options []byte, token string, err error)
	FinishRegistration(ctx context.Context, token string, response []byte) (storeauth.WebauthnCredential, error)
	BeginLogin(ctx context.Context, userID int64, username string, creds []storeauth.WebauthnCredential) (options []byte, token string, err error)
	FinishLogin(ctx context.Context, token string, response []byte, creds []storeauth.WebauthnCredential) (credentialID []byte, signCount uint32, err error)
}

// WebAuthnFactory 按请求 RP ID（绑定域名或 Host 去端口）与 Origin 构造 provider。
type WebAuthnFactory func(rpID string, origins []string) WebAuthnProvider
