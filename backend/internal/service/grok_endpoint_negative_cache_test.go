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

func TestGrokEndpoint405NegativeCacheIsAccountAndEndpointScopedFor24Hours(t *testing.T) {
	gin.SetMode(gin.TestMode)

	baseTime := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	now := baseTime
	firstAccount := healthyGrokOAuthGatewayTestAccount(6101, "access-token-a")
	secondAccount := healthyGrokOAuthGatewayTestAccount(6102, "access-token-b")
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{
				firstAccount.ID:  firstAccount,
				secondAccount.ID: secondAccount,
			},
		},
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		grokEndpointCacheTestResponse(http.StatusMethodNotAllowed, `{"error":{"message":"method not allowed"}}`),
		grokEndpointCacheTestResponse(http.StatusOK, `{"id":"resp_other_account","object":"response","model":"grok-4.5","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`),
		grokEndpointCacheTestResponse(http.StatusOK, `{"id":"chatcmpl_other_endpoint","object":"chat.completion","model":"grok-4.5","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1}}`),
		grokEndpointCacheTestResponse(http.StatusOK, `{"id":"resp_after_expiry","object":"response","model":"grok-4.5","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
		grokEndpointNow:   func() time.Time { return now },
	}
	responsesBody := []byte(`{"model":"grok","input":"hello","stream":false}`)

	result, err := svc.forwardGrokResponses(
		context.Background(),
		newGrokEndpointCacheTestContext("/v1/responses", responsesBody),
		firstAccount,
		responsesBody,
		"grok",
		false,
		now,
	)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusMethodNotAllowed, failoverErr.StatusCode)
	require.Len(t, upstream.requests, 1)

	// The same account and endpoint fail over locally; no second upstream call.
	result, err = svc.forwardGrokResponses(
		context.Background(),
		newGrokEndpointCacheTestContext("/v1/responses", responsesBody),
		firstAccount,
		responsesBody,
		"grok",
		false,
		now,
	)
	require.Nil(t, result)
	require.ErrorAs(t, err, &failoverErr)
	require.Len(t, upstream.requests, 1)

	// The negative entry belongs to the account that observed the 405.
	result, err = svc.forwardGrokResponses(
		context.Background(),
		newGrokEndpointCacheTestContext("/v1/responses", responsesBody),
		secondAccount,
		responsesBody,
		"grok",
		false,
		now,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_other_account", result.ResponseID)
	require.Len(t, upstream.requests, 2)

	// A different endpoint on the cached account remains usable.
	chatBody := []byte(`{"model":"grok","messages":[{"role":"user","content":"hello"}],"stream":false,"stop":"done"}`)
	result, err = svc.ForwardAsChatCompletions(
		context.Background(),
		newGrokEndpointCacheTestContext("/v1/chat/completions", chatBody),
		firstAccount,
		chatBody,
		"",
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 3)
	require.Equal(t, "/v1/chat/completions", upstream.requests[2].URL.Path)

	// The entry remains active for the full 24-hour window.
	now = baseTime.Add(grokEndpointUnsupportedTTL - time.Nanosecond)
	result, err = svc.forwardGrokResponses(
		context.Background(),
		newGrokEndpointCacheTestContext("/v1/responses", responsesBody),
		firstAccount,
		responsesBody,
		"grok",
		false,
		now,
	)
	require.Nil(t, result)
	require.ErrorAs(t, err, &failoverErr)
	require.Len(t, upstream.requests, 3)

	// At exactly 24 hours the endpoint is probed again and can recover.
	now = baseTime.Add(grokEndpointUnsupportedTTL)
	result, err = svc.forwardGrokResponses(
		context.Background(),
		newGrokEndpointCacheTestContext("/v1/responses", responsesBody),
		firstAccount,
		responsesBody,
		"grok",
		false,
		now,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_after_expiry", result.ResponseID)
	require.Len(t, upstream.requests, 4)
	require.Equal(t, "/v1/responses", upstream.requests[3].URL.Path)
}

func newGrokEndpointCacheTestContext(path string, body []byte) *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("api_key", &APIKey{ID: 6100})
	return c
}

func grokEndpointCacheTestResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
