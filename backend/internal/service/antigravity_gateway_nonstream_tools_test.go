package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHandleGeminiStreamToNonStreamingPreservesAllParts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name      string
		chunks    []string
		wantParts string
	}{
		{
			name: "tool call followed by empty terminal text",
			chunks: []string{
				`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call_1","name":"bash","args":{"command":"printf OK"}},"thoughtSignature":"tool-signature"}]}}]}`,
				`{"candidates":[{"content":{"role":"model","parts":[{"text":""}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":7}}`,
			},
			wantParts: `[{"functionCall":{"id":"call_1","name":"bash","args":{"command":"printf OK"}},"thoughtSignature":"tool-signature"}]`,
		},
		{
			name: "ordered text thinking tools signatures and image",
			chunks: []string{
				`{"candidates":[{"content":{"parts":[{"text":"Private reasoning","thought":true,"thoughtSignature":"thinking-signature"},{"text":"I will "}]}}]}`,
				`{"candidates":[{"content":{"parts":[{"text":"inspect."},{"functionCall":{"name":"bash","args":{"command":"pwd"}},"thoughtSignature":"first-tool"}]}}]}`,
				`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"bash","args":{"command":"ls"}},"thoughtSignature":"second-tool"},{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}},{"text":"Signed text","thoughtSignature":"text-signature"}]}}]}`,
				`{"candidates":[{"content":{"parts":[{"text":"After "},{"text":"tools."}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":7}}`,
			},
			wantParts: `[{"text":"Private reasoning","thought":true,"thoughtSignature":"thinking-signature"},{"text":"I will inspect."},{"functionCall":{"name":"bash","args":{"command":"pwd"}},"thoughtSignature":"first-tool"},{"functionCall":{"name":"bash","args":{"command":"ls"}},"thoughtSignature":"second-tool"},{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}},{"text":"Signed text","thoughtSignature":"text-signature"},{"text":"After tools."}]`,
		},
		{
			name: "tool call with empty text in the same part and usage only tail",
			chunks: []string{
				`{"candidates":[{"content":{"parts":[{"text":"","functionCall":{"name":"bash","args":{}},"thoughtSignature":"signature"}]},"finishReason":"STOP"}]}`,
				`{"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":7}}`,
			},
			wantParts: `[{"text":"","functionCall":{"name":"bash","args":{}},"thoughtSignature":"signature"}]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newAntigravityTestService(&config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}})
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			var stream strings.Builder
			for _, chunk := range tc.chunks {
				stream.WriteString("data: {\"response\":" + chunk + "}\n\n")
			}
			stream.WriteString("data: [DONE]\n\n")
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream.String()))}
			result, err := svc.handleGeminiStreamToNonStreaming(ctx, resp, time.Now())
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, recorder.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			parts, err := json.Marshal(extractGeminiParts(body))
			require.NoError(t, err)
			require.JSONEq(t, tc.wantParts, string(parts))
			require.Equal(t, "STOP", extractGeminiFinishReason(body))
			require.Equal(t, 10, result.usage.InputTokens)
			require.Equal(t, 7, result.usage.OutputTokens)
			require.NotNil(t, result.firstTokenMs)
		})
	}
}
