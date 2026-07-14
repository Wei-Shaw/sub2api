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
	}

	var blocks []AnthropicContentBlock

	for _, item := range resp.Output {
		switch item.Type {
		case "reasoning":
			summaryText := ""
			for _, s := range item.Summary {
				if s.Type == "summary_text" && s.Text != "" {
					summaryText += s.Text
				}
			}
			if summaryText != "" {
				blocks = append(blocks, AnthropicContentBlock{
					Type:     "thinking",
					Thinking: summaryText,
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
				Input: sanitizeAnthropicToolUseInput(item.Name, item.Arguments),
			})
		case "web_search_call":
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
			emptyResults, _ := json.Marshal([]struct{}{})
			blocks = append(blocks, AnthropicContentBlock{
				Type:      "web_search_tool_result",
				ToolUseID: toolUseID,
				Content:   emptyResults,
			})
		}
	}

	if len(blocks) == 0 {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: ""})
	}
	out.Content = blocks

	out.StopReason = responsesStatusToAnthropicStopReason(resp.Status, resp.IncompleteDetails, blocks)

	if resp.Usage != nil {
		out.Usage = anthropicUsageFromResponsesUsage(resp.Usage)
	}

	return out
}

func anthropicUsageFromResponsesUsage(usage *ResponsesUsage) AnthropicUsage {
	if usage == nil {
		return AnthropicUsage{}
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
		InputTokens:              inputTokens,
		OutputTokens:             usage.OutputTokens,
		CacheReadInputTokens:     cachedTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
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

type pendingToolBlock struct {
	OutputIndex int
	CallID      string
	Name        string

	BufferedArgs string
	FinalArgs    string

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
	HasToolCall                bool

	ActiveToolOutputIndex *int
	PendingTools          map[int]*pendingToolBlock
	ToolOrder             []int

	// OutputIndexToBlockIdx maps Responses output_index → Anthropic content block index.
	OutputIndexToBlockIdx map[int]int

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
		return resToAnthHandleBlockDone(evt, state, "text")
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
	case "response.reasoning_summary_text.done",
		"response.reasoning_text.done":
		return resToAnthHandleBlockDone(evt, state, "thinking")
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
				InputTokens:              state.InputTokens,
				OutputTokens:             state.OutputTokens,
				CacheReadInputTokens:     state.CacheReadInputTokens,
				CacheCreationInputTokens: state.CacheCreationInputTokens,
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

	return []AnthropicStreamEvent{{
		Type: "message_start",
		Message: &AnthropicResponse{
			ID:      state.ResponseID,
			Type:    "message",
			Role:    "assistant",
			Content: []AnthropicContentBlock{},
			Model:   state.Model,
			Usage: AnthropicUsage{
				InputTokens:  0,
				OutputTokens: 0,
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
		var events []AnthropicStreamEvent
		if state.ContentBlockOpen && state.CurrentBlockType != "tool_use" {
			events = append(events, closeCurrentBlock(state)...)
		}

		tool := ensurePendingToolBlock(state, evt.OutputIndex)
		tool.CallID = evt.Item.CallID
		tool.Name = evt.Item.Name
		if evt.Item.Type == "custom_tool_call" && evt.Item.Input != "" {
			tool.FinalArgs = evt.Item.Input
		}
		state.HasToolCall = true

		events = append(events, flushPendingToolBlocks(state, false)...)
		return events

	case "reasoning":
		// A reasoning item can be followed by a late item notification after a
		// tool block has already started. Anthropic allows only one active
		// content block here, so never let that notification close the tool.
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
	if evt.Delta == "" {
		return nil
	}
	// A late text event must never close an active tool block. Tool blocks are
	// serialized independently and advance only when their own arguments are
	// complete.
	if state.ContentBlockOpen && state.CurrentBlockType == "tool_use" {
		return nil
	}

	var events []AnthropicStreamEvent

	if !state.ContentBlockOpen || state.CurrentBlockType != "text" {
		events = append(events, closeCurrentBlock(state)...)

		idx := state.ContentBlockIndex
		state.ContentBlockOpen = true
		state.CurrentBlockType = "text"
		state.CurrentBlockOutputIndex = evt.OutputIndex
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

	idx := state.ContentBlockIndex
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_delta",
		Index: &idx,
		Delta: &AnthropicDelta{
			Type: "text_delta",
			Text: evt.Delta,
		},
	})
	return events
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
	// Some compatible upstreams omit output_index on the final arguments event.
	// If there is exactly one active tool, it is the only safe fallback.
	if callID == "" && state.ActiveToolOutputIndex != nil {
		return state.PendingTools[*state.ActiveToolOutputIndex]
	}
	return nil
}

func pendingToolArgsForEmission(tool *pendingToolBlock) string {
	if tool.Name == "Read" {
		if tool.EmittedArgs != "" {
			return ""
		}
		raw := tool.FinalArgs
		if raw == "" {
			raw = tool.BufferedArgs
		}
		if raw == "" || !json.Valid([]byte(raw)) {
			return ""
		}
		return string(sanitizeAnthropicToolUseInput(tool.Name, raw))
	}

	raw := tool.BufferedArgs
	if tool.ArgsDone && tool.FinalArgs != "" {
		switch {
		case tool.EmittedArgs == "":
			raw = tool.FinalArgs
		case strings.HasPrefix(tool.FinalArgs, tool.EmittedArgs):
			raw = strings.TrimPrefix(tool.FinalArgs, tool.EmittedArgs)
		}
	}
	if raw == "" {
		return ""
	}
	return raw
}

// flushPendingToolBlocks maintains exactly one active Anthropic tool_use block.
// Later Grok tools remain queued until the active block is stopped; any deltas
// received meanwhile are buffered and emitted after that tool's start event.
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

			raw := pendingToolArgsForEmission(tool)
			if raw != "" {
				events = append(events, AnthropicStreamEvent{
					Type:  "content_block_delta",
					Index: &blockIdx,
					Delta: &AnthropicDelta{
						Type:        "input_json_delta",
						PartialJSON: raw,
					},
				})
				tool.EmittedArgs += raw
			}
			tool.BufferedArgs = ""

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

func resToAnthHandleFuncArgsDelta(evt *ResponsesStreamEvent, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	if evt.Delta == "" {
		return nil
	}

	tool := findPendingToolBlock(state, evt.OutputIndex, evt.CallID)
	if tool == nil {
		tool = ensurePendingToolBlock(state, evt.OutputIndex)
		tool.CallID = evt.CallID
		tool.Name = evt.Name
		state.HasToolCall = true
	}
	tool.FinalArgs += evt.Delta

	// Read arguments must be sanitized as one complete JSON object. Keep its
	// deltas in the pending-tool state until the accumulated input is valid;
	// non-Read tools continue to stream partial JSON immediately.
	if tool.Name == "Read" {
		tool.BufferedArgs += evt.Delta
		return flushPendingToolBlocks(state, false)
	}

	if state.ActiveToolOutputIndex == nil || *state.ActiveToolOutputIndex != tool.OutputIndex || !tool.Started || tool.Closed {
		tool.BufferedArgs += evt.Delta
		return flushPendingToolBlocks(state, false)
	}

	blockIdx, ok := state.OutputIndexToBlockIdx[tool.OutputIndex]
	if !ok {
		tool.BufferedArgs += evt.Delta
		return nil
	}
	tool.EmittedArgs += evt.Delta

	return []AnthropicStreamEvent{{
		Type:  "content_block_delta",
		Index: &blockIdx,
		Delta: &AnthropicDelta{
			Type:        "input_json_delta",
			PartialJSON: evt.Delta,
		},
	}}
}

func resToAnthHandleFuncArgsDone(evt *ResponsesStreamEvent, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	tool := findPendingToolBlock(state, evt.OutputIndex, evt.CallID)
	if tool == nil {
		tool = ensurePendingToolBlock(state, evt.OutputIndex)
		tool.CallID = evt.CallID
		tool.Name = evt.Name
		state.HasToolCall = true
	}

	raw := evt.Arguments
	if raw == "" {
		raw = evt.Input
	}
	if raw != "" {
		tool.FinalArgs = raw
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
	if state.CurrentBlockType != expectedBlockType {
		return nil
	}
	if state.CurrentBlockHasOutputIndex && state.CurrentBlockOutputIndex != evt.OutputIndex {
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

	if evt.Item.Type == "function_call" || evt.Item.Type == "custom_tool_call" {
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
		raw := evt.Item.Arguments
		if raw == "" {
			raw = evt.Item.Input
		}
		if raw != "" {
			tool.FinalArgs = raw
		}
		tool.ArgsDone = true
		state.HasToolCall = true
		return flushPendingToolBlocks(state, false)
	}

	switch evt.Item.Type {
	case "reasoning":
		return resToAnthHandleBlockDone(evt, state, "thinking")
	case "message":
		return resToAnthHandleBlockDone(evt, state, "text")
	}
	return nil
}

// resToAnthHandleWebSearchDone converts an OpenAI web_search_call output item
// into Anthropic server_tool_use + web_search_tool_result content block pairs.
// This allows Claude Code to count the searches performed.
func resToAnthHandleWebSearchDone(evt *ResponsesStreamEvent, state *ResponsesEventToAnthropicState) []AnthropicStreamEvent {
	var events []AnthropicStreamEvent
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
	// Content is empty because OpenAI does not expose individual search results;
	// the model consumes them internally and produces text output.
	emptyResults, _ := json.Marshal([]struct{}{})
	idx2 := state.ContentBlockIndex
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_start",
		Index: &idx2,
		ContentBlock: &AnthropicContentBlock{
			Type:      "web_search_tool_result",
			ToolUseID: toolUseID,
			Content:   emptyResults,
		},
	})
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_stop",
		Index: &idx2,
	})
	state.ContentBlockIndex++

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
		tool := findPendingToolBlock(state, outputIndex, item.CallID)
		if tool == nil {
			tool = ensurePendingToolBlock(state, outputIndex)
		}
		if item.CallID != "" {
			tool.CallID = item.CallID
		}
		if item.Name != "" {
			tool.Name = item.Name
		}
		raw := item.Arguments
		if raw == "" {
			raw = item.Input
		}
		if raw != "" {
			tool.FinalArgs = raw
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
	events = append(events, flushPendingToolBlocks(state, true)...)
	events = append(events, closeCurrentBlock(state)...)

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
				InputTokens:              state.InputTokens,
				OutputTokens:             state.OutputTokens,
				CacheReadInputTokens:     state.CacheReadInputTokens,
				CacheCreationInputTokens: state.CacheCreationInputTokens,
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
	return []AnthropicStreamEvent{{
		Type:  "content_block_stop",
		Index: &idx,
	}}
}
