package dbmgmt

// MongoDB 集合/文档治理链测试（api v2.3 §4.8，D-23/D-25/I-12）：fakeMongo
// 替身承载全部链路（零 runner 实况依赖——mongo-driver 连接行为归适配器
// 纯函数测试与 Linux 冒烟）。

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/adapter/dbmongo"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// ---- fakeMongo v2.3 治理方法（记忆实况+行为注入）----

func (f *fakeMongo) DropDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, db string) error {
	if f.dropDBErr != nil {
		return f.dropDBErr
	}
	f.droppedDBs = append(f.droppedDBs, db)
	for i, d := range f.dbs {
		if d.Name == db {
			f.dbs = append(f.dbs[:i], f.dbs[i+1:]...)
			break
		}
	}
	delete(f.colls, db)
	return nil
}

func (f *fakeMongo) ListCollections(ctx context.Context, t dbconn.Target, c dbconn.Creds, db string) ([]dbmongo.CollectionInfo, error) {
	if f.collErr != nil {
		return nil, f.collErr
	}
	return f.colls[db], nil
}

func (f *fakeMongo) CreateCollection(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, name string) error {
	if f.createCollErr != nil {
		return f.createCollErr
	}
	for _, coll := range f.colls[db] {
		if coll.Name == name {
			return dbmongo.ErrCollectionExists
		}
	}
	if f.colls == nil {
		f.colls = map[string][]dbmongo.CollectionInfo{}
	}
	f.colls[db] = append(f.colls[db], dbmongo.CollectionInfo{Name: name})
	f.createdColls = append(f.createdColls, db+":"+name)
	// 隐式建库语义（D-25）：库随首集合写入产生。
	found := false
	for _, d := range f.dbs {
		if d.Name == db {
			found = true
		}
	}
	if !found {
		f.dbs = append(f.dbs, dbmongo.DatabaseInfo{Name: db})
	}
	return nil
}

func (f *fakeMongo) DropCollection(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, name string) error {
	if f.dropCollErr != nil {
		return f.dropCollErr
	}
	for i, coll := range f.colls[db] {
		if coll.Name == name {
			f.colls[db] = append(f.colls[db][:i], f.colls[db][i+1:]...)
			f.droppedColls = append(f.droppedColls, db+":"+name)
			delete(f.docs, db+"."+name)
			return nil
		}
	}
	return dbmongo.ErrCollectionNotFound
}

func (f *fakeMongo) ListDocuments(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, coll string, skip, limit int) ([]map[string]any, int64, error) {
	if f.collErr != nil {
		return nil, 0, f.collErr
	}
	all := f.docs[db+"."+coll]
	total := int64(len(all))
	if skip >= len(all) {
		return []map[string]any{}, total, nil
	}
	end := skip + limit
	if end > len(all) {
		end = len(all)
	}
	return all[skip:end], total, nil
}

func (f *fakeMongo) InsertDocument(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, coll string, doc map[string]any) (any, error) {
	if f.insertErr != nil {
		return nil, f.insertErr
	}
	key := db + "." + coll
	id := doc["_id"]
	if id == nil {
		id = f.insertedID
		if id == nil {
			id = map[string]any{"$oid": "665f000000000000000000aa"} // 区别于种子文档 _id
		}
		doc["_id"] = id
	}
	for _, d := range f.docs[key] {
		if sameID(d["_id"], id) {
			return nil, dbmongo.ErrDocumentExists
		}
	}
	f.docs[key] = append(f.docs[key], doc)
	f.insertedDocs = append(f.insertedDocs, key)
	f.lastDoc = doc
	return id, nil
}

func (f *fakeMongo) ReplaceDocument(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, coll string, id any, doc map[string]any) (int64, error) {
	if f.replaceErr != nil {
		return 0, f.replaceErr
	}
	f.lastID = id
	f.lastDoc = doc
	key := db + "." + coll
	for i, d := range f.docs[key] {
		if sameID(d["_id"], id) {
			f.docs[key][i] = doc
			f.replacedDocs = append(f.replacedDocs, key)
			return 1, nil
		}
	}
	return 0, nil
}

func (f *fakeMongo) DeleteDocument(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, coll string, id any) (int64, error) {
	if f.deleteDocErr != nil {
		return 0, f.deleteDocErr
	}
	f.lastID = id
	key := db + "." + coll
	for i, d := range f.docs[key] {
		if sameID(d["_id"], id) {
			f.docs[key] = append(f.docs[key][:i], f.docs[key][i+1:]...)
			f.deletedDocs = append(f.deletedDocs, key)
			return 1, nil
		}
	}
	return 0, nil
}

// sameID JSON 形态比对（fake 定位语义——与 service sameJSONValue 同口径）。
func sameID(a, b any) bool {
	ab, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bb, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(ab) == string(bb)
}

// ---- 测试基座扩展 ----

func newMongoGovEnv(t *testing.T) (*testEnv, *storedb.Instance) {
	t.Helper()
	e := newTestEnv(t)
	e.mongo.dbs = []dbmongo.DatabaseInfo{{Name: "appdb", SizeBytes: 4096, Collections: 1}}
	e.mongo.colls = map[string][]dbmongo.CollectionInfo{
		"appdb": {{Name: "orders", DocCount: 2, SizeBytes: 2048}},
	}
	e.mongo.docs = map[string][]map[string]any{
		"appdb.orders": {
			{"_id": map[string]any{"$oid": "665f00000000000000000001"}, "sku": "A"},
			{"_id": map[string]any{"$oid": "665f00000000000000000002"}, "sku": "B"},
		},
	}
	inst := e.seedInstance(t, storedb.EngineMongoDB)
	return e, inst
}

func rawDoc(s string) json.RawMessage { return json.RawMessage(s) }

// ---- 用例 ----

func TestMongoCollectionListCreateDrop(t *testing.T) {
	e, inst := newMongoGovEnv(t)
	ctx := context.Background()
	actor := testActor()

	data, err := e.svc.MongoListCollections(ctx, itoa(inst.ID), "appdb")
	if err != nil || len(data.Items) != 1 || data.Items[0].Name != "orders" || data.Items[0].DocCount != 2 {
		t.Fatalf("mcoll list: %+v %v", data, err)
	}
	// 库不存在 DBM_007。
	if _, err := e.svc.MongoListCollections(ctx, itoa(inst.ID), "nodb"); errCode(t, err) != "DBM_007" {
		t.Fatalf("want DBM_007, got %v", err)
	}
	// 建集合（隐式建库——新库无前置 DBM_007）。
	created, err := e.svc.MongoCreateCollection(ctx, actor, MongoCollectionCreateRequest{
		InstanceID: inst.ID, Database: "newdb", Name: "first"})
	if err != nil || created.Name != "first" {
		t.Fatalf("mcoll create implicit db: %+v %v", created, err)
	}
	if !e.hasAudit(ctx, "mongo_collection_created") {
		t.Fatal("mongo_collection_created audit missing")
	}
	// 重名 DBM_008 语义复用。
	if _, err := e.svc.MongoCreateCollection(ctx, actor, MongoCollectionCreateRequest{
		InstanceID: inst.ID, Database: "appdb", Name: "orders"}); errCode(t, err) != "DBM_008" {
		t.Fatalf("want DBM_008, got %v", err)
	}
	// 集合名形态 AUTH_012（$ 前缀/空字符/system. 保留）。
	for _, bad := range []string{"", "$bad", "system.js", "a\x00b"} {
		if _, err := e.svc.MongoCreateCollection(ctx, actor, MongoCollectionCreateRequest{
			InstanceID: inst.ID, Database: "appdb", Name: bad}); errCode(t, err) != "AUTH_012" {
			t.Fatalf("name %q want AUTH_012, got %v", bad, err)
		}
	}
	// 删集合 + 不存在 DBM_025。
	if _, err := e.svc.MongoDropCollection(ctx, actor, MongoCollectionDropRequest{
		InstanceID: inst.ID, Database: "appdb", Name: "orders"}); err != nil {
		t.Fatalf("mcoll drop: %v", err)
	}
	if !e.hasAudit(ctx, "mongo_collection_dropped") {
		t.Fatal("mongo_collection_dropped audit missing")
	}
	if _, err := e.svc.MongoDropCollection(ctx, actor, MongoCollectionDropRequest{
		InstanceID: inst.ID, Database: "appdb", Name: "orders"}); errCode(t, err) != "DBM_025" {
		t.Fatalf("want DBM_025, got %v", err)
	}
	// 引擎不匹配 DBM_018（mysql 档案走 mcoll）。
	minst := e.seedInstance(t, storedb.EngineMySQL)
	if _, err := e.svc.MongoListCollections(ctx, itoa(minst.ID), "appdb"); errCode(t, err) != "DBM_018" {
		t.Fatalf("want DBM_018, got %v", err)
	}
}

func TestMongoDropDatabaseProtection(t *testing.T) {
	e, inst := newMongoGovEnv(t)
	ctx := context.Background()
	actor := testActor()
	e.mongo.dbs = append(e.mongo.dbs,
		dbmongo.DatabaseInfo{Name: "admin"}, dbmongo.DatabaseInfo{Name: "config"}, dbmongo.DatabaseInfo{Name: "local"})

	// 系统库一律 DBM_028（I-12 硬校验，先于库存在判定）。
	for _, sys := range []string{"admin", "config", "local"} {
		if _, err := e.svc.MongoDropDatabase(ctx, actor, MongoDatabaseDropRequest{
			InstanceID: inst.ID, Database: sys}); errCode(t, err) != "DBM_028" {
			t.Fatalf("system db %q want DBM_028, got %v", sys, err)
		}
	}
	// 普通库删除成功 + 审计。
	if _, err := e.svc.MongoDropDatabase(ctx, actor, MongoDatabaseDropRequest{
		InstanceID: inst.ID, Database: "appdb"}); err != nil {
		t.Fatalf("dropdb: %v", err)
	}
	if !e.hasAudit(ctx, "mongo_database_dropped") {
		t.Fatal("mongo_database_dropped audit missing")
	}
	// 再删 DBM_007（不静默成功）。
	if _, err := e.svc.MongoDropDatabase(ctx, actor, MongoDatabaseDropRequest{
		InstanceID: inst.ID, Database: "appdb"}); errCode(t, err) != "DBM_007" {
		t.Fatalf("want DBM_007, got %v", err)
	}
}

func TestMongoDocumentListAndCRUD(t *testing.T) {
	e, inst := newMongoGovEnv(t)
	ctx := context.Background()
	actor := testActor()
	id1 := map[string]any{"$oid": "665f00000000000000000001"}

	// 分页（默认 20/上限 100 钳制 + skip 语义）。
	data, err := e.svc.MongoListDocuments(ctx, itoa(inst.ID), "appdb", "orders", "", "")
	if err != nil || data.Total != 2 || len(data.Documents) != 2 || data.PageSize != DocPageDefault {
		t.Fatalf("mdoc list: %+v %v", data, err)
	}
	data, err = e.svc.MongoListDocuments(ctx, itoa(inst.ID), "appdb", "orders", "2", "1")
	if err != nil || len(data.Documents) != 1 || data.Documents[0]["sku"] != "B" {
		t.Fatalf("mdoc page 2: %+v %v", data, err)
	}
	data, err = e.svc.MongoListDocuments(ctx, itoa(inst.ID), "appdb", "orders", "1", "500")
	if err != nil || data.PageSize != DocPageLimit {
		t.Fatalf("mdoc clamp: %+v %v", data, err)
	}
	// 集合不存在 DBM_025 / 库不存在 DBM_007。
	if _, err := e.svc.MongoListDocuments(ctx, itoa(inst.ID), "appdb", "gone", "", ""); errCode(t, err) != "DBM_025" {
		t.Fatalf("want DBM_025, got %v", err)
	}
	if _, err := e.svc.MongoListDocuments(ctx, itoa(inst.ID), "nodb", "orders", "", ""); errCode(t, err) != "DBM_007" {
		t.Fatalf("want DBM_007, got %v", err)
	}

	// 插入（缺省 _id 引擎生成形态返回）。
	ins, err := e.svc.MongoInsertDocument(ctx, actor, MongoDocumentInsertRequest{
		InstanceID: inst.ID, Database: "appdb", Collection: "orders",
		Document: rawDoc(`{"sku":"C"}`)})
	if err != nil || ins.InsertedID == nil {
		t.Fatalf("mdoc insert: %+v %v", ins, err)
	}
	if !e.hasAudit(ctx, "mongo_document_inserted") {
		t.Fatal("mongo_document_inserted audit missing")
	}
	// 插入显式 _id 冲突 → DBM_008 语义复用。
	if _, err := e.svc.MongoInsertDocument(ctx, actor, MongoDocumentInsertRequest{
		InstanceID: inst.ID, Database: "appdb", Collection: "orders",
		Document: rawDoc(`{"_id":{"$oid":"665f00000000000000000001"},"sku":"dup"}`)}); errCode(t, err) != "DBM_008" {
		t.Fatalf("dup _id want DBM_008, got %v", err)
	}
	// JSON 非法 DBM_026（数组/标量/空载荷/尾随内容）。
	for _, bad := range []string{``, `[1,2]`, `"str"`, `123`, `{"a":1} garbage`, `{invalid`} {
		if _, err := e.svc.MongoInsertDocument(ctx, actor, MongoDocumentInsertRequest{
			InstanceID: inst.ID, Database: "appdb", Collection: "orders",
			Document: rawDoc(bad)}); errCode(t, err) != "DBM_026" {
			t.Fatalf("doc %q want DBM_026, got %v", bad, err)
		}
	}

	// 编辑（ReplaceOne by _id；_id 缺省服务端补入）。
	upd, err := e.svc.MongoReplaceDocument(ctx, actor, MongoDocumentUpdateRequest{
		InstanceID: inst.ID, Database: "appdb", Collection: "orders", ID: id1,
		Document: rawDoc(`{"sku":"A2"}`)})
	if err != nil || upd.Matched != 1 {
		t.Fatalf("mdoc update: %+v %v", upd, err)
	}
	if e.mongo.lastDoc["_id"] == nil || !sameID(e.mongo.lastDoc["_id"], id1) {
		t.Fatalf("_id must be injected from locator: %+v", e.mongo.lastDoc)
	}
	if !e.hasAudit(ctx, "mongo_document_updated") {
		t.Fatal("mongo_document_updated audit missing")
	}
	// _id 不一致 → AUTH_012（不可变更）。
	if _, err := e.svc.MongoReplaceDocument(ctx, actor, MongoDocumentUpdateRequest{
		InstanceID: inst.ID, Database: "appdb", Collection: "orders", ID: id1,
		Document: rawDoc(`{"_id":{"$oid":"665f00000000000000000002"},"sku":"x"}`)}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("_id change want AUTH_012, got %v", err)
	}
	// _id 一致放行。
	if _, err := e.svc.MongoReplaceDocument(ctx, actor, MongoDocumentUpdateRequest{
		InstanceID: inst.ID, Database: "appdb", Collection: "orders", ID: id1,
		Document: rawDoc(`{"_id":{"$oid":"665f00000000000000000001"},"sku":"A3"}`)}); err != nil {
		t.Fatalf("_id consistent must pass: %v", err)
	}
	// 匹配 0 → DBM_027。
	goneID := map[string]any{"$oid": "665f000000000000000000ff"}
	if _, err := e.svc.MongoReplaceDocument(ctx, actor, MongoDocumentUpdateRequest{
		InstanceID: inst.ID, Database: "appdb", Collection: "orders", ID: goneID,
		Document: rawDoc(`{"sku":"x"}`)}); errCode(t, err) != "DBM_027" {
		t.Fatalf("want DBM_027, got %v", err)
	}
	// id 缺失 AUTH_012。
	if _, err := e.svc.MongoReplaceDocument(ctx, actor, MongoDocumentUpdateRequest{
		InstanceID: inst.ID, Database: "appdb", Collection: "orders",
		Document: rawDoc(`{"sku":"x"}`)}); errCode(t, err) != "AUTH_012" {
		t.Fatalf("missing id want AUTH_012, got %v", err)
	}

	// 删除 + DBM_027。
	del, err := e.svc.MongoDeleteDocument(ctx, actor, MongoDocumentDeleteRequest{
		InstanceID: inst.ID, Database: "appdb", Collection: "orders", ID: id1})
	if err != nil || del.Deleted != 1 {
		t.Fatalf("mdoc delete: %+v %v", del, err)
	}
	if !e.hasAudit(ctx, "mongo_document_deleted") {
		t.Fatal("mongo_document_deleted audit missing")
	}
	if _, err := e.svc.MongoDeleteDocument(ctx, actor, MongoDocumentDeleteRequest{
		InstanceID: inst.ID, Database: "appdb", Collection: "orders", ID: id1}); errCode(t, err) != "DBM_027" {
		t.Fatalf("want DBM_027, got %v", err)
	}
}

// 审计脱敏断言：mdoc insert/update detail 不含文档内容明文（D-13 v2.3 口径）。
func TestMongoDocAuditDetailRedaction(t *testing.T) {
	e, inst := newMongoGovEnv(t)
	ctx := context.Background()
	_, err := e.svc.MongoInsertDocument(ctx, testActor(), MongoDocumentInsertRequest{
		InstanceID: inst.ID, Database: "appdb", Collection: "orders",
		Document: rawDoc(`{"secret":"doc-plaintext-credential"}`)})
	if err != nil {
		t.Fatal(err)
	}
	rows, _, err := e.auditQ.Search(ctx, storeauth.AuditCondition{Action: "mongo_document_inserted"}, 0, 10)
	if err != nil || len(rows) == 0 || rows[0].Detail == nil {
		t.Fatalf("audit search: %v %d", err, len(rows))
	}
	detail := *rows[0].Detail
	if strings.Contains(detail, "doc-plaintext-credential") {
		t.Fatalf("document plaintext leaked into audit detail: %s", detail)
	}
	if !strings.Contains(detail, `"collection":"orders"`) || !strings.Contains(detail, "insertedId") {
		t.Fatalf("detail must carry locator+insertedId: %s", detail)
	}
}
