// create.go —— docker-mgmt v1.1 创建类 SDK 通道（api-docker-mgmt.md v1.1
// D-09~D-11）：容器创建+启动（镜像缺失自动拉取）、卷创建、网络创建。
// 全部经官方 SDK（R-08 纪律由 SDK 类型化参数承载，无 shell）；超时/错误归一
// 沿 dockerx.go 既有形态（SDKTimeout/sdkError）。
package dockerx

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	specs "github.com/docker/go-connections/nat"
)

// CreateContainerSpec 容器创建参数（service 层校验后的规范化值）。
type CreateContainerSpec struct {
	Name          string
	Image         string
	PortMappings  []PortMapping // 宿主端口:容器端口(:协议)
	Env           []string      // KEY=VALUE（原样透传 daemon）
	Mounts        []MountSpec
	RestartPolicy string // no|always|unless-stopped|on-failure
	NetworkName   string // 空=默认 bridge
}

// PortMapping 端口映射。
type PortMapping struct {
	HostPort      string
	ContainerPort string
	Protocol      string // tcp（默认）/udp
}

// MountSpec 挂载。
type MountSpec struct {
	Source string // 卷名或主机路径
	Target string // 容器内路径
	Mode   string // 可选（ro 等）
}

// CreateContainer 创建并启动容器（docker run 语义：create+start 两步）。
// 镜像本地缺失时由 service 层先调 PullImage（D-09），此处不再拉取。
func (g *Gateway) CreateContainer(ctx context.Context, spec CreateContainerSpec) (string, error) {
	binds := make([]string, 0, len(spec.Mounts))
	for _, m := range spec.Mounts {
		b := m.Source + ":" + m.Target
		if m.Mode != "" {
			b += ":" + m.Mode
		}
		binds = append(binds, b)
	}
	portBindings := specs.PortMap{}
	exposed := specs.PortSet{}
	for _, p := range spec.PortMappings {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		key := specs.Port(p.ContainerPort + "/" + proto)
		exposed[key] = struct{}{}
		portBindings[key] = []specs.PortBinding{{HostPort: p.HostPort}}
	}
	restart := container.RestartPolicy{Name: container.RestartPolicyMode(spec.RestartPolicy)}
	if spec.RestartPolicy == "" || spec.RestartPolicy == "no" {
		restart = container.RestartPolicy{Name: container.RestartPolicyDisabled}
	}
	var nw *network.NetworkingConfig
	endpoints := map[string]*network.EndpointSettings{}
	if spec.NetworkName != "" {
		endpoints[spec.NetworkName] = &network.EndpointSettings{}
		nw = &network.NetworkingConfig{EndpointsConfig: endpoints}
	}
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	resp, err := g.cli.ContainerCreate(cctx,
		&container.Config{
			Image:        spec.Image,
			ExposedPorts: exposed,
			Env:          spec.Env,
		},
		&container.HostConfig{
			Binds:         binds,
			PortBindings:  portBindings,
			RestartPolicy: restart,
			NetworkMode:   container.NetworkMode(spec.NetworkName),
		},
		nw, nil, spec.Name)
	if err != nil {
		return "", sdkError("container create", err)
	}
	sctx, scancel := withTimeout(ctx, SDKTimeout)
	defer scancel()
	if err := g.cli.ContainerStart(sctx, resp.ID, container.StartOptions{}); err != nil {
		// 创建成功但启动失败：保留容器（stopped 态，列表可见可手动启动——
		// 不静默回滚删除，用户可据实况处置），错误如实上抛。
		return resp.ID, fmt.Errorf("container start: %w", err)
	}
	return resp.ID, nil
}

// CreateVolume 创建命名卷（D-10：重名/非法名称由 daemon 拒绝→sdkError 归一）。
func (g *Gateway) CreateVolume(ctx context.Context, name, driver string) error {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	opts := volume.CreateOptions{Name: name}
	if driver != "" {
		opts.Driver = driver
	}
	if _, err := g.cli.VolumeCreate(cctx, opts); err != nil {
		return sdkError("volume create", err)
	}
	return nil
}

// CreateNetwork 创建自定义网络（D-10：子网/网关可选经 IPAM 承载）。
func (g *Gateway) CreateNetwork(ctx context.Context, name, driver, subnet, gateway string) error {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	opts := network.CreateOptions{
		Driver: driver,
	}
	if subnet != "" || gateway != "" {
		opts.IPAM = &network.IPAM{
			Config: []network.IPAMConfig{{Subnet: subnet, Gateway: gateway}},
		}
	}
	if _, err := g.cli.NetworkCreate(cctx, name, opts); err != nil {
		return sdkError("network create", err)
	}
	return nil
}

// ImageExists 本地镜像存在性（D-09 缺失自动拉取判定；ref 为名称[:tag]）。
func (g *Gateway) ImageExists(ctx context.Context, ref string) bool {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	_, _, err := g.cli.ImageInspectWithRaw(cctx, ref)
	return err == nil
}
