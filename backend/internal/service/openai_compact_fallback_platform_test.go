package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResolveOpenAICompactFallbackModelKeepsGlobalOpenAIModelOffForeignPlatforms(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"}}}
	tests := []struct {
		name    string
		account *Account
		want    string
	}{
		{
			name:    "openai api key keeps the global fallback",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			want:    "gpt-5.4",
		},
		{
			name:    "deepseek with empty mapping never receives an OpenAI model",
			account: &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey},
			want:    "",
		},
		{
			name:    "kimi empty mapping is allow-all for clients, not proof the upstream serves OpenAI models",
			account: &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey},
			want:    "",
		},
		{
			name: "deepseek mapping that does not declare the fallback",
			account: &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"model_mapping": map[string]any{"deepseek-v4-flash": "deepseek-v4-flash"},
			}},
			want: "",
		},
		{
			name: "deepseek mapping that declares the fallback is honored and mapped",
			account: &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"model_mapping": map[string]any{"gpt-5.4": "deepseek-v4-pro"},
			}},
			want: "deepseek-v4-pro",
		},
		{
			name: "account compact_model_mapping still wins on any platform",
			account: &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"compact_model_mapping": map[string]any{"deepseek-v4-flash": "deepseek-v4-pro"},
			}},
			want: "deepseek-v4-pro",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, svc.resolveOpenAICompactFallbackModel(tt.account, "deepseek-v4-flash"))
		})
	}
}

func deepSeekNativeCompactRequest(t *testing.T) (*gin.Context, []byte) {
	t.Helper()
	body := []byte(`{"model":"deepseek-v4-flash","stream":false,"instructions":"compact-test","input":[{"type":"message","role":"user","content":"hello"},{"type":"compaction_trigger"}]}`)
	c := newOpenAICompactFallbackTestContext(t, "/v1/responses")
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	MarkOpenAINativeCompactionV2(c)
	return c, body
}

func deepSeekResponsesAccount(credentials map[string]any) *Account {
	creds := map[string]any{"api_key": "sk-test", "api_protocol": APIProtocolResponses}
	for k, v := range credentials {
		creds[k] = v
	}
	return &Account{
		ID: 7, Name: "deepseek", Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: creds, Status: StatusActive, Schedulable: true,
	}
}

func modelUnsupportedResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"model_not_supported","message":"The requested model is not supported"}}`)),
	}
}

// #7330: a DeepSeek account must not be retried with the global OpenAI compact
// model. That retry can only fail with model_not_found on the DeepSeek upstream.
func TestOpenAIGatewayForwardDoesNotSendGlobalOpenAICompactModelToDeepSeek(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, body := deepSeekNativeCompactRequest(t)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{modelUnsupportedResponse(), modelUnsupportedResponse()}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"}},
		httpUpstream: upstream,
	}

	_, err := svc.Forward(context.Background(), c, deepSeekResponsesAccount(nil), body)

	require.Error(t, err)
	require.Len(t, upstream.bodies, 1, "no fallback retry on a DeepSeek account without a declared fallback model")
	require.Equal(t, "deepseek-v4-flash", gjson.GetBytes(upstream.bodies[0], "model").String())
	if rawEvents, ok := c.Get(OpsUpstreamErrorsKey); ok {
		events, _ := rawEvents.([]*OpsUpstreamErrorEvent)
		for _, ev := range events {
			require.NotEqual(t, "compact_model_fallback", ev.Reason)
		}
	}
}

func TestOpenAIGatewayForwardHonorsDeclaredCompactFallbackOnDeepSeek(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, body := deepSeekNativeCompactRequest(t)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		modelUnsupportedResponse(),
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_compact","status":"completed","model":"deepseek-v4-pro","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.4"}},
		httpUpstream: upstream,
	}
	account := deepSeekResponsesAccount(map[string]any{
		"model_mapping": map[string]any{"deepseek-v4-flash": "deepseek-v4-flash", "gpt-5.4": "deepseek-v4-pro"},
	})

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "deepseek-v4-pro", gjson.GetBytes(upstream.bodies[1], "model").String())
}
