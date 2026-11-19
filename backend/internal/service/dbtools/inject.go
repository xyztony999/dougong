package dbtools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/errcode"
	storedbt "github.com/dougong/dougong/internal/store/dbtools"
)

// siteRef 注入落点标识（db_tool.site_ref JSON：来源文件+server 块定位，
// D-03；撤除寻址用）。ServerName 为首个具体域名（wildcard/空=""——URL
// 构造回退 bind_domain/请求 Host）；Scheme 由 listen 443/ssl 推导。
// v1.3（D-19）：+Port=落地监听端口（既有 80/443 块=0 表默认；自动块=
// ToolLandingPort）+AutoCreated=是否面板自动创建（清块判定 I-C04）；
// 旧档案缺字段零值 port=0/autoCreated=false 向后兼容（零 DDL）。
type siteRef struct {
	File        string `json:"file"`
	Index       int    `json:"index"`
	ServerName  string `json:"serverName"`
	Scheme      string `json:"scheme"`
	Port        int    `json:"port"`
	AutoCreated bool   `json:"autoCreated"`
}

func parseSiteRef(raw string) (siteRef, error) {
	var ref siteRef
	if err := json.Unmarshal([]byte(raw), &ref); err != nil {
		return siteRef{}, fmt.Errorf("dbtools: parse site_ref: %w", err)
	}
	if ref.File == "" {
		return siteRef{}, fmt.Errorf("dbtools: site_ref missing file")
	}
	return ref, nil
}

func marshalSiteRef(ref siteRef) string {
	b, _ := json.Marshal(ref) // 结构体字段全为基础类型， marshal 不可能失败
	return string(b)
}

// selectLanding 落点选择（D-03，v1.3 D-19 修订）：bind_domain 非空→
// server_name 匹配块（优先）；未绑定→面板反代站点块（proxy_pass 指向面板
// 监听地址）；多候选→DBT_012 列出候选（操作者经站点管理明确后重试）；
// 无候选→自动创建专用工具落地块（D-19：IP 直连未绑域名形态安装即可用；
// 端口占用/目录不可用/写入失败仍 DBT_012 data.reason 如实）。dormant
// （停用）块不参与。
func (s *Service) selectLanding(ctx context.Context, pa PanelAccess) (siteRef, error) {
	files, err := s.nginx.ScanConfDirs()
	if err != nil {
		return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "扫描 nginx 配置目录失败: " + err.Error()})
	}
	type candidate struct {
		file  string
		block *nginxx.ServerBlock
	}
	var matches []candidate
	for i := range files {
		// v1.3（D-19）：自动落地块为工具专用块，不参与面板域候选——其
		// gate location proxy_pass 指向面板监听地址，会被 proxyTargetsPanel
		// 误判为面板反代站点块（第二工具安装时 siteRef 丢失 port/autoCreated）。
		if files[i].Name == ToolLandingConfName && isLandingBlock(files[i].Content) {
			continue
		}
		tree := nginxx.Parse(files[i].Content)
		for _, blk := range nginxx.ExtractServers(tree) {
			if blk.Dormant {
				continue
			}
			if pa.BindDomain != "" {
				if serverNameMatches(blk, pa.BindDomain) {
					matches = append(matches, candidate{files[i].Path, blk})
				}
				continue
			}
			if proxyTargetsPanel(blk, pa) {
				matches = append(matches, candidate{files[i].Path, blk})
			}
		}
	}
	switch len(matches) {
	case 0:
		return s.autoCreateLanding(ctx, pa)
	case 1:
		blk := matches[0].block
		ref := siteRef{File: matches[0].file, Index: blk.Index, Scheme: "http"}
		for _, name := range blk.ServerNames {
			if name != "" && name != "_" && !strings.Contains(name, "*") {
				ref.ServerName = name
				break
			}
		}
		for _, l := range blk.Listen {
			if strings.Contains(l, "443") || strings.Contains(l, "ssl") {
				ref.Scheme = "https"
				break
			}
		}
		return ref, nil
	default:
		var descs []string
		for _, m := range matches {
			descs = append(descs, fmt.Sprintf("%s#%d(%s)", m.file, m.block.Index,
				strings.Join(m.block.ServerNames, ",")))
		}
		return siteRef{}, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "面板域落点存在多个候选 server 块，请在站点管理中明确后重试: " + strings.Join(descs, "; ")})
	}
}

// serverNameMatches server_name 全等匹配（大小写不敏感；wildcard 不参与）。
func serverNameMatches(blk *nginxx.ServerBlock, domain string) bool {
	for _, n := range blk.ServerNames {
		if strings.EqualFold(n, domain) {
			return true
		}
	}
	return false
}

// proxyTargetsPanel 主反代段上游是否指向面板监听地址（nginxx ServerBlock.
// Proxy() 识别；localhost 归一 127.0.0.1；listen_scope=all 时本机 loopback
// 上游均视为面板反代）。
func proxyTargetsPanel(blk *nginxx.ServerBlock, pa PanelAccess) bool {
	p := blk.Proxy()
	if p == nil || p.Upstream == "" {
		return false
	}
	u, err := url.Parse(p.Upstream)
	if err != nil || u.Host == "" {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		host = "127.0.0.1"
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}
	if port != strconv.Itoa(pa.ListenPort) {
		return false
	}
	listenHost := pa.ListenHost
	if listenHost == "localhost" {
		listenHost = "127.0.0.1"
	}
	if host == listenHost {
		return true
	}
	// 面板绑 0.0.0.0（all）时，loopback 上游即面板反代。
	return listenHost == "0.0.0.0" && (host == "127.0.0.1" || host == "::1")
}

// injectToolLocations 工具 location + 内部 gate location 注入（D-03/D-11；
// 经 ConfigWritePipeline R-06 四步安全写回，A-DBT8 标记 span）。gate 共享
// 幂等（已存在跳过）；注入内容安装时烘焙（A-DBT6 时效声明）。
func (s *Service) injectToolLocations(ctx context.Context, toolType string, ref siteRef,
	pa PanelAccess, secret, poolSocket string) error {
	content, _, err := s.nginx.ReadFile(ref.File)
	if err != nil {
		return fmt.Errorf("dbtools: read injection target %s: %w", ref.File, err)
	}
	tree := nginxx.Parse(content)
	blocks := nginxx.ExtractServers(tree)
	if ref.Index < 0 || ref.Index >= len(blocks) {
		return fmt.Errorf("dbtools: injection target server block #%d out of range (%s)", ref.Index, ref.File)
	}
	blk := blocks[ref.Index]
	if blk.Dormant {
		return fmt.Errorf("dbtools: injection target server block #%d is dormant (%s)", ref.Index, ref.File)
	}
	indent := tree.IndentOf(blk.Node)
	insert := ""
	if !hasMarker(content, markerKeyGate) {
		insert += gateLocationInsert(indent, pa, secret)
	}
	switch toolType {
	case storedbt.ToolPHPMyAdmin:
		insert += phpMyAdminLocationInsert(indent, s.toolDir, poolSocket)
	case storedbt.ToolMongoExpress:
		insert += mongoExpressLocationInsert(indent, s.toolDir)
	default:
		return fmt.Errorf("dbtools: unknown tool type %q", toolType)
	}
	at := tree.EndOfBlockBody(blk.Node)
	if err := tree.Apply(at, at, insert); err != nil {
		return fmt.Errorf("dbtools: apply injection span: %w", err)
	}
	newContent, err := tree.Serialize()
	if err != nil {
		return fmt.Errorf("dbtools: serialize injection: %w", err)
	}
	if err := s.pipeline.ConfigApply(ctx, ref.File, newContent, false); err != nil {
		return fmt.Errorf("dbtools: config pipeline apply: %w", err)
	}
	return nil
}

// removeToolInjection 撤除注入恢复原字节（D-03/D-15：标记 span 精确移除，
// A-DBT8；最后一个工具撤除时一并撤除共享 gate location）。文件已不存在=
// 幂等跳过；标记损坏=如实报错（不静默残留）。
func (s *Service) removeToolInjection(ctx context.Context, toolType string, ref siteRef) error {
	content, _, err := s.nginx.ReadFile(ref.File)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 落点文件已被外部移除——注入随之消亡，幂等
		}
		return fmt.Errorf("dbtools: read injection target %s: %w", ref.File, err)
	}
	newContent, removed, err := stripMarker(content, toolType)
	if err != nil {
		return err
	}
	if !removed {
		return nil // 注入已被外部撤除——幂等
	}
	if !hasMarker(newContent, storedbt.ToolPHPMyAdmin) && !hasMarker(newContent, storedbt.ToolMongoExpress) {
		newContent, _, err = stripMarker(newContent, markerKeyGate)
		if err != nil {
			return err
		}
	}
	if err := s.pipeline.ConfigApply(ctx, ref.File, newContent, false); err != nil {
		return fmt.Errorf("dbtools: config pipeline apply (removal): %w", err)
	}
	return nil
}

// hasMarker 报告注入标记存在性（运行态探测/幂等判定共用）。
func hasMarker(content, key string) bool {
	return strings.Contains(content, markerBeginPrefix+key)
}

// stripMarker 精确移除标记 span（恢复注入前原字节）：移除 BEGIN 行所在行
// （含其前导换行——注入时添加）至 END 标记文本末尾。BEGIN 存在而 END 缺失
// =标记损坏如实报错（保守不静默）。
func stripMarker(content, key string) (string, bool, error) {
	begin := markerBeginPrefix + key
	end := markerEndPrefix + key
	i := strings.Index(content, begin)
	if i < 0 {
		return content, false, nil
	}
	j := strings.Index(content[i:], end)
	if j < 0 {
		return "", false, fmt.Errorf("dbtools: injection marker corrupted (BEGIN without END): %s", key)
	}
	lineStart := strings.LastIndex(content[:i], "\n") + 1
	start := lineStart
	if start > 0 && content[start-1] == '\n' {
		start-- // 注入添加的前导换行一并移除（byte 级还原）
	}
	endPos := i + j + len(end)
	// 注入在 END 标记后添加了换行+缩进（闭合 '}' 保持独立行）——一并移除
	// 至首个非空白字符（注入锚点为闭合 '}'，其间空白全为注入所有）。
	if endPos < len(content) && content[endPos] == '\n' {
		endPos++
		for endPos < len(content) && (content[endPos] == ' ' || content[endPos] == '\t') {
			endPos++
		}
	}
	return content[:start] + content[endPos:], true, nil
}
