package dbmysql

// 表级治理纯函数测试（v2.3 D-22；CI 环境耦合预防沿先例：SQL 组装/解析
// 纯逻辑本机覆盖，mysql 客户端实联归 Linux 目标环境冒烟）。

import (
	"strings"
	"testing"
)

// JSON_OBJECT 逐行聚合语句形状（键=列名字面量 quoteString、值=列引用
// quoteIdent、键序=结构列序、LIMIT 服务端整数形态）。
func TestJSONObjectStmt(t *testing.T) {
	got := jsonObjectStmt("appdb", "users", []string{"id", "name"}, 20, 20)
	want := "SELECT JSON_OBJECT('id', `id`, 'name', `name`) FROM `appdb`.`users` LIMIT 20, 20"
	if got != want {
		t.Fatalf("jsonObjectStmt =\n%s\nwant\n%s", got, want)
	}
	// 单列形态与零偏移。
	got = jsonObjectStmt("d", "t", []string{"a"}, 0, 100)
	if got != "SELECT JSON_OBJECT('a', `a`) FROM `d`.`t` LIMIT 0, 100" {
		t.Fatalf("single column stmt: %s", got)
	}
}

// 行编辑语句：SET 列序字典序稳定 + null→NULL + WHERE 主键 quoteString。
func TestUpdateRowStmt(t *testing.T) {
	got, err := updateRowStmt("appdb", "users", "id", "1", map[string]any{"name": "x", "note": nil})
	if err != nil {
		t.Fatal(err)
	}
	want := "UPDATE `appdb`.`users` SET `name` = 'x', `note` = NULL WHERE `id` = '1'"
	if got != want {
		t.Fatalf("updateRowStmt =\n%s\nwant\n%s", got, want)
	}
	if _, err := updateRowStmt("d", "t", "id", "1", map[string]any{}); err == nil {
		t.Fatal("empty values must fail")
	}
}

// 注入形态防线（A-DB3）：pkValue/行值含引号/反斜杠/控制字符全量转义，
// 不产生可逃逸字面量。
func TestRowStmtEscaping(t *testing.T) {
	got, err := updateRowStmt("d", "t", "id", "1' OR '1'='1", map[string]any{"name": "a'b\\c\nd"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `WHERE `+"`id`"+` = '1\' OR \'1\'=\'1'`) {
		t.Fatalf("pkValue must be fully escaped: %s", got)
	}
	if !strings.Contains(got, `'a\'b\\c\nd'`) {
		t.Fatalf("value must be fully escaped: %s", got)
	}
	del := deleteRowStmt("d", "t", "id", "1'; DROP TABLE x; --")
	if !strings.Contains(del, `'1\'; DROP TABLE x; --'`) {
		t.Fatalf("delete pkValue must be escaped: %s", del)
	}
	if strings.Count(del, "DELETE FROM") != 1 {
		t.Fatalf("statement must remain single DELETE: %s", del)
	}
}

// JSON_OBJECT 输出解析：batch 转义还原后逐行 JSON 解析——含换行/制表符
// 的值不破坏行界（D-22 通道核心动机）；JSON null=NULL 语义。
func TestParseJSONRows(t *testing.T) {
	// 引擎 JSON 文本（JSON 自身以 \n/\t 转义承载控制字符）。
	engineJSON := `{"id": 1, "name": "a\nb\tc"}`
	// mysql --batch 输出再转义一层（反斜杠双写）。
	batchOut := strings.ReplaceAll(engineJSON, `\`, `\\`) + "\n" +
		`{"id": 2, "name": null}` + "\n"
	rows, err := parseJSONRows(batchOut)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0]["name"] != "a\nb\tc" {
		t.Fatalf("control chars must roundtrip: %q", rows[0]["name"])
	}
	if rows[1]["name"] != nil {
		t.Fatalf("JSON null must map to nil: %+v", rows[1])
	}
	if _, err := parseJSONRows("{invalid\n"); err == nil {
		t.Fatal("invalid JSON row must fail")
	}
	if rows, err := parseJSONRows(""); err != nil || len(rows) != 0 {
		t.Fatalf("empty output → empty rows: %v %+v", err, rows)
	}
}

// 计数解析（COUNT(*)/ROW_COUNT()）。
func TestParseCount(t *testing.T) {
	if n, err := parseCount("42\n"); err != nil || n != 42 {
		t.Fatalf("count: %d %v", n, err)
	}
	if _, err := parseCount(""); err == nil {
		t.Fatal("empty count must fail")
	}
	if _, err := parseCount("abc\n"); err == nil {
		t.Fatal("non-numeric count must fail")
	}
}

// 结构查询语句形状（information_schema.COLUMNS + ORDINAL_POSITION 列序）。
func TestStructureStmt(t *testing.T) {
	got := structureStmt("appdb", "users")
	if !strings.Contains(got, "FROM information_schema.COLUMNS") ||
		!strings.Contains(got, "TABLE_SCHEMA = 'appdb'") ||
		!strings.Contains(got, "TABLE_NAME = 'users'") ||
		!strings.Contains(got, "ORDER BY ORDINAL_POSITION") {
		t.Fatalf("structureStmt: %s", got)
	}
}

// 行值字面量（null→NULL；文本 quoteString）。
func TestSQLValue(t *testing.T) {
	if sqlValue(nil) != "NULL" {
		t.Fatal("nil must be NULL")
	}
	if sqlValue("x") != "'x'" {
		t.Fatal("string must be quoted")
	}
}

// TSV 可空整数/字符串单元。
func TestTSVNullableHelpers(t *testing.T) {
	if tsvInt64Ptr("NULL") != nil {
		t.Fatal("NULL → nil")
	}
	if p := tsvInt64Ptr("123"); p == nil || *p != 123 {
		t.Fatal("123 → 123")
	}
	if tsvInt64Ptr("abc") != nil {
		t.Fatal("non-numeric → nil 如实降级")
	}
	if nullableTSVString("NULL", "") != "" {
		t.Fatal("NULL → fallback")
	}
	if nullableTSVString("InnoDB", "") != "InnoDB" {
		t.Fatal("value passthrough")
	}
}
