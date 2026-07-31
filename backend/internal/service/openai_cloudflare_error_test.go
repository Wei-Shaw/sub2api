//go:build unit

package service

import (
	"bytes"
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

func TestNormalizeOpenAICloudflareForbiddenResponse(t *testing.T) {
	account := &Account{ID: 401, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	originalBody := []byte("<!doctype html><html>Cloudflare error</html>")
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Status:     "403 Forbidden",
		Header: http.Header{
			"Cf-Error-Type":    []string{"waf"},
			"Cf-Ray":           []string{"abc123-SJC"},
			"Content-Type":     []string{"text/html"},
			"Content-Length":   []string{"49"},
			"Content-Encoding": []string{"gzip"},
		},
		Body: io.NopCloser(bytes.NewReader(originalBody)),
	}

	body, normalized := normalizeOpenAICloudflareForbiddenResponse(account, resp, originalBody)

	require.True(t, normalized)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.Equal(t, "404 Not Found", resp.Status)
	require.JSONEq(t, `{"error":{"type":"not_found_error","message":"Not found"}}`, string(body))
	require.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
	require.Empty(t, resp.Header.Get("Content-Length"))
	require.Empty(t, resp.Header.Get("Content-Encoding"))
	require.Equal(t, "abc123-SJC", resp.Header.Get("Cf-Ray"))
	rewound, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, body, rewound)
}

func TestNormalizeOpenAICloudflareForbiddenResponseDoesNotRewriteOrigin403(t *testing.T) {
	account := &Account{ID: 402, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	originalBody := []byte(`{"error":{"message":"workspace access forbidden"}}`)
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header: http.Header{
			"Cf-Ray": []string{"origin403-SJC"},
			"Server": []string{"cloudflare"},
		},
		Body: io.NopCloser(bytes.NewReader(originalBody)),
	}

	body, normalized := normalizeOpenAICloudflareForbiddenResponse(account, resp, originalBody)

	require.False(t, normalized)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.Equal(t, originalBody, body)
}

func TestOpenAICloudflare403Returns404WithoutAccountPenalty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{1}}
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	rateLimitService.SetOpenAI403CounterCache(counter)
	gateway := &OpenAIGatewayService{
		cfg:              &config.Config{},
		rateLimitService: rateLimitService,
	}
	account := &Account{ID: 403, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	cfBody := []byte("<!doctype html><html><title>Cloudflare</title></html>")
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header: http.Header{
			"Cf-Error-Origin": []string{"edge"},
			"Cf-Ray":          []string{"cf403-SJC"},
			"Content-Type":    []string{"text/html"},
		},
		Body: io.NopCloser(bytes.NewReader(cfBody)),
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/not-exist", nil)

	result, err := gateway.handleErrorResponse(
		context.Background(),
		resp,
		c,
		account,
		[]byte(`{"model":"gpt-5"}`),
	)

	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.JSONEq(t, `{"error":{"type":"not_found_error","message":"Not found"}}`, recorder.Body.String())
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 0, repo.tempCalls)
	require.Len(t, counter.counts, 1, "Cloudflare 403 must not increment the OpenAI 403 counter")
}

func TestOpenAIForwardCloudflare403DoesNotFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{1}}
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	rateLimitService.SetOpenAI403CounterCache(counter)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusForbidden,
			Header: http.Header{
				"Cf-Error-Type": []string{"waf"},
				"Cf-Ray":        []string{"forward403-SJC"},
				"Content-Type":  []string{"text/html"},
			},
			Body: io.NopCloser(strings.NewReader("<!doctype html><html>Cloudflare</html>")),
		},
	}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	gateway := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		rateLimitService: rateLimitService,
	}
	account := &Account{
		ID:          405,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://example.com",
		},
		Extra: map[string]any{"use_responses_api": true},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/not-exist", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	result, err := gateway.Forward(
		context.Background(),
		c,
		account,
		[]byte(`{"model":"gpt-5","stream":false,"input":"hello"}`),
	)

	require.Nil(t, result)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "Cloudflare 403 must terminate locally instead of switching accounts")
	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.JSONEq(t, `{"error":{"type":"not_found_error","message":"Not found"}}`, recorder.Body.String())
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "/v1/responses/not-exist", upstream.lastReq.URL.Path)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 0, repo.tempCalls)
	require.Len(t, counter.counts, 1)
}

func TestRateLimitServiceCloudflare403SkipsCustomTempRules(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{1}}
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	rateLimitService.SetOpenAI403CounterCache(counter)
	account := &Account{
		ID:       404,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       http.StatusForbidden,
					"keywords":         []any{"cloudflare"},
					"duration_minutes": 60,
				},
			},
		},
	}

	shouldDisable := rateLimitService.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusForbidden,
		http.Header{"Cf-Mitigated": []string{"challenge"}},
		[]byte("<html>Cloudflare challenge</html>"),
	)

	require.False(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 0, repo.tempCalls)
	require.Len(t, counter.counts, 1)
}
