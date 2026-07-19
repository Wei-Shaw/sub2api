package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ModelCatalogHandler exposes the durable global model catalog to admins.
// Channel-level pricing remains a separate surface under /admin/channels.
type ModelCatalogHandler struct {
	service *service.ModelCatalogAdminService
}

func NewModelCatalogHandler(catalogService *service.ModelCatalogAdminService) *ModelCatalogHandler {
	return &ModelCatalogHandler{service: catalogService}
}

type updateCatalogModelStateRequest struct {
	State           service.CatalogOperatorState `json:"state" binding:"required,oneof=enabled disabled"`
	ExpectedVersion int64                        `json:"expected_version" binding:"required,min=1"`
	Reason          string                       `json:"reason" binding:"max=500"`
}

type updateCatalogModelStateBulkItem struct {
	ModelID         int64 `json:"model_id" binding:"required,min=1"`
	ExpectedVersion int64 `json:"expected_version" binding:"required,min=1"`
}

type updateCatalogModelStateBulkRequest struct {
	ExpectedEpoch      int64                             `json:"expected_epoch" binding:"required,min=1"`
	ExpectedRevisionID int64                             `json:"expected_revision_id" binding:"required,min=1"`
	State              service.CatalogOperatorState      `json:"state" binding:"required,oneof=enabled disabled"`
	Models             []updateCatalogModelStateBulkItem `json:"models" binding:"required,min=1,max=500,dive"`
	Reason             string                            `json:"reason" binding:"max=500"`
}

type updateCatalogModelPricingRequest struct {
	ExpectedEpoch          int64    `json:"expected_epoch" binding:"required,min=1"`
	ExpectedRevisionID     int64    `json:"expected_revision_id" binding:"required,min=1"`
	ExpectedSourceHash     string   `json:"expected_source_hash" binding:"required"`
	InputCostPerToken      *float64 `json:"input_cost_per_token" binding:"omitempty,min=0"`
	OutputCostPerToken     *float64 `json:"output_cost_per_token" binding:"omitempty,min=0"`
	CacheReadCostPerToken  *float64 `json:"cache_read_cost_per_token" binding:"omitempty,min=0"`
	CacheWriteCostPerToken *float64 `json:"cache_write_cost_per_token" binding:"omitempty,min=0"`
	Reason                 string   `json:"reason" binding:"max=500"`
}

// List handles GET /api/v1/admin/model-catalog.
func (h *ModelCatalogHandler) List(c *gin.Context) {
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "Model catalog service is unavailable")
		return
	}
	snapshot, err := h.service.List(c.Request.Context())
	if err != nil {
		writeModelCatalogError(c, err)
		return
	}
	response.Success(c, snapshot)
}

// Sync handles POST /api/v1/admin/model-catalog/sync.
func (h *ModelCatalogHandler) Sync(c *gin.Context) {
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "Model catalog service is unavailable")
		return
	}
	executeAdminIdempotentJSON(c, "admin.model_catalog.sync", map[string]any{"operation": "sync"}, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.service.Sync(ctx)
	})
}

// UpdateState handles PATCH /api/v1/admin/model-catalog/models/:id/state.
func (h *ModelCatalogHandler) UpdateState(c *gin.Context) {
	modelID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || modelID <= 0 {
		response.BadRequest(c, "Invalid catalog model ID")
		return
	}
	var req updateCatalogModelStateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid model state request: "+err.Error())
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	payload := map[string]any{"model_id": modelID, "state": req.State, "expected_version": req.ExpectedVersion, "reason": strings.TrimSpace(req.Reason)}
	executeAdminIdempotentJSON(c, "admin.model_catalog.state", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.service.UpdateOperatorState(ctx, service.CatalogOperatorStateUpdateRequest{
			ModelID:         modelID,
			ExpectedVersion: req.ExpectedVersion,
			State:           req.State,
			Reason:          strings.TrimSpace(req.Reason),
			ActorUserID:     subject.UserID,
			RequestID:       c.GetHeader("X-Request-ID"),
			CorrelationID:   c.GetHeader("X-Correlation-ID"),
		})
	})
}

// UpdateBulkState handles PATCH /api/v1/admin/model-catalog/models/bulk-state.
func (h *ModelCatalogHandler) UpdateBulkState(c *gin.Context) {
	var req updateCatalogModelStateBulkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid bulk model state request: "+err.Error())
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	updates := make([]service.CatalogOperatorStateMutation, 0, len(req.Models))
	for _, model := range req.Models {
		updates = append(updates, service.CatalogOperatorStateMutation{
			ModelID: model.ModelID, ExpectedVersion: model.ExpectedVersion, State: req.State,
		})
	}
	payload := map[string]any{
		"expected_epoch": req.ExpectedEpoch, "expected_revision_id": req.ExpectedRevisionID,
		"state": req.State, "models": updates, "reason": strings.TrimSpace(req.Reason),
	}
	executeAdminIdempotentJSON(c, "admin.model_catalog.bulk_state", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.service.UpdateOperatorStates(ctx, service.CatalogOperatorStateBulkUpdateRequest{
			ExpectedEpoch: req.ExpectedEpoch, ExpectedRevisionID: req.ExpectedRevisionID, Updates: updates,
			Reason: strings.TrimSpace(req.Reason), ActorUserID: subject.UserID,
			RequestID: c.GetHeader("X-Request-ID"), CorrelationID: c.GetHeader("X-Correlation-ID"),
		})
	})
}

// UpdatePricing handles PATCH /api/v1/admin/model-catalog/models/:id/pricing.
func (h *ModelCatalogHandler) UpdatePricing(c *gin.Context) {
	modelID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || modelID <= 0 {
		response.BadRequest(c, "Invalid catalog model ID")
		return
	}
	var req updateCatalogModelPricingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid model pricing request: "+err.Error())
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	payload := map[string]any{
		"model_id":                   modelID,
		"expected_epoch":             req.ExpectedEpoch,
		"expected_revision_id":       req.ExpectedRevisionID,
		"expected_source_hash":       req.ExpectedSourceHash,
		"input_cost_per_token":       req.InputCostPerToken,
		"output_cost_per_token":      req.OutputCostPerToken,
		"cache_read_cost_per_token":  req.CacheReadCostPerToken,
		"cache_write_cost_per_token": req.CacheWriteCostPerToken,
		"reason":                     strings.TrimSpace(req.Reason),
	}
	executeAdminIdempotentJSON(c, "admin.model_catalog.pricing", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.service.UpdatePricing(ctx, service.CatalogAdminPricingUpdateRequest{
			ModelID:                modelID,
			ExpectedEpoch:          req.ExpectedEpoch,
			ExpectedRevisionID:     req.ExpectedRevisionID,
			ExpectedSourceHash:     strings.TrimSpace(req.ExpectedSourceHash),
			InputCostPerToken:      req.InputCostPerToken,
			OutputCostPerToken:     req.OutputCostPerToken,
			CacheReadCostPerToken:  req.CacheReadCostPerToken,
			CacheWriteCostPerToken: req.CacheWriteCostPerToken,
			Reason:                 strings.TrimSpace(req.Reason),
			ActorUserID:            subject.UserID,
			RequestID:              c.GetHeader("X-Request-ID"),
			CorrelationID:          c.GetHeader("X-Correlation-ID"),
		})
	})
}

func writeModelCatalogError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrCatalogAdminRequest):
		response.BadRequest(c, "Invalid model catalog request")
	case errors.Is(err, service.ErrCatalogModelNotFound):
		response.NotFound(c, "Catalog model not found")
	case errors.Is(err, service.ErrCatalogOperatorConflict), errors.Is(err, service.ErrCatalogPublicationConflict):
		response.Error(c, http.StatusConflict, "Catalog changed. Reload the model list and try again")
	case errors.Is(err, service.ErrCatalogNoPublication):
		response.Error(c, http.StatusConflict, "Model catalog is not initialized. Sync it from the pricing source first")
	default:
		response.ErrorFrom(c, err)
	}
}
