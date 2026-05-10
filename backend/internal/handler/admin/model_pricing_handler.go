package admin

import (
	"log"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ModelPricingHandler struct {
	service *service.ModelPricingAdminService
}

func NewModelPricingHandler(service *service.ModelPricingAdminService) *ModelPricingHandler {
	return &ModelPricingHandler{service: service}
}

func (h *ModelPricingHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	search := c.Query("search")
	source := c.Query("source")

	result, err := h.service.List(c.Request.Context(), service.ModelPricingListParams{
		Page:     page,
		PageSize: pageSize,
		Search:   search,
		Source:   source,
	})
	if err != nil {
		log.Printf("[ModelPricing] List failed: %v", err)
		response.InternalError(c, "Failed to list model pricing")
		return
	}
	response.Success(c, result)
}

func (h *ModelPricingHandler) Create(c *gin.Context) {
	var entry service.ModelPricingEntry
	if err := c.ShouldBindJSON(&entry); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	result, err := h.service.Create(c.Request.Context(), entry)
	if err != nil {
		log.Printf("[ModelPricing] Create failed: %v", err)
		response.InternalError(c, "Failed to create model pricing")
		return
	}
	response.Success(c, result)
}

func (h *ModelPricingHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid ID")
		return
	}

	var entry service.ModelPricingEntry
	if err := c.ShouldBindJSON(&entry); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	result, err := h.service.Update(c.Request.Context(), id, entry)
	if err != nil {
		log.Printf("[ModelPricing] Update failed: %v", err)
		response.InternalError(c, "Failed to update model pricing")
		return
	}
	response.Success(c, result)
}

func (h *ModelPricingHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid ID")
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		log.Printf("[ModelPricing] Delete failed: %v", err)
		response.InternalError(c, "Failed to delete model pricing")
		return
	}
	response.Success(c, nil)
}

func (h *ModelPricingHandler) BulkDelete(c *gin.Context) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	if err := h.service.BulkDelete(c.Request.Context(), req.IDs); err != nil {
		log.Printf("[ModelPricing] BulkDelete failed: %v", err)
		response.InternalError(c, "Failed to delete model pricing entries")
		return
	}
	response.Success(c, nil)
}

func (h *ModelPricingHandler) SyncFromRemote(c *gin.Context) {
	if err := h.service.SyncFromRemote(c.Request.Context()); err != nil {
		log.Printf("[ModelPricing] SyncFromRemote failed: %v", err)
		response.InternalError(c, "Failed to sync from remote: "+err.Error())
		return
	}

	status, _ := h.service.GetSyncStatus(c.Request.Context())
	response.Success(c, status)
}

func (h *ModelPricingHandler) GetSyncStatus(c *gin.Context) {
	status, err := h.service.GetSyncStatus(c.Request.Context())
	if err != nil {
		log.Printf("[ModelPricing] GetSyncStatus failed: %v", err)
		response.InternalError(c, "Failed to get sync status")
		return
	}
	response.Success(c, status)
}

func (h *ModelPricingHandler) SetAutoSync(c *gin.Context) {
	var req struct {
			Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	h.service.SetAutoSync(req.Enabled)
	response.Success(c, gin.H{"auto_sync_enabled": req.Enabled})
}

type PublicPricingResponse struct {
	Groups  []service.PublicPricingGroup `json:"groups"`
}

func (h *ModelPricingHandler) GetPublicPricing(c *gin.Context) {
	ctx := c.Request.Context()

	groups, err := h.service.GetGroupsWithModelsAndPricing(ctx)
	if err != nil {
		log.Printf("[ModelPricing] GetGroupsWithModelsAndPricing failed: %v", err)
		response.InternalError(c, "Failed to get pricing data")
		return
	}

	response.Success(c, PublicPricingResponse{
		Groups: groups,
	})
}
