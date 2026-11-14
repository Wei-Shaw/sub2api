package service

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
)

// openAICompatOutputSlot is everything the upstream stream said about one output
// item: the item of its output_item.added and output_item.done events and the
// text, refusal and argument deltas that arrived for it.
type openAICompatOutputSlot struct {
	index     int64
	indexed   bool
	itemType  string
	added     *apicompat.ResponsesOutput
	done      *apicompat.ResponsesOutput
	reasoning strings.Builder
	text      strings.Builder
	refusal   strings.Builder
	args      strings.Builder
	argsDone  string
}

// openAICompatOutputSlots records the output items of a buffered upstream stream
// so that an empty terminal output can be rebuilt from them.
type openAICompatOutputSlots struct {
	slots []*openAICompatOutputSlot
}

func openAICompatIsToolCallType(itemType string) bool {
	return itemType == "function_call" || itemType == "custom_tool_call"
}

func (o *openAICompatOutputSlots) Observe(payload []byte) {
	if o == nil {
		return
	}
	switch strings.TrimSpace(gjson.GetBytes(payload, "type").String()) {
	case "response.output_item.added":
		if item, ok := openAICompatEventItem(payload); ok {
			slot := o.slot(payload, item.Type, item.ID, false, true)
			slot.added = &item
		}
	case "response.output_item.done":
		if item, ok := openAICompatEventItem(payload); ok {
			slot := o.slot(payload, item.Type, item.ID, true, true)
			slot.done = &item
		}
	case "response.output_text.delta":
		if delta := gjson.GetBytes(payload, "delta").String(); delta != "" {
			_, _ = o.slot(payload, "message", "", false, true).text.WriteString(delta)
		}
	case "response.refusal.delta":
		if delta := gjson.GetBytes(payload, "delta").String(); delta != "" {
			_, _ = o.slot(payload, "message", "", false, true).refusal.WriteString(delta)
		}
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		if delta := gjson.GetBytes(payload, "delta").String(); delta != "" {
			_, _ = o.slot(payload, "reasoning", "", false, true).reasoning.WriteString(delta)
		}
	case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
		if delta := gjson.GetBytes(payload, "delta").String(); delta != "" {
			if slot := o.slot(payload, "function_call", "", false, false); slot != nil {
				_, _ = slot.args.WriteString(delta)
			}
		}
	case "response.function_call_arguments.done":
		if slot := o.slot(payload, "function_call", "", false, false); slot != nil {
			if completed := gjson.GetBytes(payload, "arguments").String(); completed != "" {
				slot.argsDone = completed
			}
		}
	case "response.custom_tool_call_input.done":
		if slot := o.slot(payload, "function_call", "", false, false); slot != nil {
			if completed := gjson.GetBytes(payload, "input").String(); completed != "" {
				slot.argsDone = completed
			}
		}
	}
}

func openAICompatEventItem(payload []byte) (apicompat.ResponsesOutput, bool) {
	var item apicompat.ResponsesOutput
	raw := gjson.GetBytes(payload, "item")
	if !raw.IsObject() || !gjson.ValidBytes(payload) || json.Unmarshal([]byte(raw.Raw), &item) != nil {
		return item, false
	}
	item.Type = strings.TrimSpace(item.Type)
	return item, item.Type != ""
}

// slot finds the slot an event belongs to. A done event whose item has an id
// belongs to the slot that already holds a done item with that id, so a repeated
// item replaces its earlier copy and items that merely share an output_index stay
// apart. Otherwise an event with an output_index belongs to the newest slot of
// that index (an added event: the newest one that has none yet; a done event
// without an id replaces the done item of that slot). An event without an
// output_index belongs to the newest unindexed slot of its kind (a done event:
// the oldest one still without a done item), and an added event always starts a
// new slot, so unindexed items keep arrival order. With create unset, a missing
// slot returns nil.
func (o *openAICompatOutputSlots) slot(payload []byte, kind, id string, done, create bool) *openAICompatOutputSlot {
	index := gjson.GetBytes(payload, "output_index")
	added := strings.TrimSpace(gjson.GetBytes(payload, "type").String()) == "response.output_item.added"
	matches := func(slot *openAICompatOutputSlot) bool {
		if slot.itemType == "" || slot.itemType == kind {
			return true
		}
		return openAICompatIsToolCallType(kind) && openAICompatIsToolCallType(slot.itemType)
	}
	newest := func(accept func(*openAICompatOutputSlot) bool) *openAICompatOutputSlot {
		for i := len(o.slots) - 1; i >= 0; i-- {
			if accept(o.slots[i]) {
				return o.slots[i]
			}
		}
		return nil
	}
	var found *openAICompatOutputSlot
	if done && id != "" {
		found = newest(func(slot *openAICompatOutputSlot) bool { return slot.done != nil && slot.done.ID == id })
	}
	if found == nil {
		switch {
		case index.Exists():
			atIndex := func(slot *openAICompatOutputSlot) bool { return slot.indexed && slot.index == index.Int() }
			switch {
			case done && id != "":
				found = newest(func(slot *openAICompatOutputSlot) bool { return atIndex(slot) && slot.done == nil })
			case added:
				found = newest(func(slot *openAICompatOutputSlot) bool { return atIndex(slot) && slot.added == nil })
			default:
				found = newest(atIndex)
			}
		case added:
		case done:
			for _, slot := range o.slots {
				if !slot.indexed && slot.done == nil && slot.itemType == kind {
					found = slot
					break
				}
			}
		default:
			found = newest(func(slot *openAICompatOutputSlot) bool { return !slot.indexed && matches(slot) })
		}
	}
	if found == nil {
		if !create {
			return nil
		}
		found = &openAICompatOutputSlot{}
		if index.Exists() {
			found.index, found.indexed = index.Int(), true
		}
		o.slots = append(o.slots, found)
	}
	if found.itemType == "" || done || added {
		found.itemType = kind
	}
	return found
}

// fillTerminalOutput rebuilds an empty terminal output from the stream's output
// items, in output_index order, items without an output_index after the indexed
// ones in arrival order. Items with the same output_index keep arrival order. An item that received output_item.done is taken from
// that event, completed with the deltas where the event left a text, refusal or
// the arguments empty; an item that never received it is rebuilt from its
// deltas. Only terminals that carry a result are filled; the output of
// response.failed and error terminals never reaches the client and is left as
// the upstream sent it.
func (o *openAICompatOutputSlots) fillTerminalOutput(eventType string, response *apicompat.ResponsesResponse) {
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done", "response.incomplete", "response.cancelled", "response.canceled":
	default:
		return
	}
	if o == nil || response == nil || len(response.Output) > 0 {
		return
	}
	slots := append([]*openAICompatOutputSlot(nil), o.slots...)
	sort.SliceStable(slots, func(i, j int) bool {
		a, b := slots[i], slots[j]
		if a.indexed != b.indexed {
			return a.indexed
		}
		return a.indexed && a.index < b.index
	})
	for _, slot := range slots {
		if item, ok := slot.build(); ok {
			response.Output = append(response.Output, item)
		}
	}
}

func (s *openAICompatOutputSlot) build() (apicompat.ResponsesOutput, bool) {
	switch s.itemType {
	case "reasoning":
		return s.buildReasoning()
	case "message":
		return s.buildMessage()
	case "function_call", "custom_tool_call":
		return s.buildToolCall()
	}
	if s.done == nil {
		return apicompat.ResponsesOutput{}, false
	}
	return *s.done, true
}

func openAICompatReasoningItemText(item apicompat.ResponsesOutput) string {
	var text strings.Builder
	for _, part := range item.Summary {
		if part.Type == "summary_text" {
			_, _ = text.WriteString(part.Text)
		}
	}
	if text.Len() == 0 {
		for _, part := range item.Content {
			if part.Type == "reasoning_text" {
				_, _ = text.WriteString(part.Text)
			}
		}
	}
	return text.String()
}

func (s *openAICompatOutputSlot) buildReasoning() (apicompat.ResponsesOutput, bool) {
	item := apicompat.ResponsesOutput{Type: "reasoning"}
	if s.done != nil {
		item = *s.done
	}
	if s.reasoning.Len() > 0 && openAICompatReasoningItemText(item) == "" {
		item.Summary = []apicompat.ResponsesSummary{{Type: "summary_text", Text: s.reasoning.String()}}
		item.Content = nil
	}
	return item, s.done != nil || s.reasoning.Len() > 0
}

func (s *openAICompatOutputSlot) buildMessage() (apicompat.ResponsesOutput, bool) {
	item := apicompat.ResponsesOutput{Type: "message", Role: "assistant"}
	if s.done != nil {
		item = *s.done
		item.Content = append([]apicompat.ResponsesContentPart(nil), item.Content...)
	}
	if s.text.Len() > 0 {
		filled := false
		hasText := false
		for i := range item.Content {
			part := &item.Content[i]
			if part.Type != "output_text" {
				continue
			}
			if strings.TrimSpace(part.Text) != "" {
				hasText = true
				break
			}
			if !filled {
				part.Text, filled = s.text.String(), true
			}
		}
		if !hasText && !filled {
			item.Content = append(item.Content, apicompat.ResponsesContentPart{Type: "output_text", Text: s.text.String()})
		}
	}
	if s.refusal.Len() > 0 {
		hasRefusal := false
		for _, part := range item.Content {
			if part.Type == "refusal" && part.Refusal != "" {
				hasRefusal = true
			}
		}
		if !hasRefusal {
			item.Content = append(item.Content, apicompat.ResponsesContentPart{Type: "refusal", Refusal: s.refusal.String()})
		}
	}
	return item, s.done != nil || s.text.Len() > 0 || s.refusal.Len() > 0
}

// buildToolCall returns a function_call item. A custom_tool_call is returned in
// that form as well, with its input as the arguments, because that is the shape
// the Chat Completions and Messages converters render as a tool call.
func (s *openAICompatOutputSlot) buildToolCall() (apicompat.ResponsesOutput, bool) {
	if s.done == nil && s.added == nil {
		return apicompat.ResponsesOutput{}, false
	}
	var item apicompat.ResponsesOutput
	if s.done != nil {
		item = *s.done
	} else {
		item = *s.added
	}
	if s.added != nil {
		if item.CallID == "" {
			item.CallID = s.added.CallID
		}
		if item.Name == "" {
			item.Name = s.added.Name
		}
		if item.Namespace == "" {
			item.Namespace = s.added.Namespace
		}
		if item.EncryptedFunctionArgs == nil {
			item.EncryptedFunctionArgs = s.added.EncryptedFunctionArgs
		}
	}
	arguments := item.Arguments
	if item.Type == "custom_tool_call" {
		arguments = item.Input
	}
	if arguments == "" {
		arguments = s.argsDone
	}
	if arguments == "" {
		arguments = s.args.String()
	}
	item.Type, item.Input, item.Arguments = "function_call", "", arguments
	return item, true
}
