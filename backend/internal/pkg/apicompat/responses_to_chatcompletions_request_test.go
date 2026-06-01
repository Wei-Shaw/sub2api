package apicompat

import (
	"encoding/json"
	"testing"
)

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestResponsesToChatCompletionsRequest_TextOnlySingleMessage(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: mustMarshal(t, []ResponsesInputItem{
			{Role: "user", Content: mustMarshal(t, "hello world")},
		}),
	}

	got, err := ResponsesToChatCompletionsRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Model != "gpt-4o" {
		t.Errorf("model: got %q want %q", got.Model, "gpt-4o")
	}
	if len(got.Messages) != 1 {
		t.Fatalf("messages len: got %d want 1", len(got.Messages))
	}
	if got.Messages[0].Role != "user" {
		t.Errorf("role: got %q want user", got.Messages[0].Role)
	}
	var s string
	if err := json.Unmarshal(got.Messages[0].Content, &s); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if s != "hello world" {
		t.Errorf("content: got %q want %q", s, "hello world")
	}
}

func TestResponsesToChatCompletionsRequest_StringInput(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: mustMarshal(t, "ping"),
	}

	got, err := ResponsesToChatCompletionsRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "user" {
		t.Fatalf("expected single user message, got %+v", got.Messages)
	}
	var s string
	_ = json.Unmarshal(got.Messages[0].Content, &s)
	if s != "ping" {
		t.Errorf("content: got %q want ping", s)
	}
}

func TestResponsesToChatCompletionsRequest_MultiMessageHistory(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: mustMarshal(t, []ResponsesInputItem{
			{Role: "system", Content: mustMarshal(t, "you are helpful")},
			{Role: "user", Content: mustMarshal(t, "hi")},
			{Role: "assistant", Content: mustMarshal(t, []ResponsesContentPart{
				{Type: "output_text", Text: "hello!"},
			})},
			{Role: "user", Content: mustMarshal(t, []ResponsesContentPart{
				{Type: "input_text", Text: "and now?"},
			})},
		}),
	}

	got, err := ResponsesToChatCompletionsRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Messages) != 4 {
		t.Fatalf("messages len: got %d want 4", len(got.Messages))
	}
	wantRoles := []string{"system", "user", "assistant", "user"}
	for i, want := range wantRoles {
		if got.Messages[i].Role != want {
			t.Errorf("messages[%d].role: got %q want %q", i, got.Messages[i].Role, want)
		}
	}

	// assistant content should be folded back to plain string
	var assistantContent string
	if err := json.Unmarshal(got.Messages[2].Content, &assistantContent); err != nil {
		t.Fatalf("assistant content not a string: %v", err)
	}
	if assistantContent != "hello!" {
		t.Errorf("assistant content: got %q want %q", assistantContent, "hello!")
	}

	// last user content (single text part) should be folded to string too
	var userContent string
	if err := json.Unmarshal(got.Messages[3].Content, &userContent); err != nil {
		t.Fatalf("user content not a string: %v", err)
	}
	if userContent != "and now?" {
		t.Errorf("user content: got %q want %q", userContent, "and now?")
	}
}

func TestResponsesToChatCompletionsRequest_ToolCallRoundTrip(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: mustMarshal(t, []ResponsesInputItem{
			{Role: "user", Content: mustMarshal(t, "what is the weather in SF?")},
			{
				Type:      "function_call",
				CallID:    "call_123",
				Name:      "get_weather",
				Arguments: `{"city":"SF"}`,
			},
			{
				Type:   "function_call_output",
				CallID: "call_123",
				Output: `{"temp":68}`,
			},
			{Role: "assistant", Content: mustMarshal(t, "It is 68F.")},
		}),
		Tools: []ResponsesTool{
			{
				Type:        "function",
				Name:        "get_weather",
				Description: "Get current weather",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		},
		ToolChoice: json.RawMessage(`"auto"`),
	}

	got, err := ResponsesToChatCompletionsRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect: user, assistant(tool_calls), tool, assistant(text)
	if len(got.Messages) != 4 {
		t.Fatalf("messages len: got %d want 4 (%+v)", len(got.Messages), got.Messages)
	}
	if got.Messages[0].Role != "user" {
		t.Errorf("[0] role: got %q want user", got.Messages[0].Role)
	}
	if got.Messages[1].Role != "assistant" {
		t.Errorf("[1] role: got %q want assistant", got.Messages[1].Role)
	}
	if len(got.Messages[1].ToolCalls) != 1 {
		t.Fatalf("[1] tool_calls len: got %d want 1", len(got.Messages[1].ToolCalls))
	}
	tc := got.Messages[1].ToolCalls[0]
	if tc.ID != "call_123" || tc.Type != "function" || tc.Function.Name != "get_weather" || tc.Function.Arguments != `{"city":"SF"}` {
		t.Errorf("tool_call mismatch: %+v", tc)
	}

	if got.Messages[2].Role != "tool" {
		t.Errorf("[2] role: got %q want tool", got.Messages[2].Role)
	}
	if got.Messages[2].ToolCallID != "call_123" {
		t.Errorf("[2] tool_call_id: got %q want call_123", got.Messages[2].ToolCallID)
	}
	var toolOutput string
	_ = json.Unmarshal(got.Messages[2].Content, &toolOutput)
	if toolOutput != `{"temp":68}` {
		t.Errorf("tool output: got %q want %q", toolOutput, `{"temp":68}`)
	}

	if got.Messages[3].Role != "assistant" {
		t.Errorf("[3] role: got %q want assistant", got.Messages[3].Role)
	}

	// tools mapping
	if len(got.Tools) != 1 {
		t.Fatalf("tools len: got %d want 1", len(got.Tools))
	}
	if got.Tools[0].Type != "function" || got.Tools[0].Function == nil ||
		got.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tool mismatch: %+v", got.Tools[0])
	}

	// tool_choice pass-through
	if string(got.ToolChoice) != `"auto"` {
		t.Errorf("tool_choice: got %s want \"auto\"", string(got.ToolChoice))
	}
}

func TestResponsesToChatCompletionsRequest_UnansweredToolCallsDropped(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: mustMarshal(t, []ResponsesInputItem{
			{Role: "user", Content: mustMarshal(t, "do two things")},
			{Type: "function_call", CallID: "c1", Name: "fn_a", Arguments: `{}`},
			{Type: "function_call", CallID: "c2", Name: "fn_b", Arguments: `{}`},
		}),
	}

	got, err := ResponsesToChatCompletionsRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("messages len: got %d want 1 (%+v)", len(got.Messages), got.Messages)
	}
	if got.Messages[0].Role != "user" {
		t.Errorf("[0] role: got %q want user", got.Messages[0].Role)
	}
}

func TestResponsesToChatCompletionsRequest_MultipleAnsweredToolCallsMerged(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: mustMarshal(t, []ResponsesInputItem{
			{Role: "user", Content: mustMarshal(t, "do two things")},
			{Type: "function_call", CallID: "c1", Name: "fn_a", Arguments: `{}`},
			{Type: "function_call", CallID: "c2", Name: "fn_b", Arguments: `{}`},
			{Type: "function_call_output", CallID: "c1", Output: `ok1`},
			{Type: "function_call_output", CallID: "c2", Output: `ok2`},
		}),
	}

	got, err := ResponsesToChatCompletionsRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Messages) != 4 {
		t.Fatalf("messages len: got %d want 4 (%+v)", len(got.Messages), got.Messages)
	}
	if got.Messages[1].Role != "assistant" || len(got.Messages[1].ToolCalls) != 2 {
		t.Fatalf("expected assistant with two tool calls, got %+v", got.Messages[1])
	}
	if got.Messages[2].Role != "tool" || got.Messages[2].ToolCallID != "c1" {
		t.Fatalf("expected tool c1, got %+v", got.Messages[2])
	}
	if got.Messages[3].Role != "tool" || got.Messages[3].ToolCallID != "c2" {
		t.Fatalf("expected tool c2, got %+v", got.Messages[3])
	}
}

func TestResponsesToChatCompletionsRequest_PartiallyAnsweredToolCallsFiltered(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: mustMarshal(t, []ResponsesInputItem{
			{Role: "user", Content: mustMarshal(t, "do two things")},
			{Type: "function_call", CallID: "c1", Name: "fn_a", Arguments: `{}`},
			{Type: "function_call", CallID: "c2", Name: "fn_b", Arguments: `{}`},
			{Type: "function_call_output", CallID: "c2", Output: `ok2`},
		}),
	}

	got, err := ResponsesToChatCompletionsRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Messages) != 3 {
		t.Fatalf("messages len: got %d want 3 (%+v)", len(got.Messages), got.Messages)
	}
	if got.Messages[1].Role != "assistant" || len(got.Messages[1].ToolCalls) != 1 || got.Messages[1].ToolCalls[0].ID != "c2" {
		t.Fatalf("expected only answered c2 tool call, got %+v", got.Messages[1])
	}
	if got.Messages[2].Role != "tool" || got.Messages[2].ToolCallID != "c2" {
		t.Fatalf("expected tool c2, got %+v", got.Messages[2])
	}
}

func TestResponsesToChatCompletionsRequest_StreamTrue(t *testing.T) {
	req := &ResponsesRequest{
		Model:  "gpt-4o",
		Input:  mustMarshal(t, "hi"),
		Stream: true,
	}
	got, err := ResponsesToChatCompletionsRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Stream {
		t.Error("stream: got false want true")
	}
}

func TestResponsesToChatCompletionsRequest_ReasoningEffortAllLevels(t *testing.T) {
	for _, effort := range []string{"low", "medium", "high"} {
		t.Run(effort, func(t *testing.T) {
			req := &ResponsesRequest{
				Model: "o1",
				Input: mustMarshal(t, "think"),
				Reasoning: &ResponsesReasoning{
					Effort:  effort,
					Summary: "auto",
				},
			}
			got, err := ResponsesToChatCompletionsRequest(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ReasoningEffort != effort {
				t.Errorf("reasoning_effort: got %q want %q", got.ReasoningEffort, effort)
			}
		})
	}
}

func TestResponsesToChatCompletionsRequest_ScalarFieldMapping(t *testing.T) {
	temp := 0.7
	topP := 0.9
	maxTok := 1024
	req := &ResponsesRequest{
		Model:           "gpt-4o",
		Input:           mustMarshal(t, "x"),
		Temperature:     &temp,
		TopP:            &topP,
		MaxOutputTokens: &maxTok,
		ServiceTier:     "auto",
		Instructions:    "be terse",
	}

	got, err := ResponsesToChatCompletionsRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Temperature == nil || *got.Temperature != 0.7 {
		t.Errorf("temperature mismatch: %v", got.Temperature)
	}
	if got.TopP == nil || *got.TopP != 0.9 {
		t.Errorf("top_p mismatch: %v", got.TopP)
	}
	if got.MaxTokens == nil || *got.MaxTokens != 1024 {
		t.Errorf("max_tokens mismatch: %v", got.MaxTokens)
	}
	if got.ServiceTier != "auto" {
		t.Errorf("service_tier mismatch: %q", got.ServiceTier)
	}
	if got.Instructions != "" {
		t.Errorf("instructions should not be passed to strict Chat Completions upstreams by default, got %q", got.Instructions)
	}
	// instructions should also be inserted as a leading system message
	if len(got.Messages) < 1 || got.Messages[0].Role != "system" {
		t.Fatalf("expected leading system message from instructions, got %+v", got.Messages)
	}
	var s string
	_ = json.Unmarshal(got.Messages[0].Content, &s)
	if s != "be terse" {
		t.Errorf("instructions system content: got %q want %q", s, "be terse")
	}
}

func TestResponsesToChatCompletionsRequestWithOptions_PreserveInstructionsField(t *testing.T) {
	req := &ResponsesRequest{
		Model:        "gpt-4o",
		Input:        mustMarshal(t, "x"),
		Instructions: "be terse",
	}

	got, err := ResponsesToChatCompletionsRequestWithOptions(req, ConvertResponsesOptions{PreserveInstructionsField: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Instructions != "be terse" {
		t.Errorf("instructions mismatch: %q", got.Instructions)
	}
}

func TestResponsesToChatCompletionsRequest_ToolChoiceObjectMapping(t *testing.T) {
	req := &ResponsesRequest{
		Model:      "gpt-4o",
		Input:      mustMarshal(t, "x"),
		ToolChoice: json.RawMessage(`{"type":"function","name":"get_weather"}`),
	}

	got, err := ResponsesToChatCompletionsRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var choice struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(got.ToolChoice, &choice); err != nil {
		t.Fatalf("unmarshal tool_choice: %v", err)
	}
	if choice.Type != "function" || choice.Function.Name != "get_weather" {
		t.Fatalf("unexpected tool_choice: %s", string(got.ToolChoice))
	}
}

func TestResponsesToChatCompletionsRequest_FunctionCallOutputObject(t *testing.T) {
	var req ResponsesRequest
	body := []byte(`{"model":"gpt-4o","input":[{"type":"function_call_output","call_id":"call_1","output":{"temp":68,"unit":"f"}}]}`)
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	got, err := ResponsesToChatCompletionsRequest(&req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "tool" {
		t.Fatalf("expected single tool message, got %+v", got.Messages)
	}
	var content string
	if err := json.Unmarshal(got.Messages[0].Content, &content); err != nil {
		t.Fatalf("unmarshal tool content: %v", err)
	}
	if content != `{"temp":68,"unit":"f"}` {
		t.Errorf("tool content: got %q", content)
	}
}

func TestResponsesToChatCompletionsRequest_IgnoredFieldsDoNotError(t *testing.T) {
	store := false
	parallel := true
	req := &ResponsesRequest{
		Model:              "gpt-4o",
		Input:              mustMarshal(t, "x"),
		PreviousResponseID: "resp_abc",
		PromptCacheKey:     "ck_1",
		Include:            []string{"reasoning.encrypted_content"},
		Store:              &store,
		ParallelToolCalls:  &parallel,
		Text:               &ResponsesText{Verbosity: "low"},
	}
	if _, err := ResponsesToChatCompletionsRequest(req); err != nil {
		t.Fatalf("unexpected error with ignored fields: %v", err)
	}
}

func TestResponsesToChatCompletionsRequest_NilInputNoCrash(t *testing.T) {
	req := &ResponsesRequest{Model: "gpt-4o"}
	got, err := ResponsesToChatCompletionsRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Messages) != 0 {
		t.Errorf("expected no messages, got %d", len(got.Messages))
	}
}

func TestResponsesToChatCompletionsRequest_NilRequestErrors(t *testing.T) {
	if _, err := ResponsesToChatCompletionsRequest(nil); err == nil {
		t.Error("expected error for nil request")
	}
}

func TestResponsesToChatCompletionsRequestWithOptions_StripReasoningEffort(t *testing.T) {
	req := &ResponsesRequest{
		Model:     "gpt-5.4",
		Reasoning: &ResponsesReasoning{Effort: "medium"},
		Input:     mustMarshal(t, "hello"),
	}

	// Default (StripReasoningEffort=false): reasoning_effort should be preserved.
	got, err := ResponsesToChatCompletionsRequestWithOptions(req, ConvertResponsesOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ReasoningEffort != "medium" {
		t.Errorf("default: expected reasoning_effort=medium, got %q", got.ReasoningEffort)
	}

	// StripReasoningEffort=true: reasoning_effort should be omitted.
	got, err = ResponsesToChatCompletionsRequestWithOptions(req, ConvertResponsesOptions{StripReasoningEffort: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ReasoningEffort != "" {
		t.Errorf("strip=true: expected empty reasoning_effort, got %q", got.ReasoningEffort)
	}
}
