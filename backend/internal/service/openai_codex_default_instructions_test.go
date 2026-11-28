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
	"github.com/tidwall/sjson"
)

func TestOpenAIGatewayService_ForwardOAuthResponsesUsesConfiguredDefaultInstructions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const upstreamJSON = `{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`

	cases := []struct {
		name         string
		cfg          *config.Config
		instructions string
		want         string
	}{
		{name: "configured default fills empty instructions", cfg: &config.Config{Gateway: config.GatewayConfig{CodexDefaultInstructions: "Be brief."}}, want: "Be brief."},
		{name: "client instructions win", cfg: &config.Config{Gateway: config.GatewayConfig{CodexDefaultInstructions: "Be brief."}}, instructions: "client says hi", want: "client says hi"},
		{name: "unset keeps codex prompt", cfg: &config.Config{}, want: defaultCodexSynthInstructions("gpt-5.4")},
		{name: "nil config keeps codex prompt", cfg: nil, want: defaultCodexSynthInstructions("gpt-5.4")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4","stream":false,"store":false,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)
			if tc.instructions != "" {
				body, _ = sjson.SetBytes(body, "instructions", tc.instructions)
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(upstreamJSON)),
			}}
			svc := &OpenAIGatewayService{cfg: tc.cfg, httpUpstream: upstream}
			account := &Account{
				ID: 9101, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
				Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"},
				Status:      StatusActive, Schedulable: true,
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

			_, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.Equal(t, tc.want, gjson.GetBytes(upstream.lastBody, "instructions").String())
		})
	}
}
