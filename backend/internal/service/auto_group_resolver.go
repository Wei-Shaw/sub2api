package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	GroupRoutingModeFixed          = "fixed"
	GroupRoutingModeAutoLowestCost = "auto_lowest_cost"
)

type AutoGroupCandidate struct {
	Group         *Group
	Available     bool
	EffectiveRate float64
}

func isEligibleAutoCandidate(candidate *Group) bool {
	return candidate != nil &&
		candidate.IsActive() &&
		candidate.SubscriptionType == SubscriptionTypeStandard &&
		!candidate.IsExclusive &&
		!candidate.IsAutoLowestCostRouting()
}

// ListAutoCandidateGroups returns the same public balance candidates used by
// runtime auto routing. It is also the source of truth for model discovery.
func (s *GatewayService) ListAutoCandidateGroups(ctx context.Context, autoGroup *Group) []*Group {
	if s == nil || s.groupRepo == nil || autoGroup == nil || !autoGroup.IsAutoLowestCostRouting() {
		return nil
	}
	groups := make([]*Group, 0, len(autoGroup.AutoCandidateGroupIDs))
	seen := make(map[int64]struct{}, len(autoGroup.AutoCandidateGroupIDs))
	for _, groupID := range autoGroup.AutoCandidateGroupIDs {
		if _, ok := seen[groupID]; ok {
			continue
		}
		seen[groupID] = struct{}{}
		candidate, err := s.groupRepo.GetByIDLite(ctx, groupID)
		if err != nil || !isEligibleAutoCandidate(candidate) {
			continue
		}
		groups = append(groups, candidate)
	}
	return groups
}

const autoGroupStickyTTL = time.Hour

func autoGroupStickySessionKey(requestedModel, sessionHash string) string {
	if strings.TrimSpace(sessionHash) == "" {
		return ""
	}
	return "auto-group:" + strings.TrimSpace(requestedModel) + ":" + sessionHash
}

// ResolveAutoGroup returns the request-local effective group. Fixed groups are
// returned unchanged. Auto groups are never used as an upstream scheduling
// bucket themselves.
func (s *GatewayService) ResolveAutoGroup(ctx context.Context, apiKey *APIKey, requestedModel, sessionHash string) (*Group, error) {
	if apiKey == nil || apiKey.Group == nil || !apiKey.Group.IsAutoLowestCostRouting() {
		if apiKey == nil {
			return nil, nil
		}
		return apiKey.Group, nil
	}
	if apiKey.User == nil {
		return nil, errorsNewAutoGroup("authenticated user is missing")
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
		candidate, err := s.groupRepo.GetByIDLite(ctx, groupID)
		if err != nil || !isEligibleAutoCandidate(candidate) {
			continue
		}
		available, err := s.autoGroupHasAvailableAccount(ctx, candidate, requestedModel)
		if err != nil {
			return nil, fmt.Errorf("check auto candidate group %d: %w", groupID, err)
		}
		candidates = append(candidates, AutoGroupCandidate{Group: candidate, Available: available})
		if s.userGroupRateResolver != nil {
			userRates[candidate.ID] = s.userGroupRateResolver.Resolve(ctx, apiKey.UserID, candidate.ID, candidate.RateMultiplier)
		}
	}

	ranked := rankAutoGroupCandidates(candidates, userRates, time.Now(), stickyGroupID)
	if len(ranked) == 0 {
		if stickyKey != "" && s.cache != nil {
			_ = s.cache.DeleteSessionAccountID(ctx, apiKey.Group.ID, stickyKey)
		}
		return nil, ErrNoAvailableAccounts
	}
	selected := ranked[0].Group
	if stickyKey != "" && s.cache != nil {
		_ = s.cache.SetSessionAccountID(ctx, apiKey.Group.ID, stickyKey, selected.ID, autoGroupStickyTTL)
	}
	return selected, nil
}

func errorsNewAutoGroup(message string) error {
	return fmt.Errorf("auto group resolution failed: %s", message)
}

func channelModelForAccountSelection(ctx context.Context, channelService *ChannelService, groupID *int64, requestedModel string) string {
	if channelService == nil || groupID == nil || strings.TrimSpace(requestedModel) == "" {
		return requestedModel
	}
	mapping := channelService.ResolveChannelMapping(ctx, *groupID, requestedModel)
	if mapping.Mapped && strings.TrimSpace(mapping.MappedModel) != "" {
		return mapping.MappedModel
	}
	return requestedModel
}

func (s *GatewayService) autoGroupHasAvailableAccount(ctx context.Context, group *Group, requestedModel string) (bool, error) {
	if group == nil {
		return false, nil
	}
	groupID := group.ID
	if s.checkChannelPricingRestriction(ctx, &groupID, requestedModel) {
		return false, nil
	}
	mappedModel := channelModelForAccountSelection(ctx, s.channelService, &groupID, requestedModel)
	accounts, useMixed, err := s.listSchedulableAccounts(ctx, &groupID, group.Platform, false)
	if err != nil {
		return false, err
	}
	for i := range accounts {
		account := &accounts[i]
		if !s.isAccountSchedulableForSelection(account) || !s.isAccountAllowedForPlatform(account, group.Platform, useMixed) {
			continue
		}
		if mappedModel != "" && !s.isModelSupportedByAccountWithContext(ctx, account, mappedModel) {
			continue
		}
		if !s.isAccountSchedulableForModelSelection(ctx, account, mappedModel) || !s.isAccountSchedulableForQuota(account) {
			continue
		}
		return true, nil
	}
	return false, nil
}

func rankAutoGroupCandidates(candidates []AutoGroupCandidate, userRates map[int64]float64, now time.Time, stickyGroupID int64) []AutoGroupCandidate {
	ranked := make([]AutoGroupCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		group := candidate.Group
		if group == nil || !candidate.Available || group.SubscriptionType != SubscriptionTypeStandard {
			continue
		}
		candidate.EffectiveRate = group.RateMultiplier
		if rate, ok := userRates[group.ID]; ok {
			candidate.EffectiveRate = rate
		}
		candidate.EffectiveRate *= group.PeakMultiplierAt(now)
		ranked = append(ranked, candidate)
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Group.ID == stickyGroupID {
			return ranked[j].Group.ID != stickyGroupID
		}
		if ranked[j].Group.ID == stickyGroupID {
			return false
		}
		if ranked[i].EffectiveRate != ranked[j].EffectiveRate {
			return ranked[i].EffectiveRate < ranked[j].EffectiveRate
		}
		if ranked[i].Group.SortOrder != ranked[j].Group.SortOrder {
			return ranked[i].Group.SortOrder < ranked[j].Group.SortOrder
		}
		return ranked[i].Group.ID < ranked[j].Group.ID
	})
	return ranked
}
