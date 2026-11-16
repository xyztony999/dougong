// Package firewallx 是 ufw / firewalld 防火墙后端适配器（api-firewall.md
// §4.1.5 FirewallGateway 契约承载；idea.md §5.3 firewallx 落位）。通道=本机
// ufw / firewall-cmd CLI：全部外部命令经 exec.CommandContext 字面量参数
// 数组执行（R-08/I-03：无 shell 无拼接），单命令等待上限 30s（api §3.4
// 编译期常量，超时归一 FW_008 语义）；子进程注入 LANG=C/LC_ALL=C 固定输出
// 语言——ufw/firewalld 随 locale 本地化文案，解析需确定性英文输出。
//
// 后端原生语义如实上抛（D-07/D-08）：ufw「Skipping adding existing rule」
// 与 firewalld「ALREADY_ENABLED」归一 ErrAlreadyExists（等价规则不静默），
// 删除缺失归一 ErrRuleNotFound；firewalld 写入为 permanent 先写、runtime
// 后应用两条独立命令（D-08，不用 --reload——其将全量 permanent 回灌
// runtime 可能冲掉面板外 runtime-only 变更），双层结果经 AddResult/DelResult
// 如实呈现，由 service 层映射 FW_006/FW_007/FW_008。
//
// 探测（D-01）：每次调用即时探测、ufw 优先→firewalld，均无返回 none；
// 探测结论即使通道不可用也如实返回（active=false），规则面由 service 层
// 据 FW_009/FW_010 拦截（I-01 不返回过期数据）。
package firewallx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// 后端类型（firewall_backend 枚举：ufw | firewalld | none）。
const (
	BackendUFW       = "ufw"
	BackendFirewalld = "firewalld"
	BackendNone      = "none"
)

// 二进制名（LookPath 解析，不信任外部传入可执行路径——backend-03 §4）。
const (
	ufwBin   = "ufw"
	fwcmdBin = "firewall-cmd"
)

// CommandTimeout 单命令等待上限（api §3.4：30s 编译期常量；超时 ctx 截断
// →命令失败→FW_008）。
const CommandTimeout = 30 * time.Second

// 哨兵错误（service 层翻译为 FW_ 错误码语义，A-W11）。
var (
	// ErrNoBackend 未检出任何防火墙后端（ufw/firewalld 均无）→ FW_009。
	ErrNoBackend = errors.New("firewallx: no firewall backend detected")
	// ErrNotActive 后端已识别但不可用（未运行/通道失败）→ FW_010。
	ErrNotActive = errors.New("firewallx: firewall backend not active")
	// ErrAlreadyExists 等价规则已存在（ufw skipping / firewalld ALREADY_ENABLED，D-07）→ FW_006。
	ErrAlreadyExists = errors.New("firewallx: equivalent rule already exists")
	// ErrRuleNotFound 删除目标不存在（面板外已删，D-07 不静默成功）→ FW_003。
	ErrRuleNotFound = errors.New("firewallx: rule not found")
)

// BackendInfo 是探测结论（api §4.1.4 status 响应数据源）。
type BackendInfo struct {
	Type    string  // ufw | firewalld | none
	Active  bool    // 后端自身运行状态实况透传（OUT-5：面板不提供启停）
	Version *string // 后端版本（读取失败为 nil——探测结论仍返回）
}

// BackendRule 是端口级规则实况投影条目（api §4.1.1 投影字段；I-01 恒为
// 当次后端读取快照，无缓存层）。
type BackendRule struct {
	RuleKey  string  // 后端原生命令定位串（D-03；前端不透明引用）
	Port     string  // 契约显示形（"80" / "8080-8090"；ufw 冒号段已转短横线 A-W2）
	Protocol string  // tcp | udp
	Source   *string // 来源限制 CIDR；nil=任意来源
	Action   string  // allow | deny | reject | limit
	RawSpec  string  // 后端原始语义（ufw 规则原文 / firewalld 端口串）
}

// RuleSpec 是写侧命令入参（D-05 校验后的契约值；Port 为契约短横线形态）。
type RuleSpec struct {
	Port     string // "N" 或 "N-M"
	Protocol string // tcp | udp
	Action   string // allow | deny | reject | limit（firewalld 仅 allow，service 层 FW_004 前置）
	Source   string // 空=任意来源；非空为合法 CIDR
}

// AddResult 是 AddRule 结果（firewalld 双写两层状态 D-08；ufw 无分层恒双真）。
type AddResult struct {
	RuleKey     string // 写入后定位引用（成功响应 data.ruleKey，§4.1.4）
	PermanentOK bool   // permanent 层写入结果
	RuntimeOK   bool   // runtime 层应用结果（false → FW_007 语义）
	Reason      string // 失败层原因（如实携带）
}

// DelResult 是 DeleteRule 结果（结构同 AddResult，D-08 双写同口径）。
type DelResult struct {
	PermanentOK bool
	RuntimeOK   bool
	Reason      string
}

// Gateway 是 ufw/firewalld CLI 网关（api §4.1.5 FirewallGateway 实现；
// 无状态——每次调用即时探测与执行，I-01/I-03）。
type Gateway struct{}

// New 构造网关。
func New() *Gateway { return &Gateway{} }

// Detect 即时探测后端（ufw 优先→firewalld；D-01 无缓存）。二进制探测经
// exec.LookPath（PATH 解析，无命令执行）；版本/运行状态读取失败不报错——
// 探测结论仍返回（active 如实 false，规则面 FW_010）。
func (g *Gateway) Detect(ctx context.Context) (BackendInfo, error) {
	switch t, _ := g.detectType(ctx); t {
	case BackendUFW:
		return g.detectUFW(ctx)
	case BackendFirewalld:
		return g.detectFirewalld(ctx)
	default:
		return BackendInfo{Type: BackendNone}, nil
	}
}

// ListRules 即时读取端口级规则全量投影（I-01/I-02）。
func (g *Gateway) ListRules(ctx context.Context) ([]BackendRule, error) {
	t, err := g.detectType(ctx)
	if err != nil {
		return nil, fmt.Errorf("firewallx detect: %w", err)
	}
	switch t {
	case BackendUFW:
		return g.listUFW(ctx)
	case BackendFirewalld:
		return g.listFirewalld(ctx)
	default:
		return nil, ErrNoBackend
	}
}

// AddRule 添加规则（ufw 四动作参数数组 / firewalld 双写两步 D-08）。
func (g *Gateway) AddRule(ctx context.Context, spec RuleSpec) (AddResult, error) {
	t, err := g.detectType(ctx)
	if err != nil {
		return AddResult{}, fmt.Errorf("firewallx detect: %w", err)
	}
	switch t {
	case BackendUFW:
		return g.addUFW(ctx, spec)
	case BackendFirewalld:
		return g.addFirewalld(ctx, spec)
	default:
		return AddResult{}, ErrNoBackend
	}
}

// DeleteRule 删除规则（ruleKey 即后端原生删除入参，D-03 不透明引用）。
func (g *Gateway) DeleteRule(ctx context.Context, ruleKey string) (DelResult, error) {
	t, err := g.detectType(ctx)
	if err != nil {
		return DelResult{}, fmt.Errorf("firewallx detect: %w", err)
	}
	switch t {
	case BackendUFW:
		return g.deleteUFW(ctx, ruleKey)
	case BackendFirewalld:
		return g.deleteFirewalld(ctx, ruleKey)
	default:
		return DelResult{}, ErrNoBackend
	}
}

// detectType 只判定后端类型（LookPath，无命令执行）。
func (g *Gateway) detectType(context.Context) (string, error) {
	if _, err := exec.LookPath(ufwBin); err == nil {
		return BackendUFW, nil
	}
	if _, err := exec.LookPath(fwcmdBin); err == nil {
		return BackendFirewalld, nil
	}
	return BackendNone, nil
}

// run 执行单条外部命令（字面量参数数组 R-08；30s 上限；LANG=C 固定输出
// 语言）；返回 stdout/stderr 原文（错误归一由调用方按哨兵/包装语义处理）。
func (g *Gateway) run(ctx context.Context, name string, args ...string) (string, string, error) {
	cctx, cancel := context.WithTimeout(ctx, CommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, name, args...)
	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}
