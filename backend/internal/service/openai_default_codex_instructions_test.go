package service

import (
	"bytes"
	"context"
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

func TestAccount_IsOpenAIDefaultCodexInstructionsDisabled(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{
			name: "enabled for OpenAI",
			account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{
				"openai_disable_default_codex_instructions": true,
			}},
			want: true,
		},
		{
			name: "explicit false",
			account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{
				"openai_disable_default_codex_instructions": false,
			}},
		},
		{
			name: "invalid value type",
			account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{
				"openai_disable_default_codex_instructions": "true",
			}},
		},
		{
			name: "other platform",
			account: &Account{Platform: PlatformAnthropic, Extra: map[string]any{
				"openai_disable_default_codex_instructions": true,
			}},
		},
		{name: "nil account"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.IsOpenAIDefaultCodexInstructionsDisabled())
		})
	}
}

func TestOpenAIGatewayService_DefaultCodexInstructionsAccountPolicy_APIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name             string
		disableInjection any
		body             string
		wantInstructions string
		wantExists       bool
	}{
		{
			name:             "default behavior remains enabled",
			body:             `{"model":"gpt-5.4","stream":false,"input":"hello"}`,
			wantInstructions: defaultCodexSynthInstructions("gpt-5.4"),
			wantExists:       true,
		},
		{
			name:             "disabled leaves missing instructions absent",
			disableInjection: true,
			body:             `{"model":"gpt-5.4","stream":false,"input":"hello"}`,
			wantExists:       false,
		},
		{
			name:             "disabled preserves blank client instructions",
			disableInjection: true,
			body:             `{"model":"gpt-5.4","stream":false,"instructions":"   ","input":"hello"}`,
			wantInstructions: "   ",
			wantExists:       true,
		},
		{
			name:             "disabled preserves explicit client instructions",
			disableInjection: true,
			body:             `{"model":"gpt-5.4","stream":false,"instructions":"client guidance","input":"hello"}`,
			wantInstructions: "client guidance",
			wantExists:       true,
		},
		{
			name:             "invalid policy type keeps default behavior",
			disableInjection: "true",
			body:             `{"model":"gpt-5.4","stream":false,"input":"hello"}`,
			wantInstructions: defaultCodexSynthInstructions("gpt-5.4"),
			wantExists:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":1}}`)),
			}}
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
			extra := map[string]any{"use_responses_api": true}
			if tt.disableInjection != nil {
				extra["openai_disable_default_codex_instructions"] = tt.disableInjection
			}
			account := &Account{
				ID:          1,
				Name:        "openai-apikey",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
				Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://example.com"},
				Extra:       extra,
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", bytes.NewBufferString(tt.body))
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

			result, err := svc.Forward(context.Background(), c, account, []byte(tt.body))
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)

			instructions := gjson.GetBytes(upstream.lastBody, "instructions")
			require.Equal(t, tt.wantExists, instructions.Exists())
			if tt.wantExists {
				require.Equal(t, tt.wantInstructions, instructions.String())
			}
		})
	}
}

func TestOpenAIGatewayService_DefaultCodexInstructionsAccountPolicy_OAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name             string
		body             string
		wantInstructions string
		wantInputRole    string
	}{
		{
			name:             "missing instructions becomes empty compatibility field",
			body:             `{"model":"gpt-5.4","stream":true,"input":[{"type":"message","role":"user","content":"hello"}]}`,
			wantInstructions: "",
			wantInputRole:    "user",
		},
		{
			name:             "client system guidance is promoted without base prompt",
			body:             `{"model":"gpt-5.4","stream":true,"input":[{"type":"message","role":"system","content":"client guidance"},{"type":"message","role":"user","content":"hello"}]}`,
			wantInstructions: "client guidance",
			wantInputRole:    "developer",
		},
		{
			name:             "explicit instructions are preserved",
			body:             `{"model":"gpt-5.4","stream":true,"instructions":"client instructions","input":[{"type":"message","role":"user","content":"hello"}]}`,
			wantInstructions: "client instructions",
			wantInputRole:    "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"stop after capture"}}`)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{
				ID:          2,
				Name:        "openai-oauth",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Concurrency: 1,
				Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"},
				Extra:       map[string]any{"openai_disable_default_codex_instructions": true},
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")

			result, err := svc.Forward(context.Background(), c, account, []byte(tt.body))
			require.Error(t, err)
			require.Nil(t, result)
			require.NotNil(t, upstream.lastReq)
			require.True(t, gjson.GetBytes(upstream.lastBody, "instructions").Exists())
			require.Equal(t, tt.wantInstructions, gjson.GetBytes(upstream.lastBody, "instructions").String())
			require.Equal(t, tt.wantInputRole, gjson.GetBytes(upstream.lastBody, "input.0.role").String())
			require.NotContains(t, string(upstream.lastBody), defaultCodexSynthInstructions("gpt-5.4"))
		})
	}
}
