package service

import (
	"context"
	"errors"
)

// APIKeyAccountSelectionResult couples an account selection with the API key
// copy whose GroupID/Group represents the request-resolved group.
type APIKeyAccountSelectionResult struct {
	Selection *AccountSelectionResult
	APIKey    *APIKey
}

func apiKeyCandidateGroups(apiKey *APIKey, preferredPlatform string) []Group {
	if apiKey == nil {
		return nil
	}
	byID := make(map[int64]Group, len(apiKey.Groups)+1)
	order := make([]int64, 0, len(apiKey.Groups)+1)
	add := func(group *Group) {
		if group == nil || group.ID <= 0 || (group.Status != "" && !group.IsActive()) {
			return
		}
		if preferredPlatform != "" && group.Platform != "" && group.Platform != preferredPlatform {
			return
		}
		if _, exists := byID[group.ID]; exists {
			return
		}
		byID[group.ID] = *group
		order = append(order, group.ID)
	}
	for i := range apiKey.Groups {
		add(&apiKey.Groups[i])
	}
	add(apiKey.Group)
	if len(apiKey.GroupIDs) > 0 {
		for _, id := range apiKey.GroupIDs {
			if _, exists := byID[id]; exists {
				continue
			}
			if apiKey.Group != nil && apiKey.Group.ID == id {
				add(apiKey.Group)
			}
		}
	}
	groups := make([]Group, 0, len(order))
	for _, id := range order {
		groups = append(groups, byID[id])
	}
	return groups
}

// APIKeyCandidateGroups returns active API-key groups in binding order, narrowed
// by platform when preferredPlatform is not empty. The returned slice is a copy.
func APIKeyCandidateGroups(apiKey *APIKey, preferredPlatform string) []Group {
	return apiKeyCandidateGroups(apiKey, preferredPlatform)
}

func APIKeyHasCandidateGroup(apiKey *APIKey, platform string) bool {
	return len(apiKeyCandidateGroups(apiKey, platform)) > 0
}

func APIKeyHasMultipleCandidateGroups(apiKey *APIKey, platform string) bool {
	return len(apiKeyCandidateGroups(apiKey, platform)) > 1
}

func APIKeyOnlyCandidateGroup(apiKey *APIKey, platform string) *Group {
	groups := apiKeyCandidateGroups(apiKey, platform)
	if len(groups) != 1 {
		return nil
	}
	return &groups[0]
}

func selectFallbackError(last error) error {
	if last != nil {
		return last
	}
	return ErrNoAvailableAccounts
}

func shouldTryNextAPIKeyGroup(err error) bool {
	return errors.Is(err, ErrNoAvailableAccounts) || errors.Is(err, ErrNoAvailableCompactAccounts)
}

func resolvedAPIKeySelection(apiKey *APIKey, selection *AccountSelectionResult, fallbackGroup *Group) *APIKeyAccountSelectionResult {
	if selection == nil {
		return &APIKeyAccountSelectionResult{Selection: selection, APIKey: apiKey}
	}
	group := selection.Group
	if group == nil {
		group = fallbackGroup
	}
	if group != nil && selection.Group == nil {
		groupCopy := *group
		groupID := groupCopy.ID
		selection.Group = &groupCopy
		selection.GroupID = &groupID
	}
	return &APIKeyAccountSelectionResult{
		Selection: selection,
		APIKey:    APIKeyWithResolvedGroup(apiKey, group),
	}
}

// SelectAccountWithLoadAwarenessForAPIKey tries the API key's bound groups in
// binding order and keeps the existing per-group scheduler semantics intact.
func (s *GatewayService) SelectAccountWithLoadAwarenessForAPIKey(
	ctx context.Context,
	apiKey *APIKey,
	preferredPlatform string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	metadataUserID string,
	sub2apiUserID int64,
) (*APIKeyAccountSelectionResult, error) {
	if apiKey == nil {
		return nil, ErrNoAvailableAccounts
	}
	groups := apiKeyCandidateGroups(apiKey, preferredPlatform)
	if len(groups) == 0 {
		selection, err := s.SelectAccountWithLoadAwareness(ctx, apiKey.GroupID, sessionHash, requestedModel, excludedIDs, metadataUserID, sub2apiUserID)
		if err != nil {
			return nil, err
		}
		return resolvedAPIKeySelection(apiKey, selection, apiKey.Group), nil
	}

	var lastErr error
	for i := range groups {
		group := &groups[i]
		groupID := group.ID
		groupCtx := withResolvedGroupContext(ctx, group)
		selection, err := s.SelectAccountWithLoadAwareness(groupCtx, &groupID, sessionHash, requestedModel, excludedIDs, metadataUserID, sub2apiUserID)
		if err == nil {
			return resolvedAPIKeySelection(apiKey, selection, group), nil
		}
		lastErr = err
		if !shouldTryNextAPIKeyGroup(err) {
			return nil, err
		}
	}
	return nil, selectFallbackError(lastErr)
}

// SelectAccountForModelForAPIKey tries bound groups without acquiring
// concurrency slots. It is intended for lightweight endpoints such as token
// counting that need the resolved group for billing and channel behavior.
func (s *GatewayService) SelectAccountForModelForAPIKey(
	ctx context.Context,
	apiKey *APIKey,
	preferredPlatform string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
) (*APIKeyAccountSelectionResult, error) {
	if apiKey == nil {
		return nil, ErrNoAvailableAccounts
	}
	groups := apiKeyCandidateGroups(apiKey, preferredPlatform)
	if len(groups) == 0 {
		account, err := s.SelectAccountForModelWithExclusions(ctx, apiKey.GroupID, sessionHash, requestedModel, excludedIDs)
		if err != nil {
			return nil, err
		}
		return resolvedAPIKeySelection(apiKey, &AccountSelectionResult{Account: account}, apiKey.Group), nil
	}

	var lastErr error
	for i := range groups {
		group := &groups[i]
		groupID := group.ID
		groupCtx := withResolvedGroupContext(ctx, group)
		account, err := s.SelectAccountForModelWithExclusions(groupCtx, &groupID, sessionHash, requestedModel, excludedIDs)
		if err == nil {
			return resolvedAPIKeySelection(apiKey, &AccountSelectionResult{Account: account}, group), nil
		}
		lastErr = err
		if !shouldTryNextAPIKeyGroup(err) {
			return nil, err
		}
	}
	return nil, selectFallbackError(lastErr)
}

// SelectOpenAIAccountWithSchedulerForAPIKey tries OpenAI-compatible bound
// groups and delegates priority/load/failover decisions to the existing
// per-group scheduler.
func (s *OpenAIGatewayService) SelectOpenAIAccountWithSchedulerForAPIKey(
	ctx context.Context,
	apiKey *APIKey,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredTransport OpenAIUpstreamTransport,
	requiredCapability OpenAIEndpointCapability,
	requireCompact bool,
	platform string,
) (*APIKeyAccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	if apiKey == nil {
		return nil, OpenAIAccountScheduleDecision{}, ErrNoAvailableAccounts
	}
	platform = normalizeOpenAICompatiblePlatform(platform)
	groups := apiKeyCandidateGroups(apiKey, platform)
	if len(groups) == 0 {
		selection, decision, err := s.SelectAccountWithSchedulerForCapability(ctx, apiKey.GroupID, previousResponseID, sessionHash, requestedModel, excludedIDs, requiredTransport, requiredCapability, requireCompact, platform)
		if err != nil {
			return nil, decision, err
		}
		return resolvedAPIKeySelection(apiKey, selection, apiKey.Group), decision, nil
	}

	var lastErr error
	var lastDecision OpenAIAccountScheduleDecision
	for i := range groups {
		group := &groups[i]
		groupID := group.ID
		groupCtx := withResolvedGroupContext(ctx, group)
		selection, decision, err := s.SelectAccountWithSchedulerForCapability(groupCtx, &groupID, previousResponseID, sessionHash, requestedModel, excludedIDs, requiredTransport, requiredCapability, requireCompact, platform)
		if err == nil {
			return resolvedAPIKeySelection(apiKey, selection, group), decision, nil
		}
		lastErr = err
		lastDecision = decision
		if !shouldTryNextAPIKeyGroup(err) {
			return nil, decision, err
		}
	}
	return nil, lastDecision, selectFallbackError(lastErr)
}

func (s *OpenAIGatewayService) SelectOpenAIAccountWithSchedulerForImagesAPIKey(
	ctx context.Context,
	apiKey *APIKey,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredCapability OpenAIImagesCapability,
) (*APIKeyAccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	if apiKey == nil {
		return nil, OpenAIAccountScheduleDecision{}, ErrNoAvailableAccounts
	}
	groups := apiKeyCandidateGroups(apiKey, PlatformOpenAI)
	if len(groups) == 0 {
		selection, decision, err := s.SelectAccountWithSchedulerForImages(ctx, apiKey.GroupID, sessionHash, requestedModel, excludedIDs, requiredCapability)
		if err != nil {
			return nil, decision, err
		}
		return resolvedAPIKeySelection(apiKey, selection, apiKey.Group), decision, nil
	}

	var lastErr error
	var lastDecision OpenAIAccountScheduleDecision
	for i := range groups {
		group := &groups[i]
		groupID := group.ID
		groupCtx := withResolvedGroupContext(ctx, group)
		selection, decision, err := s.SelectAccountWithSchedulerForImages(groupCtx, &groupID, sessionHash, requestedModel, excludedIDs, requiredCapability)
		if err == nil {
			return resolvedAPIKeySelection(apiKey, selection, group), decision, nil
		}
		lastErr = err
		lastDecision = decision
		if !shouldTryNextAPIKeyGroup(err) {
			return nil, decision, err
		}
	}
	return nil, lastDecision, selectFallbackError(lastErr)
}
