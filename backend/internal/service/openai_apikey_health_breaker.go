package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const openAIAPIKeyHealthBreakerReason = "openai_apikey_health_breaker"

func isOpenAIAPIKeyHealthBreakerAccount(account *Account) bool {
	return account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeAPIKey && account.IsPoolMode()
}

func classifyOpenAIAPIKeyHealthFailure(err error) (int, []byte, bool) {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return 0, nil, false
	}

	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		// These failures already have dedicated recovery/state handling or are not
		// attributable to the selected account.
		if failoverErr.IsCredentialFailure() ||
			failoverErr.RequestScopedTransient ||
			failoverErr.RetryableOnSameAccount ||
			failoverErr.Scope == GatewayFailureScopeRequest ||
			failoverErr.Scope == GatewayFailureScopeProvider {
			return failoverErr.StatusCode, failoverErr.ResponseBody, false
		}
		if failoverErr.StatusCode == http.StatusTooManyRequests || failoverErr.StatusCode >= http.StatusInternalServerError {
			return failoverErr.StatusCode, failoverErr.ResponseBody, true
		}
		return failoverErr.StatusCode, failoverErr.ResponseBody, false
	}

	var imageErr *OpenAIImagesUpstreamError
	if errors.As(err, &imageErr) {
		if imageErr.StatusCode == http.StatusTooManyRequests || imageErr.StatusCode >= http.StatusInternalServerError {
			return imageErr.StatusCode, []byte(strings.TrimSpace(imageErr.Message)), true
		}
	}
	return 0, nil, false
}

func (s *RateLimitService) ObserveOpenAIAPIKeyHealthFailure(ctx context.Context, account *Account, upstreamErr error) bool {
	if s == nil || s.openAIAPIKeyHealth == nil || s.settingService == nil || s.accountRepo == nil || !isOpenAIAPIKeyHealthBreakerAccount(account) {
		return false
	}
	statusCode, responseBody, eligible := classifyOpenAIAPIKeyHealthFailure(upstreamErr)
	if !eligible {
		return false
	}
	settings, err := s.settingService.GetOpenAIAPIKeyHealthBreakerSettings(ctx)
	if err != nil {
		logger.L().Warn("openai.apikey_health_breaker_settings_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return false
	}
	if settings == nil || !settings.Enabled {
		return false
	}

	count, tripped, err := s.openAIAPIKeyHealth.RecordOpenAIAPIKeyHealthFailure(ctx, account.ID, settings.WindowMinutes, settings.FailureThreshold)
	if err != nil {
		logger.L().Warn("openai.apikey_health_breaker_record_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return false
	}
	if !tripped {
		return false
	}

	now := time.Now()
	until := now.Add(time.Duration(settings.CooldownMinutes) * time.Minute)
	state := &TempUnschedState{
		UntilUnix:            until.Unix(),
		TriggeredAtUnix:      now.Unix(),
		StatusCode:           statusCode,
		MatchedKeyword:       openAIAPIKeyHealthBreakerReason,
		RuleIndex:            -1,
		ErrorMessage:         truncateTempUnschedMessage(responseBody, tempUnschedMessageMaxBytes),
		TriggerCount:         count,
		TriggerThreshold:     settings.FailureThreshold,
		TriggerWindowMinutes: settings.WindowMinutes,
	}
	reasonBytes, _ := json.Marshal(state)
	reason := string(reasonBytes)
	if reason == "" {
		reason = fmt.Sprintf("%s: %d failures in %d minute(s)", openAIAPIKeyHealthBreakerReason, count, settings.WindowMinutes)
	}

	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := s.accountRepo.SetTempUnschedulable(persistCtx, account.ID, until, reason); err != nil {
		logger.L().Warn("openai.apikey_health_breaker_persist_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return false
	}

	if account.TempUnschedulableUntil == nil || account.TempUnschedulableUntil.Before(until) {
		account.TempUnschedulableUntil = &until
		account.TempUnschedulableReason = reason
	}
	s.notifyAccountSchedulingBlocked(account, until, openAIAPIKeyHealthBreakerReason)
	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.SetTempUnsched(persistCtx, account.ID, state); err != nil {
			logger.L().Warn("openai.apikey_health_breaker_cache_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		}
	}
	logger.L().Warn("openai.apikey_health_breaker_tripped",
		zap.Int64("account_id", account.ID),
		zap.Int64("failure_count", count),
		zap.Int("failure_threshold", settings.FailureThreshold),
		zap.Int("window_minutes", settings.WindowMinutes),
		zap.Int("cooldown_minutes", settings.CooldownMinutes),
		zap.Int("upstream_status", statusCode),
		zap.Time("until", until),
	)
	return true
}

func (s *RateLimitService) ObserveOpenAIAPIKeyHealthSuccess(context.Context, *Account) {
	// Health failures are accumulated in a rolling time window. A success does
	// not reset that window and must not add a Redis round trip to the hot path.
}
