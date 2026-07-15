package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDeriveOpenAIContentSessionSeed_EmptyInputs(t *testing.T) {
	require.Empty(t, deriveOpenAIContentSessionSeed(nil))
	require.Empty(t, deriveOpenAIContentSessionSeed([]byte{}))
	require.Empty(t, deriveOpenAIContentSessionSeed([]byte(`{}`)))
}

func TestDeriveOpenAIContentSessionSeed_ModelOnly(t *testing.T) {
	seed := deriveOpenAIContentSessionSeed([]byte(`{"model":"gpt-5.4"}`))
	require.Contains(t, seed, contentSessionSeedPrefix)
	require.Contains(t, seed, "model=gpt-5.4")
}

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_StableAcrossTurns(t *testing.T) {
	turn1 := []byte(`{
		"model": "gpt-5.4",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"}
		]
	}`)
	turn2 := []byte(`{
		"model": "gpt-5.4",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi there!"},
			{"role": "user", "content": "How are you?"}
		]
	}`)
	s1 := deriveOpenAIContentSessionSeed(turn1)
	s2 := deriveOpenAIContentSessionSeed(turn2)
	require.Equal(t, s1, s2, "seed should be stable across later turns")
	require.NotEmpty(t, s1)
}

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_DifferentFirstUserDiffers(t *testing.T) {
	req1 := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Question A"}]}`)
	req2 := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Question B"}]}`)
	s1 := deriveOpenAIContentSessionSeed(req1)
	s2 := deriveOpenAIContentSessionSeed(req2)
	require.NotEqual(t, s1, s2)
}

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_DifferentSystemDiffers(t *testing.T) {
	req1 := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"A"},{"role":"user","content":"Hi"}]}`)
	req2 := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"B"},{"role":"user","content":"Hi"}]}`)
	s1 := deriveOpenAIContentSessionSeed(req1)
	s2 := deriveOpenAIContentSessionSeed(req2)
	require.NotEqual(t, s1, s2)
}

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_DifferentModelDiffers(t *testing.T) {
	req1 := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Hi"}]}`)
	req2 := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"Hi"}]}`)
	s1 := deriveOpenAIContentSessionSeed(req1)
	s2 := deriveOpenAIContentSessionSeed(req2)
	require.NotEqual(t, s1, s2)
}

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_WithTools(t *testing.T) {
	withTools := []byte(`{
		"model": "gpt-5.4",
		"tools": [{"type":"function","function":{"name":"get_weather"}}],
		"messages": [{"role": "user", "content": "Hello"}]
	}`)
	withoutTools := []byte(`{
		"model": "gpt-5.4",
		"messages": [{"role": "user", "content": "Hello"}]
	}`)
	s1 := deriveOpenAIContentSessionSeed(withTools)
	s2 := deriveOpenAIContentSessionSeed(withoutTools)
	require.NotEqual(t, s1, s2, "tools should affect the seed")
	require.Contains(t, s1, "|tools=")
}

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_WithFunctions(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"functions": [{"name":"get_weather","parameters":{}}],
		"messages": [{"role": "user", "content": "Hello"}]
	}`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|functions=")
}

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_DeveloperRole(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"messages": [
			{"role": "developer", "content": "You are helpful."},
			{"role": "user", "content": "Hello"}
		]
	}`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|system=")
	require.Contains(t, seed, "|first_user=")
}

func TestDeriveOpenAIContentSessionSeed_ChatCompletions_StructuredContent(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"messages": [
			{"role": "user", "content": [{"type":"text","text":"Hello"}]}
		]
	}`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.NotEmpty(t, seed)
	require.Contains(t, seed, "|first_user=")
}

func TestDeriveOpenAIContentSessionSeed_ResponsesAPI_InputString(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"Hello, how are you?"}`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|input=Hello, how are you?")
}

func TestDeriveOpenAIContentSessionSeed_ResponsesAPI_InputArray(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"input": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"}
		]
	}`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|system=")
	require.Contains(t, seed, "|first_user=")
}

func TestDeriveOpenAIContentSessionSeed_ResponsesAPI_WithInstructions(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"instructions": "You are a coding assistant.",
		"input": "Write a hello world"
	}`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|instructions=You are a coding assistant.")
	require.Contains(t, seed, "|input=Write a hello world")
}

func TestDeriveOpenAIContentSessionSeed_Deterministic(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"}
		]
	}`)
	s1 := deriveOpenAIContentSessionSeed(body)
	s2 := deriveOpenAIContentSessionSeed(body)
	require.Equal(t, s1, s2, "seed must be deterministic")
}

func TestDeriveOpenAIContentSessionSeed_PrefixPresent(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Hi"}]}`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.True(t, len(seed) > len(contentSessionSeedPrefix))
	require.Equal(t, contentSessionSeedPrefix, seed[:len(contentSessionSeedPrefix)])
}

func TestDeriveOpenAIContentSessionSeed_EmptyToolsIgnored(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","tools":[],"messages":[{"role":"user","content":"Hi"}]}`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.NotContains(t, seed, "|tools=")
}

func TestDeriveOpenAIContentSessionSeed_MessagesPreferredOverInput(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"messages": [{"role": "user", "content": "from messages"}],
		"input": "from input"
	}`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|first_user=")
	require.NotContains(t, seed, "|input=")
}

func TestDeriveOpenAIContentSessionSeed_JSONCanonicalisation(t *testing.T) {
	compact := []byte(`{"model":"gpt-5.4","tools":[{"type":"function","function":{"name":"get_weather","description":"Get weather"}}],"messages":[{"role":"user","content":"Hi"}]}`)
	spaced := []byte(`{
		"model": "gpt-5.4",
		"tools": [
			{ "type" : "function", "function": { "description": "Get weather", "name": "get_weather" } }
		],
		"messages": [ { "role": "user", "content": "Hi" } ]
	}`)
	s1 := deriveOpenAIContentSessionSeed(compact)
	s2 := deriveOpenAIContentSessionSeed(spaced)
	require.Equal(t, s1, s2, "different formatting of identical JSON should produce the same seed")
}

func TestDeriveOpenAIContentSessionSeed_SingleScanMatchesLegacyBytes(t *testing.T) {
	largeValue := strings.Repeat("payload", 1<<17)
	tests := []struct {
		name string
		body []byte
	}{
		{
			name: "large chat completions",
			body: []byte(`{"metadata":"` + largeValue + `","model":"gpt-5.4","tools":[{"type":"function","function":{"name":"lookup"}}],"functions":[{"name":"legacy_lookup"}],"messages":[{"role":"system","content":"System prompt"},{"role":"developer","content":[{"type":"text","text":"Developer prompt"}]},{"role":"user","content":"Hello"}]}`),
		},
		{
			name: "large responses",
			body: []byte(`{"metadata":"` + largeValue + `","model":"gpt-5.4","instructions":"Be concise.","tools":[{"type":"function","name":"lookup"}],"input":[{"role":"system","content":"System prompt"},{"role":"user","content":[{"type":"input_text","text":"Hello"}]}]}`),
		},
		{
			name: "fields in reverse order",
			body: []byte(`{"input":"fallback input","messages":[{"role":"user","content":"chat wins"}],"instructions":"Be concise.","functions":[{"name":"lookup"}],"tools":[{"type":"function","name":"lookup"}],"model":"gpt-5.4"}`),
		},
		{
			name: "missing and wrong type fields",
			body: []byte(`{"tools":[],"functions":null,"instructions":0,"messages":{},"input":[{"type":"input_text","text":"fallback"}]}`),
		},
		{
			name: "duplicate fields keep first value",
			body: []byte(`{"model":"first","model":"second","tools":[{"name":"first"}],"tools":[{"name":"second"}],"functions":[],"functions":[{"name":"second"}],"instructions":"first","instructions":"second","messages":null,"messages":[{"role":"user","content":"second"}],"input":"first input","input":"second input"}`),
		},
		{
			name: "escaped field names",
			body: []byte(`{"mo\u0064el":"gpt-5.4","mess\u0061ges":[{"role":"user","content":"Hello"}]}`),
		},
		{
			name: "trailing object fields are outside the root",
			body: []byte(`{"foo":1}{"model":"trailing","input":"trailing input"}`),
		},
		{
			name: "trailing quoted fields are outside the root",
			body: []byte(`{"model":"root"}"input":"trailing input"`),
		},
		{
			name: "leading garbage before the root",
			body: []byte(`garbage{"model":"gpt-5.4","input":"Hello"}`),
		},
		{
			name: "escaped braces remain inside string values",
			body: []byte(`{"metadata":"escaped } and [ and \" quote","model":"root","input":"Hello"}{"model":"trailing"}`),
		},
		{
			name: "nested braces do not end the root",
			body: []byte(`{"metadata":{"nested":"} ]"},"model":"root","input":"Hello"}{"model":"trailing"}`),
		},
		{
			name: "root array does not expose nested or trailing object fields",
			body: []byte(`[{"model":"nested"}]{"model":"trailing","input":"trailing input"}`),
		},
		{
			name: "trailing messages do not override root input",
			body: []byte(`{"input":"root input"}{"messages":[{"role":"user","content":"trailing"}]}`),
		},
		{
			name: "truncated string containing a closing brace",
			body: []byte(`{"model":"root","metadata":"still } inside`),
		},
		{
			name: "lenient truncated body",
			body: []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"Hello"}]`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, legacyDeriveOpenAIContentSessionSeed(test.body), deriveOpenAIContentSessionSeed(test.body))
		})
	}
}

func TestDeriveOpenAIContentSessionSeed_AllTruncationOffsetsMatchLegacyBytes(t *testing.T) {
	bodies := []string{
		`{"model":"gpt-5.4","tools":[{"type":"function","function":{"name":"lookup"}}],"functions":[{"name":"legacy"}],"instructions":"escaped \" } text","messages":[{"role":"system","content":"System"},{"role":"user","content":[{"type":"text","text":"Hello"}]}],"input":"fallback"}`,
		`{"model":"gpt-5.4","instructions":"Be concise.","tools":[{"type":"function","name":"lookup"}],"input":[{"role":"system","content":"System"},{"role":"user","content":[{"type":"input_text","text":"Hello"}]}]}`,
	}
	for bodyIndex, body := range bodies {
		for end := 1; end < len(body); end++ {
			truncated := []byte(body[:end])
			require.Equalf(t, legacyDeriveOpenAIContentSessionSeed(truncated), deriveOpenAIContentSessionSeed(truncated), "body %d truncated at byte %d", bodyIndex, end)
		}
	}
}

func legacyDeriveOpenAIContentSessionSeed(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var b strings.Builder

	if model := gjson.GetBytes(body, "model").String(); model != "" {
		_, _ = b.WriteString("model=")
		_, _ = b.WriteString(model)
	}

	if tools := gjson.GetBytes(body, "tools"); tools.Exists() && tools.IsArray() && tools.Raw != "[]" {
		_, _ = b.WriteString("|tools=")
		_, _ = b.WriteString(normalizeCompatSeedJSON(json.RawMessage(tools.Raw)))
	}

	if funcs := gjson.GetBytes(body, "functions"); funcs.Exists() && funcs.IsArray() && funcs.Raw != "[]" {
		_, _ = b.WriteString("|functions=")
		_, _ = b.WriteString(normalizeCompatSeedJSON(json.RawMessage(funcs.Raw)))
	}

	if instr := gjson.GetBytes(body, "instructions").String(); instr != "" {
		_, _ = b.WriteString("|instructions=")
		_, _ = b.WriteString(instr)
	}

	firstUserCaptured := false

	msgs := gjson.GetBytes(body, "messages")
	if msgs.Exists() && msgs.IsArray() {
		msgs.ForEach(func(_, msg gjson.Result) bool {
			role := msg.Get("role").String()
			switch role {
			case "system", "developer":
				_, _ = b.WriteString("|system=")
				if c := msg.Get("content"); c.Exists() {
					_, _ = b.WriteString(normalizeCompatSeedJSON(json.RawMessage(c.Raw)))
				}
			case "user":
				if !firstUserCaptured {
					_, _ = b.WriteString("|first_user=")
					if c := msg.Get("content"); c.Exists() {
						_, _ = b.WriteString(normalizeCompatSeedJSON(json.RawMessage(c.Raw)))
					}
					firstUserCaptured = true
				}
			}
			return true
		})
	} else if inp := gjson.GetBytes(body, "input"); inp.Exists() {
		if inp.Type == gjson.String {
			_, _ = b.WriteString("|input=")
			_, _ = b.WriteString(inp.String())
		} else if inp.IsArray() {
			inp.ForEach(func(_, item gjson.Result) bool {
				role := item.Get("role").String()
				switch role {
				case "system", "developer":
					_, _ = b.WriteString("|system=")
					if c := item.Get("content"); c.Exists() {
						_, _ = b.WriteString(normalizeCompatSeedJSON(json.RawMessage(c.Raw)))
					}
				case "user":
					if !firstUserCaptured {
						_, _ = b.WriteString("|first_user=")
						if c := item.Get("content"); c.Exists() {
							_, _ = b.WriteString(normalizeCompatSeedJSON(json.RawMessage(c.Raw)))
						}
						firstUserCaptured = true
					}
				}
				if !firstUserCaptured && item.Get("type").String() == "input_text" {
					_, _ = b.WriteString("|first_user=")
					if text := item.Get("text").String(); text != "" {
						_, _ = b.WriteString(text)
					}
					firstUserCaptured = true
				}
				return true
			})
		}
	}

	if b.Len() == 0 {
		return ""
	}
	return contentSessionSeedPrefix + b.String()
}

func TestDeriveOpenAIContentSessionSeed_ResponsesAPI_InputTextTypedItem(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"input": [{"type": "input_text", "text": "Hello world"}]
	}`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|first_user=")
	require.Contains(t, seed, "Hello world")
}

func TestDeriveOpenAIContentSessionSeed_ResponsesAPI_TypedMessageItem(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"input": [{"type": "message", "role": "user", "content": "Hello from typed message"}]
	}`)
	seed := deriveOpenAIContentSessionSeed(body)
	require.Contains(t, seed, "|first_user=")
	require.Contains(t, seed, "Hello from typed message")
}

func TestDeriveOpenAIStablePrefixSessionSeed_IgnoresUserContent(t *testing.T) {
	first := []byte(`{
		"model": "grok",
		"instructions": "Be concise.",
		"tools": [{"type":"function","name":"lookup","parameters":{"type":"object"}}],
		"input": [{"role":"user","content":"Question A"}]
	}`)
	second := []byte(`{
		"model": "grok",
		"instructions": "Be concise.",
		"tools": [{"parameters":{"type":"object"},"name":"lookup","type":"function"}],
		"input": [{"role":"user","content":"Question B"}]
	}`)

	firstSeed := deriveOpenAIStablePrefixSessionSeed(first)
	secondSeed := deriveOpenAIStablePrefixSessionSeed(second)

	require.NotEmpty(t, firstSeed)
	require.Equal(t, firstSeed, secondSeed)
	require.NotContains(t, firstSeed, "Question A")
	require.NotContains(t, firstSeed, "first_user")
}

func TestDeriveOpenAIStablePrefixSessionSeed_IsolatesStablePrefixFields(t *testing.T) {
	base := []byte(`{
		"instructions":"Be concise.",
		"tools":[{"type":"function","name":"lookup"}],
		"input":[{"role":"system","content":"System A"},{"role":"user","content":"Question"}]
	}`)
	differentInstructions := []byte(`{
		"instructions":"Be detailed.",
		"tools":[{"type":"function","name":"lookup"}],
		"input":[{"role":"system","content":"System A"},{"role":"user","content":"Question"}]
	}`)
	differentTools := []byte(`{
		"instructions":"Be concise.",
		"tools":[{"type":"function","name":"search"}],
		"input":[{"role":"system","content":"System A"},{"role":"user","content":"Question"}]
	}`)
	differentSystem := []byte(`{
		"instructions":"Be concise.",
		"tools":[{"type":"function","name":"lookup"}],
		"input":[{"role":"system","content":"System B"},{"role":"user","content":"Question"}]
	}`)

	baseSeed := deriveOpenAIStablePrefixSessionSeed(base)
	require.NotEqual(t, baseSeed, deriveOpenAIStablePrefixSessionSeed(differentInstructions))
	require.NotEqual(t, baseSeed, deriveOpenAIStablePrefixSessionSeed(differentTools))
	require.NotEqual(t, baseSeed, deriveOpenAIStablePrefixSessionSeed(differentSystem))
}

func TestDeriveOpenAIStablePrefixSessionSeed_ChatSystemAndDeveloper(t *testing.T) {
	first := []byte(`{
		"messages":[
			{"role":"system","content":"System prompt"},
			{"role":"developer","content":[{"type":"text","text":"Developer prompt"}]},
			{"role":"user","content":"Question A"}
		]
	}`)
	second := []byte(`{
		"messages":[
			{"role":"system","content":"System prompt"},
			{"role":"developer","content":[{"text":"Developer prompt","type":"text"}]},
			{"role":"user","content":"Question B"}
		]
	}`)

	firstSeed := deriveOpenAIStablePrefixSessionSeed(first)
	require.Equal(t, firstSeed, deriveOpenAIStablePrefixSessionSeed(second))
	require.Contains(t, firstSeed, "System prompt")
	require.Contains(t, firstSeed, "Developer prompt")
}

func TestDeriveOpenAIStablePrefixSessionSeed_EncodesSystemAndDeveloperRoles(t *testing.T) {
	systemThenDeveloper := []byte(`{
		"messages":[
			{"role":"system","content":"Prompt A"},
			{"role":"developer","content":"Prompt B"}
		]
	}`)
	developerThenSystem := []byte(`{
		"messages":[
			{"role":"developer","content":"Prompt A"},
			{"role":"system","content":"Prompt B"}
		]
	}`)

	firstSeed := deriveOpenAIStablePrefixSessionSeed(systemThenDeveloper)
	secondSeed := deriveOpenAIStablePrefixSessionSeed(developerThenSystem)

	require.NotEqual(t, firstSeed, secondSeed)
	require.Contains(t, firstSeed, "|system=")
	require.Contains(t, firstSeed, "|developer=")
}

func TestDeriveOpenAIStablePrefixSessionSeed_EncodesInstructionDelimiters(t *testing.T) {
	instructionOnly := []byte(`{
		"instructions":"foo|system=\"bar\""
	}`)
	instructionAndSystem := []byte(`{
		"instructions":"foo",
		"input":[{"role":"system","content":"bar"}]
	}`)

	firstSeed := deriveOpenAIStablePrefixSessionSeed(instructionOnly)
	secondSeed := deriveOpenAIStablePrefixSessionSeed(instructionAndSystem)

	require.NotEmpty(t, firstSeed)
	require.NotEmpty(t, secondSeed)
	require.NotEqual(t, firstSeed, secondSeed)
}

func TestDeriveOpenAIAnchoredContentSessionSeed_RequiresMeaningfulAnchor(t *testing.T) {
	emptyAnchors := [][]byte{
		nil,
		[]byte(`{"model":"grok"}`),
		[]byte(`{"model":"grok","messages":[{"role":"assistant","content":"answer"}]}`),
		[]byte(`{"model":"grok","messages":[{"role":"user","content":"  "}]}`),
		[]byte(`{"model":"grok","messages":[{"role":"user","content":[{"type":"text","text":""}]}]}`),
		[]byte(`{"model":"grok","input":"  "}`),
		[]byte(`{"model":"grok","input":[{"type":"input_text","text":""}]}`),
	}
	for _, body := range emptyAnchors {
		require.Empty(t, deriveOpenAIAnchoredContentSessionSeed(body))
	}

	meaningfulAnchors := [][]byte{
		[]byte(`{"model":"grok","messages":[{"role":"user","content":"question"}]}`),
		[]byte(`{"model":"grok","messages":[{"role":"user","content":[{"type":"text","text":"question"}]}]}`),
		[]byte(`{"model":"grok","input":"question"}`),
		[]byte(`{"model":"grok","input":[{"type":"input_text","text":"question"}]}`),
	}
	for _, body := range meaningfulAnchors {
		require.NotEmpty(t, deriveOpenAIAnchoredContentSessionSeed(body))
	}
}

func TestDeriveOpenAIStablePrefixSessionSeed_RequiresMeaningfulPrefix(t *testing.T) {
	tests := [][]byte{
		nil,
		[]byte(`{}`),
		[]byte(`{"model":"grok","input":"Question A"}`),
		[]byte(`{"model":"grok","tools":[],"input":"Question A"}`),
		[]byte(`{"model":"grok","functions":[],"instructions":"  ","messages":[{"role":"system","content":""},{"role":"user","content":"Question A"}]}`),
	}

	for _, body := range tests {
		require.Empty(t, deriveOpenAIStablePrefixSessionSeed(body))
	}
}
