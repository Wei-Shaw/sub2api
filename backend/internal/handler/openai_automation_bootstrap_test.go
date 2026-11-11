package handler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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

func TestCodexAutomationContextPreservesNativeInput(t *testing.T) {
	for _, lastRun := range []string{"never", "2026-09-01T02:06:34.536Z (1788228394536)"} {
		output := codexAutomationBootstrap("wiki-maintenance", lastRun, automationBootstrapPrompt)
		for _, value := range []string{output, strings.ReplaceAll(output, "\n", "\r\n")} {
			assertNativeCodexContext(t, codexAutomationBootstrapBody(t, value, ""))
		}
	}
	for _, output := range []string{
		`<heartbeat><automation_id>wiki</automation_id></heartbeat>`,
		`<heartbeat><automation_id>wiki</automation_id><current_time_iso>2026-09-09T00:33:34.775Z</current_time_iso><instructions>Preserve A &amp; B and &lt;tags&gt;.</instructions></heartbeat>`,
	} {
		assertNativeCodexContext(t, codexAutomationBootstrapBody(t, output, ""))
	}
}

func TestCodexAutomationContextRejectsMalformedOutput(t *testing.T) {
	valid := codexAutomationBootstrap("wiki", "never", automationBootstrapPrompt)
	for _, output := range []string{
		strings.Replace(valid, "/wiki/memory.md", "/other/memory.md", 1),
		codexAutomationBootstrap("../wiki", "never", automationBootstrapPrompt),
		codexAutomationBootstrap("wiki", "yesterday", automationBootstrapPrompt),
		codexAutomationBootstrap("wiki", "2026-09-01T02:06:34.536Z (1)", automationBootstrapPrompt),
		strings.Replace(valid, "\n\n"+automationBootstrapPrompt, "\n"+automationBootstrapPrompt, 1),
		codexAutomationBootstrap("wiki", "never", " \n"),
		`<result><automation_id>wiki</automation_id></result>`,
		`<heartbeat status="ok"><automation_id>wiki</automation_id></heartbeat>`,
		`<heartbeat xmlns="urn:codex"><automation_id>wiki</automation_id></heartbeat>`,
		`<heartbeat><automation_id>wiki</automation_id><status>ok</status></heartbeat>`,
		`<heartbeat><automation_id><value>wiki</value></automation_id></heartbeat>`,
		`<heartbeat><automation_id> wiki </automation_id></heartbeat>`,
		`<heartbeat><automation_id>../wiki</automation_id></heartbeat>`,
		`<heartbeat><!-- ok --><automation_id>wiki</automation_id></heartbeat>`,
		`<heartbeat><automation_id>wiki</automation_id></heartbeat>extra`,
	} {
		body := codexAutomationBootstrapBody(t, output, "")
		require.Equal(t, body, codexStandaloneContextValidationBody(body), output)
	}
	const heartbeat = `<heartbeat><automation_id>wiki</automation_id><current_time_iso>2026-09-09T00:33:34.775Z</current_time_iso><instructions>Review the project.</instructions></heartbeat>`
	for _, field := range []string{"automation_id", "current_time_iso", "instructions"} {
		require.False(t, validCodexAutomationHeartbeat(strings.Replace(heartbeat, "</heartbeat>", "<"+field+">duplicate</"+field+"></heartbeat>", 1)))
	}
	for _, output := range []string{
		strings.Replace(heartbeat, "2026-09-09T00:33:34.775Z", "yesterday", 1),
		strings.Replace(heartbeat, "<current_time_iso>2026-09-09T00:33:34.775Z</current_time_iso>", "", 1),
		strings.Replace(heartbeat, "Review the project.", " ", 1),
		strings.Replace(heartbeat, "<instructions>", `<instructions source="other">`, 1),
		strings.Replace(heartbeat, "<instructions>", "<instructions><nested/>", 1),
	} {
		require.False(t, validCodexAutomationHeartbeat(output))
	}
}

func TestCodexBootstrapPrecheck(t *testing.T) {
	output := codexAutomationBootstrap("wiki-maintenance", "never", automationBootstrapPrompt)
	escaped := []byte(strings.Replace(string(codexAutomationBootstrapBody(t, output, "")), `"automation_update"`, `"\u0061utomation_update"`, 1))
	assertNativeCodexContext(t, escaped)
	plain := []byte(`{"model":"gpt-5","input":[{"role":"user","content":"` + strings.Repeat("x", 4096) + `"},{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)
	allocs := testing.AllocsPerRun(10, func() {
		got := codexStandaloneContextValidationBody(plain)
		if &got[0] != &plain[0] {
			t.Fatal("non-candidate body must be returned unchanged")
		}
	})
	require.Zero(t, allocs)
	require.Empty(t, gjson.GetBytes(codexStandaloneContextValidationBody(escaped), "input").Array())
}
