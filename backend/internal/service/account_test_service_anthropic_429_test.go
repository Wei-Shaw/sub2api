//go:build unit

package service

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newClaude429TestAccount() *Account {
	return &Account{
		ID:          161,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key", "base_url": "http://127.0.0.1:58168"},
	}
}

func newClaude429TestService(repo AccountRepository, resp *http.Response) *AccountTestService {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	return &AccountTestService{
		accountRepo:  repo,
		httpUpstream: &queuedHTTPUpstream{responses: []*http.Response{resp}},
		cfg:          cfg,
	}
}

func TestAccountTestService_Anthropic429WithWindowResetParksAccount(t *testing.T) {
	ctx, _ := newTestContext()
	reset := time.Now().Add(47 * time.Minute).Truncate(time.Second)
	resp := newJSONResponse(http.StatusTooManyRequests, `{"type":"error","error":{"type":"rate_limit_error","code":"credit_exhausted_5h","message":"5h limit"}}`)
	resp.Header.Set("anthropic-ratelimit-unified-5h-reset", strconv.FormatInt(reset.Unix(), 10))
	resp.Header.Set("anthropic-ratelimit-unified-5h-utilization", "1.0")

	repo := &openAIAccountTestRepo{}
	svc := newClaude429TestService(repo, resp)
	account := newClaude429TestAccount()

	err := svc.testClaudeAccountConnection(ctx, account, "claude-opus-5-5")
	require.Error(t, err)
	require.Equal(t, account.ID, repo.rateLimitedID)
	require.NotNil(t, repo.rateLimitedAt)
	require.True(t, repo.rateLimitedAt.Equal(reset), "reset=%s got=%s", reset, repo.rateLimitedAt)
	require.NotNil(t, account.RateLimitResetAt)
}

func TestAccountTestService_Anthropic429AggregateResetParksAccount(t *testing.T) {
	ctx, _ := newTestContext()
	reset := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	resp := newJSONResponse(http.StatusTooManyRequests, `{"type":"error","error":{"type":"rate_limit_error","message":"limited"}}`)
	resp.Header.Set("anthropic-ratelimit-unified-reset", strconv.FormatInt(reset.Unix(), 10))

	repo := &openAIAccountTestRepo{}
	svc := newClaude429TestService(repo, resp)
	account := newClaude429TestAccount()

	err := svc.testClaudeAccountConnection(ctx, account, "claude-opus-5-5")
	require.Error(t, err)
	require.Equal(t, account.ID, repo.rateLimitedID)
	require.NotNil(t, repo.rateLimitedAt)
	require.True(t, repo.rateLimitedAt.Equal(reset))
}

// Without a reset header the reset time is unknown; like the OpenAI test path,
// the account is left schedulable instead of being parked for a guessed window.
func TestAccountTestService_Anthropic429WithoutResetHeadersDoesNotPark(t *testing.T) {
	ctx, _ := newTestContext()
	resp := newJSONResponse(http.StatusTooManyRequests, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)

	repo := &openAIAccountTestRepo{}
	svc := newClaude429TestService(repo, resp)
	account := newClaude429TestAccount()

	err := svc.testClaudeAccountConnection(ctx, account, "claude-opus-5-5")
	require.Error(t, err)
	require.Zero(t, repo.rateLimitedID)
	require.Nil(t, account.RateLimitResetAt)
}
