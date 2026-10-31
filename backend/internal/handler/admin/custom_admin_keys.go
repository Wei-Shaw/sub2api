package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func requireAdminKeyJWT(c *gin.Context) bool {
	if c.GetString("auth_method") != service.AuditAuthMethodJWT {
		response.Error(c, 403, "仅管理员 JWT 会话可管理密钥")
		return false
	}
	role, ok := middleware.GetUserRoleFromContext(c)
	if !ok || role != "admin" {
		response.Error(c, 403, "需要管理员权限")
		return false
	}
	return true
}
func (h *SettingHandler) ListScopedAdminKeys(c *gin.Context) {
	if !requireAdminKeyJWT(c) {
		return
	}
	items, e := h.settingService.ListScopedAdminKeys(c.Request.Context())
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, gin.H{"items": items, "permissions": adminaccess.PermissionCatalog()})
}
func (h *SettingHandler) CreateScopedAdminKey(c *gin.Context) {
	if !requireAdminKeyJWT(c) {
		return
	}
	var in service.ScopedAdminKeyInput
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Error(c, 400, "请求格式无效")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Error(c, 403, "需要管理员身份")
		return
	}
	key, item, e := h.settingService.CreateScopedAdminKey(c.Request.Context(), in, subject.UserID)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, gin.H{"key": key, "item": item})
}
func (h *SettingHandler) UpdateScopedAdminKey(c *gin.Context) {
	if !requireAdminKeyJWT(c) {
		return
	}
	var in service.ScopedAdminKeyInput
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Error(c, 400, "请求格式无效")
		return
	}
	item, e := h.settingService.UpdateScopedAdminKey(c.Request.Context(), c.Param("id"), in)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, item)
}
func (h *SettingHandler) RotateScopedAdminKey(c *gin.Context) {
	if !requireAdminKeyJWT(c) {
		return
	}
	key, item, e := h.settingService.RotateScopedAdminKey(c.Request.Context(), c.Param("id"))
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, gin.H{"key": key, "item": item})
}
func (h *SettingHandler) RevokeScopedAdminKey(c *gin.Context) {
	if !requireAdminKeyJWT(c) {
		return
	}
	if e := h.settingService.RevokeScopedAdminKey(c.Request.Context(), c.Param("id")); e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, gin.H{"message": "管理密钥已永久撤销"})
}

func (h *SettingHandler) DeleteScopedAdminKeyRecord(c *gin.Context) {
	if !requireAdminKeyJWT(c) {
		return
	}
	if e := h.settingService.DeleteScopedAdminKeyRecord(c.Request.Context(), c.Param("id")); e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, gin.H{"message": "密钥已从列表移除，历史操作记录仍会继续保留"})
}
