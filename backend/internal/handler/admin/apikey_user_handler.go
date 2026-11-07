package admin

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type adminKeyService interface {
	Create(context.Context, int64, service.CreateAPIKeyRequest) (*service.APIKey, error)
	GetByID(context.Context, int64) (*service.APIKey, error)
	Update(context.Context, int64, int64, service.UpdateAPIKeyRequest) (*service.APIKey, error)
	Delete(context.Context, int64, int64) error
}

func adminKeyUserID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return 0, false
	}
	return id, true
}

func (h *AdminAPIKeyHandler) ownedKey(c *gin.Context, userID int64) (*service.APIKey, bool) {
	id, err := strconv.ParseInt(c.Param("key_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid API key ID")
		return nil, false
	}
	key, err := h.keyService.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, false
	}
	if key.UserID != userID {
		response.NotFound(c, "API key not found")
		return nil, false
	}
	return key, true
}

func (h *AdminAPIKeyHandler) CreateForUser(c *gin.Context) {
	userID, ok := adminKeyUserID(c)
	if !ok {
		return
	}
	var req service.CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		response.BadRequest(c, "name is required")
		return
	}
	if c.GetHeader("Idempotency-Key") == "" {
		response.ErrorFrom(c, service.ErrIdempotencyKeyRequired)
		return
	}
	if service.DefaultIdempotencyCoordinator() == nil {
		response.ErrorFrom(c, service.ErrIdempotencyStoreUnavail)
		return
	}
	payload := struct {
		UserID  int64                       `json:"user_id"`
		Request service.CreateAPIKeyRequest `json:"request"`
	}{userID, req}
	executeAdminIdempotentJSON(c, "admin.user.api_keys.create", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		key, err := h.keyService.Create(ctx, userID, req)
		if err != nil {
			return nil, err
		}
		return dto.APIKeyFromService(key).Localized(c.GetHeader("Accept-Language")), nil
	})
}

func (h *AdminAPIKeyHandler) GetForUser(c *gin.Context) {
	userID, ok := adminKeyUserID(c)
	if !ok {
		return
	}
	key, ok := h.ownedKey(c, userID)
	if !ok {
		return
	}
	response.Success(c, dto.APIKeyFromService(key).Localized(c.GetHeader("Accept-Language")))
}

func (h *AdminAPIKeyHandler) UpdateForUser(c *gin.Context) {
	userID, ok := adminKeyUserID(c)
	if !ok {
		return
	}
	// The outer field accepts an empty string to clear expiry, as on the user API.
	var req struct {
		service.UpdateAPIKeyRequest
		ExpiresAt *string `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Status != nil && *req.Status != service.StatusActive && *req.Status != "inactive" {
		response.BadRequest(c, "Invalid status, use active or inactive")
		return
	}
	if req.ExpiresAt != nil {
		if *req.ExpiresAt == "" {
			req.ClearExpiration = true
		} else {
			expiry, err := time.Parse(time.RFC3339, *req.ExpiresAt)
			if err != nil {
				response.BadRequest(c, "Invalid expires_at, use RFC3339 or an empty string")
				return
			}
			req.UpdateAPIKeyRequest.ExpiresAt = &expiry
		}
	}
	key, ok := h.ownedKey(c, userID)
	if !ok {
		return
	}
	updated, err := h.keyService.Update(c.Request.Context(), key.ID, userID, req.UpdateAPIKeyRequest)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.APIKeyFromService(updated).Localized(c.GetHeader("Accept-Language")))
}

func (h *AdminAPIKeyHandler) DeleteForUser(c *gin.Context) {
	userID, ok := adminKeyUserID(c)
	if !ok {
		return
	}
	key, ok := h.ownedKey(c, userID)
	if !ok {
		return
	}
	if err := h.keyService.Delete(c.Request.Context(), key.ID, userID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "API key deleted successfully"})
}
