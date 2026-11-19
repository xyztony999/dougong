// v1.3 内部端点豁免回归（api-db-tools D-21/4.4.5 验收五条）：绑域名后
// 内部 gate/signon 端点不被 HostGuard/EntryPathGuard 误拦；非内部路径强
// 校验不变（fail-closed）；豁免范围最小化；链序 Entry → Host 不变。
package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// exemptNext 记录到达引擎的规范路径（断言剥前缀语义）。
func exemptNext(seen *string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
}

func doReqPath(h http.Handler, path, host string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if host != "" {
		req.Host = host
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestInternalExemptHostGuard HostGuard 豁免（D-21②）：内部 2 端点前缀
// 直通（Host≠绑定域名含 IP 直连不 404）；非内部路径强校验不变；豁免集合
// 最小化（前缀外路径不直通）。
func TestInternalExemptHostGuard(t *testing.T) {
	var seen string
	h := HostGuard(fakeHostSource{domain: "panel.example.com"}, exemptNext(&seen))
	cases := []struct {
		name, path, host string
		want             int
	}{
		{"gate/verify 豁免（IP 直连 Host）", "/dbtool/gate/verify", "127.0.0.1:8687", 200},
		{"signon/verify 豁免（IP 直连 Host）", "/dbtool/signon/verify", "10.0.0.8:8687", 200},
		{"gate 子路径豁免（前缀匹配含子路径）", "/dbtool/gate/sub", "other.example.com", 200},
		{"非内部 tool/list 强校验不变", "/dbtool/tool/list", "127.0.0.1:8687", 404},
		{"非内部 access/open 强校验不变", "/dbtool/access/open", "127.0.0.1:8687", 404},
		{"非内部 access/reopen 强校验不变", "/dbtool/access/reopen", "127.0.0.1:8687", 404},
		{"最小化：gate 无尾斜杠不豁免", "/dbtool/gate", "127.0.0.1:8687", 404},
		{"最小化：signon 无尾斜杠不豁免", "/dbtool/signon", "127.0.0.1:8687", 404},
		{"最小化：前缀近似不豁免", "/dbtool/gateX/verify", "127.0.0.1:8687", 404},
		{"最小化：dbtool 裸段不豁免", "/dbtool", "127.0.0.1:8687", 404},
		{"面板路径强校验不变", "/panel/config/detail", "127.0.0.1:8687", 404},
		{"匹配 Host非内部照常放行", "/dbtool/tool/list", "panel.example.com", 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seen = ""
			rec := doReqPath(h, tc.path, tc.host)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if tc.want == 200 && seen != tc.path {
				t.Fatalf("HostGuard 不得改写路径: seen=%q", seen)
			}
			if tc.want == 404 && rec.Body.String() != "404 page not found\n" {
				t.Fatalf("404 须为标准 net/http plain（无业务码包裹）, got %q", rec.Body.String())
			}
		})
	}
}

// TestInternalExemptHostGuardReadFailure 读态失败 fail-closed 不变——
// 内部端点豁免前置不受读态影响（其安全由 loopback+秘密头+会话/令牌承载，
// 非 Host 校验）；非内部路径仍 fail-closed 404。
func TestInternalExemptHostGuardReadFailure(t *testing.T) {
	var seen string
	h := HostGuard(fakeHostSource{err: errors.New("db down")}, exemptNext(&seen))
	if rec := doReqPath(h, "/dbtool/gate/verify", "127.0.0.1:8687"); rec.Code != 200 {
		t.Fatalf("内部端点豁免前置（读态失败不阻断）: %d", rec.Code)
	}
	if rec := doReqPath(h, "/panel/config/detail", "panel.example.com"); rec.Code != 404 {
		t.Fatalf("非内部读态失败 fail-closed 不变: %d", rec.Code)
	}
}

// TestInternalExemptEntryPath EntryPathHandler 豁免（D-21③）：携入口
// 前缀者剥前缀放行（既有语义）；未携前缀但直接命中豁免前缀者亦直通
// （防御性兜底）；非内部路径未携/错携前缀仍标准 404。
func TestInternalExemptEntryPath(t *testing.T) {
	var seen string
	h := EntryPathHandler(stubEntrySource{enabled: true, path: "s3cret-entry"}, exemptNext(&seen))
	cases := []struct {
		name, path, wantPath string
		want                 int
	}{
		{"内部携前缀剥前缀放行", "/s3cret-entry/dbtool/gate/verify", "/dbtool/gate/verify", 200},
		{"内部携前缀 signon 剥前缀放行", "/s3cret-entry/dbtool/signon/verify", "/dbtool/signon/verify", 200},
		{"内部未携前缀直通（防御性兜底）", "/dbtool/gate/verify", "/dbtool/gate/verify", 200},
		{"内部未携前缀 signon 直通", "/dbtool/signon/verify", "/dbtool/signon/verify", 200},
		{"非内部未携前缀仍404", "/dbtool/tool/list", "", 404},
		{"非内部错携前缀仍404", "/wrong/dbtool/gate/verify", "", 404},
		{"非内部携前缀照常剥前缀", "/s3cret-entry/dbtool/tool/list", "/dbtool/tool/list", 200},
		{"入口根语义不变", "/s3cret-entry", "/", 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seen = ""
			rec := doReqPath(h, tc.path, "127.0.0.1:8687")
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if tc.want == 200 && seen != tc.wantPath {
				t.Fatalf("规范路径 = %q, want %q", seen, tc.wantPath)
			}
		})
	}
	// 读态失败：内部未携前缀豁免前置不受影响；非内部 fail-closed 404。
	hErr := EntryPathHandler(stubEntrySourceErr{}, exemptNext(&seen))
	if rec := doReqPath(hErr, "/dbtool/signon/verify", ""); rec.Code != 200 {
		t.Fatalf("内部端点读态失败豁免前置: %d", rec.Code)
	}
	if rec := doReqPath(hErr, "/dbtool/tool/list", ""); rec.Code != 404 {
		t.Fatalf("非内部读态失败 fail-closed 不变: %d", rec.Code)
	}
}

// stubEntrySourceErr EntryPathSource 读态失败桩。
type stubEntrySourceErr struct{}

func (stubEntrySourceErr) EntryPathState(ctx context.Context) (bool, string, error) {
	return false, "", errors.New("db down")
}

// TestInternalExemptChainOrder Entry → Host 链序豁免协同（D-21 链序不变）：
// 绑域名+入口开启——内部端点携/未携前缀且 Host≠域名均可达；非内部路径
// 强校验不变（Host 不匹配仍 404）。
func TestInternalExemptChainOrder(t *testing.T) {
	var seen string
	src := struct {
		EntryPathSource
		HostGuardSource
	}{
		EntryPathSource: stubEntrySource{enabled: true, path: "s3cret-entry"},
		HostGuardSource: fakeHostSource{domain: "panel.example.com"},
	}
	h := EntryPathHandler(src, HostGuard(src, exemptNext(&seen)))
	cases := []struct {
		name, path, host string
		want             int
	}{
		{"内部未携前缀+IP Host 可达", "/dbtool/gate/verify", "127.0.0.1:8687", 200},
		{"内部携前缀+IP Host 可达", "/s3cret-entry/dbtool/signon/verify", "127.0.0.1:8687", 200},
		{"非内部携前缀+IP Host 仍404", "/s3cret-entry/dbtool/tool/list", "127.0.0.1:8687", 404},
		{"非内部未携前缀仍404", "/dbtool/tool/list", "panel.example.com", 404},
		{"非内部携前缀+匹配Host放行", "/s3cret-entry/dbtool/tool/list", "panel.example.com", 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doReqPath(h, tc.path, tc.host)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
