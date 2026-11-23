package appmgmt

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// ---- 业务常量（api-app-mgmt.md §3.4；编译期钉死）----

const (
	// TaskOutputTailLines 输出尾部采集行数（沿 env-mgmt D-08 同值）。
	TaskOutputTailLines = 200
	// TaskListPageSize 任务列表默认分页（上限 100）。
	TaskListPageSize = 50
	// TaskListPageSizeMax 任务列表分页上限。
	TaskListPageSizeMax = 100
	// PYOpTimeout venv 创建/单次 pip 安装命令超时上限（D-05；沿 PKG_OP_TIMEOUT 同量级）。
	PYOpTimeout = 600 * time.Second
	// UnitPrefix 服务单元命名前缀（D-06；单元名=dougong-app-{name}.service）。
	UnitPrefix = "dougong-app-"
	// LogTailOptions 日志尾部档位（D-11）。
	LogTailDefault = 200
	// EnvFileName 应用环境变量文件名（⚠️A10：应用目录 .env，unit EnvironmentFile 引用）。
	EnvFileName = ".env"
	// AppNamePattern 应用名格式约束（D-03/APP_001）。
	AppNamePattern = `^[a-z0-9][a-z0-9-]{0,31}$`
)

// ---- DTO（api §4.1.4/§4.3 接口详情逐形状；可空显式指针）----

// PythonReadyData 就绪度（GET /app/app/list 页头；D-08）。
type PythonReadyData struct {
	Installed    bool    `json:"installed"`
	Version      *string `json:"version"`
	VenvSupport  *bool   `json:"venvSupport"`
	PipAvailable *bool   `json:"pipAvailable"`
}

// NodeReadyData Node 环境就绪度（GET /app/app/list 响应；ADR-033 D-16：
// node / npm / PM2 三项可用性——PM2 为 Node 项目的进程托管依赖）。
type NodeReadyData struct {
	NodeAvailable bool    `json:"nodeAvailable"`
	NpmAvailable  bool    `json:"npmAvailable"`
	Pm2Available  bool    `json:"pm2Available"`
	NodeVersion   *string `json:"nodeVersion"` // 缺席 null 如实
}

// AppItem 应用列表条目（档案+实况投影合并，D-01；探测失败字段 null 如实）。
// ADR-033：runtime 区分 Python / Node；Node 条目的服务状态来自 PM2，另带
// PM2 原值状态、重启次数与内存占用（Python 条目这三项为 null）。
type AppItem struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	Entry     string `json:"entry"`
	Runtime   string `json:"runtime"`
	StartMode string `json:"startMode"`
	// StartSpec 应用档案里实际保存的启动参数（ADR-033 起随清单下发：参数编辑
	// 据此预填——此前前端只能拿创建表单的上次输入顶替，对早先创建的应用保存
	// 参数会把它悄悄改成默认值，或因命令为空而保存失败）。档案损坏时 null。
	StartSpec      *StartSpec `json:"startSpec"`
	ListenPort     int        `json:"listenPort"`
	ServiceRunning *bool      `json:"serviceRunning"`
	ServiceEnabled *bool      `json:"serviceEnabled"`
	VenvExists     bool       `json:"venvExists"`
	DepsInstalled  *bool      `json:"depsInstalled"`
	PortListening  *bool      `json:"portListening"`
	RunningTaskID  *int64     `json:"runningTaskId"`
	HasSite        *bool      `json:"hasSite"`
	UnitFailed     bool       `json:"unitFailed"`
	Pm2Status      *string    `json:"pm2Status"`
	Restarts       *int       `json:"restarts"`
	MemoryBytes    *int64     `json:"memoryBytes"`
	CreatedAt      string     `json:"createdAt"`
	CreatedBy      string     `json:"createdBy"`
}

// AppListData GET /app/app/list 响应。
type AppListData struct {
	PythonReadiness PythonReadyData `json:"pythonReadiness"`
	NodeReadiness   NodeReadyData   `json:"nodeReadiness"`
	Items           []AppItem       `json:"items"`
	GeneratedAt     string          `json:"generatedAt"`
}

// TaskCreatedData 任务发起响应（create/delete/deps/retry）。
type TaskCreatedData struct {
	TaskID int64 `json:"taskId"`
}

// StartSpec 启动方式值对象（D-06：preset_fastapi=module+app+args；custom=
// command；ADR-033 D-16：node_file=file、npm_script=script）。
type StartSpec struct {
	Module  string `json:"module,omitempty"`
	App     string `json:"app,omitempty"`
	Args    string `json:"args,omitempty"`
	Command string `json:"command,omitempty"`
	File    string `json:"file,omitempty"`   // Node 启动文件（项目内相对路径）
	Script  string `json:"script,omitempty"` // Node npm 脚本名（缺省 start）
}

// parseStartSpec 解析并校验启动方式值对象（D-06/APP_012 白名单：
// custom 须 .venv/bin/ 相对路径开头且无 shell 元字符；preset 须 module/app 非空）。
func parseStartSpec(mode, raw string) (StartSpec, error) {
	var sp StartSpec
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &sp); err != nil {
			return sp, errcode.ErrAppParamsInvalid.WithData(map[string]any{
				"reason": "启动参数 JSON 非法"})
		}
	}
	if mode == "custom" {
		cmd := strings.TrimSpace(sp.Command)
		if cmd == "" {
			return sp, errcode.ErrAppParamsInvalid.WithData(map[string]any{
				"reason": "自定义启动命令为空"})
		}
		if !strings.HasPrefix(cmd, ".venv/bin/") {
			return sp, errcode.ErrAppParamsInvalid.WithData(map[string]any{
				"reason": "自定义命令须以 .venv/bin/ 相对路径开头"})
		}
		if strings.ContainsAny(cmd, ";|&$`<>\"'\\\n") {
			return sp, errcode.ErrAppParamsInvalid.WithData(map[string]any{
				"reason": "自定义命令含非法字符（shell 元字符）"})
		}
	} else {
		if strings.TrimSpace(sp.Module) == "" || strings.TrimSpace(sp.App) == "" {
			return sp, errcode.ErrAppParamsInvalid.WithData(map[string]any{
				"reason": "FastAPI 预设须填写模块与实例名（如 app:app）"})
		}
	}
	return sp, nil
}

// renderExecStart 渲染 unit ExecStart（D-06：preset=uvicorn 模板；custom=原命令）。
func renderExecStart(mode string, sp StartSpec, port int) string {
	if mode == "custom" {
		return strings.TrimSpace(sp.Command)
	}
	base := fmt.Sprintf(".venv/bin/uvicorn %s:%s --host 127.0.0.1 --port %d",
		strings.TrimSpace(sp.Module), strings.TrimSpace(sp.App), port)
	if a := strings.TrimSpace(sp.Args); a != "" && !strings.ContainsAny(a, ";|&$`<>\"'\\\n") {
		base += " " + a
	}
	return base
}

// renderUnit 渲染 systemd 服务单元内容（D-06：默认绑 127.0.0.1 + Restart=on-failure
// + EnvironmentFile 引用应用目录 .env；unit 内容为外部事实源渲染产物，不入库）。
func renderUnit(appName, dir, execStart string) string {
	return fmt.Sprintf(`[Unit]
Description=dougong managed app: %s
After=network.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s
EnvironmentFile=%s/.env
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
`, appName, dir, execStart, dir)
}

// unitNameFor 派生服务单元名（I-01：dougong-app-{name}.service）。
func unitNameFor(name string) string { return UnitPrefix + name + ".service" }

// startSpecOf 解析档案里实际保存的启动方式值对象（清单下发供参数编辑预填）。
// 空串或解析失败返回 nil 如实——档案损坏不阻断清单，参数编辑时后端仍会按
// 白名单校验新提交的值。
func startSpecOf(raw string) *StartSpec {
	if raw == "" {
		return nil
	}
	var sp StartSpec
	if err := json.Unmarshal([]byte(raw), &sp); err != nil {
		return nil
	}
	return &sp
}

// AppTaskItem 任务条目（task/list·detail；retryable=D-13 前端按钮态数据源）。
type AppTaskItem struct {
	ID          int64   `json:"id"`
	AppID       int64   `json:"appId"`
	AppName     string  `json:"appName"`
	Action      string  `json:"action"`
	Status      string  `json:"status"`
	PurgeDir    bool    `json:"purgeDir"`
	RequestedBy string  `json:"requestedBy"`
	CreatedAt   string  `json:"createdAt"`
	StartedAt   *string `json:"startedAt,omitempty"`
	FinishedAt  *string `json:"finishedAt,omitempty"`
	Error       *string `json:"error,omitempty"`
	Retryable   bool    `json:"retryable"`
}

// AppTaskListData GET /app/task/list 响应。
type AppTaskListData struct {
	Items []AppTaskItem `json:"items"`
	Total int           `json:"total"`
}

// AppTaskDetailData GET /app/task/detail 响应（list 项全字段+outputTail）。
type AppTaskDetailData struct {
	AppTaskItem
	OutputTail string `json:"outputTail"`
}

// EnvData GET /app/app/env 响应（.env 现文件内容原样行——用户自有应用配置，
// ⚠️A10 admin 可见；文件不存在=空数组如实）。
type EnvData struct {
	AppID     int64    `json:"appId"`
	Lines     []string `json:"lines"`
	UpdatedAt *string  `json:"updatedAt"`
}

// ParamsSaveData POST /app/app/params 响应（needRestart 恒 true——提示归前端）。
type ParamsSaveData struct {
	AppID       int64 `json:"appId"`
	NeedRestart bool  `json:"needRestart"`
}

// ServiceOpData 服务操作响应（D-10；失败走 APP_010 data.reason）。
type ServiceOpData struct {
	AppID int64  `json:"appId"`
	Unit  string `json:"unit"`
}

// LogData GET /app/log 响应（journalctl 尾部原文；admin 专属 ⚠️A8）。
type LogData struct {
	AppID   int64  `json:"appId"`
	Unit    string `json:"unit"`
	Lines   int    `json:"lines"`
	Content string `json:"content"`
}

// SiteLinkData POST /app/site/link 响应（站点引用值快照，不落库——D-12）。
type SiteLinkData struct {
	AppID int64  `json:"appId"`
	Note  string `json:"note"`
}

// ---- 审计与时间辅助（沿 env-mgmt 先例）----

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

func formatTS(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// fmtMtime 文件 mtime 基线（纳秒精度——秒级 RFC3339 同秒写入碰撞会使基线
// 比对失效，D-09 乐观锁要求）。
func fmtMtime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
}

func timePtrStr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	v := formatTS(*t)
	return &v
}

func fmtID(id int64) string { return fmt.Sprintf("%d", id) }

// ---- errcode 便捷构造 ----

func errTaskInFlight(taskID int64) *errcode.Error {
	return errcode.ErrAppTaskInFlight.WithData(map[string]any{"taskId": taskID})
}

func errUnitOp(reason string) *errcode.Error {
	return errcode.ErrAppUnitOpFailed.WithData(map[string]any{"reason": reason})
}

func errSiteLink(reason string) *errcode.Error {
	return errcode.ErrAppSiteLinkFailed.WithData(map[string]any{"reason": reason})
}

func errParams(reason string) *errcode.Error {
	return errcode.ErrAppParamsInvalid.WithData(map[string]any{"reason": reason})
}

func errDir(reason string) *errcode.Error {
	return errcode.ErrAppDirInvalid.WithData(map[string]any{"reason": reason})
}

func strPtr(s string) *string { return &s }
