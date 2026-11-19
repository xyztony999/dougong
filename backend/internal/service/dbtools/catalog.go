package dbtools

import storedbt "github.com/dougong/dougong/internal/store/dbtools"

// CatalogEntry 工具目录条目（编译期常量 D-13/I-13；⚠️A1 两工具，扩展走
// 面板版本迭代）。
type CatalogEntry struct {
	Type           string
	DisplayName    string
	Description    string
	TargetEngines  []string // db_engine 映射（D-10 引擎匹配）
	CatalogVersion string   // 钉死版本（D-05/D-14）
}

// Catalog 两工具静态目录（D-13）。
var Catalog = []CatalogEntry{
	{Type: storedbt.ToolPHPMyAdmin, DisplayName: "phpMyAdmin",
		Description:   "MySQL/MariaDB Web 管理工具（PHP，signon 桥接面板会话）",
		TargetEngines: []string{"mysql", "mariadb"}, CatalogVersion: PHPMyAdminVersion},
	{Type: storedbt.ToolMongoExpress, DisplayName: "mongo-express",
		Description:   "MongoDB Web 管理工具（Node，basic auth 服务端注入）",
		TargetEngines: []string{"mongodb"}, CatalogVersion: MongoExpressVersion},
}

// CatalogByType 目录索引（未知类型 nil → DBT_002）。
func CatalogByType(toolType string) *CatalogEntry {
	for i := range Catalog {
		if Catalog[i].Type == toolType {
			return &Catalog[i]
		}
	}
	return nil
}

// ToolForEngine 引擎→工具映射（ToolStatusForEngine OHS，5.1：mysql/mariadb
// →phpmyadmin；mongodb→mongo_express；其余=""无对应工具）。
func ToolForEngine(engine string) string {
	for i := range Catalog {
		for _, e := range Catalog[i].TargetEngines {
			if e == engine {
				return Catalog[i].Type
			}
		}
	}
	return ""
}
