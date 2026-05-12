package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrEntitlementNotAvailable   = infraerrors.Forbidden("ENTITLEMENT_NOT_AVAILABLE", "entitlement is not available for this user")
	ErrEntitlementAPIKeyRequired = infraerrors.BadRequest("ENTITLEMENT_API_KEY_REQUIRED", "no active api key is available for entitlement binding")
)

const (
	EntitlementModeSubscription = "subscription"
	EntitlementModeBalance      = "balance"
)

type EntitlementFallback struct {
	Mode        string  `json:"mode"`
	Available   bool    `json:"available"`
	BalanceUSD  float64 `json:"balance_usd"`
	Reason      string  `json:"reason,omitempty"`
	TargetGroup *Group  `json:"target_group,omitempty"`
}

type EntitlementItem struct {
	GroupID              int64      `json:"group_id"`
	GroupName            string     `json:"group_name"`
	GroupPlatform        string     `json:"group_platform,omitempty"`
	Mode                 string     `json:"mode"`
	Status               string     `json:"status"`
	StartsAt             *time.Time `json:"starts_at,omitempty"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	DailyUsageUSD        float64    `json:"daily_usage_usd"`
	WeeklyUsageUSD       float64    `json:"weekly_usage_usd"`
	MonthlyUsageUSD      float64    `json:"monthly_usage_usd"`
	DailyLimitUSD        *float64   `json:"daily_limit_usd,omitempty"`
	WeeklyLimitUSD       *float64   `json:"weekly_limit_usd,omitempty"`
	MonthlyLimitUSD      *float64   `json:"monthly_limit_usd,omitempty"`
	RateMultiplier       float64    `json:"rate_multiplier"`
	SupportedModelScopes []string   `json:"supported_model_scopes,omitempty"`
	Switchable           bool       `json:"switchable"`
	Current              bool       `json:"current"`
	SubscriptionID       *int64     `json:"subscription_id,omitempty"`
	FallbackGroupID      *int64     `json:"fallback_group_id,omitempty"`
}

type EntitlementCurrent struct {
	APIKeyID             int64    `json:"api_key_id"`
	GroupID              *int64   `json:"group_id,omitempty"`
	GroupName            string   `json:"group_name,omitempty"`
	GroupPlatform        string   `json:"group_platform,omitempty"`
	Mode                 string   `json:"mode"`
	RateMultiplier       float64  `json:"rate_multiplier,omitempty"`
	SupportedModelScopes []string `json:"supported_model_scopes,omitempty"`
	MonthlyLimitUSD      *float64 `json:"monthly_limit_usd,omitempty"`
	MonthlyUsageUSD      *float64 `json:"monthly_usage_usd,omitempty"`
}

type EntitlementState struct {
	Current      *EntitlementCurrent `json:"current,omitempty"`
	APIKey       *APIKey             `json:"api_key,omitempty"`
	Entitlements []EntitlementItem   `json:"entitlements"`
	Fallback     EntitlementFallback `json:"fallback"`
}

type SwitchEntitlementRequest struct {
	GroupID  int64  `json:"group_id"`
	APIKeyID *int64 `json:"api_key_id,omitempty"`
}

type EntitlementSwitchResult struct {
	APIKey *APIKey           `json:"api_key"`
	State  *EntitlementState `json:"state"`
}

type EntitlementService struct {
	userRepo    entitlementUserRepository
	groupRepo   entitlementGroupRepository
	apiKeySvc   entitlementAPIKeyUpdater
	apiKeyRepo  entitlementAPIKeyRepository
	userSubRepo entitlementUserSubscriptionRepository
}

type entitlementUserRepository interface {
	GetByID(ctx context.Context, id int64) (*User, error)
}

type entitlementGroupRepository interface {
	GetByID(ctx context.Context, id int64) (*Group, error)
}

type entitlementAPIKeyUpdater interface {
	Update(ctx context.Context, id, userID int64, req UpdateAPIKeyRequest) (*APIKey, error)
}

type entitlementAPIKeyRepository interface {
	ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error)
}

type entitlementUserSubscriptionRepository interface {
	ListByUserID(ctx context.Context, userID int64) ([]UserSubscription, error)
}

func NewEntitlementService(userRepo entitlementUserRepository, groupRepo entitlementGroupRepository, apiKeySvc entitlementAPIKeyUpdater, apiKeyRepo entitlementAPIKeyRepository, userSubRepo entitlementUserSubscriptionRepository) *EntitlementService {
	return &EntitlementService{userRepo: userRepo, groupRepo: groupRepo, apiKeySvc: apiKeySvc, apiKeyRepo: apiKeyRepo, userSubRepo: userSubRepo}
}

func (s *EntitlementService) GetUserEntitlements(ctx context.Context, userID int64) (*EntitlementState, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	keys, err := s.listUserAPIKeys(ctx, userID)
	if err != nil {
		return nil, err
	}
	currentKey := selectCurrentAPIKey(keys)

	subs, err := s.userSubRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}

	items := make([]EntitlementItem, 0, len(subs))
	for i := range subs {
		sub := subs[i]
		group, _ := s.groupRepo.GetByID(ctx, sub.GroupID)
		item := entitlementItemFromSubscription(sub, group)
		item.Current = currentKey != nil && currentKey.GroupID != nil && *currentKey.GroupID == sub.GroupID
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Switchable != items[j].Switchable {
			return items[i].Switchable
		}
		if items[i].ExpiresAt == nil || items[j].ExpiresAt == nil {
			return items[i].GroupID < items[j].GroupID
		}
		return items[i].ExpiresAt.After(*items[j].ExpiresAt)
	})

	state := &EntitlementState{
		APIKey:       currentKey,
		Entitlements: items,
		Fallback: EntitlementFallback{
			Mode:       EntitlementModeBalance,
			Available:  user.Balance > 0,
			BalanceUSD: user.Balance,
		},
	}
	if currentKey != nil {
		state.Current = &EntitlementCurrent{APIKeyID: currentKey.ID, GroupID: currentKey.GroupID, Mode: EntitlementModeBalance}
		if currentKey.GroupID != nil {
			if group, err := s.groupRepo.GetByID(ctx, *currentKey.GroupID); err == nil && group != nil {
				state.Current.GroupName = group.Name
				state.Current.GroupPlatform = group.Platform
				state.Current.RateMultiplier = group.RateMultiplier
				state.Current.SupportedModelScopes = append([]string(nil), group.SupportedModelScopes...)
				state.Current.MonthlyLimitUSD = group.MonthlyLimitUSD
				if group.IsSubscriptionType() {
					state.Current.Mode = EntitlementModeSubscription
					for i := range items {
						if items[i].GroupID == group.ID {
							usage := items[i].MonthlyUsageUSD
							state.Current.MonthlyUsageUSD = &usage
							break
						}
					}
				}
			}
		}
	}
	return state, nil
}

func (s *EntitlementService) RefreshUserEntitlements(ctx context.Context, userID int64) (*EntitlementState, error) {
	return s.GetUserEntitlements(ctx, userID)
}

func (s *EntitlementService) SwitchEntitlement(ctx context.Context, userID int64, req SwitchEntitlementRequest) (*EntitlementSwitchResult, error) {
	if req.GroupID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_GROUP", "group_id is required")
	}

	keyID, err := s.resolveAPIKeyID(ctx, userID, req.APIKeyID)
	if err != nil {
		return nil, err
	}
	updated, err := s.apiKeySvc.Update(ctx, keyID, userID, UpdateAPIKeyRequest{GroupID: &req.GroupID})
	if err != nil {
		return nil, fmt.Errorf("switch entitlement: %w", err)
	}
	state, err := s.GetUserEntitlements(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &EntitlementSwitchResult{APIKey: updated, State: state}, nil
}

func (s *EntitlementService) BindUserToGroupAfterPayment(ctx context.Context, userID, groupID int64) (*EntitlementSwitchResult, error) {
	return s.SwitchEntitlement(ctx, userID, SwitchEntitlementRequest{GroupID: groupID})
}

func (s *EntitlementService) resolveAPIKeyID(ctx context.Context, userID int64, requested *int64) (int64, error) {
	if requested != nil && *requested > 0 {
		return *requested, nil
	}
	keys, err := s.listUserAPIKeys(ctx, userID)
	if err != nil {
		return 0, err
	}
	selected := selectCurrentAPIKey(keys)
	if selected == nil {
		return 0, ErrEntitlementAPIKeyRequired
	}
	return selected.ID, nil
}

func (s *EntitlementService) listUserAPIKeys(ctx context.Context, userID int64) ([]APIKey, error) {
	params := pagination.PaginationParams{Page: 1, PageSize: 1000, SortBy: "created_at", SortOrder: pagination.SortOrderDesc}
	keys, _, err := s.apiKeyRepo.ListByUserID(ctx, userID, params, APIKeyListFilters{})
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	return keys, nil
}

func selectCurrentAPIKey(keys []APIKey) *APIKey {
	if len(keys) == 0 {
		return nil
	}
	for i := range keys {
		if keys[i].Status == StatusActive && keys[i].GroupID != nil {
			return &keys[i]
		}
	}
	for i := range keys {
		if keys[i].Status == StatusActive {
			return &keys[i]
		}
	}
	return &keys[0]
}

func entitlementItemFromSubscription(sub UserSubscription, group *Group) EntitlementItem {
	mode := EntitlementModeSubscription
	name := ""
	platform := ""
	rateMultiplier := 1.0
	var modelScopes []string
	var dailyLimit, weeklyLimit, monthlyLimit *float64
	var fallbackGroupID *int64
	if group != nil {
		name = group.Name
		platform = group.Platform
		rateMultiplier = group.RateMultiplier
		modelScopes = append([]string(nil), group.SupportedModelScopes...)
		dailyLimit = group.DailyLimitUSD
		weeklyLimit = group.WeeklyLimitUSD
		monthlyLimit = group.MonthlyLimitUSD
		fallbackGroupID = group.FallbackGroupID
		if !group.IsSubscriptionType() {
			mode = EntitlementModeBalance
		}
	}
	subID := sub.ID
	startsAt := sub.StartsAt
	expiresAt := sub.ExpiresAt
	return EntitlementItem{
		GroupID:              sub.GroupID,
		GroupName:            name,
		GroupPlatform:        platform,
		Mode:                 mode,
		Status:               sub.Status,
		StartsAt:             &startsAt,
		ExpiresAt:            &expiresAt,
		DailyUsageUSD:        sub.DailyUsageUSD,
		WeeklyUsageUSD:       sub.WeeklyUsageUSD,
		MonthlyUsageUSD:      sub.MonthlyUsageUSD,
		DailyLimitUSD:        dailyLimit,
		WeeklyLimitUSD:       weeklyLimit,
		MonthlyLimitUSD:      monthlyLimit,
		RateMultiplier:       rateMultiplier,
		SupportedModelScopes: modelScopes,
		Switchable:           sub.IsActive(),
		SubscriptionID:       &subID,
		FallbackGroupID:      fallbackGroupID,
	}
}
