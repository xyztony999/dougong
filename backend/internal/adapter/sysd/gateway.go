package sysd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// systemd dbus 常量（org.freedesktop.systemd1 契约）。
const (
	systemdDest    = "org.freedesktop.systemd1"
	systemdPath    = dbus.ObjectPath("/org/freedesktop/systemd1")
	managerIface   = systemdDest + ".Manager"
	unitIface      = systemdDest + ".Unit"
	propsIface     = "org.freedesktop.DBus.Properties"
	jobRemovedSig  = managerIface + ".JobRemoved"
	noSuchUnitErr  = systemdDest + ".NoSuchUnit"
	jobModeReplace = "replace" // systemd 默认 Job 模式（排队替换既有 Job）
	jobResultDone  = "done"    // JobRemoved result：成功终态
)

// dbusUnitRow 对应 ListUnits 返回 a(ssssssouso) 的单条（字段顺序即 dbus 序）。
type dbusUnitRow struct {
	Name, Description, LoadState, ActiveState, SubState, Followed string
	UnitPath                                                      dbus.ObjectPath
	JobID                                                         uint32
	JobType                                                       string
	JobPath                                                       dbus.ObjectPath
}

// dbusFileRow 对应 ListUnitFiles 返回 a(ss) 的单条（unit 文件绝对路径 + 状态）。
type dbusFileRow struct {
	Path  string
	State string
}

// dbusFileChange 对应 Enable/DisableUnitFiles 返回 changes a(sss) 的单条
// （type/file/source；仅作 Store 落位，不消费）。
type dbusFileChange struct {
	Type, File, Source string
}

// Gateway 是 systemd dbus 网关（SysdGateway 契约实现，§4.1.5）。
// 惰性连接 + 断连自动重连（连接死亡后下次调用 ensureConn 重建——无后台
// goroutine，G-06）；waitTimeout 为 Job 终态等待上限（D-01 90s，main 注入
// 编译期常量；超时返回 ErrJobTimeout 而非挂起，§4.1.5 约束）。
type Gateway struct {
	mu          sync.Mutex
	conn        *dbus.Conn
	waitTimeout time.Duration
}

// NewGateway 构造网关（连接在首次调用时建立；systemd 不可达即
// ErrUnavailable → SVC_008，I-01 不返回过期数据）。
func NewGateway(waitTimeout time.Duration) *Gateway {
	return &Gateway{waitTimeout: waitTimeout}
}

// ensureConn 返回活连接；无连接时建立并订阅 Manager 信号（JobRemoved 等待
// 依赖，A-S7：订阅先于任何 Job 提交，无竞态）。
func (g *Gateway) ensureConn() (*dbus.Conn, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conn != nil {
		return g.conn, nil
	}
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("%w: connect system bus: %v", ErrUnavailable, err)
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath(systemdPath),
		dbus.WithMatchInterface(managerIface),
	); err != nil {
		_ = conn.Close() //nolint:errcheck // teardown 弃错：订阅失败即弃新建连接，主错误 ErrUnavailable 已 %w 上报（Gate 2 I5）
		return nil, fmt.Errorf("%w: subscribe manager signals: %v", ErrUnavailable, err)
	}
	g.conn = conn
	return conn, nil
}

// dropConn 丢弃当前连接（仅连接已死亡时调用；触发下次调用重建——A-S8）。
func (g *Gateway) dropConn() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conn != nil {
		_ = g.conn.Close() //nolint:errcheck // teardown 弃错：仅由错误路径调用关闭已死连接，被弃错误不参与业务判定（Gate 2 I5；原 `conn` 未定义为先在编译错误，本轮修正为 g.conn）
		g.conn = nil
	}
}

// callErr 归一 dbus 调用错误（A-S8）：NoSuchUnit→ErrUnitNotFound；
// 连接关闭→ErrUnavailable（并丢弃死连接）；其余 dbus.Error→ErrRejected；
// 非 dbus 错误（传输/取消等）→ErrUnavailable。op 为操作描述（含 unit 名）。
func (g *Gateway) callErr(op string, err error) error {
	var derr dbus.Error
	if errors.As(err, &derr) {
		if derr.Name == noSuchUnitErr {
			return fmt.Errorf("%s: %w", op, ErrUnitNotFound)
		}
		return fmt.Errorf("%s: %w: %s", op, ErrRejected, derr.Name)
	}
	if errors.Is(err, dbus.ErrClosed) {
		g.dropConn()
		return fmt.Errorf("%s: %w: %v", op, ErrUnavailable, err)
	}
	return fmt.Errorf("%s: %w: %v", op, ErrUnavailable, err)
}

// ListUnits 返回已装 unit 全集投影（ListUnits ∪ ListUnitFiles 合并，
// I-03/A-S2；即时读取无缓存，I-01）。
func (g *Gateway) ListUnits(ctx context.Context) ([]UnitSnapshot, error) {
	conn, err := g.ensureConn()
	if err != nil {
		return nil, err
	}
	mgr := conn.Object(systemdDest, systemdPath)
	var rows []dbusUnitRow
	if err := mgr.CallWithContext(ctx, managerIface+".ListUnits", 0).Store(&rows); err != nil {
		return nil, g.callErr("list units", err)
	}
	var fileRows []dbusFileRow
	if err := mgr.CallWithContext(ctx, managerIface+".ListUnitFiles", 0).Store(&fileRows); err != nil {
		return nil, g.callErr("list unit files", err)
	}
	loaded := make([]LoadedUnit, 0, len(rows))
	for _, r := range rows {
		loaded = append(loaded, LoadedUnit{
			Name: r.Name, Description: r.Description,
			LoadState: r.LoadState, ActiveState: r.ActiveState, SubState: r.SubState,
		})
	}
	files := make([]UnitFile, 0, len(fileRows))
	for _, f := range fileRows {
		files = append(files, UnitFile{Name: FileBaseName(f.Path), State: f.State})
	}
	return MergeUnitProjection(loaded, files), nil
}

// GetUnit 读取单 unit 属性快照（LoadUnit 加载后 Properties.GetAll；实况
// 读取，I-01；不存在返回 ErrUnitNotFound）。
func (g *Gateway) GetUnit(ctx context.Context, name string) (UnitDetail, error) {
	conn, err := g.ensureConn()
	if err != nil {
		return UnitDetail{}, err
	}
	d, err := readUnitDetail(ctx, conn, name)
	if err != nil {
		return UnitDetail{}, g.callErr("get unit "+name, err)
	}
	return d, nil
}

// StartUnit 提交 start Job 并等待终态（≤waitTimeout，I-05/D-01）。
func (g *Gateway) StartUnit(ctx context.Context, name string) (JobResult, error) {
	return g.runUnitJob(ctx, "StartUnit", name)
}

// StopUnit 提交 stop Job 并等待终态。
func (g *Gateway) StopUnit(ctx context.Context, name string) (JobResult, error) {
	return g.runUnitJob(ctx, "StopUnit", name)
}

// RestartUnit 提交 restart Job 并等待终态（failed 服务首要恢复动作）。
func (g *Gateway) RestartUnit(ctx context.Context, name string) (JobResult, error) {
	return g.runUnitJob(ctx, "RestartUnit", name)
}

// EnableUnit 开机自启启用（轻量同步，无 Job 等待——api §3.4 调用形态）。
func (g *Gateway) EnableUnit(ctx context.Context, name string) error {
	conn, err := g.ensureConn()
	if err != nil {
		return err
	}
	var carries bool
	var changes []dbusFileChange
	call := conn.Object(systemdDest, systemdPath).
		CallWithContext(ctx, managerIface+".EnableUnitFiles", 0, []string{name}, false, false)
	if call.Err != nil {
		return g.callErr("enable "+name, call.Err)
	}
	if err := call.Store(&carries, &changes); err != nil {
		return g.callErr("enable "+name, err)
	}
	return nil
}

// DisableUnit 开机自启禁用（轻量同步）。
func (g *Gateway) DisableUnit(ctx context.Context, name string) error {
	conn, err := g.ensureConn()
	if err != nil {
		return err
	}
	var changes []dbusFileChange
	call := conn.Object(systemdDest, systemdPath).
		CallWithContext(ctx, managerIface+".DisableUnitFiles", 0, []string{name}, false)
	if call.Err != nil {
		return g.callErr("disable "+name, call.Err)
	}
	if err := call.Store(&changes); err != nil {
		return g.callErr("disable "+name, err)
	}
	return nil
}

// DaemonReload systemd Manager.Reload（db-tools D-07 新增扩展：单元文件
// 写入/移除后重载管理器配置；参数化 dbus 方法调用 R-08，非 shell；既有
// 方法零改动）。interactive=false（无人值守，不触发 polkit 交互）。
func (g *Gateway) DaemonReload(ctx context.Context) error {
	conn, err := g.ensureConn()
	if err != nil {
		return err
	}
	call := conn.Object(systemdDest, systemdPath).
		CallWithContext(ctx, managerIface+".Reload", 0, false)
	if call.Err != nil {
		return g.callErr("daemon-reload", call.Err)
	}
	return nil
}

// runUnitJob 是 start/stop/restart 共用编排（§4.1.5 等待包装）：提交 Job →
// JobRemoved 等待（job path 匹配，A-S7：StartUnit 返回的 job path 即移除
// 路径——merged job 语义）→ 尽力读取终态运行态。
func (g *Gateway) runUnitJob(ctx context.Context, method, name string) (JobResult, error) {
	conn, err := g.ensureConn()
	if err != nil {
		return JobResult{}, err
	}
	wctx, cancel := context.WithTimeout(ctx, g.waitTimeout)
	defer cancel()
	var jobPath dbus.ObjectPath
	call := conn.Object(systemdDest, systemdPath).
		CallWithContext(wctx, managerIface+"."+method, 0, name, jobModeReplace)
	if call.Err != nil {
		return JobResult{}, g.callErr(method+" "+name, call.Err)
	}
	if err := call.Store(&jobPath); err != nil {
		return JobResult{}, g.callErr(method+" "+name+" store", err)
	}
	result, err := waitJobResult(wctx, conn, jobPath)
	if err != nil {
		return JobResult{}, err
	}
	jr := JobResult{Succeeded: result == jobResultDone, Reason: result}
	if d, derr := readUnitDetail(ctx, conn, name); derr == nil {
		jr.ActiveState, jr.SubState = d.ActiveState, d.SubState
	}
	return jr, nil
}

// waitJobResult 等待目标 Job 的 JobRemoved 信号（body：id/job path/unit
// path/result）；ctx 超时/取消返回 ErrJobTimeout（超时语义而非挂起）。
func waitJobResult(ctx context.Context, conn *dbus.Conn, jobPath dbus.ObjectPath) (string, error) {
	ch := make(chan *dbus.Signal, 16)
	conn.Signal(ch)
	defer conn.RemoveSignal(ch)
	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("wait job %s: %w: %v", jobPath, ErrJobTimeout, ctx.Err())
		case sig := <-ch:
			if sig.Name != jobRemovedSig || len(sig.Body) < 4 {
				continue
			}
			job, ok := sig.Body[1].(dbus.ObjectPath)
			if !ok || job != jobPath {
				continue
			}
			result, _ := sig.Body[3].(string)
			return result, nil
		}
	}
}

// readUnitDetail LoadUnit（不存在时报 NoSuchUnit）+ Unit 属性全集 → detail。
func readUnitDetail(ctx context.Context, conn *dbus.Conn, name string) (UnitDetail, error) {
	var unitPath dbus.ObjectPath
	if err := conn.Object(systemdDest, systemdPath).
		CallWithContext(ctx, managerIface+".LoadUnit", 0, name).Store(&unitPath); err != nil {
		return UnitDetail{}, fmt.Errorf("load unit %q: %w", name, err)
	}
	props := map[string]dbus.Variant{}
	if err := conn.Object(systemdDest, unitPath).
		CallWithContext(ctx, propsIface+".GetAll", 0, unitIface).Store(&props); err != nil {
		return UnitDetail{}, fmt.Errorf("read unit %q props: %w", name, err)
	}
	return DetailFromProps(name, props), nil
}

// DetailFromProps 由 org.freedesktop.systemd1.Unit 属性表组装 detail（纯函数，
// 可测；缺键/空值落零值串，由 service 层映射 null）。
func DetailFromProps(name string, props map[string]dbus.Variant) UnitDetail {
	return UnitDetail{
		Name:         name,
		UnitType:     UnitTypeOf(name),
		ActiveState:  NormalizeActiveState(strVariant(props, "ActiveState")),
		SubState:     strVariant(props, "SubState"),
		EnabledState: strVariant(props, "UnitFileState"),
		Description:  strVariant(props, "Description"),
		LoadState:    strVariant(props, "LoadState"),
	}
}

// strVariant 取字符串属性（缺键/非串/空串均落 ""）。
func strVariant(props map[string]dbus.Variant, key string) string {
	v, ok := props[key]
	if !ok {
		return ""
	}
	s, ok := v.Value().(string)
	if !ok {
		return ""
	}
	return s
}
