package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeMaxReasoningEffort(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "separator", in: "x-high", want: "xhigh"},
		{name: "max is distinct", in: "max", want: "max"},
		{name: "none is unsupported", in: "none", want: ""},
		{name: "invalid", in: "banana", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeMaxReasoningEffort(tt.in))
		})
	}
}

func TestNormalizeReasoningEffortMappings(t *testing.T) {
	t.Run("canonicalizes known values and custom sources", func(t *testing.T) {
		for _, platform := range []string{PlatformOpenAI, PlatformComposite} {
			got, err := NormalizeReasoningEffortMappings(platform, []ReasoningEffortMapping{
				{From: " MAX ", To: " x-high "},
				{Model: " GPT-5.6-SOL ", From: " Ultra ", To: "high"},
			})
			require.NoError(t, err)
			require.Equal(t, []ReasoningEffortMapping{
				{From: "max", To: "xhigh"},
				{Model: "gpt-5.6-sol", From: "ultra", To: "high"},
			}, got)
		}
	})

	t.Run("rejects empty values", func(t *testing.T) {
		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{{From: "max"}})
		require.ErrorContains(t, err, "empty or unknown")
	})

	t.Run("rejects duplicate model and source pairs case insensitively", func(t *testing.T) {
		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{Model: "gpt-5.6-sol", From: "ultra", To: "xhigh"},
			{Model: " GPT-5.6-SOL ", From: " ULTRA ", To: "high"},
		})
		require.ErrorContains(t, err, "duplicate")
	})

	t.Run("allows the same source for different model scopes", func(t *testing.T) {
		got, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: "ultra", To: "high"},
			{Model: "gpt-5.6-sol", From: "ultra", To: "xhigh"},
		})
		require.NoError(t, err)
		require.Len(t, got, 2)
	})

	t.Run("rejects mappings for non OpenAI platforms", func(t *testing.T) {
		for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformGrok} {
			_, err := NormalizeReasoningEffortMappings(platform, []ReasoningEffortMapping{{From: "low", To: "high"}})
			require.ErrorContains(t, err, "only supported for platforms \"openai\" and \"composite\"")
		}

		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{{From: "none", To: "low"}})
		require.ErrorContains(t, err, "empty or unknown")

		_, err = NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{{From: "ultra", To: "future"}})
		require.ErrorContains(t, err, "target")
	})
}

func TestNormalizeMaxReasoningEffortForPlatform(t *testing.T) {
	value, err := normalizeMaxReasoningEffortForPlatform(PlatformOpenAI, "max")
	require.NoError(t, err)
	require.Equal(t, "max", value)
	value, err = normalizeMaxReasoningEffortForPlatform(PlatformComposite, "max")
	require.NoError(t, err)
	require.Equal(t, "max", value)

	for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformGrok} {
		_, err = normalizeMaxReasoningEffortForPlatform(platform, "low")
		require.ErrorContains(t, err, "only supported for platforms \"openai\" and \"composite\"")
	}

	_, err = normalizeMaxReasoningEffortForPlatform(PlatformOpenAI, "none")
	require.ErrorContains(t, err, "not supported")
}

func TestOpenAIReasoningEffortPolicyContext(t *testing.T) {
	body := []byte(`{"reasoning":{"effort":"max"}}`)

	unbound, changed := ApplyOpenAIReasoningEffortPolicyFromContext(context.Background(), body)
	require.False(t, changed)
	require.Equal(t, body, unbound)

	mappings := []ReasoningEffortMapping{{From: "max", To: "xhigh"}}
	ctx := WithOpenAIReasoningEffortPolicy(context.Background(), "medium", mappings)
	mappings[0].To = "low"
	got, changed := ApplyOpenAIReasoningEffortPolicyFromContext(ctx, body)
	require.True(t, changed)
	require.Equal(t, "medium", gjson.GetBytes(got, "reasoning.effort").String())
}

func TestApplyOpenAIReasoningEffortPolicy(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		max      string
		mappings []ReasoningEffortMapping
		path     string
		want     string
		changed  bool
	}{
		{name: "nested caps high", body: `{"reasoning":{"effort":"xhigh"}}`, max: "medium", path: "reasoning.effort", want: "medium", changed: true},
		{name: "flat caps high", body: `{"reasoning_effort":"high"}`, max: "low", path: "reasoning_effort", want: "low", changed: true},
		{name: "does not raise omitted", body: `{"model":"gpt-5"}`, max: "low", path: "reasoning_effort", want: "", changed: false},
		{name: "keeps lower value", body: `{"reasoning_effort":"low"}`, max: "high", path: "reasoning_effort", want: "low", changed: false},
		{name: "normalizes request alias", body: `{"reasoning_effort":"x-high"}`, max: "xhigh", path: "reasoning_effort", want: "xhigh", changed: true},
		{name: "caps max below its distinct rank", body: `{"reasoning_effort":"max"}`, max: "xhigh", path: "reasoning_effort", want: "xhigh", changed: true},
		{name: "keeps xhigh below max", body: `{"reasoning_effort":"xhigh"}`, max: "max", path: "reasoning_effort", want: "xhigh", changed: false},
		{name: "ignores stale none ceiling", body: `{"reasoning_effort":"high"}`, max: "none", path: "reasoning_effort", want: "high", changed: false},
		{name: "caps both shapes", body: `{"reasoning":{"effort":"high"},"reasoning_effort":"xhigh"}`, max: "low", path: "reasoning.effort", want: "low", changed: true},
		{name: "maps before cap", body: `{"reasoning":{"effort":"MAX"}}`, max: "medium", mappings: []ReasoningEffortMapping{{From: "max", To: "xhigh"}}, path: "reasoning.effort", want: "medium", changed: true},
		{name: "does not chain mappings", body: `{"reasoning_effort":"max"}`, mappings: []ReasoningEffortMapping{{From: "max", To: "xhigh"}, {From: "xhigh", To: "low"}}, path: "reasoning_effort", want: "xhigh", changed: true},
		{name: "maps custom source for matching model", body: `{"model":"gpt-5.6-sol","reasoning":{"effort":"ULTRA"}}`, mappings: []ReasoningEffortMapping{{Model: "gpt-5.6-sol", From: "ultra", To: "xhigh"}}, path: "reasoning.effort", want: "xhigh", changed: true},
		{name: "does not map custom source for another model", body: `{"model":"gpt-5.6","reasoning":{"effort":"ultra"}}`, mappings: []ReasoningEffortMapping{{Model: "gpt-5.6-sol", From: "ultra", To: "xhigh"}}, path: "reasoning.effort", want: "ultra", changed: false},
		{name: "model mapping takes priority over global", body: `{"model":"gpt-5.6-sol","reasoning":{"effort":"ultra"}}`, mappings: []ReasoningEffortMapping{{From: "ultra", To: "high"}, {Model: "gpt-5.6-sol", From: "ultra", To: "xhigh"}}, path: "reasoning.effort", want: "xhigh", changed: true},
		{name: "global mapping is fallback for model", body: `{"model":"gpt-5.6","reasoning":{"effort":"ultra"}}`, mappings: []ReasoningEffortMapping{{From: "ultra", To: "high"}, {Model: "gpt-5.6-sol", From: "ultra", To: "xhigh"}}, path: "reasoning.effort", want: "high", changed: true},
		{name: "caps model mapping target", body: `{"model":"gpt-5.6-sol","reasoning":{"effort":"ultra"}}`, max: "medium", mappings: []ReasoningEffortMapping{{Model: "gpt-5.6-sol", From: "ultra", To: "xhigh"}}, path: "reasoning.effort", want: "medium", changed: true},
		{name: "keeps unknown without mapping", body: `{"reasoning_effort":"future"}`, max: "low", path: "reasoning_effort", want: "future", changed: false},
		{name: "keeps non string value", body: `{"reasoning_effort":{"level":"high"}}`, max: "low", path: "reasoning_effort.level", want: "high", changed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := ApplyOpenAIReasoningEffortPolicy([]byte(tt.body), tt.max, tt.mappings)
			require.Equal(t, tt.changed, changed)
			if tt.path != "" {
				require.Equal(t, tt.want, gjson.GetBytes(got, tt.path).String())
			}
		})
	}
}

func TestApplyOpenAIReasoningEffortPolicyFromContextUsesBoundClientModel(t *testing.T) {
	body := []byte(`{"model":"mapped-upstream","reasoning":{"effort":"ultra"}}`)
	mappings := []ReasoningEffortMapping{{Model: "gpt-5.6-sol", From: "ultra", To: "xhigh"}}
	ctx := WithOpenAIReasoningEffortPolicy(context.Background(), "", mappings, "gpt-5.6-sol")

	got, changed := ApplyOpenAIReasoningEffortPolicyFromContext(ctx, body)
	require.True(t, changed)
	require.Equal(t, "xhigh", gjson.GetBytes(got, "reasoning.effort").String())
}

func TestApplyOpenAIMessagesReasoningEffortPolicyUsesOriginalSourceAfterBridge(t *testing.T) {
	mappings := []ReasoningEffortMapping{{Model: "gpt-5.6-sol", From: "max", To: "high"}}
	ctx := WithOpenAIMessagesReasoningEffortPolicy(context.Background(), "", mappings, "gpt-5.6-sol", "max")

	// The Messages bridge has already converted max to xhigh for an older
	// upstream model. The policy must still match the original client source.
	body := []byte(`{"model":"mapped-upstream","reasoning":{"effort":"xhigh"}}`)
	got, changed := ApplyOpenAIReasoningEffortPolicyFromContext(ctx, body)
	require.True(t, changed)
	require.Equal(t, "high", gjson.GetBytes(got, "reasoning.effort").String())
}

func TestApplyOpenAIMessagesReasoningEffortPolicyKeepsConvertedValueWhenUnmapped(t *testing.T) {
	ctx := WithOpenAIMessagesReasoningEffortPolicy(
		context.Background(),
		"",
		[]ReasoningEffortMapping{{From: "ultra", To: "high"}},
		"gpt-5.6-sol",
		"max",
	)
	body := []byte(`{"model":"mapped-upstream","reasoning":{"effort":"xhigh"}}`)

	got, changed := ApplyOpenAIReasoningEffortPolicyFromContext(ctx, body)
	require.False(t, changed)
	require.Equal(t, body, got)
}
