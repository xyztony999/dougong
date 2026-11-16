package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// BcryptHasher 是 PasswordHasher 的 bcrypt 实现（§3.2：cost≥10 抗暴力慢哈希；
// Verify 经 CompareHashAndPassword，内建常量时间比较）。
type BcryptHasher struct {
	cost int
}

// NewBcryptHasher 构造哈希器；cost 须在 [10, 31]（bcrypt 上限 31）。
func NewBcryptHasher(cost int) (*BcryptHasher, error) {
	if cost < 10 || cost > 31 {
		return nil, fmt.Errorf("auth: bcrypt cost %d out of range [10,31]", cost)
	}
	return &BcryptHasher{cost: cost}, nil
}

// Hash 生成密码哈希（明文长度 ≤72 已由 service 校验，此处兜底返回错误）。
func (h *BcryptHasher) Hash(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), h.cost)
	if err != nil {
		return "", fmt.Errorf("auth: bcrypt hash: %w", err)
	}
	return string(b), nil
}

// Verify 常量时间口令比较（错误不区分原因，防侧信道）。
func (h *BcryptHasher) Verify(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
