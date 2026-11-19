package dbtools

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

// ToolVault 工具凭据保险库（api §4.1.1 ToolCredentialVault，I-02）：
// AES-256-GCM 加解密 + 密钥文件 dbtool-cred.key（dataDir 下 0600，首次
// 初始化 crypto/rand 32B，沿 dbmgmt CredentialVault / dns-cred.key 先例——
// 独立密钥文件风险域隔离）。密文形态 base64(nonce(12B)+ciphertext) 内嵌
// db_tool.cred_cipher；明文仅生成与注入内存态，永不回显。
type ToolVault struct {
	key []byte
}

// OpenToolVault 打开（必要时初始化）工具凭据保险库。
func OpenToolVault(dataDir string) (*ToolVault, error) {
	path := filepath.Join(dataDir, CredKeyFile)
	if b, err := os.ReadFile(path); err == nil {
		if len(b) != 32 {
			return nil, fmt.Errorf("dbtools: cred key file %s has %d bytes, want 32", path, len(b))
		}
		return &ToolVault{key: b}, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("dbtools: read cred key %s: %w", path, err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("dbtools: generate cred key: %w", err)
	}
	if err := writeKeyFile(path, key); err != nil {
		return nil, err
	}
	return &ToolVault{key: key}, nil
}

// writeKeyFile 原子写密钥文件（0600：临时文件+chmod+rename，先 chmod 后
// rename——nginxx WriteSecretFileAtomic 同法）。
func writeKeyFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("dbtools: cred key dir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".dougong-dbtoolkey-*")
	if err != nil {
		return fmt.Errorf("dbtools: cred key temp create in %s: %w", dir, err)
	}
	name := tmp.Name()
	defer os.Remove(name) // rename 成功后 no-op；失败兜底清理
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("dbtools: cred key write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("dbtools: cred key close %s: %w", name, err)
	}
	if err := os.Chmod(name, 0o600); err != nil {
		return fmt.Errorf("dbtools: cred key chmod %s: %w", name, err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("dbtools: cred key rename %s → %s: %w", name, path, err)
	}
	return nil
}

// Encrypt 加密凭据 JSON 明文 → base64(nonce+ciphertext)（落库形态）。
func (v *ToolVault) Encrypt(plaintext string) (string, error) {
	gcm, err := v.gcm()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("dbtools: cred nonce: %w", err)
	}
	out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt 解密 cred_cipher → 明文（仅服务端注入路径调用，I-02 零出口）。
func (v *ToolVault) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("dbtools: cred cipher base64: %w", err)
	}
	gcm, err := v.gcm()
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("dbtools: cred cipher too short (%d bytes)", len(raw))
	}
	nonce, body := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", fmt.Errorf("dbtools: cred cipher open: %w", err)
	}
	return string(plain), nil
}

func (v *ToolVault) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, fmt.Errorf("dbtools: cred aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("dbtools: cred gcm: %w", err)
	}
	return gcm, nil
}

// randomBytes crypto/rand 随机字节（令牌 jti/密钥/口令共用）。
func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("dbtools: crypto/rand: %w", err)
	}
	return b, nil
}
