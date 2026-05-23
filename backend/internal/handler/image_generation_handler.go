package handler

import (
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ImageGenerationHandler exposes user-facing helpers for the image workbench.
type ImageGenerationHandler struct {
	apiKeyService  *service.APIKeyService
	gatewayService *service.GatewayService
}

// NewImageGenerationHandler creates an ImageGenerationHandler.
func NewImageGenerationHandler(apiKeyService *service.APIKeyService, gatewayService *service.GatewayService) *ImageGenerationHandler {
	return &ImageGenerationHandler{
		apiKeyService:  apiKeyService,
		gatewayService: gatewayService,
	}
}

type imageGenerationModelDTO struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	SupportsGeneration bool   `json:"supports_generation"`
	SupportsEdit       bool   `json:"supports_edit"`
}

// Models returns image-generation models available to the selected API key.
// GET /api/v1/images/models?api_key_id=123
func (h *ImageGenerationHandler) Models(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	keyID, err := strconv.ParseInt(strings.TrimSpace(c.Query("api_key_id")), 10, 64)
	if err != nil || keyID <= 0 {
		response.BadRequest(c, "Invalid API key ID")
		return
	}

	apiKey, err := h.apiKeyService.GetByID(c.Request.Context(), keyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if apiKey.UserID != subject.UserID {
		response.Forbidden(c, "Not authorized to access this key")
		return
	}
	if apiKey.Status != service.StatusActive {
		response.BadRequest(c, "API key is not active")
		return
	}
	if !service.GroupAllowsImageGeneration(apiKey.Group) {
		response.Forbidden(c, service.ImageGenerationPermissionMessage())
		return
	}

	hasAccounts := h.gatewayService.HasSchedulableAccountsForGroupPlatform(c.Request.Context(), apiKey.GroupID, service.PlatformOpenAI)
	if !hasAccounts {
		response.Success(c, []imageGenerationModelDTO{})
		return
	}

	models := h.gatewayService.GetAvailableModels(c.Request.Context(), apiKey.GroupID, service.PlatformOpenAI)
	imageModels := filterImageGenerationModels(models)
	if len(imageModels) == 0 {
		imageModels = defaultImageGenerationModels()
	}

	out := make([]imageGenerationModelDTO, 0, len(imageModels))
	for _, model := range imageModels {
		out = append(out, imageGenerationModelDTO{
			ID:                 model,
			Name:               displayImageModelName(model),
			SupportsGeneration: true,
			SupportsEdit:       true,
		})
	}

	response.Success(c, out)
}

func filterImageGenerationModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, model := range models {
		normalized := strings.TrimSpace(model)
		if normalized == "" || !isImageGenerationModelName(normalized) {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func defaultImageGenerationModels() []string {
	out := make([]string, 0, 3)
	for _, model := range openai.DefaultModels {
		if isImageGenerationModelName(model.ID) {
			out = append(out, model.ID)
		}
	}
	sort.Strings(out)
	return out
}

func isImageGenerationModelName(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(normalized, "gpt-image-")
}

func displayImageModelName(model string) string {
	for _, item := range openai.DefaultModels {
		if item.ID == model && strings.TrimSpace(item.DisplayName) != "" {
			return item.DisplayName
		}
	}
	return model
}
