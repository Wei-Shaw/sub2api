package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type grokUsageResetRequest struct {
	SSOToken string `json:"sso_token" binding:"required,max=16384"`
	TokenID  string `json:"token_id" binding:"max=1024"`
}

func (h *GrokOAuthHandler) QueryUsageResetCards(c *gin.Context) { h.usageResetCards(c, false) }
func (h *GrokOAuthHandler) RedeemUsageResetCard(c *gin.Context) { h.usageResetCards(c, true) }

func (h *GrokOAuthHandler) usageResetCards(c *gin.Context, redeem bool) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h.quotaService == nil {
		response.BadRequest(c, "Grok quota service is unavailable")
		return
	}
	var input grokUsageResetRequest
	if c.ShouldBindJSON(&input) != nil || (redeem && input.TokenID == "") {
		// Never include validation errors or request data: SSO is a credential.
		response.BadRequest(c, "A Web SSO session and, for redemption, a reset card ID are required")
		return
	}
	if redeem {
		result, err := h.quotaService.RedeemUsageResetCard(c.Request.Context(), accountID, input.SSOToken, input.TokenID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		response.Success(c, result)
		return
	}
	result, err := h.quotaService.QueryUsageResetCards(c.Request.Context(), accountID, input.SSOToken)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
