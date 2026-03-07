//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIAccountTestRepo struct {
	mockAccountRepoForGemini
	updatedExtra  map[string]any
	rateLimitedID int64
	rateLimitedAt *time.Time
REDACTED

func (r *openAIAccountTestRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updatedExtra = updates
	return nil
REDACTED

func (r *openAIAccountTestRepo) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.rateLimitedID = id
	r.rateLimitedAt = &resetAt
	return nil
REDACTED

func TestAccountTestService_OpenAISuccessPersistsSnapshotFromHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newSoraTestContext()

	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader(`data: {"type":"response.completed"REDACTED

`))
	resp.Header.Set("x-codex-primary-used-percent", "88")
	resp.Header.Set("x-codex-primary-reset-after-seconds", "604800")
	resp.Header.Set("x-codex-primary-window-minutes", "10080")
	resp.Header.Set("x-codex-secondary-used-percent", "42")
	resp.Header.Set("x-codex-secondary-reset-after-seconds", "18000")
	resp.Header.Set("x-codex-secondary-window-minutes", "300")

	repo := &openAIAccountTestRepo{REDACTED
	upstream := &queuedHTTPUpstream{responses: []*http.Response{respREDACTEDREDACTED
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          89,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED"access_token": "test-token"REDACTED,
REDACTED

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4")
REDACTED
	require.NotEmpty(t, repo.updatedExtra)
	require.Equal(t, 42.0, repo.updatedExtra["codex_5h_used_percent"])
	require.Equal(t, 88.0, repo.updatedExtra["codex_7d_used_percent"])
	require.Contains(t, recorder.Body.String(), "test_complete")
REDACTED

func TestAccountTestService_OpenAI429PersistsSnapshotAndRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newSoraTestContext()

	resp := newJSONResponse(http.StatusTooManyRequests, `{"error":{"type":"usage_limit_reached","message":"limit reached"REDACTEDREDACTED`)
	resp.Header.Set("x-codex-primary-used-percent", "100")
	resp.Header.Set("x-codex-primary-reset-after-seconds", "604800")
	resp.Header.Set("x-codex-primary-window-minutes", "10080")
	resp.Header.Set("x-codex-secondary-used-percent", "100")
	resp.Header.Set("x-codex-secondary-reset-after-seconds", "18000")
	resp.Header.Set("x-codex-secondary-window-minutes", "300")

	repo := &openAIAccountTestRepo{REDACTED
	upstream := &queuedHTTPUpstream{responses: []*http.Response{respREDACTEDREDACTED
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          88,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED"access_token": "test-token"REDACTED,
REDACTED

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4")
REDACTED
	require.NotEmpty(t, repo.updatedExtra)
	require.Equal(t, 100.0, repo.updatedExtra["codex_5h_used_percent"])
	require.Equal(t, int64(88), repo.rateLimitedID)
	require.NotNil(t, repo.rateLimitedAt)
	require.NotNil(t, account.RateLimitResetAt)
	if account.RateLimitResetAt != nil && repo.rateLimitedAt != nil {
		require.WithinDuration(t, *repo.rateLimitedAt, *account.RateLimitResetAt, time.Second)
REDACTED
REDACTED
