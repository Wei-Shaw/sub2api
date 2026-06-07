package handler

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type jobsService interface {
	CreateJob(ctx context.Context, input service.CreateJobInput) (service.Job, error)
	GetJob(ctx context.Context, jobID string) (service.Job, error)
}

type JobsHandler struct {
	jobsService jobsService
}

type CreateJobRequest struct {
	Capability     string            `json:"capability" binding:"required"`
	Input          map[string]any    `json:"input"`
	Metadata       map[string]string `json:"metadata"`
	PreferExecutor string            `json:"prefer_executor"`
}

func NewJobsHandler(jobsService jobsService) *JobsHandler {
	return &JobsHandler{jobsService: jobsService}
}

func (h *JobsHandler) Create(c *gin.Context) {
	var req CreateJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	job, err := h.jobsService.CreateJob(c.Request.Context(), service.CreateJobInput{
		Capability:     req.Capability,
		Input:          req.Input,
		Metadata:       req.Metadata,
		PreferExecutor: req.PreferExecutor,
		ExecutionToken: extractJobExecutionToken(c),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Accepted(c, job)
}

func (h *JobsHandler) Get(c *gin.Context) {
	jobID := strings.TrimSpace(c.Param("job_id"))
	job, err := h.jobsService.GetJob(c.Request.Context(), jobID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, job)
}

func extractJobExecutionToken(c *gin.Context) string {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}
	for _, key := range []string{"x-api-key", "x-goog-api-key"} {
		if token := strings.TrimSpace(c.GetHeader(key)); token != "" {
			return token
		}
	}
	if apiKey, ok := middleware.GetAPIKeyFromContext(c); ok && apiKey != nil {
		return strings.TrimSpace(apiKey.Key)
	}
	return ""
}
