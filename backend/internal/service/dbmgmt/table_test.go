package dbmgmt

// MySQL 表级治理链测试（api v2.3 §4.7，D-22/I-10）：fakeMySQL 替身承载全部
// 链路（CI 环境耦合预防沿第 6/7 起先例：零 runner 实况引擎依赖——适配器
// 单测仅测纯函数 SQL 组装/解析，service 层全走替身）。

import (
	"context"
	"strings"
	"testing"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/adapter/dbmysql"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// ---- fakeMySQL v2.3 表级方法（记忆实况+行为注入）----

func (f *fakeMySQL) ListTables(ctx context.Context, t dbconn.Target, c dbconn.Creds, db string) ([]dbmysql.TableInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tableErr != nil {
		return nil, f.tableErr
	}
	return f.tables, nil
}

func (f *fakeMySQL) TableStructure(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table string) ([]dbmysql.ColumnInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tableErr != nil {
		return nil, f.tableErr
	}
	cols := f.structures[db+"."+table]
	if len(cols) == 0 {
		return nil, dbmysql.ErrTableNotFound
	}
	return cols, nil
}

func (f *fakeMySQL) TableRows(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table string, offset, limit int) ([]map[string]any, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tableErr != nil {
		return nil, 0, f.tableErr
	}
	all := f.rows[db+"."+table]
	total := int64(len(all))
	if offset >= len(all) {
		return []map[string]any{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

func (f *fakeMySQL) DropTable(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dropTableErr != nil {
		return f.dropTableErr
	}
	if _, ok := f.structures[db+"."+table]; !ok {
		return dbmysql.ErrTableNotFound
	}
	delete(f.structures, db+"."+table)
	delete(f.rows, db+"."+table)
	return nil
}

func (f *fakeMySQL) UpdateRow(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table, pkColumn, pkValue string, values map[string]any) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateRowErr != nil {
		return 0, f.updateRowErr
	}
	f.updatedRows = append(f.updatedRows, db+"."+table+":"+pkValue)
	f.lastValues = values
	return f.affected, nil
}

func (f *fakeMySQL) DeleteRow(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table, pkColumn, pkValue string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteRowErr != nil {
		return 0, f.deleteRowErr
	}
	f.deletedRows = append(f.deletedRows, db+"."+table+":"+pkValue)
	return f.affected, nil
}

// ---- 测试基座扩展 ----

// seedTable 种一张单列主键表（id PRI + name 普通列）与两行数据。
func (e *testEnv) seedTable(db, table string) {
	e.mysql.structures[db+"."+table] = []dbmysql.ColumnInfo{
		{Column: "id", Type: "int(11)", Nullable: false, Key: "PRI", Extra: "auto_increment"},
		{Column: "name", Type: "varchar(64)", Nullable: true, Key: "", Extra: ""},
	}
	e.mysql.rows[db+"."+table] = []map[string]any{
		{"id": float64(1), "name": "alpha"},
		{"id": float64(2), "name": nil},
	}
}

func newTableEnv(t *testing.T) (*testEnv, *storedb.Instance) {
	t.Helper()
	e := newTestEnv(t)
	e.mysql.dbs = []dbmysql.DatabaseInfo{{Name: "appdb", Charset: "utf8mb4"}}
	e.mysql.structures = map[string][]dbmysql.ColumnInfo{}
	e.mysql.rows = map[string][]map[string]any{}
	inst := e.seedInstance(t, storedb.EngineMySQL)
	return e, inst
}

// ---- 用例 ----

func TestTableListAndStructure(t *testing.T) {
	e, inst := newTableEnv(t)
	ctx := context.Background()
	e.seedTable("appdb", "users")
	estimate := int64(2)
	size := int64(16384)
	collation := "utf8mb4_general_ci"
	e.mysql.tables = []dbmysql.TableInfo{
		{Name: "users", Engine: "InnoDB", RowEstimate: &estimate, DataSizeBytes: &size, Collation: &collation, Comment: nil},
	}

	data, err := e.svc.TableList(ctx, itoa(inst.ID), "appdb")
	if err != nil || len(data.Items) != 1 || data.Items[0].Name != "users" {
		t.Fatalf("table list: %+v %v", data, err)
	}
	if data.Items[0].RowEstimate == nil || *data.Items[0].RowEstimate != 2 || data.Items[0].Comment != nil {
		t.Fatalf("nullable projection wrong: %+v", data.Items[0])
	}
	// 库不存在 DBM_007 / 表名非法 AUTH_012。
	if _, err := e.svc.TableList(ctx, itoa(inst.ID), "nodb"); errCode(t, err) != "DBM_007" {
		t.Fatalf("want DBM_007, got %v", err)
	}
	if _, err := e.svc.TableStructure(ctx, itoa(inst.ID), "appdb", "bad-table"); errCode(t, err) != "AUTH_012" {
		t.Fatalf("want AUTH_012, got %v", err)
	}

	st, err := e.svc.TableStructure(ctx, itoa(inst.ID), "appdb", "users")
	if err != nil || !st.SinglePk || st.PkColumn == nil || *st.PkColumn != "id" || len(st.Columns) != 2 {
		t.Fatalf("structure: %+v %v", st, err)
	}
	// 表不存在 DBM_022。
	if _, err := e.svc.TableStructure(ctx, itoa(inst.ID), "appdb", "gone"); errCode(t, err) != "DBM_022" {
		t.Fatalf("want DBM_022, got %v", err)
	}
	// 引擎不匹配 DBM_018（redis 档案走 table 接口）。
	rinst := e.seedInstance(t, storedb.EngineRedis)
	if _, err := e.svc.TableList(ctx, itoa(rinst.ID), "appdb"); errCode(t, err) != "DBM_018" {
		t.Fatalf("want DBM_018, got %v", err)
	}
}

func TestTableRowsPagination(t *testing.T) {
	e, inst := newTableEnv(t)
	ctx := context.Background()
	e.seedTable("appdb", "users")

	data, err := e.svc.TableRows(ctx, itoa(inst.ID), "appdb", "users", "", "")
	if err != nil || data.Total != 2 || data.Page != 1 || data.PageSize != TablePageDefault || len(data.Rows) != 2 {
		t.Fatalf("rows default page: %+v %v", data, err)
	}
	if len(data.Columns) != 2 || data.Columns[0] != "id" || data.Columns[1] != "name" {
		t.Fatalf("columns must follow structure order: %+v", data.Columns)
	}
	// NULL 语义如实（JSON null≠undefined）。
	if data.Rows[1]["name"] != nil {
		t.Fatalf("NULL cell must be nil: %+v", data.Rows[1])
	}
	// 分页 offset 与上限截断（pageSize>100 → 100）。
	data, err = e.svc.TableRows(ctx, itoa(inst.ID), "appdb", "users", "2", "1")
	if err != nil || len(data.Rows) != 1 || data.Rows[0]["id"] != float64(2) || data.Page != 2 {
		t.Fatalf("rows page 2: %+v %v", data, err)
	}
	data, err = e.svc.TableRows(ctx, itoa(inst.ID), "appdb", "users", "1", "999")
	if err != nil || data.PageSize != TablePageLimit {
		t.Fatalf("pageSize clamp: %+v %v", data, err)
	}
	// 形态非法 AUTH_012。
	for _, c := range [][2]string{{"0", ""}, {"-1", ""}, {"abc", ""}, {"", "0"}, {"", "x"}} {
		if _, err := e.svc.TableRows(ctx, itoa(inst.ID), "appdb", "users", c[0], c[1]); errCode(t, err) != "AUTH_012" {
			t.Fatalf("page=%q pageSize=%q want AUTH_012, got %v", c[0], c[1], err)
		}
	}
	// 表不存在 DBM_022。
	if _, err := e.svc.TableRows(ctx, itoa(inst.ID), "appdb", "gone", "", ""); errCode(t, err) != "DBM_022" {
		t.Fatalf("want DBM_022, got %v", err)
	}
}

func TestRowWriteSinglePKGuard(t *testing.T) {
	e, inst := newTableEnv(t)
	ctx := context.Background()
	actor := testActor()
	e.seedTable("appdb", "users")
	e.mysql.affected = 1

	// 成功：单列主键行编辑（null→NULL 语义透传网关）。
	data, err := e.svc.UpdateRow(ctx, actor, RowUpdateRequest{
		InstanceID: inst.ID, Database: "appdb", Table: "users", PKValue: "1",
		Values: map[string]any{"name": nil}})
	if err != nil || data.Affected != 1 || data.PKValue != "1" {
		t.Fatalf("row update: %+v %v", data, err)
	}
	if v, ok := e.mysql.lastValues["name"]; !ok || v != nil {
		t.Fatalf("null value must reach gateway as nil: %+v", e.mysql.lastValues)
	}
	if !e.hasAudit(ctx, "row_updated") {
		t.Fatal("row_updated audit missing")
	}
	// 成功：行删除。
	del, err := e.svc.DeleteRow(ctx, actor, RowDeleteRequest{
		InstanceID: inst.ID, Database: "appdb", Table: "users", PKValue: "2"})
	if err != nil || del.Affected != 1 {
		t.Fatalf("row delete: %+v %v", del, err)
	}
	if !e.hasAudit(ctx, "row_deleted") {
		t.Fatal("row_deleted audit missing")
	}

	// affected=0 → DBM_024（不静默成功）。
	e.mysql.affected = 0
	if _, err := e.svc.UpdateRow(ctx, actor, RowUpdateRequest{
		InstanceID: inst.ID, Database: "appdb", Table: "users", PKValue: "9",
		Values: map[string]any{"name": "x"}}); errCode(t, err) != "DBM_024" {
		t.Fatalf("want DBM_024, got %v", err)
	}
	if _, err := e.svc.DeleteRow(ctx, actor, RowDeleteRequest{
		InstanceID: inst.ID, Database: "appdb", Table: "users", PKValue: "9"}); errCode(t, err) != "DBM_024" {
		t.Fatalf("want DBM_024, got %v", err)
	}
	e.mysql.affected = 1

	// 无主键表 → DBM_023 reason 如实。
	e.mysql.structures["appdb.nopk"] = []dbmysql.ColumnInfo{{Column: "a", Key: ""}, {Column: "b", Key: ""}}
	_, err = e.svc.UpdateRow(ctx, actor, RowUpdateRequest{
		InstanceID: inst.ID, Database: "appdb", Table: "nopk", PKValue: "1",
		Values: map[string]any{"a": "x"}})
	if errCode(t, err) != "DBM_023" {
		t.Fatalf("want DBM_023, got %v", err)
	}
	// 复合主键 → DBM_023。
	e.mysql.structures["appdb.composite"] = []dbmysql.ColumnInfo{
		{Column: "a", Key: "PRI"}, {Column: "b", Key: "PRI"}}
	if _, err := e.svc.DeleteRow(ctx, actor, RowDeleteRequest{
		InstanceID: inst.ID, Database: "appdb", Table: "composite", PKValue: "1"}); errCode(t, err) != "DBM_023" {
		t.Fatalf("composite pk want DBM_023, got %v", err)
	}

	// values 校验：空/含主键列/列名越集/非文本值 → AUTH_012。
	badValues := []map[string]any{
		{},
		{"id": "5"},
		{"ghost": "x"},
		{"name": float64(3)},
	}
	for _, vs := range badValues {
		if _, err := e.svc.UpdateRow(ctx, actor, RowUpdateRequest{
			InstanceID: inst.ID, Database: "appdb", Table: "users", PKValue: "1", Values: vs}); errCode(t, err) != "AUTH_012" {
			t.Fatalf("values %+v want AUTH_012, got %v", vs, err)
		}
	}
	// pkValue 空 → AUTH_012。
	if _, err := e.svc.UpdateRow(ctx, actor, RowUpdateRequest{
		InstanceID: inst.ID, Database: "appdb", Table: "users", PKValue: "",
		Values: map[string]any{"name": "x"}}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("empty pkValue want AUTH_012, got %v", err)
	}
}

func TestDropTableFlow(t *testing.T) {
	e, inst := newTableEnv(t)
	ctx := context.Background()
	e.seedTable("appdb", "users")

	data, err := e.svc.DropTable(ctx, testActor(), TableDropRequest{
		InstanceID: inst.ID, Database: "appdb", Table: "users"})
	if err != nil || data.Table != "users" {
		t.Fatalf("drop table: %+v %v", data, err)
	}
	if !e.hasAudit(ctx, "table_dropped") {
		t.Fatal("table_dropped audit missing")
	}
	// 再删 → DBM_022（不静默成功）。
	if _, err := e.svc.DropTable(ctx, testActor(), TableDropRequest{
		InstanceID: inst.ID, Database: "appdb", Table: "users"}); errCode(t, err) != "DBM_022" {
		t.Fatalf("want DBM_022, got %v", err)
	}
}

// 审计脱敏断言：row_updated detail 含列名不含行值明文（D-13 v2.3 口径）。
func TestRowAuditDetailRedaction(t *testing.T) {
	e, inst := newTableEnv(t)
	ctx := context.Background()
	e.seedTable("appdb", "users")
	e.mysql.affected = 1
	_, err := e.svc.UpdateRow(ctx, testActor(), RowUpdateRequest{
		InstanceID: inst.ID, Database: "appdb", Table: "users", PKValue: "1",
		Values: map[string]any{"name": "secret-cell-value"}})
	if err != nil {
		t.Fatal(err)
	}
	rows, _, err := e.auditQ.Search(ctx, storeauth.AuditCondition{Action: "row_updated"}, 0, 10)
	if err != nil || len(rows) == 0 || rows[0].Detail == nil {
		t.Fatalf("audit search: %v %d", err, len(rows))
	}
	detail := *rows[0].Detail
	if !strings.Contains(detail, `"columns":["name"]`) || !strings.Contains(detail, `"pkValue":"1"`) {
		t.Fatalf("detail must carry locator+columns: %s", detail)
	}
	if strings.Contains(detail, "secret-cell-value") {
		t.Fatalf("row value plaintext leaked into audit detail: %s", detail)
	}
}
