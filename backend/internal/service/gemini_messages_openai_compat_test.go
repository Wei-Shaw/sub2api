package service

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestAnthropicMessagesToChatCompletions(t *testing.T) {
	body := []byte(`{
		"model": "claude-3-5-sonnet",
		"system": "You are helpful.",
		"max_tokens": 256,
		"messages": [
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi"},
			{"role": "user", "content": [{"type": "text", "text": "Bye"}]}
		]
	}`)
	out, err := anthropicMessagesToChatCompletions(body, "gpt-4o-mini", true)
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	r := gjson.ParseBytes(out)
	if r.Get("model").String() != "gpt-4o-mini" {
		t.Errorf("model = %q", r.Get("model").String())
	}
	if !r.Get("stream").Bool() || !r.Get("stream_options.include_usage").Bool() {
		t.Errorf("stream/include_usage not set: %s", out)
	}
	msgs := r.Get("messages").Array()
	if len(msgs) < 3 {
		t.Fatalf("expected >=3 messages, got %d: %s", len(msgs), out)
	}
	// 必须包含 system 角色消息
	hasSystem := false
	for _, m := range msgs {
		if m.Get("role").String() == "system" {
			hasSystem = true
		}
	}
	if !hasSystem {
		t.Errorf("system message missing: %s", out)
	}
}

func TestChatCompletionToAnthropicMessage_Text(t *testing.T) {
	cc := []byte(`{
		"choices": [{"index": 0, "message": {"role": "assistant", "content": "Hello world"}, "finish_reason": "stop"}],
		"usage": {"prompt_tokens": 12, "completion_tokens": 5, "total_tokens": 17}
	}`)
	out, usage, err := chatCompletionToAnthropicMessage(cc, "claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	r := gjson.ParseBytes(out)
	if r.Get("type").String() != "message" || r.Get("role").String() != "assistant" {
		t.Errorf("envelope wrong: %s", out)
	}
	if r.Get("content.0.type").String() != "text" || r.Get("content.0.text").String() != "Hello world" {
		t.Errorf("content wrong: %s", out)
	}
	if r.Get("stop_reason").String() != "end_turn" {
		t.Errorf("stop_reason = %s", r.Get("stop_reason").String())
	}
	if r.Get("usage.input_tokens").Int() != 12 || r.Get("usage.output_tokens").Int() != 5 {
		t.Errorf("usage wrong: %s", out)
	}
	if usage.OutputTokens != 5 {
		t.Errorf("usage struct = %+v", usage)
	}
}

func TestChatCompletionToAnthropicMessage_ToolUse(t *testing.T) {
	cc := []byte(`{
		"choices": [{"index": 0, "message": {"role": "assistant", "tool_calls": [
			{"id": "call_1", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"SF\"}"}}
		]}, "finish_reason": "tool_calls"}]
	}`)
	out, _, err := chatCompletionToAnthropicMessage(cc, "m")
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	r := gjson.ParseBytes(out)
	if r.Get("content.0.type").String() != "tool_use" {
		t.Errorf("expected tool_use block: %s", out)
	}
	if r.Get("content.0.name").String() != "get_weather" {
		t.Errorf("tool name = %s", r.Get("content.0.name").String())
	}
	if r.Get("content.0.input.city").String() != "SF" {
		t.Errorf("tool input = %s", r.Get("content.0.input").Raw)
	}
	if r.Get("content.0.id").String() != "call_1" {
		t.Errorf("tool id = %s", r.Get("content.0.id").String())
	}
	if r.Get("stop_reason").String() != "tool_use" {
		t.Errorf("stop_reason = %s", r.Get("stop_reason").String())
	}
}

func TestMapOpenAIFinishReasonToAnthropic(t *testing.T) {
	cases := map[string]string{
		"stop": "end_turn", "length": "max_tokens", "tool_calls": "tool_use",
		"function_call": "tool_use", "content_filter": "end_turn", "": "end_turn",
	}
	for in, want := range cases {
		if got := mapOpenAIFinishReasonToAnthropic(in); got != want {
			t.Errorf("map(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEstimateAnthropicInputTokens(t *testing.T) {
	body := []byte(`{
		"system": "system text here",
		"messages": [
			{"role": "user", "content": "a user message"},
			{"role": "assistant", "content": [{"type": "text", "text": "an assistant reply"}]}
		]
	}`)
	got := estimateAnthropicInputTokens(body)
	if got <= 0 {
		t.Errorf("expected positive estimate, got %d", got)
	}
}
