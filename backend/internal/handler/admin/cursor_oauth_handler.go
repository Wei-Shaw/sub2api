package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// CursorOAuthHandler exposes Cursor's deep-control browser login to the
// admin UI: start generates the login URL, poll reports the token pair once
// the browser authentication completes.
type CursorOAuthHandler struct {
	cursorOAuthService *service.CursorOAuthService
}

func NewCursorOAuthHandler(cursorOAuthService *service.CursorOAuthService) *CursorOAuthHandler {
	return &CursorOAuthHandler{cursorOAuthService: cursorOAuthService}
}

func (h *CursorOAuthHandler) StartAuth(c *gin.Context) {
	result, err := h.cursorOAuthService.StartAuth(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type CursorOAuthPollRequest struct {
	UUID     string `json:"uuid" binding:"required"`
	Verifier string `json:"verifier" binding:"required"`
	ProxyID  *int64 `json:"proxy_id"`
}

func (h *CursorOAuthHandler) PollAuth(c *gin.Context) {
	var req CursorOAuthPollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.cursorOAuthService.PollAuth(c.Request.Context(), req.UUID, req.Verifier, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
