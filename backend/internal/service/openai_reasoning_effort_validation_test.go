//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateOpenAIResponsesReasoningEffort_AllowsValidValues(t *testing.T) {
	for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh"} {
		body := []byte(`{"model":"gpt-5.5","reasoning":{"effort":"` + effort + `"},"input":"hi"}`)
		invalid, ok := ValidateOpenAIResponsesReasoningEffort(body)
		require.True(t, ok, "expected %q to pass validation", effort)
		require.Empty(t, invalid)
	}
}

func TestValidateOpenAIResponsesReasoningEffort_RejectsTypo(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":true,"reasoning":{"effort":"xhign"},"input":[{"role":"user","content":"hello"}]}`)

	invalid, ok := ValidateOpenAIResponsesReasoningEffort(body)

	require.False(t, ok)
	require.Equal(t, "xhign", invalid)
}

func TestValidateOpenAIResponsesReasoningEffort_MissingOrNonStringSkipped(t *testing.T) {
	cases := []string{
		`{"model":"gpt-5.5","input":"hi"}`,                             // 无 reasoning
		`{"model":"gpt-5.5","reasoning":{},"input":"hi"}`,              // 无 effort
		`{"model":"gpt-5.5","reasoning":{"effort":3},"input":"hi"}`,    // 非字符串类型交由上游
		`{"model":"gpt-5.5","reasoning":{"effort":null},"input":"hi"}`, // null 交由上游
	}
	for _, body := range cases {
		_, ok := ValidateOpenAIResponsesReasoningEffort([]byte(body))
		require.True(t, ok, "expected body to skip validation: %s", body)
	}
}

func TestValidateOpenAIResponsesReasoningEffort_CaseVariantsForwarded(t *testing.T) {
	// 大小写变体小写化后命中枚举 → 不本地拒绝，交由上游裁决。
	body := []byte(`{"model":"gpt-5.5","reasoning":{"effort":"High"},"input":"hi"}`)

	_, ok := ValidateOpenAIResponsesReasoningEffort(body)

	require.True(t, ok)
}

func TestOpenAIReasoningEffortInvalidValueMessage(t *testing.T) {
	msg := OpenAIReasoningEffortInvalidValueMessage("xhign")
	require.Equal(t, "Invalid value: 'xhign'. Supported values are: 'none', 'minimal', 'low', 'medium', 'high', and 'xhigh'.", msg)

	long := OpenAIReasoningEffortInvalidValueMessage(strings.Repeat("a", 200))
	require.Contains(t, long, strings.Repeat("a", 64)+"...")
	require.NotContains(t, long, strings.Repeat("a", 65))
}
