package keyprotection

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

const fictionalKey = "ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const fictionalToken = "keyx_49e7237e11464693589bca95fe317f2fde5793bb7d70da9749f55641a5fff406"

func TestDeterministicSHA256Placeholders(t *testing.T) {
	first := testState(t, DefaultConfig())
	second := testState(t, DefaultConfig())
	a, err := first.ProtectText(fictionalKey)
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.ProtectText(fictionalKey)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(fictionalKey))
	if a != b || a != fmt.Sprintf("keyx_%x", digest) || a != fictionalToken {
		t.Fatal("placeholder is not stable SHA256")
	}
	empty := testState(t, DefaultConfig())
	if empty.RestoreText(a) != a {
		t.Fatal("hash alone granted restoration access")
	}
}

func TestStateDiagnosticsExcludeSensitiveValues(t *testing.T) {
	state := testState(t, DefaultConfig(), fictionalKey)
	for _, message := range []string{fmt.Sprint(state), fmt.Sprintf("%+v", state), fmt.Sprintf("%#v", state), fmt.Sprintf("%+v", *state), fmt.Sprintf("%#v", *state)} {
		if strings.Contains(message, fictionalKey) || strings.Contains(message, fictionalToken) {
			t.Fatal("diagnostic exposed sensitive state")
		}
	}
	encoded, err := json.Marshal(state)
	if err != nil || string(encoded) != "{}" {
		t.Fatal("state unexpectedly serializes sensitive fields")
	}
}

func TestMappingByteLimits(t *testing.T) {
	config := DefaultConfig()
	config.CustomRules = []Rule{{Name: "large_fixture", Pattern: `fixture_[A-Z]+`}}
	state := testState(t, config)
	if _, err := state.ProtectText("fixture_" + strings.Repeat("A", MaxSecretBytes)); !errors.Is(err, ErrCapacity) {
		t.Fatal("single secret byte limit ignored")
	}
	for i := range 8 {
		_, err := state.ProtectText("fixture_" + strings.Repeat(string(rune('A'+i)), MaxSecretBytes-len("fixture_")))
		if i < 7 && err != nil {
			t.Fatal(err)
		}
		if i == 7 && !errors.Is(err, ErrCapacity) {
			t.Fatal("input admission bypassed aggregate mapping limit")
		}
	}
}

func testState(t *testing.T, config Config, secrets ...string) *State {
	t.Helper()
	state, err := NewState(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range secrets {
		token, err := state.ProtectText(secret)
		if err != nil {
			t.Fatal(err)
		}
		if token == secret {
			t.Fatal("fixture did not create a mapping")
		}
	}
	return state
}

func TestProtectTextRepeatAndIsolation(t *testing.T) {
	state := testState(t, DefaultConfig())
	input := "Use " + fictionalKey + " then " + fictionalKey + "."
	protected, err := state.ProtectText(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(protected, fictionalKey) || len(state.entries) != 1 || state.Counts()["github"] != 2 {
		t.Fatalf("replacement/count mismatch")
	}
	var token string
	for token = range state.entries {
	}
	if !IsPlaceholderToken(token) || strings.Count(protected, token) != 2 {
		t.Fatal("invalid or non-reused placeholder")
	}
	if state.RestoreText(protected) != input {
		t.Fatal("round trip failed")
	}
	other := testState(t, DefaultConfig(), "ghp_"+strings.Repeat("B", 36))
	if other.RestoreText(protected) != protected {
		t.Fatal("restored another request's token")
	}

}

func TestRestoreOnlyCompleteAuthorizedPlaceholders(t *testing.T) {
	state := testState(t, DefaultConfig(), fictionalKey)
	unknown := "keyx_" + strings.Repeat("f", 64)
	for _, input := range []string{unknown, fictionalToken[:36], fictionalToken + "a", fictionalToken + "-", fictionalToken + "_", "x" + fictionalToken, strings.ToUpper(fictionalToken)} {
		if state.RestoreText(input) != input {
			t.Errorf("modified incomplete, invented, or embedded token: %q", input)
		}
	}
	if got := state.RestoreText("(“" + fictionalToken + "”), " + fictionalToken + "."); got != "(“"+fictionalKey+"”), "+fictionalKey+"." {
		t.Fatal("punctuation boundary failed")
	}
}

func TestConservativeDetection(t *testing.T) {
	state := testState(t, DefaultConfig())
	for _, input := range []string{"sketch", "sk-example", "sk-" + strings.Repeat("A", 47), "https://example.invalid/repo", "c6a7cbe2-411b-4ea7-97b8-63fdfb5f3c37", strings.Repeat("X", 100), "x" + fictionalKey, fictionalKey + "x"} {
		got, err := state.ProtectText(input)
		if err != nil {
			t.Fatal(err)
		}
		if got != input {
			t.Errorf("false positive: %q", input)
		}
	}
	for _, input := range []string{fictionalKey, "sk-proj-" + strings.Repeat("A", 40), "sk-ant-api03-" + strings.Repeat("B", 90), "AIza" + strings.Repeat("C", 35), "sk_test_" + strings.Repeat("D", 24), "glpat-" + strings.Repeat("E", 20)} {
		got, err := state.ProtectText(input)
		if err != nil {
			t.Fatal(err)
		}
		if got == input {
			t.Errorf("supported fixture not found")
		}
	}
}

func TestMappingCapacity(t *testing.T) {
	state := testState(t, DefaultConfig())
	for i := range MaxMappings {
		if _, err := state.ProtectText(fmt.Sprintf("ghp_%036d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := state.ProtectText(fictionalKey); !errors.Is(err, ErrCapacity) {
		t.Fatalf("expected capacity failure, got %v", err)
	}
	if _, err := state.ProtectText(fmt.Sprintf("ghp_%036d", 0)); err != nil {
		t.Fatal("repeated key consumed capacity")
	}
}

func TestConfigScopeAndSnapshot(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.UserIDs = []int64{7}
	config.GroupIDs = []int64{9}
	config.Rules = []string{"github"}
	if !config.Applies(7, 1) || !config.Applies(8, 9) || config.Applies(8, 1) || config.Applies(0, 9) {
		t.Fatal("scope mismatch")
	}
	state := testState(t, config)
	config.Rules[0] = "google"
	got, err := state.ProtectText(fictionalKey)
	if err != nil || got != fictionalToken {
		t.Fatal("active request rules changed")
	}
	copy := config.Normalized()
	copy.UserIDs[0] = 99
	if config.UserIDs[0] != 7 {
		t.Fatal("normalized config aliases input")
	}
	config.Enabled = false
	if config.Applies(7, 9) {
		t.Fatal("disabled config applied")
	}
	for _, config := range []Config{{Rules: []string{"bad"}}, {UserIDs: []int64{-1}}, {CustomRules: []Rule{{Name: "sample", Pattern: "("}}}, {CustomRules: []Rule{{Name: "sample", Pattern: ".*"}}}} {
		if config.Validate() == nil {
			t.Errorf("invalid config accepted: %+v", config)
		}
	}
}

func TestCustomRules(t *testing.T) {
	config := DefaultConfig()
	config.CustomRules = []Rule{{Name: "fictional", Pattern: `example_secret_[A-Z]{12}`}}
	state := testState(t, config)
	got, err := state.ProtectText("example_secret_ABCDEFGHIJKL")
	if err != nil {
		t.Fatal(err)
	}
	if !IsPlaceholderToken(got) || state.Counts()["fictional"] != 1 {
		t.Fatal("custom rule failed")
	}

}

func TestCustomRuleCannotRemapReservedPlaceholders(t *testing.T) {
	config := DefaultConfig()
	config.CustomRules = []Rule{{Name: "reserved_fixture", Pattern: `keyx_[a-f0-9]{64}`}}
	state := testState(t, config)
	got, err := state.ProtectText(fictionalToken)
	if err != nil {
		t.Fatal(err)
	}
	if got != fictionalToken || len(state.entries) != 0 {
		t.Fatal("custom rule remapped reserved placeholder")
	}
}

func TestRestoreArgumentsJSONEscapingAndPrecision(t *testing.T) {
	secret := "fictional\"quote\\slash\nline"
	state := testState(t, Config{CustomRules: []Rule{{Name: "quoted_fixture", Pattern: regexp.QuoteMeta(secret)}}}, secret)
	fictionalToken := fmt.Sprintf("keyx_%x", sha256.Sum256([]byte(secret)))
	got, err := state.RestoreArguments(`{"key":"` + fictionalToken + `","id":900719925474099312345,"nested":["` + fictionalToken + `"]}`)
	if err != nil {
		t.Fatal(err)
	}
	value, err := DecodeObject([]byte(got))
	if err != nil {
		t.Fatal(err)
	}
	if value["key"] != secret || string(value["id"].(json.Number)) != "900719925474099312345" {
		t.Fatal("inner JSON restore corrupted values")
	}
	if _, err := state.RestoreArguments(`{"key":"` + fictionalToken); err == nil {
		t.Fatal("incomplete tool arguments accepted")
	}
}

func TestProtectProtocolBodies(t *testing.T) {
	cases := []struct{ protocol, body, response string }{
		{"chat", `{"model":"test","messages":[{"role":"system","content":"` + fictionalKey + `"},{"role":"user","content":[{"type":"text","text":"` + fictionalKey + `"}]},{"role":"assistant","tool_calls":[{"id":"call_1","function":{"name":"lookup","arguments":"{\"key\":\"` + fictionalKey + `\",\"number\":900719925474099312345}"}}]},{"role":"tool","tool_call_id":"call_1","content":"` + fictionalKey + `"}],"tools":[{"type":"function","function":{"name":"lookup","description":"Use ` + fictionalKey + `","parameters":{"type":"object"}}}],"seed":900719925474099312345}`, `{"choices":[{"message":{"role":"assistant","content":"TOKEN","tool_calls":[{"id":"call_1","function":{"name":"lookup","arguments":"{\"key\":\"TOKEN\"}"}}]}}],"usage":{"total_tokens":37}}`},
		{"responses", `{"model":"test","instructions":"` + fictionalKey + `","input":[{"role":"user","content":[{"type":"input_text","text":"` + fictionalKey + `"}]},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"key\":\"` + fictionalKey + `\"}"},{"type":"function_call_output","call_id":"call_1","output":"` + fictionalKey + `"}],"seed":900719925474099312345}`, `{"id":"resp_1","output":[{"type":"message","content":[{"type":"output_text","text":"TOKEN"}]},{"type":"function_call","name":"lookup","arguments":"{\"key\":\"TOKEN\"}"}],"usage":{"total_tokens":37}}`},
		{"messages", `{"model":"test","system":[{"type":"text","text":"` + fictionalKey + `"}],"messages":[{"role":"user","content":"` + fictionalKey + `"},{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"lookup","input":{"name":"` + fictionalKey + `","n":900719925474099312345}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":"` + fictionalKey + `"}]}]}],"seed":900719925474099312345}`, `{"type":"message","content":[{"type":"text","text":"TOKEN"},{"type":"tool_use","name":"lookup","input":{"key":"TOKEN"}}],"usage":{"output_tokens":37}}`},
	}
	for _, tc := range cases {
		t.Run(tc.protocol, func(t *testing.T) {
			state := testState(t, DefaultConfig())
			protected, err := state.ProtectJSON([]byte(tc.body), tc.protocol)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(protected), fictionalKey) || len(state.entries) != 1 {
				t.Fatal("model-facing body contains fixture or mapping mismatch")
			}
			root, err := DecodeObject(protected)
			if err != nil {
				t.Fatal(err)
			}
			if string(root["seed"].(json.Number)) != "900719925474099312345" {
				t.Fatal("numeric precision changed")
			}
			var token string
			for token = range state.entries {
			}
			response := strings.ReplaceAll(tc.response, "TOKEN", token)
			restored, err := state.RestoreJSON([]byte(response), tc.protocol)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(restored), token) || !strings.Contains(string(restored), fictionalKey) || !strings.Contains(string(restored), `37`) {
				t.Fatal("client restore/usage mismatch")
			}
		})
	}
}

func TestRestoreContentAndPreserveNonTargetFields(t *testing.T) {
	config := DefaultConfig()
	state := testState(t, config, fictionalKey)
	response := `{"id":"` + fictionalToken + `","metadata":{"secret":"` + fictionalToken + `"},"choices":[{"message":{"content":"` + fictionalToken + `","tool_calls":[{"id":"` + fictionalToken + `","function":{"name":"` + fictionalToken + `","arguments":"{\"key\":\"` + fictionalToken + `\"}"}}]}}]}`
	got, err := state.RestoreJSON([]byte(response), "chat")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), fictionalKey) != 2 || strings.Count(string(got), fictionalToken) != 4 {
		t.Fatal("restoration escaped content and tool argument fields")
	}
	body := `{"model":"` + fictionalKey + `","metadata":{"key":"` + fictionalKey + `"},"messages":[{"role":"user","name":"` + fictionalKey + `","content":"` + fictionalKey + `"}]}`
	protected, err := state.ProtectJSON([]byte(body), "chat")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(protected), fictionalKey) != 3 {
		t.Fatal("protocol identifiers or metadata were modified")
	}
}

func TestUnsupportedAndMalformedFailClosed(t *testing.T) {
	cases := []struct{ protocol, body string }{
		{"gemini", `{}`}, {"chat", `{"messages":`}, {"chat", `{"messages":[]} {}`},
		{"chat", `{"messages":[],"messages":[]}`},
		{"chat", `{"messages":[],"messa\u0067es":[]}`},
		{"chat", `{"messages":[{"role":"user","content":"a","content":"b"}]}`},
		{"responses", `{"input":"ok","previous_response_id":"resp_1","previous_response_id":"resp_2"}`},
		{"responses", `{"input":"ok","background":true}`},
		{"responses", `{"input":"ok","previous_response_id":"resp_1"}`},
		{"responses", `{"input":"ok","Previous_Response_ID":"resp_1"}`},
		{"responses", `{"input":"ok","Conversation":"conv_1"}`},
		{"responses", `{"input":"ok","Background":true}`},
		{"responses", `{"input":"ok","conversation":"conv_1"}`},
		{"responses", `{"input":[{"type":"reasoning","encrypted_content":"opaque"}]}`},
		{"responses", `{"input":[{"type":"item_reference","id":"msg_1"}]}`},
		{"messages", `{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"ok","signature":"signed"}]}]}`},
		{"messages", `{"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","data":"opaque"}]}]}`},
		{"chat", `{"messages":[{"role":"user","content":[{"type":"new_unknown_type","text":"` + fictionalKey + `"}]}]}`},
		{"chat", `{"messages":[],"extra_body":{"new_content":"` + fictionalKey + `"}}`},
		{"chat", `{"messages":[],"input":"` + fictionalKey + `"}`},
		{"chat", `{"messages":[{"role":"user","Content":"` + fictionalKey + `"}]}`},
		{"messages", `{"messages":[{"role":"user","content":[{"type":"text","Text":"` + fictionalKey + `"}]}]}`},
		{"chat", `{"messages":[{"role":"assistant","tool_calls":[{"function":{"name":"lookup","arguments":"{bad"}}]}]}`},
	}
	for _, tc := range cases {
		state := testState(t, DefaultConfig())
		got, err := state.ProtectJSON([]byte(tc.body), tc.protocol)
		if err == nil || got != nil {
			t.Errorf("unsupported/malformed input accepted for %s", tc.protocol)
		}
		if err != nil && strings.Contains(err.Error(), fictionalKey) {
			t.Fatal("error contains credential")
		}
	}
}

func TestSchemaPropertiesAndAdditionalToolsProtected(t *testing.T) {
	state := testState(t, DefaultConfig())
	body := `{"input":[{"type":"message","role":"user","content":"hello","additional_tools":[{"type":"function","name":"lookup","description":"` + fictionalKey + `","parameters":{"type":"object","properties":{"name":{"type":"string","default":"` + fictionalKey + `"},"type":{"type":"string","description":"` + fictionalKey + `"}}}}]}]}`
	protected, err := state.ProtectJSON([]byte(body), "responses")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(protected), fictionalKey) || state.Counts()["github"] != 3 {
		t.Fatal("additional tools or property schemas escaped transformation")
	}
}

func TestRestorationPreservesThinkingAndOpaqueSignatures(t *testing.T) {
	state := testState(t, DefaultConfig(), fictionalKey)
	for _, block := range []string{
		`{"type":"thinking","thinking":"` + fictionalToken + `","signature":"signed"}`,
		`{"type":"thinking","thinking":"` + fictionalToken + `"}`,
		`{"type":"redacted_thinking","data":"` + fictionalToken + `"}`,
		`{"type":"text","text":"` + fictionalToken + `","signature":"signed"}`,
	} {
		out, err := state.RestoreJSON([]byte(`{"content":[`+block+`]}`), "messages")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), fictionalKey) || !strings.Contains(string(out), fictionalToken) {
			t.Fatal("signed/opaque reasoning response was rewritten")
		}
	}
	body := `{"output":[{"type":"reasoning","encrypted_content":"` + fictionalToken + `","summary":[{"type":"summary_text","text":"` + fictionalToken + `"}]}]}`
	out, err := state.RestoreJSON([]byte(body), "responses")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(out), fictionalToken) != 1 || strings.Count(string(out), fictionalKey) != 1 {
		t.Fatal("visible summary and encrypted payload were not handled separately")
	}
}

func TestOpaqueMediaAndTextDocuments(t *testing.T) {
	state := testState(t, DefaultConfig())
	body := `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","data":"` + fictionalKey + `"}},{"type":"document","title":"` + fictionalKey + `","source":{"type":"text","data":"` + fictionalKey + `"}}]}]}`
	got, err := state.ProtectJSON([]byte(body), "messages")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), fictionalKey) != 1 {
		t.Fatal("opaque media changed or document text leaked")
	}
}

func TestMultiRoundFullHistoryRebuildsMapping(t *testing.T) {
	first := testState(t, DefaultConfig())
	protected, err := first.ProtectJSON([]byte(`{"input":"`+fictionalKey+`"}`), "responses")
	if err != nil {
		t.Fatal(err)
	}
	var token string
	for token = range first.entries {
	}
	second := testState(t, DefaultConfig())
	secondProtected, err := second.ProtectJSON([]byte(`{"input":"echo `+token+` and `+fictionalKey+`"}`), "responses")
	if err != nil {
		t.Fatal(err)
	}
	if len(second.entries) != 1 || strings.Contains(string(secondProtected), fictionalKey) || !strings.Contains(string(protected), token) {
		t.Fatal("history echo did not reuse mapping")
	}
	unrelated := testState(t, DefaultConfig())
	if unrelated.RestoreText(token) != token {
		t.Fatal("token without an input key guessed")
	}
}

func TestCapacityNeverReturnsPartialProtectedBody(t *testing.T) {
	state := testState(t, DefaultConfig())
	var keys []string
	for i := 0; i <= MaxMappings; i++ {
		keys = append(keys, fmt.Sprintf("ghp_%036d", i))
	}
	body, _ := json.Marshal(map[string]string{"input": strings.Join(keys, " ")})
	got, err := state.ProtectJSON(body, "responses")
	if !errors.Is(err, ErrCapacity) || got != nil {
		t.Fatal("capacity failure exposed a partial body")
	}
}

func ExampleState_ProtectJSON() {
	const secret = "ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	state, err := NewState(DefaultConfig())
	if err != nil {
		panic(err)
	}
	input := `{"messages":[{"role":"user","content":"Use ` + secret + `"}]}`
	protected, err := state.ProtectJSON([]byte(input), "chat")
	if err != nil {
		panic(err)
	}
	upstream, err := DecodeObject(protected)
	if err != nil {
		panic(err)
	}
	upstreamText := upstream["messages"].([]any)[0].(map[string]any)["content"].(string)
	token := strings.TrimPrefix(upstreamText, "Use ")
	response := `{"choices":[{"message":{"content":"Use ` + token + `","tool_calls":[{"function":{"name":"lookup","arguments":"{\"key\":\"` + token + `\"}"}}]}}]}`
	restored, err := state.RestoreJSON([]byte(response), "chat")
	if err != nil {
		panic(err)
	}
	client, err := DecodeObject(restored)
	if err != nil {
		panic(err)
	}
	message := client["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	arguments := message["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)["arguments"].(string)
	fmt.Println("input: Use " + secret)
	fmt.Println("upstream: " + upstreamText)
	fmt.Println("client text: " + message["content"].(string))
	fmt.Println("client tool arguments: " + arguments)
	// Output:
	// input: Use ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
	// upstream: Use keyx_49e7237e11464693589bca95fe317f2fde5793bb7d70da9749f55641a5fff406
	// client text: Use ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
	// client tool arguments: {"key":"ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
}
