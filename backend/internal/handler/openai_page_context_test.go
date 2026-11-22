package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func pageContextItem(name, output string) map[string]any {
	return map[string]any{"type": "function_call_output", "id": "fco_page", "namespace": "agent", "name": name, "output": output}
}

// A real tool result that lost its call_id; Codex reserves the agent namespace for context.
func toolResultWithoutCallID(name, output string) map[string]any {
	return map[string]any{"type": "function_call_output", "id": "fco_tool", "name": name, "output": output}
}

func pageContextRequest(t *testing.T, items ...any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"model": "gpt-5", "input": items})
	require.NoError(t, err)
	return body
}

func TestNormalizeCodexPageContextFirstMessage(t *testing.T) {
	open := `{"page_id":null}`
	selected := "Companion Page snapshot part 1 of 1:\n" + `{"page_id":null,"document_view":null}`
	body := pageContextRequest(t, pageContextItem("codex_apps_open_page", open), pageContextItem("codex_apps_selected_page_part_1_of_1", selected), map[string]any{"role": "user", "content": "hello"})
	h := &OpenAIGatewayHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	require.False(t, h.validateFunctionCallOutputRequest(c, body, zap.NewNop()))
	require.Equal(t, 400, w.Code)
	require.Contains(t, w.Body.String(), "function_call_output requires call_id")

	got, changed := normalizeCodexUntrustedContext(body)
	require.True(t, changed)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	require.True(t, h.validateFunctionCallOutputRequest(c, got, zap.NewNop()))
	require.False(t, c.Writer.Written())
	for i, original := range []string{open, selected} {
		item := gjson.GetBytes(got, "input").Array()[i]
		require.Equal(t, "message", item.Get("type").String())
		require.Equal(t, "user", item.Get("role").String())
		require.Contains(t, item.Get("content.0.text").String(), original)
		require.False(t, item.Get("call_id").Exists())
		require.False(t, item.Get("id").Exists())
	}
	require.Equal(t, "hello", gjson.GetBytes(got, "input.2.content").String())
	again, changed := normalizeCodexUntrustedContext(got)
	require.False(t, changed)
	require.Equal(t, got, again)
}

var codexTextContextNamesForTest = []string{"browser_info", "terminal_info", "goal_objective", "untrusted_context", "untrusted_content", "ide_context", "proposed_plan"}

func TestNormalizeCodexTerminalInfoIssue8991(t *testing.T) {
	body := []byte(`{"model":"deepseek-flash","input":[{"type":"function_call_output","namespace":"agent","name":"terminal_info","output":"retest context"}]}`)
	h := &OpenAIGatewayHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	require.False(t, h.validateFunctionCallOutputRequest(c, body, zap.NewNop()))
	require.Equal(t, 400, w.Code)
	require.Contains(t, w.Body.String(), "function_call_output requires call_id")

	got, changed := normalizeCodexUntrustedContext(body)
	require.True(t, changed)
	require.JSONEq(t, `{"model":"deepseek-flash","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"<external_terminal_info>retest context</external_terminal_info>"}]}]}`, string(got))
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	require.True(t, h.validateFunctionCallOutputRequest(c, got, zap.NewNop()))
	require.False(t, c.Writer.Written())
}

func TestNormalizeCodexTerminalInfoBoundaries(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		key   string
		value any
	}{
		{"call id with surrounding whitespace", "call_id", " call_real "},
		{"boolean call id", "call_id", false},
		{"array call id", "call_id", []any{}},
		{"object call id", "call_id", map[string]any{}},
		{"namespace whitespace", "namespace", " agent "},
		{"namespace case", "namespace", "Agent"},
		{"boolean output", "output", false},
		{"numeric output", "output", 42},
		{"object output", "output", map[string]any{"text": "context"}},
		{"name with dash", "name", "terminal-info"},
		{"name with space", "name", "terminal info"},
		{"empty name", "name", ""},
		{"other type", "type", "custom_tool_call_output"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			item := pageContextItem("terminal_info", "retest context")
			item[testCase.key] = testCase.value
			body := pageContextRequest(t, item)
			got, changed := normalizeCodexUntrustedContext(body)
			require.False(t, changed)
			require.Equal(t, body, got)
		})
	}
	for _, body := range []string{
		`{"input":[{"type":"function_call_output","namespace":"other","namespace":"agent","name":"terminal_info","output":"context"}]}`,
		`{"input":[{"type":"function_call_output","namespace":"agent","name":"read_file","name":"terminal_info","output":"context"}]}`,
		`{"input":[],"input":[{"type":"function_call_output","namespace":"agent","name":"terminal_info","output":"context"}]}`,
		`{"input":[{"type":"function_call_output","namespace":"agent","name":"terminal_info","output":"context"}]} trailing`,
	} {
		got, changed := normalizeCodexUntrustedContext([]byte(body))
		require.False(t, changed)
		require.Equal(t, body, string(got))
	}
}

func TestNormalizeCodexTerminalInfoEscapedName(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call_output","namespace":"agent","name":"terminal_\u0069nfo","output":"  context\n\t"}]}`)
	got, changed := normalizeCodexUntrustedContext(body)
	require.True(t, changed)
	require.Equal(t, "<external_terminal_info>  context\n\t</external_terminal_info>", gjson.GetBytes(got, "input.0.content.0.text").String())
}

func TestNormalizeCodexTerminalInfoPreservesUnrelatedResults(t *testing.T) {
	body := pageContextRequest(t,
		pageContextItem("terminal_info", "retest context"),
		map[string]any{"type": "function_call_output", "namespace": "agent", "name": "terminal_info", "call_id": "call_real", "output": "real result"},
		toolResultWithoutCallID("read_file", "unrelated result"))
	got, changed := normalizeCodexUntrustedContext(body)
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
	for _, path := range []string{"input.1", "input.2"} {
		require.JSONEq(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(got, path).Raw)
	}
	h := &OpenAIGatewayHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	require.False(t, h.validateFunctionCallOutputRequest(c, got, zap.NewNop()))
	require.Equal(t, 400, w.Code)
	require.Contains(t, w.Body.String(), "function_call_output requires call_id")
}

// Codex Desktop browser_tabs (Sub2API #8999) and any future additional context
// key reach HTTP as agent outputs without call_id; none may be rejected.
func TestNormalizeCodexStandaloneAgentContext(t *testing.T) {
	tabs := "\n<in-app-browser-context source=\"ambient-ui-state\">\n# In app browser\n- Example: https://example.com/\n</in-app-browser-context>"
	body := []byte(`{"model":"gpt-5","tools":[{"type":"function","name":"exec_command","parameters":{}}],"input":[` +
		`{"type":"message","role":"user","content":"open it"},` +
		`{"type":"function_call","name":"exec_command","call_id":"call_real","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"call_real","output":"done"},` +
		`{"type":"function_call_output","id":"fco_tabs","namespace":"agent","name":"browser_tabs","output":` + strconv.Quote(tabs) + `},` +
		`{"type":"function_call_output","namespace":"agent","name":"Some_Future_Source_2","output":"future context"},` +
		`{"type":"message","role":"user","content":"continue"}]}`)
	// Without paired tool history the HTTP preflight rejects the first turn.
	first := pageContextRequest(t, pageContextItem("browser_tabs", tabs), map[string]any{"role": "user", "content": "open it"})
	h := &OpenAIGatewayHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	require.False(t, h.validateFunctionCallOutputRequest(c, first, zap.NewNop()))
	require.Equal(t, 400, w.Code)
	require.Contains(t, w.Body.String(), "function_call_output requires call_id")
	normalizedFirst, changed := normalizeCodexUntrustedContext(first)
	require.True(t, changed)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	require.True(t, h.validateFunctionCallOutputRequest(c, normalizedFirst, zap.NewNop()), w.Body.String())
	require.False(t, c.Writer.Written())

	got, changed := normalizeCodexUntrustedContext(body)
	require.True(t, changed)
	require.False(t, service.ValidateFunctionCallOutputContextBytes(got).HasFunctionCallOutputMissingCallID)
	for _, index := range []string{"tools", "input.0", "input.1", "input.2", "input.5"} {
		require.JSONEq(t, gjson.GetBytes(body, index).Raw, gjson.GetBytes(got, index).Raw)
	}
	require.Equal(t, "user", gjson.GetBytes(got, "input.3.role").String())
	require.False(t, gjson.GetBytes(got, "input.3.id").Exists())
	require.Equal(t, "<external_browser_tabs>"+tabs+"</external_browser_tabs>", gjson.GetBytes(got, "input.3.content.0.text").String())
	require.Equal(t, "<external_Some_Future_Source_2>future context</external_Some_Future_Source_2>", gjson.GetBytes(got, "input.4.content.0.text").String())
	again, changed := normalizeCodexUntrustedContext(got)
	require.False(t, changed)
	require.Equal(t, got, again)
}

func TestNormalizeCodexStandaloneAgentContextNameBoundaries(t *testing.T) {
	for name, want := range map[string]bool{
		strings.Repeat("a", 128): true,
		"UPPER_lower_09":         true,
		strings.Repeat("a", 129): false,
		"has-dash":               false,
		"has space":              false,
		"has.dot":                false,
		"tag>":                   false,
		"":                       false,
	} {
		body := pageContextRequest(t, pageContextItem(name, "context"))
		got, changed := normalizeCodexUntrustedContext(body)
		require.Equal(t, want, changed, "name=%q", name)
		if !want {
			require.Equal(t, body, got)
		}
	}
	for _, output := range []string{"", " \n\t"} {
		body := pageContextRequest(t, pageContextItem("browser_tabs", output))
		_, changed := normalizeCodexUntrustedContext(body)
		require.False(t, changed)
	}
}

// The cheap probe must let every standalone agent output through and skip the rest.
func TestNormalizeCodexStandaloneAgentContextProbe(t *testing.T) {
	for body, want := range map[string]bool{
		`{"input":[{"type":"function_call_output","namespace":"agent","name":"x","output":"c"}]}`:                     true,
		`{"input":[{"type":"function_call_output","namespace":"agent","name":"x","call_id":"","output":"c"}]}`:        true,
		`{"input":[{"type":"function_call_output","namespace":"agent","name":"x","call_id":"call_1","output":"c"}]}`:  false,
		`{"input":[{"type":"function_call_output","name":"agent","output":"c"}]}`:                                     false,
		`{"input":[{"type":"function_call_output","call_id":"call_1","output":"agent"}]}`:                             false,
		`{"tools":[{"name":"function_call_output","namespace":"agent"}],"input":[{"role":"user","content":"agent"}]}`: false,
		`{"input":[{"type":"function_call_output","namespace":"\u0061gent","name":"x","output":"c"}]}`:                true,
	} {
		got, changed := normalizeCodexUntrustedContext([]byte(body))
		require.Equal(t, want, changed, body)
		if !want {
			require.Equal(t, body, string(got))
		}
	}
}

func TestNormalizeCodexAdditionalUntrustedContext(t *testing.T) {
	for _, name := range codexTextContextNamesForTest {
		t.Run(name, func(t *testing.T) {
			output := "external context\nline two"
			if name == "ide_context" {
				output = "# Context from my IDE setup:\n\n## Active file: main.go\n\n## Active selection of the file:\nfunc main() {}"
			}
			contextItem := map[string]any{"type": "function_call_output", "namespace": "agent", "name": name, "output": output}
			body := pageContextRequest(t, contextItem,
				map[string]any{"role": "user", "content": "continue"})
			h := &OpenAIGatewayHandler{}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			require.False(t, h.validateFunctionCallOutputRequest(c, body, zap.NewNop()))
			require.Equal(t, 400, w.Code)
			require.Contains(t, w.Body.String(), "function_call_output requires call_id")

			got, changed := normalizeCodexUntrustedContext(body)
			require.True(t, changed)
			w = httptest.NewRecorder()
			c, _ = gin.CreateTestContext(w)
			require.True(t, h.validateFunctionCallOutputRequest(c, got, zap.NewNop()))
			require.False(t, c.Writer.Written())
			item := gjson.GetBytes(got, "input.0")
			require.Equal(t, "message", item.Get("type").String())
			require.Equal(t, "user", item.Get("role").String())
			require.Equal(t, "<external_"+name+">"+output+"</external_"+name+">", item.Get("content.0.text").String())
			require.False(t, item.Get("call_id").Exists())
			require.Equal(t, "continue", gjson.GetBytes(got, "input.1.content").String())
			again, changed := normalizeCodexUntrustedContext(got)
			require.False(t, changed)
			require.Equal(t, got, again)
		})
	}
}

func TestNormalizeCodexAdditionalUntrustedContextPreservesRealAndMalformedOutputs(t *testing.T) {
	for _, name := range codexTextContextNamesForTest {
		for _, override := range []map[string]any{
			{"call_id": "call_real"},
			{"call_id": nil},
			{"call_id": 42},
			{"namespace": "other"},
			{"namespace": nil},
			{"namespace": 42},
			{"output": []any{map[string]any{"type": "encrypted_content", "encrypted_content": "sealed"}}},
			{"output": nil},
		} {
			item := pageContextItem(name, "context")
			for key, value := range override {
				item[key] = value
			}
			body := pageContextRequest(t, item)
			got, changed := normalizeCodexUntrustedContext(body)
			require.False(t, changed, "name=%s override=%v", name, override)
			require.Equal(t, body, got)
		}
		for _, field := range []string{"namespace", "output"} {
			item := pageContextItem(name, "context")
			delete(item, field)
			body := pageContextRequest(t, item)
			got, changed := normalizeCodexUntrustedContext(body)
			require.False(t, changed)
			require.Equal(t, body, got)
		}
	}
	// Recognized external context cannot make an unrelated tool result valid.
	body := pageContextRequest(t, pageContextItem("browser_info", "tab"),
		toolResultWithoutCallID("read_file", "result"))
	got, changed := normalizeCodexUntrustedContext(body)
	require.True(t, changed)
	h := &OpenAIGatewayHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	require.False(t, h.validateFunctionCallOutputRequest(c, got, zap.NewNop()))
	require.Equal(t, 400, w.Code)

	// Keep historical calls, prior response and exact numeric metadata intact.
	body = []byte(`{"model":"gpt-5","previous_response_id":"resp_history","metadata":{"large":9007199254740993},"input":[{"type":"function_call","name":"read_file","call_id":"call_real","arguments":"{}"},{"type":"function_call_output","call_id":"call_real","output":"result"},{"type":"function_call_output","namespace":"agent","name":"goal_objective","output":"finish"}]}`)
	got, changed = normalizeCodexUntrustedContext(body)
	require.True(t, changed)
	require.Equal(t, "resp_history", gjson.GetBytes(got, "previous_response_id").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(got, "metadata.large").Raw)
	for _, index := range []string{"input.0", "input.1"} {
		require.JSONEq(t, gjson.GetBytes(body, index).Raw, gjson.GetBytes(got, index).Raw)
	}
}

func TestNormalizeCodexAdditionalUntrustedContextBlankCallIDsAndMixedInput(t *testing.T) {
	names := codexTextContextNamesForTest
	for _, callID := range []string{"", " \t\n"} {
		items := make([]any, 0, len(names))
		for _, name := range names {
			item := pageContextItem(name, "context for "+name)
			item["call_id"] = callID
			delete(item, "id")
			items = append(items, item)
		}
		got, changed := normalizeCodexUntrustedContext(pageContextRequest(t, items...))
		require.True(t, changed)
		require.Len(t, gjson.GetBytes(got, "input").Array(), len(names))
		for i, item := range gjson.GetBytes(got, "input").Array() {
			require.Equal(t, "user", item.Get("role").String())
			require.Equal(t, "<external_"+names[i]+">context for "+names[i]+"</external_"+names[i]+">", item.Get("content.0.text").String())
		}
	}
}

func TestNormalizeCodexAdditionalUntrustedContextRejectsDuplicateMembers(t *testing.T) {
	for _, name := range codexTextContextNamesForTest {
		for _, body := range []string{
			`{"input":[{"type":"function_call_output","namespace":"agent","name":"` + name + `","call_id":"real","call_id":"","output":"context"}]}`,
			`{"input":[{"type":"function_call_output","namespace":"agent","name":"` + name + `","output":"first","output":"second"}]}`,
		} {
			got, changed := normalizeCodexUntrustedContext([]byte(body))
			require.False(t, changed)
			require.Equal(t, body, string(got))
		}
	}
}

func TestNormalizeCodexProposedPlanChunks(t *testing.T) {
	chunks := []string{"# Plan\n", " \n\t", "1. Run tests."}
	var items []any
	for _, chunk := range chunks {
		items = append(items, pageContextItem("proposed_plan", chunk))
	}
	got, changed := normalizeCodexUntrustedContext(pageContextRequest(t, items...))
	require.True(t, changed)
	messages := gjson.GetBytes(got, "input").Array()
	require.Len(t, messages, len(chunks))
	for i, message := range messages {
		require.Equal(t, "user", message.Get("role").String())
		require.Equal(t, "<external_proposed_plan>"+chunks[i]+"</external_proposed_plan>", message.Get("content.0.text").String())
	}
}

func TestNormalizeCodexAdditionalUntrustedContextBlankOutput(t *testing.T) {
	for _, name := range codexTextContextNamesForTest {
		for _, output := range []string{"", " \t"} {
			body := pageContextRequest(t, pageContextItem(name, output))
			got, changed := normalizeCodexUntrustedContext(body)
			wantChange := name == "untrusted_content" || (name == "proposed_plan" && output != "")
			require.Equal(t, wantChange, changed, "name=%s output=%q", name, output)
			if wantChange {
				require.Equal(t, "<external_"+name+">"+output+"</external_"+name+">", gjson.GetBytes(got, "input.0.content.0.text").String())
			} else {
				require.Equal(t, body, got)
			}
		}
	}
}

func TestNormalizeCodexPageContextPreservesHistoryAndToolValidation(t *testing.T) {
	body := []byte(`{"model":"gpt-5","previous_response_id":"resp_history","metadata":{"large":9007199254740993},"input":[{"type":"function_call","name":"read_file","call_id":"call_real","arguments":"{}"},{"type":"function_call_output","call_id":"call_real","output":"result"},{"type":"function_call_output","namespace":"agent","name":"codex_apps_open_page","output":"{\"page_id\":\"page_1\"}"}]}`)
	got, changed := normalizeCodexUntrustedContext(body)
	require.True(t, changed)
	require.Equal(t, "resp_history", gjson.GetBytes(got, "previous_response_id").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(got, "metadata.large").Raw)
	for _, index := range []string{"input.0", "input.1"} {
		require.JSONEq(t, gjson.GetBytes(body, index).Raw, gjson.GetBytes(got, index).Raw)
	}

	badTool := toolResultWithoutCallID("read_file", "result")
	body = pageContextRequest(t, pageContextItem("codex_apps_open_page", `{"page_id":null}`), badTool)
	got, changed = normalizeCodexUntrustedContext(body)
	require.True(t, changed)
	require.True(t, service.ValidateFunctionCallOutputContextBytes(got).HasFunctionCallOutputMissingCallID)
}

func TestNormalizeCodexPageContextMixedBootstrap(t *testing.T) {
	for _, item := range []map[string]any{
		{"type": "function_call_output", "namespace": "codex_app", "name": "create_thread", "output": delegationEnvelope},
		{"type": "function_call_output", "namespace": "codex_app", "name": "automation_update", "output": "<heartbeat><automation_id>test</automation_id></heartbeat>"},
	} {
		body := pageContextRequest(t, pageContextItem("codex_apps_open_page", `{"page_id":null}`), item)
		got, changed := normalizeCodexUntrustedContext(body)
		require.True(t, changed)
		got, _ = normalizeCodexAutomationBootstrap(got)
		got, _ = normalizeCodexDelegationBootstrap(got)
		require.False(t, service.ValidateFunctionCallOutputContextBytes(got).HasFunctionCallOutput)
	}
}

func TestNormalizeCodexPageContextMultipart(t *testing.T) {
	first := "Companion Page snapshot part 1 of 2:\n" + `{"page_id":"page1","document_view":"hello`
	second := "Companion Page snapshot part 2 of 2:\n" + ` world"}`
	body := pageContextRequest(t, pageContextItem("codex_apps_selected_page_part_1_of_2", first), pageContextItem("codex_apps_selected_page_part_2_of_2", second))
	got, changed := normalizeCodexUntrustedContext(body)
	require.True(t, changed)
	require.Contains(t, gjson.GetBytes(got, "input.0.content.0.text").String(), first)
	require.Contains(t, gjson.GetBytes(got, "input.1.content.0.text").String(), second)
}

func TestNormalizeCodexPageContextRejectsUnrelatedOrMalformedItems(t *testing.T) {
	for _, tc := range []struct {
		name  string
		key   string
		value any
	}{
		{"real call", "call_id", "call_real"},
		{"invalid call id", "call_id", 42},
		{"null call id", "call_id", nil},
		{"other namespace", "namespace", "codex_app"},
		{"missing namespace", "namespace", ""},
		{"other type", "type", "custom_tool_call_output"},
		{"not JSON", "output", "some text"},
		{"not snapshot", "output", `{"other":null}`},
		{"wrong page id", "output", `{"page_id":1}`},
		{"duplicate snapshot key", "output", `{"page_id":null,"page_id":"other"}`},
		{"part without header", "name", "codex_apps_selected_page_part_1_of_1"},
		{"part out of range", "name", "codex_apps_selected_page_part_2_of_1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := pageContextItem("codex_apps_open_page", `{"page_id":null}`)
			item[tc.key] = tc.value
			body := pageContextRequest(t, item)
			got, changed := normalizeCodexUntrustedContext(body)
			require.False(t, changed)
			require.Equal(t, body, got)
		})
	}
	for _, body := range []string{
		`{"input":[{"type":"function_call_output","namespace":"agent","name":"codex_apps_open_page","call_id":"real","call_id":"","output":"{\"page_id\":null}"}]}`,
		`{"input":[{"type":"function_call_output","namespace":"agent","name":"codex_apps_open_page","output":"{\"page_id\":null}"}]} trailing`,
	} {
		got, changed := normalizeCodexUntrustedContext([]byte(body))
		require.False(t, changed)
		require.Equal(t, body, string(got))
	}
}
