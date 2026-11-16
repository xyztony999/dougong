package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeHostSource HostGuard 数据源桩（绑定值/读态错误可注入）。
type fakeHostSource struct {
	domain string
	err    error
}

func (f fakeHostSource) BindDomainState(context.Context) (string, error) {
	return f.domain, f.err
}

func doReq(h http.Handler, host string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/panel/config/detail", nil)
	if host != "" {
		req.Host = host
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestHostGuard 表驱动：未绑定直通 / 匹配放行（大小写与可选端口）/
// 不匹配（含 IP 直连）标准 404 无业务码 / 读态失败 fail-closed 404（I-P09/D-P16）。
func TestHostGuard(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PASSED"))
	})
	cases := []struct {
		name   string
		source HostGuardSource
		host   string
		want   int
		wantBy string
	}{
		{"未绑定放行全部 Host", fakeHostSource{domain: ""}, "10.0.0.8:8080", 200, "PASSED"},
		{"精确匹配放行", fakeHostSource{domain: "panel.example.com"}, "panel.example.com", 200, "PASSED"},
		{"匹配大小写不敏感", fakeHostSource{domain: "panel.example.com"}, "PANEL.Example.COM", 200, "PASSED"},
		{"匹配忽略可选端口", fakeHostSource{domain: "panel.example.com"}, "panel.example.com:8443", 200, "PASSED"},
		{"不匹配他域名标准404", fakeHostSource{domain: "panel.example.com"}, "other.example.com", 404, ""},
		{"IP 直连标准404", fakeHostSource{domain: "panel.example.com"}, "127.0.0.1:8080", 404, ""},
		{"www 变体不自动匹配", fakeHostSource{domain: "panel.example.com"}, "www.panel.example.com", 404, ""},
		{"空 Host 头不匹配", fakeHostSource{domain: "panel.example.com"}, "", 404, ""},
		{"读态失败 fail-closed 404", fakeHostSource{err: errors.New("db down")}, "panel.example.com", 404, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doReq(HostGuard(tc.source, next), tc.host)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if tc.wantBy != "" && rec.Body.String() != tc.wantBy {
				t.Fatalf("body = %q, want %q", rec.Body.String(), tc.wantBy)
			}
			if tc.want == 404 && rec.Body.String() != "404 page not found\n" {
				t.Fatalf("404 must be standard net/http plain (无业务码包裹), got %q", rec.Body.String())
			}
		})
	}
}

// TestHostGuardStripPort stripPort 端口剥离（IPv6 字面量/无端口原样）。
func TestHostGuardStripPort(t *testing.T) {
	cases := []struct{ in, want string }{
		{"panel.example.com", "panel.example.com"},
		{"panel.example.com:8080", "panel.example.com"},
		{"127.0.0.1:8080", "127.0.0.1"},
		{"[::1]:8080", "::1"},
		{"[2001:db8::1]", "[2001:db8::1]"}, // 无端口：SplitHostPort 失败原样返回
	}
	for _, tc := range cases {
		if got := stripPort(tc.in); got != tc.want {
			t.Fatalf("stripPort(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestHostGuardChainOrder Entry → Host 链序：HostGuard 在 EntryPathHandler
// 之后（正确前缀 + 匹配 Host 才到达引擎；D-P16 中间件链顺序约束）。
func TestHostGuardChainOrder(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	src := struct {
		EntryPathSource
		HostGuardSource
	}{
		EntryPathSource: stubEntrySource{enabled: true, path: "s3cret-entry"},
		HostGuardSource: fakeHostSource{domain: "panel.example.com"},
	}
	h := EntryPathHandler(src, HostGuard(src, next))
	if rec := doReq(h, "panel.example.com"); rec.Code != 404 {
		t.Fatalf("正确 Host 未携带入口前缀应 404（Entry 先行），got %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/s3cret-entry/panel/config/detail", nil)
	req.Host = "other.example.com"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("正确前缀但 Host 不匹配应 404（Host 随后），got %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/s3cret-entry/panel/config/detail", nil)
	req.Host = "panel.example.com"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("前缀与 Host 均正确应放行，got %d", rec.Code)
	}
}

// stubEntrySource EntryPathHandler 数据源桩。
type stubEntrySource struct {
	enabled bool
	path    string
}

func (s stubEntrySource) EntryPathState(context.Context) (bool, string, error) {
	return s.enabled, s.path, nil
}
