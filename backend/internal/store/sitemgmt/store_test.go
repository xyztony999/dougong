package sitemgmt

import (
	"context"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/service/audit"
	"github.com/dougong/dougong/internal/store"
)

func openTestDB(t *testing.T) *Store {
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

func strp(s string) *string { return &s }

func TestCertRepoLifecycle(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second) // RFC3339 秒级存储往返精确
	nb, na := now.Add(-time.Hour), now.Add(90*24*time.Hour)

	id, err := s.Cert().Create(ctx, Certificate{
		PrimaryDomain: "a.example.com", Domains: []string{"a.example.com", "b.example.com"},
		Source: SourceAcme, Status: StatusPending, Challenge: strp(ChallengeHTTP01),
		StoragePath: strp("/data/certs/a-fullchain.pem"),
	})
	if err != nil || id == 0 {
		t.Fatalf("create: id=%d err=%v", id, err)
	}
	got, err := s.Cert().GetByID(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PrimaryDomain != "a.example.com" || len(got.Domains) != 2 ||
		got.Challenge == nil || *got.Challenge != ChallengeHTTP01 {
		t.Fatalf("row mismatch: %+v", got)
	}
	if got.NotBefore != nil || got.NotAfter != nil {
		t.Fatalf("pending should have nil validity, got %+v", got)
	}

	// 终态落库（SQL #4 全形）。
	if err := s.Cert().UpdateRenewalMeta(ctx, id, StatusValid, nil, &nb, &na, "R11"); err != nil {
		t.Fatalf("update meta: %v", err)
	}
	got, _ = s.Cert().GetByID(ctx, id)
	if got.Status != StatusValid || got.Issuer != "R11" || got.NotAfter == nil ||
		!got.NotAfter.Equal(na) {
		t.Fatalf("after meta update: %+v", got)
	}

	// deploy_prev_refs 读写（SQL #5）+ 成对值（A-SM14）。
	if err := s.Cert().SetDeployPrevRef(ctx, id, "/etc/nginx/sites-enabled/a.conf#0",
		DeployRef{Cert: "/old/cert.pem", Key: "/old/key.pem"}); err != nil {
		t.Fatalf("set ref: %v", err)
	}
	got, _ = s.Cert().GetByID(ctx, id)
	if ref, ok := got.DeployPrevRefs["/etc/nginx/sites-enabled/a.conf#0"]; !ok ||
		ref.Cert != "/old/cert.pem" || ref.Key != "/old/key.pem" {
		t.Fatalf("deploy refs: %+v", got.DeployPrevRefs)
	}
	if err := s.Cert().ClearDeployPrevRef(ctx, id, "/etc/nginx/sites-enabled/a.conf#0"); err != nil {
		t.Fatalf("clear ref: %v", err)
	}
	got, _ = s.Cert().GetByID(ctx, id)
	if len(got.DeployPrevRefs) != 0 {
		t.Fatalf("refs after clear: %+v", got.DeployPrevRefs)
	}
	// 幂等清除。
	if err := s.Cert().ClearDeployPrevRef(ctx, id, "missing#9"); err != nil {
		t.Fatalf("idempotent clear: %v", err)
	}

	// 失败态 + last_error。
	reason := "challenge timeout"
	if err := s.Cert().UpdateStatus(ctx, id, StatusRenewFailed, &reason); err != nil {
		t.Fatalf("update status: %v", err)
	}

	// List 排序：not_after 升序（NULL 排尾）。
	id2, err := s.Cert().Create(ctx, Certificate{
		PrimaryDomain: "c.example.com", Domains: []string{"c.example.com"},
		Source: SourceImported, Status: StatusValid, OriginPath: strp("/old/c.pem"),
	})
	if err != nil {
		t.Fatalf("create2: %v", err)
	}
	rows, err := s.Cert().List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d", len(rows))
	}
	if rows[0].ID != id { // 有 not_after 的行在前，NULL（imported 未刷新）排尾
		t.Fatalf("order: [%d %d] want id=%d first", rows[0].ID, rows[1].ID, id)
	}

	// ListActive 域（I-03 防重数据面）。
	if err := s.Cert().UpdateStatus(ctx, id, StatusValid, nil); err != nil {
		t.Fatal(err)
	}
	active, err := s.Cert().ListActive(ctx)
	if err != nil || len(active) != 2 {
		t.Fatalf("active=%d err=%v", len(active), err)
	}

	// SetAccountRef。
	if err := s.Cert().SetAccountRef(ctx, id, "https://acme.example/acct/1"); err != nil {
		t.Fatalf("account ref: %v", err)
	}
	got, _ = s.Cert().GetByID(ctx, id)
	if got.AcmeAccountRef == nil || *got.AcmeAccountRef != "https://acme.example/acct/1" {
		t.Fatalf("account ref: %+v", got.AcmeAccountRef)
	}

	// Delete。
	if err := s.Cert().Delete(ctx, id2); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Cert().GetByID(ctx, id2); err != ErrCertNotFound {
		t.Fatalf("after delete: %v", err)
	}
}

// 同事务：certificate 终态落库与审计原子（D-12 例外；失败全回滚）。
func TestTxSameTxCommitAndRollback(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	id, err := s.Cert().Create(ctx, Certificate{
		PrimaryDomain: "x.example.com", Domains: []string{"x.example.com"},
		Source: SourceAcme, Status: StatusPending, Challenge: strp(ChallengeTLSALPN),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	tx, err := s.BeginTx(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	na := time.Now().UTC().Truncate(time.Second).Add(60 * 24 * time.Hour)
	if err := tx.Cert().UpdateRenewalMeta(ctx, id, StatusValid, nil, nil, &na, "E5"); err != nil {
		t.Fatalf("tx meta: %v", err)
	}
	if err := tx.Audit().Append(ctx, audit.Entry{
		ActorName: "admin", Category: audit.CategoryChange, Action: "cert_requested",
		ObjectType: "certificate", Result: audit.ResultSuccess, Source: audit.SourceWeb,
	}, time.Now()); err != nil {
		t.Fatalf("tx audit: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	row, err := s.Cert().GetByID(ctx, id)
	if err != nil || row.Status != StatusValid || row.Issuer != "E5" {
		t.Fatalf("committed row: %+v err=%v", row, err)
	}

	// 回滚路径：更新不提交 → 状态不变。
	tx2, err := s.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx2.Cert().UpdateStatus(ctx, id, StatusRenewFailed, strp("x")); err != nil {
		t.Fatal(err)
	}
	if err := tx2.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	row, _ = s.Cert().GetByID(ctx, id)
	if row.Status != StatusValid {
		t.Fatalf("status after rollback = %s", row.Status)
	}
}
