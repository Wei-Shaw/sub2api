package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Shape of a captured Codex Desktop first turn: browser_tabs agent context with
// an id and no call_id, no tool history, no previous_response_id. HTTP
// validation used to reject it with 400 before any account was selected.
func TestValidateFunctionCallOutputRequestAcceptsCodexAgentContext(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	for _, tc := range []struct {
		name, body string
		accepted   bool
	}{
		{name: "browser_tabs first turn", accepted: true, body: `{"model":"gpt-5","store":false,"input":[` +
			`{"type":"message","role":"user","content":[{"type":"input_text","text":"open it"}]},` +
			`{"type":"function_call_output","id":"fco_tabs","namespace":"agent","name":"browser_tabs","output":"<in-app-browser-context>1 tab</in-app-browser-context>"}]}`},
		{name: "any future context name", accepted: true, body: `{"model":"gpt-5","input":[` +
			`{"type":"function_call_output","namespace":"agent","name":"Some_Future_Source_2","output":"ctx"},` +
			`{"type":"function_call_output","namespace":"agent","name":"terminal_info","call_id":"","output":"ctx"}]}`},
		{name: "real tool result without call_id", body: `{"model":"gpt-5","input":[` +
			`{"type":"function_call_output","namespace":"agent","name":"browser_tabs","output":"ctx"},` +
			`{"type":"function_call_output","name":"exec_command","output":"done"}]}`},
		{name: "agent output with unpaired call_id", body: `{"model":"gpt-5","input":[` +
			`{"type":"function_call_output","namespace":"agent","name":"browser_tabs","call_id":"call_x","output":"ctx"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			require.Equal(t, tc.accepted, h.validateFunctionCallOutputRequest(c, []byte(tc.body), zap.NewNop()), w.Body.String())
			if !tc.accepted {
				require.Equal(t, 400, w.Code)
			}
		})
	}
}
