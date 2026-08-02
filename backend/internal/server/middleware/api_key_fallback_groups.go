package middleware

import (
	"context"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func prepareAPIKeyRoutingState(
	ctx context.Context,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	apiKey *service.APIKey,
	skipBilling bool,
) (*service.APIKeyRoutingState, error) {
	if apiKey == nil || apiKey.OrganizationSubscriptionID != nil || len(apiKey.FallbackGroupIDs) == 0 {
		return nil, nil
	}
	candidates := apiKeyService.ResolveAPIKeyRoutingCandidates(ctx, apiKey)
	for index := range candidates {
		candidate := &candidates[index]
		if candidate.Unavailable != nil || candidate.Group == nil || skipBilling || !candidate.Group.IsSubscriptionType() {
			continue
		}
		if subscriptionService == nil {
			continue
		}
		subscription, err := subscriptionService.GetActiveSubscription(ctx, apiKey.UserID, candidate.Group.ID)
		if err != nil {
			if errors.Is(err, service.ErrSubscriptionNotFound) {
				candidate.Unavailable = err
				continue
			}
			return nil, err
		}
		needsMaintenance, validateErr := subscriptionService.ValidateAndCheckLimits(subscription, candidate.Group)
		if needsMaintenance {
			refreshed, maintenanceErr := subscriptionService.EnsureWindowMaintenance(ctx, subscription)
			if maintenanceErr != nil {
				return nil, maintenanceErr
			}
			subscription = refreshed
			_, validateErr = subscriptionService.ValidateAndCheckLimits(subscription, candidate.Group)
		}
		if validateErr != nil {
			candidate.Unavailable = validateErr
			continue
		}
		candidate.Subscription = subscription
	}
	state := service.NewAPIKeyRoutingState(apiKey, candidates)
	index, ok := state.FirstAvailable()
	if !ok {
		return nil, service.ErrNoAvailableAccounts
	}
	state.Activate(index)
	return state, nil
}
