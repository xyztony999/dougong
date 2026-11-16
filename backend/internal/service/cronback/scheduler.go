package cronback

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	storecron "github.com/dougong/dougong/internal/store/cronback"
)

// scheduler.go 是 BackupScheduler（§3.2 进程内契约）：进程内分钟级 tick
// （D-13）；面板重启后重算各任务下次触发点——错过的调度点跳过不补跑、不落
// 任何跳过记录（run 只记实际发生的执行，无事实不虚构）。

// schedulerState 是各任务下一触发点内存表（重启即失——重算语义的载体）。
type schedulerState struct {
	mu    sync.Mutex
	next  map[int64]time.Time
	clock func() time.Time
}

// RunScheduler 调度主循环（main 编排启动；ctx 取消退出——G-06；tick panic
// 隔离）。
func (s *Service) RunScheduler(ctx context.Context) error {
	ticker := time.NewTicker(SchedulerTick)
	defer ticker.Stop()
	state := &schedulerState{next: map[int64]time.Time{}, clock: s.clock}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.scheduleTick(ctx, state)
		}
	}
}

// scheduleTick 单轮调度：检查全部 job 的 schedule_cron 到期（判定误差 <1min，
// D-13 编译期常量语义）；I-01 冲突（ErrRunMutex）= 该 job 已在执行 → 本周期
// 静默跳过（D-08 调度侧）。
func (s *Service) scheduleTick(ctx context.Context, state *schedulerState) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "cronback: scheduler tick panic", "panic", r)
		}
	}()
	jobs, err := s.store.Job().List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "cronback: scheduler list jobs", "err", err)
		return
	}
	now := s.clock()
	state.mu.Lock()
	defer state.mu.Unlock()
	nextState := make(map[int64]time.Time, len(jobs))
	for _, j := range jobs {
		expr, errs := ParseCronExpr(j.ScheduleCron)
		if len(errs) > 0 {
			// 定义落库前已校验（I-07）——防御性跳过并留运行日志。
			slog.WarnContext(ctx, "cronback: job schedule invalid, skip",
				"jobId", j.ID, "expr", j.ScheduleCron)
			continue
		}
		next, seen := state.next[j.ID]
		if !seen || next.IsZero() {
			// 重启/新任务：从当前时刻重算（错过的调度点跳过，D-13）。
			next = expr.Next(now)
		}
		if !next.IsZero() && !next.After(now) {
			s.fireCronRun(ctx, j, now)
			next = expr.Next(now)
		}
		nextState[j.ID] = next
	}
	state.next = nextState
}

// fireCronRun 到期触发：与立即执行共用同一 BackupExecutor 通道（D-13）；
// INSERT 冲突=已有执行在途→跳过该周期（D2 口径-4/I-01）。
func (s *Service) fireCronRun(ctx context.Context, j storecron.Job, now time.Time) {
	runID, err := s.store.Run().Create(ctx, j.ID, j.Name, storecron.TriggerCron, now)
	if err != nil {
		if errors.Is(err, storecron.ErrRunMutex) {
			slog.InfoContext(ctx, "cronback: skip schedule (job running)",
				"jobId", j.ID, "jobName", j.Name)
			return
		}
		slog.ErrorContext(ctx, "cronback: scheduler create run", "jobId", j.ID, "err", err)
		return
	}
	if !s.enqueue(workItem{runID: runID, startedAt: now, spec: s.jobSpecOf(&j)}) {
		s.failRunDirect(ctx, runID, now, "执行队列已满，请稍后重试")
	}
	slog.InfoContext(ctx, "cronback: scheduled run started",
		"jobId", j.ID, "jobName", j.Name, "runId", runID)
}
