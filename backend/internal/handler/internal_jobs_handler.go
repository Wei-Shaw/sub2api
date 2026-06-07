package handler

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type internalJobsService interface {
	VerifyToken(token string) error
	Execute(ctx context.Context, input service.WorkerJobExecuteInput) (service.WorkerJobExecuteResult, error)
	Health(ctx context.Context) (service.WorkerHealthStatus, error)
}

type InternalJobsHandler struct {
	jobsService internalJobsService
}

func NewInternalJobsHandler(jobsService internalJobsService) *InternalJobsHandler {
	return &InternalJobsHandler{jobsService: jobsService}
}

func (h *InternalJobsHandler) Execute(c *gin.Context) {
	if !h.authorize(c) {
		return
	}

	var req service.WorkerJobExecuteInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.jobsService.Execute(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *InternalJobsHandler) Health(c *gin.Context) {
	if !h.authorize(c) {
		return
	}

	health, err := h.jobsService.Health(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, health)
}

func (h *InternalJobsHandler) authorize(c *gin.Context) bool {
	token := strings.TrimSpace(c.GetHeader(service.WorkerJobsTokenHeader))
	if err := h.jobsService.VerifyToken(token); err != nil {
		response.ErrorFrom(c, err)
		return false
	}
	return true
}
