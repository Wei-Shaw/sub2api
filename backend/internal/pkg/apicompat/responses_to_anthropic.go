package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Non-streaming: ResponsesResponse → AnthropicResponse
// ---------------------------------------------------------------------------

// ResponsesToAnthropic converts a Responses API response directly into an
// Anthropic Messages response. Reasoning output items are mapped to thinking
// blocks; function_call items become tool_use blocks.
func ResponsesToAnthropic(resp *ResponsesResponse, model string) *AnthropicResponse {
	out := &AnthropicResponse{
		ID:    resp.ID,
		Type:  "message",
		Role:  "assistant",
		Model: model,
		Usage: anthropicUsageFromResponsesUsage(nil),
	}

	var blocks []AnthropicContentBlock

	for _, item := range resp.Output {
		switch item.Type {
		case "reasoning":
			reasoningText := responsesReasoningText(item)
			// Always surface encrypted_content as thinking.signature so Claude
			// Code / multi-turn clients can send it back. Signature-only
			// thinking blocks are valid when the model omits a visible summary.
			if reasoningText != "" || strings.TrimSpace(item.EncryptedContent) != "" {
				blocks = append(blocks, AnthropicContentBlock{
					Type:      "thinking",
					Thinking:  reasoningText,
					Signature: item.EncryptedContent,
				})
			}
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" && part.Text != "" {
					blocks = append(blocks, AnthropicContentBlock{
						Type: "text",
						Text: part.Text,
					})
				}
			}
		case "function_call":
			blocks = append(blocks, AnthropicContentBlock{
				Type:  "tool_use",
				ID:    fromResponsesCallID(item.CallID),
				Name:  item.Name,
				Input: anthropicToolUseInput(item.Type, item.Name, item.Arguments),
			})
		case "custom_tool_call":
			if !resp.RenderCustomToolCalls || (resp.XAIServerSideSearch && IsServerSideSearchCall(item.Type, item.CallID)) {
				continue
			}
			blocks = append(blocks, AnthropicContentBlock{
				Type:  "tool_use",
				ID:    fromResponsesCallID(item.CallID),
				Name:  item.Name,
				Input: anthropicToolUseInput(item.Type, item.Name, item.Input),
			})
		case "tool_search_call":
			blocks = append(blocks, toolSearchCallToolUse(item))
		case "web_search_call":
			if item.Status != "completed" {
				continue
			}
			toolUseID := "srvtoolu_" + item.ID
			query := ""
			if item.Action != nil {
				query = item.Action.Query
			}
			inputJSON, _ := json.Marshal(map[string]string{"query": query})
			blocks = append(blocks, AnthropicContentBlock{
				Type:  "server_tool_use",
				ID:    toolUseID,
				Name:  "web_search",
				Input: inputJSON,
			})
			blocks = append(blocks, AnthropicContentBlock{
				Type:      "web_search_tool_result",
				ToolUseID: toolUseID,
				Content:   webSearchCallResults(&item),
			})
		}
	}

	if len(blocks) == 0 {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: ""})
	}
	out.Content = blocks

	out.StopReason = AnthropicStopReasonPtr(responsesStatusToAnthropicStopReason(resp.Status, resp.IncompleteDetails, blocks))

	if resp.Usage != nil {
		out.Usage = anthropicUsageFromResponsesUsage(resp.Usage)
	}

	return out
}

func anthropicUsageFromResponsesUsage(usage *ResponsesUsage) AnthropicUsage {
	if usage == nil {
		return AnthropicUsage{CacheCreationInputTokensPresent: true, CacheReadInputTokensPresent: true}
	}

	cachedTokens := 0
	if usage.InputTokensDetails != nil {
		cachedTokens = usage.InputTokensDetails.CachedTokens
	}

	inputTokens := usage.InputTokens - cachedTokens - usage.CacheCreationInputTokens
	if inputTokens < 0 {
		inputTokens = 0
	}

	return AnthropicUsage{
		InputTokens:                     inputTokens,
		OutputTokens:                    usage.OutputTokens,
		CacheReadInputTokens:            cachedTokens,
		CacheCreationInputTokens:        usage.CacheCreationInputTokens,
		CacheCreationInputTokensPresent: true,
		CacheReadInputTokensPresent:     true,
	}
}

func responsesStatusToAnthropicStopReason(status string, details *ResponsesIncompleteDetails, blocks []AnthropicContentBlock) string {
	switch status {
	case "incomplete":
		if details != nil && details.Reason == "max_output_tokens" {
			return "max_tokens"
		}
		return "end_turn"
	case "completed":
		if containsAnthropicToolUseBlock(blocks) {
			return "tool_use"
		}
		return "end_turn"
	default:
		return "end_turn"
	}
}

func containsAnthropicToolUseBlock(blocks []AnthropicContentBlock) bool {
	for _, block := range blocks {
		if block.Type == "tool_use" {
			return true
		}
	}
	return false
}

// anthropicToolUseInput returns the input of a non-streaming tool_use block,
// which must be a JSON object. An object is kept as it is. A function_call whose
// arguments are not an object (truncated by max_output_tokens, or empty) gets {};
// a custom_tool_call's free-form input is wrapped in the same {"input": ...}
// shape that downgrades a custom tool to a function tool, so that
// extractCustomToolCallInput gives the text back; an empty custom input is {} as
// it is in the streamed tool_use.
func anthropicToolUseInput(itemType, name, raw string) json.RawMessage {
	if trimmed := strings.TrimSpace(raw); strings.HasPrefix(trimmed, "{") && json.Valid([]byte(trimmed)) {
		return sanitizeAnthropicToolUseInput(name, raw)
	}
	if itemType == "custom_tool_call" && raw != "" {
		wrapped, err := json.Marshal(map[string]string{"input": raw})
		if err == nil {
			return wrapped
		}
	}
	return json.RawMessage("{}")
}

func sanitizeAnthropicToolUseInput(name string, raw string) json.RawMessage {
	if name != "Read" || raw == "" {
		return json.RawMessage(raw)
	}

	var input map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return json.RawMessage(raw)
	}

	if pages, ok := input["pages"]; !ok || string(pages) != `""` {
		return json.RawMessage(raw)
	}

	delete(input, "pages")
	sanitized, err := json.Marshal(input)
	if err != nil {
		return json.RawMessage(raw)
	}
	return sanitized
}

// ---------------------------------------------------------------------------
// Streaming: ResponsesStreamEvent → []AnthropicStreamEvent (stateful converter)
// ---------------------------------------------------------------------------

// responsesTextPart identifies one output_text part of a streamed response.
type responsesTextPart struct {
	OutputIndex  int
	ContentIndex int
}

type pendingToolBlock struct {
	OutputIndex int
	CallID      string
	Name        string
	ItemType    string

	BufferedArgs     string
	FinalArgs        string
	bufferedBuilder  *strings.Builder
	finalArgsBuilder *strings.Builder

	ArgsDone    bool
	EmittedArgs string
	Started     bool
	Closed      bool
}

// ResponsesEventToAnthropicState tracks state for converting a sequence of
// Responses SSE events directly into Anthropic SSE events.
type ResponsesEventToAnthropicState struct {
	MessageStartSent bool
	MessageStopSent  bool

	ContentBlockIndex          int
	ContentBlockOpen           bool
	CurrentBlockType           string // "text" | "thinking" | "tool_use"
	CurrentBlockOutputIndex    int
	CurrentBlockHasOutputIndex bool
	CurrentToolName            string
	CurrentToolArgs            string
	CurrentToolHadDelta        bool
	currentToolArgsBuffer      *strings.Builder
	currentToolArgsValidation  *streamStringBytes
	// PendingThinkingSignature is filled from reasoning.encrypted_content and
	// emitted as signature_delta before the thinking block is closed.
	PendingThinkingSignature string
	HasToolCall              bool

	ActiveToolOutputIndex *int
	PendingTools          map[int]*pendingToolBlock
	ToolOrder             []int
	ignoredToolOutputs    map[int]bool

	// XAIServerSideSearch is set by callers whose upstream is xAI. A
	// custom_tool_call that IsServerSideSearchCall then never becomes a tool_use
	// block, because xAI runs it itself.
	XAIServerSideSearch bool

	// OutputIndexToBlockIdx maps Responses output_index → Anthropic content block index.
	OutputIndexToBlockIdx map[int]int

	// textByPart records the text already delivered for each output_text part
	// so that a done payload can be reconciled against it. It outlives the
	// content block; closeCurrentBlock must not reset it.
	textByPart map[responsesTextPart]*strings.Builder
	// textDelivered records whether any assistant text reached the client.
	textDelivered bool

	InputTokens              int
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int

	ResponseID string
	Model      string
	Created    int64
}

// NewResponsesEventToAnthropicState returns an initialised stream state.
func NewResponsesEventToAnthropicState() *ResponsesEventToAnthropicState {
	return &ResponsesEventToAnthropicState{
		OutputIndexToBlockIdx: make(map[int]int),
		PendingTools:          make(map[int]*pendingToolBlock),
		ignoredToolOutputs:    make(map[int]bool),
		textByPart:            make(map[responsesTextPart]*strings.Builder),
		Created:               time.Now().Unix(),
	}
}

// ResponsesEventToAnthropicEvents converts a single Responses SSE event into
// zero or more Anthropic SSE events, updating state as it goes.
func ResponsesEventToAnthropicEvents(
	evt *ResponsesStreamEvent,
	state *ResponsesEventToAnthropicState,
) []AnthropicStreamEvent {
	switch evt.Type {
	case "response.created":
		return resToAnthHandleCreated(evt, state)
	case "response.output_item.added":
		return resToAnthHandleOutputItemAdded(evt, state)
	case "response.output_text.delta":
		return resToAnthHandleTextDelta(evt, state)
	case "response.output_text.done":
		return resToAnthHandleTextDone(evt, state)
	case "response.function_call_arguments.delta",
		// custom/freeform 工具的输入增量与 function_call 参数增量同形。
		"response.custom_tool_call_input.delta":
		return resToAnthHandleFuncArgsDelta(evt, state)
	case "response.function_call_arguments.done",
		"response.custom_tool_call_input.done":
		return resToAnthHandleFuncArgsDone(evt, state)
	case "response.output_item.done":
		return resToAnthHandleOutputItemDone(evt, state)
	case "response.reasoning_summary_text.delta",
		// 原始推理文本增量，与 reasoning summary 一样映射为 thinking。
		"response.reasoning_text.delta":
		return resToAnthHandleReasoningDelta(evt, state)
	case "response.reasoning_summary_text.done":
		// Keep the thinking block open until response.output_item.done.
		// Grok/Codex attach encrypted_content on the finished reasoning item;
		// closing early would drop signature_delta and break multi-turn cache.
		return nil
	// response.done 是 Realtime/WS 与项目透传路径使用的终止别名；
	// 普通 Responses HTTP SSE 的公开终止事件仍以 response.completed 为主。
	case "response.completed", "response.done", "response.incomplete", "response.failed":
		return resToAnthHandleCompleted(evt, state)
	default:
		return nil
	}
}

// FinalizeResponsesAnthropicStream emits synthetic termination events if the
// stream ended without a proper completion event.
func FinalizeResponsesAnthropicStream(state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	if !state.MessageStartSent || state.MessageStopSent {
		return nil
	}

	var events []AnthropicStreamEvent
	if state.ContentBlockOpen && state.CurrentBlockType != "tool_use" {
		events = append(events, closeCurrentBlock(state)...)
	}
	events = append(events, flushPendingToolBlocks(state, true)...)
	events = append(events, closeCurrentBlock(state)...)

	stopReason := "end_turn"
	if state.HasToolCall {
		stopReason = "tool_use"
	}

	events = append(events,
		AnthropicStreamEvent{
			Type: "message_delta",
			Delta: &AnthropicDelta{
				StopReason: stopReason,
			},
			Usage: &AnthropicUsage{
				InputTokens:                     state.InputTokens,
				OutputTokens:                    state.OutputTokens,
				CacheReadInputTokens:            state.CacheReadInputTokens,
				CacheCreationInputTokens:        state.CacheCreationInputTokens,
				CacheCreationInputTokensPresent: true,
				CacheReadInputTokensPresent:     true,
			},
		},
		AnthropicStreamEvent{Type: "message_stop"},
	)
	state.MessageStopSent = true
	return events
}

// ResponsesAnthropicEventToSSE formats an AnthropicStreamEvent as an SSE line pair.
func ResponsesAnthropicEventToSSE(evt AnthropicStreamEvent) (string, error) {
	data, err := json.Marshal(evt)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", evt.Type, data), nil
}

// --- internal handlers ---

func resToAnthHandleCreated(evt *ResponsesStreamEvent, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	if evt.Response != nil {
		state.ResponseID = evt.Response.ID
		// Only use upstream model if no override was set (e.g. originalModel)
		if state.Model == "" {
			state.Model = evt.Response.Model
		}
	}

	if state.MessageStartSent {
		return nil
	}
	state.MessageStartSent = true

	// Official Anthropic message_start uses stop_reason: null and usage with
	// input_tokens when known. We leave StopReason nil (JSON null) and usage
	// zeros until response.completed; never emit stop_reason:"" which breaks
	// strict clients' turn-finalization / session usage accounting.
	return []AnthropicStreamEvent{{
		Type: "message_start",
		Message: &AnthropicResponse{
			ID:         state.ResponseID,
			Type:       "message",
			Role:       "assistant",
			Content:    []AnthropicContentBlock{},
			Model:      state.Model,
			StopReason: nil,
			Usage: AnthropicUsage{
				InputTokens:                     0,
				OutputTokens:                    0,
				CacheCreationInputTokensPresent: true,
				CacheReadInputTokensPresent:     true,
			},
		},
	}}
}

func resToAnthHandleOutputItemAdded(evt *ResponsesStreamEvent, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	if evt.Item == nil {
		return nil
	}

	switch evt.Item.Type {
	// function_call 与 custom_tool_call（custom/freeform 工具，如新版 apply_patch）
	// 同样映射为 Anthropic 的 tool_use 块。
	case "function_call", "custom_tool_call":
		if state.XAIServerSideSearch && IsServerSideSearchCall(evt.Item.Type, evt.Item.CallID) {
			if state.ignoredToolOutputs == nil {
				state.ignoredToolOutputs = make(map[int]bool)
			}
			state.ignoredToolOutputs[evt.OutputIndex] = true
			return nil
		}
		var events []AnthropicStreamEvent
		if state.ContentBlockOpen && state.CurrentBlockType != "tool_use" {
			events = append(events, closeCurrentBlock(state)...)
		}

		tool := ensurePendingToolBlock(state, evt.OutputIndex)
		tool.CallID = evt.Item.CallID
		tool.Name = evt.Item.Name
		tool.ItemType = evt.Item.Type
		if evt.Item.Type == "custom_tool_call" && evt.Item.Input != "" {
			tool.FinalArgs = evt.Item.Input
		}
		state.HasToolCall = true

		events = append(events, flushPendingToolBlocks(state, false)...)
		return events

	case "reasoning":
		if state.ContentBlockOpen && state.CurrentBlockType == "tool_use" {
			return nil
		}
		var events []AnthropicStreamEvent
		events = append(events, closeCurrentBlock(state)...)

		idx := state.ContentBlockIndex
		state.OutputIndexToBlockIdx[evt.OutputIndex] = idx
		state.ContentBlockOpen = true
		state.CurrentBlockType = "thinking"
		state.CurrentBlockOutputIndex = evt.OutputIndex
		state.CurrentBlockHasOutputIndex = true
		state.PendingThinkingSignature = strings.TrimSpace(evt.Item.EncryptedContent)

		events = append(events, AnthropicStreamEvent{
			Type:  "content_block_start",
			Index: &idx,
			ContentBlock: &AnthropicContentBlock{
				Type:     "thinking",
				Thinking: "",
			},
		})
		return events

	case "message":
		return nil
	}

	return nil
}

func resToAnthHandleTextDelta(evt *ResponsesStreamEvent, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	return resToAnthEmitText(evt.Delta, resToAnthTextPartOf(evt), state)
}

func resToAnthTextPartOf(evt *ResponsesStreamEvent) responsesTextPart {
	return responsesTextPart{OutputIndex: evt.OutputIndex, ContentIndex: evt.ContentIndex}
}

// resToAnthEmitText opens a text block when needed, emits text, and records it
// against its part so that a later payload for the same part is reconciled
// against what the client already received.
func resToAnthEmitText(text string, part responsesTextPart, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	if text == "" {
		return nil
	}

	var events []AnthropicStreamEvent

	if !state.ContentBlockOpen || state.CurrentBlockType != "text" {
		events = append(events, flushPendingToolBlocks(state, true)...)
		events = append(events, closeCurrentBlock(state)...)

		idx := state.ContentBlockIndex
		state.ContentBlockOpen = true
		state.CurrentBlockType = "text"
		state.CurrentBlockOutputIndex = part.OutputIndex
		state.CurrentBlockHasOutputIndex = true

		events = append(events, AnthropicStreamEvent{
			Type:  "content_block_start",
			Index: &idx,
			ContentBlock: &AnthropicContentBlock{
				Type: "text",
				Text: "",
			},
		})
	}

	delivered, ok := state.textByPart[part]
	if !ok {
		delivered = &strings.Builder{}
		state.textByPart[part] = delivered
	}
	_, _ = delivered.WriteString(text)
	state.textDelivered = true

	idx := state.ContentBlockIndex
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_delta",
		Index: &idx,
		Delta: &AnthropicDelta{
			Type: "text_delta",
			Text: text,
		},
	})
	return events
}

// resToAnthRecoverText emits the tail of a finished text payload that never
// reached the client. Streamed text cannot be recalled, so a payload that does
// not extend what was already delivered is left alone.
func resToAnthRecoverText(text string, part responsesTextPart, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	builder, known := state.textByPart[part]
	if !known && state.textDelivered {
		// The payload is indexed differently from every delta seen so far, so
		// which part it finishes cannot be established. Recovering it could
		// repeat an answer the client already has, which is worse than leaving
		// a partially delivered one alone.
		return nil
	}

	var delivered string
	if known {
		delivered = builder.String()
	}
	if text == delivered || !strings.HasPrefix(text, delivered) {
		return nil
	}
	return resToAnthEmitText(text[len(delivered):], part, state)
}

// resToAnthHandleTextDone recovers text that upstream carried only on the done
// event before closing the block, which some streams use instead of sending
// output_text.delta at all.
func resToAnthHandleTextDone(evt *ResponsesStreamEvent, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	if state.MessageStopSent {
		// The message is already terminated; a late payload cannot be delivered
		// without emitting a content block after message_stop.
		return resToAnthHandleBlockDone(evt, state, "text")
	}

	events := resToAnthRecoverText(evt.Text, resToAnthTextPartOf(evt), state)
	return append(events, resToAnthHandleBlockDone(evt, state, "text")...)
}

func ensurePendingToolBlock(state *ResponsesEventToAnthropicState, outputIndex int) *pendingToolBlock {
	if state.PendingTools == nil {
		state.PendingTools = make(map[int]*pendingToolBlock)
	}
	if tool, ok := state.PendingTools[outputIndex]; ok {
		return tool
	}
	tool := &pendingToolBlock{OutputIndex: outputIndex}
	state.PendingTools[outputIndex] = tool
	state.ToolOrder = append(state.ToolOrder, outputIndex)
	return tool
}

func findPendingToolBlock(
	state *ResponsesEventToAnthropicState,
	outputIndex int,
	callID string,
) *pendingToolBlock {
	if tool := state.PendingTools[outputIndex]; tool != nil &&
		(callID == "" || tool.CallID == "" || tool.CallID == callID) {
		return tool
	}
	if callID != "" {
		for _, queuedOutputIndex := range state.ToolOrder {
			tool := state.PendingTools[queuedOutputIndex]
			if tool != nil && tool.CallID == callID {
				return tool
			}
		}
	}
	if tool := state.PendingTools[outputIndex]; tool != nil {
		return tool
	}
	if callID == "" && state.ActiveToolOutputIndex != nil {
		return state.PendingTools[*state.ActiveToolOutputIndex]
	}
	return nil
}

// flushPendingToolBlocks maintains at most one active Anthropic tool_use block.
// Sibling parallel tools announced while the active tool is still streaming its
// arguments remain queued in PendingTools and are emitted sequentially once the
// active block finishes, preventing premature empty-input closes and deltas on
// already-closed blocks.
func flushPendingToolBlocks(
	state *ResponsesEventToAnthropicState,
	force bool,
) []AnthropicStreamEvent {
	var events []AnthropicStreamEvent

	if force {
		for _, outputIndex := range state.ToolOrder {
			if tool := state.PendingTools[outputIndex]; tool != nil && !tool.Closed {
				tool.ArgsDone = true
			}
		}
	}

	for {
		if state.ActiveToolOutputIndex != nil {
			tool := state.PendingTools[*state.ActiveToolOutputIndex]
			if tool == nil || tool.Closed {
				state.ActiveToolOutputIndex = nil
				continue
			}

			blockIdx, ok := state.OutputIndexToBlockIdx[tool.OutputIndex]
			if !ok {
				return events
			}

			if !state.CurrentToolHadDelta {
				if state.CurrentToolName == "Read" {
					raw := state.CurrentToolArgs
					if tool.ArgsDone && tool.FinalArgs != "" {
						raw = tool.FinalArgs
						state.CurrentToolArgs = raw
					} else if raw == "" {
						raw = tool.BufferedArgs
						state.CurrentToolArgs = raw
					}
					if raw != "" {
						valid := tool.ArgsDone || json.Valid(appendStreamStringBytes(&state.currentToolArgsValidation, "", raw, raw))
						if valid {
							sanitized := sanitizeAnthropicToolUseInput(state.CurrentToolName, raw)
							if len(sanitized) > 0 && (json.Valid([]byte(raw)) || tool.ArgsDone) {
								events = append(events, AnthropicStreamEvent{
									Type:  "content_block_delta",
									Index: &blockIdx,
									Delta: &AnthropicDelta{
										Type:        "input_json_delta",
										PartialJSON: string(sanitized),
									},
								})
								state.CurrentToolHadDelta = true
								tool.EmittedArgs = raw
							}
						}
					}
				} else {
					raw := tool.BufferedArgs
					if tool.ArgsDone && tool.FinalArgs != "" {
						raw = tool.FinalArgs
					} else if raw == "" && tool.ArgsDone {
						raw = state.CurrentToolArgs
					}
					if raw != "" {
						events = append(events, AnthropicStreamEvent{
							Type:  "content_block_delta",
							Index: &blockIdx,
							Delta: &AnthropicDelta{
								Type:        "input_json_delta",
								PartialJSON: raw,
							},
						})
						state.CurrentToolArgs = raw
						tool.EmittedArgs = raw
						state.CurrentToolHadDelta = true
					}
				}
			} else if state.CurrentToolName != "Read" && tool.ArgsDone && tool.FinalArgs != "" {
				delivered := state.CurrentToolArgs
				if len(tool.FinalArgs) > len(delivered) && strings.HasPrefix(tool.FinalArgs, delivered) {
					tail := tool.FinalArgs[len(delivered):]
					events = append(events, AnthropicStreamEvent{
						Type:  "content_block_delta",
						Index: &blockIdx,
						Delta: &AnthropicDelta{
							Type:        "input_json_delta",
							PartialJSON: tail,
						},
					})
					state.CurrentToolArgs = tool.FinalArgs
					tool.EmittedArgs = tool.FinalArgs
				}
			}
			tool.BufferedArgs = ""
			tool.bufferedBuilder = nil

			if !tool.ArgsDone {
				return events
			}

			events = append(events, closeCurrentBlock(state)...)
			continue
		}

		if state.ContentBlockOpen {
			return events
		}

		var next *pendingToolBlock
		for _, outputIndex := range state.ToolOrder {
			tool := state.PendingTools[outputIndex]
			if tool != nil && !tool.Started && !tool.Closed {
				next = tool
				break
			}
		}
		if next == nil {
			return events
		}

		idx := state.ContentBlockIndex
		state.OutputIndexToBlockIdx[next.OutputIndex] = idx
		state.ContentBlockOpen = true
		state.CurrentBlockType = "tool_use"
		state.CurrentBlockOutputIndex = next.OutputIndex
		state.CurrentBlockHasOutputIndex = true
		state.CurrentToolName = next.Name
		state.CurrentToolArgs = next.FinalArgs
		state.currentToolArgsBuffer = nil
		state.currentToolArgsValidation = nil
		state.CurrentToolHadDelta = false
		activeOutputIndex := next.OutputIndex
		state.ActiveToolOutputIndex = &activeOutputIndex
		next.Started = true

		events = append(events, AnthropicStreamEvent{
			Type:  "content_block_start",
			Index: &idx,
			ContentBlock: &AnthropicContentBlock{
				Type:  "tool_use",
				ID:    fromResponsesCallID(next.CallID),
				Name:  next.Name,
				Input: json.RawMessage("{}"),
			},
		})
	}
}

func resToAnthEmitActiveToolDelta(
	evt *ResponsesStreamEvent,
	state *ResponsesEventToAnthropicState,
	outputIndex int,
) []AnthropicStreamEvent {
	previousArgs := state.CurrentToolArgs
	state.CurrentToolArgs = appendStreamString(&state.currentToolArgsBuffer, state.CurrentToolArgs, evt.Delta)

	if state.CurrentToolName == "Read" {
		if state.CurrentToolHadDelta {
			return nil
		}
		if !json.Valid(appendStreamStringBytes(&state.currentToolArgsValidation, previousArgs, state.CurrentToolArgs, evt.Delta)) {
			return nil
		}
		blockIdx, ok := state.OutputIndexToBlockIdx[outputIndex]
		if !ok {
			return nil
		}
		state.CurrentToolHadDelta = true
		sanitized := sanitizeAnthropicToolUseInput(state.CurrentToolName, state.CurrentToolArgs)
		return []AnthropicStreamEvent{{
			Type:  "content_block_delta",
			Index: &blockIdx,
			Delta: &AnthropicDelta{
				Type:        "input_json_delta",
				PartialJSON: string(sanitized),
			},
		}}
	}

	blockIdx, ok := state.OutputIndexToBlockIdx[outputIndex]
	if !ok {
		return nil
	}
	state.CurrentToolHadDelta = true
	return []AnthropicStreamEvent{{
		Type:  "content_block_delta",
		Index: &blockIdx,
		Delta: &AnthropicDelta{
			Type:        "input_json_delta",
			PartialJSON: evt.Delta,
		},
	}}
}

func resToAnthHandleFuncArgsDelta(evt *ResponsesStreamEvent, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	if evt.Delta == "" || state.ignoredToolOutputs[evt.OutputIndex] {
		return nil
	}

	// Fast path for the active tool block (and legacy white-box tests that
	// initialize state fields directly without PendingTools): mutate only value
	// fields on *state so shallow struct copies remain isolated.
	if state.CurrentBlockType == "tool_use" &&
		((state.ActiveToolOutputIndex == nil && len(state.PendingTools) == 0) ||
			(state.ContentBlockOpen && state.ActiveToolOutputIndex != nil && *state.ActiveToolOutputIndex == evt.OutputIndex)) {
		return resToAnthEmitActiveToolDelta(evt, state, evt.OutputIndex)
	}

	tool := findPendingToolBlock(state, evt.OutputIndex, evt.CallID)
	if tool == nil {
		tool = ensurePendingToolBlock(state, evt.OutputIndex)
		tool.CallID = evt.CallID
		tool.Name = evt.Name
		state.HasToolCall = true
	}
	if tool.Closed {
		return nil
	}
	if state.ActiveToolOutputIndex != nil && *state.ActiveToolOutputIndex == tool.OutputIndex && tool.Started {
		return resToAnthEmitActiveToolDelta(evt, state, tool.OutputIndex)
	}

	tool.FinalArgs = appendStreamString(&tool.finalArgsBuilder, tool.FinalArgs, evt.Delta)
	tool.BufferedArgs = appendStreamString(&tool.bufferedBuilder, tool.BufferedArgs, evt.Delta)
	return flushPendingToolBlocks(state, false)
}

func resToAnthHandleFuncArgsDone(evt *ResponsesStreamEvent, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	if state.ignoredToolOutputs[evt.OutputIndex] {
		return nil
	}

	if state.ActiveToolOutputIndex == nil && len(state.PendingTools) == 0 {
		if !state.ContentBlockOpen {
			return nil
		}
		if state.CurrentBlockType != "tool_use" {
			return resToAnthHandleBlockDone(evt, state, state.CurrentBlockType)
		}
		raw := evt.Arguments
		if raw == "" {
			raw = evt.Input
		}
		if raw == "" {
			raw = state.CurrentToolArgs
		}
		if raw == "" || state.CurrentToolHadDelta {
			return closeCurrentBlock(state)
		}
		if state.CurrentToolName == "Read" {
			sanitized := sanitizeAnthropicToolUseInput(state.CurrentToolName, raw)
			if len(sanitized) == 0 {
				return closeCurrentBlock(state)
			}
			raw = string(sanitized)
		}
		blockIdx, ok := state.OutputIndexToBlockIdx[evt.OutputIndex]
		if !ok {
			blockIdx = state.ContentBlockIndex
		}
		if !state.ContentBlockOpen || blockIdx != state.ContentBlockIndex {
			return nil
		}
		events := []AnthropicStreamEvent{{
			Type:  "content_block_delta",
			Index: &blockIdx,
			Delta: &AnthropicDelta{
				Type:        "input_json_delta",
				PartialJSON: raw,
			},
		}}
		return append(events, closeCurrentBlock(state)...)
	}

	tool := findPendingToolBlock(state, evt.OutputIndex, evt.CallID)
	if tool == nil {
		if !state.ContentBlockOpen {
			return nil
		}
		if state.CurrentBlockType != "tool_use" {
			return resToAnthHandleBlockDone(evt, state, state.CurrentBlockType)
		}
		tool = ensurePendingToolBlock(state, evt.OutputIndex)
		tool.CallID = evt.CallID
		tool.Name = evt.Name
		state.HasToolCall = true
	}
	if tool.Closed {
		return nil
	}

	raw := evt.Arguments
	if raw == "" {
		raw = evt.Input
	}
	if raw != "" {
		tool.FinalArgs = raw
		tool.finalArgsBuilder = nil
	}
	tool.ArgsDone = true
	return flushPendingToolBlocks(state, false)
}

func resToAnthHandleReasoningDelta(evt *ResponsesStreamEvent, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	if evt.Delta == "" {
		return nil
	}
	if !state.ContentBlockOpen ||
		state.CurrentBlockType != "thinking" ||
		(state.CurrentBlockHasOutputIndex && state.CurrentBlockOutputIndex != evt.OutputIndex) {
		return nil
	}

	blockIdx, ok := state.OutputIndexToBlockIdx[evt.OutputIndex]
	if !ok {
		return nil
	}

	return []AnthropicStreamEvent{{
		Type:  "content_block_delta",
		Index: &blockIdx,
		Delta: &AnthropicDelta{
			Type:     "thinking_delta",
			Thinking: evt.Delta,
		},
	}}
}

func resToAnthHandleBlockDone(
	evt *ResponsesStreamEvent,
	state *ResponsesEventToAnthropicState,
	expectedBlockType string,
) []AnthropicStreamEvent {
	if !state.ContentBlockOpen {
		return nil
	}
	if expectedBlockType != "" && state.CurrentBlockType != expectedBlockType {
		return nil
	}
	if evt != nil && state.CurrentBlockHasOutputIndex && state.CurrentBlockOutputIndex != evt.OutputIndex {
		return nil
	}
	return closeCurrentBlock(state)
}

func resToAnthHandleOutputItemDone(evt *ResponsesStreamEvent, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	if evt.Item == nil {
		return nil
	}

	// Handle web_search_call → synthesize server_tool_use + web_search_tool_result blocks.
	if evt.Item.Type == "web_search_call" && evt.Item.Status == "completed" {
		return resToAnthHandleWebSearchDone(evt, state)
	}
	// The native tool_search_call carries its arguments only on done.
	if evt.Item.Type == "tool_search_call" {
		return resToAnthHandleToolSearchDone(evt, state)
	}

	if evt.Item.Type == "function_call" || evt.Item.Type == "custom_tool_call" {
		if state.ignoredToolOutputs[evt.OutputIndex] || (state.XAIServerSideSearch && IsServerSideSearchCall(evt.Item.Type, evt.Item.CallID)) {
			return nil
		}
		tool := findPendingToolBlock(state, evt.OutputIndex, evt.Item.CallID)
		if tool == nil {
			tool = ensurePendingToolBlock(state, evt.OutputIndex)
		}
		if evt.Item.CallID != "" {
			tool.CallID = evt.Item.CallID
		}
		if evt.Item.Name != "" {
			tool.Name = evt.Item.Name
		}
		if evt.Item.Type != "" {
			tool.ItemType = evt.Item.Type
		}
		raw := evt.Item.Arguments
		if raw == "" {
			raw = evt.Item.Input
		}
		if raw != "" {
			tool.FinalArgs = raw
			tool.finalArgsBuilder = nil
		}
		tool.ArgsDone = true
		state.HasToolCall = true
		return flushPendingToolBlocks(state, false)
	}

	// Capture encrypted_content on reasoning item done (often only present here).
	if evt.Item.Type == "reasoning" {
		if sig := strings.TrimSpace(evt.Item.EncryptedContent); sig != "" {
			state.PendingThinkingSignature = sig
		}
		return resToAnthHandleBlockDone(evt, state, "thinking")
	}
	if evt.Item.Type == "message" {
		return resToAnthHandleBlockDone(evt, state, "text")
	}

	return nil
}

// resToAnthHandleWebSearchDone converts an OpenAI web_search_call output item
// into Anthropic server_tool_use + web_search_tool_result content block pairs.
// This allows Claude Code to count the searches performed.
func resToAnthHandleWebSearchDone(evt *ResponsesStreamEvent, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	var events []AnthropicStreamEvent
	if state.ContentBlockOpen && state.CurrentBlockType == "tool_use" {
		events = append(events, flushPendingToolBlocks(state, false)...)
	}
	events = append(events, closeCurrentBlock(state)...)

	toolUseID := "srvtoolu_" + evt.Item.ID
	query := ""
	if evt.Item.Action != nil {
		query = evt.Item.Action.Query
	}
	inputJSON, _ := json.Marshal(map[string]string{"query": query})

	// Emit server_tool_use block (start + stop).
	idx1 := state.ContentBlockIndex
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_start",
		Index: &idx1,
		ContentBlock: &AnthropicContentBlock{
			Type:  "server_tool_use",
			ID:    toolUseID,
			Name:  "web_search",
			Input: inputJSON,
		},
	})
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_stop",
		Index: &idx1,
	})
	state.ContentBlockIndex++

	// Emit web_search_tool_result block (start + stop).
	idx2 := state.ContentBlockIndex
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_start",
		Index: &idx2,
		ContentBlock: &AnthropicContentBlock{
			Type:      "web_search_tool_result",
			ToolUseID: toolUseID,
			Content:   webSearchCallResults(evt.Item),
		},
	})
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_stop",
		Index: &idx2,
	})
	state.ContentBlockIndex++

	return events
}

// resToAnthRecoverTerminalText emits assistant text that only ever appeared in
// the terminal response payload, which some streams populate without sending
// any output_text event.
//
// It only runs when no text at all reached the client. Streamed events and the
// terminal output array carry no guaranteed common identity, so reconciling
// them part by part risks repeating an answer the client already has, which is
// worse than leaving a partially streamed response as it is.
func resToAnthRecoverTerminalText(evt *ResponsesStreamEvent, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	if state.textDelivered || evt.Response == nil {
		return nil
	}

	var events []AnthropicStreamEvent
	for outputIndex, item := range evt.Response.Output {
		if item.Type != "message" {
			continue
		}
		for contentIndex, content := range item.Content {
			if content.Type != "output_text" {
				continue
			}
			part := responsesTextPart{OutputIndex: outputIndex, ContentIndex: contentIndex}
			events = append(events, resToAnthEmitText(content.Text, part, state)...)
		}
	}
	if len(events) > 0 {
		events = append(events, closeCurrentBlock(state)...)
	}
	return events
}

func hydratePendingToolsFromCompletedResponse(
	state *ResponsesEventToAnthropicState,
	resp *ResponsesResponse,
) {
	if resp == nil {
		return
	}
	for outputIndex, item := range resp.Output {
		if item.Type != "function_call" && item.Type != "custom_tool_call" {
			continue
		}
		if state.ignoredToolOutputs[outputIndex] || (state.XAIServerSideSearch && IsServerSideSearchCall(item.Type, item.CallID)) {
			continue
		}
		tool := findPendingToolBlock(state, outputIndex, item.CallID)
		if tool == nil {
			if item.Type == "custom_tool_call" && !resp.RenderCustomToolCalls {
				continue
			}
			tool = ensurePendingToolBlock(state, outputIndex)
		}
		if item.CallID != "" {
			tool.CallID = item.CallID
		}
		if item.Name != "" {
			tool.Name = item.Name
		}
		if item.Type != "" {
			tool.ItemType = item.Type
		}
		raw := item.Arguments
		if raw == "" {
			raw = item.Input
		}
		if raw != "" {
			tool.FinalArgs = raw
			tool.finalArgsBuilder = nil
		}
		tool.ArgsDone = true
		state.HasToolCall = true
	}
}

func resToAnthHandleCompleted(evt *ResponsesStreamEvent, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	if state.MessageStopSent {
		return nil
	}

	var events []AnthropicStreamEvent
	hydratePendingToolsFromCompletedResponse(state, evt.Response)
	if state.ContentBlockOpen && state.CurrentBlockType != "tool_use" {
		events = append(events, closeCurrentBlock(state)...)
	}
	if state.ActiveToolOutputIndex == nil {
		events = append(events, resToAnthRecoverTerminalText(evt, state)...)
	}
	events = append(events, flushPendingToolBlocks(state, true)...)
	events = append(events, closeCurrentBlock(state)...)
	events = append(events, resToAnthRecoverTerminalText(evt, state)...)

	stopReason := "end_turn"
	if evt.Usage != nil {
		usage := anthropicUsageFromResponsesUsage(evt.Usage)
		state.InputTokens = usage.InputTokens
		state.OutputTokens = usage.OutputTokens
		state.CacheReadInputTokens = usage.CacheReadInputTokens
		state.CacheCreationInputTokens = usage.CacheCreationInputTokens
	}
	if evt.Response != nil {
		if evt.Response.Usage != nil {
			usage := anthropicUsageFromResponsesUsage(evt.Response.Usage)
			state.InputTokens = usage.InputTokens
			state.OutputTokens = usage.OutputTokens
			state.CacheReadInputTokens = usage.CacheReadInputTokens
			state.CacheCreationInputTokens = usage.CacheCreationInputTokens
		}
		switch evt.Response.Status {
		case "incomplete":
			if evt.Response.IncompleteDetails != nil && evt.Response.IncompleteDetails.Reason == "max_output_tokens" {
				stopReason = "max_tokens"
			}
		case "completed":
			if state.HasToolCall {
				stopReason = "tool_use"
			}
		}
	}

	events = append(events,
		AnthropicStreamEvent{
			Type: "message_delta",
			Delta: &AnthropicDelta{
				StopReason: stopReason,
			},
			Usage: &AnthropicUsage{
				InputTokens:                     state.InputTokens,
				OutputTokens:                    state.OutputTokens,
				CacheReadInputTokens:            state.CacheReadInputTokens,
				CacheCreationInputTokens:        state.CacheCreationInputTokens,
				CacheCreationInputTokensPresent: true,
				CacheReadInputTokensPresent:     true,
			},
		},
		AnthropicStreamEvent{Type: "message_stop"},
	)
	state.MessageStopSent = true
	return events
}

func closeCurrentBlock(state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	if !state.ContentBlockOpen {
		return nil
	}
	idx := state.ContentBlockIndex
	var events []AnthropicStreamEvent
	// Emit signature_delta before stop so Claude clients retain encrypted
	// reasoning for the next turn (required for Grok multi-turn cache).
	if state.CurrentBlockType == "thinking" {
		if sig := strings.TrimSpace(state.PendingThinkingSignature); sig != "" {
			events = append(events, AnthropicStreamEvent{
				Type:  "content_block_delta",
				Index: &idx,
				Delta: &AnthropicDelta{
					Type:      "signature_delta",
					Signature: sig,
				},
			})
		}
		state.PendingThinkingSignature = ""
	}
	if state.CurrentBlockType == "tool_use" && state.ActiveToolOutputIndex != nil {
		if tool := state.PendingTools[*state.ActiveToolOutputIndex]; tool != nil {
			tool.Closed = true
		}
		state.ActiveToolOutputIndex = nil
	}
	state.ContentBlockOpen = false
	state.ContentBlockIndex++
	state.CurrentBlockType = ""
	state.CurrentBlockOutputIndex = 0
	state.CurrentBlockHasOutputIndex = false
	state.CurrentToolName = ""
	state.CurrentToolArgs = ""
	state.currentToolArgsBuffer = nil
	state.currentToolArgsValidation = nil
	state.CurrentToolHadDelta = false
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_stop",
		Index: &idx,
	})
	return events
}
