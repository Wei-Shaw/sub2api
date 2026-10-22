package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexTimezoneCompatibilityRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, route := range []string{"chat_responses", "chat_messages", "anthropic", "legacy_messages", "legacy_prompt"} {
		t.Run(route, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{err: errors.New("stop after capturing outbound body")}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := codexTimezoneRoutesAccount()
			request := map[string]any{"model": "gpt-5.4", "stream": true, "instructions": "Reply briefly."}
			switch route {
			case "chat_responses":
				request["input"] = []any{map[string]any{"role": "user", "content": codexTimezoneTestContext}}
			case "legacy_prompt":
				request["prompt"] = codexTimezoneTestContext
			default:
				request["messages"] = []any{map[string]any{"role": "user", "content": codexTimezoneTestContext}}
				request["max_tokens"] = 32
			}
			body, err := json.Marshal(request)
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			path := "/v1/responses"
			if route == "chat_responses" || route == "chat_messages" {
				path = "/v1/chat/completions"
			}
			if route == "anthropic" {
				path = "/v1/messages"
			}
			c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			switch route {
			case "chat_responses", "chat_messages":
				_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			case "anthropic":
				_, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
			default:
				_, err = svc.Forward(context.Background(), c, account, body)
			}
			require.Error(t, err)
			require.NotEmpty(t, upstream.lastBody)
			require.Contains(t, string(upstream.lastBody), "Asia/Taipei")
			require.NotContains(t, string(upstream.lastBody), "America/Los_Angeles")
			require.Contains(t, string(body), "America/Los_Angeles", "the original client body must remain intact")
		})
	}
}

func codexTimezoneRoutesAccount() *Account {
	return &Account{
		ID: 1724, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"},
		Extra:       map[string]any{"codex_timezone": "Asia/Taipei"},
	}
}

func TestCodexTimezoneWSDefaultPoolMultipleTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 5
	capture := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.completed","response":{"id":"resp_timezone_pool_1","model":"gpt-5.4","usage":{"input_tokens":1,"output_tokens":1}}}`),
		[]byte(`{"type":"response.completed","response":{"id":"resp_timezone_pool_2","model":"gpt-5.4","usage":{"input_tokens":1,"output_tokens":1}}}`),
	}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: capture})
	defer pool.Close()
	svc := &OpenAIGatewayService{
		cfg: cfg, httpUpstream: &httpUpstreamRecorder{}, cache: &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool,
	}
	account := codexTimezoneRoutesAccount()
	// Leave the account mode unset to exercise the actual ctx_pool default.
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	server, serverErr := startPassthroughLifecycleServer(t, ctx, svc, account)
	defer server.Close()
	body, err := json.Marshal(map[string]any{
		"type": "response.create", "model": "gpt-5.4", "stream": true,
		"input": []any{map[string]any{"role": "user", "content": codexTimezoneTestContext}},
	})
	require.NoError(t, err)
	client := dialPassthroughLifecycleClientWithPayload(t, server, string(body))
	defer func() { _ = client.CloseNow() }()
	for turn := 0; turn < 2; turn++ {
		if turn > 0 {
			writeCtx, cancelWrite := context.WithTimeout(ctx, 3*time.Second)
			err = client.Write(writeCtx, coderws.MessageText, body)
			cancelWrite()
			require.NoError(t, err)
		}
		completed, readErr := readPassthroughLifecycleFrame(t, client, 3*time.Second)
		require.NoError(t, readErr)
		require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())
	}
	_ = client.Close(coderws.StatusNormalClosure, "done")
	select {
	case err = <-serverErr:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("default pool websocket did not stop")
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	require.Len(t, capture.writes, 2)
	for turn, payload := range capture.writes {
		out, marshalErr := json.Marshal(payload)
		require.NoError(t, marshalErr)
		require.Contains(t, string(out), "Asia/Taipei", "turn %d", turn+1)
		require.NotContains(t, string(out), "America/Los_Angeles", "turn %d", turn+1)
	}
}

func TestCodexTimezoneWSPassthroughLegacyIngress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 5
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	upstream := newStagedPassthroughConn()
	defer func() { _ = upstream.Close() }()
	svc := newPassthroughLifecycleService(cfg, upstream)
	account := codexTimezoneRoutesAccount()
	account.Extra["openai_oauth_responses_websockets_v2_mode"] = OpenAIWSIngressModePassthrough
	server, serverErr := startPassthroughLifecycleServer(t, ctx, svc, account)
	defer server.Close()
	body, err := json.Marshal(map[string]any{
		"type": "response.create", "model": "gpt-5.4", "stream": true,
		"messages": []any{map[string]any{"role": "user", "content": codexTimezoneTestContext}},
	})
	require.NoError(t, err)
	client := dialPassthroughLifecycleClientWithPayload(t, server, string(body))
	defer func() { _ = client.CloseNow() }()
	out := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Contains(t, string(out), "Asia/Taipei")
	require.NotContains(t, string(out), "America/Los_Angeles")
	require.False(t, gjson.GetBytes(out, "messages").Exists(), "the assertion must inspect the normalized upstream frame")
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_timezone_legacy","model":"gpt-5.4","usage":{"input_tokens":1,"output_tokens":1}}}`)
	completed, readErr := readPassthroughLifecycleFrame(t, client, 3*time.Second)
	require.NoError(t, readErr)
	require.Equal(t, "response.completed", gjson.GetBytes(completed, "type").String())
	_ = client.CloseNow()
	cancel(context.Canceled)
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough websocket did not stop")
	}
}

func TestCodexTimezoneLegacyWSForwarder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := passthroughLifecycleConfig()
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	capture := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.completed","response":{"id":"resp_timezone_forward","model":"gpt-5.4","status":"completed","usage":{"input_tokens":1,"output_tokens":1}}}`),
	}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: capture})
	defer pool.Close()
	svc := &OpenAIGatewayService{
		cfg: cfg, httpUpstream: &httpUpstreamRecorder{}, cache: &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool,
	}
	account := codexTimezoneRoutesAccount()
	account.Extra["responses_websockets_v2_enabled"] = true
	body, err := json.Marshal(map[string]any{
		"model": "gpt-5.4", "stream": false,
		"input": []any{map[string]any{"role": "user", "content": codexTimezoneTestContext}},
	})
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	// Unknown client transport preserves this older service-level WS branch.
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.OpenAIWSMode)
	out, err := json.Marshal(capture.lastWrite)
	require.NoError(t, err)
	require.Contains(t, gjson.GetBytes(out, "input.0.content").String(), "Asia/Taipei")
	require.NotContains(t, gjson.GetBytes(out, "input.0.content").String(), "America/Los_Angeles")
}
