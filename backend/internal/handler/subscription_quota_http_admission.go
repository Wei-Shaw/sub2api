package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

const subscriptionQuotaHTTPFrozenKey = "subscription_quota_http_frozen"

// freezeHTTPSubscriptionQuota runs after the final account-slot wait. Retries
// of the same logical HTTP request retain this admission rather than moving an
// in-flight request to a newly replenished quota bucket.
func (h *OpenAIGatewayHandler) freezeHTTPSubscriptionQuota(c *gin.Context) error {
	if frozen, _ := c.Get(subscriptionQuotaHTTPFrozenKey); frozen == true {
		return nil
	}
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || key == nil || key.User == nil || key.Group == nil || !key.Group.IsSubscriptionType() {
		return nil
	}
	subscription, ok := middleware.GetSubscriptionFromContext(c)
	if !ok || subscription == nil || h.billingCacheService == nil {
		return nil
	}
	if err := h.billingCacheService.RefreshSubscriptionQuotaAdmission(c.Request.Context(), key.User.ID, key.Group, subscription); err != nil {
		return err
	}
	c.Set(subscriptionQuotaHTTPFrozenKey, true)
	return nil
}
