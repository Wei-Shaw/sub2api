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
			want       string
		}{
			{name: "omitted unchanged", effort: "medium", want: "medium"},
			{name: "omitted ignores medium mapping", effort: "medium", mappings: []service.ReasoningEffortMapping{{From: "medium", To: "low"}}, want: "medium"},
			{name: "omitted ignores ceiling", effort: "medium", ceiling: "low", want: "medium"},
			{name: "omitted none mapping does not cap medium default", effort: "medium", ceiling: "low", mappings: []service.ReasoningEffortMapping{{From: "none", To: "low"}}, want: "medium"},
			{name: "disabled omitted applies none only once", preference: `,"thinking":{"type":"disabled"}`, effort: "none", mappings: []service.ReasoningEffortMapping{{From: "none", To: "low"}, {From: "low", To: "high"}}, want: "low"},
			{name: "disabled explicit applies none", preference: `,"output_config":{"effort":"max"},"thinking":{"type":"disabled"}`, effort: "none", mappings: []service.ReasoningEffortMapping{{From: "none", To: "low"}}, want: "low"},
			{name: "explicit adaptive retains policy", preference: `,"output_config":{"effort":"high"},"thinking":{"type":"adaptive"}`, effort: "high", ceiling: "medium", want: "medium"},
			{name: "omitted retains model scope", preference: `,"thinking":{"type":"disabled"}`, effort: "none", mappings: []service.ReasoningEffortMapping{{From: "none", To: "low", Model: "gpt-6-sol", MatchType: "exact"}}, want: "none"},
		} {
			t.Run(platform+"/"+test.name, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
				apiKey := &service.APIKey{Group: &service.Group{Platform: platform, MaxReasoningEffort: test.ceiling, ReasoningEffortMappings: test.mappings}}
				body := []byte(`{"model":"gpt-6.1-sol"` + test.preference + `}`)
				ensureCompositeTargetPlatform(c, apiKey, "gpt-6.1-sol")
				bindOpenAIReasoningEffortPolicyForMessagesRequest(c, apiKey, body)
				converted := []byte(`{"model":"gpt-6.1-sol","reasoning":{"effort":"` + test.effort + `"}}`)
				got, changed, err := service.ApplyOpenAIReasoningEffortPolicyFromContext(c.Request.Context(), converted)
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
