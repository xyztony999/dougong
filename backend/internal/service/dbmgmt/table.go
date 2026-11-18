package dbmgmt

// MySQL 表级治理编排（api-database-mgmt.md v2.3 §4.7，D-22；TableGovernanceService
// 领域服务落点）：表清单/结构/行数据分页/删表/单列主键行编辑/行删除。
// 零台账实况投影（I-04 扩展）——恒经 MySQLGateway 实时读取；前置链 I-05
// （engine∈mysql/mariadb）+ 库存在 DBM_007 + 表存在 DBM_022 + 单列主键
// I-10（无则 DBM_023）；行写 WHERE 主键=旧值参数化（quoteString A-DB3 网关
// 承载）；匹配 0 行 DBM_024 如实不静默；审计同事务脱敏（detail 仅对象定位
// 与列名，不含行值明文，A-DB16）。数据内容 GET（row/list）admin 专属
// （D-24/I-11 路由层承载）。

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/adapter/dbmysql"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// TableList GET /db/table/list：表清单实况投影（元数据两角色，D-24）。
func (s *Service) TableList(ctx context.Context, instanceID, database string) (*TableListData, error) {
	_, t, c, err := s.governTarget(ctx, instanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	if err := validateDatabaseName(database); err != nil {
		return nil, err
	}
	if err := s.mysqlDatabaseExists(ctx, t, c, database); err != nil {
		return nil, err
	}
	tables, err := s.mysql.ListTables(ctx, t, c, database)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	items := make([]TableItem, 0, len(tables))
	for _, tb := range tables {
		items = append(items, TableItem{
			Name: tb.Name, Engine: tb.Engine, RowEstimate: tb.RowEstimate,
			DataSizeBytes: tb.DataSizeBytes, Collation: tb.Collation, Comment: tb.Comment,
		})
	}
	return &TableListData{Items: items, GeneratedAt: s.clock().UTC()}, nil
}

// TableStructure GET /db/table/structure：表结构实况投影 + 单列主键派生
// （元数据两角色；singlePk/pkColumn 为前端行写入口禁用判定同源，I-10）。
func (s *Service) TableStructure(ctx context.Context, instanceID, database, table string) (*TableStructureData, error) {
	_, t, c, err := s.governTarget(ctx, instanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	cols, err := s.tableColumns(ctx, t, c, database, table)
	if err != nil {
		return nil, err
	}
	return structureData(table, cols, s.clock().UTC()), nil
}

// TableRows GET /db/row/list：行数据分页（JSON_OBJECT 逐行聚合通道 D-22；
// admin 专属——行数据属数据内容取出，I-11/D-24 路由层承载）。
func (s *Service) TableRows(ctx context.Context, instanceID, database, table, pageRaw, pageSizeRaw string) (*RowListData, error) {
	_, t, c, err := s.governTarget(ctx, instanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	page, pageSize, err := parsePagination(pageRaw, pageSizeRaw, TablePageDefault, TablePageLimit)
	if err != nil {
		return nil, err
	}
	cols, err := s.tableColumns(ctx, t, c, database, table)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(cols))
	for _, col := range cols {
		names = append(names, col.Column)
	}
	rows, total, err := s.mysql.TableRows(ctx, t, c, database, table, (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	return &RowListData{Columns: names, Rows: rows, Total: total,
		Page: page, PageSize: pageSize, GeneratedAt: s.clock().UTC()}, nil
}

// DropTable POST /db/table/drop：删表（DROP TABLE 不可逆；表存在 DBM_022
// 前置不静默成功；审计 table_dropped 同事务脱敏）。
func (s *Service) DropTable(ctx context.Context, actor *authsvc.Identity, req TableDropRequest) (*TableDropData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	if _, err := s.tableColumns(ctx, t, c, req.Database, req.Table); err != nil {
		return nil, err
	}
	if err := s.mysql.DropTable(ctx, t, c, req.Database, req.Table); err != nil {
		return nil, translateGatewayErr(err)
	}
	if err := s.txAuditOnly(ctx, auditEntry(actor, ActionTableDropped, ObjectTypeDataTable,
		tableObjectID(inst, req.Database, req.Table),
		map[string]any{"instanceId": inst.ID, "database": req.Database, "table": req.Table}, true)); err != nil {
		return nil, err
	}
	return &TableDropData{InstanceID: inst.ID, Database: req.Database, Table: req.Table}, nil
}

// UpdateRow POST /db/row/update：单列主键行编辑（I-10：无单列主键 DBM_023；
// values 非空且不含主键列、列名 ∈ 结构列集、文本值 string|null；affected=0
// → DBM_024；审计 row_updated 同事务——detail 含列名不含行值明文）。
func (s *Service) UpdateRow(ctx context.Context, actor *authsvc.Identity, req RowUpdateRequest) (*RowUpdateData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	pkColumn, cols, err := s.rowWriteGuard(ctx, t, c, req.Database, req.Table, req.PKValue)
	if err != nil {
		return nil, err
	}
	if len(req.Values) == 0 {
		return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "values", Message: "values 必填（至少一列）"})
	}
	columnSet := make(map[string]bool, len(cols))
	for _, col := range cols {
		columnSet[col.Column] = true
	}
	changed := make([]string, 0, len(req.Values))
	for name, v := range req.Values {
		if name == pkColumn {
			return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
				Field: "values", Message: "主键列为定位键不可编辑（I-10）"})
		}
		if !columnSet[name] {
			return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
				Field: "values", Message: fmt.Sprintf("列 %q 不在表结构列集内", name)})
		}
		if v != nil {
			if _, ok := v.(string); !ok {
				return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
					Field: "values", Message: "列值须为文本或 null（MySQL 隐式类型转换，D-22）"})
			}
		}
		changed = append(changed, name)
	}
	sort.Strings(changed)
	affected, err := s.mysql.UpdateRow(ctx, t, c, req.Database, req.Table, pkColumn, req.PKValue, req.Values)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	if affected == 0 {
		return nil, errcode.ErrDBMRowNotFound
	}
	if err := s.txAuditOnly(ctx, auditEntry(actor, ActionRowUpdated, ObjectTypeTableRow,
		rowObjectID(inst, req.Database, req.Table, req.PKValue),
		map[string]any{"instanceId": inst.ID, "database": req.Database, "table": req.Table,
			"pkValue": req.PKValue, "columns": changed}, true)); err != nil {
		return nil, err
	}
	return &RowUpdateData{InstanceID: inst.ID, Database: req.Database, Table: req.Table,
		PKValue: req.PKValue, Affected: affected}, nil
}

// DeleteRow POST /db/row/delete：单列主键行删除（I-10 同链；affected=0 →
// DBM_024；审计 row_deleted 同事务脱敏）。
func (s *Service) DeleteRow(ctx context.Context, actor *authsvc.Identity, req RowDeleteRequest) (*RowDeleteData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMySQL)
	if err != nil {
		return nil, err
	}
	pkColumn, _, err := s.rowWriteGuard(ctx, t, c, req.Database, req.Table, req.PKValue)
	if err != nil {
		return nil, err
	}
	affected, err := s.mysql.DeleteRow(ctx, t, c, req.Database, req.Table, pkColumn, req.PKValue)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	if affected == 0 {
		return nil, errcode.ErrDBMRowNotFound
	}
	if err := s.txAuditOnly(ctx, auditEntry(actor, ActionRowDeleted, ObjectTypeTableRow,
		rowObjectID(inst, req.Database, req.Table, req.PKValue),
		map[string]any{"instanceId": inst.ID, "database": req.Database, "table": req.Table,
			"pkValue": req.PKValue}, true)); err != nil {
		return nil, err
	}
	return &RowDeleteData{InstanceID: inst.ID, Database: req.Database, Table: req.Table,
		PKValue: req.PKValue, Affected: affected}, nil
}

// ---- 共用前置与派生 ----

// mysqlDatabaseExists 库实况存在前置（DBM_007）。
func (s *Service) mysqlDatabaseExists(ctx context.Context, t dbconn.Target, c dbconn.Creds, database string) error {
	exists, err := s.mysql.DatabaseExists(ctx, t, c, database)
	if err != nil {
		return translateGatewayErr(err)
	}
	if !exists {
		return errcode.ErrDBMDatabaseNotFound
	}
	return nil
}

// tableColumns 库/表形态校验 + 库存在 + 表结构实况（表不存在 → DBM_022）。
func (s *Service) tableColumns(ctx context.Context, t dbconn.Target, c dbconn.Creds, database, table string) ([]dbmysql.ColumnInfo, error) {
	if err := validateDatabaseName(database); err != nil {
		return nil, err
	}
	if err := validateTableName(table); err != nil {
		return nil, err
	}
	if err := s.mysqlDatabaseExists(ctx, t, c, database); err != nil {
		return nil, err
	}
	cols, err := s.mysql.TableStructure(ctx, t, c, database, table)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	return cols, nil
}

// rowWriteGuard 行写统一前置（I-10）：库/表存在 + 单列主键判定（无则
// DBM_023 data.reason 如实——无主键/复合主键）+ pkValue 非空形态。
func (s *Service) rowWriteGuard(ctx context.Context, t dbconn.Target, c dbconn.Creds,
	database, table, pkValue string) (string, []dbmysql.ColumnInfo, error) {
	cols, err := s.tableColumns(ctx, t, c, database, table)
	if err != nil {
		return "", nil, err
	}
	pkColumn, ok := singlePKColumn(cols)
	if !ok {
		pkCount := 0
		for _, col := range cols {
			if col.Key == "PRI" {
				pkCount++
			}
		}
		reason := "该表无主键，无法稳定定位行"
		if pkCount > 1 {
			reason = fmt.Sprintf("该表为复合主键（%d 列），不支持行级编辑/删除", pkCount)
		}
		return "", nil, errcode.ErrDBMNoSinglePK.WithData(map[string]string{"reason": reason})
	}
	if pkValue == "" || len(pkValue) > maxPKValueLen {
		return "", nil, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "pkValue", Message: fmt.Sprintf("pkValue 须为 1~%d 字符", maxPKValueLen)})
	}
	return pkColumn, cols, nil
}

// singlePKColumn 单列主键派生（恰好 1 列 COLUMN_KEY=PRI → 列名+true；
// 无主键/复合主键 → ""+false，I-10/D-22）。
func singlePKColumn(cols []dbmysql.ColumnInfo) (string, bool) {
	pk := ""
	n := 0
	for _, col := range cols {
		if col.Key == "PRI" {
			n++
			pk = col.Column
		}
	}
	if n == 1 {
		return pk, true
	}
	return "", false
}

// structureData 结构投影 + singlePk/pkColumn 派生（generatedAt 由调用方时钟）。
func structureData(table string, cols []dbmysql.ColumnInfo, at time.Time) *TableStructureData {
	items := make([]ColumnItem, 0, len(cols))
	for _, col := range cols {
		items = append(items, ColumnItem{
			Column: col.Column, Type: col.Type, Nullable: col.Nullable, Key: col.Key,
			Default: col.Default, Extra: col.Extra, Comment: col.Comment,
		})
	}
	data := &TableStructureData{Table: table, Columns: items, GeneratedAt: at}
	if pk, ok := singlePKColumn(cols); ok {
		data.SinglePk = true
		data.PkColumn = &pk
	}
	return data
}

// ---- 审计对象标识（v2.3）----

func tableObjectID(inst *storedb.Instance, database, table string) string {
	return dbObjectID(inst, database) + ":" + table
}

func rowObjectID(inst *storedb.Instance, database, table, pkValue string) string {
	return tableObjectID(inst, database, table) + ":" + pkValue
}
