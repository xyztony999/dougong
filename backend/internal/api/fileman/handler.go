package fileman

import (
	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	filemansvc "github.com/dougong/dougong/internal/service/fileman"
	"github.com/gin-gonic/gin"
)

// Handler 聚合文件管理器接口的 service 依赖（薄壳：绑定 → service → 包络）。
type Handler struct {
	svc *filemansvc.Service
}

// NewHandler 构造 handler。
func NewHandler(svc *filemansvc.Service) *Handler { return &Handler{svc: svc} }

// identity 提取认证身份（防御双保险：AuthRequired 链路下必可达）。
func identity(c *gin.Context) (*authsvc.Identity, bool) { return middleware.Identity(c) }

// bindError 将 gin 绑定错误转 AUTH_012（body 字段级提示，A-S9 先例）。
func bindError(err error) *errcode.Error {
	return errcode.ErrValidation.WithFields(errcode.FieldError{
		Field: "body", Message: err.Error(),
	})
}

// ScopeGet GET /files/path-scope/get（admin）：白名单配置查看（FR-02）。
func (h *Handler) ScopeGet(c *gin.Context) {
	response.OK(c, h.svc.GetScope())
}

// scopeUpdateRequest 是 path-scope/update 请求体（excludedDirs 可空数组）。
type scopeUpdateRequest struct {
	RootPath     string   `json:"rootPath"`
	ExcludedDirs []string `json:"excludedDirs"`
}

// ScopeUpdate POST /files/path-scope/update（admin+CSRF）：受管根与排除目录
// 修改（同事务审计+刷新副本，D-02）。
func (h *Handler) ScopeUpdate(c *gin.Context) {
	var req scopeUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.UpdateScope(c.Request.Context(), actor, req.RootPath, req.ExcludedDirs)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// List GET /files/entry/list（admin+readonly）：目录全量投影 + 服务端排序
// （D-03；S-20-02 导航为前端路径拆分复用本接口）。
func (h *Handler) List(c *gin.Context) {
	data, err := h.svc.ListDir(c.Request.Context(), c.Query("path"), c.Query("sort"), c.Query("order"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// Content GET /files/entry/content（admin）：编辑打开（D-05 可编辑性检测）。
func (h *Handler) Content(c *gin.Context) {
	data, err := h.svc.Content(c.Request.Context(), c.Query("path"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// contentSaveRequest 是 content-save 请求体。
type contentSaveRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ContentSave POST /files/entry/content-save（admin+CSRF）：全量覆盖保存
// （I-09 最后写者胜，D-05）。
func (h *Handler) ContentSave(c *gin.Context) {
	var req contentSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.ContentSave(c.Request.Context(), actor, req.Path, req.Content)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// createRequest 是 entry/create 请求体（type=file|directory）。
type createRequest struct {
	Dir  string `json:"dir"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// Create POST /files/entry/create（admin+CSRF）：新建空文件/空目录（FR-09）。
func (h *Handler) Create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.Create(c.Request.Context(), actor, req.Dir, req.Name, req.Type)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// renameRequest 是 entry/rename 请求体。
type renameRequest struct {
	Path    string `json:"path"`
	NewName string `json:"newName"`
}

// Rename POST /files/entry/rename（admin+CSRF）：原目录内重命名（D-13）。
func (h *Handler) Rename(c *gin.Context) {
	var req renameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.Rename(c.Request.Context(), actor, req.Path, req.NewName)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// moveRequest 是 entry/move 请求体。
type moveRequest struct {
	SourcePath string `json:"sourcePath"`
	TargetDir  string `json:"targetDir"`
	OnConflict string `json:"onConflict"`
}

// Move POST /files/entry/move（admin+CSRF）：跨目录移动（含子树；D-07/D-15）。
func (h *Handler) Move(c *gin.Context) {
	var req moveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.Move(c.Request.Context(), actor, req.SourcePath, req.TargetDir, req.OnConflict)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// deleteRequest 是 entry/delete 请求体。
type deleteRequest struct {
	Path string `json:"path"`
}

// Delete POST /files/entry/delete（admin+CSRF）：删除入回收站（FR-11/FR-16
// 同事务入站登记，D-15）。
func (h *Handler) Delete(c *gin.Context) {
	var req deleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.Delete(c.Request.Context(), actor, req.Path)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// chmodRequest 是 entry/chmod 请求体（八进制串，如 0644）。
type chmodRequest struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
}

// Chmod POST /files/entry/chmod（admin+CSRF）：权限位修改（不递归，D-06）。
func (h *Handler) Chmod(c *gin.Context) {
	var req chmodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.Chmod(c.Request.Context(), actor, req.Path, req.Mode)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// chownRequest 是 entry/chown 请求体（ownerUser/ownerGroup 至少一项）。
type chownRequest struct {
	Path       string `json:"path"`
	OwnerUser  string `json:"ownerUser"`
	OwnerGroup string `json:"ownerGroup"`
}

// Chown POST /files/entry/chown（admin+CSRF）：属主修改（候选经
// system-accounts/list，D-14；不递归 D-06）。
func (h *Handler) Chown(c *gin.Context) {
	var req chownRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.Chown(c.Request.Context(), actor, req.Path, req.OwnerUser, req.OwnerGroup)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// SystemAccounts GET /files/system-accounts/list（admin）：候选清单实时读取
// （D-14；仅 name/uid、name/gid 下发）。
func (h *Handler) SystemAccounts(c *gin.Context) {
	data, err := h.svc.SystemAccounts(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// archiveRequest 是 entry/archive 请求体。
type archiveRequest struct {
	Paths      []string `json:"paths"`
	Format     string   `json:"format"`
	TargetName string   `json:"targetName"`
}

// Archive POST /files/entry/archive（admin+CSRF）：压缩（FR-14；D-13）。
func (h *Handler) Archive(c *gin.Context) {
	var req archiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.Archive(c.Request.Context(), actor, req.Paths, req.Format, req.TargetName)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// unarchiveRequest 是 entry/unarchive 请求体。
type unarchiveRequest struct {
	ArchivePath string `json:"archivePath"`
	TargetDir   string `json:"targetDir"`
	OnConflict  string `json:"onConflict"`
}

// Unarchive POST /files/entry/unarchive（admin+CSRF）：解压（zip slip 全量
// 预检，D-07/FR-15）。
func (h *Handler) Unarchive(c *gin.Context) {
	var req unarchiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.Unarchive(c.Request.Context(), actor, req.ArchivePath, req.TargetDir, req.OnConflict)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// TrashList GET /files/trash/list（admin+readonly）：在站条目清单（D-03）。
func (h *Handler) TrashList(c *gin.Context) {
	data, err := h.svc.TrashList(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// trashRestoreRequest 是 trash/restore 请求体（onConflict: ask|overwrite）。
type trashRestoreRequest struct {
	TrashID    int64  `json:"trashId"`
	OnConflict string `json:"onConflict"`
}

// TrashRestore POST /files/trash/restore（admin+CSRF）：还原至原路径（FR-18）。
func (h *Handler) TrashRestore(c *gin.Context) {
	var req trashRestoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.TrashRestore(c.Request.Context(), actor, req.TrashID, req.OnConflict)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// TrashPurge POST /files/trash/purge（admin+CSRF）：清空全部在站（终局销毁；
// 空站幂等成功，PRD N3）。
func (h *Handler) TrashPurge(c *gin.Context) {
	actor, ok := identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	data, err := h.svc.TrashPurge(c.Request.Context(), actor)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}
