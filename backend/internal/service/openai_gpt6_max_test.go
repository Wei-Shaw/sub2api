package service

import (
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

func TestNormalizeOpenAIReasoningEffortForGPT6Astra(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		raw   string
		model string
		want  string
	}{
		{name: "max", raw: "max", model: "gpt-6-astra", want: "max"},
		{name: "provider and spelling", raw: " MAX ", model: " openai/GPT_6_ASTRA ", want: "max"},
		{name: "version suffix", raw: "max", model: "gpt-6-astra-2026-09-03", want: "max"},
		{name: "effort suffix", raw: "max", model: "gpt-6-astra-max", want: "max"},
		{name: "xhigh stays distinct", raw: "x-high", model: "gpt-6-astra", want: "xhigh"},
		{name: "similar name", raw: "max", model: "gpt-6-astral", want: "xhigh"},
		{name: "other GPT-6 model", raw: "max", model: "gpt-6-other", want: "xhigh"},
		{name: "legacy GPT model", raw: "max", model: "gpt-5.5", want: "xhigh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeOpenAIReasoningEffortForModel(tt.raw, tt.model))
		})
	}
}

func TestOpenAIGatewayServicePassthroughPreservesGPT6AstraMaxEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, model := range []string{"gpt-6-astra", "astra"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", model, stream), func(t *testing.T) {
				responseBody := `{"model":"gpt-6-astra","usage":{"input_tokens":1,"output_tokens":2}}`
				contentType := "application/json"
				if stream {
					responseBody = "data: {\"type\":\"response.completed\",\"response\":" + responseBody + "}\n\ndata: [DONE]\n\n"
					contentType = "text/event-stream"
				}
				upstream := &httpUpstreamRecorder{
					resp: &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{contentType}},
						Body:       io.NopCloser(strings.NewReader(responseBody)),
					},
				}
				cfg := &config.Config{}
				cfg.Security.URLAllowlist.Enabled = false
				svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
				account := &Account{
					ID:          1,
					Name:        "openai-apikey-passthrough",
					Platform:    PlatformOpenAI,
					Type:        AccountTypeAPIKey,
					Concurrency: 1,
					Credentials: map[string]any{
						"api_key":  "sk-test",
						"base_url": "https://example.com",
						"model_mapping": map[string]any{
							"astra": "gpt-6-astra",
						},
					},
					Extra: map[string]any{"openai_passthrough": true},
				}
				body := []byte(fmt.Sprintf(`{"model":%q,"stream":%t,"reasoning":{"effort":"max"},"input":"hello"}`, model, stream))
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
				SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

				result, err := svc.Forward(context.Background(), c, account, body)

				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, http.StatusOK, rec.Code)
				require.Equal(t, "gpt-6-astra", gjson.GetBytes(upstream.lastBody, "model").String())
				require.Equal(t, "max", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
				require.NotNil(t, result.ReasoningEffort)
				require.Equal(t, "max", *result.ReasoningEffort)
			})
		}
	}
}
