package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

func (s *GatewayService) SelectFalAccountInGroup(ctx context.Context, groupID *int64, sessionHash, requestedModel string, excludedIDs map[int64]struct{}, api string) (*Account, error) {
	account, err := s.selectAccountForModelWithPlatform(ctx, groupID, sessionHash, requestedModel, excludedIDs, PlatformFal, api)
	if err != nil {
		return nil, err
	}
	return s.hydrateSelectedAccount(ctx, account)
}

func (s *GatewayService) SelectImageAccountMixed(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	imageCapability OpenAIImagesCapability,
	falAPI string,
	preferPlatform string,
) (*Account, error) {
	accounts, err := s.listSchedulableImageAccounts(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, ErrNoAvailableAccounts
	}

	ctx = s.withWindowCostPrefetch(ctx, accounts)
	ctx = s.withRPMPrefetch(ctx, accounts)
	eligible := func(account *Account) bool {
		if account == nil {
			return false
		}
		if _, excluded := excludedIDs[account.ID]; excluded {
			return false
		}
		if !s.isAccountSchedulableForSelection(account) {
			return false
		}
		if !s.imageAccountSupportsRequest(ctx, account, requestedModel, imageCapability, falAPI) {
			return false
		}
		if !s.falAccountPricingConfigured(ctx, account, requestedModel, falAPI, groupID) {
			return false
		}
		return s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) &&
			s.isAccountSchedulableForQuota(account) &&
			s.isAccountSchedulableForWindowCost(ctx, account, false) &&
			s.isAccountSchedulableForRPM(ctx, account, false)
	}

	if sessionHash != "" && s.cache != nil {
		if accountID, cacheErr := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash); cacheErr == nil && accountID > 0 {
			for i := range accounts {
				account := &accounts[i]
				if account.ID == accountID && eligible(account) {
					return s.hydrateSelectedAccount(ctx, account)
				}
			}
		}
	}

	pick := func(platform string) *Account {
		var selected *Account
		for i := range accounts {
			account := &accounts[i]
			if platform != "" && account.Platform != platform {
				continue
			}
			if !eligible(account) {
				continue
			}
			if selected == nil || imageAccountPreferred(account, selected) {
				selected = account
			}
		}
		return selected
	}

	var selected *Account
	if strings.EqualFold(strings.TrimSpace(preferPlatform), PlatformFal) {
		selected = pick(PlatformFal)
		if selected == nil {
			selected = pick(PlatformOpenAI)
		}
	} else {
		selected = pick("")
	}
	if selected == nil {
		return nil, ErrNoAvailableAccounts
	}
	if sessionHash != "" && s.cache != nil {
		if bindErr := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.ID, stickySessionTTLForGroup(s.groupFromContext(ctx, derefGroupID(groupID)))); bindErr != nil {
			logger.LegacyPrintf("service.gateway", "set image session account failed: session=%s account_id=%d err=%v", sessionHash, selected.ID, bindErr)
		}
	}
	return s.hydrateSelectedAccount(ctx, selected)
}

func imageAccountPreferred(candidate, selected *Account) bool {
	if candidate.Priority != selected.Priority {
		return candidate.Priority < selected.Priority
	}
	switch {
	case candidate.LastUsedAt == nil && selected.LastUsedAt != nil:
		return true
	case candidate.LastUsedAt != nil && selected.LastUsedAt == nil:
		return false
	case candidate.LastUsedAt == nil:
		return false
	default:
		return candidate.LastUsedAt.Before(*selected.LastUsedAt)
	}
}

func (s *GatewayService) listSchedulableImageAccounts(ctx context.Context, groupID *int64) ([]Account, error) {
	openAIAccounts, _, err := s.listSchedulableAccounts(ctx, groupID, PlatformOpenAI, false)
	if err != nil {
		return nil, fmt.Errorf("query openai accounts failed: %w", err)
	}
	falAccounts, _, err := s.listSchedulableAccounts(ctx, groupID, PlatformFal, false)
	if err != nil {
		return nil, fmt.Errorf("query fal accounts failed: %w", err)
	}
	return append(openAIAccounts, falAccounts...), nil
}

func (s *GatewayService) imageAccountSupportsRequest(ctx context.Context, account *Account, requestedModel string, capability OpenAIImagesCapability, falAPI string) bool {
	switch account.Platform {
	case PlatformOpenAI:
		return (requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, account, requestedModel, "")) && account.SupportsOpenAIImageCapability(capability)
	case PlatformFal:
		return s.isModelSupportedByAccountWithContext(ctx, account, requestedModel, falAPI)
	default:
		return false
	}
}

func (s *GatewayService) falAccountPricingConfigured(ctx context.Context, account *Account, requestedModel, falAPI string, groupID *int64) bool {
	if account == nil || account.Platform != PlatformFal {
		return true
	}
	if s.resolver == nil {
		return false
	}
	upstreamModel := resolveFalUpstreamModel(account, requestedModel, falAPI == FalAPIEdit)
	resolved := s.resolver.Resolve(ctx, PricingInput{Model: upstreamModel, GroupID: groupID})
	if resolved != nil && resolved.Source == PricingSourceChannel {
		return (resolved.Mode == BillingModeImage || resolved.Mode == BillingModePerRequest) &&
			(len(resolved.RequestTiers) > 0 || resolved.DefaultPerRequestPrice > 0)
	}
	if groupID == nil {
		return false
	}
	group := s.groupFromContext(ctx, *groupID)
	if group == nil && s.groupRepo != nil {
		group, _ = s.groupRepo.GetByIDLite(ctx, *groupID)
	}
	if group == nil {
		return false
	}
	for _, row := range group.ImagePricingMatrix {
		for _, price := range row {
			if price > 0 {
				return true
			}
		}
	}
	return false
}
