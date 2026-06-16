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
	imageBody := []byte(`{"error":{"message":"Rate limit reached for gpt-image-2-codex (for limit gpt-image) in organization org on input-images per min: Limit 4000, Used 4000. Please try again in 467ms."}}`)
	textBody := []byte(`{"error":{"message":"Rate limit reached for gpt-5.4 in organization org on tokens per min: Limit 30000, Used 30000. Please try again in 1s."}}`)

	require.True(t, isOpenAIImageRateLimitError(http.StatusTooManyRequests, imageBody))
	require.False(t, isOpenAIImageRateLimitError(http.StatusTooManyRequests, textBody))
	require.False(t, isOpenAIImageRateLimitError(http.StatusBadRequest, imageBody))
}

func TestRateLimitService_HandleOpenAIImageRateLimit_ParsesTryAgainCooldown(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	account := &Account{ID: 201, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := []byte(`{"error":{"type":"rate_limit_exceeded","message":"Rate limit reached for gpt-image-2-codex (for limit gpt-image) on input-images per min. Please try again in 2s."}}`)

	before := time.Now()
	handled := svc.HandleOpenAIImageRateLimit(context.Background(), account, http.StatusTooManyRequests, http.Header{}, body)

	require.True(t, handled)
	require.Len(t, repo.modelRateLimitCalls, 1)
	call := repo.modelRateLimitCalls[0]
	require.Equal(t, account.ID, call.accountID)
	require.Equal(t, openAIImageGenerationRateLimitKey, call.scope)
	require.Equal(t, openAIImageRateLimitReason, call.reason)
	require.WithinDuration(t, before.Add(2*time.Second), call.resetAt, time.Second)
}

func TestRateLimitService_HandleOpenAIImageRateLimit_DefaultsToOneMinute(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	account := &Account{ID: 202, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := []byte(`{"error":{"type":"rate_limit_exceeded","message":"Rate limit reached for gpt-image-2-codex (for limit gpt-image) on input-images per min."}}`)

	before := time.Now()
	handled := svc.HandleOpenAIImageRateLimit(context.Background(), account, http.StatusTooManyRequests, http.Header{}, body)

	require.True(t, handled)
	require.Len(t, repo.modelRateLimitCalls, 1)
	call := repo.modelRateLimitCalls[0]
	require.Equal(t, openAIImageGenerationRateLimitKey, call.scope)
	require.Equal(t, openAIImageRateLimitReason, call.reason)
	require.WithinDuration(t, before.Add(time.Minute), call.resetAt, time.Second)
}

func TestOpenAIGatewayService_HandleOpenAIAccountUpstreamError_ImageRateLimitDoesNotBlockWholeAccount(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{}
	svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
	account := &Account{ID: 203, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := []byte(`{"error":{"type":"rate_limit_exceeded","message":"Rate limit reached for gpt-image-2-codex (for limit gpt-image) on input-images per min. Please try again in 1s."}}`)

	disabled := svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{}, body, "gpt-image-2")

	require.False(t, disabled)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, openAIImageGenerationRateLimitKey, repo.modelRateLimitCalls[0].scope)
	_, wholeAccountBlocked := svc.openaiAccountRuntimeBlockUntil.Load(account.ID)
	require.False(t, wholeAccountBlocked)
}

func TestOpenAIGatewayServiceForwardImages_ImageRateLimitReturnsFailoverAndCoolsCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &modelNotFoundAccountRepoStub{}
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat"}`)
	errorBody := `{"error":{"type":"rate_limit_exceeded","message":"Rate limit reached for gpt-image-2-codex (for limit gpt-image) in organization org on input-images per min: Limit 4000, Used 4000. Please try again in 1s."}}`

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{
		rateLimitService: &RateLimitService{accountRepo: repo},
		httpUpstream: &httpUpstreamRecorder{
			resp: &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"X-Request-Id": []string{"req_img_rate_limited"}},
				Body:       io.NopCloser(strings.NewReader(errorBody)),
			},
		},
	}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	account := &Account{
		ID:       204,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "token-123",
		},
	}

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "input-images per min")
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, openAIImageGenerationRateLimitKey, repo.modelRateLimitCalls[0].scope)
}
