package admin

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ModelHandler handles admin model listing requests.
type ModelHandler struct {
	sora2apiService *service.Sora2APIService
REDACTED

// NewModelHandler creates a new ModelHandler.
func NewModelHandler(sora2apiService *service.Sora2APIService) *ModelHandler {
	return &ModelHandler{
		sora2apiService: sora2apiService,
REDACTED
REDACTED

// List handles listing models for a specific platform
// GET /api/v1/admin/models?platform=sora
func (h *ModelHandler) List(c *gin.Context) {
	platform := strings.TrimSpace(strings.ToLower(c.Query("platform")))
	if platform == "" {
		response.BadRequest(c, "platform is required")
		return
REDACTED

	switch platform {
	case service.PlatformSora:
		if h.sora2apiService == nil || !h.sora2apiService.Enabled() {
			response.Error(c, http.StatusServiceUnavailable, "sora2api not configured")
			return
	REDACTED
		models, err := h.sora2apiService.ListModels(c.Request.Context())
		if err != nil {
			response.Error(c, http.StatusServiceUnavailable, "failed to fetch sora models")
			return
	REDACTED
		ids := make([]string, 0, len(models))
		for _, m := range models {
			if strings.TrimSpace(m.ID) != "" {
				ids = append(ids, m.ID)
		REDACTED
	REDACTED
		response.Success(c, ids)
	default:
		response.BadRequest(c, "unsupported platform")
REDACTED
REDACTED
