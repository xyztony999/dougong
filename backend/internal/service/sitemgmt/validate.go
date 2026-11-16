package sitemgmt

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/errcode"
)

// R-14 输入校验（handler 不信任前端；服务端唯一权威校验通道）。

// validateDomainList server_name 集合校验（SITE_007：空/含空格/通配符越界）。
func validateDomainList(names []string) *errcode.Error {
	if len(names) == 0 {
		return errcode.ErrSiteInvalidDomain
	}
	for _, n := range names {
		if !validDomain(n) {
			return errcode.ErrSiteInvalidDomain
		}
	}
	return nil
}

// validDomain 单域名形态：1..253 字符；通配符仅允许首个标签整体为 "*"；
// 标签 [A-Za-z0-9-] 且不以 - 起讫；无空白无通配混排。
func validDomain(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	if strings.ContainsAny(name, " \t") {
		return false
	}
	labels := strings.Split(name, ".")
	for i, l := range labels {
		if l == "" {
			return false
		}
		if i == 0 && l == "*" {
			continue // *.example.com 单层通配（http-01 不支持通配签发，签发侧另行失败如实呈现）
		}
		if strings.Contains(l, "*") {
			return false
		}
		if len(l) > 63 {
			return false
		}
		for _, r := range l {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
				return false
			}
		}
		if strings.HasPrefix(l, "-") || strings.HasSuffix(l, "-") {
			return false
		}
	}
	return true
}

// validateListenDecl 监听声明校验（SITE_006：端口 1~65535、地址组合形态）。
// 支持形态：80 / 443 ssl / 127.0.0.1:8080 / [::]:80 default_server。
func validateListenDecl(decl string) *errcode.Error {
	fields := strings.Fields(decl)
	if len(fields) == 0 {
		return errcode.ErrSiteInvalidListen
	}
	head := fields[0]
	portPart := head
	switch {
	case strings.HasPrefix(head, "["): // [::]:80
		i := strings.LastIndex(head, "]")
		if i < 0 || i+1 >= len(head) || head[i+1] != ':' {
			return errcode.ErrSiteInvalidListen
		}
		portPart = head[i+2:]
	case strings.Contains(head, ":"): // addr:port
		portPart = head[strings.LastIndex(head, ":")+1:]
	}
	port, err := strconv.Atoi(portPart)
	if err != nil || port < 1 || port > 65535 {
		return errcode.ErrSiteInvalidListen
	}
	for _, m := range fields[1:] { // 修饰词形态防御（无分号/花括号/空白切分）
		if m == "" || strings.ContainsAny(m, ";{}") {
			return errcode.ErrSiteInvalidListen
		}
	}
	return nil
}

// validateUpstream 反代上游校验（SITE_008：http(s)://host[:port] 形态；
// 允许携带路径——proxy_pass 原生语义）。
func validateUpstream(u string) *errcode.Error {
	if u == "" || strings.ContainsAny(u, " \t;{}") {
		return errcode.ErrSiteInvalidUpstream
	}
	pu, err := url.Parse(u)
	if err != nil || (pu.Scheme != "http" && pu.Scheme != "https") || pu.Host == "" {
		return errcode.ErrSiteInvalidUpstream
	}
	if p := pu.Port(); p != "" {
		if n, perr := strconv.Atoi(p); perr != nil || n < 1 || n > 65535 {
			return errcode.ErrSiteInvalidUpstream
		}
	}
	return nil
}

// normalizeDomain 主域名规范化（D-03 文件名派生：小写 + 非 [a-z0-9.-] 归一
// 为 '-'，压缩连续分隔符，截断 100 字符；空值回退 "site"）。
func normalizeDomain(domain string) string {
	var sb strings.Builder
	lastSep := false
	for _, r := range strings.ToLower(domain) {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-'
		if ok && (!lastSep || r != '-') {
			sb.WriteRune(r)
			lastSep = r == '-'
			continue
		}
		if !ok && !lastSep {
			sb.WriteRune('-')
			lastSep = true
		}
	}
	out := strings.Trim(sb.String(), "-.")
	if out == "" {
		return "site"
	}
	if len(out) > 100 {
		out = out[:100]
	}
	return out
}

// fieldError AUTH_012 逐字段错误（A-S6 先例：root 存在性等无专码校验）。
func fieldError(field, msg string) *errcode.Error {
	return errcode.ErrValidation.WithFields(errcode.FieldError{Field: field, Message: msg})
}

// bindFieldError 同 fieldError（查询参数版命名别名）。
func bindFieldError(field, msg string) *errcode.Error { return fieldError(field, msg) }
