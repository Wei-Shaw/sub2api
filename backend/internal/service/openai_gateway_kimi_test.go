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

	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type kimiGatewayAccountRepo struct {
	*mockAccountRepoForPlatform
	tempUnschedCalls      int
	lastTempUnschedID     int64
	lastTempUnschedUntil  time.Time
	lastTempUnschedReason string
}

func (r *kimiGatewayAccountRepo) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.tempUnschedCalls++
	r.lastTempUnschedID = id
	r.lastTempUnschedUntil = until
	r.lastTempUnschedReason = reason
	return nil
}

func kimiOAuthTestAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "kimi",
		Platform:    PlatformKimi,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			"device_id":    "device-123",
		},
	}
}

func TestForwardAsChatCompletionsForKimiMapsModelAndInjectsFingerprintHeaders(t *testing.T) {
	// 指纹头用 env 固定，避免依赖运行环境 GOOS/hostname
	t.Setenv(kimi.EnvDeviceName, "test-host")
	t.Setenv(kimi.EnvDeviceModel, "TestOS 1.0 arm64")
	t.Setenv(kimi.EnvOSVersion, "test-kernel-1")
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"kimi","messages":[{"role":"user","content":"hi"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	account := kimiOAuthTestAccount(71)
	repo := &kimiGatewayAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{71: account},
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(`{"id":"chatcmpl","object":"chat.completion","model":"kimi-for-coding","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2}}`)),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		kimiTokenProvider: NewKimiTokenProvider(repo, nil),
		accountRepo:       repo,
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)

	// 上游 URL 与鉴权
	require.Equal(t, kimi.DefaultBaseURL+"/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, http.MethodPost, upstream.lastReq.Method)
	require.Equal(t, "Bearer access-token", upstream.lastReq.Header.Get("Authorization"))

	// 8 个头：Authorization + 7 个指纹头
	require.Equal(t, "KimiCLI/"+kimi.CLIVersion, upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, "kimi_cli", upstream.lastReq.Header.Get("X-Msh-Platform"))
	require.Equal(t, kimi.CLIVersion, upstream.lastReq.Header.Get("X-Msh-Version"))
	require.Equal(t, "test-host", upstream.lastReq.Header.Get("X-Msh-Device-Name"))
	require.Equal(t, "TestOS 1.0 arm64", upstream.lastReq.Header.Get("X-Msh-Device-Model"))
	require.Equal(t, "test-kernel-1", upstream.lastReq.Header.Get("X-Msh-Os-Version"))
	require.Equal(t, "device-123", upstream.lastReq.Header.Get("X-Msh-Device-Id"), "device_id 必须取 credentials 中的稳定值")

	// body patch：默认映射 kimi → kimi-for-coding
	require.Equal(t, "kimi-for-coding", gjson.GetBytes(upstream.lastBody, "model").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "stream_options").Exists(), "非流式不应强制 stream_options")

	require.Equal(t, "kimi", result.Model)
	require.Equal(t, "kimi-for-coding", result.UpstreamModel)
	require.Equal(t, 1, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestForwardAsChatCompletionsForKimiUsesCustomModelMapping(t *testing.T) {
	t.Setenv(kimi.EnvDeviceName, "test-host")
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"my-alias","messages":[{"role":"user","content":"hi"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	account := kimiOAuthTestAccount(72)
	account.Credentials["model_mapping"] = map[string]any{"my-alias": "k2p7"}
	repo := &kimiGatewayAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{72: account},
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl","object":"chat.completion","model":"k2p7","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4}}`)),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		kimiTokenProvider: NewKimiTokenProvider(repo, nil),
		accountRepo:       repo,
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.Equal(t, "k2p7", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "my-alias", result.Model)
	require.Equal(t, "k2p7", result.UpstreamModel)
}

func TestForwardAsChatCompletionsForKimiStreamingForcesIncludeUsage(t *testing.T) {
	t.Setenv(kimi.EnvDeviceName, "test-host")
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"kimi-for-coding","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	account := kimiOAuthTestAccount(73)
	repo := &kimiGatewayAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{73: account},
		},
	}
	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_kimi","object":"chat.completion.chunk","model":"kimi-for-coding","choices":[{"index":0,"delta":{"content":"ok"}}]}`,
		"",
		`data: {"id":"chatcmpl_kimi","object":"chat.completion.chunk","model":"kimi-for-coding","choices":[],"usage":{"prompt_tokens":6,"completion_tokens":4,"total_tokens":10}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"kimi-stream-req"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:               rawChatCompletionsTestConfig(),
		httpUpstream:      upstream,
		kimiTokenProvider: NewKimiTokenProvider(repo, nil),
		accountRepo:       repo,
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
	require.NoError(t, err)
	require.Equal(t, "text/event-stream", upstream.lastReq.Header.Get("Accept"))
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream_options.include_usage").Bool(), "流式必须强制 include_usage 保证计费完整")
	require.True(t, result.Stream)
	require.Equal(t, 6, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Contains(t, recorder.Body.String(), "data: [DONE]")
}

func TestHandleKimiAccountUpstreamErrorTempUnschedulesReadinessStates(t *testing.T) {
	tests := []struct {
		name            string
		status          int
		headers         http.Header
		wantReason      string
		wantMinCooldown time.Duration
		wantMaxCooldown time.Duration
	}{
		{
			name:            "unauthorized reauth",
			status:          http.StatusUnauthorized,
			wantReason:      "kimi oauth token unauthorized",
			wantMinCooldown: 10*time.Minute - time.Second,
			wantMaxCooldown: 10*time.Minute + time.Second,
		},
		{
			name:            "forbidden fingerprint or subscription",
			status:          http.StatusForbidden,
			wantReason:      "kimi fingerprint or subscription denied",
			wantMinCooldown: 30*time.Minute - time.Second,
			wantMaxCooldown: 30*time.Minute + time.Second,
		},
		{
			name:            "rate limited retry after",
			status:          http.StatusTooManyRequests,
			headers:         http.Header{"Retry-After": []string{"45"}},
			wantReason:      "kimi rate limited",
			wantMinCooldown: 44 * time.Second,
			wantMaxCooldown: 46 * time.Second,
		},
		{
			name:            "upstream 5xx",
			status:          http.StatusBadGateway,
			wantReason:      "kimi upstream temporary error",
			wantMinCooldown: 2*time.Minute - time.Second,
			wantMaxCooldown: 2*time.Minute + time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{ID: 81, Platform: PlatformKimi, Type: AccountTypeOAuth}
			repo := &kimiGatewayAccountRepo{}
			svc := &OpenAIGatewayService{accountRepo: repo}
			before := time.Now()

			svc.handleKimiAccountUpstreamError(context.Background(), account, tt.status, tt.headers, nil)

			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
			require.Equal(t, 1, repo.tempUnschedCalls)
			require.Equal(t, account.ID, repo.lastTempUnschedID)
			require.Equal(t, tt.wantReason, repo.lastTempUnschedReason)
			require.True(t, repo.lastTempUnschedUntil.After(before.Add(tt.wantMinCooldown)))
			require.True(t, repo.lastTempUnschedUntil.Before(before.Add(tt.wantMaxCooldown)))
		})
	}
}

func TestHandleKimiAccountUpstreamErrorDoesNotShortenExistingPause(t *testing.T) {
	existingUntil := time.Now().Add(15 * time.Minute)
	account := &Account{
		ID:                      82,
		Platform:                PlatformKimi,
		Type:                    AccountTypeOAuth,
		TempUnschedulableUntil:  &existingUntil,
		TempUnschedulableReason: "existing pause",
	}
	repo := &kimiGatewayAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}

	svc.handleKimiAccountUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{"Retry-After": []string{"45"}}, nil)

	require.Equal(t, 1, repo.tempUnschedCalls)
	require.WithinDuration(t, existingUntil, repo.lastTempUnschedUntil, time.Second)
	value, ok := svc.openaiAccountRuntimeBlockUntil.Load(account.ID)
	require.True(t, ok)
	runtimeUntil, ok := value.(time.Time)
	require.True(t, ok)
	require.WithinDuration(t, existingUntil, runtimeUntil, time.Second)
}
