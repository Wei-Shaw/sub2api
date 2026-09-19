package admin

import (
	"context"
	"net/http"
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type codexAccountTicketManager interface {
	GetCodexAccountTicketStatus(context.Context, int64) (*service.CodexAccountTicketStatus, error)
	ConfigureCodexAccountTicket(context.Context, int64, service.CodexAccountTicketUpdate) (*service.CodexAccountTicketStatus, error)
	HarvestCodexAccountTicket(context.Context, int64) (*service.CodexAccountTicketStatus, error)
}

func (h *AccountHandler) SetCodexAccountTicketService(s *service.OpenAIGatewayService) {
	h.codexAccountTickets = s
}

func (h *AccountHandler) codexTicketAccountID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, false
	}
	if h.codexAccountTickets == nil {
		response.Error(c, http.StatusServiceUnavailable, "Account STATE ticket service unavailable")
		return 0, false
	}
	return id, true
}

// Only explicit, public application errors may be returned; never echo a proxy
// URL from a JSON binding, database, or transport error into the admin response.
func codexTicketControlError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if infraerrors.Code(err) >= http.StatusInternalServerError {
		response.InternalError(c, "Account STATE ticket operation failed")
	} else {
		response.ErrorFrom(c, err)
	}
	return true
}

func (h *AccountHandler) GetCodexAccountTicket(c *gin.Context) {
	id, ok := h.codexTicketAccountID(c)
	if !ok {
		return
	}
	status, err := h.codexAccountTickets.GetCodexAccountTicketStatus(c.Request.Context(), id)
	if !codexTicketControlError(c, err) {
		response.Success(c, status)
	}
}

func (h *AccountHandler) UpdateCodexAccountTicket(c *gin.Context) {
	id, ok := h.codexTicketAccountID(c)
	if !ok {
		return
	}
	var req struct {
		TicketPlan string `json:"ticket_plan"`
		Enabled    *bool  `json:"enabled"`
		ProxyURL   string `json:"proxy_url"`
		Model      string `json:"model"`
		ClearProxy bool   `json:"clear_proxy"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		response.BadRequest(c, "Invalid STATE settings; enabled is required")
		return
	}
	status, err := h.codexAccountTickets.ConfigureCodexAccountTicket(c.Request.Context(), id, service.CodexAccountTicketUpdate{
		TicketPlan: req.TicketPlan, Enabled: *req.Enabled, ProxyURL: req.ProxyURL, Model: req.Model, ClearProxy: req.ClearProxy,
	})
	if !codexTicketControlError(c, err) {
		response.Success(c, status)
	}
}

func (h *AccountHandler) HarvestCodexAccountTicket(c *gin.Context) {
	id, ok := h.codexTicketAccountID(c)
	if !ok {
		return
	}
	status, err := h.codexAccountTickets.HarvestCodexAccountTicket(c.Request.Context(), id)
	if !codexTicketControlError(c, err) {
		response.Accepted(c, status)
	}
}
