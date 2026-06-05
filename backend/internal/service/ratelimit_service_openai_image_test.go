//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIsOpenAIImageRateLimitError(t *testing.T) {
	imageBody := []byte(`{"error":{"message":"Rate limit reached for gpt-image-2-codex (for limit gpt-image) in organization org on input-images per min: Limit 4000, Used 4000. Please try again in 467ms."REDACTEDREDACTED`)
	textBody := []byte(`{"error":{"message":"Rate limit reached for gpt-5.4 in organization org on tokens per min: Limit 30000, Used 30000. Please try again in 1s."REDACTEDREDACTED`)

	require.True(t, isOpenAIImageRateLimitError(http.StatusTooManyRequests, imageBody))
	require.False(t, isOpenAIImageRateLimitError(http.StatusTooManyRequests, textBody))
	require.False(t, isOpenAIImageRateLimitError(http.StatusBadRequest, imageBody))
REDACTED

func TestRateLimitService_HandleOpenAIImageRateLimit_ParsesTryAgainCooldown(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := &Account{ID: 201, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	body := []byte(`{"error":{"type":"rate_limit_exceeded","message":"Rate limit reached for gpt-image-2-codex (for limit gpt-image) on input-images per min. Please try again in 2s."REDACTEDREDACTED`)

	before := time.Now()
	handled := svc.HandleOpenAIImageRateLimit(context.Background(), account, http.StatusTooManyRequests, http.Header{REDACTED, body)

	require.True(t, handled)
	require.Len(t, repo.modelRateLimitCalls, 1)
	call := repo.modelRateLimitCalls[0]
	require.Equal(t, account.ID, call.accountID)
	require.Equal(t, openAIImageGenerationRateLimitKey, call.scope)
	require.Equal(t, openAIImageRateLimitReason, call.reason)
	require.WithinDuration(t, before.Add(2*time.Second), call.resetAt, time.Second)
REDACTED

func TestRateLimitService_HandleOpenAIImageRateLimit_DefaultsToOneMinute(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &RateLimitService{accountRepo: repoREDACTED
	account := &Account{ID: 202, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	body := []byte(`{"error":{"type":"rate_limit_exceeded","message":"Rate limit reached for gpt-image-2-codex (for limit gpt-image) on input-images per min."REDACTEDREDACTED`)

	before := time.Now()
	handled := svc.HandleOpenAIImageRateLimit(context.Background(), account, http.StatusTooManyRequests, http.Header{REDACTED, body)

	require.True(t, handled)
	require.Len(t, repo.modelRateLimitCalls, 1)
	call := repo.modelRateLimitCalls[0]
	require.Equal(t, openAIImageGenerationRateLimitKey, call.scope)
	require.Equal(t, openAIImageRateLimitReason, call.reason)
	require.WithinDuration(t, before.Add(time.Minute), call.resetAt, time.Second)
REDACTED

func TestOpenAIGatewayService_HandleOpenAIAccountUpstreamError_ImageRateLimitDoesNotBlockWholeAccount(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{REDACTED
	svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repoREDACTEDREDACTED
	account := &Account{ID: 203, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	body := []byte(`{"error":{"type":"rate_limit_exceeded","message":"Rate limit reached for gpt-image-2-codex (for limit gpt-image) on input-images per min. Please try again in 1s."REDACTEDREDACTED`)

	disabled := svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{REDACTED, body, "gpt-image-2")

	require.False(t, disabled)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, openAIImageGenerationRateLimitKey, repo.modelRateLimitCalls[0].scope)
	_, wholeAccountBlocked := svc.openaiAccountRuntimeBlockUntil.Load(account.ID)
	require.False(t, wholeAccountBlocked)
REDACTED

func TestOpenAIGatewayServiceForwardImages_ImageRateLimitReturnsFailoverAndCoolsCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &modelNotFoundAccountRepoStub{REDACTED
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat"REDACTED`)
	errorBody := `{"error":{"type":"rate_limit_exceeded","message":"Rate limit reached for gpt-image-2-codex (for limit gpt-image) in organization org on input-images per min: Limit 4000, Used 4000. Please try again in 1s."REDACTEDREDACTED`

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{
		rateLimitService: &RateLimitService{accountRepo: repoREDACTED,
		httpUpstream: &httpUpstreamRecorder{
			resp: &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"X-Request-Id": []string{"req_img_rate_limited"REDACTEDREDACTED,
				Body:       io.NopCloser(strings.NewReader(errorBody)),
		REDACTED,
	REDACTED,
REDACTED
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
REDACTED
	account := &Account{
		ID:       204,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "token-123",
	REDACTED,
REDACTED

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "input-images per min")
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, openAIImageGenerationRateLimitKey, repo.modelRateLimitCalls[0].scope)
REDACTED
