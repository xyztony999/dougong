// create.go —— docker-mgmt v1.1 创建类服务（api-docker-mgmt.md v1.1 §4.3
// FR-19~22 / D-09~D-11）：容器创建（docker run 语义：镜像缺失自动拉取→
// create+start）、卷创建、网络创建、Compose 项目创建（YAML 校验→落盘→up -d→
// 归属标签 ⚠️A22）。全部 admin 专属（路由层承载）、审计 success/failure 如实
// （I-06）、daemon 拒绝为最终权威（I-05 同构）。
package dockermgmt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dougong/dougong/internal/adapter/dockerx"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	"gopkg.in/yaml.v3"
)

// 编译期常量（api §3.4 D-11）。
const (
	ComposeRootDir  = "/opt/dougong-compose" // Compose 项目落盘根（防逃逸：项目名白名单+根前缀约束）
	ComposeFileName = "docker-compose.yml"
	// AppOwnershipLabel 归属标签键（⚠️A22：dougong.app={projectName}，供
	// app-mgmt 容器化应用标记归属；全网视图经标签区分但保持可见）。
	AppOwnershipLabel = "dougong.app"
	// ContainerNamePattern 容器名白名单（D-09；docker 自身约束的子集）。
	ContainerNamePattern = `^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`
)

var containerNameRe = regexp.MustCompile(ContainerNamePattern)

// ---------- 请求/响应 DTO ----------

// PortMappingDTO 端口映射（宿主:容器）。
type PortMappingDTO struct {
	HostPort      string `json:"hostPort"`
	ContainerPort string `json:"containerPort"`
	Protocol      string `json:"protocol,omitempty"`
}

// EnvKVDTO 环境变量键值。
type EnvKVDTO struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MountDTOMount 挂载。
type MountDTO struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Mode   string `json:"mode,omitempty"`
}

// ContainerCreateRequest POST /docker/container/create 请求。
type ContainerCreateRequest struct {
	Name          string           `json:"name"`
	Image         string           `json:"image"`
	PortMappings  []PortMappingDTO `json:"ports,omitempty"`
	Env           []EnvKVDTO       `json:"env,omitempty"`
	Mounts        []MountDTO       `json:"mounts,omitempty"`
	RestartPolicy string           `json:"restartPolicy,omitempty"`
	Network       string           `json:"network,omitempty"`
}

// ContainerCreateData 创建响应。
type ContainerCreateData struct {
	ContainerID string `json:"containerId"`
	Name        string `json:"name"`
	State       string `json:"state"`
}

// VolumeCreateRequest POST /docker/volume/create 请求。
type VolumeCreateRequest struct {
	Name   string `json:"name"`
	Driver string `json:"driver,omitempty"`
}

// VolumeCreateData 创建响应。
type VolumeCreateData struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
}

// NetworkCreateRequest POST /docker/network/create 请求。
type NetworkCreateRequest struct {
	Name    string `json:"name"`
	Driver  string `json:"driver,omitempty"`
	Subnet  string `json:"subnet,omitempty"`
	Gateway string `json:"gateway,omitempty"`
}

// NetworkCreateData 创建响应。
type NetworkCreateData struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
}

// ComposeCreateRequest POST /docker/compose/create 请求。
type ComposeCreateRequest struct {
	ProjectName string `json:"projectName"`
	ComposeYaml string `json:"composeYaml"`
	AutoUp      *bool  `json:"autoUp,omitempty"` // 缺省 true（D0 Q3 自动启动）
}

// ComposeCreateData 创建响应。
type ComposeCreateData struct {
	ProjectName string `json:"projectName"`
	WorkingDir  string `json:"workingDir"`
	ConfigFile  string `json:"configFile"`
	Started     bool   `json:"started"`
}

// ---------- 校验（D-09 逐项；R-14 服务端权威）----------

var portRe = regexp.MustCompile(`^[0-9]{1,5}$`)

func validPort(p string) bool {
	if !portRe.MatchString(p) {
		return false
	}
	n := 0
	for _, ch := range p {
		n = n*10 + int(ch-'0')
	}
	return n >= 1 && n <= 65535
}

func validEnvKV(kv string) bool {
	i := strings.Index(kv, "=")
	return i > 0 && !strings.Contains(kv[:i], " ")
}

// validateContainerCreate 逐项校验（顺序即 api §4.3 详情呈现顺序）。
func validateContainerCreate(req ContainerCreateRequest) error {
	if !containerNameRe.MatchString(req.Name) {
		return errcode.ErrDKContainerNameInvalid
	}
	if strings.TrimSpace(req.Image) == "" {
		return errcode.ErrDKContainerCreateFailed.WithData(map[string]any{"reason": "镜像必填"})
	}
	for _, p := range req.PortMappings {
		if !validPort(p.HostPort) || !validPort(p.ContainerPort) {
			return errcode.ErrDKPortMappingInvalid
		}
		if p.Protocol != "" && p.Protocol != "tcp" && p.Protocol != "udp" {
			return errcode.ErrDKPortMappingInvalid
		}
	}
	for _, e := range req.Env {
		if !validEnvKV(e.Key + "=" + e.Value) {
			return errcode.ErrDKEnvFormatInvalid
		}
	}
	for _, m := range req.Mounts {
		if m.Source == "" || m.Target == "" || strings.Contains(m.Source, "..") {
			return errcode.ErrDKMountFormatInvalid
		}
	}
	switch req.RestartPolicy {
	case "", "no", "always", "unless-stopped", "on-failure":
	default:
		return errcode.ErrDKContainerNameInvalid.WithData(map[string]any{"reason": "重启策略取值非法"})
	}
	return nil
}

// toAdapterSpec DTO → dockerx 创建参数（规范转化唯一出口）。
func toAdapterSpec(req ContainerCreateRequest) dockerx.CreateContainerSpec {
	spec := dockerx.CreateContainerSpec{
		Name:          req.Name,
		Image:         strings.TrimSpace(req.Image),
		RestartPolicy: req.RestartPolicy,
		NetworkName:   req.Network,
	}
	for _, p := range req.PortMappings {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		spec.PortMappings = append(spec.PortMappings, dockerx.PortMapping{
			HostPort: p.HostPort, ContainerPort: p.ContainerPort, Protocol: proto})
	}
	for _, e := range req.Env {
		if e.Key != "" {
			spec.Env = append(spec.Env, e.Key+"="+e.Value)
		}
	}
	for _, m := range req.Mounts {
		spec.Mounts = append(spec.Mounts, dockerx.MountSpec{Source: m.Source, Target: m.Target, Mode: m.Mode})
	}
	return spec
}

// ---------- 容器创建（FR-19/D-09）----------

// CreateContainer 创建并启动容器：校验 → 镜像缺失自动拉取（复用 pull 通道）
// → SDK Create+Start → 即读实况返回 → 审计（success/failure 如实 I-06）。
func (s *Service) CreateContainer(ctx context.Context, actor string, req ContainerCreateRequest) (*ContainerCreateData, error) {
	if err := validateContainerCreate(req); err != nil {
		s.auditFailure(ctx, actor, AuditContainerCreated, ObjectTypeContainer, req.Name,
			map[string]any{"name": req.Name, "image": req.Image}, err)
		return nil, err
	}
	spec := toAdapterSpec(req)
	// 镜像缺失自动拉取（D0 Q2；拉取失败 DK_013 透传不创建）。
	if !s.gw.ImageExists(ctx, spec.Image) {
		if _, err := s.gw.PullImage(ctx, spec.Image); err != nil {
			pullErr := errcode.ErrDkPullFailed.WithData(map[string]any{
				"reason": fmt.Sprintf("镜像 %s 拉取失败：%v", spec.Image, err)})
			s.auditFailure(ctx, actor, AuditContainerCreated, ObjectTypeContainer, req.Name,
				map[string]any{"name": req.Name, "image": spec.Image}, pullErr)
			return nil, pullErr
		}
	}
	id, err := s.gw.CreateContainer(ctx, spec)
	if err != nil {
		createErr := errcode.ErrDKContainerCreateFailed.WithData(map[string]any{"reason": err.Error()})
		s.auditFailure(ctx, actor, AuditContainerCreated, ObjectTypeContainer, req.Name,
			map[string]any{"name": req.Name, "image": spec.Image}, createErr)
		return nil, createErr
	}
	state, _ := s.gw.ContainerState(ctx, id)
	s.recordAudit(ctx, auditEntry(actor, AuditContainerCreated, ObjectTypeContainer, id,
		map[string]any{"name": req.Name, "image": spec.Image}, audit.ResultSuccess))
	return &ContainerCreateData{ContainerID: id, Name: req.Name, State: state}, nil
}

// ---------- 卷/网络创建（FR-20/21/D-10）----------

// CreateVolume 创建命名卷（D-10：daemon 拒绝 DK_024 透传）。
func (s *Service) CreateVolume(ctx context.Context, actor string, name, driver string) (*VolumeCreateData, error) {
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, "/\\") {
		err := errcode.ErrDKVolumeCreateFailed.WithData(map[string]any{"reason": "卷名非法"})
		s.auditFailure(ctx, actor, AuditVolumeCreated, ObjectTypeVolume, name,
			map[string]any{"name": name}, err)
		return nil, err
	}
	if driver == "" {
		driver = "local"
	}
	if err := s.gw.CreateVolume(ctx, name, driver); err != nil {
		vErr := errcode.ErrDKVolumeCreateFailed.WithData(map[string]any{"reason": err.Error()})
		s.auditFailure(ctx, actor, AuditVolumeCreated, ObjectTypeVolume, name,
			map[string]any{"name": name, "driver": driver}, vErr)
		return nil, vErr
	}
	s.recordAudit(ctx, auditEntry(actor, AuditVolumeCreated, ObjectTypeVolume, name,
		map[string]any{"name": name, "driver": driver}, audit.ResultSuccess))
	return &VolumeCreateData{Name: name, Driver: driver}, nil
}

// CreateNetwork 创建自定义网络（D-10：重名/子网冲突 DK_025 透传）。
func (s *Service) CreateNetwork(ctx context.Context, actor string, name, driver, subnet, gateway string) (*NetworkCreateData, error) {
	if name == "" || strings.Contains(name, "..") {
		err := errcode.ErrDKNetworkCreateFailed.WithData(map[string]any{"reason": "网络名非法"})
		s.auditFailure(ctx, actor, AuditNetworkCreated, ObjectTypeNetwork, name,
			map[string]any{"name": name}, err)
		return nil, err
	}
	if driver == "" {
		driver = "bridge"
	}
	if err := s.gw.CreateNetwork(ctx, name, driver, subnet, gateway); err != nil {
		nErr := errcode.ErrDKNetworkCreateFailed.WithData(map[string]any{"reason": err.Error()})
		s.auditFailure(ctx, actor, AuditNetworkCreated, ObjectTypeNetwork, name,
			map[string]any{"name": name, "driver": driver}, nErr)
		return nil, nErr
	}
	s.recordAudit(ctx, auditEntry(actor, AuditNetworkCreated, ObjectTypeNetwork, name,
		map[string]any{"name": name, "driver": driver}, audit.ResultSuccess))
	return &NetworkCreateData{Name: name, Driver: driver}, nil
}

// ---------- Compose 项目创建（FR-22/D-11）----------

var composeProjectRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// injectOwnershipLabel 向 YAML labels 段注入 dougong.app 归属标签（⚠️A22 预埋；
// compose.yml 已含 labels 段时原样保留——预埋标签由文件顶层 labels 承载，
// 与服务级 labels 并存合法）。
func injectOwnershipLabel(yamlText, projectName string) string {
	if strings.Contains(yamlText, "dougong.app:") {
		return yamlText // 已有归属标记不重复注入
	}
	labelLine := "labels:\n  - \"dougong.app=" + projectName + "\""
	// 顶层追加（文件级 labels 与服务级 labels 均为合法 compose 语法）。
	if !strings.HasSuffix(yamlText, "\n") {
		yamlText += "\n"
	}
	return yamlText + labelLine + "\n"
}

// validateComposeYAML YAML 语法校验（失败 DK_026 data.reason 带行号）。
func validateComposeYAML(text string) error {
	var doc any
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return errcode.ErrDKComposeYAMLInvalid.WithData(map[string]any{"reason": err.Error()})
	}
	return nil
}

// yamlTypeError 携带行号的 YAML 解析错误（go-yaml TypeError）。
type yamlTypeError struct{ msg string }

func (e *yamlTypeError) Error() string { return e.msg }

// CreateComposeProject 新建 Compose 项目（D-11）：项目名白名单 → YAML 校验 →
// 落盘（目录防逃逸+原子写）→ autoUp（缺省 true）经 ComposeRunner up -d（互斥
// DK_018；up 失败不回滚落盘——项目可在列表重试 up）→ 审计（不含 compose 全文）。
func (s *Service) CreateComposeProject(ctx context.Context, actor string, projectName, composeYaml string, autoUp *bool) (*ComposeCreateData, error) {
	start := true
	if autoUp != nil {
		start = *autoUp
	}
	fail := func(err error, codeAudit string) error {
		s.auditFailure(ctx, actor, codeAudit, ObjectTypeComposeProject, projectName,
			map[string]any{"project": projectName}, err)
		return err
	}
	if !composeProjectRe.MatchString(projectName) {
		return nil, fail(errcode.ErrDKComposeYAMLInvalid.WithData(map[string]any{
			"reason": "项目名须为小写字母/数字/连字符（不超过 64 个字符）"}), AuditComposeProjectCreated)
	}
	if err := validateComposeYAML(composeYaml); err != nil {
		return nil, fail(err, AuditComposeProjectCreated)
	}
	workDir := filepath.Join(ComposeRootDir, projectName)
	if _, err := os.Stat(workDir); err == nil {
		return nil, fail(errcode.ErrDKComposeProjectExists.WithData(map[string]any{
			"projectName": projectName}), AuditComposeProjectCreated)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fail(fmt.Errorf("创建目录失败：%w", err), AuditComposeProjectCreated)
	}
	configFile := filepath.Join(workDir, ComposeFileName)
	yamlText := injectOwnershipLabel(composeYaml, projectName)
	if err := os.WriteFile(configFile, []byte(yamlText), 0o644); err != nil {
		return nil, fail(fmt.Errorf("写入 compose.yml 失败：%w", err), AuditComposeProjectCreated)
	}
	started := false
	if start {
		fact := dockerx.ComposeProjectFact{
			Project: projectName, WorkingDir: workDir, ConfigFile: configFile, Locatable: true}
		if err := s.comp.Run(ctx, fact, "up -d"); err != nil {
			// up 失败不回滚落盘（项目可在 Compose 列表重试启动，D-11）。
			upErr := errcode.ErrDkComposeExecFailed.WithData(map[string]any{"reason": err.Error()})
			return nil, fail(upErr, AuditComposeProjectCreated)
		}
		started = true
	}
	s.recordAudit(ctx, auditEntry(actor, AuditComposeProjectCreated, ObjectTypeComposeProject, projectName,
		map[string]any{"project": projectName, "workingDir": workDir, "started": started}, audit.ResultSuccess))
	return &ComposeCreateData{
		ProjectName: projectName, WorkingDir: workDir,
		ConfigFile: configFile, Started: started,
	}, nil
}
