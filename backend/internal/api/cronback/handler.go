// Package cronback 是计划任务与备份 HTTP 传输层（api-cron-backup.md §4）：
// 15 接口薄壳（绑定 → service → 包络响应）；log/tail 为 200+纯文本、
// download 为 200+文件流（Content-Disposition: attachment，§3.1）。
package cronback

import (
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strconv"

	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	cronbacksvc "github.com/dougong/dougong/internal/service/cronback"
	"github.com/gin-gonic/gin"
)

// Handler 是 cron-backup 接口处理器（薄壳，无业务逻辑）。
type Handler struct {
	svc *cronbacksvc.Service
}

// NewHandler 构造。
func NewHandler(svc *cronbacksvc.Service) *Handler { return &Handler{svc: svc} }

// identity 读取当前认证身份（AuthRequired 链路保障非空）。
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

// ---- cron_job（§4.3.4）----

// JobList GET /cron/job/list（两角色）。
func (h *Handler) JobList(c *gin.Context) {
	data, err := h.svc.ListCronJobs(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// JobCreate POST /cron/job/create（admin+CSRF）。
func (h *Handler) JobCreate(c *gin.Context) {
	var req cronbacksvc.CronJobCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.CreateCronJob(c.Request.Context(), identity(c), &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// JobUpdate POST /cron/job/update（admin+CSRF）。
func (h *Handler) JobUpdate(c *gin.Context) {
	var req cronbacksvc.CronJobUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.UpdateCronJob(c.Request.Context(), identity(c), &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// JobDelete POST /cron/job/delete（admin+CSRF）。
func (h *Handler) JobDelete(c *gin.Context) {
	var req cronbacksvc.CronJobDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	if err := h.svc.DeleteCronJob(c.Request.Context(), identity(c), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// JobValidate POST /cron/job/validate（admin+CSRF；计算服务用 POST，D-03）。
func (h *Handler) JobValidate(c *gin.Context) {
	var req cronbacksvc.CronExprValidateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.ValidateCronExpr(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// JobTrigger POST /cron/job/trigger（admin+CSRF；同步 120s，D-05——执行失败/
// 超时为业务结果非请求错误，200+code=0）。
func (h *Handler) JobTrigger(c *gin.Context) {
	var req cronbacksvc.CronJobTriggerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.TriggerCronJob(c.Request.Context(), identity(c), &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// LogTail GET /cron/log/tail（两角色；200+纯文本片段，D-04——日志本质为行流
// 非 JSON 数组）。
func (h *Handler) LogTail(c *gin.Context) {
	text, err := h.svc.TailCronLog(c.Request.Context(), c.Query("lines"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, "%s", text)
}

// ---- backup_job（§4.1.4）----

// BackupJobList GET /cron/backup-job/list（两角色）。
func (h *Handler) BackupJobList(c *gin.Context) {
	data, err := h.svc.ListBackupJobs(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// BackupJobCreate POST /cron/backup-job/create（admin+CSRF）。
func (h *Handler) BackupJobCreate(c *gin.Context) {
	var req cronbacksvc.BackupJobReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.CreateBackupJob(c.Request.Context(), identity(c), &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// BackupJobUpdate POST /cron/backup-job/update（admin+CSRF）。
func (h *Handler) BackupJobUpdate(c *gin.Context) {
	var req cronbacksvc.BackupJobReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	if err := h.svc.UpdateBackupJob(c.Request.Context(), identity(c), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// BackupJobDelete POST /cron/backup-job/delete（admin+CSRF）。
func (h *Handler) BackupJobDelete(c *gin.Context) {
	var req cronbacksvc.BackupJobIDReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	if err := h.svc.DeleteBackupJob(c.Request.Context(), identity(c), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// BackupJobRun POST /cron/backup-job/run（admin+CSRF；异步返回 runId，D-07）。
func (h *Handler) BackupJobRun(c *gin.Context) {
	var req cronbacksvc.BackupJobIDReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.RunBackupJob(c.Request.Context(), identity(c), req.JobID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// ---- backup_run（§4.2.4）----

// BackupRunList GET /cron/backup-run/list（两角色）。
func (h *Handler) BackupRunList(c *gin.Context) {
	var q cronbacksvc.RunQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	data, err := h.svc.ListBackupRuns(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// BackupRunDetail GET /cron/backup-run/detail（两角色；query {runId}）。
func (h *Handler) BackupRunDetail(c *gin.Context) {
	runID, err := strconv.ParseInt(c.Query("runId"), 10, 64)
	if err != nil || runID <= 0 {
		response.Fail(c, errcode.ErrCBValidation.WithFields(errcode.FieldError{
			Field: "runId", Message: "runId 须为正整数"}))
		return
	}
	data, derr := h.svc.BackupRunDetail(c.Request.Context(), runID)
	if derr != nil {
		response.Fail(c, derr)
		return
	}
	response.OK(c, data)
}

// BackupRunDownload GET /cron/backup-run/download（admin 专属，ADR-018 禁
// readonly 上传下载；D-16 流式响应账：校验全过才开流——开流后 IO 错误以截断
// 流呈现、服务端日志兜底）。
func (h *Handler) BackupRunDownload(c *gin.Context) {
	runID, err := strconv.ParseInt(c.Query("runId"), 10, 64)
	if err != nil || runID <= 0 {
		response.Fail(c, errcode.ErrCBValidation.WithFields(errcode.FieldError{
			Field: "runId", Message: "runId 须为正整数"}))
		return
	}
	target, derr := h.svc.DownloadArtifact(c.Request.Context(), identity(c), runID, c.Query("file"))
	if derr != nil {
		response.Fail(c, derr)
		return
	}
	defer func() {
		if target.Cleanup != nil {
			target.Cleanup()
		}
	}()
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": target.Name,
	}))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", strconv.FormatInt(target.Size, 10))
	c.Status(http.StatusOK)
	src := mustOpen(target.Path)
	defer func() { _ = src.Close() }()
	if _, cerr := io.Copy(c.Writer, src); cerr != nil {
		slog.WarnContext(c.Request.Context(), "cron-backup: download stream truncated",
			"runId", runID, "file", target.Name, "err", cerr)
	}
}

// mustOpen 打开下载源文件（校验链已过，残余 IO 失败以空流兜底——响应头已
// 定，Content-Length 不符即客户端如实感知截断，D-16 同判）。
func mustOpen(path string) io.ReadCloser {
	f, err := os.Open(path)
	if err != nil {
		slog.Error("cron-backup: open artifact failed", "path", path, "err", err)
		return emptyFile{}
	}
	return f
}

// emptyFile 是打开失败的空流兜底。
type emptyFile struct{}

func (emptyFile) Read([]byte) (int, error) { return 0, io.EOF }
func (emptyFile) Close() error             { return nil }
