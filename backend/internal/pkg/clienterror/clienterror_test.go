//go:build unit

package clienterror

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessage_TranslatesKnownChineseToEnglish(t *testing.T) {
	require.Equal(t, "Invalid request format or parameters", Message(http.StatusBadRequest, "请求格式或参数不正确，请检查 messages 参数"))
	require.Equal(t, "Upstream request failed", Message(http.StatusBadGateway, "OpenAI上游失败"))
}

func TestMessage_UnknownChineseUsesLocalFallback(t *testing.T) {
	require.Equal(t, "Invalid request", Message(http.StatusBadRequest, "未知错误"))
	require.Equal(t, "Internal server error", Message(http.StatusInternalServerError, "服务器爆炸了"))
}

func TestUpstreamMessage_UnknownChineseUsesUpstreamFallback(t *testing.T) {
	require.Equal(t, "Invalid upstream request", UpstreamMessage(http.StatusBadRequest, "未知错误"))
	require.Equal(t, "Upstream service temporarily unavailable", UpstreamMessage(http.StatusInternalServerError, "服务器爆炸了"))
}

func TestMessageWithCode_PrefersKnownMachineCode(t *testing.T) {
	require.Equal(t, "API key quota exhausted", MessageWithCode(http.StatusTooManyRequests, "API_KEY_QUOTA_EXHAUSTED", "API key 额度已用完", ""))
	require.Equal(t, "API key has expired", MessageWithCode(http.StatusForbidden, "api-key-expired", "API key 已过期", ""))
	require.Equal(t, "API key group is no longer available for this user", MessageWithCode(http.StatusForbidden, "GROUP_NOT_ALLOWED", "未知错误", ""))
}

func TestUpstreamMessageWithCode_PrefersUpstreamKnownMachineCode(t *testing.T) {
	require.Equal(t, "Upstream quota exhausted", UpstreamMessageWithCode(http.StatusTooManyRequests, "insufficient-quota", "额度不足"))
	require.Equal(t, "Context length exceeded", UpstreamMessageWithCode(http.StatusBadRequest, "context_length_exceeded", "上下文长度超限"))
}

func TestUpstreamMessageWithCode_KnownCodeOverridesEnglishMessage(t *testing.T) {
	require.Equal(t, "Upstream quota exhausted", UpstreamMessageWithCode(http.StatusTooManyRequests, "insufficient_quota", "too many requests"))
	require.Equal(t, "Upstream service temporarily unavailable", UpstreamMessageWithCode(http.StatusServiceUnavailable, "unavailable", "service unavailable"))
}

func TestUpstreamMessageWithCode_UnknownCodeUsesUpstreamFallbackForLocalizedMessage(t *testing.T) {
	require.Equal(t, "Invalid upstream request", UpstreamMessageWithCode(http.StatusBadRequest, "unknown_code", "未知错误"))
	require.Equal(t, "upstream says no", UpstreamMessageWithCode(http.StatusBadRequest, "unknown_code", "upstream says no"))
}

func TestJSONBody_RewritesKnownChineseErrorMessage(t *testing.T) {
	body := []byte(`{"error":{"type":"invalid_request_error","message":"请求格式或参数不正确，请检查 messages 参数"}}`)
	got := JSONBody(http.StatusBadRequest, body, "invalid_request_error", "Upstream request failed")

	var payload map[string]map[string]string
	require.NoError(t, json.Unmarshal(got, &payload))
	require.Equal(t, "invalid_request_error", payload["error"]["type"])
	require.Equal(t, "Invalid request format or parameters", payload["error"]["message"])
	require.NotContains(t, string(got), "请求格式")
}

func TestJSONBody_UnknownChineseJSONUsesEnglishEnvelope(t *testing.T) {
	body := []byte(`{"error":{"message":"未知错误"}}`)
	got := JSONBody(http.StatusInternalServerError, body, "upstream_error", "OpenAI上游失败")

	var payload map[string]map[string]string
	require.NoError(t, json.Unmarshal(got, &payload))
	require.Equal(t, "upstream_error", payload["error"]["type"])
	require.Equal(t, "Upstream request failed", payload["error"]["message"])
	require.NotContains(t, string(got), "未知错误")
}

func TestJSONBody_NonJSONChineseUsesEnglishEnvelope(t *testing.T) {
	got := JSONBody(http.StatusBadGateway, []byte("服务不可用"), "upstream_error", "")

	var payload map[string]map[string]string
	require.NoError(t, json.Unmarshal(got, &payload))
	require.Equal(t, "upstream_error", payload["error"]["type"])
	require.Equal(t, "Upstream request failed", payload["error"]["message"])
}

func TestJSONBody_PreservesEnglishJSON(t *testing.T) {
	body := []byte(`{"error":{"type":"invalid_request_error","message":"model is required"}}`)
	got := JSONBody(http.StatusBadRequest, body, "invalid_request_error", "Upstream request failed")

	require.JSONEq(t, string(body), string(got))
}
