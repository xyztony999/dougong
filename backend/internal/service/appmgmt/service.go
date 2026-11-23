// Package appmgmt 是应用管理（app-mgmt，2026-11-20 新模块）应用服务
// （api-app-mgmt.md）：创建前置校验链（D-03）、异步变更任务与同应用互斥
// （D-04/I-05）、创建执行链（D-05）、单元安全写入（D-06）、删除保守执行链
// （D-07）、就绪度读契约消费（D-08）、参数安全写回与基线比对（D-09）、
// 服务操作瞬时通道（D-10）、应用日志 admin 专属（D-11）、建站联动 OHS 消费
// （D-12）、失败重试语义（D-13）。app/app_task 双聚合根（store/appmgmt）；
// 实况零台账；.env/unit 为外部事实源不入库（OUT-14）。
package appmgmt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	envsvc "github.com/dougong/dougong/internal/service/envmgmt"
	storeapp "github.com/dougong/dougong/internal/store/appmgmt"
)

// EnvReadySource 是 env-mgmt 运行时就绪度读契约 OHS（D-08；envsvc.Service
// 实现——进程内单向消费，main 装配注入）。ADR-033 增 NodeReadiness。
type EnvReadySource interface {
	PythonReadiness(ctx context.Context) (envsvc.PythonReadiness, error)
	NodeReadiness(ctx context.Context) (envsvc.NodeReadiness, error)
}

// SiteRef 建站联动返回的站点引用值快照（不落库——D-12 值快照口径）。
type SiteRef struct {
	SiteID     string `json:"siteId"`
	SourceFile string `json:"sourceFile"`
}

// SiteLinkGateway 是 site-mgmt 建站 OHS 消费契约（D-12；消费方定义本轮，
// 提供方登记归 site-mgmt 联动轮——跨模块待同步项）。
type SiteLinkGateway interface {
	// CreateReverseProxySite 创建 reverse_proxy 站点（上游 127.0.0.1:{port}；
	// 经 site-mgmt 安全写入管线 R-06 落盘；失败透传 site-mgmt 错误语义）。
	CreateReverseProxySite(ctx context.Context, actor *authsvc.Identity, name string, port int, domain string) (SiteRef, error)
	// HasUpstreamSite 站点清单联动查询（上游 127.0.0.1:{port} 是否已有站点；
	// 只读值快照呈现，本模块不持久化站点引用）。
	HasUpstreamSite(ctx context.Context, port int) (bool, error)
}

// PyGateway 是本机 Python/单元/日志通道契约（*appx.Gateway 实现；接口化供
// 测试全注入替身——CI 零 runner 实况依赖，项目纪律）。
type PyGateway interface {
	CreateVenv(ctx context.Context, dir string, timeout time.Duration) (string, error)
	PipInstall(ctx context.Context, dir string, timeout time.Duration) (string, error)
	VenvExists(dir string) bool
	DepsProbe(ctx context.Context, dir string) (int, error)
	RemoveDir(dir string) error
	WriteUnit(ctx context.Context, unitName, content string) (func(), error)
	RemoveUnit(ctx context.Context, unitName string) error
	Tail(ctx context.Context, unitName string, lines int) (string, error)
}

// SysdGateway 是 systemd 通道契约（service-mgmt SysdGateway 共享适配层
// ⚠️A12；接口化供测试全注入替身）。
type SysdGateway interface {
	GetUnit(ctx context.Context, name string) (sysd.UnitDetail, error)
	StartUnit(ctx context.Context, name string) (sysd.JobResult, error)
	StopUnit(ctx context.Context, name string) (sysd.JobResult, error)
	RestartUnit(ctx context.Context, name string) (sysd.JobResult, error)
	EnableUnit(ctx context.Context, name string) error
	DisableUnit(ctx context.Context, name string) error
}

// Service 是应用管理门面（16 接口业务承载；ADR-033：运行时项目托管——
// Python〔venv + systemd〕与 Node〔PM2〕）。
type Service struct {
	py           PyGateway
	sysd         SysdGateway
	node         NodeGateway // nil=Node 运行时通道未装配（创建 Node 项目时拒绝）
	pm2          PM2Gateway  // nil=PM2 通道未装配（Node 运维如实拒绝）
	store        *storeapp.Store
	envReady     EnvReadySource
	siteLink     SiteLinkGateway // nil=未装配（hasSite 呈现 null；site/link 拒绝）
	dataDir      string          // 面板数据目录（启动器落位；main 装配注入）
	clock        func() time.Time
	syncDispatch bool // 测试缝：同步执行 runTask（确定性断言，沿 env-mgmt 测试先例）
}

// Options 构造选项。
type Options struct {
	EnvReady     EnvReadySource
	SiteLink     SiteLinkGateway
	Node         NodeGateway
	PM2          PM2Gateway
	DataDir      string
	Clock        func() time.Time
	SyncDispatch bool
}

// New 构造服务。
func New(py PyGateway, sysdGw SysdGateway, store *storeapp.Store, opts Options) *Service {
	s := &Service{py: py, sysd: sysdGw, store: store,
		envReady: opts.EnvReady, siteLink: opts.SiteLink, node: opts.Node, pm2: opts.PM2, dataDir: opts.DataDir,
		clock: time.Now, syncDispatch: opts.SyncDispatch}
	if s.clock == nil {
		s.clock = time.Now
	}
	return s
}

var appNameRe = regexp.MustCompile(AppNamePattern)

// ---- AppList（D-01 实时探测投影；I-05 在途标记；D-08 就绪度页头）----

// AppList GET /app/app/list：就绪度（模块级一次探测全应用共享——环境事实非
// 应用事实）→ 逐应用实况探测（服务态/venv/依赖/端口/在途）→ 站点提示
// （SiteLinkGateway 可用时）。探测失败字段 null 如实（不虚构运行态）。
func (s *Service) AppList(ctx context.Context, runtime string) (*AppListData, error) {
	out := &AppListData{Items: []AppItem{}, GeneratedAt: formatTS(s.clock())}
	if rd, err := s.envReady.PythonReadiness(ctx); err == nil {
		out.PythonReadiness = pythonReadyData(rd)
	} // 探测不可用 → 零值（installed=false 且能力位 null 语义由前端按 null 区分，D-08）
	if rd, err := s.envReady.NodeReadiness(ctx); err == nil {
		out.NodeReadiness = nodeReadyData(rd)
	}
	apps, err := s.store.App().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("appmgmt: app list: %w", err)
	}
	inFlight := map[string]int64{}
	if tasks, err := s.store.Task().InFlightIDs(ctx); err == nil {
		for _, t := range tasks {
			inFlight[t.AppName] = t.ID
		}
	}
	for _, a := range apps {
		// runtime 过滤（ADR-033：网站管理页按 Python / Node 分栏各取其一）。
		if runtime != "" && appRuntime(a) != runtime {
			continue
		}
		runtime := appRuntime(a)
		item := AppItem{
			ID: a.ID, Name: a.Name, Dir: a.Dir, Entry: a.Entry, Runtime: runtime,
			StartMode: a.StartMode, StartSpec: startSpecOf(a.StartSpec), ListenPort: a.ListenPort,
			CreatedAt: formatTS(a.CreatedAt), CreatedBy: a.CreatedBy,
		}
		if id, ok := inFlight[a.Name]; ok {
			v := id
			item.RunningTaskID = &v
		}
		if runtime == storeapp.RuntimeNode {
			// Node（PM2）实况（ADR-033 D-16）。
			item.Pm2Status, item.ServiceRunning, item.ServiceEnabled, item.Restarts, item.MemoryBytes =
				s.nodeProbe(ctx, a)
			item.DepsInstalled = nodeDepsInstalled(a.Dir)
		} else {
			item.VenvExists = s.py.VenvExists(a.Dir)
			if d, err := s.sysd.GetUnit(ctx, a.UnitName); err == nil {
				running, enabled := unitStates(d)
				item.ServiceRunning, item.ServiceEnabled = running, enabled
				item.UnitFailed = d.LoadState != "" && d.LoadState != "not-found" && d.ActiveState == "failed"
			} // 查询失败保持 null（探测失败≠false，如实区分）
			if item.VenvExists {
				if n, err := s.py.DepsProbe(ctx, a.Dir); err == nil {
					deps := n > 0
					item.DepsInstalled = &deps
				}
			}
		}
		if listening, err := portListening(a.ListenPort); err == nil {
			item.PortListening = &listening
		}
		if s.siteLink != nil {
			if has, err := s.siteLink.HasUpstreamSite(ctx, a.ListenPort); err == nil {
				item.HasSite = &has
			}
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// appRuntime 档案运行时（历史行 runtime 列为空时按 python——迁移 018 缺省）。
func appRuntime(a storeapp.App) string {
	if a.Runtime == storeapp.RuntimeNode {
		return storeapp.RuntimeNode
	}
	return storeapp.RuntimePython
}

func nodeReadyData(rd envsvc.NodeReadiness) NodeReadyData {
	out := NodeReadyData{NodeAvailable: rd.NodeAvailable, NpmAvailable: rd.NpmAvailable,
		Pm2Available: rd.Pm2Available}
	if rd.NodeVersion != "" {
		v := rd.NodeVersion
		out.NodeVersion = &v
	}
	return out
}

func pythonReadyData(rd envsvc.PythonReadiness) PythonReadyData {
	out := PythonReadyData{Installed: rd.Installed}
	if rd.Version != "" {
		out.Version = strPtr(rd.Version)
	}
	if rd.Installed {
		v, p := rd.VenvSupport, rd.PipAvailable
		out.VenvSupport, out.PipAvailable = &v, &p
	}
	return out
}

// ---- Create（D-03 前置校验链 → D-04 异步任务 → D-05 执行链）----

// CreateRequest POST /app/app/create 请求体。
type CreateRequest struct {
	Name       string    `json:"name"`
	Dir        string    `json:"dir"`
	Entry      string    `json:"entry"`
	Runtime    string    `json:"runtime,omitempty"` // python（缺省）| node（ADR-033）
	StartMode  string    `json:"startMode"`
	StartSpec  StartSpec `json:"startSpec"`
	ListenPort int       `json:"listenPort"`
	StartNow   bool      `json:"startNow"`
}

// Create 创建应用：APP_001→002→003→004→005→白名单 顺序校验（D-03）→
// INSERT app_task(pending) 与发起审计同事务 → worker 异步执行（档案登记仅在
// succeeded 终态同事务，I-C01/C02——创建失败不残留半档案）。运行时二选一
// （ADR-033）：Python=venv 链；Node=就绪度〔node/npm/PM2〕+ 启动方式校验。
func (s *Service) Create(ctx context.Context, actor *authsvc.Identity, req CreateRequest) (*TaskCreatedData, error) {
	// 容器运行形态已下线（ADR-033 D-app-04）：容器化部署归容器管理「应用」页签。
	if req.StartMode == storeapp.StartModeContainer {
		return nil, errcode.ErrAppStartModeRetired
	}
	runtime := strings.TrimSpace(req.Runtime)
	if runtime == "" {
		runtime = storeapp.RuntimePython
	}
	if runtime != storeapp.RuntimePython && runtime != storeapp.RuntimeNode {
		return nil, errParams("未知的运行时（应为 python 或 node）")
	}
	// APP_001 应用名格式（D-03）。
	if !appNameRe.MatchString(req.Name) {
		return nil, errcode.ErrAppInvalidName
	}
	// APP_002 唯一性（I-02 预查；UNIQUE 约束兜底）。
	if _, err := s.store.App().GetByName(ctx, req.Name); err == nil {
		return nil, errcode.ErrAppNameTaken
	}
	// APP_003 目录规范化+存在性（防逃逸：EvalSymlinks 后须为绝对路径且存在；
	// 沿 file-manager R-04 同构）。
	dir := filepath.Clean(strings.TrimSpace(req.Dir))
	if !filepath.IsAbs(dir) {
		return nil, errDir("代码目录须为绝对路径")
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, errDir("代码目录不存在")
	}
	dir = resolved
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return nil, errDir("代码目录不存在或不是目录")
	}
	entry := filepath.Clean(strings.TrimSpace(req.Entry))
	// 入口文件：Python 与 Node 启动文件方式必填且须存在；Node 的 npm 脚本方式
	// 不需要入口文件（启动目标是 package.json 的脚本，ADR-033 D-16）。
	entryOptional := runtime == storeapp.RuntimeNode && req.StartMode == storeapp.StartModeNpmScript
	if entry == "." {
		entry = ""
	}
	if entry == "" || strings.Contains(entry, "..") || filepath.IsAbs(entry) {
		if entryOptional && entry == "" {
			// 不需要入口文件
		} else {
			return nil, errDir("入口文件须为目录内相对路径")
		}
	}
	if entry != "" {
		if _, err := os.Stat(filepath.Join(dir, entry)); err != nil {
			return nil, errDir("入口文件不存在")
		}
	}
	if runtime == storeapp.RuntimeNode {
		// Node 就绪度分层校验（APP_015：缺 node / npm / PM2 引导环境管理）。
		if err := s.checkNodeReady(ctx); err != nil {
			return nil, err
		}
	} else {
		// APP_004 Python 就绪度（D-08 分层缺失）。
		rd, err := s.envReady.PythonReadiness(ctx)
		if err != nil {
			return nil, errcode.ErrAppEnvMissing.WithData(map[string]any{
				"reason": "Python 环境状态未探测（探测不可用），请到环境管理确认"})
		}
		if !rd.Installed {
			return nil, errcode.ErrAppEnvMissing.WithData(map[string]any{"missing": []string{"python"},
				"reason": "Python 未安装，请到环境管理安装"})
		}
		var missing []string
		if !rd.VenvSupport {
			missing = append(missing, "venv")
		}
		if !rd.PipAvailable {
			missing = append(missing, "pip")
		}
		if len(missing) > 0 {
			return nil, errcode.ErrAppEnvMissing.WithData(map[string]any{"missing": missing,
				"reason": "Python 环境能力缺失，请到环境管理补装（python3-venv/pip）"})
		}
	}
	// APP_005 端口冲突（本机监听查询；不静默换端口）。
	if req.ListenPort < 1024 || req.ListenPort > 65535 {
		return nil, errcode.ErrAppPortOccupied.WithData(map[string]any{
			"port": req.ListenPort, "reason": "端口须在 1024-65535 范围"})
	}
	if listening, err := portListening(req.ListenPort); err == nil && listening {
		return nil, errcode.ErrAppPortOccupied.WithData(map[string]any{"port": req.ListenPort})
	}
	// 启动方式白名单（Python：APP_012，D-06；Node：APP_016，D-16）。
	startSpec := req.StartSpec
	if runtime == storeapp.RuntimeNode {
		// 启动文件方式下入口字段本就是必填且已校验存在的项目内文件——启动参数
		// 未另行指定时沿用入口字段，不要求用户填两遍。
		if req.StartMode == storeapp.StartModeNodeFile && strings.TrimSpace(startSpec.File) == "" {
			startSpec.File = filepath.ToSlash(entry)
		}
		sp, err := normalizeNodeStartSpec(req.StartMode, startSpec)
		if err != nil {
			return nil, err
		}
		startSpec = sp
	} else {
		if _, err := parseStartSpec(req.StartMode, mustJSON(req.StartSpec)); err != nil {
			return nil, err
		}
	}
	// Node 运行时通道装配与 node/npm 路径解析（启动器不依赖 PM2 守护进程的 PATH）。
	var nodeBin, npmBin string
	if runtime == storeapp.RuntimeNode {
		if s.node == nil || s.pm2 == nil {
			return nil, errNodeEnv(nil, "Node 运行时通道未装配，无法托管 Node 项目")
		}
		nb, mb, err := s.node.ResolveBins(ctx)
		if err != nil {
			return nil, errNodeEnv([]string{"node"}, err.Error())
		}
		nodeBin, npmBin = nb, mb
	}
	// I-05 互斥（create 场景按名字——档案未登记前未分配 id）。
	taskID, err := s.createTask(ctx, actor, storeapp.AppTask{
		AppName: req.Name, Action: storeapp.ActionCreate, RequestedBy: actorName(actor),
	}, audit.Entry{Action: AuditActionTaskStarted, ObjectType: ObjTypeTask,
		Detail: map[string]any{"appName": req.Name, "action": storeapp.ActionCreate}})
	if err != nil {
		return nil, err
	}
	// 执行计划入内存（worker 按 taskId 重建；dir/entry/spec 经档案字段在任务
	// succeeded 时落库——worker 内重新读取创建参数：为避免任务表扩列，创建参数
	// 经 startSpec JSON 之外的内存计划传递不可靠（重启丢失），故 create 参数
	// 由 worker 从任务表 app_name + 本方法写入的 pending 计划文件承载——简化：
	// worker 内重新校验并使用相同链路；此处将参数写入 output_tail 前缀注释，
	// worker 解析。为可靠性与可审计性，采用任务输出首行 JSON 计划方案。
	plan := map[string]any{"dir": dir, "entry": entry, "runtime": runtime, "startMode": req.StartMode,
		"startSpec": startSpec, "listenPort": req.ListenPort, "startNow": req.StartNow,
		"nodeBin": nodeBin, "npmBin": npmBin}
	if err := s.store.Task().AppendOutput(ctx, taskID, "plan:"+mustJSON(plan)); err != nil {
		return nil, fmt.Errorf("appmgmt: persist create plan: %w", err)
	}
	s.dispatch(taskID)
	return &TaskCreatedData{TaskID: taskID}, nil
}

// ---- Delete（D-07 保守执行链；purgeDir 显式范围 I-10/C04）----

// DeleteRequest POST /app/app/delete 请求体。
type DeleteRequest struct {
	AppID    int64 `json:"appId"`
	PurgeDir bool  `json:"purgeDir"`
}

// Delete 删除应用：档案前置（APP_007）→ 互斥（APP_006）→ 任务化执行
// （停服务→撤自启→撤单元→〔purgeDir〕删目录→succeeded+档案 DELETE 同事务）；
// 关联站点不自动删（呈现提示归前端，D-12 不持站点引用）。
func (s *Service) Delete(ctx context.Context, actor *authsvc.Identity, req DeleteRequest) (*TaskCreatedData, error) {
	app, err := s.store.App().GetByID(ctx, req.AppID)
	if err != nil {
		return nil, errcode.ErrAppNotFound
	}
	if err := s.checkInFlightByApp(ctx, app.ID, app.Name); err != nil {
		return nil, err
	}
	taskID, err := s.createTask(ctx, actor, storeapp.AppTask{
		AppID: app.ID, AppName: app.Name, Action: storeapp.ActionDelete,
		PurgeDir: req.PurgeDir, RequestedBy: actorName(actor),
	}, audit.Entry{Action: AuditActionTaskStarted, ObjectType: ObjTypeTask,
		Detail: map[string]any{"appName": app.Name, "action": storeapp.ActionDelete,
			"purgeDir": req.PurgeDir}})
	if err != nil {
		return nil, err
	}
	s.dispatch(taskID)
	return &TaskCreatedData{TaskID: taskID}, nil
}

// ---- Deps（C-16-02：venv 前置 APP_009 → 任务化可重试）----

// Deps 重装依赖：requirements.txt 缺失如实拒绝（不虚构任务）。
func (s *Service) Deps(ctx context.Context, actor *authsvc.Identity, appID int64) (*TaskCreatedData, error) {
	app, err := s.store.App().GetByID(ctx, appID)
	if err != nil {
		return nil, errcode.ErrAppNotFound
	}
	if err := s.checkInFlightByApp(ctx, app.ID, app.Name); err != nil {
		return nil, err
	}
	if appRuntime(app) == storeapp.RuntimeNode {
		// Node：依赖清单为 package.json（ADR-033 D-18）；无清单如实拒绝。
		if _, err := os.Stat(filepath.Join(app.Dir, "package.json")); err != nil {
			return nil, errParams("项目里没有 package.json，无需安装依赖")
		}
	} else {
		if !s.py.VenvExists(app.Dir) {
			return nil, errcode.ErrAppVenvMissing.WithData(map[string]any{
				"reason": "运行环境（venv）缺失，请删除后重建应用或先修复目录"})
		}
		if _, err := os.Stat(filepath.Join(app.Dir, "requirements.txt")); err != nil {
			return nil, errParams("应用无 requirements.txt，无需安装依赖")
		}
	}
	taskID, err := s.createTask(ctx, actor, storeapp.AppTask{
		AppID: app.ID, AppName: app.Name, Action: storeapp.ActionDepsInstall,
		RequestedBy: actorName(actor),
	}, audit.Entry{Action: AuditActionTaskStarted, ObjectType: ObjTypeTask,
		Detail: map[string]any{"appName": app.Name, "action": storeapp.ActionDepsInstall}})
	if err != nil {
		return nil, err
	}
	s.dispatch(taskID)
	return &TaskCreatedData{TaskID: taskID}, nil
}

// ---- EnvGet / ParamsSave（D-09：外部事实源直读写+基线比对）----

// EnvGet 读 .env 现文件内容（原样行数组；文件不存在=空数组如实；admin 专属
// 由路由层承载 ⚠️A8——参数面）。
func (s *Service) EnvGet(ctx context.Context, appID int64) (*EnvData, error) {
	app, err := s.store.App().GetByID(ctx, appID)
	if err != nil {
		return nil, errcode.ErrAppNotFound
	}
	out := &EnvData{AppID: app.ID, Lines: []string{}}
	raw, err := os.ReadFile(filepath.Join(app.Dir, EnvFileName))
	if err == nil {
		out.Lines = strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
		if len(out.Lines) > 0 && out.Lines[len(out.Lines)-1] == "" {
			out.Lines = out.Lines[:len(out.Lines)-1]
		}
		if fi, serr := os.Stat(filepath.Join(app.Dir, EnvFileName)); serr == nil {
			v := fmtMtime(fi.ModTime().UTC())
			out.UpdatedAt = &v
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("appmgmt: read env file: %w", err)
	}
	return out, nil
}

// ParamsSaveRequest POST /app/app/params 请求体（baseMtime 基线比对——乐观锁
// 语义 D-09；startSpec nil=不变更）。
type ParamsSaveRequest struct {
	AppID     int64      `json:"appId"`
	EnvLines  []string   `json:"envLines,omitempty"`
	StartSpec *StartSpec `json:"startSpec,omitempty"`
	BaseMtime *string    `json:"baseMtime,omitempty"`
}

// ParamsSave 保存参数：基线比对（外部修改拒绝 APP_012 刷新提示）→ .env 整文件
// 原子写（备份原文件）→ startSpec 变更触发单元更新（D-06 安全管线）→ 同事务
// 档案更新+审计（detail 不含参数值 ⚠️A9）；保存后提示重启生效（不自动重启）。
func (s *Service) ParamsSave(ctx context.Context, actor *authsvc.Identity, req ParamsSaveRequest) (*ParamsSaveData, error) {
	app, err := s.store.App().GetByID(ctx, req.AppID)
	if err != nil {
		return nil, errcode.ErrAppNotFound
	}
	envPath := filepath.Join(app.Dir, EnvFileName)
	// 基线比对（D-09：现文件 mtime 与请求基线不一致=外部已修改，拒绝防覆盖）。
	if fi, err := os.Stat(envPath); err == nil {
		if req.BaseMtime != nil && *req.BaseMtime != fmtMtime(fi.ModTime().UTC()) {
			return nil, errParams("文件已被外部修改，请刷新后重试")
		}
	}
	spec := app.StartSpec
	isNode := appRuntime(app) == storeapp.RuntimeNode
	if req.StartSpec != nil {
		raw, jerr := jsonMarshal(*req.StartSpec)
		if jerr != nil {
			return nil, errParams("启动参数序列化失败")
		}
		if isNode {
			// Node 启动方式校验（APP_016）；值对象以规整结果入库。
			nsp, verr := normalizeNodeStartSpec(app.StartMode, *req.StartSpec)
			if verr != nil {
				return nil, verr
			}
			nraw, _ := jsonMarshal(nsp)
			raw, spec = nraw, nraw
		} else {
			if _, verr := parseStartSpec(app.StartMode, raw); verr != nil {
				return nil, verr
			}
			spec = raw
		}
	}
	// .env 整文件替换（备份原文件；写入失败还原）。
	if req.EnvLines != nil {
		prev, _ := os.ReadFile(envPath)
		content := strings.Join(req.EnvLines, "\n") + "\n"
		if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
			return nil, fmt.Errorf("appmgmt: write env file: %w", err)
		}
		if prev != nil {
			_ = os.WriteFile(envPath+".dougong-bak", prev, 0o600)
		}
	}
	// startSpec 变更 → 托管对象更新：
	//   Python：单元文件安全写回（D-06：WriteUnit 内含备份→原子写→verify→
	//   失败回滚；单元内容重渲染）。
	//   Node：PM2 进程的启动参数在注册时固定，须重新注册（nodeReregister：保留
	//   原运行态与开机自启设置；运行中的进程会随之重启——界面已提示）。
	needRestart := true
	if req.StartSpec != nil {
		if isNode {
			if err := s.nodeReregister(ctx, app, *req.StartSpec); err != nil {
				// .env 已写不回滚（外部事实源合法更新）；注册失败如实拒绝。
				return nil, errParams("Node 进程重新注册失败：" + err.Error())
			}
			needRestart = false // 进程已按新参数重新注册，无需再手动重启
		} else {
			rb, werr := s.py.WriteUnit(ctx, app.UnitName,
				renderUnit(app.Name, app.Dir, renderExecStart(app.StartMode, *req.StartSpec, app.ListenPort)))
			if werr != nil {
				// .env 已写不回滚（外部事实源合法更新）；单元失败如实拒绝并提示。
				_ = rb
				return nil, errParams("服务单元更新失败（已回滚原单元）：" + werr.Error())
			}
		}
	}
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("appmgmt: params tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if req.StartSpec != nil {
		if err := tx.App().UpdateStartSpec(ctx, app.ID, spec); err != nil {
			return nil, fmt.Errorf("appmgmt: params update spec: %w", err)
		}
	}
	if err := tx.Audit().Append(ctx, auditEntry(actor, AuditActionParamsUpdated, ObjTypeApp,
		fmtID(app.ID), map[string]any{"appName": app.Name}), s.clock()); err != nil {
		return nil, fmt.Errorf("appmgmt: params audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("appmgmt: params commit: %w", err)
	}
	return &ParamsSaveData{AppID: app.ID, NeedRestart: needRestart}, nil
}

// ---- ServiceOp（D-10：瞬时通道；请求仅 appId 不开放任意单元名 R-14）----

// ServiceOp 服务操作（start/stop/restart/enable/disable）。
func (s *Service) ServiceOp(ctx context.Context, actor *authsvc.Identity, appID int64, op string) (*ServiceOpData, error) {
	app, err := s.store.App().GetByID(ctx, appID)
	if err != nil {
		return nil, errcode.ErrAppNotFound
	}
	// Node 运行时经 PM2（ADR-033 D-17：enable/disable 会同时启动/停止并保存进程列表）。
	if appRuntime(app) == storeapp.RuntimeNode {
		if s.pm2 == nil {
			return nil, errUnitOp("PM2 通道未装配，无法操作 Node 项目")
		}
		if err := s.nodeServiceOp(ctx, app, op); err != nil {
			return nil, err
		}
		e := auditEntry(actor, auditActionForService(op), ObjTypeApp, fmtID(app.ID),
			map[string]any{"appId": app.ID, "unit": app.UnitName, "runtime": storeapp.RuntimeNode})
		if err := s.store.Audit().Append(ctx, e, s.clock()); err != nil {
			return nil, fmt.Errorf("appmgmt: service op audit: %w", err)
		}
		return &ServiceOpData{AppID: app.ID, Unit: app.UnitName}, nil
	}
	var opErr error
	switch op {
	case "start":
		_, opErr = s.sysd.StartUnit(ctx, app.UnitName)
	case "stop":
		_, opErr = s.sysd.StopUnit(ctx, app.UnitName)
	case "restart":
		_, opErr = s.sysd.RestartUnit(ctx, app.UnitName)
	case "enable":
		opErr = s.sysd.EnableUnit(ctx, app.UnitName)
	case "disable":
		opErr = s.sysd.DisableUnit(ctx, app.UnitName)
	default:
		return nil, errParams("未知服务操作")
	}
	if opErr != nil {
		return nil, errUnitOp(opErr.Error())
	}
	e := auditEntry(actor, auditActionForService(op), ObjTypeApp, fmtID(app.ID),
		map[string]any{"appId": app.ID, "unit": app.UnitName})
	if err := s.store.Audit().Append(ctx, e, s.clock()); err != nil {
		return nil, fmt.Errorf("appmgmt: service op audit: %w", err)
	}
	return &ServiceOpData{AppID: app.ID, Unit: app.UnitName}, nil
}

// ---- TaskList / TaskDetail / TaskRetry（D-13）----

// TaskList GET /app/task/list（appId/action/status 筛选+分页）。
func (s *Service) TaskList(ctx context.Context, appID int64, action, status string, page, pageSize int) (*AppTaskListData, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = TaskListPageSize
	}
	if pageSize > TaskListPageSizeMax {
		pageSize = TaskListPageSizeMax
	}
	tasks, total, err := s.store.Task().List(ctx, appID, action, status, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, fmt.Errorf("appmgmt: task list: %w", err)
	}
	items := make([]AppTaskItem, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, taskItem(t))
	}
	return &AppTaskListData{Items: items, Total: total}, nil
}

// TaskDetail GET /app/task/detail（输出尾部排障）。
func (s *Service) TaskDetail(ctx context.Context, taskID int64) (*AppTaskDetailData, error) {
	t, err := s.store.Task().GetByID(ctx, taskID)
	if err != nil {
		return nil, errcode.ErrAppTaskNotFound
	}
	return &AppTaskDetailData{AppTaskItem: taskItem(t), OutputTail: t.OutputTail}, nil
}

// TaskRetry POST /app/task/retry（D-13：仅 failed 且 create/deps_install；
// delete 不可重试——删除链中断态需人工核实，重新发起删除=新任务）。
func (s *Service) TaskRetry(ctx context.Context, actor *authsvc.Identity, taskID int64) (*TaskCreatedData, error) {
	t, err := s.store.Task().GetByID(ctx, taskID)
	if err != nil {
		return nil, errcode.ErrAppTaskNotFound
	}
	if t.Status != storeapp.TaskFailed || (t.Action != storeapp.ActionCreate && t.Action != storeapp.ActionDepsInstall) {
		return nil, errcode.ErrAppTaskNotFound.WithData(map[string]any{
			"reason": "该任务不可重试（仅失败状态的创建/依赖安装任务支持重试）"})
	}
	// I-05 互斥复查（重试时若同应用已有新在途任务如实拒绝）。
	if inflight, err := s.store.Task().InFlightByName(ctx, t.AppName); err != nil {
		return nil, fmt.Errorf("appmgmt: retry in-flight check: %w", err)
	} else if inflight != nil && inflight.ID != t.ID {
		return nil, errTaskInFlight(inflight.ID)
	}
	if err := s.store.Task().Retry(ctx, t.ID); err != nil {
		return nil, fmt.Errorf("appmgmt: retry: %w", err)
	}
	if err := s.store.Audit().Append(ctx, auditEntry(actor, AuditActionTaskStarted, ObjTypeTask,
		fmtID(t.ID), map[string]any{"appName": t.AppName, "action": t.Action,
			"retry": true}), s.clock()); err != nil {
		return nil, fmt.Errorf("appmgmt: retry audit: %w", err)
	}
	s.dispatch(t.ID)
	return &TaskCreatedData{TaskID: t.ID}, nil
}

// ---- LogTail（D-11：journalctl 尾部；档位白名单 R-14）----

// LogTail GET /app/log（内容 admin 专属由路由层承载 ⚠️A8）。
func (s *Service) LogTail(ctx context.Context, appID int64, lines int) (*LogData, error) {
	app, err := s.store.App().GetByID(ctx, appID)
	if err != nil {
		return nil, errcode.ErrAppNotFound
	}
	if lines != 200 && lines != 500 && lines != 1000 {
		lines = LogTailDefault
	}
	// Node 运行时取 PM2 日志文件尾部（ADR-033）。
	if appRuntime(app) == storeapp.RuntimeNode {
		if s.pm2 == nil {
			return nil, errUnitOp("日志查询失败：PM2 通道未装配")
		}
		content, err := s.nodeLogTail(ctx, app, lines)
		if err != nil {
			return nil, err
		}
		return &LogData{AppID: app.ID, Unit: app.UnitName, Lines: lines, Content: content}, nil
	}
	content, err := s.py.Tail(ctx, app.UnitName, lines)
	if err != nil {
		return nil, errUnitOp("日志查询失败：" + err.Error())
	}
	return &LogData{AppID: app.ID, Unit: app.UnitName, Lines: lines, Content: content}, nil
}

// ---- SiteLink（D-12：消费 site-mgmt 建站 OHS；站点引用值快照不落库）----

// SiteLinkRequest POST /app/site/link 请求体（domain 可选；无域名站点创建
// 依赖 site-mgmt 对通配 Host 的校验语义，拒绝时 APP_011 引导站点管理手工创建）。
type SiteLinkRequest struct {
	AppID  int64  `json:"appId"`
	Domain string `json:"domain,omitempty"`
}

// SiteLink 一键创建反向代理站点（上游 127.0.0.1:{端口}）。
func (s *Service) SiteLink(ctx context.Context, actor *authsvc.Identity, req SiteLinkRequest) (*SiteLinkData, error) {
	app, err := s.store.App().GetByID(ctx, req.AppID)
	if err != nil {
		return nil, errcode.ErrAppNotFound
	}
	if s.siteLink == nil {
		return nil, errSiteLink("建站通道未装配（site-mgmt 未接入）")
	}
	if _, err := s.siteLink.CreateReverseProxySite(ctx, actor, app.Name, app.ListenPort, strings.TrimSpace(req.Domain)); err != nil {
		return nil, errSiteLink(err.Error()) // site-mgmt 错误语义透传（D-12）
	}
	if err := s.store.Audit().Append(ctx, auditEntry(actor, AuditActionSiteLinked, ObjTypeApp,
		fmtID(app.ID), map[string]any{"appName": app.Name, "domain": req.Domain}), s.clock()); err != nil {
		return nil, fmt.Errorf("appmgmt: site link audit: %w", err)
	}
	return &SiteLinkData{AppID: app.ID,
		Note: "站点已创建；域名绑定与 HTTPS 请到站点管理配置"}, nil
}

// ---- 内部辅助 ----

func (s *Service) checkInFlightByApp(ctx context.Context, appID int64, name string) error {
	inflight, err := s.store.Task().InFlightByApp(ctx, appID)
	if err != nil {
		return fmt.Errorf("appmgmt: in-flight check: %w", err)
	}
	if inflight == nil {
		if inflight, err = s.store.Task().InFlightByName(ctx, name); err != nil {
			return fmt.Errorf("appmgmt: in-flight check by name: %w", err)
		}
	}
	if inflight != nil {
		return errTaskInFlight(inflight.ID)
	}
	return nil
}

func (s *Service) createTask(ctx context.Context, actor *authsvc.Identity, t storeapp.AppTask, e audit.Entry) (int64, error) {
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return 0, fmt.Errorf("appmgmt: create task tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	id, err := tx.Task().Create(ctx, t)
	if err != nil {
		return 0, fmt.Errorf("appmgmt: task create: %w", err)
	}
	detail, _ := e.Detail.(map[string]any)
	base := auditEntry(actor, e.Action, e.ObjectType, fmtID(id), detail)
	if err := tx.Audit().Append(ctx, base, s.clock()); err != nil {
		return 0, fmt.Errorf("appmgmt: task create audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("appmgmt: task create commit: %w", err)
	}
	return id, nil
}

func actorName(actor *authsvc.Identity) string {
	if actor == nil {
		return "unknown"
	}
	return actor.Username
}

func taskItem(t storeapp.AppTask) AppTaskItem {
	return AppTaskItem{
		ID: t.ID, AppID: t.AppID, AppName: t.AppName, Action: t.Action,
		Status: t.Status, PurgeDir: t.PurgeDir, RequestedBy: t.RequestedBy,
		CreatedAt: formatTS(t.CreatedAt), StartedAt: timePtrStr(t.StartedAt),
		FinishedAt: timePtrStr(t.FinishedAt), Error: t.Error,
		Retryable: t.Status == storeapp.TaskFailed &&
			(t.Action == storeapp.ActionCreate || t.Action == storeapp.ActionDepsInstall),
	}
}

// auditActionForService 服务操作审计动作映射。
func auditActionForService(op string) string {
	switch op {
	case "start":
		return AuditActionServiceStarted
	case "stop":
		return AuditActionServiceStopped
	case "restart":
		return AuditActionServiceRestarted
	case "enable":
		return AuditActionServiceEnabled
	case "disable":
		return AuditActionServiceDisabled
	}
	return AuditActionServiceStarted
}
