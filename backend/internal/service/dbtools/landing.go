package dbtools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/errcode"
	panelsvc "github.com/dougong/dougong/internal/service/panel"
)

// 编译期断言：Service 实现 panel-core ConfigChangeHook 契约（D-20⑤，
// main 装配注册）。
var _ panelsvc.ConfigChangeHook = (*Service)(nil)

// 本文件是 LandingService 落地自动供给机制（v1.3，api-db-tools D-19/D-20/
// D-23，d0-clarification-dbtools-auto-landing-2026-11-19.md Q1 拍板）：
// selectLanding 0 匹配自动建块（dougong-tools.conf listen 18090）+幂等复用
// +端口探针+域名联动（钩子主通道+惰性兜底）+卸载清块（I-C04）。安全边界：
// 自动块公网可达但每请求 auth_request gating（D-11 不变），工具本体仍仅
// localhost；写入全经 site-mgmt ConfigApply/ConfigRemove R-06 管线。

// autoCreateLanding 0 匹配自动落地（D-19）：已存在自动块→幂等复用（两工具
// 共用一块，复用先于端口探针——nginx 已占用 18090 为自身常态）；不存在→
// 端口探针（占用→DBT_012 reason 不静默换端口）→渲染块模板（server_name
// 按当前 BindDomain）→ConfigApply(allowNew=true)。探针通过不保证 nginx
// 绑定成功，nginx -t/reload 失败仍沿 R-06 回滚+DBT_012 reason 兜底。
func (s *Service) autoCreateLanding(ctx context.Context, pa PanelAccess) (siteRef, error) {
	file, err := s.landingConfPath()
	if err != nil {
		return siteRef{}, err
	}
	if content, _, rerr := s.nginx.ReadFile(file); rerr == nil {
		// 文件存在：块标记判定（文件名+listen 18090，D-19④）。非自动块
		// 保守拒绝——不覆盖用户内容（fail-closed）。
		if !isLandingBlock(content) {
			return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
				"reason": ToolLandingConfName + " 已存在但非面板自动落地块，请经站点管理处理后重试"})
		}
		// 惰性校正（D-20④）：server_name 跟随当前 BindDomain；安装链刷新
		// 失败=任务 failed 如实（块域名不一致将致工具不可达）。
		if _, rerr := s.refreshLandingServerName(ctx, file, content, pa.BindDomain); rerr != nil {
			return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
				"reason": "域名联动刷新失败: " + rerr.Error()})
		}
		return landingRef(file, pa.BindDomain), nil
	} else if !errors.Is(rerr, os.ErrNotExist) {
		// 注意用 errors.Is 而非 os.IsNotExist：nginxx.ReadFile 以 %w 包装错误，
		// os.IsNotExist 不拆包装，会把「文件不存在」误判成读取失败（新服务器首装必踩）。
		return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "读取自动落地块失败: " + rerr.Error()})
	}
	// 端口占用探针（D-19③：即探即放，参数化非常量拼接）。
	ok, err := s.deploy.ProbePortAvailable(ToolLandingPort)
	if err != nil {
		return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "工具端口探针失败: " + err.Error()})
	}
	if !ok {
		return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": fmt.Sprintf("工具端口 %d 被占用，请释放后重试", ToolLandingPort)})
	}
	if err := s.pipeline.ConfigApply(ctx, file, renderLandingConf(pa.BindDomain), true); err != nil {
		return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "自动落地块写入失败: " + err.Error()})
	}
	return landingRef(file, pa.BindDomain), nil
}

// landingConfPath 自动落地块文件路径：已存在文件（任一惯例目录）→其实际
// 路径（幂等复用寻址）；否则 nginxx FirstWritableDir（PriorityDirs 首个实存
// 可写目录）+常量文件名。全部目录不可用→DBT_012 reason（D-19①）。
func (s *Service) landingConfPath() (string, error) {
	files, err := s.nginx.ScanConfDirs()
	if err != nil {
		return "", errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "扫描 nginx 配置目录失败: " + err.Error()})
	}
	for _, f := range files {
		if f.Name == ToolLandingConfName {
			return f.Path, nil
		}
	}
	dir, err := s.nginx.FirstWritableDir()
	if err != nil {
		return "", errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "nginx 配置目录不可用: " + err.Error()})
	}
	return filepath.Join(dir, ToolLandingConfName), nil
}

// landingRef 自动块落点引用（D-19：port=ToolLandingPort/autoCreated=true；
// scheme=http——HTTPS 工具落地 OUT；ServerName 未绑=""→URL 回退请求 Host）。
func landingRef(file, bindDomain string) siteRef {
	ref := siteRef{File: file, Index: 0, Scheme: "http", Port: ToolLandingPort, AutoCreated: true}
	if bindDomain != "" {
		ref.ServerName = bindDomain
	}
	return ref
}

// findAutoLanding 定位既有自动落地块（文件名+块标记判定；钩子/惰性校正
// 共用）。不存在/扫描失败→ok=false（钩子无块可刷=正常态）。
func (s *Service) findAutoLanding() (file, content string, ok bool) {
	files, err := s.nginx.ScanConfDirs()
	if err != nil {
		return "", "", false
	}
	for _, f := range files {
		if f.Name == ToolLandingConfName && isLandingBlock(f.Content) {
			return f.Path, f.Content, true
		}
	}
	return "", "", false
}

// isLandingBlock 自动块判定（D-19④：landing 标记+listen 18090；dormant
// 块不参与）。
func isLandingBlock(content string) bool {
	if !hasMarker(content, markerKeyLanding) {
		return false
	}
	for _, blk := range nginxx.ExtractServers(nginxx.Parse(content)) {
		if blk.Dormant {
			continue
		}
		for _, l := range blk.Listen {
			if strings.Contains(l, strconv.Itoa(ToolLandingPort)) {
				return true
			}
		}
	}
	return false
}

// renderLandingConf 自动块骨架模板（D-19/D-20，4.4.4）：listen 18090
// （nginx 缺省绑 0.0.0.0=ToolLandingBind，IP 直连可达）+server_name 跟随
// 绑定状态+已绑时块内首行 if 非匹配 Host 404（镜像 HostGuard）。工具
// location 由 injectToolLocations 后续注入（两工具共用 gate 去重）。
func renderLandingConf(bindDomain string) string {
	var b strings.Builder
	b.WriteString(markerBeginPrefix + markerKeyLanding + "\n")
	b.WriteString("# dougong panel auto-created tool landing block (db-tools D-19).\n")
	b.WriteString("# Panel-managed: removed when both tools are uninstalled. Do not edit manually.\n")
	b.WriteString("server {\n")
	if bindDomain != "" {
		b.WriteString("    " + landingHostIf(bindDomain) + "\n")
	}
	fmt.Fprintf(&b, "    listen %d;\n", ToolLandingPort)
	b.WriteString("    " + landingServerName(bindDomain) + "\n")
	b.WriteString("}\n")
	b.WriteString(markerEndPrefix + markerKeyLanding + "\n")
	return b.String()
}

// landingServerName server_name 指令（D-20：未绑 _=IP/任意 Host 可达；
// 已绑=BindDomain 全等）。
func landingServerName(bindDomain string) string {
	if bindDomain == "" {
		return "server_name _;"
	}
	return "server_name " + bindDomain + ";"
}

// landingHostIf 非匹配 Host 404 行（D-20：大小写不敏感全等镜像 HostGuard
// stripPort+EqualFold 语义——$host 不含端口；域名经 RFC 1123 上游校验，
// 拼接前转义正则元字符防注入）。
func landingHostIf(bindDomain string) string {
	return `if ($host !~* "^` + regexp.QuoteMeta(bindDomain) + `$") { return 404; }`
}

// refreshLandingServerName 域名联动刷新（D-20）：重渲染自动块 server_name
// （含 if 404 行）经 ConfigApply；一致零写入（幂等）。仅改 server_name/if
// 两处 span——工具 location 注入字节不动（R-06 其余字节恒不变）。
func (s *Service) refreshLandingServerName(ctx context.Context, file, content, bindDomain string) (bool, error) {
	tree := nginxx.Parse(content)
	var blk *nginxx.ServerBlock
	for _, b := range nginxx.ExtractServers(tree) {
		if b.Dormant {
			continue
		}
		for _, l := range b.Listen {
			if strings.Contains(l, strconv.Itoa(ToolLandingPort)) {
				blk = b
				break
			}
		}
		if blk != nil {
			break
		}
	}
	if blk == nil {
		return false, fmt.Errorf("dbtools: auto landing block missing listen %d (%s)", ToolLandingPort, file)
	}
	desired := "_"
	if bindDomain != "" {
		desired = bindDomain
	}
	var snNode, ifNode *nginxx.Node
	for _, ch := range blk.Node.Children {
		switch {
		case ch.Kind == nginxx.KindDirective && ch.Name == "server_name":
			snNode = ch
		case ch.Kind == nginxx.KindBlock && ch.Name == "if":
			for _, a := range ch.Args {
				// nginx 词法以空白/引号/花括号分词——"($host" 为单一词元
				// （括号非分隔符），按包含判定。
				if strings.Contains(a, "$host") {
					ifNode = ch
					break
				}
			}
		}
	}
	if snNode == nil {
		return false, fmt.Errorf("dbtools: auto landing block missing server_name (%s)", file)
	}
	ifText := ""
	if bindDomain != "" {
		ifText = landingHostIf(bindDomain)
	}
	// 幂等判定：server_name 全等（大小写不敏感）且 if 行状态一致→零写入。
	nameOK := len(blk.ServerNames) == 1 && strings.EqualFold(blk.ServerNames[0], desired)
	ifOK := (bindDomain == "" && ifNode == nil) || (ifNode != nil && tree.TextOf(ifNode) == ifText)
	if nameOK && ifOK {
		return false, nil
	}
	indent := tree.IndentOf(blk.Node)
	if err := tree.Apply(snNode.Start, snNode.End, landingServerName(bindDomain)); err != nil {
		return false, fmt.Errorf("dbtools: landing server_name edit: %w", err)
	}
	switch {
	case bindDomain == "" && ifNode != nil:
		// 解绑：移除 if 行（含行尾换行）。
		start, end := tree.LineStart(ifNode), tree.LineEnd(ifNode)
		if end < len(tree.Source) && tree.Source[end] == '\n' {
			end++
		}
		if err := tree.Apply(start, end, ""); err != nil {
			return false, fmt.Errorf("dbtools: landing if removal: %w", err)
		}
	case bindDomain != "" && ifNode != nil:
		if err := tree.Apply(ifNode.Start, ifNode.End, ifText); err != nil {
			return false, fmt.Errorf("dbtools: landing if edit: %w", err)
		}
	case bindDomain != "":
		// 绑定：块内首行插入 if（开 '{' 之后）。
		brace := strings.IndexByte(tree.Source[blk.Node.Start:], '{')
		if brace < 0 {
			return false, fmt.Errorf("dbtools: auto landing block malformed (%s)", file)
		}
		at := blk.Node.Start + brace + 1
		if err := tree.Apply(at, at, "\n"+indent+"    "+ifText); err != nil {
			return false, fmt.Errorf("dbtools: landing if insert: %w", err)
		}
	}
	newContent, err := tree.Serialize()
	if err != nil {
		return false, fmt.Errorf("dbtools: landing refresh serialize: %w", err)
	}
	if err := s.pipeline.ConfigApply(ctx, file, newContent, false); err != nil {
		return false, fmt.Errorf("dbtools: landing refresh apply: %w", err)
	}
	return true, nil
}

// OnBindDomainChanged panel-core 配置变更钩子实现（v1.3，D-20③⑤，
// ConfigChangeHook 消费契约，main 装配注册）：bindDomain 变更（""=解绑）→
// 自动块存在则重渲染 server_name（含 if 404 行）经 ConfigApply 刷新；一致
// 零写入。刷新失败仅记日志——钩子失败不阻断配置变更（panel_config 权威已
// 成功），install/open 惰性校正兜底自愈。
func (s *Service) OnBindDomainChanged(ctx context.Context, bindDomain string) {
	file, content, ok := s.findAutoLanding()
	if !ok {
		return // 无自动块=正常态（既有面板域落地块形态不受本决策影响）
	}
	if _, err := s.refreshLandingServerName(ctx, file, content, bindDomain); err != nil {
		slog.WarnContext(ctx, "dbtools: bind-domain hook landing refresh failed (lazy self-heal pending)",
			"file", file, "err", err)
	}
}

// lazyRefreshLanding 惰性校正（D-20④：open/reopen 兜底——自愈钩子失败/
// 面板重启窗口）：自动块形态按当前 BindDomain 比对刷新；失败不阻断打开
// （URL 仍按当前 BindDomain 生成，slog 降级口径）。
func (s *Service) lazyRefreshLanding(ctx context.Context, ref siteRef, pa PanelAccess) {
	if !ref.AutoCreated {
		return
	}
	content, _, err := s.nginx.ReadFile(ref.File)
	if err != nil {
		slog.WarnContext(ctx, "dbtools: lazy landing refresh read failed", "file", ref.File, "err", err)
		return
	}
	if _, err := s.refreshLandingServerName(ctx, ref.File, content, pa.BindDomain); err != nil {
		slog.WarnContext(ctx, "dbtools: lazy landing refresh failed (open not blocked)",
			"file", ref.File, "err", err)
	}
}

// appendLandingHint 自动块形态安装成功输出尾部放行提示（D-19⑦：仅提示
// 不自动放行——防火墙联动 OUT）。
func appendLandingHint(ring *outputRing, ref siteRef) {
	if !ref.AutoCreated {
		return
	}
	ring.append(fmt.Sprintf("[landing] 自动创建工具落地块 %s（listen %d）；请放行 TCP %d 端口（防火墙/云安全组）",
		ref.File, ToolLandingPort, ToolLandingPort))
}

// cleanupAutoLanding 卸载清块判定（v1.3，D-19⑥/I-C04）：工具 location 撤除
// 后——两工具均卸载（另一工具档案已不存在；本工具档案终态事务删除）且
// siteRef.autoCreated=true → ConfigRemove 移除整块（恢复原字节=文件不存在
// 态）。非自动块（autoCreated=false）绝不动（不误删用户站点）；块已不存在
// 视为清块成功（幂等）；失败=DBT_012 reason 档案不变（I-C01）。
func (s *Service) cleanupAutoLanding(ctx context.Context, toolType string, ref siteRef, ring *outputRing) error {
	if !ref.AutoCreated {
		return nil
	}
	archives, err := s.store.Tool().List(ctx)
	if err != nil {
		return fmt.Errorf("dbtools: landing cleanup list archives: %w", err)
	}
	for _, a := range archives {
		if a.ToolType != toolType {
			return nil // 另一工具仍在装——共享块保留
		}
	}
	if _, _, rerr := s.nginx.ReadFile(ref.File); rerr != nil {
		if errors.Is(rerr, os.ErrNotExist) {
			return nil // 块已不存在——清块成功（幂等）
		}
		return fmt.Errorf("dbtools: landing cleanup read %s: %w", ref.File, rerr)
	}
	if err := s.pipeline.ConfigRemove(ctx, ref.File); err != nil {
		return errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "清块失败: " + err.Error()})
	}
	ring.append("[landing] 两工具均卸载，自动落地块已移除（" + ref.File + "）")
	return nil
}
