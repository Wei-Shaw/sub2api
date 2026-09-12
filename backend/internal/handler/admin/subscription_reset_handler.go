package admin

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type subscriptionResetAdmin interface {
	GetPolicy(context.Context, int64) (*service.SubscriptionResetPolicy, error)
	SavePolicy(context.Context, *service.SubscriptionResetPolicy) (*service.SubscriptionResetPolicy, error)
	GetStatus(context.Context, int64, int) (*service.SubscriptionResetStatus, error)
}

func subscriptionResetGroupID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid group ID")
		return 0, false
	}
	return id, true
}

func (h *SubscriptionHandler) GetResetPolicy(c *gin.Context) {
	id, ok := subscriptionResetGroupID(c)
	if !ok {
		return
	}
	policy, err := h.resetObserver.GetPolicy(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, policy)
}

func (h *SubscriptionHandler) SaveResetPolicy(c *gin.Context) {
	id, ok := subscriptionResetGroupID(c)
	if !ok {
		return
	}
	var policy service.SubscriptionResetPolicy
	if err := c.ShouldBindJSON(&policy); err != nil {
		response.BadRequest(c, "Invalid reset policy")
		return
	}
	policy.GroupID = id
	saved, err := h.resetObserver.SavePolicy(c.Request.Context(), &policy)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, saved)
}

func (h *SubscriptionHandler) GetResetStatus(c *gin.Context) {
	id, ok := subscriptionResetGroupID(c)
	if !ok {
		return
	}
	limit := 20
	if value := c.Query("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			response.BadRequest(c, "limit must be 1..100")
			return
		}
		limit = parsed
	}
	status, err := h.resetObserver.GetStatus(c.Request.Context(), id, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}
