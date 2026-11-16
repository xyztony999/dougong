package cronback

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/store"
)

// orphanMessageLiteral 与 service 层 OrphanRunMessage 同文案（store 层不依赖 service）。
const orphanMessageLiteral = "面板重启中断"

// newStore 打开 :memory: SQLite 并应用全部迁移（含 006_cron_backup.sql）。
func newStore(t *testing.T) *Store {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if cerr := db.Close(); cerr != nil {
			t.Errorf("close db: %v", cerr)
		}
	})
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(db)
}

func sampleJob(name string) *Job {
	return &Job{
		Name: name,
		SourceItems: []SourceItem{
			{Type: SourceSiteDir, Path: "/var/www/site"},
		},
		TargetType:     TargetLocal,
		TargetConfig:   TargetConfig{Path: "/var/backups"},
		RetentionCount: 3,
		ScheduleCron:   "0 3 * * *",
	}
}

func TestJobRepoCRUD(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	id, err := s.Job().Create(ctx, sampleJob("nightly"), now)
	if err != nil || id <= 0 {
		t.Fatalf("create: %v (id=%d)", err, id)
	}
	// I-06 唯一：同名再建 → ErrJobNameTaken。
	if _, err := s.Job().Create(ctx, sampleJob("nightly"), now); !errors.Is(err, ErrJobNameTaken) {
		t.Fatalf("dup name err = %v, want ErrJobNameTaken", err)
	}
	got, err := s.Job().GetByName(ctx, "nightly")
	if err != nil || got == nil {
		t.Fatalf("get by name: %v %v", got, err)
	}
	if got.SourceItems[0].Path != "/var/www/site" || got.TargetType != TargetLocal ||
		got.RetentionCount != 3 || got.ScheduleCron != "0 3 * * *" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	// 整体替换（值对象替换语义）。
	got.Name = "nightly-v2"
	got.RetentionCount = 7
	if err := s.Job().Update(ctx, got, now.Add(time.Minute)); err != nil {
		t.Fatalf("update: %v", err)
	}
	again, _ := s.Job().GetByID(ctx, id)
	if again.Name != "nightly-v2" || again.RetentionCount != 7 {
		t.Fatalf("update not applied: %+v", again)
	}
	// 更新撞他人名字 → ErrJobNameTaken。
	s.Job().Create(ctx, sampleJob("other"), now)
	again.Name = "other"
	if err := s.Job().Update(ctx, again, now); !errors.Is(err, ErrJobNameTaken) {
		t.Fatalf("update dup err = %v, want ErrJobNameTaken", err)
	}
	// 删除；run 不级联（D-12）。
	if _, err := s.Run().Create(ctx, id, "nightly-v2", TriggerManual, now); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := s.Job().Delete(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.Job().Delete(ctx, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("second delete err = %v, want ErrNoRows", err)
	}
	run, err := s.Run().GetByID(ctx, 1)
	if err != nil || run == nil {
		t.Fatalf("run must survive job deletion (D-12): %v %v", run, err)
	}
	if run.JobName != "nightly-v2" { // I-03 快照
		t.Fatalf("job_name snapshot = %q", run.JobName)
	}
}

func TestRunMutexAndTerminalImmutability(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	id, _ := s.Job().Create(ctx, sampleJob("j"), now)

	// I-01：同 job 第二条 running 冲突（部分唯一索引）。
	if _, err := s.Run().Create(ctx, id, "j", TriggerManual, now); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if _, err := s.Run().Create(ctx, id, "j", TriggerCron, now); !errors.Is(err, ErrRunMutex) {
		t.Fatalf("mutex err = %v, want ErrRunMutex", err)
	}
	// 不同 job 不互斥。
	id2, _ := s.Job().Create(ctx, sampleJob("j2"), now)
	if _, err := s.Run().Create(ctx, id2, "j2", TriggerManual, now); err != nil {
		t.Fatalf("other job run: %v", err)
	}

	// 终态一次转移（I-02）：success 后再 Finish 失败 → 0 行。
	if n, err := s.Run().FinishSuccess(ctx, 1, now.Add(time.Minute), "/bk/a", 123,
		[]FileEntry{{Name: "site-1.tar.gz", Size: 123}}); err != nil || n != 1 {
		t.Fatalf("finish success: n=%d err=%v", n, err)
	}
	if n, err := s.Run().FinishFailure(ctx, 1, now.Add(2*time.Minute), "late"); err != nil || n != 0 {
		t.Fatalf("terminal re-write must be rejected: n=%d err=%v", n, err)
	}
	run, _ := s.Run().GetByID(ctx, 1)
	if run.Status != StatusSuccess || run.ArtifactPath == nil || *run.ArtifactPath != "/bk/a" ||
		len(run.Files) != 1 || run.ErrorMessage != nil {
		t.Fatalf("success state wrong: %+v", run)
	}
	// running 态 FinishSuccess 正常路径先验证（run 2 → failure）。
	if n, err := s.Run().FinishFailure(ctx, 2, now.Add(time.Minute), "db dump failed"); err != nil || n != 1 {
		t.Fatalf("finish failure: n=%d err=%v", n, err)
	}
	run2, _ := s.Run().GetByID(ctx, 2)
	if run2.Status != StatusFailure || run2.ArtifactPath != nil || *run2.ErrorMessage != "db dump failed" {
		t.Fatalf("failure state wrong: %+v", run2)
	}
}

func TestRotationSemantics(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	id, _ := s.Job().Create(ctx, sampleJob("rot"), now)

	// 构造历史：S1,S2,F3,S4,S5,S6（id 序），retention=3 → 保留 S4,S5,S6（最近
	// 3 条 success），删除 id < 4 的全部记录（S1、S2、F3——更早 success 与更早
	// failure 都删，D2 口径-5/D-09）。
	states := []struct {
		status string
		path   string
	}{
		{StatusSuccess, "/bk/1"}, {StatusSuccess, "/bk/2"}, {StatusFailure, ""},
		{StatusSuccess, "/bk/4"}, {StatusSuccess, "/bk/5"}, {StatusSuccess, "/bk/6"},
	}
	for i, st := range states {
		runID, err := s.Run().Create(ctx, id, "rot", TriggerCron, now.Add(time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatalf("create run %d: %v", i, err)
		}
		if st.status == StatusSuccess {
			if _, err := s.Run().FinishSuccess(ctx, runID,
				now.Add(time.Duration(i)*time.Minute+time.Second), st.path, 10,
				[]FileEntry{{Name: "x", Size: 10}}); err != nil {
				t.Fatalf("finish %d: %v", i, err)
			}
		} else {
			if _, err := s.Run().FinishFailure(ctx, runID,
				now.Add(time.Duration(i)*time.Minute+time.Second), "boom"); err != nil {
				t.Fatalf("finish %d: %v", i, err)
			}
		}
	}
	// 先互斥恢复：最后一条 running 已终态，无阻塞。
	tx, err := s.BeginTx(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	victims, err := tx.Run().Rotate(ctx, id, 3)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if len(victims) != 3 {
		t.Fatalf("victims = %d, want 3 (S1,S2,F3)", len(victims))
	}
	paths := map[string]bool{}
	for _, v := range victims {
		paths[v.ArtifactPath] = true
	}
	if !paths["/bk/1"] || !paths["/bk/2"] || len(victims) != 3 {
		t.Fatalf("victim paths wrong: %+v", victims)
	}
	runs, total, err := s.Run().Search(ctx, RunFilter{JobID: &id}, 0, 100)
	if err != nil || total != 3 {
		t.Fatalf("remaining = %d err=%v, want 3", total, err)
	}
	if runs[0].ID != 6 { // id 倒序
		t.Fatalf("order broken: first = %d", runs[0].ID)
	}
	// retention 内不足 N 条 success：无更早记录可删 → 空集。
	if v, err := func() ([]RotateVictim, error) {
		tx, err := s.BeginTx(ctx)
		if err != nil {
			return nil, err
		}
		v, rerr := tx.Run().Rotate(ctx, id, 10)
		if rerr != nil {
			_ = tx.Rollback()
			return nil, rerr
		}
		return v, tx.Commit()
	}(); err != nil || len(v) != 0 {
		t.Fatalf("over-retention rotate = %v %v, want empty", v, err)
	}
}

func TestSearchFiltersAndLatest(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	idA, _ := s.Job().Create(ctx, sampleJob("a"), now)
	idB, _ := s.Job().Create(ctx, sampleJob("b"), now)
	for _, tc := range []struct {
		job    int64
		trig   string
		status string
		offMin int
	}{
		{idA, TriggerManual, StatusSuccess, 0},
		{idA, TriggerCron, StatusFailure, 30},
		{idB, TriggerCron, StatusSuccess, 60},
	} {
		runID, err := s.Run().Create(ctx, tc.job, "n", tc.trig, now.Add(time.Duration(tc.offMin)*time.Minute))
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if tc.status == StatusSuccess {
			_, err = s.Run().FinishSuccess(ctx, runID, now.Add(time.Duration(tc.offMin)*time.Minute+time.Second), "/p", 1, nil)
		} else {
			_, err = s.Run().FinishFailure(ctx, runID, now.Add(time.Duration(tc.offMin)*time.Minute+time.Second), "e")
		}
		if err != nil {
			t.Fatalf("finish: %v", err)
		}
	}
	// jobId 过滤（已删任务仍有效——弱引用归组 D-12：先删 b）。
	if err := s.Job().Delete(ctx, idB); err != nil {
		t.Fatalf("delete b: %v", err)
	}
	_, total, err := s.Run().Search(ctx, RunFilter{JobID: &idB}, 0, 10)
	if err != nil || total != 1 {
		t.Fatalf("jobId filter after job deletion: total=%d err=%v, want 1", total, err)
	}
	// status/trigger 组合。
	_, total, _ = s.Run().Search(ctx, RunFilter{Status: StatusSuccess}, 0, 10)
	if total != 2 {
		t.Fatalf("status success total = %d, want 2", total)
	}
	_, total, _ = s.Run().Search(ctx, RunFilter{Trigger: TriggerCron, Status: StatusFailure}, 0, 10)
	if total != 1 {
		t.Fatalf("cron+failure total = %d, want 1", total)
	}
	// 时间窗（from/to）。
	from := now.Add(29 * time.Minute)
	_, total, _ = s.Run().Search(ctx, RunFilter{From: &from}, 0, 10)
	if total != 2 {
		t.Fatalf("from-window total = %d, want 2", total)
	}
	// 分页。
	runs, total, _ := s.Run().Search(ctx, RunFilter{}, 0, 2)
	if len(runs) != 2 || total != 3 {
		t.Fatalf("page1 = %d/%d, want 2/3", len(runs), total)
	}
	// LatestByJob（SQL #8 读时派生）。
	last, err := s.Run().LatestByJob(ctx, idA)
	if err != nil || last == nil || last.ID != 2 {
		t.Fatalf("latest of a = %+v err=%v, want id 2", last, err)
	}
	if none, err := s.Run().LatestByJob(ctx, 9999); err != nil || none != nil {
		t.Fatalf("latest of missing = %v %v, want nil", none, err)
	}
}

func TestMarkOrphanFailed(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	id, _ := s.Job().Create(ctx, sampleJob("orphan"), now)
	r1, _ := s.Run().Create(ctx, id, "orphan", TriggerCron, now)
	if r1 != 1 {
		t.Fatalf("runID = %d", r1)
	}
	n, err := s.Run().MarkOrphanFailed(ctx, now.Add(time.Hour), orphanMessageLiteral)
	if err != nil || n != 1 {
		t.Fatalf("mark orphan: n=%d err=%v", n, err)
	}
	run, _ := s.Run().GetByID(ctx, 1)
	if run.Status != StatusFailure || run.ErrorMessage == nil || *run.ErrorMessage != orphanMessageLiteral ||
		run.FinishedAt == nil {
		t.Fatalf("orphan state wrong: %+v", run)
	}
	// 孤儿清理后互斥解除：同 job 可再建 running（I-01 不再永久堵塞，D-08）。
	if _, err := s.Run().Create(ctx, id, "orphan", TriggerManual, now); err != nil {
		t.Fatalf("mutex must be released after recovery: %v", err)
	}
}
