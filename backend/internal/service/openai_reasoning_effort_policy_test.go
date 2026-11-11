package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func TestGLMReasoningEffortPolicyPreservesNoneAndMinimal(t *testing.T) {
	for _, model := range []string{"glm-5.2", "glm-5.3"} {
		for _, field := range []string{`"reasoning":{"effort":"%s"}`, `"reasoning_effort":"%s"`} {
			for _, effort := range []string{"none", "minimal"} {
				for _, ceiling := range []string{"minimal", "low"} {
					for _, overLimit := range []string{ReasoningEffortOverLimitDowngrade, ReasoningEffortOverLimitDeny} {
						t.Run(model+"/"+field+"/"+effort+"/"+ceiling+"/"+overLimit, func(t *testing.T) {
							body := []byte(fmt.Sprintf(`{"model":%q,`+field+`}`, model, effort))
							ctx := WithOpenAIReasoningEffortPolicy(context.Background(), ceiling, nil, overLimit)
							normalized, normalizationChanged := NormalizeGLMOpenAIReasoningEffort(body, model)
							require.False(t, normalizationChanged)
							got, changed, err := ApplyOpenAIReasoningEffortPolicyFromContext(ctx, normalized)
							require.NoError(t, err)
							require.False(t, changed)
							require.Equal(t, body, got)
						})
					}
				}
			}
		}
	}
}

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
	require.Nil(t, CanonicalRequestedReasoningEffort([]byte(`{"reasoning_effort":"none"}`)))
	require.Nil(t, CanonicalRequestedReasoningEffort([]byte(`{"reasoning":{"effort":"NoNe"}}`)))
}

func TestRequestedReasoningEffortContext(t *testing.T) {
	t.Parallel()

	require.Nil(t, RequestedReasoningEffortFromContext(context.Background()))
	ctx := WithRequestedReasoningEffort(context.Background(), " max ")
	got := RequestedReasoningEffortFromContext(ctx)
	require.NotNil(t, got)
	require.Equal(t, "max", *got)
}

func TestRawRequestedReasoningEffortDiagnostics(t *testing.T) {
	for _, field := range []string{"reasoning.effort", "reasoning_effort", "output_config.effort"} {
		for _, token := range []string{"none", "NoNe", " HIGH ", "MAX", "vendor-token"} {
			t.Run(field+"/"+token, func(t *testing.T) {
				body, err := sjson.SetBytes([]byte(`{"model":"glm-4.6"}`), field, token)
				require.NoError(t, err)
				ctx := WithRawRequestedReasoningEffort(context.Background(), body)
				require.Nil(t, RequestedReasoningEffortFromContext(ctx))
				canonical := CanonicalRequestedReasoningEffort(body)
				if canonical != nil {
					ctx = WithRequestedReasoningEffort(ctx, *canonical)
				}
				converted := []byte(`{"model":"glm-4.6","output_config":{"effort":"max"}}`)
				ctx = WithRawRequestedReasoningEffort(ctx, converted)
				ctx = WithOpenAIReasoningEffortPolicy(ctx, "xhigh", nil, "deny")
				_, _, err = applyFinalOpenAIReasoningEffortCeiling(ctx, &Account{Platform: PlatformZhipu}, converted, "glm-4.6")
				var capDenial *ReasoningEffortOverLimitError
				require.ErrorAs(t, err, &capDenial)
				require.Equal(t, token, capDenial.Requested)
				_, _, err = ApplyOpenAIReasoningEffortPolicyFromContext(ctx, converted)
				require.ErrorAs(t, err, &capDenial)
				require.Equal(t, token, capDenial.Requested)
				ctx = WithOpenAIReasoningEffortPolicy(ctx, "", []ReasoningEffortMapping{{From: "max", To: "deny"}}, "")
				_, _, err = ApplyOpenAIReasoningEffortPolicyFromContext(ctx, converted)
				var mappingDenial *ReasoningEffortMappingDeniedError
				require.ErrorAs(t, err, &mappingDenial)
				require.Equal(t, token, mappingDenial.Requested)
				require.Equal(t, canonical, RequestedReasoningEffortFromContext(ctx))
			})
		}
	}
	for _, body := range []string{`{}`, `{"reasoning_effort":null}`, `{"reasoning_effort":123}`, `{"reasoning_effort":" "}`} {
		require.Nil(t, diagnosticRequestedReasoningEffortFromContext(WithRawRequestedReasoningEffort(context.Background(), []byte(body))))
	}
	ctx := WithRequestedReasoningEffort(context.Background(), "max")
	require.Equal(t, RequestedReasoningEffortFromContext(ctx), diagnosticRequestedReasoningEffortFromContext(ctx))
}

func TestFinalCNGLMReasoningCeiling(t *testing.T) {
	for _, test := range []struct {
		name, model, effort, ceiling, action, want string
		denied                                     bool
	}{
		{name: "53 medium selects low", model: "glm-5.3", effort: "high", ceiling: "medium", want: "low"},
		{name: "53 xhigh selects high", model: "glm-5.3", effort: "max", ceiling: "xhigh", want: "high"},
		{name: "older xhigh selects high", model: "glm-4.6", effort: "max", ceiling: "xhigh", want: "high"},
		{name: "older medium has no tier", model: "glm-4.6", effort: "high", ceiling: "medium", denied: true},
		{name: "53 minimal has no tier", model: "glm-5.3", effort: "low", ceiling: "minimal", denied: true},
		{name: "deny does not downgrade", model: "glm-5.3", effort: "high", ceiling: "medium", action: "deny", denied: true},
		{name: "none is not a tier", model: "glm-4.6", effort: "none", ceiling: "low", action: "deny", want: "none"},
		{name: "minimal is not a tier", model: "glm-4.6", effort: "minimal", ceiling: "low", want: "minimal"},
		{name: "mapping only none", model: "glm-4.6", effort: "none", want: "none"},
		{name: "unknown tier cannot bypass ceiling", model: "glm-4.6", effort: "unknown", ceiling: "medium", denied: true},
	} {
		for _, path := range []string{"reasoning.effort", "reasoning_effort", "output_config.effort"} {
			t.Run(test.name+"/"+path, func(t *testing.T) {
				body, err := sjson.SetBytes([]byte(`{}`), path, test.effort)
				require.NoError(t, err)
				ctx := WithRequestedReasoningEffort(context.Background(), "medium")
				ctx = WithOpenAIReasoningEffortPolicy(ctx, test.ceiling, []ReasoningEffortMapping{{From: "high", To: "deny"}}, test.action)
				got, changed, err := applyFinalOpenAIReasoningEffortCeiling(ctx, &Account{Platform: PlatformZhipu}, body, test.model)
				if test.denied {
					var denied *ReasoningEffortOverLimitError
					require.ErrorAs(t, err, &denied)
					require.Equal(t, "medium", denied.Requested)
					require.Equal(t, test.ceiling, denied.Max)
					require.False(t, changed)
					require.Equal(t, body, got)
					if test.action != "deny" {
						require.ErrorContains(t, err, "no supported native reasoning tier")
					}
					return
				}
				require.NoError(t, err)
				require.Equal(t, test.want, gjson.GetBytes(got, path).String())
				require.Equal(t, test.want != test.effort, changed)
			})
		}
	}
}

func TestReasoningPolicyDenyPreservesInboundEffort(t *testing.T) {
	body := []byte(`{"model":"routed-model","output_config":{"effort":"low"}}`)
	mappings := []ReasoningEffortMapping{{From: "low", To: "max", MatchType: "exact", Model: "client-model"}}
	ctx := WithRequestedReasoningEffort(context.Background(), "minimal")
	ctx = WithOpenAIReasoningEffortPolicyForModel(ctx, "high", mappings, "deny", "client-model")
	_, _, err := ApplyOpenAIReasoningEffortPolicyFromContext(ctx, body)
	var denied *ReasoningEffortOverLimitError
	require.ErrorAs(t, err, &denied)
	require.Equal(t, "minimal", denied.Requested)
	_, _, err = ApplyReasoningEffortPolicy(body, "high", []ReasoningEffortMapping{{From: "low", To: "max"}}, "deny")
	require.ErrorAs(t, err, &denied)
	require.Equal(t, "low", denied.Requested)
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

	t.Run("allows same source across different model scopes", func(t *testing.T) {
		got, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: "max", To: "low", MatchType: "prefix", Model: " gpt "},
			{From: "max", To: "medium", MatchType: "exact", Model: "GPT-5.4"},
			{From: "max", To: "high"},
		})
		require.NoError(t, err)
		require.Equal(t, []ReasoningEffortMapping{
			{From: "max", To: "low", MatchType: domain.ReasoningEffortMatchPrefix, Model: "gpt"},
			{From: "max", To: "medium", MatchType: domain.ReasoningEffortMatchExact, Model: "GPT-5.4"},
			{From: "max", To: "high"},
		}, got)
	})

	t.Run("defaults missing match type to exact when model is set", func(t *testing.T) {
		got, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: "max", To: "low", Model: "gpt-5.4"},
		})
		require.NoError(t, err)
		require.Equal(t, []ReasoningEffortMapping{
			{From: "max", To: "low", MatchType: domain.ReasoningEffortMatchExact, Model: "gpt-5.4"},
		}, got)
	})

	t.Run("empty type and model collapse to a global mapping", func(t *testing.T) {
		got, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: "max", To: "low", MatchType: "prefix"},
			{From: "high", To: "low", MatchType: "suffix"},
		})
		require.NoError(t, err)
		require.Equal(t, []ReasoningEffortMapping{
			{From: "max", To: "low"},
			{From: "high", To: "low"},
		}, got)
	})

	t.Run("canonicalizes suffix match", func(t *testing.T) {
		got, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: "max", To: "low", MatchType: " SUFFIX ", Model: " mini "},
		})
		require.NoError(t, err)
		require.Equal(t, []ReasoningEffortMapping{
			{From: "max", To: "low", MatchType: domain.ReasoningEffortMatchSuffix, Model: "mini"},
		}, got)
	})

	t.Run("rejects invalid match type", func(t *testing.T) {
		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: "max", To: "low", MatchType: "wildcard", Model: "gpt"},
		})
		require.ErrorContains(t, err, "invalid match_type")
	})

	t.Run("rejects duplicate source within the same model scope", func(t *testing.T) {
		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: "max", To: "low", MatchType: "prefix", Model: "gpt"},
			{From: "MAX", To: "high", MatchType: "PREFIX", Model: " GPT "},
		})
		require.ErrorContains(t, err, "duplicate")
		require.ErrorContains(t, err, "gpt")
	})

	t.Run("rejects mappings for non OpenAI platforms", func(t *testing.T) {
		for _, platform := range []string{PlatformGemini, PlatformAntigravity, PlatformGrok} {
			_, err := NormalizeReasoningEffortMappings(platform, []ReasoningEffortMapping{{From: "low", To: "high"}})
			require.ErrorContains(t, err, "only supported for platforms")
		}

		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{{From: "ultra", To: "high"}})
		require.ErrorContains(t, err, "empty or unknown")
	})

	t.Run("allows none only as a source for OpenAI routes", func(t *testing.T) {
		for _, platform := range []string{PlatformOpenAI, PlatformComposite} {
			got, err := NormalizeReasoningEffortMappings(platform, []ReasoningEffortMapping{{From: " NONE ", To: "low"}})
			require.NoError(t, err)
			require.Equal(t, []ReasoningEffortMapping{{From: "none", To: "low"}}, got)
		}

		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{{From: "low", To: "none"}})
		require.ErrorContains(t, err, "empty or unknown")
	})

	t.Run("allows deny as a mapping target", func(t *testing.T) {
		for _, platform := range []string{PlatformOpenAI, PlatformComposite, PlatformAnthropic} {
			got, err := NormalizeReasoningEffortMappings(platform, []ReasoningEffortMapping{{From: " XHIGH ", To: " DENY "}})
			require.NoError(t, err)
			require.Equal(t, []ReasoningEffortMapping{{From: "xhigh", To: ReasoningEffortMappingDeny}}, got)
		}

		got, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: "none", To: "deny"},
			{From: "max", To: "deny", MatchType: "prefix", Model: "gpt"},
		})
		require.NoError(t, err)
		require.Equal(t, []ReasoningEffortMapping{
			{From: "none", To: ReasoningEffortMappingDeny},
			{From: "max", To: ReasoningEffortMappingDeny, MatchType: domain.ReasoningEffortMatchPrefix, Model: "gpt"},
		}, got)

		_, err = NormalizeReasoningEffortMappings(PlatformGemini, []ReasoningEffortMapping{{From: "high", To: "deny"}})
		require.ErrorContains(t, err, "only supported for platforms")
	})

	t.Run("supports Anthropic values except minimal", func(t *testing.T) {
		got, err := NormalizeReasoningEffortMappings(PlatformAnthropic, []ReasoningEffortMapping{{From: " MAX ", To: " x-high "}})
		require.NoError(t, err)
		require.Equal(t, []ReasoningEffortMapping{{From: "max", To: "xhigh"}}, got)

		_, err = NormalizeReasoningEffortMappings(PlatformAnthropic, []ReasoningEffortMapping{{From: "minimal", To: "low"}})
		require.ErrorContains(t, err, "not supported for platform \"anthropic\"")
	})
}

func TestNormalizeMaxReasoningEffortForPlatform(t *testing.T) {
	value, err := normalizeMaxReasoningEffortForPlatform(PlatformOpenAI, "max")
	require.NoError(t, err)
	require.Equal(t, "max", value)
	value, err = normalizeMaxReasoningEffortForPlatform(PlatformComposite, "max")
	require.NoError(t, err)
	require.Equal(t, "max", value)

	value, err = normalizeMaxReasoningEffortForPlatform(PlatformAnthropic, "xhigh")
	require.NoError(t, err)
	require.Equal(t, "xhigh", value)
	_, err = normalizeMaxReasoningEffortForPlatform(PlatformAnthropic, "minimal")
	require.ErrorContains(t, err, "not supported")

	for _, platform := range []string{PlatformGemini, PlatformAntigravity, PlatformGrok} {
		_, err = normalizeMaxReasoningEffortForPlatform(platform, "low")
		require.ErrorContains(t, err, "only supported for platforms")
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

	value, err = normalizeMaxReasoningEffortOverLimitForPlatform(PlatformAnthropic, "deny")
	require.NoError(t, err)
	require.Equal(t, ReasoningEffortOverLimitDeny, value)

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
		name        string
		body        string
		max         string
		overLimit   string
		mappings    []ReasoningEffortMapping
		path        string
		want        string
		changed     bool
		deny        bool
		mappingDeny bool
	}{
		{name: "nested caps high", body: `{"reasoning":{"effort":"xhigh"}}`, max: "medium", path: "reasoning.effort", want: "medium", changed: true},
		{name: "flat caps high", body: `{"reasoning_effort":"high"}`, max: "low", path: "reasoning_effort", want: "low", changed: true},
		{name: "Anthropic output config caps max", body: `{"output_config":{"effort":"max"}}`, max: "xhigh", path: "output_config.effort", want: "xhigh", changed: true},
		{name: "Anthropic output config maps before cap", body: `{"output_config":{"effort":"max"}}`, max: "high", mappings: []ReasoningEffortMapping{{From: "max", To: "xhigh"}}, path: "output_config.effort", want: "high", changed: true},
		{name: "does not raise omitted", body: `{"model":"gpt-5"}`, max: "low", path: "reasoning_effort", want: "", changed: false},
		{name: "keeps lower value", body: `{"reasoning_effort":"low"}`, max: "high", path: "reasoning_effort", want: "low", changed: false},
		{name: "normalizes request alias", body: `{"reasoning_effort":"x-high"}`, max: "xhigh", path: "reasoning_effort", want: "xhigh", changed: true},
		{name: "caps max below its distinct rank", body: `{"reasoning_effort":"max"}`, max: "xhigh", path: "reasoning_effort", want: "xhigh", changed: true},
		{name: "keeps xhigh below max", body: `{"reasoning_effort":"xhigh"}`, max: "max", path: "reasoning_effort", want: "xhigh", changed: false},
		{name: "ignores stale none ceiling", body: `{"reasoning_effort":"high"}`, max: "none", path: "reasoning_effort", want: "high", changed: false},
		{name: "caps both shapes", body: `{"reasoning":{"effort":"high"},"reasoning_effort":"xhigh"}`, max: "low", path: "reasoning.effort", want: "low", changed: true},
		{name: "maps before cap", body: `{"reasoning":{"effort":"MAX"}}`, max: "medium", mappings: []ReasoningEffortMapping{{From: "max", To: "xhigh"}}, path: "reasoning.effort", want: "medium", changed: true},
		{name: "does not chain mappings", body: `{"reasoning_effort":"max"}`, mappings: []ReasoningEffortMapping{{From: "max", To: "xhigh"}, {From: "xhigh", To: "low"}}, path: "reasoning_effort", want: "xhigh", changed: true},
		{name: "maps nested none", body: `{"reasoning":{"effort":"none"}}`, mappings: []ReasoningEffortMapping{{From: "none", To: "low"}}, path: "reasoning.effort", want: "low", changed: true},
		{name: "maps flat none", body: `{"reasoning_effort":"none"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "low"}}, path: "reasoning_effort", want: "low", changed: true},
		{name: "none mapping runs before cap", body: `{"reasoning_effort":"none"}`, max: "low", mappings: []ReasoningEffortMapping{{From: "none", To: "high"}}, path: "reasoning_effort", want: "low", changed: true},
		{name: "keeps none byte for byte without mappings", body: `{ "reasoning_effort": " none " }`, max: "low", path: "reasoning_effort", want: " none ", changed: false},
		{name: "keeps none byte for byte when scope does not match", body: `{ "model":"o3", "reasoning_effort":"none" }`, mappings: []ReasoningEffortMapping{{From: "none", To: "low", MatchType: domain.ReasoningEffortMatchExact, Model: "gpt-5"}}, path: "reasoning_effort", want: "none", changed: false},
		{name: "exact scope maps none", body: `{"model":"gpt-5","reasoning_effort":"none"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "low", MatchType: domain.ReasoningEffortMatchExact, Model: "gpt-5"}}, path: "reasoning_effort", want: "low", changed: true},
		{name: "prefix scope maps none", body: `{"model":"gpt-5-mini","reasoning_effort":"none"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "low", MatchType: domain.ReasoningEffortMatchPrefix, Model: "gpt-5"}}, path: "reasoning_effort", want: "low", changed: true},
		{name: "suffix scope maps none", body: `{"model":"gpt-5-mini","reasoning_effort":"none"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "low", MatchType: domain.ReasoningEffortMatchSuffix, Model: "mini"}}, path: "reasoning_effort", want: "low", changed: true},
		{name: "global scope maps none", body: `{"model":"o3","reasoning_effort":"none"}`, mappings: []ReasoningEffortMapping{{From: "none", To: "low"}}, path: "reasoning_effort", want: "low", changed: true},
		{name: "keeps unknown without mapping", body: `{"reasoning_effort":"future"}`, max: "low", path: "reasoning_effort", want: "future", changed: false},
		{name: "keeps non string value", body: `{"reasoning_effort":{"level":"high"}}`, max: "low", path: "reasoning_effort.level", want: "high", changed: false},
		{
			name:     "prefix mapping applies to matching model",
			body:     `{"model":"gpt-5.4","reasoning_effort":"max"}`,
			mappings: []ReasoningEffortMapping{{From: "max", To: "low", MatchType: domain.ReasoningEffortMatchPrefix, Model: "gpt"}},
			path:     "reasoning_effort",
			want:     "low",
			changed:  true,
		},
		{
			name:     "prefix mapping skips non matching model",
			body:     `{"model":"o3","reasoning_effort":"max"}`,
			mappings: []ReasoningEffortMapping{{From: "max", To: "low", MatchType: domain.ReasoningEffortMatchPrefix, Model: "gpt"}},
			path:     "reasoning_effort",
			want:     "max",
			changed:  false,
		},
		{
			name: "exact mapping beats prefix",
			body: `{"model":"gpt-5.4","reasoning":{"effort":"max"}}`,
			mappings: []ReasoningEffortMapping{
				{From: "max", To: "low", MatchType: domain.ReasoningEffortMatchPrefix, Model: "gpt"},
				{From: "max", To: "medium", MatchType: domain.ReasoningEffortMatchExact, Model: "gpt-5.4"},
			},
			path:    "reasoning.effort",
			want:    "medium",
			changed: true,
		},
		{
			name: "longer prefix beats shorter prefix",
			body: `{"model":"gpt-5.4","reasoning_effort":"max"}`,
			mappings: []ReasoningEffortMapping{
				{From: "max", To: "low", MatchType: domain.ReasoningEffortMatchPrefix, Model: "gpt"},
				{From: "max", To: "high", MatchType: domain.ReasoningEffortMatchPrefix, Model: "gpt-5"},
			},
			path:    "reasoning_effort",
			want:    "high",
			changed: true,
		},
		{
			name: "falls back to global mapping when no model scope hits",
			body: `{"model":"o3","reasoning_effort":"max"}`,
			mappings: []ReasoningEffortMapping{
				{From: "max", To: "low", MatchType: domain.ReasoningEffortMatchPrefix, Model: "gpt"},
				{From: "max", To: "medium"},
			},
			path:    "reasoning_effort",
			want:    "medium",
			changed: true,
		},
		{
			name: "model scoped mapping still respects ceiling",
			body: `{"model":"gpt-5.4","reasoning":{"effort":"max"}}`,
			max:  "medium",
			mappings: []ReasoningEffortMapping{
				{From: "max", To: "xhigh", MatchType: domain.ReasoningEffortMatchPrefix, Model: "gpt"},
			},
			path:    "reasoning.effort",
			want:    "medium",
			changed: true,
		},
		{
			name:     "model match is case insensitive",
			body:     `{"model":"GPT-5.4","reasoning_effort":"MAX"}`,
			mappings: []ReasoningEffortMapping{{From: "max", To: "low", MatchType: domain.ReasoningEffortMatchExact, Model: "gpt-5.4"}},
			path:     "reasoning_effort",
			want:     "low",
			changed:  true,
		},
		{
			name:     "suffix mapping applies to matching model",
			body:     `{"model":"gpt-5.4-mini","reasoning_effort":"max"}`,
			mappings: []ReasoningEffortMapping{{From: "max", To: "low", MatchType: domain.ReasoningEffortMatchSuffix, Model: "mini"}},
			path:     "reasoning_effort",
			want:     "low",
			changed:  true,
		},
		{
			name:     "suffix mapping skips non matching model",
			body:     `{"model":"gpt-5.4","reasoning_effort":"max"}`,
			mappings: []ReasoningEffortMapping{{From: "max", To: "low", MatchType: domain.ReasoningEffortMatchSuffix, Model: "mini"}},
			path:     "reasoning_effort",
			want:     "max",
			changed:  false,
		},
		{
			name: "exact mapping beats suffix",
			body: `{"model":"gpt-5.4-mini","reasoning":{"effort":"max"}}`,
			mappings: []ReasoningEffortMapping{
				{From: "max", To: "low", MatchType: domain.ReasoningEffortMatchSuffix, Model: "mini"},
				{From: "max", To: "medium", MatchType: domain.ReasoningEffortMatchExact, Model: "gpt-5.4-mini"},
			},
			path:    "reasoning.effort",
			want:    "medium",
			changed: true,
		},
		{
			name: "longer suffix beats shorter suffix",
			body: `{"model":"gpt-5.4-mini","reasoning_effort":"max"}`,
			mappings: []ReasoningEffortMapping{
				{From: "max", To: "low", MatchType: domain.ReasoningEffortMatchSuffix, Model: "i"},
				{From: "max", To: "high", MatchType: domain.ReasoningEffortMatchSuffix, Model: "mini"},
			},
			path:    "reasoning_effort",
			want:    "high",
			changed: true,
		},
		{
			name: "longer affix beats the other affix",
			body: `{"model":"gpt-5.4-mini","reasoning_effort":"max"}`,
			mappings: []ReasoningEffortMapping{
				{From: "max", To: "low", MatchType: domain.ReasoningEffortMatchSuffix, Model: "mini"},
				{From: "max", To: "high", MatchType: domain.ReasoningEffortMatchPrefix, Model: "gpt-5.4"},
			},
			path:    "reasoning_effort",
			want:    "high",
			changed: true,
		},
		{
			name:     "empty type and model apply to every model",
			body:     `{"model":"o3","reasoning_effort":"max"}`,
			mappings: []ReasoningEffortMapping{{From: "max", To: "low"}},
			path:     "reasoning_effort",
			want:     "low",
			changed:  true,
		},
		{name: "deny rejects over ceiling", body: `{"reasoning_effort":"high"}`, max: "low", overLimit: ReasoningEffortOverLimitDeny, path: "reasoning_effort", want: "high", deny: true},
		{name: "deny keeps value at ceiling", body: `{"reasoning_effort":"low"}`, max: "low", overLimit: ReasoningEffortOverLimitDeny, path: "reasoning_effort", want: "low"},
		{name: "deny after mapping still over ceiling", body: `{"reasoning":{"effort":"max"}}`, max: "medium", overLimit: ReasoningEffortOverLimitDeny, mappings: []ReasoningEffortMapping{{From: "max", To: "xhigh"}}, path: "reasoning.effort", want: "max", deny: true},
		{name: "deny allows mapping under ceiling", body: `{"reasoning":{"effort":"max"}}`, max: "medium", overLimit: ReasoningEffortOverLimitDeny, mappings: []ReasoningEffortMapping{{From: "max", To: "low"}}, path: "reasoning.effort", want: "low", changed: true},
		{name: "deny ignored without ceiling", body: `{"reasoning_effort":"high"}`, overLimit: ReasoningEffortOverLimitDeny, path: "reasoning_effort", want: "high"},
		{name: "mapping deny rejects matching nested effort", body: `{"reasoning":{"effort":"xhigh"}}`, mappings: []ReasoningEffortMapping{{From: "xhigh", To: ReasoningEffortMappingDeny}}, path: "reasoning.effort", want: "xhigh", mappingDeny: true},
		{name: "mapping deny rejects matching flat effort", body: `{"reasoning_effort":"X-HIGH"}`, mappings: []ReasoningEffortMapping{{From: "xhigh", To: " DENY "}}, path: "reasoning_effort", want: "X-HIGH", mappingDeny: true},
		{name: "mapping deny rejects Anthropic output config", body: `{"output_config":{"effort":"max"}}`, mappings: []ReasoningEffortMapping{{From: "max", To: ReasoningEffortMappingDeny}}, path: "output_config.effort", want: "max", mappingDeny: true},
		{name: "mapping deny rejects none source", body: `{"reasoning_effort":"none"}`, mappings: []ReasoningEffortMapping{{From: "none", To: ReasoningEffortMappingDeny}}, path: "reasoning_effort", want: "none", mappingDeny: true},
		{
			name:        "mapping deny respects model scope",
			body:        `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh"}}`,
			mappings:    []ReasoningEffortMapping{{From: "xhigh", To: ReasoningEffortMappingDeny, MatchType: domain.ReasoningEffortMatchPrefix, Model: "gpt-5.6-sol"}},
			path:        "reasoning.effort",
			want:        "xhigh",
			mappingDeny: true,
		},
		{
			name:     "mapping deny skips non matching model",
			body:     `{"model":"gpt-5.6-terra","reasoning":{"effort":"xhigh"}}`,
			mappings: []ReasoningEffortMapping{{From: "xhigh", To: ReasoningEffortMappingDeny, MatchType: domain.ReasoningEffortMatchExact, Model: "gpt-5.6-sol"}},
			path:     "reasoning.effort",
			want:     "xhigh",
		},
		{
			name:     "mapping deny leaves other efforts unchanged",
			body:     `{"reasoning_effort":"high"}`,
			mappings: []ReasoningEffortMapping{{From: "xhigh", To: ReasoningEffortMappingDeny}},
			path:     "reasoning_effort",
			want:     "high",
		},
		{
			name:        "mapping deny takes precedence over ceiling rewrite",
			body:        `{"reasoning":{"effort":"xhigh"}}`,
			max:         "medium",
			mappings:    []ReasoningEffortMapping{{From: "xhigh", To: ReasoningEffortMappingDeny}},
			path:        "reasoning.effort",
			want:        "xhigh",
			mappingDeny: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed, err := ApplyOpenAIReasoningEffortPolicy([]byte(tt.body), tt.max, tt.mappings, tt.overLimit)
			if tt.deny {
				require.Error(t, err)
				var overLimit *ReasoningEffortOverLimitError
				require.ErrorAs(t, err, &overLimit)
				require.True(t, IsReasoningEffortPolicyDenied(err))
				require.False(t, changed)
				require.Equal(t, tt.body, string(got))
				return
			}
			if tt.mappingDeny {
				require.Error(t, err)
				var mappingDenied *ReasoningEffortMappingDeniedError
				require.ErrorAs(t, err, &mappingDenied)
				require.True(t, IsReasoningEffortPolicyDenied(err))
				require.Contains(t, mappingDenied.Error(), "denied by this group's mapping policy")
				if requested := normalizeReasoningEffortMappingSource(tt.want); requested != "" {
					require.Equal(t, requested, mappingDenied.Requested)
				}
				require.False(t, changed)
				require.Equal(t, tt.body, string(got))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.changed, changed)
			if !tt.changed {
				require.Equal(t, tt.body, string(got))
			}
			if tt.path != "" {
				require.Equal(t, tt.want, gjson.GetBytes(got, tt.path).String())
			}
		})
	}
}

func TestReasoningEffortMappingDeniedError(t *testing.T) {
	t.Parallel()

	err := &ReasoningEffortMappingDeniedError{Requested: "xhigh"}
	require.Equal(t, `reasoning effort "xhigh" is denied by this group's mapping policy`, err.Error())
	require.True(t, IsReasoningEffortPolicyDenied(err))
	require.True(t, IsReasoningEffortPolicyDenied(&ReasoningEffortOverLimitError{Requested: "max", Max: "low"}))
	require.False(t, IsReasoningEffortPolicyDenied(nil))
	require.False(t, IsReasoningEffortPolicyDenied(context.Canceled))
}
