package service

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpsRequestParameters(t *testing.T) {
	for _, tc := range []struct {
		name, body, effort, tier string
	}{
		{"responses", `{"reasoning":{"effort":"xhigh"},"service_tier":"priority"}`, "xhigh", "priority"},
		{"chat", `{"reasoning_effort":"none","service_tier":"default"}`, "none", "default"},
		{"messages", `{"output_config":{"effort":"max"}}`, "max", ""},
		{"unknown values", `{"reasoning":{"effort":"Not-Allowed"},"service_tier":"turbo"}`, "Not-Allowed", "turbo"},
		{"whitespace", `{"reasoning_effort":" high ","service_tier":" fast "}`, "high", "fast"},
		{"non string", `{"reasoning":{"effort":true},"service_tier":{"nested":"priority"}}`, "", ""},
		{"missing", `{"model":"gpt-5-high"}`, "", ""},
		{"malformed", `{"reasoning_effort":"high",`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			SetOpsRequestParameters(c, []byte(tc.body))
			effort, tier := OpsRequestParameters(c)
			require.Equal(t, tc.effort, effort)
			require.Equal(t, tc.tier, tier)
		})
	}
}

func TestOpsRequestParametersBoundedAndReset(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	SetOpsRequestParameters(c, []byte(`{"reasoning_effort":"`+strings.Repeat("强", 300)+`","service_tier":"priority"}`))
	effort, _ := OpsRequestParameters(c)
	require.Len(t, []rune(effort), 256)
	SetOpsRequestParameters(c, []byte(`{}`))
	effort, tier := OpsRequestParameters(c)
	require.Empty(t, effort)
	require.Empty(t, tier)
}

func TestOpsStreamErrorSnapshotsRequestParametersPerTurn(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	SetOpenAIClientTransport(c, OpenAIClientTransportWS)
	BeginOpsStreamTurn(c, 1)
	SetOpsRequestParameters(c, []byte(`{"reasoning":{"effort":"xhigh"},"service_tier":"priority"}`))
	MarkOpsStreamFailure(c, "upstream_error", "reasoning_effort_not_allowed", "invalid effort", 403)
	BeginOpsStreamTurn(c, 2)
	SetOpsRequestParameters(c, []byte(`{"reasoning_effort":"low","service_tier":"flex"}`))
	MarkOpsStreamErrorValue(c, OpsStreamError{RequestScoped: true, ErrType: "invalid_request_error", Message: "invalid tier", IntendedStatus: 400})
	errors := GetOpsStreamErrors(c)
	require.Len(t, errors, 2)
	require.Equal(t, "xhigh", errors[0].ReasoningEffort)
	require.Equal(t, "priority", errors[0].ServiceTier)
	require.Equal(t, "low", errors[1].ReasoningEffort)
	require.Equal(t, "flex", errors[1].ServiceTier)
}
