package admin

import (
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	grokMediaEligibilityModeAuto     = "auto"
	grokMediaEligibilityModeEnabled  = "enabled"
	grokMediaEligibilityModeDisabled = "disabled"
)

type grokMediaEligibilityResponse struct {
	AccountID int64  `json:"account_id"`
	Mode      string `json:"mode"`
	Eligible  bool   `json:"eligible"`
	Reason    string `json:"reason"`
}

type updateGrokMediaEligibilityRequest struct {
	Mode string `json:"mode" binding:"required"`
}

// GetGrokMediaEligibility returns the current manual override and evaluated
// media-routing decision for a Grok OAuth account.
// GET /api/v1/admin/accounts/:id/grok-media-eligibility
func (h *AccountHandler) GetGrokMediaEligibility(c *gin.Context) {
	account, err := h.getGrokOAuthAccount(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, buildGrokMediaEligibilityResponse(account))
}

// UpdateGrokMediaEligibility updates only the manual media eligibility
// override. It deliberately uses UpdateAccountExtra so unrelated account
// extra fields are preserved.
// PUT /api/v1/admin/accounts/:id/grok-media-eligibility
func (h *AccountHandler) UpdateGrokMediaEligibility(c *gin.Context) {
	account, err := h.getGrokOAuthAccount(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	var req updateGrokMediaEligibilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	var value any
	switch req.Mode {
	case grokMediaEligibilityModeAuto:
		value = nil
	case grokMediaEligibilityModeEnabled:
		value = true
	case grokMediaEligibilityModeDisabled:
		value = false
	default:
		response.ErrorFrom(c, infraerrors.BadRequest(
			"GROK_MEDIA_ELIGIBILITY_INVALID_MODE",
			"mode must be one of auto, enabled, disabled",
		))
		return
	}

	updates := map[string]any{service.GrokMediaEligibleExtraKey: value}
	if err := service.ValidateGrokMediaEligibilityExtra(account.Platform, updates); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.adminService.UpdateAccountExtra(c.Request.Context(), account.ID, updates); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	updated, err := h.adminService.GetAccount(c.Request.Context(), account.ID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := ensureGrokOAuthAccount(updated); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, buildGrokMediaEligibilityResponse(updated))
}

func (h *AccountHandler) getGrokOAuthAccount(c *gin.Context) (*service.Account, error) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_ACCOUNT_ID", "Invalid account ID")
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		return nil, err
	}
	if err := ensureGrokOAuthAccount(account); err != nil {
		return nil, err
	}
	return account, nil
}

func ensureGrokOAuthAccount(account *service.Account) error {
	if account == nil {
		return infraerrors.NotFound("ACCOUNT_NOT_FOUND", "account not found")
	}
	if account.Platform != service.PlatformGrok || account.Type != service.AccountTypeOAuth {
		return infraerrors.BadRequest(
			"GROK_MEDIA_ELIGIBILITY_UNSUPPORTED_ACCOUNT",
			"media eligibility is only supported for Grok OAuth accounts",
		)
	}
	return nil
}

func buildGrokMediaEligibilityResponse(account *service.Account) grokMediaEligibilityResponse {
	eligible, reason := account.GrokMediaGenerationEligibility()
	return grokMediaEligibilityResponse{
		AccountID: account.ID,
		Mode:      grokMediaEligibilityModeFromExtra(account.Extra),
		Eligible:  eligible,
		Reason:    reason,
	}
}

func grokMediaEligibilityModeFromExtra(extra map[string]any) string {
	if extra != nil {
		if enabled, ok := extra[service.GrokMediaEligibleExtraKey].(bool); ok {
			if enabled {
				return grokMediaEligibilityModeEnabled
			}
			return grokMediaEligibilityModeDisabled
		}
	}
	return grokMediaEligibilityModeAuto
}
