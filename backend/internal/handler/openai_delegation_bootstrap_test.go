package handler

import (
	"encoding/json"
	"maps"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const delegationEnvelope = `<codex_delegation><source_thread_id>thread-1</source_thread_id><input>do the work</input></codex_delegation>`

func assertNativeCodexContext(t *testing.T, body []byte) {
	t.Helper()
	original := append([]byte(nil), body...)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.True(t, (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c, body, zap.NewNop()))
	require.False(t, c.Writer.Written())
	require.Empty(t, gjson.GetBytes(codexStandaloneContextValidationBody(body), "input").Array())
	require.Equal(t, original, body)
}

func codexBootstrapContextItems() []map[string]any {
	items := []map[string]any{{"type": "function_call_output", "id": "fco_automation", "namespace": "codex_app", "name": "automation_update", "output": `<heartbeat><automation_id>wiki</automation_id></heartbeat>`}}
	for _, namespace := range []string{"codex_app", "codex_tui"} {
		for _, name := range []string{"create_thread", "send_message_to_thread"} {
			items = append(items, map[string]any{"type": "function_call_output", "id": "fco_delivery", "namespace": namespace, "name": name, "output": delegationEnvelope})
		}
	}
	return items
}

func TestCodexBootstrapContextPreservesNativeItemsAndHistory(t *testing.T) {
	for _, item := range codexBootstrapContextItems() {
		assertNativeCodexContext(t, pageContextRequest(t, item))
		body := pageContextRequest(t,
			map[string]any{"role": "user", "content": "before"}, item,
			map[string]any{"type": "function_call", "name": "read_file", "call_id": "call_real", "arguments": "{}"},
			map[string]any{"type": "function_call_output", "call_id": "call_real", "output": "result"},
			map[string]any{"role": "user", "content": "after"})
		// Preserve opaque fields and exact JSON numbers in the validation copy.
		body = append([]byte(`{"previous_response_id":"resp_history","metadata":{"large":9007199254740993},`), body[1:]...)
		original := append([]byte(nil), body...)
		validation := codexStandaloneContextValidationBody(body)
		require.Equal(t, "9007199254740993", gjson.GetBytes(validation, "metadata.large").Raw)
		require.Equal(t, "resp_history", gjson.GetBytes(validation, "previous_response_id").String())
		for i, originalIndex := range []int{0, 2, 3, 4} {
			require.JSONEq(t, gjson.GetBytes(body, "input").Array()[originalIndex].Raw, gjson.GetBytes(validation, "input").Array()[i].Raw)
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		require.True(t, (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c, body, zap.NewNop()))
		require.Equal(t, original, body)
	}
}

func TestCodexBootstrapContextRetainsToolValidation(t *testing.T) {
	for _, item := range codexBootstrapContextItems() {
		for field, values := range map[string][]any{
			"call_id":   {"", " ", "call_real", nil, 42},
			"namespace": {"other", " codex_app "}, "name": {"unknown", " create_thread "},
			"type": {" function_call_output ", "custom_tool_call_output"}, "output": {"not an envelope", nil, 42},
		} {
			for _, value := range values {
				bad := maps.Clone(item)
				bad[field] = value
				body := pageContextRequest(t, bad)
				require.Equal(t, body, codexStandaloneContextValidationBody(body), "%s=%v", field, value)
			}
		}
		for _, typ := range []string{"function_call_output", "custom_tool_call_output", "mcp_tool_call_output", "tool_search_output"} {
			body := pageContextRequest(t, item, map[string]any{"type": typ, "name": "read_file", "output": "unpaired"})
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			require.False(t, (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c, body, zap.NewNop()), typ)
			require.Equal(t, 400, c.Writer.Status())
		}
		// Built-in outputs outside this validator's remit must still reach upstream validation.
		body := pageContextRequest(t, item, map[string]any{"type": "computer_call_output", "output": "unpaired"})
		require.JSONEq(t, gjson.GetBytes(body, "input.1").Raw,
			gjson.GetBytes(codexStandaloneContextValidationBody(body), "input.0").Raw)
	}
	tuiAutomation := maps.Clone(codexBootstrapContextItems()[0])
	tuiAutomation["namespace"] = "codex_tui"
	body := pageContextRequest(t, tuiAutomation)
	require.Equal(t, body, codexStandaloneContextValidationBody(body))
}

func TestCodexBootstrapContextRejectsDuplicateMembers(t *testing.T) {
	for _, body := range []string{
		`{"previous_response_id":"","previous_response_id":"resp-1","input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"` + delegationEnvelope + `"}]}`,
		`{"input":[{"type":"message","type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"` + delegationEnvelope + `"}]}`,
		`{"input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","call_id":"call-1","call_id":"","output":"` + delegationEnvelope + `"}]}`,
		`{"input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"not a delegation","output":"` + delegationEnvelope + `"}]}`,
	} {
		require.Equal(t, body, string(codexStandaloneContextValidationBody([]byte(body))))
	}
}

func TestCodexDelegationContextRequiresCompleteEnvelope(t *testing.T) {
	for _, envelope := range []string{
		`<codex_delegation><source_thread_id>thread-1</source_thread_id></codex_delegation>`,
		`<codex_delegation><source_thread_id></source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input> </input></codex_delegation>`,
		`prefix<codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>suffix`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input><extra>x</extra></codex_delegation>`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input><nested>work</nested></input></codex_delegation>`,
		`<x:codex_delegation xmlns:x="urn:test"><source_thread_id>thread-1</source_thread_id><input>work</input></x:codex_delegation>`,
		`<x:codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input></x:codex_delegation>`,
		`<codex_delegation xmlns="urn:test"><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation xmlns:x="urn:test"><x:source_thread_id>thread-1</x:source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation><x:source_thread_id>thread-1</x:source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation xmlns=""><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>`,
	} {
		body := []byte(`{"input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":` + mustJSON(t, envelope) + `}]}`)
		require.Equal(t, body, codexStandaloneContextValidationBody(body), envelope)
	}
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}
