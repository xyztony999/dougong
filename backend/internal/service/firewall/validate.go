package firewall

import (
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/errcode"
)

// createPortPattern 契约端口形态（api §3.4 业务常量：^([1-9]\d{0,4})(
// -([1-9]\d{0,4}))?$ + 1-65535 + N≤M——D-05 校验依据，FW_001）。
var createPortPattern = regexp.MustCompile(`^([1-9]\d{0,4})(-([1-9]\d{0,4}))?$`)

// validatePort 校验 create 请求端口（契约短横线形态；R-14 全部服务端执行）。
func validatePort(port string) error {
	if !createPortPattern.MatchString(port) {
		return errcode.ErrFwInvalidRuleParam
	}
	if _, _, ok := parsePortRange(port); !ok { // 1..65535 且 N≤M
		return errcode.ErrFwInvalidRuleParam
	}
	return nil
}

// validateProtocol 校验协议枚举值域（FW_001）。
func validateProtocol(p string) error {
	if !validProtocols[p] {
		return errcode.ErrFwInvalidRuleParam
	}
	return nil
}

// validateAction 校验动作枚举值域（FW_001；支持面越界另由 FW_004 承载）。
func validateAction(a string) error {
	if !validActions[a] {
		return errcode.ErrFwInvalidRuleParam
	}
	return nil
}

// validateSource 校验来源限制 CIDR（缺省=任意来源；非缺省时合法 IPv4/IPv6
// 网段格式——D-05，FW_002）。
func validateSource(src string) error {
	if src == "" {
		return nil
	}
	if _, _, err := net.ParseCIDR(src); err != nil {
		return errcode.ErrFwInvalidSource
	}
	return nil
}

// parseEnumFilter 解析 list 多值逗号分隔筛选（如 action=allow,limit）：
// 空=不过滤；任一值非法/空段 → FW_001（R-14，D-02）。
func parseEnumFilter(raw string, valid map[string]bool) (map[string]bool, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	out := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" || !valid[part] {
			return nil, errcode.ErrFwInvalidRuleParam
		}
		out[part] = true
	}
	return out, nil
}

// portSpan 是端口数值区间（单端口 lo==hi）。
type portSpan struct{ lo, hi int }

// parsePortRange 解析 "N" 或 "N-M"（短横线形态）→ 1..65535 且 N≤M。
func parsePortRange(s string) (int, int, bool) {
	loS, hiS, isRange := strings.Cut(s, "-")
	lo, err := strconv.Atoi(loS)
	if err != nil || lo < 1 || lo > 65535 {
		return 0, 0, false
	}
	hi := lo
	if isRange {
		hi, err = strconv.Atoi(hiS)
		if err != nil || hi < 1 || hi > 65535 || lo > hi {
			return 0, 0, false
		}
	}
	return lo, hi, true
}

// parseRulePortSpans 解析投影条目端口显示形（"80" / "8080-8090" / 面板外
// 手工添加的逗号多端口形 "80,443"——A-W6）为数值区间集；不可解析返回 false
// （调用方跳过标注/对照，不虚构）。
func parseRulePortSpans(port string) ([]portSpan, bool) {
	if port == "" {
		return nil, false
	}
	var spans []portSpan
	for _, member := range strings.Split(port, ",") {
		lo, hi, ok := parsePortRange(member)
		if !ok {
			return nil, false
		}
		spans = append(spans, portSpan{lo, hi})
	}
	return spans, true
}

// spansCover 端口数值命中判定（D-06 从严口径：区间含该端口即命中）。
func spansCover(spans []portSpan, port int) bool {
	for _, sp := range spans {
		if port >= sp.lo && port <= sp.hi {
			return true
		}
	}
	return false
}

// spansFirstPort 排序键（D-02：端口数值升序）；不可解析排尾（防御——解析器
// 保证可解析，此处兜底）。
func spansFirstPort(spans []portSpan) int {
	const tail = int(^uint32(0) >> 1)
	if len(spans) == 0 {
		return tail
	}
	return spans[0].lo
}

// isExternalAddr 对外监听判定（D-04：监听地址非 loopback；127.0.0.1/::1 为
// loopback；非法地址文本从严视为对外——风险敞口提示不漏报）。
func isExternalAddr(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return true
	}
	return !ip.IsLoopback()
}

// protoBase 协议族归并（tcp6→tcp / udp6→udp；D-04：tcp 族匹配 tcp/tcp6、
// udp 族匹配 udp/udp6）。
func protoBase(proto string) string {
	if len(proto) > 3 {
		return proto[:3]
	}
	return proto
}
