// Package acme 是 ACME 协议外呼防腐层（api-site-mgmt.md §4.1.5 AcmeGateway
// 契约，D-09）：本项目首个真实外部系统 ACL——ACME order/challenge/authorization
// 等外部概念止步于本包，出参仅证书产物与有效期元数据；R-09 零遥测除外条款的
// 唯一网络出站口（用户主动申请与既有证书续期调度）。
//
// 实现：go-acme/lego v4（idea.md §5.3 选型，A-SM13）。账户密钥 ECDSA P-256
// 存面板数据目录独立文件（acme-account.key，权限 0600，不入 SQLite、不经任何
// 接口下发，D-09）；注册资源持久化 acme-account.json；CA 目录取配置键
// acme.directory_url（默认 Let's Encrypt 生产，可指向 staging，重启生效）。
// 挑战通道（A-SM10）：http_01=webroot（面板数据目录 acme-webroot，经既有站点
// server 块服务 80 通路）；tls_alpn=面板 tlsalpn01 监听器（443 被 nginx 独占
// 时绑定失败如实报错，环境性约束非代码缺陷）。
//
// 超时纪律：ACME_TIMEOUT（120s）经 ctx 注入 HTTP RoundTripper——ctx 到期后
// lego 全部后续网络调用即刻失败、Obtain 随之返回；无自起 goroutine（G-06）。
package acme

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/tlsalpn01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/http/webroot"
	"github.com/go-acme/lego/v4/registration"
)

// 挑战方式取值（acme_challenge 枚举，service 层校验后透传）。
const (
	ChallengeHTTP01  = "http_01"
	ChallengeTLSALPN = "tls_alpn"
)

// Account 文件名约定（面板数据目录内，A-SM11）。
const (
	accountKeyFile  = "acme-account.key"
	accountRegFile  = "acme-account.json"
	httpRequestWait = 30 * time.Second // 单次 ACME API 请求上限（总上限由 ctx 决定）
)

// DefaultWebrootDir http-01 挑战 webroot 默认目录（v2.4 迁出数据目录）：
// nginx worker（www-data/nginx）须可读挑战文件，而面板数据目录为 0700 root
// 不可穿越——webroot 留在数据目录内则注入修好后必然 403（第二层根因）；
// /var/www 为 Debian/RHEL 标准 httpd 内容目录（RHEL SELinux
// httpd_sys_content_t 覆盖），certbot webroot 插件同族路径。
// 测试经 sitemgmt Options.WebrootDir 注入隔离。
const DefaultWebrootDir = "/var/www/dougong-acme"

// Gateway 是 AcmeGateway 适配器（并发安全：无共享可变状态，账户文件按次加载）。
type Gateway struct {
	dirURL  string // CA 目录 URL（acme.directory_url）
	dataDir string // 面板数据目录（账户密钥/注册资源/webroot 所在域）
}

// NewGateway 构造；dirURL 为 CA 目录 URL，dataDir 为面板数据目录。
func NewGateway(dirURL, dataDir string) *Gateway {
	return &Gateway{dirURL: dirURL, dataDir: dataDir}
}

// WebrootDir 返回 http-01 webroot 目录（v2.4：公共可读默认路径，不再派生自
// 0700 数据目录；A-SM10/A-SM11 语义不变——挑战文件落点唯一来源）。
func (g *Gateway) WebrootDir() string { return DefaultWebrootDir }

// IssueResult 是签发/续期产物（外部 ACME 概念翻译后的唯一出口形态）。
type IssueResult struct {
	CertPEM    []byte // fullchain（含中间证书）
	KeyPEM     []byte // 私钥 PEM
	NotBefore  time.Time
	NotAfter   time.Time
	Issuer     string
	AccountURI string // ACME 账户标识（acme_account_ref 落库值）
}

// accountUser 实现 lego registration.User（账户身份：密钥 + 注册资源）。
type accountUser struct {
	email string // 空 contact + ToS agreed（A-SM12：不虚构联系人）
	key   crypto.PrivateKey
	reg   *registration.Resource
}

func (u *accountUser) GetEmail() string                        { return u.email }
func (u *accountUser) GetRegistration() *registration.Resource { return u.reg }
func (u *accountUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

// Issue 执行一次 ACME 签发（续期=同域集重新 Obtain，复用账户与挑战方式，
// D-09 契约 Renew 的实现形态）。domains 为 SAN 全集；challenge ∈
// {http_01, tls_alpn}；webroot 为 http-01 挑战文件落点根目录（空=面板默认
// webroot；站点已有 acme-challenge location 时服务层传入其 root 复用，
// A-SM10）。ctx 到期（ACME_TIMEOUT）→ 错误返回（服务侧转 renew_failed）。
func (g *Gateway) Issue(ctx context.Context, domains []string, challenge, webroot string) (*IssueResult, error) {
	user, err := g.loadAccount()
	if err != nil {
		return nil, err
	}
	cfg := lego.NewConfig(user)
	cfg.CADirURL = g.dirURL
	cfg.Certificate.KeyType = certcryptoDefault()
	cfg.HTTPClient = g.ctxClient(ctx)
	client, err := lego.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("acme: new client: %w", err)
	}
	if err := g.setChallenge(client, challenge, webroot); err != nil {
		return nil, err
	}
	// 账户注册（首次）与注册资源持久化；已注册账户 ResolveAccountByKey 校验
	// 由首次 Obtain 隐式完成（lego 携带 key kid 调用，CA 拒绝时如实报错）。
	if err := g.ensureRegistered(client, user); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("acme: issue deadline: %w", err)
	}
	return g.obtain(ctx, client, user, domains)
}

// certcryptoDefault 证书密钥类型（ECDSA P-256，A-SM13）。
func certcryptoDefault() certcrypto.KeyType { return certcrypto.EC256 }

// ctxClient 构造 ctx 注入的 HTTP 客户端（ACME_TIMEOUT 总上限经 RoundTripper
// 承载；Issue/IssueDNS 共用）。
func (g *Gateway) ctxClient(ctx context.Context) *http.Client {
	return &http.Client{
		Timeout:   httpRequestWait,
		Transport: &ctxTransport{ctx: ctx, base: http.DefaultTransport},
	}
}

// registrationOptions 注册选项（TermsOfServiceAgreed——账户注册共用）。
func registrationOptions() registration.RegisterOptions {
	return registration.RegisterOptions{TermsOfServiceAgreed: true}
}

// ensureRegistered 账户注册（首次）与注册资源持久化（Issue/IssueDNS 共用）。
func (g *Gateway) ensureRegistered(client *lego.Client, user *accountUser) error {
	if user.reg != nil {
		return nil
	}
	reg, err := client.Registration.Register(registrationOptions())
	if err != nil {
		return fmt.Errorf("acme: register account: %w", err)
	}
	user.reg = reg
	return g.saveAccount(user)
}

// obtain Obtain + 证书产物解析（Issue/IssueDNS 共用尾部）。
func (g *Gateway) obtain(ctx context.Context, client *lego.Client, user *accountUser, domains []string) (*IssueResult, error) {
	res, err := client.Certificate.Obtain(certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("acme: issue deadline: %w (cause: %v)", ctx.Err(), err)
		}
		return nil, fmt.Errorf("acme: obtain: %w", err)
	}
	leaf, err := parseLeafCert(res.Certificate)
	if err != nil {
		return nil, err
	}
	return &IssueResult{
		CertPEM:    res.Certificate,
		KeyPEM:     res.PrivateKey,
		NotBefore:  leaf.NotBefore,
		NotAfter:   leaf.NotAfter,
		Issuer:     issuerName(leaf),
		AccountURI: user.reg.URI,
	}, nil
}

// setChallenge 装配挑战 provider（A-SM10；webrootPath 为 http-01 挑战文件
// 落点根目录，空=面板默认 webroot——参数名避开 webroot 包名遮蔽）。
func (g *Gateway) setChallenge(client *lego.Client, challenge, webrootPath string) error {
	switch challenge {
	case ChallengeHTTP01:
		root := webrootPath
		if root == "" {
			root = g.WebrootDir()
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			return fmt.Errorf("acme: webroot mkdir: %w", err)
		}
		p, err := webroot.NewHTTPProvider(root)
		if err != nil {
			return fmt.Errorf("acme: webroot provider: %w", err)
		}
		if err := client.Challenge.SetHTTP01Provider(p); err != nil {
			return fmt.Errorf("acme: set http-01: %w", err)
		}
		return nil
	case ChallengeTLSALPN:
		// 面板独立 tlsalpn01 监听器（443 被 nginx 独占时绑定失败→CERT_004
		// data.reason 如实呈现——环境性约束，A-SM10）。
		p := tlsalpn01.NewProviderServer("", "443")
		if err := client.Challenge.SetTLSALPN01Provider(p); err != nil {
			return fmt.Errorf("acme: set tls-alpn-01: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("acme: unknown challenge %q", challenge)
	}
}

// loadAccount 加载（或首建）账户身份：密钥文件 0600；注册资源 JSON 缺失时
// reg=nil（Issue 内注册后回存）。
func (g *Gateway) loadAccount() (*accountUser, error) {
	u := &accountUser{}
	keyPath := filepath.Join(g.dataDir, accountKeyFile)
	if b, err := os.ReadFile(keyPath); err == nil {
		k, perr := decodeKeyPEM(b)
		if perr != nil {
			return nil, fmt.Errorf("acme: account key %s: %w", keyPath, perr)
		}
		u.key = k
	} else if os.IsNotExist(err) {
		k, kerr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if kerr != nil {
			return nil, fmt.Errorf("acme: generate account key: %w", kerr)
		}
		if derr := os.MkdirAll(g.dataDir, 0o755); derr != nil {
			return nil, fmt.Errorf("acme: account dir: %w", derr)
		}
		pemB, _ := encodeKeyPEM(k)
		if werr := os.WriteFile(keyPath, pemB, 0o600); werr != nil {
			return nil, fmt.Errorf("acme: write account key: %w", werr)
		}
		u.key = k
	} else {
		return nil, fmt.Errorf("acme: read account key: %w", err)
	}
	if b, err := os.ReadFile(filepath.Join(g.dataDir, accountRegFile)); err == nil {
		reg := &registration.Resource{}
		if jerr := json.Unmarshal(b, reg); jerr != nil {
			return nil, fmt.Errorf("acme: parse account reg: %w", jerr)
		}
		u.reg = reg
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("acme: read account reg: %w", err)
	}
	return u, nil
}

// saveAccount 持久化注册资源（密钥已在 loadAccount 首建时落盘）。
func (g *Gateway) saveAccount(u *accountUser) error {
	b, err := json.Marshal(u.reg)
	if err != nil {
		return fmt.Errorf("acme: marshal account reg: %w", err)
	}
	if err := os.WriteFile(filepath.Join(g.dataDir, accountRegFile), b, 0o600); err != nil {
		return fmt.Errorf("acme: write account reg: %w", err)
	}
	return nil
}

// ctxTransport 把面板 ctx 注入 lego 全部 HTTP 请求（ACME_TIMEOUT 总上限的
// 无 goroutine 实现法：ctx 到期后所有在途与后续请求即刻失败，Obtain 随之
// 返回——G-06 纪律下不引入自起 goroutine）。
type ctxTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

// RoundTrip 注入 ctx 并转发底层传输（不可撤销的请求由 http 层随 ctx 取消）。
func (t *ctxTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.ctx.Err(); err != nil {
		return nil, fmt.Errorf("acme: transport deadline: %w", err)
	}
	return t.base.RoundTrip(req.WithContext(t.ctx))
}

// parseLeafCert 解析 fullchain PEM 的叶证书（首块）。
func parseLeafCert(pemBytes []byte) (*x509.Certificate, error) {
	for len(pemBytes) > 0 {
		var block *pem.Block
		block, pemBytes = pem.Decode(pemBytes)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("acme: parse leaf certificate: %w", err)
		}
		return c, nil
	}
	return nil, errors.New("acme: no certificate block in issued PEM")
}

// issuerName 签发者可读名（CN 优先，缺省取 Organization 列表）。
func issuerName(c *x509.Certificate) string {
	if c.Issuer.CommonName != "" {
		return c.Issuer.CommonName
	}
	return strings.Join(c.Issuer.Organization, ", ")
}

// encodeKeyPEM / decodeKeyPEM：ECDSA 私钥 SEC1 PEM 编解码。
func encodeKeyPEM(k *ecdsa.PrivateKey) ([]byte, error) {
	b, err := x509.MarshalECPrivateKey(k)
	if err != nil {
		return nil, fmt.Errorf("acme: marshal account key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b}), nil
}

func decodeKeyPEM(b []byte) (crypto.PrivateKey, error) {
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}
