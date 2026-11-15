package admin

import (
	"context"
	"errors"
	"io"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type claudeResetReader interface {
	Query(context.Context, int64) (*service.ClaudeResetCredits, error)
	Redeem(context.Context, int64, string, string) (*service.ClaudeResetOutcome, error)
}

func (h *AccountHandler) SetClaudeResetCreditService(s *service.ClaudeResetCreditService) {
	h.claudeResetCredits = s
}
func (h *AccountHandler) ClaudeResetCredits(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h.claudeResetCredits == nil {
		response.Error(c, 503, "Claude reset service unavailable")
		return
	}
	status, err := h.claudeResetCredits.Query(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

// RedeemClaudeResetCredit consumes one reset credit. The optional body
// {"kind":"weekly"|"fable"|"session"|"full"} names which reset; without it the
// upstream next credit is used. The server picks the grant; the Idempotency-Key
// header identifies one operator confirmation and replays its outcome.
func (h *AccountHandler) RedeemClaudeResetCredit(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h.claudeResetCredits == nil {
		response.Error(c, 503, "Claude reset service unavailable")
		return
	}
	var body struct {
		Kind string `json:"kind"`
	}
	if c.Request.ContentLength != 0 && c.Request.Body != nil {
		if err = c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
			response.BadRequest(c, "Invalid request body")
			return
		}
	}
	result, err := h.claudeResetCredits.Redeem(c.Request.Context(), id, c.GetHeader("Idempotency-Key"), body.Kind)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
