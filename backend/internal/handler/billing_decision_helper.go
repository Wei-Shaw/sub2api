package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func resolveRequestBillingDecision(
	c *gin.Context,
	apiKey *service.APIKey,
	currentSubscription *service.UserSubscription,
	subscriptionService *service.SubscriptionService,
	billingCacheService *service.BillingCacheService,
) (*service.BillingDecision, error) {
	if subscriptionService == nil {
		if billingCacheService != nil {
			if err := billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, currentSubscription); err != nil {
				return nil, err
			}
		}
		decision := &service.BillingDecision{Subscription: currentSubscription}
		if currentSubscription != nil {
			decision.SourceType = service.BillingSourceTypeSubscription
			subscriptionID := currentSubscription.ID
			billingGroupID := currentSubscription.GroupID
			decision.SubscriptionID = &subscriptionID
			decision.BillingGroupID = &billingGroupID
		} else {
			decision.SourceType = service.BillingSourceTypeBalance
		}
		middleware2.BindBillingDecision(c, decision)
		return decision, nil
	}
	decision, err := subscriptionService.ResolveBillingDecision(c.Request.Context(), apiKey, service.ResolveBillingDecisionOptions{
		EnforceAPIKeyRateLimits: true,
		EnforceRPM:              true,
	})
	if err != nil {
		return nil, err
	}
	middleware2.BindBillingDecision(c, decision)
	return decision, nil
}
