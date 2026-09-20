// 本文件验证 Connect 错误码到 OpenAI / Anthropic error type 的映射。
package common

import "testing"

func TestOpenAIErrorType(t *testing.T) {
	cases := []struct {
		message string
		want    string
	}{
		{"invalid_argument: model does not support image", "invalid_request_error"},
		{"resource_exhausted: rate limit exceeded", "rate_limit_error"},
		{"permission_denied: not allowed", "invalid_request_error"},
		{"not_found: model missing", "not_found_error"},
		{"unavailable: upstream offline", "server_error"},
		{"some unknown error", "server_error"},
	}
	for _, c := range cases {
		if got := OpenAIErrorType(c.message); got != c.want {
			t.Fatalf("OpenAIErrorType(%q) = %q, want %q", c.message, got, c.want)
		}
	}
}

func TestAnthropicErrorType(t *testing.T) {
	cases := []struct {
		message string
		want    string
	}{
		{"invalid_argument: bad request", "invalid_request_error"},
		{"resource_exhausted: rate limit exceeded", "rate_limit_error"},
		{"permission_denied: not allowed", "invalid_request_error"},
		{"not_found: model missing", "not_found_error"},
		{"unavailable: upstream offline", "api_error"},
		{"some unknown error", "api_error"},
	}
	for _, c := range cases {
		if got := AnthropicErrorType(c.message); got != c.want {
			t.Fatalf("AnthropicErrorType(%q) = %q, want %q", c.message, got, c.want)
		}
	}
}

// TestHTTPStatus 的测试动机是 HTTP 状态码决定下游网关的冷却分类：
// 请求级错误必须落到 4xx，上下文超长落到 413（客户端级、不冷却）。
func TestHTTPStatus(t *testing.T) {
	cases := []struct {
		message string
		want    int
	}{
		{"permission_denied: blocked by content policy", 400},
		{"invalid_argument: The prompt is too long for this model", 413},
		{"invalid_argument: internal error", 400},
		{"unauthenticated: bad token", 401},
		{"not_found: model missing", 404},
		{"resource_exhausted: quota", 429},
		{"unavailable: TLS handshake timeout", 502},
		{"stream disconnected", 502},
	}
	for _, c := range cases {
		if got := HTTPStatus(c.message); got != c.want {
			t.Fatalf("HTTPStatus(%q) = %d, want %d", c.message, got, c.want)
		}
	}
}

// TestErrorCode 的测试动机是 error.code 让下游网关把上下文超长识别为
// 请求级问题；其他错误返回 nil。
func TestErrorCode(t *testing.T) {
	if got := ErrorCode("invalid_argument: The prompt is too long for this model"); got != "context_length_exceeded" {
		t.Fatalf("ErrorCode = %v, want context_length_exceeded", got)
	}
	if got := ErrorCode("permission_denied: blocked"); got != nil {
		t.Fatalf("ErrorCode = %v, want nil", got)
	}
}

// TestRetryAfterSeconds 验证从上游限流文案解析重置窗口——上游没有
// Retry-After/RetryInfo，"reset in N seconds" 是唯一可行动 hint。
func TestRetryAfterSeconds(t *testing.T) {
	if seconds, ok := RetryAfterSeconds("resource_exhausted: rate limited. Your limit will reset in 42 seconds."); !ok || seconds != 42 {
		t.Fatalf("RetryAfterSeconds = %d,%v, want 42,true", seconds, ok)
	}
	if _, ok := RetryAfterSeconds("resource_exhausted: quota exceeded"); ok {
		t.Fatal("no reset hint must report false")
	}
	if _, ok := RetryAfterSeconds("reset in 0 seconds"); ok {
		t.Fatal("zero reset must report false")
	}
}

// TestUpstreamTraceID 验证从错误文案尾部提取 "(trace ID: …)"——上游错误
// 全是模糊 internal error，trace ID 是唯一的报障锚点。
func TestUpstreamTraceID(t *testing.T) {
	if got := UpstreamTraceID("internal: an internal error occurred (trace ID: abc-def)"); got != "abc-def" {
		t.Fatalf("UpstreamTraceID = %q, want abc-def", got)
	}
	if got := UpstreamTraceID("internal: boom"); got != "" {
		t.Fatalf("UpstreamTraceID = %q, want empty", got)
	}
}
