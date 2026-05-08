package apicompat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Non-streaming: ResponsesResponse → ChatCompletionsResponse
// ---------------------------------------------------------------------------

// ResponsesToChatCompletions converts a Responses API response into a Chat
// Completions response. Text output items are concatenated into
// choices[0].message.content; function_call items become tool_calls.
func ResponsesToChatCompletions(resp *ResponsesResponse, model string) *ChatCompletionsResponse {
	id := resp.ID
	if id == "" {
		id = generateChatCmplID()
	}

	out := &ChatCompletionsResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
	}

	var contentText string
	var reasoningText string
	var toolCalls []ChatToolCall

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" && part.Text != "" {
					contentText += part.Text
				}
			}
		case "function_call":
			toolCalls = append(toolCalls, ChatToolCall{
				ID:   item.CallID,
				Type: "function",
				Function: ChatFunctionCall{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		case "reasoning":
			for _, s := range item.Summary {
				if s.Type == "summary_text" && s.Text != "" {
					reasoningText += s.Text
				}
			}
		case "web_search_call":
			// silently consumed — results already incorporated into text output
		}
	}

	msg := ChatMessage{Role: "assistant"}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	if contentText != "" {
		raw, _ := json.Marshal(contentText)
		msg.Content = raw
	}
	if reasoningText != "" {
		msg.ReasoningContent = reasoningText
	}

	finishReason := responsesStatusToChatFinishReason(resp.Status, resp.IncompleteDetails, toolCalls)

	out.Choices = []ChatChoice{{
		Index:        0,
		Message:      msg,
		FinishReason: finishReason,
	}}

	if resp.Usage != nil {
		usage := &ChatUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		}
		if resp.Usage.InputTokensDetails != nil && resp.Usage.InputTokensDetails.CachedTokens > 0 {
			usage.PromptTokensDetails = &ChatTokenDetails{
				CachedTokens: resp.Usage.InputTokensDetails.CachedTokens,
			}
		}
		out.Usage = usage
	}

	return out
}

func responsesStatusToChatFinishReason(status string, details *ResponsesIncompleteDetails, toolCalls []ChatToolCall) string {
	switch status {
	case "incomplete":
		if details != nil && details.Reason == "max_output_tokens" {
			return "length"
		}
		return "stop"
	case "completed":
		if len(toolCalls) > 0 {
			return "tool_calls"
		}
		return "stop"
	default:
		return "stop"
	}
}

// ---------------------------------------------------------------------------
// Streaming: ResponsesStreamEvent → []ChatCompletionsChunk (stateful converter)
// ---------------------------------------------------------------------------

// ResponsesEventToChatState tracks state for converting a sequence of Responses
// SSE events into Chat Completions SSE chunks.
type ResponsesEventToChatState struct {
	ID                     string
	Model                  string
	Created                int64
	SentRole               bool
	SawToolCall            bool
	SawText                bool
	Finalized              bool        // true after finish chunk has been emitted
	NextToolCallIndex      int         // next sequential tool_call index to assign
	OutputIndexToToolIndex map[int]int // Responses output_index → Chat tool_calls index
	IncludeUsage           bool
	Usage                  *ChatUsage
}

// NewResponsesEventToChatState returns an initialised stream state.
func NewResponsesEventToChatState() *ResponsesEventToChatState {
	return &ResponsesEventToChatState{
		ID:                     generateChatCmplID(),
		Created:                time.Now().Unix(),
		OutputIndexToToolIndex: make(map[int]int),
	}
}

// ResponsesEventToChatChunks converts a single Responses SSE event into zero
// or more Chat Completions chunks, updating state as it goes.
func ResponsesEventToChatChunks(evt *ResponsesStreamEvent, state *ResponsesEventToChatState) []ChatCompletionsChunk {
	switch evt.Type {
	case "response.created":
		return resToChatHandleCreated(evt, state)
	case "response.output_text.delta":
		return resToChatHandleTextDelta(evt, state)
	case "response.output_item.added":
		return resToChatHandleOutputItemAdded(evt, state)
	case "response.function_call_arguments.delta":
		return resToChatHandleFuncArgsDelta(evt, state)
	case "response.reasoning_summary_text.delta":
		return resToChatHandleReasoningDelta(evt, state)
	case "response.reasoning_summary_text.done":
		return nil
	// response.done 是 Realtime/WS 与项目透传路径使用的终止别名；
	// 普通 Responses HTTP SSE 的公开终止事件仍以 response.completed 为主。
	case "response.completed", "response.done", "response.incomplete", "response.failed":
		return resToChatHandleCompleted(evt, state)
	default:
		return nil
	}
}

// FinalizeResponsesChatStream emits a final chunk with finish_reason if the
// stream ended without a proper completion event (e.g. upstream disconnect).
// It is idempotent: if a completion event already emitted the finish chunk,
// this returns nil.
func FinalizeResponsesChatStream(state *ResponsesEventToChatState) []ChatCompletionsChunk {
	if state.Finalized {
		return nil
	}
	state.Finalized = true

	finishReason := "stop"
	if state.SawToolCall {
		finishReason = "tool_calls"
	}

	chunks := []ChatCompletionsChunk{makeChatFinishChunk(state, finishReason)}

	if state.IncludeUsage && state.Usage != nil {
		chunks = append(chunks, ChatCompletionsChunk{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []ChatChunkChoice{},
			Usage:   state.Usage,
		})
	}

	return chunks
}

// ChatChunkToSSE formats a ChatCompletionsChunk as an SSE data line.
func ChatChunkToSSE(chunk ChatCompletionsChunk) (string, error) {
	data, err := json.Marshal(chunk)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("data: %s\n\n", data), nil
}

// --- internal handlers ---

func resToChatHandleCreated(evt *ResponsesStreamEvent, state *ResponsesEventToChatState) []ChatCompletionsChunk {
	if evt.Response != nil {
		if evt.Response.ID != "" {
			state.ID = evt.Response.ID
		}
		if state.Model == "" && evt.Response.Model != "" {
			state.Model = evt.Response.Model
		}
	}
	// Emit the role chunk.
	if state.SentRole {
		return nil
	}
	state.SentRole = true

	role := "assistant"
	return []ChatCompletionsChunk{makeChatDeltaChunk(state, ChatDelta{Role: role})}
}

func resToChatHandleTextDelta(evt *ResponsesStreamEvent, state *ResponsesEventToChatState) []ChatCompletionsChunk {
	if evt.Delta == "" {
		return nil
	}
	state.SawText = true
	content := evt.Delta
	return []ChatCompletionsChunk{makeChatDeltaChunk(state, ChatDelta{Content: &content})}
}

func resToChatHandleOutputItemAdded(evt *ResponsesStreamEvent, state *ResponsesEventToChatState) []ChatCompletionsChunk {
	if evt.Item == nil || evt.Item.Type != "function_call" {
		return nil
	}

	state.SawToolCall = true
	idx := state.NextToolCallIndex
	state.OutputIndexToToolIndex[evt.OutputIndex] = idx
	state.NextToolCallIndex++

	return []ChatCompletionsChunk{makeChatDeltaChunk(state, ChatDelta{
		ToolCalls: []ChatToolCall{{
			Index: &idx,
			ID:    evt.Item.CallID,
			Type:  "function",
			Function: ChatFunctionCall{
				Name: evt.Item.Name,
			},
		}},
	})}
}

func resToChatHandleFuncArgsDelta(evt *ResponsesStreamEvent, state *ResponsesEventToChatState) []ChatCompletionsChunk {
	if evt.Delta == "" {
		return nil
	}

	idx, ok := state.OutputIndexToToolIndex[evt.OutputIndex]
	if !ok {
		return nil
	}

	return []ChatCompletionsChunk{makeChatDeltaChunk(state, ChatDelta{
		ToolCalls: []ChatToolCall{{
			Index: &idx,
			Function: ChatFunctionCall{
				Arguments: evt.Delta,
			},
		}},
	})}
}

func resToChatHandleReasoningDelta(evt *ResponsesStreamEvent, state *ResponsesEventToChatState) []ChatCompletionsChunk {
	if evt.Delta == "" {
		return nil
	}
	reasoning := evt.Delta
	return []ChatCompletionsChunk{makeChatDeltaChunk(state, ChatDelta{ReasoningContent: &reasoning})}
}

func resToChatHandleCompleted(evt *ResponsesStreamEvent, state *ResponsesEventToChatState) []ChatCompletionsChunk {
	state.Finalized = true
	finishReason := "stop"

	if evt.Response != nil {
		if evt.Response.Usage != nil {
			u := evt.Response.Usage
			usage := &ChatUsage{
				PromptTokens:     u.InputTokens,
				CompletionTokens: u.OutputTokens,
				TotalTokens:      u.InputTokens + u.OutputTokens,
			}
			if u.InputTokensDetails != nil && u.InputTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails = &ChatTokenDetails{
					CachedTokens: u.InputTokensDetails.CachedTokens,
				}
			}
			state.Usage = usage
		}

		switch evt.Response.Status {
		case "incomplete":
			if evt.Response.IncompleteDetails != nil && evt.Response.IncompleteDetails.Reason == "max_output_tokens" {
				finishReason = "length"
			}
		case "completed":
			if state.SawToolCall {
				finishReason = "tool_calls"
			}
		}
	} else if state.SawToolCall {
		finishReason = "tool_calls"
	}

	var chunks []ChatCompletionsChunk
	chunks = append(chunks, makeChatFinishChunk(state, finishReason))

	if state.IncludeUsage && state.Usage != nil {
		chunks = append(chunks, ChatCompletionsChunk{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []ChatChunkChoice{},
			Usage:   state.Usage,
		})
	}

	return chunks
}

func makeChatDeltaChunk(state *ResponsesEventToChatState, delta ChatDelta) ChatCompletionsChunk {
	return ChatCompletionsChunk{
		ID:      state.ID,
		Object:  "chat.completion.chunk",
		Created: state.Created,
		Model:   state.Model,
		Choices: []ChatChunkChoice{{
			Index:        0,
			Delta:        delta,
			FinishReason: nil,
		}},
	}
}

func makeChatFinishChunk(state *ResponsesEventToChatState, finishReason string) ChatCompletionsChunk {
	empty := ""
	return ChatCompletionsChunk{
		ID:      state.ID,
		Object:  "chat.completion.chunk",
		Created: state.Created,
		Model:   state.Model,
		Choices: []ChatChunkChoice{{
			Index:        0,
			Delta:        ChatDelta{Content: &empty},
			FinishReason: &finishReason,
		}},
	}
}

// generateChatCmplID returns a "chatcmpl-" prefixed random hex ID.
func generateChatCmplID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "chatcmpl-" + hex.EncodeToString(b)
}

// ---------------------------------------------------------------------------
// BufferedResponseAccumulator: accumulates SSE delta events for non-streaming
// paths where the terminal event may have empty output.
// ---------------------------------------------------------------------------
type bufferedFuncCall struct {
	CallID string
	Name   string
	Args   strings.Builder
}

type indexedOutputSlot struct {
	item      *ResponsesOutput
	text      strings.Builder
	reasoning strings.Builder
	funcArgs  strings.Builder
}

// IndexedResponsesOutput is a canonical output item paired with its original
// Responses output_index position.
type IndexedResponsesOutput struct {
	OutputIndex int
	Item        ResponsesOutput
}

// BufferedResponseAccumulator collects content from Responses SSE delta events
// so that non-streaming handlers can reconstruct output when the terminal event
// (response.completed / response.done) carries an empty output array.
type BufferedResponseAccumulator struct {
	text                 strings.Builder
	reasoning            strings.Builder
	funcCalls            []bufferedFuncCall
	outputIndexToFuncIdx map[int]int
	indexedSlots         map[int]*indexedOutputSlot
}

// NewBufferedResponseAccumulator returns an initialised accumulator.
func NewBufferedResponseAccumulator() *BufferedResponseAccumulator {
	return &BufferedResponseAccumulator{
		outputIndexToFuncIdx: make(map[int]int),
		indexedSlots:         make(map[int]*indexedOutputSlot),
	}
}

// ProcessEvent inspects a single Responses SSE event and accumulates any
// content it carries. Only delta events that contribute to the final output
// are handled; all other event types are silently ignored.
func (a *BufferedResponseAccumulator) ProcessEvent(event *ResponsesStreamEvent) {
	switch event.Type {
	case "response.output_text.delta":
		if event.Delta != "" {
			_, _ = a.text.WriteString(event.Delta)
			_, _ = a.ensureIndexedSlot(event.OutputIndex).text.WriteString(event.Delta)
		}
	case "response.output_item.added":
		if event.Item != nil {
			a.recordIndexedSlotItem(event.OutputIndex, event.Item)
		}
		if event.Item != nil && event.Item.Type == "function_call" {
			idx := len(a.funcCalls)
			a.outputIndexToFuncIdx[event.OutputIndex] = idx
			a.funcCalls = append(a.funcCalls, bufferedFuncCall{
				CallID: event.Item.CallID,
				Name:   event.Item.Name,
			})
		}
	case "response.function_call_arguments.delta":
		if event.Delta != "" {
			slot := a.ensureIndexedSlot(event.OutputIndex)
			_, _ = slot.funcArgs.WriteString(event.Delta)
			if slot.item != nil {
				if slot.item.Type == "" {
					slot.item.Type = "function_call"
				}
				if slot.item.CallID == "" {
					slot.item.CallID = event.CallID
				}
				if slot.item.Name == "" {
					slot.item.Name = event.Name
				}
			} else {
				slot.item = &ResponsesOutput{
					Type:   "function_call",
					CallID: event.CallID,
					Name:   event.Name,
				}
			}
			if idx, ok := a.outputIndexToFuncIdx[event.OutputIndex]; ok {
				_, _ = a.funcCalls[idx].Args.WriteString(event.Delta)
			}
		}
	case "response.reasoning_summary_text.delta":
		if event.Delta != "" {
			_, _ = a.reasoning.WriteString(event.Delta)
			_, _ = a.ensureIndexedSlot(event.OutputIndex).reasoning.WriteString(event.Delta)
		}
	case "response.output_item.done":
		if event.Item != nil {
			a.recordIndexedSlotItem(event.OutputIndex, event.Item)
		}
	}
}

// HasContent reports whether any content has been accumulated.
func (a *BufferedResponseAccumulator) HasContent() bool {
	return a.text.Len() > 0 || len(a.funcCalls) > 0 || a.reasoning.Len() > 0
}

// BuildOutput constructs a []ResponsesOutput from the accumulated delta
// content. The order matches what ResponsesToChatCompletions expects:
// reasoning → message → function_calls.
func (a *BufferedResponseAccumulator) BuildOutput() []ResponsesOutput {
	var out []ResponsesOutput

	if a.reasoning.Len() > 0 {
		out = append(out, ResponsesOutput{
			Type: "reasoning",
			Summary: []ResponsesSummary{{
				Type: "summary_text",
				Text: a.reasoning.String(),
			}},
		})
	}

	if a.text.Len() > 0 {
		out = append(out, ResponsesOutput{
			Type: "message",
			Role: "assistant",
			Content: []ResponsesContentPart{{
				Type: "output_text",
				Text: a.text.String(),
			}},
		})
	}

	for i := range a.funcCalls {
		out = append(out, ResponsesOutput{
			Type:      "function_call",
			CallID:    a.funcCalls[i].CallID,
			Name:      a.funcCalls[i].Name,
			Arguments: a.funcCalls[i].Args.String(),
		})
	}

	return out
}

// BuildIndexedOutput constructs a canonical SSE output view ordered by
// output_index without changing the legacy BuildOutput ordering.
func (a *BufferedResponseAccumulator) BuildIndexedOutput() []IndexedResponsesOutput {
	if len(a.indexedSlots) == 0 {
		return nil
	}

	indexes := make([]int, 0, len(a.indexedSlots))
	for outputIndex := range a.indexedSlots {
		indexes = append(indexes, outputIndex)
	}
	sort.Ints(indexes)

	out := make([]IndexedResponsesOutput, 0, len(indexes))
	for _, outputIndex := range indexes {
		item, ok := buildIndexedResponsesItem(a.indexedSlots[outputIndex])
		if !ok {
			continue
		}
		out = append(out, IndexedResponsesOutput{
			OutputIndex: outputIndex,
			Item:        item,
		})
	}

	return out
}

// SupplementResponseOutput fills resp.Output from accumulated delta content
// when the terminal event delivered an empty output array. If resp.Output is
// already populated, this is a no-op (preserves backward compatibility).
func (a *BufferedResponseAccumulator) SupplementResponseOutput(resp *ResponsesResponse) {
	if resp == nil || len(resp.Output) > 0 {
		return
	}
	if !a.HasContent() {
		return
	}
	resp.Output = a.BuildOutput()
}

func (a *BufferedResponseAccumulator) ensureIndexedSlot(outputIndex int) *indexedOutputSlot {
	if slot, ok := a.indexedSlots[outputIndex]; ok {
		return slot
	}
	slot := &indexedOutputSlot{}
	a.indexedSlots[outputIndex] = slot
	return slot
}

func (a *BufferedResponseAccumulator) recordIndexedSlotItem(outputIndex int, item *ResponsesOutput) {
	slot := a.ensureIndexedSlot(outputIndex)
	if slot.item != nil && item != nil {
		switch {
		case slot.item.Type == "function_call" || item.Type == "function_call":
			slot.item = mergeIndexedFunctionCallItem(slot.item, item)
			return
		case slot.item.Type == "web_search_call" || item.Type == "web_search_call":
			slot.item = mergeIndexedWebSearchCallItem(slot.item, item)
			return
		}
	}
	slot.item = cloneResponsesOutput(item)
}

func buildIndexedResponsesItem(slot *indexedOutputSlot) (ResponsesOutput, bool) {
	if slot == nil {
		return ResponsesOutput{}, false
	}

	if slot.item != nil {
		item := *cloneResponsesOutput(slot.item)
		switch item.Type {
		case "message":
			if len(item.Content) == 0 && slot.text.Len() > 0 {
				item.Role = firstNonEmpty(item.Role, "assistant")
				item.Content = []ResponsesContentPart{{
					Type: "output_text",
					Text: slot.text.String(),
				}}
			}
		case "reasoning":
			if len(item.Summary) == 0 && slot.reasoning.Len() > 0 {
				item.Summary = []ResponsesSummary{{
					Type: "summary_text",
					Text: slot.reasoning.String(),
				}}
			}
		case "function_call":
			if item.Arguments == "" && slot.funcArgs.Len() > 0 {
				item.Arguments = slot.funcArgs.String()
			}
		}
		return item, item.Type != ""
	}

	if slot.reasoning.Len() > 0 {
		return ResponsesOutput{
			Type: "reasoning",
			Summary: []ResponsesSummary{{
				Type: "summary_text",
				Text: slot.reasoning.String(),
			}},
		}, true
	}

	if slot.text.Len() > 0 {
		return ResponsesOutput{
			Type: "message",
			Role: "assistant",
			Content: []ResponsesContentPart{{
				Type: "output_text",
				Text: slot.text.String(),
			}},
		}, true
	}

	if slot.funcArgs.Len() > 0 {
		return ResponsesOutput{
			Type:      "function_call",
			Arguments: slot.funcArgs.String(),
		}, true
	}

	return ResponsesOutput{}, false
}

func cloneResponsesOutput(item *ResponsesOutput) *ResponsesOutput {
	if item == nil {
		return nil
	}
	cloned := *item
	if item.Content != nil {
		cloned.Content = append([]ResponsesContentPart(nil), item.Content...)
	}
	if item.Summary != nil {
		cloned.Summary = append([]ResponsesSummary(nil), item.Summary...)
	}
	if item.Action != nil {
		action := *item.Action
		if item.Action.Sources != nil {
			action.Sources = append(json.RawMessage(nil), item.Action.Sources...)
		}
		cloned.Action = &action
	}
	return &cloned
}

func mergeIndexedFunctionCallItem(existing, incoming *ResponsesOutput) *ResponsesOutput {
	if existing == nil {
		return cloneResponsesOutput(incoming)
	}
	merged := cloneResponsesOutput(incoming)
	if merged == nil {
		return cloneResponsesOutput(existing)
	}
	merged.Type = firstNonEmpty(merged.Type, existing.Type)
	merged.CallID = firstNonEmpty(merged.CallID, existing.CallID)
	merged.Name = firstNonEmpty(merged.Name, existing.Name)
	merged.Arguments = firstNonEmpty(merged.Arguments, existing.Arguments)
	return merged
}

func mergeIndexedWebSearchCallItem(existing, incoming *ResponsesOutput) *ResponsesOutput {
	if existing == nil {
		return cloneResponsesOutput(incoming)
	}
	merged := cloneResponsesOutput(incoming)
	if merged == nil {
		return cloneResponsesOutput(existing)
	}
	merged.Type = firstNonEmpty(merged.Type, existing.Type)
	merged.ID = firstNonEmpty(merged.ID, existing.ID)
	merged.Status = firstNonEmpty(merged.Status, existing.Status)

	if existing.Action != nil {
		if merged.Action == nil {
			action := *existing.Action
			if existing.Action.Sources != nil {
				action.Sources = append(json.RawMessage(nil), existing.Action.Sources...)
			}
			merged.Action = &action
		} else {
			merged.Action.Type = firstNonEmpty(merged.Action.Type, existing.Action.Type)
			merged.Action.Query = firstNonEmpty(merged.Action.Query, existing.Action.Query)
			if isWeakWebSearchSources(merged.Action.Sources) && !isWeakWebSearchSources(existing.Action.Sources) {
				merged.Action.Sources = append(json.RawMessage(nil), existing.Action.Sources...)
			}
		}
	}

	return merged
}

func isWeakWebSearchSources(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "null" || trimmed == "[]"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
