package handler

import (
	"fmt"
	"net/http"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// apiUsageResponse is the balance-query payload for POST/GET /v1/user/balance and /user/balance.
// Compatible with custom client extractors:
//
//	isValid  <- !response.error
//	remaining <- response.balance
//	unit     <- "USD" (also returned explicitly)
type apiUsageResponse struct {
	Balance        float64  `json:"balance"`
	Remaining      float64  `json:"remaining"`
	Unit           string   `json:"unit"`
	IsValid        bool     `json:"isValid"`
	PlanName       string   `json:"planName,omitempty"`
	Total          *float64 `json:"total,omitempty"`
	Used           *float64 `json:"used,omitempty"`
	Extra          string   `json:"extra,omitempty"`
	Error          string   `json:"error,omitempty"`
	InvalidMessage string   `json:"invalidMessage,omitempty"`
	Status         string   `json:"status,omitempty"`
	Mode           string   `json:"mode,omitempty"`
}

// UsageBalance handles balance queries by API key for custom clients (e.g. cc-switch style).
// POST/GET /v1/user/balance  and  POST/GET /user/balance
//
// Auth: Authorization: Bearer <apiKey> (same as gateway middleware)
// Response focuses on wallet/quota remaining exposed as `balance`.
func (h *GatewayHandler) UsageBalance(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		writeAPIUsageError(c, http.StatusUnauthorized, "Invalid API key")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		writeAPIUsageError(c, http.StatusUnauthorized, "Invalid API key")
		return
	}

	ctx := c.Request.Context()
	isValid := apiKey.Status == service.StatusAPIKeyActive ||
		apiKey.Status == service.StatusAPIKeyQuotaExhausted ||
		apiKey.Status == service.StatusAPIKeyExpired

	// 1) Key-level quota takes precedence when configured.
	if apiKey.Quota > 0 {
		remaining := apiKey.GetQuotaRemaining()
		total := apiKey.Quota
		used := apiKey.QuotaUsed
		resp := apiUsageResponse{
			Balance:   remaining,
			Remaining: remaining,
			Unit:      "USD",
			IsValid:   isValid,
			PlanName:  apiKey.Name,
			Total:     &total,
			Used:      &used,
			Status:    apiKey.Status,
			Mode:      "quota_limited",
			Extra:     fmt.Sprintf("quota_limit=%.4f quota_used=%.4f", apiKey.Quota, apiKey.QuotaUsed),
		}
		if !isValid {
			resp.Error = "API key is not active"
			resp.InvalidMessage = resp.Error
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	// 2) Subscription group: remaining across configured windows.
	if apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
		resp := apiUsageResponse{
			Unit:     "USD",
			IsValid:  isValid,
			PlanName: apiKey.Group.Name,
			Status:   apiKey.Status,
			Mode:     "subscription",
		}
		if !isValid {
			resp.Error = "API key is not active"
			resp.InvalidMessage = resp.Error
			c.JSON(http.StatusOK, resp)
			return
		}

		subscription, ok := middleware2.GetSubscriptionFromContext(c)
		if !ok || subscription == nil {
			// No active subscription — still return a structured payload so
			// clients can show an invalid/zero remaining state via extractor.
			resp.Balance = 0
			resp.Remaining = 0
			resp.Error = "No active subscription"
			resp.InvalidMessage = resp.Error
			resp.IsValid = false
			c.JSON(http.StatusOK, resp)
			return
		}

		remaining := h.calculateSubscriptionRemaining(apiKey.Group, subscription)
		resp.Balance = remaining
		resp.Remaining = remaining
		resp.Extra = fmt.Sprintf(
			"daily=%.4f/%s weekly=%.4f/%s monthly=%.4f/%s",
			subscription.DailyUsageUSD, formatOptionalLimit(apiKey.Group.DailyLimitUSD),
			subscription.WeeklyUsageUSD, formatOptionalLimit(apiKey.Group.WeeklyLimitUSD),
			subscription.MonthlyUsageUSD, formatOptionalLimit(apiKey.Group.MonthlyLimitUSD),
		)
		if apiKey.Group.MonthlyLimitUSD != nil {
			total := *apiKey.Group.MonthlyLimitUSD
			used := subscription.MonthlyUsageUSD
			resp.Total = &total
			resp.Used = &used
		} else if apiKey.Group.WeeklyLimitUSD != nil {
			total := *apiKey.Group.WeeklyLimitUSD
			used := subscription.WeeklyUsageUSD
			resp.Total = &total
			resp.Used = &used
		} else if apiKey.Group.DailyLimitUSD != nil {
			total := *apiKey.Group.DailyLimitUSD
			used := subscription.DailyUsageUSD
			resp.Total = &total
			resp.Used = &used
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	// 3) Wallet balance mode (default).
	if h.userService == nil {
		writeAPIUsageError(c, http.StatusInternalServerError, "Failed to get user balance")
		return
	}
	latestUser, err := h.userService.GetByID(ctx, subject.UserID)
	if err != nil {
		writeAPIUsageError(c, http.StatusInternalServerError, "Failed to get user balance")
		return
	}

	planName := "钱包余额"
	if apiKey.Group != nil && apiKey.Group.Name != "" {
		planName = apiKey.Group.Name
	}

	resp := apiUsageResponse{
		Balance:   latestUser.Balance,
		Remaining: latestUser.Balance,
		Unit:      "USD",
		IsValid:   isValid,
		PlanName:  planName,
		Status:    apiKey.Status,
		Mode:      "wallet",
	}
	if !isValid {
		resp.Error = "API key is not active"
		resp.InvalidMessage = resp.Error
	}
	if latestUser.FrozenBalance > 0 {
		resp.Extra = fmt.Sprintf("frozen_balance=%.4f", latestUser.FrozenBalance)
	}
	c.JSON(http.StatusOK, resp)
}

func writeAPIUsageError(c *gin.Context, status int, message string) {
	c.JSON(status, apiUsageResponse{
		IsValid:        false,
		Unit:           "USD",
		Error:          message,
		InvalidMessage: message,
	})
}

func formatOptionalLimit(v *float64) string {
	if v == nil {
		return "unlimited"
	}
	return fmt.Sprintf("%.4f", *v)
}
