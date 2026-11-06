//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

func TestAntigravityThinkingBudgetForModel(t *testing.T) {
	tests := []struct {
		model  string
		budget int
		ok     bool
	}{
		{"gemini-3.8-flash-high", 10000, true},
		{"gemini-3.8-flash-xhigh", 10000, true},
		{"gemini-3.1-pro-high", 10001, true},
		{"gemini-3.8-flash-medium", 4000, true},
		{"gemini-3.8-flash-low", 1000, true},
		{"gemini-3.8-flash-extra-low", 1000, true},
		{"gemini-4-argon-low", 1024, true},
		{"gemini-3.8-flash", 4000, true},
		{"gemini-3.1-pro", 10001, true},
		{"gemini-2.5-flash", 0, false},
		{"claude-opus-5-5-20251001", 0, false},
		{"gemini-3.8-flash-image", 0, false},
	}
	for _, tt := range tests {
		budget, ok := antigravityThinkingBudgetForModel(tt.model)
		require.Equal(t, tt.ok, ok, tt.model)
		require.Equal(t, tt.budget, budget, tt.model)
	}
}

func TestApplyAntigravityGeminiThinkingConfigAddsBudgetAndHeadroom(t *testing.T) {
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":1024}}`)
	out := applyAntigravityGeminiThinkingConfig(body, []byte(`{}`), "gemini-3.8-flash-high")

	require.True(t, gjson.GetBytes(out, "generationConfig.thinkingConfig.includeThoughts").Bool())
	require.Equal(t, int64(10000), gjson.GetBytes(out, "generationConfig.thinkingConfig.thinkingBudget").Int())
	require.Equal(t, int64(18192), gjson.GetBytes(out, "generationConfig.maxOutputTokens").Int(), "maxOutputTokens 必须给思考留出 8192 余量")
}

func TestApplyAntigravityGeminiThinkingConfigKeepsLargerOutputBudget(t *testing.T) {
	body := []byte(`{"generationConfig":{"maxOutputTokens":32000}}`)
	out := applyAntigravityGeminiThinkingConfig(body, []byte(`{}`), "gemini-3.8-flash-high")
	require.Equal(t, int64(32000), gjson.GetBytes(out, "generationConfig.maxOutputTokens").Int())
}

func TestApplyAntigravityGeminiThinkingConfigPreservesTinyExplicitLimit(t *testing.T) {
	body := []byte(`{"generationConfig":{"maxOutputTokens":8}}`)
	out := applyAntigravityGeminiThinkingConfig(body, []byte(`{}`), "gemini-3.8-flash-high")
	require.Equal(t, int64(8), gjson.GetBytes(out, "generationConfig.maxOutputTokens").Int())
}

func TestApplyAntigravityGeminiThinkingConfigRespectsDisabledThinking(t *testing.T) {
	body := []byte(`{"generationConfig":{"maxOutputTokens":1024}}`)
	out := applyAntigravityGeminiThinkingConfig(body, []byte(`{"thinking":{"type":"disabled"}}`), "gemini-3.8-flash-high")
	require.Equal(t, int64(1024), gjson.GetBytes(out, "generationConfig.maxOutputTokens").Int())
	require.False(t, gjson.GetBytes(out, "generationConfig.thinkingConfig").Exists())
}

func TestApplyAntigravityGeminiThinkingConfigSkipsNonGemini34(t *testing.T) {
	body := []byte(`{"generationConfig":{"maxOutputTokens":1024}}`)
	out := applyAntigravityGeminiThinkingConfig(body, []byte(`{}`), "claude-opus-5-5-20251001")
	require.Equal(t, string(body), string(out))
}

func TestConvertClaudeToolUseReplaysCachedThoughtSignature(t *testing.T) {
	const toolID = "call-replay-1"
	antigravity.CacheToolThoughtSignature(toolID, "sig-replayed")

	body := []byte(`{"max_tokens":1024,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"` + toolID + `","name":"bash","input":{"cmd":"ls"}}]}]}`)
	out, err := convertClaudeMessagesToGeminiGenerateContent(body)
	require.NoError(t, err)
	require.Equal(t, "sig-replayed", gjson.GetBytes(out, "contents.0.parts.0.thoughtSignature").String())
}

func TestConvertClaudeToolUseFallsBackToDummySignature(t *testing.T) {
	body := []byte(`{"max_tokens":1024,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call-no-cache-1","name":"bash","input":{"cmd":"ls"}}]}]}`)
	out, err := convertClaudeMessagesToGeminiGenerateContent(body)
	require.NoError(t, err)
	require.Equal(t, geminiDummyThoughtSignature, gjson.GetBytes(out, "contents.0.parts.0.thoughtSignature").String())
}
