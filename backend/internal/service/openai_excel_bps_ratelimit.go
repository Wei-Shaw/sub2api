package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// ExcelBPSRateLimitedReason marks a BPS 429 that another account may still
// serve. It doubles as the client-facing error code once failover is exhausted.
const ExcelBPSRateLimitedReason = GatewayFailureReason("basispoints_rate_limited")

const excelBPSRateLimitedClientMessage = "Excel BPS rate limit exceeded, please retry later"

// BPS throttles its own endpoint independently of the account's Codex quota.
// A BPS 429 is therefore handed back to the handler's account-switch loop
// before any response byte is written, and cools only this account's BPS
// route. Natively forwarded models, persisted quota fields and the shared
// scheduler health score are left alone. Like other runtime blocks, the
// cooldown is local to this process.
func newExcelBPSRateLimitedFailoverError(retryAfter string) *UpstreamFailoverError {
	failoverErr := &UpstreamFailoverError{
		StatusCode:        http.StatusTooManyRequests,
		Stage:             GatewayFailureStageInference,
		Scope:             GatewayFailureScopeAccount,
		Reason:            ExcelBPSRateLimitedReason,
		NextAccountAction: NextAccountRetry,
		ClientStatusCode:  http.StatusTooManyRequests,
		ClientMessage:     excelBPSRateLimitedClientMessage,
	}
	// The BPS body may echo request data and is never forwarded. Only a valid
	// Retry-After survives; the handler checks it again before responding.
	if _, ok := excelBPSRetryAfter(retryAfter, time.Now()); ok {
		failoverErr.ResponseHeaders = http.Header{"Retry-After": {strings.TrimSpace(retryAfter)}}
	}
	return failoverErr
}

// coolDownExcelBPS skips the account for BPS requests until the upstream
// Retry-After, or the configured 429 fallback when none is given. A shorter
// cooldown never replaces a longer one.
func (s *OpenAIGatewayService) coolDownExcelBPS(ctx context.Context, account *Account, retryAfter string) {
	if s == nil || account == nil {
		return
	}
	cooldown, ok := excelBPSRetryAfter(retryAfter, time.Now())
	if !ok {
		if cooldown, ok = s.excelBPS429FallbackCooldown(ctx, account); !ok {
			return
		}
	}
	// Same bounds as the configurable 429 fallback.
	if cooldown < time.Second {
		cooldown = time.Second
	}
	if limit := maxRateLimit429CooldownSeconds * time.Second; cooldown > limit {
		cooldown = limit
	}
	until := time.Now().Add(cooldown)
	for {
		current, loaded := s.excelBPSCooldownUntil.LoadOrStore(account.ID, until)
		if !loaded {
			break
		}
		if currentUntil, valid := current.(time.Time); valid && !until.After(currentUntil) {
			return
		}
		if s.excelBPSCooldownUntil.CompareAndSwap(account.ID, current, until) {
			break
		}
	}
	logger.LegacyPrintf("service.openai_excel_bps", "rate limited; skipping account for BPS requests: account_id=%d cooldown=%s", account.ID, cooldown)
}

func (s *OpenAIGatewayService) excelBPS429FallbackCooldown(ctx context.Context, account *Account) (time.Duration, bool) {
	if s.rateLimitService == nil {
		return defaultRateLimit429CooldownSeconds * time.Second, true
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	return s.rateLimitService.get429FallbackCooldown(stateCtx, account)
}

// excelBPSRetryAfter accepts delta-seconds or an HTTP date.
func excelBPSRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return 0, false
	}
	if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
		return time.Duration(seconds) * time.Second, true
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		return retryAt.Sub(now), true
	}
	return 0, false
}

// isExcelBPSCoolingDown reports a BPS cooldown only for requests this account
// would route through BPS.
func (s *OpenAIGatewayService) isExcelBPSCoolingDown(account *Account, requestedModel string) bool {
	if s == nil || account == nil {
		return false
	}
	value, ok := s.excelBPSCooldownUntil.Load(account.ID)
	if !ok {
		return false
	}
	if until, valid := value.(time.Time); valid && time.Now().Before(until) {
		return account.IsExcelBPSEnabledForModel(requestedModel)
	}
	s.excelBPSCooldownUntil.CompareAndDelete(account.ID, value)
	return false
}
