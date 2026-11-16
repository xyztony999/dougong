package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // TOTP RFC 6238 标准算法为 HMAC-SHA1（#nosec G505 非完整性用途）
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

// TOTP 参数（RFC 6238：30 秒步长、6 位数字码、允许 ±1 窗口时钟偏移）。
const (
	totpStep   = 30 * time.Second
	totpDigits = 6
	totpSkew   = 1
	totpIssuer = "dougong-panel"
)

// TotpService 是 TotpVerifier 领域服务实现。
type TotpService struct {
	rand io.Reader
}

// NewTotpService 构造 TOTP 服务（随机源可注入便于测试）。
func NewTotpService(r io.Reader) *TotpService {
	if r == nil {
		r = rand.Reader
	}
	return &TotpService{rand: r}
}

// GenerateSecret 生成 20 字节（160-bit）Base32 密钥（无填充）。
func (t *TotpService) GenerateSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := io.ReadFull(t.rand, b); err != nil {
		return "", fmt.Errorf("auth: totp random secret: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// Verify 校验 6 位动态码（窗口 ±1；subtle 常量时间比较）。
func (t *TotpService) Verify(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false
	}
	key, err := decodeBase32Secret(secret)
	if err != nil {
		return false
	}
	now = now.UTC()
	for drift := -totpSkew; drift <= totpSkew; drift++ {
		counter := uint64((now.Add(time.Duration(drift) * totpStep)).Unix() / int64(totpStep.Seconds()))
		expect := fmt.Sprintf("%06d", hotp(key, counter))
		if subtle.ConstantTimeCompare([]byte(expect), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// OtpauthURI 生成 otpauth:// 认证器导入 URI（algorithm=SHA1, digits=6, period=30）。
func (t *TotpService) OtpauthURI(secret, account string) string {
	u := url.URL{
		Scheme: "otpauth",
		Host:   "totp",
		Path:   "/" + totpIssuer + ":" + account,
	}
	q := u.Query()
	q.Set("secret", secret)
	q.Set("issuer", totpIssuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", "6")
	q.Set("period", "30")
	u.RawQuery = q.Encode()
	return u.String()
}

func decodeBase32Secret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
}

// hotp 计算 RFC 4226 HOTP（HMAC-SHA1 + 动态截断）。
func hotp(key []byte, counter uint64) int {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key) // #nosec G505 -- RFC 4226 规定 SHA1
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	bin := (int(sum[offset])&0x7f)<<24 |
		(int(sum[offset+1])&0xff)<<16 |
		(int(sum[offset+2])&0xff)<<8 |
		(int(sum[offset+3]) & 0xff)
	return bin % 1_000_000
}

// GenerateCode 依据密钥与时刻推算当前动态码（测试与向导校验复用）。
func GenerateCode(secret string, now time.Time) (string, error) {
	key, err := decodeBase32Secret(secret)
	if err != nil {
		return "", fmt.Errorf("auth: decode totp secret: %w", err)
	}
	counter := uint64(now.UTC().Unix() / int64(totpStep.Seconds()))
	return fmt.Sprintf("%06d", hotp(key, counter)), nil
}
