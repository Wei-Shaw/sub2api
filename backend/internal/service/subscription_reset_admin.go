package service

import (
	"context"
	"slices"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// AdminSubscriptionResetService validates references against current group
// membership before saving. Runtime monitoring rechecks membership as well.
type AdminSubscriptionResetService struct {
	repo     SubscriptionResetObserverRepository
	groups   GroupRepository
	accounts AccountRepository
}

func NewAdminSubscriptionResetService(repo SubscriptionResetObserverRepository, groups GroupRepository, accounts AccountRepository) *AdminSubscriptionResetService {
	return &AdminSubscriptionResetService{repo: repo, groups: groups, accounts: accounts}
}

func (s *AdminSubscriptionResetService) validateGroup(ctx context.Context, groupID int64) error {
	group, err := s.groups.GetByIDLite(ctx, groupID)
	if err != nil {
		return err
	}
	if group == nil || !group.IsSubscriptionType() || group.Platform != PlatformOpenAI {
		return infraerrors.BadRequest("RESET_GROUP_NOT_SUBSCRIPTION", "reset observation requires an OpenAI subscription group")
	}
	return nil
}

func (s *AdminSubscriptionResetService) GetPolicy(ctx context.Context, groupID int64) (*SubscriptionResetPolicy, error) {
	if err := s.validateGroup(ctx, groupID); err != nil {
		return nil, err
	}
	policy, err := s.repo.GetPolicy(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		policy = DefaultSubscriptionResetPolicy(groupID)
	}
	return policy, nil
}

func (s *AdminSubscriptionResetService) SavePolicy(ctx context.Context, policy *SubscriptionResetPolicy) (*SubscriptionResetPolicy, error) {
	if policy == nil {
		return nil, infraerrors.BadRequest("RESET_POLICY_REQUIRED", "reset policy is required")
	}
	if err := s.validateGroup(ctx, policy.GroupID); err != nil {
		return nil, err
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if len(policy.AccountIDs) > 200 {
		return nil, infraerrors.BadRequest("RESET_REFERENCE_LIMIT", "select at most 200 reference accounts")
	}
	if policy.Mode != "off" {
		accounts, err := s.accounts.GetByIDs(ctx, policy.AccountIDs)
		if err != nil {
			return nil, err
		}
		eligible := make(map[int64]bool, len(accounts))
		for _, account := range accounts {
			if IsOpenAIQuotaHistoryAccount(account) && slices.Contains(account.GroupIDs, policy.GroupID) {
				eligible[account.ID] = true
			}
		}
		for _, id := range policy.AccountIDs {
			if !eligible[id] {
				return nil, infraerrors.BadRequest("RESET_REFERENCE_INVALID", "all reference accounts must be ordinary OpenAI OAuth accounts in this group")
			}
		}
	}
	return s.repo.SavePolicy(ctx, policy)
}

func (s *AdminSubscriptionResetService) GetStatus(ctx context.Context, groupID int64, limit int) (*SubscriptionResetStatus, error) {
	if err := s.validateGroup(ctx, groupID); err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return s.repo.GetStatus(ctx, groupID, limit)
}
