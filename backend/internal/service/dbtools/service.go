package dbtools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storedbt "github.com/dougong/dougong/internal/store/dbtools"
)

// Service 是 db-tools 门面（面板面 9 接口 + 内部 2 端点业务承载 + 进程内
// OHS 提供 ToolStatusForEngine）。分层纪律：不感知 HTTP。
type Service struct {
	store    *storedbt.Store
	vault    *ToolVault
	deploy   DeployGateway
	creds    CredentialGateway
	pipeline ConfigPipeline
	env      EnvReadiness
	sysd     SysdGateway
	phpfpm   PhpFpmGateway
	nginx    NginxConfReader
	panel    PanelInfo
	session  SessionVerifier
	audit    AuditRecorder
	signon   *SignonService

	dataDir     string
	toolDir     string // {dataDir}/tools（TOOL_DIR，D-13）
	mirror      string // tools.mirror（下载镜像前缀，默认空=官方源）
	npmRegistry string // tools.npm_registry（默认空=官方 registry）
	clock       func() time.Time

	disableDispatch bool // 测试注入：同步 runTask 确定性断言（生产恒 false）
}

// Deps 网关与 OHS 装配组（main 注入；测试全替身）。
type Deps struct {
	Deploy   DeployGateway
	Creds    CredentialGateway
	Pipeline ConfigPipeline
	Env      EnvReadiness
	Sysd     SysdGateway
	PhpFpm   PhpFpmGateway
	Nginx    NginxConfReader
	Panel    PanelInfo
	Session  SessionVerifier
	Audit    AuditRecorder
}

// Options 构造选项（DataDir 必填——工具目录/密钥文件派生基点）。
type Options struct {
	DataDir     string
	Mirror      string
	NpmRegistry string
	Clock       func() time.Time
	// DisableDispatch 测试专用：创建任务不自动派发 worker goroutine（测试
	// 同步调用 runTask 确定性断言，避免双执行竞态）；生产恒 false。
	DisableDispatch bool
}

// New 构造服务（signon 台账内建；清扫 goroutine 由 main 经 RunTokenSweeper
// 编排，G-06）。
func New(store *storedbt.Store, vault *ToolVault, deps Deps, opts Options) *Service {
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = "."
	}
	s := &Service{
		store: store, vault: vault,
		deploy: deps.Deploy, creds: deps.Creds, pipeline: deps.Pipeline,
		env: deps.Env, sysd: deps.Sysd, phpfpm: deps.PhpFpm, nginx: deps.Nginx,
		panel: deps.Panel, session: deps.Session, audit: deps.Audit,
		dataDir: dataDir, toolDir: filepath.Join(dataDir, "tools"),
		mirror: opts.Mirror, npmRegistry: opts.NpmRegistry, clock: clock,
		disableDispatch: opts.DisableDispatch,
	}
	s.signon = newSignonService(store, vault, deps.Creds, deps.Session, clock)
	return s
}

// ToolDir 工具管理目录根（{dataDir}/tools）。
func (s *Service) ToolDir() string { return s.toolDir }

// ---- ToolList（D-02 实时探测投影，I-11 无缓存）----

// ToolList GET /dbtool/tool/list：目录两条目 × 逐工具实况探测（部署状态/
// 版本经档案；运行态经 systemd/注入 location+pool；依赖就绪度经 env-mgmt
// 实况读契约 + php -m 扩展比对）+ 在途任务标记。探测失败字段 null 不虚构。
func (s *Service) ToolList(ctx context.Context) (*ToolListData, error) {
	archives, err := s.store.Tool().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("dbtools: tool list archives: %w", err)
	}
	byType := map[string]storedbt.Tool{}
	for _, a := range archives {
		byType[a.ToolType] = a
	}
	inFlight := map[string]int64{}
	if tasks, terr := s.store.Task().InFlightAll(ctx); terr == nil {
		for _, t := range tasks {
			inFlight[t.ToolType] = t.ID
		}
	}
	items := make([]ToolItem, 0, len(Catalog))
	for i := range Catalog {
		e := &Catalog[i]
		item := ToolItem{
			ToolType: e.Type, DisplayName: e.DisplayName, Description: e.Description,
			TargetEngines: e.TargetEngines, CatalogVersion: e.CatalogVersion,
			DepsMissing: []DepMissing{},
		}
		if a, ok := byType[e.Type]; ok {
			item.Installed = true
			v := a.InstalledVersion
			item.InstalledVersion = &v
			item.BoundInstanceID = a.InstanceID // phpmyadmin=NULL（打开时绑定）
			item.Running = s.probeRunning(ctx, e.Type, &a)
			// v1.3 落地形态投影（D-19/I-14）：siteRef 解析失败=null 不虚构。
			if ref, rerr := parseSiteRef(a.SiteRef); rerr == nil {
				port, auto := ref.Port, ref.AutoCreated
				item.LandingPort, item.AutoCreated = &port, &auto
			}
		}
		if id, ok := inFlight[e.Type]; ok {
			v := id
			item.RunningTaskID = &v
		}
		item.DepsMissing = s.depsMissing(ctx, e.Type)
		item.DepsReady = len(item.DepsMissing) == 0
		items = append(items, item)
	}
	return &ToolListData{Items: items, GeneratedAt: formatTS(s.clock())}, nil
}

// ToolStatusForEngine 进程内 OHS（5.1 提供：database-mgmt 高级管理入口
// 状态源——按引擎映射工具，部署状态+运行态+打开可用性+绑定实例）。
func (s *Service) ToolStatusForEngine(ctx context.Context, engine string) ToolStatusInfo {
	toolType := ToolForEngine(engine)
	info := ToolStatusInfo{ToolType: toolType}
	if toolType == "" {
		return info
	}
	a, err := s.store.Tool().GetByType(ctx, toolType)
	if err != nil || a == nil {
		return info
	}
	info.Installed = true
	info.BoundInstanceID = a.InstanceID
	if r := s.probeRunning(ctx, toolType, a); r != nil && *r {
		info.Running = true
		info.Openable = true
	}
	return info
}

// probeRunning 运行态实况投影（探测失败=nil 不虚构，D-02）：
// mongo_express 经 systemd 单元状态；phpmyadmin 经注入 location 存在 +
// PhpFpmGateway pool 可用性（D-06）。
func (s *Service) probeRunning(ctx context.Context, toolType string, a *storedbt.Tool) *bool {
	switch toolType {
	case storedbt.ToolMongoExpress:
		d, err := s.sysd.GetUnit(ctx, MongoExpressUnitName)
		if err != nil || d.LoadState == "not-found" || d.LoadState == "" {
			return nil
		}
		r := d.ActiveState == "active"
		return &r
	case storedbt.ToolPHPMyAdmin:
		ref, err := parseSiteRef(a.SiteRef)
		if err != nil {
			return nil
		}
		content, _, rerr := s.nginx.ReadFile(ref.File)
		if rerr != nil {
			return nil
		}
		if !hasMarker(content, toolType) {
			r := false
			return &r
		}
		pools, perr := s.phpfpm.ListPools(ctx)
		if perr != nil {
			return nil
		}
		r := len(pools) > 0
		return &r
	}
	return nil
}

// depsMissing 依赖就绪度探测（D-06/D-18：env-mgmt 实况读契约 + php -m）：
// phpmyadmin 需 php_fpm+nginx+必需扩展；mongo_express 需 node+nginx。
// 环境未装/探测不可确认 → kind=env；扩展缺失/不可确认 → kind=extension。
func (s *Service) depsMissing(ctx context.Context, toolType string) []DepMissing {
	missing := []DepMissing{}
	envDeps := []string{"nginx"}
	if toolType == storedbt.ToolPHPMyAdmin {
		envDeps = append(envDeps, "php_fpm")
	} else {
		envDeps = append(envDeps, "node")
	}
	for _, name := range envDeps {
		installed, _, ok := s.env.EnvStatus(ctx, name)
		if !ok || !installed {
			missing = append(missing, DepMissing{Kind: "env", Name: name})
		}
	}
	if toolType == storedbt.ToolPHPMyAdmin {
		exts, err := s.deploy.PhpExtensions(ctx)
		if err != nil {
			// php -m 不可用：必需清单全部按缺失如实（不虚构就绪）。
			for _, req := range PHPMyAdminRequiredExts {
				missing = append(missing, DepMissing{Kind: "extension", Name: req})
			}
			return missing
		}
		have := map[string]bool{}
		for _, e := range exts {
			have[e] = true
		}
		for _, req := range PHPMyAdminRequiredExts {
			if !have[req] {
				missing = append(missing, DepMissing{Kind: "extension", Name: req})
			}
		}
	}
	return missing
}

// ---- 任务台账（§4.2.4）----

// TaskList GET /dbtool/task/list（筛选+分页，created_at 倒序）。
func (s *Service) TaskList(ctx context.Context, toolType, status string, page, pageSize int) (*TaskListData, error) {
	if toolType != "" && CatalogByType(toolType) == nil {
		return nil, errcode.ErrDBTInvalidToolType
	}
	known := map[string]bool{storedbt.TaskPending: true, storedbt.TaskRunning: true,
		storedbt.TaskSucceeded: true, storedbt.TaskFailed: true}
	if status != "" && !known[status] {
		return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "status", Message: "status 筛选值非法（db_tool_task_status 取值域）"})
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = TaskListPageSize
	}
	rows, total, err := s.store.Task().List(ctx, toolType, status, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, fmt.Errorf("dbtools: task list: %w", err)
	}
	items := make([]TaskItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, taskItemOf(r))
	}
	return &TaskListData{Items: items, Total: total}, nil
}

// TaskDetail GET /dbtool/task/detail（输出尾部排障；DBT_005）。
func (s *Service) TaskDetail(ctx context.Context, taskID int64) (*TaskDetailData, error) {
	r, err := s.store.Task().GetByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, storedbt.ErrTaskNotFound) {
			return nil, errcode.ErrDBTTaskNotFound
		}
		return nil, fmt.Errorf("dbtools: task detail: %w", err)
	}
	return &TaskDetailData{TaskItem: taskItemOf(r), OutputTail: r.OutputTail}, nil
}

func taskItemOf(r storedbt.Task) TaskItem {
	return TaskItem{
		ID: r.ID, ToolType: r.ToolType, Action: r.Action, Status: r.Status,
		InstanceID: r.InstanceID, RequestedBy: r.RequestedBy, CreatedAt: formatTS(r.CreatedAt),
		StartedAt: timePtrStr(r.StartedAt), FinishedAt: timePtrStr(r.FinishedAt), Error: r.Error,
	}
}

// ---- 审计与通用辅助 ----

// recordAudit 非事务审计（打开/轮换成功侧；失败重试一次后日志兜底——
// 沿 env-mgmt D-12 口径）。detail 脱敏：不含凭据/口令/令牌明文（D-16）。
func (s *Service) recordAudit(ctx context.Context, actor *authsvc.Identity,
	action, objectType, objectID string, detail map[string]any) {
	e := auditEntry(actor, action, objectType, objectID, detail)
	if err := s.audit.Record(ctx, e); err != nil {
		if err := s.audit.Record(ctx, e); err != nil {
			slog.WarnContext(ctx, "dbtools: audit record degraded", "action", action, "err", err)
		}
	}
}

func auditEntry(actor *authsvc.Identity, action, objectType, objectID string, detail map[string]any) audit.Entry {
	e := audit.Entry{
		Category: audit.CategoryChange, Action: action,
		ObjectType: objectType, ObjectID: objectID,
		Detail: detail, Result: audit.ResultSuccess, Source: audit.SourceWeb,
	}
	if actor != nil {
		id := actor.UserID
		e.ActorID = &id
		e.ActorName = actor.Username
	} else {
		e.ActorName = "unknown"
	}
	return e
}

func actorName(actor *authsvc.Identity) string {
	if actor == nil {
		return "unknown"
	}
	return actor.Username
}

func formatTS(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func timePtrStr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	v := formatTS(*t)
	return &v
}

// unitRunningP 单元详情→运行态指针（not-found 形态 nil 如实，沿 env-mgmt
// unitServiceStates 实机教训）。
func unitRunningP(d sysd.UnitDetail) *bool {
	if d.LoadState == "not-found" || d.LoadState == "" {
		return nil
	}
	r := d.ActiveState == "active"
	return &r
}
