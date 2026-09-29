//go:build unit

package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func a112RunMockInterceptStream(t *testing.T, interceptType InterceptType) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	sendMockInterceptStream(c, "claude-haiku-4-5", interceptType)
	return rec.Body.String()
}

// 流式 max_tokens=1 haiku 探测应与非流式一致：返回 "#"、1 个输出 token、stop_reason=max_tokens。
func TestA112SendMockInterceptStreamMaxTokensOneHaiku(t *testing.T) {
	body := a112RunMockInterceptStream(t, InterceptTypeMaxTokensOneHaiku)

	require.Contains(t, body, `"text":"#"`)
	require.Contains(t, body, `"stop_reason":"max_tokens"`)
	require.Contains(t, body, `"usage":{"output_tokens":1}`)
	require.NotContains(t, body, "Conversation")
	require.NotContains(t, body, `"stop_reason":"end_turn"`)
}

// Warmup 流式 mock 保持不变。
func TestA112SendMockInterceptStreamWarmupUnchanged(t *testing.T) {
	body := a112RunMockInterceptStream(t, InterceptTypeWarmup)

	require.Contains(t, body, `"text":"New"`)
	require.Contains(t, body, `"text":" Conversation"`)
	require.Contains(t, body, `"stop_reason":"end_turn"`)
	require.Contains(t, body, `"usage":{"output_tokens":2}`)
}
