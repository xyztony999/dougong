package dbmgmt

// MongoDB 集合与文档治理编排（api-database-mgmt.md v2.3 §4.8，D-23/D-25；
// MongoGovernanceService 领域服务落点）：集合清单/建删集合/删库/文档分页
// 与 CRUD。零台账实况投影（I-04 扩展）——恒经 MongoGateway 实时读取；
// 前置链 I-05（engine=mongodb）+ 库存在 DBM_007 + 集合存在 DBM_025 +
// JSON 服务端校验 DBM_026（R-14 双层校验服务端兜底）+ 系统库删除保护
// DBM_028（I-12 硬校验）；_id 不可变更（D-23）；匹配 0 DBM_027 如实不静默；
// 审计同事务脱敏（detail 仅对象定位，不含文档内容明文，A-DB16）。
// 数据内容 GET（mdoc/list）admin 专属（D-24/I-11 路由层承载）。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// MongoListCollections GET /db/mcoll/list：集合清单实况投影（元数据两角色；
// 空库返回空列表非错误）。
func (s *Service) MongoListCollections(ctx context.Context, instanceID, database string) (*MongoCollectionListData, error) {
	_, t, c, err := s.governTarget(ctx, instanceID, familyMongo)
	if err != nil {
		return nil, err
	}
	if err := validateDatabaseName(database); err != nil {
		return nil, err
	}
	if err := s.mongoDatabaseExists(ctx, t, c, database); err != nil {
		return nil, err
	}
	colls, err := s.mongo.ListCollections(ctx, t, c, database)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	items := make([]MongoCollectionItem, 0, len(colls))
	for _, coll := range colls {
		items = append(items, MongoCollectionItem{Name: coll.Name, DocCount: coll.DocCount, SizeBytes: coll.SizeBytes})
	}
	return &MongoCollectionListData{Items: items, GeneratedAt: s.clock().UTC()}, nil
}

// MongoCreateCollection POST /db/mcoll/create：建集合（createCollection 显式；
// 隐式建库语义——库随首集合写入产生，无独立建库接口 D-25，故不做库存在
// 前置；同名实况 DBM_008 语义复用；审计 mongo_collection_created 同事务）。
func (s *Service) MongoCreateCollection(ctx context.Context, actor *authsvc.Identity, req MongoCollectionCreateRequest) (*MongoCollectionCreateData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMongo)
	if err != nil {
		return nil, err
	}
	if err := validateDatabaseName(req.Database); err != nil {
		return nil, err
	}
	if err := validateCollectionName(req.Name); err != nil {
		return nil, err
	}
	if err := s.mongo.CreateCollection(ctx, t, c, req.Database, req.Name); err != nil {
		return nil, translateGatewayErr(err)
	}
	if err := s.txAuditOnly(ctx, auditEntry(actor, ActionMongoCollectionCreated, ObjectTypeMongoCollection,
		collObjectID(inst, req.Database, req.Name),
		map[string]any{"instanceId": inst.ID, "database": req.Database, "name": req.Name}, true)); err != nil {
		return nil, err
	}
	return &MongoCollectionCreateData{InstanceID: inst.ID, Database: req.Database, Name: req.Name}, nil
}

// MongoDropCollection POST /db/mcoll/drop：删集合（不可逆；集合存在 DBM_025
// 前置不静默成功；审计 mongo_collection_dropped 同事务）。
func (s *Service) MongoDropCollection(ctx context.Context, actor *authsvc.Identity, req MongoCollectionDropRequest) (*MongoCollectionDropData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMongo)
	if err != nil {
		return nil, err
	}
	if err := s.mongoCollTarget(ctx, t, c, req.Database, req.Name); err != nil {
		return nil, err
	}
	if err := s.mongo.DropCollection(ctx, t, c, req.Database, req.Name); err != nil {
		return nil, translateGatewayErr(err)
	}
	if err := s.txAuditOnly(ctx, auditEntry(actor, ActionMongoCollectionDropped, ObjectTypeMongoCollection,
		collObjectID(inst, req.Database, req.Name),
		map[string]any{"instanceId": inst.ID, "database": req.Database, "name": req.Name}, true)); err != nil {
		return nil, err
	}
	return &MongoCollectionDropData{InstanceID: inst.ID, Database: req.Database, Name: req.Name}, nil
}

// MongoDropDatabase POST /db/mdb/dropdb：删库（dropDatabase 不可逆；系统库
// admin/config/local 一律 DBM_028 拒绝——I-12 服务端硬校验先于库存在判定，
// 防自毁纳管凭据语境；审计 mongo_database_dropped 同事务）。
func (s *Service) MongoDropDatabase(ctx context.Context, actor *authsvc.Identity, req MongoDatabaseDropRequest) (*MongoDatabaseDropData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMongo)
	if err != nil {
		return nil, err
	}
	if err := validateDatabaseName(req.Database); err != nil {
		return nil, err
	}
	if MongoSystemDatabases[req.Database] {
		return nil, errcode.ErrDBMSystemDatabaseProtected
	}
	if err := s.mongoDatabaseExists(ctx, t, c, req.Database); err != nil {
		return nil, err
	}
	if err := s.mongo.DropDatabase(ctx, t, c, req.Database); err != nil {
		return nil, translateGatewayErr(err)
	}
	if err := s.txAuditOnly(ctx, auditEntry(actor, ActionMongoDatabaseDropped, ObjectTypeMongoDatabase,
		dbObjectID(inst, req.Database),
		map[string]any{"instanceId": inst.ID, "database": req.Database}, true)); err != nil {
		return nil, err
	}
	return &MongoDatabaseDropData{InstanceID: inst.ID, Database: req.Database}, nil
}

// MongoListDocuments GET /db/mdoc/list：文档分页（find skip/limit；admin
// 专属——文档属数据内容取出，I-11/D-24 路由层承载；扩展 JSON 形态透传）。
func (s *Service) MongoListDocuments(ctx context.Context, instanceID, database, collection, pageRaw, pageSizeRaw string) (*MongoDocumentListData, error) {
	_, t, c, err := s.governTarget(ctx, instanceID, familyMongo)
	if err != nil {
		return nil, err
	}
	page, pageSize, err := parsePagination(pageRaw, pageSizeRaw, DocPageDefault, DocPageLimit)
	if err != nil {
		return nil, err
	}
	if err := s.mongoCollTarget(ctx, t, c, database, collection); err != nil {
		return nil, err
	}
	docs, total, err := s.mongo.ListDocuments(ctx, t, c, database, collection, (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	if docs == nil {
		docs = []map[string]any{}
	}
	return &MongoDocumentListData{Documents: docs, Total: total,
		Page: page, PageSize: pageSize, GeneratedAt: s.clock().UTC()}, nil
}

// MongoInsertDocument POST /db/mdoc/insert：文档插入（InsertOne；JSON 服务端
// 校验 DBM_026；_id 可选缺省引擎自动生成；_id 冲突 DBM_008 语义复用；审计
// mongo_document_inserted 同事务——detail 含 insertedId 不含文档内容）。
func (s *Service) MongoInsertDocument(ctx context.Context, actor *authsvc.Identity, req MongoDocumentInsertRequest) (*MongoDocumentInsertData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMongo)
	if err != nil {
		return nil, err
	}
	if err := s.mongoCollTarget(ctx, t, c, req.Database, req.Collection); err != nil {
		return nil, err
	}
	doc, err := parseDocumentJSON(req.Document)
	if err != nil {
		return nil, err
	}
	insertedID, err := s.mongo.InsertDocument(ctx, t, c, req.Database, req.Collection, doc)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	if err := s.txAuditOnly(ctx, auditEntry(actor, ActionMongoDocumentInserted, ObjectTypeMongoDocument,
		docObjectID(inst, req.Database, req.Collection),
		map[string]any{"instanceId": inst.ID, "database": req.Database,
			"collection": req.Collection, "insertedId": insertedID}, true)); err != nil {
		return nil, err
	}
	return &MongoDocumentInsertData{InstanceID: inst.ID, Database: req.Database,
		Collection: req.Collection, InsertedID: insertedID}, nil
}

// MongoReplaceDocument POST /db/mdoc/update：文档编辑（ReplaceOne by _id
// 整体替换非字段级 patch；id=mdoc/list 响应 _id 原值透传；document._id 若
// 存在必须与 id 一致——_id 不可变更 D-23，缺省服务端以 id 补入；matched=0
// → DBM_027；审计 mongo_document_updated 同事务脱敏）。
func (s *Service) MongoReplaceDocument(ctx context.Context, actor *authsvc.Identity, req MongoDocumentUpdateRequest) (*MongoDocumentUpdateData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMongo)
	if err != nil {
		return nil, err
	}
	if err := s.mongoCollTarget(ctx, t, c, req.Database, req.Collection); err != nil {
		return nil, err
	}
	if req.ID == nil {
		return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "id", Message: "id 必填（mdoc/list 响应 _id 原值透传）"})
	}
	doc, err := parseDocumentJSON(req.Document)
	if err != nil {
		return nil, err
	}
	if docID, present := doc["_id"]; present {
		same, cerr := sameJSONValue(docID, req.ID)
		if cerr != nil || !same {
			return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
				Field: "document._id", Message: "_id 不可变更（须与 id 一致，D-23）"})
		}
	} else {
		doc["_id"] = req.ID // 缺省由服务端以 id 补入
	}
	matched, err := s.mongo.ReplaceDocument(ctx, t, c, req.Database, req.Collection, req.ID, doc)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	if matched == 0 {
		return nil, errcode.ErrDBMDocumentNotFound
	}
	if err := s.txAuditOnly(ctx, auditEntry(actor, ActionMongoDocumentUpdated, ObjectTypeMongoDocument,
		docObjectID(inst, req.Database, req.Collection),
		map[string]any{"instanceId": inst.ID, "database": req.Database,
			"collection": req.Collection, "id": req.ID}, true)); err != nil {
		return nil, err
	}
	return &MongoDocumentUpdateData{InstanceID: inst.ID, Database: req.Database,
		Collection: req.Collection, ID: req.ID, Matched: matched}, nil
}

// MongoDeleteDocument POST /db/mdoc/delete：文档删除（DeleteOne by _id；
// deleted=0 → DBM_027；审计 mongo_document_deleted 同事务脱敏）。
func (s *Service) MongoDeleteDocument(ctx context.Context, actor *authsvc.Identity, req MongoDocumentDeleteRequest) (*MongoDocumentDeleteData, error) {
	inst, t, c, err := s.governTargetByID(ctx, req.InstanceID, familyMongo)
	if err != nil {
		return nil, err
	}
	if err := s.mongoCollTarget(ctx, t, c, req.Database, req.Collection); err != nil {
		return nil, err
	}
	if req.ID == nil {
		return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "id", Message: "id 必填（mdoc/list 响应 _id 原值透传）"})
	}
	deleted, err := s.mongo.DeleteDocument(ctx, t, c, req.Database, req.Collection, req.ID)
	if err != nil {
		return nil, translateGatewayErr(err)
	}
	if deleted == 0 {
		return nil, errcode.ErrDBMDocumentNotFound
	}
	if err := s.txAuditOnly(ctx, auditEntry(actor, ActionMongoDocumentDeleted, ObjectTypeMongoDocument,
		docObjectID(inst, req.Database, req.Collection),
		map[string]any{"instanceId": inst.ID, "database": req.Database,
			"collection": req.Collection, "id": req.ID}, true)); err != nil {
		return nil, err
	}
	return &MongoDocumentDeleteData{InstanceID: inst.ID, Database: req.Database,
		Collection: req.Collection, ID: req.ID, Deleted: deleted}, nil
}

// ---- 共用前置与校验 ----

// mongoDatabaseExists 库实况存在前置（DBM_007，ListDatabases 实况判据）。
func (s *Service) mongoDatabaseExists(ctx context.Context, t dbconn.Target, c dbconn.Creds, database string) error {
	dbs, err := s.mongo.ListDatabases(ctx, t, c)
	if err != nil {
		return translateGatewayErr(err)
	}
	for _, d := range dbs {
		if d.Name == database {
			return nil
		}
	}
	return errcode.ErrDBMDatabaseNotFound
}

// mongoCollTarget 库/集合形态校验 + 库存在 DBM_007 + 集合存在 DBM_025 前置。
func (s *Service) mongoCollTarget(ctx context.Context, t dbconn.Target, c dbconn.Creds, database, collection string) error {
	if err := validateDatabaseName(database); err != nil {
		return err
	}
	// 访问已存在集合（下方 ListCollections 存在性核验）只走通用形态校验，放行
	// system. 系统集合的浏览/治理——system. 保留前缀禁令仅属建集合（v0.1.13）。
	if err := validateCollectionNameForm(collection); err != nil {
		return err
	}
	if err := s.mongoDatabaseExists(ctx, t, c, database); err != nil {
		return err
	}
	colls, err := s.mongo.ListCollections(ctx, t, c, database)
	if err != nil {
		return translateGatewayErr(err)
	}
	for _, coll := range colls {
		if coll.Name == collection {
			return nil
		}
	}
	return errcode.ErrDBMCollectionNotFound
}

// parseDocumentJSON 文档 JSON 服务端校验（DBM_026，R-14 双层校验服务端兜底）：
// 须为合法 JSON 对象（非数组/标量/空载荷）；UseNumber 解码保数值精度
// （_id 大整数 int64 不经 float64 失真，A-DB18）。
func parseDocumentJSON(raw json.RawMessage) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errcode.ErrDBMInvalidDocumentJSON.WithData(map[string]string{"reason": "document 必填"})
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil || doc == nil {
		return nil, errcode.ErrDBMInvalidDocumentJSON.WithData(map[string]string{
			"reason": "document 须为合法 JSON 对象"})
	}
	// 拒绝尾随内容（"{...} garbage" 形态）。
	if dec.More() {
		return nil, errcode.ErrDBMInvalidDocumentJSON.WithData(map[string]string{
			"reason": "document 须为单一 JSON 对象"})
	}
	return doc, nil
}

// sameJSONValue 判定两个 JSON 值形态一致（_id 不可变更校验——规范化 JSON
// 文本比对；前端透传 mdoc/list 原值故形态恒同，数值字面形态差异从紧拒绝）。
func sameJSONValue(a, b any) (bool, error) {
	ab, err := json.Marshal(a)
	if err != nil {
		return false, fmt.Errorf("dbmgmt: marshal id: %w", err)
	}
	bb, err := json.Marshal(b)
	if err != nil {
		return false, fmt.Errorf("dbmgmt: marshal id: %w", err)
	}
	return bytes.Equal(ab, bb), nil
}

// ---- 审计对象标识（v2.3）----

func collObjectID(inst *storedb.Instance, database, collection string) string {
	return dbObjectID(inst, database) + ":" + collection
}

func docObjectID(inst *storedb.Instance, database, collection string) string {
	return collObjectID(inst, database, collection)
}
