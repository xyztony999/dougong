// Package dockermgmt 是容器管理应用服务（api-docker-mgmt.md §4）：DaemonProbeService
// 结论组装（D-01 即时探测）、ContainerProjector（投影+筛选排序+Compose 归属+
// 权威动作面，D-02）、ContainerActionService（矩阵守卫→SDK 执行→审计，D-04/
// I-04）、LogStreamService（尾部读取与 WS 跟随订阅，D-03）、StatsService（单次
// 快照，D-07）、PullService（同步拉取，D-05）、ResourceDeleteService（删除编排
// + daemon 拒绝归一，D-08/I-05）、ComposeProjector/ComposeRunner 编排（label
// 聚合 + locatable + CLI 参数数组，D-06）。零持久化（D-10：无表无 SQL，投影
// 恒为当次 daemon 实况快照，I-01/I-02 无缓存层）。
package dockermgmt

import (
	"context"

	"github.com/dougong/dougong/internal/adapter/dockerx"
	"github.com/dougong/dougong/internal/service/audit"
)

// DockerGateway 是 daemon SDK 通道契约（api §4.1.5/§4.2.5；接口定义在
// 消费方，实现为 internal/adapter/dockerx.Gateway，main 装配注入——
// backend-01 §1）。全部方法 ctx 首参；错误以哨兵（ErrNotFound/ErrInUse/
// ErrPredefinedNetwork）与 ExecError（原文透传）表达，服务层按调用面归一
// DK_ 错误码（D-08）；测试以 fake 实现注入（daemon 交互不出本模块单测边界）。
type DockerGateway interface {
	// ProbeDaemon 即时探测接管结论（D-01：state/version/detail；只读）。
	ProbeDaemon(ctx context.Context) dockerx.DaemonInfo
	// ListContainers 容器实况全量投影（I-01/I-02；含 compose label）。
	ListContainers(ctx context.Context) ([]dockerx.ContainerFact, error)
	// ContainerState 单容器终态复核（A-D18：动作后即读实况）。
	ContainerState(ctx context.Context, id string) (string, error)
	// ContainerExists 存在性实况复核（DK_003 判定；WS 握手拒绝通道）。
	ContainerExists(ctx context.Context, id string) (bool, error)
	// GetContainerLogs 尾部日志（D-03 REST 通道）。
	GetContainerLogs(ctx context.Context, id string, tail int) ([]string, error)
	// FollowContainerLogs 跟随订阅（D-03 WS 通道：回放 tail+跟随单流）。
	FollowContainerLogs(ctx context.Context, id string, tail int) (*dockerx.LogStream, error)
	// GetContainerStats 单次资源占用快照（D-07）。
	GetContainerStats(ctx context.Context, id string) (dockerx.StatsFact, error)
	// ContainerAction 生命周期动作（start/stop/restart/pause/unpause；矩阵校验在服务层 D-04）。
	ContainerAction(ctx context.Context, id, action string) error
	// DeleteContainer 删除容器（无 force，I-04/I-05）。
	DeleteContainer(ctx context.Context, id string) error
	// ListImages 镜像投影（A-D6 每 RepoTag 一条）。
	ListImages(ctx context.Context) ([]dockerx.ImageFact, error)
	// PullImage 同步拉取（D-05：≤600s 阻塞终态）。
	PullImage(ctx context.Context, ref string) (dockerx.ImageFact, error)
	// DeleteImage 删除镜像（被引用由 daemon 拒绝，I-05）。
	DeleteImage(ctx context.Context, id string) error
	// ListVolumes 卷投影（sizeBytes 经 /system/df，A-D5）。
	ListVolumes(ctx context.Context) ([]dockerx.VolumeFact, error)
	// DeleteVolume 删除卷（被挂载由 daemon 拒绝，I-05）。
	DeleteVolume(ctx context.Context, name string) error
	// ListNetworks 网络投影（预定义如实呈现，D-08）。
	ListNetworks(ctx context.Context) ([]dockerx.NetworkFact, error)
	// DeleteNetwork 删除网络（预定义/被接入由 daemon 拒绝，I-05）。
	DeleteNetwork(ctx context.Context, id string) error
}

// ComposeRunner 是 Compose CLI 通道契约（api §4.3.5；实现为
// internal/adapter/dockerx.ComposeRunner——docker compose 参数数组 R-08、
// per-project 串行互斥 I-08、≤300s）。
type ComposeRunner interface {
	// ListProjectFacts label 聚合 + locatable 判定（R-13/D-06）。
	ListProjectFacts(ctx context.Context) ([]dockerx.ComposeProjectFact, error)
	// Run 执行动词（up/down/restart；同项目进行中 ErrBusy→DK_018）。
	Run(ctx context.Context, project dockerx.ComposeProjectFact, verb string) error
	// Logs 项目聚合尾部日志（D-03）。
	Logs(ctx context.Context, project dockerx.ComposeProjectFact, tail int) ([]string, error)
	// FollowLogs 项目聚合跟随订阅（D-03 WS 通道）。
	FollowLogs(ctx context.Context, project dockerx.ComposeProjectFact, tail int) (*dockerx.LogStream, error)
}

// AuditRecorder 是审计写入契约（account-auth OHS：AuditService.Record，R-03；
// D-09 降级语义：重试一次仍失败记面板运行日志，不影响操作结果返回）。
type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}
