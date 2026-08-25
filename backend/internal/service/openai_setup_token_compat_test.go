package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIsOpenAIOAuthLike(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
		codex   bool
	}{
		{name: "openai_oauth", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, want: true, codex: true},
		{name: "openai_setup_token", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken}, want: true, codex: true},
		{name: "openai_api_key", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, want: false, codex: false},
		{name: "anthropic_setup_token", account: &Account{Platform: PlatformAnthropic, Type: AccountTypeSetupToken}, want: false, codex: false},
		{name: "grok_setup_token", account: &Account{Platform: PlatformGrok, Type: AccountTypeSetupToken}, want: false, codex: false},
		{name: "nil", account: nil, want: false, codex: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.IsOpenAIOAuthLike())
			require.Equal(t, tt.codex, tt.account.UsesOpenAICodexProtocol())
		})
	}
}

func TestOpenAIGatewayServiceGetAccessTokenSetupToken(t *testing.T) {
	svc := &OpenAIGatewayService{openAITokenProvider: &OpenAITokenProvider{}}
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeSetupToken,
		Credentials: map[string]any{"access_token": "setup-token-value"},
	}

	token, tokenType, err := svc.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "setup-token-value", token)
	require.Equal(t, "oauth", tokenType)

	delete(account.Credentials, "access_token")
	_, _, err = svc.GetAccessToken(context.Background(), account)
	require.EqualError(t, err, "access_token not found in credentials")

	for _, platform := range []string{PlatformAnthropic, PlatformGrok} {
		foreign := &Account{
			Platform:    platform,
			Type:        AccountTypeSetupToken,
			Credentials: map[string]any{"access_token": "foreign-token"},
		}
		_, _, err = svc.GetAccessToken(context.Background(), foreign)
		require.EqualError(t, err, "unsupported account type: setup-token")
	}
}

func TestOpenAISetupTokenImagesUsesOAuthResponsesPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:          73,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeSetupToken,
		Credentials: map[string]any{"access_token": "setup-token"},
	}
	parsed := &OpenAIImagesRequest{
		Endpoint:       openAIImagesGenerationsEndpoint,
		Model:          "gpt-image-2",
		Prompt:         "draw a square",
		N:              1,
		ResponseFormat: "b64_json",
	}

	result, err := svc.ForwardImages(context.Background(), c, account, nil, parsed, "")

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.True(t, failoverErr.RetryableOnSameAccount)
	require.False(t, failoverErr.SameAccountRetryDeadline.IsZero())
	require.Contains(t, upstream.lastReq.URL.String(), "/backend-api/codex/responses")
}

func TestOpenAISetupTokenWSCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{}`))
	c.Request.Header.Set("session_id", "session-one")
	c.Set("api_key", &APIKey{ID: 17})

	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeSetupToken,
		Credentials: map[string]any{
			"access_token":       "setup-token-value",
			"chatgpt_account_id": "chatgpt-setup",
		},
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	wsURL, err := svc.buildOpenAIResponsesWSURL(account)
	require.NoError(t, err)
	require.Equal(t, "wss://chatgpt.com/backend-api/codex/responses", wsURL)
	foreignURL, err := svc.buildOpenAIResponsesWSURL(&Account{Platform: PlatformGrok, Type: AccountTypeSetupToken})
	require.NoError(t, err)
	require.Equal(t, "wss://api.openai.com/v1/responses", foreignURL)

	headers, session, err := svc.buildOpenAIWSHeaders(
		context.Background(), c, account, "setup-token-value",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		true, "", "", "", "gpt-5.1-codex", "",
	)
	require.NoError(t, err)
	require.Equal(t, "Bearer setup-token-value", headers.Get("authorization"))
	require.Equal(t, "chatgpt-setup", headers.Get("chatgpt-account-id"))
	require.NotEmpty(t, headers.Get("originator"))
	require.Equal(t, "session-one", session.SessionID)
	require.NotEqual(t, session.SessionID, headers.Get("session_id"))

	payload := svc.buildOpenAIWSCreatePayload(map[string]any{"store": true}, account)
	require.Equal(t, false, payload["store"])
}

func TestOpenAISetupTokenChatCompletionsUsesCodexTransform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"setup instructions"},{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop after request capture"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := openAISetupTokenCompatAccount(71)

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.4")

	require.Error(t, err)
	require.Nil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, chatgptCodexURL, upstream.lastReq.URL.String())
	require.Equal(t, "Bearer setup-token-value", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "chatgpt-setup", upstream.lastReq.Header.Get("chatgpt-account-id"))
	require.NotEmpty(t, upstream.lastReq.Header.Get("originator"))
	require.Equal(t, "setup instructions", gjson.GetBytes(upstream.lastBody, "instructions").String())
	require.Equal(t, int64(1), gjson.GetBytes(upstream.lastBody, "input.#").Int())
	require.Equal(t, "user", gjson.GetBytes(upstream.lastBody, "input.0.role").String())
	require.NotEmpty(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
}

func TestOpenAISetupTokenMessagesUsesCodexBridgeAndTurnState(t *testing.T) {
	gin.SetMode(gin.TestMode)

	firstResp := openAICompatSSECompletedResponse("resp_setup_first", "gpt-5.4")
	firstResp.Header.Set("x-codex-turn-state", "turn_state_setup")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		firstResp,
		openAICompatSSECompletedResponse("resp_setup_second", "gpt-5.4"),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		httpUpstream: upstream,
	}
	account := openAISetupTokenCompatAccount(72)

	messages := make([]string, 0, openAICompatAnthropicReplayMaxTailMessages+3)
	for i := 0; i < openAICompatAnthropicReplayMaxTailMessages+3; i++ {
		messages = append(messages, `{"role":"user","content":"message-`+fmt.Sprintf("%02d", i)+`"}`)
	}
	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[` + strings.Join(messages, ",") + `],"stream":false}`)
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "stable-cache-key", "gpt-5.4")

	require.NoError(t, err)
	require.NotNil(t, firstResult)
	require.True(t, isOpenAICompatMessagesBridgeContext(firstCtx))
	require.Equal(t, int64(openAICompatAnthropicReplayMaxTailMessages+4), gjson.GetBytes(upstream.bodies[0], "input.#").Int())
	require.Equal(t, "developer", gjson.GetBytes(upstream.bodies[0], "input.0.role").String())
	require.Contains(t, gjson.GetBytes(upstream.bodies[0], "input.0.content.0.text").String(), openAICompatClaudeCodeTodoGuardMarker)
	require.Equal(t, "message-00", gjson.GetBytes(upstream.bodies[0], "input.1.content.0.text").String())
	require.False(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").Exists())
	require.Equal(t, chatgptCodexURL, upstream.requests[0].URL.String())
	require.Equal(t, "Bearer setup-token-value", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "chatgpt-setup", upstream.requests[0].Header.Get("chatgpt-account-id"))
	requireOpenAIMessagesCodexIdentity(t, upstream.requests[0], codexCLIUserAgent, "codex-tui")
	require.Empty(t, upstream.requests[0].Header.Get("x-codex-turn-state"))

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"next"}],"stream":false}`)
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "stable-cache-key", "gpt-5.4")

	require.NoError(t, err)
	require.NotNil(t, secondResult)
	require.True(t, isOpenAICompatMessagesBridgeContext(secondCtx))
	require.Equal(t, "turn_state_setup", upstream.requests[1].Header.Get("x-codex-turn-state"))
	require.Equal(t, generateSessionUUID(isolateOpenAIUpstreamSessionID(0, account, "stable-cache-key")), upstream.requests[1].Header.Get("session_id"))
	require.Empty(t, upstream.requests[1].Header.Get("conversation_id"))
	requireOpenAIMessagesCodexIdentity(t, upstream.requests[1], codexCLIUserAgent, "codex-tui")
}

func openAISetupTokenCompatAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "openai-setup-token",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeSetupToken,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "setup-token-value",
			"chatgpt_account_id": "chatgpt-setup",
		},
	}
}
