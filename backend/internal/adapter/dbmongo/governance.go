package dbmongo

// MongoDB 集合与文档治理网关扩展（api-database-mgmt.md v2.3 §4.5.1/§4.8，
// D-23/D-25）：删库/集合清单/建删集合/文档分页与 CRUD。通道沿用既有
// dial/withClient（每操作独立建连即断，D-15；连接超时 5s/命令超时 30s）。
//
// BSON↔JSON 映射（A-DB18）：驱动 bson.MarshalExtJSON/UnmarshalExtJSON
// relaxed 形态——ObjectId→{"$oid":"hex"}、日期→{"$date":...} 扩展 JSON
// 透传（api §4.8.4）；mdoc/update id=前端原值透传，网关内还原 BSON 定位；
// _id 不可变更（service 校验一致性，网关以还原值权威补入）。
//
// 系统库删除保护（I-12/DBM_028）由 service 层前置硬校验——本层不重复判定
// （引擎侧 dropDatabase admin 亦会被权限语境拒绝，如实呈现）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// 治理哨兵错误（service 层翻译 DBM 语义）。
var (
	// ErrCollectionExists 同名集合已存在（code 48）→ DBM_008 语义复用。
	ErrCollectionExists = errors.New("dbmongo: collection already exists")
	// ErrCollectionNotFound 集合不存在（code 26 NamespaceNotFound）→ DBM_025。
	ErrCollectionNotFound = errors.New("dbmongo: collection not found")
	// ErrDocumentExists 文档 _id 冲突（code 11000 DuplicateKey）→ DBM_008 语义复用。
	ErrDocumentExists = errors.New("dbmongo: document already exists")
)

// CollectionInfo 是集合清单条目（§4.8.4 mcoll/list items；docCount=
// estimatedDocumentCount 估算语义如实，sizeBytes=collStats.size）。
type CollectionInfo struct {
	Name      string
	DocCount  int64
	SizeBytes int64
}

// DropDatabase 删库（dropDatabase 连同全部集合与文档移除，不可逆；系统库
// 保护 I-12 由 service 前置 DBM_028）。
func (g *Gateway) DropDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, db string) error {
	return g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		return cl.Database(db).Drop(ctx)
	})
}

// ListCollections 集合清单实况投影（ListCollectionSpecifications + 逐集合
// estimatedDocumentCount/collStats；空库返回空列表非错误）。
func (g *Gateway) ListCollections(ctx context.Context, t dbconn.Target, c dbconn.Creds, db string) ([]CollectionInfo, error) {
	var out []CollectionInfo
	err := g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		specs, err := cl.Database(db).ListCollectionSpecifications(ctx, bson.D{})
		if err != nil {
			return err
		}
		for _, spec := range specs {
			info := CollectionInfo{Name: spec.Name}
			coll := cl.Database(db).Collection(spec.Name)
			if n, cerr := coll.EstimatedDocumentCount(ctx); cerr == nil {
				info.DocCount = n
			} // 估算失败不阻断清单（置 0 如实降级，沿 ListDatabases dbStats 先例）
			var stats struct {
				Size int64 `bson:"size"`
			}
			if serr := cl.Database(db).RunCommand(ctx,
				bson.D{{Key: "collStats", Value: spec.Name}}).Decode(&stats); serr == nil {
				info.SizeBytes = stats.Size
			}
			out = append(out, info)
		}
		return nil
	})
	return out, err
}

// CreateCollection 建集合（createCollection 显式；隐式建库语义——库随首
// 集合写入产生，D-25；重名 → ErrCollectionExists）。
func (g *Gateway) CreateCollection(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, name string) error {
	return g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		return cl.Database(db).CreateCollection(ctx, name)
	})
}

// DropCollection 删集合（dropCollection 不可逆；不存在 → ErrCollectionNotFound，
// 不静默成功）。
func (g *Gateway) DropCollection(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, name string) error {
	return g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		return cl.Database(db).Collection(name).Drop(ctx)
	})
}

// ListDocuments 文档分页（find skip/limit + CountDocuments；响应保引擎自然
// 序，_id 置顶呈现归前端排序；文档值为 BSON→扩展 JSON 映射）。
func (g *Gateway) ListDocuments(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, coll string, skip, limit int) ([]map[string]any, int64, error) {
	var docs []map[string]any
	var total int64
	err := g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		col := cl.Database(db).Collection(coll)
		n, err := col.CountDocuments(ctx, bson.D{})
		if err != nil {
			return err
		}
		total = n
		cursor, err := col.Find(ctx, bson.D{},
			options.Find().SetSkip(int64(skip)).SetLimit(int64(limit)))
		if err != nil {
			return err
		}
		defer func() { _ = cursor.Close(ctx) }()
		var raw []bson.D
		if err := cursor.All(ctx, &raw); err != nil {
			return err
		}
		docs = make([]map[string]any, 0, len(raw))
		for _, d := range raw {
			m, derr := docFromBSON(d)
			if derr != nil {
				return derr
			}
			docs = append(docs, m)
		}
		return nil
	})
	return docs, total, err
}

// InsertDocument 文档插入（InsertOne；_id 缺省引擎自动生成 ObjectId；
// _id 冲突 → ErrDocumentExists，data.reason 引擎原文）。返回插入文档 _id
// 的 JSON 形态（ObjectId 为 {"$oid":"hex"}）。
func (g *Gateway) InsertDocument(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, coll string, doc map[string]any) (any, error) {
	var insertedID any
	err := g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		bdoc, err := docToBSON(doc)
		if err != nil {
			return err
		}
		res, err := cl.Database(db).Collection(coll).InsertOne(ctx, bdoc)
		if err != nil {
			return err
		}
		id, err := idToJSON(res.InsertedID)
		if err != nil {
			return err
		}
		insertedID = id
		return nil
	})
	return insertedID, err
}

// ReplaceDocument 文档编辑（ReplaceOne by _id 整体替换，非字段级 patch，
// D-23；_id 不可变更——以 id 还原的 BSON 值权威补入 replacement）。返回
// matched 行数（0 → service DBM_027）。
func (g *Gateway) ReplaceDocument(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, coll string, id any, doc map[string]any) (int64, error) {
	var matched int64
	err := g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		bid, err := idFromJSON(id)
		if err != nil {
			return err
		}
		bdoc, err := docToBSON(doc)
		if err != nil {
			return err
		}
		replacement := make(bson.D, 0, len(bdoc)+1)
		replacement = append(replacement, bson.E{Key: "_id", Value: bid})
		for _, e := range bdoc {
			if e.Key == "_id" {
				continue // _id 以定位键权威值为准（不可变更）
			}
			replacement = append(replacement, e)
		}
		res, err := cl.Database(db).Collection(coll).ReplaceOne(ctx, bson.D{{Key: "_id", Value: bid}}, replacement)
		if err != nil {
			return err
		}
		matched = res.MatchedCount
		return nil
	})
	return matched, err
}

// DeleteDocument 文档删除（DeleteOne by _id）。返回 deleted 行数
// （0 → service DBM_027）。
func (g *Gateway) DeleteDocument(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, coll string, id any) (int64, error) {
	var deleted int64
	err := g.withClient(t, c, func(ctx context.Context, cl *mongo.Client) error {
		bid, err := idFromJSON(id)
		if err != nil {
			return err
		}
		res, err := cl.Database(db).Collection(coll).DeleteOne(ctx, bson.D{{Key: "_id", Value: bid}})
		if err != nil {
			return err
		}
		deleted = res.DeletedCount
		return nil
	})
	return deleted, err
}

// ---- BSON↔扩展 JSON 纯函数（单测锚点，零 runner 实况依赖）----

// docToBSON JSON 形态文档 → bson.D（relaxed extjson 还原：{"$oid"}→ObjectID、
// {"$date"}→time.Time、数值按字面形态还原 int32/int64/double）。
func docToBSON(doc map[string]any) (bson.D, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("dbmongo: marshal document: %w", err)
	}
	var d bson.D
	if err := bson.UnmarshalExtJSON(raw, false, &d); err != nil {
		return nil, fmt.Errorf("dbmongo: document to BSON: %w", err)
	}
	return d, nil
}

// docFromBSON bson.D → 扩展 JSON 形态 map（ObjectId→{"$oid":"hex"}、
// 日期→{"$date":...}；嵌套对象/数组如实）。
func docFromBSON(d bson.D) (map[string]any, error) {
	raw, err := bson.MarshalExtJSON(d, false, false)
	if err != nil {
		return nil, fmt.Errorf("dbmongo: marshal extjson: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("dbmongo: parse extjson: %w", err)
	}
	return m, nil
}

// idFromJSON _id JSON 原值 → BSON 定位值（{"$oid":"hex"}→ObjectID、字符串/
// 数值等如实还原；mdoc/list 响应原值透传闭环）。驱动 extjson 顶层 any 解码
// 落 bson.D 不识别扩展类型——经 {"_id": <id>} 包装文档解码取元素值
// （元素值位 extjson 类型识别生效）。
func idFromJSON(id any) (any, error) {
	raw, err := json.Marshal(id)
	if err != nil {
		return nil, fmt.Errorf("dbmongo: marshal id: %w", err)
	}
	var wrapper bson.D
	if err := bson.UnmarshalExtJSON([]byte(`{"_id":`+string(raw)+`}`), false, &wrapper); err != nil {
		return nil, fmt.Errorf("dbmongo: id to BSON: %w", err)
	}
	for _, e := range wrapper {
		if e.Key == "_id" {
			return e.Value, nil
		}
	}
	return nil, fmt.Errorf("dbmongo: id wrapper missing _id element")
}

// idToJSON BSON _id → 扩展 JSON 形态（insertedId 响应载荷）。驱动 extjson
// 编码器要求顶层文档——经 {"_id": <id>} 包装 marshal 后取 _id 值。
func idToJSON(id any) (any, error) {
	raw, err := bson.MarshalExtJSON(bson.D{{Key: "_id", Value: id}}, false, false)
	if err != nil {
		return nil, fmt.Errorf("dbmongo: marshal id extjson: %w", err)
	}
	var wrapper map[string]any
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("dbmongo: parse id extjson: %w", err)
	}
	v, ok := wrapper["_id"]
	if !ok {
		return nil, fmt.Errorf("dbmongo: id extjson missing _id field")
	}
	return v, nil
}
