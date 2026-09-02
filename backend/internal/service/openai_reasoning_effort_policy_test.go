package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCanonicalRequestedReasoningEffort(t *testing.T) {
	t.Parallel()

	max := CanonicalRequestedReasoningEffort([]byte(`{"model":"gpt-5.4","reasoning":{"effort":"MAX"}}`), "gpt-5.4")
	require.NotNil(t, max)
	require.Equal(t, "max", *max)

	fromSuffix := CanonicalRequestedReasoningEffort([]byte(`{"model":"gpt-5.4-max"}`), "gpt-5.4-max")
	require.NotNil(t, fromSuffix)
	require.Equal(t, "max", *fromSuffix)

	claude := CanonicalRequestedReasoningEffort([]byte(`{"model":"claude-sonnet-4","output_config":{"effort":"high"}}`))
	require.NotNil(t, claude)
	require.Equal(t, "high", *claude)

	require.Nil(t, CanonicalRequestedReasoningEffort([]byte(`{"model":"gpt-5.4"}`), "gpt-5.4"))
}

func TestRequestedReasoningEffortContext(t *testing.T) {
	t.Parallel()

	require.Nil(t, RequestedReasoningEffortFromContext(context.Background()))
	ctx := WithRequestedReasoningEffort(context.Background(), " max ")
	got := RequestedReasoningEffortFromContext(ctx)
	require.NotNil(t, got)
	require.Equal(t, "max", *got)
}

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
		for _, platform := range []string{PlatformOpenAI, PlatformComposite} {
			got, err := NormalizeReasoningEffortMappings(platform, []ReasoningEffortMapping{
				{From: " MAX ", To: " x-high "},
				{From: "minimal", To: "high"},
			})
			require.NoError(t, err)
			require.Equal(t, []ReasoningEffortMapping{
				{From: "max", To: "xhigh"},
				{From: "minimal", To: "high"},
			}, got)
		}
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
			require.ErrorContains(t, err, "only supported for platforms \"openai\" and \"composite\"")
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

func TestNormalizeMaxReasoningEffortOverLimit(t *testing.T) {
	require.Equal(t, ReasoningEffortOverLimitDowngrade, NormalizeMaxReasoningEffortOverLimit(""))
	require.Equal(t, ReasoningEffortOverLimitDowngrade, NormalizeMaxReasoningEffortOverLimit(" downgrade "))
	require.Equal(t, ReasoningEffortOverLimitDeny, NormalizeMaxReasoningEffortOverLimit("DENY"))
	require.Empty(t, NormalizeMaxReasoningEffortOverLimit("block"))
}

func TestNormalizeMaxReasoningEffortOverLimitForPlatform(t *testing.T) {
	value, err := normalizeMaxReasoningEffortOverLimitForPlatform(PlatformOpenAI, "")
	require.NoError(t, err)
	require.Equal(t, ReasoningEffortOverLimitDowngrade, value)

	value, err = normalizeMaxReasoningEffortOverLimitForPlatform(PlatformComposite, "deny")
	require.NoError(t, err)
	require.Equal(t, ReasoningEffortOverLimitDeny, value)

	value, err = normalizeMaxReasoningEffortOverLimitForPlatform(PlatformAnthropic, "")
	require.NoError(t, err)
	require.Equal(t, ReasoningEffortOverLimitDowngrade, value)

	_, err = normalizeMaxReasoningEffortOverLimitForPlatform(PlatformAnthropic, "deny")
	require.ErrorContains(t, err, "only supported for platforms \"openai\" and \"composite\"")

	_, err = normalizeMaxReasoningEffortOverLimitForPlatform(PlatformOpenAI, "block")
	require.ErrorContains(t, err, "not supported")
}

func TestOpenAIReasoningEffortPolicyContext(t *testing.T) {
	body := []byte(`{"reasoning":{"effort":"max"}}`)

	unbound, changed, err := ApplyOpenAIReasoningEffortPolicyFromContext(context.Background(), body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, unbound)

	mappings := []ReasoningEffortMapping{{From: "max", To: "xhigh"}}
	ctx := WithOpenAIReasoningEffortPolicy(context.Background(), "medium", mappings, "")
	mappings[0].To = "low"
	got, changed, err := ApplyOpenAIReasoningEffortPolicyFromContext(ctx, body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "medium", gjson.GetBytes(got, "reasoning.effort").String())

	denyCtx := WithOpenAIReasoningEffortPolicy(context.Background(), "medium", nil, ReasoningEffortOverLimitDeny)
	_, _, err = ApplyOpenAIReasoningEffortPolicyFromContext(denyCtx, body)
	require.Error(t, err)
	var overLimit *ReasoningEffortOverLimitError
	require.ErrorAs(t, err, &overLimit)
	require.Equal(t, "max", overLimit.Requested)
	require.Equal(t, "medium", overLimit.Max)
}

func TestApplyOpenAIReasoningEffortPolicy(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		max       string
		overLimit string
		mappings  []ReasoningEffortMapping
		path      string
		want      string
		changed   bool
		deny      bool
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
		{name: "deny rejects over ceiling", body: `{"reasoning_effort":"high"}`, max: "low", overLimit: ReasoningEffortOverLimitDeny, path: "reasoning_effort", want: "high", deny: true},
		{name: "deny keeps value at ceiling", body: `{"reasoning_effort":"low"}`, max: "low", overLimit: ReasoningEffortOverLimitDeny, path: "reasoning_effort", want: "low"},
		{name: "deny after mapping still over ceiling", body: `{"reasoning":{"effort":"max"}}`, max: "medium", overLimit: ReasoningEffortOverLimitDeny, mappings: []ReasoningEffortMapping{{From: "max", To: "xhigh"}}, path: "reasoning.effort", want: "max", deny: true},
		{name: "deny allows mapping under ceiling", body: `{"reasoning":{"effort":"max"}}`, max: "medium", overLimit: ReasoningEffortOverLimitDeny, mappings: []ReasoningEffortMapping{{From: "max", To: "low"}}, path: "reasoning.effort", want: "low", changed: true},
		{name: "deny ignored without ceiling", body: `{"reasoning_effort":"high"}`, overLimit: ReasoningEffortOverLimitDeny, path: "reasoning_effort", want: "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed, err := ApplyOpenAIReasoningEffortPolicy([]byte(tt.body), tt.max, tt.mappings, tt.overLimit)
			if tt.deny {
				require.Error(t, err)
				var overLimit *ReasoningEffortOverLimitError
				require.ErrorAs(t, err, &overLimit)
				require.False(t, changed)
				require.Equal(t, tt.body, string(got))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.changed, changed)
			if tt.path != "" {
				require.Equal(t, tt.want, gjson.GetBytes(got, tt.path).String())
			}
		})
	}
}
