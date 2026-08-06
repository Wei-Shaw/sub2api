package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeOpenAIModelEffortSuffix(t *testing.T) {
	for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"} {
		t.Run(effort, func(t *testing.T) {
			got, changed, err := NormalizeOpenAIModelEffortSuffix([]byte(`{"model":"gpt-5.4-`+effort+`","messages":[]}`), false)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, "gpt-5.4", gjson.GetBytes(got, "model").String())
			require.Equal(t, effort, gjson.GetBytes(got, "reasoning_effort").String())
		})
	}
}

func TestNormalizeOpenAIModelEffortSuffixSupportsArbitraryAliases(t *testing.T) {
	for _, tc := range []struct {
		model  string
		base   string
		effort string
	}{
		{model: "my-alias-low", base: "my-alias", effort: "low"},
		{model: "gpt-5.6-sol-max", base: "gpt-5.6-sol", effort: "max"},
		{model: "claude-sonnet-4-5-minimal", base: "claude-sonnet-4-5", effort: "minimal"},
	} {
		got, changed, err := NormalizeOpenAIModelEffortSuffix([]byte(`{"model":"`+tc.model+`"}`), true)
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, tc.base, gjson.GetBytes(got, "model").String())
		require.Equal(t, tc.effort, gjson.GetBytes(got, "reasoning.effort").String())
	}
}

func TestNormalizeOpenAIModelEffortSuffixPreservesUnsupportedSuffix(t *testing.T) {
	const model = "my-alias-ultra"
	got, changed, err := NormalizeOpenAIModelEffortSuffix([]byte(`{"model":"`+model+`"}`), true)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, model, gjson.GetBytes(got, "model").String())
}

func TestNormalizeOpenAIModelEffortSuffixResponsesAndExplicitPrecedence(t *testing.T) {
	got, changed, err := NormalizeOpenAIModelEffortSuffix([]byte(`{"model":"gpt-5.4-high","input":"hello"}`), true)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "gpt-5.4", gjson.GetBytes(got, "model").String())
	require.Equal(t, "high", gjson.GetBytes(got, "reasoning.effort").String())

	got, changed, err = NormalizeOpenAIModelEffortSuffix([]byte(`{"model":"my-alias-high","reasoning":{"effort":"low"}}`), true)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "my-alias", gjson.GetBytes(got, "model").String())
	require.Equal(t, "low", gjson.GetBytes(got, "reasoning.effort").String())
}

func TestNormalizeOpenAIModelEffortSuffixIgnoresInvalidExplicitEffort(t *testing.T) {
	for _, explicit := range []string{"null", `""`, `"   "`, "123", `{"level":"low"}`} {
		t.Run(explicit, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4-high","reasoning_effort":` + explicit + `}`)
			got, changed, err := NormalizeOpenAIModelEffortSuffix(body, false)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, "gpt-5.4", gjson.GetBytes(got, "model").String())
			require.Equal(t, "high", gjson.GetBytes(got, "reasoning_effort").String())
		})
	}
}

func TestNormalizeOpenAIModelEffortSuffixResponsesCanonicalizesEffort(t *testing.T) {
	got, changed, err := NormalizeOpenAIModelEffortSuffix(
		[]byte(`{"model":"gpt-5.4-high","reasoning_effort":"low"}`),
		true,
	)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "gpt-5.4", gjson.GetBytes(got, "model").String())
	require.Equal(t, "high", gjson.GetBytes(got, "reasoning.effort").String())

	for _, explicit := range []string{"null", `""`, `"   "`, "123", `{"level":"low"}`} {
		t.Run(explicit, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4-high","reasoning":{"effort":` + explicit + `},"reasoning_effort":"low"}`)
			got, changed, err := NormalizeOpenAIModelEffortSuffix(body, true)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, "high", gjson.GetBytes(got, "reasoning.effort").String())
		})
	}
}
