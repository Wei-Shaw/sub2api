package apicompat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Non-streaming: AnthropicResponse → ResponsesResponse
// ---------------------------------------------------------------------------

// AnthropicToResponsesResponse converts an Anthropic Messages response into a
// Responses API response. This is the reverse of ResponsesToAnthropic and
// enables Anthropic upstream responses to be returned in OpenAI Responses format.
func AnthropicToResponsesResponse(resp *AnthropicResponse) *ResponsesResponse {
	id := resp.ID
	if id == "" {
		id = generateResponsesID()
	}

	out := &ResponsesResponse{
		ID:     id,
		Object: "response",
		Model:  resp.Model,
	}

	var outputs []ResponsesOutput
	var msgParts []ResponsesContentPart

	for _, block := range resp.Content {
		switch block.Type {
		case "thinking":
			if block.Thinking != "" {
				outputs = append(outputs, ResponsesOutput{
					Type: "reasoning",
					ID:   generateItemID(),
					Summary: []ResponsesSummary{{
						Type: "summary_text",
						Text: block.Thinking,
					}},
				})
			}
		case "text":
			if block.Text != "" {
				msgParts = append(msgParts, ResponsesContentPart{
					Type: "output_text",
					Text: block.Text,
				})
			}
		case "tool_use":
			args := "{}"
			if len(block.Input) > 0 {
				args = string(block.Input)
			}
			outputs = append(outputs, ResponsesOutput{
				Type:      "function_call",
				ID:        generateItemID(),
				CallID:    toResponsesCallID(block.ID),
				Name:      block.Name,
				Arguments: args,
				Status:    "completed",
			})
		}
	}

	// Assemble message output item from text parts
	if len(msgParts) > 0 {
		outputs = append(outputs, ResponsesOutput{
			Type:    "message",
			ID:      generateItemID(),
			Role:    "assistant",
			Content: msgParts,
			Status:  "completed",
		})
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

	// Map stop_reason → status
	out.Status = anthropicStopReasonToResponsesStatus(AnthropicStopReasonString(resp.StopReason), resp.Content)
	if out.Status == "incomplete" {
		out.IncompleteDetails = &ResponsesIncompleteDetails{Reason: "max_output_tokens"}
	}

	// Usage
	// Anthropic's input_tokens excludes cache_read/cache_creation, while OpenAI
	// Responses' input_tokens is the total including cached tokens. Add them back
	// when converting so downstream consumers see OpenAI semantics.
	totalInputTokens := resp.Usage.InputTokens +
		resp.Usage.CacheReadInputTokens +
		resp.Usage.CacheCreationInputTokens
	out.Usage = &ResponsesUsage{
		InputTokens:              totalInputTokens,
		OutputTokens:             resp.Usage.OutputTokens,
		TotalTokens:              totalInputTokens + resp.Usage.OutputTokens,
		CacheCreationInputTokens: resp.Usage.CacheCreationInputTokens,
	}
	if resp.Usage.CacheReadInputTokens > 0 {
		out.Usage.InputTokensDetails = &ResponsesInputTokensDetails{
			CachedTokens: resp.Usage.CacheReadInputTokens,
		}
	}

	return out
}

// anthropicStopReasonToResponsesStatus maps Anthropic stop_reason to Responses status.
func anthropicStopReasonToResponsesStatus(stopReason string, blocks []AnthropicContentBlock) string {
	switch stopReason {
	case "max_tokens":
		return "incomplete"
	case "end_turn", "tool_use", "stop_sequence":
		return "completed"
	default:
		return "completed"
	}
}

// ---------------------------------------------------------------------------
// Streaming: AnthropicStreamEvent → []ResponsesStreamEvent (stateful converter)
// ---------------------------------------------------------------------------

// AnthropicEventToResponsesState tracks state for converting a sequence of
// Anthropic SSE events into Responses SSE events.
type AnthropicEventToResponsesState struct {
	ResponseID     string
	Model          string
	Created        int64
	SequenceNumber int

	// CreatedSent tracks whether response.created has been emitted.
	CreatedSent bool
	// CompletedSent tracks whether the terminal event has been emitted.
	CompletedSent bool

	// Current output tracking
	OutputIndex     int
	CurrentItemID   string
	CurrentItemType string // "message" | "function_call" | "reasoning"

	// For message output: accumulate text parts
	ContentIndex int
	// TextAccum accumulates the current text part so that output_text.done and
	// content_part.done can carry the full text (deltas carry increments only).
	TextAccum string

	// For function_call: track per-output info
	CurrentCallID string
	CurrentName   string

	// Content of the currently open item, folded into Outputs when it closes.
	CurrentContent []ResponsesContentPart // message
	CurrentArgs    string                 // function_call
	CurrentSummary string                 // reasoning

	// Outputs accumulates every closed output item so that response.completed
	// can carry the full output list. The OpenAI SDK's get_final_response()
	// parses the terminal event's response directly; without this, clients see
	// an empty output_text.
	Outputs []ResponsesOutput

	// Usage from message_start / message_delta. InputTokens here follows
	// Anthropic semantics (excludes cached tokens); they are added back when
	// emitting the OpenAI Responses usage.
	InputTokens              int
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int

	StopReason string
}

// NewAnthropicEventToResponsesState returns an initialised stream state.
func NewAnthropicEventToResponsesState() *AnthropicEventToResponsesState {
	return &AnthropicEventToResponsesState{
		Created: time.Now().Unix(),
	}
}

// AnthropicEventToResponsesEvents converts a single Anthropic SSE event into
// zero or more Responses SSE events, updating state as it goes.
func AnthropicEventToResponsesEvents(
	evt *AnthropicStreamEvent,
	state *AnthropicEventToResponsesState,
) []ResponsesStreamEvent {
	switch evt.Type {
	case "message_start":
		return anthToResHandleMessageStart(evt, state)
	case "content_block_start":
		return anthToResHandleContentBlockStart(evt, state)
	case "content_block_delta":
		return anthToResHandleContentBlockDelta(evt, state)
	case "content_block_stop":
		return anthToResHandleContentBlockStop(evt, state)
	case "message_delta":
		return anthToResHandleMessageDelta(evt, state)
	case "message_stop":
		return anthToResHandleMessageStop(state)
	default:
		return nil
	}
}

// FinalizeAnthropicResponsesStream emits synthetic termination events if the
// stream ended without a proper message_stop.
func FinalizeAnthropicResponsesStream(state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if !state.CreatedSent || state.CompletedSent {
		return nil
	}

	var events []ResponsesStreamEvent

	// Close any open item
	events = append(events, closeCurrentResponsesItem(state)...)

	status, incompleteDetails := anthropicResponsesStreamTerminalState(state.StopReason)
	events = append(events, makeResponsesCompletedEvent(state, status, incompleteDetails))
	state.CompletedSent = true
	return events
}

// ResponsesEventToSSE formats a ResponsesStreamEvent as an SSE data line.
func ResponsesEventToSSE(evt ResponsesStreamEvent) (string, error) {
	data, err := json.Marshal(evt)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", evt.Type, data), nil
}

// --- internal handlers ---

func anthToResHandleMessageStart(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if evt.Message != nil {
		state.ResponseID = evt.Message.ID
		if state.Model == "" {
			state.Model = evt.Message.Model
		}
		if evt.Message.Usage.InputTokens > 0 {
			state.InputTokens = evt.Message.Usage.InputTokens
		}
		if evt.Message.Usage.CacheReadInputTokens > 0 {
			state.CacheReadInputTokens = evt.Message.Usage.CacheReadInputTokens
		}
		if evt.Message.Usage.CacheCreationInputTokens > 0 {
			state.CacheCreationInputTokens = evt.Message.Usage.CacheCreationInputTokens
		}
	}

	if state.CreatedSent {
		return nil
	}
	state.CreatedSent = true

	// Emit response.created
	return []ResponsesStreamEvent{makeResponsesCreatedEvent(state)}
}

func anthToResHandleContentBlockStart(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if evt.ContentBlock == nil {
		return nil
	}

	var events []ResponsesStreamEvent

	switch evt.ContentBlock.Type {
	case "thinking":
		state.CurrentItemID = generateItemID()
		state.CurrentItemType = "reasoning"
		state.ContentIndex = 0

		events = append(events, makeResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
			OutputIndex: state.OutputIndex,
			Item: &ResponsesOutput{
				Type: "reasoning",
				ID:   state.CurrentItemID,
			},
		}))

	case "text":
		// If we don't have an open message item, open one
		if state.CurrentItemType != "message" {
			state.CurrentItemID = generateItemID()
			state.CurrentItemType = "message"
			state.ContentIndex = 0

			events = append(events, makeResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
				OutputIndex: state.OutputIndex,
				Item: &ResponsesOutput{
					Type:   "message",
					ID:     state.CurrentItemID,
					Role:   "assistant",
					Status: "in_progress",
				},
			}))
		}

		// response.content_part.added must precede the output_text.delta events
		// for that part. The message item is added with content: [], and the
		// OpenAI SDK's accumulating stream helper (client.responses.stream) only
		// appends a content part when it sees content_part.added. Without it the
		// following output_text.delta indexes output.content[content_index] and
		// raises IndexError. Raw event iteration
		// (responses.create(stream=True)) does not accumulate, which is why this
		// went unnoticed.
		events = append(events, makeResponsesEvent(state, "response.content_part.added", &ResponsesStreamEvent{
			OutputIndex:  state.OutputIndex,
			ContentIndex: state.ContentIndex,
			ItemID:       state.CurrentItemID,
			Part:         &ResponsesContentPart{Type: "output_text", Text: ""},
		}))
		state.TextAccum = ""

	case "tool_use":
		// Close previous item if any
		events = append(events, closeCurrentResponsesItem(state)...)

		state.CurrentItemID = generateItemID()
		state.CurrentItemType = "function_call"
		state.CurrentCallID = toResponsesCallID(evt.ContentBlock.ID)
		state.CurrentName = evt.ContentBlock.Name

		events = append(events, makeResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
			OutputIndex: state.OutputIndex,
			Item: &ResponsesOutput{
				Type:   "function_call",
				ID:     state.CurrentItemID,
				CallID: state.CurrentCallID,
				Name:   state.CurrentName,
				Status: "in_progress",
			},
		}))
	}

	return events
}

func anthToResHandleContentBlockDelta(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if evt.Delta == nil {
		return nil
	}

	switch evt.Delta.Type {
	case "text_delta":
		if evt.Delta.Text == "" {
			return nil
		}
		state.TextAccum += evt.Delta.Text
		return []ResponsesStreamEvent{makeResponsesEvent(state, "response.output_text.delta", &ResponsesStreamEvent{
			OutputIndex:  state.OutputIndex,
			ContentIndex: state.ContentIndex,
			Delta:        evt.Delta.Text,
			ItemID:       state.CurrentItemID,
		})}

	case "thinking_delta":
		if evt.Delta.Thinking == "" {
			return nil
		}
		state.CurrentSummary += evt.Delta.Thinking
		return []ResponsesStreamEvent{makeResponsesEvent(state, "response.reasoning_summary_text.delta", &ResponsesStreamEvent{
			OutputIndex:  state.OutputIndex,
			SummaryIndex: 0,
			Delta:        evt.Delta.Thinking,
			ItemID:       state.CurrentItemID,
		})}

	case "input_json_delta":
		if evt.Delta.PartialJSON == "" {
			return nil
		}
		state.CurrentArgs += evt.Delta.PartialJSON
		return []ResponsesStreamEvent{makeResponsesEvent(state, "response.function_call_arguments.delta", &ResponsesStreamEvent{
			OutputIndex: state.OutputIndex,
			Delta:       evt.Delta.PartialJSON,
			ItemID:      state.CurrentItemID,
			CallID:      state.CurrentCallID,
			Name:        state.CurrentName,
		})}

	case "signature_delta":
		// Anthropic signature deltas have no Responses equivalent; skip
		return nil
	}

	return nil
}

func anthToResHandleContentBlockStop(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	switch state.CurrentItemType {
	case "reasoning":
		// Emit reasoning summary done + output item done
		events := []ResponsesStreamEvent{
			makeResponsesEvent(state, "response.reasoning_summary_text.done", &ResponsesStreamEvent{
				OutputIndex:  state.OutputIndex,
				SummaryIndex: 0,
				ItemID:       state.CurrentItemID,
			}),
		}
		events = append(events, closeCurrentResponsesItem(state)...)
		return events

	case "function_call":
		// Emit function_call_arguments.done + output item done
		events := []ResponsesStreamEvent{
			makeResponsesEvent(state, "response.function_call_arguments.done", &ResponsesStreamEvent{
				OutputIndex: state.OutputIndex,
				ItemID:      state.CurrentItemID,
				CallID:      state.CurrentCallID,
				Name:        state.CurrentName,
			}),
		}
		events = append(events, closeCurrentResponsesItem(state)...)
		return events

	case "message":
		// Text block is done: emit output_text.done then content_part.done (the
		// order OpenAI uses), both carrying the part's full text. The message
		// item itself stays open since more blocks may follow.
		text := state.TextAccum
		state.TextAccum = ""
		state.CurrentContent = append(state.CurrentContent, ResponsesContentPart{Type: "output_text", Text: text})
		return []ResponsesStreamEvent{
			makeResponsesEvent(state, "response.output_text.done", &ResponsesStreamEvent{
				OutputIndex:  state.OutputIndex,
				ContentIndex: state.ContentIndex,
				ItemID:       state.CurrentItemID,
				Text:         text,
			}),
			makeResponsesEvent(state, "response.content_part.done", &ResponsesStreamEvent{
				OutputIndex:  state.OutputIndex,
				ContentIndex: state.ContentIndex,
				ItemID:       state.CurrentItemID,
				Part:         &ResponsesContentPart{Type: "output_text", Text: text},
			}),
		}
	}

	return nil
}

func anthToResHandleMessageDelta(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if evt.Usage != nil {
		state.OutputTokens = evt.Usage.OutputTokens
		if evt.Usage.InputTokens > 0 {
			state.InputTokens = evt.Usage.InputTokens
		}
		if evt.Usage.CacheReadInputTokens > 0 {
			state.CacheReadInputTokens = evt.Usage.CacheReadInputTokens
		}
		if evt.Usage.CacheCreationInputTokens > 0 {
			state.CacheCreationInputTokens = evt.Usage.CacheCreationInputTokens
		}
	}
	if evt.Delta != nil && evt.Delta.StopReason != "" {
		state.StopReason = evt.Delta.StopReason
	}

	return nil
}

func anthToResHandleMessageStop(state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if state.CompletedSent {
		return nil
	}

	var events []ResponsesStreamEvent
	events = append(events, closeCurrentResponsesItem(state)...)

	status, incompleteDetails := anthropicResponsesStreamTerminalState(state.StopReason)
	events = append(events, makeResponsesCompletedEvent(state, status, incompleteDetails))
	state.CompletedSent = true
	return events
}

// --- helper functions ---

func anthropicResponsesStreamTerminalState(stopReason string) (string, *ResponsesIncompleteDetails) {
	if stopReason == "max_tokens" {
		return "incomplete", &ResponsesIncompleteDetails{Reason: "max_output_tokens"}
	}
	return "completed", nil
}

func closeCurrentResponsesItem(state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if state.CurrentItemType == "" {
		return nil
	}

	// Assemble the full item: both output_item.done and response.completed must
	// carry its content. Emitting only {type,id,status} makes SDK-side
	// accumulation produce an empty output.
	item := ResponsesOutput{
		Type:   state.CurrentItemType,
		ID:     state.CurrentItemID,
		Status: "completed",
	}
	switch state.CurrentItemType {
	case "message":
		item.Role = "assistant"
		item.Content = state.CurrentContent
	case "function_call":
		item.CallID = state.CurrentCallID
		item.Name = state.CurrentName
		args := state.CurrentArgs
		if args == "" {
			args = "{}"
		}
		item.Arguments = args
	case "reasoning":
		if state.CurrentSummary != "" {
			item.Summary = []ResponsesSummary{{Type: "summary_text", Text: state.CurrentSummary}}
		}
	}
	state.Outputs = append(state.Outputs, item)

	// Reset
	state.CurrentItemType = ""
	state.CurrentItemID = ""
	state.CurrentCallID = ""
	state.CurrentName = ""
	state.CurrentContent = nil
	state.CurrentArgs = ""
	state.CurrentSummary = ""
	state.TextAccum = ""
	state.OutputIndex++
	state.ContentIndex = 0

	return []ResponsesStreamEvent{makeResponsesEvent(state, "response.output_item.done", &ResponsesStreamEvent{
		OutputIndex: state.OutputIndex - 1, // Use the index before increment
		Item:        &item,
	})}
}

func makeResponsesCreatedEvent(state *AnthropicEventToResponsesState) ResponsesStreamEvent {
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

func makeResponsesCompletedEvent(
	state *AnthropicEventToResponsesState,
	status string,
	incompleteDetails *ResponsesIncompleteDetails,
) ResponsesStreamEvent {
	seq := state.SequenceNumber
	state.SequenceNumber++

	// Anthropic's input_tokens excludes cache_read/cache_creation; add them
	// back to match OpenAI Responses semantics where input_tokens is the total.
	totalInputTokens := state.InputTokens + state.CacheReadInputTokens + state.CacheCreationInputTokens
	usage := &ResponsesUsage{
		InputTokens:              totalInputTokens,
		OutputTokens:             state.OutputTokens,
		TotalTokens:              totalInputTokens + state.OutputTokens,
		CacheCreationInputTokens: state.CacheCreationInputTokens,
	}
	if state.CacheReadInputTokens > 0 {
		usage.InputTokensDetails = &ResponsesInputTokensDetails{
			CachedTokens: state.CacheReadInputTokens,
		}
	}

	eventType := "response.completed"
	if status == "incomplete" {
		eventType = "response.incomplete"
	}

	// Carry the output items accumulated over the stream. The SDK's
	// get_final_response() reads them straight from the terminal event, so an
	// empty list leaves clients with an empty result.
	outputs := state.Outputs
	if outputs == nil {
		outputs = []ResponsesOutput{}
	}

	return ResponsesStreamEvent{
		Type:           eventType,
		SequenceNumber: seq,
		Response: &ResponsesResponse{
			ID:                state.ResponseID,
			Object:            "response",
			Model:             state.Model,
			Status:            status,
			Output:            outputs,
			Usage:             usage,
			IncompleteDetails: incompleteDetails,
		},
	}
}

func makeResponsesEvent(state *AnthropicEventToResponsesState, eventType string, template *ResponsesStreamEvent) ResponsesStreamEvent {
	seq := state.SequenceNumber
	state.SequenceNumber++

	evt := *template
	evt.Type = eventType
	evt.SequenceNumber = seq
	return evt
}

func generateResponsesID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "resp_" + hex.EncodeToString(b)
}

func generateItemID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "item_" + hex.EncodeToString(b)
}
