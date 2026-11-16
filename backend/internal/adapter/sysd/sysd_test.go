package sysd

import (
	"reflect"
	"testing"

	"github.com/godbus/dbus/v5"
)

// 纯函数表驱动测试（dbus 通道逻辑由 service 层 fake 网关覆盖；Windows 开发
// 机无 system bus，通道路径不设单测——Linux 目标环境冒烟归属部署验证）。

func TestUnitTypeOf(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"service", "nginx.service", "service"},
		{"socket", "dbus.socket", "socket"},
		{"timer", "logrotate.timer", "timer"},
		{"mount", "-.mount", "mount"},
		{"target", "multi-user.target", "target"},
		{"scope", "session-1.scope", "scope"},
		{"device", "sys-devices.device", "device"},
		{"template", "user@.service", "service"},
		{"instance", "user@1000.service", "service"},
		{"unknown suffix", "foo.gz", ""},
		{"no dot", "fooservice", ""},
		{"trailing dot", "foo.", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UnitTypeOf(tt.in); got != tt.want {
				t.Fatalf("UnitTypeOf(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeActiveState(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"active", "active"},
		{"reloading", "active"},        // 瞬时态方向归一（A-S3）
		{"activating", "active"},       //
		{"inactive", "inactive"},       //
		{"deactivating", "inactive"},   //
		{"failed", "failed"},           //
		{"maintenance", "failed"},      //
		{"weird-state", "weird-state"}, // 防御透传（封闭集外）
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := NormalizeActiveState(tt.in); got != tt.want {
				t.Fatalf("NormalizeActiveState(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMergeUnitProjection(t *testing.T) {
	loaded := []LoadedUnit{
		{Name: "nginx.service", Description: "nginx web server", LoadState: "loaded",
			ActiveState: "active", SubState: "running"},
		{Name: "dbus.service", LoadState: "loaded", ActiveState: "activating", SubState: "start"},
		{Name: "session-2.scope", ActiveState: "active", SubState: "running"}, // 无 unit 文件
		{Name: "old.socket", LoadState: "loaded", ActiveState: "inactive", SubState: "dead"},
	}
	files := []UnitFile{
		{Name: "nginx.service", State: "enabled"},
		{Name: "sshd.service", State: "disabled"}, // 已装未加载（file-only）
		{Name: "old.socket", State: "static"},
		{Name: "alias.service", State: "alias"}, // 别名透传（非两态）
	}
	got := MergeUnitProjection(loaded, files)

	want := []UnitSnapshot{
		{Name: "alias.service", UnitType: "service", ActiveState: "inactive", SubState: "dead", EnabledState: "alias"},
		{Name: "dbus.service", UnitType: "service", ActiveState: "active", SubState: "start", EnabledState: "", LoadState: "loaded"}, // activating 归一
		{Name: "nginx.service", UnitType: "service", ActiveState: "active", SubState: "running",
			EnabledState: "enabled", Description: "nginx web server", LoadState: "loaded"},
		{Name: "old.socket", UnitType: "socket", ActiveState: "inactive", SubState: "dead", EnabledState: "static", LoadState: "loaded"},
		{Name: "session-2.scope", UnitType: "scope", ActiveState: "active", SubState: "running", EnabledState: ""},
		{Name: "sshd.service", UnitType: "service", ActiveState: "inactive", SubState: "dead", EnabledState: "disabled"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge mismatch:\ngot  %+v\nwant %+v", got, want)
	}
	// 字典序升序（D-02）
	for i := 1; i < len(got); i++ {
		if got[i-1].Name >= got[i].Name {
			t.Fatalf("not sorted ascending: %s >= %s", got[i-1].Name, got[i].Name)
		}
	}
}

func TestMergeUnitProjectionEmptyAndBlank(t *testing.T) {
	if got := MergeUnitProjection(nil, nil); len(got) != 0 {
		t.Fatalf("empty input should yield empty output, got %+v", got)
	}
	// 空名条目跳过（防御：dbus 不会返回空名）
	got := MergeUnitProjection(
		[]LoadedUnit{{Name: "", ActiveState: "active"}},
		[]UnitFile{{Name: "", State: "enabled"}},
	)
	if len(got) != 0 {
		t.Fatalf("blank names should be skipped, got %+v", got)
	}
}

func TestFileBaseName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/lib/systemd/system/nginx.service", "nginx.service"},
		{"/etc/systemd/system/foo.timer", "foo.timer"},
		{"nginx.service", "nginx.service"},
	}
	for _, tt := range tests {
		if got := FileBaseName(tt.in); got != tt.want {
			t.Fatalf("FileBaseName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDetailFromProps(t *testing.T) {
	props := map[string]dbus.Variant{
		"Id":            dbus.MakeVariant("nginx.service"),
		"Description":   dbus.MakeVariant("nginx web server"),
		"LoadState":     dbus.MakeVariant("loaded"),
		"ActiveState":   dbus.MakeVariant("active"),
		"SubState":      dbus.MakeVariant("running"),
		"UnitFileState": dbus.MakeVariant("enabled"),
		"NotAString":    dbus.MakeVariant(uint32(3)),
	}
	d := DetailFromProps("nginx.service", props)
	want := UnitDetail{
		Name: "nginx.service", UnitType: "service", ActiveState: "active",
		SubState: "running", EnabledState: "enabled",
		Description: "nginx web server", LoadState: "loaded",
	}
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("detail mismatch:\ngot  %+v\nwant %+v", d, want)
	}

	// 缺键 → 零值串（service 层落 null）
	d2 := DetailFromProps("x.socket", map[string]dbus.Variant{})
	if d2.ActiveState != "" || d2.EnabledState != "" || d2.UnitType != "socket" {
		t.Fatalf("missing props should zero fields, got %+v", d2)
	}
}
