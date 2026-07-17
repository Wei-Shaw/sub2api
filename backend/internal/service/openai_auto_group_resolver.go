package service

import (
	"context"
	"fmt"
	"time"
)

func (s *OpenAIGatewayService) ResolveAutoGroup(ctx context.Context, apiKey *APIKey, requestedModel, sessionHash string) (*Group, error) {
	if apiKey == nil || apiKey.Group == nil || !apiKey.Group.IsAutoLowestCostRouting() {
		if apiKey == nil {
			return nil, nil
		}
		return apiKey.Group, nil
	}
	if apiKey.User == nil || s.schedulerSnapshot == nil {
		return nil, errorsNewAutoGroup("OpenAI resolver dependencies are missing")
	}

	stickyKey := autoGroupStickySessionKey(requestedModel, sessionHash)
	stickyGroupID := int64(0)
	if stickyKey != "" && s.cache != nil {
		if id, err := s.cache.GetSessionAccountID(ctx, apiKey.Group.ID, stickyKey); err == nil {
			stickyGroupID = id
		}
	}

	candidates := make([]AutoGroupCandidate, 0, len(apiKey.Group.AutoCandidateGroupIDs))
	userRates := make(map[int64]float64, len(apiKey.Group.AutoCandidateGroupIDs))
	for _, groupID := range apiKey.Group.AutoCandidateGroupIDs {
		candidate, err := s.schedulerSnapshot.GetGroupByID(ctx, groupID)
		if err != nil || candidate == nil || !candidate.IsActive() || candidate.SubscriptionType != SubscriptionTypeStandard || candidate.IsAutoLowestCostRouting() {
			continue
		}
		if candidate.Platform != apiKey.Group.Platform {
			continue
		}
		if candidate.Platform != PlatformOpenAI && candidate.Platform != PlatformGrok {
			continue
		}
		if !apiKey.User.CanBindGroup(candidate.ID, candidate.IsExclusive) {
			continue
		}
		available, err := s.openAIAutoGroupHasAvailableAccount(ctx, candidate, requestedModel)
		if err != nil {
			return nil, fmt.Errorf("check OpenAI auto candidate group %d: %w", groupID, err)
		}
		candidates = append(candidates, AutoGroupCandidate{Group: candidate, Available: available})
		if s.userGroupRateResolver != nil {
			userRates[candidate.ID] = s.userGroupRateResolver.Resolve(ctx, apiKey.UserID, candidate.ID, candidate.RateMultiplier)
		}
	}

	ranked := rankAutoGroupCandidates(candidates, userRates, time.Now(), stickyGroupID)
	if len(ranked) == 0 {
		return nil, ErrNoAvailableAccounts
	}
	selected := ranked[0].Group
	if stickyKey != "" && s.cache != nil {
		_ = s.cache.SetSessionAccountID(ctx, apiKey.Group.ID, stickyKey, selected.ID, autoGroupStickyTTL)
	}
	return selected, nil
}

func (s *OpenAIGatewayService) openAIAutoGroupHasAvailableAccount(ctx context.Context, group *Group, requestedModel string) (bool, error) {
	groupID := group.ID
	mappedModel := requestedModel
	if s.channelService != nil {
		mappedModel = s.channelService.ResolveChannelMapping(ctx, groupID, requestedModel).MappedModel
	}
	accounts, err := s.listSchedulableAccounts(ctx, &groupID, group.Platform)
	if err != nil {
		return false, err
	}
	for i := range accounts {
		account := &accounts[i]
		if !account.IsSchedulable() || normalizeOpenAICompatiblePlatform(account.Platform) != normalizeOpenAICompatiblePlatform(group.Platform) {
			continue
		}
		if mappedModel != "" && !account.IsModelSupported(mappedModel) {
			continue
		}
		return true, nil
	}
	return false, nil
}
