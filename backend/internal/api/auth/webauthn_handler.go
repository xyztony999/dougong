package auth

import (
	"github.com/dougong/dougong/internal/api/middleware"
	"github.com/dougong/dougong/internal/api/response"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// ---- 登录安全增量（passkey/恢复码，v0.7.0）----

// WebAuthnRegisterBegin POST /auth/webauthn/register-begin（会话）。
func (h *Handler) WebAuthnRegisterBegin(c *gin.Context) {
	id, ok := middleware.Identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	res, err := h.svc.RegisterBegin(c.Request.Context(), id, h.meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// WebAuthnRegisterFinish POST /auth/webauthn/register-finish（会话）。
func (h *Handler) WebAuthnRegisterFinish(c *gin.Context) {
	id, ok := middleware.Identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	var req authsvc.WebAuthnRegisterFinishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	item, err := h.svc.RegisterFinish(c.Request.Context(), id, req, h.meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// WebAuthnList GET /auth/webauthn/list（会话）。
func (h *Handler) WebAuthnList(c *gin.Context) {
	id, ok := middleware.Identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	items, err := h.svc.ListCredentials(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"items": items})
}

// WebAuthnDelete POST /auth/webauthn/delete（会话）。
func (h *Handler) WebAuthnDelete(c *gin.Context) {
	id, ok := middleware.Identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	var req authsvc.WebAuthnDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	if err := h.svc.DeleteCredential(c.Request.Context(), id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// WebAuthnLoginBegin POST /auth/webauthn/login-begin（公开，无密码流）。
func (h *Handler) WebAuthnLoginBegin(c *gin.Context) {
	var req authsvc.WebAuthnLoginBeginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	res, err := h.svc.LoginBegin(c.Request.Context(), req, h.meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// WebAuthnLoginFinish POST /auth/webauthn/login-finish（公开，无密码登录）。
func (h *Handler) WebAuthnLoginFinish(c *gin.Context) {
	var req authsvc.WebAuthnLoginFinishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	issued, err := h.svc.LoginFinish(c.Request.Context(), req, h.meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	h.setSessionCookie(c, issued.RawToken, issued.MaxAgeSec)
	response.OK(c, gin.H{"csrfToken": issued.CsrfToken, "user": issued.User, "newDevice": issued.NewDevice})
}

// PasskeyVerify POST /auth/session/passkey-verify（公开凭 ticket，二因素）。
func (h *Handler) PasskeyVerify(c *gin.Context) {
	var req authsvc.WebAuthnLoginFinishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	issued, err := h.svc.PasskeyVerify(c.Request.Context(), req, h.meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	h.setSessionCookie(c, issued.RawToken, issued.MaxAgeSec)
	response.OK(c, gin.H{"csrfToken": issued.CsrfToken, "user": issued.User, "newDevice": issued.NewDevice})
}

// RecoveryGenerate POST /auth/recovery-codes/generate（会话）。
func (h *Handler) RecoveryGenerate(c *gin.Context) {
	id, ok := middleware.Identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	res, err := h.svc.GenerateRecoveryCodes(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// RecoveryCount GET /auth/recovery-codes（会话）。
func (h *Handler) RecoveryCount(c *gin.Context) {
	id, ok := middleware.Identity(c)
	if !ok {
		response.FailWith(c, errcode.ErrUnauthenticated)
		return
	}
	res, err := h.svc.RecoveryCount(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// RecoveryLogin POST /auth/session/recovery-login（公开）。
func (h *Handler) RecoveryLogin(c *gin.Context) {
	var req authsvc.RecoveryLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, bindError(err))
		return
	}
	issued, err := h.svc.RecoveryLogin(c.Request.Context(), req, h.meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	h.setSessionCookie(c, issued.RawToken, issued.MaxAgeSec)
	response.OK(c, gin.H{"csrfToken": issued.CsrfToken, "user": issued.User, "newDevice": issued.NewDevice})
}
