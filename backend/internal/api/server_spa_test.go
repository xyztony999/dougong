package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// stubModule 最小模块桩（serveSPA 前缀分支行为测试）。
type stubModule struct{ prefix string }

func (m stubModule) Prefix() string { return m.prefix }
func (m stubModule) Register(rg *gin.RouterGroup) {
	rg.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
}

func newSPAServer(prefix string) *Server {
	gin.SetMode(gin.TestMode)
	s := NewServer()
	s.RegisterModules(stubModule{prefix: prefix})
	return s
}

// TestServeSPAAPIClientKeeps404Envelope API 客户端视角：模块前缀下未知路径
// （无 text/html Accept）维持 404 包络契约（不把 index 回给 API 客户端）。
func TestServeSPAAPIClientKeeps404Envelope(t *testing.T) {
	s := newSPAServer("/env")
	for _, path := range []string{"/env", "/env/env/lis", "/env/task"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		s.engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "NOT_FOUND") {
			t.Fatalf("GET %s: want NOT_FOUND envelope, got %d %q", path, w.Code, w.Body.String())
		}
	}
}

// TestServeSPADeepLinkUnderModulePrefix 浏览器深链视角（v0.1.4 修复回归）：
// 模块前缀与 SPA 页面同名（/env 等）时，带 text/html Accept 的导航请求不再
// 吐 404 包络——dist 已构建时回 index.html（200 HTML）；dist 占位（CI 后端
// job 未构建前端）时跳过（placeholder 分支不可区分，属构建期形态）。
func TestServeSPADeepLinkUnderModulePrefix(t *testing.T) {
	s := newSPAServer("/env")
	req := httptest.NewRequest(http.MethodGet, "/env", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	w := httptest.NewRecorder()
	s.engine.ServeHTTP(w, req)

	// 占位探测：GET / 同样带 text/html 仍回 404 包络 → dist 未构建，跳过
	probe := httptest.NewRequest(http.MethodGet, "/", nil)
	probe.Header.Set("Accept", "text/html")
	pw := httptest.NewRecorder()
	s.engine.ServeHTTP(pw, probe)
	if strings.Contains(pw.Body.String(), "NOT_FOUND") {
		t.Skip("dist placeholder (frontend not built) — deep-link 200 assertion needs built assets")
	}

	if w.Code != http.StatusOK {
		t.Fatalf("GET /env deep link: want 200, got %d body=%q", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("GET /env deep link: want html content-type, got %q", ct)
	}
}
