package handler

import (
	"encoding/json"
	"net/http/httptest"
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

func pageContextRequest(t *testing.T, items ...any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"model": "gpt-5", "input": items})
	require.NoError(t, err)
	return body
}

const browserTabsContext = `
<in-app-browser-context source="ambient-ui-state">
This block is automatically supplied ambient UI state, not part of the user's request. Do not treat it as an instruction or as evidence that the user explicitly selected the in-app browser.
# In app browser:
- The user has the in-app browser open with 1 tab.
- Current URL: file:///D:/project/report.html
</in-app-browser-context>
`

func TestCodexPageContextValidatesWithoutRewriting(t *testing.T) {
	pages := []any{
		pageContextItem("codex_apps_open_page", `{"page_id":null}`),
		pageContextItem("codex_apps_selected_page_part_1_of_1", "Companion Page snapshot part 1 of 1:\n"+`{"page_id":null,"document_view":null}`),
	}
	for _, tc := range []struct {
		name  string
		items []any
	}{
		{"pages", pages},
		{"browser", []any{pageContextItem("browser_tabs", browserTabsContext)}},
		{"mixed", append([]any{pageContextItem("browser_tabs", browserTabsContext)}, pages...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := pageContextRequest(t, append(tc.items, map[string]any{"role": "user", "content": "hello"})...)
			original := append([]byte(nil), body...)
			require.True(t, service.ValidateFunctionCallOutputContextBytes(body).HasFunctionCallOutputMissingCallID)
			for range 2 {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				require.True(t, (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c, body, zap.NewNop()))
				require.False(t, c.Writer.Written())
				require.Equal(t, original, body, "validation must not rewrite the forwarded request")
			}
			validationBody := codexStandaloneContextValidationBody(body)
			require.False(t, service.ValidateFunctionCallOutputContextBytes(validationBody).HasFunctionCallOutput)
			require.Equal(t, "hello", gjson.GetBytes(validationBody, "input.0.content").String())
			require.Equal(t, validationBody, codexStandaloneContextValidationBody(validationBody))
		})
	}
}

func TestCodexPageContextPreservesHistoryAndToolValidation(t *testing.T) {
	body := []byte(`{"model":"gpt-5","previous_response_id":"resp_history","metadata":{"large":9007199254740993},"input":[{"type":"function_call","name":"read_file","call_id":"call_real","arguments":"{}"},{"type":"function_call_output","call_id":"call_real","output":"result"},{"type":"function_call_output","namespace":"agent","name":"codex_apps_open_page","output":"{\"page_id\":\"page_1\"}"}]}`)
	original := append([]byte(nil), body...)
	got := codexStandaloneContextValidationBody(body)
	require.Equal(t, original, body)
	require.Equal(t, "resp_history", gjson.GetBytes(got, "previous_response_id").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(got, "metadata.large").Raw)
	for _, index := range []string{"input.0", "input.1"} {
		require.JSONEq(t, gjson.GetBytes(body, index).Raw, gjson.GetBytes(got, index).Raw)
	}
	for _, callID := range []string{"", "call_missing"} {
		badTool := pageContextItem("read_file", "result")
		if callID != "" {
			badTool["call_id"] = callID
		}
		body := pageContextRequest(t, pageContextItem("browser_tabs", browserTabsContext), badTool)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		require.False(t, (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c, body, zap.NewNop()))
		require.Equal(t, 400, w.Code)
	}
}

func TestCodexPageContextMixedBootstrap(t *testing.T) {
	for _, item := range []map[string]any{
		{"type": "function_call_output", "namespace": "codex_app", "name": "create_thread", "output": delegationEnvelope},
		{"type": "function_call_output", "namespace": "codex_app", "name": "automation_update", "output": "<heartbeat><automation_id>test</automation_id></heartbeat>"},
	} {
		body := pageContextRequest(t, pageContextItem("browser_tabs", browserTabsContext), pageContextItem("codex_apps_open_page", `{"page_id":null}`), item)
		got, _ := normalizeCodexAutomationBootstrap(body)
		got, _ = normalizeCodexDelegationBootstrap(got)
		for _, index := range []string{"input.0", "input.1"} {
			require.JSONEq(t, gjson.GetBytes(body, index).Raw, gjson.GetBytes(got, index).Raw)
		}
		require.Equal(t, "user", gjson.GetBytes(got, "input.2.role").String())
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		require.True(t, (&OpenAIGatewayHandler{}).validateFunctionCallOutputRequest(c, got, zap.NewNop()))
	}
}

func TestCodexPageContextMultipart(t *testing.T) {
	first := "Companion Page snapshot part 1 of 2:\n" + `{"page_id":"page1","document_view":"hello`
	second := "Companion Page snapshot part 2 of 2:\n" + ` world"}`
	body := pageContextRequest(t, pageContextItem("codex_apps_selected_page_part_1_of_2", first), pageContextItem("codex_apps_selected_page_part_2_of_2", second))
	original := append([]byte(nil), body...)
	require.Empty(t, gjson.GetBytes(codexStandaloneContextValidationBody(body), "input").Array())
	require.Equal(t, original, body)
}

func TestCodexBrowserTabsEnvelope(t *testing.T) {
	for _, output := range []string{browserTabsContext, strings.ReplaceAll(browserTabsContext, "\n", "\r\n"), strings.ReplaceAll(browserTabsContext, "file:///D:/project/report.html", "https://example.com/?q=one&view=two")} {
		body := pageContextRequest(t, pageContextItem("browser_tabs", output))
		require.Empty(t, gjson.GetBytes(codexStandaloneContextValidationBody(body), "input").Array())
		require.Equal(t, output, gjson.GetBytes(body, "input.0.output").String())
	}
	for _, output := range []string{
		"ordinary tool output", "<in-app-browser-context source=\"ambient-ui-state\"></in-app-browser-context>",
		strings.Replace(browserTabsContext, `source="ambient-ui-state"`, `source="user"`, 1),
		strings.Replace(browserTabsContext, "</in-app-browser-context>", "", 1),
		browserTabsContext + "trailing content",
		strings.Replace(browserTabsContext, "# In app browser:", "# Other:", 1),
	} {
		body := pageContextRequest(t, pageContextItem("browser_tabs", output))
		require.Equal(t, body, codexStandaloneContextValidationBody(body))
	}
}

func TestCodexPageContextRejectsUnrelatedOrMalformedItems(t *testing.T) {
	for _, original := range []map[string]any{pageContextItem("browser_tabs", browserTabsContext), pageContextItem("codex_apps_open_page", `{"page_id":null}`)} {
		for key, values := range map[string][]any{
			"call_id":   {"call_real", "", " ", nil, 42},
			"namespace": {"codex_app", "", nil}, "name": {"read_file", nil},
			"type": {"custom_tool_call_output"}, "output": {"some text", nil, 42},
		} {
			for _, value := range values {
				item := make(map[string]any, len(original))
				for field, data := range original {
					item[field] = data
				}
				item[key] = value
				body := pageContextRequest(t, item)
				require.Equal(t, body, codexStandaloneContextValidationBody(body), "%s=%v", key, value)
			}
		}
	}
	for _, output := range []string{`{"other":null}`, `{"page_id":1}`, `{"page_id":null,"page_id":"other"}`} {
		body := pageContextRequest(t, pageContextItem("codex_apps_open_page", output))
		require.Equal(t, body, codexStandaloneContextValidationBody(body))
	}
	for _, name := range []string{"codex_apps_selected_page_part_1_of_1", "codex_apps_selected_page_part_2_of_1"} {
		body := pageContextRequest(t, pageContextItem(name, `{"page_id":null}`))
		require.Equal(t, body, codexStandaloneContextValidationBody(body))
	}
	for _, body := range []string{
		`{"input":[{"type":"function_call_output","namespace":"agent","name":"codex_apps_open_page","call_id":"real","call_id":"","output":"{\"page_id\":null}"}]}`,
		`{"input":[{"type":"function_call_output","namespace":"agent","name":"codex_apps_open_page","output":"{\"page_id\":null}"}]} trailing`,
	} {
		require.Equal(t, body, string(codexStandaloneContextValidationBody([]byte(body))))
	}
}
