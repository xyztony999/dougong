package dbmysql

// MySQL 表级治理网关扩展（api-database-mgmt.md v2.3 §4.1.5/§4.7，D-22）：
// 表清单/结构走 information_schema 实况投影（零台账 I-04）；行数据分页走
// CLI query() 通道 JSON_OBJECT 逐行聚合（先取列名再构造 SELECT
// JSON_OBJECT('col',`col`,...) LIMIT off,lim，batch 转义还原后逐行 JSON
// 解析——避免 TSV 对含换行/制表符值的解析脆弱）；行写仅限单列主键表
// （I-10 判定在 service 层，本层承载 WHERE 主键=旧值参数化执行）。
//
// 安全纪律沿既有口径：标识符（库/表/列名）service 白名单前置（R-14）+
// quoteIdent 反引号双防线（A-DB3）；行值/pkValue 自由文本 quoteString 全量
// 转义；LIMIT/OFFSET 为服务端整数形态（strconv 组装，非用户字符串拼接）；
// 全部语句经 g.query 字面量参数数组执行（R-08，密码 MYSQL_PWD env 注入）。
// 引擎侧 SQL 组装归本适配器（A-DB10 裁定：DbGateway 契约通道非面板 SQL）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/adapter/dbconn"
)

// ErrTableNotFound 目标表不存在（实况）→ DBM_022。
var ErrTableNotFound = errors.New("dbmysql: table not found")

// TableInfo 是表清单条目（§4.7.4 table/list items；information_schema.TABLES
// 实况投影——rowEstimate=TABLE_ROWS 统计估算，InnoDB 非精确值；视图/特殊
// 引擎可空如实）。
type TableInfo struct {
	Name          string
	Engine        string // 视图等 ENGINE NULL → ""（DTO 非空字符串如实）
	RowEstimate   *int64
	DataSizeBytes *int64
	Collation     *string
	Comment       *string // 无注释 → nil
}

// ColumnInfo 是表结构列条目（§4.7.4 table/structure columns；
// information_schema.COLUMNS 实况投影；Key=引擎 COLUMN_KEY 原值
// ""/PRI/UNI/MUL 开放字符串非枚举）。
type ColumnInfo struct {
	Column   string
	Type     string // COLUMN_TYPE 全量形态（如 int(11) unsigned）
	Nullable bool
	Key      string
	Default  *string // COLUMN_DEFAULT 无默认 → nil
	Extra    string  // EXTRA（auto_increment 等；无 → ""）
	Comment  *string // 无注释 → nil
}

// ListTables 表清单实况投影（information_schema.TABLES；含视图如实——
// TABLE_TYPE 不过滤，估算/可空字段语义由 DTO 承载）。
func (g *Gateway) ListTables(ctx context.Context, t dbconn.Target, c dbconn.Creds, db string) ([]TableInfo, error) {
	out, err := g.query(ctx, t, c,
		"SELECT TABLE_NAME, ENGINE, TABLE_ROWS, DATA_LENGTH, TABLE_COLLATION, TABLE_COMMENT "+
			"FROM information_schema.TABLES WHERE TABLE_SCHEMA = "+quoteString(db)+" ORDER BY TABLE_NAME")
	if err != nil {
		return nil, err
	}
	var tables []TableInfo
	for _, line := range splitTSV(out) {
		cols := strings.Split(line, "\t")
		if len(cols) < 6 {
			continue
		}
		ti := TableInfo{
			Name:        unescapeTSV(cols[0]),
			Engine:      nullableTSVString(cols[1], ""),
			RowEstimate: tsvInt64Ptr(cols[2]),
			Collation:   tsvStringPtr(cols[4]),
		}
		ti.DataSizeBytes = tsvInt64Ptr(cols[3])
		if cm := tsvStringPtr(cols[5]); cm != nil && *cm != "" {
			ti.Comment = cm // 无注释（空串）→ nil（DTO comment:string|null）
		}
		tables = append(tables, ti)
	}
	return tables, nil
}

// TableStructure 表结构实况投影（information_schema.COLUMNS 按 ORDINAL_POSITION
// 列序；空结果=表不存在 → ErrTableNotFound，DBM_022 前置判据）。
func (g *Gateway) TableStructure(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table string) ([]ColumnInfo, error) {
	out, err := g.query(ctx, t, c, structureStmt(db, table))
	if err != nil {
		return nil, err
	}
	var columns []ColumnInfo
	for _, line := range splitTSV(out) {
		cols := strings.Split(line, "\t")
		if len(cols) < 7 {
			continue
		}
		ci := ColumnInfo{
			Column:   unescapeTSV(cols[0]),
			Type:     unescapeTSV(cols[1]),
			Nullable: unescapeTSV(cols[2]) == "YES",
			Key:      unescapeTSV(cols[3]),
			Default:  tsvStringPtr(cols[4]),
			Extra:    nullableTSVString(cols[5], ""),
		}
		if cm := tsvStringPtr(cols[6]); cm != nil && *cm != "" {
			ci.Comment = cm
		}
		columns = append(columns, ci)
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("%w: %s.%s", ErrTableNotFound, db, table)
	}
	return columns, nil
}

// TableRows 行数据分页（JSON_OBJECT 逐行聚合通道，D-22）：先取列名（结构
// 查询）→ SELECT JSON_OBJECT(...) LIMIT offset,limit → batch 转义还原后
// 逐行 JSON 解析；total=COUNT(*) 实况。列值 JSON null=NULL 语义（≠TS
// undefined）；类型随列类型 JSON_OBJECT 原生映射。
func (g *Gateway) TableRows(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table string, offset, limit int) ([]map[string]any, int64, error) {
	columns, err := g.TableStructure(ctx, t, c, db, table)
	if err != nil {
		return nil, 0, err
	}
	names := make([]string, 0, len(columns))
	for _, col := range columns {
		names = append(names, col.Column)
	}
	totalOut, err := g.query(ctx, t, c, "SELECT COUNT(*) FROM "+quoteIdent(db)+"."+quoteIdent(table))
	if err != nil {
		return nil, 0, err
	}
	total, err := parseCount(totalOut)
	if err != nil {
		return nil, 0, err
	}
	out, err := g.query(ctx, t, c, jsonObjectStmt(db, table, names, offset, limit))
	if err != nil {
		return nil, 0, err
	}
	rows, err := parseJSONRows(out)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// DropTable 删表（DROP TABLE 不可逆；不存在 → ErrTableNotFound，不静默成功）。
func (g *Gateway) DropTable(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table string) error {
	_, err := g.query(ctx, t, c, "DROP TABLE "+quoteIdent(db)+"."+quoteIdent(table))
	if err != nil && errors.Is(err, ErrQueryFailed) &&
		(strings.Contains(err.Error(), "Unknown table") || strings.Contains(err.Error(), "doesn't exist")) {
		return fmt.Errorf("%w: %s.%s", ErrTableNotFound, db, table)
	}
	return err
}

// UpdateRow 单列主键行编辑（WHERE pkColumn=pkValue 定位，quoteString A-DB3；
// values 文本值经 MySQL 隐式类型转换，null→SET col=NULL；主键列不可编辑由
// service 层校验 I-10）。返回受影响行数（0 → service DBM_024）。
func (g *Gateway) UpdateRow(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table, pkColumn, pkValue string, values map[string]any) (int64, error) {
	if len(values) == 0 {
		return 0, fmt.Errorf("dbmysql: empty row update values")
	}
	stmt, err := updateRowStmt(db, table, pkColumn, pkValue, values)
	if err != nil {
		return 0, err
	}
	return g.execAffected(ctx, t, c, stmt)
}

// DeleteRow 单列主键行删除（WHERE pkColumn=pkValue 定位）。返回受影响行数
// （0 → service DBM_024）。
func (g *Gateway) DeleteRow(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table, pkColumn, pkValue string) (int64, error) {
	return g.execAffected(ctx, t, c, deleteRowStmt(db, table, pkColumn, pkValue))
}

// execAffected 执行写语句并取受影响行数（mysql --batch -N -e 写语句 stdout
// 为空——受影响行数经 SELECT ROW_COUNT() 同通道二次查询如实取得；两次查询
// 各自独立建连，ROW_COUNT() 会话级语义要求同连接——故合并为单条多语句
// 执行：stmt; SELECT ROW_COUNT()，mysql 客户端默认允许多语句）。
func (g *Gateway) execAffected(ctx context.Context, t dbconn.Target, c dbconn.Creds, stmt string) (int64, error) {
	out, err := g.query(ctx, t, c, stmt+"; SELECT ROW_COUNT()")
	if err != nil {
		return 0, err
	}
	return parseCount(out)
}

// ---- 纯函数（SQL 组装/解析——单测锚点，零 runner 实况依赖）----

// structureStmt 组装 information_schema.COLUMNS 结构查询（标识符白名单
// 前置后 quoteString 双防线）。
func structureStmt(db, table string) string {
	return "SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY, COLUMN_DEFAULT, EXTRA, COLUMN_COMMENT " +
		"FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = " + quoteString(db) +
		" AND TABLE_NAME = " + quoteString(table) + " ORDER BY ORDINAL_POSITION"
}

// jsonObjectStmt 组装 JSON_OBJECT 逐行聚合查询（D-22：键=列名字符串字面量
// quoteString，值=列引用 quoteIdent；键序=结构列序；LIMIT/OFFSET 服务端
// 整数形态）。
func jsonObjectStmt(db, table string, columns []string, offset, limit int) string {
	var b strings.Builder
	b.WriteString("SELECT JSON_OBJECT(")
	for i, col := range columns {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(quoteString(col))
		b.WriteString(", ")
		b.WriteString(quoteIdent(col))
	}
	b.WriteString(") FROM ")
	b.WriteString(quoteIdent(db))
	b.WriteString(".")
	b.WriteString(quoteIdent(table))
	b.WriteString(" LIMIT ")
	b.WriteString(strconv.Itoa(offset))
	b.WriteString(", ")
	b.WriteString(strconv.Itoa(limit))
	return b.String()
}

// updateRowStmt 组装行编辑语句（SET 列=quoteString 值〔null→NULL〕，
// WHERE 主键=quoteString 旧值；列序按键字典序稳定产出——map 遍历序不确定
// 消除，审计/日志可复现）。列名 ∈ 结构列集由 service 校验（AUTH_012），
// quoteIdent 双防线。
func updateRowStmt(db, table, pkColumn, pkValue string, values map[string]any) (string, error) {
	if len(values) == 0 {
		return "", fmt.Errorf("dbmysql: empty row update values")
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("UPDATE ")
	b.WriteString(quoteIdent(db))
	b.WriteString(".")
	b.WriteString(quoteIdent(table))
	b.WriteString(" SET ")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(quoteIdent(k))
		b.WriteString(" = ")
		b.WriteString(sqlValue(values[k]))
	}
	b.WriteString(" WHERE ")
	b.WriteString(quoteIdent(pkColumn))
	b.WriteString(" = ")
	b.WriteString(quoteString(pkValue))
	return b.String(), nil
}

// deleteRowStmt 组装行删除语句（WHERE 主键=旧值 quoteString 定位）。
func deleteRowStmt(db, table, pkColumn, pkValue string) string {
	return "DELETE FROM " + quoteIdent(db) + "." + quoteIdent(table) +
		" WHERE " + quoteIdent(pkColumn) + " = " + quoteString(pkValue)
}

// sqlValue 行值 → SQL 字面量（null→NULL 语义；其余文本值 quoteString 全量
// 转义——MySQL 隐式类型转换承载数值/布尔形态，D-22）。
func sqlValue(v any) string {
	if v == nil {
		return "NULL"
	}
	switch x := v.(type) {
	case string:
		return quoteString(x)
	default:
		return quoteString(fmt.Sprint(x))
	}
}

// parseJSONRows 解析 JSON_OBJECT 聚合输出（每行一个 JSON 对象文本；batch
// 转义还原后逐行 json.Unmarshal——含换行/制表符的值经转义还原不破坏行界）。
func parseJSONRows(out string) ([]map[string]any, error) {
	rows := make([]map[string]any, 0)
	for _, line := range splitTSV(out) {
		text := unescapeTSV(line)
		var row map[string]any
		if err := json.Unmarshal([]byte(text), &row); err != nil {
			return nil, fmt.Errorf("dbmysql: parse JSON_OBJECT row: %w", err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// parseCount 解析单值计数输出（COUNT(*)/ROW_COUNT()——首行首列整数）。
func parseCount(out string) (int64, error) {
	lines := splitTSV(out)
	if len(lines) == 0 {
		return 0, fmt.Errorf("dbmysql: empty count result")
	}
	cols := strings.Split(lines[0], "\t")
	n, err := strconv.ParseInt(strings.TrimSpace(cols[0]), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("dbmysql: parse count %q: %w", cols[0], err)
	}
	return n, nil
}

// tsvInt64Ptr TSV 单元 → 可空整数（NULL → nil；解析失败 → nil 如实降级）。
func tsvInt64Ptr(s string) *int64 {
	if s == "NULL" {
		return nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(unescapeTSV(s)), 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

// nullableTSVString TSV 单元 → 非空字符串（NULL → fallback）。
func nullableTSVString(s, fallback string) string {
	if s == "NULL" {
		return fallback
	}
	return unescapeTSV(s)
}
