package keyprotection

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const fictionalKey = "ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const fictionalToken = "keyx_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestDeterministicScopedHMACPlaceholders(t *testing.T) {
	seed := bytes.Repeat([]byte{0x42}, 32)
	first, err := NewStateWithSeed(DefaultConfig(), nil, seed)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewStateWithSeed(DefaultConfig(), nil, seed)
	if err != nil {
		t.Fatal(err)
	}
	a, err := first.ProtectText(fictionalKey)
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.ProtectText(fictionalKey)
	if err != nil {
		t.Fatal(err)
	}
	digest := hmac.New(sha256.New, seed)
	_, _ = digest.Write([]byte(fictionalKey))
	if a != b || a != "keyx_"+hex.EncodeToString(digest.Sum(nil)) || len(a) != PlaceholderLength {
		t.Fatal("scoped digest is not stable HMAC-SHA256")
	}
	seed[0]++
	other, err := NewStateWithSeed(DefaultConfig(), nil, seed)
	if err != nil {
		t.Fatal(err)
	}
	c, err := other.ProtectText(fictionalKey)
	if err != nil {
		t.Fatal(err)
	}
	if c == a {
		t.Fatal("different scope seeds produced same token")
	}
	if _, err := NewStateWithSeed(DefaultConfig(), nil, []byte("short")); err == nil {
		t.Fatal("weak seed accepted")
	}
	if first.RestoreText(c) != c {
		t.Fatal("digest from another scope gained restoration access")
	}
}

func TestStateDiagnosticsExcludeSensitiveValues(t *testing.T) {
	seed := bytes.Repeat([]byte{0x42}, 32)
	state, err := NewStateWithSeed(DefaultConfig(), map[string]string{fictionalToken: fictionalKey}, seed)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{fmt.Sprint(state), fmt.Sprintf("%+v", state), fmt.Sprintf("%#v", state), fmt.Sprintf("%+v", *state), fmt.Sprintf("%#v", *state)} {
		if strings.Contains(message, fictionalKey) || strings.Contains(message, fictionalToken) || strings.Contains(message, "BBBB") {
			t.Fatal("diagnostic exposed sensitive state")
		}
	}
	encoded, err := json.Marshal(state)
	if err != nil || string(encoded) != "{}" {
		t.Fatal("state unexpectedly serializes sensitive fields")
	}
}

func TestMappingByteLimits(t *testing.T) {
	entries := make(map[string]string)
	for i := range 8 {
		entries["keyx_"+fmt.Sprintf("%064x", i)] = strings.Repeat(string(rune('A'+i)), MaxSecretBytes)
	}
	if _, err := NewState(DefaultConfig(), entries); !errors.Is(err, ErrCapacity) {
		t.Fatal("aggregate mapping byte limit ignored")
	}
	config := DefaultConfig()
	config.CustomRules = []Rule{{Name: "large_fixture", Pattern: `fixture_[A-Z]+`}}
	state := testState(t, config, nil)
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

func testState(t *testing.T, config Config, entries map[string]string) *State {
	t.Helper()
	state, err := NewState(config, entries)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestProtectTextRepeatAndIsolation(t *testing.T) {
	state := testState(t, DefaultConfig(), nil)
	input := "Use " + fictionalKey + " then " + fictionalKey + "."
	protected, err := state.ProtectText(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(protected, fictionalKey) || len(state.Entries()) != 1 || state.Counts()["github"] != 2 {
		t.Fatalf("replacement/count mismatch")
	}
	var token string
	for token = range state.Entries() {
	}
	if !IsPlaceholderToken(token) || strings.Count(protected, token) != 2 {
		t.Fatal("invalid or non-reused placeholder")
	}
	if state.RestoreText(protected) != input {
		t.Fatal("round trip failed")
	}
	other := testState(t, DefaultConfig(), nil)
	otherProtected, err := other.ProtectText(input)
	if err != nil {
		t.Fatal(err)
	}
	if protected == otherProtected {
		t.Fatal("independent states reused placeholder")
	}
	if other.RestoreText(protected) != protected {
		t.Fatal("restored another state's token")
	}
	clone := state.Entries()
	clone[token] = "changed"
	if state.RestoreText(token) != fictionalKey {
		t.Fatal("entries are externally mutable")
	}
}

func TestRestoreOnlyCompleteAuthorizedPlaceholders(t *testing.T) {
	state := testState(t, DefaultConfig(), map[string]string{fictionalToken: fictionalKey})
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
	state := testState(t, DefaultConfig(), nil)
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

func TestRedactAndCapacity(t *testing.T) {
	config := DefaultConfig()
	config.Mode = ModeRedact
	state := testState(t, config, nil)
	got, err := state.ProtectText(fictionalKey)
	if err != nil {
		t.Fatal(err)
	}
	if got != RedactedMarker || len(state.Entries()) != 0 {
		t.Fatal("redact unexpectedly established mapping")
	}
	config.Mode = ModeReversible
	config.MaxMappings = 1
	state = testState(t, config, nil)
	if _, err := state.ProtectText(fictionalKey); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ProtectText("ghp_" + strings.Repeat("B", 36)); !errors.Is(err, ErrCapacity) {
		t.Fatalf("expected capacity failure, got %v", err)
	}
	if _, err := state.ProtectText(fictionalKey); err != nil {
		t.Fatal("repeated key should not consume capacity")
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
	state := testState(t, config, nil)
	config.Rules[0] = "google"
	copy := state.Config()
	copy.Rules[0] = "google"
	if state.Config().Rules[0] != "github" {
		t.Fatal("request config is mutable")
	}
	config.Enabled = false
	if config.Applies(7, 9) {
		t.Fatal("disabled config applied")
	}
	for _, config := range []Config{{Mode: "bad"}, {RestoreScope: "bad"}, {TTLSeconds: -1}, {MaxMappings: 10001}, {MaxSessions: -1}, {Rules: []string{"bad"}}, {UserIDs: []int64{-1}}, {CustomRules: []Rule{{Name: "sample", Pattern: "("}}}, {CustomRules: []Rule{{Name: "sample", Pattern: ".*"}}}} {
		if config.Validate() == nil {
			t.Errorf("invalid config accepted: %+v", config)
		}
	}
}

func TestCustomRulesAndInvalidMappings(t *testing.T) {
	config := DefaultConfig()
	config.CustomRules = []Rule{{Name: "fictional", Pattern: `example_secret_[A-Z]{12}`}}
	state := testState(t, config, nil)
	got, err := state.ProtectText("example_secret_ABCDEFGHIJKL")
	if err != nil {
		t.Fatal(err)
	}
	if !IsPlaceholderToken(got) || state.Counts()["fictional"] != 1 {
		t.Fatal("custom rule failed")
	}
	for _, entries := range []map[string]string{{"bad": fictionalKey}, {fictionalToken: ""}, {fictionalToken: fictionalToken}, {fictionalToken: fictionalKey, "keyx_" + strings.Repeat("f", 64): fictionalKey}} {
		if _, err := NewState(DefaultConfig(), entries); err == nil {
			t.Fatal("invalid mapping accepted")
		}
	}
}

func TestCustomRuleCannotRemapReservedPlaceholders(t *testing.T) {
	config := DefaultConfig()
	config.CustomRules = []Rule{{Name: "reserved_fixture", Pattern: `keyx_[a-f0-9]{64}`}}
	state := testState(t, config, nil)
	got, err := state.ProtectText(fictionalToken)
	if err != nil {
		t.Fatal(err)
	}
	if got != fictionalToken || len(state.Entries()) != 0 {
		t.Fatal("custom rule remapped reserved placeholder")
	}
}

func TestRestoreArgumentsJSONEscapingAndPrecision(t *testing.T) {
	secret := "fictional\"quote\\slash\nline"
	state := testState(t, DefaultConfig(), map[string]string{fictionalToken: secret})
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
			state := testState(t, DefaultConfig(), nil)
			protected, err := state.ProtectJSON([]byte(tc.body), tc.protocol)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(protected), fictionalKey) || len(state.Entries()) != 1 {
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
			for token = range state.Entries() {
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

func TestRestoreScopeAndNonTargetFields(t *testing.T) {
	config := DefaultConfig()
	config.RestoreScope = ScopeToolsOnly
	state := testState(t, config, map[string]string{fictionalToken: fictionalKey})
	response := `{"id":"` + fictionalToken + `","metadata":{"secret":"` + fictionalToken + `"},"choices":[{"message":{"content":"` + fictionalToken + `","tool_calls":[{"id":"` + fictionalToken + `","function":{"name":"` + fictionalToken + `","arguments":"{\"key\":\"` + fictionalToken + `\"}"}}]}}]}`
	got, err := state.RestoreJSON([]byte(response), "chat")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), fictionalKey) != 1 || strings.Count(string(got), fictionalToken) != 5 {
		t.Fatal("restoration escaped authorized tool argument field")
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
		state := testState(t, DefaultConfig(), nil)
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
	state := testState(t, DefaultConfig(), nil)
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
	state := testState(t, DefaultConfig(), map[string]string{fictionalToken: fictionalKey})
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
	state := testState(t, DefaultConfig(), nil)
	body := `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","data":"` + fictionalKey + `"}},{"type":"document","title":"` + fictionalKey + `","source":{"type":"text","data":"` + fictionalKey + `"}}]}]}`
	got, err := state.ProtectJSON([]byte(body), "messages")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), fictionalKey) != 1 {
		t.Fatal("opaque media changed or document text leaked")
	}
}

func TestMultiRoundEchoAndExpiredMappings(t *testing.T) {
	first := testState(t, DefaultConfig(), nil)
	protected, err := first.ProtectJSON([]byte(`{"input":"`+fictionalKey+`"}`), "responses")
	if err != nil {
		t.Fatal(err)
	}
	var token string
	for token = range first.Entries() {
	}
	second := testState(t, DefaultConfig(), first.Entries())
	secondProtected, err := second.ProtectJSON([]byte(`{"input":"echo `+token+` and `+fictionalKey+`"}`), "responses")
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Entries()) != 1 || strings.Contains(string(secondProtected), fictionalKey) || !strings.Contains(string(protected), token) {
		t.Fatal("history echo did not reuse mapping")
	}
	expired := testState(t, DefaultConfig(), nil)
	if expired.RestoreText(token) != token {
		t.Fatal("unknown/expired token guessed")
	}
}

func TestCapacityNeverReturnsPartialProtectedBody(t *testing.T) {
	config := DefaultConfig()
	config.MaxMappings = 1
	state := testState(t, config, nil)
	body := `{"input":"` + fictionalKey + ` and ghp_` + strings.Repeat("B", 36) + `"}`
	got, err := state.ProtectJSON([]byte(body), "responses")
	if !errors.Is(err, ErrCapacity) || got != nil {
		t.Fatal("capacity failure exposed a partial body")
	}
}

// Production seeds are derived from a platform secret and authenticated scope.
// This deliberately public, fictional fixture only makes the example reproducible.
func ExampleState_ProtectJSON() {
	const secret = "ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	state, err := NewStateWithSeed(DefaultConfig(), nil, bytes.Repeat([]byte{0x42}, 32))
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
	// upstream: Use keyx_042a631f3210074debd0c896834d5a9024b1202715e5bce903bd673f3095e7a2
	// client text: Use ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
	// client tool arguments: {"key":"ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
}
