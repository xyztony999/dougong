package appmgmt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/service/audit"
	storeapp "github.com/dougong/dougong/internal/store/appmgmt"
)

// worker（D-04/D-05/D-07）：任务执行链 running 落库 → 步骤执行 → 终态落库+
// 档案协作+终态审计同事务（I-C01/C02）。goroutine 生命周期=命令链结束（单命令
// ≤PYOpTimeout，链路上界 3×PYOpTimeout）——无长驻无泄漏（G-06：有限时自然终止）。

// dispatch 创建即触发（D-04：ctx 独立于请求——请求返回后任务继续；进程退出时
// 命令随 ctx 取消）。SyncDispatch=true 时同步执行（测试缝：确定性断言沿
// env-mgmt 测试先例，不依赖 goroutine 时序）。
func (s *Service) dispatch(taskID int64) {
	if s.syncDispatch {
		s.runTask(taskID)
		return
	}
	go s.runTask(taskID)
}

// createPlan create 任务执行计划（发起时经任务输出首行 plan:{json} 承载，
// worker 解析；字段与 CreateRequest 校验后子集一致）。
type createPlan struct {
	Dir        string    `json:"dir"`
	Entry      string    `json:"entry"`
	StartMode  string    `json:"startMode"`
	StartSpec  StartSpec `json:"startSpec"`
	ListenPort int       `json:"listenPort"`
	StartNow   bool      `json:"startNow"`
}

// runTask 单任务执行链。
func (s *Service) runTask(taskID int64) {
	ctx := context.Background()
	task, err := s.store.Task().GetByID(ctx, taskID)
	if err != nil || task.Status == storeapp.TaskSucceeded || task.Status == storeapp.TaskFailed {
		return // 任务不存在/已终态（含重启恢复终态化）——不执行；running=重试
		// 再入（D-13：Retry 已置 running，此处不得以非 pending 早退吞掉重试）
	}
	if err := s.store.Task().MarkRunning(ctx, taskID); err != nil {
		slog.Error("appmgmt: mark running", "task", taskID, "err", err)
		return
	}
	planJSON, ringText := splitPlan(task.OutputTail)
	ring := &outputRing{lines: splitLines(ringText)}
	switch task.Action {
	case storeapp.ActionCreate:
		s.runCreate(ctx, task, planJSON, ring)
	case storeapp.ActionDepsInstall:
		s.runDeps(ctx, task, ring)
	case storeapp.ActionDelete:
		s.runDelete(ctx, task, ring)
	default:
		e := "任务动作不在取值域"
		s.finish(ctx, taskID, task, storeapp.TaskFailed, &e, ring, nil)
	}
}

// runCreate 创建执行链（D-05）：CreateVenv →〔requirements.txt 存在〕PipInstall
// → WriteUnit（安全管线 D-06）→〔startNow〕Start → succeeded+档案登记同事务
// （I-C01；任一步失败=failed 无档案残留 I-C02——venv 等已建产物保留供重试）。
func (s *Service) runCreate(ctx context.Context, task storeapp.AppTask, planJSON string, ring *outputRing) {
	var plan createPlan
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
		e := "创建计划解析失败（任务参数损坏）"
		s.finish(ctx, task.ID, task, storeapp.TaskFailed, &e, ring, nil)
		return
	}
	ring.append("[venv] python3 -m venv .venv")
	if out, err := s.py.CreateVenv(ctx, plan.Dir, PYOpTimeout); err != nil {
		e := "创建 venv 失败：" + err.Error()
		s.finish(ctx, task.ID, task, storeapp.TaskFailed, &e, ring.appendErr(out), nil)
		return
	}
	if _, err := lookupFile(plan.Dir, "requirements.txt"); err == nil {
		ring.append("[deps] pip install -r requirements.txt")
		if out, err := s.py.PipInstall(ctx, plan.Dir, PYOpTimeout); err != nil {
			e := "依赖安装失败：" + err.Error()
			s.finish(ctx, task.ID, task, storeapp.TaskFailed, &e, ring.appendErr(out), nil)
			return
		}
	} else {
		ring.append("[deps] 无 requirements.txt，跳过依赖安装")
	}
	execStart := renderExecStart(plan.StartMode, plan.StartSpec, plan.ListenPort)
	ring.append("[unit] 写入服务单元 " + unitNameFor(task.AppName))
	if _, err := s.py.WriteUnit(ctx, unitNameFor(task.AppName),
		renderUnit(task.AppName, plan.Dir, execStart)); err != nil {
		e := "服务单元写入失败：" + err.Error()
		s.finish(ctx, task.ID, task, storeapp.TaskFailed, &e, ring, nil)
		return
	}
	if plan.StartNow {
		if _, err := s.sysd.StartUnit(ctx, unitNameFor(task.AppName)); err != nil {
			// 单元已就绪但启动失败：档案照常登记（服务异常态如实呈现，N2-8）。
			ring.append("[warn] 服务启动失败（档案照常登记，可在列表查看服务异常）：" + err.Error())
		}
	}
	archive := func(ctx context.Context, tx *storeapp.TxStore) error {
		_, err := tx.App().Create(ctx, storeapp.App{
			Name: task.AppName, Dir: plan.Dir, Entry: plan.Entry,
			StartMode: plan.StartMode, StartSpec: mustJSON(plan.StartSpec),
			ListenPort: plan.ListenPort, UnitName: unitNameFor(task.AppName),
			CreatedBy: task.RequestedBy,
		})
		return err
	}
	s.finish(ctx, task.ID, task, storeapp.TaskSucceeded, nil, ring, archive)
}

// runDeps 依赖安装执行链（C-16-02：requirements.txt 重装；失败可重试 D-13；
// 完成不自动重启服务——提示归前端，D-09）。
func (s *Service) runDeps(ctx context.Context, task storeapp.AppTask, ring *outputRing) {
	app, err := s.store.App().GetByID(ctx, task.AppID)
	if err != nil {
		e := "应用档案不存在（可能已被删除）"
		s.finish(ctx, task.ID, task, storeapp.TaskFailed, &e, ring, nil)
		return
	}
	ring.append("[deps] pip install -r requirements.txt")
	out, err := s.py.PipInstall(ctx, app.Dir, PYOpTimeout)
	ring.append(out)
	if err != nil {
		e := "依赖安装失败：" + err.Error()
		s.finish(ctx, task.ID, task, storeapp.TaskFailed, &e, ring, nil)
		return
	}
	s.finish(ctx, task.ID, task, storeapp.TaskSucceeded, nil, ring, nil)
}

// runDelete 删除执行链（D-07）：stop → disable → RemoveUnit（幂等，已不存在
// 跳过如实记输出）→〔purgeDir〕RemoveDir → succeeded+档案 DELETE 同事务
// （I-C01/C04；绝不静默删代码——目录删除仅 purgeDir=true）。
func (s *Service) runDelete(ctx context.Context, task storeapp.AppTask, ring *outputRing) {
	app, err := s.store.App().GetByID(ctx, task.AppID)
	if err != nil {
		e := "应用档案不存在（可能已被删除）"
		s.finish(ctx, task.ID, task, storeapp.TaskFailed, &e, ring, nil)
		return
	}
	ring.append("[service] 停止服务 " + app.UnitName)
	if _, err := s.sysd.StopUnit(ctx, app.UnitName); err != nil {
		ring.append("[warn] 停止服务失败（保守继续，单元撤除不依赖运行态）：" + err.Error())
	}
	if err := s.sysd.DisableUnit(ctx, app.UnitName); err != nil {
		ring.append("[warn] 撤销自启失败（保守继续）：" + err.Error())
	}
	ring.append("[unit] 移除服务单元")
	if err := s.py.RemoveUnit(ctx, app.UnitName); err != nil {
		e := "移除服务单元失败：" + err.Error()
		s.finish(ctx, task.ID, task, storeapp.TaskFailed, &e, ring, nil)
		return
	}
	if task.PurgeDir {
		ring.append("[dir] 删除代码目录（purgeDir=true）：" + app.Dir)
		if err := s.py.RemoveDir(app.Dir); err != nil {
			// 目录删除失败不阻断档案撤除（I-C04：残留如实记输出，用户可手动清理）。
			ring.append("[warn] 代码目录删除失败（档案照常撤除，目录残留请手动清理）：" + err.Error())
		}
	} else {
		ring.append("[dir] 代码目录保留：" + app.Dir)
	}
	ring.append("[note] 关联站点不受影响（如已建站请到站点管理自行处置）")
	archive := func(ctx context.Context, tx *storeapp.TxStore) error {
		return tx.App().Delete(ctx, app.ID)
	}
	s.finish(ctx, task.ID, task, storeapp.TaskSucceeded, nil, ring, archive)
}

// finish 终态落库+档案协作+终态审计同事务（I-C01/C03；succeeded/failed 均记；
// archive 为 create/delete 任务的档案协作步骤——deps 任务为 nil）。
func (s *Service) finish(ctx context.Context, taskID int64, task storeapp.AppTask,
	status string, taskErr *string, ring *outputRing, archive func(context.Context, *storeapp.TxStore) error) {
	_ = s.store.Task().AppendOutput(ctx, taskID, tailLines(ring.String(), TaskOutputTailLines))
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		slog.Error("appmgmt: finish tx", "task", taskID, "err", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.Task().Finish(ctx, taskID, status, taskErr); err != nil {
		slog.Error("appmgmt: finish", "task", taskID, "err", err)
		return
	}
	if archive != nil {
		if err := archive(ctx, tx); err != nil {
			slog.Error("appmgmt: finish archive", "task", taskID, "err", err)
			return // 事务回滚——任务终态与档案保持协作一致（I-C01 两半不可分）
		}
	}
	result := audit.ResultSuccess
	if status != storeapp.TaskSucceeded {
		result = audit.ResultFailure
	}
	detail := map[string]any{"action": task.Action, "status": status,
		"appName": task.AppName}
	if taskErr != nil {
		detail["error"] = *taskErr
	}
	if err := tx.Audit().Append(ctx, audit.Entry{
		ActorName: task.RequestedBy, Category: audit.CategoryChange,
		Action: AuditActionTaskFinished, ObjectType: ObjTypeTask,
		ObjectID: strconv.FormatInt(taskID, 10),
		Detail:   detail, Result: result, Source: audit.SourceWeb,
	}, s.clock()); err != nil {
		slog.Error("appmgmt: finish audit", "task", taskID, "err", err)
		return
	}
	if err := tx.Commit(); err != nil {
		slog.Error("appmgmt: finish commit", "task", taskID, "err", err)
	}
}

// RecoverInFlight 启动恢复（I-08：running→failed「面板重启中断」；main 启动
// 调用一次——pending 任务不恢复执行〔venv/pip 中断态不安全〕，一并终态化）。
func (s *Service) RecoverInFlight(ctx context.Context) {
	const reason = "面板重启中断"
	if n, err := s.store.Task().FailRunningAll(ctx, reason); err != nil {
		slog.Error("appmgmt: recover in-flight", "err", err)
	} else if n > 0 {
		slog.Warn("appmgmt: recovered in-flight tasks after restart", "count", n)
	}
}

// ---- 内部辅助 ----

// outputRing 任务输出尾部内存采集（≤TaskOutputTailLines 行；终态定格落库；
// 并发安全——单任务单 worker 串行写）。
type outputRing struct{ lines []string }

func appendRing(r *outputRing, s string) *outputRing {
	for _, l := range splitLines(s) {
		if l == "" && len(r.lines) == 0 {
			continue
		}
		r.lines = append(r.lines, l)
	}
	if len(r.lines) > TaskOutputTailLines {
		r.lines = append([]string(nil), r.lines[len(r.lines)-TaskOutputTailLines:]...)
	}
	return r
}

func (r *outputRing) append(s string) { appendRing(r, s) }

func (r *outputRing) appendErr(out string) *outputRing { return appendRing(r, out) }

func (r *outputRing) String() string { return strings.Join(r.lines, "\n") }

// splitPlan 拆分任务输出首行 plan:{json}（create 参数承载）与余下输出。
func splitPlan(outputTail string) (planJSON, rest string) {
	if !strings.HasPrefix(outputTail, planPrefix) {
		return "", outputTail
	}
	idx := strings.Index(outputTail, "\n")
	if idx < 0 {
		return strings.TrimPrefix(outputTail, planPrefix), ""
	}
	return strings.TrimPrefix(outputTail[:idx], planPrefix), outputTail[idx+1:]
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, strings.TrimSuffix(s[start:i], "\r"))
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, strings.TrimSuffix(s[start:], "\r"))
	}
	return out
}

func tailLines(s string, n int) string {
	lines := splitLines(s)
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// unitStates 单元详情→服务态指针（沿 env-mgmt unitServiceStates 同构：
// not-found/空=未装载 → null 如实不虚构「已停止」）。
func unitStates(d sysd.UnitDetail) (running, enabled *bool) {
	if d.LoadState == "not-found" || d.LoadState == "" {
		return nil, nil
	}
	r, e := d.ActiveState == "active", d.EnabledState == "enabled"
	return &r, &e
}

// portListening 本机端口监听探测（D-03 端口冲突/D-01 列表呈现；探测失败=错误
// 如实，调用方保持 null）。IPv4 回环+通配双探测。
func portListening(port int) (bool, error) {
	for _, addr := range []string{
		net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		net.JoinHostPort("0.0.0.0", strconv.Itoa(port)),
	} {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return true, nil // 绑定失败=被占用
		}
		_ = l.Close()
	}
	return false, nil
}

func lookupFile(dir, name string) (string, error) {
	p := filepath.Join(dir, name)
	fi, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return "", fmt.Errorf("not a file: %s", p)
	}
	return p, nil
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func jsonMarshal(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
