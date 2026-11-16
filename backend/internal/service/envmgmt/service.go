// Package envmgmt 是环境管理（env-mgmt，v0.1.2 用户反馈第 4 项新模块）应用
// 服务（api-env-mgmt.md）：EnvCatalog 编译期目录与映射（D-02）、实时探测投影
// （D-01）、异步变更任务（D-04：创建即触发 worker、同环境互斥 I-01、重启恢复
// I-04）、升级=重装幂等（D-05）、官方仓库代配（D-06）、卸载保守（D-07）、
// 输出尾部采集（D-08）、服务操作复用 SysdGateway（D-09）、超时与最小权限
// （D-10）。env_install_task 为唯一持久化聚合根（store/envmgmt）；环境条目
// 实况零台账。
package envmgmt

import (
	"context"
	"fmt"
	"time"

	"github.com/dougong/dougong/internal/adapter/envpack"
	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storeenv "github.com/dougong/dougong/internal/store/envmgmt"
)

// Service 是环境管理门面（10 接口业务承载）。
type Service struct {
	pack       *envpack.Gateway
	sysd       *sysd.Gateway
	store      *storeenv.Store
	clock      func() time.Time
	taskOutput *outputRing // 采集回调（worker 内存态 ring 挂钩，D-08）
}

// Options 构造选项（taskOutput 为测试注入钩子，生产用默认内存 ring）。
type Options struct {
	Clock      func() time.Time
	TaskOutput *outputRing
}

// New 构造服务。
func New(pack *envpack.Gateway, sysdGw *sysd.Gateway, store *storeenv.Store, opts Options) *Service {
	s := &Service{pack: pack, sysd: sysdGw, store: store, clock: time.Now, taskOutput: opts.TaskOutput}
	if s.clock == nil {
		s.clock = time.Now
	}
	if s.taskOutput == nil {
		s.taskOutput = newOutputRing()
	}
	return s
}

// ---- 目录（D-02 编译期常量，⚠️A1/A7）----

// CatalogEntry 是环境目录条目（显示元数据 + 发行版映射）。
type CatalogEntry struct {
	Type          string
	DisplayName   string
	Description   string
	RelatedModule string            // 关联模块提示（软引导文案）
	UnitByFamily  map[string]string // family → systemd 单元名
	PkgByFamily   map[string]string // family → 包名
	VendorRepo    map[string]bool   // family → 是否需官方仓库源（D-06）
}

// Catalog 六环境静态目录（⚠️A1；扩展走面板版本迭代）。
var Catalog = []CatalogEntry{
	{Type: storeenv.EnvNginx, DisplayName: "nginx",
		Description:   "高性能 Web 服务器与反向代理（网站管理模块的运行环境）",
		RelatedModule: "网站管理",
		UnitByFamily:  map[string]string{"apt": "nginx", "dnf": "nginx", "yum": "nginx"},
		PkgByFamily:   map[string]string{"apt": "nginx", "dnf": "nginx", "yum": "nginx"}},
	{Type: storeenv.EnvApache, DisplayName: "Apache",
		Description:   "Apache HTTP 服务器（httpd）",
		RelatedModule: "—",
		UnitByFamily:  map[string]string{"apt": "apache2", "dnf": "httpd", "yum": "httpd"},
		PkgByFamily:   map[string]string{"apt": "apache2", "dnf": "httpd", "yum": "httpd"}},
	{Type: storeenv.EnvDocker, DisplayName: "Docker",
		Description:   "容器运行时（容器管理模块的运行环境）",
		RelatedModule: "容器管理",
		UnitByFamily:  map[string]string{"apt": "docker", "dnf": "docker", "yum": "docker"},
		PkgByFamily:   map[string]string{"apt": "docker.io", "dnf": "docker-ce", "yum": "docker-ce"},
		VendorRepo:    map[string]bool{"apt": false, "dnf": true, "yum": true}},
	{Type: storeenv.EnvMySQL, DisplayName: "MySQL",
		Description:   "MySQL 数据库服务器（数据库管理模块的运行环境）",
		RelatedModule: "数据库管理",
		UnitByFamily:  map[string]string{"apt": "mysql", "dnf": "mysqld", "yum": "mysqld"},
		PkgByFamily:   map[string]string{"apt": "mysql-server", "dnf": "mysql-server", "yum": "mysql-server"}},
	{Type: storeenv.EnvRedis, DisplayName: "Redis",
		Description:   "Redis 内存数据库（数据库管理模块的运行环境）",
		RelatedModule: "数据库管理",
		UnitByFamily:  map[string]string{"apt": "redis-server", "dnf": "redis", "yum": "redis"},
		PkgByFamily:   map[string]string{"apt": "redis-server", "dnf": "redis", "yum": "redis"}},
	{Type: storeenv.EnvPHPFPM, DisplayName: "PHP-FPM",
		Description:   "PHP FastCGI 进程管理器（网站管理 PHP 站点的运行环境）",
		RelatedModule: "网站管理",
		UnitByFamily:  map[string]string{"apt": "php-fpm", "dnf": "php-fpm", "yum": "php-fpm"},
		PkgByFamily:   map[string]string{"apt": "php-fpm", "dnf": "php-fpm", "yum": "php-fpm"}},
}

// CatalogByType 目录索引。
func CatalogByType(envType string) *CatalogEntry {
	for i := range Catalog {
		if Catalog[i].Type == envType {
			return &Catalog[i]
		}
	}
	return nil
}

// ---- EnvList（D-01 实时探测投影）----

// EnvList GET /env/env/list：通道探测 → 逐条目实况探测（版本/服务状态）→
// 在途任务标记（I-01 查询）。通道不可用时 installed/version=null「探测不可用」。
func (s *Service) EnvList(ctx context.Context) (*EnvListData, error) {
	ch := s.pack.DetectChannel()
	inFlight := map[string]int64{}
	if tasks, err := s.store.Task().InFlightAll(ctx); err == nil {
		for _, t := range tasks {
			inFlight[t.EnvType] = t.ID
		}
	}
	items := make([]EnvItem, 0, len(Catalog))
	for i := range Catalog {
		e := &Catalog[i]
		item := EnvItem{
			EnvType: e.Type, DisplayName: e.DisplayName, Description: e.Description,
			RelatedModule: e.RelatedModule,
			Installed:     nil, Version: nil, ServiceRunning: nil, ServiceEnabled: nil,
			UnitName: e.UnitByFamily[ch.Family],
		}
		if id, ok := inFlight[e.Type]; ok {
			v := id
			item.RunningTaskID = &v
		}
		if ch.Available {
			pkg := e.PkgByFamily[ch.Family]
			if ver, ok, err := s.pack.QueryInstalled(ctx, ch.Family, pkg); err == nil && ok {
				installed := true
				item.Installed = &installed
				item.Version = &ver
			} else if err == nil {
				installed := false
				item.Installed = &installed
			}
			// 查询失败保持 nil（探测不可用如实降级，D-01/I-06）。
		}
		if item.UnitName != "" {
			if u, err := s.sysd.GetUnit(ctx, item.UnitName); err == nil {
				running := u.ActiveState == "active"
				enabled := u.EnabledState == "enabled"
				item.ServiceRunning = &running
				item.ServiceEnabled = &enabled
			}
			// 单元不存在（未装/非 service）：保持 nil——探测失败≠false（如实区分）。
		}
		items = append(items, item)
	}
	return &EnvListData{
		Channel: ChannelDTO{Family: ch.Family, Available: ch.Available},
		Items:   items, GeneratedAt: formatTS(s.clock()),
	}, nil
}

// ---- Install / Uninstall（D-04 异步任务）----

// Install POST /env/env/install：通道（ENV_001）→ envType（ENV_002）→ 互斥
// （ENV_003）→ action 实况判定（D-05）→ 创建 pending 与发起审计同事务 →
// worker goroutine 异步执行（创建即触发，非轮询调度）。
func (s *Service) Install(ctx context.Context, actor *authsvc.Identity, envType string) (*TaskCreatedData, error) {
	entry := CatalogByType(envType)
	if entry == nil {
		return nil, errcodeInvalidEnvType()
	}
	ch := s.pack.DetectChannel()
	if !ch.Available {
		return nil, errcodeChannelUnavailable("包管理器通道不可用（apt/dnf·yum 均未检出）")
	}
	action := storeenv.ActionInstall
	if ver, ok, err := s.pack.QueryInstalled(ctx, ch.Family, entry.PkgByFamily[ch.Family]); err == nil && ok && ver != "" {
		action = storeenv.ActionUpgrade // D-05：已装=升级
	}
	taskID, err := s.createTask(ctx, actor, envType, action,
		auditActionFor(action, false), map[string]any{"envType": envType, "action": action})
	if err != nil {
		return nil, err
	}
	s.dispatchWorker(taskID)
	return &TaskCreatedData{TaskID: taskID, Action: action}, nil
}

// Uninstall POST /env/env/uninstall：已装前置（ENV_004）→ 创建任务（强确认
// 归前端 D-07；服务端知情决策不硬阻断）。
func (s *Service) Uninstall(ctx context.Context, actor *authsvc.Identity, envType string) (*TaskCreatedData, error) {
	entry := CatalogByType(envType)
	if entry == nil {
		return nil, errcodeInvalidEnvType()
	}
	ch := s.pack.DetectChannel()
	if !ch.Available {
		return nil, errcodeChannelUnavailable("包管理器通道不可用（apt/dnf·yum 均未检出）")
	}
	if _, ok, err := s.pack.QueryInstalled(ctx, ch.Family, entry.PkgByFamily[ch.Family]); err != nil || !ok {
		return nil, errcodeNotInstalled()
	}
	taskID, err := s.createTask(ctx, actor, envType, storeenv.ActionUninstall,
		auditActionFor(storeenv.ActionUninstall, false), map[string]any{"envType": envType})
	if err != nil {
		return nil, err
	}
	s.dispatchWorker(taskID)
	return &TaskCreatedData{TaskID: taskID, Action: storeenv.ActionUninstall}, nil
}

// createTask 互斥判定与任务创建+发起审计同事务（I-01/I-03；SQLite 单连接写
// 串行保证原子——互斥 SELECT 与 INSERT 事务内完成）。
func (s *Service) createTask(ctx context.Context, actor *authsvc.Identity, envType, action, auditAction string,
	detail map[string]any) (int64, error) {
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return 0, fmt.Errorf("envmgmt: create task tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if inflight, err := tx.Task().InFlight(ctx, envType); err != nil {
		return 0, fmt.Errorf("envmgmt: in-flight check: %w", err)
	} else if inflight != nil {
		return 0, errcodeTaskInFlight(inflight.ID)
	}
	id, err := tx.Task().Create(ctx, storeenv.Task{
		EnvType: envType, Action: action, RequestedBy: actorName(actor),
	})
	if err != nil {
		return 0, fmt.Errorf("envmgmt: task create: %w", err)
	}
	if err := tx.Audit().Append(ctx, auditEntry(actor, auditAction, "env_install_task",
		fmtAppendID(id), detail), s.clock()); err != nil {
		return 0, fmt.Errorf("envmgmt: task create audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("envmgmt: task create commit: %w", err)
	}
	return id, nil
}

// actorName 发起者（nil 防御——AuthRequired 链路下不可达）。
func actorName(actor *authsvc.Identity) string {
	if actor == nil {
		return "unknown"
	}
	return actor.Username
}

func fmtAppendID(id int64) string { return fmt.Sprintf("%d", id) }

// audit 非事务审计写入（服务操作等；任务创建/终态走 TxStore 同事务）。
func (s *Service) audit(ctx context.Context, e audit.Entry) error {
	return s.store.Audit().Append(ctx, e, s.clock())
}
