package api

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// 内嵌前端页面文件的传输规则（api D-P19）。用确定性的内存文件系统替换内嵌 dist，
// 断言不依赖真实构建产物——真实产物由浏览器级验证覆盖。

// 夹具内容。strings.Repeat 不是常量表达式，只能用 var。
var (
	spaFixtureJS    = "const a = 1;\n" + strings.Repeat("// compressible filler line for the transport test\n", 60)
	spaFixtureTiny  = "const t = 1;\n"
	spaFixtureSVG   = "<svg>" + strings.Repeat("<!-- filler -->", 200) + "</svg>"
	spaFixtureTTF   = strings.Repeat("fontbytes", 300)
	spaFixtureBin   = strings.Repeat("binary00", 300)
	spaFixtureIndex = "<!doctype html><html><script type=\"module\" src=\"/assets/app-abc123.js\"></script></html>"
	spaAssetCache   = "public, max-age=31536000, immutable"
)

// useSPAFixture 把包级 spaFS 换成内存夹具（测试结束恢复），返回已注册模块桩的 Server。
func useSPAFixture(t *testing.T) *Server {
	t.Helper()
	prev := spaFS
	spaFS = fstest.MapFS{
		"index.html":             {Data: []byte(spaFixtureIndex)},
		"favicon.svg":            {Data: []byte("<svg/>")},
		"assets/app-abc123.js":   {Data: []byte(spaFixtureJS)},
		"assets/UPPER-ABC123.JS": {Data: []byte(spaFixtureJS)},
		"assets/tiny-abc123.js":  {Data: []byte(spaFixtureTiny)},
		"assets/logo-abc123.svg": {Data: []byte(spaFixtureSVG)},
		"assets/font-abc123.ttf": {Data: []byte(spaFixtureTTF)},
		"assets/data-abc123.bin": {Data: []byte(spaFixtureBin)},
	}
	t.Cleanup(func() { spaFS = prev })
	return newSPAServer("/env")
}

func spaDo(s *Server, method, target, acceptEncoding string, extra map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.engine.ServeHTTP(w, req)
	return w
}

func spaBody(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	if w.Header().Get("Content-Encoding") != "gzip" {
		return w.Body.String()
	}
	// 从副本解压：gzip.Reader 会把 Recorder 的缓冲读空，后面还要拿它量字节数。
	r, err := gzip.NewReader(bytes.NewReader(append([]byte(nil), w.Body.Bytes()...)))
	if err != nil {
		t.Fatalf("gzip body: %v", err)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	return string(b)
}

// TestServeSPAAssetTransport 同一个页面文件在不同 Accept-Encoding 下的回应：压缩与否、
// 解压后内容不变、Vary / 缓存头 / 显式 Content-Length（压缩后 = 压缩结果长度）。
func TestServeSPAAssetTransport(t *testing.T) {
	s := useSPAFixture(t)
	cases := []struct {
		name        string
		target      string
		encoding    string
		want        string // 解压后应得的内容
		wantGzip    bool
		description string
	}{
		{"gzip", "/assets/app-abc123.js", "gzip, deflate, br, zstd", spaFixtureJS, true, "浏览器默认的 Accept-Encoding"},
		{"upper-case extension", "/assets/UPPER-ABC123.JS", "gzip", spaFixtureJS, true, "扩展名判定不区分大小写"},
		{"no header", "/assets/app-abc123.js", "", spaFixtureJS, false, "不支持压缩的客户端原样下发"},
		{"identity", "/assets/app-abc123.js", "identity", spaFixtureJS, false, "显式只要 identity"},
		{"gzip;q=0", "/assets/app-abc123.js", "gzip;q=0", spaFixtureJS, false, "显式拒绝 gzip"},
		{"gzip;q=0, *", "/assets/app-abc123.js", "gzip;q=0, *", spaFixtureJS, false, "显式 gzip 优先于通配"},
		{"wildcard", "/assets/app-abc123.js", "*", spaFixtureJS, true, "通配接受"},
		{"wildcard;q=0", "/assets/app-abc123.js", "*;q=0", spaFixtureJS, false, "通配被拒"},
		{"br only", "/assets/app-abc123.js", "br", spaFixtureJS, false, "只要 br，没有 gzip"},
		{"GZIP case + q", "/assets/app-abc123.js", "GZIP;q=0.5", spaFixtureJS, true, "编码名大小写不敏感"},
		{"gzip space", "/assets/app-abc123.js", "deflate, gzip;q=1.0", spaFixtureJS, true, "gzip 在列表后段"},
		{"below min size", "/assets/tiny-abc123.js", "gzip", spaFixtureTiny, false, "小于 1KB 不压缩"},
		{"not compressible type", "/assets/data-abc123.bin", "gzip", spaFixtureBin, false, "不在可压缩类型内"},
		{"svg", "/assets/logo-abc123.svg", "gzip", spaFixtureSVG, true, "svg 可压缩"},
		{"ttf", "/assets/font-abc123.ttf", "gzip", spaFixtureTTF, true, "字体可压缩"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := spaDo(s, http.MethodGet, tc.target, tc.encoding, nil)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
			}
			got := spaBody(t, w)
			if got != tc.want {
				t.Fatalf("decoded body mismatch: len=%d want len=%d", len(got), len(tc.want))
			}
			enc := w.Header().Get("Content-Encoding")
			if tc.wantGzip && enc != "gzip" {
				t.Fatalf("Content-Encoding = %q, want gzip", enc)
			}
			if !tc.wantGzip && enc != "" {
				t.Fatalf("Content-Encoding = %q, want none", enc)
			}
			if cc := w.Header().Get("Cache-Control"); cc != spaAssetCache {
				t.Fatalf("Cache-Control = %q, want %q", cc, spaAssetCache)
			}
			if vary := w.Header().Get("Vary"); vary != "Accept-Encoding" {
				t.Fatalf("Vary = %q, want Accept-Encoding (with or without compression)", vary)
			}
			if cl := w.Header().Get("Content-Length"); cl != strconv.Itoa(w.Body.Len()) {
				t.Fatalf("Content-Length = %q, want %d (the bytes actually sent)", cl, w.Body.Len())
			}
			if w.Header().Get("ETag") != "" || w.Header().Get("Last-Modified") != "" {
				t.Fatalf("no validators expected, got ETag=%q Last-Modified=%q", w.Header().Get("ETag"), w.Header().Get("Last-Modified"))
			}
			if tc.wantGzip && w.Body.Len() >= len(tc.want) {
				t.Fatalf("gzip did not shrink the file: %d -> %d", len(tc.want), w.Body.Len())
			}
		})
	}
}

// TestServeSPACompressOnce 同一文件两次请求只压缩一次（缓存），两次回应一致。
func TestServeSPACompressOnce(t *testing.T) {
	s := useSPAFixture(t)
	first := spaDo(s, http.MethodGet, "/assets/app-abc123.js", "gzip", nil)
	second := spaDo(s, http.MethodGet, "/assets/app-abc123.js", "gzip", nil)
	if first.Body.String() != second.Body.String() {
		t.Fatal("two requests for the same file got different compressed bytes")
	}
	if len(s.spaGzip) != 1 {
		t.Fatalf("gzip cache entries = %d, want 1 (compress once per file)", len(s.spaGzip))
	}
	spaDo(s, http.MethodGet, "/assets/tiny-abc123.js", "gzip", nil)
	if len(s.spaGzip) != 1 {
		t.Fatalf("files below the size threshold must not be cached, entries = %d", len(s.spaGzip))
	}
}

// TestServeSPAGzipCacheIsReused 第二次取同一文件必须复用已压缩的字节（同一底层数组），
// 不是重新压一份。
func TestServeSPAGzipCacheIsReused(t *testing.T) {
	s := useSPAFixture(t)
	first := s.gzipAsset("assets/app-abc123.js")
	second := s.gzipAsset("assets/app-abc123.js")
	if first == nil || second == nil {
		t.Fatal("gzipAsset returned nil")
	}
	if &first[0] != &second[0] {
		t.Fatal("second call did not reuse the cached bytes (cache lookup bypassed)")
	}
	if s.gzipAsset("assets/missing.js") != nil {
		t.Fatal("missing file must yield nil so the caller sends it raw")
	}
}

// TestServeSPAHeadMatchesGetHeaders HEAD 与 GET 回同样的头（Content-Length = 将要发送的
// 字节数），不回响应体——前端探测入口脚本是否还在就靠它。
func TestServeSPAHeadMatchesGetHeaders(t *testing.T) {
	s := useSPAFixture(t)
	for _, encoding := range []string{"gzip", ""} {
		get := spaDo(s, http.MethodGet, "/assets/app-abc123.js", encoding, nil)
		head := spaDo(s, http.MethodHead, "/assets/app-abc123.js", encoding, nil)
		if head.Code != http.StatusOK {
			t.Fatalf("HEAD status = %d, want 200", head.Code)
		}
		if head.Body.Len() != 0 {
			t.Fatalf("HEAD must not write a body, got %d bytes", head.Body.Len())
		}
		for _, h := range []string{"Content-Type", "Content-Length", "Content-Encoding", "Cache-Control", "Vary"} {
			if head.Header().Get(h) != get.Header().Get(h) {
				t.Fatalf("HEAD %s = %q, GET = %q", h, head.Header().Get(h), get.Header().Get(h))
			}
		}
		if head.Header().Get("Content-Length") != strconv.Itoa(get.Body.Len()) {
			t.Fatalf("HEAD Content-Length = %q, want %d", head.Header().Get("Content-Length"), get.Body.Len())
		}
	}
}

// TestServeSPAMissingAssetIs404 `/assets/` 下不存在的文件回标准 404，不落到 SPA 兜底
// （兜底会把 index.html 当脚本发出去）；其它路径的深链兜底不变。
func TestServeSPAMissingAssetIs404(t *testing.T) {
	s := useSPAFixture(t)
	html := map[string]string{"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"}
	cases := []struct {
		name       string
		method     string
		target     string
		encoding   string
		extra      map[string]string
		wantStatus int
		wantIndex  bool // 回 index.html
		wantNoBody bool // 标准 404：既不是 index 也不是包络
	}{
		{"stale hashed chunk", http.MethodGet, "/assets/old-000000.js", "gzip", nil, http.StatusNotFound, false, true},
		{"stale chunk asked as html", http.MethodGet, "/assets/old-000000.js", "gzip", html, http.StatusNotFound, false, true},
		{"stale stylesheet", http.MethodGet, "/assets/old-000000.css", "gzip", nil, http.StatusNotFound, false, true},
		{"missing nested asset", http.MethodGet, "/assets/nested/old-000000.js", "gzip", nil, http.StatusNotFound, false, true},
		{"HEAD stale chunk (the probe)", http.MethodHead, "/assets/old-000000.js", "gzip", nil, http.StatusNotFound, false, true},
		{"deep link /sites", http.MethodGet, "/sites", "", html, http.StatusOK, true, false},
		{"deep link /login", http.MethodGet, "/login", "", html, http.StatusOK, true, false},
		{"deep link with params", http.MethodGet, "/databases/mysql/3/browse/db", "", html, http.StatusOK, true, false},
		{"root", http.MethodGet, "/", "", html, http.StatusOK, true, false},
		{"missing non-asset file keeps fallback", http.MethodGet, "/favicon.ico", "", nil, http.StatusOK, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := spaDo(s, tc.method, tc.target, tc.encoding, tc.extra)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tc.wantStatus, w.Body.String())
			}
			body := w.Body.String()
			hasIndex := strings.Contains(body, "<!doctype html")
			if tc.wantIndex && !hasIndex {
				t.Fatalf("want index.html fallback, got %q", body)
			}
			if tc.wantNoBody {
				if hasIndex || strings.Contains(body, `"code"`) || strings.Contains(body, "NOT_FOUND") {
					t.Fatalf("missing asset must be a plain 404, got %q", body)
				}
				if cc := w.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
					t.Fatalf("404 must not carry the immutable cache header, got %q", cc)
				}
			}
		})
	}
}

// TestServeSPAIndexNeverLongCached index.html 永远 no-cache、不压缩（小于阈值）、不带 Vary。
func TestServeSPAIndexNeverLongCached(t *testing.T) {
	s := useSPAFixture(t)
	for _, target := range []string{"/", "/sites", "/login"} {
		w := spaDo(s, http.MethodGet, target, "gzip", map[string]string{"Accept": "text/html"})
		if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Fatalf("GET %s Cache-Control = %q, want no-cache", target, cc)
		}
		if w.Header().Get("Vary") != "" || w.Header().Get("Content-Encoding") != "" {
			t.Fatalf("GET %s: entry document must stay uncompressed and Vary-free", target)
		}
		if w.Body.String() != spaFixtureIndex {
			t.Fatalf("GET %s: body is not index.html", target)
		}
	}
}

// TestServeSPARangeIgnoredFull200 沿用现状：忽略 Range，回 200 全量（不声称支持断点续传）。
func TestServeSPARangeIgnoredFull200(t *testing.T) {
	s := useSPAFixture(t)
	w := spaDo(s, http.MethodGet, "/assets/app-abc123.js", "", map[string]string{"Range": "bytes=0-9"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != spaFixtureJS {
		t.Fatalf("Range request must still get the whole file, got %d bytes", w.Body.Len())
	}
	if w.Header().Get("Content-Range") != "" || w.Header().Get("Accept-Ranges") != "" {
		t.Fatalf("must not advertise range support: %v", w.Header())
	}
}

// TestAcceptsGzip 判定函数的边界（传输层用例之外的补充）。
func TestAcceptsGzip(t *testing.T) {
	cases := map[string]bool{
		"gzip":                       true,
		"GZip":                       true,
		"gzip;q=0.001":               true,
		"gzip;q=0":                   false,
		"gzip;q=0, *":                false,
		"*, gzip;q=0":                false,
		"*":                          true,
		"*;q=0":                      false,
		"*;q=0.2":                    true,
		"br, zstd":                   false,
		"":                           false,
		"gzip;level=9;q=0.5":         true,
		"gzip;q=bogus":               true,
		"gzip ; q=0":                 false,
		"deflate, gzip;q=1.0, *;q=0": true,
	}
	for header, want := range cases {
		if got := acceptsGzip(header); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}
