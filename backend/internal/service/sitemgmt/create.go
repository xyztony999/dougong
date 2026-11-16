package sitemgmt

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dougong/dougong/internal/adapter/phpfpm"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// SiteCreateService（§4.2.1）：三模板建站（FR-14~16）+ D-15 防重 + D-03 落点。
// 模板内置默认值：php_fpm 含 FastCGI 参数模板（⚠️A14，不依赖发行版 snippet）；
// reverse_proxy 含 Host 头与 WS 透传默认值（A-SM 模板口径）。

// CreateSite POST /sites/site/create：校验链（type/域名/端口/根目录/上游/
// pool 实况 → D-15 防重）→ 模板生成 → 写入 D-03 落点文件（dougong-{主域名}.
// conf）→ WritePipeline + reload → 审计 site_created。
func (s *Service) CreateSite(ctx context.Context, actor *authsvc.Identity, req CreateSiteRequest) (*CreateSiteData, error) {
	switch req.Type {
	case SiteTypeStatic, SiteTypePHPFPM, SiteTypeReverseProx:
	default:
		return nil, fieldError("type", "type 须为 static|php_fpm|reverse_proxy")
	}
	if e := validateDomainList(req.ServerNames); e != nil {
		return nil, e
	}
	if e := validateListenDecl(req.Listen); e != nil {
		return nil, e
	}
	if err := s.requireNginx(); err != nil { // I-12/D-15：nginx 未检出空态前置
		return nil, err
	}
	primary := req.ServerNames[0]
	switch req.Type {
	case SiteTypeStatic:
		if req.Root == "" {
			return nil, fieldError("root", "type=static 时 root 必填")
		}
	case SiteTypePHPFPM:
		// root 为可选项（契约 §4.2.4：root?(type=static 必填)——Gate 2 F-1
		// 修复：php_fpm 不强制 root，空则模板省略 root 指令）。
		if req.PoolName == "" {
			return nil, fieldError("poolName", "type=php_fpm 时 poolName 必填")
		}
	case SiteTypeReverseProx:
		if e := validateUpstream(req.Upstream); e != nil {
			return nil, e
		}
	}
	var poolSocket string
	if req.Type == SiteTypePHPFPM {
		pool, err := s.requirePool(ctx, req.PoolName)
		if err != nil {
			return nil, err
		}
		poolSocket = pool.Socket // fastcgi_pass 绑定 socket 实况（D-14）
	}
	if req.Root != "" && !s.gw.Exists(req.Root) {
		return nil, fieldError("root", "根目录不存在")
	}
	// D-15 防重：同「主域名+监听端口」server 块已存在（扫描实况）。
	scan, err := s.scanSites(ctx)
	if err != nil {
		return nil, err
	}
	if existing := findDuplicateSite(scan, primary, listenPort(req.Listen)); existing != "" {
		return nil, errcode.ErrSiteExists.WithData(ExistingSiteData{ExistingSiteID: existing})
	}
	// D-03 落点：首个存在可写惯例目录 + dougong-{规范化主域名}.conf。
	dir, err := s.gw.FirstWritableDir()
	if err != nil {
		return nil, errcode.ErrNginxUnavailable.WithData(ReasonData{Reason: err.Error()})
	}
	file := filepath.Join(dir, NewSiteFilePrefix+normalizeDomain(primary)+".conf")
	content := renderTemplate(req, poolSocket)
	if err := s.applyPipeline(ctx, file, content, true); err != nil {
		return nil, err
	}
	if rerr := s.reloadAfterWrite(ctx); rerr != nil {
		return &CreateSiteData{
			SiteID: buildSiteID(file, 0), Type: req.Type,
			ServerNames: req.ServerNames, Listen: req.Listen, SourceFile: file,
		}, rerr
	}
	s.auditAction(ctx, actor, AuditActionSiteCreated, ObjectTypeSite, buildSiteID(file, 0),
		map[string]any{"type": req.Type}, true)
	return &CreateSiteData{
		SiteID: buildSiteID(file, 0), Type: req.Type,
		ServerNames: req.ServerNames, Listen: req.Listen, SourceFile: file,
	}, nil
}

// requirePool pool 实况存在性（SITE_009，D-14 实时读取）；命中返回 pool
// （socket 供 fastcgi_pass 模板绑定）。
func (s *Service) requirePool(ctx context.Context, name string) (*phpfpm.Pool, error) {
	pools, err := s.phpfpm.ListPools(ctx)
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: list php pools: %w", err)
	}
	for i := range pools {
		if pools[i].Name == name {
			return &pools[i], nil
		}
	}
	return nil, errcode.ErrPoolNotFound.WithData(ReasonData{Reason: name})
}

// listenPort 提取监听声明的端口部分（防重签名；形态已校验）。
func listenPort(decl string) string {
	head := strings.Fields(decl)[0]
	if i := strings.LastIndex(head, ":"); i >= 0 && !strings.HasSuffix(head, "]") {
		return head[i+1:]
	}
	return head
}

// findDuplicateSite D-15：扫描实况中同主域名+端口的 server 块（enabled 或
// disabled 均算存在——停用≠删除，OUT-6）。
func findDuplicateSite(scan *ScanResult, primary, port string) string {
	for i := range scan.Entries {
		e := &scan.Entries[i]
		if len(e.Block.ServerNames) == 0 || e.Block.Listen == nil {
			continue
		}
		if !strings.EqualFold(e.Block.ServerNames[0], primary) {
			continue
		}
		for _, l := range e.Block.Listen {
			if listenPort(l) == port {
				return e.SiteID
			}
		}
	}
	return ""
}

// renderTemplate 按类型生成 server 块文本（三模板，FR-14~16；poolSocket 为
// php_fpm pool 的 listen 实况值）。
func renderTemplate(req CreateSiteRequest, poolSocket string) string {
	var sb strings.Builder
	sb.WriteString("server {\n")
	sb.WriteString("    listen " + req.Listen + ";\n")
	sb.WriteString("    server_name " + strings.Join(req.ServerNames, " ") + ";\n")
	switch req.Type {
	case SiteTypeStatic:
		sb.WriteString("    root " + req.Root + ";\n")
		sb.WriteString("    index index.html index.htm;\n")
	case SiteTypePHPFPM:
		if req.Root != "" { // root 可选（Gate 2 F-1）：空则省略指令（继承上层或默认值）
			sb.WriteString("    root " + req.Root + ";\n")
		}
		sb.WriteString("    index index.php index.html;\n\n")
		sb.WriteString("    location / {\n")
		sb.WriteString("        try_files $uri $uri/ /index.php?$query_string;\n")
		sb.WriteString("    }\n\n")
		sb.WriteString("    location ~ \\.php$ {\n")
		sb.WriteString("        fastcgi_pass " + poolSocket + ";\n")
		// ⚠️A14：FastCGI 参数模板内置，不依赖发行版 snippet。
		sb.WriteString("        fastcgi_index index.php;\n")
		sb.WriteString("        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;\n")
		sb.WriteString("        include fastcgi_params;\n")
		sb.WriteString("    }\n")
	case SiteTypeReverseProx:
		sb.WriteString("\n")
		sb.WriteString("    location / {\n")
		sb.WriteString("        proxy_pass " + req.Upstream + ";\n")
		host := req.HostHeader
		if host == "" {
			host = defaultHostHeader
		}
		sb.WriteString("        proxy_set_header Host " + host + ";\n")
		sb.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
		sb.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
		if req.Websocket == nil || *req.Websocket { // WS 透传默认值（模板内置）
			sb.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
			sb.WriteString("        proxy_set_header Connection \"upgrade\";\n")
		}
		sb.WriteString("    }\n")
	}
	sb.WriteString("}\n")
	return sb.String()
}
