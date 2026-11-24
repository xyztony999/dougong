package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/dougong/dougong/internal/api/middleware"
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
	spaFixtureIndex = "<!doctype html>\n<html lang=\"zh-CN\">\n  <head>\n    <meta charset=\"UTF-8\" />\n  </head>\n  <body><div id=\"app\"></div><script type=\"module\" src=\"./assets/app-abc123.js\"></script></body>\n</html>"
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
		// 未注入入口状态来源时基址恒为 /：正文 = 原 index.html 加上 <base href="/">
		if want := string(spaIndexWithBase([]byte(spaFixtureIndex), "/")); w.Body.String() != want {
			t.Fatalf("GET %s: body is not index.html with base /", target)
		}
	}
}

// fakeEntrySource 是安全入口状态来源的替身。
type fakeEntrySource struct {
	enabled bool
	path    string
	err     error
}

func (f fakeEntrySource) EntryPathState(context.Context) (bool, string, error) {
	return f.enabled, f.path, f.err
}

// TestServeSPAEntryBaseInjection 入口 HTML 按安全入口状态注入 <base>（api D-P03）：
// 页面里的相对地址据此落在入口路径之下——注入错了，开着安全入口的面板在浏览器里就是一片空白。
func TestServeSPAEntryBaseInjection(t *testing.T) {
	cases := []struct {
		name    string
		source  *fakeEntrySource
		wantTag string
	}{
		{"no source: base is /", nil, `<base href="/">`},
		{"entry disabled: base is /", &fakeEntrySource{enabled: false, path: "abc12345"}, `<base href="/">`},
		{"entry enabled without a path: base is /", &fakeEntrySource{enabled: true, path: ""}, `<base href="/">`},
		{"entry enabled: base is /{entryPath}/", &fakeEntrySource{enabled: true, path: "abc12345"}, `<base href="/abc12345/">`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := useSPAFixture(t)
			if tc.source != nil {
				s.SetEntryPathSource(*tc.source)
			}
			for _, target := range []string{"/", "/sites", "/databases/mysql/3/browse/db", "/login"} {
				w := spaDo(s, http.MethodGet, target, "gzip", map[string]string{"Accept": "text/html"})
				body := w.Body.String()
				if w.Code != http.StatusOK {
					t.Fatalf("GET %s status = %d, want 200", target, w.Code)
				}
				if !strings.Contains(body, tc.wantTag) {
					t.Fatalf("GET %s: base tag %q missing from:\n%s", target, tc.wantTag, body)
				}
				// 注入位置：<head> 之后、入口脚本之前；除此之外正文原样
				head := strings.Index(body, "<head>")
				base := strings.Index(body, "<base")
				script := strings.Index(body, "<script")
				if !(head >= 0 && base > head && script > base) {
					t.Fatalf("GET %s: base tag must sit right after <head> and before the entry script:\n%s", target, body)
				}
				want := string(spaIndexWithBase([]byte(spaFixtureIndex), strings.TrimSuffix(strings.TrimPrefix(tc.wantTag, `<base href="`), `">`)))
				if body != want {
					t.Fatalf("GET %s: body differs from index.html plus the base tag", target)
				}
				if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
					t.Fatalf("GET %s Cache-Control = %q, want no-cache (the base follows the entry setting)", target, cc)
				}
				if cl := w.Header().Get("Content-Length"); cl != strconv.Itoa(len(body)) {
					t.Fatalf("GET %s Content-Length = %q, want %d (length of the document actually sent)", target, cl, len(body))
				}
			}
			// HEAD 与 GET 同头：Content-Length 是注入后的长度，不回响应体
			get := spaDo(s, http.MethodGet, "/", "", nil)
			head := spaDo(s, http.MethodHead, "/", "", nil)
			if head.Body.Len() != 0 {
				t.Fatalf("HEAD must not return a body: %d bytes", head.Body.Len())
			}
			if cl, want := head.Header().Get("Content-Length"), get.Header().Get("Content-Length"); cl == "" || cl != want {
				t.Fatalf("HEAD Content-Length = %q, want the GET value %q", cl, want)
			}
		})
	}
}

// TestServeSPAEntryBaseFailsClosed 读不到安全入口状态时，入口文档回标准 404（与入口网关同口径），
// 不能带着错误的基址发出去；静态文件不受影响。
func TestServeSPAEntryBaseFailsClosed(t *testing.T) {
	s := useSPAFixture(t)
	s.SetEntryPathSource(fakeEntrySource{err: errors.New("state unreadable")})
	w := spaDo(s, http.MethodGet, "/", "", map[string]string{"Accept": "text/html"})
	if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "<base") {
		t.Fatalf("status = %d body %q, want a plain 404 without the document", w.Code, w.Body.String())
	}
	if a := spaDo(s, http.MethodGet, "/assets/app-abc123.js", "gzip", nil); a.Code != http.StatusOK {
		t.Fatalf("static assets must not depend on the entry state: status = %d", a.Code)
	}
}

// TestEntryGatewayAndBaseAgree 入口网关和入口文档必须对得上：文档里的 <base> 加上页面里的
// 相对地址，得到的每一个地址都要能通过同一个网关。基址少了结尾斜杠（/abc 而不是 /abc/）时，
// 浏览器会把 ./assets/x.js 解析成不带前缀的 /assets/x.js，被网关拦掉——开着安全入口就是白屏。
func TestEntryGatewayAndBaseAgree(t *testing.T) {
	s := useSPAFixture(t)
	src := fakeEntrySource{enabled: true, path: "abc12345"}
	s.SetEntryPathSource(src)
	gate := middleware.EntryPathHandler(src, s.engine)
	entryPrefix := "/abc12345/"
	do := func(target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		gate.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		return w
	}
	page := do(entryPrefix)
	if page.Code != http.StatusOK {
		t.Fatalf("GET the entry address: status = %d, want 200", page.Code)
	}
	m := regexp.MustCompile(`<base href="([^"]*)">`).FindStringSubmatch(page.Body.String())
	if m == nil {
		t.Fatalf("the entry document has no base tag:\n%s", page.Body.String())
	}
	base, err := url.Parse(html.UnescapeString(m[1]))
	if err != nil {
		t.Fatalf("base %q is not a valid address: %v", m[1], err)
	}
	refs := regexp.MustCompile(`(?:src|href)="([^"]+)"`).FindAllStringSubmatch(page.Body.String(), -1)
	if len(refs) == 0 {
		t.Fatal("the fixture index.html must reference at least one file")
	}
	for _, ref := range refs {
		rel, err := url.Parse(html.UnescapeString(ref[1]))
		if err != nil {
			t.Fatalf("reference %q is not a valid address: %v", ref[1], err)
		}
		resolved := base.ResolveReference(rel)
		target := resolved.RequestURI()
		if !strings.HasPrefix(target, entryPrefix) {
			t.Fatalf("%s resolves to %q under base %s: not under the entry path", ref[1], target, base)
		}
		if w := do(target); w.Code != http.StatusOK {
			t.Fatalf("%s resolves to %s: the gateway answered %d, want 200", ref[1], target, w.Code)
		}
	}
	// 同一个文件不带入口前缀：网关照旧拦掉
	if w := do("/assets/app-abc123.js"); w.Code != http.StatusNotFound {
		t.Fatalf("asset without the entry prefix: status = %d, want 404", w.Code)
	}
}

// TestSpaIndexWithBase 纯函数的边界。
func TestSpaIndexWithBase(t *testing.T) {
	raw := []byte("<!doctype html><HTML><HEAD><TITLE>x</TITLE></HEAD><BODY></BODY></HTML>")
	got := string(spaIndexWithBase(raw, "/e/"))
	want := "<!doctype html><HTML><HEAD><base href=\"/e/\"><TITLE>x</TITLE></HEAD><BODY></BODY></HTML>"
	if got != want {
		t.Fatalf("case-insensitive head match:\n got %s\nwant %s", got, want)
	}
	if string(spaIndexWithBase([]byte("<!doctype html><html><body></body></html>"), "/e/")) != "<!doctype html><html><body></body></html>" {
		t.Fatal("no <head>: return the document untouched")
	}
	if string(spaIndexWithBase([]byte("<html><head"), "/e/")) != "<html><head" {
		t.Fatal("unterminated <head: return the document untouched")
	}
	if out := spaIndexWithBase(raw, `/"x`); !strings.Contains(string(out), `<base href="/&#34;x">`) {
		t.Fatalf("base must be HTML-escaped: %s", out)
	}
	// 真正核对输入没被改动：调用前留一份副本，调用后逐字节比对
	before := append([]byte(nil), raw...)
	spaIndexWithBase(raw, "/e/")
	if !bytes.Equal(raw, before) {
		t.Fatalf("input must not be modified in place: %s", raw)
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
