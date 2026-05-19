package kiro

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustUnmarshal(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTransformAnthropicRequest_TextOnly(t *testing.T) {
	req := &AnthropicRequest{
		Model: "claude-sonnet-4-6",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "hello kiro"},
		},
		MaxTokens: 1024,
	}
	p, err := TransformAnthropicRequest(req, "arn:profile")
	if err != nil {
		t.Fatal(err)
	}
	if p.ProfileARN != "arn:profile" {
		t.Fatalf("profile = %q", p.ProfileARN)
	}
	if p.ConversationState.CurrentMessage.UserInputMessage.ModelID != "claude-sonnet-4.6" {
		t.Fatalf("model = %q", p.ConversationState.CurrentMessage.UserInputMessage.ModelID)
	}
	if !strings.Contains(p.ConversationState.CurrentMessage.UserInputMessage.Content, "hello kiro") {
		t.Fatalf("content missing user text: %q", p.ConversationState.CurrentMessage.UserInputMessage.Content)
	}
	if p.InferenceConfig == nil || p.InferenceConfig.MaxTokens != 1024 {
		t.Fatalf("inference cfg wrong: %+v", p.InferenceConfig)
	}
	if p.ConversationState.History != nil {
		t.Fatalf("single-message request should have empty history")
	}
}

func TestTransformAnthropicRequest_ThinkingSuffixForcesThinking(t *testing.T) {
	req := &AnthropicRequest{
		Model: "claude-sonnet-4-6-thinking",
		Messages: []AnthropicMessage{{Role: "user", Content: "hi"}},
	}
	p, _ := TransformAnthropicRequest(req, "")
	if p.ConversationState.CurrentMessage.UserInputMessage.ModelID != "claude-sonnet-4.6" {
		t.Fatalf("model = %q (suffix should be stripped)", p.ConversationState.CurrentMessage.UserInputMessage.ModelID)
	}
	if !strings.Contains(p.ConversationState.CurrentMessage.UserInputMessage.Content, "<thinking_mode>enabled</thinking_mode>") {
		t.Fatalf("thinking prompt missing: %q", p.ConversationState.CurrentMessage.UserInputMessage.Content)
	}
}

func TestTransformAnthropicRequest_ThinkingConfigForcesThinking(t *testing.T) {
	req := &AnthropicRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []AnthropicMessage{{Role: "user", Content: "hi"}},
		Thinking: &AnthropicThinking{Type: "enabled"},
	}
	p, _ := TransformAnthropicRequest(req, "")
	if !strings.Contains(p.ConversationState.CurrentMessage.UserInputMessage.Content, "<thinking_mode>enabled</thinking_mode>") {
		t.Fatal("thinking prompt missing")
	}
}

func TestTransformAnthropicRequest_SystemPromptWrapped(t *testing.T) {
	req := &AnthropicRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []AnthropicMessage{{Role: "user", Content: "hi"}},
		System:   "You are helpful.",
	}
	p, _ := TransformAnthropicRequest(req, "")
	c := p.ConversationState.CurrentMessage.UserInputMessage.Content
	if !strings.Contains(c, "--- SYSTEM PROMPT ---") || !strings.Contains(c, "You are helpful.") {
		t.Fatalf("system prompt not wrapped: %q", c)
	}
}

func TestTransformAnthropicRequest_HistoryRolesAlternate(t *testing.T) {
	req := &AnthropicRequest{
		Model: "claude-sonnet-4-6",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "first user"},
			{Role: "assistant", Content: "first reply"},
			{Role: "user", Content: "second user"},
		},
	}
	p, _ := TransformAnthropicRequest(req, "")
	if len(p.ConversationState.History) != 2 {
		t.Fatalf("history len = %d, want 2", len(p.ConversationState.History))
	}
	if p.ConversationState.History[0].UserInputMessage == nil {
		t.Fatal("history[0] should be a user message")
	}
	if p.ConversationState.History[1].AssistantResponseMessage == nil {
		t.Fatal("history[1] should be an assistant message")
	}
	if !strings.Contains(p.ConversationState.CurrentMessage.UserInputMessage.Content, "second user") {
		t.Fatalf("trailing user content missing: %q", p.ConversationState.CurrentMessage.UserInputMessage.Content)
	}
}

func TestTransformAnthropicRequest_ToolsSanitizedAndMapped(t *testing.T) {
	req := &AnthropicRequest{
		Model: "claude-sonnet-4-6",
		Messages: []AnthropicMessage{{Role: "user", Content: "test"}},
		Tools: []AnthropicTool{
			{
				Name:        "list_files",
				Description: "List files in a directory",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string"},
					},
				},
			},
		},
	}
	p, _ := TransformAnthropicRequest(req, "")
	if p.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext == nil {
		t.Fatal("expected userInputMessageContext for tools")
	}
	tools := p.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools
	if len(tools) != 1 {
		t.Fatalf("tools len = %d", len(tools))
	}
	if tools[0].ToolSpecification.Name != "listFiles" {
		t.Fatalf("sanitized tool name = %q", tools[0].ToolSpecification.Name)
	}
	if p.ToolNameMap["listFiles"] != "list_files" {
		t.Fatalf("name map = %+v", p.ToolNameMap)
	}
}

func TestTransformAnthropicRequest_ImageBlock(t *testing.T) {
	req := &AnthropicRequest{
		Model: "claude-sonnet-4-6",
		Messages: []AnthropicMessage{
			{Role: "user", Content: []any{
				map[string]any{
					"type": "image",
					"source": map[string]any{
						"type":       "base64",
						"media_type": "image/png",
						"data":       "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
					},
				},
				map[string]any{"type": "text", "text": "what is this?"},
			}},
		},
	}
	p, _ := TransformAnthropicRequest(req, "")
	imgs := p.ConversationState.CurrentMessage.UserInputMessage.Images
	if len(imgs) != 1 {
		t.Fatalf("expected 1 image, got %d", len(imgs))
	}
	if imgs[0].Format != "png" {
		t.Fatalf("format = %q", imgs[0].Format)
	}
}

func TestTransformAnthropicRequest_ToolResultContinuation(t *testing.T) {
	// User message containing only a tool_result. Final content should be
	// the synthesized "Tool results:" continuation, not minimalFallback.
	req := &AnthropicRequest{
		Model: "claude-sonnet-4-6",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "use a tool"},
			{Role: "assistant", Content: []any{
				map[string]any{"type": "tool_use", "id": "tu_1", "name": "f", "input": map[string]any{}},
			}},
			{Role: "user", Content: []any{
				map[string]any{"type": "tool_result", "tool_use_id": "tu_1", "content": "42"},
			}},
		},
	}
	p, _ := TransformAnthropicRequest(req, "")
	if !strings.Contains(p.ConversationState.CurrentMessage.UserInputMessage.Content, "Tool results:") {
		t.Fatalf("continuation missing: %q", p.ConversationState.CurrentMessage.UserInputMessage.Content)
	}
	ctx := p.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
	if ctx == nil || len(ctx.ToolResults) != 1 {
		t.Fatalf("tool results not attached: %+v", ctx)
	}
	if ctx.ToolResults[0].ToolUseID != "tu_1" {
		t.Fatalf("tool use id = %q", ctx.ToolResults[0].ToolUseID)
	}
}

func TestTransformAnthropicRequest_TrimsLeadingAssistantHistory(t *testing.T) {
	req := &AnthropicRequest{
		Model: "claude-sonnet-4-6",
		Messages: []AnthropicMessage{
			{Role: "assistant", Content: "stale response we shouldn't replay"},
			{Role: "user", Content: "real first message"},
		},
	}
	p, _ := TransformAnthropicRequest(req, "")
	// "real first message" is the last user turn → currentMessage, so
	// history is empty after trimming.
	if p.ConversationState.History != nil {
		t.Fatalf("history should be trimmed empty, got %d entries", len(p.ConversationState.History))
	}
}

func TestTransformAnthropicRequest_MarshalsToValidJSON(t *testing.T) {
	req := &AnthropicRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []AnthropicMessage{{Role: "user", Content: "hi"}},
		Tools: []AnthropicTool{{
			Name:        "f",
			Description: "f",
			InputSchema: map[string]any{"type": "object", "required": nil},
		}},
	}
	p, _ := TransformAnthropicRequest(req, "arn")
	b, err := MarshalPayload(p)
	if err != nil {
		t.Fatal(err)
	}
	body := mustUnmarshal(t, b)
	if _, ok := body["conversationState"]; !ok {
		t.Fatalf("body missing conversationState: %s", string(b))
	}
	// Ensure ToolNameMap is NOT serialized.
	if strings.Contains(string(b), "ToolNameMap") || strings.Contains(string(b), "tool_name_map") {
		t.Fatalf("ToolNameMap should not appear in JSON: %s", string(b))
	}
}
