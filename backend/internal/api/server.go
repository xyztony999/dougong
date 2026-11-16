// Package api 是 HTTP 传输层骨架：gin engine 装配、全局中间件、模块路由
// 注册器。后续 10 个模块实现 Module 接口即可挂载（panel-core 契约 R-10）。
package api

import (
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"path"
	"strings"

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
}

// spaFS 是内嵌前端 dist 子文件系统（web.DistFS 的 "dist" 子树）。
var spaFS fs.FS

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
//   - 其余路径按 dist 静态文件服务（MIME + 缓存头：vite 哈希资源 immutable，
//     index.html no-cache）；
//   - 文件不存在 → 回退 index.html（SPA 深链 /login、/sites/... 由前端路由
//     接管）；dist 尚未构建（占位态）→ 404 包络并记录告警日志。
func (s *Server) serveSPA(c *gin.Context) {
	p := path.Clean(c.Request.URL.Path)
	for _, mod := range s.modules {
		if p == mod.Prefix() || strings.HasPrefix(p, mod.Prefix()+"/") {
			response.FailWith(c, errcode.ErrNotFound)
			return
		}
	}
	name := strings.TrimPrefix(p, "/")
	if name == "" {
		name = "index.html"
	}
	f, err := spaFS.Open(name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		response.FailWith(c, errcode.ErrInternal)
		return
	}
	if err != nil { // SPA 深链回退 index.html
		name = "index.html"
		if f, err = spaFS.Open(name); err != nil {
			slog.WarnContext(c.Request.Context(), "spa: dist not populated (placeholder only)")
			response.FailWith(c, errcode.ErrNotFound)
			return
		}
	}
	defer f.Close()
	if info, statErr := f.Stat(); statErr != nil || info.IsDir() {
		response.FailWith(c, errcode.ErrNotFound)
		return
	}
	if strings.HasPrefix(name, "assets/") {
		c.Header("Cache-Control", "public, max-age=31536000, immutable") // 内容哈希命名
	} else {
		c.Header("Cache-Control", "no-cache") // index.html 等入口文件
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		c.Header("Content-Type", ct)
	}
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, f)
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
