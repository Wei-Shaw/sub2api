package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func antigravityTestReadCloser(body string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(body))
}

const antigravityRequestedEntityNotFoundBody = `{"error":{"code":404,"message":"Requested entity was not found.","status":"NOT_FOUND"}}`

func antigravityRequestedEntityNotFoundAccount() *Account {
	return &Account{
		ID:          42,
		Name:        "antigravity-model-gap",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "token",
			"project_id":   "project-42",
			"model_mapping": map[string]any{
				"claude-opus-4-6":     "claude-opus-4-6",
				"gemini-3.1-pro-high": "gemini-3.1-pro-high",
			},
		},
	}
}

func TestAntigravityRequestedEntityNotFoundTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("claude forward", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		body := []byte(`{"model":"claude-opus-4-6","messages":[{"role":"user","content":"hi"}],"max_tokens":16,"stream":false}`)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

		svc := &AntigravityGatewayService{
			tokenProvider: &AntigravityTokenProvider{},
			httpUpstream:  &httpUpstreamStub{resp: antigravityRequestedEntityNotFoundResponse()},
		}

		result, err := svc.Forward(context.Background(), c, antigravityRequestedEntityNotFoundAccount(), body, false)
		assertAntigravityRequestedEntityFailover(t, result, err, recorder, c)
	})

	t.Run("claude forward v1internal wrapped", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		body := []byte(`{"model":"claude-opus-4-6","messages":[{"role":"user","content":"hi"}],"max_tokens":16,"stream":false}`)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

		svc := &AntigravityGatewayService{
			tokenProvider: &AntigravityTokenProvider{},
			httpUpstream: &httpUpstreamStub{resp: antigravityRequestedEntityNotFoundResponseWith(
				`{"response":{"error":{"code":404,"message":"Requested entity was not found."}}}`,
			)},
		}

		result, err := svc.Forward(context.Background(), c, antigravityRequestedEntityNotFoundAccount(), body, false)
		assertAntigravityRequestedEntityFailover(t, result, err, recorder, c)
	})

	t.Run("claude forward plain text", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		body := []byte(`{"model":"claude-opus-4-6","messages":[{"role":"user","content":"hi"}],"max_tokens":16,"stream":false}`)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

		svc := &AntigravityGatewayService{
			tokenProvider: &AntigravityTokenProvider{},
			httpUpstream:  &httpUpstreamStub{resp: antigravityRequestedEntityNotFoundResponseWith("Requested entity was not found.")},
		}

		result, err := svc.Forward(context.Background(), c, antigravityRequestedEntityNotFoundAccount(), body, false)
		assertAntigravityRequestedEntityFailover(t, result, err, recorder, c)
	})

	t.Run("gemini forward", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3.1-pro-high:generateContent", nil)

		svc := &AntigravityGatewayService{
			tokenProvider: &AntigravityTokenProvider{},
			httpUpstream:  &httpUpstreamStub{resp: antigravityRequestedEntityNotFoundResponse()},
		}

		result, err := svc.ForwardGemini(context.Background(), c, antigravityRequestedEntityNotFoundAccount(), "gemini-3.1-pro-high", "generateContent", false, body, false)
		assertAntigravityRequestedEntityFailover(t, result, err, recorder, c)
	})

	t.Run("chat completions compat", func(t *testing.T) {
		body := []byte(`{"model":"gemini-3.1-pro-high","messages":[{"role":"user","content":"hi"}]}`)
		c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)
		svc := newAntigravityCompatService(
			config.GatewayConfig{MaxLineSize: defaultMaxLineSize},
			&httpUpstreamStub{resp: antigravityRequestedEntityNotFoundResponse()},
		)

		result, err := svc.ForwardAsChatCompletions(context.Background(), c, antigravityRequestedEntityNotFoundAccount(), body, nil)
		assertAntigravityRequestedEntityFailover(t, result, err, recorder, c)
	})

	t.Run("responses compat", func(t *testing.T) {
		body := []byte(`{"model":"gemini-3.1-pro-high","input":"hi"}`)
		c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/responses", body)
		svc := newAntigravityCompatService(
			config.GatewayConfig{MaxLineSize: defaultMaxLineSize},
			&httpUpstreamStub{resp: antigravityRequestedEntityNotFoundResponse()},
		)

		result, err := svc.ForwardAsResponses(context.Background(), c, antigravityRequestedEntityNotFoundAccount(), body, nil)
		assertAntigravityRequestedEntityFailover(t, result, err, recorder, c)
	})
}

func TestAntigravityNonModelUnavailableErrorsRemainTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		statusCode int
		body       string
		wantStatus int
	}{
		{
			name:       "400 with entity message is still a request error",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"code":400,"message":"Requested entity was not found."}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "generic 404 is not a model availability signal",
			statusCode: http.StatusNotFound,
			body:       `{"error":{"message":"endpoint not found"}}`,
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "404 with extra text is not a model availability signal",
			statusCode: http.StatusNotFound,
			body:       `{"error":{"message":"Requested entity was not found. extra"}}`,
			wantStatus: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			body := []byte(`{"model":"claude-opus-4-6","messages":[{"role":"user","content":"hi"}],"max_tokens":16,"stream":false}`)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

			svc := &AntigravityGatewayService{
				tokenProvider: &AntigravityTokenProvider{},
				httpUpstream: &httpUpstreamStub{resp: &http.Response{
					StatusCode: tt.statusCode,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       antigravityTestReadCloser(tt.body),
				}},
			}

			result, err := svc.Forward(context.Background(), c, antigravityRequestedEntityNotFoundAccount(), body, false)
			require.Nil(t, result)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr), "ordinary 400/404 must not trigger account failover")
			require.Equal(t, tt.wantStatus, recorder.Code)
			require.True(t, IsResponseCommitted(c))
		})
	}
}

func antigravityRequestedEntityNotFoundResponse() *http.Response {
	return antigravityRequestedEntityNotFoundResponseWith(antigravityRequestedEntityNotFoundBody)
}

func antigravityRequestedEntityNotFoundResponseWith(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       antigravityTestReadCloser(body),
	}
}

func assertAntigravityRequestedEntityFailover(
	t *testing.T,
	result *ForwardResult,
	err error,
	recorder *httptest.ResponseRecorder,
	c *gin.Context,
) {
	t.Helper()
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusNotFound, failoverErr.StatusCode)
	require.True(t, failoverErr.ShouldRetryNextAccount())
	require.False(t, failoverErr.RetryableOnSameAccount, "the same account cannot serve the missing model")
	require.False(t, failoverErr.ForceCacheBilling)
	require.False(t, IsResponseCommitted(c))
	require.Empty(t, recorder.Body.String())
}
