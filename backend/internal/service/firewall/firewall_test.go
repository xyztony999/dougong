package firewall

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/firewallx"
	"github.com/dougong/dougong/internal/adapter/netlisten"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
)

// fakeGateway 是 FirewallGateway 进程内替身（CLI 交互不出本模块单测边界）。
type fakeGateway struct {
	mu        sync.Mutex
	detect    firewallx.BackendInfo
	detectErr error
	detectN   int
	list      []firewallx.BackendRule
	listErr   error
	listN     int
	addRes    firewallx.AddResult
	addErr    error
	addCalls  []firewallx.RuleSpec
	delRes    firewallx.DelResult
	delErr    error
	delCalls  []string
}

func (f *fakeGateway) Detect(context.Context) (firewallx.BackendInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.detectN++
	return f.detect, f.detectErr
}

func (f *fakeGateway) ListRules(context.Context) ([]firewallx.BackendRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listN++
	return f.list, f.listErr
}

func (f *fakeGateway) AddRule(_ context.Context, spec firewallx.RuleSpec) (firewallx.AddResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addCalls = append(f.addCalls, spec)
	return f.addRes, f.addErr
}

func (f *fakeGateway) DeleteRule(_ context.Context, ruleKey string) (firewallx.DelResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delCalls = append(f.delCalls, ruleKey)
	return f.delRes, f.delErr
}

func (f *fakeGateway) writes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.addCalls) + len(f.delCalls)
}

// fakeListen 是 ListenPortSource 替身。
type fakeListen struct {
	ports []netlisten.ListenPort
	err   error
}

func (f *fakeListen) List(context.Context) ([]netlisten.ListenPort, error) {
	return f.ports, f.err
}

// fakeRecorder 是 AuditRecorder 替身（failFirst>0 时前 N 次失败——D-09 重试）。
type fakeRecorder struct {
	mu        sync.Mutex
	entries   []audit.Entry
	failFirst int
}

func (r *fakeRecorder) Record(_ context.Context, e audit.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failFirst > 0 {
		r.failFirst--
		return errors.New("recorder boom")
	}
	r.entries = append(r.entries, e)
	return nil
}

func (r *fakeRecorder) snapshot() []audit.Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]audit.Entry(nil), r.entries...)
}

const testPanelPort = uint16(8443)

func newTestService(gw *fakeGateway, listen *fakeListen, rec *fakeRecorder) *Service {
	s := New(gw, listen, StaticPanelListen(testPanelPort), rec)
	s.clock = func() time.Time { return time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC) }
	return s
}

func activeUFW() firewallx.BackendInfo {
	v := "0.36.1"
	return firewallx.BackendInfo{Type: firewallx.BackendUFW, Active: true, Version: &v}
}

func activeFirewalld() firewallx.BackendInfo {
	v := "0.9.3"
	return firewallx.BackendInfo{Type: firewallx.BackendFirewalld, Active: true, Version: &v}
}

func rule(ruleKey, port, proto, action string, source *string) firewallx.BackendRule {
	return firewallx.BackendRule{RuleKey: ruleKey, Port: port, Protocol: proto, Action: action, Source: source, RawSpec: ruleKey}
}

func wantErr(t *testing.T, err error, symbol string) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want %s", symbol)
	}
	var ec *errcode.Error
	if !errors.As(err, &ec) || ec.Symbol() != symbol {
		t.Fatalf("err = %v, want %s", err, symbol)
	}
}

// ---- 校验与纯函数 ----

func TestSupportedActions(t *testing.T) {
	if got := SupportedActions(firewallx.BackendUFW); !reflect.DeepEqual(got, []string{"allow", "deny", "reject", "limit"}) {
		t.Fatalf("ufw supportedActions = %v（D1 口径-2 全四值）", got)
	}
	if got := SupportedActions(firewallx.BackendFirewalld); !reflect.DeepEqual(got, []string{"allow"}) {
		t.Fatalf("firewalld supportedActions = %v（仅 allow）", got)
	}
	if got := SupportedActions(firewallx.BackendNone); len(got) != 0 {
		t.Fatalf("none supportedActions = %v, want []", got)
	}
}

func TestValidatePortSpec(t *testing.T) {
	tests := []struct {
		port string
		ok   bool
	}{
		{"80", true}, {"1", true}, {"65535", true}, {"8080-8090", true}, {"8080-8080", true},
		{"0", false}, {"65536", false}, {"70000", false}, {"9000-100", false},
		{"80,443", false}, {"08", false}, {"", false}, {"abc", false}, {"80-", false}, {"-80", false},
	}
	for _, tt := range tests {
		err := validatePort(tt.port)
		if tt.ok && err != nil {
			t.Fatalf("validatePort(%q) = %v, want nil", tt.port, err)
		}
		if !tt.ok {
			wantErr(t, err, "FW_001")
		}
	}
}

func TestValidateSource(t *testing.T) {
	for _, src := range []string{"", "10.0.0.0/8", "192.168.1.0/24", "2001:db8::/32", "::/0"} {
		if err := validateSource(src); err != nil {
			t.Fatalf("validateSource(%q) = %v, want nil", src, err)
		}
	}
	for _, src := range []string{"10.0.0.5", "10.0.0.0/33", "abc", "10.0.0.0/"} {
		wantErr(t, validateSource(src), "FW_002")
	}
}

func TestParseEnumFilter(t *testing.T) {
	got, err := parseEnumFilter("allow,limit", validActions)
	if err != nil || len(got) != 2 || !got["allow"] || !got["limit"] {
		t.Fatalf("parseEnumFilter(allow,limit) = %v, %v", got, err)
	}
	if got, err := parseEnumFilter("", validActions); err != nil || got != nil {
		t.Fatalf("空筛选应不过滤: %v, %v", got, err)
	}
	for _, raw := range []string{"allow,bogus", "allow,", ",allow", "ALLOW"} {
		wantErr(t, func() error { _, err := parseEnumFilter(raw, validActions); return err }(), "FW_001")
	}
}

func TestPortSpansAndHelpers(t *testing.T) {
	if spans, ok := parseRulePortSpans("80"); !ok || len(spans) != 1 || spans[0] != (portSpan{80, 80}) {
		t.Fatalf("parseRulePortSpans(80) = %+v %v", spans, ok)
	}
	if spans, ok := parseRulePortSpans("80,443-445"); !ok || len(spans) != 2 {
		t.Fatalf("parseRulePortSpans(80,443-445) = %+v %v", spans, ok)
	}
	if _, ok := parseRulePortSpans("abc"); ok {
		t.Fatal("parseRulePortSpans(abc) 应不可解析")
	}
	if !spansCover([]portSpan{{8080, 8090}}, 8085) || spansCover([]portSpan{{8080, 8090}}, 8079) ||
		spansCover([]portSpan{{8080, 8090}}, 8091) {
		t.Fatal("spansCover 区间命中判定错位")
	}
	for addr, want := range map[string]bool{"127.0.0.1": false, "::1": false, "0.0.0.0": true, "::": true, "192.168.1.5": true, "garbage": true} {
		if got := isExternalAddr(addr); got != want {
			t.Fatalf("isExternalAddr(%q) = %v, want %v（D-04 loopback 口径）", addr, got, want)
		}
	}
	for proto, want := range map[string]string{"tcp": "tcp", "tcp6": "tcp", "udp": "udp", "udp6": "udp"} {
		if got := protoBase(proto); got != want {
			t.Fatalf("protoBase(%q) = %q, want %q", proto, got, want)
		}
	}
}

// ---- 读侧 ----

func TestBackendStatus(t *testing.T) {
	gw := &fakeGateway{detect: activeUFW()}
	s := newTestService(gw, &fakeListen{}, &fakeRecorder{})
	data, err := s.BackendStatus(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if data.Backend != "ufw" || !data.Active || data.Version == nil || *data.Version != "0.36.1" {
		t.Fatalf("status data = %+v", data)
	}
	if !reflect.DeepEqual(data.SupportedActions, []string{"allow", "deny", "reject", "limit"}) {
		t.Fatalf("supportedActions = %v（D-01 权威面）", data.SupportedActions)
	}
	if data.GeneratedAt != "2026-11-15T08:00:00Z" {
		t.Fatalf("generatedAt = %q（RFC3339 UTC）", data.GeneratedAt)
	}

	gw.detect = firewallx.BackendInfo{Type: firewallx.BackendNone}
	data, err = s.BackendStatus(context.Background())
	if err != nil || data.Backend != "none" || data.Active || data.Version != nil || len(data.SupportedActions) != 0 {
		t.Fatalf("none 探测结论 = %+v, %v（引导页数据形态，§4.1.4）", data, err)
	}

	gw.detectErr = errors.New("exec boom")
	_, err = s.BackendStatus(context.Background())
	wantErr(t, err, "FW_008")
	var ec *errcode.Error
	errors.As(err, &ec)
	if d, ok := ec.Data().(ReasonData); !ok || d.Reason == "" {
		t.Fatalf("FW_008 data.reason 须如实携带: %+v", ec.Data())
	}
}

func TestRuleListProjection(t *testing.T) {
	src := "10.0.0.0/8"
	gw := &fakeGateway{
		detect: activeUFW(),
		list: []firewallx.BackendRule{
			rule("deny 9000/tcp", "9000", "tcp", "deny", nil),
			rule("limit 22/tcp", "22", "tcp", "limit", nil),
			rule("allow 8443/tcp", "8443", "tcp", "allow", nil),
			rule("allow 8400-8500/tcp", "8400-8500", "tcp", "allow", nil),
			rule("allow 22/tcp", "22", "tcp", "allow", nil),
			rule("allow 8080-8090/udp from 10.0.0.0/8", "8080-8090", "udp", "allow", &src),
		},
	}
	s := newTestService(gw, &fakeListen{}, &fakeRecorder{})
	data, err := s.RuleList(context.Background(), "allow,limit", "tcp")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if data.Total != 4 || len(data.Items) != 4 {
		t.Fatalf("total = %d（action 筛选 deny 应剔除）: %+v", data.Total, data.Items)
	}
	// 排序（D-02：端口数值升序、同端口 ruleKey 字典序）：22/allow < 22/limit
	// < 8400 < 8443；udp 条目被 protocol=tcp 剔除。
	wantKeys := []string{"allow 22/tcp", "limit 22/tcp", "allow 8400-8500/tcp", "allow 8443/tcp"}
	for i, it := range data.Items {
		if it.RuleKey != wantKeys[i] {
			t.Fatalf("items[%d].ruleKey = %s, want %s（排序）", i, it.RuleKey, wantKeys[i])
		}
		if it.Direction != DirectionIngress {
			t.Fatalf("items[%d].direction = %s, want ingress（固定入站，D-05）", i, it.Direction)
		}
	}
	// isPanelPort 标注（D-06 从严：8443 直接命中与 8400-8500 区间含 8443）。
	if data.Items[0].IsPanelPort || data.Items[1].IsPanelPort {
		t.Fatal("22/tcp 两条不应标注 isPanelPort")
	}
	if !data.Items[2].IsPanelPort || !data.Items[3].IsPanelPort {
		t.Fatalf("面板端口命中标注缺失: %+v %+v", data.Items[2], data.Items[3])
	}
	if data.Backend.Backend != "ufw" || !data.Backend.Active {
		t.Fatalf("backend 摘要 = %+v（S-42-01 随行）", data.Backend)
	}
}

func TestRuleListGates(t *testing.T) {
	// 筛选值域先行（R-14）——网关零调用。
	gw := &fakeGateway{detect: activeUFW()}
	s := newTestService(gw, &fakeListen{}, &fakeRecorder{})
	_, err := s.RuleList(context.Background(), "bogus", "")
	wantErr(t, err, "FW_001")
	if gw.detectN != 0 {
		t.Fatal("非法筛选不应触网关（校验先行）")
	}
	// backend=none → FW_009（不返回空规则列表，I-01）。
	gw.detect = firewallx.BackendInfo{Type: firewallx.BackendNone}
	wantErr(t, func() error { _, err := s.RuleList(context.Background(), "", ""); return err }(), "FW_009")
	// 已识别不活跃 → FW_010（不返回过期数据）。
	gw.detect = firewallx.BackendInfo{Type: firewallx.BackendFirewalld, Active: false}
	wantErr(t, func() error { _, err := s.RuleList(context.Background(), "", ""); return err }(), "FW_010")
	// 通道失败 → FW_008 reason 如实。
	gw.detect = activeUFW()
	gw.listErr = errors.New("ufw status: exit 1: You need to be root")
	err = func() error { _, err := s.RuleList(context.Background(), "", ""); return err }()
	wantErr(t, err, "FW_008")
	var ec *errcode.Error
	errors.As(err, &ec)
	if d, ok := ec.Data().(ReasonData); !ok || d.Reason == "" {
		t.Fatalf("FW_008 data.reason: %+v", ec.Data())
	}
}

func TestPortListComparison(t *testing.T) {
	gw := &fakeGateway{
		detect: activeUFW(),
		list: []firewallx.BackendRule{
			rule("allow 8443/tcp", "8443", "tcp", "allow", nil),
			rule("limit 53/udp", "53", "udp", "limit", nil),
			rule("deny 9000/tcp", "9000", "tcp", "deny", nil), // deny 族不参与对照（D-04）
		},
	}
	listen := &fakeListen{ports: []netlisten.ListenPort{
		{Proto: "tcp", Addr: "0.0.0.0", Port: 8443, Pid: 1, Name: "dougong"},
		{Proto: "tcp6", Addr: "::", Port: 22},
		{Proto: "udp", Addr: "127.0.0.1", Port: 53},
		{Proto: "tcp6", Addr: "::1", Port: 8443, Pid: 4},
	}}
	s := newTestService(gw, listen, &fakeRecorder{})
	data, err := s.PortList(context.Background())
	if err != nil {
		t.Fatalf("port/list: %v", err)
	}
	if data.Total != 4 || len(data.Items) != 4 { // 4 个监听套接字实况全量呈现
		t.Fatalf("total = %d items = %+v", data.Total, data.Items)
	}
	it := data.Items[0] // 0.0.0.0:8443
	if !it.External || it.OpenState != OpenAllowedByRule || it.Process == nil ||
		it.Process.Pid == nil || *it.Process.Pid != 1 || it.Process.Name == nil || *it.Process.Name != "dougong" {
		t.Fatalf("0.0.0.0:8443 对照错位: %+v（对外+放行+进程）", it)
	}
	if it := data.Items[1]; it.OpenState != OpenNoAllowRule || !it.External || it.Process != nil {
		t.Fatalf("[::]:22 对照错位: %+v（无放行规则+对外+进程 null 不虚构）", it)
	}
	if it := data.Items[2]; it.External || it.OpenState != OpenAllowedByRule {
		t.Fatalf("127.0.0.1:53 对照错位: %+v（loopback 非对外；limit=allow 族命中 udp）", it)
	}
	if it := data.Items[3]; it.External || it.OpenState != OpenAllowedByRule || it.Process == nil ||
		it.Process.Pid == nil || *it.Process.Pid != 4 || it.Process.Name != nil {
		t.Fatalf("[::1]:8443 对照错位: %+v（loopback+tcp6→tcp 族命中+pid 有 name null）", it)
	}
}

// ---- 写侧：create ----

func TestCreateValidationChain(t *testing.T) {
	tests := []struct {
		name    string
		detect  firewallx.BackendInfo
		req     CreateParams
		wantSym string
	}{
		{"bad port", activeUFW(), CreateParams{Port: "0", Protocol: "tcp", Action: "allow"}, "FW_001"},
		{"bad protocol", activeUFW(), CreateParams{Port: "80", Protocol: "icmp", Action: "allow"}, "FW_001"},
		{"bad action", activeUFW(), CreateParams{Port: "80", Protocol: "tcp", Action: "block"}, "FW_001"},
		{"bad source", activeUFW(), CreateParams{Port: "80", Protocol: "tcp", Action: "allow", Source: "10.0.0.5"}, "FW_002"},
		{"no backend", firewallx.BackendInfo{Type: firewallx.BackendNone}, CreateParams{Port: "80", Protocol: "tcp", Action: "allow"}, "FW_009"},
		{"not active", firewallx.BackendInfo{Type: firewallx.BackendUFW, Active: false}, CreateParams{Port: "80", Protocol: "tcp", Action: "allow"}, "FW_010"},
		{"action not supported", activeFirewalld(), CreateParams{Port: "80", Protocol: "tcp", Action: "deny"}, "FW_004"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &fakeGateway{detect: tt.detect}
			rec := &fakeRecorder{}
			s := newTestService(gw, &fakeListen{}, rec)
			_, err := s.CreateRule(context.Background(), nil, tt.req)
			wantErr(t, err, tt.wantSym)
			if gw.writes() != 0 {
				t.Fatalf("校验拒绝不应触写命令")
			}
			entries := rec.snapshot()
			if len(entries) != 1 || entries[0].Result != audit.ResultFailure {
				t.Fatalf("拒绝审计（A-W11 除 FW_005 外恰好一条 failure）: %+v", entries)
			}
			if d, ok := entries[0].Detail.(map[string]any); !ok || d["error"] != tt.wantSym {
				t.Fatalf("审计 detail.error = %+v, want %s", entries[0].Detail, tt.wantSym)
			}
		})
	}
}

func TestCreatePanelPortGuardAndSuccess(t *testing.T) {
	gw := &fakeGateway{detect: activeUFW()}
	rec := &fakeRecorder{}
	s := newTestService(gw, &fakeListen{}, rec)

	// deny 命中面板端口且未确认 → FW_005 + data 命中详情 + 零审计（I-05/I-07）。
	_, err := s.CreateRule(context.Background(), nil, CreateParams{Port: "8443", Protocol: "tcp", Action: "deny"})
	wantErr(t, err, "FW_005")
	var ec *errcode.Error
	errors.As(err, &ec)
	if d, ok := ec.Data().(DangerData); !ok || d.PanelPort != 8443 || d.RulePort != "8443" ||
		d.Action != "deny" || d.Hint == "" {
		t.Fatalf("FW_005 data 命中详情错位: %+v", ec.Data())
	}
	if gw.writes() != 0 || len(rec.snapshot()) != 0 {
		t.Fatalf("FW_005 拒绝未发生变更：零写命令零审计（I-07 明文豁免）")
	}
	// 区间含面板端口同触发（D-06 从严）。
	_, err = s.CreateRule(context.Background(), nil, CreateParams{Port: "8000-9000", Protocol: "udp", Action: "reject"})
	wantErr(t, err, "FW_005")
	// deny 不命中面板端口：无需确认直接执行。
	gw.addRes = firewallx.AddResult{RuleKey: "deny 9000/tcp", PermanentOK: true, RuntimeOK: true}
	data, err := s.CreateRule(context.Background(), nil, CreateParams{Port: "9000", Protocol: "tcp", Action: "deny"})
	if err != nil || data.RuleKey != "deny 9000/tcp" || data.Port != "9000" {
		t.Fatalf("deny 9000 应直通: %+v %v", data, err)
	}
	// 确认后放行（保护不阻止操作本身，I-05）。
	gw.mu.Lock()
	gw.addRes = firewallx.AddResult{RuleKey: "deny 8443/tcp", PermanentOK: true, RuntimeOK: true}
	gw.mu.Unlock()
	data, err = s.CreateRule(context.Background(), nil, CreateParams{Port: "8443", Protocol: "tcp", Action: "deny", ConfirmDanger: true})
	if err != nil || data.Action != "deny" {
		t.Fatalf("confirmDanger 后应放行: %+v %v", data, err)
	}
	if calls := gw.addCallsSnapshot(); len(calls) != 2 {
		t.Fatalf("add calls = %d, want 2", len(calls))
	}
	if spec := gw.addCallsSnapshot()[1]; spec.Port != "8443" || spec.Protocol != "tcp" || spec.Action != "deny" {
		t.Fatalf("AddRule spec = %+v", spec)
	}
	// 恰好两条 success 审计（I-07），object_id=写入后 ruleKey。
	entries := rec.snapshot()
	if len(entries) != 2 {
		t.Fatalf("audit entries = %d, want 2", len(entries))
	}
	for _, e := range entries {
		if e.Action != AuditActionRuleCreated || e.Result != audit.ResultSuccess ||
			e.ObjectType != ObjectTypeFirewallRule || e.Category != audit.CategoryChange {
			t.Fatalf("success 审计错位: %+v", e)
		}
	}
	if entries[1].ObjectID != "deny 8443/tcp" {
		t.Fatalf("success 审计 object_id = %q, want 写入后 ruleKey", entries[1].ObjectID)
	}
	if d, ok := entries[0].Detail.(map[string]any); !ok || d["confirmDanger"] != false || d["port"] != "9000" {
		t.Fatalf("审计 detail 契约字段（port/protocol/source/action/confirmDanger）: %+v", entries[0].Detail)
	}
}

func TestCreateGatewaySemantics(t *testing.T) {
	tests := []struct {
		name    string
		addErr  error
		addRes  firewallx.AddResult
		wantSym string
	}{
		{"already exists", firewallx.ErrAlreadyExists, firewallx.AddResult{}, "FW_006"},
		{"runtime apply failed", nil, firewallx.AddResult{RuleKey: "80/tcp", PermanentOK: true}, "FW_007"},
		{"exec failed", errors.New("ufw allow: exit 1: not root"), firewallx.AddResult{}, "FW_008"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &fakeGateway{detect: activeUFW(), addErr: tt.addErr, addRes: tt.addRes}
			rec := &fakeRecorder{}
			s := newTestService(gw, &fakeListen{}, rec)
			_, err := s.CreateRule(context.Background(), nil, CreateParams{Port: "80", Protocol: "tcp", Action: "allow"})
			wantErr(t, err, tt.wantSym)
			var ec *errcode.Error
			errors.As(err, &ec)
			if tt.wantSym == "FW_007" {
				if d, ok := ec.Data().(DualLayerData); !ok || d.Permanent != LayerWritten || d.Runtime != LayerFailed {
					t.Fatalf("FW_007 data 两层状态错位: %+v（D-08）", ec.Data())
				}
			}
			if entries := rec.snapshot(); len(entries) != 1 || entries[0].Result != audit.ResultFailure {
				t.Fatalf("failure 审计恰好一条（I-07）: %+v", entries)
			}
		})
	}
}

// ---- 写侧：delete ----

func TestDeleteChain(t *testing.T) {
	gw := &fakeGateway{
		detect: activeUFW(),
		list: []firewallx.BackendRule{
			rule("allow 8443/tcp", "8443", "tcp", "allow", nil),
			rule("deny 9000/tcp", "9000", "tcp", "deny", nil),
		},
	}
	rec := &fakeRecorder{}
	s := newTestService(gw, &fakeListen{}, rec)

	// 空 ruleKey → FW_001（审计 failure）。
	_, err := s.DeleteRule(context.Background(), nil, "  ", false)
	wantErr(t, err, "FW_001")
	// 不存在 → FW_003（实况复核，D-07 不静默成功）。
	_, err = s.DeleteRule(context.Background(), nil, "allow 9999/tcp", false)
	wantErr(t, err, "FW_003")
	// allow 族命中面板端口未确认 → FW_005 + 零审计。
	_, err = s.DeleteRule(context.Background(), nil, "allow 8443/tcp", false)
	wantErr(t, err, "FW_005")
	if gw.writes() != 0 || len(rec.snapshot()) != 2 {
		t.Fatalf("FW_005 豁免审计（前两拒各一条 failure，涉险零条）: writes=%d entries=%d",
			gw.writes(), len(rec.snapshot()))
	}
	// deny 族命中面板端口：删除放行规则才是涉险面（I-05 触发面=allow 族）。
	gw.delRes = firewallx.DelResult{PermanentOK: true, RuntimeOK: true}
	data, err := s.DeleteRule(context.Background(), nil, "deny 9000/tcp", false)
	if err != nil || !data.Deleted || data.RuleKey != "deny 9000/tcp" {
		t.Fatalf("deny 族删除应直通: %+v %v", data, err)
	}
	// 确认后放行删除 allow 8443。
	data, err = s.DeleteRule(context.Background(), nil, "allow 8443/tcp", true)
	if err != nil || !data.Deleted {
		t.Fatalf("confirmDanger 后删除应放行: %+v %v", data, err)
	}
	if calls := gw.delCallsSnapshot(); !reflect.DeepEqual(calls, []string{"deny 9000/tcp", "allow 8443/tcp"}) {
		t.Fatalf("delete calls = %v", calls)
	}
	entries := rec.snapshot()
	if len(entries) != 4 { // FW_001 + FW_003 两条 failure + 两条 success
		t.Fatalf("audit entries = %d, want 4", len(entries))
	}
	if entries[2].Action != AuditActionRuleDeleted || entries[2].Result != audit.ResultSuccess ||
		entries[3].ObjectID != "allow 8443/tcp" {
		t.Fatalf("delete success 审计错位: %+v %+v", entries[2], entries[3])
	}
}

func TestDeleteGatewaySemantics(t *testing.T) {
	base := func() *fakeGateway {
		return &fakeGateway{detect: activeUFW(), list: []firewallx.BackendRule{
			rule("allow 80/tcp", "80", "tcp", "allow", nil),
		}}
	}
	// 复核与执行间被面板外删除 → FW_003（如实，D-07）。
	gw := base()
	gw.delErr = firewallx.ErrRuleNotFound
	s := newTestService(gw, &fakeListen{}, &fakeRecorder{})
	wantErr(t, func() error { _, err := s.DeleteRule(context.Background(), nil, "allow 80/tcp", false); return err }(), "FW_003")
	// runtime 失败 → FW_007（D-08 双写）。
	gw = base()
	gw.delRes = firewallx.DelResult{PermanentOK: true, Reason: "firewall-cmd: not running"}
	s = newTestService(gw, &fakeListen{}, &fakeRecorder{})
	err := func() error { _, err := s.DeleteRule(context.Background(), nil, "allow 80/tcp", false); return err }()
	wantErr(t, err, "FW_007")
	var ec *errcode.Error
	errors.As(err, &ec)
	if d, ok := ec.Data().(DualLayerData); !ok || d.Permanent != LayerWritten || d.Runtime != LayerFailed || d.Reason == "" {
		t.Fatalf("FW_007 data: %+v", ec.Data())
	}
	// 通道失败 → FW_008。
	gw = base()
	gw.delErr = errors.New("ufw delete: exit 1")
	s = newTestService(gw, &fakeListen{}, &fakeRecorder{})
	wantErr(t, func() error { _, err := s.DeleteRule(context.Background(), nil, "allow 80/tcp", false); return err }(), "FW_008")
}

// ---- D-09 审计降级 ----

func TestAuditRetryDegradation(t *testing.T) {
	gw := &fakeGateway{detect: activeUFW(), addRes: firewallx.AddResult{RuleKey: "allow 80/tcp", PermanentOK: true, RuntimeOK: true}}
	rec := &fakeRecorder{failFirst: 1} // 首次失败 → 重试成功落一条
	s := newTestService(gw, &fakeListen{}, rec)
	if _, err := s.CreateRule(context.Background(), nil, CreateParams{Port: "80", Protocol: "tcp", Action: "allow"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if entries := rec.snapshot(); len(entries) != 1 {
		t.Fatalf("重试成功应落一条: %d", len(entries))
	}
	rec2 := &fakeRecorder{failFirst: 5} // 持续失败 → 日志兜底，不影响操作结果
	s2 := newTestService(gw, &fakeListen{}, rec2)
	if _, err := s2.CreateRule(context.Background(), nil, CreateParams{Port: "80", Protocol: "tcp", Action: "allow"}); err != nil {
		t.Fatalf("审计降级不影响操作结果返回（D-09）: %v", err)
	}
	if len(rec2.snapshot()) != 0 {
		t.Fatal("持续失败不应落条目")
	}
}

// addCallsSnapshot / delCallsSnapshot 线程安全读辅助。
func (f *fakeGateway) addCallsSnapshot() []firewallx.RuleSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]firewallx.RuleSpec(nil), f.addCalls...)
}

func (f *fakeGateway) delCallsSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.delCalls...)
}
