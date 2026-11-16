package acme

import (
	"context"
	"fmt"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/alidns"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/tencentcloud"
)

// v2.0（⚠️A20/Gate 1 改判，D-20）：DNS-01 经 DNS 服务商 API 自动化——
// TXT 记录添加/验证/清理由 lego 官方 DNS provider 完成（服务商 API 模型止步于
// 本包）；初始清单 Cloudflare/阿里云 DNS/腾讯云 DNSPod（注册表可扩展——
// 新增服务商=providerFor 增分支 + DnsCredential 增字段）。
//
// R-09 例外语义与 Issue 同判（用户主动申请/续期调度）；明文凭据由 service 层
// 解密后传入，仅在本函数作用域内构造 provider，不落任何日志/审计（I-D01）。

// DnsCredential 是 DNS 服务商 API 凭据（内存态明文；按 provider 取用字段子集）。
type DnsCredential struct {
	APIToken        string // cloudflare
	AccessKeyID     string // aliyun
	AccessKeySecret string // aliyun
	SecretID        string // dnspod（腾讯云）
	SecretKey       string // dnspod
}

// DNS 提供商标识（dns_provider 枚举同值）。
const (
	ProviderCloudflare = "cloudflare"
	ProviderAliyun     = "aliyun"
	ProviderDNSPod     = "dnspod"
)

// IssueDNS dns_01 签发（lego Obtain 复用账户与 CA 通道；DNS provider 自动
// 完成 TXT Present/CleanUp——清理失败不阻断签发产物返回，由 lego 内部
// Present/CleanUp 语义与 CA 验证轮询承载，D-20）。
func (g *Gateway) IssueDNS(ctx context.Context, domains []string, provider string, cred DnsCredential) (*IssueResult, error) {
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
		return nil, fmt.Errorf("acme: new client (dns): %w", err)
	}
	p, err := providerFor(provider, cred)
	if err != nil {
		return nil, err
	}
	if err := client.Challenge.SetDNS01Provider(p,
		dns01.CondOption(true, dns01.AddRecursiveNameservers([]string{"1.1.1.1", "8.8.8.8"}))); err != nil {
		return nil, fmt.Errorf("acme: set dns-01 provider: %w", err)
	}
	if user.reg == nil {
		reg, err := client.Registration.Register(registrationOptions())
		if err != nil {
			return nil, fmt.Errorf("acme: register account (dns): %w", err)
		}
		user.reg = reg
		if err := g.saveAccount(user); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("acme: issue deadline: %w", err)
	}
	return g.obtain(ctx, client, user, domains)
}

// providerFor 构造 lego DNS provider（凭据形态校验在 service 层完成，此处
// 空值防御返回错误）。
func providerFor(provider string, cred DnsCredential) (challenge.Provider, error) {
	switch provider {
	case ProviderCloudflare:
		if cred.APIToken == "" {
			return nil, fmt.Errorf("acme: cloudflare api token empty")
		}
		return cloudflare.NewDNSProviderConfig(&cloudflare.Config{AuthToken: cred.APIToken})
	case ProviderAliyun:
		if cred.AccessKeyID == "" || cred.AccessKeySecret == "" {
			return nil, fmt.Errorf("acme: aliyun credentials incomplete")
		}
		return alidns.NewDNSProviderConfig(&alidns.Config{
			APIKey: cred.AccessKeyID, SecretKey: cred.AccessKeySecret})
	case ProviderDNSPod:
		// 腾讯云 DNSPod：经 tencentcloud API（SecretId/SecretKey——腾讯云 API
		// 密钥，D-20 字段口径）；lego dnspod 包为 DNSPod 传统 Token 形态不匹配。
		if cred.SecretID == "" || cred.SecretKey == "" {
			return nil, fmt.Errorf("acme: dnspod credentials incomplete")
		}
		return tencentcloud.NewDNSProviderConfig(&tencentcloud.Config{
			SecretID: cred.SecretID, SecretKey: cred.SecretKey})
	default:
		return nil, fmt.Errorf("acme: unknown dns provider %q", provider)
	}
}
