package kiro

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTransformOpenAIRequest_TextOnly(t *testing.T) {
	req := &OpenAIRequest{
		Model: "claude-sonnet-4-6",
		Messages: []OpenAIMessage{
			{Role: "user", Content: "hello"},
		},
		MaxTokens: 100,
	}
	p, err := TransformOpenAIRequest(req, "arn")
	if err != nil {
		t.Fatal(err)
	}
	if p.ConversationState.CurrentMessage.UserInputMessage.ModelID != "claude-sonnet-4.6" {
		t.Fatalf("model = %q", p.ConversationState.CurrentMessage.UserInputMessage.ModelID)
	}
	if !strings.Contains(p.ConversationState.CurrentMessage.UserInputMessage.Content, "hello") {
		t.Fatalf("content missing user text: %q", p.ConversationState.CurrentMessage.UserInputMessage.Content)
	}
	if p.InferenceConfig == nil || p.InferenceConfig.MaxTokens != 100 {
		t.Fatalf("inference config = %+v", p.InferenceConfig)
	}
}

func TestTransformOpenAIRequest_SystemMergedIntoFirstUser(t *testing.T) {
	req := &OpenAIRequest{
		Model: "claude-sonnet-4-6",
		Messages: []OpenAIMessage{
			{Role: "system", Content: "be concise"},
			{Role: "user", Content: "hi"},
		},
	}
	p, _ := TransformOpenAIRequest(req, "")
	c := p.ConversationState.CurrentMessage.UserInputMessage.Content
	if !strings.Contains(c, "be concise") || !strings.Contains(c, "hi") {
		t.Fatalf("system+user not merged: %q", c)
	}
}

func TestTransformOpenAIRequest_MultiSystemConcatenated(t *testing.T) {
	req := &OpenAIRequest{
		Model: "claude-sonnet-4-6",
		Messages: []OpenAIMessage{
			{Role: "system", Content: "policy one"},
			{Role: "system", Content: "policy two"},
			{Role: "user", Content: "go"},
		},
	}
	p, _ := TransformOpenAIRequest(req, "")
	c := p.ConversationState.CurrentMessage.UserInputMessage.Content
	if !strings.Contains(c, "policy one") || !strings.Contains(c, "policy two") {
		t.Fatalf("multi-system not concatenated: %q", c)
	}
}

func TestTransformOpenAIRequest_ThinkingSuffix(t *testing.T) {
	req := &OpenAIRequest{
		Model:    "claude-sonnet-4-6-thinking",
		Messages: []OpenAIMessage{{Role: "user", Content: "x"}},
	}
	p, _ := TransformOpenAIRequest(req, "")
	if !strings.Contains(p.ConversationState.CurrentMessage.UserInputMessage.Content, "<thinking_mode>enabled") {
		t.Fatalf("thinking prompt missing: %q", p.ConversationState.CurrentMessage.UserInputMessage.Content)
	}
}

func TestTransformOpenAIRequest_AssistantToolCallHistory(t *testing.T) {
	req := &OpenAIRequest{
		Model: "claude-sonnet-4-6",
		Messages: []OpenAIMessage{
			{Role: "user", Content: "use the tool"},
			{Role: "assistant", ToolCalls: []OpenAIToolCall{{
				ID:   "tc_1",
				Type: "function",
				Function: OpenAIToolCallFunction{
					Name:      "lookup",
					Arguments: `{"q":"42"}`,
				},
			}}},
			{Role: "tool", ToolCallID: "tc_1", Content: "answer = 42"},
			{Role: "user", Content: "thanks"},
		},
	}
	p, _ := TransformOpenAIRequest(req, "")
	if len(p.ConversationState.History) < 2 {
		t.Fatalf("history too short: %+v", p.ConversationState.History)
	}
	// History[1] should be the assistant turn with tool_uses.
	var assistantIdx int = -1
	for i, h := range p.ConversationState.History {
		if h.AssistantResponseMessage != nil {
			assistantIdx = i
			break
		}
	}
	if assistantIdx == -1 {
		t.Fatal("assistant turn missing")
	}
	asst := p.ConversationState.History[assistantIdx].AssistantResponseMessage
	if len(asst.ToolUses) != 1 || asst.ToolUses[0].Name != "lookup" {
		t.Fatalf("tool use not preserved: %+v", asst.ToolUses)
	}
	if asst.ToolUses[0].Input["q"] != "42" {
		t.Fatalf("tool input not decoded: %+v", asst.ToolUses[0].Input)
	}
}

func TestTransformOpenAIRequest_ToolsSanitized(t *testing.T) {
	req := &OpenAIRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []OpenAIMessage{{Role: "user", Content: "x"}},
		Tools: []OpenAITool{{
			Type: "function",
			Function: OpenAIToolFunction{
				Name:        "list_files",
				Description: "list",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	}
	p, _ := TransformOpenAIRequest(req, "")
	tools := p.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools
	if len(tools) != 1 || tools[0].ToolSpecification.Name != "listFiles" {
		t.Fatalf("tool sanitization failed: %+v", tools)
	}
	if p.ToolNameMap["listFiles"] != "list_files" {
		t.Fatalf("tool name map = %+v", p.ToolNameMap)
	}
}

func TestTransformOpenAIRequest_ImageURL(t *testing.T) {
	req := &OpenAIRequest{
		Model: "claude-sonnet-4-6",
		Messages: []OpenAIMessage{{
			Role: "user",
			Content: []any{
				map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "data:image/png;base64,iVBORw0KGgo=",
					},
				},
				map[string]any{"type": "text", "text": "what?"},
			},
		}},
	}
	p, _ := TransformOpenAIRequest(req, "")
	imgs := p.ConversationState.CurrentMessage.UserInputMessage.Images
	if len(imgs) != 1 || imgs[0].Format != "png" {
		t.Fatalf("image not parsed: %+v", imgs)
	}
}

func TestParseDataURL_ValidBase64(t *testing.T) {
	img := parseDataURL("data:image/jpeg;base64,abc123")
	if img == nil || img.Format != "jpeg" || img.Source.Bytes != "abc123" {
		t.Fatalf("parse failed: %+v", img)
	}
}

func TestParseDataURL_NotDataURL(t *testing.T) {
	if parseDataURL("https://example.com/x.png") != nil {
		t.Fatal("https URL should not parse as data URL")
	}
}

func TestOpenAISSEWriter_TextStreamFlow(t *testing.T) {
	var sb strings.Builder
	w := NewOpenAISSEWriter(&sb, nil, "m")
	w.WriteText("hello", false)
	w.WriteText(" world", false)
	w.WriteFinal(3, 2)

	out := sb.String()
	if !strings.Contains(out, "\"content\":\"hello\"") {
		t.Fatalf("first chunk missing: %s", out)
	}
	if !strings.Contains(out, "\"content\":\" world\"") {
		t.Fatalf("second chunk missing: %s", out)
	}
	if !strings.Contains(out, "\"finish_reason\":\"stop\"") {
		t.Fatalf("finish_reason missing: %s", out)
	}
	if !strings.Contains(out, "data: [DONE]") {
		t.Fatalf("[DONE] missing: %s", out)
	}
	if !strings.Contains(out, "\"prompt_tokens\":3") {
		t.Fatalf("usage missing: %s", out)
	}
}

func TestOpenAISSEWriter_ToolCall(t *testing.T) {
	var sb strings.Builder
	w := NewOpenAISSEWriter(&sb, nil, "m")
	w.WriteToolUse(ToolUse{
		ToolUseID: "tu_1",
		Name:      "list_files",
		Input:     map[string]any{"path": "/tmp"},
	})
	w.WriteFinal(0, 0)
	out := sb.String()
	if !strings.Contains(out, "\"tool_calls\"") {
		t.Fatalf("tool_calls missing: %s", out)
	}
	if !strings.Contains(out, "\"name\":\"list_files\"") {
		t.Fatalf("tool name missing: %s", out)
	}
}

func TestOpenAISSEWriter_ReasoningContent(t *testing.T) {
	var sb strings.Builder
	w := NewOpenAISSEWriter(&sb, nil, "m")
	w.WriteText("let me think", true)
	w.WriteText("answer", false)
	w.WriteFinal(0, 0)
	out := sb.String()
	if !strings.Contains(out, "\"reasoning_content\":\"let me think\"") {
		t.Fatalf("reasoning chunk missing: %s", out)
	}
	if !strings.Contains(out, "\"content\":\"answer\"") {
		t.Fatalf("answer chunk missing: %s", out)
	}
}

func TestBuildOpenAINonStreamingResponse_Aggregates(t *testing.T) {
	events := []Event{
		{Type: "assistantResponseEvent", Payload: map[string]any{"content": "hi"}},
		{Type: "assistantResponseEvent", Payload: map[string]any{"content": "hi there"}},
		{Type: "toolUseEvent", Payload: map[string]any{
			"toolUseId": "tu_1", "name": "f", "input": `{"a":1}`, "stop": true,
		}},
		{Type: "meteringEvent", Payload: map[string]any{
			"usage": map[string]any{"inputTokens": 3.0, "outputTokens": 2.0},
		}},
	}
	resp := BuildOpenAINonStreamingResponse(events, "m", nil)
	if resp.Object != "chat.completion" {
		t.Fatalf("object = %q", resp.Object)
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("finish = %q", resp.Choices[0].FinishReason)
	}
	if resp.Choices[0].Message.Content != "hi there" {
		t.Fatalf("content = %q", resp.Choices[0].Message.Content)
	}
	if len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d", len(resp.Choices[0].Message.ToolCalls))
	}
	if resp.Usage["prompt_tokens"] != 3 || resp.Usage["completion_tokens"] != 2 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
}

func TestOpenAINonStreamingResponse_JSONShape(t *testing.T) {
	resp := BuildOpenAINonStreamingResponse([]Event{
		{Type: "assistantResponseEvent", Payload: map[string]any{"content": "ok"}},
	}, "m", nil)
	b, _ := json.Marshal(resp)
	if !strings.Contains(string(b), "\"object\":\"chat.completion\"") {
		t.Fatalf("JSON malformed: %s", string(b))
	}
}
