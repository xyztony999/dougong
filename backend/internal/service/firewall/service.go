package firewall

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/dougong/dougong/internal/adapter/firewallx"
	"github.com/dougong/dougong/internal/adapter/netlisten"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// Service 是防火墙门面（5 接口的业务承载；handler 薄壳，不感知 HTTP）。
// 读侧：即时探测/读取（I-01 无缓存）→ 投影/对照；写侧编排（RuleMutationService）：
// 校验（R-14）→ 后端可用 → 动作面（I-04）→ 面板端口涉险两段式（I-05）→
// 命令执行（I-03）→ 恰好一条审计（I-07/D-09）。
type Service struct {
	gw     Gateway
	listen ListenPortSource
	panel  PanelListenConfig
	audit  AuditRecorder
	clock  func() time.Time
}

// New 构造服务（panel 为涉险判定基准 A-W9；rec 为审计 OHS R-03）。
func New(gw Gateway, listen ListenPortSource, panel PanelListenConfig, rec AuditRecorder) *Service {
	return &Service{gw: gw, listen: listen, panel: panel, audit: rec, clock: time.Now}
}

// CreateParams 是 create 请求参数（§4.1.4：{port, protocol, source?, action,
// confirmDanger?}；方向固定入站不作为请求参数——D-05）。
type CreateParams struct {
	Port          string
	Protocol      string
	Action        string
	Source        string // 空=任意来源
	ConfirmDanger bool
}

// now 输出 RFC3339 UTC 时间戳（api §3.1 时间格式）。
func (s *Service) now() string { return s.clock().UTC().Format(time.RFC3339) }

// BackendStatus GET /firewall/backend/status（admin+readonly）：即时探测
// （D-01）→ 结论+权威动作面（探测结论即使通道不可用也如实返回，I-01）。
func (s *Service) BackendStatus(ctx context.Context) (*BackendStatusData, error) {
	info, err := s.gw.Detect(ctx)
	if err != nil {
		return nil, mapGatewayError(err)
	}
	return &BackendStatusData{
		Backend:          info.Type,
		Active:           info.Active,
		Version:          info.Version,
		SupportedActions: SupportedActions(info.Type),
		GeneratedAt:      s.now(),
	}, nil
}

// RuleList GET /firewall/rule/list（admin+readonly）：筛选值域（R-14）→
// 后端可用前置（FW_009/FW_010，I-01）→ 即时全量投影（I-02）→ 服务端筛选+
// 排序（D-02）→ isPanelPort 标注（D-06 从严口径）。
func (s *Service) RuleList(ctx context.Context, actionFilter, protocolFilter string) (*RuleListData, error) {
	acts, err := parseEnumFilter(actionFilter, validActions)
	if err != nil {
		return nil, err
	}
	protos, err := parseEnumFilter(protocolFilter, validProtocols)
	if err != nil {
		return nil, err
	}
	info, err := s.gw.Detect(ctx)
	if err != nil {
		return nil, mapGatewayError(err)
	}
	if e := ensureUsable(info); e != nil {
		return nil, e
	}
	rules, err := s.gw.ListRules(ctx)
	if err != nil {
		return nil, mapGatewayError(err)
	}
	panelPort, err := s.panel.GetListenPort()
	if err != nil {
		return nil, fmt.Errorf("firewall panel listen port: %w", err)
	}
	items := projectRules(rules, acts, protos, int(panelPort))
	return &RuleListData{
		Items: items, Total: len(items),
		Backend:     BackendSummary{Backend: info.Type, Active: info.Active},
		GeneratedAt: s.now(),
	}, nil
}

// PortList GET /firewall/port/list（admin+readonly）：后端可用前置 → 规则
// 投影 + 监听端口实况对照（D-04：allow 族命中端口+协议→allowed_by_rule；
// external=监听地址非 loopback；对照仅入站，出站 OUT-3）。
func (s *Service) PortList(ctx context.Context) (*PortListData, error) {
	info, err := s.gw.Detect(ctx)
	if err != nil {
		return nil, mapGatewayError(err)
	}
	if e := ensureUsable(info); e != nil {
		return nil, e
	}
	rules, err := s.gw.ListRules(ctx)
	if err != nil {
		return nil, mapGatewayError(err)
	}
	listeners, err := s.listen.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("firewall listen ports: %w", err)
	}
	idx := buildAllowIndex(rules)
	items := make([]PortItem, 0, len(listeners))
	for _, lp := range listeners {
		state := OpenNoAllowRule
		if spansCover(idx[protoBase(lp.Proto)], lp.Port) {
			state = OpenAllowedByRule
		}
		items = append(items, PortItem{
			Proto: lp.Proto, Addr: lp.Addr, Port: lp.Port,
			Process: processInfoOf(lp), External: isExternalAddr(lp.Addr),
			OpenState: state,
		})
	}
	return &PortListData{Items: items, Total: len(items), GeneratedAt: s.now()}, nil
}

// CreateRule POST /firewall/rule/create（admin）：RuleMutationService 编排。
// 校验顺序（§4.1.4）：参数值域（FW_001/FW_002）→ 后端可用（FW_009/FW_010）→
// 动作支持面（FW_004）→ 面板端口涉险（FW_005，未发生变更不记审计 I-07）→
// 后端执行（FW_006 等价/FW_007 双写部分失败/FW_008 失败）。
// 审计（I-07/A-W11）：除 FW_005 外每次调用恰好一条（failure 如实、含
// detail.error 符号）；成功 object_id=写入后 ruleKey，失败=请求确定引用
// （port/proto 或 ruleKey 参数）。
func (s *Service) CreateRule(ctx context.Context, actor *authsvc.Identity, req CreateParams) (*CreateData, error) {
	objectID := req.Port + "/" + req.Protocol // 失败路径审计对象引用（ruleKey 未生成前稳定可辨识）
	detail := func() map[string]any {
		var src any
		if req.Source != "" {
			src = req.Source
		}
		return map[string]any{
			"port": req.Port, "protocol": req.Protocol, "source": src,
			"action": req.Action, "confirmDanger": req.ConfirmDanger,
		}
	}
	fail := func(err error) (*CreateData, error) {
		s.auditMutation(ctx, actor, AuditActionRuleCreated, objectID,
			audit.ResultFailure, withErr(detail(), err))
		return nil, err
	}

	// ① 参数值域（D-05/R-14）。
	if e := validatePort(req.Port); e != nil {
		return fail(e)
	}
	if e := validateProtocol(req.Protocol); e != nil {
		return fail(e)
	}
	if e := validateAction(req.Action); e != nil {
		return fail(e)
	}
	if e := validateSource(req.Source); e != nil {
		return fail(e)
	}
	// ② 后端可用（I-01：none 不返回空规则面语义、不活跃不返回过期数据）。
	info, err := s.gw.Detect(ctx)
	if err != nil {
		return fail(mapGatewayError(err))
	}
	if e := ensureUsable(info); e != nil {
		return fail(e)
	}
	// ③ 动作支持面（I-04：supportedActions 服务端权威计算，D-01）。
	if !supportedAction(info.Type, req.Action) {
		return fail(errcode.ErrFwActionNotSupported)
	}
	// ④ 面板端口涉险两段式（I-05/D-06；FW_005 拒绝未发生变更不记审计——I-07）。
	if e := s.guardCreateDanger(req); e != nil {
		return nil, e
	}
	// ⑤ 后端执行（I-03 唯一出口）。
	res, err := s.gw.AddRule(ctx, firewallx.RuleSpec{
		Port: req.Port, Protocol: req.Protocol, Action: req.Action, Source: req.Source,
	})
	if err != nil {
		if errors.Is(err, firewallx.ErrAlreadyExists) { // D-07 如实不静默
			return fail(errcode.ErrFwRuleAlreadyExists)
		}
		if errors.Is(err, firewallx.ErrNoBackend) || errors.Is(err, firewallx.ErrNotActive) {
			return fail(mapGatewayError(err)) // 复核与执行间状态翻转，如实反馈
		}
		return fail(errcode.ErrFwBackendExecFailed.WithData(ReasonData{Reason: err.Error()}))
	}
	if !res.PermanentOK || !res.RuntimeOK { // D-08：permanent 已写、runtime 应用失败 → FW_007（不回滚）
		return fail(errcode.ErrFwRuntimeApplyFailed.WithData(DualLayerData{
			Permanent: LayerWritten, Runtime: LayerFailed, Reason: res.Reason}))
	}
	// ⑥ 成功：恰好一条 success 审计（object_id=写入后 ruleKey）。
	s.auditMutation(ctx, actor, AuditActionRuleCreated, res.RuleKey, audit.ResultSuccess, detail())
	return &CreateData{RuleKey: res.RuleKey, Port: req.Port, Protocol: req.Protocol, Action: req.Action}, nil
}

// DeleteRule POST /firewall/rule/delete（admin）：ruleKey 寻址（D-03 不透明
// 引用）。校验顺序（§4.1.4）：ruleKey 非空（FW_001）→ 后端可用 → 存在性
// 实况复核（FW_003）→ 涉险（allow 族命中面板端口 FW_005，豁免审计）→ 执行。
func (s *Service) DeleteRule(ctx context.Context, actor *authsvc.Identity, ruleKey string, confirmDanger bool) (*DeleteData, error) {
	fail := func(err error) (*DeleteData, error) {
		s.auditMutation(ctx, actor, AuditActionRuleDeleted, ruleKey,
			audit.ResultFailure, withErr(map[string]any{"ruleKey": ruleKey}, err))
		return nil, err
	}

	// ① ruleKey 非空（FW_001）。
	if strings.TrimSpace(ruleKey) == "" {
		return fail(errcode.ErrFwInvalidRuleParam)
	}
	// ② 后端可用。
	info, err := s.gw.Detect(ctx)
	if err != nil {
		return fail(mapGatewayError(err))
	}
	if e := ensureUsable(info); e != nil {
		return fail(e)
	}
	// ③ 存在性实况复核（FW_003：面板外已删不静默成功，D-07）。
	rules, err := s.gw.ListRules(ctx)
	if err != nil {
		return fail(mapGatewayError(err))
	}
	var target *firewallx.BackendRule
	for i := range rules {
		if rules[i].RuleKey == ruleKey {
			target = &rules[i]
			break
		}
	}
	if target == nil {
		return fail(errcode.ErrFwRuleNotFound)
	}
	// ④ 涉险两段式（I-05：delete 目标为 allow 族且命中面板端口）。
	if e := s.guardDeleteDanger(target, confirmDanger); e != nil {
		return nil, e
	}
	// ⑤ 后端执行。
	res, err := s.gw.DeleteRule(ctx, ruleKey)
	if err != nil {
		if errors.Is(err, firewallx.ErrRuleNotFound) { // 复核与执行间被面板外删除，如实
			return fail(errcode.ErrFwRuleNotFound)
		}
		if errors.Is(err, firewallx.ErrNoBackend) || errors.Is(err, firewallx.ErrNotActive) {
			return fail(mapGatewayError(err))
		}
		return fail(errcode.ErrFwBackendExecFailed.WithData(ReasonData{Reason: err.Error()}))
	}
	if !res.PermanentOK || !res.RuntimeOK {
		return fail(errcode.ErrFwRuntimeApplyFailed.WithData(DualLayerData{
			Permanent: LayerWritten, Runtime: LayerFailed, Reason: res.Reason}))
	}
	// ⑥ 成功：恰好一条 success 审计。
	s.auditMutation(ctx, actor, AuditActionRuleDeleted, ruleKey,
		audit.ResultSuccess, map[string]any{"ruleKey": ruleKey})
	return &DeleteData{Deleted: true, RuleKey: ruleKey}, nil
}

// guardCreateDanger create 涉险判定（I-05：action∈{deny,reject} 且端口数值
// 直接命中面板监听端口且未确认——D-06 从严口径不分协议）。
func (s *Service) guardCreateDanger(req CreateParams) error {
	if req.Action != ActionDeny && req.Action != ActionReject {
		return nil
	}
	spans, ok := parseRulePortSpans(req.Port) // 契约形态，① 已校验恒可解析
	if !ok {
		return nil
	}
	port, err := s.panel.GetListenPort()
	if err != nil {
		return fmt.Errorf("firewall panel listen port: %w", err)
	}
	if port > 0 && spansCover(spans, int(port)) && !req.ConfirmDanger {
		return errcode.ErrFwPanelPortAtRisk.WithData(DangerData{
			PanelPort: int(port), RulePort: req.Port, Action: req.Action, Hint: DangerHint,
		})
	}
	return nil
}

// guardDeleteDanger delete 涉险判定（I-05：目标为 allow 族且命中面板端口——
// 删除放行规则同样可封锁面板入口）。
func (s *Service) guardDeleteDanger(target *firewallx.BackendRule, confirmDanger bool) error {
	if !isAllowFamily(target.Action) || confirmDanger {
		return nil
	}
	spans, ok := parseRulePortSpans(target.Port)
	if !ok {
		return nil
	}
	port, err := s.panel.GetListenPort()
	if err != nil {
		return fmt.Errorf("firewall panel listen port: %w", err)
	}
	if port > 0 && spansCover(spans, int(port)) {
		return errcode.ErrFwPanelPortAtRisk.WithData(DangerData{
			PanelPort: int(port), RulePort: target.Port, Action: target.Action, Hint: DangerHint,
		})
	}
	return nil
}

// projectRules 组装规则投影（D-02）：筛选（action?/protocol? 多值）→ 排序
// （端口数值升序、同端口 ruleKey 字典序）→ isPanelPort 标注（D-06）。
func projectRules(rules []firewallx.BackendRule, acts, protos map[string]bool, panelPort int) []RuleItem {
	filtered := make([]firewallx.BackendRule, 0, len(rules))
	for _, r := range rules {
		if acts != nil && !acts[r.Action] {
			continue
		}
		if protos != nil && !protos[r.Protocol] {
			continue
		}
		filtered = append(filtered, r)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		fi, fj := ruleSortPort(filtered[i]), ruleSortPort(filtered[j])
		if fi != fj {
			return fi < fj
		}
		return filtered[i].RuleKey < filtered[j].RuleKey
	})
	items := make([]RuleItem, 0, len(filtered))
	for _, r := range filtered {
		spans, ok := parseRulePortSpans(r.Port)
		items = append(items, RuleItem{
			RuleKey: r.RuleKey, Port: r.Port, Protocol: r.Protocol,
			Direction: DirectionIngress, Source: r.Source, Action: r.Action,
			RawSpec:     r.RawSpec,
			IsPanelPort: ok && panelPort > 0 && spansCover(spans, panelPort),
		})
	}
	return items
}

// ruleSortPort 排序首端口键（不可解析排尾——解析器保证可解析，防御兜底）。
func ruleSortPort(r firewallx.BackendRule) int {
	spans, ok := parseRulePortSpans(r.Port)
	if !ok {
		return 1 << 30
	}
	return spansFirstPort(spans)
}

// buildAllowIndex 组装 allow 族（allow/limit）规则的端口+协议族覆盖索引
// （D-04：deny/reject 族与来源限制维度不参与两态判定；tcp 族匹配 tcp/tcp6、
// udp 族匹配 udp/udp6）。
func buildAllowIndex(rules []firewallx.BackendRule) map[string][]portSpan {
	idx := make(map[string][]portSpan)
	for _, r := range rules {
		if !isAllowFamily(r.Action) {
			continue
		}
		spans, ok := parseRulePortSpans(r.Port)
		if !ok {
			continue
		}
		base := protoBase(r.Protocol)
		idx[base] = append(idx[base], spans...)
	}
	return idx
}

// processInfoOf 组装监听进程载荷（取不到整体 null；pid/name 各自可 null——
// 不虚构，monitoring system_info 同口径）。
func processInfoOf(lp netlisten.ListenPort) *ProcessInfo {
	if lp.Pid <= 0 && lp.Name == "" {
		return nil
	}
	pi := &ProcessInfo{}
	if lp.Pid > 0 {
		p := int(lp.Pid)
		pi.Pid = &p
	}
	if lp.Name != "" {
		n := lp.Name
		pi.Name = &n
	}
	return pi
}

// auditMutation 记录一条操作审计（I-07：每个 create/delete 调用恰好一条；
// D-09：写入失败重试一次，仍失败记面板运行日志兜底——不影响操作结果返回）。
func (s *Service) auditMutation(ctx context.Context, actor *authsvc.Identity,
	action, objectID, result string, detail any) {
	entry := audit.Entry{
		Category:   audit.CategoryChange,
		Action:     action,
		ObjectType: ObjectTypeFirewallRule,
		ObjectID:   objectID,
		Detail:     detail,
		Result:     result,
		Source:     audit.SourceWeb,
	}
	if actor != nil {
		uid := actor.UserID
		entry.ActorID = &uid
		entry.ActorName = actor.Username
	} else { // 防御：AuthRequired 链路下不可达
		entry.ActorName = "unknown"
	}
	if err := s.audit.Record(ctx, entry); err != nil {
		if err := s.audit.Record(ctx, entry); err != nil { // 重试一次（D-09）
			slog.WarnContext(ctx, "firewall: audit record degraded to runtime log",
				"action", action, "objectID", objectID, "result", result, "err", err)
		}
	}
}

// withErr 向审计 detail 附加 {error: 错误码符号}（servicemgmt A-S5 先例）。
func withErr(detail map[string]any, err error) map[string]any {
	var ec *errcode.Error
	if errors.As(err, &ec) {
		detail["error"] = ec.Symbol()
	} else {
		detail["error"] = err.Error()
	}
	return detail
}

// ensureUsable 后端可用性前置（I-01：none→FW_009 不返回空列表、不活跃→
// FW_010 不返回过期数据）。
func ensureUsable(info firewallx.BackendInfo) error {
	switch {
	case info.Type == firewallx.BackendNone:
		return errcode.ErrFwNoBackend
	case !info.Active:
		return errcode.ErrFwBackendNotActive
	}
	return nil
}

// supportedAction 动作是否在后端支持面（I-04）。
func supportedAction(backendType, action string) bool {
	for _, a := range SupportedActions(backendType) {
		if a == action {
			return true
		}
	}
	return false
}

// mapGatewayError 网关哨兵错误 → FW 错误码（§3.2 通道失败归一 FW_008/FW_010
// 语义，reason 如实携带）；未知通道错误亦归一 FW_008（本模块无 INTERNAL 泄漏面）。
func mapGatewayError(err error) error {
	switch {
	case errors.Is(err, firewallx.ErrNoBackend):
		return errcode.ErrFwNoBackend
	case errors.Is(err, firewallx.ErrNotActive):
		return errcode.ErrFwBackendNotActive
	case errors.Is(err, firewallx.ErrAlreadyExists):
		return errcode.ErrFwRuleAlreadyExists
	case errors.Is(err, firewallx.ErrRuleNotFound):
		return errcode.ErrFwRuleNotFound
	default:
		return errcode.ErrFwBackendExecFailed.WithData(ReasonData{Reason: err.Error()})
	}
}
