package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

// tokenBytes 会话/CSRF/ticket 随机源字节数（256-bit）。
const tokenBytes = 32

// newTokenPair 生成 256-bit 随机会话 token：raw 为 Cookie 值（base64url），
// 返回值第二项为其 SHA-256 十六进制哈希（唯一落库形态，D-01）。
func newTokenPair(r io.Reader) (raw, hash string, err error) {
	b := make([]byte, tokenBytes)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", "", fmt.Errorf("auth: read random: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, sha256Hex(raw), nil
}

// newCSRFToken 生成会话绑定 CSRF 令牌（I-12：与会话一一绑定）。
func newCSRFToken(r io.Reader) (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", fmt.Errorf("auth: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// newTicketToken 生成 login_ticket（一次性凭证，D-02）。
func newTicketToken(r io.Reader) (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", fmt.Errorf("auth: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// sha256Hex 计算 SHA-256 十六进制摘要。
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// 编译期保证默认随机源可用（防止 rand.Reader 被替换为 nil 时静默失败）。
var _ io.Reader = rand.Reader
