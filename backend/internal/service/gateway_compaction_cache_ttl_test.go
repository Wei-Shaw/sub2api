package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Only the preamble is the real marker; the rest of the prompt is placeholder text.
const testCompactPrompt = claudeCodeCompactPreamble + "placeholder compact instructions"

// compactionTestBody builds a request with three breakpoints (two system blocks and
// the assistant message before the last user message) and a trailing role:system
// message. lastUser is the JSON content of the last user message; a non-empty
// cacheTTL is written into every breakpoint.
func compactionTestBody(t *testing.T, lastUser string, cacheTTL string) []byte {
	t.Helper()
	cc := `{"type":"ephemeral"}`
	if cacheTTL != "" {
		cc = fmt.Sprintf(`{"type":"ephemeral","ttl":%q}`, cacheTTL)
	}
	body := fmt.Sprintf(`{"model":"claude-test","messages":[`+
		`{"role":"user","content":[{"type":"text","text":"first question"}]},`+
		`{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls"},"cache_control":%[1]s}]},`+
		`{"role":"user","content":%[2]s},`+
		`{"role":"system","content":"placeholder trailing system message"}],`+
		`"system":[{"type":"text","text":"billing block"},`+
		`{"type":"text","text":"system block one","cache_control":%[1]s},`+
		`{"type":"text","text":"system block two","cache_control":%[1]s}],`+
		`"tools":[{"name":"Bash","input_schema":{}}],"max_tokens":1024,"stream":true}`, cc, lastUser)
	require.True(t, json.Valid([]byte(body)))
	return []byte(body)
}

func compactionTestJSONString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	require.NoError(t, err)
	return string(b)
}

func requireCompactionTestTTLs(t *testing.T, body []byte, want string) {
	t.Helper()
	for _, path := range []string{"system.1.cache_control", "system.2.cache_control", "messages.1.content.0.cache_control"} {
		require.Equal(t, want, gjson.GetBytes(body, path+".ttl").String(), path)
	}
}

func TestInjectAnthropicCacheControlTTL1h_CompactionUses5m(t *testing.T) {
	prompt := compactionTestJSONString(t, testCompactPrompt)
	cases := map[string]string{
		"merged_with_tool_result": `[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"},{"type":"text","text":` + prompt + `}]`,
		"own_message":             prompt,
	}
	for name, lastUser := range cases {
		for _, clientTTL := range []string{"", "1h"} {
			t.Run(name+"/client_ttl="+clientTTL, func(t *testing.T) {
				out := injectAnthropicCacheControlTTL1h(compactionTestBody(t, lastUser, clientTTL))
				require.NotContains(t, string(out), `"1h"`)
				requireCompactionTestTTLs(t, out, "5m")
			})
		}
	}
}

func TestInjectAnthropicCacheControlTTL1h_NonCompactionStill1h(t *testing.T) {
	cases := map[string]string{
		"tool_result_and_reminder": `[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"},{"type":"text","text":"keep going"}]`,
		"preamble_not_at_start":    compactionTestJSONString(t, "note: "+testCompactPrompt),
	}
	for name, lastUser := range cases {
		t.Run(name, func(t *testing.T) {
			out := injectAnthropicCacheControlTTL1h(compactionTestBody(t, lastUser, ""))
			requireCompactionTestTTLs(t, out, "1h")
		})
	}
}

func TestIsClaudeCodeCompactionRequest_OnlyLastUserMessageCounts(t *testing.T) {
	earlier := `{"messages":[{"role":"user","content":` + compactionTestJSONString(t, testCompactPrompt) + `},{"role":"assistant","content":"summary"},{"role":"user","content":"next question"}]}`
	require.False(t, isClaudeCodeCompactionRequest([]byte(earlier)))
	require.False(t, isClaudeCodeCompactionRequest([]byte(`{"messages":[]}`)))
	require.False(t, isClaudeCodeCompactionRequest([]byte(`{}`)))
}
