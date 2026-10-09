//go:build unit

package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestClassifyClineErrorRequiresStructuredFields(t *testing.T) {
	for name, body := range map[string]string{
		"code in model message":        `{"error":{"code":"invalid_request_error","message":"Model INFERENCE_CAP_ERROR not found"}}`,
		"code suffix":                  `{"error":{"code":"model-SPEND_LIMIT_EXCEEDED","message":"Invalid model"}}`,
		"credits message without code": `{"error":{"message":"Insufficient credits"}}`,
		"spend message without code":   `{"error":{"message":"SPEND_LIMIT_EXCEEDED"}}`,
		"metadata code":                `{"error":{"code":429,"message":"Rate limit exceeded","metadata":{"code":"SPEND_LIMIT_EXCEEDED"}}}`,
		"metadata pass message":        `{"error":{"message":"Invalid model","metadata":{"message":"You have reached your monthly ClinePass limit. Please try again later."}}}`,
		"metadata free message":        `{"error":{"message":"Invalid model","metadata":{"message":"Free limit reached on model x. Try again in 3h 20m"}}}`,
		"root request code":            `{"error":{"message":"Invalid model"},"request":{"model":"insufficient_credits"}}`,
		"root message":                 `{"message":"You have reached your monthly ClinePass limit. Please try again later."}`,
		"error string":                 `{"error":"SPEND_LIMIT_EXCEEDED"}`,
		"plain code":                   `Model SPEND_LIMIT_EXCEEDED not found`,
		"plain pass message":           `You have reached your weekly ClinePass limit. Please try again later.`,
		"malformed json":               `{"error":{"code":"insufficient_credits"}`,
	} {
		t.Run(name, func(t *testing.T) {
			kind, _ := parseClineError([]byte(body))
			require.Equal(t, clineErrorNone, kind)
		})
	}
}

func TestHandleUpstreamError_Cline400DoesNotCoolWallet(t *testing.T) {
	for name, body := range map[string]string{
		"credits model echo": `{"error":{"code":"invalid_request_error","message":"Model insufficient_credits not found"}}`,
		"spend model echo":   `{"error":{"message":"Model SPEND_LIMIT_EXCEEDED not found"}}`,
		"pass model echo":    `{"error":{"message":"Model cline-pass/INFERENCE_CAP_ERROR not found"}}`,
		"metadata echo":      `{"error":{"code":400,"message":"Invalid model","metadata":{"model":"SPEND_LIMIT_EXCEEDED"}}}`,
		"plain echo":         `Model SPEND_LIMIT_EXCEEDED not found`,
		"business code":      clineInferenceCapBody,
		"pass message":       clinePassMonthlyBody,
	} {
		t.Run(name, func(t *testing.T) {
			account := clineTestAccount(400)
			repo := &commandCodeRateLimitRepo{}
			shouldDisable := NewRateLimitService(repo, nil, &config.Config{}, nil, nil).HandleUpstreamError(
				context.Background(), account, http.StatusBadRequest, http.Header{}, []byte(body), "cline-pass/INFERENCE_CAP_ERROR")

			require.False(t, shouldDisable)
			require.Empty(t, repo.modelLimits)
			require.Zero(t, repo.setErrorCalls)
			require.Zero(t, repo.tempCalls)
			require.False(t, account.isModelRateLimitedWithContext(context.Background(), "cline-pass/glm-5.3"))
			require.False(t, account.isModelRateLimitedWithContext(context.Background(), "anthropic/claude-sonnet-4-6"))
		})
	}
}

func TestHandleUpstreamError_Cline429EchoedKeywordsDoNotCoolWallet(t *testing.T) {
	for name, body := range map[string]string{
		"message without code": `{"error":{"message":"Model INFERENCE_CAP_ERROR not found"}}`,
		"metadata model":       `{"error":{"code":429,"message":"Rate limit exceeded","metadata":{"model":"SPEND_LIMIT_EXCEEDED"}}}`,
		"request model":        `{"error":{"code":429,"message":"Rate limit exceeded"},"request":{"model":"insufficient_credits"}}`,
		"plain text":           `Model SPEND_LIMIT_EXCEEDED not found`,
	} {
		t.Run(name, func(t *testing.T) {
			repo := &commandCodeRateLimitRepo{}
			NewRateLimitService(repo, nil, &config.Config{}, nil, nil).HandleUpstreamError(context.Background(),
				clineTestAccount(429), http.StatusTooManyRequests, http.Header{}, []byte(body), "deepseek/deepseek-v4-flash")
			require.Empty(t, repo.modelLimits)
		})
	}
}

func TestHandleUpstreamError_ClineStructuredErrorsPreserveCooldown(t *testing.T) {
	for _, tc := range []struct {
		name, model, body, scope, reason string
		status                           int
		wait                             time.Duration
	}{
		{
			name: "credits code before echoed metadata", model: "deepseek/deepseek-v4-flash",
			body:  `{"error":{"code":"insufficient_credits","message":"Insufficient credits","metadata":{"model":"SPEND_LIMIT_EXCEEDED"}}}`,
			scope: clineCreditsRateLimitKey, reason: clineCreditsReason, status: http.StatusPaymentRequired, wait: cnBalanceCheckCooldown(nil),
		},
		{
			name: "spend code before pass wording", model: "deepseek/deepseek-v4-flash",
			body:  `{"error":{"code":"SPEND_LIMIT_EXCEEDED","message":"You have reached your monthly ClinePass limit. Please try again later."}}`,
			scope: clineCreditsRateLimitKey, reason: clineSpendLimitReason, status: http.StatusTooManyRequests, wait: clineLimitRecheck,
		},
		{
			name: "details spend code", model: "deepseek/deepseek-v4-flash",
			body:  `{"error":{"code":429,"message":"Rate limit exceeded","details":{"code":"SPEND_LIMIT_EXCEEDED"}}}`,
			scope: clineCreditsRateLimitKey, reason: clineSpendLimitReason, status: http.StatusTooManyRequests, wait: clineLimitRecheck,
		},
		{
			name: "inference code", model: "cline-pass/glm-5.3", body: clineInferenceCapBody,
			scope: clinePassRateLimitKey, reason: clineInferenceCapReason, status: http.StatusTooManyRequests, wait: clineLimitRecheck,
		},
		{
			name: "pass message ignores metadata window", model: "cline-pass/glm-5.3",
			body:  `{"error":{"code":429,"metadata":{"message":"You have reached your monthly ClinePass limit. Please try again later."},"message":"You have reached your 5-hour ClinePass limit. Please try again later."}}`,
			scope: clinePassRateLimitKey, reason: clinePassLimitReason, status: http.StatusTooManyRequests, wait: clineLimitRecheck,
		},
		{
			name: "details pass message", model: "cline-pass/glm-5.3",
			body:  `{"error":{"code":429,"message":"Rate limit exceeded","details":{"message":"You have reached your monthly ClinePass limit. Please try again later."}}}`,
			scope: clinePassRateLimitKey, reason: clinePassLimitReason, status: http.StatusTooManyRequests, wait: clinePassMonthlyRecheck,
		},
		{
			name: "details entitlement message", model: "cline-pass/glm-5.3",
			body:  `{"error":{"code":403,"message":"Forbidden","details":{"message":"The user is not subscribed to required model plan"}}}`,
			scope: clinePassRateLimitKey, reason: clinePassUnavailableReason, status: http.StatusForbidden, wait: clineLimitRecheck,
		},
		{
			name: "free message ignores metadata retry", model: "cline-free/kimi-k3",
			body:  `{"error":{"code":429,"metadata":{"message":"Try again in 20h"},"message":"Free limit reached on model cline-free/kimi-k3. Try again in 5m"}}`,
			scope: "cline-free/kimi-k3", reason: clineFreeModelLimitReason, status: http.StatusTooManyRequests, wait: 5 * time.Minute,
		},
		{
			name: "details free message", model: "cline-free/kimi-k3",
			body:  `{"error":{"code":429,"message":"Rate limit exceeded","details":{"message":"Free limit reached on model cline-free/kimi-k3. Try again in 3h 20m"}}}`,
			scope: "cline-free/kimi-k3", reason: clineFreeModelLimitReason, status: http.StatusTooManyRequests, wait: 3*time.Hour + 20*time.Minute,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &commandCodeRateLimitRepo{}
			account := clineTestAccount(430)
			shouldDisable := NewRateLimitService(repo, nil, &config.Config{}, nil, nil).HandleUpstreamError(
				context.Background(), account, tc.status, http.Header{}, []byte(tc.body), tc.model)

			require.False(t, shouldDisable)
			require.Len(t, repo.modelLimits, 1)
			require.WithinDuration(t, time.Now().Add(tc.wait), repo.modelLimits[tc.scope], time.Second)
			require.True(t, strings.HasPrefix(repo.reasons[tc.scope], tc.reason), repo.reasons[tc.scope])
			require.Zero(t, repo.setErrorCalls)
			require.Zero(t, repo.tempCalls)
		})
	}
}
