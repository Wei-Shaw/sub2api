package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type keyGroupInfoResponse struct {
	Object        string `json:"object"`
	SchemaVersion int    `json:"schema_version"`
	Key           struct {
		ID      int64 `json:"id"`
		GroupID int64 `json:"group_id"`
	} `json:"key"`
	Group struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Platform string `json:"platform"`
		Status   string `json:"status"`
	} `json:"group"`
	Models   []service.KeyGroupModel `json:"models"`
	Revision string                  `json:"revision"`
}

// KeyGroupInfo returns only the authenticated key's current group and requestable
// model aliases. Authentication for this endpoint reads live key/group state.
// GET /v1/sub2api/key-group
func (h *GatewayHandler) KeyGroupInfo(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if apiKey.GroupID == nil || apiKey.Group == nil || apiKey.Group.ID != *apiKey.GroupID {
		h.errorResponse(c, http.StatusForbidden, "permission_error", "API key is not assigned to a group")
		return
	}
	models, err := h.gatewayService.GetKeyGroupModels(c.Request.Context(), apiKey.Group)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Key group model configuration is unavailable")
		return
	}
	response := keyGroupInfoResponse{Object: "sub2api.key_group", SchemaVersion: 1, Models: models}
	response.Key.ID = apiKey.ID
	response.Key.GroupID = *apiKey.GroupID
	response.Group.ID = apiKey.Group.ID
	response.Group.Name = apiKey.Group.Name
	response.Group.Platform = apiKey.Group.Platform
	response.Group.Status = apiKey.Group.Status
	// Hash only safe response fields, not credentials or account records.
	encoded, _ := json.Marshal(response)
	revision := sha256.Sum256(encoded)
	response.Revision = hex.EncodeToString(revision[:])
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, response)
}
