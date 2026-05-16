//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchSignaturePatterns_EmptyPatterns(t *testing.T) {
	body := []byte(`{"error":{"message":"signature error"}}`)
	assert.False(t, matchSignaturePatterns(body, nil))
	assert.False(t, matchSignaturePatterns(body, []string{}))
}

func TestMatchSignaturePatterns_MatchesCaseInsensitive(t *testing.T) {
	body := []byte(`{"error":{"message":"Invalid Signature detected in response"}}`)
	assert.True(t, matchSignaturePatterns(body, []string{"invalid signature"}))
	assert.True(t, matchSignaturePatterns(body, []string{"INVALID SIGNATURE"}))
}

func TestMatchSignaturePatterns_NoMatch(t *testing.T) {
	body := []byte(`{"error":{"message":"rate limit exceeded"}}`)
	assert.False(t, matchSignaturePatterns(body, []string{"signature", "thinking block"}))
}

func TestMatchSignaturePatterns_MultiplePatterns(t *testing.T) {
	body := []byte(`{"error":{"message":"thinking block order violation"}}`)
	assert.True(t, matchSignaturePatterns(body, []string{"signature", "thinking block order"}))
}

func TestMatchSignaturePatterns_EmptyBody(t *testing.T) {
	assert.False(t, matchSignaturePatterns(nil, []string{"error"}))
	assert.False(t, matchSignaturePatterns([]byte{}, []string{"error"}))
}

func TestIsThinkingBlockSignatureError_SignatureError(t *testing.T) {
	svc := &GatewayService{}
	body := []byte(`{"error":{"message":"could not validate the signature on one of the thinking blocks"}}`)
	assert.True(t, svc.isThinkingBlockSignatureError(body))
}

func TestIsThinkingBlockSignatureError_ThinkingBlockTypeError(t *testing.T) {
	svc := &GatewayService{}
	// "Expected `thinking` or `redacted_thinking`, but found `text`"
	body := []byte(`{"error":{"message":"Expected thinking or redacted_thinking, but found text"}}`)
	assert.True(t, svc.isThinkingBlockSignatureError(body))
}

func TestIsThinkingBlockSignatureError_NoMatch(t *testing.T) {
	svc := &GatewayService{}
	body := []byte(`{"error":{"message":"rate limit exceeded"}}`)
	assert.False(t, svc.isThinkingBlockSignatureError(body))
}

func TestIsThinkingBlockSignatureError_EmptyBody(t *testing.T) {
	svc := &GatewayService{}
	assert.False(t, svc.isThinkingBlockSignatureError(nil))
	assert.False(t, svc.isThinkingBlockSignatureError([]byte{}))
}

func TestShouldFailoverOn400_BetaRequired(t *testing.T) {
	svc := &GatewayService{}
	body := []byte(`{"error":{"message":"header 'anthropic-beta' must include 'interleaved-thinking-2025-05-14'"}}`)
	assert.True(t, svc.shouldFailoverOn400(body))
}

func TestShouldFailoverOn400_ThinkingToolIncompatible(t *testing.T) {
	svc := &GatewayService{}
	body := []byte(`{"error":{"message":"thinking is not supported with tool_choice"}}`)
	assert.True(t, svc.shouldFailoverOn400(body))
}

func TestShouldFailoverOn400_NormalError(t *testing.T) {
	svc := &GatewayService{}
	body := []byte(`{"error":{"message":"invalid model specified"}}`)
	assert.False(t, svc.shouldFailoverOn400(body))
}

func TestExtractUpstreamErrorMessage_ClaudeFormat(t *testing.T) {
	body := []byte(`{"error":{"type":"invalid_request_error","message":"model not found"}}`)
	assert.Equal(t, "model not found", ExtractUpstreamErrorMessage(body))
}

func TestExtractUpstreamErrorMessage_OpenAIFormat(t *testing.T) {
	body := []byte(`{"error":{"message":"Rate limit reached","type":"rate_limit_error"}}`)
	assert.Equal(t, "Rate limit reached", ExtractUpstreamErrorMessage(body))
}

func TestExtractUpstreamErrorMessage_EmptyBody(t *testing.T) {
	assert.Equal(t, "", ExtractUpstreamErrorMessage(nil))
	assert.Equal(t, "", ExtractUpstreamErrorMessage([]byte{}))
}

func TestExtractUpstreamErrorMessage_MalformedJSON(t *testing.T) {
	assert.Equal(t, "", ExtractUpstreamErrorMessage([]byte(`not json`)))
}

func TestExtractUpstreamErrorCode_FromCodeField(t *testing.T) {
	body := []byte(`{"error":{"code":"overloaded_error","message":"overloaded"}}`)
	require.Equal(t, "overloaded_error", extractUpstreamErrorCode(body))
}

func TestExtractUpstreamErrorCode_NoCodeField(t *testing.T) {
	body := []byte(`{"error":{"type":"overloaded_error","message":"overloaded"}}`)
	assert.Equal(t, "", extractUpstreamErrorCode(body))
}

func TestExtractUpstreamErrorCode_NestedJSON(t *testing.T) {
	body := []byte(`{"error":{"message":"{\"error\":{\"type\":\"inner_error\"}}"}}`)
	code := extractUpstreamErrorCode(body)
	assert.Contains(t, []string{"inner_error", ""}, code)
}

func TestExtractUpstreamErrorCode_NoError(t *testing.T) {
	body := []byte(`{"result":"ok"}`)
	assert.Equal(t, "", extractUpstreamErrorCode(body))
}

func TestIsCountTokensUnsupported404_Extended(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"404 with count_tokens not found", 404, `{"error":{"message":"count_tokens endpoint not found"}}`, true},
		{"404 without matching type", 404, `{"error":{"type":"invalid_request"}}`, false},
		{"200 with not_found_error", 200, `{"error":{"type":"not_found_error"}}`, false},
		{"404 empty body", 404, `{}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isCountTokensUnsupported404(tt.status, []byte(tt.body)))
		})
	}
}
