package apicompat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicUsageFromResponsesUsage_CacheCreation(t *testing.T) {
	usage := &ResponsesUsage{
		InputTokens:              20,
		OutputTokens:             5,
		CacheCreationInputTokens: 6,
		InputTokensDetails: &ResponsesInputTokensDetails{
			CachedTokens: 4,
	REDACTED,
REDACTED

	got := anthropicUsageFromResponsesUsage(usage)

	assert.Equal(t, 10, got.InputTokens, "input = total(20) - cache_read(4) - cache_creation(6)")
	assert.Equal(t, 5, got.OutputTokens)
	assert.Equal(t, 4, got.CacheReadInputTokens)
	assert.Equal(t, 6, got.CacheCreationInputTokens, "cache creation must be preserved")
REDACTED

func TestAnthropicUsageFromResponsesUsage_NoCacheCreation(t *testing.T) {
	usage := &ResponsesUsage{
		InputTokens:  10,
		OutputTokens: 5,
		InputTokensDetails: &ResponsesInputTokensDetails{
			CachedTokens: 3,
	REDACTED,
REDACTED

	got := anthropicUsageFromResponsesUsage(usage)

	assert.Equal(t, 7, got.InputTokens)
	assert.Equal(t, 3, got.CacheReadInputTokens)
	assert.Equal(t, 0, got.CacheCreationInputTokens)
REDACTED

func TestAnthropicToResponsesResponse_CacheCreation(t *testing.T) {
	resp := AnthropicResponse{
		ID:    "msg_test",
		Type:  "message",
		Role:  "assistant",
		Model: "claude-opus-4-6",
		Usage: AnthropicUsage{
			InputTokens:              10,
			OutputTokens:             5,
			CacheReadInputTokens:     4,
			CacheCreationInputTokens: 6,
	REDACTED,
		StopReason: "end_turn",
REDACTED

	out := AnthropicToResponsesResponse(&resp)

	require.NotNil(t, out.Usage)
	assert.Equal(t, 20, out.Usage.InputTokens, "total = input(10) + cache_read(4) + cache_creation(6)")
	assert.Equal(t, 6, out.Usage.CacheCreationInputTokens, "cache creation must round-trip")
REDACTED
