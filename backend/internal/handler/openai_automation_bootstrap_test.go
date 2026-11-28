package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const automationBootstrapPrompt = "Review the project and report any important changes."

func codexAutomationBootstrap(automationID, lastRun, prompt string) string {
	return "Automation: Scheduled project review\n" +
		"Automation ID: " + automationID + "\n" +
		"Automation memory: $CODEX_HOME/automations/" + automationID + "/memory.md\n" +
		"Last run: " + lastRun + "\n\n" + prompt
}

func codexAutomationBootstrapBody(t *testing.T, output, callID string) []byte {
	t.Helper()
	return []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` +
		mustJSON(t, output) + callID + `}]}`)
}

func TestNormalizeCodexAutomationBootstrapSupportedLastRunValues(t *testing.T) {
	tests := []struct {
		name    string
		lastRun string
		crlf    bool
	}{
		{name: "never", lastRun: "never"},
		{name: "timestamp", lastRun: "2026-09-01T02:06:34.536Z (1788228394536)"},
		{name: "crlf", lastRun: "never", crlf: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := codexAutomationBootstrap("wiki-maintenance", tt.lastRun, automationBootstrapPrompt)
			if tt.crlf {
				output = strings.ReplaceAll(output, "\n", "\r\n")
			}
			got, changed := normalizeCodexAutomationBootstrap(codexAutomationBootstrapBody(t, output, ""))
			require.True(t, changed)
			require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
			require.Equal(t, "user", gjson.GetBytes(got, "input.0.role").String())
			require.Equal(t, output, gjson.GetBytes(got, "input.0.content.0.text").String())
			require.False(t, gjson.GetBytes(got, "input.0.call_id").Exists())
		})
	}
}

func TestNormalizeCodexAutomationBootstrapHeartbeat(t *testing.T) {
	output := `<heartbeat><automation_id>wiki</automation_id></heartbeat>`
	got, changed := normalizeCodexAutomationBootstrap(codexAutomationBootstrapBody(t, output, ""))
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
	require.Equal(t, "user", gjson.GetBytes(got, "input.0.role").String())
	require.Equal(t, output, gjson.GetBytes(got, "input.0.content.0.text").String())
	require.False(t, gjson.GetBytes(got, "input.0.call_id").Exists())

	again, changedAgain := normalizeCodexAutomationBootstrap(got)
	require.False(t, changedAgain)
	require.Equal(t, got, again)
}

func TestNormalizeCodexAutomationBootstrapRejectsUnsafeShapes(t *testing.T) {
	validOutput := codexAutomationBootstrap("wiki", "never", automationBootstrapPrompt)
	tests := []struct {
		name string
		body []byte
	}{
		{
			name: "ordinary missing call id",
			body: []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"other","output":` + mustJSON(t, validOutput) + `}]}`),
		},
		{
			name: "tui namespace",
			body: []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_tui","name":"automation_update","output":` + mustJSON(t, validOutput) + `}]}`),
		},
		{
			name: "valid call id",
			body: codexAutomationBootstrapBody(t, validOutput, `,"call_id":"call-1"`),
		},
		{
			name: "ambiguous call context",
			body: []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` + mustJSON(t, validOutput) + `},{"type":"function_call"}]}`),
		},
		{
			name: "ambiguous built-in call context",
			body: []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` + mustJSON(t, validOutput) + `},{"type":"computer_call"}]}`),
		},
		{
			name: "ambiguous tool search call without call id or id",
			body: []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` + mustJSON(t, validOutput) + `},{"type":"tool_search_call"}]}`),
		},
		{
			name: "non-string previous response id",
			body: []byte(`{"model":"gpt-5","previous_response_id":null,"input":[{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` + mustJSON(t, validOutput) + `}]}`),
		},
		{
			name: "mismatched memory id",
			body: codexAutomationBootstrapBody(t, strings.Replace(validOutput, "/wiki/memory.md", "/other/memory.md", 1), ""),
		},
		{
			name: "unsafe automation id",
			body: codexAutomationBootstrapBody(t, codexAutomationBootstrap("../wiki", "never", automationBootstrapPrompt), ""),
		},
		{
			name: "invalid timestamp",
			body: codexAutomationBootstrapBody(t, codexAutomationBootstrap("wiki", "yesterday", automationBootstrapPrompt), ""),
		},
		{
			name: "mismatched timestamp epoch",
			body: codexAutomationBootstrapBody(t, codexAutomationBootstrap("wiki", "2026-09-01T02:06:34.536Z (1)", automationBootstrapPrompt), ""),
		},
		{
			name: "missing separator",
			body: codexAutomationBootstrapBody(t, strings.Replace(validOutput, "\n\n"+automationBootstrapPrompt, "\n"+automationBootstrapPrompt, 1), ""),
		},
		{
			name: "empty prompt",
			body: codexAutomationBootstrapBody(t, codexAutomationBootstrap("wiki", "never", " \n"), ""),
		},
		{
			name: "mixed missing call id output",
			body: []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` + mustJSON(t, validOutput) + `},{"type":"computer_call_output","output":"done"}]}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := normalizeCodexAutomationBootstrap(tt.body)
			require.False(t, changed)
			require.Equal(t, tt.body, got)
		})
	}
}

func TestNormalizeCodexAutomationBootstrapRejectsUnsafeHeartbeatShapes(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{name: "arbitrary tool output", output: `<result><automation_id>wiki</automation_id></result>`},
		{name: "root attribute", output: `<heartbeat status="ok"><automation_id>wiki</automation_id></heartbeat>`},
		{name: "namespaced root", output: `<heartbeat xmlns="urn:codex"><automation_id>wiki</automation_id></heartbeat>`},
		{name: "extra child", output: `<heartbeat><automation_id>wiki</automation_id><status>ok</status></heartbeat>`},
		{name: "nested id content", output: `<heartbeat><automation_id><value>wiki</value></automation_id></heartbeat>`},
		{name: "padded id", output: `<heartbeat><automation_id> wiki </automation_id></heartbeat>`},
		{name: "unsafe id", output: `<heartbeat><automation_id>../wiki</automation_id></heartbeat>`},
		{name: "comment", output: `<heartbeat><!-- ok --><automation_id>wiki</automation_id></heartbeat>`},
		{name: "trailing content", output: `<heartbeat><automation_id>wiki</automation_id></heartbeat>extra`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := codexAutomationBootstrapBody(t, tt.output, "")
			got, changed := normalizeCodexAutomationBootstrap(body)
			require.False(t, changed)
			require.Equal(t, body, got)
		})
	}
}

func TestNormalizeCodexAutomationBootstrapPreservesOrderAndIsIdempotent(t *testing.T) {
	output := codexAutomationBootstrap("wiki", "never", automationBootstrapPrompt)
	body := []byte(`{"model":"gpt-5","input":[{"type":"message","role":"user","content":"before"},{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` + mustJSON(t, output) + `},{"type":"message","role":"user","content":"after"}]}`)

	got, changed := normalizeCodexAutomationBootstrap(body)
	require.True(t, changed)
	require.Equal(t, "before", gjson.GetBytes(got, "input.0.content").String())
	require.Equal(t, output, gjson.GetBytes(got, "input.1.content.0.text").String())
	require.Equal(t, "after", gjson.GetBytes(got, "input.2.content").String())

	again, changedAgain := normalizeCodexAutomationBootstrap(got)
	require.False(t, changedAgain)
	require.Equal(t, got, again)
}

func TestNormalizeCodexAutomationBootstrapFullHeartbeat(t *testing.T) {
	const output = `<heartbeat>
  <automation_id>flutter</automation_id>
  <current_time_iso>2026-09-09T00:33:34.775Z</current_time_iso>
  <instructions>
Read the reference index and report changes. Preserve A &amp; B and &lt;tags&gt;.
  </instructions>
</heartbeat>`
	body := codexAutomationBootstrapBody(t, output, "")
	got, changed := normalizeCodexAutomationBootstrap(body)
	require.True(t, changed)
	require.Equal(t, "user", gjson.GetBytes(got, "input.0.role").String())
	require.Equal(t, output, gjson.GetBytes(got, "input.0.content.0.text").String())
	again, changedAgain := normalizeCodexAutomationBootstrap(got)
	require.False(t, changedAgain)
	require.Equal(t, got, again)

	for _, field := range []string{"automation_id", "current_time_iso", "instructions"} {
		duplicate := strings.Replace(output, "</heartbeat>", "<"+field+">duplicate</"+field+"></heartbeat>", 1)
		require.False(t, validCodexAutomationHeartbeat(duplicate), field)
	}
	for _, invalid := range []string{
		strings.Replace(output, "2026-09-09T00:33:34.775Z", "yesterday", 1),
		strings.Replace(output, "2026-09-09T00:33:34.775Z", " 2026-09-09T00:33:34.775Z ", 1),
		strings.Replace(output, "2026-09-09T00:33:34.775Z", "<v>2026-09-09T00:33:34.775Z</v>", 1),
		strings.Replace(output, "<current_time_iso>2026-09-09T00:33:34.775Z</current_time_iso>", "", 1),
		strings.Replace(output, "Read the reference index and report changes. Preserve A &amp; B and &lt;tags&gt;.", " ", 1),
		strings.Replace(output, "<instructions>", `<instructions source="other">`, 1),
		strings.Replace(output, "</instructions>", "", 1),
		output + "<heartbeat><automation_id>dup</automation_id></heartbeat>",
		`<heartbeat><automation_id>wiki</automation_id><current_time_iso>2026-09-09T00:33:34.775Z</current_time_iso><instructions>a</heartbeat>OUTSIDE<heartbeat><fake>b</fake></instructions></heartbeat>`,
	} {
		require.False(t, validCodexAutomationHeartbeat(invalid), invalid)
	}
	for _, guarded := range [][]byte{
		codexAutomationBootstrapBody(t, output, `,"call_id":"call-1"`),
		[]byte(strings.Replace(string(body), `"namespace":"codex_app"`, `"namespace":"other"`, 1)),
	} {
		unchanged, normalized := normalizeCodexAutomationBootstrap(guarded)
		require.False(t, normalized)
		require.Equal(t, guarded, unchanged)
	}
}

func TestNormalizeCodexAutomationBootstrapHeartbeatLiteralClosingTagAndHistoryVariants(t *testing.T) {
	// 正文内包含字面量 </instructions> 时仍应按外层边界正确识别。
	promptWithClosingTag := "<heartbeat>\n" +
		"  <automation_id>prompt-audit</automation_id>\n" +
		"  <current_time_iso>2026-11-22T14:55:28.776Z</current_time_iso>\n" +
		"  <instructions>\n" +
		"Verify all <instructions>...</instructions> blocks in prompts/ are properly closed.\n" +
		"  </instructions>\n" +
		"</heartbeat>"
	got, changed := normalizeCodexAutomationBootstrap(codexAutomationBootstrapBody(t, promptWithClosingTag, ""))
	require.True(t, changed)
	require.Equal(t, promptWithClosingTag, gjson.GetBytes(got, "input.0.content.0.text").String())

	// Bootstrap（Automation: ...）格式与历史上下文、托管工具（含无 id 的 web_search_call / x_search_call /
	// 带 id 的 tool_search_call）共存时也能正常归一化。
	bootstrapText := codexAutomationBootstrap("wiki", "never", automationBootstrapPrompt)
	withHistory := []byte(`{"model":"gpt-6.1-sol","previous_response_id":"resp_1","input":[` +
		`{"type":"function_call","call_id":"call_1","name":"exec","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"call_1","output":"ok"},` +
		`{"type":"web_search_call","status":"completed"},` +
		`{"type":"x_search_call","id":"xs_1","status":"completed"},` +
		`{"type":"tool_search_call","id":"ts_1","arguments":{"query":"repo"}},` +
		`{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` + mustJSON(t, bootstrapText) + `}` +
		`]}`)
	got, changed = normalizeCodexAutomationBootstrap(withHistory)
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(got, "input.5.type").String())
	require.Equal(t, "user", gjson.GetBytes(got, "input.5.role").String())
	require.Equal(t, bootstrapText, gjson.GetBytes(got, "input.5.content.0.text").String())

	// automation_update 与 delegation（create_thread）同帧共存时不得互相死锁。
	mixedDelegation := []byte(`{"model":"gpt-6.1-sol","input":[` +
		`{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":` + mustJSON(t, delegationEnvelope) + `},` +
		`{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":"<heartbeat><automation_id>wiki</automation_id></heartbeat>"}` +
		`]}`)
	got, changed = normalizeCodexAutomationBootstrap(mixedDelegation)
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(got, "input.1.type").String())
	got, changed = normalizeCodexDelegationBootstrap(got)
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())

	// 未在 untrusted 白名单里的 standalone agent ExternalMessage（如 browser_tabs）
	// 不得阻断同请求中 automation_update 的归一化。
	mixedAgent := []byte(`{"model":"gpt-6.1-sol","input":[` +
		`{"type":"function_call_output","id":"fco_1","namespace":"agent","name":"browser_tabs","output":"<tabs/>"},` +
		`{"type":"function_call_output","id":"fco_2","namespace":"codex_app","name":"automation_update","output":` + mustJSON(t, promptWithClosingTag) + `}` +
		`]}`)
	got, changed = normalizeCodexAutomationBootstrap(mixedAgent)
	require.True(t, changed)
	require.Equal(t, "function_call_output", gjson.GetBytes(got, "input.0.type").String())
	require.Equal(t, "agent", gjson.GetBytes(got, "input.0.namespace").String())
	require.Equal(t, "message", gjson.GetBytes(got, "input.1.type").String())
	require.Equal(t, "user", gjson.GetBytes(got, "input.1.role").String())
}

func TestNormalizeCodexAutomationBootstrapHeartbeatWithUnescapedInstructionsAndHistory(t *testing.T) {
	// 复现 Codex Desktop Windows 定时任务真实载荷：
	// 1. <instructions> 含未 XML 转义的 PowerShell `&`、`<OrderNo>`、`<单号>` 与 CRLF；
	// 2. 长对话复用同一线程，包含历史配对的 function_call / function_call_output、
	//    item_reference、web_search_call（无 call_id 仅有 id）与多轮历史 heartbeat。
	heartbeat1 := "<heartbeat>\r\n" +
		"  <automation_id>automation</automation_id>\r\n" +
		"  <current_time_iso>2026-11-22T14:55:28.776Z</current_time_iso>\r\n" +
		"  <instructions>\r\n" +
		"每轮仅运行一次：\n" +
		"& 'C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe' -File 'Run-Monitor.ps1'\n" +
		"再定位slt<OrderNo>所在行，证据 runs/verified_<单号>.png，支持 <nested/> 标签。\r\n" +
		"  </instructions>\r\n" +
		"</heartbeat>\r\n"
	heartbeat2 := "<heartbeat>\r\n" +
		"  <automation_id>automation</automation_id>\r\n" +
		"  <current_time_iso>2026-11-23T04:58:10.100Z</current_time_iso>\r\n" +
		"  <instructions>\r\n" +
		"第二轮定时检查：a < b && c > d\r\n" +
		"  </instructions>\r\n" +
		"</heartbeat>\r\n"

	body := []byte(`{` +
		`"model":"gpt-6.1-sol",` +
		`"previous_response_id":"resp_hist_1",` +
		`"metadata":{"large":9007199254740993},` +
		`"input":[` +
		`{"type":"message","role":"user","content":"initial"},` +
		`{"type":"function_call","call_id":"call_1","name":"exec","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"call_1","output":"ok"},` +
		`{"type":"web_search_call","id":"ws_1","status":"completed"},` +
		`{"type":"item_reference","id":"item_1"},` +
		`{"type":"function_call_output","id":"fco_1","namespace":"codex_app","name":"automation_update","output":` + mustJSON(t, heartbeat1) + `},` +
		`{"type":"function_call_output","id":"fco_2","namespace":"codex_app","name":"automation_update","output":` + mustJSON(t, heartbeat2) + `},` +
		`{"type":"message","role":"user","content":"？？"}` +
		`]}`)

	got, changed := normalizeCodexAutomationBootstrap(body)
	require.True(t, changed)
	require.Equal(t, "resp_hist_1", gjson.GetBytes(got, "previous_response_id").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(got, "metadata.large").Raw)
	// 历史调用上下文保持原样
	require.Equal(t, "call_1", gjson.GetBytes(got, "input.1.call_id").String())
	require.Equal(t, "call_1", gjson.GetBytes(got, "input.2.call_id").String())
	require.Equal(t, "ws_1", gjson.GetBytes(got, "input.3.id").String())
	require.Equal(t, "item_1", gjson.GetBytes(got, "input.4.id").String())
	// 两轮 heartbeat 均被转为标准 user message，内容原样保留
	require.Equal(t, "message", gjson.GetBytes(got, "input.5.type").String())
	require.Equal(t, "user", gjson.GetBytes(got, "input.5.role").String())
	require.Equal(t, heartbeat1, gjson.GetBytes(got, "input.5.content.0.text").String())
	require.False(t, gjson.GetBytes(got, "input.5.namespace").Exists())
	require.Equal(t, "message", gjson.GetBytes(got, "input.6.type").String())
	require.Equal(t, "user", gjson.GetBytes(got, "input.6.role").String())
	require.Equal(t, heartbeat2, gjson.GetBytes(got, "input.6.content.0.text").String())
	require.Equal(t, "？？", gjson.GetBytes(got, "input.7.content").String())

	// 幂等
	again, changedAgain := normalizeCodexAutomationBootstrap(got)
	require.False(t, changedAgain)
	require.Equal(t, got, again)

	// 首轮无历史上下文时也能通过 HTTP call_id 校验
	firstTurn := codexAutomationBootstrapBody(t, heartbeat1, "")
	firstTurnNormalized, firstChanged := normalizeCodexAutomationBootstrap(firstTurn)
	require.True(t, firstChanged)
	h := &OpenAIGatewayHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	require.True(t, h.validateFunctionCallOutputRequest(c, firstTurnNormalized, zap.NewNop()))
	require.False(t, c.Writer.Written())
}

// The byte precheck must stay a necessary condition: an escaped type or tool
// name is still a candidate, and a body without either is returned untouched
// without the token scan and decode.
func TestNormalizeCodexBootstrapPrecheck(t *testing.T) {
	output := codexAutomationBootstrap("wiki-maintenance", "never", automationBootstrapPrompt)
	escaped := []byte(strings.Replace(string(codexAutomationBootstrapBody(t, output, "")), `"automation_update"`, `"\u0061utomation_update"`, 1))
	got, changed := normalizeCodexAutomationBootstrap(escaped)
	require.True(t, changed)
	require.Equal(t, output, gjson.GetBytes(got, "input.0.content.0.text").String())

	plain := []byte(`{"model":"gpt-5","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"` + strings.Repeat("x", 4096) + `"}]},{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)
	for name, normalize := range map[string]func([]byte) ([]byte, bool){
		"automation": normalizeCodexAutomationBootstrap,
		"delegation": normalizeCodexDelegationBootstrap,
	} {
		t.Run(name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(10, func() {
				got, changed := normalize(plain)
				if changed || &got[0] != &plain[0] {
					t.Fatal("non-candidate body must be returned unchanged")
				}
			})
			require.Zero(t, allocs)
		})
	}
}
