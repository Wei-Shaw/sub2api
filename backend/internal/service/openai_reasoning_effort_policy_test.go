package service

import (
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
	t.Run("canonicalizes fixed OpenAI values", func(t *testing.T) {
		got, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: " MAX ", To: " x-high "},
			{From: "minimal", To: "high"},
		})
		require.NoError(t, err)
		require.Equal(t, []ReasoningEffortMapping{
			{From: "max", To: "xhigh"},
			{From: "minimal", To: "high"},
		}, got)
	})

	t.Run("rejects empty values", func(t *testing.T) {
		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{{From: "max"}})
		require.ErrorContains(t, err, "empty or unknown")
	})

	t.Run("rejects duplicate sources case insensitively", func(t *testing.T) {
		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: "max", To: "xhigh"},
			{From: " MAX ", To: "high"},
		})
		require.ErrorContains(t, err, "duplicate")
	})

	t.Run("rejects mappings for non OpenAI platforms", func(t *testing.T) {
		for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformGrok} {
			_, err := NormalizeReasoningEffortMappings(platform, []ReasoningEffortMapping{{From: "low", To: "high"}})
			require.ErrorContains(t, err, "only supported for platform \"openai\"")
		}

		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{{From: "none", To: "low"}})
		require.ErrorContains(t, err, "empty or unknown")

		_, err = NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{{From: "ultra", To: "high"}})
		require.ErrorContains(t, err, "empty or unknown")
	})
}

func TestNormalizeMaxReasoningEffortForPlatform(t *testing.T) {
	value, err := normalizeMaxReasoningEffortForPlatform(PlatformOpenAI, "max")
	require.NoError(t, err)
	require.Equal(t, "max", value)

	for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformGrok} {
		_, err = normalizeMaxReasoningEffortForPlatform(platform, "low")
		require.ErrorContains(t, err, "only supported for platform \"openai\"")
	}

	_, err = normalizeMaxReasoningEffortForPlatform(PlatformOpenAI, "none")
	require.ErrorContains(t, err, "not supported")
}

func TestNormalizeModelReasoningEffortRules(t *testing.T) {
	t.Run("canonicalizes complete exact model overrides", func(t *testing.T) {
		got, err := NormalizeModelReasoningEffortRules(PlatformOpenAI, []ModelReasoningEffortRule{
			{
				Model:              " gpt-5.6-luna ",
				MaxReasoningEffort: " X-HIGH ",
				ReasoningEffortMappings: []ReasoningEffortMapping{
					{From: " MAX ", To: " high "},
				},
			},
			{Model: "gpt-5.6-luna-max"},
		})
		require.NoError(t, err)
		require.Equal(t, []ModelReasoningEffortRule{
			{
				Model:              "gpt-5.6-luna",
				MaxReasoningEffort: "xhigh",
				ReasoningEffortMappings: []ReasoningEffortMapping{
					{From: "max", To: "high"},
				},
			},
			{
				Model:                   "gpt-5.6-luna-max",
				MaxReasoningEffort:      "",
				ReasoningEffortMappings: []ReasoningEffortMapping{},
			},
		}, got)
	})

	t.Run("rejects empty and duplicate exact models", func(t *testing.T) {
		_, err := NormalizeModelReasoningEffortRules(PlatformOpenAI, []ModelReasoningEffortRule{{Model: " "}})
		require.ErrorContains(t, err, "empty model")

		_, err = NormalizeModelReasoningEffortRules(PlatformOpenAI, []ModelReasoningEffortRule{
			{Model: "gpt-5.6-luna"},
			{Model: " gpt-5.6-luna "},
		})
		require.ErrorContains(t, err, "duplicate")
	})

	t.Run("rejects invalid nested policies and non OpenAI groups", func(t *testing.T) {
		_, err := NormalizeModelReasoningEffortRules(PlatformOpenAI, []ModelReasoningEffortRule{
			{Model: "gpt-5.6-luna", MaxReasoningEffort: "ultra"},
		})
		require.ErrorContains(t, err, "ceiling")

		_, err = NormalizeModelReasoningEffortRules(PlatformOpenAI, []ModelReasoningEffortRule{
			{Model: "gpt-5.6-luna", ReasoningEffortMappings: []ReasoningEffortMapping{{From: "max"}}},
		})
		require.ErrorContains(t, err, "mappings")

		_, err = NormalizeModelReasoningEffortRules(PlatformAnthropic, []ModelReasoningEffortRule{
			{Model: "claude-opus-4-1"},
		})
		require.ErrorContains(t, err, "only supported")
	})
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

func TestApplyOpenAIReasoningEffortPolicyForModel(t *testing.T) {
	defaultMappings := []ReasoningEffortMapping{{From: "max", To: "high"}}
	modelRules := []ModelReasoningEffortRule{
		{
			Model:                   "gpt-5.6-luna",
			ReasoningEffortMappings: []ReasoningEffortMapping{},
		},
		{
			Model:              "gpt-5.6-sol",
			MaxReasoningEffort: "low",
			ReasoningEffortMappings: []ReasoningEffortMapping{
				{From: "max", To: "medium"},
			},
		},
	}

	t.Run("exact model can explicitly disable the group policy", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.6-luna","reasoning":{"effort":"max"}}`)
		got, changed := ApplyOpenAIReasoningEffortPolicyForModel(body, "gpt-5.6-luna", "medium", defaultMappings, modelRules)
		require.False(t, changed)
		require.Equal(t, "max", gjson.GetBytes(got, "reasoning.effort").String())
	})

	t.Run("exact model applies its own mapping then ceiling", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.6-sol","reasoning":{"effort":"max"}}`)
		got, changed := ApplyOpenAIReasoningEffortPolicyForModel(body, "gpt-5.6-sol", "high", defaultMappings, modelRules)
		require.True(t, changed)
		require.Equal(t, "low", gjson.GetBytes(got, "reasoning.effort").String())
	})

	t.Run("unmatched model falls back to the group default", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.6-terra","reasoning":{"effort":"max"}}`)
		got, changed := ApplyOpenAIReasoningEffortPolicyForModel(body, "gpt-5.6-terra", "medium", defaultMappings, modelRules)
		require.True(t, changed)
		require.Equal(t, "medium", gjson.GetBytes(got, "reasoning.effort").String())
	})

	t.Run("matching is exact and case sensitive", func(t *testing.T) {
		body := []byte(`{"model":"GPT-5.6-LUNA","reasoning":{"effort":"max"}}`)
		got, changed := ApplyOpenAIReasoningEffortPolicyForModel(body, "GPT-5.6-LUNA", "medium", nil, modelRules)
		require.True(t, changed)
		require.Equal(t, "medium", gjson.GetBytes(got, "reasoning.effort").String())
	})
}
