package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

func (s *AuthService) provisionInviteBootstrapAPIKeys(ctx context.Context, userID int64, redeemCode *RedeemCode) ([]InviteBootstrapAPIKey, error) {
	if s == nil || s.inviteBootstrapAPIKeySvc == nil || redeemCode == nil {
		return nil, ErrBootstrapAPIKeyUnavailable
	}

	groups, err := s.loadInviteBootstrapGroups(ctx, userID, redeemCode)
	if err != nil {
		return nil, ErrBootstrapAPIKeyUnavailable
	}
	selectedGroups := selectInviteBootstrapGroupsForRedeem(redeemCode, groups)
	if err := s.ensureInviteBootstrapSubscriptions(ctx, userID, redeemCode, selectedGroups); err != nil {
		return nil, ErrBootstrapAPIKeyUnavailable
	}
	if len(selectedGroups) == 0 {
		return nil, ErrBootstrapAPIKeyUnavailable
	}

	platforms := make([]string, 0, len(selectedGroups))
	for platform := range selectedGroups {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)

	keys := make([]InviteBootstrapAPIKey, 0, len(platforms))
	for _, platform := range platforms {
		group := selectedGroups[platform]
		groupID := group.ID
		created, createErr := s.inviteBootstrapAPIKeySvc.Create(ctx, userID, CreateAPIKeyRequest{
			Name:    "bootstrap-" + sanitizePlatformForBootstrapKey(platform),
			GroupID: &groupID,
		})
		if createErr != nil || created == nil {
			logger.LegacyPrintf("service.auth", "[InviteBootstrap] create api key failed: user_id=%d platform=%s group_id=%d err=%v", userID, platform, groupID, createErr)
			continue
		}
		keys = append(keys, InviteBootstrapAPIKey{
			ID:       created.ID,
			Name:     created.Name,
			Key:      created.Key,
			GroupID:  group.ID,
			Platform: group.Platform,
		})
	}

	if len(keys) == 0 {
		return nil, ErrBootstrapAPIKeyUnavailable
	}
	return keys, nil
}

func (s *AuthService) ensureInviteBootstrapSubscriptions(ctx context.Context, userID int64, redeemCode *RedeemCode, selectedGroups map[string]Group) error {
	if redeemCode == nil || len(selectedGroups) == 0 {
		return ErrBootstrapAPIKeyUnavailable
	}
	if redeemCode.Type != RedeemTypeSubscription && redeemCode.Type != RedeemTypeInvitation {
		return nil
	}
	if s.defaultSubAssigner == nil {
		return ErrBootstrapAPIKeyUnavailable
	}

	validityDays := inviteBootstrapValidityDays(redeemCode)
	platforms := make([]string, 0, len(selectedGroups))
	for platform := range selectedGroups {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)
	for _, platform := range platforms {
		group := selectedGroups[platform]
		if _, _, err := s.defaultSubAssigner.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
			UserID:       userID,
			GroupID:      group.ID,
			ValidityDays: validityDays,
			AssignedBy:   0,
			Notes:        "invite-login bootstrap subscription",
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *AuthService) applyInviteLoginEntitlement(ctx context.Context, userID int64, redeemCode *RedeemCode) error {
	if redeemCode == nil || userID <= 0 {
		return ErrInvitationCodeInvalid
	}

	switch redeemCode.Type {
	case RedeemTypeBalance:
		if redeemCode.Value != 0 && s.userRepo.UpdateBalance(ctx, userID, redeemCode.Value) != nil {
			return ErrServiceUnavailable
		}
		return nil
	case RedeemTypeSubscription:
		if redeemCode.GroupID == nil || s.defaultSubAssigner == nil {
			return ErrServiceUnavailable
		}
		_, _, err := s.defaultSubAssigner.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
			UserID:       userID,
			GroupID:      *redeemCode.GroupID,
			ValidityDays: inviteBootstrapValidityDays(redeemCode),
			AssignedBy:   0,
			Notes:        fmt.Sprintf("invite-login redeem subscription code %s", redeemCode.Code),
		})
		if err != nil {
			return ErrServiceUnavailable
		}
		return nil
	case RedeemTypeInvitation:
		return nil
	default:
		return ErrInvitationCodeInvalid
	}
}

func inviteBootstrapValidityDays(redeemCode *RedeemCode) int {
	if redeemCode != nil && redeemCode.ValidityDays > 0 {
		return redeemCode.ValidityDays
	}
	return 30
}

func isInviteLoginBootstrapRedeemType(redeemType string) bool {
	switch redeemType {
	case RedeemTypeInvitation, RedeemTypeSubscription, RedeemTypeBalance, RedeemTypeDeviceLogin:
		return true
	default:
		return false
	}
}

func (s *AuthService) loadInviteBootstrapGroups(ctx context.Context, userID int64, redeemCode *RedeemCode) ([]Group, error) {
	if s == nil || redeemCode == nil || s.inviteBootstrapAPIKeySvc == nil {
		return nil, ErrBootstrapAPIKeyUnavailable
	}
	if redeemCode.Type == RedeemTypeSubscription || redeemCode.Type == RedeemTypeInvitation {
		if s.groupRepo == nil {
			return nil, ErrBootstrapAPIKeyUnavailable
		}
		groups, err := s.groupRepo.ListActive(ctx)
		if err != nil {
			return nil, err
		}
		return loadInviteBootstrapSubscriptionCandidates(groups, redeemCode), nil
	}
	return s.inviteBootstrapAPIKeySvc.GetAvailableGroups(ctx, userID)
}

func loadInviteBootstrapSubscriptionCandidates(groups []Group, redeemCode *RedeemCode) []Group {
	if redeemCode == nil || (redeemCode.Type != RedeemTypeSubscription && redeemCode.Type != RedeemTypeInvitation) {
		return groups
	}
	candidates := make([]Group, 0, len(groups))
	for _, group := range groups {
		if !group.IsActive() || strings.TrimSpace(group.Platform) == "" || !group.IsSubscriptionType() {
			continue
		}
		if redeemCode.Type == RedeemTypeSubscription && redeemCode.GroupID != nil && group.ID != *redeemCode.GroupID {
			continue
		}
		candidates = append(candidates, group)
	}
	return candidates
}

func selectInviteBootstrapGroupsForRedeem(redeemCode *RedeemCode, groups []Group) map[string]Group {
	if redeemCode == nil {
		return nil
	}
	selected := make(map[string]Group)
	for _, group := range groups {
		platform := strings.TrimSpace(group.Platform)
		if platform == "" || !group.IsActive() || group.ActiveAccountCount <= 0 || !isGroupEligibleForInviteBootstrap(redeemCode, group) {
			continue
		}
		current, exists := selected[platform]
		if !exists || isInviteBootstrapGroupBetter(redeemCode, group, current) {
			selected[platform] = group
		}
	}
	return selected
}

func isGroupEligibleForInviteBootstrap(redeemCode *RedeemCode, group Group) bool {
	switch redeemCode.Type {
	case RedeemTypeSubscription, RedeemTypeInvitation:
		return group.IsSubscriptionType()
	case RedeemTypeBalance:
		return !group.IsSubscriptionType()
	case RedeemTypeDeviceLogin:
		return true
	default:
		return false
	}
}

func isInviteBootstrapGroupBetter(redeemCode *RedeemCode, a, b Group) bool {
	switch redeemCode.Type {
	case RedeemTypeBalance:
		if a.RateMultiplier != b.RateMultiplier {
			return a.RateMultiplier < b.RateMultiplier
		}
	case RedeemTypeSubscription, RedeemTypeInvitation:
		if a.DefaultValidityDays != b.DefaultValidityDays {
			return a.DefaultValidityDays > b.DefaultValidityDays
		}
	case RedeemTypeDeviceLogin:
		if a.IsSubscriptionType() != b.IsSubscriptionType() {
			return a.IsSubscriptionType()
		}
	}
	if a.SortOrder != b.SortOrder {
		return a.SortOrder < b.SortOrder
	}
	return a.ID < b.ID
}

func sanitizePlatformForBootstrapKey(platform string) string {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == "" {
		return "unknown"
	}
	var out []rune
	lastDash := false
	for _, r := range platform {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out = append(out, r)
			lastDash = false
			continue
		}
		if !lastDash {
			out = append(out, '-')
			lastDash = true
		}
	}
	if result := strings.Trim(string(out), "-"); result != "" {
		return result
	}
	return "unknown"
}
