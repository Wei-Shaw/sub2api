package apicompat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Non-streaming: ChatCompletionsResponse → ResponsesResponse
// ---------------------------------------------------------------------------

// ChatCompletionsToResponsesResponse converts a Chat Completions response into a
// Responses API response. choices[0].message becomes the output items, and usage
// is mapped to the Responses usage shape.
//
// Mapping rules:
//   - choices[0].message.reasoning_content → reasoning output item (with summary_text)
//   - choices[0].message.content (string)  → message output item with output_text part
//   - choices[0].message.tool_calls[]      → function_call output items
//   - choices[0].finish_reason             → status / incomplete_details
//   - usage.prompt_tokens / completion_tokens → usage.input_tokens / output_tokens
//   - usage.prompt_tokens_details.cached_tokens → usage.input_tokens_details.cached_tokens
func ChatCompletionsToResponsesResponse(resp *ChatCompletionsResponse, model string) *ResponsesResponse {
	id := resp.ID
	if id == "" {
		id = generateResponsesID()
	}
	if model == "" {
		model = resp.Model
	}

	out := &ResponsesResponse{
		ID:     id,
		Object: "response",
		Model:  model,
	}

	var outputs []ResponsesOutput
	finishReason := ""

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		finishReason = choice.FinishReason
		msg := choice.Message

		if msg.ReasoningContent != "" {
			outputs = append(outputs, ResponsesOutput{
				Type: "reasoning",
				ID:   generateItemID(),
				Summary: []ResponsesSummary{{
					Type: "summary_text",
					Text: msg.ReasoningContent,
				}},
			})
		}

		text := extractChatMessageText(msg.Content)
		if text != "" {
			outputs = append(outputs, ResponsesOutput{
				Type: "message",
				ID:   generateItemID(),
				Role: "assistant",
				Content: []ResponsesContentPart{{
					Type: "output_text",
					Text: text,
				}},
				Status: "completed",
			})
		}

		for _, tc := range msg.ToolCalls {
			args := tc.Function.Arguments
			if args == "" {
				args = "{}"
			}
			callID := tc.ID
			outputs = append(outputs, ResponsesOutput{
				Type:      "function_call",
				ID:        generateItemID(),
				CallID:    callID,
				Name:      tc.Function.Name,
				Arguments: args,
				Status:    "completed",
			})
		}

		if msg.FunctionCall != nil {
			args := msg.FunctionCall.Arguments
			if args == "" {
				args = "{}"
			}
			outputs = append(outputs, ResponsesOutput{
				Type:      "function_call",
				ID:        generateItemID(),
				CallID:    generateCallID(),
				Name:      msg.FunctionCall.Name,
				Arguments: args,
				Status:    "completed",
			})
		}
	}

	if len(outputs) == 0 {
		outputs = append(outputs, ResponsesOutput{
			Type:    "message",
			ID:      generateItemID(),
			Role:    "assistant",
			Content: []ResponsesContentPart{{Type: "output_text", Text: ""}},
			Status:  "completed",
		})
	}
	out.Output = outputs

	out.Status = chatFinishReasonToResponsesStatus(finishReason)
	if out.Status == "incomplete" {
		reason := "max_output_tokens"
		if finishReason == "content_filter" {
			reason = "content_filter"
		}
		out.IncompleteDetails = &ResponsesIncompleteDetails{Reason: reason}
	}

	if resp.Usage != nil {
		out.Usage = &ResponsesUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		}
		if out.Usage.TotalTokens == 0 {
			out.Usage.TotalTokens = out.Usage.InputTokens + out.Usage.OutputTokens
		}
		if resp.Usage.PromptTokensDetails != nil && resp.Usage.PromptTokensDetails.CachedTokens > 0 {
			out.Usage.InputTokensDetails = &ResponsesInputTokensDetails{
				CachedTokens: resp.Usage.PromptTokensDetails.CachedTokens,
			}
		}
		if resp.Usage.CompletionTokensDetails != nil && resp.Usage.CompletionTokensDetails.ReasoningTokens > 0 {
			out.Usage.OutputTokensDetails = &ResponsesOutputTokensDetails{
				ReasoningTokens: resp.Usage.CompletionTokensDetails.ReasoningTokens,
			}
		}
	}

	return out
}

// extractChatMessageText returns the textual content of a Chat message Content
// field. Content may be either a JSON string or an array of typed parts; for
// arrays, text parts are concatenated and non-text parts are ignored.
func extractChatMessageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []ChatContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" && p.Text != "" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return ""
}

// chatFinishReasonToResponsesStatus maps a Chat Completions finish_reason to
// the Responses API status field.
func chatFinishReasonToResponsesStatus(finishReason string) string {
	switch finishReason {
	case "length":
		return "incomplete"
	case "content_filter":
		return "incomplete"
	case "stop", "tool_calls", "function_call", "":
		return "completed"
	default:
		return "completed"
	}
}

// ---------------------------------------------------------------------------
// Streaming: SSE chunk (Chat Completions) → []ResponsesStreamEvent (stateful)
// ---------------------------------------------------------------------------

// CCStreamState tracks state for converting a stream of Chat Completions SSE
// chunks into Responses SSE events.
type CCStreamState struct {
	ResponseID  string
	Model       string
	OutputIndex int

	CreatedSent   bool
	CompletedSent bool

	// Message item tracking — opened lazily on first text/reasoning delta.
	MessageItemID    string
	MessageOpen      bool
	MessageOutputIdx int
	ContentIndex     int
	MessageText      string

	// Reasoning item tracking — opened lazily on first reasoning delta.
	ReasoningItemID     string
	ReasoningOpen       bool
	ReasoningOutputIdx  int
	ReasoningSummaryIdx int
	ReasoningText       string

	// Tool calls indexed by Chat tool_call index → state.
	ToolCalls map[int]*ccToolCallState

	// CompletedOutputs keeps terminal response.output complete even when items
	// were emitted incrementally in earlier SSE events.
	CompletedOutputs []ResponsesOutput

	// Sequence number for emitted Responses events.
	SequenceNumber int

	// Final finish reason / usage captured from the closing chunk.
	FinishReason string
	Usage        *ResponsesUsage
}

type ccToolCallState struct {
	ItemID      string
	CallID      string
	Name        string
	Arguments   string
	OutputIndex int
	Opened      bool
}

// NewCCStreamState returns an initialised stream state.
func NewCCStreamState() *CCStreamState {
	return &CCStreamState{
		ToolCalls: make(map[int]*ccToolCallState),
	}
}

// ConvertChatCompletionsSSEChunkToResponsesEvents parses a single SSE line from
// a Chat Completions stream (the raw bytes of one "data: ..." line, with or
// without trailing newline) and emits the corresponding Responses SSE events as
// "event: <type>\ndata: <json>\n\n" formatted byte slices.
//
// Supported inputs:
//   - "data: {chunk JSON}"
//   - "data: [DONE]" → finalises the stream (emits response.completed if not
//     already sent, plus a literal "data: [DONE]\n\n" terminator)
//   - blank lines / event lines without a payload → ignored
//   - non-data lines (comments, SSE event:) → passed through unchanged so that
//     any upstream noise reaches the client transparently
//
// Errors are returned only when JSON parsing fails for a non-[DONE] data line.
// In that case the original bytes are still returned (transparent passthrough)
// so the caller can decide whether to abort or forward.
func ConvertChatCompletionsSSEChunkToResponsesEvents(chunk []byte, state *CCStreamState) ([][]byte, error) {
	line := bytes.TrimRight(chunk, "\r\n")
	if len(line) == 0 {
		return nil, nil
	}

	// Pass through SSE comments and event: lines untouched.
	if bytes.HasPrefix(line, []byte(":")) || bytes.HasPrefix(line, []byte("event:")) {
		return [][]byte{appendNewlines(line)}, nil
	}

	if !bytes.HasPrefix(line, []byte("data:")) {
		// Unknown line shape — pass through so we don't lose information.
		return [][]byte{appendNewlines(line)}, nil
	}

	payload := bytes.TrimSpace(line[len("data:"):])
	if len(payload) == 0 {
		return nil, nil
	}

	if bytes.Equal(payload, []byte("[DONE]")) {
		return finalizeCCStream(state), nil
	}

	var chk ChatCompletionsChunk
	if err := json.Unmarshal(payload, &chk); err != nil {
		// Transparent passthrough for malformed chunks; surface the error so
		// callers can log it without dropping the data.
		return [][]byte{appendNewlines(line)}, fmt.Errorf("parse chat completions chunk: %w", err)
	}

	events := convertChatChunk(&chk, state)
	out := make([][]byte, 0, len(events))
	for _, evt := range events {
		b, err := marshalResponsesEvent(evt)
		if err != nil {
			return out, err
		}
		out = append(out, b)
	}
	return out, nil
}

// FinalizeCCStream emits any closing events that have not yet been written
// (response.completed + [DONE]) when the upstream stream ended without an
// explicit [DONE] marker. Idempotent.
func FinalizeCCStream(state *CCStreamState) [][]byte {
	return finalizeCCStream(state)
}

func finalizeCCStream(state *CCStreamState) [][]byte {
	var out [][]byte

	if !state.CompletedSent {
		if state.ResponseID == "" {
			state.ResponseID = generateResponsesID()
		}
		if !state.CreatedSent {
			created := makeCCCreatedEvent(state)
			if b, err := marshalResponsesEvent(created); err == nil {
				out = append(out, b)
			}
			state.CreatedSent = true
		}
		// Close any still-open items.
		for _, evt := range closeCCOpenItems(state) {
			if b, err := marshalResponsesEvent(evt); err == nil {
				out = append(out, b)
			}
		}
		completed := makeCCCompletedEvent(state)
		if b, err := marshalResponsesEvent(completed); err == nil {
			out = append(out, b)
		}
		state.CompletedSent = true
	}

	out = append(out, []byte("data: [DONE]\n\n"))
	return out
}

func convertChatChunk(chk *ChatCompletionsChunk, state *CCStreamState) []ResponsesStreamEvent {
	var events []ResponsesStreamEvent

	if state.ResponseID == "" {
		if chk.ID != "" {
			state.ResponseID = chk.ID
		} else {
			state.ResponseID = generateResponsesID()
		}
	}
	if chk.Model != "" {
		state.Model = chk.Model
	}

	if chk.Usage != nil {
		state.Usage = &ResponsesUsage{
			InputTokens:  chk.Usage.PromptTokens,
			OutputTokens: chk.Usage.CompletionTokens,
			TotalTokens:  chk.Usage.TotalTokens,
		}
		if state.Usage.TotalTokens == 0 {
			state.Usage.TotalTokens = state.Usage.InputTokens + state.Usage.OutputTokens
		}
		if chk.Usage.PromptTokensDetails != nil && chk.Usage.PromptTokensDetails.CachedTokens > 0 {
			state.Usage.InputTokensDetails = &ResponsesInputTokensDetails{
				CachedTokens: chk.Usage.PromptTokensDetails.CachedTokens,
			}
		}
		if chk.Usage.CompletionTokensDetails != nil && chk.Usage.CompletionTokensDetails.ReasoningTokens > 0 {
			state.Usage.OutputTokensDetails = &ResponsesOutputTokensDetails{
				ReasoningTokens: chk.Usage.CompletionTokensDetails.ReasoningTokens,
			}
		}
	}

	if len(chk.Choices) == 0 {
		return nil
	}

	if !state.CreatedSent {
		events = append(events, makeCCCreatedEvent(state))
		state.CreatedSent = true
	}

	for _, ch := range chk.Choices {
		// Reasoning delta
		if ch.Delta.ReasoningContent != nil && *ch.Delta.ReasoningContent != "" {
			if !state.ReasoningOpen {
				state.ReasoningItemID = generateItemID()
				state.ReasoningOpen = true
				state.ReasoningOutputIdx = state.OutputIndex
				state.OutputIndex++
				state.ReasoningText = ""
				events = append(events, makeCCEvent(state, "response.output_item.added", ResponsesStreamEvent{
					OutputIndex: state.ReasoningOutputIdx,
					Item: &ResponsesOutput{
						Type: "reasoning",
						ID:   state.ReasoningItemID,
					},
				}))
			}
			events = append(events, makeCCEvent(state, "response.reasoning_summary_text.delta", ResponsesStreamEvent{
				OutputIndex:  state.ReasoningOutputIdx,
				SummaryIndex: state.ReasoningSummaryIdx,
				Delta:        *ch.Delta.ReasoningContent,
				ItemID:       state.ReasoningItemID,
			}))
			state.ReasoningText += *ch.Delta.ReasoningContent
		}

		// Text content delta
		if ch.Delta.Content != nil && *ch.Delta.Content != "" {
			// If reasoning was open, close it before opening message.
			if state.ReasoningOpen {
				events = append(events, closeCCReasoningItem(state)...)
			}
			if !state.MessageOpen {
				state.MessageItemID = generateItemID()
				state.MessageOpen = true
				state.MessageOutputIdx = state.OutputIndex
				state.OutputIndex++
				state.MessageText = ""
				events = append(events, makeCCEvent(state, "response.output_item.added", ResponsesStreamEvent{
					OutputIndex: state.MessageOutputIdx,
					Item: &ResponsesOutput{
						Type:   "message",
						ID:     state.MessageItemID,
						Role:   "assistant",
						Status: "in_progress",
					},
				}))
				events = append(events, makeCCEvent(state, "response.content_part.added", ResponsesStreamEvent{
					OutputIndex:  state.MessageOutputIdx,
					ContentIndex: state.ContentIndex,
					ItemID:       state.MessageItemID,
					Part: &ResponsesContentPart{
						Type: "output_text",
						Text: "",
					},
				}))
			}
			state.MessageText += *ch.Delta.Content
			events = append(events, makeCCEvent(state, "response.output_text.delta", ResponsesStreamEvent{
				OutputIndex:  state.MessageOutputIdx,
				ContentIndex: state.ContentIndex,
				Delta:        *ch.Delta.Content,
				ItemID:       state.MessageItemID,
			}))
		}

		// Tool call deltas
		for _, tc := range ch.Delta.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			st, ok := state.ToolCalls[idx]
			if !ok {
				st = &ccToolCallState{}
				state.ToolCalls[idx] = st
			}
			if tc.ID != "" {
				st.CallID = tc.ID
			}
			if tc.Function.Name != "" {
				st.Name = tc.Function.Name
			}
			if !st.Opened && (st.CallID != "" || st.Name != "" || tc.Function.Arguments != "") {
				// Close message/reasoning before opening function_call so that
				// output indices stay consistent with the order they appear.
				if state.MessageOpen {
					events = append(events, closeCCMessageItem(state)...)
				}
				if state.ReasoningOpen {
					events = append(events, closeCCReasoningItem(state)...)
				}
				if st.CallID == "" {
					st.CallID = generateCallID()
				}
				st.ItemID = generateItemID()
				st.OutputIndex = state.OutputIndex
				state.OutputIndex++
				st.Opened = true
				events = append(events, makeCCEvent(state, "response.output_item.added", ResponsesStreamEvent{
					OutputIndex: st.OutputIndex,
					Item: &ResponsesOutput{
						Type:   "function_call",
						ID:     st.ItemID,
						CallID: st.CallID,
						Name:   st.Name,
						Status: "in_progress",
					},
				}))
			}
			if tc.Function.Arguments != "" && st.Opened {
				st.Arguments += tc.Function.Arguments
				events = append(events, makeCCEvent(state, "response.function_call_arguments.delta", ResponsesStreamEvent{
					OutputIndex: st.OutputIndex,
					Delta:       tc.Function.Arguments,
					ItemID:      st.ItemID,
					CallID:      st.CallID,
					Name:        st.Name,
				}))
			}
		}

		// Finish reason → close items + completed
		if ch.FinishReason != nil && *ch.FinishReason != "" {
			state.FinishReason = *ch.FinishReason
			events = append(events, closeCCOpenItems(state)...)
			if !state.CompletedSent {
				events = append(events, makeCCCompletedEvent(state))
				state.CompletedSent = true
			}
		}
	}

	return events
}

func closeCCOpenItems(state *CCStreamState) []ResponsesStreamEvent {
	var events []ResponsesStreamEvent
	if state.MessageOpen {
		events = append(events, closeCCMessageItem(state)...)
	}
	if state.ReasoningOpen {
		events = append(events, closeCCReasoningItem(state)...)
	}
	for _, idx := range sortedToolCallKeys(state.ToolCalls) {
		st := state.ToolCalls[idx]
		if st.Opened {
			events = append(events, closeCCToolCall(state, st)...)
		}
	}
	return events
}

func closeCCMessageItem(state *CCStreamState) []ResponsesStreamEvent {
	if !state.MessageOpen {
		return nil
	}
	events := []ResponsesStreamEvent{
		makeCCEvent(state, "response.output_text.done", ResponsesStreamEvent{
			OutputIndex:  state.MessageOutputIdx,
			ContentIndex: state.ContentIndex,
			ItemID:       state.MessageItemID,
			Text:         state.MessageText,
		}),
		makeCCEvent(state, "response.content_part.done", ResponsesStreamEvent{
			OutputIndex:  state.MessageOutputIdx,
			ContentIndex: state.ContentIndex,
			ItemID:       state.MessageItemID,
			Part: &ResponsesContentPart{
				Type: "output_text",
				Text: state.MessageText,
			},
		}),
		makeCCEvent(state, "response.output_item.done", ResponsesStreamEvent{
			OutputIndex: state.MessageOutputIdx,
			Item: &ResponsesOutput{
				Type:   "message",
				ID:     state.MessageItemID,
				Role:   "assistant",
				Status: "completed",
				Content: []ResponsesContentPart{{
					Type: "output_text",
					Text: state.MessageText,
				}},
			},
		}),
	}
	state.MessageOpen = false
	state.CompletedOutputs = append(state.CompletedOutputs, ResponsesOutput{
		Type:   "message",
		ID:     state.MessageItemID,
		Role:   "assistant",
		Status: "completed",
		Content: []ResponsesContentPart{{
			Type: "output_text",
			Text: state.MessageText,
		}},
	})
	return events
}

func closeCCReasoningItem(state *CCStreamState) []ResponsesStreamEvent {
	if !state.ReasoningOpen {
		return nil
	}
	events := []ResponsesStreamEvent{
		makeCCEvent(state, "response.reasoning_summary_text.done", ResponsesStreamEvent{
			OutputIndex:  state.ReasoningOutputIdx,
			SummaryIndex: state.ReasoningSummaryIdx,
			ItemID:       state.ReasoningItemID,
			Text:         state.ReasoningText,
		}),
		makeCCEvent(state, "response.output_item.done", ResponsesStreamEvent{
			OutputIndex: state.ReasoningOutputIdx,
			Item: &ResponsesOutput{
				Type:   "reasoning",
				ID:     state.ReasoningItemID,
				Status: "completed",
				Summary: []ResponsesSummary{{
					Type: "summary_text",
					Text: state.ReasoningText,
				}},
			},
		}),
	}
	state.ReasoningOpen = false
	state.CompletedOutputs = append(state.CompletedOutputs, ResponsesOutput{
		Type:   "reasoning",
		ID:     state.ReasoningItemID,
		Status: "completed",
		Summary: []ResponsesSummary{{
			Type: "summary_text",
			Text: state.ReasoningText,
		}},
	})
	return events
}

func closeCCToolCall(state *CCStreamState, st *ccToolCallState) []ResponsesStreamEvent {
	args := st.Arguments
	if args == "" {
		args = "{}"
	}
	events := []ResponsesStreamEvent{
		makeCCEvent(state, "response.function_call_arguments.done", ResponsesStreamEvent{
			OutputIndex: st.OutputIndex,
			ItemID:      st.ItemID,
			CallID:      st.CallID,
			Name:        st.Name,
			Arguments:   args,
		}),
		makeCCEvent(state, "response.output_item.done", ResponsesStreamEvent{
			OutputIndex: st.OutputIndex,
			Item: &ResponsesOutput{
				Type:      "function_call",
				ID:        st.ItemID,
				CallID:    st.CallID,
				Name:      st.Name,
				Arguments: args,
				Status:    "completed",
			},
		}),
	}
	st.Opened = false
	state.CompletedOutputs = append(state.CompletedOutputs, ResponsesOutput{
		Type:      "function_call",
		ID:        st.ItemID,
		CallID:    st.CallID,
		Name:      st.Name,
		Arguments: args,
		Status:    "completed",
	})
	return events
}

func sortedToolCallKeys(m map[int]*ccToolCallState) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// simple insertion sort to keep deterministic order without importing sort.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}

func makeCCCreatedEvent(state *CCStreamState) ResponsesStreamEvent {
	seq := state.SequenceNumber
	state.SequenceNumber++
	return ResponsesStreamEvent{
		Type:           "response.created",
		SequenceNumber: seq,
		Response: &ResponsesResponse{
			ID:     state.ResponseID,
			Object: "response",
			Model:  state.Model,
			Status: "in_progress",
			Output: []ResponsesOutput{},
		},
	}
}

func makeCCCompletedEvent(state *CCStreamState) ResponsesStreamEvent {
	seq := state.SequenceNumber
	state.SequenceNumber++

	status := chatFinishReasonToResponsesStatus(state.FinishReason)
	var incompleteDetails *ResponsesIncompleteDetails
	if status == "incomplete" {
		reason := "max_output_tokens"
		if state.FinishReason == "content_filter" {
			reason = "content_filter"
		}
		incompleteDetails = &ResponsesIncompleteDetails{Reason: reason}
	}

	output := append([]ResponsesOutput(nil), state.CompletedOutputs...)

	return ResponsesStreamEvent{
		Type:           "response.completed",
		SequenceNumber: seq,
		Response: &ResponsesResponse{
			ID:                state.ResponseID,
			Object:            "response",
			Model:             state.Model,
			Status:            status,
			Output:            output,
			Usage:             state.Usage,
			IncompleteDetails: incompleteDetails,
		},
	}
}

func generateCallID() string {
	id := generateItemID()
	return "call_" + strings.TrimPrefix(id, "item_")
}

func makeCCEvent(state *CCStreamState, eventType string, template ResponsesStreamEvent) ResponsesStreamEvent {
	seq := state.SequenceNumber
	state.SequenceNumber++
	template.Type = eventType
	template.SequenceNumber = seq
	return template
}

func marshalResponsesEvent(evt ResponsesStreamEvent) ([]byte, error) {
	data, err := json.Marshal(evt)
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", evt.Type, data)), nil
}

func appendNewlines(line []byte) []byte {
	out := make([]byte, 0, len(line)+2)
	out = append(out, line...)
	out = append(out, '\n', '\n')
	return out
}

func ChatCompletionsResponseToResponses(resp *ChatCompletionsResponse, model string) *ResponsesResponse {
	id := ""
	if resp != nil {
		id = resp.ID
	}
	if id == "" {
		id = generateResponsesID()
	}

	out := &ResponsesResponse{
		ID:     id,
		Object: "response",
		Model:  model,
		Status: "completed",
	}
	if resp == nil {
		out.Output = []ResponsesOutput{emptyResponsesMessageOutput()}
		return out
	}
	if out.Model == "" {
		out.Model = resp.Model
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		out.Output = chatMessageToResponsesOutput(choice.Message)
		if choice.FinishReason == "length" {
			out.Status = "incomplete"
			out.IncompleteDetails = &ResponsesIncompleteDetails{Reason: "max_output_tokens"}
		}
	}
	if len(out.Output) == 0 {
		out.Output = []ResponsesOutput{emptyResponsesMessageOutput()}
	}
	if resp.Usage != nil {
		out.Usage = ChatUsageToResponsesUsage(resp.Usage)
	}
	return out
}

func chatMessageToResponsesOutput(message ChatMessage) []ResponsesOutput {
	var outputs []ResponsesOutput
	if message.ReasoningContent != "" {
		outputs = append(outputs, ResponsesOutput{
			Type: "reasoning",
			ID:   generateItemID(),
			Summary: []ResponsesSummary{{
				Type: "summary_text",
				Text: message.ReasoningContent,
			}},
		})
	}

	text := chatMessageContentText(message.Content)
	if text != "" || len(message.ToolCalls) == 0 {
		outputs = append(outputs, ResponsesOutput{
			Type: "message",
			ID:   generateItemID(),
			Role: "assistant",
			Content: []ResponsesContentPart{{
				Type: "output_text",
				Text: text,
			}},
			Status: "completed",
		})
	}

	for _, toolCall := range message.ToolCalls {
		arguments := toolCall.Function.Arguments
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		outputs = append(outputs, ResponsesOutput{
			Type:      "function_call",
			ID:        generateItemID(),
			CallID:    toolCall.ID,
			Name:      toolCall.Function.Name,
			Arguments: arguments,
			Status:    "completed",
		})
	}

	return outputs
}

func emptyResponsesMessageOutput() ResponsesOutput {
	return ResponsesOutput{
		Type:    "message",
		ID:      generateItemID(),
		Role:    "assistant",
		Content: []ResponsesContentPart{{Type: "output_text", Text: ""}},
		Status:  "completed",
	}
}

func chatMessageContentText(raw json.RawMessage) string {
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var parts []ChatContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var texts []string
		for _, part := range parts {
			if part.Type == "text" && part.Text != "" {
				texts = append(texts, part.Text)
			}
		}
		return strings.Join(texts, "\n\n")
	}
	return ""
}

// ChatUsageToResponsesUsage converts Chat Completions token usage to Responses
// usage shape.
func ChatUsageToResponsesUsage(usage *ChatUsage) *ResponsesUsage {
	if usage == nil {
		return nil
	}
	out := &ResponsesUsage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
		TotalTokens:  usage.TotalTokens,
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.InputTokens + out.OutputTokens
	}
	if usage.PromptTokensDetails != nil && usage.PromptTokensDetails.CachedTokens > 0 {
		out.InputTokensDetails = &ResponsesInputTokensDetails{
			CachedTokens: usage.PromptTokensDetails.CachedTokens,
		}
	}
	return out
}

// ChatCompletionsToResponsesStreamState tracks state while converting Chat
// Completions SSE chunks into Responses SSE events.
type ChatCompletionsToResponsesStreamState struct {
	ResponseID     string
	Model          string
	Created        int64
	SequenceNumber int
	CreatedSent    bool
	CompletedSent  bool

	MessageItemID string
	Text          strings.Builder
	Reasoning     strings.Builder
	ToolCalls     map[int]*ChatToolCall

	FinishReason string
	Usage        *ResponsesUsage
}

// NewChatCompletionsToResponsesStreamState returns an initialized stream state.
func NewChatCompletionsToResponsesStreamState(model string) *ChatCompletionsToResponsesStreamState {
	return &ChatCompletionsToResponsesStreamState{
		ResponseID: generateResponsesID(),
		Model:      model,
		Created:    time.Now().Unix(),
		ToolCalls:  make(map[int]*ChatToolCall),
	}
}

// ChatCompletionsChunkToResponsesEvents converts one Chat Completions stream
// chunk into zero or more Responses stream events.
func ChatCompletionsChunkToResponsesEvents(
	chunk *ChatCompletionsChunk,
	state *ChatCompletionsToResponsesStreamState,
) []ResponsesStreamEvent {
	if chunk == nil || state == nil {
		return nil
	}
	if chunk.ID != "" {
		state.ResponseID = chunk.ID
	}
	if state.Model == "" && chunk.Model != "" {
		state.Model = chunk.Model
	}
	if chunk.Usage != nil {
		state.Usage = ChatUsageToResponsesUsage(chunk.Usage)
	}

	var events []ResponsesStreamEvent
	events = append(events, ensureChatToResponsesCreated(state)...)

	for _, choice := range chunk.Choices {
		if choice.Delta.Content != nil {
			events = append(events, ensureChatToResponsesMessageItem(state)...)
			_, _ = state.Text.WriteString(*choice.Delta.Content)
			events = append(events, chatToResponsesEvent(state, "response.output_text.delta", &ResponsesStreamEvent{
				OutputIndex:  0,
				ContentIndex: 0,
				Delta:        *choice.Delta.Content,
				ItemID:       state.MessageItemID,
			}))
		}
		if choice.Delta.ReasoningContent != nil {
			_, _ = state.Reasoning.WriteString(*choice.Delta.ReasoningContent)
			events = append(events, chatToResponsesEvent(state, "response.reasoning_summary_text.delta", &ResponsesStreamEvent{
				OutputIndex:  0,
				SummaryIndex: 0,
				Delta:        *choice.Delta.ReasoningContent,
			}))
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			idx := 0
			if toolCall.Index != nil {
				idx = *toolCall.Index
			}
			stored, ok := state.ToolCalls[idx]
			if !ok {
				copyCall := toolCall
				if copyCall.ID == "" {
					copyCall.ID = generateItemID()
				}
				copyCall.Type = "function"
				state.ToolCalls[idx] = &copyCall
				stored = &copyCall
				events = append(events, chatToResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
					OutputIndex: idx + 1,
					Item: &ResponsesOutput{
						Type:   "function_call",
						ID:     generateItemID(),
						CallID: stored.ID,
						Name:   stored.Function.Name,
						Status: "in_progress",
					},
				}))
			} else {
				if toolCall.ID != "" {
					stored.ID = toolCall.ID
				}
				if toolCall.Function.Name != "" {
					stored.Function.Name = toolCall.Function.Name
				}
			}
			if toolCall.Function.Arguments != "" {
				stored.Function.Arguments += toolCall.Function.Arguments
				events = append(events, chatToResponsesEvent(state, "response.function_call_arguments.delta", &ResponsesStreamEvent{
					OutputIndex: idx + 1,
					Delta:       toolCall.Function.Arguments,
					CallID:      stored.ID,
					Name:        stored.Function.Name,
				}))
			}
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			state.FinishReason = *choice.FinishReason
		}
	}

	return events
}

// FinalizeChatCompletionsResponsesStream emits terminal Responses events.
func FinalizeChatCompletionsResponsesStream(state *ChatCompletionsToResponsesStreamState) []ResponsesStreamEvent {
	if state == nil || state.CompletedSent {
		return nil
	}
	var events []ResponsesStreamEvent
	events = append(events, ensureChatToResponsesCreated(state)...)
	if state.MessageItemID != "" {
		events = append(events, chatToResponsesEvent(state, "response.output_text.done", &ResponsesStreamEvent{
			OutputIndex:  0,
			ContentIndex: 0,
			Text:         state.Text.String(),
			ItemID:       state.MessageItemID,
		}))
		events = append(events, chatToResponsesEvent(state, "response.output_item.done", &ResponsesStreamEvent{
			OutputIndex: 0,
			Item: &ResponsesOutput{
				Type:   "message",
				ID:     state.MessageItemID,
				Role:   "assistant",
				Status: "completed",
			},
		}))
	}

	status := "completed"
	var incompleteDetails *ResponsesIncompleteDetails
	if state.FinishReason == "length" {
		status = "incomplete"
		incompleteDetails = &ResponsesIncompleteDetails{Reason: "max_output_tokens"}
	}

	state.CompletedSent = true
	events = append(events, chatToResponsesEvent(state, "response.completed", &ResponsesStreamEvent{
		Response: &ResponsesResponse{
			ID:                state.ResponseID,
			Object:            "response",
			Model:             state.Model,
			Status:            status,
			Output:            state.chatOutput(),
			Usage:             state.Usage,
			IncompleteDetails: incompleteDetails,
		},
	}))
	return events
}

func ensureChatToResponsesCreated(state *ChatCompletionsToResponsesStreamState) []ResponsesStreamEvent {
	if state.CreatedSent {
		return nil
	}
	state.CreatedSent = true
	return []ResponsesStreamEvent{chatToResponsesEvent(state, "response.created", &ResponsesStreamEvent{
		Response: &ResponsesResponse{
			ID:     state.ResponseID,
			Object: "response",
			Model:  state.Model,
			Status: "in_progress",
			Output: []ResponsesOutput{},
		},
	})}
}

func ensureChatToResponsesMessageItem(state *ChatCompletionsToResponsesStreamState) []ResponsesStreamEvent {
	if state.MessageItemID != "" {
		return nil
	}
	state.MessageItemID = generateItemID()
	return []ResponsesStreamEvent{chatToResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
		OutputIndex: 0,
		Item: &ResponsesOutput{
			Type:   "message",
			ID:     state.MessageItemID,
			Role:   "assistant",
			Status: "in_progress",
		},
	})}
}

func (state *ChatCompletionsToResponsesStreamState) chatOutput() []ResponsesOutput {
	var outputs []ResponsesOutput
	if state.Reasoning.Len() > 0 {
		outputs = append(outputs, ResponsesOutput{
			Type: "reasoning",
			ID:   generateItemID(),
			Summary: []ResponsesSummary{{
				Type: "summary_text",
				Text: state.Reasoning.String(),
			}},
		})
	}
	if state.MessageItemID != "" || len(state.ToolCalls) == 0 {
		outputs = append(outputs, ResponsesOutput{
			Type: "message",
			ID:   nonEmpty(state.MessageItemID, generateItemID()),
			Role: "assistant",
			Content: []ResponsesContentPart{{
				Type: "output_text",
				Text: state.Text.String(),
			}},
			Status: "completed",
		})
	}
	for i := 0; i < len(state.ToolCalls); i++ {
		toolCall, ok := state.ToolCalls[i]
		if !ok || toolCall == nil {
			continue
		}
		arguments := toolCall.Function.Arguments
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		outputs = append(outputs, ResponsesOutput{
			Type:      "function_call",
			ID:        generateItemID(),
			CallID:    toolCall.ID,
			Name:      toolCall.Function.Name,
			Arguments: arguments,
			Status:    "completed",
		})
	}
	return outputs
}

func chatToResponsesEvent(
	state *ChatCompletionsToResponsesStreamState,
	eventType string,
	template *ResponsesStreamEvent,
) ResponsesStreamEvent {
	seq := state.SequenceNumber
	state.SequenceNumber++
	evt := *template
	evt.Type = eventType
	evt.SequenceNumber = seq
	return evt
}

func bytesTrimSpace(raw json.RawMessage) json.RawMessage {
	return bytes.TrimSpace(raw)
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
