package sitemgmt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

// DnsCredentialVault 是 DNS 凭据材料保护服务（v2.0 ⚠️A20，D-20）：AES-256-GCM
// + 独立密钥文件 dns-cred.key 0600（沿 database-mgmt CredentialVault 先例
// dbmgmt/vault.go 同构实现——服务包间不互相依赖，密钥文件隔离风险域：
// db-cred.key 归 database-mgmt、dns-cred.key 归本模块）。明文仅签发/续期
// 内存态解密（I-D01 零出口：任何接口不回显、审计 detail 零材料）。
type DnsCredentialVault struct{ key []byte }

const dnsCredKeyFile = "dns-cred.key"

// OpenDnsCredentialVault 打开（必要时初始化）密钥库：缺失时生成 32 字节
// 随机密钥 0600 原子落盘（临时文件+chmod+rename）。
func OpenDnsCredentialVault(dataDir string) (*DnsCredentialVault, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("sitemgmt: dns cred key dir: %w", err)
	}
	path := filepath.Join(dataDir, dnsCredKeyFile)
	if b, err := os.ReadFile(path); err == nil {
		if len(b) != 32 {
			return nil, fmt.Errorf("sitemgmt: dns cred key file %s has %d bytes, want 32", path, len(b))
		}
		return &DnsCredentialVault{key: b}, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("sitemgmt: read dns cred key %s: %w", path, err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("sitemgmt: generate dns cred key: %w", err)
	}
	if err := writeDnsSecretFile(path, key); err != nil {
		return nil, err
	}
	return &DnsCredentialVault{key: key}, nil
}

func writeDnsSecretFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".dougong-dnskey-*")
	if err != nil {
		return fmt.Errorf("sitemgmt: dns key temp create in %s: %w", dir, err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("sitemgmt: dns key write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("sitemgmt: dns key close %s: %w", name, err)
	}
	if err := os.Chmod(name, 0o600); err != nil {
		return fmt.Errorf("sitemgmt: dns key chmod %s: %w", name, err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("sitemgmt: dns key rename %s → %s: %w", name, path, err)
	}
	return nil
}

func (v *DnsCredentialVault) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, fmt.Errorf("sitemgmt: dns cred aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("sitemgmt: dns cred gcm: %w", err)
	}
	return gcm, nil
}

// Encrypt 加密凭据材料 JSON → base64(nonce+ciphertext)（credential_enc 落库形态）。
func (v *DnsCredentialVault) Encrypt(plaintext string) (string, error) {
	gcm, err := v.gcm()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("sitemgmt: dns cred nonce: %w", err)
	}
	out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt 解密 credential_enc → 明文 JSON（仅 IssueDNS 建连路径调用）。
func (v *DnsCredentialVault) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("sitemgmt: dns cred cipher base64: %w", err)
	}
	gcm, err := v.gcm()
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("sitemgmt: dns cred cipher too short (%d bytes)", len(raw))
	}
	nonce, body := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", fmt.Errorf("sitemgmt: dns cred cipher open: %w", err)
	}
	return string(plain), nil
}
