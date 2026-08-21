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
REDACTED

func classifyOpenAIAPIKeyHealthFailure(err error) (int, []byte, bool) {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return 0, nil, false
REDACTED

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
	REDACTED
		if failoverErr.StatusCode == http.StatusTooManyRequests || failoverErr.StatusCode >= http.StatusInternalServerError {
			return failoverErr.StatusCode, failoverErr.ResponseBody, true
	REDACTED
		return failoverErr.StatusCode, failoverErr.ResponseBody, false
REDACTED

	var imageErr *OpenAIImagesUpstreamError
	if errors.As(err, &imageErr) {
		if imageErr.StatusCode == http.StatusTooManyRequests || imageErr.StatusCode >= http.StatusInternalServerError {
			return imageErr.StatusCode, []byte(strings.TrimSpace(imageErr.Message)), true
	REDACTED
REDACTED
	return 0, nil, false
REDACTED

func (s *RateLimitService) ObserveOpenAIAPIKeyHealthFailure(ctx context.Context, account *Account, upstreamErr error) bool {
	if s == nil || s.openAIAPIKeyHealth == nil || s.settingService == nil || s.accountRepo == nil || !isOpenAIAPIKeyHealthBreakerAccount(account) {
		return false
REDACTED
	statusCode, responseBody, eligible := classifyOpenAIAPIKeyHealthFailure(upstreamErr)
	if !eligible {
		return false
REDACTED
	settings, err := s.settingService.GetOpenAIAPIKeyHealthBreakerSettings(ctx)
	if err != nil {
		logger.L().Warn("openai.apikey_health_breaker_settings_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return false
REDACTED
	if settings == nil || !settings.Enabled {
		return false
REDACTED

	count, tripped, err := s.openAIAPIKeyHealth.RecordOpenAIAPIKeyHealthFailure(ctx, account.ID, settings.WindowMinutes, settings.FailureThreshold)
	if err != nil {
		logger.L().Warn("openai.apikey_health_breaker_record_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return false
REDACTED
	if !tripped {
		return false
REDACTED

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
REDACTED
	reasonBytes, _ := json.Marshal(state)
	reason := string(reasonBytes)
	if reason == "" {
		reason = fmt.Sprintf("%s: %d failures in %d minute(s)", openAIAPIKeyHealthBreakerReason, count, settings.WindowMinutes)
REDACTED

	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := s.accountRepo.SetTempUnschedulable(persistCtx, account.ID, until, reason); err != nil {
		logger.L().Warn("openai.apikey_health_breaker_persist_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return false
REDACTED

	if account.TempUnschedulableUntil == nil || account.TempUnschedulableUntil.Before(until) {
		account.TempUnschedulableUntil = &until
		account.TempUnschedulableReason = reason
REDACTED
	s.notifyAccountSchedulingBlocked(account, until, openAIAPIKeyHealthBreakerReason)
	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.SetTempUnsched(persistCtx, account.ID, state); err != nil {
			logger.L().Warn("openai.apikey_health_breaker_cache_failed", zap.Int64("account_id", account.ID), zap.Error(err))
	REDACTED
REDACTED
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
REDACTED

func (s *RateLimitService) ObserveOpenAIAPIKeyHealthSuccess(context.Context, *Account) {
	// Health failures are accumulated in a rolling time window. A success does
	// not reset that window and must not add a Redis round trip to the hot path.
REDACTED
