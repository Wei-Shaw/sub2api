package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type geminiCapacityExhaustedAccountRepoStub struct {
	mockAccountRepoForGemini
	rateLimitCalls     int
	lastRateLimitID    int64
	lastRateLimitReset time.Time
}

func (r *geminiCapacityExhaustedAccountRepoStub) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.rateLimitCalls++
	r.lastRateLimitID = id
	r.lastRateLimitReset = resetAt
	return nil
}

func TestGeminiMessagesCompatService_HandleGeminiUpstreamError_ModelCapacityExhaustedUsesShortCooldown(t *testing.T) {
	repo := &geminiCapacityExhaustedAccountRepoStub{}
	svc := &GeminiMessagesCompatService{accountRepo: repo}
	account := &Account{
		ID:       42,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"oauth_type": "google_one",
		},
	}
	before := time.Now()
	body := []byte(`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"No capacity available for model gemini-3.1-pro-preview on the server","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"MODEL_CAPACITY_EXHAUSTED"}]}}`)

	svc.handleGeminiUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{}, body)

	require.Equal(t, 1, repo.rateLimitCalls)
	require.Equal(t, int64(42), repo.lastRateLimitID)
	require.WithinDuration(t, before.Add(time.Minute), repo.lastRateLimitReset, 2*time.Second)
}