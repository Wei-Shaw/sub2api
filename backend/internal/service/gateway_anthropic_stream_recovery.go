package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// Keep stream failures separate from pre-response failover errors: once bytes
// have been sent, another account must never be appended to the same stream.
type anthropicStreamFailure struct {
	reason    string
	transient bool
}

func (e *anthropicStreamFailure) Error() string { return "anthropic stream failed: " + e.reason }

func anthropicStreamError(data string) error {
	if gjson.Get(data, "type").String() != "error" {
		return nil
	}
	return classifyAnthropicStreamError(data)
}

func classifyAnthropicStreamError(data string) error {
	kind := gjson.Get(data, "error.type").String()
	switch kind {
	case "invalid_request_error", "not_found_error", "request_too_large":
		return &anthropicStreamFailure{reason: "request_rejected"}
	default:
		return &anthropicStreamFailure{reason: "upstream_error_event", transient: true}
	}
}

// anthropicStreamErrorStatus converts an Anthropic SSE error event into the
// HTTP-like status used by the normal failover/rate-limit policy.  Anthropic
// can return these errors inside an HTTP 200 stream, so callers must classify
// them before deciding whether another account may be tried.
func anthropicStreamErrorStatus(data string) int {
	switch gjson.Get(data, "error.type").String() {
	case "not_found_error":
		return http.StatusNotFound
	case "authentication_error", "permission_error":
		return http.StatusForbidden
	case "rate_limit_error":
		return http.StatusTooManyRequests
	case "overloaded_error":
		return 529
	case "invalid_request_error", "request_too_large":
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
	}
}

func newAnthropicStreamFailoverError(data string) *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode:   anthropicStreamErrorStatus(data),
		ResponseBody: []byte(data),
	}
}

func anthropicStreamHasSemanticOutput(data string) bool {
	if gjson.Get(data, "type").String() == "content_block_start" {
		block := gjson.Get(data, "content_block")
		return block.Get("text").String() != "" || block.Get("thinking").String() != "" || block.Get("type").String() == "tool_use" || block.Get("type").String() == "server_tool_use"
	}
	if gjson.Get(data, "type").String() != "content_block_delta" {
		return false
	}
	delta := gjson.Get(data, "delta")
	for _, key := range []string{"text", "thinking", "partial_json"} {
		if delta.Get(key).String() != "" {
			return true
		}
	}
	return false
}

const (
	anthropicModelPermissionCooldown = 10 * time.Minute
	anthropicModelPermissionReason   = "anthropic_model_permission_error"
	anthropicModelNotFoundReason     = "anthropic_model_not_found"
)

// anthropicModelScopedFailoverReason identifies deterministic model capability
// failures whose scheduler side effect is already persisted per account+model.
// The returned reason is copied onto the failover sentinel so the outer handler
// does not report the failure as an account-wide scheduling outage.
func anthropicModelScopedFailoverReason(body []byte) GatewayFailureReason {
	switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "error.type").String())) {
	case "not_found_error":
		return anthropicModelNotFoundReason
	case "permission_error":
		return anthropicModelPermissionReason
	default:
		return ""
	}
}

func markAnthropicModelScopedFailover(err *UpstreamFailoverError, body []byte) *UpstreamFailoverError {
	if err == nil {
		return nil
	}
	if reason := anthropicModelScopedFailoverReason(body); reason != "" {
		err.Scope = GatewayFailureScopeRequest
		err.Reason = reason
		err.NextAccountAction = NextAccountRetry
	}
	return err
}

// applyAnthropicStreamFailoverPolicy keeps deterministic Anthropic model
// capability failures scoped to the selected account+model pair.  A generic
// 403 handler would otherwise pause or disable the whole account, which also
// removes unrelated Claude models from scheduling.
func applyAnthropicStreamFailoverPolicy(ctx context.Context, rateLimitService *RateLimitService, account *Account, model string, statusCode int, body []byte, headers http.Header) bool {
	// The production service always has an account repository.  Keep the
	// streaming classifier usable in isolated/unit contexts too: calling the
	// policy with a zero-value RateLimitService would panic in the auth/error
	// handlers that persist account state.
	if rateLimitService == nil || rateLimitService.accountRepo == nil || account == nil {
		return false
	}

	kind := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "error.type").String()))
	switch kind {
	case "invalid_request_error", "request_too_large":
		// Client/request errors are not account availability failures.
		return false
	case "not_found_error":
		return rateLimitService.HandleUpstreamModelNotFound(ctx, account, model, statusCode, body)
	case "permission_error":
		modelKey := modelRateLimitKeyForUpstreamModelNotFound(ctx, account, model)
		if modelKey == "" {
			return false
		}
		resetAt := time.Now().Add(anthropicModelPermissionCooldown)
		if err := rateLimitService.accountRepo.SetModelRateLimit(ctx, account.ID, modelKey, resetAt, anthropicModelPermissionReason); err != nil {
			// Fail over even if persistence failed.  Widening this to an account
			// block would make a transient DB problem more destructive.
			logger.FromContext(ctx).Warn("gateway.anthropic_model_permission_cooldown_persist_failed",
				zap.Int64("account_id", account.ID), zap.String("model", modelKey), zap.Error(err))
		}
		return true
	default:
		return rateLimitService.HandleUpstreamError(ctx, account, statusCode, headers, body, model)
	}
}

func (s *GatewayService) anthropicStreamRecovery() *openAIAccountModelTransientState {
	s.anthropicStreamTransientOnce.Do(func() {
		s.anthropicStreamTransient = newOpenAIAccountModelTransientState(openAIModelTransientDefaultMax)
	})
	return s.anthropicStreamTransient
}

func anthropicStreamRecoveryAccount(account *Account) bool {
	return account != nil && account.Platform == PlatformAnthropic && account.Type == AccountTypeAPIKey
}

func (s *GatewayService) recordAnthropicStreamFailure(ctx context.Context, account *Account, model string, err error) {
	var failure *anthropicStreamFailure
	if !anthropicStreamRecoveryAccount(account) || ctx.Err() != nil || !errors.As(err, &failure) || !failure.transient {
		return
	}
	threshold := 1
	if failure.reason == "upstream_http_error" {
		threshold = 2
	}
	d := s.anthropicStreamRecovery().recordFailureWithInitialCooldown(account.ID, model, time.Now(), 0,
		openAITransientFailureOptions{requireProbe: true, reason: "anthropic_" + failure.reason,
			backoffBase: 30 * time.Second, backoffCap: 5 * time.Minute, failureThreshold: threshold})
	if d.Cooldown > 0 {
		logger.FromContext(ctx).Warn("gateway.anthropic_model_stream_cooldown", zap.Int64("account_id", account.ID),
			zap.String("model", model), zap.String("reason", failure.reason), zap.Duration("cooldown", d.Cooldown))
	}
}

// Both forward paths check admission after mapping. Cancellation releases probes.
func (s *GatewayService) admitAnthropicStreamRecoveryModel(ctx context.Context, account *Account, model string, release func()) (func(), func(bool), bool) {
	if !anthropicStreamRecoveryAccount(account) {
		return release, func(bool) {}, true
	}
	state := s.anthropicStreamRecovery()
	blocked, id := state.reserveProbe(account.ID, model, time.Now())
	if blocked {
		if release != nil {
			release()
		}
		return nil, nil, false
	}
	var once sync.Once
	wrapped := func() {
		once.Do(func() {
			state.releaseProbe(account.ID, model, id)
			if release != nil {
				release()
			}
		})
	}
	stop := context.AfterFunc(ctx, wrapped)
	return func() { stop(); wrapped() }, func(success bool) {
		if success && ctx.Err() == nil {
			if id == 0 {
				state.recordSuccess(account.ID, model)
			} else {
				state.completeProbe(account.ID, model, id, time.Now())
			}
		}
	}, true
}

func incompleteAnthropicStream(reason string) error {
	return &anthropicStreamFailure{reason: "stream " + reason, transient: true}
}
