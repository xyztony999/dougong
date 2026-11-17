package envmgmt

import (
	"testing"

	"github.com/dougong/dougong/internal/adapter/sysd"
)

// TestUnitServiceStatesNotInstalled systemd LoadUnit 对系统上不存在的单元也
// 返回对象路径（LoadState=not-found）——v0.1.4 实机教训回归：该形态服务态
// 必须为 null（未装环境不得显示「已停止/自启关」），不得落 false/false。
func TestUnitServiceStatesNotInstalled(t *testing.T) {
	for _, ls := range []string{"not-found", ""} {
		d := sysd.UnitDetail{Name: "apache2.service", LoadState: ls, ActiveState: "inactive"}
		running, enabled := unitServiceStates(d)
		if running != nil || enabled != nil {
			t.Fatalf("LoadState=%q: want nil/nil, got %v/%v", ls, *running, *enabled)
		}
	}
}

// TestUnitServiceStatesLoaded 已装载单元按 ActiveState/UnitFileState 映射
// 运行/自启指针。
func TestUnitServiceStatesLoaded(t *testing.T) {
	cases := []struct {
		name                     string
		active, fileState, load  string
		wantRunning, wantEnabled bool
	}{
		{"running-enabled", "active", "enabled", "loaded", true, true},
		{"stopped-disabled", "inactive", "disabled", "loaded", false, false},
		{"failed-enabled", "failed", "enabled", "loaded", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := sysd.UnitDetail{Name: "nginx.service", LoadState: c.load,
				ActiveState: c.active, EnabledState: c.fileState}
			running, enabled := unitServiceStates(d)
			if running == nil || enabled == nil {
				t.Fatal("want non-nil states for loaded unit")
			}
			if *running != c.wantRunning || *enabled != c.wantEnabled {
				t.Fatalf("got running=%v enabled=%v, want %v/%v",
					*running, *enabled, c.wantRunning, c.wantEnabled)
			}
		})
	}
}
