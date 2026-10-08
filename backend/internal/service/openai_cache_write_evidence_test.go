package service

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func cacheWriteEvidenceRequest(input ...any) []byte {
	body, _ := json.Marshal(map[string]any{"model": "test-model", "input": input})
	return body
}

func cacheWriteEvidenceResponse(output ...any) []byte {
	if output == nil {
		output = []any{}
	}
	body, _ := json.Marshal(map[string]any{"status": "completed", "output": output})
	return body
}

func cacheWriteEvidenceMessage(role, text string) map[string]any {
	return map[string]any{"role": role, "content": text}
}

func cacheWriteEvidenceCall(id string) map[string]any {
	return map[string]any{"type": "function_call", "call_id": id, "name": "search", "arguments": `{"query":"example"}`}
}

func cacheWriteEvidenceResult(id string) map[string]any {
	return map[string]any{"type": "function_call_output", "call_id": id, "output": "result"}
}

func TestOpenAICacheWriteEvidence_NormalizesKnownTextWrappers(t *testing.T) {
	t.Parallel()
	base := buildOpenAICacheWritePromptEvidence([]byte(`{"model":"test-model","input":"hello"}`))
	if !base.Valid {
		t.Fatal("input string should form one user message")
	}
	websocket := buildOpenAICacheWritePromptEvidence([]byte(`{"type":"response.create","model":"test-model","input":"hello"}`))
	if !openAICacheWritePromptsIdentical(base, websocket) {
		t.Fatal("response.create transport envelope should not change prompt identity")
	}
	for _, content := range []string{
		`"hello"`,
		`[{"type":"text","text":"hello"}]`,
		`[{"type":"input_text","text":"hello"}]`,
		`[{"type":"output_text","text":"hello","annotations":[]}]`,
	} {
		next := buildOpenAICacheWritePromptEvidence([]byte(`{"input":[{"id":"not-retained","type":"message","status":"completed","role":"user","content":` + content + `}],"model":"test-model","stream":true,"stream_options":{"include_obfuscation":false}}`))
		if !openAICacheWritePromptsIdentical(base, next) {
			t.Fatalf("known content wrapper should normalize: %s", content)
		}
	}
	segmented := buildOpenAICacheWritePromptEvidence([]byte(`{"model":"test-model","input":[{"role":"user","content":[{"type":"text","text":"hel"},{"type":"text","text":"lo"}]}]}`))
	if openAICacheWritePromptsIdentical(base, segmented) {
		t.Fatal("different block boundaries must not be silently flattened")
	}
}

func TestOpenAICacheWriteEvidence_ConfigIsConservative(t *testing.T) {
	t.Parallel()
	base := buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(cacheWriteEvidenceMessage("user", "hello")))
	for _, field := range []string{
		`"instructions":"changed"`, `"model":"changed"`, `"tools":[{"type":"function","name":"other"}]`,
		`"tool_choice":"none"`, `"parallel_tool_calls":false`, `"temperature":0.5`, `"top_p":0.5`,
		`"top_logprobs":2`, `"generate":true`,
		`"reasoning":{"effort":"high"}`, `"text":{"format":{"type":"json_object"}}`,
		`"max_output_tokens":999`, `"max_tool_calls":2`, `"include":["reasoning.encrypted_content"]`,
		`"service_tier":"priority"`, `"prompt_cache_key":"changed"`, `"prompt_cache_retention":"24h"`,
		`"metadata":{"x":"y"}`, `"store":false`, `"safety_identifier":"changed"`, `"user":"changed"`,
		`"background":false`, `"truncation":"disabled"`, `"previous_response_id":null`,
		`"client_metadata":{"session_id":"session","turn_id":"turn"}`, `"prompt_cache_options":{"mode":"auto","ttl":"30m","breakpoints":[1]}`,
	} {
		t.Run(field, func(t *testing.T) {
			prefix := `{"model":"test-model",`
			if strings.HasPrefix(field, `"model":`) {
				prefix = `{`
			}
			next := buildOpenAICacheWritePromptEvidence([]byte(prefix + field + `,"input":[{"role":"user","content":"hello"}]}`))
			if !next.Valid {
				t.Fatal("known configuration should be hashable")
			}
			if openAICacheWritePromptsIdentical(base, next) {
				t.Fatal("configuration change must break lineage")
			}
		})
	}
	a := buildOpenAICacheWritePromptEvidence([]byte(`{"model":"test-model","input":"hello","tools":[{"name":"x","parameters":{"b":2,"a":1}}]}`))
	b := buildOpenAICacheWritePromptEvidence([]byte(`{"tools":[{"parameters":{"a":1,"b":2},"name":"x"}],"input":"hello","model":"test-model"}`))
	if !openAICacheWritePromptsIdentical(a, b) {
		t.Fatal("JSON object key order should not matter")
	}
	b = buildOpenAICacheWritePromptEvidence([]byte(`{"tools":[{"parameters":{"a":1,"b":3},"name":"x"}],"input":"hello","model":"test-model"}`))
	if openAICacheWritePromptsIdentical(a, b) {
		t.Fatal("nested configuration changes must remain visible")
	}
}

func TestOpenAICacheWriteEvidence_PreservesCacheOptionsAndNonTransportMetadata(t *testing.T) {
	t.Parallel()
	for name, values := range map[string][]any{
		"client_metadata": {
			map[string]any{"session_id": "session", "turn_id": "turn-one", "x-codex-turn-metadata": `{"turn_started_at_unix_ms":1000}`},
			map[string]any{"session_id": "other-session", "turn_id": "turn-one", "x-codex-turn-metadata": `{"turn_started_at_unix_ms":1000}`},
			map[string]any{"session_id": "session", "turn_id": "turn-one", "x-codex-turn-metadata": `{"turn_started_at_unix_ms":1000,"unknown":"different"}`},
			map[string]any{"session_id": "session", "turn_id": "turn-one", "x-codex-turn-metadata": `{"turn_started_at_unix_ms":1000}`, "new_nested_field": true},
		},
		"prompt_cache_options": {
			map[string]any{"mode": "auto", "ttl": "30m", "breakpoints": []any{1}},
			map[string]any{"mode": "manual", "ttl": "30m", "breakpoints": []any{1}},
			map[string]any{"mode": "auto", "ttl": "24h", "breakpoints": []any{1}},
			map[string]any{"mode": "auto", "ttl": "30m", "breakpoints": []any{2}},
			map[string]any{"mode": "auto", "ttl": "30m", "breakpoints": []any{1}, "new_nested_field": true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			build := func(value any) openAICacheWritePromptEvidence {
				body, _ := json.Marshal(map[string]any{"model": "test-model", "input": "hello", name: value})
				return buildOpenAICacheWritePromptEvidence(body)
			}
			base := build(values[0])
			if !base.Valid || !openAICacheWritePromptsIdentical(base, build(values[0])) {
				t.Fatal("unchanged known metadata/configuration should preserve equality")
			}
			for _, changed := range values[1:] {
				next := build(changed)
				if !next.Valid || openAICacheWritePromptsIdentical(base, next) {
					t.Fatal("a changed nested field must change configuration identity")
				}
			}
		})
	}
}

func TestOpenAICacheWriteEvidence_NormalizesOnlyVerifiedCodexTurnMetadata(t *testing.T) {
	t.Parallel()
	embeddedBase := map[string]any{
		"session_id": "session", "thread_id": "thread", "installation_id": "installation", "window_id": "window",
		"turn_id": "turn-one", "turn_started_at_unix_ms": 1000, "request_kind": "turn", "sandbox": "seatbelt",
		"unknown": map[string]any{"turn_id": "must-remain-significant"},
	}
	flatBase := map[string]any{
		"session_id": "session", "thread_id": "thread", "x-codex-installation-id": "installation", "x-codex-window-id": "window",
		"turn_id": "turn-one", "turn_started_at_unix_ms": 1000, "unknown": map[string]any{"turn_id": "must-remain-significant"},
	}
	clone := func(source map[string]any) map[string]any {
		result := make(map[string]any, len(source))
		for key, value := range source {
			result[key] = value
		}
		return result
	}
	build := func(flat, embedded map[string]any, input ...any) openAICacheWritePromptEvidence {
		metadata := clone(flat)
		raw, _ := json.Marshal(embedded)
		metadata["x-codex-turn-metadata"] = string(raw)
		body, _ := json.Marshal(map[string]any{"model": "test-model", "client_metadata": metadata, "input": input})
		return buildOpenAICacheWritePromptEvidence(body)
	}
	user := cacheWriteEvidenceMessage("user", "question")
	base := build(flatBase, embeddedBase, user)
	if !base.Valid {
		t.Fatal("normal Codex metadata should be supported")
	}
	for _, path := range []string{"flat turn", "embedded turn", "embedded timestamp", "all verified paths"} {
		t.Run(path, func(t *testing.T) {
			flat, embedded := clone(flatBase), clone(embeddedBase)
			if path == "flat turn" || path == "all verified paths" {
				flat["turn_id"] = "turn-two"
			}
			if path == "embedded turn" || path == "all verified paths" {
				embedded["turn_id"] = "turn-two"
			}
			if path == "embedded timestamp" || path == "all verified paths" {
				embedded["turn_started_at_unix_ms"] = 2000
			}
			if next := build(flat, embedded, user); !openAICacheWritePromptsIdentical(base, next) {
				t.Fatal("verified per-turn correlation changes must not invalidate prompt identity")
			}
			assistant := cacheWriteEvidenceMessage("assistant", "answer")
			next := build(flat, embedded, user, assistant, cacheWriteEvidenceMessage("user", "continue"))
			if !openAICacheWriteDirectSuccessor(base, buildOpenAICacheWriteOutputEvidence(cacheWriteEvidenceResponse(assistant)), next) {
				t.Fatal("normal Codex direct successor must tolerate verified correlation changes")
			}
		})
	}
	for _, field := range []string{"session_id", "thread_id", "x-codex-installation-id", "x-codex-window-id", "turn_started_at_unix_ms", "unknown"} {
		t.Run("preserve flat "+field, func(t *testing.T) {
			flat := clone(flatBase)
			flat[field] = "different"
			next := build(flat, embeddedBase, user)
			if !next.Valid || openAICacheWritePromptsIdentical(base, next) {
				t.Fatal("unapproved flat metadata path was ignored")
			}
		})
	}
	for _, field := range []string{"session_id", "thread_id", "installation_id", "window_id", "request_kind", "sandbox", "unknown", "future_field"} {
		t.Run("preserve embedded "+field, func(t *testing.T) {
			embedded := clone(embeddedBase)
			embedded[field] = "different"
			next := build(flatBase, embedded, user)
			if !next.Valid || openAICacheWritePromptsIdentical(base, next) {
				t.Fatal("unapproved embedded metadata path was ignored")
			}
		})
	}
	if openAICacheWritePromptsIdentical(base, build(flatBase, embeddedBase, cacheWriteEvidenceMessage("user", "different prompt"))) {
		t.Fatal("normalizing transport fields must not weaken prompt comparison")
	}
}

func TestOpenAICacheWriteEvidence_RejectsMalformedCodexTurnMetadata(t *testing.T) {
	t.Parallel()
	for name, embedded := range map[string]any{
		"not string":              map[string]any{"turn_id": "one"},
		"null":                    nil,
		"empty":                   "",
		"invalid":                 "{",
		"array":                   "[]",
		"scalar":                  `"metadata"`,
		"duplicate ignored key":   `{"turn_id":"one","turn_id":"two"}`,
		"duplicate preserved key": `{"session_id":"one","session_id":"two"}`,
		"bad Unicode":             `{"turn_id":"\ud800"}`,
		"bad turn ID":             `{"turn_id":{"input":"must-not-be-ignored"}}`,
		"timestamp string":        `{"turn_started_at_unix_ms":"1000"}`,
		"timestamp fraction":      `{"turn_started_at_unix_ms":1.5}`,
		"timestamp negative":      `{"turn_started_at_unix_ms":-1}`,
		"timestamp null":          `{"turn_started_at_unix_ms":null}`,
		"timestamp overflow":      `{"turn_started_at_unix_ms":9223372036854775808}`,
		"too deep":                `{"unknown":` + strings.Repeat("[", openAICacheWriteEvidenceMaxDepth+1) + "0" + strings.Repeat("]", openAICacheWriteEvidenceMaxDepth+1) + "}",
	} {
		t.Run(name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"model": "test-model", "input": "hello", "client_metadata": map[string]any{"x-codex-turn-metadata": embedded}})
			if buildOpenAICacheWritePromptEvidence(body).Valid {
				t.Fatal("malformed embedded metadata must fail closed")
			}
		})
	}
	for _, metadata := range []any{nil, "metadata", []any{}, map[string]any{"turn_id": nil}, map[string]any{"turn_id": 42}} {
		body, _ := json.Marshal(map[string]any{"model": "test-model", "input": "hello", "client_metadata": metadata})
		if buildOpenAICacheWritePromptEvidence(body).Valid {
			t.Fatal("malformed client metadata or correlation ID must fail closed")
		}
	}
}

func TestOpenAICacheWriteEvidence_RejectsUnsafeRequests(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"missing model":           `{"input":"hello"}`,
		"missing input":           `{"model":"test-model"}`,
		"empty input":             `{"model":"test-model","input":[]}`,
		"null input":              `{"model":"test-model","input":null}`,
		"chat completions":        `{"model":"test-model","messages":[{"role":"user","content":"hello"}]}`,
		"unknown config":          `{"model":"test-model","input":"hello","future_semantic_field":true}`,
		"unknown event":           `{"type":"response.append","model":"test-model","input":"hello"}`,
		"prewarm request":         `{"type":"response.create","generate":false,"model":"test-model","input":"hello"}`,
		"invalid generate":        `{"type":"response.create","generate":"true","model":"test-model","input":"hello"}`,
		"previous response":       `{"model":"test-model","input":"hello","previous_response_id":"resp_1"}`,
		"empty previous ID":       `{"model":"test-model","input":"hello","previous_response_id":""}`,
		"conversation":            `{"model":"test-model","input":"hello","conversation":"conv_1"}`,
		"template":                `{"model":"test-model","input":"hello","prompt":{"id":"pmpt_1"}}`,
		"compaction config":       `{"model":"test-model","input":"hello","context_management":[]}`,
		"automatic truncation":    `{"model":"test-model","input":"hello","truncation":"auto"}`,
		"unknown stream option":   `{"model":"test-model","input":"hello","stream_options":{"unknown":true}}`,
		"bad stream type":         `{"model":"test-model","input":"hello","stream":"true"}`,
		"unknown item key":        `{"model":"test-model","input":[{"role":"user","content":"hello","name":"different-speaker"}]}`,
		"reference":               `{"model":"test-model","input":[{"type":"item_reference","id":"msg_1"}]}`,
		"message ID only":         `{"model":"test-model","input":[{"type":"message","id":"msg_1","role":"assistant"}]}`,
		"compaction":              `{"model":"test-model","input":[{"type":"compaction","encrypted_content":"cipher"}]}`,
		"image":                   `{"model":"test-model","input":[{"role":"user","content":[{"type":"input_image","image_url":"https://example.com/x.png"}]}]}`,
		"audio":                   `{"model":"test-model","input":[{"role":"user","content":[{"type":"input_audio","data":"audio"}]}]}`,
		"refusal":                 `{"model":"test-model","input":[{"role":"assistant","content":[{"type":"refusal","refusal":"no"}]}]}`,
		"incomplete message":      `{"model":"test-model","input":[{"role":"assistant","content":"partial","status":"in_progress"}]}`,
		"plain reasoning":         `{"model":"test-model","input":[{"type":"reasoning","summary":[]}]}`,
		"unknown encrypted field": `{"model":"test-model","input":[{"type":"reasoning","encrypted_content":"cipher","unknown":true}]}`,
		"missing function ID":     `{"model":"test-model","input":[{"type":"function_call","name":"x","arguments":"{}"}]}`,
		"unknown tool":            `{"model":"test-model","input":[{"type":"custom_tool_call","call_id":"c","name":"x","input":"x"}]}`,
		"missing function result": `{"model":"test-model","input":[{"type":"function_call_output","call_id":"c"}]}`,
		"tool message":            `{"model":"test-model","input":[{"role":"tool","content":"result"}]}`,
		"non-string text":         `{"model":"test-model","input":[{"role":"user","content":[{"type":"text","text":["a"]}]}]}`,
		"unknown text field":      `{"model":"test-model","input":[{"role":"user","content":[{"type":"text","text":"x","future_field":true}]}]}`,
		"non-array annotations":   `{"model":"test-model","input":[{"role":"assistant","content":[{"type":"output_text","text":"x","annotations":{}}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := buildOpenAICacheWritePromptEvidence([]byte(body)); got.Valid || len(got.InputHashes) != 0 || got.ConfigHash != "" || len(got.items) != 0 {
				t.Fatal("unsafe request retained usable or partial evidence")
			}
		})
	}
}

func TestOpenAICacheWriteEvidence_OutputRequiresCompleteKnownItems(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"empty completed": `{"status":"completed","output":[]}`,
		"empty implicit":  `{"output":[]}`,
		"text message":    `{"status":"completed","output":[{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"answer","annotations":[]}]}]}`,
		"terminal event":  `{"type":"response.completed","response":{"status":"completed","output":[]}}`,
		"done event":      `{"type":"response.done","response":{"output":[]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if !buildOpenAICacheWriteOutputEvidence([]byte(body)).Valid {
				t.Fatal("complete known output should be valid")
			}
		})
	}
	for name, body := range map[string]string{
		"empty body":           ``,
		"missing output":       `{"status":"completed"}`,
		"null output":          `{"status":"completed","output":null}`,
		"incomplete status":    `{"status":"incomplete","output":[]}`,
		"failed status":        `{"status":"failed","output":[]}`,
		"null status":          `{"status":null,"output":[]}`,
		"error":                `{"output":[],"error":{"message":"failure"}}`,
		"incomplete details":   `{"output":[],"incomplete_details":{"reason":"max_output_tokens"}}`,
		"nonterminal event":    `{"type":"response.in_progress","response":{"output":[]}}`,
		"terminal error":       `{"type":"response.completed","error":{"message":"failure"},"response":{"output":[]}}`,
		"failed done response": `{"type":"response.done","response":{"status":"failed","output":[]}}`,
		"missing event body":   `{"type":"response.completed","output":[]}`,
		"unknown output":       `{"output":[{"type":"web_search_call","status":"completed","id":"ws_1"}]}`,
		"partial item":         `{"output":[{"type":"message","role":"assistant","content":"a","status":"in_progress"}]}`,
		"nonassistant message": `{"output":[{"role":"user","content":"x"}]}`,
		"function result":      `{"output":[{"type":"function_call_output","call_id":"c","output":"x"}]}`,
		"unencrypted reasoning": `{"output":[{"type":"reasoning","summary":[]},
			{"role":"assistant","content":"x"}]}`,
		"partial known output": `{"output":[{"role":"assistant","content":"x"},{"type":"unknown"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := buildOpenAICacheWriteOutputEvidence([]byte(body)); got.Valid || len(got.OutputHashes) != 0 || len(got.items) != 0 {
				t.Fatal("missing, partial or unknown output must not retain evidence")
			}
		})
	}
}

func TestOpenAICacheWriteEvidence_DirectTextSuccessor(t *testing.T) {
	t.Parallel()
	user := cacheWriteEvidenceMessage("user", "question")
	assistant := cacheWriteEvidenceMessage("assistant", "answer")
	nextUser := cacheWriteEvidenceMessage("user", "follow-up")
	previous := buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(user))
	output := buildOpenAICacheWriteOutputEvidence(cacheWriteEvidenceResponse(assistant))
	next := buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(user, assistant, nextUser))
	if !openAICacheWriteDirectSuccessor(previous, output, next) {
		t.Fatal("observed direct successor should match")
	}
	for name, input := range map[string][]any{
		"identical input":           {user},
		"missing output":            {user, nextUser},
		"missing next turn":         {user, assistant},
		"extra user turn":           {user, assistant, nextUser, nextUser},
		"unobserved assistant turn": {user, assistant, nextUser, assistant, nextUser},
		"assistant tail":            {user, assistant, assistant},
		"tool without call":         {user, assistant, cacheWriteEvidenceResult("c")},
		"changed output":            {user, cacheWriteEvidenceMessage("assistant", "other"), nextUser},
		"different branch":          {cacheWriteEvidenceMessage("user", "sibling"), assistant, nextUser},
		"truncated history":         {assistant, nextUser},
		"changed role":              {cacheWriteEvidenceMessage("system", "question"), assistant, nextUser},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(input...))
			if openAICacheWriteDirectSuccessor(previous, output, candidate) {
				t.Fatal("unsafe lineage was accepted")
			}
		})
	}
	if openAICacheWriteDirectSuccessor(previous, openAICacheWriteOutputEvidence{}, next) {
		t.Fatal("unknown output must fail")
	}
	empty := buildOpenAICacheWriteOutputEvidence(cacheWriteEvidenceResponse())
	if !openAICacheWriteDirectSuccessor(previous, empty, buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(user, nextUser))) {
		t.Fatal("explicit completed empty output is different from missing output")
	}
}

func TestOpenAICacheWriteEvidence_ToolBatchMustExactlyMatchOutput(t *testing.T) {
	t.Parallel()
	user := cacheWriteEvidenceMessage("user", "question")
	c1, c2 := cacheWriteEvidenceCall("call-1"), cacheWriteEvidenceCall("call-2")
	previous := buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(user))
	output := buildOpenAICacheWriteOutputEvidence(cacheWriteEvidenceResponse(c1, c2))
	for name, tail := range map[string][]any{
		"original order": {cacheWriteEvidenceResult("call-1"), cacheWriteEvidenceResult("call-2")},
		"reverse order":  {cacheWriteEvidenceResult("call-2"), cacheWriteEvidenceResult("call-1")},
	} {
		t.Run(name, func(t *testing.T) {
			next := buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(append([]any{user, c1, c2}, tail...)...))
			if !openAICacheWriteDirectSuccessor(previous, output, next) {
				t.Fatal("one complete result batch should match")
			}
		})
	}
	for name, tail := range map[string][]any{
		"missing result":   {cacheWriteEvidenceResult("call-1")},
		"duplicate result": {cacheWriteEvidenceResult("call-1"), cacheWriteEvidenceResult("call-1")},
		"different ID":     {cacheWriteEvidenceResult("call-1"), cacheWriteEvidenceResult("call-x")},
		"extra result":     {cacheWriteEvidenceResult("call-1"), cacheWriteEvidenceResult("call-2"), cacheWriteEvidenceResult("call-x")},
		"user interrupts":  {cacheWriteEvidenceMessage("user", "continue")},
		"extra user turn":  {cacheWriteEvidenceResult("call-1"), cacheWriteEvidenceResult("call-2"), cacheWriteEvidenceMessage("user", "continue")},
		"assistant after":  {cacheWriteEvidenceResult("call-1"), cacheWriteEvidenceResult("call-2"), cacheWriteEvidenceMessage("assistant", "answer")},
	} {
		t.Run(name, func(t *testing.T) {
			next := buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(append([]any{user, c1, c2}, tail...)...))
			if openAICacheWriteDirectSuccessor(previous, output, next) {
				t.Fatal("unsafe tool-result batch was accepted")
			}
		})
	}
	duplicateOutput := buildOpenAICacheWriteOutputEvidence(cacheWriteEvidenceResponse(c1, c1))
	next := buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(user, c1, c1, cacheWriteEvidenceResult("call-1")))
	if openAICacheWriteDirectSuccessor(previous, duplicateOutput, next) {
		t.Fatal("duplicate output call IDs must be rejected")
	}
}

func TestOpenAICacheWriteEvidence_EncryptedReasoningRequiresExactReplay(t *testing.T) {
	t.Parallel()
	user := cacheWriteEvidenceMessage("user", "question")
	assistant := cacheWriteEvidenceMessage("assistant", "answer")
	last := cacheWriteEvidenceMessage("user", "continue")
	reasoning := map[string]any{"type": "reasoning", "id": "rs-not-retained", "encrypted_content": "secret-cipher", "summary": []any{map[string]any{"type": "summary_text", "text": "summary"}}}
	previous := buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(user))
	output := buildOpenAICacheWriteOutputEvidence(cacheWriteEvidenceResponse(reasoning, assistant))
	next := buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(user, reasoning, assistant, last))
	if !openAICacheWriteDirectSuccessor(previous, output, next) {
		t.Fatal("exact encrypted reasoning should replay")
	}
	for _, changed := range []any{
		map[string]any{"type": "reasoning", "encrypted_content": "other-cipher", "summary": reasoning["summary"]},
		map[string]any{"type": "reasoning", "encrypted_content": "secret-cipher", "summary": []any{}},
		map[string]any{"type": "reasoning", "encrypted_content": "secret-cipher"},
		map[string]any{"type": "reasoning", "summary": reasoning["summary"]},
	} {
		next := buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(user, changed, assistant, last))
		if openAICacheWriteDirectSuccessor(previous, output, next) {
			t.Fatal("changed or missing encrypted reasoning must break lineage")
		}
	}
	if openAICacheWriteDirectSuccessor(previous, output, buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(user, assistant, last))) {
		t.Fatal("dropping reasoning cannot establish exact full-output replay")
	}
}

func TestOpenAICacheWriteEvidence_PreservesPhaseNamespaceAndArguments(t *testing.T) {
	t.Parallel()
	for _, pair := range [][2]any{
		{map[string]any{"role": "assistant", "content": "answer", "phase": "commentary"}, map[string]any{"role": "assistant", "content": "answer", "phase": "final_answer"}},
		{map[string]any{"type": "function_call", "call_id": "c", "name": "f", "namespace": "one", "arguments": "{}"}, map[string]any{"type": "function_call", "call_id": "c", "name": "f", "namespace": "two", "arguments": "{}"}},
		{map[string]any{"type": "function_call", "call_id": "c", "name": "f", "arguments": "{}"}, map[string]any{"type": "function_call", "call_id": "c", "name": "f", "arguments": "{ }"}},
	} {
		a := buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(pair[0]))
		b := buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(pair[1]))
		if !a.Valid || !b.Valid || openAICacheWritePromptsIdentical(a, b) {
			t.Fatal("known optional fields and argument bytes must remain distinct")
		}
	}
}

func TestOpenAICacheWriteEvidence_RetryEqualityIsNotSuccessorProof(t *testing.T) {
	t.Parallel()
	previous := buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(cacheWriteEvidenceMessage("user", "question")))
	if !openAICacheWritePromptsIdentical(previous, previous) {
		t.Fatal("identical request equality should not require output")
	}
	if openAICacheWriteDirectSuccessor(previous, buildOpenAICacheWriteOutputEvidence(cacheWriteEvidenceResponse()), previous) {
		t.Fatal("equality is not a new turn; tracker must require admission ordering")
	}
	for _, invalid := range []openAICacheWritePromptEvidence{
		{}, {Valid: true}, {Valid: true, ConfigHash: previous.ConfigHash},
		{Valid: true, ConfigHash: previous.ConfigHash, InputHashes: []string{""}},
	} {
		if openAICacheWritePromptsIdentical(previous, invalid) || openAICacheWritePromptsIdentical(invalid, invalid) {
			t.Fatal("missing hashes must not establish equality")
		}
	}
}

func TestOpenAICacheWriteEvidence_StrictJSON(t *testing.T) {
	t.Parallel()
	for name, body := range map[string][]byte{
		"duplicate root":      []byte(`{"model":"test-model","input":"one","input":"two"}`),
		"escaped duplicate":   []byte(`{"model":"test-model","input":"one","\u0069nput":"two"}`),
		"duplicate nested":    []byte(`{"model":"test-model","input":[{"role":"user","content":"one","content":"two"}]}`),
		"duplicate config":    []byte(`{"model":"test-model","input":"x","reasoning":{"effort":"low","effort":"high"}}`),
		"trailing value":      []byte(`{"model":"test-model","input":"x"}{}`),
		"trailing malformed":  []byte(`{"model":"test-model","input":"x"}garbage`),
		"bad UTF8":            append([]byte(`{"model":"test-model","input":"`), 0xff, '"', '}'),
		"lone high surrogate": []byte(`{"model":"test-model","input":"\ud800"}`),
		"lone low surrogate":  []byte(`{"model":"test-model","input":"\udc00"}`),
		"reversed surrogates": []byte(`{"model":"test-model","input":"\udc00\ud800"}`),
		"two high surrogates": []byte(`{"model":"test-model","input":"\ud800\ud800"}`),
		"bad Unicode escape":  []byte(`{"model":"test-model","input":"\uZZZZ"}`),
		"incomplete escape":   []byte(`{"model":"test-model","input":"\u123"}`),
		"not object":          []byte(`[]`),
	} {
		t.Run(name, func(t *testing.T) {
			if buildOpenAICacheWritePromptEvidence(body).Valid {
				t.Fatal("ambiguous JSON accepted")
			}
		})
	}
	for _, text := range []string{`"\ud83d\ude00"`, `"😀"`, `"\\ud800"`, `"\"quoted\""`, `"\ufffd"`} {
		if !buildOpenAICacheWritePromptEvidence([]byte(`{"model":"test-model","input":` + text + `}`)).Valid {
			t.Fatalf("valid Unicode/escaped input rejected: %s", text)
		}
	}
	a := buildOpenAICacheWritePromptEvidence([]byte(`{"model":"test-model","input":"\ud83d\ude00"}`))
	b := buildOpenAICacheWritePromptEvidence([]byte(`{"model":"test-model","input":"😀"}`))
	if !openAICacheWritePromptsIdentical(a, b) {
		t.Fatal("valid equivalent Unicode encodings should normalize")
	}
}

func TestOpenAICacheWriteEvidence_BoundedWorkAndMetadata(t *testing.T) {
	t.Parallel()
	items := make([]any, openAICacheWriteEvidenceMaxItems)
	for i := range items {
		items[i] = cacheWriteEvidenceMessage("assistant", "answer")
	}
	if !buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(items...)).Valid || !buildOpenAICacheWriteOutputEvidence(cacheWriteEvidenceResponse(items...)).Valid {
		t.Fatal("item cap boundary should remain supported")
	}
	items = append(items, cacheWriteEvidenceMessage("assistant", "one too many"))
	if buildOpenAICacheWritePromptEvidence(cacheWriteEvidenceRequest(items...)).Valid || buildOpenAICacheWriteOutputEvidence(cacheWriteEvidenceResponse(items...)).Valid {
		t.Fatal("oversized history/output should fail closed")
	}
	large := bytes.Repeat([]byte{'x'}, openAICacheWriteEvidenceMaxBodyBytes)
	if buildOpenAICacheWritePromptEvidence(append([]byte(`{"model":"test-model","input":"`), large...)).Valid || buildOpenAICacheWriteOutputEvidence(large).Valid {
		t.Fatal("oversized body should be rejected")
	}
	deep := `{"model":"test-model","input":"x","metadata":` + strings.Repeat(`[`, openAICacheWriteEvidenceMaxDepth+1) + `0` + strings.Repeat(`]`, openAICacheWriteEvidenceMaxDepth+1) + `}`
	if buildOpenAICacheWritePromptEvidence([]byte(deep)).Valid {
		t.Fatal("excessive JSON nesting should be rejected")
	}
	nodes := `{"model":"test-model","input":"x","metadata":[` + strings.Repeat(`0,`, openAICacheWriteEvidenceMaxNodes) + `0]}`
	if buildOpenAICacheWritePromptEvidence([]byte(nodes)).Valid {
		t.Fatal("excessive JSON node count should be rejected")
	}
}

func TestOpenAICacheWriteEvidence_RetainsHashesOnly(t *testing.T) {
	t.Parallel()
	secret := "DO-NOT-RETAIN-THIS-PRIVATE-TEXT"
	input := cacheWriteEvidenceRequest(cacheWriteEvidenceMessage("user", secret), cacheWriteEvidenceCall(secret), cacheWriteEvidenceResult(secret))
	prompt := buildOpenAICacheWritePromptEvidence(input)
	output := buildOpenAICacheWriteOutputEvidence(cacheWriteEvidenceResponse(cacheWriteEvidenceMessage("assistant", secret), cacheWriteEvidenceCall(secret)))
	if !prompt.Valid || !output.Valid {
		t.Fatal("fixture should produce evidence")
	}
	for _, value := range []any{prompt, output} {
		if strings.Contains(fmt.Sprintf("%+v", value), secret) {
			t.Fatal("evidence retained raw content or identifier")
		}
		cacheWriteEvidenceAssertHashStrings(t, reflect.ValueOf(value))
	}
	before := append([]string(nil), prompt.InputHashes...)
	for i := range input {
		input[i] = 'x'
	}
	if !reflect.DeepEqual(before, prompt.InputHashes) {
		t.Fatal("evidence must not alias original request bytes")
	}
}

func cacheWriteEvidenceAssertHashStrings(t *testing.T, value reflect.Value) {
	t.Helper()
	switch value.Kind() {
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			cacheWriteEvidenceAssertHashStrings(t, value.Field(i))
		}
	case reflect.Slice:
		for i := 0; i < value.Len(); i++ {
			cacheWriteEvidenceAssertHashStrings(t, value.Index(i))
		}
	case reflect.String:
		text := value.String()
		if text == "" {
			return
		}
		if decoded, err := hex.DecodeString(text); err != nil || len(decoded) != 32 {
			t.Fatal("evidence contains a non-SHA256 string")
		}
	case reflect.Bool, reflect.Uint8:
	default:
		t.Fatalf("unexpected retained evidence type: %s", value.Kind())
	}
}

func FuzzOpenAICacheWriteEvidence_NoPanicOrPlaintextRetention(f *testing.F) {
	for _, seed := range []string{
		`{"model":"m","input":"hello"}`,
		`{"output":[]}`,
		`{"model":"m","input":[{"type":"function_call","call_id":"c","name":"f","arguments":"{}"}]}`,
		`{"type":"response.completed","response":{"output":[{"role":"assistant","content":"hi"}]}}`,
		`{"model":"m","input":"hello","client_metadata":{"turn_id":"one","x-codex-turn-metadata":"{\"turn_id\":\"one\",\"turn_started_at_unix_ms\":1000,\"thread_id\":\"thread\"}"}}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		prompt := buildOpenAICacheWritePromptEvidence(body)
		output := buildOpenAICacheWriteOutputEvidence(body)
		cacheWriteEvidenceAssertHashStrings(t, reflect.ValueOf(prompt))
		cacheWriteEvidenceAssertHashStrings(t, reflect.ValueOf(output))
		_ = openAICacheWritePromptsIdentical(prompt, prompt)
		_ = openAICacheWriteDirectSuccessor(prompt, output, prompt)
	})
}
