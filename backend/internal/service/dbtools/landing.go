package dbtools

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

// landingTLS 落地块 HTTPS 监听材料（v1.4 D-24）：面板证书落盘产物+内容
// 指纹。active()=false 时不追加 TLS 监听行，工具 URL 沿 http 口径。
type landingTLS struct {
	certPath string
	keyPath  string
	fp       string // 证书内容指纹（sha256 前 16 hex）——轮换→conf 变更→reload
}

func (t landingTLS) active() bool { return t.fp != "" }

// ensureLandingTLS 落地块 TLS 材料供给（D-24）：HTTPS 面板且 panel_cert
// 可用→CertMaterial 物化 landing-cert.pem/landing-key.pem（0600，nginx
// master 以 root 读取）并返回材料；未配置证书（无行/空 PEM）→非 active
// nil err（HTTPS 模式签发中=常态，不虚设监听）；其余读失败→如实报错
// （安装链 fail loud，URL 与监听不虚设）。每次调用重写文件：证书轮换
// （ACME 续期/导入替换）后 conf 指纹注释随内容变更→ConfigApply 触发
// reload，nginx 装载新证书（否则磁盘新证书不生效）。
func (s *Service) ensureLandingTLS(ctx context.Context, pa PanelAccess) (landingTLS, error) {
	if !pa.HTTPSEnabled || s.panelCert == nil {
		return landingTLS{}, nil
	}
	certPEM, keyPEM, err := s.panelCert.CertMaterial(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return landingTLS{}, nil // 未配置证书=常态
		}
		return landingTLS{}, fmt.Errorf("读取面板证书失败: %w", err)
	}
	if certPEM == "" || keyPEM == "" {
		return landingTLS{}, nil
	}
	if err := s.deploy.MkdirAll(s.toolDir, 0o700); err != nil {
		return landingTLS{}, fmt.Errorf("工具目录创建失败: %w", err)
	}
	certPath := filepath.Join(s.toolDir, LandingCertFileName)
	keyPath := filepath.Join(s.toolDir, LandingKeyFileName)
	if err := s.deploy.WriteSecretFile(certPath, []byte(certPEM)); err != nil {
		return landingTLS{}, fmt.Errorf("落地证书落盘失败: %w", err)
	}
	if err := s.deploy.WriteSecretFile(keyPath, []byte(keyPEM)); err != nil {
		return landingTLS{}, fmt.Errorf("落地私钥落盘失败: %w", err)
	}
	sum := sha256.Sum256([]byte(certPEM))
	return landingTLS{certPath: certPath, keyPath: keyPath, fp: hex.EncodeToString(sum[:8])}, nil
}

// landingTLSPort 落地块 HTTPS 端口判定（v1.4 D-24，只读不落盘——ToolList
// 投影与刷新兜底判定用；与 renderLandingConf 的 TLS 追加条件同口径）：
// HTTPS 面板 + 面板证书可用→ToolLandingTLSPort；否则 0。
func (s *Service) landingTLSPort(ctx context.Context, pa PanelAccess) int {
	if !pa.HTTPSEnabled || s.panelCert == nil {
		return 0
	}
	certPEM, keyPEM, err := s.panelCert.CertMaterial(ctx)
	if err != nil || certPEM == "" || keyPEM == "" {
		return 0
	}
	return ToolLandingTLSPort
}

// autoCreateLanding 0 匹配自动落地（D-19）：已存在自动块→幂等复用（两工具
// 共用一块，复用先于端口探针——nginx 已占用 18090 为自身常态）；不存在→
// 端口探针（占用→DBT_012 reason 不静默换端口）→渲染块模板（server_name
// 按当前 BindDomain、TLS 监听按面板证书可用态 D-24）→ConfigApply(
// allowNew=true)。探针通过不保证 nginx 绑定成功，nginx -t/reload 失败仍沿
// R-06 回滚+DBT_012 reason 兜底。
func (s *Service) autoCreateLanding(ctx context.Context, pa PanelAccess) (siteRef, error) {
	file, err := s.landingConfPath()
	if err != nil {
		return siteRef{}, err
	}
	tls, terr := s.ensureLandingTLS(ctx, pa)
	if terr != nil {
		return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": terr.Error()})
	}
	if content, _, rerr := s.nginx.ReadFile(file); rerr == nil {
		// 文件存在：块标记判定（文件名+listen 18090，D-19④）。非自动块
		// 保守拒绝——不覆盖用户内容（fail-closed）。
		if !isLandingBlock(content) {
			return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
				"reason": ToolLandingConfName + " 已存在但非面板自动落地块，请经站点管理处理后重试"})
		}
		// 惰性校正（D-20④）：server_name 跟随当前 BindDomain、TLS 监听行
		// 跟随面板 HTTPS 态（旧版本所建 http-only 块升级后经此补齐 D-24）；
		// 安装链刷新失败=任务 failed 如实（块形态不一致将致工具不可达）。
		if _, rerr := s.refreshLandingBlock(ctx, file, content, pa.BindDomain, tls); rerr != nil {
			return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
				"reason": "落地块刷新失败: " + rerr.Error()})
		}
		return landingRef(file, pa.BindDomain), nil
	} else if !errors.Is(rerr, os.ErrNotExist) {
		// 注意用 errors.Is 而非 os.IsNotExist：nginxx.ReadFile 以 %w 包装错误，
		// os.IsNotExist 不拆包装，会把「文件不存在」误判成读取失败（新服务器首装必踩）。
		return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "读取自动落地块失败: " + rerr.Error()})
	}
	// 端口占用探针（D-19③：即探即放，参数化非常量拼接；TLS 态加探 2087）。
	ok, err := s.deploy.ProbePortAvailable(ToolLandingPort)
	if err != nil {
		return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "工具端口探针失败: " + err.Error()})
	}
	if !ok {
		return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": fmt.Sprintf("工具端口 %d 被占用，请释放后重试", ToolLandingPort)})
	}
	if tls.active() {
		okTLS, err := s.deploy.ProbePortAvailable(ToolLandingTLSPort)
		if err != nil {
			return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
				"reason": "工具端口探针失败: " + err.Error()})
		}
		if !okTLS {
			return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
				"reason": fmt.Sprintf("工具 HTTPS 端口 %d 被占用，请释放后重试", ToolLandingTLSPort)})
		}
	}
	if err := s.pipeline.ConfigApply(ctx, file, renderLandingConf(pa.BindDomain, tls), true); err != nil {
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

// renderLandingConf 自动块骨架模板（D-19/D-20，4.4.4；v1.4 D-24 +TLS）：
// listen 18090（nginx 缺省绑 0.0.0.0=ToolLandingBind，IP 直连可达）+
// server_name 跟随绑定状态+已绑时块内首行 if 非匹配 Host 404（镜像
// HostGuard）；tls.active() 时追加 listen 2087 ssl+证书行（面板证书落盘
// 引用，Cloudflare 代理/直连均经域名 HTTPS 访问）+块外指纹注释（证书轮换
// 强制 conf 变更→reload）。工具 location 由 injectToolLocations 后续注入
// （两工具共用 gate 去重）。
func renderLandingConf(bindDomain string, tls landingTLS) string {
	var b strings.Builder
	b.WriteString(markerBeginPrefix + markerKeyLanding + "\n")
	b.WriteString("# dougong panel auto-created tool landing block (db-tools D-19).\n")
	b.WriteString("# Panel-managed: removed when both tools are uninstalled. Do not edit manually.\n")
	if tls.active() {
		fmt.Fprintf(&b, "# Landing cert fingerprint: %s (cert rotation rewrites files; fingerprint change forces conf reload).\n", tls.fp)
	}
	b.WriteString("server {\n")
	if bindDomain != "" {
		b.WriteString("    " + landingHostIf(bindDomain) + "\n")
	}
	fmt.Fprintf(&b, "    listen %d;\n", ToolLandingPort)
	if tls.active() {
		fmt.Fprintf(&b, "    listen %d ssl;\n", ToolLandingTLSPort)
		fmt.Fprintf(&b, "    ssl_certificate %q;\n", tls.certPath)
		fmt.Fprintf(&b, "    ssl_certificate_key %q;\n", tls.keyPath)
	}
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

// landingFPComment 落地块指纹注释行（D-24：证书轮换→文件重写内容变、conf
// 路径不变——指纹行随证书内容变更使 conf 产生差异，ConfigApply 触发 reload
// 装载新证书；与 renderLandingConf 同源单一出处）。
func landingFPComment(fp string) string {
	return fmt.Sprintf("# Landing cert fingerprint: %s (cert rotation rewrites files; fingerprint change forces conf reload).", fp)
}

// landingTLSLines TLS 监听行组（D-24；不含缩进——落盘时按 listen 18090 行
// 缩进对齐）。
func landingTLSLines(tls landingTLS) []string {
	return []string{
		fmt.Sprintf("listen %d ssl;", ToolLandingTLSPort),
		fmt.Sprintf("ssl_certificate %q;", tls.certPath),
		fmt.Sprintf("ssl_certificate_key %q;", tls.keyPath),
	}
}

// refreshLandingBlock 自动块形态对账刷新（v1.4 D-24 扩展 D-20 server_name
// 口径）：server_name（含 if 404 行）+TLS 监听行组（listen 2087 ssl+证书
// 两行）+块外指纹注释按 desired 对账，经 ConfigApply；全一致零写入（幂等）。
// 仅改上述行 span——工具 location 注入字节不动（R-06 其余字节恒不变）。
// 旧版本所建 http-only 块升级后经本函数补齐 TLS 行（升级自愈主通道）。
func (s *Service) refreshLandingBlock(ctx context.Context, file, content, bindDomain string, tls landingTLS) (bool, error) {
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
	var snNode, ifNode, listenNode *nginxx.Node
	var tlsNodes []*nginxx.Node // listen 2087 ssl + ssl_certificate + ssl_certificate_key（在场序）
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
		case ch.Kind == nginxx.KindDirective && ch.Name == "listen" && len(ch.Args) > 0 &&
			ch.Args[0] == strconv.Itoa(ToolLandingPort):
			listenNode = ch
		case ch.Kind == nginxx.KindDirective && (ch.Name == "listen" || ch.Name == "ssl_certificate" || ch.Name == "ssl_certificate_key"):
			isTLSListen := ch.Name == "listen" && len(ch.Args) > 0 && ch.Args[0] == strconv.Itoa(ToolLandingTLSPort)
			isSSLLine := ch.Name != "listen"
			if isTLSListen || isSSLLine {
				tlsNodes = append(tlsNodes, ch)
			}
		}
	}
	if snNode == nil || listenNode == nil {
		return false, fmt.Errorf("dbtools: auto landing block missing server_name/listen %d (%s)", ToolLandingPort, file)
	}
	ifText := ""
	if bindDomain != "" {
		ifText = landingHostIf(bindDomain)
	}
	// 幂等判定：server_name 全等（大小写不敏感）+ if 行状态一致 + TLS 行组
	// 状态一致 + 指纹注释一致→零写入。
	nameOK := len(blk.ServerNames) == 1 && strings.EqualFold(blk.ServerNames[0], desired)
	ifOK := (bindDomain == "" && ifNode == nil) || (ifNode != nil && tree.TextOf(ifNode) == ifText)
	tlsOK := true
	if tls.active() {
		want := landingTLSLines(tls)
		tlsOK = len(tlsNodes) == len(want)
		for i := 0; tlsOK && i < len(tlsNodes); i++ {
			tlsOK = tree.TextOf(tlsNodes[i]) == want[i]
		}
	} else {
		tlsOK = len(tlsNodes) == 0
	}
	fpNode := findLandingFPComment(tree)
	fpOK := (tls.active() && fpNode != nil && strings.Contains(tree.TextOf(fpNode), "fingerprint: "+tls.fp)) ||
		(!tls.active() && fpNode == nil)
	if nameOK && ifOK && tlsOK && fpOK {
		return false, nil
	}
	indent := tree.IndentOf(blk.Node)
	if err := tree.Apply(snNode.Start, snNode.End, landingServerName(bindDomain)); err != nil {
		return false, fmt.Errorf("dbtools: landing server_name edit: %w", err)
	}
	switch {
	case bindDomain == "" && ifNode != nil:
		// 解绑：移除 if 行（含行尾换行）。
		if err := removeLine(tree, ifNode, "landing if removal"); err != nil {
			return false, err
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
	// TLS 行组对账：不在场/内容漂移→移除旧行后按 listen 18090 行重插（幂等
	// 重写式对账——路径/端口变化均收敛到期望形态）；HTTPS 关闭→仅移除。
	tlsChanged := !tlsOK
	if tlsChanged {
		for _, n := range tlsNodes {
			if err := removeLine(tree, n, "landing tls line removal"); err != nil {
				return false, err
			}
		}
	}
	if tls.active() && tlsChanged {
		ind := tree.IndentOf(listenNode)
		var ins strings.Builder
		for _, ln := range landingTLSLines(tls) {
			ins.WriteString("\n" + ind + ln)
		}
		// listenNode 偏移随前序编辑重映射（Tree.Apply 维护节点偏移），行尾
		// 插入（LineEnd 指向 '\n'，文本落在行文本与换行之间）。
		at := tree.LineEnd(listenNode)
		if err := tree.Apply(at, at, ins.String()); err != nil {
			return false, fmt.Errorf("dbtools: landing tls insert: %w", err)
		}
	}
	// 指纹注释对账（块外 server 行前）。
	switch {
	case tls.active() && fpNode != nil && !strings.Contains(tree.TextOf(fpNode), "fingerprint: "+tls.fp):
		if err := tree.Apply(fpNode.Start, fpNode.End, landingFPComment(tls.fp)); err != nil {
			return false, fmt.Errorf("dbtools: landing fp edit: %w", err)
		}
	case tls.active() && fpNode == nil:
		at := tree.LineStart(blk.Node)
		if err := tree.Apply(at, at, landingFPComment(tls.fp)+"\n"); err != nil {
			return false, fmt.Errorf("dbtools: landing fp insert: %w", err)
		}
	case !tls.active() && fpNode != nil:
		if err := removeLine(tree, fpNode, "landing fp removal"); err != nil {
			return false, err
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

// findLandingFPComment 定位块外指纹注释行（v1.4 D-24；无→nil）。
func findLandingFPComment(tree *nginxx.Tree) *nginxx.Node {
	for _, n := range tree.Nodes {
		if n.Kind == nginxx.KindComment && strings.Contains(n.Text, "Landing cert fingerprint:") {
			return n
		}
	}
	return nil
}

// removeLine 移除节点所在行（含行尾换行；刷新对账共用）。
func removeLine(tree *nginxx.Tree, n *nginxx.Node, what string) error {
	start, end := tree.LineStart(n), tree.LineEnd(n)
	if end < len(tree.Source) && tree.Source[end] == '\n' {
		end++
	}
	if err := tree.Apply(start, end, ""); err != nil {
		return fmt.Errorf("dbtools: %s: %w", what, err)
	}
	return nil
}

// OnBindDomainChanged panel-core 配置变更钩子实现（v1.3，D-20③⑤，
// ConfigChangeHook 消费契约，main 装配注册）：bindDomain 变更（""=解绑）→
// 自动块存在则重渲染 server_name（含 if 404 行）+TLS 监听行组（D-24）经
// ConfigApply 刷新；一致零写入。刷新失败仅记日志——钩子失败不阻断配置变更
// （panel_config 权威已成功），install/open 惰性校正兜底自愈。
func (s *Service) OnBindDomainChanged(ctx context.Context, bindDomain string) {
	file, content, ok := s.findAutoLanding()
	if !ok {
		return // 无自动块=正常态（既有面板域落地块形态不受本决策影响）
	}
	pa, err := s.panel.PanelAccess(ctx)
	tls := landingTLS{}
	if err != nil {
		slog.WarnContext(ctx, "dbtools: bind-domain hook panel access read failed (refresh skipped)",
			"err", err)
		return
	}
	// TLS 材料供给失败（非「未配置证书」）→跳过本次刷新保住既有 TLS 行，
	// 降级为删行会让 HTTPS 落地悄然失联；惰性校正兜底自愈。
	tls, terr := s.ensureLandingTLS(ctx, pa)
	if terr != nil {
		slog.WarnContext(ctx, "dbtools: bind-domain hook landing tls material failed (refresh skipped)",
			"err", terr)
		return
	}
	if _, err := s.refreshLandingBlock(ctx, file, content, bindDomain, tls); err != nil {
		slog.WarnContext(ctx, "dbtools: bind-domain hook landing refresh failed (lazy self-heal pending)",
			"file", file, "err", err)
	}
}

// lazyRefreshLanding 惰性校正（D-20④：open/reopen 兜底——自愈钩子失败/
// 面板重启窗口）：自动块形态按当前 BindDomain+TLS 态比对刷新；失败不阻断
// 打开（slog 降级口径）。返回值=落盘实况下自动块的 TLS 监听端口（0=无——
// 刷新成功与否均以磁盘为准，URL 分支必须与 nginx 实际监听一致，D-24）。
func (s *Service) lazyRefreshLanding(ctx context.Context, ref siteRef, pa PanelAccess) int {
	if !ref.AutoCreated {
		return 0
	}
	content, _, err := s.nginx.ReadFile(ref.File)
	if err != nil {
		slog.WarnContext(ctx, "dbtools: lazy landing refresh read failed", "file", ref.File, "err", err)
		return 0
	}
	tls, terr := s.ensureLandingTLS(ctx, pa)
	if terr != nil {
		slog.WarnContext(ctx, "dbtools: lazy landing tls material failed (refresh skipped, disk state rules)",
			"err", terr)
		return s.landingTLSPortFromConf(ref.File)
	}
	if _, rerr := s.refreshLandingBlock(ctx, ref.File, content, pa.BindDomain, tls); rerr != nil {
		slog.WarnContext(ctx, "dbtools: lazy landing refresh failed (open not blocked)",
			"file", ref.File, "err", rerr)
	}
	return s.landingTLSPortFromConf(ref.File)
}

// landingTLSPortFromConf 落盘实况判定：自动落地块是否已具 listen 2087 ssl
// 行（v1.4 D-24；刷新失败/跳过窗口 URL 分支的权威口径——读失败=0 走 http
// 兜底，不虚构 TLS）。
func (s *Service) landingTLSPortFromConf(file string) int {
	content, _, err := s.nginx.ReadFile(file)
	if err != nil {
		return 0
	}
	for _, blk := range nginxx.ExtractServers(nginxx.Parse(content)) {
		if blk.Dormant {
			continue
		}
		for _, l := range blk.Listen {
			if strings.Contains(l, strconv.Itoa(ToolLandingTLSPort)) {
				return ToolLandingTLSPort
			}
		}
	}
	return 0
}

// appendLandingHint 自动块形态安装成功输出尾部放行提示（D-19⑦：仅提示
// 不自动放行——防火墙联动 OUT；tlsActive 时提示加放行 2087）。
func appendLandingHint(ring *outputRing, ref siteRef, tlsActive bool) {
	if !ref.AutoCreated {
		return
	}
	listen := fmt.Sprintf("listen %d", ToolLandingPort)
	ports := strconv.Itoa(ToolLandingPort)
	if tlsActive {
		listen += fmt.Sprintf(" + %d ssl(HTTPS)", ToolLandingTLSPort)
		ports += "、" + strconv.Itoa(ToolLandingTLSPort)
	}
	ring.append(fmt.Sprintf("[landing] 自动创建工具落地块 %s（%s）；请放行 TCP %s 端口（防火墙/云安全组）",
		ref.File, listen, ports))
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
