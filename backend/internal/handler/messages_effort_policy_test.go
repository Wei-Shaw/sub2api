package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMessagesEffortPolicyBinding(t *testing.T) {
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformComposite} {
		for _, test := range []struct {
			name       string
			preference string
			effort     string
			mappings   []service.ReasoningEffortMapping
			ceiling    string
			overLimit  string
			want       string
			deny       bool
		}{
			{name: "omitted unchanged", effort: "medium", want: "medium"},
			{name: "omitted ignores medium mapping", effort: "medium", mappings: []service.ReasoningEffortMapping{{From: "medium", To: "low"}}, want: "medium"},
			{name: "omitted ignores ceiling", effort: "medium", ceiling: "low", want: "medium"},
			{name: "omitted ignores deny ceiling", effort: "medium", ceiling: "low", overLimit: service.ReasoningEffortOverLimitDeny, want: "medium"},
			{name: "numeric effort is not explicit", preference: `,"output_config":{"effort":123}`, effort: "medium", ceiling: "low", mappings: []service.ReasoningEffortMapping{{From: "medium", To: "deny"}}, want: "medium"},
			{name: "boolean effort is not explicit", preference: `,"output_config":{"effort":true}`, effort: "medium", ceiling: "low", want: "medium"},
			{name: "object effort is not explicit", preference: `,"output_config":{"effort":{"value":"high"}}`, effort: "medium", ceiling: "low", want: "medium"},
			{name: "blank effort is omitted", preference: `,"output_config":{"effort":" "}`, effort: "medium", ceiling: "low", want: "medium"},
			{name: "omitted none mapping does not cap medium default", effort: "medium", ceiling: "low", mappings: []service.ReasoningEffortMapping{{From: "none", To: "low"}}, want: "medium"},
			{name: "disabled omitted applies none only once", preference: `,"thinking":{"type":"disabled"}`, effort: "none", mappings: []service.ReasoningEffortMapping{{From: "none", To: "low"}, {From: "low", To: "high"}}, want: "low"},
			{name: "disabled omitted mapping capped", preference: `,"thinking":{"type":"disabled"}`, effort: "none", mappings: []service.ReasoningEffortMapping{{From: "none", To: "xhigh"}}, ceiling: "low", want: "low"},
			{name: "disabled omitted mapping denied", preference: `,"thinking":{"type":"disabled"}`, effort: "none", mappings: []service.ReasoningEffortMapping{{From: "none", To: "xhigh"}}, ceiling: "low", overLimit: service.ReasoningEffortOverLimitDeny, deny: true},
			{name: "disabled unmapped none not capped", preference: `,"thinking":{"type":"disabled"}`, effort: "none", ceiling: "low", overLimit: service.ReasoningEffortOverLimitDeny, want: "none"},
			{name: "disabled explicit applies none", preference: `,"output_config":{"effort":"max"},"thinking":{"type":"disabled"}`, effort: "none", mappings: []service.ReasoningEffortMapping{{From: "none", To: "low"}}, want: "low"},
			{name: "explicit adaptive retains policy", preference: `,"output_config":{"effort":"high"},"thinking":{"type":"adaptive"}`, effort: "high", ceiling: "medium", want: "medium"},
			{name: "omitted retains model scope", preference: `,"thinking":{"type":"disabled"}`, effort: "none", mappings: []service.ReasoningEffortMapping{{From: "none", To: "low", Model: "gpt-6-sol", MatchType: "exact"}}, want: "none"},
		} {
			t.Run(platform+"/"+test.name, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
				apiKey := &service.APIKey{Group: &service.Group{Platform: platform, MaxReasoningEffort: test.ceiling, ReasoningEffortMappings: test.mappings, MaxReasoningEffortOverLimit: test.overLimit}}
				body := []byte(`{"model":"gpt-6.1-sol"` + test.preference + `}`)
				ensureCompositeTargetPlatform(c, apiKey, "gpt-6.1-sol")
				bindOpenAIReasoningEffortPolicyForMessagesRequest(c, apiKey, body)
				converted := []byte(`{"model":"gpt-6.1-sol","reasoning":{"effort":"` + test.effort + `"}}`)
				got, changed, err := service.ApplyOpenAIReasoningEffortPolicyFromContext(c.Request.Context(), converted)
				if test.deny {
					var overLimit *service.ReasoningEffortOverLimitError
					require.ErrorAs(t, err, &overLimit)
					require.Equal(t, "xhigh", overLimit.Requested)
					require.Equal(t, "low", overLimit.Max)
					require.False(t, changed)
					require.Equal(t, converted, got)
					return
				}
				require.NoError(t, err)
				require.Equal(t, test.want, gjson.GetBytes(got, "reasoning.effort").String())
				require.Equal(t, test.effort != test.want, changed)
				if !changed {
					require.Equal(t, converted, got)
				}
			})
		}
	}
}

func TestMessagesEffortPolicyBindingCNTargetsOmitted(t *testing.T) {
	for _, target := range []string{service.PlatformKimi, service.PlatformZhipu, service.PlatformDeepseek, service.PlatformMiniMax} {
		for _, platform := range []string{target, service.PlatformComposite, service.PlatformOpenAI} {
			for _, preference := range []string{
				``,
				`,"thinking":{"type":"disabled"}`,
				`,"thinking":{"type":"adaptive"}`,
				`,"thinking":{"type":"disabled"},"output_config":{"effort":" "}`,
				`,"thinking":{"type":"disabled"},"output_config":{"effort":123}`,
				`,"thinking":{"type":"disabled"},"output_config":{"effort":true}`,
				`,"thinking":{"type":"disabled"},"output_config":{"effort":null}`,
				`,"thinking":{"type":"disabled"},"output_config":{"effort":{}}`,
			} {
				t.Run(target+"/"+platform+"/"+preference, func(t *testing.T) {
					requestContext, _ := gin.CreateTestContext(httptest.NewRecorder())
					requestContext.Request = httptest.NewRequest("POST", "/v1/messages", nil)
					if platform != target {
						requestContext.Request = requestContext.Request.WithContext(service.WithResolvedTargetPlatform(requestContext.Request.Context(), target))
					}
					apiKey := &service.APIKey{Group: &service.Group{
						Platform: platform, MaxReasoningEffort: "low", MaxReasoningEffortOverLimit: service.ReasoningEffortOverLimitDeny,
						ReasoningEffortMappings: []service.ReasoningEffortMapping{{From: "none", To: "deny"}, {From: "medium", To: "deny"}},
					}}
					bindOpenAIReasoningEffortPolicyForMessagesRequest(requestContext, apiKey, []byte(`{"model":"glm-5.2"`+preference+`}`))
					for _, effort := range []string{"none", "medium", "high"} {
						converted := []byte(`{"model":"glm-5.2","reasoning_effort":"` + effort + `"}`)
						got, changed, err := service.ApplyOpenAIReasoningEffortPolicyFromContext(requestContext.Request.Context(), converted)
						require.NoError(t, err)
						require.False(t, changed)
						require.Equal(t, converted, got)
					}
				})
			}
		}
	}
}
