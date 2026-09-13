package keyprotection

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var writerToken = fmt.Sprintf("keyx_%x", sha256.Sum256([]byte(writerSecret)))

const writerSecret = "fictional-secret-\"quote\\slash\nnewline"

const writerPrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nZmFrZS1zdWIyYXBpLXNzaC1wcml2YXRlLWtleS1maXh0dXJl\nZmFrZS1wYXlsb2FkLW5vdC1hLXZhbGlkLWtleQ==\n-----END OPENSSH PRIVATE KEY-----"

func writerState(t *testing.T, fixtures ...string) *State {
	t.Helper()
	secret := writerSecret
	if len(fixtures) > 0 {
		secret = fixtures[0]
	}
	config := DefaultConfig()
	if secret == writerSecret {
		config.CustomRules = []Rule{{Name: "quoted_fixture", Pattern: regexp.QuoteMeta(secret)}}
	}
	return testState(t, config, secret)
}
func testWriter(t *testing.T, protocol string, stream bool, fixtures ...string) (*ResponseWriter, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	w := NewResponseWriter(ctx.Writer, writerState(t, fixtures...), protocol)
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	return w, recorder
}
func frame(event string, obj any) string {
	data, _ := json.Marshal(obj)
	prefix := ""
	if event != "" {
		prefix = "event: " + event + "\n"
	}
	return prefix + "data: " + string(data) + "\n\n"
}
func chatDelta(choice int, field string, value any) string {
	return frame("", map[string]any{"id": "chat_test", "choices": []any{map[string]any{"index": choice, "delta": map[string]any{field: value}, "finish_reason": nil}}})
}
func finishChat() string { return "data: [DONE]\n\n" }
func events(t *testing.T, body string) []map[string]any {
	t.Helper()
	var result []map[string]any
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
			continue
		}
		obj, err := decodeObject([]byte(strings.TrimPrefix(line, "data: ")))
		require.NoError(t, err)
		result = append(result, obj)
	}
	return result
}
func joinedText(t *testing.T, body, protocol string) string {
	t.Helper()
	var out strings.Builder
	for _, obj := range events(t, body) {
		switch protocol {
		case "chat":
			choices, _ := obj["choices"].([]any)
			for _, raw := range choices {
				choice := raw.(map[string]any)
				delta, _ := choice["delta"].(map[string]any)
				out.WriteString(str(delta["content"]))
			}
		case "responses":
			if str(obj["type"]) == "response.output_text.delta" {
				out.WriteString(str(obj["delta"]))
			}
		case "messages":
			if delta, ok := obj["delta"].(map[string]any); ok {
				out.WriteString(str(delta["text"]))
			}
		}
	}
	return out.String()
}

func TestProtectedWriterJSONProtocols(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		t.Run(protocol, func(t *testing.T) {
			w, recorder := testWriter(t, protocol, false)
			var body any
			switch protocol {
			case "chat":
				body = map[string]any{"id": "response_test", "choices": []any{map[string]any{"message": map[string]any{"content": writerToken, "tool_calls": []any{map[string]any{"function": map[string]any{"arguments": `{"key":"` + writerToken + `","n":9007199254740993}`}}}}}}}
			case "responses":
				body = map[string]any{"id": "response_test", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": writerToken}}}, map[string]any{"type": "function_call", "arguments": `{"key":"` + writerToken + `"}`}}}
			case "messages":
				body = map[string]any{"id": "response_test", "content": []any{map[string]any{"type": "text", "text": writerToken}, map[string]any{"type": "tool_use", "input": map[string]any{"key": writerToken}}}}
			}
			data, _ := json.Marshal(body)
			w.Header().Set("Content-Length", "1")
			_, err := w.Write(data)
			require.NoError(t, err)
			require.Empty(t, recorder.Body.String())
			require.NoError(t, w.Finish())
			require.NotContains(t, recorder.Body.String(), writerToken)
			var decoded any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &decoded))
			require.Empty(t, recorder.Header().Get("Content-Length"))
			if protocol == "chat" {
				require.Contains(t, recorder.Body.String(), "9007199254740993")
			}
		})
	}
}

func TestProtectedWriterTextEveryPlaceholderAndNetworkSplit(t *testing.T) {
	for _, secret := range []string{writerSecret, writerPrivateKey, strings.ReplaceAll(writerPrivateKey, "\n", "\r\n")} {
		writerToken := fmt.Sprintf("keyx_%x", sha256.Sum256([]byte(secret)))
		for _, protocol := range []string{"chat", "responses", "messages"} {
			for split := 0; split <= len(writerToken); split++ {
				w, recorder := testWriter(t, protocol, true, secret)
				parts := []string{"hello " + writerToken[:split], writerToken[split:] + " 世界"}
				var input string
				for _, part := range parts {
					switch protocol {
					case "chat":
						input += chatDelta(0, "content", part)
					case "responses":
						input += frame("response.output_text.delta", map[string]any{"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "delta": part})
					case "messages":
						input += frame("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": part}})
					}
				}
				switch protocol {
				case "chat":
					input += finishChat()
				case "responses":
					input += frame("response.completed", map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp_test", "output": []any{}}})
				case "messages":
					input += frame("message_stop", map[string]any{"type": "message_stop"})
				}
				// Byte-at-a-time delivery splits every SSE field, JSON escape and UTF-8 sequence.
				for i := range []byte(input) {
					_, err := w.Write([]byte{input[i]})
					require.NoError(t, err, "%s split %d", protocol, split)
				}
				require.NoError(t, w.Finish())
				require.Equal(t, "hello "+secret+" 世界", joinedText(t, recorder.Body.String(), protocol), "%s split %d", protocol, split)
			}
		}

	}
}

func TestProtectedWriterPlaceholderBoundariesAndForgery(t *testing.T) {
	for _, source := range []string{writerToken + "suffix", "prefix" + writerToken, writerToken + "-x", "_" + writerToken, writerToken[:len(writerToken)-1], "keyx_ffffffffffffffffffffffffffffffff", writerToken + "!", writerToken} {
		for split := 0; split <= len(source); split++ {
			w, recorder := testWriter(t, "chat", true)
			_, err := w.WriteString(chatDelta(0, "content", source[:split]) + chatDelta(0, "content", source[split:]) + finishChat())
			require.NoError(t, err)
			require.NoError(t, w.Finish())
			require.Equal(t, w.state.RestoreText(source), joinedText(t, recorder.Body.String(), "chat"), "split %d", split)
		}
	}
}

func TestProtectedWriterInterleavedToolArguments(t *testing.T) {
	for _, secret := range []string{writerSecret, writerPrivateKey, strings.ReplaceAll(writerPrivateKey, "\n", "\r\n")} {
		writerToken := fmt.Sprintf("keyx_%x", sha256.Sum256([]byte(secret)))
		w, recorder := testWriter(t, "chat", true, secret)
		tool := func(index int, arg string) string {
			return chatDelta(0, "tool_calls", []any{map[string]any{"index": index, "function": map[string]any{"arguments": arg}}})
		}
		_, err := w.WriteString(tool(0, `{"key":"`+writerToken[:13]) + tool(1, `{"other":"`+writerToken) + chatDelta(1, "content", "independent text"))
		require.NoError(t, err)
		require.Contains(t, recorder.Body.String(), "independent text")
		require.NotContains(t, recorder.Body.String(), "fictional-secret")
		_, err = w.WriteString(tool(0, writerToken[13:]+`","n":9007199254740993}`) + tool(1, `"}`) + finishChat())
		require.NoError(t, err)
		require.NoError(t, w.Finish())
		arguments := map[string]string{}
		for _, obj := range events(t, recorder.Body.String()) {
			for _, raw := range obj["choices"].([]any) {
				delta := raw.(map[string]any)["delta"].(map[string]any)
				calls, _ := delta["tool_calls"].([]any)
				for _, rawCall := range calls {
					call := rawCall.(map[string]any)
					arguments[idx(call["index"])] += str(call["function"].(map[string]any)["arguments"])
				}
			}
		}
		for _, value := range arguments {
			obj, err := decodeObject([]byte(value))
			require.NoError(t, err)
			if key, ok := obj["key"]; ok {
				require.Equal(t, secret, key)
				require.Equal(t, json.Number("9007199254740993"), obj["n"])
			} else {
				require.Equal(t, secret, obj["other"])
			}
		}
		require.Len(t, arguments, 2)

	}
}

func TestProtectedWriterBoundedTailAndCancellation(t *testing.T) {
	w, recorder := testWriter(t, "chat", true)
	_, err := w.WriteString(chatDelta(0, "content", strings.Repeat("normal ", 1000)+writerToken))
	require.NoError(t, err)
	require.Contains(t, recorder.Body.String(), "normal ")
	require.LessOrEqual(t, len(w.stream.channels["chat:0:content"].pending), PlaceholderLength)
	w.Abort()
	require.Error(t, w.Finish())
	require.NotContains(t, recorder.Body.String(), "fictional-secret")
}

func TestProtectedWriterFailureIsFixedAndNoUncheckedTail(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		w, recorder := testWriter(t, protocol, true)
		_, err := w.WriteString(": ping\n\n")
		require.NoError(t, err)
		_, err = w.WriteString("data: {broken fictional-private-value}\n\n")
		require.Error(t, err)
		require.Error(t, w.Finish())
		require.NotContains(t, recorder.Body.String(), "fictional-private-value")
		require.Contains(t, recorder.Body.String(), "key_protection_failed")
	}
	w, recorder := testWriter(t, "chat", false)
	w.Header().Set("Content-Encoding", "gzip")
	_, err := w.Write([]byte("compressed-secret"))
	require.Error(t, err)
	require.Equal(t, 502, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "compressed-secret")
	w, recorder = testWriter(t, "chat", true)
	_, err = w.WriteString(chatDelta(0, "content", writerToken))
	require.NoError(t, err)
	require.Error(t, w.Finish())
	require.NotContains(t, recorder.Body.String(), "fictional-secret")
}

func TestProtectedWriterResponsesAndMessagesToolsEverySplit(t *testing.T) {
	for _, secret := range []string{writerSecret, writerPrivateKey, strings.ReplaceAll(writerPrivateKey, "\n", "\r\n")} {
		writerToken := fmt.Sprintf("keyx_%x", sha256.Sum256([]byte(secret)))
		arguments := `{"key":"` + writerToken + `","quoted":"a\\b\"c","n":9007199254740993}`
		for _, protocol := range []string{"responses", "messages"} {
			for split := 0; split <= len(arguments); split++ {
				w, recorder := testWriter(t, protocol, true, secret)
				var input string
				for _, part := range []string{arguments[:split], arguments[split:]} {
					if protocol == "responses" {
						input += frame("response.function_call_arguments.delta", map[string]any{"type": "response.function_call_arguments.delta", "item_id": "call_1", "output_index": 1, "delta": part})
					} else {
						input += frame("content_block_delta", map[string]any{"type": "content_block_delta", "index": 1, "delta": map[string]any{"type": "input_json_delta", "partial_json": part}})
					}
				}
				if protocol == "responses" {
					input += frame("response.function_call_arguments.done", map[string]any{"type": "response.function_call_arguments.done", "item_id": "call_1", "output_index": 1, "arguments": arguments})
					input += frame("response.completed", map[string]any{"type": "response.completed", "response": map[string]any{"id": "response_1", "output": []any{map[string]any{"type": "function_call", "arguments": arguments}}}}) + "data: [DONE]\n\n"
				} else {
					input += frame("content_block_stop", map[string]any{"type": "content_block_stop", "index": 1}) + frame("message_stop", map[string]any{"type": "message_stop"})
				}
				_, err := w.WriteString(input)
				require.NoError(t, err, "%s split %d", protocol, split)
				require.NoError(t, w.Finish())
				var got string
				for _, obj := range events(t, recorder.Body.String()) {
					if protocol == "responses" {
						switch str(obj["type"]) {
						case "response.function_call_arguments.delta":
							got += str(obj["delta"])
						case "response.function_call_arguments.done":
							require.Equal(t, got, str(obj["arguments"]))
						case "response.completed":
							require.Equal(t, got, str(obj["response"].(map[string]any)["output"].([]any)[0].(map[string]any)["arguments"]))
						}
					} else if delta, ok := obj["delta"].(map[string]any); ok {
						got += str(delta["partial_json"])
					}
				}
				obj, err := decodeObject([]byte(got))
				require.NoError(t, err)
				require.Equal(t, secret, obj["key"])
				require.Equal(t, json.Number("9007199254740993"), obj["n"])
			}
		}

	}
}

func TestProtectedWriterSeparateChoicesAndTerminalContent(t *testing.T) {
	w, recorder := testWriter(t, "chat", true)
	input := chatDelta(0, "content", writerToken[:20]) + chatDelta(1, "content", writerToken[20:])
	input += frame("", map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": writerToken[20:] + " and " + writerToken}, "finish_reason": "stop"}}}) + finishChat()
	_, err := w.WriteString(input)
	require.NoError(t, err)
	require.NoError(t, w.Finish())
	byChoice := map[string]string{}
	for _, obj := range events(t, recorder.Body.String()) {
		for _, raw := range obj["choices"].([]any) {
			choice := raw.(map[string]any)
			byChoice[idx(choice["index"])] += str(choice["delta"].(map[string]any)["content"])
		}
	}
	require.Equal(t, writerSecret+" and "+writerSecret, byChoice["0"])
	require.Equal(t, writerToken[20:], byChoice["1"])
}

func TestProtectedWriterSSELineEndingsAndStrictParsing(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n", "\r"} {
		w, recorder := testWriter(t, "chat", true)
		input := strings.ReplaceAll(chatDelta(0, "content", writerToken)+finishChat(), "\n", ending)
		for i := range input {
			_, err := w.WriteString(input[i : i+1])
			require.NoError(t, err)
		}
		require.NoError(t, w.Finish())
		require.Equal(t, writerSecret, joinedText(t, recorder.Body.String(), "chat"))
	}
	w, recorder := testWriter(t, "chat", true)
	_, err := w.WriteString("data: {\"choices\":[],\"choices\":[]}\n\n")
	require.Error(t, err)
	require.Contains(t, recorder.Body.String(), "key_protection_failed")
	w, recorder = testWriter(t, "responses", true)
	_, err = w.WriteString("data: [DONE]\n\n")
	require.Error(t, err)
	require.Contains(t, recorder.Body.String(), "key_protection_failed")
	w, recorder = testWriter(t, "responses", true)
	_, err = w.WriteString(frame("response.failed", map[string]any{"type": "response.failed", "response": map[string]any{"status": "failed"}}) + "data: [DONE]\n\n")
	require.NoError(t, err)
	require.NoError(t, w.Finish())
}

func TestProtectedWriterAnthropicStartTextContinuesAcrossDelta(t *testing.T) {
	w, recorder := testWriter(t, "messages", true)
	input := frame("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": "prefix " + writerToken[:18]}})
	input += frame("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": writerToken[18:]}})
	input += frame("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0}) + frame("message_stop", map[string]any{"type": "message_stop"})
	_, err := w.WriteString(input)
	require.NoError(t, err)
	require.NoError(t, w.Finish())
	all := events(t, recorder.Body.String())
	require.Equal(t, "prefix ", str(all[0]["content_block"].(map[string]any)["text"]))
	require.Equal(t, writerSecret, joinedText(t, recorder.Body.String(), "messages"))
}

func TestProtectedWriterCustomToolsEverySplit(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		for split := 0; split <= len(writerToken); split++ {
			w, recorder := testWriter(t, protocol, true)
			var input string
			for _, part := range []string{"curl " + writerToken[:split], writerToken[split:]} {
				if protocol == "chat" {
					input += chatDelta(0, "tool_calls", []any{map[string]any{"index": 0, "custom": map[string]any{"input": part}}})
				} else {
					input += frame("response.custom_tool_call_input.delta", map[string]any{"type": "response.custom_tool_call_input.delta", "output_index": 0, "delta": part, "sequence_number": 1})
				}
			}
			if protocol == "chat" {
				input += finishChat()
			} else {
				input += frame("response.custom_tool_call_input.done", map[string]any{"type": "response.custom_tool_call_input.done", "output_index": 0, "input": "curl " + writerToken, "sequence_number": 2})
				input += frame("response.completed", map[string]any{"type": "response.completed", "sequence_number": 3, "response": map[string]any{"output": []any{map[string]any{"type": "custom_tool_call", "input": "curl " + writerToken}}}})
			}
			_, err := w.WriteString(input)
			require.NoError(t, err)
			require.NoError(t, w.Finish())
			var got string
			var sequence int64
			for _, obj := range events(t, recorder.Body.String()) {
				if protocol == "chat" {
					for _, raw := range obj["choices"].([]any) {
						delta := raw.(map[string]any)["delta"].(map[string]any)
						calls, _ := delta["tool_calls"].([]any)
						for _, rawCall := range calls {
							got += str(rawCall.(map[string]any)["custom"].(map[string]any)["input"])
						}
					}
				} else {
					if seq, ok := obj["sequence_number"].(json.Number); ok {
						next, err := seq.Int64()
						require.NoError(t, err)
						require.Greater(t, next, sequence)
						sequence = next
					}
					switch str(obj["type"]) {
					case "response.custom_tool_call_input.delta":
						got += str(obj["delta"])
					case "response.custom_tool_call_input.done":
						require.Equal(t, got, str(obj["input"]))
					case "response.completed":
						require.Equal(t, got, str(obj["response"].(map[string]any)["output"].([]any)[0].(map[string]any)["input"]))
					}
				}
			}
			require.Equal(t, "curl "+writerSecret, got)
		}
	}
}

func TestProtectedWriterRejectsConflictingFinalAndUnknownDelta(t *testing.T) {
	w, recorder := testWriter(t, "responses", true)
	input := frame("response.output_text.delta", map[string]any{"type": "response.output_text.delta", "delta": writerToken, "output_index": 0, "content_index": 0})
	input += frame("response.output_text.done", map[string]any{"type": "response.output_text.done", "text": "different " + writerToken, "output_index": 0, "content_index": 0})
	_, err := w.WriteString(input)
	require.Error(t, err)
	require.NotContains(t, recorder.Body.String(), "fictional-secret")
	w, recorder = testWriter(t, "responses", true)
	_, err = w.WriteString(frame("response.unknown.delta", map[string]any{"type": "response.unknown.delta", "delta": writerToken}))
	require.Error(t, err)
	require.Contains(t, recorder.Body.String(), "key_protection_failed")
}

func TestProtectedWriterChecksExpansionBeforeRestoration(t *testing.T) {
	w, recorder := testWriter(t, "chat", false)
	state := testState(t, Config{CustomRules: []Rule{{Name: "large_fixture", Pattern: `(?:fake)+`}}})
	token, err := state.ProtectText(strings.Repeat("fake", 256*1024))
	require.NoError(t, err)
	w.state = state
	body := map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": strings.Repeat(token+" ", 100)}}}}
	data, _ := json.Marshal(body)
	_, err = w.Write(data)
	require.NoError(t, err)
	require.Error(t, w.Finish())
	require.Less(t, recorder.Body.Len(), 1024)
}

func TestProtectedWriterDisablesDownstreamCachesAndBoundsChannelIDs(t *testing.T) {
	w, recorder := testWriter(t, "chat", true)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, err := w.WriteString(chatDelta(0, "content", "safe"))
	require.NoError(t, err)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, err = w.WriteString(finishChat())
	require.NoError(t, err)
	require.NoError(t, w.Finish())
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	w, recorder = testWriter(t, "responses", true)
	_, err = w.WriteString(frame("response.output_text.delta", map[string]any{"type": "response.output_text.delta", "delta": writerToken, "output_index": strings.Repeat("x", 1024)}))
	require.Error(t, err)
	require.Contains(t, recorder.Body.String(), "key_protection_failed")
}
