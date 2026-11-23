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
	// finish 终态回写（bootstrapStatus/bootstrapNote 为 nil 时无引导步骤；
	// 数据库类 install/upgrade 成功后经 runBootstrap 定格引导结果，D-18/I-09）。
	finish := func(status string, taskErr *string, out string, bootstrapStatus, bootstrapNote *string) {
		_ = s.store.Task().AppendOutput(ctx, taskID, tailLines(out, TaskOutputTailLines))
		if err := s.finishTask(ctx, taskID, status, taskErr, bootstrapStatus, bootstrapNote, task); err != nil {
			slog.Error("envmgmt: finish task", "task", taskID, "err", err)
		}
	}
	entry := CatalogByType(task.EnvType)
	if entry == nil {
		e := "环境类型不在目录内"
		finish(storeenv.TaskFailed, &e, ring.String(), nil, nil)
		return
	}
	ch := s.pack.DetectChannel()
	if !ch.Available {
		e := "包管理器通道不可用"
		finish(storeenv.TaskFailed, &e, ring.String(), nil, nil)
		return
	}
	ring.append(fmt.Sprintf("[channel] %s", ch.Family))
	// D-06 v1.1 次序：代配先于索引刷新（新写入源须参与本次 update/makecache）；
	// 仅变更安装类动作（卸载不需要网络与第三方源）。
	if task.Action != storeenv.ActionUninstall && entry.VendorRepo[ch.Family] {
		summary, verr := s.pack.EnsureVendorRepo(ctx, task.EnvType, ch.Family)
		if summary != "" {
			ring.append("[vendor-repo] " + summary)
		}
		if verr != nil {
			e := "第三方源代配失败: " + verr.Error()
			finish(storeenv.TaskFailed, &e, ring.String(), nil, nil)
			return
		}
		s.recordRepoAudit(ctx, task, ch.Family, summary)
	}
	// 包名解析（D-02 多候选）：安装/升级=候选首选；卸载=实际命中（已装）候选，
	// 命中丢失（创建后被外部卸载）→ 任务 failed 如实，不盲删首选。
	var pkg string
	if task.Action == storeenv.ActionUninstall {
		p, _, found, _ := s.installedPkg(ctx, entry, ch.Family)
		if !found {
			e := "包数据库未检出已装候选（可能已被系统外部变更），卸载中止"
			finish(storeenv.TaskFailed, &e, ring.String(), nil, nil)
			return
		}
		pkg = p
	} else {
		cands := entry.PkgByFamily[ch.Family]
		if len(cands) == 0 {
			e := "目录映射缺少候选包名（family=" + ch.Family + "）"
			finish(storeenv.TaskFailed, &e, ring.String(), nil, nil)
			return
		}
		pkg = cands[0]
	}
	out, err := s.pack.RefreshIndex(ctx, ch.Family)
	ring.append(out)
	if err != nil {
		e := "刷新包索引失败: " + err.Error()
		finish(storeenv.TaskFailed, &e, ring.String(), nil, nil)
		return
	}
	if task.Action == storeenv.ActionUninstall {
		out, err = s.pack.Remove(ctx, ch.Family, pkg)
	} else {
		out, err = s.pack.Install(ctx, ch.Family, pkg)
	}
	ring.append(out)
	if err != nil {
		e := err.Error()
		finish(storeenv.TaskFailed, &e, ring.String(), nil, nil)
		return
	}
	// 附加包随主包安装（v0.1.15 实机修复：PHP-FPM 必需扩展 mysqli/mbstring/zip/gd/curl，
	// phpMyAdmin 依赖）——best-effort：失败记日志+输出环注记，不阻断主包终态（发行版
	// 差异：部分扩展内置或包名不同；phpMyAdmin 依赖检测仍如实报告缺失项引导补装）。
	if task.Action != storeenv.ActionUninstall {
		for _, extra := range entry.ExtraPkgsByFamily[ch.Family] {
			eout, eerr := s.pack.Install(ctx, ch.Family, extra)
			ring.append(eout)
			if eerr != nil {
				slog.WarnContext(ctx, "envmgmt: extra package install failed (best-effort)",
					"env", entry.Type, "pkg", extra, "err", eerr)
				ring.append("[warn] 附加包 " + extra + " 安装失败（不阻断主包）: " + eerr.Error())
			}
		}
	}
	// Node 环境随装 PM2 并注册开机自启（ADR-033 D-16：Node 项目由 PM2 托管进程）。
	// best-effort：失败记日志+输出环注记，不阻断 Node 主包终态——就绪度探测会
	// 如实呈现缺 PM2，创建 Node 项目时拒绝并引导回环境管理升级补装（与 v0.3.0
	// pip 补装同一机制）。卸载不触碰 PM2 进程列表（面板无法判断是否仍有项目依赖）。
	if task.Action != storeenv.ActionUninstall && entry.Type == storeenv.EnvNode {
		pout, perr := s.pack.NpmInstallGlobal(ctx, "pm2")
		ring.append(pout)
		if perr != nil {
			slog.WarnContext(ctx, "envmgmt: pm2 install failed (best-effort)", "err", perr)
			ring.append("[warn] PM2 安装失败（不阻断 Node 安装；Node 项目需要 PM2，可稍后升级补装）: " + perr.Error())
		} else {
			sout, serr := s.pack.Pm2Startup(ctx)
			ring.append(sout)
			if serr != nil {
				slog.WarnContext(ctx, "envmgmt: pm2 startup registration failed (best-effort)", "err", serr)
				ring.append("[warn] PM2 开机自启注册失败（不阻断；开机后需手动 pm2 resurrect）: " + serr.Error())
			}
		}
	}
	// 安装后配置就绪引导（v1.2，D-15~D-19）：仅数据库类 install/upgrade 成功后
	// 执行（runBootstrap 内部按 bootstrapEnvTypes 判定，非数据库返回 nil）；引导
	// 失败不改包安装 succeeded（独立维度，⚠️A14），结果随终态同事务定格（I-09）。
	bStatus, bNote := s.runBootstrap(ctx, task, ring)
	finish(storeenv.TaskSucceeded, nil, ring.String(), bStatus, bNote)
}

// recordRepoAudit 代配写入审计（env_repo_configured，D-06；失败重试一次后
// 日志兜底——审计失败不阻断任务，与 recordAudit 同口径）。
func (s *Service) recordRepoAudit(ctx context.Context, task storeenv.Task, family, summary string) {
	e := audit.Entry{
		ActorName: task.RequestedBy, Category: audit.CategoryChange,
		Action: AuditActionEnvRepoConfigured, ObjectType: ObjectTypeEnv, ObjectID: task.EnvType,
		Detail: map[string]any{"envType": task.EnvType, "family": family, "summary": summary},
		Result: audit.ResultSuccess, Source: audit.SourceWeb,
	}
	if err := s.audit(ctx, e); err != nil {
		if err := s.audit(ctx, e); err != nil {
			slog.WarnContext(ctx, "envmgmt: repo-config audit degraded", "envType", task.EnvType, "err", err)
		}
	}
}

// finishTask 终态落库与终态审计同事务（I-03；succeeded/failed 均记；引导结果
// bootstrap_status/note 同事务定格，v1.2 I-09）。
func (s *Service) finishTask(ctx context.Context, taskID int64, status string, taskErr, bootstrapStatus, bootstrapNote *string, task storeenv.Task) error {
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("envmgmt: finish tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.Task().Finish(ctx, taskID, status, taskErr, bootstrapStatus, bootstrapNote); err != nil {
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
