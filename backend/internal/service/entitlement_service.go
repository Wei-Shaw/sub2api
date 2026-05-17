package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
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

type EntitlementCreditQuota struct {
	PurchasedLedgerAmount float64  `json:"purchased_ledger_amount"`
	PurchasedCredits      float64  `json:"purchased_credits"`
	UsedLedgerAmount      float64  `json:"used_ledger_amount"`
	UsedCredits           float64  `json:"used_credits"`
	RemainingCredits      float64  `json:"remaining_credits"`
	UsedPercent           float64  `json:"used_percent"`
	NearLimit             bool     `json:"near_limit"`
	CreditUnitScale       float64  `json:"credit_unit_scale"`
	Accuracy              string   `json:"accuracy,omitempty"`
	AccuracyNotes         []string `json:"accuracy_notes,omitempty"`
}

type EntitlementItem struct {
	GroupID              int64                   `json:"group_id"`
	GroupName            string                  `json:"group_name"`
	GroupPlatform        string                  `json:"group_platform,omitempty"`
	Mode                 string                  `json:"mode"`
	Status               string                  `json:"status"`
	StartsAt             *time.Time              `json:"starts_at,omitempty"`
	ExpiresAt            *time.Time              `json:"expires_at,omitempty"`
	DailyUsageUSD        float64                 `json:"daily_usage_usd"`
	WeeklyUsageUSD       float64                 `json:"weekly_usage_usd"`
	MonthlyUsageUSD      float64                 `json:"monthly_usage_usd"`
	DailyLimitUSD        *float64                `json:"daily_limit_usd,omitempty"`
	WeeklyLimitUSD       *float64                `json:"weekly_limit_usd,omitempty"`
	MonthlyLimitUSD      *float64                `json:"monthly_limit_usd,omitempty"`
	RateMultiplier       float64                 `json:"rate_multiplier"`
	TokenPricePerMillion *float64                `json:"token_price_per_million,omitempty"`
	SupportedModelScopes []string                `json:"supported_model_scopes,omitempty"`
	Switchable           bool                    `json:"switchable"`
	Current              bool                    `json:"current"`
	SubscriptionID       *int64                  `json:"subscription_id,omitempty"`
	FallbackGroupID      *int64                  `json:"fallback_group_id,omitempty"`
	CreditQuota          *EntitlementCreditQuota `json:"credit_quota,omitempty"`
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
	Current      *EntitlementCurrent            `json:"current,omitempty"`
	APIKey       *APIKey                        `json:"api_key,omitempty"`
	Entitlements []EntitlementItem              `json:"entitlements"`
	Fallback     EntitlementFallback            `json:"fallback"`
	CreditUsage  *usagestats.CreditUsageSummary `json:"credit_usage,omitempty"`
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
	usageRepo   entitlementUsageRepository
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

// entitlementUsageRepository is the narrow contract EntitlementService needs from the usage log
// repository. It is intentionally small so service tests can stub credit usage without pulling in
// the full UsageLogRepository surface.
type entitlementUsageRepository interface {
	GetUserCreditUsageSummary(ctx context.Context, userID int64) (*usagestats.CreditUsageSummary, error)
}

func NewEntitlementService(userRepo entitlementUserRepository, groupRepo entitlementGroupRepository, apiKeySvc entitlementAPIKeyUpdater, apiKeyRepo entitlementAPIKeyRepository, userSubRepo entitlementUserSubscriptionRepository) *EntitlementService {
	return &EntitlementService{userRepo: userRepo, groupRepo: groupRepo, apiKeySvc: apiKeySvc, apiKeyRepo: apiKeyRepo, userSubRepo: userSubRepo}
}

// SetUsageRepository wires the optional credit usage repository. When set, GetUserEntitlements will
// best-effort attach aggregate credit usage to the response. Errors from this repository never fail
// the entitlement response itself; they are logged and the credit_usage field is left empty.
func (s *EntitlementService) SetUsageRepository(repo entitlementUsageRepository) {
	if s == nil {
		return
	}
	s.usageRepo = repo
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

	s.attachCreditUsage(ctx, state, userID)

	return state, nil
}

const entitlementCreditNearLimitPercent = 80.0

// attachCreditUsage best-effort hydrates aggregate CreditUsage on the state and per-balance-group
// credit_quota on each balance entitlement item. It deliberately swallows repository errors and
// logs them: credit usage is presentation/quota-progress data, not authorization, so a transient
// SQL failure must not break entitlement listing/switching.
func (s *EntitlementService) attachCreditUsage(ctx context.Context, state *EntitlementState, userID int64) {
	if s == nil || s.usageRepo == nil || state == nil {
		return
	}
	summary, err := s.usageRepo.GetUserCreditUsageSummary(ctx, userID)
	if err != nil {
		logger.LegacyPrintf("service.entitlement", "credit usage summary lookup failed for user %d: %v", userID, err)
		return
	}
	if summary == nil {
		return
	}
	state.CreditUsage = summary

	// Index group estimates so each balance entitlement can render exact remaining/percent without
	// re-querying. Only balance/credit groups participate; subscription groups still rely on
	// daily/weekly/monthly USD counters.
	estimates := make(map[int64]usagestats.CreditUsageGroupEstimate, len(summary.GroupEstimates))
	for _, est := range summary.GroupEstimates {
		if est.GroupID == 0 {
			continue
		}
		estimates[est.GroupID] = est
	}

	totalPurchasedCredits := summary.TotalPurchasedCredits
	totalUsedCredits := summary.TotalUsedCredits
	for i := range state.Entitlements {
		item := &state.Entitlements[i]
		if item.Mode != EntitlementModeBalance {
			continue
		}
		est, ok := estimates[item.GroupID]
		if !ok {
			continue
		}
		quota := buildEntitlementCreditQuota(est, totalPurchasedCredits, totalUsedCredits, summary.TotalUsedLedgerAmount, 1.0)
		item.CreditQuota = quota
	}
}

func buildEntitlementCreditQuota(est usagestats.CreditUsageGroupEstimate, totalPurchasedCredits, totalUsedCredits, totalUsedLedger, creditUnitScale float64) *EntitlementCreditQuota {
	// Each balance group's purchased credits comes from immutable payment_orders rows; allocation
	// of usage to a specific group is not tracked, so we approximate per-group used credits as a
	// share of aggregate used credits proportional to purchased credits. This is the same accuracy
	// caveat documented in the credit-usage skill — use accuracy=aggregate_estimate downstream.
	usedShareCredits := 0.0
	usedShareLedger := 0.0
	if totalPurchasedCredits > 0 && est.PurchasedCredits > 0 {
		share := est.PurchasedCredits / totalPurchasedCredits
		usedShareCredits = totalUsedCredits * share
		usedShareLedger = totalUsedLedger * share
	}
	remaining := est.PurchasedCredits - usedShareCredits
	if remaining < 0 {
		remaining = 0
	}
	percent := 0.0
	if est.PurchasedCredits > 0 {
		percent = usedShareCredits / est.PurchasedCredits * 100.0
		if percent < 0 {
			percent = 0
		}
		if percent > 100 {
			percent = 100
		}
	}
	return &EntitlementCreditQuota{
		PurchasedLedgerAmount: est.PurchasedLedgerAmount,
		PurchasedCredits:      est.PurchasedCredits,
		UsedLedgerAmount:      usedShareLedger,
		UsedCredits:           usedShareCredits,
		RemainingCredits:      remaining,
		UsedPercent:           percent,
		NearLimit:             percent >= entitlementCreditNearLimitPercent,
		CreditUnitScale:       creditUnitScale,
		Accuracy:              "balance_derived",
		AccuracyNotes: []string{
			"purchased_credits from SUM(payment_orders.actual_credits); remaining derived proportionally from current balance",
			"used_credits = purchased_credits - remaining_credits",
		},
	}
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
	var tokenPricePerMillion *float64
	if group != nil {
		name = group.Name
		platform = group.Platform
		rateMultiplier = group.RateMultiplier
		modelScopes = append([]string(nil), group.SupportedModelScopes...)
		dailyLimit = group.DailyLimitUSD
		weeklyLimit = group.WeeklyLimitUSD
		monthlyLimit = group.MonthlyLimitUSD
		fallbackGroupID = group.FallbackGroupID
		tokenPricePerMillion = group.TokenPricePerMillion
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
		TokenPricePerMillion: tokenPricePerMillion,
		SupportedModelScopes: modelScopes,
		Switchable:           sub.IsActive(),
		SubscriptionID:       &subID,
		FallbackGroupID:      fallbackGroupID,
	}
}
