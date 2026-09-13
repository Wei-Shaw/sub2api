package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CursorOAuthHandler struct {
	cursorOAuthService *service.CursorOAuthService
	accountRepo        service.AccountRepository
}

func NewCursorOAuthHandler(cursorOAuthService *service.CursorOAuthService, accountRepo service.AccountRepository) *CursorOAuthHandler {
	return &CursorOAuthHandler{cursorOAuthService: cursorOAuthService, accountRepo: accountRepo}
}

type cursorGenerateAuthURLRequest struct {
	ProxyID *int64 `json:"proxy_id"`
}

func (h *CursorOAuthHandler) GenerateAuthURL(c *gin.Context) {
	var req cursorGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = cursorGenerateAuthURLRequest{}
	}
	result, err := h.cursorOAuthService.GenerateAuthURL(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type cursorPollRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
}

func (h *CursorOAuthHandler) Poll(c *gin.Context) {
	var req cursorPollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	tokenInfo, err := h.cursorOAuthService.Poll(c.Request.Context(), req.SessionID, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

type cursorRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
	ProxyID      *int64 `json:"proxy_id"`
}

func (h *CursorOAuthHandler) RefreshToken(c *gin.Context) {
	var req cursorRefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	tokenInfo, err := h.cursorOAuthService.RefreshToken(c.Request.Context(), strings.TrimSpace(req.RefreshToken), req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, tokenInfo)
}

func (h *CursorOAuthHandler) QueryQuota(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid account id")
		return
	}
	account, err := h.lookupAccount(c, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	usage, err := h.cursorOAuthService.QueryUsage(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, usage)
}

func (h *CursorOAuthHandler) lookupAccount(c *gin.Context, id int64) (*service.Account, error) {
	if h.accountRepo == nil {
		return nil, service.ErrAccountNotFound
	}
	account, err := h.accountRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}
	if account == nil || !account.IsCursor() {
		return nil, service.ErrAccountNotFound
	}
	return account, nil
}
