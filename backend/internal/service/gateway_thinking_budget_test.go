//go:build unit

package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const basetenBudgetError = "Error from provider (Console Go): Upstream request failed: [invalid_request_error] must be greater than 1024 to reserve tokens for a final answer when Baseten reasoning is enabled"

func TestThinkingBudgetConstraintErrors(t *testing.T) {
	for _, tc := range []struct {
		message string
		want    bool
	}{
		{basetenBudgetError, true},
		{"MUST BE GREATER THAN 1024 TO RESERVE TOKENS FOR A FINAL ANSWER WHEN BASETEN REASONING IS ENABLED", true},
		{"thinking.budget_tokens must be >= 1024", true},
		{"thinking budget tokens must be greater than or equal to 1024", true},
		{"thinking.budget_tokens: Input should be greater than 1024", true},
		{"reasoning quota exceeded: 1024 tokens", false},
		{"must be greater than 1024", false},
		{"reserve tokens for a final answer when Baseten reasoning is enabled", false},
		{"must be greater than 2048 to reserve tokens for a final answer when Baseten reasoning is enabled", false},
		{"", false},
	} {
		t.Run(tc.message, func(t *testing.T) {
			require.Equal(t, tc.want, isThinkingBudgetConstraintError(tc.message))
		})
	}
}

func TestRectifyThinkingBudgetFinalAnswerReserve(t *testing.T) {
	for _, tc := range []struct {
		maxTokens int
		want      int64
		changed   bool
	}{
		{1024, 64000, true},
		{32000, 64000, true},
		{32001, 32001, false},
		{40000, 40000, false},
		{64000, 64000, false},
		{100000, 100000, false},
	} {
		t.Run(fmt.Sprint(tc.maxTokens), func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"example","max_tokens":%d,"thinking":{"type":"enabled","budget_tokens":32000},"messages":[{"role":"user","content":"hi"}]}`, tc.maxTokens))
			require.True(t, isThinkingBudgetConstraintError(basetenBudgetError))
			got, changed := RectifyThinkingBudget(body)
			require.Equal(t, tc.changed, changed)
			require.Equal(t, tc.want, gjson.GetBytes(got, "max_tokens").Int())
			require.Equal(t, int64(32000), gjson.GetBytes(got, "thinking.budget_tokens").Int())
			require.Equal(t, "hi", gjson.GetBytes(got, "messages.0.content").String())
		})
	}
	adaptive := []byte(`{"max_tokens":1024,"thinking":{"type":"adaptive"}}`)
	got, changed := RectifyThinkingBudget(adaptive)
	require.False(t, changed)
	require.Equal(t, adaptive, got)
	legacy := []byte(`{"max_tokens":32001,"thinking":{"type":"enabled","budget_tokens":32000}}`)
	got, changed = RectifyThinkingBudget(legacy)
	require.False(t, changed)
	require.Equal(t, legacy, got)
}
