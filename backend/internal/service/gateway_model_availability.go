package service

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// ModelAvailabilityDiagnosis describes whether the requested model can be
// served by any persistently eligible account in the group (active with its
// schedulable setting enabled). The candidate query does not filter transient
// state, allowing diagnosis to distinguish unsupported models, pure full-pool
// rate limiting, and other temporary unavailability.
type ModelAvailabilityDiagnosis struct {
	// HasAccountsInPool is true if the group has at least one persistently
	// eligible account on the queried platform (or, for Anthropic/Gemini, on
	// the platform plus mixed-scheduled Antigravity accounts).
	HasAccountsInPool bool
	// HasModelSupport is true if at least one account's model mapping admits
	// the requested model.
	HasModelSupport bool
	// AllMatchingAccountsRateLimited is true only when every account that can
	// serve the requested model is unavailable solely because of an active
	// account-level or model-level rate limit.
	AllMatchingAccountsRateLimited bool
	// MinRateLimitResetAt is the earliest time at which one of those accounts
	// can become schedulable again.
	MinRateLimitResetAt *time.Time
}

// ModelAvailabilityDiagnoser is implemented by gateway services that can
// report whether the requested model is configured to be served by any
// account. Both *GatewayService and *OpenAIGatewayService implement this so
// handlers in either package can share a single classifier.
type ModelAvailabilityDiagnoser interface {
	DiagnoseModelAvailabilityForPlatform(
		ctx context.Context,
		groupID *int64,
		requestedModel string,
		platform string,
	) ModelAvailabilityDiagnosis
}

// DiagnoseModelAvailabilityForPlatform inspects accounts enabled for scheduling
// by persistent configuration and returns whether the requested model is
// configured to be served by any of them. The dedicated repository query
// bypasses scheduler snapshots and retains transient fields so this layer can
// recognize a pool blocked exclusively by known rate-limit reset times.
//
// Safe to call on the error path: returns {true,true} on any internal failure
// or when the inputs preclude meaningful diagnosis (empty model, etc.), so
// callers stay on the 503 fallback branch.
func (s *GatewayService) DiagnoseModelAvailabilityForPlatform(
	ctx context.Context,
	groupID *int64,
	requestedModel string,
	platform string,
) ModelAvailabilityDiagnosis {
	if s == nil {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		// No model specified — cannot decide model_not_found. Caller falls back to 503.
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}
	if strings.TrimSpace(platform) == "" {
		// Without a platform we cannot scope the lookup; bail out to the
		// 503 branch rather than make an unscoped scan.
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}

	if s.accountRepo == nil {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}

	useMixed := platform == PlatformAnthropic || platform == PlatformGemini
	platforms := []string{platform}
	if useMixed {
		platforms = append(platforms, PlatformAntigravity)
	}

	queryGroupID := groupID
	includeGrouped := false
	if useMixed {
		// Preserve the generic scheduler's scope rules: an explicit group wins
		// for mixed scheduling, while group-less simple mode scans all accounts.
		if groupID == nil && s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
			includeGrouped = true
		}
	} else if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		queryGroupID = nil
		includeGrouped = true
	}

	accounts, err := s.accountRepo.ListModelAvailabilityCandidates(ctx, queryGroupID, platforms, includeGrouped)
	if err != nil {
		// Conservative fallback: pretend everything is fine so the caller
		// returns 503 (we don't want to flip to 404 just because a lookup
		// hiccup'd).
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}

	return diagnoseModelAvailabilityCandidates(
		ctx,
		accounts,
		requestedModel,
		platform == PlatformAnthropic,
		func(account *Account) bool {
			return !useMixed || account.Platform != PlatformAntigravity || account.IsMixedSchedulingEnabled()
		},
		func(account *Account, model string) bool {
			return s.isModelSupportedByAccountWithContext(ctx, account, model)
		},
	)
}

// diagnoseModelAvailabilityCandidates summarizes one already platform-scoped
// candidate pool. Platform-specific services provide only their inclusion and
// model-matching rules; full-pool rate-limit semantics remain shared.
func diagnoseModelAvailabilityCandidates(
	ctx context.Context,
	accounts []Account,
	requestedModel string,
	classifyFullPoolRateLimit bool,
	include func(*Account) bool,
	supportsModel func(*Account, string) bool,
) ModelAvailabilityDiagnosis {
	diag := ModelAvailabilityDiagnosis{}
	matchingAccounts := 0
	rateLimitedAccounts := 0
	var minResetAt time.Time
	now := time.Now()

	for i := range accounts {
		account := &accounts[i]
		if include != nil && !include(account) {
			continue
		}
		diag.HasAccountsInPool = true
		if supportsModel == nil || !supportsModel(account, requestedModel) {
			continue
		}
		diag.HasModelSupport = true
		if !classifyFullPoolRateLimit {
			return diag
		}

		matchingAccounts++
		resetAt, onlyRateLimited := accountOnlyRateLimitedUntil(ctx, account, requestedModel, now)
		if !onlyRateLimited {
			continue
		}
		rateLimitedAccounts++
		if minResetAt.IsZero() || resetAt.Before(minResetAt) {
			minResetAt = resetAt
		}
	}

	if matchingAccounts > 0 && rateLimitedAccounts == matchingAccounts && !minResetAt.IsZero() {
		diag.AllMatchingAccountsRateLimited = true
		diag.MinRateLimitResetAt = &minResetAt
	}
	return diag
}

// accountOnlyRateLimitedUntil returns the time at which the account can be
// retried when rate limiting is its only active transient blocker. If another
// blocker is active, the second return value is false so callers keep the
// generic service-unavailable response.
func accountOnlyRateLimitedUntil(ctx context.Context, account *Account, requestedModel string, now time.Time) (time.Time, bool) {
	if account == nil {
		return time.Time{}, false
	}
	if account.OverloadUntil != nil && account.OverloadUntil.After(now) {
		return time.Time{}, false
	}
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(now) {
		return time.Time{}, false
	}
	if account.AutoPauseOnExpired && account.ExpiresAt != nil && !now.Before(*account.ExpiresAt) {
		return time.Time{}, false
	}
	if account.IsAPIKeyOrBedrock() && account.IsQuotaExceeded() {
		return time.Time{}, false
	}

	var resetAt time.Time
	if account.RateLimitResetAt != nil && account.RateLimitResetAt.After(now) {
		resetAt = *account.RateLimitResetAt
	}
	if remaining := account.GetRateLimitRemainingTimeWithContext(ctx, requestedModel); remaining > 0 {
		modelResetAt := now.Add(remaining)
		if resetAt.IsZero() || modelResetAt.After(resetAt) {
			resetAt = modelResetAt
		}
	}
	return resetAt, !resetAt.IsZero()
}
