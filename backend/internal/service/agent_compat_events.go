package service

import (
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

type agentResponsesEmitter struct {
	id        string
	model     string
	created   int64
	seq       int
	textOpen  bool
	thinkOpen bool
	textID    string
	thinkID   string
	toolID    string
	toolName  string
	toolArgs  string
	outIndex  int
	usage     apicompat.ResponsesUsage
	createdOK bool
	finished  bool
}

func newAgentResponsesEmitter(model string) *agentResponsesEmitter {
	return &agentResponsesEmitter{
		id:      "resp_" + fmt.Sprintf("%d", time.Now().UnixNano()),
		model:   model,
		created: time.Now().Unix(),
	}
}

func (e *agentResponsesEmitter) nextSeq() int {
	e.seq++
	return e.seq
}

func (e *agentResponsesEmitter) createdEvent() apicompat.ResponsesStreamEvent {
	e.createdOK = true
	return apicompat.ResponsesStreamEvent{
		Type:           "response.created",
		SequenceNumber: e.nextSeq(),
		Response: &apicompat.ResponsesResponse{
			ID:        e.id,
			Object:    "response",
			CreatedAt: e.created,
			Model:     e.model,
			Status:    "in_progress",
		},
	}
}

func (e *agentResponsesEmitter) apply(ev agentEvent) []apicompat.ResponsesStreamEvent {
	if e.finished {
		return nil
	}
	var events []apicompat.ResponsesStreamEvent
	if !e.createdOK {
		events = append(events, e.createdEvent())
	}
	switch ev.Kind {
	case "thinking":
		if ev.Text == "" {
			return events
		}
		events = append(events, e.closeText()...)
		if !e.thinkOpen {
			e.thinkID = "rs_" + fmt.Sprintf("%d", e.outIndex)
			e.thinkOpen = true
			events = append(events, apicompat.ResponsesStreamEvent{
				Type:           "response.output_item.added",
				SequenceNumber: e.nextSeq(),
				OutputIndex:    e.outIndex,
				Item:           &apicompat.ResponsesOutput{Type: "reasoning", ID: e.thinkID},
			})
		}
		events = append(events, apicompat.ResponsesStreamEvent{
			Type:           "response.reasoning_summary_text.delta",
			SequenceNumber: e.nextSeq(),
			OutputIndex:    e.outIndex,
			ItemID:         e.thinkID,
			Delta:          ev.Text,
		})
	case "text":
		if ev.Text == "" {
			return events
		}
		events = append(events, e.closeThink()...)
		if !e.textOpen {
			e.textID = "msg_" + fmt.Sprintf("%d", e.outIndex)
			e.textOpen = true
			events = append(events, apicompat.ResponsesStreamEvent{
				Type:           "response.output_item.added",
				SequenceNumber: e.nextSeq(),
				OutputIndex:    e.outIndex,
				Item: &apicompat.ResponsesOutput{
					Type:    "message",
					ID:      e.textID,
					Role:    "assistant",
					Status:  "in_progress",
					Content: []apicompat.ResponsesContentPart{{Type: "output_text"}},
				},
			})
		}
		events = append(events, apicompat.ResponsesStreamEvent{
			Type:           "response.output_text.delta",
			SequenceNumber: e.nextSeq(),
			OutputIndex:    e.outIndex,
			ItemID:         e.textID,
			Delta:          ev.Text,
		})
	case "tool_start":
		events = append(events, e.closeThink()...)
		events = append(events, e.closeText()...)
		e.toolID = firstNonEmpty(ev.CallID, "call_"+fmt.Sprintf("%d", e.outIndex))
		e.toolName = ev.ToolName
		e.toolArgs = ""
		events = append(events, apicompat.ResponsesStreamEvent{
			Type:           "response.output_item.added",
			SequenceNumber: e.nextSeq(),
			OutputIndex:    e.outIndex,
			Item: &apicompat.ResponsesOutput{
				Type:   "function_call",
				ID:     e.toolID,
				CallID: e.toolID,
				Name:   e.toolName,
			},
		})
	case "tool_delta":
		e.toolArgs += ev.Arguments
		events = append(events, apicompat.ResponsesStreamEvent{
			Type:           "response.function_call_arguments.delta",
			SequenceNumber: e.nextSeq(),
			OutputIndex:    e.outIndex,
			ItemID:         firstNonEmpty(ev.CallID, e.toolID),
			Delta:          ev.Arguments,
		})
	case "tool_end":
		callID := firstNonEmpty(ev.CallID, e.toolID)
		events = append(events, apicompat.ResponsesStreamEvent{
			Type:           "response.function_call_arguments.done",
			SequenceNumber: e.nextSeq(),
			OutputIndex:    e.outIndex,
			ItemID:         callID,
			Arguments:      e.toolArgs,
		})
		events = append(events, apicompat.ResponsesStreamEvent{
			Type:           "response.output_item.done",
			SequenceNumber: e.nextSeq(),
			OutputIndex:    e.outIndex,
			Item: &apicompat.ResponsesOutput{
				Type:      "function_call",
				ID:        callID,
				CallID:    callID,
				Name:      firstNonEmpty(ev.ToolName, e.toolName),
				Arguments: e.toolArgs,
				Status:    "completed",
			},
		})
		e.outIndex++
		e.toolID = ""
	case "usage":
		if ev.Input > 0 {
			e.usage.InputTokens = ev.Input
		}
		if ev.Output > 0 {
			e.usage.OutputTokens += ev.Output
		}
		if ev.CacheR > 0 {
			if e.usage.InputTokensDetails == nil {
				e.usage.InputTokensDetails = &apicompat.ResponsesInputTokensDetails{}
			}
			e.usage.InputTokensDetails.CachedTokens = ev.CacheR
		}
		if ev.CacheW > 0 {
			e.usage.CacheCreationInputTokens = ev.CacheW
		}
	case "done":
		events = append(events, e.finish()...)
	}
	return events
}

func (e *agentResponsesEmitter) closeThink() []apicompat.ResponsesStreamEvent {
	if !e.thinkOpen {
		return nil
	}
	events := []apicompat.ResponsesStreamEvent{{
		Type:           "response.output_item.done",
		SequenceNumber: e.nextSeq(),
		OutputIndex:    e.outIndex,
		Item:           &apicompat.ResponsesOutput{Type: "reasoning", ID: e.thinkID, Status: "completed"},
	}}
	e.thinkOpen = false
	e.outIndex++
	return events
}

func (e *agentResponsesEmitter) closeText() []apicompat.ResponsesStreamEvent {
	if !e.textOpen {
		return nil
	}
	events := []apicompat.ResponsesStreamEvent{
		{
			Type:           "response.output_text.done",
			SequenceNumber: e.nextSeq(),
			OutputIndex:    e.outIndex,
			ItemID:         e.textID,
		},
		{
			Type:           "response.output_item.done",
			SequenceNumber: e.nextSeq(),
			OutputIndex:    e.outIndex,
			Item: &apicompat.ResponsesOutput{
				Type:    "message",
				ID:      e.textID,
				Role:    "assistant",
				Status:  "completed",
				Content: []apicompat.ResponsesContentPart{{Type: "output_text"}},
			},
		},
	}
	e.textOpen = false
	e.outIndex++
	return events
}

func (e *agentResponsesEmitter) finish() []apicompat.ResponsesStreamEvent {
	if e.finished {
		return nil
	}
	e.finished = true
	var events []apicompat.ResponsesStreamEvent
	if !e.createdOK {
		events = append(events, e.createdEvent())
	}
	events = append(events, e.closeThink()...)
	events = append(events, e.closeText()...)
	e.usage.TotalTokens = e.usage.InputTokens + e.usage.OutputTokens
	events = append(events, apicompat.ResponsesStreamEvent{
		Type:           "response.completed",
		SequenceNumber: e.nextSeq(),
		Usage:          &e.usage,
		Response: &apicompat.ResponsesResponse{
			ID:        e.id,
			Object:    "response",
			CreatedAt: e.created,
			Model:     e.model,
			Status:    "completed",
			Usage:     &e.usage,
		},
	})
	return events
}

type agentResponsesAccumulator struct {
	model    string
	id       string
	created  int64
	text     string
	thinking string
	tools    []apicompat.ResponsesOutput
	usage    *apicompat.ResponsesUsage
}

func agentCacheReadTokens(usage apicompat.ResponsesUsage) int {
	if usage.InputTokensDetails == nil {
		return 0
	}
	return usage.InputTokensDetails.CachedTokens
}

func newAgentResponsesAccumulator(model string) *agentResponsesAccumulator {
	return &agentResponsesAccumulator{model: model, created: time.Now().Unix()}
}

func (a *agentResponsesAccumulator) apply(evt *apicompat.ResponsesStreamEvent) {
	if evt == nil {
		return
	}
	if evt.Response != nil && evt.Response.ID != "" {
		a.id = evt.Response.ID
		if evt.Response.CreatedAt > 0 {
			a.created = evt.Response.CreatedAt
		}
	}
	switch evt.Type {
	case "response.output_text.delta":
		a.text += evt.Delta
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		a.thinking += evt.Delta
	case "response.output_item.done":
		if evt.Item != nil && evt.Item.Type == "function_call" {
			a.tools = append(a.tools, *evt.Item)
		}
	case "response.completed":
		if evt.Usage != nil {
			a.usage = evt.Usage
		} else if evt.Response != nil {
			a.usage = evt.Response.Usage
		}
	}
}

func (a *agentResponsesAccumulator) response() *apicompat.ResponsesResponse {
	id := a.id
	if id == "" {
		id = "resp_" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	var output []apicompat.ResponsesOutput
	if a.thinking != "" {
		output = append(output, apicompat.ResponsesOutput{
			Type:    "reasoning",
			ID:      "rs_0",
			Summary: []apicompat.ResponsesSummary{{Type: "summary_text", Text: a.thinking}},
		})
	}
	if a.text != "" {
		output = append(output, apicompat.ResponsesOutput{
			Type:    "message",
			ID:      "msg_0",
			Role:    "assistant",
			Status:  "completed",
			Content: []apicompat.ResponsesContentPart{{Type: "output_text", Text: a.text}},
		})
	}
	output = append(output, a.tools...)
	return &apicompat.ResponsesResponse{
		ID:        id,
		Object:    "response",
		CreatedAt: a.created,
		Model:     a.model,
		Status:    "completed",
		Output:    output,
		Usage:     a.usage,
	}
}
