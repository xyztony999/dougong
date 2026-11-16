// Package dbmgmt 是数据库管理（database-mgmt）模块 HTTP 传输层（23 接口
// 薄壳：绑定 → service → 包络响应；api-database-mgmt.md §3.1/§3.3/§4.*）。
// backup/export 为双形态响应（D-06：成功=application/octet-stream 文件流 +
// Content-Disposition，失败=JSON 错误体）；backup/import 为 multipart。
package dbmgmt

import (
	"io"
	"net/http"
	"strconv"

	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	dbmgmtsvc "github.com/dougong/dougong/internal/service/dbmgmt"
	"github.com/gin-gonic/gin"
)

// Handler 聚合数据库管理接口的 service 依赖（薄壳）。
type Handler struct {
	svc *dbmgmtsvc.Service
}

// NewHandler 构造 handler。
func NewHandler(svc *dbmgmtsvc.Service) *Handler { return &Handler{svc: svc} }

// identity 取认证身份（AuthRequired 链路下必有；防御双保险）。
func identity(c *gin.Context) *authsvc.Identity {
	id, ok := middleware.Identity(c)
	if !ok {
		return nil
	}
	return id
}

// bindError 绑定错误 → AUTH_012（先例同法）。
func bindError(err error) *errcode.Error {
	return errcode.ErrValidation.WithFields(errcode.FieldError{
		Field: "body", Message: err.Error(),
	})
}

// ---- instance（§4.1.4）----

// List GET /db/instance/list：query {engine?}。
func (h *Handler) List(c *gin.Context) {
	data, err := h.svc.ListInstances(c.Request.Context(), c.Query("engine"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// Rescan POST /db/instance/rescan。
func (h *Handler) Rescan(c *gin.Context) {
	data, err := h.svc.Rescan(c.Request.Context(), identity(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// SetCredentials POST /db/instance/credentials。
func (h *Handler) SetCredentials(c *gin.Context) {
	var req dbmgmtsvc.CredentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.SetCredentials(c.Request.Context(), identity(c), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// Remove POST /db/instance/remove。
func (h *Handler) Remove(c *gin.Context) {
	var req dbmgmtsvc.InstanceIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.RemoveInstance(c.Request.Context(), identity(c), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ---- mysql 观测与 root 密码（§4.1.4）----

// MySQLStatus GET /db/mysql/status：query {instanceId}。
func (h *Handler) MySQLStatus(c *gin.Context) {
	data, err := h.svc.MySQLStatus(c.Request.Context(), c.Query("instanceId"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// MySQLProcessList GET /db/mysql/processlist。
func (h *Handler) MySQLProcessList(c *gin.Context) {
	data, err := h.svc.MySQLProcessList(c.Request.Context(), c.Query("instanceId"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// RootPassword POST /db/mysql/root-password。
func (h *Handler) RootPassword(c *gin.Context) {
	var req dbmgmtsvc.PasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.ChangeRootPassword(c.Request.Context(), identity(c), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ---- database（§4.2.4）----

// ListDatabases GET /db/database/list。
func (h *Handler) ListDatabases(c *gin.Context) {
	data, err := h.svc.ListDatabases(c.Request.Context(), c.Query("instanceId"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ListCharsets GET /db/database/charsets。
func (h *Handler) ListCharsets(c *gin.Context) {
	data, err := h.svc.ListCharsets(c.Request.Context(), c.Query("instanceId"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// CreateDatabase POST /db/database/create。
func (h *Handler) CreateDatabase(c *gin.Context) {
	var req dbmgmtsvc.DatabaseCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.CreateDatabase(c.Request.Context(), identity(c), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// DeleteDatabase POST /db/database/delete。
func (h *Handler) DeleteDatabase(c *gin.Context) {
	var req dbmgmtsvc.DatabaseDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.DeleteDatabase(c.Request.Context(), identity(c), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ---- dbuser（§4.3.4）----

// ListDBUsers GET /db/dbuser/list。
func (h *Handler) ListDBUsers(c *gin.Context) {
	data, err := h.svc.ListDBUsers(c.Request.Context(), c.Query("instanceId"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// CreateDBUser POST /db/dbuser/create。
func (h *Handler) CreateDBUser(c *gin.Context) {
	var req dbmgmtsvc.DBUserCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.CreateDBUser(c.Request.Context(), identity(c), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// DeleteDBUser POST /db/dbuser/delete。
func (h *Handler) DeleteDBUser(c *gin.Context) {
	var req dbmgmtsvc.DBUserRevokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.DeleteDBUser(c.Request.Context(), identity(c), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// GrantDBUser POST /db/dbuser/grant。
func (h *Handler) GrantDBUser(c *gin.Context) {
	var req dbmgmtsvc.DBUserGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.GrantDBUser(c.Request.Context(), identity(c), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// RevokeDBUser POST /db/dbuser/revoke。
func (h *Handler) RevokeDBUser(c *gin.Context) {
	var req dbmgmtsvc.DBUserRevokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.RevokeDBUser(c.Request.Context(), identity(c), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ---- backup（§4.1.4；双形态/特殊 Content-Type 由本文件专用方法承载）----

// ExportBackup POST /db/backup/export：成功=SQL 文件流（octet-stream +
// Content-Disposition，D-06 命名规则）；失败=JSON 错误体（DBM_012 data.reason）。
// 首字节探测（A-DB7）：mysqldump 启动即失败时响应头未写，可回 JSON。
func (h *Handler) ExportBackup(c *gin.Context) {
	var req struct {
		InstanceID int64  `json:"instanceId"`
		Database   string `json:"database"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	stream, err := h.svc.ExportDatabase(c.Request.Context(), identity(c), req.InstanceID, req.Database)
	if err != nil {
		response.Fail(c, err)
		return
	}
	br, err := dbmgmtsvc.PeekExport(stream)
	if err != nil {
		response.Fail(c, err) // 启动即失败：JSON 错误体（双形态失败侧）
		return
	}
	defer func() { _ = stream.Body.Close() }()
	c.Header("Content-Disposition", `attachment; filename="`+stream.Filename+`"`)
	c.Data(200, "application/octet-stream", nil) // 写响应头（200 + octet-stream）
	cw := &dbmgmtsvc.CountingWriter{W: c.Writer}
	if _, err := io.Copy(cw, br); err != nil {
		// 流中途失败：响应已开始，只能断流（D-06「不虚构成功」；HTTP 语义极限，
		// A-DB7）——审计与 busy 解除仍需收尾。
		stream.Broken(cw.N)
		return
	}
	stream.Finish(cw.N)
}

// ImportBackup POST /db/backup/import：multipart {instanceId, database, file}。
// 请求体经 MaxBytesReader 限制（512MiB + 表单开销，DBM_014 双保险）。
func (h *Handler) ImportBackup(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body,
		dbmgmtsvc.DumpSizeLimit+(1<<20))
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		response.Fail(c, errcode.ErrDBMDumpSizeExceeded)
		return
	}
	instanceIDRaw := c.PostForm("instanceId")
	instanceID, err := strconv.ParseInt(instanceIDRaw, 10, 64)
	if err != nil || instanceID <= 0 {
		response.Fail(c, errcode.ErrDBMInvalidInstanceID)
		return
	}
	database := c.PostForm("database")
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "file", Message: "导入文件缺失"}))
		return
	}
	if fileHeader.Size > dbmgmtsvc.DumpSizeLimit {
		response.Fail(c, errcode.ErrDBMDumpSizeExceeded)
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		response.Fail(c, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "file", Message: "导入文件无法读取"}))
		return
	}
	defer func() { _ = f.Close() }() // multipart 临时文件随请求清理（用后即删 D-06）
	data, err := h.svc.ImportDatabase(c.Request.Context(), identity(c),
		instanceID, database, fileHeader.Size, f)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ---- redis（§4.1.4）----

// RedisInfo GET /db/redis/info。
func (h *Handler) RedisInfo(c *gin.Context) {
	data, err := h.svc.RedisInfo(c.Request.Context(), c.Query("instanceId"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// RedisSlowlog GET /db/redis/slowlog：query {instanceId, limit?}。
func (h *Handler) RedisSlowlog(c *gin.Context) {
	data, err := h.svc.RedisSlowlog(c.Request.Context(), c.Query("instanceId"), c.Query("limit"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// RedisPassword POST /db/redis/password。
func (h *Handler) RedisPassword(c *gin.Context) {
	var req dbmgmtsvc.PasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.SetRedisPassword(c.Request.Context(), identity(c), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ---- key（§4.4.4；admin 专属 GET）----

// ListKeys GET /db/key/list：query {instanceId, cursor?, pattern?}。
func (h *Handler) ListKeys(c *gin.Context) {
	data, err := h.svc.ListKeys(c.Request.Context(),
		c.Query("instanceId"), c.Query("cursor"), c.Query("pattern"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// KeyTTL GET /db/key/ttl：query {instanceId, key}。
func (h *Handler) KeyTTL(c *gin.Context) {
	data, err := h.svc.KeyTTL(c.Request.Context(), c.Query("instanceId"), c.Query("key"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}
