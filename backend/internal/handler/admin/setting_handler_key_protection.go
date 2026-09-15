package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/keyprotection"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) GetKeyProtectionConfig(c *gin.Context) {
	cfg, err := h.settingService.GetKeyProtectionConfig(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Key protection settings unavailable")
		return
	}
	response.Success(c, cfg)
}

func (h *SettingHandler) UpdateKeyProtectionConfig(c *gin.Context) {
	// A dedicated, bounded JSON document avoids accepting misspelled policy
	// fields and never includes submitted rule text in an error or audit log.
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	cfg := keyprotection.DefaultConfig()
	document := &cfg
	if err := decoder.Decode(&document); err != nil || document == nil {
		response.BadRequest(c, "Invalid key protection settings document")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		response.BadRequest(c, "Invalid key protection settings document")
		return
	}
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		response.BadRequest(c, "Invalid key protection policy: check rules and user/group IDs")
		return
	}
	if err := h.settingService.SetKeyProtectionConfig(c.Request.Context(), cfg); err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Unable to save key protection settings")
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	role, _ := middleware.GetUserRoleFromContext(c)
	slog.Info("key protection settings updated", "audit", true,
		"user_id", subject.UserID, "role", role, "enabled", cfg.Enabled)
	response.Success(c, cfg)
}
