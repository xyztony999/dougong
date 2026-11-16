package firewallx

import (
	"context"
	"fmt"
	"strings"
)

// ufw 输出/语义标记（LANG=C 确定性文案）。
const (
	ufwStatusActive   = "Status: active"
	skipExistingRule  = "Skipping adding existing rule"
	deleteMissingHint = "non-existent" // "Could not delete non-existent rule"
)

// detectUFW 探测 ufw：--version 取版本、status 取运行状态；任一读取失败
// 不报错（结论仍返回：version=nil / active=false，D-01/I-01）。
func (g *Gateway) detectUFW(ctx context.Context) (BackendInfo, error) {
	info := BackendInfo{Type: BackendUFW}
	if out, _, err := g.run(ctx, ufwBin, "--version"); err == nil {
		// 首行形如 "ufw 0.36.1"。
		if fields := strings.Fields(strings.TrimSpace(out)); len(fields) >= 2 {
			v := fields[1]
			info.Version = &v
		}
	}
	if out, _, err := g.run(ctx, ufwBin, "status"); err == nil {
		info.Active = strings.Contains(out, ufwStatusActive)
	}
	return info, nil
}

// listUFW 读取 ufw 端口级规则投影（`ufw status` 表格解析）。
func (g *Gateway) listUFW(ctx context.Context) ([]BackendRule, error) {
	out, errOut, err := g.run(ctx, ufwBin, "status")
	if err != nil {
		return nil, fmt.Errorf("ufw status: %w: %s", err, strings.TrimSpace(errOut))
	}
	return parseUFWStatus(out), nil
}

// parseUFWStatus 解析 `ufw status` 表格（LANG=C）：
//
//	Status: active
//
//	To                         Action      From
//	--                         ------      ----
//	22/tcp                     ALLOW IN    Anywhere
//	6000:6007/tcp              ALLOW IN    Anywhere
//	80                         ALLOW IN    Anywhere
//	80,443/tcp                 ALLOW IN    10.0.0.0/8
//	22/tcp (v6)                ALLOW IN    Anywhere (v6)
//
// 投影口径（api §4.1.1 + tasks.md A-W2~A-W6）：端口段冒号形→契约短横线形；
// 裸端口（无 /proto=双栈）按协议族展开 tcp/udp 两条（ruleKey 同为原生 spec，
// 一次 delete 双栈同删）；v6 复制行与 v4 同 ruleKey 按 ruleKey+protocol 去重；
// OUT 方向行、接口绑定（To 列含多个词元如「80/tcp on eth0」）、非端口级
// （To=Anywhere 源限制类）与不可解析行不进端口级入站投影（D-05 方向单值
// 维度——读取面只呈现可忠实映射条目，ruleKey 可靠寻址 D-03）。
func parseUFWStatus(out string) []BackendRule {
	var rules []BackendRule
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue // 表头/分隔线/状态行/残行
		}
		actionIdx := -1
		for i, f := range fields {
			switch strings.ToUpper(f) {
			case "ALLOW", "DENY", "REJECT", "LIMIT":
				actionIdx = i
			}
		}
		if actionIdx < 1 {
			continue // 无动作词或 To 列缺失
		}
		var toTokens []string
		for _, f := range fields[:actionIdx] {
			if f != "(v6)" {
				toTokens = append(toTokens, f)
			}
		}
		if len(toTokens) != 1 {
			continue // 接口绑定等复合形态（A-W4）
		}
		toSpec := toTokens[0]
		action := strings.ToLower(fields[actionIdx])
		if strings.ToUpper(fields[actionIdx+1]) != "IN" {
			continue // OUT 方向不进入站投影（A-W4）
		}
		from := strings.Join(fields[actionIdx+2:], " ")
		if strings.Contains(from, " on ") {
			continue // 来源接口绑定形态不可忠实寻址（A-W4 同判）
		}
		var source *string
		switch from {
		case "", "Anywhere", "Anywhere (v6)":
			source = nil
		default:
			src := from
			source = &src
		}
		portPart, proto, hasProto := strings.Cut(toSpec, "/")
		if portPart == "" || portPart == "Anywhere" {
			continue // 非端口级规则
		}
		if hasProto && proto != "tcp" && proto != "udp" {
			continue
		}
		ruleKey := ufwRuleKey(action, toSpec, source)
		raw := strings.TrimSpace(line)
		display := strings.ReplaceAll(portPart, ":", "-") // A-W2 冒号→短横线
		protocols := []string{"tcp", "udp"}
		if hasProto {
			protocols = []string{proto}
		}
		for _, p := range protocols { // A-W3 裸端口双栈展开
			key := ruleKey + "\x00" + p
			if seen[key] {
				continue
			}
			seen[key] = true
			rules = append(rules, BackendRule{
				RuleKey: ruleKey, Port: display, Protocol: p,
				Source: source, Action: action, RawSpec: raw,
			})
		}
	}
	return rules
}

// ufwRuleKey 组装 ufw 原生定位串（delete 子命令接受的 spec 形态，D-03）。
func ufwRuleKey(action, toSpec string, source *string) string {
	key := action + " " + toSpec
	if source != nil && *source != "" {
		key += " from " + *source
	}
	return key
}

// ufwAddArgs 组装 ufw 添加命令参数数组（契约短横线端口段→ufw 原生冒号形
// A-W2；来源经 from 关键字独立词元——R-08 字面量参数数组）。
func ufwAddArgs(spec RuleSpec) []string {
	portPart := strings.ReplaceAll(spec.Port, "-", ":")
	toSpec := portPart + "/" + spec.Protocol
	args := make([]string, 0, 5)
	args = append(args, spec.Action, toSpec)
	if spec.Source != "" {
		args = append(args, "from", spec.Source)
	}
	return args
}

// addUFW 执行 ufw 添加：等价规则（跳过语义，退出码可为 0）如实归一
// ErrAlreadyExists（D-07 不静默）；其余失败包装原文（service→FW_008）。
func (g *Gateway) addUFW(ctx context.Context, spec RuleSpec) (AddResult, error) {
	args := ufwAddArgs(spec)
	out, errOut, err := g.run(ctx, ufwBin, args...)
	combined := out + errOut
	if strings.Contains(combined, skipExistingRule) {
		return AddResult{}, ErrAlreadyExists
	}
	if err != nil {
		return AddResult{}, fmt.Errorf("ufw add %s %s/%s: %w: %s",
			spec.Action, spec.Port, spec.Protocol, err, strings.TrimSpace(errOut))
	}
	portPart := strings.ReplaceAll(spec.Port, "-", ":")
	toSpec := portPart + "/" + spec.Protocol
	var src *string
	if spec.Source != "" {
		s := spec.Source
		src = &s
	}
	return AddResult{
		RuleKey:     ufwRuleKey(spec.Action, toSpec, src),
		PermanentOK: true,
		RuntimeOK:   true,
	}, nil
}

// ufwDeleteArgs 组装 ufw 删除命令参数数组：ruleKey 词元与 delete 子命令
// 原生入参一一对应（D-03——ruleKey 由本网关在 list 组装、服务端存在性
// 复核后才触达此处；即使畸形输入也只是独立 argv 元素，无 shell 语义）。
func ufwDeleteArgs(ruleKey string) []string {
	return append([]string{"delete"}, strings.Fields(ruleKey)...)
}

// deleteUFW 执行 ufw 删除：目标不存在（非 0 退出+non-existent 文案）如实
// 归一 ErrRuleNotFound（D-07 不静默成功）。
func (g *Gateway) deleteUFW(ctx context.Context, ruleKey string) (DelResult, error) {
	out, errOut, err := g.run(ctx, ufwBin, ufwDeleteArgs(ruleKey)...)
	combined := out + errOut
	if strings.Contains(combined, deleteMissingHint) {
		return DelResult{}, ErrRuleNotFound
	}
	if err != nil {
		return DelResult{}, fmt.Errorf("ufw delete %q: %w: %s", ruleKey, err, strings.TrimSpace(errOut))
	}
	return DelResult{PermanentOK: true, RuntimeOK: true}, nil
}
