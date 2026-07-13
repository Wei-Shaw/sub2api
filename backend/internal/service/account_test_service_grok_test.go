//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type grokAccountTestRateLimitRepo struct {
	*mockAccountRepoForGemini
	rateLimitedCalls int
	resetAt          time.Time
REDACTED

func (r *grokAccountTestRateLimitRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitedCalls++
	r.resetAt = resetAt
	return nil
REDACTED

func TestAccountTestService_TestAccountConnection_GrokUsesXAIResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	account := &Account{
		ID:          13,
		Name:        "grok-oauth",
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"access_token": "grok-access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			"model_mapping": map[string]any{
				"grok": "grok-4.3",
		REDACTED,
	REDACTED,
REDACTED
	repo := &mockAccountRepoForGemini{
		accountsByID: map[int64]*Account{account.ID: accountREDACTED,
REDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"REDACTED\n\n" +
				"data: {\"type\":\"response.completed\"REDACTED\n\n",
		)),
REDACTEDREDACTED
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		httpUpstream:      upstream,
REDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/13/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "grok", "", AccountTestModeDefault)
REDACTED

	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer grok-access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, grokCLIVersion, upstream.lastReq.Header.Get("X-Grok-Client-Version"))
	require.Equal(t, "grok-4.3", gjson.GetBytes(upstream.lastBody, "model").String())
	require.NotContains(t, rec.Body.String(), "claude")
	require.Contains(t, rec.Body.String(), `"model":"grok-4.3"`)
	require.Contains(t, rec.Body.String(), `"type":"test_complete"`)
REDACTED

func TestAccountTestService_Grok429PersistsRateLimitReset(t *testing.T) {
	gin.SetMode(gin.TestMode)

	account := &Account{
		ID:          14,
		Name:        "grok-oauth-limited",
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
REDACTED
			"access_token": "grok-access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	REDACTED,
REDACTED
	baseRepo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: accountREDACTEDREDACTED
	repo := &grokAccountTestRateLimitRepo{mockAccountRepoForGemini: baseRepoREDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{"45"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"REDACTEDREDACTED`)),
REDACTEDREDACTED
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		httpUpstream:      upstream,
REDACTED
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/14/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "grok", "", AccountTestModeDefault)

REDACTED
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.WithinDuration(t, time.Now().Add(45*time.Second), repo.resetAt, time.Second)
REDACTED

func TestAccountTestService_Grok429WithoutQuotaHeadersUsesFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 15, Name: "grok-oauth-limited-no-headers", Platform: PlatformGrok,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
REDACTED
			"access_token": "grok-access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	REDACTED,
REDACTED
	baseRepo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: accountREDACTEDREDACTED
	repo := &grokAccountTestRateLimitRepo{mockAccountRepoForGemini: baseRepoREDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"quota exhausted"REDACTEDREDACTED`)),
REDACTEDREDACTED
	svc := &AccountTestService{
		accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, nil), httpUpstream: upstream,
REDACTED
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/15/test", nil)
	before := time.Now()

	err := svc.TestAccountConnection(c, account.ID, "grok", "", AccountTestModeDefault)

REDACTED
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.WithinDuration(t, before.Add(grokRateLimitFallbackCooldown), repo.resetAt, time.Second)
REDACTED
