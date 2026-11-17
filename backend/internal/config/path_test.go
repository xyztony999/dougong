package config

import (
	"path/filepath"
	"testing"
)

// TestResolveDBPath db_path 锚定语义（v0.1.3 CLI 数据目录 bug 修正的回归锁）：
// 绝对路径原样；相对路径锚定到配置文件所在目录（安装布局与开发布局均与
// 历史服务行为逐字节一致）。
func TestResolveDBPath(t *testing.T) {
	absDB := filepath.Join(t.TempDir(), "panel.db") // 跨平台绝对路径
	tests := []struct {
		name, cfgPath, dbPath string
		want                  string
	}{
		{"绝对路径原样", "/etc/dougong/dougong.json", absDB, absDB},
		{"安装布局锚定", "/var/lib/dougong/dougong.json", "data/dougong.db",
			filepath.Join("/var/lib/dougong", "data", "dougong.db")},
		{"开发布局点号", "dougong.json", "data/dougong.db",
			filepath.Join(".", "data", "dougong.db")},
		{"显式配置目录", "/srv/panel/prod.json", "data/dougong.db",
			filepath.Join("/srv/panel", "data", "dougong.db")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveDBPath(tt.cfgPath, tt.dbPath); got != tt.want {
				t.Fatalf("ResolveDBPath(%q, %q) = %q, want %q", tt.cfgPath, tt.dbPath, got, tt.want)
			}
		})
	}
}
