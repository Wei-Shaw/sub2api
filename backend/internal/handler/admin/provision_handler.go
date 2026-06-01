package admin

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type ProvisionHandler struct {
	provisionService *service.ProvisionService
}

func NewProvisionHandler(provisionService *service.ProvisionService) *ProvisionHandler {
	return &ProvisionHandler{provisionService: provisionService}
}

type ProvisionPlanRequest struct {
	Code          string  `json:"code" binding:"required"`
	Name          string  `json:"name" binding:"required"`
	GroupID       int64   `json:"group_id" binding:"required,gt=0"`
	Balance       float64 `json:"balance" binding:"required,gt=0"`
	Quota         float64 `json:"quota" binding:"omitempty,gte=0"`
	ExpiresInDays *int    `json:"expires_in_days" binding:"omitempty,gt=0"`
	RateLimit5h   float64 `json:"rate_limit_5h" binding:"omitempty,gte=0"`
	RateLimit1d   float64 `json:"rate_limit_1d" binding:"omitempty,gte=0"`
	RateLimit7d   float64 `json:"rate_limit_7d" binding:"omitempty,gte=0"`
	Concurrency   int     `json:"concurrency" binding:"omitempty,gte=0"`
	RPMLimit      int     `json:"rpm_limit" binding:"omitempty,gte=0"`
	Enabled       bool    `json:"enabled"`
}

type ProvisionAPIKeyRequest struct {
	OrderID       string `json:"order_id" binding:"required"`
	PlanCode      string `json:"plan_code" binding:"required"`
	CustomerLabel string `json:"customer_label"`
}

func (h *ProvisionHandler) ListPlans(c *gin.Context) {
	plans, err := h.provisionService.ListPlans(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, plans)
}

func (h *ProvisionHandler) CreatePlan(c *gin.Context) {
	var req ProvisionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	executeAdminIdempotentJSON(c, "admin.provision.plans.create", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.provisionService.CreatePlan(ctx, provisionPlanInput(req))
	})
}

func (h *ProvisionHandler) UpdatePlan(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid provision plan ID")
		return
	}
	var req ProvisionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	executeAdminIdempotentJSON(c, "admin.provision.plans.update", gin.H{"id": id, "request": req}, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.provisionService.UpdatePlan(ctx, id, provisionPlanInput(req))
	})
}

func (h *ProvisionHandler) DeletePlan(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid provision plan ID")
		return
	}
	if err := h.provisionService.DeletePlan(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Provision plan deleted successfully"})
}

func (h *ProvisionHandler) ProvisionAPIKey(c *gin.Context) {
	var req ProvisionAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	executeAdminIdempotentJSON(c, "admin.provision.api_keys.create", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.provisionService.ProvisionAPIKey(ctx, service.ProvisionAPIKeyInput{
			OrderID:       req.OrderID,
			PlanCode:      req.PlanCode,
			CustomerLabel: req.CustomerLabel,
		})
	})
}

func provisionPlanInput(req ProvisionPlanRequest) service.ProvisionPlanInput {
	return service.ProvisionPlanInput{
		Code:          req.Code,
		Name:          req.Name,
		GroupID:       req.GroupID,
		Balance:       req.Balance,
		Quota:         req.Quota,
		ExpiresInDays: req.ExpiresInDays,
		RateLimit5h:   req.RateLimit5h,
		RateLimit1d:   req.RateLimit1d,
		RateLimit7d:   req.RateLimit7d,
		Concurrency:   req.Concurrency,
		RPMLimit:      req.RPMLimit,
		Enabled:       req.Enabled,
	}
}
