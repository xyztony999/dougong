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

// Register 注册 30 接口（§3.3 权限矩阵：GET 两角色 10——instance/list、
// mysql/status、mysql/processlist、database/list、dbuser/list、redis/info、
// redis/slowlog、mongo/status、mdb/list、muser/list；GET admin 专属 3——
// database/charsets、key/list、key/ttl；POST admin+CSRF 17）。
func (m *Module) Register(rg *gin.RouterGroup) {
	rg.Use(middleware.InitGuard(m.auth))

	// 读组 A（GET；两角色同等——查看类，ADR-018/D0 口径）。
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

	// 读组 B（GET；admin 专属——charsets 建库辅助数据（readonly 无建库入口）
	// 与 key 浏览 ×2（key 值属数据内容取出，D0 readonly 清单未含），§3.3）。
	readAdmin := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin))
	readAdmin.GET("/database/charsets", m.h.ListCharsets)
	readAdmin.GET("/key/list", m.h.ListKeys)
	readAdmin.GET("/key/ttl", m.h.KeyTTL)

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
}

// 编译期断言：Module 实现 api.Module 契约。
var _ api.Module = (*Module)(nil)
