package service

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type openAIResetScopeKey struct{}
type openAIResetScope struct {
	request   OpenAIAccountScheduleRequest
	scheduler *defaultOpenAIAccountScheduler
	expires   time.Time
}

func notifyOpenAIAutoResetScoped(ctx context.Context, accountID int64) {
	openAIAutoResetNotifierRegistry.RLock()
	service := openAIAutoResetNotifierRegistry.service
	openAIAutoResetNotifierRegistry.RUnlock()
	if service == nil {
		return
	}
	if scope, ok := ctx.Value(openAIResetScopeKey{}).(openAIResetScope); ok && NormalizeOpenAICompatiblePlatform(scope.request.Platform) == PlatformOpenAI && scope.request.RequestedModel != "" {
		// Retain only scheduling metadata; never retain request context, auth,
		// session IDs, exclusions, or user data across the async boundary.
		req := scope.request
		scope.request = OpenAIAccountScheduleRequest{GroupID: req.GroupID, Platform: req.Platform, RequestedModel: req.RequestedModel, RequirePrivacySet: req.RequirePrivacySet, RequiredTransport: req.RequiredTransport, RequiredCapability: req.RequiredCapability, RequiredImageCapability: req.RequiredImageCapability, RequireCompact: req.RequireCompact}
		service.scopes.Store(accountID, scope)
	}

	service.Notify(accountID)
}

func (s *OpenAIQuotaAutoResetService) exhaustedCohort(ctx context.Context, target *Account, usage *OpenAIQuotaUsage, now time.Time) bool {
	scopeRaw, ok := s.scopes.Load(target.ID)
	if !ok {
		return false
	}
	scope, ok := scopeRaw.(openAIResetScope)
	if !ok {
		s.scopes.Delete(target.ID)
		return false
	}
	if !scope.expires.After(now) {
		s.scopes.Delete(target.ID)
		return false
	}
	req := scope.request
	if req.RequireCompact {
		return false
	}
	if req.GroupID != nil && scope.scheduler != nil && scope.scheduler.service != nil {
		snapshots := scope.scheduler.service.schedulerSnapshot
		if snapshots == nil {
			return false
		}
		group, err := snapshots.GetGroupByIDLite(ctx, *req.GroupID)
		if err != nil || group == nil || group.Status != StatusActive || group.ProfitControlEnabled {
			return false
		}
		req.RequirePrivacySet = group.RequirePrivacySet
	}
	eligible := func(a *Account) bool {
		if a.Platform != PlatformOpenAI || !a.IsActive() || !a.Schedulable || !openAIStickyAccountMatchesGroup(a, req.GroupID) {
			return false
		}
		ok, _ := scope.scheduler.isAccountStructurallyCompatibleReason(ctx, a, req)
		return ok
	}
	if !eligible(target) {
		return false
	}
	if !openAIResetNativeExhausted(usage, now) {
		return false
	}
	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return false
	}
	observations := []*OpenAIQuotaUsage{usage}
	found := false
	for i := range accounts {
		account := &accounts[i]
		if !eligible(account) {
			continue
		}
		if account.ID == target.ID {
			found = true
			continue
		}
		// API keys and shadow dimensions have no trustworthy parent-global quota
		// signal here. Their presence conservatively blocks automatic spending.
		if cfg := ResolveOpenAIAutoResetCreditConfig(account); cfg.Enabled && cfg.Mode == OpenAIAutoResetModeThreshold {
			return false
		}
		if !isOpenAIAutoResetCreditAccount(account) {
			return false
		}
		fresh, err := s.quota.QueryUsage(ctx, account.ID)
		if err != nil || fresh == nil {
			return false
		}
		observations = append(observations, fresh)
		if !openAIResetNativeExhausted(fresh, time.Now()) {
			return false
		}
	}
	for _, u := range observations {
		if !openAIResetNativeExhausted(u, time.Now()) {
			return false
		}
	}
	return found
}

// A provider-wide lease serializes the cohort observation through redemption
// and post-reset observation across processes. Without coordination fail closed.
func (s *OpenAIQuotaAutoResetService) acquireResetPolicyLease(ctx context.Context) (func(), bool) {
	if s.leaderLock == nil {
		return nil, false
	}
	owner := uuid.NewString()
	const key = "jobs:openai-auto-reset-credit:provider-policy"
	ok, err := s.leaderLock.TryAcquireLeaderLock(ctx, key, owner, 65*time.Second)
	if err != nil || !ok {
		return nil, false
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.leaderLock.ReleaseLeaderLock(releaseCtx, key, owner)
	}, true
}

func openAIResetNativeExhausted(usage *OpenAIQuotaUsage, now time.Time) bool {
	if usage == nil || usage.RateLimit == nil || usage.FetchedAt <= 0 {
		return false
	}
	fetched := time.Unix(usage.FetchedAt, 0)
	if fetched.After(now) || now.Sub(fetched) >= openAIAutoResetSnapshotTTL {
		return false
	}
	for _, window := range []*OpenAIRateLimitWindow{usage.RateLimit.PrimaryWindow, usage.RateLimit.SecondaryWindow} {
		if window == nil || (window.LimitWindowSeconds != 18000 && window.LimitWindowSeconds != 604800) {
			continue
		}
		reset := window.ResetAt
		if reset <= 0 && window.ResetAfterSeconds > 0 {
			reset = usage.FetchedAt + window.ResetAfterSeconds
		}
		if window.UsedPercent == 100 && reset > now.Unix() {
			return true
		}
	}
	return false
}
