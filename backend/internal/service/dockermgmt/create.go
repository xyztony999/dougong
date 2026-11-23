// create.go —— docker-mgmt v1.1 创建类服务（api-docker-mgmt.md v1.1 §4.3
// FR-19~22 / D-09~D-11）：容器创建（docker run 语义：镜像缺失自动拉取→
// create+start）、卷创建、网络创建、Compose 项目创建（YAML 校验→落盘→up -d→
// 归属标签 ⚠️A22）。全部 admin 专属（路由层承载）、审计 success/failure 如实
// （I-06）、daemon 拒绝为最终权威（I-05 同构）。
package dockermgmt

import (
	"bytes"
	"context"
	"errors"
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
	ComposeFileName = "docker-compose.yml"
	// DefaultComposeRootDir 是 Compose 项目落盘根的生产默认值（防逃逸：项目名
	// 白名单 + 根前缀约束）。服务内实际取值经 WithComposeRootDir 注入（main
	// 装配来自配置 docker.compose_root，默认即本值）——仅测试隔离与本机验证
	// 需要覆盖，生产部署保持默认。
	DefaultComposeRootDir = "/opt/dougong-compose"
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

// validateContainerCreate 逐项校验（顺序即 api §4.3 详情呈现顺序）。格式类
// 失败按语义归码：名称/端口/环境变量/挂载各有专属码（DK_019~022）；镜像必填与
// 重启策略取值属参数错误归 DK_001（此前分别误归 DK_023/DK_019——重启策略非法
// 会向用户呈现「容器名格式非法」）。
func validateContainerCreate(req ContainerCreateRequest) error {
	if !containerNameRe.MatchString(req.Name) {
		return errcode.ErrDKContainerNameInvalid
	}
	if strings.TrimSpace(req.Image) == "" {
		return errcode.ErrDkInvalidParam.WithData(map[string]any{"reason": "镜像必填"})
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
		return errcode.ErrDkInvalidParam.WithData(map[string]any{"reason": "重启策略取值非法"})
	}
	return nil
}

// ensureNetworkExists 网络名存在于清单（api D-09：不存在 → DK_009，在创建前
// 拦截而不是留给 daemon 以 DK_023 泛化失败）。清单读取失败不拦截——交由
// daemon 在创建时给出最终结论（I-05 daemon 为最终权威）。
func (s *Service) ensureNetworkExists(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}
	networks, err := s.gw.ListNetworks(ctx)
	if err != nil {
		return nil
	}
	for _, n := range networks {
		if n.Name == name {
			return nil
		}
	}
	return errcode.ErrDkNetworkNotFound
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
	if err := s.ensureNetworkExists(ctx, req.Network); err != nil {
		s.auditFailure(ctx, actor, AuditContainerCreated, ObjectTypeContainer, req.Name,
			map[string]any{"name": req.Name, "image": req.Image, "network": req.Network}, err)
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

// dereference 跟随 YAML 别名（`services: *anchor`、`web: *base` 等写法）。
func dereference(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	return n
}

// mappingEntry 取映射节点中指定键的原始值节点（键按字面量比较；值可能是别名节点，
// 由调用方决定是否解引用）；不存在返回 ok=false。
func mappingEntry(m *yaml.Node, key string) (raw *yaml.Node, ok bool) {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if dereference(m.Content[i]).Value == key {
			return m.Content[i+1], true
		}
	}
	return nil, false
}

// mappingValue 取映射节点中指定键的值节点（已解引用）；不存在返回 nil。
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	raw, ok := mappingEntry(m, key)
	if !ok {
		return nil
	}
	return dereference(raw)
}

func scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// replaceMappingValue 替换映射中指定键的值节点（键须已存在）。
func replaceMappingValue(m *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if dereference(m.Content[i]).Value == key {
			m.Content[i+1] = value
			return
		}
	}
}

// isNullNode 判定 YAML 空值节点（`labels:` 后无内容）。
func isNullNode(n *yaml.Node) bool {
	return n != nil && n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}

// serviceHasOwnershipLabel 服务是否已声明归属标签键（声明过则不改写用户内容）。
func serviceHasOwnershipLabel(svc *yaml.Node) bool {
	labels := mappingValue(svc, "labels")
	switch {
	case labels == nil || isNullNode(labels):
		return false
	case labels.Kind == yaml.SequenceNode:
		for _, item := range labels.Content {
			if v := dereference(item).Value; v == AppOwnershipLabel ||
				strings.HasPrefix(v, AppOwnershipLabel+"=") {
				return true
			}
		}
		return false
	case labels.Kind == yaml.MappingNode:
		return mappingValue(labels, AppOwnershipLabel) != nil
	}
	return false
}

// appendOwnershipLabel 向单个服务追加归属标签：labels 为列表 → 追加一项；为键值对
// → 追加一键；缺失或为空 → 新建列表。labels 经别名共享（`labels: *common`）时不
// 改动被共享的节点——复制出该服务独立的一份再追加，避免连带改写其它服务。
func appendOwnershipLabel(svc *yaml.Node, projectName string) error {
	item := scalar(AppOwnershipLabel + "=" + projectName)
	raw, ok := mappingEntry(svc, "labels")
	if !ok || isNullNode(dereference(raw)) {
		svc.Content = append(svc.Content, scalar("labels"), &yaml.Node{
			Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{item}})
		return nil
	}
	labels := dereference(raw)
	switch labels.Kind {
	case yaml.SequenceNode:
		if raw.Kind == yaml.AliasNode {
			seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			seq.Content = append(append(seq.Content, labels.Content...), item)
			replaceMappingValue(svc, "labels", seq)
			return nil
		}
		labels.Content = append(labels.Content, item)
		return nil
	case yaml.MappingNode:
		if raw.Kind == yaml.AliasNode {
			cp := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			cp.Content = append(cp.Content, labels.Content...)
			labels = cp
			replaceMappingValue(svc, "labels", cp)
		}
		labels.Content = append(labels.Content, scalar(AppOwnershipLabel), scalar(projectName))
		return nil
	}
	return errors.New("labels 段必须是列表或键值对")
}

// prepareComposeFile 是落盘前处理：YAML 解析 → 结构校验（顶层为映射、services 为
// 非空映射、逐服务为映射）→ 服务级归属标签注入 → 重新编码（保留注释）。全部在
// 创建目录之前完成，任何一步失败都不留落盘产物（D-11）。
//
// 归属标签必须落在服务级 labels（⚠️A22）：compose 规范没有顶层 labels 键，写在
// 顶层会被 CLI 拒绝——`validating …: additional properties 'labels' not allowed`
// （2026-11-23 以 Compose v5.5.1 实测复现；此前实现在顶层追加，导致每个新建
// 项目在 up 阶段必失败）。服务级标签由 compose 写入该服务的容器 label，全网视图
// 据此区分归属。
func prepareComposeFile(composeYaml, projectName string) (string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(composeYaml), &root); err != nil {
		return "", fmt.Errorf("YAML 语法错误：%w", err) // go-yaml 错误自带行号
	}
	if len(root.Content) == 0 {
		return "", errors.New("内容为空")
	}
	top := dereference(root.Content[0])
	if top.Kind != yaml.MappingNode {
		return "", errors.New("顶层必须是键值对（需要 services 段）")
	}
	servicesRaw, ok := mappingEntry(top, "services")
	if !ok {
		return "", errors.New("缺少 services 段")
	}
	services := dereference(servicesRaw)
	if services.Kind != yaml.MappingNode || len(services.Content) == 0 {
		return "", errors.New("services 段必须包含至少一个服务")
	}
	for i := 0; i+1 < len(services.Content); i += 2 {
		name := dereference(services.Content[i]).Value
		svc := dereference(services.Content[i+1])
		if svc.Kind != yaml.MappingNode {
			return "", fmt.Errorf("服务 %s 的定义必须是键值对", name)
		}
		if serviceHasOwnershipLabel(svc) {
			continue
		}
		if err := appendOwnershipLabel(services.Content[i+1], projectName); err != nil {
			return "", fmt.Errorf("服务 %s：%w", name, err)
		}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return "", fmt.Errorf("compose.yml 重新编码失败：%w", err)
	}
	_ = enc.Close()
	return buf.String(), nil
}

// writeFileAtomic 原子写（D-11：同目录临时文件 + rename；中途失败不留半截文件，
// 也不会被并发的 compose 进程读到写了一半的配置）。
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// projectHasContainers 项目当前是否有容器。Compose 清单按容器 label 聚合——零容器
// 的项目在清单中不可见。读取失败时无法判定，按「有」处理：宁可保留目录也不误删。
func (s *Service) projectHasContainers(ctx context.Context, project string) bool {
	facts, err := s.comp.ListProjectFacts(ctx)
	if err != nil {
		return true
	}
	for _, f := range facts {
		if f.Project == project {
			return len(f.Containers) > 0
		}
	}
	return false
}

// CreateComposeProject 新建 Compose 项目（D-11）：项目名白名单 → 解析+结构校验+
// 归属标签注入（全部在落盘前）→ 目录创建 + 原子写 → autoUp（缺省 true）经
// ComposeRunner up -d（互斥 DK_018）→ 审计（不含 compose 全文）。
//
// up 失败的落盘处置：项目已有容器 → 保留（清单可见，可在 Compose 页重试 up）；
// 项目零容器 → 清理本次落盘。D-11「up 失败不回滚」的前提是「项目可在列表重试
// up」，而零容器项目在列表中不可见，保留目录只会占住项目名（再建同名报 DK_027）
// 且用户无从清理。
func (s *Service) CreateComposeProject(ctx context.Context, actor string, projectName, composeYaml string, autoUp *bool) (*ComposeCreateData, error) {
	start := autoUp == nil || *autoUp
	fail := func(err error, codeAudit string) error {
		s.auditFailure(ctx, actor, codeAudit, ObjectTypeComposeProject, projectName,
			map[string]any{"project": projectName}, err)
		return err
	}
	if !composeProjectRe.MatchString(projectName) {
		return nil, fail(errcode.ErrDkInvalidParam.WithData(map[string]any{
			"reason": "项目名须为小写字母/数字/连字符（不超过 64 个字符）"}), AuditComposeProjectCreated)
	}
	// 解析/结构校验/标签注入先于建目录：失败时磁盘上不留任何东西。
	yamlText, err := prepareComposeFile(composeYaml, projectName)
	if err != nil {
		return nil, fail(errcode.ErrDKComposeYAMLInvalid.WithData(map[string]any{
			"reason": err.Error()}), AuditComposeProjectCreated)
	}
	workDir := filepath.Join(s.composeRoot, projectName)
	if _, err := os.Stat(workDir); err == nil {
		return nil, fail(errcode.ErrDKComposeProjectExists.WithData(map[string]any{
			"projectName": projectName}), AuditComposeProjectCreated)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fail(fmt.Errorf("创建目录失败：%w", err), AuditComposeProjectCreated)
	}
	configFile := filepath.Join(workDir, ComposeFileName)
	if err := writeFileAtomic(configFile, []byte(yamlText), 0o644); err != nil {
		_ = os.RemoveAll(workDir) // 本次调用新建的目录（上方已确认此前不存在）
		return nil, fail(fmt.Errorf("写入 compose.yml 失败：%w", err), AuditComposeProjectCreated)
	}
	started := false
	if start {
		fact := dockerx.ComposeProjectFact{
			Project: projectName, WorkingDir: workDir, ConfigFile: configFile, Locatable: true}
		// 动词用 VerbUp 常量：-d 由 composeArgs 追加在 up 之后。此前传字面量
		// "up -d"，被 ComposeRunner.Run 的动词白名单直接拒绝（unknown compose verb），
		// 新建项目的自动启动自 v0.3.0 起必失败。
		if err := s.comp.Run(ctx, fact, dockerx.VerbUp); err != nil {
			if !s.projectHasContainers(ctx, projectName) {
				_ = os.RemoveAll(workDir)
			}
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
