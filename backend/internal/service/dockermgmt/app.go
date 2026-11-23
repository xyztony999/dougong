// app.go —— docker-mgmt 的「应用」页签（ADR-033 S-62 面板应用，api-docker-mgmt
// v1.2）：应用 = 面板生成的单服务 compose 编排 + dougong.app 归属标签，提供
// 单应用视角的创建 / 启停 / 日志 / 删除 / 建站。零持久化（D-app-06）：清单由
// 落盘根扫描与容器实况合并而成，无表无 SQL。主机端口只绑 127.0.0.1（D-app-03），
// 对外暴露经建站。本文件另含 Compose 项目删除（S-50-05，生命周期缺口 D0 Q4）。
package dockermgmt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dougong/dougong/internal/adapter/dockerx"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"gopkg.in/yaml.v3"
)

// AppSiteRef 建站返回的站点引用值快照（不落库）。
type AppSiteRef struct {
	SiteID     string
	SourceFile string
}

// SiteLinker 是 site-mgmt 反向代理建站 OHS 消费契约（应用建站 S-62-05；沿
// app-mgmt 建站联动的同一通道——提供方 *sitesvc.Service，main 装配注入）。
type SiteLinker interface {
	// CreateReverseProxySite 创建反向代理站点（上游 127.0.0.1:{port}；domains 为
	// 站点域名，经 site-mgmt 安全校验，拒绝原因透传）。
	CreateReverseProxySite(ctx context.Context, actor *authsvc.Identity, name string, port int, domains []string) (AppSiteRef, error)
	// HasUpstreamSite 上游 127.0.0.1:{port} 是否已有站点（只读值快照）。
	HasUpstreamSite(ctx context.Context, port int) (bool, error)
}

// WithSiteLinker 注入建站通道（未装配时应用清单 siteLinked 为 null、建站拒绝）。
func WithSiteLinker(l SiteLinker) Option { return func(s *Service) { s.siteLink = l } }

// ---------- DTO（api v1.2 §B 逐形状）----------

// AppItem 应用清单条目。
type AppItem struct {
	Name          string                 `json:"name"`
	State         string                 `json:"state"` // docker_app_state：running / stopped / partial
	Containers    []ComposeContainerItem `json:"containers"`
	Image         string                 `json:"image"`
	HostPort      int                    `json:"hostPort"`
	ContainerPort int                    `json:"containerPort"`
	WorkingDir    string                 `json:"workingDir"`
	ConfigFile    string                 `json:"configFile"`
	SiteLinked    *bool                  `json:"siteLinked"` // 建站通道未装配时 null
}

// AppListData GET /docker/app/list 响应。
type AppListData struct {
	Items       []AppItem `json:"items"`
	Total       int       `json:"total"`
	GeneratedAt string    `json:"generatedAt"`
}

// AppCreateRequest POST /docker/app/create 请求体。
type AppCreateRequest struct {
	Name          string     `json:"name"`
	Image         string     `json:"image"`
	HostPort      int        `json:"hostPort"`
	ContainerPort int        `json:"containerPort"`
	Env           []EnvKVDTO `json:"env,omitempty"`
	Mounts        []MountDTO `json:"mounts,omitempty"`
	RestartPolicy string     `json:"restartPolicy,omitempty"` // 缺省 unless-stopped
}

// AppCreateData 创建应用响应。
type AppCreateData struct {
	Name       string `json:"name"`
	WorkingDir string `json:"workingDir"`
	ConfigFile string `json:"configFile"`
	HostPort   int    `json:"hostPort"`
	State      string `json:"state"`
}

// AppServiceRequest POST /docker/app/service 请求体。
type AppServiceRequest struct {
	Name string `json:"name"`
	Op   string `json:"op"` // start | stop | restart
}

// AppServiceData 应用启停重启响应。
type AppServiceData struct {
	Name string `json:"name"`
	Op   string `json:"op"`
}

// AppDeleteRequest POST /docker/app/delete 请求体。
type AppDeleteRequest struct {
	Name string `json:"name"`
}

// AppDeleteData 删除应用响应。
type AppDeleteData struct {
	Name       string `json:"name"`
	DeletedDir bool   `json:"deletedDir"`
}

// AppSiteRequest POST /docker/app/site 请求体。
type AppSiteRequest struct {
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
}

// AppSiteData 应用建站响应（站点引用值快照）。
type AppSiteData struct {
	Name       string `json:"name"`
	SiteID     string `json:"siteId"`
	SourceFile string `json:"sourceFile"`
}

// ComposeDeleteData POST /docker/compose/delete 响应。DeletedDir=false 表示项目
// 目录不在面板落盘根下（面板外项目只停止、不删目录）。
type ComposeDeleteData struct {
	Project    string `json:"project"`
	DeletedDir bool   `json:"deletedDir"`
}

// ---------- 定位与实况 ----------

// findApp 按应用名定位面板应用（落盘根下带归属标签、标签值等于应用名的项目）。
func (s *Service) findApp(name string) (panelCompose, error) {
	panel, err := scanPanelRoot(s.composeRoot)
	if err != nil {
		return panelCompose{}, err
	}
	for _, pc := range panel {
		if pc.isApp && pc.appName == name {
			return pc, nil
		}
	}
	return panelCompose{}, errcode.ErrDKAppNotFound
}

// appFact 应用的 Compose 项目事实（用于 CLI 定位）。
func appFact(pc panelCompose) dockerx.ComposeProjectFact {
	return dockerx.ComposeProjectFact{Project: pc.project, WorkingDir: pc.workingDir,
		ConfigFile: pc.configFile, Locatable: true, PanelManaged: true}
}

// appState 由项目容器实况聚合应用状态（非状态机；枚举池 docker_app_state）。
func appState(f dockerx.ComposeProjectFact) string {
	if len(f.Containers) == 0 {
		return AppStateStopped
	}
	running := 0
	for _, c := range f.Containers {
		if c.State == dockerx.StatusRunning {
			running++
		}
	}
	switch running {
	case 0:
		return AppStateStopped
	case len(f.Containers):
		return AppStateRunning
	default:
		return AppStatePartial
	}
}

// mapAppRunError 应用相关 compose 命令失败的归一：互斥 DK_018；其余 DK_030
// 并透传原因。
func mapAppRunError(err error) error {
	if errors.Is(err, dockerx.ErrBusy) {
		return errcode.ErrDkComposeBusy.WithData(map[string]any{"busy": true})
	}
	var exec *dockerx.ExecError
	if errors.As(err, &exec) {
		return errcode.ErrDKAppOpFailed.WithData(map[string]any{"reason": exec.Reason})
	}
	return errcode.ErrDKAppOpFailed.WithData(map[string]any{"reason": err.Error()})
}

// appCreateFailure 把落盘启动阶段的错误归到应用语义：项目已存在（DK_027）与
// 互斥原样透出，其余转 DK_030 并保留原因。
func appCreateFailure(err error) error {
	var ec *errcode.Error
	if errors.As(err, &ec) {
		switch ec.Symbol() {
		case "DK_027", "DK_018":
			return err
		}
		if data, ok := ec.Data().(map[string]any); ok {
			if reason, ok := data["reason"].(string); ok {
				return errcode.ErrDKAppOpFailed.WithData(map[string]any{"reason": reason})
			}
		}
	}
	return errcode.ErrDKAppOpFailed.WithData(map[string]any{"reason": err.Error()})
}

// ---------- AppList（S-62-01）----------

// AppList 应用清单：落盘根扫描（带归属标签的项目）∪ 容器实况（按项目名聚合
// 运行态）。站点提示经建站通道查询（未装配或查询失败为 null 如实）。
func (s *Service) AppList(ctx context.Context) (*AppListData, error) {
	panel, err := scanPanelRoot(s.composeRoot)
	if err != nil {
		return nil, errcode.ErrDKAppOpFailed.WithData(map[string]any{"reason": err.Error()})
	}
	facts, err := s.comp.ListProjectFacts(ctx)
	if err != nil {
		return nil, mapReadError(err)
	}
	byProject := make(map[string]dockerx.ComposeProjectFact, len(facts))
	for _, f := range facts {
		byProject[f.Project] = f
	}
	items := make([]AppItem, 0, len(panel))
	for _, pc := range panel {
		if !pc.isApp {
			continue
		}
		f := byProject[pc.project]
		containers := make([]ComposeContainerItem, 0, len(f.Containers))
		for _, c := range f.Containers {
			containers = append(containers, ComposeContainerItem{ContainerID: c.ContainerID, Name: c.Name, State: c.State})
		}
		item := AppItem{Name: pc.appName, State: appState(f), Containers: containers,
			Image: pc.image, HostPort: pc.hostPort, ContainerPort: pc.containerPort,
			WorkingDir: pc.workingDir, ConfigFile: pc.configFile}
		if pc.hostPort > 0 && s.siteLink != nil {
			if has, herr := s.siteLink.HasUpstreamSite(ctx, pc.hostPort); herr == nil {
				item.SiteLinked = &has
			}
		}
		items = append(items, item)
	}
	return &AppListData{Items: items, Total: len(items), GeneratedAt: s.now()}, nil
}

// ---------- AppCreate（S-62-02）----------

// validateAppCreate 创建校验链（R-14 服务端权威；错误按语义归码）。
func validateAppCreate(req AppCreateRequest) error {
	if !composeProjectRe.MatchString(req.Name) {
		return errcode.ErrDKAppNameInvalid
	}
	if strings.TrimSpace(req.Image) == "" {
		return errcode.ErrDkInvalidParam.WithData(map[string]any{"reason": "镜像必填"})
	}
	if !ValidateImageRef(req.Image) {
		return errcode.ErrDkImageRefInvalid
	}
	if req.HostPort < 1 || req.HostPort > 65535 {
		return errcode.ErrDkInvalidParam.WithData(map[string]any{"reason": "主机端口须为 1~65535 的数字"})
	}
	if req.ContainerPort < 1 || req.ContainerPort > 65535 {
		return errcode.ErrDkInvalidParam.WithData(map[string]any{"reason": "容器端口须为 1~65535 的数字"})
	}
	switch req.RestartPolicy {
	case "", "no", "always", "unless-stopped", "on-failure":
	default:
		return errcode.ErrDkInvalidParam.WithData(map[string]any{"reason": "重启策略取值非法"})
	}
	for _, e := range req.Env {
		if !appEnvKeyRe.MatchString(e.Key) {
			return errcode.ErrDKEnvFormatInvalid
		}
	}
	for _, m := range req.Mounts {
		if m.Source == "" || !strings.HasPrefix(m.Target, "/") || strings.Contains(m.Source, "..") {
			return errcode.ErrDKMountFormatInvalid
		}
	}
	return nil
}

// appComposeFile 应用编排的内存形状（yaml 库序列化，保证写出的文件合法）。
type appComposeFile struct {
	Services map[string]appComposeService `yaml:"services"`
}

type appComposeService struct {
	Image       string            `yaml:"image"`
	Ports       []string          `yaml:"ports"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	Restart     string            `yaml:"restart,omitempty"`
	Labels      map[string]string `yaml:"labels"`
}

// renderAppCompose 渲染单服务应用编排（D-app-03：主机端口只绑 127.0.0.1；
// D-app-05：写入归属标签）。
func renderAppCompose(req AppCreateRequest) (string, error) {
	svc := appComposeService{
		Image:   req.Image,
		Ports:   []string{fmt.Sprintf("127.0.0.1:%d:%d", req.HostPort, req.ContainerPort)},
		Restart: req.RestartPolicy,
		Labels:  map[string]string{AppOwnershipLabel: req.Name},
	}
	if svc.Restart == "" {
		svc.Restart = "unless-stopped"
	}
	if len(req.Env) > 0 {
		svc.Environment = make(map[string]string, len(req.Env))
		for _, e := range req.Env {
			svc.Environment[e.Key] = e.Value
		}
	}
	for _, m := range req.Mounts {
		v := m.Source + ":" + m.Target
		if m.Mode != "" {
			v += ":" + m.Mode
		}
		svc.Volumes = append(svc.Volumes, v)
	}
	out, err := yaml.Marshal(appComposeFile{Services: map[string]appComposeService{appServiceName: svc}})
	if err != nil {
		return "", fmt.Errorf("应用编排渲染失败：%w", err)
	}
	return string(out), nil
}

// AppCreate 创建应用：校验 → 主机端口占用 → 渲染编排 → 落盘并启动（启动失败
// 且零容器则清理落盘）→ 审计（detail 不含环境变量值）。
func (s *Service) AppCreate(ctx context.Context, actor string, req AppCreateRequest) (*AppCreateData, error) {
	fail := func(err error) error {
		s.auditFailure(ctx, actor, AuditAppCreated, ObjectTypeApp, req.Name,
			map[string]any{"app": req.Name, "image": req.Image, "hostPort": req.HostPort}, err)
		return err
	}
	if err := validateAppCreate(req); err != nil {
		return nil, fail(err)
	}
	if hostPortInUse(req.HostPort) {
		return nil, fail(errcode.ErrDKAppPortInUse.WithData(map[string]any{"port": req.HostPort}))
	}
	yamlText, err := renderAppCompose(req)
	if err != nil {
		return nil, fail(errcode.ErrDKAppOpFailed.WithData(map[string]any{"reason": err.Error()}))
	}
	workDir, configFile, started, err := s.createProjectOnDisk(ctx, req.Name, yamlText, true)
	if err != nil {
		return nil, fail(appCreateFailure(err))
	}
	state := AppStateStopped
	if started {
		// up -d 成功即视为已启动；容器随后异常退出会在清单实况里如实呈现。
		state = AppStateRunning
	}
	s.recordAudit(ctx, auditEntry(actor, AuditAppCreated, ObjectTypeApp, req.Name,
		map[string]any{"app": req.Name, "image": req.Image, "hostPort": req.HostPort,
			"workingDir": workDir, "started": started}, audit.ResultSuccess))
	return &AppCreateData{Name: req.Name, WorkingDir: workDir, ConfigFile: configFile,
		HostPort: req.HostPort, State: state}, nil
}

// ---------- AppService（S-62-03）----------

// AppService 应用启停重启：start=up -d / stop=stop（保留容器）/ restart。
func (s *Service) AppService(ctx context.Context, actor string, name, op string) (*AppServiceData, error) {
	if err := requireRef("name", name); err != nil {
		return nil, err
	}
	verbs := map[string]string{"start": dockerx.VerbUp, "stop": dockerx.VerbStop, "restart": dockerx.VerbRestart}
	verb, ok := verbs[op]
	if !ok {
		return nil, errcode.ErrDkInvalidParam.WithData(map[string]any{"reason": "op 取值须为 start / stop / restart"})
	}
	fail := func(err error) error {
		s.auditFailure(ctx, actor, AuditAppService, ObjectTypeApp, name,
			map[string]any{"app": name, "op": op}, err)
		return err
	}
	pc, err := s.findApp(name)
	if err != nil {
		return nil, fail(err)
	}
	if err := s.comp.Run(ctx, appFact(pc), verb); err != nil {
		return nil, fail(mapAppRunError(err))
	}
	s.recordAudit(ctx, auditEntry(actor, AuditAppService, ObjectTypeApp, name,
		map[string]any{"app": name, "op": op}, audit.ResultSuccess))
	return &AppServiceData{Name: name, Op: op}, nil
}

// ---------- AppLogs（S-62-03）----------

// AppLogs 应用日志（经 compose logs 通道；档位白名单同 Compose 日志）。
func (s *Service) AppLogs(ctx context.Context, name, tailRaw string) (*LogsData, error) {
	if err := requireRef("name", name); err != nil {
		return nil, err
	}
	pc, err := s.findApp(name)
	if err != nil {
		return nil, err
	}
	tail, err := ParseTail(tailRaw)
	if err != nil {
		return nil, err
	}
	lines, err := s.comp.Logs(ctx, appFact(pc), tail)
	if err != nil {
		return nil, mapReadError(err)
	}
	return &LogsData{Lines: lines, GeneratedAt: s.now()}, nil
}

// ---------- AppDelete（S-62-04）----------

// AppDelete 删除应用：compose down（停止并移除容器与网络）→ 删除项目目录。
// 已关联站点不自动删除（呈现提示归前端）。
func (s *Service) AppDelete(ctx context.Context, actor string, name string) (*AppDeleteData, error) {
	if err := requireRef("name", name); err != nil {
		return nil, err
	}
	fail := func(err error) error {
		s.auditFailure(ctx, actor, AuditAppDeleted, ObjectTypeApp, name,
			map[string]any{"app": name}, err)
		return err
	}
	pc, err := s.findApp(name)
	if err != nil {
		return nil, fail(err)
	}
	if err := s.comp.Run(ctx, appFact(pc), dockerx.VerbDown); err != nil {
		return nil, fail(mapAppRunError(err))
	}
	if err := os.RemoveAll(pc.workingDir); err != nil {
		return nil, fail(errcode.ErrDKAppOpFailed.WithData(map[string]any{
			"reason": "容器已移除，但删除项目目录失败：" + err.Error()}))
	}
	s.recordAudit(ctx, auditEntry(actor, AuditAppDeleted, ObjectTypeApp, name,
		map[string]any{"app": name, "workingDir": pc.workingDir}, audit.ResultSuccess))
	return &AppDeleteData{Name: name, DeletedDir: true}, nil
}

// ---------- AppSite（S-62-05）----------

// AppSite 应用建站：为应用创建反向代理站点（上游 127.0.0.1:{主机端口}）。
// domains 为空则由 site-mgmt 按通配 Host 规则处理（拒绝原因透传）。
func (s *Service) AppSite(ctx context.Context, actor *authsvc.Identity, req AppSiteRequest) (*AppSiteData, error) {
	if err := requireRef("name", req.Name); err != nil {
		return nil, err
	}
	if s.siteLink == nil {
		return nil, errcode.ErrDKAppOpFailed.WithData(map[string]any{"reason": "建站通道未装配（site-mgmt 未接入）"})
	}
	pc, err := s.findApp(req.Name)
	if err != nil {
		return nil, err
	}
	if pc.hostPort == 0 {
		return nil, errcode.ErrDKAppOpFailed.WithData(map[string]any{
			"reason": "应用编排里没有可识别的端口映射，无法建站"})
	}
	domains := make([]string, 0, len(req.Domains))
	for _, d := range req.Domains {
		if d = strings.TrimSpace(d); d != "" {
			domains = append(domains, d)
		}
	}
	ref, err := s.siteLink.CreateReverseProxySite(ctx, actor, req.Name, pc.hostPort, domains)
	if err != nil {
		return nil, errcode.ErrDKAppOpFailed.WithData(map[string]any{"reason": err.Error()})
	}
	actorName := ""
	if actor != nil {
		actorName = actor.Username
	}
	s.recordAudit(ctx, auditEntry(actorName, AuditAppSiteLinked, ObjectTypeApp, req.Name,
		map[string]any{"app": req.Name, "domains": domains, "siteId": ref.SiteID}, audit.ResultSuccess))
	return &AppSiteData{Name: req.Name, SiteID: ref.SiteID, SourceFile: ref.SourceFile}, nil
}

// ---------- ComposeDelete（S-50-05）----------

// ComposeDelete 删除 Compose 项目：compose down（停止并移除项目容器与网络）→
// 目录位于面板落盘根下则删除目录（面板外项目只停止、不删目录，响应如实标注）。
// 高危操作：强确认由前端承载；审计记录删除范围。
func (s *Service) ComposeDelete(ctx context.Context, actor, project string) (*ComposeDeleteData, error) {
	if err := requireRef("project", project); err != nil {
		return nil, err
	}
	fail := func(err error, detail map[string]any) error {
		s.auditFailure(ctx, actor, AuditComposeProjectDeleted, ObjectTypeComposeProject, project, detail, err)
		return err
	}
	detail := map[string]any{"project": project}
	fact, err := s.findProject(ctx, project)
	if err != nil {
		return nil, fail(err, detail)
	}
	if fact.Locatable {
		if err := s.comp.Run(ctx, *fact, dockerx.VerbDown); err != nil {
			if errors.Is(err, dockerx.ErrBusy) {
				return nil, fail(errcode.ErrDkComposeBusy.WithData(map[string]any{"busy": true}), detail)
			}
			return nil, fail(errcode.ErrDKComposeDeleteFailed.WithData(map[string]any{"reason": err.Error()}), detail)
		}
	} else if !fact.PanelManaged {
		// 面板外且已无法定位（目录/文件被删）：没有可执行的 down，也没有可删的目录。
		return nil, fail(errcode.ErrDkComposeLocateFailed.WithData(LocateFailedData{
			WorkingDir: fact.WorkingDir, ConfigFile: fact.ConfigFile, Missing: fact.Missing}), detail)
	}
	deletedDir := false
	if fact.PanelManaged {
		// 目录路径由「落盘根 + 已过白名单的项目名」拼出，不取自 label 透传值——
		// 防止把 label 里伪造的路径当成删除目标。
		if !composeProjectRe.MatchString(project) {
			return nil, fail(errcode.ErrDkInvalidParam.WithData(map[string]any{
				"reason": "项目名不符合面板命名规则，拒绝删除目录"}), detail)
		}
		dir := filepath.Join(s.composeRoot, project)
		if err := os.RemoveAll(dir); err != nil {
			return nil, fail(errcode.ErrDKComposeDeleteFailed.WithData(map[string]any{
				"reason": "项目容器已停止并移除，但删除项目目录失败：" + err.Error()}), detail)
		}
		deletedDir = true
	}
	detail["deletedDir"] = deletedDir
	s.recordAudit(ctx, auditEntry(actor, AuditComposeProjectDeleted, ObjectTypeComposeProject, project,
		detail, audit.ResultSuccess))
	return &ComposeDeleteData{Project: project, DeletedDir: deletedDir}, nil
}
