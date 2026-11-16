package dbmgmt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

// CredentialVault 是凭据保护领域服务（D-02）：AES-256-GCM 加解密 + 密钥
// 文件 db-cred.key 管理（首次初始化随机生成 32 字节，存面板数据目录独立
// 文件 0600，不入 SQLite、不经任何接口下发；密钥轮换 MVP 不做）。
//
// 密文形态：base64(nonce(12B) + ciphertext) 内嵌 cred_cipher 列；明文仅
// 内存态建连使用（DbGateway 内解密，⚠️A18 零出口）。
type CredentialVault struct {
	key []byte
}

// OpenVault 打开（必要时初始化）凭据保险库：密钥文件位于 dataDir/db-cred.key；
// 缺失时生成 32 字节随机密钥并以 0600 原子写落盘（临时文件+rename，密钥先
// chmod 后 rename——nginxx WriteSecretFileAtomic 同法）。
func OpenVault(dataDir string) (*CredentialVault, error) {
	path := filepath.Join(dataDir, CredKeyFile)
	if b, err := os.ReadFile(path); err == nil {
		if len(b) != 32 {
			return nil, fmt.Errorf("dbmgmt: cred key file %s has %d bytes, want 32", path, len(b))
		}
		return &CredentialVault{key: b}, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("dbmgmt: read cred key %s: %w", path, err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("dbmgmt: generate cred key: %w", err)
	}
	if err := writeSecretFile(path, key); err != nil {
		return nil, err
	}
	return &CredentialVault{key: key}, nil
}

// writeSecretFile 原子写敏感文件（0600）：同目录临时文件 + chmod + rename。
func writeSecretFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".dougong-credkey-*")
	if err != nil {
		return fmt.Errorf("dbmgmt: cred key temp create in %s: %w", dir, err)
	}
	name := tmp.Name()
	defer os.Remove(name) // rename 成功后为 no-op；失败路径兜底清理
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("dbmgmt: cred key write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("dbmgmt: cred key close %s: %w", name, err)
	}
	if err := os.Chmod(name, 0o600); err != nil {
		return fmt.Errorf("dbmgmt: cred key chmod %s: %w", name, err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("dbmgmt: cred key rename %s → %s: %w", name, path, err)
	}
	return nil
}

// Encrypt 加密明文密码 → base64(nonce+ciphertext)（落库形态）。
func (v *CredentialVault) Encrypt(plaintext string) (string, error) {
	gcm, err := v.gcm()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("dbmgmt: cred nonce: %w", err)
	}
	out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt 解密 cred_cipher 列 → 明文（仅建连路径调用，I-02 零出口）。
func (v *CredentialVault) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("dbmgmt: cred cipher base64: %w", err)
	}
	gcm, err := v.gcm()
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("dbmgmt: cred cipher too short (%d bytes)", len(raw))
	}
	nonce, body := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", fmt.Errorf("dbmgmt: cred cipher open: %w", err)
	}
	return string(plain), nil
}

func (v *CredentialVault) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, fmt.Errorf("dbmgmt: cred aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("dbmgmt: cred gcm: %w", err)
	}
	return gcm, nil
}
