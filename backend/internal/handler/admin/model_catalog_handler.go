package admin

import (
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ModelCatalogHandler 模型目录管理后台 handler。
type ModelCatalogHandler struct {
	catalogService *service.ModelCatalogService
}

// NewModelCatalogHandler 创建 handler。
func NewModelCatalogHandler(catalogService *service.ModelCatalogService) *ModelCatalogHandler {
	return &ModelCatalogHandler{catalogService: catalogService}
}

// --- DTO ---

// modelCatalogRequest 创建/更新请求。价格单位为 USD / token（与渠道定价一致，前端按 $/MTok 展示）。
type modelCatalogRequest struct {
	ModelID         string   `json:"model_id" binding:"required,max=200"`
	Platform        string   `json:"platform" binding:"required,oneof=anthropic openai gemini antigravity"`
	Vendor          string   `json:"vendor" binding:"max=50"`
	DisplayName     string   `json:"display_name" binding:"max=200"`
	Description     string   `json:"description" binding:"max=1000"`
	InputPrice      *float64 `json:"input_price"`
	OutputPrice     *float64 `json:"output_price"`
	CacheWritePrice *float64 `json:"cache_write_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	ContextWindow   int      `json:"context_window"`
	MaxOutputTokens int      `json:"max_output_tokens"`
	ReleasedAt      *string  `json:"released_at"` // YYYY-MM-DD 或 RFC3339，空表示未知
	Enabled         *bool    `json:"enabled"`
	SortOrder       int      `json:"sort_order"`
}

type modelCatalogResponse struct {
	ID              int64    `json:"id"`
	ModelID         string   `json:"model_id"`
	Platform        string   `json:"platform"`
	Vendor          string   `json:"vendor"`
	DisplayName     string   `json:"display_name"`
	Description     string   `json:"description"`
	InputPrice      *float64 `json:"input_price"`
	OutputPrice     *float64 `json:"output_price"`
	CacheWritePrice *float64 `json:"cache_write_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	ContextWindow   int      `json:"context_window"`
	MaxOutputTokens int      `json:"max_output_tokens"`
	ReleasedAt      *string  `json:"released_at"`
	Enabled         bool     `json:"enabled"`
	SortOrder       int      `json:"sort_order"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

func toModelCatalogResponse(e *service.ModelCatalogEntry) *modelCatalogResponse {
	if e == nil {
		return nil
	}
	resp := &modelCatalogResponse{
		ID:              e.ID,
		ModelID:         e.ModelID,
		Platform:        e.Platform,
		Vendor:          e.Vendor,
		DisplayName:     e.DisplayName,
		Description:     e.Description,
		InputPrice:      e.InputPrice,
		OutputPrice:     e.OutputPrice,
		CacheWritePrice: e.CacheWritePrice,
		CacheReadPrice:  e.CacheReadPrice,
		ContextWindow:   e.ContextWindow,
		MaxOutputTokens: e.MaxOutputTokens,
		Enabled:         e.Enabled,
		SortOrder:       e.SortOrder,
		CreatedAt:       e.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:       e.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if e.ReleasedAt != nil {
		s := e.ReleasedAt.UTC().Format("2006-01-02")
		resp.ReleasedAt = &s
	}
	return resp
}

func parseModelCatalogReleasedAt(raw *string) (*time.Time, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	v := strings.TrimSpace(*raw)
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, v); err == nil {
			return &t, nil
		}
	}
	return nil, infraerrors.BadRequest("INVALID_RELEASED_AT", "released_at must be YYYY-MM-DD or RFC3339")
}

func (req *modelCatalogRequest) toEntry() (*service.ModelCatalogEntry, error) {
	releasedAt, err := parseModelCatalogReleasedAt(req.ReleasedAt)
	if err != nil {
		return nil, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return &service.ModelCatalogEntry{
		ModelID:         req.ModelID,
		Platform:        req.Platform,
		Vendor:          req.Vendor,
		DisplayName:     req.DisplayName,
		Description:     req.Description,
		InputPrice:      req.InputPrice,
		OutputPrice:     req.OutputPrice,
		CacheWritePrice: req.CacheWritePrice,
		CacheReadPrice:  req.CacheReadPrice,
		ContextWindow:   req.ContextWindow,
		MaxOutputTokens: req.MaxOutputTokens,
		ReleasedAt:      releasedAt,
		Enabled:         enabled,
		SortOrder:       req.SortOrder,
	}, nil
}

func parseModelCatalogID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_MODEL_CATALOG_ID", "invalid model catalog id"))
		return 0, false
	}
	return id, true
}

// --- Handlers ---

// List GET /api/v1/admin/model-catalog?platform=&vendor=&search=&enabled=
func (h *ModelCatalogHandler) List(c *gin.Context) {
	params := service.ModelCatalogListParams{
		Platform: strings.TrimSpace(c.Query("platform")),
		Vendor:   strings.TrimSpace(c.Query("vendor")),
		Search:   strings.TrimSpace(c.Query("search")),
	}
	if raw := strings.TrimSpace(c.Query("enabled")); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", "enabled must be true or false"))
			return
		}
		params.Enabled = &enabled
	}
	items, err := h.catalogService.List(c.Request.Context(), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]*modelCatalogResponse, 0, len(items))
	for _, e := range items {
		out = append(out, toModelCatalogResponse(e))
	}
	response.Success(c, gin.H{"items": out})
}

// GetByID GET /api/v1/admin/model-catalog/:id
func (h *ModelCatalogHandler) GetByID(c *gin.Context) {
	id, ok := parseModelCatalogID(c)
	if !ok {
		return
	}
	e, err := h.catalogService.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toModelCatalogResponse(e))
}

// Create POST /api/v1/admin/model-catalog
func (h *ModelCatalogHandler) Create(c *gin.Context) {
	var req modelCatalogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	entry, err := req.toEntry()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.catalogService.Create(c.Request.Context(), entry); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, toModelCatalogResponse(entry))
}

// Update PUT /api/v1/admin/model-catalog/:id
func (h *ModelCatalogHandler) Update(c *gin.Context) {
	id, ok := parseModelCatalogID(c)
	if !ok {
		return
	}
	var req modelCatalogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	entry, err := req.toEntry()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	entry.ID = id
	if err := h.catalogService.Update(c.Request.Context(), entry); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toModelCatalogResponse(entry))
}

// Delete DELETE /api/v1/admin/model-catalog/:id
func (h *ModelCatalogHandler) Delete(c *gin.Context) {
	id, ok := parseModelCatalogID(c)
	if !ok {
		return
	}
	if err := h.catalogService.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}
