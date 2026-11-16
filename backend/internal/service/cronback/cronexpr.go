package cronback

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cronexpr.go 是 CronExpr 领域服务（§4.1.1/D-03）：标准五段（分 时 日 月 周）
// 解析，支持 `*` `,` `-` `/步进`；@yearly/@monthly/@weekly/@daily/@hourly 宏
// 仅输入侧接受、写入时归一化为等价五段；预览/调度推算共用 Next 扫描。
//
// 语义对齐 vixie cron：dom 与 dow 同时受限（均非 *）时按 OR 联合匹配；
// 周域 0 与 7 均为周日。

// 宏归一化表（D-03：crontab 存五段，与系统语义一致）。
var cronMacros = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

// fieldDomain 是各段取值域（dow 0..6，7 归一为 0）。
type fieldDomain struct {
	lo, hi int
}

var exprDomains = []fieldDomain{
	{0, 59}, // minute
	{0, 23}, // hour
	{1, 31}, // day of month
	{1, 12}, // month
	{0, 7},  // day of week（7→0 周日）
}

var fieldNames = []string{"分钟", "小时", "日", "月", "周"}

// CronExpr 是解析后的五段表达式。
type CronExpr struct {
	minutes map[int]bool
	hours   map[int]bool
	doms    map[int]bool
	months  map[int]bool
	dows    map[int]bool

	domStar bool // dom 是否为 *（vixie OR 语义判定）
	dowStar bool
}

// NormalizeCronExpr 宏归一化（非宏原样返回；@reboot 等非周期宏不支持——
// 面板调度域外，校验层拒绝）。
func NormalizeCronExpr(expr string) string {
	trimmed := strings.TrimSpace(expr)
	if five, ok := cronMacros[strings.ToLower(trimmed)]; ok {
		return five
	}
	return trimmed
}

// ParseCronExpr 解析五段表达式；失败返回段位级错误清单（CB_002 data.errors[]）。
func ParseCronExpr(expr string) (*CronExpr, []string) {
	fields := strings.Fields(NormalizeCronExpr(expr))
	var errs []string
	if len(fields) != 5 {
		return nil, []string{fmt.Sprintf("表达式须为五段（分 时 日 月 周），当前 %d 段", len(fields))}
	}
	sets := make([]map[int]bool, 5)
	star := make([]bool, 5)
	for i, f := range fields {
		s, isStar, err := parseField(f, exprDomains[i])
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s段 %q: %v", fieldNames[i], f, err))
			continue
		}
		sets[i] = s
		star[i] = isStar
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return &CronExpr{
		minutes: sets[0], hours: sets[1], doms: sets[2], months: sets[3], dows: sets[4],
		domStar: star[2], dowStar: star[4],
	}, nil
}

// parseField 解析单段：`*` `,` `-` `/步进` 组合；返回值集合与是否纯 *。
func parseField(f string, d fieldDomain) (map[int]bool, bool, error) {
	set := make(map[int]bool)
	step := 1
	rangePart := f
	if i := strings.IndexByte(f, '/'); i >= 0 {
		rangePart = f[:i]
		var err error
		step, err = strconv.Atoi(f[i+1:])
		if err != nil || step < 1 {
			return nil, false, fmt.Errorf("步进非法（/后须为正整数）")
		}
	}
	isStar := rangePart == "*"
	if isStar {
		for v := d.lo; v <= d.hi; v += step {
			set[normDow(v)] = true
		}
		return set, true, nil
	}
	for _, part := range strings.Split(rangePart, ",") {
		lo, hi := 0, 0
		if j := strings.IndexByte(part, '-'); j >= 0 {
			var lerr, rerr error
			lo, lerr = strconv.Atoi(part[:j])
			hi, rerr = strconv.Atoi(part[j+1:])
			if lerr != nil || rerr != nil {
				return nil, false, fmt.Errorf("区间非法")
			}
			if lo > hi {
				return nil, false, fmt.Errorf("区间起点大于终点")
			}
		} else {
			var perr error
			lo, perr = strconv.Atoi(part)
			if perr != nil {
				return nil, false, fmt.Errorf("非数字")
			}
			hi = lo
		}
		if lo < d.lo || hi > d.hi {
			return nil, false, fmt.Errorf("取值越界（%d..%d）", d.lo, d.hi)
		}
		for v := lo; v <= hi; v += step {
			set[normDow(v)] = true
		}
	}
	if len(set) == 0 {
		return nil, false, fmt.Errorf("空集合")
	}
	return set, false, nil
}

// normDow 周域 7 归一为 0（周日，vixie 语义）。
func normDow(v int) int {
	if v == 7 {
		return 0
	}
	return v
}

// matches 判定时刻是否命中（dom/dow vixie OR 语义：两者均受限时任一命中即可）。
func (e *CronExpr) matches(t time.Time) bool {
	if !e.minutes[t.Minute()] || !e.hours[t.Hour()] || !e.months[int(t.Month())] {
		return false
	}
	domHit := e.doms[t.Day()]
	dowHit := e.dows[int(t.Weekday())]
	switch {
	case e.domStar && e.dowStar:
		return true
	case e.domStar:
		return dowHit
	case e.dowStar:
		return domHit
	default:
		return domHit || dowHit
	}
}

// nextScanLimit 扫描上限（4 年 ≈ 2.1M 分钟——覆盖月/日组合的最长空转周期；
// crontab 语义下无解表达式（如 2 月 30 日）如实返回无命中）。
const nextScanLimit = 4 * 366 * 24 * 60

// Next 返回 after 之后（不含）的下一个触发时刻（本地时区=cron 语义）；
// 无命中（超限，如 2 月 30 日）返回零值。
func (e *CronExpr) Next(after time.Time) time.Time {
	t := after.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < nextScanLimit; i++ {
		if e.matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}

// NextFireStr 便捷形态：解析并返回下一触发时刻的 RFC3339 串（无命中空串）。
func NextFireStr(expr string, after time.Time) string {
	e, errs := ParseCronExpr(expr)
	if len(errs) > 0 {
		return ""
	}
	next := e.Next(after)
	if next.IsZero() {
		return ""
	}
	return next.UTC().Format(time.RFC3339)
}

// PreviewFireStrs 预览未来 n 次触发（ISO-8601 UTC，D-03）。
func PreviewFireStrs(expr string, from time.Time, n int) []string {
	e, errs := ParseCronExpr(expr)
	if len(errs) > 0 {
		return nil
	}
	out := make([]string, 0, n)
	t := from
	for len(out) < n {
		next := e.Next(t)
		if next.IsZero() {
			break
		}
		out = append(out, next.UTC().Format(time.RFC3339))
		t = next
	}
	return out
}
