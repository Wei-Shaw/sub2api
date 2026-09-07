//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// qwenTokenPlanRepo records the durable state changes made when an inference
// response proves that a one-time Token Plan allowance is exhausted.
type qwenTokenPlanRepo struct {
	AccountRepository
	extraWrites      []map[string]any
	updateExtraErr   error
	schedulableCalls []bool
	setScheduleErr   error
	rateLimitCalls   int
}

func (r *qwenTokenPlanRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	copyOfUpdates := make(map[string]any, len(updates))
	for key, value := range updates {
		copyOfUpdates[key] = value
	}
	r.extraWrites = append(r.extraWrites, copyOfUpdates)
	return r.updateExtraErr
}

func (r *qwenTokenPlanRepo) SetSchedulable(_ context.Context, _ int64, schedulable bool) error {
	r.schedulableCalls = append(r.schedulableCalls, schedulable)
	return r.setScheduleErr
}

func (r *qwenTokenPlanRepo) SetRateLimited(_ context.Context, _ int64, _ time.Time) error {
	r.rateLimitCalls++
	return nil
}

func newQwenTokenPlanAccount() *Account {
	return &Account{
		ID:       771,
		Platform: PlatformQwen,
		Type:     AccountTypeAPIKey,
		Status:   StatusActive,
		Credentials: map[string]any{
			"account_mode": AccountModeTokenPlan,
			"api_key":      "qwen-token-plan-key",
		},
	}
}

func TestQwenTokenPlanQuotaExhausted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "documented one week message",
			body: `{"error":{"message":"Your token-plan 1-week quota has been exhausted. The quota will reset at 2026-09-14T00:00:00Z."}}`,
			want: true,
		},
		{
			name: "json field wording variant",
			body: `{"message":"weekly quota exceeded; will reset at 2026-09-14T00:00:00Z"}`,
			want: true,
		},
		{
			name: "allocated quota exceeded without reset",
			body: `{"error":{"code":"Throttling.AllocationQuota","message":"Allocated quota exceeded, please increase your quota limit."}}`,
			want: true,
		},
		{
			name: "insufficient_quota plan wording",
			body: `{"error":{"code":"insufficient_quota","message":"You exceeded your current quota, please check your plan and billing details."}}`,
			want: true,
		},
		{
			name: "one-time quota exhausted without reset",
			body: `{"error":{"message":"quota exhausted"}}`,
			want: true,
		},
		{
			name: "chinese one-time exhaustion",
			body: `{"error":{"message":"套餐额度已用尽"}}`,
			want: true,
		},
		{
			name: "generic rate limit",
			body: `{"error":{"message":"rate limit exceeded"}}`,
			want: false,
		},
		{
			name: "requests rate limit exceeded",
			body: `{"error":{"message":"Requests rate limit exceeded, please try again later."}}`,
			want: false,
		},
		{
			name: "api-key requests rate limit",
			body: `{"error":{"message":"API-Key Requests rate limit exceeded"}}`,
			want: false,
		},
		{
			name: "request surge protection",
			body: `{"error":{"message":"Request rate increased too quickly"}}`,
			want: false,
		},
		{
			name: "reset message without exhaustion",
			body: `{"error":{"message":"quota will reset at 2026-09-14T00:00:00Z"}}`,
			want: false,
		},
		{
			name: "authentication failure",
			body: `{"error":{"message":"invalid api key"}}`,
			want: false,
		},
		{
			name: "transport-like text",
			body: `upstream connection reset`,
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, qwenTokenPlanQuotaExhausted([]byte(tt.body)))
		})
	}
}

func TestHandle429_QwenTokenPlanExhaustionDisablesScheduling(t *testing.T) {
	repo := &qwenTokenPlanRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := newQwenTokenPlanAccount()

	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"message":"Your token-plan 1-week quota has been exhausted. The quota will reset at 2026-09-14T00:00:00Z."}}`))

	require.Equal(t, []bool{false}, repo.schedulableCalls)
	require.Len(t, repo.extraWrites, 1)
	require.Equal(t, true, repo.extraWrites[0][qwenTokenPlanExhaustedExtraKey])
	require.NotEmpty(t, repo.extraWrites[0][qwenTokenPlanExhaustedAtExtraKey])
	require.Contains(t, repo.extraWrites[0][qwenTokenPlanExhaustedReasonExtraKey], "quota exhausted")
}

func TestHandle429_QwenTokenPlanGeneric429DoesNotDisableScheduling(t *testing.T) {
	repo := &qwenTokenPlanRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := newQwenTokenPlanAccount()

	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"message":"rate limit exceeded"}}`))

	require.Empty(t, repo.extraWrites)
	require.Empty(t, repo.schedulableCalls)
	require.Equal(t, 1, repo.rateLimitCalls, "generic 429 may use the short fallback cooldown")
}

func TestHandle429_QwenTokenPlanAllocatedQuotaDisablesScheduling(t *testing.T) {
	repo := &qwenTokenPlanRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := newQwenTokenPlanAccount()

	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"message":"Allocated quota exceeded"}}`))

	require.Equal(t, []bool{false}, repo.schedulableCalls)
	require.Len(t, repo.extraWrites, 1)
	require.Equal(t, true, repo.extraWrites[0][qwenTokenPlanExhaustedExtraKey])
	require.Zero(t, repo.rateLimitCalls)
}

func TestHandle429_QwenTokenPlanMarkerWriteFailureStillPausesScheduling(t *testing.T) {
	repo := &qwenTokenPlanRepo{updateExtraErr: errors.New("database unavailable")}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := newQwenTokenPlanAccount()

	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"message":"quota exhausted; will reset at 2026-09-14T00:00:00Z"}}`))

	require.Equal(t, []bool{false}, repo.schedulableCalls)
	require.Len(t, repo.extraWrites, 1)
}

func TestHandle429_QwenTokenPlanPauseFailureStillPersistsMarker(t *testing.T) {
	repo := &qwenTokenPlanRepo{setScheduleErr: errors.New("database unavailable")}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := newQwenTokenPlanAccount()

	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"message":"quota exhausted; will reset at 2026-09-14T00:00:00Z"}}`))

	require.Equal(t, []bool{false}, repo.schedulableCalls)
	require.Len(t, repo.extraWrites, 1)
	require.Equal(t, true, repo.extraWrites[0][qwenTokenPlanExhaustedExtraKey])
}
