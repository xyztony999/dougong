package servicemgmt

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// ---- 测试替身 ----

// getResult 是 fake 网关 GetUnit 的 scripted 结果（按调用次序出队）。
type getResult struct {
	d   sysd.UnitDetail
	err error
}

// fakeGateway 是 SysdGateway 契约的进程内替身（systemd 交互不出单测边界）。
type fakeGateway struct {
	mu sync.Mutex

	list    []sysd.UnitSnapshot
	listErr error

	getSeq []getResult // 按次序出队；耗尽后退 get
	get    getResult

	jobRes  sysd.JobResult
	jobErr  error
	starts  []string // 已提交的 job 动作（"StartUnit:nginx.service"）
	toggles []string // 已提交的自启动作（"enable:nginx.service"）

	enableErr  error
	disableErr error
}

func (f *fakeGateway) ListUnits(context.Context) ([]sysd.UnitSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.list, f.listErr
}

func (f *fakeGateway) GetUnit(_ context.Context, name string) (sysd.UnitDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.getSeq) > 0 {
		r := f.getSeq[0]
		f.getSeq = f.getSeq[1:]
		return r.d, r.err
	}
	if f.get.d.Name != "" {
		return f.get.d, f.get.err
	}
	// 默认实况：getSeq/get 未配置时按 name 产出 loaded 快照（存在性成功路径）。
	return sysd.UnitDetail{
		Name: name, UnitType: sysd.UnitTypeOf(name),
		ActiveState: "active", SubState: "running", EnabledState: "enabled",
		Description: "desc of " + name, LoadState: "loaded",
	}, f.get.err
}

func (f *fakeGateway) submit(kind, name string) (sysd.JobResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts = append(f.starts, kind+":"+name)
	return f.jobRes, f.jobErr
}

func (f *fakeGateway) StartUnit(_ context.Context, name string) (sysd.JobResult, error) {
	return f.submit("StartUnit", name)
}

func (f *fakeGateway) StopUnit(_ context.Context, name string) (sysd.JobResult, error) {
	return f.submit("StopUnit", name)
}

func (f *fakeGateway) RestartUnit(_ context.Context, name string) (sysd.JobResult, error) {
	return f.submit("RestartUnit", name)
}

func (f *fakeGateway) EnableUnit(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.toggles = append(f.toggles, "enable:"+name)
	return f.enableErr
}

func (f *fakeGateway) DisableUnit(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.toggles = append(f.toggles, "disable:"+name)
	return f.disableErr
}

func (f *fakeGateway) submitted() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.starts...)
}

func (f *fakeGateway) toggled() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.toggles...)
}

// fakeRecorder 是 AuditRecorder 替身（可注入失败验证 D-06 降级）。
type fakeRecorder struct {
	mu       sync.Mutex
	entries  []audit.Entry
	failAll  bool
	failOnce bool
	calls    int
}

func (r *fakeRecorder) Record(_ context.Context, e audit.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.failAll {
		return errors.New("audit backend down")
	}
	if r.failOnce && r.calls == 1 {
		return errors.New("audit transient error")
	}
	r.entries = append(r.entries, e)
	return nil
}

func (r *fakeRecorder) snapshot() ([]audit.Entry, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]audit.Entry(nil), r.entries...), r.calls
}

// ---- 通用夹具 ----

var testActor = &authsvc.Identity{UserID: 7, Username: "admin", Role: "admin"}

func newTestService(gw *fakeGateway, rec *fakeRecorder) *Service {
	return New(gw, rec)
}

func wantSVC(t *testing.T, err error, symbol string) {
	t.Helper()
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("err = %v, want errcode %s", err, symbol)
	}
	if ec.Symbol() != symbol {
		t.Fatalf("err symbol = %s, want %s (%s)", ec.Symbol(), symbol, ec.Error())
	}
}

// ---- SVC_002 状态过滤参数 ----

func TestParseStateFilter(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    map[string]bool
		wantSVC string // 非空表示期望错误
	}{
		{"empty = no filter", "", nil, ""},
		{"single", "active", map[string]bool{"active": true}, ""},
		{"multi", "active,failed", map[string]bool{"active": true, "failed": true}, ""},
		{"duplicate ok", "active,active", map[string]bool{"active": true}, ""},
		{"bad value", "running", nil, "SVC_002"},
		{"empty segment", "active,", nil, "SVC_002"},
		{"uppercase rejected", "Active", nil, "SVC_002"},
		{"whitespace rejected", "active, failed", nil, "SVC_002"},
		{"unknown enum mix", "active,inactive,failed,activating", nil, "SVC_002"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStateFilter(tt.raw)
			if tt.wantSVC != "" {
				wantSVC(t, err, tt.wantSVC)
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("filter = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---- list 投影 ----

func listFixture() []sysd.UnitSnapshot {
	return []sysd.UnitSnapshot{
		{Name: "zulu.service", UnitType: "service", ActiveState: "failed", SubState: "failed", EnabledState: "enabled"},
		{Name: "Alpha.service", UnitType: "service", ActiveState: "active", SubState: "running", EnabledState: "disabled"},
		{Name: "alpha-notify.path", UnitType: "path", ActiveState: "inactive", SubState: "dead", EnabledState: "static"},
		{Name: "backup.timer", UnitType: "timer", ActiveState: "inactive", SubState: "waiting", EnabledState: "enabled"},
		{Name: "nginx.service", UnitType: "service", ActiveState: "active", SubState: "running", EnabledState: "static"},
	}
}

func TestListProjectAndSearch(t *testing.T) {
	gw := &fakeGateway{list: listFixture()}
	svc := newTestService(gw, &fakeRecorder{})

	data, err := svc.List(context.Background(), "active,failed", "ALPHA")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// 过滤（active,failed）+ 搜索（不区分大小写命中 Alpha.service；zulu 不含
	// alpha；alpha-notify.path 被 activeState=inactive 过滤）→ 1 条。
	if data.Total != 1 || len(data.Items) != 1 {
		t.Fatalf("total=%d items=%d, want 1（data=%+v）", data.Total, len(data.Items), data.Items)
	}
	it := data.Items[0]
	if it.Name != "Alpha.service" || it.UnitType != "service" || it.ActiveState != "active" {
		t.Fatalf("item mismatch: %+v", it)
	}
	if !it.CanOperate || !it.CanToggleAutostart {
		t.Fatalf("service+disabled 应可操作可切自启: %+v", it)
	}
	if _, err := time.Parse(time.RFC3339, data.GeneratedAt); err != nil {
		t.Fatalf("generatedAt 非 RFC3339: %q", data.GeneratedAt)
	}
}

func TestListSortAndAuthorityFields(t *testing.T) {
	gw := &fakeGateway{list: listFixture()}
	svc := newTestService(gw, &fakeRecorder{})

	data, err := svc.List(context.Background(), "", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if data.Total != 5 {
		t.Fatalf("total = %d, want 5（全量不分页 D-02）", data.Total)
	}
	// 名称字典序升序（字节序：大写 A=65 < 小写 a/n/b/z）
	wantOrder := []string{"Alpha.service", "alpha-notify.path", "backup.timer", "nginx.service", "zulu.service"}
	for i, w := range wantOrder {
		if data.Items[i].Name != w {
			t.Fatalf("order[%d] = %s, want %s（items=%+v）", i, data.Items[i].Name, w, data.Items)
		}
	}
	// 权威计算矩阵（I-07/D-05）：非 service → 不可操作不可切自启；
	// service+static → 可操作但不可切自启；timer+enabled → 类型不过关。
	byName := map[string]UnitListItem{}
	for _, it := range data.Items {
		byName[it.Name] = it
	}
	if it := byName["backup.timer"]; it.CanOperate || it.CanToggleAutostart {
		t.Fatalf("timer 不应可操作: %+v", it)
	}
	if it := byName["nginx.service"]; !it.CanOperate || it.CanToggleAutostart {
		t.Fatalf("service+static 应可操作不可切自启: %+v", it)
	}
	if it := byName["zulu.service"]; !it.CanOperate || !it.CanToggleAutostart {
		t.Fatalf("service+enabled 应双可: %+v", it)
	}
}

func TestListChannelUnavailable(t *testing.T) {
	gw := &fakeGateway{listErr: wrapSentinel(sysd.ErrUnavailable)}
	svc := newTestService(gw, &fakeRecorder{})
	_, err := svc.List(context.Background(), "", "")
	wantSVC(t, err, "SVC_008")
}

func TestListInvalidFilterNoGatewayCall(t *testing.T) {
	gw := &fakeGateway{listErr: wrapSentinel(sysd.ErrUnavailable)} // 参数错应先于通道错
	svc := newTestService(gw, &fakeRecorder{})
	_, err := svc.List(context.Background(), "running", "")
	wantSVC(t, err, "SVC_002")
}

// ---- detail ----

func TestDetail(t *testing.T) {
	tests := []struct {
		name    string
		unit    string
		get     getResult
		wantSVC string
		wantNil bool // description/loadState 应为 null
	}{
		{
			name: "ok with description",
			unit: "nginx.service",
			get: getResult{d: sysd.UnitDetail{Name: "nginx.service", UnitType: "service",
				ActiveState: "active", SubState: "running", EnabledState: "enabled",
				Description: "nginx web server", LoadState: "loaded"}},
		},
		{
			name:    "empty description/loadState → null",
			unit:    "bare.service",
			get:     getResult{d: sysd.UnitDetail{Name: "bare.service", UnitType: "service", ActiveState: "inactive", EnabledState: "static"}},
			wantNil: true,
		},
		{name: "not found", unit: "ghost.service", get: getResult{err: wrapSentinel(sysd.ErrUnitNotFound)}, wantSVC: "SVC_003"},
		{name: "unavailable", unit: "x.service", get: getResult{err: wrapSentinel(sysd.ErrUnavailable)}, wantSVC: "SVC_008"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &fakeGateway{getSeq: []getResult{tt.get}}
			svc := newTestService(gw, &fakeRecorder{})
			data, err := svc.Detail(context.Background(), tt.unit)
			if tt.wantSVC != "" {
				wantSVC(t, err, tt.wantSVC)
				return
			}
			if err != nil {
				t.Fatalf("detail: %v", err)
			}
			if data.Name != tt.unit || data.UnitType != "service" || !data.CanOperate {
				t.Fatalf("detail shape: %+v", data)
			}
			if tt.wantNil {
				if data.Description != nil || data.LoadState != nil {
					t.Fatalf("空属性应落 null: %+v", data)
				}
			} else if data.Description == nil || *data.Description != "nginx web server" || data.LoadState == nil {
				t.Fatalf("属性透传失败: %+v", data)
			}
		})
	}
}

func TestDetailInvalidNameSkipsGateway(t *testing.T) {
	// 网关未配置（默认路径=存在性成功）；若 Detail 触网关则不会返回 SVC_001，
	// 故 SVC_001 即证明形态校验先行未触网关。
	gw := &fakeGateway{}
	svc := newTestService(gw, &fakeRecorder{})
	_, err := svc.Detail(context.Background(), "no-suffix")
	wantSVC(t, err, "SVC_001")
}

// ---- job 动作矩阵（start/stop/restart）----

func TestJobActionMatrix(t *testing.T) {
	svcOK := sysd.UnitDetail{Name: "nginx.service", UnitType: "service", ActiveState: "active",
		SubState: "running", EnabledState: "enabled", LoadState: "loaded"}

	tests := []struct {
		name      string
		unit      string
		getSeq    []getResult
		jobRes    sysd.JobResult
		jobErr    error
		wantSVC   string
		wantData  *ActionData
		wantAudit string // "" = success；否则 failure+detail 匹配
	}{
		{
			name:     "start success（操作后快照）",
			unit:     "nginx.service",
			getSeq:   []getResult{{d: svcOK}, {d: sysd.UnitDetail{Name: "nginx.service", ActiveState: "active", SubState: "running", EnabledState: "enabled"}}},
			jobRes:   sysd.JobResult{Succeeded: true, Reason: "done"},
			wantData: &ActionData{Name: "nginx.service", ActiveState: "active", SubState: "running", EnabledState: "enabled"},
		},
		{
			name:    "invalid name SVC_001",
			unit:    "nginx",
			wantSVC: "SVC_001", wantAudit: "SVC_001",
		},
		{
			name:    "non-service SVC_004",
			unit:    "backup.timer",
			getSeq:  []getResult{{d: sysd.UnitDetail{Name: "backup.timer", UnitType: "timer", ActiveState: "inactive", EnabledState: "enabled"}}},
			wantSVC: "SVC_004", wantAudit: "SVC_004",
		},
		{
			name:    "not found SVC_003",
			unit:    "ghost.service",
			getSeq:  []getResult{{err: wrapSentinel(sysd.ErrUnitNotFound)}},
			wantSVC: "SVC_003", wantAudit: "SVC_003",
		},
		{
			name:    "channel down SVC_008",
			unit:    "x.service",
			getSeq:  []getResult{{err: wrapSentinel(sysd.ErrUnavailable)}},
			wantSVC: "SVC_008", wantAudit: "SVC_008",
		},
		{
			name:      "job timeout SVC_006",
			unit:      "slow.service",
			getSeq:    []getResult{{d: svcOK}},
			jobErr:    wrapSentinel(sysd.ErrJobTimeout),
			wantSVC:   "SVC_006",
			wantAudit: "SVC_006",
		},
		{
			name:      "job failed SVC_007（原因+实际终态）",
			unit:      "broken.service",
			getSeq:    []getResult{{d: sysd.UnitDetail{Name: "broken.service", UnitType: "service", ActiveState: "inactive", EnabledState: "disabled"}}, {d: sysd.UnitDetail{Name: "broken.service", ActiveState: "failed", SubState: "failed", EnabledState: "disabled"}}},
			jobRes:    sysd.JobResult{Succeeded: false, Reason: "failed", ActiveState: "failed", SubState: "failed"},
			wantSVC:   "SVC_007",
			wantAudit: "SVC_007",
		},
		{
			name:    "rejected at submit SVC_007（调用期拒绝，activeState null）",
			unit:    "locked.service",
			getSeq:  []getResult{{d: svcOK}},
			jobErr:  wrapSentinel(sysd.ErrRejected),
			wantSVC: "SVC_007", wantAudit: "SVC_007",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &fakeGateway{getSeq: tt.getSeq, jobRes: tt.jobRes, jobErr: tt.jobErr}
			rec := &fakeRecorder{}
			svc := newTestService(gw, rec)

			data, err := svc.Start(context.Background(), testActor, tt.unit)
			entries, calls := rec.snapshot()

			// I-06：每个动作接口调用恰好一条审计（A-S5：guard 拒绝也记）。
			if len(entries) != 1 {
				t.Fatalf("audit entries = %d, want 1（I-06 恰好一条；calls=%d entries=%+v）", len(entries), calls, entries)
			}
			e := entries[0]
			if e.Action != AuditActionServiceStarted || e.ObjectType != ObjectTypeSystemdService || e.ObjectID != tt.unit {
				t.Fatalf("audit 四要素错位: %+v", e)
			}
			if e.ActorName != "admin" || e.ActorID == nil || *e.ActorID != 7 || e.Category != audit.CategoryChange || e.Source != audit.SourceWeb {
				t.Fatalf("audit 基底错位: %+v", e)
			}

			if tt.wantSVC != "" {
				wantSVC(t, err, tt.wantSVC)
				if e.Result != audit.ResultFailure {
					t.Fatalf("失败动作审计 result = %s, want failure", e.Result)
				}
				if d, ok := e.Detail.(map[string]any); !ok || d["error"] != tt.wantAudit {
					t.Fatalf("审计 detail = %+v, want error=%s", e.Detail, tt.wantAudit)
				}
				return
			}
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			if e.Result != audit.ResultSuccess {
				t.Fatalf("成功动作审计 result = %s, want success", e.Result)
			}
			if !reflect.DeepEqual(data, tt.wantData) {
				t.Fatalf("data = %+v, want %+v", data, tt.wantData)
			}
			if subs := gw.submitted(); len(subs) != 1 || subs[0] != "StartUnit:"+tt.unit {
				t.Fatalf("submits = %v", subs)
			}
		})
	}
}

func TestTimeoutDataAndAuditDetail(t *testing.T) {
	gw := &fakeGateway{
		getSeq: []getResult{{d: sysd.UnitDetail{Name: "slow.service", UnitType: "service", EnabledState: "enabled"}}},
		jobErr: wrapSentinel(sysd.ErrJobTimeout),
	}
	rec := &fakeRecorder{}
	svc := newTestService(gw, rec)

	_, err := svc.Restart(context.Background(), testActor, "slow.service")
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("want errcode, got %v", err)
	}
	wantSVC(t, err, "SVC_006")
	td, ok := ec.Data().(TimeoutData)
	if !ok || td.Name != "slow.service" || td.Note != TimeoutNote {
		t.Fatalf("SVC_006 data = %+v（note 应为契约文案）", ec.Data())
	}
	entries, _ := rec.snapshot()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	d, ok := entries[0].Detail.(map[string]any)
	if !ok || d["timeout"] != true || d["error"] != "SVC_006" {
		t.Fatalf("超时审计 detail 应含 timeout=true: %+v", entries[0].Detail)
	}
	if entries[0].Action != AuditActionServiceRestarted {
		t.Fatalf("action = %s, want %s", entries[0].Action, AuditActionServiceRestarted)
	}
}

func TestJobFailureData(t *testing.T) {
	gw := &fakeGateway{
		getSeq: []getResult{
			{d: sysd.UnitDetail{Name: "broken.service", UnitType: "service", EnabledState: "disabled"}},
			{d: sysd.UnitDetail{Name: "broken.service", ActiveState: "failed", SubState: "failed", EnabledState: "disabled"}},
		},
		jobRes: sysd.JobResult{Succeeded: false, Reason: "dependency", ActiveState: "failed"},
	}
	svc := newTestService(gw, &fakeRecorder{})
	_, err := svc.Stop(context.Background(), testActor, "broken.service")
	var ec *errcode.Error
	if !errors.As(err, &ec) || ec.Symbol() != "SVC_007" {
		t.Fatalf("want SVC_007, got %v", err)
	}
	fd, ok := ec.Data().(ActionFailureData)
	if !ok || fd.Name != "broken.service" || fd.Reason != "dependency" || fd.ActiveState == nil || *fd.ActiveState != "failed" {
		t.Fatalf("SVC_007 data = %+v（应含失败原因与实际终态）", ec.Data())
	}
}

// A-S6：动作成功但操作后快照读取失败 → JobResult 终态 + 前置 enabledState 降级。
func TestPostSnapshotDegraded(t *testing.T) {
	gw := &fakeGateway{
		getSeq: []getResult{
			{d: sysd.UnitDetail{Name: "nginx.service", UnitType: "service", EnabledState: "enabled"}},
			{err: wrapSentinel(sysd.ErrUnavailable)}, // post 读取失败
		},
		jobRes: sysd.JobResult{Succeeded: true, Reason: "done", ActiveState: "inactive", SubState: "dead"},
	}
	svc := newTestService(gw, &fakeRecorder{})
	data, err := svc.Stop(context.Background(), testActor, "nginx.service")
	if err != nil {
		t.Fatalf("stop: %v（快照失败不得翻转成功结果）", err)
	}
	want := ActionData{Name: "nginx.service", ActiveState: "inactive", SubState: "dead", EnabledState: "enabled"}
	if !reflect.DeepEqual(*data, want) {
		t.Fatalf("data = %+v, want %+v", *data, want)
	}
}

// ---- 自启切换 ----

func TestToggleAutostart(t *testing.T) {
	tests := []struct {
		name      string
		op        string // enable | disable
		unit      string
		enabled   string // 前置 UnitFileState
		opErr     error
		wantSVC   string
		wantState string
	}{
		{name: "enable ok", op: "enable", unit: "nginx.service", enabled: "disabled", wantState: "enabled"},
		{name: "disable ok", op: "disable", unit: "nginx.service", enabled: "enabled", wantState: "disabled"},
		{name: "static not toggleable SVC_005", op: "enable", unit: "nginx.service", enabled: "static", wantSVC: "SVC_005"},
		{name: "masked SVC_005", op: "disable", unit: "nginx.service", enabled: "masked", wantSVC: "SVC_005"},
		{name: "indirect SVC_005", op: "enable", unit: "nginx.service", enabled: "indirect", wantSVC: "SVC_005"},
		{name: "enabled-runtime not two-state SVC_005", op: "disable", unit: "nginx.service", enabled: "enabled-runtime", wantSVC: "SVC_005"},
		{name: "no unit file SVC_005", op: "enable", unit: "nginx.service", enabled: "", wantSVC: "SVC_005"},
		{name: "rejected SVC_007", op: "enable", unit: "nginx.service", enabled: "disabled", opErr: wrapSentinel(sysd.ErrRejected), wantSVC: "SVC_007"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getSeq := []getResult{{d: sysd.UnitDetail{Name: tt.unit, UnitType: "service",
				ActiveState: "active", EnabledState: tt.enabled}}}
			if tt.wantSVC == "" { // 成功路径补操作后刷新读取
				getSeq = append(getSeq, getResult{d: sysd.UnitDetail{
					Name: tt.unit, UnitType: "service", EnabledState: tt.wantState}})
			}
			gw := &fakeGateway{
				getSeq:     getSeq,
				enableErr:  tt.opErr,
				disableErr: tt.opErr,
			}
			rec := &fakeRecorder{}
			svc := newTestService(gw, rec)

			var data *AutostartData
			var err error
			if tt.op == "disable" {
				data, err = svc.Disable(context.Background(), testActor, tt.unit)
			} else {
				data, err = svc.Enable(context.Background(), testActor, tt.unit)
			}

			entries, _ := rec.snapshot()
			if len(entries) != 1 {
				t.Fatalf("audit entries = %d, want 1（I-06）", len(entries))
			}
			if tt.wantSVC != "" {
				wantSVC(t, err, tt.wantSVC)
				if entries[0].Result != audit.ResultFailure {
					t.Fatalf("拒绝动作审计 result = %s, want failure", entries[0].Result)
				}
				// SVC_005 拒绝不得触自启调用（guard 先于提交）
				if tt.wantSVC == "SVC_005" && len(gw.toggled()) != 0 {
					t.Fatalf("guard 拒绝后不应提交: %v", gw.toggled())
				}
				return
			}
			if err != nil {
				t.Fatalf("toggle: %v", err)
			}
			if data.EnabledState != tt.wantState {
				t.Fatalf("enabledState = %s, want %s（刷新值）", data.EnabledState, tt.wantState)
			}
			wantAction := AuditActionServiceEnabled
			if tt.op == "disable" {
				wantAction = AuditActionServiceDisabled
			}
			if entries[0].Result != audit.ResultSuccess || entries[0].Action != wantAction {
				t.Fatalf("审计错位: %+v", entries[0])
			}
			if tog := gw.toggled(); len(tog) != 1 || tog[0] != tt.op+":"+tt.unit {
				t.Fatalf("toggles = %v", tog)
			}
		})
	}
}

func TestToggleGuardOrder(t *testing.T) {
	// D-09 逐级：形态（001）→ 存在性（003）→ 类型（004）→ 两态（005）。
	gw := &fakeGateway{getSeq: []getResult{{err: wrapSentinel(sysd.ErrUnitNotFound)}}}
	svc := newTestService(gw, &fakeRecorder{})
	// timer：形态合法但类型不过 → 004（即使存在性会失败也不先报 003——顺序为
	// 形态→存在性→类型，故 timer 先触存在性；此处网关报 003 验证存在性先于类型）
	_, err := svc.Enable(context.Background(), testActor, "backup.timer")
	wantSVC(t, err, "SVC_003")

	// 合法名 + 存在 + timer → 004
	gw2 := &fakeGateway{getSeq: []getResult{{d: sysd.UnitDetail{Name: "backup.timer", UnitType: "timer", EnabledState: "enabled"}}}}
	svc2 := newTestService(gw2, &fakeRecorder{})
	_, err = svc2.Disable(context.Background(), testActor, "backup.timer")
	wantSVC(t, err, "SVC_004")
}

// ---- D-06 审计降级 ----

func TestAuditWriteDegradedD06(t *testing.T) {
	gw := &fakeGateway{
		getSeq: []getResult{{d: sysd.UnitDetail{Name: "nginx.service", UnitType: "service", EnabledState: "enabled"}}},
		jobRes: sysd.JobResult{Succeeded: true, Reason: "done", ActiveState: "active", SubState: "running"},
	}
	rec := &fakeRecorder{failAll: true}
	svc := newTestService(gw, rec)

	data, err := svc.Start(context.Background(), testActor, "nginx.service")
	if err != nil {
		t.Fatalf("审计写入失败不得影响操作结果: %v", err)
	}
	if data.ActiveState != "active" {
		t.Fatalf("data = %+v", data)
	}
	if _, calls := rec.snapshot(); calls != 2 {
		t.Fatalf("D-06 应重试一次（calls=%d, want 2）", calls)
	}
}

func TestAuditRetrySucceedsD06(t *testing.T) {
	gw := &fakeGateway{
		getSeq: []getResult{{d: sysd.UnitDetail{Name: "nginx.service", UnitType: "service", EnabledState: "disabled"}}},
	}
	rec := &fakeRecorder{failOnce: true}
	svc := newTestService(gw, rec)

	if _, err := svc.Enable(context.Background(), testActor, "nginx.service"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	entries, calls := rec.snapshot()
	if calls != 2 || len(entries) != 1 {
		t.Fatalf("首次失败重试成功应落一条（calls=%d entries=%d）", calls, len(entries))
	}
}

// ---- 常量与契约口径 ----

func TestContractConstants(t *testing.T) {
	if ActionWaitLimit != 90*time.Second {
		t.Fatalf("ActionWaitLimit = %s, want 90s（D-01）", ActionWaitLimit)
	}
	for name, want := range map[string]string{
		"nginx.service":     "",        // 合法
		"bad":               "SVC_001", // 无后缀
		"nginx.serv":        "SVC_001", // 非官方后缀
		"nginx.SERVICE":     "SVC_001", // 大写后缀
		"nginx .service":    "SVC_001", // 空格
		"user@1000.service": "",        // 实例名合法
		"-.mount":           "",        // 转义根挂载合法
		"":                  "SVC_001", // 空名
	} {
		if e := guardUnitName(name); (e == nil) != (want == "") {
			t.Fatalf("guardUnitName(%q) 期望%s", name, want)
		} else if want != "" {
			wantSVC(t, e, want)
		}
	}
}

// wrapSentinel 构造哨兵包装错误（模拟 adapter 层 %w 包装链）。
func wrapSentinel(sentinel error) error {
	return errors.Join(sentinel) // 保持 errors.Is 可解链的最小包装
}
