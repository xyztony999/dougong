package nginxx

import (
	"strings"
)

// LocationInfo 是 server 块内 location 的投影（api §4.2.1 locations 字段数据源）。
type LocationInfo struct {
	Path        string // location 参数拼接（"/"、"~ \.php$" 等）
	ProxyPass   string // 首个 proxy_pass 值（无则空）
	FastCGIPass string // 首个 fastcgi_pass 值（无则空）
	Node        *Node  // location 块节点（写回定位用）
}

// RawBlock 是未建模指令/块的原文投影（R-07/A-SM4）。
type RawBlock struct {
	Line int
	Text string
}

// ServerBlock 是一个 server 块的解析快照（D-02 定位 + 投影数据源 + 写回锚点）。
// Dormant=true 表示整体被注释（D-16 停用态）：Node 为剥离注释后重解析所得
// （其区间相对剥离文本），CommentStart/End 为文件内注释组区间。
type ServerBlock struct {
	Index        int // 文件内序号（含 dormant，D-02/A-SM3）
	Dormant      bool
	Node         *Node
	CommentStart int
	CommentEnd   int
	ServerNames  []string
	Listen       []string // 每项为完整声明（"443 ssl"）
	Root         string
	SSLCert      string
	SSLCertKey   string
	AccessLog    string
	ErrorLog     string
	Locations    []LocationInfo
	RawBlocks    []RawBlock
}

// ProxyInfo 是主反代段参数（§4.2.1 proxy 值对象）。
type ProxyInfo struct {
	Upstream   string
	HostHeader string
	Websocket  bool
}

// Proxy 提取主反代段：首个含 proxy_pass 的 location（多反代仅呈现首个，
// api 1.3 D-03 口径）；无则 nil。
func (s *ServerBlock) Proxy() *ProxyInfo {
	for _, loc := range s.Locations {
		if loc.ProxyPass == "" || loc.Node == nil {
			continue
		}
		p := &ProxyInfo{Upstream: loc.ProxyPass}
		for _, ch := range loc.Node.Children {
			if ch.Kind != KindDirective || len(ch.Args) == 0 {
				continue
			}
			if ch.Name == "proxy_set_header" && ch.Args[0] == "Host" && len(ch.Args) > 1 {
				p.HostHeader = ch.Args[1]
			}
			if ch.Name == "proxy_set_header" && ch.Args[0] == "Upgrade" {
				p.Websocket = true
			}
		}
		return p
	}
	return nil
}

// modeledDirectives：吸收进结构化字段的指令（其余入 rawBlocks，A-SM4——
// 建模清单=server_name/listen/root/ssl_certificate(_key)/access_log/error_log）。
var modeledDirectives = map[string]bool{
	"server_name": true, "listen": true, "root": true,
	"ssl_certificate": true, "ssl_certificate_key": true,
	"access_log": true, "error_log": true,
}

// ExtractServers 提取文件内全部 server 块（active + dormant，按出现顺序编号，
// A-SM3：序号含 dormant 保证 disable→enable 期间 siteId 稳定）。
func ExtractServers(t *Tree) []*ServerBlock {
	var out []*ServerBlock
	i := 0
	for i < len(t.Nodes) {
		n := t.Nodes[i]
		switch {
		case n.Kind == KindBlock && strings.EqualFold(n.Name, "server"):
			out = append(out, buildServer(len(out), n, false, t.Source))
			i++
		case n.Kind == KindComment:
			// 收集连续注释组（空白相隔且不含空行分段——空行隔开的普通注释
			// 不并入 server 组，否则整体剥离判定被无关前缀破坏）。
			j, end := i, n.End
			for j+1 < len(t.Nodes) && t.Nodes[j+1].Kind == KindComment &&
				mergeableGap(t.Source[end:t.Nodes[j+1].Start]) {
				j++
				end = t.Nodes[j].End
			}
			if blk := tryDormant(t.Source, t.Nodes[i].Start, end); blk != nil {
				blk.Index = len(out)
				out = append(out, blk)
			}
			i = j + 1
		default:
			i++
		}
	}
	return out
}

// tryDormant 判断注释组剥离后是否为单个合法 server 块（D-16 停用投影）。
func tryDormant(source string, start, end int) *ServerBlock {
	stripped := StripCommentPrefix(source[start:end])
	if !strings.Contains(stripped, "{") {
		return nil
	}
	sub := Parse(stripped)
	var server *Node
	for _, n := range sub.Nodes {
		if n.Kind == KindComment {
			continue
		}
		if n.Kind == KindBlock && strings.EqualFold(n.Name, "server") && server == nil {
			server = n
			continue
		}
		return nil // 组内含 server 之外的内容：非整体注释的 server 块
	}
	if server == nil {
		return nil
	}
	blk := buildServer(0, server, true, stripped)
	blk.CommentStart, blk.CommentEnd = start, end
	return blk
}

// StripCommentPrefix 逐行剥离行首空白 + '#' + 至多一个空白（D-16 注释形态；
// 导出供 service 层 dormant 块编辑时重建可解析文本）。
func StripCommentPrefix(s string) string {
	lines := strings.SplitAfter(s, "\n")
	var sb strings.Builder
	for _, ln := range lines {
		k := 0
		for k < len(ln) && (ln[k] == ' ' || ln[k] == '\t' || ln[k] == '\r') {
			k++
		}
		if k < len(ln) && ln[k] == '#' {
			k++
			if k < len(ln) && ln[k] == ' ' {
				k++
			}
		}
		sb.WriteString(ln[k:])
	}
	return sb.String()
}

// CommentOut 对文本逐行加 "# " 前缀（缩进移至前缀后："#     listen 80;"——
// StripCommentPrefix 剥离 '#'+至多一空格即得原行，保证 disable→enable
// byte 级还原；纯换行段原样。D-16 停用实现）。
func CommentOut(text string) string {
	lines := strings.SplitAfter(text, "\n")
	var sb strings.Builder
	for _, ln := range lines {
		if strings.TrimRight(ln, "\r\n") == "" {
			sb.WriteString(ln)
			continue
		}
		sb.WriteString("# ")
		sb.WriteString(strings.TrimRight(ln, "\r\n"))
		sb.WriteString(trailingNewline(ln))
	}
	return sb.String()
}

func trailingNewline(ln string) string {
	if strings.HasSuffix(ln, "\r\n") {
		return "\r\n"
	}
	if strings.HasSuffix(ln, "\n") {
		return "\n"
	}
	return ""
}

// mergeableGap 报告相邻注释节点间的间隙是否可视为同组（空白或单换行，
// 不含空行分段）。
func mergeableGap(s string) bool {
	return strings.TrimSpace(s) == "" && strings.Count(s, "\n") <= 1
}

// buildServer 从 server 块节点构建投影；source 为该节点所属解析树的原文本
// （dormant 时为剥离文本），RawBlocks 原文取自区间切片。
func buildServer(index int, n *Node, dormant bool, source string) *ServerBlock {
	s := &ServerBlock{Index: index, Dormant: dormant, Node: n}
	text := func(nd *Node) string {
		if nd.End <= len(source) && nd.Start <= nd.End {
			return source[nd.Start:nd.End]
		}
		return nd.Text
	}
	for _, ch := range n.Children {
		switch ch.Kind {
		case KindDirective:
			if !modeledDirectives[ch.Name] {
				s.RawBlocks = append(s.RawBlocks, RawBlock{Line: ch.Line, Text: text(ch)})
				continue
			}
			switch ch.Name {
			case "server_name":
				s.ServerNames = append(s.ServerNames, ch.Args...)
			case "listen":
				if len(ch.Args) > 0 {
					s.Listen = append(s.Listen, strings.Join(ch.Args, " "))
				}
			case "root":
				if len(ch.Args) > 0 {
					s.Root = ch.Args[0]
				}
			case "ssl_certificate":
				if len(ch.Args) > 0 {
					s.SSLCert = ch.Args[0]
				}
			case "ssl_certificate_key":
				if len(ch.Args) > 0 {
					s.SSLCertKey = ch.Args[0]
				}
			case "access_log":
				if len(ch.Args) > 0 {
					s.AccessLog = ch.Args[0]
				}
			case "error_log":
				if len(ch.Args) > 0 {
					s.ErrorLog = ch.Args[0]
				}
			}
		case KindBlock:
			if strings.EqualFold(ch.Name, "location") {
				loc := LocationInfo{Path: strings.Join(ch.Args, " "), Node: ch}
				for _, lc := range ch.Children {
					if lc.Kind == KindDirective && len(lc.Args) > 0 {
						switch lc.Name {
						case "proxy_pass":
							if loc.ProxyPass == "" {
								loc.ProxyPass = lc.Args[0]
							}
						case "fastcgi_pass":
							if loc.FastCGIPass == "" {
								loc.FastCGIPass = lc.Args[0]
							}
						}
					}
				}
				s.Locations = append(s.Locations, loc)
			} else {
				s.RawBlocks = append(s.RawBlocks, RawBlock{Line: ch.Line, Text: text(ch)})
			}
		case KindRaw:
			s.RawBlocks = append(s.RawBlocks, RawBlock{Line: ch.Line, Text: text(ch)})
		}
	}
	return s
}
