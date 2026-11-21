// container_form.go —— app-mgmt v1.1 容器化应用形态（⚠️A13，D0 Q5）：创建
// 参数/compose 生成（含 dougong-app 归属标签 ⚠️A22）+ 跨模块 OHS 消费
// 契约（docker-mgmt Compose 项目创建，api-docker-mgmt.md v1.1 S-50-04/D-11
// 提供方在位——消费方契约定义于本文件，main 装配注入）。
package appmgmt

import (
	"context"
	"fmt"
	"strings"
)

// ComposeProjectCreator 是 docker-mgmt Compose 项目创建 OHS 消费契约
// （D-12/⚠️A13；提供方 api-docker-mgmt.md v1.1 S-50-04 在位——落盘根
// /opt/dougong-compose/ + 归属标签注入 + 自动 up -d；消费方契约本侧定义）。
type ComposeProjectCreator interface {
	// CreateComposeProject 创建 Compose 项目（落盘 + autoUp 时启动）。
	// 重名/daemon 拒绝错误透传（调用方转 APP 族呈现）。
	CreateComposeProject(ctx context.Context, projectName, composeYaml string, autoUp bool) error
}

// ContainerFormSpec 容器化形态创建参数（start_spec JSON 承载；api-app-mgmt
// v1.1 create 请求 container 分支）。
type ContainerFormSpec struct {
	Image         string             `json:"image"`
	Ports         []ContainerPortDTO `json:"ports,omitempty"`
	Env           []EnvKVDTO         `json:"env,omitempty"`
	Mounts        []MountDTOMount    `json:"mounts,omitempty"`
	RestartPolicy string             `json:"restartPolicy,omitempty"`
}

// EnvKVDTO 环境变量键值（容器化/venv 形态共用）。
type EnvKVDTO struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MountDTOMount 挂载项。
type MountDTOMount struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// ContainerPortDTO 容器端口映射。
type ContainerPortDTO struct {
	Host      string `json:"host"`
	Container string `json:"container"`
	Protocol  string `json:"protocol,omitempty"`
}

// validateImageRef 镜像引用格式校验（名称[:tag]，R-14/APP_013）。
func validateImageRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || len(ref) > 256 {
		return false
	}
	for _, ch := range ref {
		if !('a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || '0' <= ch && ch <= '9' ||
			ch == '/' || ch == ':' || ch == '.' || ch == '-' || ch == '_' || ch == '@') {
			return false
		}
	}
	return true
}

// renderAppCompose 生成容器化应用 compose.yml（⚠️A13：doungo-app 归属标签 +
// 端口映射 + 环境变量 + 挂载 + 重启策略；镜像必填已由校验承载）。
func renderAppCompose(name, image string, port int, env []EnvKVDTO, mounts []MountDTOMount, restart string) string {
	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  " + name + ":\n")
	b.WriteString("    image: " + image + "\n")
	if len(env) > 0 {
		b.WriteString("    environment:\n")
		for _, e := range env {
			if e.Key != "" {
				b.WriteString("      " + e.Key + ": \"" + e.Value + "\"\n")
			}
		}
	}
	if len(mounts) > 0 {
		b.WriteString("    volumes:\n")
		for _, m := range mounts {
			b.WriteString("      - " + m.Source + ":" + m.Target + "\n")
		}
	}
	b.WriteString("    ports:\n")
	b.WriteString("      - \"" + itoa(port) + ":" + itoa(port) + "\"\n")
	rp := restart
	if rp == "" {
		rp = "unless-stopped"
	}
	b.WriteString("    restart: " + rp + "\n")
	b.WriteString("    labels:\n")
	b.WriteString("      dougong.app: \"" + name + "\"\n")
	return b.String()
}

func itoa(n int) string                 { return fmt.Sprintf("%d", n) }
func composeAppRoot(name string) string { return "/opt/dougong-compose/" + name }

// ComposeCreatorFunc 是 ComposeProjectCreator 的函数适配器（main.go 闭包
// 装配——消费方零耦合，跨模块通道经 dockermgmt 服务方法承载）。
type ComposeCreatorFunc func(ctx context.Context, projectName, composeYaml string, autoUp bool) error

// CreateComposeProject 实现 ComposeProjectCreator。
func (f ComposeCreatorFunc) CreateComposeProject(ctx context.Context, projectName, composeYaml string, autoUp bool) error {
	return f(ctx, projectName, composeYaml, autoUp)
}
