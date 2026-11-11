package handler

import (
	"maps"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func TestCodexSourceContextAllowlist(t *testing.T) {
	for _, tc := range []struct{ namespace, name, output string }{
		{"agent", "goal_objective", "Finish the task"},
		{"agent", "proposed_plan", "# Approved plan\n1. Implement the change"},
		{"agent", "untrusted_content", "External context"},
		{"guardian", "guardian_housekeeper_input", `{"sources":[]}`},
		{"guardian", "guardian_channel_selection_input", `{"channels":[]}`},
		{"guardian", "guardian_original_evidence", `{"sources":[]}`},
		{"cloud_threads", "send_message", delegationEnvelope},
		{"cloud_threads", "send_message", `<codex_delegation><source_thread_id>thread-1</source_thread_id><source_orbit_id>orbit-1</source_orbit_id><input>do the work</input></codex_delegation>`},
	} {
		t.Run(tc.namespace+"/"+tc.name, func(t *testing.T) {
			item := pageContextItem(tc.name, tc.output)
			item["namespace"] = tc.namespace
			body := pageContextRequest(t, item, map[string]any{"role": "user", "content": "hello"})
			original := append([]byte(nil), body...)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			require.True(t, (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c, body, zap.NewNop()))
			require.False(t, c.Writer.Written())
			require.Equal(t, original, body)
			require.Len(t, gjson.GetBytes(codexStandaloneContextValidationBody(body), "input").Array(), 1)
			for key, values := range map[string][]any{
				"namespace": {"unknown", nil}, "name": {"read_file", tc.name + "_extra"},
				"call_id": {"", nil, 123, "call_missing"}, "output": {nil, 123, []any{}},
			} {
				for _, value := range values {
					bad := maps.Clone(item)
					bad[key] = value
					badBody := pageContextRequest(t, bad)
					require.Equal(t, badBody, codexStandaloneContextValidationBody(badBody), "%s=%v", key, value)
				}
			}
			// A recognized context must not hide a genuine orphan result beside it.
			c, _ = gin.CreateTestContext(httptest.NewRecorder())
			require.False(t, (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c,
				pageContextRequest(t, item, pageContextItem("read_file", "unpaired result")), zap.NewNop()))
		})
	}
}

func TestCodexSourceContextRejectsMalformedEnvelopes(t *testing.T) {
	for _, tc := range []struct{ namespace, name, output string }{
		{"agent", "goal_objective", " "},
		{"guardian", "guardian_original_evidence", "not JSON"},
		{"guardian", "guardian_original_evidence", `{"sources":[],"sources":null}`},
		{"cloud_threads", "send_message", "ordinary result"},
		{"cloud_threads", "send_message", `<codex_delegation><source_thread_id>thread-1</source_thread_id><source_orbit_id></source_orbit_id><input>work</input></codex_delegation>`},
		{"cloud_threads", "send_message", `<codex_delegation><source_thread_id>thread-1</source_thread_id><source_orbit_id>one</source_orbit_id><source_orbit_id>two</source_orbit_id><input>work</input></codex_delegation>`},
		{"cloud_threads", "send_message", `<codex_delegation><source_orbit_id>one</source_orbit_id><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>`},
	} {
		item := pageContextItem(tc.name, tc.output)
		item["namespace"] = tc.namespace
		body := pageContextRequest(t, item)
		require.Equal(t, body, codexStandaloneContextValidationBody(body))
	}
}
