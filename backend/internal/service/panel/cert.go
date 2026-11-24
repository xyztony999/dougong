package panel

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"regexp"
	"strings"
	"time"
)

// CertFields 是证书摘要提取结果（CertPairValidator 输出；R-14：解析、
// 配对校验与 issuer/有效期提取，upload 与 detail 共用，api §4.1.4）。
type CertFields struct {
	Issuer    string
	NotBefore time.Time
	NotAfter  time.Time
}

// ValidateCertPair 解析并配对校验证书/私钥 PEM 文本（I-P04/R-14）：
// 解析失败或 cert.PublicKey 与私钥不配对返回错误（上层统一映射 PANEL_002）。
func ValidateCertPair(certPEM, keyPEM string) (*CertFields, error) {
	cert, err := parseCertificatePEM(certPEM)
	if err != nil {
		return nil, fmt.Errorf("parse cert pem: %w", err)
	}
	key, err := parsePrivateKeyPEM(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse key pem: %w", err)
	}
	if err := matchKeyPair(cert, key); err != nil {
		return nil, fmt.Errorf("cert/key mismatch: %w", err)
	}
	return &CertFields{
		Issuer:    cert.Issuer.CommonName,
		NotBefore: cert.NotBefore.UTC(),
		NotAfter:  cert.NotAfter.UTC(),
	}, nil
}

// parseCertificatePEM 解析首块 CERTIFICATE PEM。
func parseCertificatePEM(pemText string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(normalizePEM(pemText)))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("expect CERTIFICATE pem block")
	}
	return x509.ParseCertificate(block.Bytes)
}

// parsePrivateKeyPEM 解析私钥 PEM（PKCS#1 RSA / SEC1 EC / PKCS#8 通用）。
func parsePrivateKeyPEM(pemText string) (crypto.PrivateKey, error) {
	block, _ := pem.Decode([]byte(normalizePEM(pemText)))
	if block == nil {
		return nil, fmt.Errorf("expect private key pem block")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return x509.ParsePKCS8PrivateKey(block.Bytes)
}

// PEM 标记匹配（RE2 不支持反向引用，BEGIN/END 分开匹配，取首块 BEGIN 到其后首个 END 之间的正文）。
var (
	pemBeginRe = regexp.MustCompile(`-----BEGIN ([A-Za-z0-9 ]+)-----`)
	pemEndRe   = regexp.MustCompile(`-----END ([A-Za-z0-9 ]+)-----`)
)

// normalizePEM 容错重排 PEM 文本。背景：单行输入框粘贴多行 PEM 时，浏览器会把换行符替换成
// 空格，得到「-----BEGIN PRIVATE KEY----- MIGH… o+83… -----END PRIVATE KEY-----」这种
// 标记与正文挤在一行、base64 里夹空格的坏格式，pem.Decode 直接拒收。这里把首块标记之间的
// 正文去掉全部空白、按 64 字符折行，恢复标准 PEM；只修空白，不改 base64 内容，配对校验照常。
// 找不到合法标记时原样返回，交给 pem.Decode 报原本的错。
func normalizePEM(text string) string {
	b := pemBeginRe.FindStringSubmatchIndex(text)
	if b == nil {
		return text
	}
	e := pemEndRe.FindStringIndex(text[b[1]:])
	if e == nil {
		return text
	}
	typ := text[b[2]:b[3]]
	body := strings.Join(strings.Fields(text[b[1]:b[1]+e[0]]), "")
	var out strings.Builder
	out.WriteString("-----BEGIN " + typ + "-----\n")
	for i := 0; i < len(body); i += 64 {
		end := i + 64
		if end > len(body) {
			end = len(body)
		}
		out.WriteString(body[i:end])
		out.WriteByte('\n')
	}
	out.WriteString("-----END " + typ + "-----\n")
	return out.String()
}

// matchKeyPair 校验证书公钥与私钥配对（I-P04）。
func matchKeyPair(cert *x509.Certificate, key crypto.PrivateKey) error {
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		priv, ok := key.(*rsa.PrivateKey)
		if !ok || pub.E != priv.E || pub.N.Cmp(priv.N) != 0 {
			return fmt.Errorf("rsa public key does not match private key")
		}
	case *ecdsa.PublicKey:
		priv, ok := key.(*ecdsa.PrivateKey)
		if !ok || pub.Curve != priv.Curve || pub.X.Cmp(priv.X) != 0 || pub.Y.Cmp(priv.Y) != 0 {
			return fmt.Errorf("ecdsa public key does not match private key")
		}
	case ed25519.PublicKey:
		priv, ok := key.(ed25519.PrivateKey)
		if !ok || !pub.Equal(priv.Public()) {
			return fmt.Errorf("ed25519 public key does not match private key")
		}
	default:
		return fmt.Errorf("unsupported certificate public key type %T", cert.PublicKey)
	}
	return nil
}

// IssueSelfSigned 即时生成自签证书（SelfSignedCertIssuer 领域服务，D-P06：
// 切换 https_mode=self_signed 时生成并落 panel_cert，source=self_signed）。
// ECDSA P-256；SAN 含 localhost/127.0.0.1/::1（本机访问兼容）。
func IssueSelfSigned(rnd io.Reader, now time.Time) (certPEM, keyPEM string, fields CertFields, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rnd)
	if err != nil {
		return "", "", CertFields{}, fmt.Errorf("generate key: %w", err)
	}
	serial, err := rand.Int(rnd, new(big.Int).SetInt64(1<<62-1))
	if err != nil {
		return "", "", CertFields{}, fmt.Errorf("serial: %w", err)
	}
	notBefore := now.UTC()
	notAfter := notBefore.Add(selfSignedValidity)
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "dougong-panel", Organization: []string{"dougong"}},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	der, err := x509.CreateCertificate(rnd, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", CertFields{}, fmt.Errorf("create certificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", CertFields{}, fmt.Errorf("marshal pkcs8: %w", err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	return certPEM, keyPEM, CertFields{Issuer: tmpl.Subject.CommonName, NotBefore: notBefore, NotAfter: notAfter}, nil
}
