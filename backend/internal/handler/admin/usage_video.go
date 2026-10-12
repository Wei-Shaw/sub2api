package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *UsageHandler) videoLog(c *gin.Context) *service.UsageLog {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 || c.Param("index") != "0" {
		response.NotFound(c, "video preview not found")
		return nil
	}
	log, err := h.usageService.GetByID(c.Request.Context(), id)
	if err != nil || len(service.ListAdminVideoPreviewURLs(log, time.Now())) == 0 {
		response.NotFound(c, "video preview unavailable or older than 7 days")
		return nil
	}
	return log
}

// Video prepares native <video> playback without exposing upstream credentials.
// This endpoint itself requires the normal Authorization header.
func (h *UsageHandler) Video(c *gin.Context) {
	log := h.videoLog(c)
	if log == nil {
		return
	}
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		response.BadRequest(c, "Sign in as an administrator to preview videos")
		return
	}
	path := fmt.Sprintf("/api/v1/admin/usage/%d/videos/0/content", log.ID)
	http.SetCookie(c.Writer, &http.Cookie{
		Name: middleware.AdminVideoPreviewCookie, Value: parts[1], Path: path,
		MaxAge: 300, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	c.Header("Cache-Control", "private, no-store")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(path))
}

// VideoContent reads existing media only: no generation, polling or billing writes.
func (h *UsageHandler) VideoContent(c *gin.Context) {
	log := h.videoLog(c)
	if log == nil {
		return
	}
	if h.videoGateway != nil && h.videoGateway.ServeCachedAdminVideoPreview(c, log) {
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), log.AccountID)
	if err != nil || account == nil || h.videoGateway == nil {
		response.NotFound(c, "video account is unavailable")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if err := h.videoGateway.StreamAdminVideoPreview(c, account, log.RequestID); err != nil && !c.Writer.Written() {
		// Do not expose provider error bodies, URLs or credentials to the browser.
		if errors.Is(err, service.ErrAdminVideoPreviewUnavailable) {
			response.Error(c, http.StatusGone, "Video is no longer available from the upstream provider")
			return
		}
		response.Error(c, http.StatusBadGateway, "Video is unavailable from the upstream provider")
	}
}
