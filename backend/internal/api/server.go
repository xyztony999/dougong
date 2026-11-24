// Package api 是 HTTP 传输层骨架：gin engine 装配、全局中间件、模块路由
// 注册器。后续 10 个模块实现 Module 接口即可挂载（panel-core 契约 R-10）。
package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"html"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"path"
	"strconv"
	"strings"
	"sync"

	"net/http"

	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/web"
	"github.com/gin-gonic/gin"
)

// Module 是业务模块路由注册契约：Prefix 为挂载路径段（如 "/auth"），
// Register 在该前缀的路由组上注册子路由与模块级中间件
// （推荐链序：InitGuard → AuthRequired → CsrfGuard(POST) → RoleRequired）。
type Module interface {
	Prefix() string
	Register(rg *gin.RouterGroup)
}

// Server 持有 gin engine 与已注册模块。
type Server struct {
	engine  *gin.Engine
	modules []Module

	// spaGzip 是内嵌页面文件的 gzip 压缩缓存。文件名带内容哈希、内容在进程生命周期内不变，
	// 所以每个文件只压缩一次（首次被请求时），之后直接发缓存——不是每个请求现压（api D-P19）。
	spaGzip   map[string][]byte
	spaGzipMu sync.Mutex

	// entryPath 是安全入口状态来源（可选，由 SetEntryPathSource 注入）：下发入口 HTML 时
	// 据此注入 <base>，让页面里的相对地址落在入口路径之下（api D-P03）。nil 时基址恒为 /。
	entryPath middleware.EntryPathSource
}

// spaFS 是内嵌前端 dist 子文件系统（web.DistFS 的 "dist" 子树）。
var spaFS fs.FS

// SetEntryPathSource 注入安全入口状态来源（main 装配时调用）。
func (s *Server) SetEntryPathSource(src middleware.EntryPathSource) { s.entryPath = src }

// 内嵌页面文件的传输参数（api D-P19）。
const (
	// spaGzipLevel 压缩级别。实测 v0.4.0 构建的全部可压缩文件（16.8MB）：级别 1 / 5 / 6 分别压到
	// 5.10 / 4.31 / 4.28MB，耗时 0.12 / 0.24 / 0.31 秒——5 比 1 小 15%，比 6 只大 0.6% 而快约 20%。
	spaGzipLevel = 5
	// spaGzipMinSize 小于该大小的文件不压缩（压完不一定更小，白占 CPU）。
	spaGzipMinSize = 1024
)

func init() {
	sub, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		panic("api: embed dist subtree: " + err.Error()) // 启动期不可达（embed 编译期保证）
	}
	spaFS = sub
}

// NewServer 构造 HTTP 骨架：gin.New + 自定义 recovery + 请求访问日志 +
// NoRoute=内嵌前端 SPA 服务（API 前缀未命中维持 404 包络，统一 200+code 语义）。
// 业务中间件由各模块路由组自挂。
func NewServer() *Server {
	engine := gin.New()
	engine.Use(middleware.Recovery(), middleware.RequestLog())
	s := &Server{engine: engine}
	engine.NoRoute(s.serveSPA)
	return s
}

// serveSPA 在 NoRoute 上服务内嵌前端（go:embed，panel-core「入口根=SPA
// 挂载点」决策与 EntryPathHandler 的入口根→"/" 映射配套，ADR-009 单二进制）：
//   - 命中已注册 API 模块前缀的未知路径 → 统一 404 包络（API 契约不变，
//     不把前端 index 回给 API 客户端）；
//   - 其余路径按 dist 静态文件服务，传输规则见 api D-P19：
//     缓存头（哈希资源 immutable、index.html no-cache）、gzip 压缩并带 Vary、
//     显式 Content-Length、HEAD 与 GET 同头；
//   - `/assets/` 下不存在的文件 → 标准 404（不走 SPA 兜底，见下）；
//   - 其它路径文件不存在 → 回退 index.html（SPA 深链 /login、/sites/... 由前端路由
//     接管）；dist 尚未构建（占位态）→ 404 包络并记录告警日志。
func (s *Server) serveSPA(c *gin.Context) {
	p := path.Clean(c.Request.URL.Path)
	for _, mod := range s.modules {
		if p == mod.Prefix() || strings.HasPrefix(p, mod.Prefix()+"/") {
			// 浏览器导航（刷新/深链/收藏夹，Accept 含 text/html）继续走 SPA
			// 服务——九个模块前缀与 SPA 页面路由同名（/env /sites /services
			// /files /monitoring /terminal /firewall /docker /cron），v0.1.4 前
			// 刷新这些页面会吐 404 包络裸 JSON（v0.1.3 实机 /env 反馈）；
			// API 客户端（fetch/curl，Accept 无 text/html）维持 404 包络契约。
			if strings.Contains(c.Request.Header.Get("Accept"), "text/html") {
				break
			}
			response.FailWith(c, errcode.ErrNotFound)
			return
		}
	}
	name := strings.TrimPrefix(p, "/")
	if name == "" {
		name = "index.html"
	}
	asset := strings.HasPrefix(name, "assets/")
	f, err := spaFS.Open(name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		response.FailWith(c, errcode.ErrInternal)
		return
	}
	if err != nil {
		if asset && path.Ext(name) != "" {
			// 带内容哈希的页面文件不存在：面板已升级而浏览器还开着旧页面，或前端在探测
			// 入口脚本是否还在。回标准 404、不落到下面的 SPA 兜底——兜底会把 index.html
			// 当这个文件发出去：中间缓存可能把 HTML 当脚本缓存下来，前端也分不清
			// 「面板已升级」和「网络失败」（api D-P19）。
			c.String(http.StatusNotFound, "404 page not found")
			return
		}
		name = "index.html" // SPA 深链回退 index.html
		if f, err = spaFS.Open(name); err != nil {
			slog.WarnContext(c.Request.Context(), "spa: dist not populated (placeholder only)")
			response.FailWith(c, errcode.ErrNotFound)
			return
		}
	}
	defer f.Close()
	info, statErr := f.Stat()
	if statErr != nil || info.IsDir() {
		response.FailWith(c, errcode.ErrNotFound)
		return
	}
	if asset {
		c.Header("Cache-Control", "public, max-age=31536000, immutable") // 内容哈希命名
		// 同一地址按 Accept-Encoding 有压缩 / 未压缩两种回应，中间缓存必须区分。
		c.Header("Vary", "Accept-Encoding")
	} else {
		c.Header("Cache-Control", "no-cache") // index.html 等入口文件
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		c.Header("Content-Type", ct)
	}
	size := info.Size()
	var compressed []byte
	if asset && size >= spaGzipMinSize && spaCompressible(name) && acceptsGzip(c.GetHeader("Accept-Encoding")) {
		if compressed = s.gzipAsset(name); compressed != nil {
			size = int64(len(compressed))
			c.Header("Content-Encoding", "gzip")
		}
	}
	// 入口 HTML：注入 <base>，让页面里的相对地址（脚本、图标、样式）落在入口路径之下。
	// 开启安全入口后，入口网关只放行带入口前缀的请求，而页面引用的是相对地址——基址
	// 不对的话脚本全部 404，浏览器里就是一片空白（api D-P03）。
	var document []byte
	if name == "index.html" {
		base, baseErr := s.spaBasePath(c.Request.Context())
		if baseErr != nil {
			// 与入口网关同口径：读不到入口状态时 fail-closed，标准 404
			slog.ErrorContext(c.Request.Context(), "spa: read entry path state", "err", baseErr)
			http.NotFound(c.Writer, c.Request)
			return
		}
		raw, readErr := io.ReadAll(f)
		if readErr != nil {
			response.FailWith(c, errcode.ErrInternal)
			return
		}
		document = spaIndexWithBase(raw, base)
		size = int64(len(document))
	}
	c.Header("Content-Length", strconv.FormatInt(size, 10))
	c.Status(http.StatusOK)
	if c.Request.Method == http.MethodHead {
		return // 与 GET 同样的头，不回响应体
	}
	if document != nil {
		_, _ = c.Writer.Write(document)
		return
	}
	if compressed != nil {
		_, _ = c.Writer.Write(compressed)
		return
	}
	_, _ = io.Copy(c.Writer, f)
}

// spaBasePath 返回页面基址（以 / 开头和结尾）：安全入口开启时为 /{entryPath}/，否则 /。
func (s *Server) spaBasePath(ctx context.Context) (string, error) {
	if s.entryPath == nil {
		return "/", nil
	}
	enabled, entryPath, err := s.entryPath.EntryPathState(ctx)
	if err != nil {
		return "", err
	}
	if !enabled || entryPath == "" {
		return "/", nil
	}
	return "/" + entryPath + "/", nil
}

// spaIndexWithBase 在入口 HTML 的 <head> 开头注入 <base href=…>（入口只有几百字节，
// 每次请求现拼，不缓存——内容随安全入口配置变化）。找不到 <head> 时原样返回。
func spaIndexWithBase(raw []byte, base string) []byte {
	lower := bytes.ToLower(raw)
	open := bytes.Index(lower, []byte("<head"))
	if open < 0 {
		return raw
	}
	rel := bytes.IndexByte(lower[open:], '>')
	if rel < 0 {
		return raw
	}
	at := open + rel + 1
	tag := []byte("<base href=\"" + html.EscapeString(base) + "\">")
	out := make([]byte, 0, len(raw)+len(tag))
	out = append(out, raw[:at]...)
	out = append(out, tag...)
	out = append(out, raw[at:]...)
	return out
}

// gzipAsset 返回内嵌页面文件的 gzip 内容，进程内缓存、每个文件只压缩一次。
// 读文件或压缩失败时返回 nil，调用方原样下发。持锁压缩：同一文件的并发首请求
// 只压一次，别的文件排在后面（最大的 7MB 文件约 0.09 秒）。
func (s *Server) gzipAsset(name string) []byte {
	s.spaGzipMu.Lock()
	defer s.spaGzipMu.Unlock()
	if cached, ok := s.spaGzip[name]; ok {
		return cached
	}
	raw, err := fs.ReadFile(spaFS, name)
	if err != nil {
		return nil
	}
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, spaGzipLevel)
	if err != nil {
		return nil
	}
	if _, err = w.Write(raw); err != nil {
		return nil
	}
	if err = w.Close(); err != nil {
		return nil
	}
	if s.spaGzip == nil {
		s.spaGzip = make(map[string][]byte)
	}
	s.spaGzip[name] = buf.Bytes()
	return s.spaGzip[name]
}

// spaCompressible 按扩展名判断是否压缩，不看 MIME 表——MIME 表因平台而异
// （Windows 读注册表、Linux 读 /etc/mime.types），同一文件两边必须判定一致。
func spaCompressible(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".js", ".mjs", ".css", ".svg", ".json", ".html", ".txt", ".map", ".ttf":
		return true
	}
	return false
}

// acceptsGzip 判断 Accept-Encoding 是否接受 gzip：gzip 或 * 且 q 不为 0；
// 显式的 gzip 优先于 *（`gzip;q=0, *` 不接受）。非法 q 值的参数忽略。
func acceptsGzip(header string) bool {
	gzipQ, wildcardQ := 1.0, 1.0
	seenGzip, seenWildcard := false, false
	for _, item := range strings.Split(header, ",") {
		parts := strings.Split(strings.TrimSpace(item), ";")
		coding := strings.ToLower(strings.TrimSpace(parts[0]))
		if coding != "gzip" && coding != "*" {
			continue
		}
		q := 1.0
		for _, param := range parts[1:] {
			kv := strings.SplitN(strings.TrimSpace(param), "=", 2)
			if len(kv) != 2 || !strings.EqualFold(strings.TrimSpace(kv[0]), "q") {
				continue
			}
			if parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(kv[1]), 64); parseErr == nil {
				q = parsed
			}
		}
		if coding == "gzip" {
			gzipQ, seenGzip = q, true
		} else {
			wildcardQ, seenWildcard = q, true
		}
	}
	if seenGzip {
		return gzipQ > 0
	}
	return seenWildcard && wildcardQ > 0
}

// RegisterModules 按序挂载业务模块（前缀冲突由 gin panic 快速暴露——启动期）。
func (s *Server) RegisterModules(mods ...Module) {
	for _, mod := range mods {
		s.modules = append(s.modules, mod)
		mod.Register(s.engine.Group(mod.Prefix()))
	}
}

// Handler 返回 http.Handler（供 http.Server 使用）。
func (s *Server) Handler() http.Handler { return s.engine }
