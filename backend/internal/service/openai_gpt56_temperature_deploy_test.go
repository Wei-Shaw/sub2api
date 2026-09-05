package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayOmitsGPT56TemperatureForOAuthUpstream(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		credentials   map[string]any
		wantRequested string
		wantPolicy    string
		wantStatus    string
	}{
		{name: "inherit explicit zero", body: `{"model":"gpt-5.6-sol","stream":false,"temperature":0,"input":"hello"}`, credentials: map[string]any{}, wantRequested: "0", wantPolicy: "inherit", wantStatus: "omitted-unsupported"},
		{name: "inherit omitted temperature", body: `{"model":"gpt-5.6-sol","stream":false,"input":"hello"}`, credentials: map[string]any{}, wantRequested: "omitted", wantPolicy: "inherit", wantStatus: "omitted"},
		{name: "inherit temperature one", body: `{"model":"gpt-5.6-sol","stream":false,"temperature":1,"input":"hello"}`, credentials: map[string]any{}, wantRequested: "1", wantPolicy: "inherit", wantStatus: "omitted-unsupported"},
		{name: "inherit temperature two", body: `{"model":"gpt-5.6-sol","stream":false,"temperature":2,"input":"hello"}`, credentials: map[string]any{}, wantRequested: "2", wantPolicy: "inherit", wantStatus: "omitted-unsupported"},
		{name: "account override", body: `{"model":"gpt-5.6-sol","stream":false,"temperature":0,"input":"hello"}`, credentials: map[string]any{"temperature_mode": "override", "temperature": 0.4}, wantRequested: "0", wantPolicy: "override", wantStatus: "omitted-unsupported"},
		{name: "account omit", body: `{"model":"gpt-5.6-sol","stream":false,"temperature":0.65,"input":"hello"}`, credentials: map[string]any{"temperature_mode": "omit"}, wantRequested: "0.65", wantPolicy: "omit", wantStatus: "omitted-policy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			body := []byte(tt.body)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			upstream := &httpUpstreamRecorder{resp: newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"id":"resp_temperature","object":"response","status":"completed","model":"gpt-5.6-sol","temperature":1,"output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`)}
			service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := newGPT56TemperatureOAuthAccount(tt.credentials)

			result, err := service.Forward(context.Background(), c, account, body)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, gjson.GetBytes(upstream.lastBody, "temperature").Exists())
			require.InDelta(t, 1, gjson.GetBytes(recorder.Body.Bytes(), "temperature").Float(), 1e-9)
			require.Equal(t, tt.wantRequested, recorder.Header().Get(openAIRequestedTemperatureHeader))
			require.Equal(t, "omitted", recorder.Header().Get(openAIForwardedTemperatureHeader))
			require.Equal(t, tt.wantPolicy, recorder.Header().Get(openAITemperaturePolicyHeader))
			require.Equal(t, tt.wantStatus, recorder.Header().Get(openAITemperatureStatusHeader))
		})
	}
}

func TestOpenAIGatewayReportsForwardedTemperatureForCompatibleAPIKey(t *testing.T) {
	body := []byte(`{"model":"gpt-4.1","stream":false,"temperature":0,"input":"hello"}`)
	c := newOpenAIRejectedFieldTestContext(body)
	upstream := &httpUpstreamRecorder{resp: newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"id":"resp_forwarded_temperature","object":"response","status":"completed","model":"gpt-4.1","temperature":1,"output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`)}
	service := newOpenAIRejectedFieldTestService(upstream)
	account := newOpenAIRejectedFieldTestAccount()
	account.Credentials["temperature_mode"] = "override"
	account.Credentials["temperature"] = 0.4

	result, err := service.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.InDelta(t, 0.4, gjson.GetBytes(upstream.lastBody, "temperature").Float(), 1e-9)
	require.Equal(t, "0", c.Writer.Header().Get(openAIRequestedTemperatureHeader))
	require.Equal(t, "0.4", c.Writer.Header().Get(openAIForwardedTemperatureHeader))
	require.Equal(t, "override", c.Writer.Header().Get(openAITemperaturePolicyHeader))
	require.Equal(t, "forwarded", c.Writer.Header().Get(openAITemperatureStatusHeader))
}

func TestOpenAITemperatureObservationPreservesOriginalRequestAcrossRetries(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	account := newOpenAIRejectedFieldTestAccount()

	beginOpenAITemperatureObservation(c, account, []byte(`{"temperature":0}`))
	setOpenAITemperaturePolicyObservation(c, accountTemperaturePolicy{mode: accountTemperatureModeOverride}, []byte(`{"temperature":0.4}`))
	beginOpenAITemperatureObservation(c, account, []byte(`{"temperature":0.4}`))
	observeOpenAIForwardedTemperature(c, []byte(`{}`))

	require.Equal(t, "0", recorder.Header().Get(openAIRequestedTemperatureHeader))
	require.Equal(t, "omitted-unsupported", recorder.Header().Get(openAITemperatureStatusHeader))
}

func TestOpenAITemperatureObservationIsDisabledForNonOpenAIAccounts(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	beginOpenAITemperatureObservation(c, newOpenAIRejectedFieldTestAccount(), []byte(`{"temperature":0}`))
	setOpenAITemperaturePolicyObservation(c, accountTemperaturePolicy{mode: accountTemperatureModeInherit}, []byte(`{"temperature":0}`))
	observeOpenAIForwardedTemperature(c, []byte(`{"temperature":0}`))
	beginOpenAITemperatureObservation(c, &Account{Platform: PlatformGrok}, []byte(`{"temperature":0}`))

	require.Empty(t, recorder.Header().Get(openAIRequestedTemperatureHeader))
	require.Empty(t, recorder.Header().Get(openAIForwardedTemperatureHeader))
	require.Empty(t, recorder.Header().Get(openAITemperaturePolicyHeader))
	require.Empty(t, recorder.Header().Get(openAITemperatureStatusHeader))
}

func TestOpenAIChatCompletionsOmitsAccountTemperatureForMappedGPT56(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true,"temperature":0}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_chat_temperature", "gpt-5.6-sol")}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := newGPT56TemperatureOAuthAccount(map[string]any{"temperature_mode": "override", "temperature": 0.2})

	result, err := service.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.6-sol")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 1)
	require.False(t, gjson.GetBytes(upstream.bodies[0], "temperature").Exists())
	require.Equal(t, "0", recorder.Header().Get(openAIRequestedTemperatureHeader))
	require.Equal(t, "omitted", recorder.Header().Get(openAIForwardedTemperatureHeader))
	require.Equal(t, "override", recorder.Header().Get(openAITemperaturePolicyHeader))
	require.Equal(t, "omitted-unsupported", recorder.Header().Get(openAITemperatureStatusHeader))
}

func TestOpenAIMessagesOmitsAccountTemperatureForMappedGPT56(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4","max_tokens":1024,"messages":[{"role":"user","content":"hello"}],"temperature":0}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_messages_temperature", "gpt-5.6-terra")}
	service := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := newGPT56TemperatureOAuthAccount(map[string]any{"temperature_mode": "override", "temperature": 0.2})

	result, err := service.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.6-terra")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 1)
	require.False(t, gjson.GetBytes(upstream.bodies[0], "temperature").Exists())
	require.Equal(t, "0", recorder.Header().Get(openAIRequestedTemperatureHeader))
	require.Equal(t, "omitted", recorder.Header().Get(openAIForwardedTemperatureHeader))
	require.Equal(t, "override", recorder.Header().Get(openAITemperaturePolicyHeader))
	require.Equal(t, "omitted-unsupported", recorder.Header().Get(openAITemperatureStatusHeader))
}

func newGPT56TemperatureOAuthAccount(extraCredentials map[string]any) *Account {
	credentials := map[string]any{
		"access_token":       "oauth-token",
		"chatgpt_account_id": "chatgpt-account",
	}
	for key, value := range extraCredentials {
		credentials[key] = value
	}
	return &Account{
		ID:          123,
		Name:        "temperature-test",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: credentials,
		Status:      StatusActive,
		Schedulable: true,
	}
}
