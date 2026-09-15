//go:build unit

package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCopyOpenAIUsageFromResponsesUsageTrustsCanonicalCacheCreationValue(t *testing.T) {
	usage := &apicompat.ResponsesUsage{
		InputTokens:              20,
		OutputTokens:             2,
		CacheCreationInputTokens: 0,
		InputTokensDetails: &apicompat.ResponsesInputTokensDetails{
			CachedTokens:     3,
			CacheWriteTokens: 19,
		},
	}

	got := copyOpenAIUsageFromResponsesUsage(usage)

	require.Equal(t, 20, got.InputTokens)
	require.Equal(t, 3, got.CacheReadInputTokens)
	require.Zero(t, got.CacheCreationInputTokens)
}

func TestHandleAnthropicStreamingResponseMergesKiroCredits(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_messages_stream_kiro_credits"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"resp_kiro","model":"gpt-5.6-terra","status":"in_progress","output":[]}}`,
			``,
			`data: {"type":"response.output_text.delta","delta":"ok"}`,
			``,
			`data: {"type":"response.completed","response":{"id":"resp_kiro","object":"response","model":"gpt-5.6-terra","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":31,"output_tokens":7,"total_tokens":38,"input_tokens_details":{"cached_tokens":5},"_sub2api_kiro_credits":0.41}}}`,
			``,
		}, "\n"))),
	}

	svc := &OpenAIGatewayService{}
	result, err := svc.handleAnthropicStreamingResponse(resp, c, &Account{ID: 1, Platform: PlatformKiro}, "gpt-5.6-terra", "gpt-5.6-terra", "gpt-5.6-terra", time.Now())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 31, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 5, result.Usage.CacheReadInputTokens)
	require.InDelta(t, 0.41, result.Usage.KiroCredits, 0.000001)
	require.Contains(t, rec.Body.String(), `message_stop`)
	require.NotContains(t, rec.Body.String(), `_sub2api_kiro_credits`)
}
