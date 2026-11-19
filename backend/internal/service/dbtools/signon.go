package dbtools

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	storedbt "github.com/dougong/dougong/internal/store/dbtools"
)

// SignonService signon 桥接令牌服务（D-09 安全核心）：HMAC-SHA256 签发 +
// 四重校验（①签名防伪 ②exp 60s 时效 ③jti 一次性防重放 ④instanceId+
// sessionDigest 双绑定）+ 凭据 loopback 注入响应。消费台账内存态（P7：
// 面板重启全部失效=安全保守）；TTL 清扫 goroutine 由 main 编排（G-06）。
type SignonService struct {
	store   *storedbt.Store
	vault   *ToolVault
	creds   CredentialGateway
	session SessionVerifier
	clock   func() time.Time

	mu     sync.Mutex
	ledger map[string]ledgerEntry // jti → 时效/消费态
}

type ledgerEntry struct {
	exp      time.Time
	consumed bool
}

// tokenPayload 令牌载荷（D-09/4.4.3：规范化 JSON 序列化——结构体字段序固定）。
type tokenPayload struct {
	Jti           string `json:"jti"`
	ToolType      string `json:"toolType"`
	InstanceID    int64  `json:"instanceId"`
	SessionDigest string `json:"sessionDigest"`
	Iat           int64  `json:"iat"`
	Exp           int64  `json:"exp"`
}

func newSignonService(store *storedbt.Store, vault *ToolVault, creds CredentialGateway,
	session SessionVerifier, clock func() time.Time) *SignonService {
	return &SignonService{
		store: store, vault: vault, creds: creds, session: session, clock: clock,
		ledger: map[string]ledgerEntry{},
	}
}

// signonKey 从 phpmyadmin 档案密文取 signon 密钥（仅服务端内存态，I-02）。
func (s *SignonService) signonKey(ctx context.Context) ([]byte, error) {
	a, err := s.store.Tool().GetByType(ctx, storedbt.ToolPHPMyAdmin)
	if err != nil {
		return nil, fmt.Errorf("dbtools: signon get archive: %w", err)
	}
	if a == nil {
		return nil, errcode.ErrDBTNotInstalled.WithData(map[string]any{"installed": false})
	}
	plain, err := s.vault.Decrypt(a.CredCipher)
	if err != nil {
		return nil, err
	}
	var cj toolCredJSON
	if err := json.Unmarshal([]byte(plain), &cj); err != nil {
		return nil, fmt.Errorf("dbtools: signon cred json: %w", err)
	}
	key, err := hex.DecodeString(cj.SignonKey)
	if err != nil || len(key) != SignonKeyBytes {
		return nil, fmt.Errorf("dbtools: signon key invalid")
	}
	return key, nil
}

// Issue 签发令牌（open/reopen 共用同一规约）：payload+HMAC-SHA256，
// base64url(payload).base64url(sig)；jti 入台账未消费态；TTL=60s。
func (s *SignonService) Issue(ctx context.Context, sessionDigest string, instanceID int64) (string, time.Time, error) {
	key, err := s.signonKey(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	jtiRaw, err := randomBytes(16)
	if err != nil {
		return "", time.Time{}, err
	}
	now := s.clock()
	exp := now.Add(SignonTokenTTL)
	p := tokenPayload{
		Jti:           hex.EncodeToString(jtiRaw),
		ToolType:      storedbt.ToolPHPMyAdmin,
		InstanceID:    instanceID,
		SessionDigest: sessionDigest,
		Iat:           now.Unix(),
		Exp:           exp.Unix(),
	}
	payloadJSON, err := json.Marshal(p)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("dbtools: signon payload marshal: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payloadJSON)
	sig := mac.Sum(nil)
	token := base64.RawURLEncoding.EncodeToString(payloadJSON) + "." + base64.RawURLEncoding.EncodeToString(sig)
	s.mu.Lock()
	s.ledger[p.Jti] = ledgerEntry{exp: exp}
	s.mu.Unlock()
	return token, exp, nil
}

// Verify 四重校验 + 凭据取出（D-09/4.4.2；仅 loopback 内部端点调用）。
// forwardedSession=SignonScript 转发的 dg_session cookie（W4 会话绑定校验
// 来源）；instanceCookie=dg_tool_instance（实例绑定辅助校验）。失败 slog.Warn
// 安全日志（jti 摘要，不含令牌/凭据）；不落业务审计（D-16）。
func (s *SignonService) Verify(ctx context.Context, token, forwardedSession, instanceCookie string) (SignonVerifyData, error) {
	fail := func(jti string, ec *errcode.Error) (SignonVerifyData, error) {
		digest := jti
		if len(digest) > 8 {
			digest = digest[:8]
		}
		slog.WarnContext(ctx, "dbtools: signon verify rejected", "jti", digest, "code", ec.Symbol())
		return SignonVerifyData{}, ec
	}
	// ① 签名防伪（HMAC-SHA256 常量时间比对）。
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return fail("", errcode.ErrDBTTokenInvalid)
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fail("", errcode.ErrDBTTokenInvalid)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fail("", errcode.ErrDBTTokenInvalid)
	}
	key, err := s.signonKey(ctx)
	if err != nil {
		return fail("", errcode.ErrDBTTokenInvalid)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payloadJSON)
	if subtle.ConstantTimeCompare(mac.Sum(nil), sig) != 1 {
		return fail("", errcode.ErrDBTTokenInvalid)
	}
	var p tokenPayload
	if err := json.Unmarshal(payloadJSON, &p); err != nil {
		return fail("", errcode.ErrDBTTokenInvalid)
	}
	if p.ToolType != storedbt.ToolPHPMyAdmin || p.Jti == "" {
		return fail(p.Jti, errcode.ErrDBTTokenInvalid)
	}
	// ② exp 时效（服务端时钟权威）。
	if !s.clock().Before(time.Unix(p.Exp, 0)) {
		return fail(p.Jti, errcode.ErrDBTTokenExpired)
	}
	// ③ jti 一次性（消费即标记；台账缺席=重启/清扫后失效，保守拒绝）。
	s.mu.Lock()
	entry, ok := s.ledger[p.Jti]
	alreadyConsumed := ok && entry.consumed
	if ok && !entry.consumed {
		entry.consumed = true
		s.ledger[p.Jti] = entry
	}
	s.mu.Unlock()
	if !ok || alreadyConsumed {
		return fail(p.Jti, errcode.ErrDBTTokenReplayed)
	}
	// ④ 双绑定：instanceId（cookie 辅助）+ sessionDigest（会话标识）。
	if instanceCookie != "" {
		if id, perr := strconv.ParseInt(instanceCookie, 10, 64); perr != nil || id != p.InstanceID {
			return fail(p.Jti, errcode.ErrDBTTokenInvalid)
		}
	}
	if forwardedSession == "" {
		return fail(p.Jti, errcode.ErrDBTTokenInvalid)
	}
	if _, _, verr := s.session.VerifyWs(ctx, forwardedSession); verr != nil {
		return fail(p.Jti, errcode.ErrDBTTokenInvalid) // 会话失效→绑定不符
	}
	if sessionDigestOf(forwardedSession) != p.SessionDigest {
		return fail(p.Jti, errcode.ErrDBTTokenInvalid)
	}
	// 凭据取出（D-10 vault 解密仅服务端；明文仅此次 loopback 响应）。
	creds, err := s.creds.InstanceCredentialsForTool(ctx, p.InstanceID)
	if err != nil {
		return fail(p.Jti, errcode.ErrDBTTokenInvalid.WithData(map[string]any{
			"reason": "实例凭据不可用（请到数据库管理配置）"}))
	}
	return SignonVerifyData{User: creds.Username, Password: creds.Password, Host: creds.Host, Port: creds.Port}, nil
}

// sweep 清扫过期台账条目（TTL 到期条目移除——重放窗口收敛至 exp）。
func (s *SignonService) sweep() {
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	for jti, e := range s.ledger {
		if now.After(e.exp) {
			delete(s.ledger, jti)
		}
	}
}

// RunTokenSweeper 令牌台账 TTL 清扫（main 编排 goroutine 入 wg；ctx 取消
// 退出——G-06；TOKEN_SWEEP_INTERVAL=30s）。
func (s *Service) RunTokenSweeper(ctx context.Context) error {
	ticker := time.NewTicker(TokenSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.signon.sweep()
		}
	}
}

// sessionDigestOf 会话标识推导（account-auth Session.Digest 同口径：
// token SHA-256 hex 前 8 位——store/auth models.go:73；只读推导不续期）。
func sessionDigestOf(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	hexed := hex.EncodeToString(sum[:])
	if len(hexed) >= 8 {
		return hexed[:8]
	}
	return hexed
}
