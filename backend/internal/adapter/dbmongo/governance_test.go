package dbmongo

// 集合/文档治理纯函数测试（v2.3 D-23/A-DB18；CI 环境耦合预防沿第 6 起
// 先例：BSON↔扩展 JSON 映射与错误分类纯逻辑本机覆盖，mongod 实联归
// Linux 目标环境冒烟）。

import (
	"encoding/json"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ObjectId 扩展 JSON 往返：bson.D → {"$oid":"hex"} → idFromJSON 还原 ObjectID。
func TestObjectIDExtJSONRoundTrip(t *testing.T) {
	oid := bson.NewObjectID()
	m, err := docFromBSON(bson.D{{Key: "_id", Value: oid}, {Key: "sku", Value: "A"}})
	if err != nil {
		t.Fatal(err)
	}
	idForm, ok := m["_id"].(map[string]any)
	if !ok || idForm["$oid"] != oid.Hex() {
		t.Fatalf("_id must present as {\"$oid\":hex}: %+v", m["_id"])
	}
	restored, err := idFromJSON(m["_id"])
	if err != nil {
		t.Fatal(err)
	}
	roid, ok := restored.(bson.ObjectID)
	if !ok || roid != oid {
		t.Fatalf("idFromJSON must restore ObjectID: %T %+v", restored, restored)
	}
	// idToJSON 闭环（insertedId 响应载荷形态）。
	back, err := idToJSON(roid)
	if err != nil {
		t.Fatal(err)
	}
	bm, ok := back.(map[string]any)
	if !ok || bm["$oid"] != oid.Hex() {
		t.Fatalf("idToJSON: %+v", back)
	}
}

// 文档 BSON↔JSON 往返（嵌套对象/数组/数值/布尔/null 如实）。
func TestDocBSONRoundTrip(t *testing.T) {
	doc := map[string]any{
		"sku":   "A",
		"qty":   json.Number("3"),
		"price": json.Number("19.5"),
		"ok":    true,
		"note":  nil,
		"tags":  []any{"x", "y"},
		"meta":  map[string]any{"k": "v"},
	}
	bdoc, err := docToBSON(doc)
	if err != nil {
		t.Fatal(err)
	}
	back, err := docFromBSON(bdoc)
	if err != nil {
		t.Fatal(err)
	}
	if back["sku"] != "A" || back["ok"] != true || back["note"] != nil {
		t.Fatalf("scalar roundtrip: %+v", back)
	}
	if back["qty"] != json.Number("3") && back["qty"] != float64(3) {
		t.Fatalf("int roundtrip: %+v (%T)", back["qty"], back["qty"])
	}
	tags, ok := back["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "x" {
		t.Fatalf("array roundtrip: %+v", back["tags"])
	}
	meta, ok := back["meta"].(map[string]any)
	if !ok || meta["k"] != "v" {
		t.Fatalf("nested roundtrip: %+v", back["meta"])
	}
}

// _id 多形态还原（字符串/数值/扩展 JSON）——mdoc/list 原值透传闭环。
func TestIDFromJSONForms(t *testing.T) {
	cases := []any{"abc", json.Number("42"), map[string]any{"$oid": bson.NewObjectID().Hex()}}
	for _, id := range cases {
		bid, err := idFromJSON(id)
		if err != nil {
			t.Fatalf("idFromJSON(%+v): %v", id, err)
		}
		back, err := idToJSON(bid)
		if err != nil {
			t.Fatal(err)
		}
		ab, _ := json.Marshal(id)
		bb, _ := json.Marshal(back)
		if string(ab) != string(bb) {
			t.Fatalf("id roundtrip mismatch: %s vs %s", ab, bb)
		}
	}
}

// 非法文档载荷（不可 JSON 序列化形态）如实报错。
func TestDocToBSONInvalid(t *testing.T) {
	if _, err := docToBSON(map[string]any{"x": make(chan int)}); err == nil {
		t.Fatal("unmarshalable document must fail")
	}
}

// 文档内显式 _id 扩展 JSON 形态还原（insert 显式 _id 路径——元素值位
// extjson 类型识别：{"$oid"} → ObjectID 而非子文档）。
func TestDocToBSONExplicitID(t *testing.T) {
	oid := bson.NewObjectID()
	bdoc, err := docToBSON(map[string]any{
		"_id": map[string]any{"$oid": oid.Hex()},
		"ref": map[string]any{"$oid": oid.Hex()},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range bdoc {
		if e.Key == "_id" || e.Key == "ref" {
			if got, ok := e.Value.(bson.ObjectID); !ok || got != oid {
				t.Fatalf("%s must restore ObjectID, got %T %+v", e.Key, e.Value, e.Value)
			}
		}
	}
}

// classify 治理新码（48 集合重名 / 26 集合不存在 / 11000 _id 冲突）。
func TestClassifyGovernanceCodes(t *testing.T) {
	cases := []struct {
		code int32
		want error
	}{
		{48, ErrCollectionExists},
		{26, ErrCollectionNotFound},
		{11000, ErrDocumentExists},
	}
	for _, c := range cases {
		got := classify(mongo.CommandError{Code: c.code, Message: "governance conflict"})
		if !errors.Is(got, c.want) {
			t.Errorf("code %d: want %v, got %v", c.code, c.want, got)
		}
	}
}
