package dbredis

import (
	"strconv"
	"strings"
)

// parseInfo 解析 redis-cli INFO 文本（# Section 头 → 节名小写；k:v 行；
// 忽略注释与空行）。
func parseInfo(out string) InfoSections {
	sections := InfoSections{}
	current := ""
	for _, line := range splitLines(out) {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#"):
			current = strings.ToLower(strings.TrimSpace(line[1:]))
			if current != "" && sections[current] == nil {
				sections[current] = map[string]string{}
			}
		default:
			if current == "" {
				continue // 节头前的游离行（异常形态，跳过）
			}
			k, v, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			sections[current][strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return sections
}

// parseSlowlog 解析 SLOWLOG GET --raw 行块输出：每条 4 行（id/timestamp/
// durationUs/command），其后可跟随至多 2 行客户端元数据（addr/name，redis
// 4.0+）——以「下一纯整数码」为下一块起点启发式；MULTI 批量命令的多行
// command 为已知边界（尽力解析，设计未定义其呈现）。
func parseSlowlog(out string) []SlowlogEntry {
	lines := splitLines(out)
	var entries []SlowlogEntry
	i := 0
	for i+3 < len(lines)+1 && i+4 <= len(lines) {
		id, err1 := strconv.ParseInt(lines[i], 10, 64)
		ts, err2 := strconv.ParseInt(lines[i+1], 10, 64)
		us, err3 := strconv.ParseInt(lines[i+2], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			i++ // 对齐到下一个可解析块起点
			continue
		}
		e := SlowlogEntry{ID: id, Timestamp: ts, DurationUs: us, Command: lines[i+3]}
		i += 4
		// 消费可选元数据行（addr 如 127.0.0.1:54321 / client name），最多 2 行。
		for extra := 0; extra < 2 && i < len(lines); extra++ {
			if _, aerr := strconv.ParseInt(lines[i], 10, 64); aerr == nil {
				break // 纯整数码=下一块 id
			}
			i++
		}
		entries = append(entries, e)
	}
	return entries
}
