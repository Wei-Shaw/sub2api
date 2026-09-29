package admin

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"
)

type claudeResetReader interface {
	Query(context.Context, int64) (*service.ClaudeResetCredits, error)
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
