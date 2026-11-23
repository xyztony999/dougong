// Package appmgmt 是应用管理（app-mgmt）模块的数据访问层（api-app-mgmt.md
// §5.2/§5.3；SQL 只允许出现在本包）。app（应用档案）与 app_task（应用变更
// 任务）为本模块双聚合根（domain-model.md v1.27）；实况投影零台账（D-01
// 实时探测）；.env/unit 文件为外部事实源不入库（OUT-14）。
package appmgmt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dougong/dougong/internal/store"
)

// Executor 是 store.Executor 的别名（*sql.DB 与 *sql.Tx 共同满足）。
type Executor = store.Executor

// app_runtime / app_start_mode / app_task_action / app_task_status 取值域
// （DDL CHECK 兜底；ADR-033 宝塔式重组）。
const (
	RuntimePython = "python" // Python 项目（venv + systemd，既有）
	RuntimeNode   = "node"   // Node 项目（PM2 托管进程，ADR-033 D-16）

	StartModePresetFastAPI = "preset_fastapi" // FastAPI 预设（venv 内 uvicorn 模板，D-06）
	StartModeCustom        = "custom"         // 自定义命令（venv 内相对路径白名单，D-06）
	StartModeNodeFile      = "node_file"      // Node 启动文件（项目内相对路径，启动器加载，D-16）
	StartModeNpmScript     = "npm_script"     // Node npm 脚本（npm run <脚本名>，D-16）
	// StartModeContainer 容器化形态——已下线（ADR-033 D-app-04）：DDL CHECK 保留
	// 历史值，服务层拒绝（APP_014）；容器化部署归 docker-mgmt「应用」页签。
	StartModeContainer = "container"

	ActionCreate      = "create"       // 创建应用（Python：venv→依赖→单元；Node：依赖→PM2 注册）
	ActionDepsInstall = "deps_install" // 依赖重装（Python：requirements.txt；Node：npm install）
	ActionDelete      = "delete"       // 删除应用（停进程撤托管撤档案）

	TaskPending   = "pending"
	TaskRunning   = "running"
	TaskSucceeded = "succeeded"
	TaskFailed    = "failed"
)

// ErrAppNotFound / ErrTaskNotFound 应用或任务不存在（APP_007/APP_008 语义）。
var (
	ErrAppNotFound  = errors.New("store/appmgmt: app not found")
	ErrTaskNotFound = errors.New("store/appmgmt: task not found")
)

// App 对应 app 表一行（聚合根：运行时项目档案，api §4.1.1；ADR-033 扩 runtime）。
type App struct {
	ID         int64
	Name       string
	Dir        string
	Entry      string
	Runtime    string // python | node（迁移 018；缺省 python）
	StartMode  string
	StartSpec  string // 启动方式值对象 JSON（D-06/D-16；整列替换式更新）
	ListenPort int
	UnitName   string // Python：systemd 单元名；Node：PM2 进程名（dougong-app-{名称}）
	CreatedBy  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// AppTask 对应 app_task 表一行（聚合根：应用变更任务，api §4.2.1）。
type AppTask struct {
	ID          int64
	AppID       int64 // create 任务=0（档案登记前未分配语义，api §4.2.1 备注）
	AppName     string
	Action      string
	Status      string
	PurgeDir    bool
	OutputTail  string
	Error       *string
	RequestedBy string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func scanTime(src any) (time.Time, error) {
	switch v := src.(type) {
	case string:
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}, fmt.Errorf("store/appmgmt: parse time %q: %w", v, err)
		}
		return t, nil
	case time.Time:
		return v, nil
	default:
		return time.Time{}, fmt.Errorf("store/appmgmt: unexpected time column type %T", src)
	}
}

func scanTimePtr(src any) (*time.Time, error) {
	if src == nil {
		return nil, nil
	}
	t, err := scanTime(src)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("store/appmgmt: %s: %w", op, err)
}

func strPtrArg(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// Store 聚合 app-mgmt 仓储 + 复用 account-auth audit_log 仓储（任务创建/
// 终态与审计同事务 I-07）。
type Store struct{ db *sql.DB }

// New 构造仓储聚合。
func New(db *sql.DB) *Store { return &Store{db: db} }

// App 返回应用档案仓储。
func (s *Store) App() *AppRepo { return &AppRepo{x: s.db} }

// Task 返回任务仓储。
func (s *Store) Task() *TaskRepo { return &TaskRepo{x: s.db} }

// BeginTx 开启事务视图（任务创建+发起审计 / 终态+档案协作+审计 同事务，
// I-C01/C02/C03）。
type TxStore struct{ tx *sql.Tx }

func (s *Store) BeginTx(ctx context.Context) (*TxStore, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store/appmgmt: begin tx: %w", err)
	}
	return &TxStore{tx: tx}, nil
}

// Commit 提交事务。
func (t *TxStore) Commit() error {
	if err := t.tx.Commit(); err != nil {
		return fmt.Errorf("store/appmgmt: commit: %w", err)
	}
	return nil
}

// Rollback 回滚（已结束时错误由调用方按语义判断）。
func (t *TxStore) Rollback() error {
	if err := t.tx.Rollback(); err != nil {
		return fmt.Errorf("store/appmgmt: rollback: %w", err)
	}
	return nil
}

// App 返回事务内应用档案仓储。
func (t *TxStore) App() *AppRepo { return &AppRepo{x: t.tx} }

// Task 返回事务内任务仓储。
func (t *TxStore) Task() *TaskRepo { return &TaskRepo{x: t.tx} }
