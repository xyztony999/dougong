// Package nginxx 是 nginx 配置解析/序列化与 nginx 进程通道适配器（api-site-mgmt.md
// §4.2.5 NginxGateway 契约的实现；nginxx 为 ADR-010 自研核心资产，R-08 全部
// 外部命令经 exec.CommandContext 参数数组执行，无 shell 无拼接）。
//
// round-trip 保真采用 span 替换模型（tasks.md A-SM2）：Parse 产出携带原文精确
// 字节区间 [Start,End) 的节点树；Serialize = 原文 + 定点区间替换——未编辑字节
// 恒不变，I-09（Raw 保全）/I-10（其余站点零影响）由构造保证，优于全量重序列化。
package nginxx

import (
	"fmt"
	"sort"
	"strings"
)

// NodeKind 是指令树节点类型。
type NodeKind int

const (
	// KindDirective 简单指令：name args ... ;
	KindDirective NodeKind = iota
	// KindBlock 块指令：name args ... { children }
	KindBlock
	// KindComment 注释行（# 至行尾，原文保留）。
	KindComment
	// KindRaw 无法结构化的残段（缺终结符/游离标点/坏引号——R-07 Raw 保全）。
	KindRaw
)

// Node 是指令树节点；Start/End 为原文字节区间 [Start,End)（指令含 ';'，块含
// 匹配 '}'，注释含 '#' 至行尾，不含行尾换行）。
type Node struct {
	Kind     NodeKind
	Name     string
	Args     []string
	Children []*Node
	Text     string // 注释原文 / Raw 残段原文
	Start    int
	End      int
	Line     int // Start 所在行（1 起）
}

// Tree 是一个配置文件的解析产物（原文 + 顶层节点 + 待应用编辑）。
type Tree struct {
	Source string
	Nodes  []*Node
	edits  []edit
}

type edit struct {
	start, end int
	text       string
}

// ---- 词法 ----

type tokKind int

const (
	tokWord tokKind = iota
	tokPunct
	tokComment
	tokBad // 坏引号等不可恢复词法残段
)

type token struct {
	kind  tokKind
	text  string // 原文
	val   string // 词值（引号解码后）
	start int
	end   int
	line  int
}

func lex(src string) []token {
	var toks []token
	i, line := 0, 1
	advance := func(n int) {
		for k := 0; k < n && i+k < len(src); k++ {
			if src[i+k] == '\n' {
				line++
			}
		}
		i += n
	}
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			advance(1)
		case c == '#':
			start, ln := i, line
			for i < len(src) && src[i] != '\n' {
				advance(1)
			}
			toks = append(toks, token{kind: tokComment, text: src[start:i], val: src[start:i], start: start, end: i, line: ln})
		case c == '{' || c == '}' || c == ';':
			start, ln := i, line
			advance(1)
			toks = append(toks, token{kind: tokPunct, text: string(c), val: string(c), start: start, end: i, line: ln})
		case c == '"' || c == '\'':
			start, ln := i, line
			quote := c
			advance(1)
			var sb strings.Builder
			closed := false
			for i < len(src) {
				ch := src[i]
				if ch == '\\' && quote == '"' && i+1 < len(src) && (src[i+1] == '"' || src[i+1] == '\\') {
					sb.WriteByte(src[i+1])
					advance(2)
					continue
				}
				if ch == quote {
					advance(1)
					closed = true
					break
				}
				if ch == '\n' { // 引号跨行：nginx 不允许，按残段处理
					break
				}
				sb.WriteByte(ch)
				advance(1)
			}
			kind := tokWord
			if !closed {
				kind = tokBad
			}
			toks = append(toks, token{kind: kind, text: src[start:i], val: sb.String(), start: start, end: i, line: ln})
		default:
			start, ln := i, line
			for i < len(src) {
				ch := src[i]
				if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == '#' ||
					ch == '{' || ch == '}' || ch == ';' || ch == '"' || ch == '\'' {
					break
				}
				advance(1)
			}
			toks = append(toks, token{kind: tokWord, text: src[start:i], val: src[start:i], start: start, end: i, line: ln})
		}
	}
	return toks
}

// ---- 语法（永不出错：不可结构化内容降级为 KindRaw 节点，R-07） ----

type parser struct {
	src  string
	toks []token
	pos  int
}

// Parse 将源文本解析为节点树（永不返回错误——语法残段以 KindRaw 保全原文；
// 语法问题检测用 ValidateSyntax）。
func Parse(src string) *Tree {
	p := &parser{src: src, toks: lex(src)}
	t := &Tree{Source: src}
	t.Nodes = p.parseNodes(0)
	return t
}

// parseNodes 解析节点序列，直至 token 耗尽或遇本层 '}'（由块构造者消费）。
func (p *parser) parseNodes(depth int) []*Node {
	var nodes []*Node
	for p.pos < len(p.toks) {
		tk := p.toks[p.pos]
		switch {
		case tk.kind == tokComment:
			nodes = append(nodes, &Node{Kind: KindComment, Text: tk.text, Start: tk.start, End: tk.end, Line: tk.line})
			p.pos++
		case tk.kind == tokPunct && tk.val == "}" && depth > 0:
			return nodes // '}' 留给块构造者
		case tk.kind == tokPunct || tk.kind == tokBad:
			// 游离 ';'/'}'/坏引号：残段
			nodes = append(nodes, &Node{Kind: KindRaw, Text: p.src[tk.start:tk.end], Start: tk.start, End: tk.end, Line: tk.line})
			p.pos++
		default: // tokWord：指令或块
			nodes = append(nodes, p.parseDirective()...)
		}
	}
	return nodes
}

// parseDirective 解析一条指令（简单指令或块）；语法残段降级 KindRaw。
func (p *parser) parseDirective() []*Node {
	head := p.toks[p.pos]
	name, start, line := head.val, head.start, head.line
	p.pos++
	var nodes []*Node
	var args []string
	for p.pos < len(p.toks) {
		tk := p.toks[p.pos]
		switch {
		case tk.kind == tokWord || tk.kind == tokBad:
			args = append(args, tk.val)
			p.pos++
		case tk.kind == tokPunct && tk.val == ";":
			p.pos++
			return append(nodes, &Node{Kind: KindDirective, Name: name, Args: args, Start: start, End: tk.end, Line: line})
		case tk.kind == tokPunct && tk.val == "{":
			p.pos++
			children := p.parseNodes(depth1)
			end := lastChildEnd(children)
			if p.pos < len(p.toks) && p.toks[p.pos].kind == tokPunct && p.toks[p.pos].val == "}" {
				end = p.toks[p.pos].end
				p.pos++
			} else {
				// 未闭合块：尽力终点（EOF）
				end = len(p.src)
			}
			return append(nodes, &Node{Kind: KindBlock, Name: name, Args: args, Children: children, Start: start, End: end, Line: line})
		case tk.kind == tokPunct && tk.val == "}":
			// 缺 ';' 的指令：残段至 '}' 前（'}' 留给上层）
			return append(nodes, &Node{Kind: KindRaw, Text: p.src[start:tk.start], Start: start, End: tk.start, Line: line})
		default: // 注释出现在参数间：独立注释节点先行输出，指令继续吸收参数
			nodes = append(nodes, &Node{Kind: KindComment, Text: tk.text, Start: tk.start, End: tk.end, Line: tk.line})
			p.pos++
		}
	}
	// EOF 挂起：残段
	return append(nodes, &Node{Kind: KindRaw, Text: p.src[start:], Start: start, End: len(p.src), Line: line})
}

const depth1 = 1

func lastChildEnd(children []*Node) int {
	if len(children) == 0 {
		return 0
	}
	return children[len(children)-1].End
}

// ---- 编辑与序列化（span 替换模型，A-SM2） ----

// Apply 记录一处区间替换（[start,end) → text）；区间重叠返回错误。
func (t *Tree) Apply(start, end int, text string) error {
	for _, e := range t.edits {
		if start < e.end && e.start < end {
			return fmt.Errorf("nginxx: overlapping edits [%d,%d) vs [%d,%d)", start, end, e.start, e.end)
		}
	}
	t.edits = append(t.edits, edit{start: start, end: end, text: text})
	return nil
}

// Serialize 应用全部编辑产出新文本（未编辑字节原样保留——round-trip 保真；
// 同起点零宽插入按登记顺序稳定输出——多次插入组合为确定性行序列）。
func (t *Tree) Serialize() (string, error) {
	es := append([]edit(nil), t.edits...)
	sort.SliceStable(es, func(i, j int) bool { return es[i].start < es[j].start })
	var sb strings.Builder
	prev := 0
	for _, e := range es {
		if e.start < prev {
			return "", fmt.Errorf("nginxx: serialize overlap at %d", e.start)
		}
		if e.start > len(t.Source) || e.end > len(t.Source) || e.start > e.end {
			return "", fmt.Errorf("nginxx: serialize out of range [%d,%d)", e.start, e.end)
		}
		sb.WriteString(t.Source[prev:e.start])
		sb.WriteString(e.text)
		prev = e.end
	}
	sb.WriteString(t.Source[prev:])
	return sb.String(), nil
}

// LineStart 返回节点所在行的行首偏移。
func (t *Tree) LineStart(n *Node) int {
	i := n.Start
	for i > 0 && t.Source[i-1] != '\n' {
		i--
	}
	return i
}

// IndentOf 返回节点行首到节点起点的缩进串（保留原缩进风格）。
func (t *Tree) IndentOf(n *Node) string {
	ls := t.LineStart(n)
	return t.Source[ls:n.Start]
}

// LineEnd 返回节点所在行的行尾偏移（不含换行符）。
func (t *Tree) LineEnd(n *Node) int {
	i := n.End
	for i < len(t.Source) && t.Source[i] != '\n' {
		i++
	}
	return i
}

// TextOf 返回节点原文区间文本。
func (t *Tree) TextOf(n *Node) string { return t.Source[n.Start:n.End] }

// EndOfBlockBody 返回块节点体的内容尾偏移（闭合 '}' 前，用于尾部插入）。
func (t *Tree) EndOfBlockBody(n *Node) int {
	i := n.End
	if i > 0 && t.Source[i-1] == '}' {
		return i - 1
	}
	return i
}

// ValidateSyntax 做落盘前语法自检（A-SM8：括号配平/语句终结/引号闭合的
// 近似校验——真实 `nginx -t` 仍由 WritePipeline 执行）；返回人读问题列表，
// 空切片=通过。
func ValidateSyntax(src string) []string {
	var issues []string
	depth := 0
	for _, tk := range lex(src) {
		if tk.kind == tokBad {
			issues = append(issues, fmt.Sprintf("line %d: unterminated quoted string", tk.line))
		}
		if tk.kind == tokPunct {
			switch tk.val {
			case "{":
				depth++
			case "}":
				depth--
				if depth < 0 {
					issues = append(issues, fmt.Sprintf("line %d: unexpected '}'", tk.line))
					depth = 0
				}
			}
		}
	}
	if depth != 0 {
		issues = append(issues, fmt.Sprintf("unbalanced braces: %d unclosed", depth))
	}
	tree := Parse(src)
	var walk func(nodes []*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			if n.Kind == KindRaw {
				issues = append(issues, fmt.Sprintf("line %d: unrecognized segment: %.60q", n.Line, n.Text))
			}
			if n.Kind == KindBlock {
				walk(n.Children)
			}
		}
	}
	walk(tree.Nodes)
	return issues
}
