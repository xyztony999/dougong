package sitemgmt

import (
	"context"
	"fmt"
	"strings"

	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// SiteEditService（§4.2.1）：结构化字段/反代参数语义写回编排——span 定点
// 替换（A-SM2，I-09 Raw 保全/I-10 隔离由构造保证）→ WritePipeline（D-04）
// → reload（D-05）。dormant 块编辑在剥离文本子树上进行，写回时整体重新
// 注释（enabled 投影不因编辑改变，D-16）。

// editTarget 是一次结构化编辑的载体（active=文件树；dormant=剥离文本子树）。
type editTarget struct {
	file            string
	tree            *nginxx.Tree
	block           *nginxx.Node
	origFileContent string
	dormant         bool
	commentStart    int
	commentEnd      int
}

// openEdit 解析站点并构建编辑载体（I-08 前置链内含）。
func (s *Service) openEdit(ctx context.Context, siteID string) (*editTarget, *editEntry, error) {
	entry, err := s.resolveSite(ctx, siteID)
	if err != nil {
		return nil, nil, err
	}
	orig, _, rerr := s.gw.ReadFile(entry.File)
	if rerr != nil {
		return nil, nil, fmt.Errorf("site-mgmt: read source %s: %w", entry.File, rerr)
	}
	if !entry.Block.Dormant {
		return &editTarget{
			file: entry.File, tree: entry.Tree, block: entry.Block.Node,
			origFileContent: orig,
		}, &editEntry{entry: entry}, nil
	}
	stripped := nginxx.StripCommentPrefix(orig[entry.Block.CommentStart:entry.Block.CommentEnd])
	sub := nginxx.Parse(stripped)
	blk := findServerNode(sub)
	if blk == nil { // 防御：tryDormant 已保证存在
		return nil, nil, fmt.Errorf("site-mgmt: dormant server block lost after reparse: %s", siteID)
	}
	return &editTarget{
		file: entry.File, tree: sub, block: blk,
		origFileContent: orig, dormant: true,
		commentStart: entry.Block.CommentStart, commentEnd: entry.Block.CommentEnd,
	}, &editEntry{entry: entry}, nil
}

// editEntry 携带原始扫描条目（响应组装用）。
type editEntry struct{ entry *SiteEntry }

// serialize 产出写回全文（dormant：子树序列化后整体重新注释回填注释区间）。
func (t *editTarget) serialize() (string, error) {
	if !t.dormant {
		return t.tree.Serialize()
	}
	stripped, err := t.tree.Serialize()
	if err != nil {
		return "", err
	}
	return t.origFileContent[:t.commentStart] +
		nginxx.CommentOut(stripped) +
		t.origFileContent[t.commentEnd:], nil
}

// findServerNode 定位树内首个顶层 server 块（dormant 子树入口）。
func findServerNode(t *nginxx.Tree) *nginxx.Node {
	for _, n := range t.Nodes {
		if n.Kind == nginxx.KindBlock && strings.EqualFold(n.Name, "server") {
			return n
		}
	}
	return nil
}

// ---- span 指令编辑原语（A-SM2）----

// renderDirective 渲染一条简单指令文本（不含缩进换行）。
func renderDirective(name string, args ...string) string {
	return name + " " + strings.Join(args, " ") + ";"
}

// indentOfBlock 取块内子指令缩进基准（首子行首缩进；无子取块缩进+4 空格）。
func indentOfBlock(t *nginxx.Tree, block *nginxx.Node) string {
	for _, ch := range block.Children {
		if ch.Kind == nginxx.KindDirective || ch.Kind == nginxx.KindBlock {
			return t.IndentOf(ch)
		}
	}
	return t.IndentOf(block) + "    "
}

// setDirectives 对块内同名指令做「首条原位替换为 lines、其余删除、无则块体
// 首部插入」的 span 编辑（lines 为已渲染指令文本，不含缩进）。
func setDirectives(t *nginxx.Tree, block *nginxx.Node, name string, lines []string) error {
	if len(lines) == 0 { // 空集=删除全部同名指令（undeploy 恢复空原值用）
		return removeDirectives(t, block, name)
	}
	indent := indentOfBlock(t, block)
	var rendered []string
	for _, ln := range lines {
		rendered = append(rendered, indent+ln)
	}
	var first *nginxx.Node
	for _, ch := range block.Children {
		if ch.Kind == nginxx.KindDirective && ch.Name == name && first == nil {
			first = ch
			break
		}
	}
	if first != nil {
		// 首条原位替换：首行不带缩进（原行缩进在 span 之前保留），后续行带缩进。
		repl := make([]string, len(lines))
		for i, ln := range lines {
			repl[i] = ln
			if i > 0 {
				repl[i] = indent + ln
			}
		}
		if err := t.Apply(first.Start, first.End, strings.Join(repl, "\n")); err != nil {
			return err
		}
		for _, ch := range block.Children {
			if ch != first && ch.Kind == nginxx.KindDirective && ch.Name == name {
				if err := t.Apply(ch.Start, t.LineEnd(ch), ""); err != nil {
					return err
				}
			}
		}
		return nil
	}
	pos := insertPos(t, block)
	// 自包含整行插入（每行以换行收尾）：同锚点多次插入按登记顺序稳定叠加为
	// 独立行，不与后续插入的缩进互相渗透。
	return t.Apply(pos, pos, strings.Join(rendered, "\n")+"\n")
}

// removeDirectives 删除块内全部同名指令（行内其余内容保留——空行残迹无害）。
func removeDirectives(t *nginxx.Tree, block *nginxx.Node, name string) error {
	for _, ch := range block.Children {
		if ch.Kind == nginxx.KindDirective && ch.Name == name {
			if err := t.Apply(ch.Start, t.LineEnd(ch), ""); err != nil {
				return err
			}
		}
	}
	return nil
}

// insertPos 块体首部插入点（首子行首；无子=块开始行的下一行行首——插入的
// 整行文本不与 '{' 行或既有行拼接）。
func insertPos(t *nginxx.Tree, block *nginxx.Node) int {
	for _, ch := range block.Children {
		if ch.Kind == nginxx.KindDirective || ch.Kind == nginxx.KindBlock {
			return t.LineStart(ch)
		}
	}
	i := block.Start
	for i < block.End && t.Source[i] != '{' {
		i++
	}
	for i < block.End && t.Source[i] != '\n' {
		i++
	}
	if i < block.End { // 跳过换行，落在下一行行首
		i++
	}
	return i
}

// findNamedDirective 取块内首条（指定谓词的）指令节点；无则 nil。
func findDirective(block *nginxx.Node, name string) *nginxx.Node {
	for _, ch := range block.Children {
		if ch.Kind == nginxx.KindDirective && ch.Name == name {
			return ch
		}
	}
	return nil
}

// findSetHeader 取块内首条 proxy_set_header 且首参为 header 的节点。
func findSetHeader(block *nginxx.Node, header string) *nginxx.Node {
	for _, ch := range block.Children {
		if ch.Kind == nginxx.KindDirective && ch.Name == "proxy_set_header" &&
			len(ch.Args) > 0 && ch.Args[0] == header {
			return ch
		}
	}
	return nil
}

// setSSLRefs 写回 ssl_certificate/_key 引用（certRef/keyRef 空串=移除指令）。
func setSSLRefs(t *nginxx.Tree, block *nginxx.Node, certRef, keyRef string) error {
	var certLines, keyLines []string
	if certRef != "" {
		certLines = []string{renderDirective("ssl_certificate", quoteIfNeeded(certRef))}
	}
	if keyRef != "" {
		keyLines = []string{renderDirective("ssl_certificate_key", quoteIfNeeded(keyRef))}
	}
	if err := setDirectives(t, block, "ssl_certificate", certLines); err != nil {
		return err
	}
	return setDirectives(t, block, "ssl_certificate_key", keyLines)
}

// quoteIfNeeded 含空白的参数加双引号（nginx 词法）。
func quoteIfNeeded(v string) string {
	if strings.ContainsAny(v, " \t") {
		return `"` + v + `"`
	}
	return v
}

// ---- POST /sites/site/update（§4.2.4）----

// UpdateSite 站点基本信息结构化编辑：校验（SITE_006/007 + 根目录 R-14）→
// 字段级语义写回（I-09 Raw 保全）→ WritePipeline → reload → 审计 site_updated。
func (s *Service) UpdateSite(ctx context.Context, actor *authsvc.Identity, req UpdateSiteRequest) (*UpdateSiteData, error) {
	if e := validateDomainList(req.ServerNames); e != nil {
		return nil, e
	}
	listenLines := make([]string, 0, len(req.Listen))
	for _, l := range req.Listen {
		if e := validateListenDecl(l); e != nil {
			return nil, e
		}
		// listen 声明整体原样写回（"443 ssl" 是多参数形态而非单参数值，不加引号）
		listenLines = append(listenLines, renderDirective("listen", l))
	}
	if err := s.requireNginx(); err != nil { // I-12 写前置
		return nil, err
	}
	tgt, _, err := s.openEdit(ctx, req.SiteID)
	if err != nil {
		return nil, err
	}
	fields := changedFields(tgt, req)
	var nameLines []string
	if len(req.ServerNames) > 0 {
		nameLines = []string{renderDirective("server_name", req.ServerNames...)}
	}
	if err := setDirectives(tgt.tree, tgt.block, "server_name", nameLines); err != nil {
		return nil, fmt.Errorf("site-mgmt: edit server_name: %w", err)
	}
	if err := setDirectives(tgt.tree, tgt.block, "listen", listenLines); err != nil {
		return nil, fmt.Errorf("site-mgmt: edit listen: %w", err)
	}
	if req.Root != "" {
		if err := setDirectives(tgt.tree, tgt.block, "root",
			[]string{renderDirective("root", quoteIfNeeded(req.Root))}); err != nil {
			return nil, fmt.Errorf("site-mgmt: edit root: %w", err)
		}
	}
	data, err := s.commitEdit(ctx, actor, AuditActionSiteUpdated, req.SiteID, tgt,
		map[string]any{"fields": fields})
	if err != nil {
		return nil, err
	}
	return &UpdateSiteData{
		SiteID: req.SiteID, ServerNames: req.ServerNames,
		Listen: req.Listen, Root: req.Root,
	}, data.err()
}

// changedFields 计算审计 detail.fields（变更字段名清单）。
func changedFields(t *editTarget, req UpdateSiteRequest) []string {
	var out []string
	if len(req.ServerNames) > 0 && !sameStrings(findDirectiveArgs(t.block, "server_name"), req.ServerNames) {
		out = append(out, "serverNames")
	}
	if len(req.Listen) > 0 && !sameStrings(findDirectiveArgs(t.block, "listen"), req.Listen) {
		out = append(out, "listen")
	}
	if req.Root != "" && findDirective(t.block, "root") != nil &&
		firstArg(t.block, "root") != req.Root {
		out = append(out, "root")
	}
	if len(out) == 0 {
		out = append(out, "serverNames", "listen", "root") // 防御：无差异仍记录字段集
	}
	return out
}

func findDirectiveArgs(block *nginxx.Node, name string) []string {
	if n := findDirective(block, name); n != nil {
		return n.Args
	}
	return nil
}

func firstArg(block *nginxx.Node, name string) string {
	if n := findDirective(block, name); n != nil && len(n.Args) > 0 {
		return n.Args[0]
	}
	return ""
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// commitEdit 统一收尾：序列化 → WritePipeline（R-06 四步）→ 审计（文件写入
// 成功即记 success，A-SM7——reload 失败不影响写入事实）→ reload（D-05，
// 失败作为返回错误上浮 SITE_004，配置保留）。
type commitResult struct{ reloadErr error }

func (c commitResult) err() error { return c.reloadErr }

func (s *Service) commitEdit(ctx context.Context, actor *authsvc.Identity,
	action, siteID string, tgt *editTarget, detail map[string]any) (commitResult, error) {
	content, err := tgt.serialize()
	if err != nil {
		return commitResult{}, fmt.Errorf("site-mgmt: serialize %s: %w", tgt.file, err)
	}
	if err := s.applyPipeline(ctx, tgt.file, content, false); err != nil {
		return commitResult{}, err
	}
	s.auditAction(ctx, actor, action, ObjectTypeSite, siteID, detail, true)
	return commitResult{reloadErr: s.reloadAfterWrite(ctx)}, nil
}

// ---- POST /sites/proxy/update（§4.2.4，M3 验收主场景）----

// UpdateProxy 主反代段参数语义写回：站点须存在反代段（无 proxy → SITE_002
// 语义拒绝）；upstream 形态（SITE_008）；Host 头默认 $host、WS 透传默认开
// （模板默认值口径，A-SM 模板内置）。改写后 `nginx -t` 通过且其余站点零变化
// （R-16/D-04）。
func (s *Service) UpdateProxy(ctx context.Context, actor *authsvc.Identity, req UpdateProxyRequest) (*UpdateProxyData, error) {
	if e := validateUpstream(req.Upstream); e != nil {
		return nil, e
	}
	if err := s.requireNginx(); err != nil {
		return nil, err
	}
	tgt, _, err := s.openEdit(ctx, req.SiteID)
	if err != nil {
		return nil, err
	}
	loc := proxyLocation(tgt.block)
	if loc == nil {
		return nil, errcode.ErrSiteNotFound.WithData(ReasonData{
			Reason: "站点不存在反代段（proxy）",
		})
	}
	hostHeader := req.HostHeader
	if hostHeader == "" {
		hostHeader = defaultHostHeader
	}
	websock := true
	if req.Websocket != nil {
		websock = *req.Websocket
	}
	if err := applyProxyParams(tgt.tree, loc, req.Upstream, hostHeader, websock); err != nil {
		return nil, fmt.Errorf("site-mgmt: edit proxy params: %w", err)
	}
	if _, err := s.commitEdit(ctx, actor, AuditActionProxyUpdated, req.SiteID, tgt, nil); err != nil {
		return nil, err
	}
	return &UpdateProxyData{
		SiteID: req.SiteID,
		Proxy:  ProxyDTO{Upstream: req.Upstream, HostHeader: hostHeader, Websocket: websock},
	}, nil
}

// proxyLocation 定位主反代段（首个含 proxy_pass 的 location 块）。
func proxyLocation(block *nginxx.Node) *nginxx.Node {
	for _, ch := range block.Children {
		if ch.Kind != nginxx.KindBlock || !strings.EqualFold(ch.Name, "location") {
			continue
		}
		if findDirective(ch, "proxy_pass") != nil {
			return ch
		}
	}
	return nil
}

// applyProxyParams 反代段参数 span 写回。
func applyProxyParams(t *nginxx.Tree, loc *nginxx.Node, upstream, hostHeader string, websocket bool) error {
	if err := setDirectives(t, loc, "proxy_pass",
		[]string{renderDirective("proxy_pass", quoteIfNeeded(upstream))}); err != nil {
		return err
	}
	if err := setHeaderDirective(t, loc, "Host", hostHeader); err != nil {
		return err
	}
	if websocket {
		if err := setHeaderDirective(t, loc, "Upgrade", "$http_upgrade"); err != nil {
			return err
		}
		return setHeaderDirective(t, loc, "Connection", `"upgrade"`)
	}
	// 关闭 WS 透传：移除 Upgrade/Connection 头（其余 proxy_set_header 保全）。
	if n := findSetHeader(loc, "Upgrade"); n != nil {
		if err := t.Apply(n.Start, t.LineEnd(n), ""); err != nil {
			return err
		}
	}
	if n := findSetHeader(loc, "Connection"); n != nil {
		return t.Apply(n.Start, t.LineEnd(n), "")
	}
	return nil
}

// setHeaderDirective 设定（或插入）proxy_set_header <header> <value>。
func setHeaderDirective(t *nginxx.Tree, loc *nginxx.Node, header, value string) error {
	if n := findSetHeader(loc, header); n != nil {
		return t.Apply(n.Start, n.End, renderDirective("proxy_set_header", header, quoteIfNeeded(value)))
	}
	// 自包含整行插入（与 setDirectives 插入路径同语义：行首缩进+语句+换行）。
	indent := indentOfBlock(t, loc)
	pos := insertPos(t, loc)
	return t.Apply(pos, pos, indent+renderDirective("proxy_set_header", header, quoteIfNeeded(value))+"\n")
}

// ---- POST /sites/site/{enable,disable}（§4.2.4，D-16）----

// EnableSite 启用站点（注释还原；已 enabled 幂等成功，D-16/I-06 实况语义）。
func (s *Service) EnableSite(ctx context.Context, actor *authsvc.Identity, siteID string) (*ToggleData, error) {
	return s.toggleSite(ctx, actor, siteID, true)
}

// DisableSite 停用站点（server 块整体注释；停用≠删除 OUT-6；已 disabled
// 幂等成功）。
func (s *Service) DisableSite(ctx context.Context, actor *authsvc.Identity, siteID string) (*ToggleData, error) {
	return s.toggleSite(ctx, actor, siteID, false)
}

// toggleSite 启停编排：目标态已达成 → 幂等成功零写回；否则 span 注释/还原
// → WritePipeline → reload → 审计。
func (s *Service) toggleSite(ctx context.Context, actor *authsvc.Identity, siteID string, enable bool) (*ToggleData, error) {
	if err := s.requireNginx(); err != nil {
		return nil, err
	}
	entry, err := s.resolveSite(ctx, siteID)
	if err != nil {
		return nil, err
	}
	targetState := SiteDisabled
	action := AuditActionSiteDisabled
	if enable {
		targetState, action = SiteEnabled, AuditActionSiteEnabled
	}
	if entry.Enabled() == targetState { // 幂等（I-06 实况即真）
		return &ToggleData{SiteID: siteID, Enabled: targetState}, nil
	}
	orig, _, rerr := s.gw.ReadFile(entry.File)
	if rerr != nil {
		return nil, fmt.Errorf("site-mgmt: read %s: %w", entry.File, rerr)
	}
	var content string
	if entry.Block.Dormant { // enable：注释区间还原为剥离文本
		content = orig[:entry.Block.CommentStart] +
			nginxx.StripCommentPrefix(orig[entry.Block.CommentStart:entry.Block.CommentEnd]) +
			orig[entry.Block.CommentEnd:]
	} else { // disable：server 块区间整体注释
		content = orig[:entry.Block.Node.Start] +
			nginxx.CommentOut(orig[entry.Block.Node.Start:entry.Block.Node.End]) +
			orig[entry.Block.Node.End:]
	}
	if err := s.applyPipeline(ctx, entry.File, content, false); err != nil {
		return nil, err
	}
	if rerr := s.reloadAfterWrite(ctx); rerr != nil {
		return &ToggleData{SiteID: siteID, Enabled: targetState}, rerr
	}
	s.auditAction(ctx, actor, action, ObjectTypeSite, siteID, nil, true)
	return &ToggleData{SiteID: siteID, Enabled: targetState}, nil
}
