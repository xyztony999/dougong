package dbmysql

import (
	"strings"
)

// quoteIdent 引用标识符（调用方已过白名单校验 R-14——本函数只做反引号
// 包裹防御，白名单保证内容不含反引号；A-DB3）。
func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// quoteUser 组装 'user'@'host' 引用形态（username/host 已过白名单；仍走
// 字符串转义双保险）。
func quoteUser(username, host string) string {
	return quoteString(username) + "@" + quoteString(host)
}

// quoteString 组装 MySQL 字符串字面量（密码等自由文本全量转义：' " \ 与
// 控制字符——A-DB3；NO_BACKSLASH_ESCAPES 两种模式服务器下均安全，因引号
// 双写与反斜杠转义并用了标准子集）。
func quoteString(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\'':
			b.WriteString("\\'")
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case 0:
			b.WriteString("\\0")
		case 0x1a:
			b.WriteString("\\Z")
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// splitTSV 切分 mysql --batch 输出行（空行剔除）。
func splitTSV(out string) []string {
	out = strings.ReplaceAll(strings.TrimRight(out, "\r\n"), "\r\n", "\n")
	if out == "" {
		return nil
	}
	lines := strings.Split(out, "\n")
	kept := lines[:0]
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			kept = append(kept, l)
		}
	}
	return kept
}

// unescapeTSV 反转义 mysql --batch 字段转义（\t \n \\ \0 \Z → 原字节）。
func unescapeTSV(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case '0':
			b.WriteByte(0)
		case 'Z':
			b.WriteByte(0x1a)
		case '\\':
			b.WriteByte('\\')
		default: // 非转义序列（如字面 \x）：保留反斜杠与字符
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// tsvStringPtr 将 TSV 单元转换为可空字符串指针（"NULL" → nil——mysql batch
// 模式 NULL 输出形态）。
func tsvStringPtr(s string) *string {
	if s == "NULL" {
		return nil
	}
	v := unescapeTSV(s)
	return &v
}

// parseGrantLine 解析 SHOW GRANTS 单行（GRANT <perms> ON `db`.* TO 'u'@'h'）
// 提取库级授权目标；未识别形态 DB=""（raw 保留，D-10 不呈现不回收）。
func parseGrantLine(raw string) GrantLine {
	idx := strings.Index(raw, " ON ")
	if idx < 0 {
		return GrantLine{DB: "", Raw: raw}
	}
	rest := raw[idx+4:]
	switch {
	case strings.HasPrefix(rest, "`"):
		if end := strings.Index(rest[1:], "`"); end >= 0 {
			return GrantLine{DB: rest[1 : 1+end], Raw: raw}
		}
	case strings.HasPrefix(rest, "*.*"):
		return GrantLine{DB: "*", Raw: raw}
	}
	return GrantLine{DB: "", Raw: raw}
}

// GrantPrivilegesOf 提取授权行的权限段（GRANT 之后、ON 之前；D-10 反向
// 映射数据源，service 层 GrantLevelMapper 消费）。
func GrantPrivilegesOf(raw string) string {
	const lead = "GRANT "
	if !strings.HasPrefix(raw, lead) {
		return ""
	}
	rest := raw[len(lead):]
	if idx := strings.Index(rest, " ON "); idx >= 0 {
		return rest[:idx]
	}
	return ""
}
