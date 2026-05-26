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
	ErrEntitlementNotAvailable           = infraerrors.Forbidden("ENTITLEMENT_NOT_AVAILABLE", "entitlement is not available for this user")
	ErrEntitlementAPIKeyRequired         = infraerrors.BadRequest("ENTITLEMENT_API_KEY_REQUIRED", "no active api key is available for entitlement binding")
	ErrEntitlementAutoSwitchNotAvailable = infraerrors.Conflict("ENTITLEMENT_AUTO_SWITCH_NOT_AVAILABLE", "no entitlement switch target is available")
)

const (
	EntitlementModeSubscription = "subscription"
	EntitlementModeBalance      = "balance"

	EntitlementSwitchActionGroup  = "switch_group"
	EntitlementSwitchActionAPIKey = "switch_api_key"
)

type EntitlementTargetGroup struct {
	GroupID              int64    `json:"group_id"`
	GroupName            string   `json:"group_name"`
	GroupPlatform        string   `json:"group_platform,omitempty"`
	ProviderID           string   `json:"provider_id,omitempty"`
	SupportedModelScopes []string `json:"supported_model_scopes,omitempty"`
	Source               string   `json:"source,omitempty"`
}

type EntitlementFallback struct {
	Mode        string                  `json:"mode"`
	Available   bool                    `json:"available"`
	BalanceUSD  float64                 `json:"balance_usd"`
	Reason      string                  `json:"reason,omitempty"`
	TargetGroup *EntitlementTargetGroup `json:"target_group,omitempty"`
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
	ProviderID           string   `json:"provider_id,omitempty"`
	Mode                 string   `json:"mode"`
	RateMultiplier       float64  `json:"rate_multiplier,omitempty"`
	SupportedModelScopes []string `json:"supported_model_scopes,omitempty"`
	MonthlyLimitUSD      *float64 `json:"monthly_limit_usd,omitempty"`
	MonthlyUsageUSD      *float64 `json:"monthly_usage_usd,omitempty"`
}

type EntitlementAPIKeyView struct {
	ID             int64      `json:"id"`
	Key            string     `json:"-"`
	Name           string     `json:"name,omitempty"`
	Status         string     `json:"status"`
	GroupID        *int64     `json:"group_id,omitempty"`
	QuotaRemaining float64    `json:"quota_remaining_usd"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

type EntitlementSwitchTarget struct {
	Mode                 string   `json:"mode"`
	APIKeyID             int64    `json:"api_key_id"`
	GroupID              int64    `json:"group_id"`
	GroupName            string   `json:"group_name,omitempty"`
	GroupPlatform        string   `json:"group_platform,omitempty"`
	ProviderID           string   `json:"provider_id,omitempty"`
	Priority             int      `json:"priority"`
	Reason               string   `json:"reason"`
	EstimatedBalanceUSD  float64  `json:"estimated_balance_usd,omitempty"`
	Switchable           bool     `json:"switchable"`
	SupportedModelScopes []string `json:"supported_model_scopes,omitempty"`
}

type EntitlementState struct {
	Current       *EntitlementCurrent            `json:"current,omitempty"`
	APIKey        *EntitlementAPIKeyView         `json:"api_key,omitempty"`
	Entitlements  []EntitlementItem              `json:"entitlements"`
	Fallback      EntitlementFallback            `json:"fallback"`
	CreditUsage   *usagestats.CreditUsageSummary `json:"credit_usage,omitempty"`
	SwitchTargets []EntitlementSwitchTarget      `json:"switch_targets,omitempty"`
}

type SwitchEntitlementRequest struct {
	GroupID  int64  `json:"group_id"`
	APIKeyID *int64 `json:"api_key_id,omitempty"`
}

type EntitlementSwitchResult struct {
	APIKey *EntitlementAPIKeyView `json:"api_key"`
	State  *EntitlementState      `json:"state"`
}

type AutoSwitchEntitlementRequest struct {
	Reason              string `json:"reason,omitempty"`
	ErrorCode           string `json:"error_code,omitempty"`
	CurrentAPIKeyID     *int64 `json:"current_api_key_id,omitempty"`
	CurrentGroupID      *int64 `json:"current_group_id,omitempty"`
	ProviderID          string `json:"provider_id,omitempty"`
	ModelID             string `json:"model_id,omitempty"`
	AllowAPIKeyChange   bool   `json:"allow_api_key_change"`
	AllowProviderChange bool   `json:"allow_provider_change"`
}

type EntitlementRuntimeAction struct {
	RequiresRestart      bool   `json:"requires_restart"`
	RetryOriginalRequest bool   `json:"retry_original_request"`
	RetryLimit           int    `json:"retry_limit"`
	MessageKey           string `json:"message_key,omitempty"`
}

type EntitlementAutoSwitchResult struct {
	Switched bool                      `json:"switched"`
	Action   string                    `json:"action"`
	Target   *EntitlementSwitchTarget  `json:"target,omitempty"`
	State    *EntitlementState         `json:"state,omitempty"`
	Runtime  *EntitlementRuntimeAction `json:"runtime,omitempty"`
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
		APIKey:       apiKeyView(currentKey),
		Entitlements: items,
		Fallback: EntitlementFallback{
			Mode:       EntitlementModeBalance,
			Available:  user.Balance > 0,
			BalanceUSD: user.Balance,
			Reason:     entitlementFallbackReason(user.Balance),
		},
	}
	if currentKey != nil {
		state.Current = &EntitlementCurrent{APIKeyID: currentKey.ID, GroupID: currentKey.GroupID, Mode: EntitlementModeBalance}
		if currentKey.GroupID != nil {
			if group, err := s.groupRepo.GetByID(ctx, *currentKey.GroupID); err == nil && group != nil {
				state.Current.GroupName = group.Name
				state.Current.GroupPlatform = group.Platform
				state.Current.ProviderID = providerIDForPlatform(group.Platform)
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
	state.SwitchTargets = s.buildSwitchTargets(ctx, user, keys, currentKey, items, AutoSwitchEntitlementRequest{})
	if len(state.SwitchTargets) > 0 {
		state.Fallback.TargetGroup = targetGroupFromSwitchTarget(state.SwitchTargets[0])
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
	return &EntitlementSwitchResult{APIKey: apiKeyView(updated), State: state}, nil
}

func (s *EntitlementService) AutoSwitchEntitlement(ctx context.Context, userID int64, req AutoSwitchEntitlementRequest) (*EntitlementAutoSwitchResult, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	keys, err := s.listUserAPIKeys(ctx, userID)
	if err != nil {
		return nil, err
	}
	currentKey := selectAutoSwitchCurrentAPIKey(keys, req.CurrentAPIKeyID)
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
	targets := s.buildSwitchTargets(ctx, user, keys, currentKey, items, req)
	if len(targets) == 0 {
		return nil, autoSwitchUnavailable(autoSwitchNoCandidateReason(currentKey, user, req))
	}
	target := targets[0]
	action := EntitlementSwitchActionGroup
	if currentKey == nil || target.APIKeyID != currentKey.ID {
		action = EntitlementSwitchActionAPIKey
	}
	if action == EntitlementSwitchActionGroup || (currentKey != nil && target.APIKeyID == currentKey.ID && (currentKey.GroupID == nil || *currentKey.GroupID != target.GroupID)) {
		if _, err := s.apiKeySvc.Update(ctx, target.APIKeyID, userID, UpdateAPIKeyRequest{GroupID: &target.GroupID}); err != nil {
			return nil, fmt.Errorf("auto switch entitlement: %w", err)
		}
	}
	state, err := s.GetUserEntitlements(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &EntitlementAutoSwitchResult{
		Switched: true,
		Action:   action,
		Target:   &target,
		State:    state,
		Runtime: &EntitlementRuntimeAction{
			RequiresRestart:      true,
			RetryOriginalRequest: true,
			RetryLimit:           1,
			MessageKey:           "entitlements.switchedToCredit",
		},
	}, nil
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

func apiKeyView(key *APIKey) *EntitlementAPIKeyView {
	if key == nil {
		return nil
	}
	return &EntitlementAPIKeyView{
		ID:             key.ID,
		Name:           key.Name,
		Status:         key.Status,
		GroupID:        key.GroupID,
		QuotaRemaining: key.GetQuotaRemaining(),
		ExpiresAt:      key.ExpiresAt,
	}
}

func entitlementFallbackReason(balance float64) string {
	if balance > 0 {
		return "credit_balance_available"
	}
	return "insufficient_balance"
}

func providerIDForPlatform(platform string) string {
	if platform == "" {
		return ""
	}
	return "v-claw-" + platform
}

func targetGroupFromSwitchTarget(target EntitlementSwitchTarget) *EntitlementTargetGroup {
	return &EntitlementTargetGroup{
		GroupID:              target.GroupID,
		GroupName:            target.GroupName,
		GroupPlatform:        target.GroupPlatform,
		ProviderID:           target.ProviderID,
		SupportedModelScopes: append([]string(nil), target.SupportedModelScopes...),
		Source:               target.Reason,
	}
}

func (s *EntitlementService) buildSwitchTargets(ctx context.Context, user *User, keys []APIKey, currentKey *APIKey, items []EntitlementItem, req AutoSwitchEntitlementRequest) []EntitlementSwitchTarget {
	if user == nil || user.Balance <= 0 {
		return nil
	}
	var targets []EntitlementSwitchTarget
	if currentKey != nil && currentKey.GroupID != nil {
		for _, item := range items {
			if item.GroupID != *currentKey.GroupID || item.FallbackGroupID == nil {
				continue
			}
			if target, ok := s.switchTargetForGroup(ctx, user, keys, currentKey, *item.FallbackGroupID, req, "subscription_fallback_group", 100); ok {
				target.EstimatedBalanceUSD = user.Balance
				targets = append(targets, target)
			}
		}
	}
	if len(targets) == 0 {
		for _, key := range keys {
			if key.GroupID == nil {
				continue
			}
			if currentKey != nil && key.ID == currentKey.ID && !isUsableAPIKeyForSwitch(key) {
				continue
			}
			if target, ok := s.switchTargetForGroup(ctx, user, keys, &key, *key.GroupID, req, "active_balance_api_key", 50); ok {
				target.EstimatedBalanceUSD = user.Balance
				targets = append(targets, target)
				break
			}
		}
	}
	sort.SliceStable(targets, func(i, j int) bool {
		if targets[i].Priority != targets[j].Priority {
			return targets[i].Priority > targets[j].Priority
		}
		return targets[i].APIKeyID < targets[j].APIKeyID
	})
	return targets
}

func (s *EntitlementService) switchTargetForGroup(ctx context.Context, user *User, keys []APIKey, preferredKey *APIKey, groupID int64, req AutoSwitchEntitlementRequest, reason string, priority int) (EntitlementSwitchTarget, bool) {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil || group == nil || group.IsSubscriptionType() || !group.IsActive() || !user.CanBindGroup(group.ID, group.IsExclusive) {
		return EntitlementSwitchTarget{}, false
	}
	if req.ProviderID != "" && providerIDForPlatform(group.Platform) != req.ProviderID && !req.AllowProviderChange {
		return EntitlementSwitchTarget{}, false
	}
	key := preferredKey
	if key == nil || !isUsableAPIKeyForSwitch(*key) {
		if !req.AllowAPIKeyChange {
			return EntitlementSwitchTarget{}, false
		}
		key = firstUsableAPIKeyForGroup(keys, group.ID)
	}
	if key == nil {
		return EntitlementSwitchTarget{}, false
	}
	return EntitlementSwitchTarget{
		Mode:                 EntitlementModeBalance,
		APIKeyID:             key.ID,
		GroupID:              group.ID,
		GroupName:            group.Name,
		GroupPlatform:        group.Platform,
		ProviderID:           providerIDForPlatform(group.Platform),
		Priority:             priority,
		Reason:               reason,
		Switchable:           true,
		SupportedModelScopes: append([]string(nil), group.SupportedModelScopes...),
	}, true
}

func firstUsableAPIKeyForGroup(keys []APIKey, groupID int64) *APIKey {
	for i := range keys {
		if keys[i].GroupID == nil || *keys[i].GroupID != groupID {
			continue
		}
		if isUsableAPIKeyForSwitch(keys[i]) {
			return &keys[i]
		}
	}
	return nil
}

func isUsableAPIKeyForSwitch(key APIKey) bool {
	return key.Status == StatusActive && !key.IsExpired() && !key.IsQuotaExhausted()
}

func selectAutoSwitchCurrentAPIKey(keys []APIKey, requested *int64) *APIKey {
	if requested != nil {
		for i := range keys {
			if keys[i].ID == *requested {
				return &keys[i]
			}
		}
	}
	return selectCurrentAPIKey(keys)
}

func autoSwitchUnavailable(reason string) error {
	return ErrEntitlementAutoSwitchNotAvailable.WithMetadata(map[string]string{
		"reason": reason,
		"action": autoSwitchActionForReason(reason),
	})
}

func AutoSwitchUnavailableReason(err error) string {
	appErr := infraerrors.FromError(err)
	if appErr == nil || appErr.Metadata == nil {
		return ""
	}
	return appErr.Metadata["reason"]
}

func autoSwitchNoCandidateReason(currentKey *APIKey, user *User, req AutoSwitchEntitlementRequest) string {
	if currentKey != nil && (currentKey.Status == StatusAPIKeyQuotaExhausted || currentKey.IsQuotaExhausted()) {
		return "api_key_quota_exhausted_no_candidate"
	}
	if user == nil || user.Balance <= 0 {
		return "insufficient_balance"
	}
	if req.Reason != "" {
		return req.Reason + "_no_candidate"
	}
	return "no_switch_candidate"
}

func autoSwitchActionForReason(reason string) string {
	if reason == "insufficient_balance" {
		return "buy_credit"
	}
	return "open_settings"
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
