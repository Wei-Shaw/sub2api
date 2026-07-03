//go:build unit

package service

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fableRejected429Headers reproduces the real header set observed when a
// claude-fable-5 request is rejected by the model-scoped weekly window while the
// general 5h / 7d windows still have headroom (see probe against api.anthropic.com).
func fableRejected429Headers(reset time.Time) http.Header {
	h := http.Header{}
	h.Set("anthropic-ratelimit-unified-status", "rejected")
	h.Set("anthropic-ratelimit-unified-5h-status", "allowed")
	h.Set("anthropic-ratelimit-unified-5h-utilization", "0.16")
	h.Set("anthropic-ratelimit-unified-7d-status", "allowed")
	h.Set("anthropic-ratelimit-unified-7d-utilization", "0.70")
	h.Set("anthropic-ratelimit-unified-7d_oi-status", "rejected")
	h.Set("anthropic-ratelimit-unified-7d_oi-utilization", "1.01")
	h.Set("anthropic-ratelimit-unified-representative-claim", "seven_day_overage_included")
	h.Set("anthropic-ratelimit-unified-reset", strconv.FormatInt(reset.Unix(), 10))
	return h
}

func TestIsAnthropicScopedRejection(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		want    bool
	}{
		{
			name:    "fable scoped rejection (5h/7d allowed, overall rejected)",
			headers: fableRejected429Headers(time.Now().Add(96 * time.Hour)),
			want:    true,
		},
		{
			name: "general 7d exhausted → not scoped",
			headers: func() http.Header {
				h := fableRejected429Headers(time.Now().Add(96 * time.Hour))
				h.Set("anthropic-ratelimit-unified-7d-utilization", "1.05")
				return h
			}(),
			want: false,
		},
		{
			name: "general 5h rejected → not scoped",
			headers: func() http.Header {
				h := fableRejected429Headers(time.Now().Add(96 * time.Hour))
				h.Set("anthropic-ratelimit-unified-5h-status", "rejected")
				return h
			}(),
			want: false,
		},
		{
			name: "general 7d status rejected → not scoped",
			headers: func() http.Header {
				h := fableRejected429Headers(time.Now().Add(96 * time.Hour))
				h.Set("anthropic-ratelimit-unified-7d-status", "rejected")
				return h
			}(),
			want: false,
		},
		{
			name: "overall not rejected → not scoped",
			headers: func() http.Header {
				h := fableRejected429Headers(time.Now().Add(96 * time.Hour))
				h.Set("anthropic-ratelimit-unified-status", "allowed")
				return h
			}(),
			want: false,
		},
		{
			name:    "no headers → not scoped",
			headers: http.Header{},
			want:    false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isAnthropicScopedRejection(tc.headers))
		})
	}
}

func TestAnthropicScopedResetAt(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	t.Run("uses unified-reset when in the future", func(t *testing.T) {
		reset := now.Add(96 * time.Hour)
		h := http.Header{}
		h.Set("anthropic-ratelimit-unified-reset", strconv.FormatInt(reset.Unix(), 10))
		got, ok := anthropicScopedResetAt(h, now)
		require.True(t, ok)
		require.Equal(t, reset, got)
	})

	t.Run("falls back to retry-after seconds", func(t *testing.T) {
		h := http.Header{}
		h.Set("retry-after", "3600")
		got, ok := anthropicScopedResetAt(h, now)
		require.True(t, ok)
		require.Equal(t, now.Add(time.Hour), got)
	})

	t.Run("clamps values beyond 8 days", func(t *testing.T) {
		h := http.Header{}
		h.Set("anthropic-ratelimit-unified-reset", strconv.FormatInt(now.Add(30*24*time.Hour).Unix(), 10))
		got, ok := anthropicScopedResetAt(h, now)
		require.True(t, ok)
		require.Equal(t, now.Add(8*24*time.Hour), got)
	})

	t.Run("no usable header → not ok", func(t *testing.T) {
		_, ok := anthropicScopedResetAt(http.Header{}, now)
		require.False(t, ok)
	})
}

type anthropicModelScopedRepo struct {
	mockAccountRepoForGemini
	rateLimitCalls  int
	modelLimitCalls []modelScopedCall
}

type modelScopedCall struct {
	scope   string
	resetAt time.Time
	reason  string
}

func (r *anthropicModelScopedRepo) SetRateLimited(_ context.Context, _ int64, _ time.Time) error {
	r.rateLimitCalls++
	return nil
}

func (r *anthropicModelScopedRepo) SetModelRateLimit(_ context.Context, _ int64, scope string, resetAt time.Time, reason ...string) error {
	call := modelScopedCall{scope: scope, resetAt: resetAt}
	if len(reason) > 0 {
		call.reason = reason[0]
	}
	r.modelLimitCalls = append(r.modelLimitCalls, call)
	return nil
}

func anthropicOAuthAccount() *Account {
	return &Account{ID: 42, Type: AccountTypeOAuth, Platform: PlatformAnthropic}
}

const rateLimitBody = `{"type":"error","error":{"type":"rate_limit_error","message":"This request would exceed your account's rate limit. Please try again later."}}`

// A Fable 429 with headroom on the general windows should limit ONLY the
// requested model, never the whole account.
func TestHandleUpstreamError_FableScoped429_LimitsModelOnly(t *testing.T) {
	reset := time.Now().Add(96 * time.Hour).Truncate(time.Second)
	repo := &anthropicModelScopedRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)

	svc.HandleUpstreamError(
		context.Background(),
		anthropicOAuthAccount(),
		http.StatusTooManyRequests,
		fableRejected429Headers(reset),
		[]byte(rateLimitBody),
		"claude-fable-5",
	)

	require.Zero(t, repo.rateLimitCalls, "scoped 429 must not rate-limit the whole account")
	require.Len(t, repo.modelLimitCalls, 1)
	require.Equal(t, "claude-fable-5", repo.modelLimitCalls[0].scope)
	require.Equal(t, reset, repo.modelLimitCalls[0].resetAt)
	require.Equal(t, "anthropic_scoped_weekly_exhausted", repo.modelLimitCalls[0].reason)
}

// A genuine general 7d exhaustion must still block the whole account.
func TestHandleUpstreamError_General7dExhausted_LimitsAccount(t *testing.T) {
	reset := time.Now().Add(96 * time.Hour).Truncate(time.Second)
	headers := fableRejected429Headers(reset)
	headers.Set("anthropic-ratelimit-unified-7d-status", "rejected")
	headers.Set("anthropic-ratelimit-unified-7d-utilization", "1.05")
	headers.Set("anthropic-ratelimit-unified-7d-reset", strconv.FormatInt(reset.Unix(), 10))

	repo := &anthropicModelScopedRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)

	svc.HandleUpstreamError(
		context.Background(),
		anthropicOAuthAccount(),
		http.StatusTooManyRequests,
		headers,
		[]byte(rateLimitBody),
		"claude-fable-5",
	)

	require.Equal(t, 1, repo.rateLimitCalls, "general 7d exhaustion should block the account")
	require.Empty(t, repo.modelLimitCalls, "account-wide path must not also set a model limit")
}

// Without a requested model we cannot scope the limit; fall back to existing behavior.
func TestHandleUpstreamError_Scoped429_NoModel_DoesNotSetModelLimit(t *testing.T) {
	repo := &anthropicModelScopedRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)

	svc.HandleUpstreamError(
		context.Background(),
		anthropicOAuthAccount(),
		http.StatusTooManyRequests,
		fableRejected429Headers(time.Now().Add(96*time.Hour)),
		[]byte(rateLimitBody),
	)

	require.Empty(t, repo.modelLimitCalls, "no requested model → no model-scoped limit")
}
