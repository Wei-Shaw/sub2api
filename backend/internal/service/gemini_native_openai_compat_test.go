package service

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

func TestGeminiNativeRequestToChatCompletions_TextAndConfig(t *testing.T) {
	body := []byte(`{
		"systemInstruction": {"parts": [{"text": "You are helpful."}]},
		"contents": [
			{"role": "user", "parts": [{"text": "Hello"}]},
			{"role": "model", "parts": [{"text": "Hi there"}]},
			{"role": "user", "parts": [{"text": "Bye"}]}
		],
		"generationConfig": {"temperature": 0.5, "topP": 0.9, "maxOutputTokens": 256, "stopSequences": ["STOP"]}
	}`)

	out, err := geminiNativeRequestToChatCompletions(body, "gpt-4o-mini", true)
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	r := gjson.ParseBytes(out)

	if r.Get("model").String() != "gpt-4o-mini" {
		t.Errorf("model = %q", r.Get("model").String())
	}
	if !r.Get("stream").Bool() {
		t.Errorf("stream should be true")
	}
	if !r.Get("stream_options.include_usage").Bool() {
		t.Errorf("include_usage should be true for stream")
	}
	msgs := r.Get("messages").Array()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages (system + 3), got %d: %s", len(msgs), out)
	}
	if msgs[0].Get("role").String() != "system" || msgs[0].Get("content").String() != "You are helpful." {
		t.Errorf("system message wrong: %s", msgs[0].Raw)
	}
	if msgs[1].Get("role").String() != "user" || msgs[1].Get("content").String() != "Hello" {
		t.Errorf("user message wrong: %s", msgs[1].Raw)
	}
	if msgs[2].Get("role").String() != "assistant" || msgs[2].Get("content").String() != "Hi there" {
		t.Errorf("assistant role mapping wrong: %s", msgs[2].Raw)
	}
	if r.Get("temperature").Float() != 0.5 {
		t.Errorf("temperature = %v", r.Get("temperature").Float())
	}
	if r.Get("top_p").Float() != 0.9 {
		t.Errorf("top_p = %v", r.Get("top_p").Float())
	}
	if r.Get("max_tokens").Int() != 256 {
		t.Errorf("max_tokens = %v", r.Get("max_tokens").Int())
	}
	if r.Get("stop.0").String() != "STOP" {
		t.Errorf("stop = %s", r.Get("stop").Raw)
	}
}

func TestGeminiNativeRequestToChatCompletions_Tools(t *testing.T) {
	body := []byte(`{
		"contents": [{"role": "user", "parts": [{"text": "weather?"}]}],
		"tools": [{"functionDeclarations": [
			{"name": "get_weather", "description": "Get weather", "parameters": {"type": "object", "properties": {"city": {"type": "string"}}}}
		]}]
	}`)
	out, err := geminiNativeRequestToChatCompletions(body, "m", false)
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	r := gjson.ParseBytes(out)
	if r.Get("tools.0.type").String() != "function" {
		t.Errorf("tool type = %s", r.Get("tools.0.type").String())
	}
	if r.Get("tools.0.function.name").String() != "get_weather" {
		t.Errorf("tool name = %s", r.Get("tools.0.function.name").String())
	}
	if r.Get("tools.0.function.parameters.type").String() != "object" {
		t.Errorf("tool parameters not preserved: %s", r.Get("tools.0.function").Raw)
	}
	// non-stream should not set stream_options
	if r.Get("stream_options").Exists() {
		t.Errorf("stream_options should be absent for non-stream")
	}
}

func TestChatCompletionToGeminiNativeResponse_Text(t *testing.T) {
	cc := []byte(`{
		"choices": [{"index": 0, "message": {"role": "assistant", "content": "Hello world"}, "finish_reason": "stop"}],
		"usage": {"prompt_tokens": 12, "completion_tokens": 5, "total_tokens": 17}
	}`)
	out, usage, err := chatCompletionToGeminiNativeResponse(cc, "gpt-4o-mini")
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	r := gjson.ParseBytes(out)
	if r.Get("candidates.0.content.role").String() != "model" {
		t.Errorf("role = %s", r.Get("candidates.0.content.role").String())
	}
	if r.Get("candidates.0.content.parts.0.text").String() != "Hello world" {
		t.Errorf("text = %s", r.Get("candidates.0.content.parts.0.text").String())
	}
	if r.Get("candidates.0.finishReason").String() != "STOP" {
		t.Errorf("finishReason = %s", r.Get("candidates.0.finishReason").String())
	}
	if r.Get("usageMetadata.promptTokenCount").Int() != 12 {
		t.Errorf("promptTokenCount = %d", r.Get("usageMetadata.promptTokenCount").Int())
	}
	if r.Get("usageMetadata.candidatesTokenCount").Int() != 5 {
		t.Errorf("candidatesTokenCount = %d", r.Get("usageMetadata.candidatesTokenCount").Int())
	}
	if r.Get("usageMetadata.totalTokenCount").Int() != 17 {
		t.Errorf("totalTokenCount = %d", r.Get("usageMetadata.totalTokenCount").Int())
	}
	if usage.OutputTokens != 5 || usage.InputTokens != 12 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestChatCompletionToGeminiNativeResponse_ToolCall(t *testing.T) {
	cc := []byte(`{
		"choices": [{"index": 0, "message": {"role": "assistant", "tool_calls": [
			{"id": "c1", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"SF\"}"}}
		]}, "finish_reason": "tool_calls"}],
		"usage": {"prompt_tokens": 8, "completion_tokens": 3, "total_tokens": 11}
	}`)
	out, _, err := chatCompletionToGeminiNativeResponse(cc, "m")
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	r := gjson.ParseBytes(out)
	fc := r.Get("candidates.0.content.parts.0.functionCall")
	if fc.Get("name").String() != "get_weather" {
		t.Errorf("functionCall name = %s", fc.Get("name").String())
	}
	if fc.Get("args.city").String() != "SF" {
		t.Errorf("functionCall args = %s", fc.Get("args").Raw)
	}
	if r.Get("candidates.0.finishReason").String() != "STOP" {
		t.Errorf("tool_calls finishReason should map to STOP, got %s", r.Get("candidates.0.finishReason").String())
	}
}

func TestBuildGeminiStreamChunk(t *testing.T) {
	// delta chunk
	chunk := buildGeminiStreamChunk("partial", "", nil)
	r := gjson.Parse(chunk)
	if r.Get("candidates.0.content.parts.0.text").String() != "partial" {
		t.Errorf("delta text = %s", chunk)
	}
	if r.Get("candidates.0.finishReason").Exists() {
		t.Errorf("delta chunk should not carry finishReason")
	}
	// final chunk with usage
	final := buildGeminiStreamChunk("", "STOP", &ClaudeUsage{InputTokens: 10, OutputTokens: 4})
	rf := gjson.Parse(final)
	if rf.Get("candidates.0.finishReason").String() != "STOP" {
		t.Errorf("final finishReason = %s", final)
	}
	if rf.Get("usageMetadata.totalTokenCount").Int() != 14 {
		t.Errorf("final usage total = %s", final)
	}
}

func TestMapOpenAIFinishReasonToGemini(t *testing.T) {
	cases := map[string]string{
		"stop": "STOP", "tool_calls": "STOP", "length": "MAX_TOKENS",
		"content_filter": "SAFETY", "": "", "weird": "STOP",
	}
	for in, want := range cases {
		if got := mapOpenAIFinishReasonToGemini(in); got != want {
			t.Errorf("map(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGeminiContentToChatMessage_Multimodal(t *testing.T) {
	body := []byte(`{"contents":[{"role":"user","parts":[
		{"text":"describe"},
		{"inlineData":{"mimeType":"image/png","data":"AAAA"}}
	]}]}`)
	out, err := geminiNativeRequestToChatCompletions(body, "m", false)
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	r := gjson.ParseBytes(out)
	content := r.Get("messages.0.content")
	if !content.IsArray() {
		t.Fatalf("multimodal content should be array: %s", content.Raw)
	}
	var parts []map[string]any
	_ = json.Unmarshal([]byte(content.Raw), &parts)
	if len(parts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(parts))
	}
	if r.Get("messages.0.content.1.image_url.url").String() != "data:image/png;base64,AAAA" {
		t.Errorf("image url = %s", r.Get("messages.0.content.1.image_url.url").String())
	}
}
