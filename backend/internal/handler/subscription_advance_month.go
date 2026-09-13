package handler

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func (h *SubscriptionHandler) PreviewAdvanceMonth(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}
	result, err := h.subscriptionService.PreviewAdvanceMonth(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *SubscriptionHandler) AdvanceMonth(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid subscription ID")
		return
	}
	var req struct {
		MonthlyWindowStart time.Time       `json:"monthly_window_start" binding:"required"`
		WeeklyWindowStart  json.RawMessage `json:"weekly_window_start" binding:"required"`
		ExpiresAt          time.Time       `json:"expires_at" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid confirmation; refresh the preview")
		return
	}
	// A missing weekly anchor is not the same as an explicitly confirmed null.
	// RawMessage accepts null while binding:"required" rejects an omitted field.
	var weeklyStart *time.Time
	if err := json.Unmarshal(req.WeeklyWindowStart, &weeklyStart); err != nil {
		response.BadRequest(c, "Invalid confirmation; refresh the preview")
		return
	}
	result, err := h.subscriptionService.AdvanceMonth(c.Request.Context(), subject.UserID, id, req.MonthlyWindowStart, weeklyStart, req.ExpiresAt)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
