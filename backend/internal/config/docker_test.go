// docker_test.go 覆盖 docker.compose_root 配置键（api-docker-mgmt.md D-11：落盘根
// 生产默认 /opt/dougong-compose；仅测试隔离/本机验证经环境变量覆盖）。
package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerComposeRootDefault(t *testing.T) {
	if got := Defaults().Docker.ComposeRoot; got != "/opt/dougong-compose" {
		t.Fatalf("默认落盘根 = %q, want /opt/dougong-compose", got)
	}
}

func TestDockerComposeRootEnvOverride(t *testing.T) {
	t.Setenv("DOUGONG_COMPOSE_ROOT", filepath.Join(t.TempDir(), "compose-root"))
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.HasSuffix(cfg.Docker.ComposeRoot, "compose-root") {
		t.Fatalf("环境变量未生效: %q", cfg.Docker.ComposeRoot)
	}
}

func TestDockerComposeRootValidation(t *testing.T) {
	root := t.TempDir()
	fsRoot := filepath.VolumeName(root) + string(filepath.Separator)
	for _, bad := range []string{"relative/dir", fsRoot} {
		t.Setenv("DOUGONG_COMPOSE_ROOT", bad)
		_, err := Load("")
		if err == nil || !strings.Contains(err.Error(), "docker.compose_root") {
			t.Errorf("compose_root=%q: err = %v, want docker.compose_root 拒绝", bad, err)
		}
	}
}
