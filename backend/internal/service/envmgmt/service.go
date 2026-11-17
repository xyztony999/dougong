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
	"regexp"
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

// ---- 目录（D-02 编译期常量，⚠️A1/A7；v1.1 七环境+多候选+单元全名）----

// CatalogEntry 是环境目录条目（显示元数据 + 发行版映射）。
type CatalogEntry struct {
	Type                string
	DisplayName         string
	Description         string
	UnitByFamily        map[string]string   // family → systemd 单元规范全名（⚠️A8/D-13：含 .service 后缀——EnableUnitFiles DBus 契约无后缀补全）
	UnitPatternByFamily map[string]string   // family → 版本化单元解析正则（php-fpm apt 系 phpX.Y-fpm.service；空=无）
	PkgByFamily         map[string][]string // family → 有序候选包名（D-02 v1.1：检测任一命中/安装取首选/卸载移除命中者）
	VendorRepo          map[string]bool     // family → 是否需第三方官方源代配（D-06）
}

// Catalog 七环境静态目录（⚠️A1；v1.1 W1 修复轮六→七 +MongoDB；扩展走面板版本迭代）。
var Catalog = []CatalogEntry{
	{Type: storeenv.EnvNginx, DisplayName: "nginx",
		Description:  "高性能 Web 服务器与反向代理",
		UnitByFamily: map[string]string{"apt": "nginx.service", "dnf": "nginx.service", "yum": "nginx.service"},
		PkgByFamily:  map[string][]string{"apt": {"nginx"}, "dnf": {"nginx"}, "yum": {"nginx"}}},
	{Type: storeenv.EnvApache, DisplayName: "Apache",
		Description:  "Apache HTTP 服务器（httpd）",
		UnitByFamily: map[string]string{"apt": "apache2.service", "dnf": "httpd.service", "yum": "httpd.service"},
		PkgByFamily:  map[string][]string{"apt": {"apache2"}, "dnf": {"httpd"}, "yum": {"httpd"}}},
	{Type: storeenv.EnvDocker, DisplayName: "Docker",
		Description:  "容器运行时",
		UnitByFamily: map[string]string{"apt": "docker.service", "dnf": "docker.service", "yum": "docker.service"},
		PkgByFamily: map[string][]string{
			// 首选发行版原生 docker.io（W4 拍板）；检测兼容官方源自装 docker-ce 形态（D-02）。
			"apt": {"docker.io", "docker-ce"}, "dnf": {"docker-ce", "docker"}, "yum": {"docker-ce", "docker"}},
		VendorRepo: map[string]bool{"apt": false, "dnf": true, "yum": true}},
	{Type: storeenv.EnvMySQL, DisplayName: "MySQL",
		Description:  "MySQL 数据库服务器",
		UnitByFamily: map[string]string{"apt": "mysql.service", "dnf": "mysqld.service", "yum": "mysqld.service"},
		PkgByFamily: map[string][]string{
			// W3 拍板：MySQL 官方源不代配；检测兼容 mysql-community-server 自装形态。
			"apt": {"mysql-server", "mysql-community-server"},
			"dnf": {"mysql-server", "mysql-community-server"}, "yum": {"mysql-server", "mysql-community-server"}}},
	{Type: storeenv.EnvRedis, DisplayName: "Redis",
		Description:  "Redis 内存数据库",
		UnitByFamily: map[string]string{"apt": "redis-server.service", "dnf": "redis.service", "yum": "redis.service"},
		PkgByFamily:  map[string][]string{"apt": {"redis-server"}, "dnf": {"redis"}, "yum": {"redis"}}},
	{Type: storeenv.EnvPHPFPM, DisplayName: "PHP-FPM",
		Description:  "PHP FastCGI 进程管理器",
		UnitByFamily: map[string]string{"apt": "php-fpm.service", "dnf": "php-fpm.service", "yum": "php-fpm.service"},
		// Ubuntu/Debian 实际单元带主版本号（php8.1-fpm.service 等）——GetUnit 落空时按
		// 前缀扫描 systemd ListUnits 全集解析（D-02 v1.1）。
		UnitPatternByFamily: map[string]string{"apt": `^php[0-9][0-9.]*-fpm\.service$`},
		PkgByFamily:         map[string][]string{"apt": {"php-fpm"}, "dnf": {"php-fpm"}, "yum": {"php-fpm"}}},
	{Type: storeenv.EnvMongoDB, DisplayName: "MongoDB",
		Description:  "MongoDB 文档数据库",
		UnitByFamily: map[string]string{"apt": "mongod.service", "dnf": "mongod.service", "yum": "mongod.service"},
		PkgByFamily:  map[string][]string{"apt": {"mongodb-org"}, "dnf": {"mongodb-org"}, "yum": {"mongodb-org"}},
		// mongodb-org 不在任何发行版官方源——三系均需第三方官方仓库代配（⚠️A10，W1/W2 拍板）。
		VendorRepo: map[string]bool{"apt": true, "dnf": true, "yum": true}},
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

// installedPkg 多候选已装探测（D-02/D-14）：逐候选查询，任一命中返回该候选
// 包名与版本（found=true）；全部候选确定性未装 found=false/probeOK=true；
// 任一候选探测真错误且无命中 probeOK=false（条目级「探测不可用」，非未装）。
func (s *Service) installedPkg(ctx context.Context, e *CatalogEntry, family string) (pkg, version string, found, probeOK bool) {
	anyErr := false
	for _, p := range e.PkgByFamily[family] {
		v, inst, err := s.pack.QueryInstalled(ctx, family, p)
		if err != nil {
			anyErr = true // 真错误（超时/通道异常）——不与「未装」混同（W1 缺陷 2）
			continue
		}
		if inst {
			return p, v, true, true
		}
	}
	return "", "", false, !anyErr
}

// probeInstalled installedPkg 的布尔投影（EnvList/Install/ServiceOp 用）。
func (s *Service) probeInstalled(ctx context.Context, e *CatalogEntry, family string) (version string, installed, ok bool) {
	_, v, found, probeOK := s.installedPkg(ctx, e, family)
	return v, found, probeOK
}

// unitFor 解析环境在目标族的实际 systemd 单元（规范全名）：字面映射名 GetUnit
// 命中即用；php-fpm apt 系版本化单元经 ListUnits 正则扫描解析；解析失败仍
// 返回映射全名（下发供呈现，操作时 systemd 如实报 ENV_006）。
func (s *Service) unitFor(ctx context.Context, e *CatalogEntry, family string) (unit string, detail sysd.UnitDetail, ok bool) {
	lit := e.UnitByFamily[family]
	if lit != "" {
		if d, err := s.sysd.GetUnit(ctx, lit); err == nil {
			return lit, d, true
		}
	}
	if p := e.UnitPatternByFamily[family]; p != "" {
		if units, err := s.sysd.ListUnits(ctx); err == nil {
			if re, cerr := regexp.Compile(p); cerr == nil {
				for _, un := range units {
					if re.MatchString(un.Name) {
						name := un.Name
						if d, err := s.sysd.GetUnit(ctx, name); err == nil {
							return name, d, true
						}
						return name, sysd.UnitDetail{}, false
					}
				}
			}
		}
	}
	return lit, sysd.UnitDetail{}, false
}

// ---- EnvList（D-01 实时探测投影；v1.1 三态如实）----

// EnvList GET /env/env/list：通道探测 → 逐条目实况探测（版本/服务状态）→
// 在途任务标记（I-01 查询）。三态（D-01/⚠️A9）：通道不可用=全条目
// installed/version=null（页头横幅承载语义）；条目级探测真错误=该条目 null；
// 未装（含包名不在包数据库形态）=installed=false。
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
			Installed: nil, Version: nil, ServiceRunning: nil, ServiceEnabled: nil,
		}
		if id, ok := inFlight[e.Type]; ok {
			v := id
			item.RunningTaskID = &v
		}
		if ch.Available {
			if ver, inst, ok := s.probeInstalled(ctx, e, ch.Family); !ok {
				// 探测真错误：保持 nil（条目级「探测不可用」，D-14/I-06）。
			} else if inst {
				installed := true
				item.Installed = &installed
				item.Version = &ver
			} else {
				installed := false
				item.Installed = &installed
			}
		}
		if unit, d, ok := s.unitFor(ctx, e, ch.Family); unit != "" {
			item.UnitName = unit
			if ok {
				running := d.ActiveState == "active"
				enabled := d.EnabledState == "enabled"
				item.ServiceRunning = &running
				item.ServiceEnabled = &enabled
			}
			// 单元不存在（未装/非 service）：ServiceRunning/Enabled 保持 nil——
			// 探测失败≠false（如实区分）。
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
	if _, inst, ok := s.probeInstalled(ctx, entry, ch.Family); ok && inst {
		action = storeenv.ActionUpgrade // D-05：已装（多候选任一命中）=升级
	}
	taskID, err := s.createTask(ctx, actor, envType, action,
		auditActionFor(action, false), map[string]any{"envType": envType, "action": action})
	if err != nil {
		return nil, err
	}
	s.dispatchWorker(taskID)
	return &TaskCreatedData{TaskID: taskID, Action: action}, nil
}

// Uninstall POST /env/env/uninstall：已装前置（ENV_004，多候选任一命中；探测
// 失败同样拒绝——破坏性操作保守）→ 创建任务（强确认归前端 D-07；服务端知情
// 决策不硬阻断）。
func (s *Service) Uninstall(ctx context.Context, actor *authsvc.Identity, envType string) (*TaskCreatedData, error) {
	entry := CatalogByType(envType)
	if entry == nil {
		return nil, errcodeInvalidEnvType()
	}
	ch := s.pack.DetectChannel()
	if !ch.Available {
		return nil, errcodeChannelUnavailable("包管理器通道不可用（apt/dnf·yum 均未检出）")
	}
	if _, _, found, ok := s.installedPkg(ctx, entry, ch.Family); !ok || !found {
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
