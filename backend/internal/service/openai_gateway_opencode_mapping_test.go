//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func openCodeMappedTestAccount() *Account {
	return &Account{
		ID:          801,
		Name:        "oc",
		Platform:    PlatformOpenCodeGo,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-opencode",
			"account_mode": AccountModeZen,
			"api_protocol": APIProtocolAdaptive,
			"base_url":     "https://opencode.ai/zen/v1",
			"api_base_urls": map[string]any{
				APIProtocolChatCompletions: "https://opencode.ai/zen/v1",
				APIProtocolAnthropic:       "https://opencode.ai/zen",
				APIProtocolResponses:       "https://opencode.ai/zen/v1",
			},
			"model_mapping": map[string]any{
				"opencode/muse-spark-1.3-contributior-free": "muse-spark-1.3-contributior-free",
				"opencode/claude-sonnet-4":                  "claude-sonnet-4",
				"opencode/glm-5.3":                          "glm-5.3",
			},
		},
	}
}

func TestOpenCodeGatewayAppliesMappedModelOnAllIngresses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		path         string
		body         string
		wantURL      string
		wantModel    string
		wantNotModel string
		forward      func(*OpenAIGatewayService, *gin.Context, *Account, []byte) error
		upstream     *http.Response
	}{
		{
			name:         "responses muse-spark",
			path:         "/v1/responses",
			body:         `{"model":"opencode/muse-spark-1.3-contributior-free","stream":false,"input":"hi"}`,
			wantURL:      "https://opencode.ai/zen/v1/responses",
			wantModel:    "muse-spark-1.3-contributior-free",
			wantNotModel: "opencode/muse-spark-1.3-contributior-free",
			upstream: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid-oc-responses"}},
				Body: io.NopCloser(strings.NewReader(
					`{"id":"resp_oc","object":"response","model":"muse-spark-1.3-contributior-free","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`,
				)),
			},
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) error {
				SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
				_, err := svc.Forward(context.Background(), c, account, body)
				return err
			},
		},
		{
			name:         "chat completions glm",
			path:         "/v1/chat/completions",
			body:         `{"model":"opencode/glm-5.3","stream":false,"messages":[{"role":"user","content":"hi"}]}`,
			wantURL:      "https://opencode.ai/zen/v1/chat/completions",
			wantModel:    "glm-5.3",
			wantNotModel: "opencode/glm-5.3",
			upstream: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid-oc-cc"}},
				Body: io.NopCloser(strings.NewReader(
					`{"id":"chatcmpl_oc","object":"chat.completion","model":"glm-5.3","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
				)),
			},
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) error {
				_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
				return err
			},
		},
		{
			name:         "messages claude",
			path:         "/v1/messages",
			body:         `{"model":"opencode/claude-sonnet-4","max_tokens":32,"stream":false,"messages":[{"role":"user","content":"hi"}]}`,
			wantURL:      "https://opencode.ai/zen/v1/messages",
			wantModel:    "claude-sonnet-4",
			wantNotModel: "opencode/claude-sonnet-4",
			upstream:     nativeAnthropicBufferedResponse(),
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) error {
				_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: tt.upstream}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			c := adaptiveProtocolTestContext(tt.path, []byte(tt.body))

			err := tt.forward(svc, c, openCodeMappedTestAccount(), []byte(tt.body))
			require.NoError(t, err)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, tt.wantURL, upstream.lastReq.URL.String())
			require.Equal(t, tt.wantModel, gjson.GetBytes(upstream.lastBody, "model").String())
			require.NotContains(t, string(upstream.lastBody), tt.wantNotModel)
		})
	}
}
