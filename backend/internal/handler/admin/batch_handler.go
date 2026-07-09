package admin

import (
	"errors"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// BatchHandler 批次管理 API handler
type BatchHandler struct {
	batchRepo service.BatchRepository
}

func NewBatchHandler(batchRepo service.BatchRepository) *BatchHandler {
	return &BatchHandler{batchRepo: batchRepo}
}

// List GET /api/v1/admin/batches
func (h *BatchHandler) List(c *gin.Context) {
	batches, err := h.batchRepo.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if batches == nil {
		batches = []service.Batch{}
	}
	response.Success(c, batches)
}

// Create POST /api/v1/admin/batches
func (h *BatchHandler) Create(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Source      string `json:"source"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		response.BadRequest(c, "name is required")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	source := req.Source
	if source == "" {
		source = "manual"
	}
	batch := &service.Batch{
		Name:        req.Name,
		Description: req.Description,
		Source:      source,
	}
	if err := h.batchRepo.Create(c.Request.Context(), batch); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, batch)
}

// Update PUT /api/v1/admin/batches/:id
func (h *BatchHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid batch id")
		return
	}
	batch, err := h.batchRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if batch == nil {
		response.ErrorFrom(c, errors.New("batch not found"))
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Name != nil {
		newName := strings.TrimSpace(*req.Name)
		if newName == "" {
			response.BadRequest(c, "name cannot be empty")
			return
		}
		batch.Name = newName
	}
	if req.Description != nil {
		batch.Description = *req.Description
	}
	if err := h.batchRepo.Update(c.Request.Context(), batch); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, batch)
}

// Delete DELETE /api/v1/admin/batches/:id
func (h *BatchHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid batch id")
		return
	}
	if err := h.batchRepo.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}
