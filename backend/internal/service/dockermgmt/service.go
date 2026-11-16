// service.go 是容器管理门面（22 HTTP + 2 WS 订阅的业务承载；handler 薄壳，
// 不感知 HTTP）。读侧：即时读取投影（I-01 无缓存）→ 筛选/排序/标注（D-02）；
// 写侧编排：校验（R-14）→ 矩阵守卫（D-04）→ daemon/CLI 执行（I-03）→ 恰好
// 一条审计（I-06/D-09 降级语义）。daemon 可用性前置由 DaemonGuard 中间件
// 承载（DK_002 拒绝不进本层，天然零审计——A-D15）。
package dockermgmt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/dougong/dougong/internal/adapter/dockerx"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
)

// Service 是容器管理应用服务（构造注入网关与审计 OHS；clock 测试可注入）。
type Service struct {
	gw    DockerGateway
	comp  ComposeRunner
	audit AuditRecorder
	clock func() time.Time
}

// New 构造服务（gw=SDK 通道、comp=CLI 通道、rec=审计 OHS R-03）。
func New(gw DockerGateway, comp ComposeRunner, rec AuditRecorder) *Service {
	return &Service{gw: gw, comp: comp, audit: rec, clock: time.Now}
}

// now 输出 RFC3339 UTC 时间戳（api §3.1 时间格式）。
func (s *Service) now() string { return s.clock().UTC().Format(time.RFC3339) }

// ProbeDaemon 即时探测（DaemonGuard 中间件消费——业务面前置复核 DK_002）。
func (s *Service) ProbeDaemon(ctx context.Context) dockerx.DaemonInfo {
	return s.gw.ProbeDaemon(ctx)
}

// ---------- daemon 接管（D-01） ----------

// DaemonStatus GET /docker/daemon/status（admin+readonly）：即时探测结论。
func (s *Service) DaemonStatus(ctx context.Context) (*DaemonStatusData, error) {
	info := s.gw.ProbeDaemon(ctx)
	return &DaemonStatusData{
		State: info.State, Version: info.Version, Detail: info.Detail,
		GeneratedAt: s.now(),
	}, nil
}

// ---------- 容器投影与生命周期（§4.1） ----------

// ContainerList GET /docker/container/list（admin+readonly）：筛选值域（DK_001）
// → 即时全量投影 → 筛选+排序（名称字典序 D-02）→ Compose 归属 + 权威动作面。
// daemon 摘要由 DaemonGuard 当次探测结论传入（单次探测单请求，A-D14 同源）。
func (s *Service) ContainerList(ctx context.Context, daemon dockerx.DaemonInfo,
	stateFilter, composeProject string) (*ContainerListData, error) {
	states, err := ParseStateFilter(stateFilter)
	if err != nil {
		return nil, err
	}
	facts, err := s.gw.ListContainers(ctx)
	if err != nil {
		return nil, mapReadError(err)
	}
	items := make([]ContainerItem, 0, len(facts))
	for _, f := range facts {
		if states != nil && !states[f.State] {
			continue
		}
		if composeProject != "" && f.ComposeProject != composeProject { // A-D11 等值匹配
			continue
		}
		items = append(items, projectContainer(f))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return &ContainerListData{
		Items: items, Total: len(items),
		Daemon:      DaemonSummary{State: daemon.State, Version: daemon.Version},
		GeneratedAt: s.now(),
	}, nil
}

// projectContainer 容器投影组装（composeProject 无项目为 null；动作面权威
// 计算 D-02）。
func projectContainer(f dockerx.ContainerFact) ContainerItem {
	item := ContainerItem{
		ContainerID:      f.ContainerID,
		Name:             f.Name,
		Image:            f.Image,
		State:            f.State,
		SupportedActions: SupportedActions(f.State),
	}
	if f.ComposeProject != "" {
		p := f.ComposeProject
		item.ComposeProject = &p
	}
	return item
}

// findContainer 存在性+实况复核（List 全量内寻址；不存在 DK_003，通道失败
// DK_017）。
func (s *Service) findContainer(ctx context.Context, id string) (*dockerx.ContainerFact, error) {
	facts, err := s.gw.ListContainers(ctx)
	if err != nil {
		return nil, mapReadError(err)
	}
	for i := range facts {
		if facts[i].ContainerID == id {
			return &facts[i], nil
		}
	}
	return nil, errcode.ErrDkContainerNotFound
}

// ContainerLogs GET /docker/container/logs（admin+readonly）：DK_001 →
// DK_003（实况复核）→ tail 取值域（DK_001，§4.1.4 校验顺序）。
func (s *Service) ContainerLogs(ctx context.Context, containerID, tailRaw string) (*LogsData, error) {
	if err := requireRef("containerId", containerID); err != nil {
		return nil, err
	}
	if _, err := s.findContainer(ctx, containerID); err != nil {
		return nil, err
	}
	tail, err := ParseTail(tailRaw)
	if err != nil {
		return nil, err
	}
	lines, err := s.gw.GetContainerLogs(ctx, containerID, tail)
	if err != nil {
		return nil, mapReadError(err)
	}
	return &LogsData{Lines: lines, GeneratedAt: s.now()}, nil
}

// ContainerStats GET /docker/container/stats（admin+readonly）：DK_001 →
// DK_003 → 单次快照读取（D-07）。
func (s *Service) ContainerStats(ctx context.Context, containerID string) (*StatsData, error) {
	if err := requireRef("containerId", containerID); err != nil {
		return nil, err
	}
	if _, err := s.findContainer(ctx, containerID); err != nil {
		return nil, err
	}
	fact, err := s.gw.GetContainerStats(ctx, containerID)
	if err != nil {
		return nil, mapReadError(err)
	}
	return &StatsData{
		CPUPercent: fact.CPUPercent, MemUsageByte: fact.MemUsageByte,
		MemLimitByte: fact.MemLimitByte, MemPercent: fact.MemPercent,
		GeneratedAt: s.now(),
	}, nil
}

// containerActionAudit 动作→审计码映射（api §3.4 业务常量）。
var containerActionAudit = map[string]string{
	dockerx.ActionStart:   AuditContainerStarted,
	dockerx.ActionStop:    AuditContainerStopped,
	dockerx.ActionRestart: AuditContainerRestarted,
	dockerx.ActionPause:   AuditContainerPaused,
	dockerx.ActionUnpause: AuditContainerUnpaused,
}

// ContainerAction POST /docker/container/{start|stop|restart|pause|unpause}
// （admin）：DK_001（零审计）→ DK_003 → 矩阵守卫 DK_004（零审计，I-04/
// A-D15）→ SDK 执行（失败 DK_017 记 failure）→ 即读实况 → 成功审计。
func (s *Service) ContainerAction(ctx context.Context, actor, action, containerID string) (*ContainerActionData, error) {
	if err := requireRef("containerId", containerID); err != nil {
		return nil, err
	}
	actionCode, ok := containerActionAudit[action]
	if !ok {
		return nil, fmt.Errorf("dockermgmt: unknown container action %q", action)
	}
	fail := func(err error, detail map[string]any) error {
		s.auditFailure(ctx, actor, actionCode, ObjectTypeContainer, containerID, detail, err)
		return err
	}
	fact, err := s.findContainer(ctx, containerID)
	if err != nil {
		return nil, fail(err, map[string]any{
			"containerId": containerID, "action": action})
	}
	if !ActionAllowed(fact.State, action) {
		return nil, errcode.ErrDkActionStateMismatch.WithData(ActionStateData{
			CurrentState:   fact.State,
			Action:         action,
			AllowedActions: SupportedActions(fact.State),
		}) // DK_004 拒绝未发生变更，零审计（I-06/A-D15）
	}
	if err := s.gw.ContainerAction(ctx, containerID, action); err != nil {
		return nil, fail(mapReadError(err), map[string]any{
			"containerId": containerID, "name": fact.Name, "action": action})
	}
	state, stateErr := s.gw.ContainerState(ctx, containerID)
	if stateErr != nil {
		// 动作已生效而终态复核失败：成功审计如实，currentState 无法供 → DK_017。
		s.recordAudit(ctx, auditEntry(actor, actionCode, ObjectTypeContainer, containerID,
			map[string]any{"containerId": containerID, "name": fact.Name, "action": action},
			audit.ResultSuccess))
		return nil, mapReadError(stateErr)
	}
	s.recordAudit(ctx, auditEntry(actor, actionCode, ObjectTypeContainer, containerID,
		map[string]any{"containerId": containerID, "name": fact.Name, "action": action},
		audit.ResultSuccess))
	return &ContainerActionData{ContainerID: containerID, CurrentState: state}, nil
}

// ContainerDelete POST /docker/container/delete（admin）：DK_001 → DK_003 →
// 矩阵（仅 stopped 开放，D-04；无 force 字段 I-04/I-05）→ SDK 删除 → 审计。
func (s *Service) ContainerDelete(ctx context.Context, actor, containerID string) (*ContainerDeleteData, error) {
	if err := requireRef("containerId", containerID); err != nil {
		return nil, err
	}
	fail := func(err error, detail map[string]any) error {
		s.auditFailure(ctx, actor, AuditContainerDeleted, ObjectTypeContainer, containerID, detail, err)
		return err
	}
	fact, err := s.findContainer(ctx, containerID)
	if err != nil {
		return nil, fail(err, map[string]any{"containerId": containerID})
	}
	if !ActionAllowed(fact.State, dockerx.ActionDelete) {
		return nil, errcode.ErrDkActionStateMismatch.WithData(ActionStateData{
			CurrentState:   fact.State,
			Action:         dockerx.ActionDelete,
			AllowedActions: SupportedActions(fact.State),
		}) // DK_004 零审计（I-06/A-D15）
	}
	if err := s.gw.DeleteContainer(ctx, containerID); err != nil {
		return nil, fail(mapReadError(err), map[string]any{
			"containerId": containerID, "name": fact.Name, "image": fact.Image})
	}
	s.recordAudit(ctx, auditEntry(actor, AuditContainerDeleted, ObjectTypeContainer, containerID,
		map[string]any{"containerId": containerID, "name": fact.Name, "image": fact.Image},
		audit.ResultSuccess))
	return &ContainerDeleteData{Deleted: true, ContainerID: containerID}, nil
}

// ---------- image / volume / network（§4.2） ----------

// ImageList GET /docker/image/list（admin+readonly）：即时全量投影。
func (s *Service) ImageList(ctx context.Context) (*ImageListData, error) {
	facts, err := s.gw.ListImages(ctx)
	if err != nil {
		return nil, mapReadError(err)
	}
	items := make([]ImageItem, 0, len(facts))
	for _, f := range facts {
		items = append(items, ImageItem{ImageID: f.ImageID, Ref: f.Ref, SizeBytes: f.SizeBytes})
	}
	return &ImageListData{Items: items, Total: len(items), GeneratedAt: s.now()}, nil
}

// ImagePull POST /docker/image/pull（admin）：ref 格式校验（DK_012——空 ref
// 同属格式非法；§4.2.4 明文 DK_012 按结果记审计）→ daemon 同步拉取（≤600s，
// 失败 DK_013 reason 透传）→ 审计 image_pulled（detail={ref}）。
func (s *Service) ImagePull(ctx context.Context, actor, ref string) (*ImagePullData, error) {
	if !ValidateImageRef(ref) {
		err := errcode.ErrDkImageRefInvalid
		s.auditFailure(ctx, actor, AuditImagePulled, ObjectTypeImage, ref,
			map[string]any{"ref": ref}, err)
		return nil, err
	}
	fact, err := s.gw.PullImage(ctx, ref)
	if err != nil {
		var exec *dockerx.ExecError
		if errors.As(err, &exec) {
			err = errcode.ErrDkPullFailed.WithData(map[string]any{"reason": exec.Reason})
		} else {
			err = mapReadError(err)
		}
		s.auditFailure(ctx, actor, AuditImagePulled, ObjectTypeImage, ref,
			map[string]any{"ref": ref}, err)
		return nil, err
	}
	s.recordAudit(ctx, auditEntry(actor, AuditImagePulled, ObjectTypeImage, fact.ImageID,
		map[string]any{"ref": ref}, audit.ResultSuccess))
	return &ImagePullData{ImageID: fact.ImageID, Ref: fact.Ref}, nil
}

// ImageDelete POST /docker/image/delete（admin）：DK_001 → 存在性（DK_005）→
// daemon 删除（被引用 DK_006，D-08）→ 审计（detail={imageId, ref}）。
func (s *Service) ImageDelete(ctx context.Context, actor, imageID string) (*ImageDeleteData, error) {
	if err := requireRef("imageId", imageID); err != nil {
		return nil, err
	}
	fail := func(err error, detail map[string]any) error {
		s.auditFailure(ctx, actor, AuditImageDeleted, ObjectTypeImage, imageID, detail, err)
		return err
	}
	ref := ""
	if images, err := s.gw.ListImages(ctx); err == nil {
		for _, img := range images {
			if img.ImageID == imageID {
				ref = img.Ref
				break
			}
		}
	}
	if ref == "" {
		return nil, fail(errcode.ErrDkImageNotFound, map[string]any{"imageId": imageID})
	}
	if err := s.gw.DeleteImage(ctx, imageID); err != nil {
		return nil, fail(mapImageDeleteError(err), map[string]any{"imageId": imageID, "ref": ref})
	}
	s.recordAudit(ctx, auditEntry(actor, AuditImageDeleted, ObjectTypeImage, imageID,
		map[string]any{"imageId": imageID, "ref": ref}, audit.ResultSuccess))
	return &ImageDeleteData{Deleted: true, ImageID: imageID}, nil
}

// VolumeList GET /docker/volume/list（admin+readonly）。
func (s *Service) VolumeList(ctx context.Context) (*VolumeListData, error) {
	facts, err := s.gw.ListVolumes(ctx)
	if err != nil {
		return nil, mapReadError(err)
	}
	items := make([]VolumeItem, 0, len(facts))
	for _, f := range facts {
		items = append(items, VolumeItem{
			VolumeName: f.VolumeName, Mountpoint: f.Mountpoint, SizeBytes: f.SizeBytes})
	}
	return &VolumeListData{Items: items, Total: len(items), GeneratedAt: s.now()}, nil
}

// VolumeDelete POST /docker/volume/delete（admin）：DK_001 → 存在性（DK_007）→
// daemon 删除（被挂载 DK_008）→ 审计（detail={volumeName}）。
func (s *Service) VolumeDelete(ctx context.Context, actor, volumeName string) (*VolumeDeleteData, error) {
	if err := requireRef("volumeName", volumeName); err != nil {
		return nil, err
	}
	fail := func(err error) error {
		s.auditFailure(ctx, actor, AuditVolumeDeleted, ObjectTypeVolume, volumeName,
			map[string]any{"volumeName": volumeName}, err)
		return err
	}
	found := false
	if volumes, err := s.gw.ListVolumes(ctx); err == nil {
		for _, v := range volumes {
			if v.VolumeName == volumeName {
				found = true
				break
			}
		}
	}
	if !found {
		return nil, fail(errcode.ErrDkVolumeNotFound)
	}
	if err := s.gw.DeleteVolume(ctx, volumeName); err != nil {
		return nil, fail(mapVolumeDeleteError(err))
	}
	s.recordAudit(ctx, auditEntry(actor, AuditVolumeDeleted, ObjectTypeVolume, volumeName,
		map[string]any{"volumeName": volumeName}, audit.ResultSuccess))
	return &VolumeDeleteData{Deleted: true, VolumeName: volumeName}, nil
}

// NetworkList GET /docker/network/list（admin+readonly）。
func (s *Service) NetworkList(ctx context.Context) (*NetworkListData, error) {
	facts, err := s.gw.ListNetworks(ctx)
	if err != nil {
		return nil, mapReadError(err)
	}
	items := make([]NetworkItem, 0, len(facts))
	for _, f := range facts {
		items = append(items, NetworkItem{
			NetworkID: f.NetworkID, Name: f.Name, Driver: f.Driver, Subnet: f.Subnet})
	}
	return &NetworkListData{Items: items, Total: len(items), GeneratedAt: s.now()}, nil
}

// NetworkDelete POST /docker/network/delete（admin）：DK_001 → 存在性（DK_009）→
// daemon 删除（被接入 DK_010 / 预定义 DK_011，D-08）→ 审计（detail=
// {networkId, name}）。
func (s *Service) NetworkDelete(ctx context.Context, actor, networkID string) (*NetworkDeleteData, error) {
	if err := requireRef("networkId", networkID); err != nil {
		return nil, err
	}
	fail := func(err error, detail map[string]any) error {
		s.auditFailure(ctx, actor, AuditNetworkDeleted, ObjectTypeNetwork, networkID, detail, err)
		return err
	}
	name := ""
	if networks, err := s.gw.ListNetworks(ctx); err == nil {
		for _, n := range networks {
			if n.NetworkID == networkID {
				name = n.Name
				break
			}
		}
	}
	if name == "" {
		return nil, fail(errcode.ErrDkNetworkNotFound, map[string]any{"networkId": networkID})
	}
	if err := s.gw.DeleteNetwork(ctx, networkID); err != nil {
		return nil, fail(mapNetworkDeleteError(err), map[string]any{"networkId": networkID, "name": name})
	}
	s.recordAudit(ctx, auditEntry(actor, AuditNetworkDeleted, ObjectTypeNetwork, networkID,
		map[string]any{"networkId": networkID, "name": name}, audit.ResultSuccess))
	return &NetworkDeleteData{Deleted: true, NetworkID: networkID}, nil
}

// ---------- compose（§4.3） ----------

// ComposeList GET /docker/compose/list（admin+readonly）：label 聚合 + locatable。
func (s *Service) ComposeList(ctx context.Context) (*ComposeListData, error) {
	facts, err := s.comp.ListProjectFacts(ctx)
	if err != nil {
		return nil, mapReadError(err)
	}
	items := make([]ComposeItem, 0, len(facts))
	for _, f := range facts {
		containers := make([]ComposeContainerItem, 0, len(f.Containers))
		for _, c := range f.Containers {
			containers = append(containers, ComposeContainerItem{
				ContainerID: c.ContainerID, Name: c.Name, State: c.State})
		}
		item := ComposeItem{
			Project: f.Project, Containers: containers, Locatable: f.Locatable,
		}
		if f.WorkingDir != "" {
			wd := f.WorkingDir
			item.WorkingDir = &wd
		}
		if f.ConfigFile != "" {
			cf := f.ConfigFile
			item.ConfigFile = &cf
		}
		items = append(items, item)
	}
	return &ComposeListData{Items: items, Total: len(items), GeneratedAt: s.now()}, nil
}

// composeVerbAudit Compose 动词→审计码映射（api §3.4）。
var composeVerbAudit = map[string]string{
	dockerx.VerbUp:      AuditComposeUp,
	dockerx.VerbDown:    AuditComposeDown,
	dockerx.VerbRestart: AuditComposeRestarted,
}

// ComposeAction POST /docker/compose/{up|down|restart}（admin）：DK_001 →
// 项目存在（DK_014）→ 定位（DK_015，data 携 label 记录路径）→ CLI 执行
// （互斥 DK_018 / 失败 DK_016，D-06/I-08/I-09）→ 审计（§4.3.4 按结果如实）。
func (s *Service) ComposeAction(ctx context.Context, actor, project, verb string) (*ComposeActionData, error) {
	if err := requireRef("project", project); err != nil {
		return nil, err
	}
	actionCode, ok := composeVerbAudit[verb]
	if !ok {
		return nil, fmt.Errorf("dockermgmt: unknown compose verb %q", verb)
	}
	fail := func(err error, detail map[string]any) error {
		s.auditFailure(ctx, actor, actionCode, ObjectTypeComposeProject, project, detail, err)
		return err
	}
	fact, err := s.findProject(ctx, project)
	if err != nil {
		return nil, fail(err, map[string]any{"project": project, "verb": verb})
	}
	if !fact.Locatable {
		return nil, fail(
			errcode.ErrDkComposeLocateFailed.WithData(LocateFailedData{
				WorkingDir: fact.WorkingDir, ConfigFile: fact.ConfigFile, Missing: fact.Missing}),
			map[string]any{"project": project, "workingDir": fact.WorkingDir, "verb": verb})
	}
	if err := s.comp.Run(ctx, *fact, verb); err != nil {
		return nil, fail(mapComposeRunError(err),
			map[string]any{"project": project, "workingDir": fact.WorkingDir, "verb": verb})
	}
	s.recordAudit(ctx, auditEntry(actor, actionCode, ObjectTypeComposeProject, project,
		map[string]any{"project": project, "workingDir": fact.WorkingDir, "verb": verb},
		audit.ResultSuccess))
	return &ComposeActionData{Project: project, Verb: verb, Done: true}, nil
}

// ComposeLogs GET /docker/compose/logs（admin+readonly）：DK_001 → DK_014 →
// DK_015 → tail 取值域（§4.3.4 校验顺序）→ 项目聚合尾部日志。
func (s *Service) ComposeLogs(ctx context.Context, project, tailRaw string) (*LogsData, error) {
	if err := requireRef("project", project); err != nil {
		return nil, err
	}
	fact, err := s.findProject(ctx, project)
	if err != nil {
		return nil, err
	}
	if !fact.Locatable {
		return nil, errcode.ErrDkComposeLocateFailed.WithData(LocateFailedData{
			WorkingDir: fact.WorkingDir, ConfigFile: fact.ConfigFile, Missing: fact.Missing})
	}
	tail, err := ParseTail(tailRaw)
	if err != nil {
		return nil, err
	}
	lines, err := s.comp.Logs(ctx, *fact, tail)
	if err != nil {
		return nil, mapReadError(err)
	}
	return &LogsData{Lines: lines, GeneratedAt: s.now()}, nil
}

// findProject 项目存在性实况复核（label 聚合寻址；不存在 DK_014）。
func (s *Service) findProject(ctx context.Context, project string) (*dockerx.ComposeProjectFact, error) {
	facts, err := s.comp.ListProjectFacts(ctx)
	if err != nil {
		return nil, mapReadError(err)
	}
	for i := range facts {
		if facts[i].Project == project {
			return &facts[i], nil
		}
	}
	return nil, errcode.ErrDkComposeProjectNotFound
}

// ---------- WS 订阅（D-03） ----------

// ContainerLogFollow WS /docker/container/logs-follow 订阅建立（握手校验：
// DK_001 → 存在性 DK_003 → tail DK_001；失败拒绝握手由 HTTP 状态+包络上线）。
// 成功返回回放+跟随单流（A-D16），帧转发由 api 层承载。
func (s *Service) ContainerLogFollow(ctx context.Context, containerID, tailRaw string) (*dockerx.LogStream, error) {
	if err := requireRef("containerId", containerID); err != nil {
		return nil, err
	}
	exists, err := s.gw.ContainerExists(ctx, containerID)
	if err != nil {
		return nil, mapReadError(err)
	}
	if !exists {
		return nil, errcode.ErrDkContainerNotFound
	}
	tail, err := ParseTail(tailRaw)
	if err != nil {
		return nil, err
	}
	stream, err := s.gw.FollowContainerLogs(ctx, containerID, tail)
	if err != nil {
		return nil, mapReadError(err)
	}
	return stream, nil
}

// ComposeLogFollow WS /docker/compose/logs-follow 订阅建立（握手校验：
// DK_001 → DK_014 → DK_015 → tail DK_001）。
func (s *Service) ComposeLogFollow(ctx context.Context, project, tailRaw string) (*dockerx.LogStream, error) {
	if err := requireRef("project", project); err != nil {
		return nil, err
	}
	fact, err := s.findProject(ctx, project)
	if err != nil {
		return nil, err
	}
	if !fact.Locatable {
		return nil, errcode.ErrDkComposeLocateFailed.WithData(LocateFailedData{
			WorkingDir: fact.WorkingDir, ConfigFile: fact.ConfigFile, Missing: fact.Missing})
	}
	tail, err := ParseTail(tailRaw)
	if err != nil {
		return nil, err
	}
	stream, err := s.comp.FollowLogs(ctx, *fact, tail)
	if err != nil {
		return nil, mapReadError(err)
	}
	return stream, nil
}

// ---------- 审计与错误归一 ----------

// auditEntry 组装审计四要素（I-15；web 通道，change 类）。
func auditEntry(actor, action, objectType, objectID string, detail map[string]any, result string) audit.Entry {
	return audit.Entry{
		ActorName:  actor,
		Category:   audit.CategoryChange,
		Action:     action,
		ObjectType: objectType,
		ObjectID:   objectID,
		Detail:     detail,
		Result:     result,
		Source:     audit.SourceWeb,
	}
}

// auditFailure 记 failure 审计（detail 附 error 符号，A-W11 先例；DK_001/
// 002/004 拒绝不经此路径——零审计，I-06/A-D15）。
func (s *Service) auditFailure(ctx context.Context, actor, action, objectType, objectID string,
	detail map[string]any, cause error) {
	symbol := "INTERNAL"
	var ec *errcode.Error
	if errors.As(cause, &ec) {
		symbol = ec.Symbol()
	}
	detail["error"] = symbol
	s.recordAudit(ctx, auditEntry(actor, action, objectType, objectID, detail, audit.ResultFailure))
}

// recordAudit 审计写入（D-09：失败重试一次，仍失败记面板运行日志兜底，
// 不影响操作结果返回——操作与审计非同事务）。
func (s *Service) recordAudit(ctx context.Context, e audit.Entry) {
	if err := s.audit.Record(ctx, e); err != nil {
		slog.WarnContext(ctx, "docker audit record retry", "action", e.Action, "err", err)
		if err := s.audit.Record(ctx, e); err != nil {
			slog.ErrorContext(ctx, "docker audit record failed, fallback to runtime log",
				"action", e.Action, "objectType", e.ObjectType, "objectId", e.ObjectID,
				"actor", e.ActorName, "result", e.Result, "detail", e.Detail, "err", err)
		}
	}
}

// mapReadError 通道错误归一（D-08）：ExecError→DK_017（reason 原文透传不
// 虚构）；其余非 errcode 错误原样上抛（handler 层脱敏 INTERNAL）。
func mapReadError(err error) error {
	var exec *dockerx.ExecError
	if errors.As(err, &exec) {
		return errcode.ErrDkDaemonExecFailed.WithData(map[string]any{"reason": exec.Reason})
	}
	return err
}

// mapImageDeleteError 镜像删除拒绝归一（D-08：被引用 DK_006）。
func mapImageDeleteError(err error) error {
	switch {
	case errors.Is(err, dockerx.ErrInUse):
		return errcode.ErrDkImageInUse
	case errors.Is(err, dockerx.ErrNotFound):
		return errcode.ErrDkImageNotFound // 竞态窗口：复核后被删
	default:
		return mapReadError(err)
	}
}

// mapVolumeDeleteError 卷删除拒绝归一（被挂载 DK_008）。
func mapVolumeDeleteError(err error) error {
	switch {
	case errors.Is(err, dockerx.ErrInUse):
		return errcode.ErrDkVolumeInUse
	case errors.Is(err, dockerx.ErrNotFound):
		return errcode.ErrDkVolumeNotFound
	default:
		return mapReadError(err)
	}
}

// mapNetworkDeleteError 网络删除拒绝归一（被接入 DK_010 / 预定义 DK_011）。
func mapNetworkDeleteError(err error) error {
	switch {
	case errors.Is(err, dockerx.ErrInUse):
		return errcode.ErrDkNetworkInUse
	case errors.Is(err, dockerx.ErrPredefinedNetwork):
		return errcode.ErrDkNetworkPredefined
	case errors.Is(err, dockerx.ErrNotFound):
		return errcode.ErrDkNetworkNotFound
	default:
		return mapReadError(err)
	}
}

// mapComposeRunError Compose CLI 结果归一（D-06：互斥 DK_018 / 失败 DK_016
// reason=stderr 透传 / 竞态 DK_014）。
func mapComposeRunError(err error) error {
	var exec *dockerx.ExecError
	switch {
	case errors.Is(err, dockerx.ErrBusy):
		return errcode.ErrDkComposeBusy.WithData(map[string]any{"busy": true})
	case errors.As(err, &exec):
		return errcode.ErrDkComposeExecFailed.WithData(map[string]any{"reason": exec.Reason})
	case errors.Is(err, dockerx.ErrNotFound):
		return errcode.ErrDkComposeProjectNotFound
	default:
		return mapReadError(err)
	}
}
