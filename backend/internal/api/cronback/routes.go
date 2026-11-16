package cronback

import (
	"github.com/dougong/dougong/internal/api"
	"github.com/dougong/dougong/internal/api/middleware"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	cronbacksvc "github.com/dougong/dougong/internal/service/cronback"
	"github.com/gin-gonic/gin"
)

// Module 是计划任务与备份路由模块（实现 api.Module，挂载 /cron 前缀）。中间件
// 链（api §3.1，复用 account-auth）：InitGuard → AuthRequired → [GET 两角色
// 5 / GET admin 专属 1] / [POST → CsrfGuard → RoleRequired(admin) 9]。
type Module struct {
	auth *authsvc.Service
	svc  *cronbacksvc.Service
	h    *Handler
}

// NewModule 构造模块。
func NewModule(auth *authsvc.Service, svc *cronbacksvc.Service) *Module {
	return &Module{auth: auth, svc: svc, h: NewHandler(svc)}
}

// Prefix 挂载前缀 /cron（api §3.1 域段）。
func (m *Module) Prefix() string { return "/cron" }

// Register 注册 15 接口（§3.3 权限矩阵：GET 两角色 5——job/list、log/tail、
// backup-job/list、backup-run/list、backup-run/detail；GET admin 专属
// 1——backup-run/download（ADR-018 禁 readonly 下载）；POST admin+CSRF 9——
// job/{create,update,delete,validate,trigger} + backup-job/{create,update,
// delete,run}）。
func (m *Module) Register(rg *gin.RouterGroup) {
	rg.Use(middleware.InitGuard(m.auth))

	// 读组 A（GET；两角色同等——查看类，ADR-018 全可看）。
	readBoth := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin, authsvc.RoleReadonly))
	readBoth.GET("/job/list", m.h.JobList)
	readBoth.GET("/log/tail", m.h.LogTail)
	readBoth.GET("/backup-job/list", m.h.BackupJobList)
	readBoth.GET("/backup-run/list", m.h.BackupRunList)
	readBoth.GET("/backup-run/detail", m.h.BackupRunDetail)

	// 读组 B（GET；admin 专属——产物下载属数据取出，ADR-018 禁 readonly；
	// GET 只读无 CSRF，§3.3）。
	readAdmin := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.RequireRole(authsvc.RoleAdmin))
	readAdmin.GET("/backup-run/download", m.h.BackupRunDownload)

	// 写组（POST 全部 admin 专属 + CSRF，R-02）。
	write := rg.Group("", middleware.AuthRequired(m.auth),
		middleware.CsrfGuard(), middleware.RequireRole(authsvc.RoleAdmin))
	write.POST("/job/create", m.h.JobCreate)
	write.POST("/job/update", m.h.JobUpdate)
	write.POST("/job/delete", m.h.JobDelete)
	write.POST("/job/validate", m.h.JobValidate)
	write.POST("/job/trigger", m.h.JobTrigger)
	write.POST("/backup-job/create", m.h.BackupJobCreate)
	write.POST("/backup-job/update", m.h.BackupJobUpdate)
	write.POST("/backup-job/delete", m.h.BackupJobDelete)
	write.POST("/backup-job/run", m.h.BackupJobRun)
}

// 编译期断言：Module 实现 api.Module 契约。
var _ api.Module = (*Module)(nil)
