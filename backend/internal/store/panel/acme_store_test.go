package panel

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/store"
)

// newTestDBACME 迁移后的 :memory: 库（007 增量后形态；复用 store_test 的
// newTestDB 命名空间另起）。
func newTestDBACME(t *testing.T) *sql.DB {
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
	return db
}

// TestMigration007Shape 迁移 007 增量形态：bind_domain 列就位、https_mode/source
// CHECK 扩展接受 acme（DDL-文档一致，api v2.0 §5.3）。
func TestMigration007Shape(t *testing.T) {
	db := newTestDBACME(t)
	if _, err := db.Exec(`UPDATE panel_config SET https_mode = 'acme' WHERE id = 1`); err != nil {
		t.Fatalf("https_mode=acme rejected (CHECK 未扩展): %v", err)
	}
	if _, err := db.Exec(`INSERT OR REPLACE INTO panel_cert
		(id, cert_pem, key_pem, source, updated_at) VALUES (1, 'PEM', 'PEM', 'acme', ?)`,
		time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("panel_cert source=acme rejected (CHECK 未扩展): %v", err)
	}
	var hasBind int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('panel_config')
		WHERE name = 'bind_domain'`).Scan(&hasBind); err != nil {
		t.Fatalf("table_info: %v", err)
	}
	if hasBind != 1 {
		t.Fatalf("bind_domain column missing")
	}
	var hasLastErr int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('panel_cert')
		WHERE name = 'last_error'`).Scan(&hasLastErr); err != nil {
		t.Fatalf("table_info: %v", err)
	}
	if hasLastErr != 1 {
		t.Fatalf("last_error column missing")
	}
}

// TestMigration007CarriesExistingRow 既有单行经表重建迁移保留（INSERT SELECT）。
func TestMigration007CarriesExistingRow(t *testing.T) {
	db := newTestDBACME(t)
	ctx := context.Background()
	now := time.Now().UTC()
	s := New(db)
	if err := s.Config().EnsureSingleton(ctx, 23456, now); err != nil {
		t.Fatalf("ensure singleton: %v", err)
	}
	domain := "panel.example.com"
	if err := s.Config().Update(ctx, ConfigChanges{BindDomain: &domain}, now); err != nil {
		t.Fatalf("update bind domain: %v", err)
	}
	cfg, err := s.Config().Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.BindDomain == nil || *cfg.BindDomain != domain {
		t.Fatalf("bind_domain roundtrip = %v", cfg.BindDomain)
	}
	// 解绑：空串 → NULL（D-P12 置空语义）。
	empty := ""
	if err := s.Config().Update(ctx, ConfigChanges{BindDomain: &empty}, now); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	cfg, err = s.Config().Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.BindDomain != nil {
		t.Fatalf("unbind left bind_domain = %v", *cfg.BindDomain)
	}
}

// TestCertLastErrorAndAcmeResult last_error 读写 + UpdateAcmeResult 成功/失败
// 两形态（SQL 清单 #6/#7）。
func TestCertLastErrorAndAcmeResult(t *testing.T) {
	db := newTestDBACME(t)
	ctx := context.Background()
	now := time.Now().UTC()
	s := New(db)
	if err := s.Cert().Upsert(ctx, Cert{
		CertPEM: "PEM", KeyPEM: "KEY", Source: CertSourceAcme,
		LastError: strp("旧失败原因"), // Upsert 恒清 last_error（acme 落库即 NULL，D-P13）
	}, now); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := s.Cert().Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LastError != nil {
		t.Fatalf("upsert must clear last_error, got %q", *got.LastError)
	}
	// 失败回写（#7）：仅 last_error。
	reason := "challenge unreachable"
	if err := s.Cert().UpdateAcmeResult(ctx, nil, nil, nil, &reason, now); err != nil {
		t.Fatalf("update acme failure: %v", err)
	}
	got, err = s.Cert().Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LastError == nil || *got.LastError != reason {
		t.Fatalf("last_error = %v", got.LastError)
	}
	// 成功回写（#6）：not_before/not_after/issuer 落定且 last_error=NULL。
	//（DATETIME 以 RFC3339 秒精度存储，比较值先取整秒。）
	sec := now.Truncate(time.Second)
	nb := sec.Add(-time.Hour)
	na := sec.Add(90 * 24 * time.Hour)
	issuer := "Let's Encrypt"
	if err := s.Cert().UpdateAcmeResult(ctx, &nb, &na, &issuer, nil, now); err != nil {
		t.Fatalf("update acme success: %v", err)
	}
	got, err = s.Cert().Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LastError != nil {
		t.Fatalf("success must clear last_error, got %q", *got.LastError)
	}
	if got.Issuer == nil || *got.Issuer != issuer || got.NotAfter == nil || !got.NotAfter.Equal(na) {
		t.Fatalf("success fields = %+v", got)
	}
}

func strp(v string) *string { return &v }
