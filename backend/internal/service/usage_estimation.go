package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const (
	UsageSourceUpstream                  = "upstream"
	UsageSourceEstimated                 = "estimated"
	usageObservedResponseBytesContextKey = "usage_observed_response_bytes"
	usagePresentContextKey               = "usage_present"
	missingUsageAccountCooldown          = 10 * time.Minute
	missingUsageThreshold                = 3
)

type missingUsageObservation struct { count int; first time.Time; cooldownUntil time.Time }
var missingUsageState sync.Map

// estimateTokensFromBytes is deliberately conservative about the accounting
// fallback: it charges a small lower bound for every non-empty payload and
// uses the common roughly-four-bytes-per-token approximation for text.
func estimateTokensFromBytes(payloadBytes int) int {
	if payloadBytes <= 0 {
		return 0
	}
	tokens := (payloadBytes + 3) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func finalizeOpenAIUsageEstimate(result *OpenAIForwardResult) {
	if result != nil && (result.ImageCount > 0 || result.VideoCount > 0 || result.WebSearchCalls > 0 || result.SearchCount > 0 || result.AudioUsage != nil) {
		return
	}
	if result == nil {
		return
	}
	if result.UsagePresent || result.UsageSource == UsageSourceUpstream {
		result.UsageSource = UsageSourceUpstream
		return
	}
	if result.UsageSource == "" && openAIUsageHasTokens(&result.Usage) {
		result.UsageSource = UsageSourceUpstream
		return
	}
	inputTokens := estimateTokensFromBytes(result.RequestBytes)
	outputTokens := estimateTokensFromBytes(result.ObservedResponseBytes)
	if inputTokens == 0 && outputTokens == 0 {
		return
	}
	result.Usage = OpenAIUsage{InputTokens: inputTokens, OutputTokens: outputTokens}
	result.UsageSource = UsageSourceEstimated
}

func finalizeClaudeUsageEstimate(result *ForwardResult) {
	if result == nil {
		return
	}
	if result.UsagePresent || result.UsageSource == UsageSourceUpstream {
		result.UsageSource = UsageSourceUpstream
		return
	}
	if result.UsageSource == "" && result.Usage.hasObservedTokens() {
		result.UsageSource = UsageSourceUpstream
		return
	}
	inputTokens := estimateTokensFromBytes(result.RequestBytes)
	outputTokens := estimateTokensFromBytes(result.ObservedResponseBytes)
	if inputTokens == 0 && outputTokens == 0 {
		return
	}
	result.Usage = ClaudeUsage{InputTokens: inputTokens, OutputTokens: outputTokens}
	result.UsageSource = UsageSourceEstimated
}

func requestBodyBytesForUsageEstimate(body []byte) int {
	return estimateJSONContentBytes(body, false)
}

func responseBodyBytesForUsageEstimate(body []byte) int {
	return estimateJSONContentBytes(body, true)
}

func claudeUsagePresentInJSON(body []byte) bool {
	return gjson.GetBytes(body, "usage").Exists() || gjson.GetBytes(body, "message.usage").Exists()
}

func estimateJSONContentBytes(body []byte, response bool) int {
	if len(body) == 0 {
		return 0
	}
	var value any
	if json.Unmarshal(body, &value) != nil {
		return len(body)
	}
	return measuredJSONContentBytes(value, response, "")
}

func measuredJSONContentBytes(value any, response bool, key string) int {
	switch typed := value.(type) {
	case map[string]any:
		total := 0
		for childKey, child := range typed {
			total += measuredJSONContentBytes(child, response, strings.ToLower(childKey))
		}
		return total
	case []any:
		total := 0
		for _, child := range typed {
			total += measuredJSONContentBytes(child, response, key)
		}
		return total
	case string:
		if response {
			switch key {
			case "id", "object", "type", "role", "model", "status", "finish_reason", "call_id", "name":
				return 0
			}
		}
		trimmed := strings.TrimSpace(typed)
		if strings.HasPrefix(strings.ToLower(trimmed), "data:") {
			if comma := strings.Index(trimmed, ","); comma >= 0 {
				return comma + 1
			}
		}
		return len(typed)
	default:
		return 0
	}
}

func setObservedResponseBytes(c interface{ Set(string, any) }, bytes int) {
	if c != nil && bytes > 0 {
		c.Set(usageObservedResponseBytesContextKey, bytes)
	}
}

func setUsagePresent(c interface{ Set(string, any) }, present bool) {
	if c != nil && present {
		c.Set(usagePresentContextKey, true)
	}
}

func usageObservedResponseBytes(c *gin.Context) int {
	if c == nil {
		return 0
	}
	if value, ok := c.Get(usageObservedResponseBytesContextKey); ok {
		if bytes, ok := value.(int); ok && bytes > 0 {
			return bytes
		}
	}
	if c.Writer != nil && c.Writer.Size() > 0 {
		return c.Writer.Size()
	}
	return 0
}

func clearUsageEstimateAttempt(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(usageObservedResponseBytesContextKey, 0)
	c.Set(usagePresentContextKey, false)
}

func (s *OpenAIGatewayService) finalizeUsageEstimate(ctx context.Context, c *gin.Context, account *Account, requestBytes int, result *OpenAIForwardResult, forwardErr error) {
	if result == nil {
		return
	}
	result.RequestBytes = requestBytes
	if result.ObservedResponseBytes == 0 {
		result.ObservedResponseBytes = usageObservedResponseBytes(c)
	}
	if !result.UsagePresent && c != nil {
		if value, ok := c.Get(usagePresentContextKey); ok {
			result.UsagePresent, _ = value.(bool)
		}
	}
	if forwardErr != nil {
		if result.UsagePresent || result.UsageSource == UsageSourceUpstream || openAIUsageHasTokens(&result.Usage) {
			result.UsageSource = UsageSourceUpstream
		}
		return
	}
	if account != nil && (result.UsagePresent || result.UsageSource == UsageSourceUpstream || openAIUsageHasTokens(&result.Usage)) {
		missingUsageState.Delete(account.ID)
	}
	finalizeOpenAIUsageEstimate(result)
	if result.UsageSource == UsageSourceEstimated {
		s.markMissingUsageAccount(ctx, account, result)
	}
}

func (s *GatewayService) finalizeUsageEstimate(ctx context.Context, c *gin.Context, account *Account, requestBytes int, result *ForwardResult, forwardErr error) {
	if result == nil {
		return
	}
	result.RequestBytes = requestBytes
	if result.ObservedResponseBytes == 0 {
		result.ObservedResponseBytes = usageObservedResponseBytes(c)
	}
	if !result.UsagePresent && c != nil {
		if value, ok := c.Get(usagePresentContextKey); ok {
			result.UsagePresent, _ = value.(bool)
		}
	}
	if forwardErr != nil {
		if result.UsagePresent || result.UsageSource == UsageSourceUpstream || result.Usage.hasObservedTokens() {
			result.UsageSource = UsageSourceUpstream
		}
		return
	}
	finalizeClaudeUsageEstimate(result)
	if result.UsageSource == UsageSourceEstimated {
		s.markMissingUsageAccount(ctx, account, result)
	}
}

func (s *OpenAIGatewayService) markMissingUsageAccount(ctx context.Context, account *Account, result *OpenAIForwardResult) {
	if s == nil || s.accountRepo == nil || account == nil || result == nil {
		return
	}
	stateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIAccountStateUpdateTimeout)
	defer cancel()
	now := time.Now()
	value, _ := missingUsageState.Load(account.ID)
	state, _ := value.(missingUsageObservation)
	if state.first.IsZero() || now.Sub(state.first) > missingUsageAccountCooldown { state = missingUsageObservation{first: now} }
	if now.Before(state.cooldownUntil) { return }
	state.count++
	missingUsageState.Store(account.ID, state)
	if state.count < missingUsageThreshold {
		logger.L().With(zap.String("component", "service.openai_gateway")).Warn("openai.account_missing_usage_observed", zap.Int64("account_id", account.ID), zap.Int("count", state.count))
		return
	}
	until := time.Now().Add(missingUsageAccountCooldown)
	state.cooldownUntil = until
	missingUsageState.Store(account.ID, state)
	reason := "upstream missing usage; estimated billing"
	if err := s.accountRepo.SetTempUnschedulable(stateCtx, account.ID, until, reason); err != nil {
		logger.L().With(zap.String("component", "service.openai_gateway")).Warn(
			"openai.account_temp_unschedule_missing_usage_failed",
			zap.Int64("account_id", account.ID), zap.Error(err))
		return
	}
	account.TempUnschedulableUntil = &until
	account.TempUnschedulableReason = reason
	s.BlockAccountScheduling(account, until, reason)
	logger.L().With(zap.String("component", "service.openai_gateway")).Warn(
		"openai.account_temp_unscheduled_missing_usage",
		zap.Int64("account_id", account.ID), zap.String("account_name", account.Name),
		zap.String("platform", account.Platform), zap.Time("until", until),
		zap.String("request_id", strings.TrimSpace(result.RequestID)))
}

func (s *GatewayService) markMissingUsageAccount(ctx context.Context, account *Account, result *ForwardResult) {
	if s == nil || s.accountRepo == nil || account == nil || result == nil {
		return
	}
	stateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIAccountStateUpdateTimeout)
	defer cancel()
	until := time.Now().Add(missingUsageAccountCooldown)
	reason := "upstream missing usage; estimated billing"
	if err := s.accountRepo.SetTempUnschedulable(stateCtx, account.ID, until, reason); err != nil {
		logger.L().With(zap.String("component", "service.gateway")).Warn(
			"gateway.account_temp_unschedule_missing_usage_failed",
			zap.Int64("account_id", account.ID), zap.Error(err))
		return
	}
	account.TempUnschedulableUntil = &until
	account.TempUnschedulableReason = reason
	logger.L().With(zap.String("component", "service.gateway")).Warn(
		"gateway.account_temp_unscheduled_missing_usage",
		zap.Int64("account_id", account.ID), zap.String("account_name", account.Name),
		zap.String("platform", account.Platform), zap.Time("until", until),
		zap.String("request_id", strings.TrimSpace(result.RequestID)))
}
