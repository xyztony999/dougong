package firewallx

import (
	"context"
	"fmt"
	"strings"
)

// firewalld 输出/语义标记（LANG=C 确定性文案）。
const (
	fwcStateRunning = "running"
	fwcNotRunning   = "not running"
	fwcAlreadyOn    = "ALREADY_ENABLED"
	fwcNotEnabled   = "NOT_ENABLED"
	fwcActionAllow  = "allow" // firewalld 支持面唯一动作（D1 口径-2/I-04）
)

// detectFirewalld 探测 firewalld：--version 取版本、--state 取运行状态；
// 读取失败不报错（结论仍返回，D-01/I-01；--state 非 0 退出=not running）。
func (g *Gateway) detectFirewalld(ctx context.Context) (BackendInfo, error) {
	info := BackendInfo{Type: BackendFirewalld}
	if out, _, err := g.run(ctx, fwcmdBin, "--version"); err == nil {
		if fields := strings.Fields(strings.TrimSpace(out)); len(fields) > 0 {
			v := fields[0]
			info.Version = &v
		}
	}
	if out, _, err := g.run(ctx, fwcmdBin, "--state"); err == nil {
		info.Active = strings.TrimSpace(out) == fwcStateRunning
	}
	return info, nil
}

// listFirewalld 读取 firewalld 端口级规则投影：--list-ports（端口规则，
// runtime 实况——面板写侧恒双写，runtime 即生效面）+ --list-rich-rules
// （来源限制规则，A-W7）；服务类规则（--list-services）非端口级不进投影
// （A-W8）。
func (g *Gateway) listFirewalld(ctx context.Context) ([]BackendRule, error) {
	out, errOut, err := g.run(ctx, fwcmdBin, "--list-ports")
	if err != nil {
		combined := out + errOut
		if strings.Contains(combined, fwcNotRunning) {
			return nil, ErrNotActive
		}
		return nil, fmt.Errorf("firewall-cmd --list-ports: %w: %s", err, strings.TrimSpace(errOut))
	}
	rules := parsePortTokens(out)
	richOut, richErrOut, richErr := g.run(ctx, fwcmdBin, "--list-rich-rules")
	if richErr != nil {
		combined := richOut + richErrOut
		if strings.Contains(combined, fwcNotRunning) {
			return nil, ErrNotActive
		}
		// 端口规则已取得：rich 面读取失败不整体失败（部分失败如实降级，
		// 端口级主投影可用；原文进错误信息供 FW_008 reason）。
		return rules, fmt.Errorf("firewall-cmd --list-rich-rules: %w: %s", richErr, strings.TrimSpace(richErrOut))
	}
	return append(rules, parseRichRules(richOut)...), nil
}

// parsePortTokens 解析 --list-ports 输出（空白分隔端口串，如
// "80/tcp 53/udp 8080-8090/tcp"）→ 端口规则投影（action 恒 allow，
// firewalld 端口规则无来源维度）。
func parsePortTokens(out string) []BackendRule {
	var rules []BackendRule
	for _, tok := range strings.Fields(out) {
		port, proto, ok := strings.Cut(tok, "/")
		if !ok || port == "" || (proto != "tcp" && proto != "udp") {
			continue
		}
		rules = append(rules, BackendRule{
			RuleKey:  tok, // D-03：firewalld 端口串即原生定位串
			Port:     port,
			Protocol: proto,
			Action:   fwcActionAllow,
			RawSpec:  tok,
		})
	}
	return rules
}

// parseRichRules 解析 --list-rich-rules 输出（每行一条原生 rich rule）。
func parseRichRules(out string) []BackendRule {
	var rules []BackendRule
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if r, ok := parseRichRule(line); ok {
			rules = append(rules, r)
		}
	}
	return rules
}

// parseRichRule 解析单条 rich rule（原生形态如
// `rule family="ipv4" source address="10.0.0.0/8" port port="80" protocol="tcp" accept`）。
// ruleKey=rich rule 原文（--remove-rich-rule 原生入参，D-03/A-W7）；
// 动作映射 accept→allow / drop→deny / reject→reject；无法忠实解析的行跳过
// （A-W4 同判——不可寻址不投影）。
func parseRichRule(line string) (BackendRule, bool) {
	r := BackendRule{RuleKey: line, RawSpec: line, Action: ""}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return r, false
	}
	switch fields[len(fields)-1] {
	case "accept":
		r.Action = fwcActionAllow
	case "drop":
		r.Action = "deny"
	case "reject":
		r.Action = "reject"
	default:
		return r, false // 未知/无动作形态（mark 等）
	}
	port := quotedValue(line, "port=")
	proto := quotedValue(line, "protocol=")
	if port == "" || (proto != "tcp" && proto != "udp") {
		return r, false
	}
	r.Port, r.Protocol = port, proto
	if addr := quotedValue(line, "address="); addr != "" {
		r.Source = &addr
	}
	return r, true
}

// quotedValue 提取 `key="value"` 形态的引号值（首个匹配；rich rule 中
// `port port="80"` 的子串 `port="` 唯一命中第二个词元）。
func quotedValue(s, key string) string {
	i := strings.Index(s, key+"\"")
	if i < 0 {
		return ""
	}
	rest := s[i+len(key)+1:]
	if j := strings.IndexByte(rest, '"'); j >= 0 {
		return rest[:j]
	}
	return ""
}

// buildRichRule 组装来源限制规则的原生 rich rule 串（A-W7：--add-port 无
// source 入参，来源维度走原生 rich rule；family 按 CIDR 地址族推导）。
func buildRichRule(spec RuleSpec) string {
	family := "ipv4"
	if strings.Contains(spec.Source, ":") {
		family = "ipv6"
	}
	return fmt.Sprintf("rule family=%q source address=%q port port=%q protocol=%q accept",
		family, spec.Source, spec.Port, spec.Protocol)
}

// addFirewalld D-08 双写两步：permanent 先写（--permanent --add-port=/
// --add-rich-rule=）→ runtime 后应用（无 --permanent）两条独立命令。
// permanent 成功 + runtime 失败 → PermanentOK=true/RuntimeOK=false（不回滚
// permanent——写入是用户意图，重启后仍生效；service→FW_007）；
// ALREADY_ENABLED（permanent 层）→ ErrAlreadyExists；runtime 层
// ALREADY_ENABLED 视为已达成（runtime 已开放）。
func (g *Gateway) addFirewalld(ctx context.Context, spec RuleSpec) (AddResult, error) {
	var ruleKey, addFlag string
	if spec.Source == "" {
		ruleKey = spec.Port + "/" + spec.Protocol
		addFlag = "--add-port=" + ruleKey
	} else {
		ruleKey = buildRichRule(spec)
		addFlag = "--add-rich-rule=" + ruleKey
	}
	out, errOut, err := g.run(ctx, fwcmdBin, "--permanent", addFlag)
	if strings.Contains(out+errOut, fwcAlreadyOn) {
		return AddResult{}, ErrAlreadyExists
	}
	if err != nil {
		return AddResult{}, fmt.Errorf("firewall-cmd --permanent %s: %w: %s",
			addFlag, err, strings.TrimSpace(errOut))
	}
	out2, errOut2, err2 := g.run(ctx, fwcmdBin, addFlag)
	if err2 != nil && !strings.Contains(out2+errOut2, fwcAlreadyOn) {
		return AddResult{
			RuleKey: ruleKey, PermanentOK: true, RuntimeOK: false,
			Reason: strings.TrimSpace(err2.Error() + ": " + errOut2),
		}, nil
	}
	return AddResult{RuleKey: ruleKey, PermanentOK: true, RuntimeOK: true}, nil
}

// deleteFirewalld D-08 双写同口径：permanent --remove-port=/--remove-rich-rule=
// → runtime 同参数；NOT_ENABLED（permanent 层）→ ErrRuleNotFound；
// runtime 失败 → 双层结果（service→FW_007）。
func (g *Gateway) deleteFirewalld(ctx context.Context, ruleKey string) (DelResult, error) {
	var flag string
	if strings.ContainsAny(ruleKey, " \t") {
		flag = "--remove-rich-rule=" + ruleKey // 含空白=rich rule 原文（A-W7）
	} else {
		flag = "--remove-port=" + ruleKey
	}
	out, errOut, err := g.run(ctx, fwcmdBin, "--permanent", flag)
	if strings.Contains(out+errOut, fwcNotEnabled) {
		return DelResult{}, ErrRuleNotFound
	}
	if err != nil {
		return DelResult{}, fmt.Errorf("firewall-cmd --permanent %s: %w: %s",
			flag, err, strings.TrimSpace(errOut))
	}
	out2, errOut2, err2 := g.run(ctx, fwcmdBin, flag)
	if err2 != nil && !strings.Contains(out2+errOut2, fwcNotEnabled) {
		return DelResult{
			PermanentOK: true, RuntimeOK: false,
			Reason: strings.TrimSpace(err2.Error() + ": " + errOut2),
		}, nil
	}
	return DelResult{PermanentOK: true, RuntimeOK: true}, nil
}
