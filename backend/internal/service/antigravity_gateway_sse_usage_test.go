package service

import (
	"net/http"
	"net/http/httptest"

	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExtractSSEUsage(t *testing.T) {
	svc := &AntigravityGatewayService{}
	tests := []struct {
		name     string
		line     string
		expected ClaudeUsage
	}{
		{
			name:     "message_delta with output_tokens",
			line:     `data: {"type":"message_delta","usage":{"output_tokens":42}}`,
			expected: ClaudeUsage{OutputTokens: 42},
		},
		{
			name:     "non-data line ignored",
			line:     `event: message_start`,
			expected: ClaudeUsage{},
		},
		{
			name:     "top-level usage with all fields",
			line:     `data: {"usage":{"input_tokens":10,"output_tokens":20,"cache_read_input_tokens":5,"cache_creation_input_tokens":3}}`,
			expected: ClaudeUsage{InputTokens: 10, OutputTokens: 20, CacheReadInputTokens: 5, CacheCreationInputTokens: 3},
		},
		{
			// Anthropic message_start 把 usage 嵌套在 message.usage 下，
			// 必须从这里提取输入侧字段（含 cache_read/cache_creation_input_tokens）。
			name:     "message_start nested usage with input/cache tokens",
			line:     `data: {"type":"message_start","message":{"id":"msg_01","usage":{"input_tokens":35576,"cache_creation_input_tokens":0,"cache_read_input_tokens":12000,"output_tokens":1}}}`,
			expected: ClaudeUsage{InputTokens: 35576, OutputTokens: 1, CacheReadInputTokens: 12000},
		},
		{
			// message_start.message.usage.cache_creation 内的 5m/1h 明细也要解析。
			name:     "message_start nested usage with cache_creation breakdown",
			line:     `data: {"type":"message_start","message":{"usage":{"input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":30,"ephemeral_1h_input_tokens":70}}}}`,
			expected: ClaudeUsage{InputTokens: 100, CacheCreation5mTokens: 30, CacheCreation1hTokens: 70},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := &ClaudeUsage{}
			svc.extractSSEUsage(tt.line, usage)
			require.Equal(t, tt.expected, *usage)
		})
	}
}

// TestExtractSSEUsage_StreamingSequence 复现 issue #2332：完整的 Anthropic streaming
// 序列（message_start → message_delta）必须把两类事件中的 usage 字段都汇入同一份累计值，
// 否则透传账号产出的 usage_logs 会出现 input_tokens=0、仅有 output_tokens 的"残缺"记录。
func TestExtractSSEUsage_StreamingSequence(t *testing.T) {
	svc := &AntigravityGatewayService{}
	usage := &ClaudeUsage{}

	// 1) message_start：携带完整输入侧 usage（input_tokens + cache_read）
	svc.extractSSEUsage(
		`data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":35576,"cache_creation_input_tokens":0,"cache_read_input_tokens":12000,"output_tokens":1}}}`,
		usage,
	)
	// 2) message_delta：流结束时只带 output_tokens（无 input_tokens 字段）
	svc.extractSSEUsage(
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":816}}`,
		usage,
	)

	require.Equal(t, 35576, usage.InputTokens, "message_start 的 input_tokens 必须被记录，否则记账会缺失输入侧 token (#2332)")
	require.Equal(t, 12000, usage.CacheReadInputTokens, "message_start 的 cache_read_input_tokens 必须被记录")
	require.Equal(t, 816, usage.OutputTokens, "message_delta 的最终 output_tokens 必须被记录")
}

// TestAntigravityClientWriter 验证 antigravityClientWriter 的断开检测
func TestAntigravityClientWriter(t *testing.T) {
	t.Run("normal write succeeds", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		flusher, _ := c.Writer.(http.Flusher)
		cw := newAntigravityClientWriter(c.Writer, flusher, "test")

		ok := cw.Write([]byte("hello"))
		require.True(t, ok)
		require.False(t, cw.Disconnected())
		require.Contains(t, rec.Body.String(), "hello")
	})

	t.Run("write failure marks disconnected", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		fw := &antigravityFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
		flusher, _ := c.Writer.(http.Flusher)
		cw := newAntigravityClientWriter(fw, flusher, "test")

		ok := cw.Write([]byte("hello"))
		require.False(t, ok)
		require.True(t, cw.Disconnected())
	})

	t.Run("subsequent writes are no-op", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		fw := &antigravityFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
		flusher, _ := c.Writer.(http.Flusher)
		cw := newAntigravityClientWriter(fw, flusher, "test")

		cw.Write([]byte("first"))
		ok := cw.Fprintf("second %d", 2)
		require.False(t, ok)
		require.True(t, cw.Disconnected())
	})
}

// TestUnwrapV1InternalResponse 测试 unwrapV1InternalResponse 的各种输入场景
