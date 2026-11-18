package dbmgmt

import (
	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/api/middleware"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	dbmgmtsvc "github.com/dougong/dougong/internal/service/dbmgmt"
	"github.com/gin-gonic/gin"
)

// Module 是数据库管理路由模块（实现 api.Module，挂载 /db 前缀）。中间件链
// （api §3.1，复用 account-auth）：InitGuard → AuthRequired → [GET 两角色 /
// GET admin 专属] / [POST → CsrfGuard → RoleRequired(admin)]。
type Module struct {
	auth *authsvc.Service
	svc  *dbmgmtsvc.Service
	h    *Handler
}

// NewModule 构造模块。
func NewModule(auth *authsvc.Service, svc *dbmgmtsvc.Service) *Module {
	return &Module{auth: auth, svc: svc, h: NewHandler(svc)}
}

// Prefix 挂载前缀 /db（api §3.1 域段）。
func (m *Module) Prefix() string { return "/db" }

// Register 注册 44 接口（api v2.3 §3.3 权限矩阵：GET 两角色 13——instance/list、
// mysql/status、mysql/processlist、database/list、dbuser/list、redis/info、
// redis/slowlog、mongo/status、mdb/list、muser/list + v2.3 元数据 table/list、
// table/structure、mcoll/list（Q6/D-24）；GET admin 专属 5——database/charsets、
// key/list、key/ttl + v2.3 数据内容 row/list、mdoc/list（I-11 沿 key 先例）；
// POST admin+CSRF 26）。
func (m *Module) Register(rg *gin.RouterGroup) {
	rg.Use(middleware.InitGuard(m.auth))

	// 读组 A（GET；两角色同等——查看类，ADR-018/D0 口径；v2.3 元数据 Q6：
	// 表清单/表结构/集合清单两角色全可看，D-24）。
	readBoth := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin, authsvc.RoleReadonly))
	readBoth.GET("/instance/list", m.h.List)
	readBoth.GET("/mysql/status", m.h.MySQLStatus)
	readBoth.GET("/mysql/processlist", m.h.MySQLProcessList)
	readBoth.GET("/database/list", m.h.ListDatabases)
	readBoth.GET("/dbuser/list", m.h.ListDBUsers)
	readBoth.GET("/redis/info", m.h.RedisInfo)
	readBoth.GET("/redis/slowlog", m.h.RedisSlowlog)
	readBoth.GET("/mongo/status", m.h.MongoStatus)
	readBoth.GET("/mdb/list", m.h.MongoListDatabases)
	readBoth.GET("/muser/list", m.h.MongoListUsers)
	readBoth.GET("/table/list", m.h.TableList)
	readBoth.GET("/table/structure", m.h.TableStructure)
	readBoth.GET("/mcoll/list", m.h.MongoListCollections)

	// 读组 B（GET；admin 专属——charsets 建库辅助数据（readonly 无建库入口）、
	// key 浏览 ×2（key 值属数据内容取出，D0 readonly 清单未含）+ v2.3 数据
	// 内容 row/list、mdoc/list（行/文档属数据内容取出，Q6 沿 key 先例，
	// I-11/D-24），§3.3）。
	readAdmin := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin))
	readAdmin.GET("/database/charsets", m.h.ListCharsets)
	readAdmin.GET("/key/list", m.h.ListKeys)
	readAdmin.GET("/key/ttl", m.h.KeyTTL)
	readAdmin.GET("/row/list", m.h.TableRows)
	readAdmin.GET("/mdoc/list", m.h.MongoListDocuments)

	// 写组（POST 全部 admin 专属 + CSRF，R-02）。
	write := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.CsrfGuard(), middleware.RequireRole(authsvc.RoleAdmin))
	write.POST("/instance/rescan", m.h.Rescan)
	write.POST("/instance/credentials", m.h.SetCredentials)
	write.POST("/instance/remove", m.h.Remove)
	write.POST("/mysql/root-password", m.h.RootPassword)
	write.POST("/database/create", m.h.CreateDatabase)
	write.POST("/database/delete", m.h.DeleteDatabase)
	write.POST("/dbuser/create", m.h.CreateDBUser)
	write.POST("/dbuser/delete", m.h.DeleteDBUser)
	write.POST("/dbuser/grant", m.h.GrantDBUser)
	write.POST("/dbuser/revoke", m.h.RevokeDBUser)
	write.POST("/backup/export", m.h.ExportBackup)
	write.POST("/backup/import", m.h.ImportBackup)
	write.POST("/redis/password", m.h.RedisPassword)
	write.POST("/muser/create", m.h.MongoCreateUser)
	write.POST("/muser/delete", m.h.MongoDeleteUser)
	write.POST("/muser/grant", m.h.MongoGrantRole)
	write.POST("/muser/revoke", m.h.MongoRevokeRole)
	// v2.3 表级/集合文档治理写组（D-26 资源段 table/row/mcoll/mdoc + mdb/dropdb）。
	write.POST("/table/drop", m.h.DropTable)
	write.POST("/row/update", m.h.UpdateRow)
	write.POST("/row/delete", m.h.DeleteRow)
	write.POST("/mcoll/create", m.h.MongoCreateCollection)
	write.POST("/mcoll/drop", m.h.MongoDropCollection)
	write.POST("/mdb/dropdb", m.h.MongoDropDatabase)
	write.POST("/mdoc/insert", m.h.MongoInsertDocument)
	write.POST("/mdoc/update", m.h.MongoReplaceDocument)
	write.POST("/mdoc/delete", m.h.MongoDeleteDocument)
}

// 编译期断言：Module 实现 api.Module 契约。
var _ api.Module = (*Module)(nil)
