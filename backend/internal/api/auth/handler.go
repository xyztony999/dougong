// Package auth 是 account-auth 模块 HTTP 传输层（18 接口，薄壳：
// 绑定 → service → 包络响应；业务逻辑一律在 service）。
package auth

import (
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// Handler 聚合 account-auth 接口的 service 依赖。
type Handler struct {
	svc          *authsvc.Service
	audits       *auditsvc.Service
	cookieSecure bool
}

// NewHandler 构造 handler；cookieSecure 决定会话 Cookie 的 Secure 位
// （HTTPS 部署时置 true，§3.1）。
func NewHandler(svc *authsvc.Service, audits *auditsvc.Service, cookieSecure bool) *Handler {
	return &Handler{svc: svc, audits: audits, cookieSecure: cookieSecure}
}

// ---- 初始化（2） ----

// InitCheck GET /auth/init/check。
func (h *Handler) InitCheck(c *gin.Context) {
	st, err := h.svc.InitCheck(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, st)
}

// InitWizard POST /auth/init/wizard。
func (h *Handler) InitWizard(c *gin.Context) {
	var req authsvc.InitWizardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	issued, err := h.svc.InitWizard(c.Request.Context(), req, h.meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	h.setSessionCookie(c, issued.RawToken, issued.MaxAgeSec)
	response.OK(c, gin.H{"csrfToken": issued.CsrfToken, "user": issued.User})
}

// ---- 会话（6） ----

// Login POST /auth/session/login（两步式入口）。
func (h *Handler) Login(c *gin.Context) {
	var req authsvc.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	outcome, err := h.svc.Login(c.Request.Context(), req, h.meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	if outcome.RequiresTOTP {
		// AUTH_007：携一次性 loginTicket，前端进入二次验证。
		response.FailWith(c, errcode.ErrTotpRequired.WithData(
			gin.H{"loginTicket": outcome.LoginTicket}))
		return
	}
	h.setSessionCookie(c, outcome.Session.RawToken, outcome.Session.MaxAgeSec)
	response.OK(c, gin.H{"csrfToken": outcome.Session.CsrfToken, "user": outcome.Session.User})
}

// TOTPVerify POST /auth/session/totp-verify（二阶段换会话）。
func (h *Handler) TOTPVerify(c *gin.Context) {
	var req authsvc.TOTPVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	issued, err := h.svc.TOTPVerify(c.Request.Context(), req, h.meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	h.setSessionCookie(c, issued.RawToken, issued.MaxAgeSec)
	response.OK(c, gin.H{"csrfToken": issued.CsrfToken, "user": issued.User})
}

// CurrentSession GET /auth/session/current。
func (h *Handler) CurrentSession(c *gin.Context) {
	id, ok := middleware.Identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	response.OK(c, h.svc.CurrentUserInfo(id))
}

// Logout POST /auth/session/logout。
func (h *Handler) Logout(c *gin.Context) {
	id, ok := middleware.Identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	raw, err := c.Cookie(authsvc.SessionCookieName)
	if err != nil || raw == "" {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	if err := h.svc.Logout(c.Request.Context(), id, raw); err != nil {
		response.Fail(c, err)
		return
	}
	h.clearSessionCookie(c)
	response.OK(c, nil)
}

// ListSessions GET /auth/session/list。
func (h *Handler) ListSessions(c *gin.Context) {
	id, _ := middleware.Identity(c)
	items, err := h.svc.ListSessions(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"items": items})
}

// RevokeSession POST /auth/session/revoke（admin）。
func (h *Handler) RevokeSession(c *gin.Context) {
	var req struct {
		SessionID string `json:"sessionId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	id, _ := middleware.Identity(c)
	if err := h.svc.RevokeSession(c.Request.Context(), id, req.SessionID); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ---- 用户（8） ----

// ListUsers GET /auth/user/list。
func (h *Handler) ListUsers(c *gin.Context) {
	items, err := h.svc.ListUsers(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"items": items})
}

// CreateUser POST /auth/user/create（admin）。
func (h *Handler) CreateUser(c *gin.Context) {
	var req authsvc.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	id, _ := middleware.Identity(c)
	created, err := h.svc.CreateUser(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, created)
}

// DeleteUser POST /auth/user/delete（admin）。
func (h *Handler) DeleteUser(c *gin.Context) {
	var req struct {
		UserID int64 `json:"userId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	id, _ := middleware.Identity(c)
	if err := h.svc.DeleteUser(c.Request.Context(), id, req.UserID); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ResetPassword POST /auth/user/reset-password（admin）。
func (h *Handler) ResetPassword(c *gin.Context) {
	var req struct {
		UserID      int64  `json:"userId" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	id, _ := middleware.Identity(c)
	if err := h.svc.ResetPassword(c.Request.Context(), id, req.UserID, req.NewPassword); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ChangePassword POST /auth/user/password-change（本人）。
func (h *Handler) ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"oldPassword" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	id, _ := middleware.Identity(c)
	if err := h.svc.ChangePassword(c.Request.Context(), id, req.OldPassword, req.NewPassword); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// TOTPSetup POST /auth/user/totp-setup（本人）。
func (h *Handler) TOTPSetup(c *gin.Context) {
	id, _ := middleware.Identity(c)
	res, err := h.svc.TOTPSetup(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// TOTPEnable POST /auth/user/totp-enable（本人）。
func (h *Handler) TOTPEnable(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	id, _ := middleware.Identity(c)
	if err := h.svc.TOTPEnable(c.Request.Context(), id, req.Code); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// TOTPDisable POST /auth/user/totp-disable（本人）。
func (h *Handler) TOTPDisable(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	id, _ := middleware.Identity(c)
	if err := h.svc.TOTPDisable(c.Request.Context(), id, req.Password); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ---- 审计（2） ----

// AuditList GET /auth/audit-log/list。
func (h *Handler) AuditList(c *gin.Context) {
	cond, page, pageSize, ferr := parseAuditQuery(c)
	if ferr != nil {
		response.FailWith(c, ferr)
		return
	}
	items, total, err := h.audits.Query(c.Request.Context(), cond, page, pageSize)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"items": items, "total": total})
}

// AuditExport GET /auth/audit-log/export（admin，CSV 流）。
func (h *Handler) AuditExport(c *gin.Context) {
	cond, _, _, ferr := parseAuditQuery(c)
	if ferr != nil {
		response.FailWith(c, ferr)
		return
	}
	ctx := c.Request.Context()
	// 行数上限保护（>100k 拒绝并提示缩小范围）。
	_, total, err := h.audits.Query(ctx, cond, 1, 1)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if total > auditsvc.MaxExportRows {
		response.FailWith(c, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field:   "range",
			Message: fmt.Sprintf("结果共 %d 行超过上限 %d，请缩小筛选范围", total, auditsvc.MaxExportRows),
		}))
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition",
		`attachment; filename="audit-log-`+time.Now().UTC().Format("20060102T150405Z")+`.csv"`)
	c.Status(200)
	// UTF-8 BOM（Excel 兼容）。
	if _, err := c.Writer.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		slog.ErrorContext(ctx, "audit export: write BOM", "err", err)
		return
	}
	w := csv.NewWriter(c.Writer)
	w.UseCRLF = true
	if err := w.Write([]string{"id", "createdAt", "actorName", "category", "action",
		"objectType", "objectId", "result", "source", "detail"}); err != nil {
		slog.ErrorContext(ctx, "audit export: write header", "err", err)
		return
	}
	exported := int64(0)
	err = h.audits.StreamBatch(ctx, cond, 1000, func(batch []auditsvc.Record) error {
		for _, r := range batch {
			if err := w.Write([]string{
				strconv.FormatInt(r.ID, 10), r.CreatedAt.Format(time.RFC3339), r.ActorName,
				r.Category, r.Action, r.ObjectType, strOrEmpty(r.ObjectID), r.Result, r.Source, strOrEmpty(r.Detail),
			}); err != nil {
				return fmt.Errorf("csv write: %w", err)
			}
			exported++
		}
		return nil
	})
	w.Flush()
	if ferr := w.Error(); ferr != nil {
		// 缓冲写出失败（客户端断开等）：与流错误同级处理，不静默截断。
		err = errors.Join(err, ferr)
	}
	if err != nil {
		// 流已开始（200 已下发），无法改包络：记录错误并尽力标记截断。
		slog.ErrorContext(ctx, "audit export aborted", "rows", exported, "err", err)
		if _, werr := c.Writer.WriteString("\n[EXPORT ABORTED]"); werr != nil {
			slog.ErrorContext(ctx, "audit export: mark abort", "err", werr)
		}
		return
	}
	// 审计 audit_exported（security，§4.3.4）。
	id, _ := middleware.Identity(c)
	if rerr := h.audits.Record(ctx, auditsvc.Entry{
		ActorID:    &id.UserID,
		ActorName:  id.Username,
		Category:   auditsvc.CategorySecurity,
		Action:     auditsvc.ActionAuditExported,
		ObjectType: auditsvc.ObjectTypeAuditLog,
		Detail:     map[string]any{"rows": exported},
		Result:     auditsvc.ResultSuccess,
		Source:     auditsvc.SourceWeb,
	}); rerr != nil {
		slog.WarnContext(ctx, "audit export: record audit_exported", "err", rerr)
	}
}

// strOrEmpty 解引用可空字符串（CSV 单元格不接受 nil）。
func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ---- 助手 ----

func (h *Handler) meta(c *gin.Context) authsvc.RequestMeta {
	return authsvc.RequestMeta{SourceIP: c.ClientIP(), UserAgent: c.Request.UserAgent()}
}

func (h *Handler) setSessionCookie(c *gin.Context, token string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     authsvc.SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearSessionCookie 以 Max-Age=0 清除会话 Cookie（登出，§4.2.4）。
func (h *Handler) clearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     authsvc.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // net/http 语义：<0 序列化为 Max-Age=0（立即过期）
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

// bindError 将 gin 绑定错误转 AUTH_012（body 字段级提示）。
func bindError(err error) *errcode.Error {
	return errcode.ErrValidation.WithFields(errcode.FieldError{
		Field: "body", Message: err.Error(),
	})
}

// parseAuditQuery 解析审计查询 query（from/to=RFC3339、page/pageSize 公共分页）。
func parseAuditQuery(c *gin.Context) (cond auditsvc.Condition, page, pageSize int, ferr *errcode.Error) {
	page, pageSize = 1, 20
	var fields []errcode.FieldError
	if v := c.Query("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			fields = append(fields, errcode.FieldError{Field: "page", Message: "page 须为正整数"})
		} else {
			page = n
		}
	}
	if v := c.Query("pageSize"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			fields = append(fields, errcode.FieldError{Field: "pageSize", Message: "pageSize 须为正整数"})
		} else {
			pageSize = n
		}
	}
	for _, fn := range []struct {
		key   string
		field string
		dest  **time.Time
	}{
		{"from", "from", &cond.From},
		{"to", "to", &cond.To},
	} {
		if v := c.Query(fn.key); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				fields = append(fields, errcode.FieldError{Field: fn.field, Message: "时间须为 RFC3339 格式"})
			} else {
				*fn.dest = &t
			}
		}
	}
	cond.ActorName = c.Query("actorName")
	cond.Category = c.Query("category")
	cond.Action = c.Query("action")
	cond.ObjectType = c.Query("objectType")
	cond.ObjectID = c.Query("objectId")
	if len(fields) > 0 {
		return cond, 0, 0, errcode.ErrValidation.WithFields(fields...)
	}
	return cond, page, pageSize, nil
}
