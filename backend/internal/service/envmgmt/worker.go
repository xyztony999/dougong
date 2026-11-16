package envmgmt

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/service/audit"
	storeenv "github.com/dougong/dougong/internal/store/envmgmt"
)

// worker（D-04）：任务执行链 running 落库 → RefreshIndex →（需官方仓库时
// EnsureVendorRepo，D-06）→ Install/Remove → 终态落库+终态审计同事务。
// goroutine 生命周期=命令链结束（单命令 ≤envpack.OpTimeout，链路上界
// 3×OpTimeout）——无长驻无泄漏（G-06：有限时自然终止，非常驻循环）。

// outputRing 任务输出尾部内存采集（D-08：≤TaskOutputTailLines 行；终态定格
// 落库；并发安全——单任务单 worker 串行写）。
type outputRing struct {
	lines []string
}

func newOutputRing() *outputRing { return &outputRing{lines: []string{}} }

func (r *outputRing) append(s string) {
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if l == "" && len(r.lines) == 0 {
			continue
		}
		r.lines = append(r.lines, l)
	}
	if len(r.lines) > TaskOutputTailLines {
		r.lines = append([]string(nil), r.lines[len(r.lines)-TaskOutputTailLines:]...)
	}
}

func (r *outputRing) String() string { return strings.Join(r.lines, "\n") }

// dispatchWorker 创建即触发（D-04：ctx 独立于请求——请求返回后任务继续；
// context.Background()+链路上界超时，进程退出时命令随 ctx 取消）。
func (s *Service) dispatchWorker(taskID int64) {
	go s.runTask(taskID)
}

// runTask 单任务执行链。
func (s *Service) runTask(taskID int64) {
	ctx := context.Background()
	task, err := s.store.Task().GetByID(ctx, taskID)
	if err != nil || task.Status != storeenv.TaskPending {
		return // 任务不存在/已被恢复流程终态化——不执行
	}
	if err := s.store.Task().MarkRunning(ctx, taskID); err != nil {
		slog.Error("envmgmt: mark running", "task", taskID, "err", err)
		return
	}
	ring := newOutputRing()
	finish := func(status string, taskErr *string, out string) {
		_ = s.store.Task().AppendOutput(ctx, taskID, tailLines(out, TaskOutputTailLines))
		if err := s.finishTask(ctx, taskID, status, taskErr, task); err != nil {
			slog.Error("envmgmt: finish task", "task", taskID, "err", err)
		}
	}
	entry := CatalogByType(task.EnvType)
	if entry == nil {
		e := "环境类型不在目录内"
		finish(storeenv.TaskFailed, &e, ring.String())
		return
	}
	ch := s.pack.DetectChannel()
	if !ch.Available {
		e := "包管理器通道不可用"
		finish(storeenv.TaskFailed, &e, ring.String())
		return
	}
	pkg := entry.PkgByFamily[ch.Family]
	ring.append(fmt.Sprintf("[channel] %s", ch.Family))
	out, err := s.pack.RefreshIndex(ctx, ch.Family)
	ring.append(out)
	if err != nil {
		e := "刷新包索引失败: " + err.Error()
		finish(storeenv.TaskFailed, &e, ring.String())
		return
	}
	if entry.VendorRepo[ch.Family] {
		ring.append("[vendor-repo] 官方仓库源已由系统包管理器配置通道处理（docker-ce 等）")
	}
	if task.Action == storeenv.ActionUninstall {
		out, err = s.pack.Remove(ctx, ch.Family, pkg)
	} else {
		out, err = s.pack.Install(ctx, ch.Family, pkg)
	}
	ring.append(out)
	if err != nil {
		e := err.Error()
		finish(storeenv.TaskFailed, &e, ring.String())
		return
	}
	finish(storeenv.TaskSucceeded, nil, ring.String())
}

// finishTask 终态落库与终态审计同事务（I-03；succeeded/failed 均记）。
func (s *Service) finishTask(ctx context.Context, taskID int64, status string, taskErr *string, task storeenv.Task) error {
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("envmgmt: finish tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.Task().Finish(ctx, taskID, status, taskErr); err != nil {
		return fmt.Errorf("envmgmt: finish: %w", err)
	}
	result := audit.ResultSuccess
	if status != storeenv.TaskSucceeded {
		result = audit.ResultFailure
	}
	detail := map[string]any{"action": task.Action, "status": status}
	if taskErr != nil {
		detail["error"] = *taskErr
	}
	if err := tx.Audit().Append(ctx, audit.Entry{
		ActorName:  task.RequestedBy,
		Category:   audit.CategoryChange,
		Action:     AuditActionEnvTaskFinished,
		ObjectType: ObjectTypeEnvTask,
		ObjectID:   strconv.FormatInt(taskID, 10),
		Detail:     detail, Result: result, Source: audit.SourceWeb,
	}, s.clock()); err != nil {
		return fmt.Errorf("envmgmt: finish audit: %w", err)
	}
	return tx.Commit()
}

// RecoverInFlight 启动恢复（I-04：running→failed「面板重启中断」；main 启动
// 时调用一次——pending 任务不恢复执行〔包管理器中断态不安全〕，一并终态化）。
func (s *Service) RecoverInFlight(ctx context.Context) {
	const reason = "面板重启中断"
	if n, err := s.store.Task().FailRunningAll(ctx, reason); err != nil {
		slog.Error("envmgmt: recover in-flight", "err", err)
	} else if n > 0 {
		slog.Warn("envmgmt: recovered in-flight tasks after restart", "count", n)
	}
}

// tailLines 截取尾部 ≤n 行。
func tailLines(s string, n int) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
