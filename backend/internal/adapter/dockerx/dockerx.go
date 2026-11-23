// Package dockerx 是本机 Docker daemon 适配器（api-docker-mgmt.md §4.1.5/
// §4.2.5/§4.3.5 契约承载；idea.md §5.3 dockerx 落位）。双通道：
//
//   - SDK 通道（DockerGateway）：容器/镜像/卷/网络经官方 Go SDK
//     （github.com/docker/docker/client，/var/run/docker.sock——api §1.5 明文）；
//   - CLI 通道（ComposeRunner）：`docker compose` 经 exec.CommandContext
//     字面量参数数组执行（R-08/ADR-013 禁 shell 拼接；compose.go）。
//
// 零持久化（D-10）：全部数据为当次 daemon 实况读取快照（I-01/I-02 无缓存层）。
// 超时编译期常量（api §3.4）：SDK 单操作 60s / pull 600s / compose CLI 300s，
// 超时 ctx 截断→通道错误→服务层归一 DK_017/DK_013/DK_016（A-D17）。
//
// daemon 原生语义如实上抛（D-08）：NotFound/Conflict/Forbidden 经 SDK errdefs
// 归一为哨兵（ErrNotFound/ErrInUse/ErrPredefinedNetwork），其余原文包装进
// ExecError（.Reason 原文透传不虚构）；等价/不存在不静默成功。
package dockerx

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

// 编译期常量（api §3.4 业务常量表）。
const (
	// SDKTimeout 单操作等待上限（60s；超时归一 DK_017）。
	SDKTimeout = 60 * time.Second
	// PullTimeout daemon pull 同步等待上限（600s，D-05；超时归一 DK_013）。
	PullTimeout = 600 * time.Second
	// ProbeTimeout 探测等待上限（探测为轻量本机判定，快速失败利于引导页呈现）。
	ProbeTimeout = 5 * time.Second
)

// daemon 接管结论细节（D-01 透传属性：非枚举池条目，前端按值区分未装/不可达）。
const (
	DetailNotInstalled      = "not_installed"
	DetailSocketUnreachable = "socket_unreachable"
)

// 接管判定值（docker_daemon_state 枚举：available | unavailable）。
const (
	StateAvailable   = "available"
	StateUnavailable = "unavailable"
)

// 容器状态（container_status 枚举 + 过渡态原始值，BA 附录B.1 映射归并）。
const (
	StatusRunning = "running"
	StatusPaused  = "paused"
	StatusStopped = "stopped"
)

// 哨兵错误（服务层翻译为 DK_ 错误码语义，D-08）。
var (
	// ErrNotFound 资源不存在（daemon 404——面板外已删，不静默成功）
	// → DK_003/005/007/009/014。
	ErrNotFound = errors.New("dockerx: resource not found")
	// ErrInUse 资源被引用/挂载/接入（daemon 409 conflict）→ DK_006/008/010。
	ErrInUse = errors.New("dockerx: resource in use")
	// ErrPredefinedNetwork 预定义网络不可删除（daemon 403）→ DK_011。
	ErrPredefinedNetwork = errors.New("dockerx: predefined network")
	// ErrBusy 同项目 Compose 操作进行中（进程内 per-project 互斥，I-08）→ DK_018。
	ErrBusy = errors.New("dockerx: compose operation busy")
)

// ExecError 是通道失败的原文载体（D-08：reason 原 stderr/daemon 错误透传，
// 不虚构；服务层按调用面归一 DK_013/DK_016/DK_017）。
type ExecError struct {
	Verb   string // 动作语境（如 "pull"/"compose up"/"stats"）
	Reason string // daemon/CLI 原文
}

// Error 实现 error 接口。
func (e *ExecError) Error() string {
	return fmt.Sprintf("dockerx: %s failed: %s", e.Verb, e.Reason)
}

// DaemonInfo 是接管探测结论（api §4.1.1 接管结论条目数据源，D-01）。
type DaemonInfo struct {
	State   string  // available | unavailable
	Version *string // available 时伴随（能力代际识别）；unavailable 为 nil
	Detail  *string // unavailable 时探测结论细节（not_installed / socket_unreachable）
}

// ContainerFact 是容器实况投影条目（api §4.1.1 容器投影字段清单数据源）。
type ContainerFact struct {
	ContainerID    string // daemon 64-hex ID（D-11 不透明引用）
	Name           string
	Image          string // 镜像引用（名称:tag）
	State          string // 映射态（running/stopped/paused）或过渡态原始值
	ComposeProject string // label 透传；空=无项目（投影层转 null）
	Labels         map[string]string
}

// ImageFact 是镜像投影条目（api §4.2.1）。
type ImageFact struct {
	ImageID   string
	Ref       string // 名称:tag（无 tag 为 "<none>"，A-D6）
	SizeBytes int64
}

// VolumeFact 是卷投影条目（api §4.2.1；sizeBytes 经 SystemDF，A-D5）。
type VolumeFact struct {
	VolumeName string
	Mountpoint string
	SizeBytes  int64
	Driver     string
}

// NetworkFact 是网络投影条目（api §4.2.1）。
type NetworkFact struct {
	NetworkID string
	Name      string
	Driver    string
	Subnet    string // 首个 IPAM 子网；无 IPAM 配置为空
}

// StatsFact 是单次资源占用快照（api §4.1.1 快照条目，D-07；过程数据）。
type StatsFact struct {
	CPUPercent   float64
	MemUsageByte uint64
	MemLimitByte uint64
	MemPercent   float64
}

// 容器生命周期动作（BA 附录B.2 六动作，业务常量非枚举池条目）。
const (
	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"
	ActionPause   = "pause"
	ActionUnpause = "unpause"
	ActionDelete  = "delete"
)

// Gateway 是 Docker daemon SDK 网关（api §4.1.5/§4.2.5 DockerGateway 契约
// 实现；无状态——每次调用即时读取，I-01/I-03 唯一 SDK 出口）。
type Gateway struct {
	cli *client.Client
}

// New 构造 SDK 网关（FromEnv：DOCKER_HOST 可覆盖默认 unix socket——运维
// 逃生通道；API 版本自动协商）。构造失败仅发生在宿主解析失败等启动期问题。
func New() (*Gateway, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("dockerx: new client: %w", err)
	}
	return &Gateway{cli: cli}, nil
}

// Close 释放底层 HTTP 连接（main 优雅退出时调用）。
func (g *Gateway) Close() error { return g.cli.Close() }

// socketPath 返回当前 host 的 unix socket 文件路径（非 unix 协议返回空）。
func (g *Gateway) socketPath() string {
	host := g.cli.DaemonHost()
	if strings.HasPrefix(host, "unix://") {
		return strings.TrimPrefix(host, "unix://")
	}
	return ""
}

// ProbeDaemon 即时探测接管结论（D-01：每次调用即时判定，无缓存）。
// 判定链：unix socket 时 stat 路径（不存在→not_installed，A-D3）→ Ping
// （失败→socket_unreachable）→ ServerVersion（available 伴随版本）。
// 探测结论恒有值返回（error 仅保留给编程性异常）。
func (g *Gateway) ProbeDaemon(ctx context.Context) DaemonInfo {
	pctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()

	if p := g.socketPath(); p != "" {
		if _, err := os.Stat(p); err != nil {
			return unavailable(nil, DetailNotInstalled)
		}
	}
	if _, err := g.cli.Ping(pctx); err != nil {
		return unavailable(nil, DetailSocketUnreachable)
	}
	v, err := g.cli.ServerVersion(pctx)
	if err != nil {
		// Ping 通而版本读取失败：判定 available（接管成立），版本缺省 nil。
		return DaemonInfo{State: StateAvailable}
	}
	ver := v.Version
	return DaemonInfo{State: StateAvailable, Version: &ver}
}

// unavailable 组装不可用结论（detail 透传属性，A-D3）。
func unavailable(version *string, detail string) DaemonInfo {
	d := detail
	return DaemonInfo{State: StateUnavailable, Version: version, Detail: &d}
}

// withTimeout 施加单操作超时（A-D17）。
func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}

// ListContainers 即时读取全量容器实况（I-01/I-02：当次快照，含 All 停止态）。
func (g *Gateway) ListContainers(ctx context.Context) ([]ContainerFact, error) {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	list, err := g.cli.ContainerList(cctx, container.ListOptions{All: true})
	if err != nil {
		return nil, sdkError("container list", err)
	}
	facts := make([]ContainerFact, 0, len(list))
	for _, c := range list {
		facts = append(facts, ContainerFact{
			ContainerID:    c.ID,
			Name:           containerName(c.Names),
			Image:          c.Image,
			State:          MapState(c.State),
			ComposeProject: c.Labels[LabelProject],
			Labels:         c.Labels,
		})
	}
	return facts, nil
}

// ContainerState 单容器终态复核（D-04/A-D18：动作后即读实况返回映射态）。
func (g *Gateway) ContainerState(ctx context.Context, id string) (string, error) {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	insp, err := g.cli.ContainerInspect(cctx, id)
	if err != nil {
		return "", sdkError("container inspect", err)
	}
	return MapState(insp.State.Status), nil
}

// ContainerExists 存在性实况复核（DK_003 判定；不匹配任何 ID 前缀语义——
// id 为前端透传的不透明引用原样下发 daemon）。
func (g *Gateway) ContainerExists(ctx context.Context, id string) (bool, error) {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	_, err := g.cli.ContainerInspect(cctx, id)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, sdkError("container inspect", err)
	}
	return true, nil
}

// GetContainerLogs 尾部日志读取（D-03 REST 通道；tail 行数，无时间戳）。
func (g *Gateway) GetContainerLogs(ctx context.Context, id string, tail int) ([]string, error) {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	rc, err := g.cli.ContainerLogs(cctx, id, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Tail: tailStr(tail),
	})
	if err != nil {
		return nil, sdkError("container logs", err)
	}
	defer rc.Close()
	return readLogLines(rc)
}

// GetContainerStats 单次快照（D-07：CPU%/内存 usage/limit/percent 一次读取）。
func (g *Gateway) GetContainerStats(ctx context.Context, id string) (StatsFact, error) {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	reader, err := g.cli.ContainerStats(cctx, id, false)
	if err != nil {
		return StatsFact{}, sdkError("container stats", err)
	}
	defer reader.Body.Close()
	var v container.StatsResponse
	if err := jsonDecode(reader.Body, &v); err != nil {
		return StatsFact{}, &ExecError{Verb: "container stats", Reason: err.Error()}
	}
	return computeStats(v), nil
}

// ContainerAction 生命周期动作下发（start/stop/restart/pause/unpause；
// SDK 直达——矩阵校验在服务层 D-04，此处不重复判定）。
func (g *Gateway) ContainerAction(ctx context.Context, id, action string) error {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	var err error
	switch action {
	case ActionStart:
		err = g.cli.ContainerStart(cctx, id, container.StartOptions{})
	case ActionStop:
		err = g.cli.ContainerStop(cctx, id, container.StopOptions{})
	case ActionRestart:
		err = g.cli.ContainerRestart(cctx, id, container.StopOptions{})
	case ActionPause:
		err = g.cli.ContainerPause(cctx, id)
	case ActionUnpause:
		err = g.cli.ContainerUnpause(cctx, id)
	default:
		return fmt.Errorf("dockerx: unknown container action %q", action)
	}
	if err != nil {
		return sdkError("container "+action, err)
	}
	return nil
}

// DeleteContainer 删除容器（无 force 参数——I-04/I-05 MVP 无强制删除；
// running 态由服务层矩阵前置拒绝，竞态窗口 daemon 拒绝如实归一）。
func (g *Gateway) DeleteContainer(ctx context.Context, id string) error {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	if err := g.cli.ContainerRemove(cctx, id, container.RemoveOptions{}); err != nil {
		return sdkError("container delete", err)
	}
	return nil
}

// ListImages 即时读取镜像投影（A-D6：每 RepoTag 一条；docker CLI 同构）。
func (g *Gateway) ListImages(ctx context.Context) ([]ImageFact, error) {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	list, err := g.cli.ImageList(cctx, image.ListOptions{})
	if err != nil {
		return nil, sdkError("image list", err)
	}
	facts := make([]ImageFact, 0, len(list))
	for _, img := range list {
		refs := img.RepoTags
		if len(refs) == 0 {
			refs = []string{"<none>"} // 无 tag 镜像如实单行呈现（A-D6）
		}
		for _, ref := range refs {
			facts = append(facts, ImageFact{ImageID: img.ID, Ref: ref, SizeBytes: img.Size})
		}
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].Ref < facts[j].Ref })
	return facts, nil
}

// PullImage 同步拉取（D-05：阻塞至完成/失败，≤600s；daemon 进度流中
// error 消息原文透传；成功后 inspect 取 imageId，ref 回显请求原值 A-D7）。
func (g *Gateway) PullImage(ctx context.Context, ref string) (ImageFact, error) {
	pctx, cancel := withTimeout(ctx, PullTimeout)
	defer cancel()
	rc, err := g.cli.ImagePull(pctx, ref, image.PullOptions{})
	if err != nil {
		return ImageFact{}, sdkError("image pull", err)
	}
	defer rc.Close()
	if reason := scanPullError(rc); reason != "" {
		return ImageFact{}, &ExecError{Verb: "image pull", Reason: reason}
	}
	cctx, cancel2 := withTimeout(ctx, SDKTimeout)
	defer cancel2()
	insp, _, err := g.cli.ImageInspectWithRaw(cctx, ref)
	if err != nil {
		return ImageFact{}, sdkError("image inspect", err)
	}
	return ImageFact{ImageID: insp.ID, Ref: ref}, nil
}

// DeleteImage 删除镜像（被引用由 daemon 拒绝→ErrInUse，I-05 无 force）。
func (g *Gateway) DeleteImage(ctx context.Context, id string) error {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	if _, err := g.cli.ImageRemove(cctx, id, image.RemoveOptions{}); err != nil {
		return sdkError("image delete", err)
	}
	return nil
}

// ListVolumes 即时读取卷投影（sizeBytes 经 SystemDF 合并，A-D5）。
func (g *Gateway) ListVolumes(ctx context.Context) ([]VolumeFact, error) {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	resp, err := g.cli.VolumeList(cctx, volume.ListOptions{})
	if err != nil {
		return nil, sdkError("volume list", err)
	}
	// 辅通道 DiskUsage（/system/df）取 daemon 计算的卷用量（VolumeList API
	// 不回 UsageData）；失败降级 sizeBytes=0 不阻断主清单（A-D5）。
	sizes := map[string]int64{}
	dfctx, dfcancel := withTimeout(ctx, SDKTimeout)
	if df, err := g.cli.DiskUsage(dfctx, types.DiskUsageOptions{}); err == nil {
		for _, v := range df.Volumes {
			if v != nil && v.UsageData != nil {
				sizes[v.Name] = v.UsageData.Size
			}
		}
	}
	dfcancel()
	facts := make([]VolumeFact, 0, len(resp.Volumes))
	for _, v := range resp.Volumes {
		if v == nil {
			continue
		}
		facts = append(facts, VolumeFact{
			VolumeName: v.Name, Mountpoint: v.Mountpoint,
			SizeBytes: sizes[v.Name], Driver: v.Driver,
		})
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].VolumeName < facts[j].VolumeName })
	return facts, nil
}

// DeleteVolume 删除卷（被挂载由 daemon 拒绝→ErrInUse，无 force I-05）。
func (g *Gateway) DeleteVolume(ctx context.Context, name string) error {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	if err := g.cli.VolumeRemove(cctx, name, false); err != nil {
		return sdkError("volume delete", err)
	}
	return nil
}

// ListNetworks 即时读取网络投影（预定义网络如实呈现，删除时 DK_011）。
func (g *Gateway) ListNetworks(ctx context.Context) ([]NetworkFact, error) {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	list, err := g.cli.NetworkList(cctx, network.ListOptions{})
	if err != nil {
		return nil, sdkError("network list", err)
	}
	facts := make([]NetworkFact, 0, len(list))
	for _, n := range list {
		facts = append(facts, NetworkFact{
			NetworkID: n.ID, Name: n.Name, Driver: n.Driver,
			Subnet: firstSubnet(n.IPAM),
		})
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].Name < facts[j].Name })
	return facts, nil
}

// DeleteNetwork 删除网络（预定义→ErrPredefinedNetwork；被接入→ErrInUse，D-08；
// 两者均为 daemon 403，按报错原文区分——见 mapNetworkRemoveError）。
func (g *Gateway) DeleteNetwork(ctx context.Context, id string) error {
	cctx, cancel := withTimeout(ctx, SDKTimeout)
	defer cancel()
	if err := g.cli.NetworkRemove(cctx, id); err != nil {
		return mapNetworkRemoveError(err)
	}
	return nil
}

// mapNetworkRemoveError 归一 NetworkRemove 的 daemon 拒绝。
//
// 实测口径（Docker 29.8.1，2026-11-23 探针）：daemon 对「预定义网络」与「有活动
// 端点的网络」都回 403 Forbidden——
//   - "bridge is a pre-defined network and cannot be removed"
//   - "error while removing network: network x has active endpoints (name:\"c\" id:…)"
//
// 仅凭 errdefs 类别分不开。此前一律经 sdkError 归为 ErrPredefinedNetwork，删除被
// 接入的网络时用户看到的是「预定义网络不可删除」（DK_011）而非「被容器接入」
// （DK_010）。这里按 daemon 原文区分：含 "pre-defined network" 为预定义，其余
// Forbidden 为被接入。
func mapNetworkRemoveError(err error) error {
	if errdefs.IsForbidden(err) && !strings.Contains(err.Error(), "pre-defined network") {
		return fmt.Errorf("%w: network delete: %v", ErrInUse, err)
	}
	return sdkError("network delete", err)
}

// scanPullError 消费 daemon 拉取进度流并提取失败消息（D-05/D-08：tag 不
// 存在/网络不通/限流等 daemon 错误原文透传；流正常走完返回空=成功）。
func scanPullError(rc io.Reader) string {
	type pullMessage struct {
		Error string `json:"error"`
	}
	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var msg pullMessage
		if json.Unmarshal(scanner.Bytes(), &msg) == nil && msg.Error != "" {
			return msg.Error
		}
	}
	return ""
}

// sdkError 将 SDK 错误按 errdefs 语义归一（D-08）：NotFound→ErrNotFound、
// Conflict→ErrInUse、Forbidden→ErrPredefinedNetwork（其余调用面 Forbidden 罕见，
// 归一后由服务层按面翻译）、其余原文 ExecError。NotFound/Conflict/Forbidden 之外
// 的原文包装保留上下文。
// 注意：网络删除不经此处的 Forbidden 分支——被接入的网络同样回 403，须按原文
// 区分，见 mapNetworkRemoveError。
func sdkError(verb string, err error) error {
	switch {
	case errdefs.IsNotFound(err):
		return fmt.Errorf("%w: %s: %v", ErrNotFound, verb, err)
	case errdefs.IsConflict(err):
		return fmt.Errorf("%w: %s: %v", ErrInUse, verb, err)
	case errdefs.IsForbidden(err):
		return fmt.Errorf("%w: %s: %v", ErrPredefinedNetwork, verb, err)
	default:
		return &ExecError{Verb: verb, Reason: err.Error()}
	}
}

// containerName 取容器名（daemon Names 带 "/" 前缀，取首条并剥前缀）。
func containerName(names []string) string {
	for _, n := range names {
		return strings.TrimPrefix(n, "/")
	}
	return ""
}

// firstSubnet 取首个 IPAM 子网（无配置为空）。
func firstSubnet(ipam network.IPAM) string {
	for _, cfg := range ipam.Config {
		if cfg.Subnet != "" {
			return cfg.Subnet
		}
	}
	return ""
}

// computeStats 按 docker CLI 同源公式计算单次快照（D-07）。
func computeStats(v container.StatsResponse) StatsFact {
	fact := StatsFact{
		MemUsageByte: v.MemoryStats.Usage,
		MemLimitByte: v.MemoryStats.Limit,
	}
	// cgroup v2：usage 需扣除 inactive_file（docker CLI 同源口径）。
	if inactive, ok := v.MemoryStats.Stats["inactive_file"]; ok && inactive < v.MemoryStats.Usage {
		fact.MemUsageByte = v.MemoryStats.Usage - inactive
	}
	if v.MemoryStats.Limit > 0 {
		fact.MemPercent = float64(fact.MemUsageByte) / float64(v.MemoryStats.Limit) * 100
	}
	cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage) - float64(v.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(v.CPUStats.SystemUsage) - float64(v.PreCPUStats.SystemUsage)
	online := v.CPUStats.OnlineCPUs
	if online == 0 {
		online = uint32(len(v.CPUStats.CPUUsage.PercpuUsage))
	}
	if sysDelta > 0 && cpuDelta > 0 && online > 0 {
		fact.CPUPercent = cpuDelta / sysDelta * float64(online) * 100
	}
	return fact
}

// jsonDecode 解析 daemon JSON 响应体（stats 单次读取）。
func jsonDecode(r io.Reader, v any) error {
	return json.NewDecoder(r).Decode(v)
}
