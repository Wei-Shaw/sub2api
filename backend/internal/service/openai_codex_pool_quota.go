package service

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	OpenAICodexQuotaCooldownExtraKey = "codex_quota_cooldown"
	openAICodexSparkQuotaReason      = "openai_codex_spark_quota_exhausted"
)

// CodexPoolQuotaRequest uses the same route inputs as the account scheduler.
// FailedAccounts identifies encountered failures, never accounts skipped by an attempt budget.
type CodexPoolQuotaRequest struct {
	GroupID                 *int64
	Model                   string
	PreviousResponseID      string
	PreviousResponseCanMove bool
	Transport               OpenAIUpstreamTransport
	Capability              OpenAIEndpointCapability
	FailedAccounts          map[int64]struct{}
}

// EarliestCodexPoolQuotaReset returns zero when a candidate can still serve
// the request or all blocked candidates have an unknown recovery time.
func (s *OpenAIGatewayService) EarliestCodexPoolQuotaReset(ctx context.Context, req CodexPoolQuotaRequest) time.Time {
	if s == nil || s.accountRepo == nil || req.GroupID == nil || req.Model == "" {
		return time.Time{}
	}
	includeGrouped := s.cfg != nil && s.cfg.RunMode == config.RunModeSimple
	groupID := req.GroupID
	if includeGrouped {
		groupID = nil
	}
	if s.checkChannelPricingRestriction(ctx, req.GroupID, req.Model) {
		return time.Time{}
	}
	var pinnedAccountID int64
	if req.PreviousResponseID != "" && !req.PreviousResponseCanMove {
		var err error
		pinnedAccountID, err = s.getOpenAIWSStateStore().GetResponseAccount(ctx, *req.GroupID, req.PreviousResponseID)
		if err != nil || pinnedAccountID == 0 {
			return time.Time{}
		}
	}
	accounts, err := s.accountRepo.ListModelAvailabilityCandidates(ctx, groupID, []string{PlatformOpenAI}, includeGrouped)
	if err != nil {
		return time.Time{}
	}
	ctx = s.withOpenAIQuotaAutoPauseContext(ctx)
	ctx = s.withOpenAIGroupPrivacyRequirement(ctx, req.GroupID)
	checkUpstream := s.needsUpstreamChannelRestrictionCheck(ctx, req.GroupID)
	var thresholds map[string]int
	if s.rateLimitService != nil && s.rateLimitService.settingService != nil {
		thresholds = s.rateLimitService.settingService.GetAccountSchedulingThresholds(ctx)
	}
	imageSize, _ := openAIImageSizeRoutingFromContext(ctx)
	imageTier, explicitImageTier := ClassifyImageBillingTier(imageSize)
	now := time.Now()
	var earliest time.Time
	for i := range accounts {
		account := &accounts[i]
		if pinnedAccountID != 0 && account.ID != pinnedAccountID ||
			s.openAIGroupRequiresPrivacySet(ctx, req.GroupID) && !account.IsPrivacySet() ||
			!s.isOpenAIAccountTransportCompatible(account, req.Transport) ||
			explicitImageTier && !account.SupportsOpenAIImageSizeTier(imageTier) ||
			!parentHealthyForShadow(account, s.parentAccountLookup(ctx)) ||
			s.isOpenAIProxyStreamQuarantined(ctx, account) ||
			checkUpstream && s.isUpstreamModelRestrictedByChannel(ctx, *req.GroupID, account, req.Model, false) {
			continue
		}
		reset, admissible := s.codexQuotaCandidateReset(ctx, account, req, thresholds, now)
		_, failed := req.FailedAccounts[account.ID]
		if admissible && !failed {
			return time.Time{}
		}
		if !reset.IsZero() && (earliest.IsZero() || reset.Before(earliest)) {
			earliest = reset
		}
	}
	return earliest
}

// A zero reset for a blocked account represents infinity. Check all blockers
// before recording a finite timestamp, since they must all clear on that account.
func (s *OpenAIGatewayService) codexQuotaCandidateReset(ctx context.Context, account *Account, req CodexPoolQuotaRequest, thresholds map[string]int, now time.Time) (time.Time, bool) {
	remaining := *account
	var reset time.Time
	if current := account.RateLimitResetAt; current != nil && current.After(now) {
		reset = recordedCodexQuotaReset(account, now)
		if reset.IsZero() {
			return time.Time{}, false
		}
		remaining.RateLimitResetAt = nil
	}
	for _, key := range account.modelRateLimitKeysForRequest(ctx, req.Model) {
		if current := account.modelRateLimitResetAt(key); current != nil && current.After(now) {
			if account.modelRateLimitReason(key) != openAICodexSparkQuotaReason {
				return time.Time{}, false
			}
			reset = laterQuotaReset(reset, *current)
			limits, _ := remaining.Extra[modelRateLimitsKey].(map[string]any)
			clonedLimits := maps.Clone(limits)
			delete(clonedLimits, key)
			remaining.Extra = maps.Clone(remaining.Extra)
			remaining.Extra[modelRateLimitsKey] = clonedLimits
		}
	}
	if until, ok := s.openaiAccountRuntimeBlockUntil.Load(account.ID); ok {
		if deadline, ok := until.(time.Time); ok && deadline.After(now) &&
			deadline.Truncate(time.Microsecond).After(reset) && accountPersistedSchedulingCooldownActive(account) {
			return time.Time{}, false
		}
	}
	if s.isOpenAIAccountModelRuntimeBlocked(account, req.Model) {
		return time.Time{}, false
	}

	for decision := range openAIAccountQuotaPauseDecisions(ctx, account, now) {
		if decision.reason != "" && !openAIAutoResetNoCreditConfirmed(openAIAutoResetStateFromExtra(account.Extra), now) {
			// A reset may be evaluated or redeemed before the natural window ends.
			return time.Time{}, false
		}
		windowReset := codexPausedWindowReset(account, decision.window, decision.utilization, now)
		if windowReset.IsZero() {
			return time.Time{}, false
		}
		reset = laterQuotaReset(reset, windowReset)
	}
	if threshold, ok := resolveEffectiveAccountSchedulingThreshold(account, thresholds, PlatformOpenAI); thresholds != nil && ok && threshold < 100 {
		for _, candidate := range openAIThresholdCandidates(account, now) {
			if !candidateMatchesThreshold(candidate, threshold, now) {
				continue
			}
			windowReset := codexPausedWindowReset(account, candidate.window, candidate.usedPercent/100, now)
			if windowReset.IsZero() {
				return time.Time{}, false
			}
			reset = laterQuotaReset(reset, windowReset)
			if IsAccountSchedulingThresholdReason(remaining.TempUnschedulableReason) &&
				remaining.TempUnschedulableUntil != nil && !remaining.TempUnschedulableUntil.After(reset) {
				remaining.TempUnschedulableUntil = nil
			}
		}
	}
	if openAICompatibleAccountEligibilityFailureReasonWithQuotaPause(ctx, &remaining, PlatformOpenAI, req.Model, false, req.Capability, false) != "" {
		return time.Time{}, false
	}
	if vetoed, _ := openAIProfitControlVetoReason(ctx, &remaining); vetoed {
		return time.Time{}, false
	}
	if reset.IsZero() {
		return time.Time{}, true
	}
	if !isOpenAIOAuthAccount(account) {
		return time.Time{}, false
	}
	if state := openAIAutoResetStateFromExtra(account.Extra); state != nil && !openAIAutoResetStateStale(state, now) &&
		(state.Status == OpenAIAutoResetStatusChecking || state.Status == OpenAIAutoResetStatusResetting) {
		return time.Time{}, false
	}
	// An additional exhausted window can outlast the one named by the last 429.
	if (!isCodexSparkModel(account.GetMappedModel(req.Model)) || account.IsShadow()) &&
		openAICodexSnapshotIdentityTrusted(account) && !codexAccountHasSpendableCredits(account) {
		for _, window := range []string{"5h", "7d"} {
			if utilization, ok := resolveOpenAIQuotaUtilization(account.Extra, window, now); ok && utilization >= 1 {
				windowReset := codexPausedWindowReset(account, window, utilization, now)
				if windowReset.IsZero() {
					return time.Time{}, false
				}
				reset = laterQuotaReset(reset, windowReset)
			}
		}
	}
	if account.AutoPauseOnExpired && account.ExpiresAt != nil && !account.ExpiresAt.After(reset) {
		return time.Time{}, false
	}
	return reset, false
}

func codexPausedWindowReset(account *Account, window string, utilizationRatio float64, now time.Time) time.Time {
	if utilizationRatio < 1 || !openAICodexSnapshotIdentityTrusted(account) || codexAccountHasSpendableCredits(account) {
		return time.Time{}
	}
	if reset, ok := openAICodexWindowResetAt(account.Extra, window); ok && reset.After(now) {
		return reset
	}
	return time.Time{}
}

func laterQuotaReset(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func codexHeadersHaveSpendableCredits(headers http.Header) bool {
	hasCredits, _ := strconv.ParseBool(headers.Get("x-codex-credits-has-credits"))
	unlimited, _ := strconv.ParseBool(headers.Get("x-codex-credits-unlimited"))
	balance, _ := strconv.ParseFloat(strings.TrimSpace(headers.Get("x-codex-credits-balance")), 64)
	return hasCredits || unlimited || balance > 0
}

func codexAccountHasSpendableCredits(account *Account) bool {
	raw, err := json.Marshal(account.Extra[openaiQuotaCreditsKey])
	if err != nil {
		return false
	}
	var snapshot openAICreditsSnapshot
	if json.Unmarshal(raw, &snapshot) != nil || snapshot.Credits == nil {
		return false
	}
	credits := snapshot.Credits
	if credits.HasCredits || credits.Unlimited {
		return true
	}
	if credits.Balance != nil {
		balance, _ := strconv.ParseFloat(strings.TrimSpace(*credits.Balance), 64)
		return balance > 0
	}
	return false
}

// Store the observation with the cooldown generation. An ordinary 429 or an
// account reset that changes or clears either timestamp invalidates it.
func recordedCodexQuotaReset(account *Account, now time.Time) time.Time {
	raw, ok := account.Extra[OpenAICodexQuotaCooldownExtraKey].(map[string]any)
	if !ok || account.RateLimitedAt == nil || account.RateLimitResetAt == nil || !account.RateLimitResetAt.After(now) {
		return time.Time{}
	}
	limitedAt, _ := time.Parse(time.RFC3339Nano, stringValue(raw["limited_at"]))
	cooldown, _ := time.Parse(time.RFC3339Nano, stringValue(raw["cooldown_until"]))
	reset, _ := time.Parse(time.RFC3339Nano, stringValue(raw["reset_at"]))
	explicit, _ := raw["explicit"].(bool)
	if !limitedAt.Equal(*account.RateLimitedAt) || !cooldown.Equal(*account.RateLimitResetAt) ||
		!reset.After(now) || !explicit && codexAccountHasSpendableCredits(account) {
		return time.Time{}
	}
	return laterQuotaReset(reset, cooldown)
}

func codexQuota429Reset(account *Account, headers http.Header, body []byte, now time.Time) (time.Time, bool) {
	if !isOpenAIOAuthAccount(account) || account.IsShadow() {
		return time.Time{}, false
	}
	typ := gjson.GetBytes(body, "error.type").String()
	if typ == "" {
		typ = gjson.GetBytes(body, "response.error.type").String()
		if typ != "" {
			body = []byte(gjson.GetBytes(body, "response").Raw)
		}
	}
	explicit := typ == "usage_limit_reached"
	var reset time.Time
	if explicit {
		if unix := parseOpenAIRateLimitResetTime(body); unix != nil && time.Unix(*unix, 0).After(now) && *unix <= 253402300799 {
			reset = time.Unix(*unix, 0)
		}
	} else if typ != "" ||
		firstNonEmpty(gjson.GetBytes(body, "error.code").String(), gjson.GetBytes(body, "response.error.code").String()) != "" ||
		codexAccountHasSpendableCredits(account) || codexHeadersHaveSpendableCredits(headers) {
		return time.Time{}, false
	}
	snapshot := ParseCodexRateLimitHeaders(headers)
	if snapshot == nil {
		return reset, explicit
	}
	normalized := snapshot.Normalize()
	if normalized == nil {
		return reset, explicit
	}
	for _, window := range []struct {
		used    *float64
		seconds *int
	}{
		{normalized.Used5hPercent, normalized.Reset5hSeconds},
		{normalized.Used7dPercent, normalized.Reset7dSeconds},
	} {
		if window.used == nil || *window.used < 100 {
			continue
		}
		if window.seconds == nil {
			return time.Time{}, explicit
		}
		if int64(*window.seconds) > int64(time.Duration(1<<63-1)/time.Second) {
			return time.Time{}, explicit
		}
		if *window.seconds > 0 {
			reset = laterQuotaReset(reset, now.Add(time.Duration(*window.seconds)*time.Second))
		}
	}
	return reset, explicit
}

func (s *RateLimitService) setOpenAICodexRateLimited(ctx context.Context, account *Account, cooldown time.Time, headers http.Header, body []byte) error {
	if limit := strings.TrimSpace(headers.Get("x-codex-active-limit")); limit != "" && !strings.EqualFold(limit, "codex") {
		return s.accountRepo.SetRateLimited(ctx, account.ID, cooldown)
	}
	reset, explicit := codexQuota429Reset(account, headers, body, time.Now())
	if repo, ok := s.accountRepo.(interface {
		SetCodexQuotaRateLimited(context.Context, int64, time.Time, time.Time, bool) error
	}); ok && !reset.IsZero() {
		return repo.SetCodexQuotaRateLimited(ctx, account.ID, cooldown, reset, explicit)
	}
	return s.accountRepo.SetRateLimited(ctx, account.ID, cooldown)
}

// A continuation tied to upstream state cannot establish pool availability.
func openAIWSQuotaRouteIndependent(payload []byte) bool {
	if len(payload) == 0 || openAIWSPayloadStringFromRaw(payload, "previous_response_id") != "" {
		return false
	}
	coverage := AnalyzeToolCallOutputContextCoverageBytes(payload)
	return !coverage.HasFunctionCallOutput || coverage.ContextCoversAllCallIDs
}

type codexPoolQuotaHTTPKey struct{}

// WithCodexPoolQuotaHTTP reserves the HTTP status until the first semantic
// output. Heartbeats themselves would commit a 200 response.
func WithCodexPoolQuotaHTTP(ctx context.Context) context.Context {
	return context.WithValue(ctx, codexPoolQuotaHTTPKey{}, true)
}

func deferCodexPoolQuotaHTTPCommit(c *gin.Context) bool {
	return c != nil && c.Request != nil && c.Request.Context().Value(codexPoolQuotaHTTPKey{}) == true
}
