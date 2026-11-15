package service

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
)

// openAICompatPartText keeps the delta text of one kind per part index, the
// content_index or summary_index of its events, so that a part of an item is
// completed with the text of that part only.
type openAICompatPartText struct {
	parts map[int64]*strings.Builder
}

func (p *openAICompatPartText) write(index int64, delta string) {
	if p.parts == nil {
		p.parts = map[int64]*strings.Builder{}
	}
	if p.parts[index] == nil {
		p.parts[index] = &strings.Builder{}
	}
	_, _ = p.parts[index].WriteString(delta)
}

func (p *openAICompatPartText) any() bool {
	return len(p.parts) > 0
}

func (p *openAICompatPartText) at(index int64) string {
	if builder := p.parts[index]; builder != nil {
		return builder.String()
	}
	return ""
}

// unclaimed joins, in part order, the text of the parts from index from on that
// are not in claimed.
func (p *openAICompatPartText) unclaimed(claimed map[int64]bool, from int64) string {
	indexes := make([]int64, 0, len(p.parts))
	for index := range p.parts {
		if index >= from && !claimed[index] {
			indexes = append(indexes, index)
		}
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i] < indexes[j] })
	var text strings.Builder
	for _, index := range indexes {
		_, _ = text.WriteString(p.parts[index].String())
	}
	return text.String()
}

// openAICompatOutputSlot is everything the upstream stream said about one output
// item: the item of its output_item.added and output_item.done events and the
// text, refusal and argument deltas that arrived for it. Text, refusal and the
// two kinds of reasoning text are kept per part, and the reasoning text also in
// arrival order across its kinds.
type openAICompatOutputSlot struct {
	index         int64
	indexed       bool
	itemType      string
	id            string
	added         *apicompat.ResponsesOutput
	done          *apicompat.ResponsesOutput
	reasoning     strings.Builder
	summaryText   openAICompatPartText
	reasoningText openAICompatPartText
	text          openAICompatPartText
	refusal       openAICompatPartText
	args          strings.Builder
	argsDone      string
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
			slot := o.slot(payload, item.Type, item.ID, item.CallID, false, true)
			slot.added = &item
		}
	case "response.output_item.done":
		if item, ok := openAICompatEventItem(payload); ok {
			slot := o.slot(payload, item.Type, item.ID, item.CallID, true, true)
			slot.done = &item
		}
	case "response.output_text.delta":
		if delta := gjson.GetBytes(payload, "delta").String(); delta != "" {
			o.slot(payload, "message", gjson.GetBytes(payload, "item_id").String(), "", false, true).text.write(gjson.GetBytes(payload, "content_index").Int(), delta)
		}
	case "response.refusal.delta":
		if delta := gjson.GetBytes(payload, "delta").String(); delta != "" {
			o.slot(payload, "message", gjson.GetBytes(payload, "item_id").String(), "", false, true).refusal.write(gjson.GetBytes(payload, "content_index").Int(), delta)
		}
	case "response.reasoning_summary_text.delta":
		if delta := gjson.GetBytes(payload, "delta").String(); delta != "" {
			slot := o.slot(payload, "reasoning", gjson.GetBytes(payload, "item_id").String(), "", false, true)
			_, _ = slot.reasoning.WriteString(delta)
			slot.summaryText.write(gjson.GetBytes(payload, "summary_index").Int(), delta)
		}
	case "response.reasoning_text.delta":
		if delta := gjson.GetBytes(payload, "delta").String(); delta != "" {
			slot := o.slot(payload, "reasoning", gjson.GetBytes(payload, "item_id").String(), "", false, true)
			_, _ = slot.reasoning.WriteString(delta)
			slot.reasoningText.write(gjson.GetBytes(payload, "content_index").Int(), delta)
		}
	case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
		if delta := gjson.GetBytes(payload, "delta").String(); delta != "" {
			if slot := o.slot(payload, "function_call", gjson.GetBytes(payload, "item_id").String(), "", false, false); slot != nil {
				_, _ = slot.args.WriteString(delta)
			}
		}
	case "response.function_call_arguments.done":
		if slot := o.slot(payload, "function_call", gjson.GetBytes(payload, "item_id").String(), "", false, false); slot != nil {
			if completed := gjson.GetBytes(payload, "arguments").String(); completed != "" {
				slot.argsDone = completed
			}
		}
	case "response.custom_tool_call_input.done":
		if slot := o.slot(payload, "function_call", gjson.GetBytes(payload, "item_id").String(), "", false, false); slot != nil {
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

// slot finds the slot an event belongs to. id is the id of the item the event
// is about (the item of an added or done event, the item_id of a delta) and
// callID the call_id of a tool call item. A done event whose item has an id
// belongs to the slot that already holds a done item with that id, so a repeated
// item replaces its earlier copy. Otherwise an event whose id a slot already
// has, because the deltas or the added event of that item opened it, belongs to
// that slot, also when the event has no output_index. Otherwise an event with an
// output_index belongs to the newest slot of that index that can hold its kind
// of item and has no other id (an added event: the newest one without an added or
// done item; a done event with an id: the newest one without a done item; a done
// event without an id replaces the done item of that slot), so items that merely
// share an output_index stay apart. An event without an output_index belongs to
// the newest slot of its kind that is not done, else to the newest unindexed
// one; a done event with a call_id belongs to the slot of that call, else to the
// oldest unindexed slot of its kind without a done item, and an added event
// always starts a new slot, so unindexed items keep arrival order. With create
// unset, a missing slot returns nil.
func (o *openAICompatOutputSlots) slot(payload []byte, kind, id, callID string, done, create bool) *openAICompatOutputSlot {
	index := gjson.GetBytes(payload, "output_index")
	added := strings.TrimSpace(gjson.GetBytes(payload, "type").String()) == "response.output_item.added"
	matches := func(slot *openAICompatOutputSlot) bool {
		if slot.itemType == "" || slot.itemType == kind {
			return true
		}
		return openAICompatIsToolCallType(kind) && openAICompatIsToolCallType(slot.itemType)
	}
	fits := func(slot *openAICompatOutputSlot) bool {
		return matches(slot) && (slot.id == "" || id == "" || slot.id == id)
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
	if found == nil && id != "" && (done || added || !index.Exists()) {
		found = newest(func(slot *openAICompatOutputSlot) bool {
			return slot.id == id && matches(slot) && slot.done == nil && (!added || slot.added == nil)
		})
	}
	if found == nil {
		switch {
		case index.Exists():
			atIndex := func(slot *openAICompatOutputSlot) bool {
				return slot.indexed && slot.index == index.Int() && matches(slot)
			}
			switch {
			case done && id != "":
				found = newest(func(slot *openAICompatOutputSlot) bool { return atIndex(slot) && slot.done == nil })
			case added:
				found = newest(func(slot *openAICompatOutputSlot) bool { return atIndex(slot) && slot.added == nil && slot.done == nil })
			default:
				found = newest(atIndex)
			}
		case added:
		case done:
			if callID != "" {
				found = newest(func(slot *openAICompatOutputSlot) bool {
					return slot.done == nil && slot.added != nil && slot.added.CallID == callID && matches(slot)
				})
			}
			for _, slot := range o.slots {
				if found == nil && !slot.indexed && slot.done == nil && slot.itemType == kind {
					found = slot
				}
			}
		default:
			found = newest(func(slot *openAICompatOutputSlot) bool { return slot.done == nil && fits(slot) })
			if found == nil {
				found = newest(func(slot *openAICompatOutputSlot) bool { return !slot.indexed && fits(slot) })
			}
		}
	}
	if found == nil && index.Exists() && (added || done) {
		// Deltas that came with no output_index and before their item was announced
		// belong to the newest such item of this kind.
		found = newest(func(slot *openAICompatOutputSlot) bool {
			return !slot.indexed && slot.added == nil && slot.done == nil && fits(slot)
		})
	}
	if found == nil {
		if !create {
			return nil
		}
		found = &openAICompatOutputSlot{}
		o.slots = append(o.slots, found)
	}
	if index.Exists() && !found.indexed {
		found.index, found.indexed = index.Int(), true
	}
	if found.id == "" {
		found.id = id
	}
	if found.itemType == "" || done || added {
		found.itemType = kind
	}
	return found
}

// fillTerminalOutput rebuilds an empty terminal output from the stream's output
// items, in output_index order, items without an output_index after the indexed
// ones in arrival order. Items with the same output_index keep arrival order.
// An item that received output_item.done is taken from that event, completed with the deltas where the event left a text, refusal or
// the arguments empty, a part with the deltas of its own content_index or
// summary_index; an item that never received it is rebuilt from its deltas. Only terminals that carry a result are filled; the output of
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
			// The streaming converters render a web search only once it is completed.
			if item.Type == "web_search_call" && item.Status != "completed" {
				continue
			}
			response.Output = append(response.Output, item)
			if item.Type == "custom_tool_call" {
				response.RenderCustomToolCalls = true
			}
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

func (s *openAICompatOutputSlot) buildReasoning() (apicompat.ResponsesOutput, bool) {
	item := apicompat.ResponsesOutput{Type: "reasoning"}
	if s.done != nil {
		item = *s.done
	}
	// The signature may be on the added event only.
	if s.added != nil && strings.TrimSpace(item.EncryptedContent) == "" {
		item.EncryptedContent = s.added.EncryptedContent
	}
	// The converters render the summary text and fall back to the reasoning_text
	// content only when the summary has none, so an item carrying both would lose
	// one. The deltas carried every text the client is shown, so they stand in
	// for a text field that is empty or that would be dropped.
	if s.reasoning.Len() > 0 {
		var summary, content bool
		for _, part := range item.Summary {
			summary = summary || (part.Type == "summary_text" && part.Text != "")
		}
		for _, part := range item.Content {
			content = content || (part.Type == "reasoning_text" && part.Text != "")
		}
		switch {
		case summary == content:
			item.Summary = []apicompat.ResponsesSummary{{Type: "summary_text", Text: s.reasoning.String()}}
			item.Content = nil
		case summary:
			// The item keeps its text; a summary part it left empty takes the
			// deltas of its own summary_index.
			item.Summary = append([]apicompat.ResponsesSummary(nil), item.Summary...)
			claimed := map[int64]bool{}
			for i := range item.Summary {
				if part := &item.Summary[i]; part.Type == "summary_text" {
					claimed[int64(i)] = true
					if part.Text == "" {
						part.Text = s.summaryText.at(int64(i))
					}
				}
			}
			if rest := s.summaryText.unclaimed(claimed, int64(len(item.Summary))); rest != "" {
				item.Summary = append(item.Summary, apicompat.ResponsesSummary{Type: "summary_text", Text: rest})
			}
		default:
			item.Content = append([]apicompat.ResponsesContentPart(nil), item.Content...)
			claimed := map[int64]bool{}
			for i := range item.Content {
				if part := &item.Content[i]; part.Type == "reasoning_text" {
					claimed[int64(i)] = true
					if part.Text == "" {
						part.Text = s.reasoningText.at(int64(i))
					}
				}
			}
			if rest := s.reasoningText.unclaimed(claimed, int64(len(item.Content))); rest != "" {
				item.Content = append(item.Content, apicompat.ResponsesContentPart{Type: "reasoning_text", Text: rest})
			}
		}
	}
	return item, s.done != nil || s.reasoning.Len() > 0 || strings.TrimSpace(item.EncryptedContent) != ""
}

func (s *openAICompatOutputSlot) buildMessage() (apicompat.ResponsesOutput, bool) {
	item := apicompat.ResponsesOutput{Type: "message", Role: "assistant"}
	if s.done != nil {
		item = *s.done
		item.Content = append([]apicompat.ResponsesContentPart(nil), item.Content...)
	}
	declared := int64(len(item.Content))
	if s.text.any() {
		blank := func(text string) bool { return strings.TrimSpace(text) == "" }
		if rest := completeContentParts(item.Content, &s.text, "output_text", declared, blank); rest != "" {
			item.Content = append(item.Content, apicompat.ResponsesContentPart{Type: "output_text", Text: rest})
		}
	}
	if s.refusal.any() {
		blank := func(text string) bool { return text == "" }
		if rest := completeContentParts(item.Content, &s.refusal, "refusal", declared, blank); rest != "" {
			item.Content = append(item.Content, apicompat.ResponsesContentPart{Type: "refusal", Refusal: rest})
		}
	}
	return item, s.done != nil || s.text.any() || s.refusal.any()
}

// completeContentParts gives each blank part of partType the deltas of its own
// content_index, the position of the part in the content, and leaves the parts
// that have text as they are. It returns the deltas of the indexes that have no
// part of that type: all of them when no part has text, else those beyond the
// last of the declared parts of the done item.
func completeContentParts(content []apicompat.ResponsesContentPart, deltas *openAICompatPartText, partType string, declared int64, blank func(string) bool) string {
	claimed := map[int64]bool{}
	present := false
	for i := range content {
		part := &content[i]
		if part.Type != partType {
			continue
		}
		field := &part.Text
		if partType == "refusal" {
			field = &part.Refusal
		}
		claimed[int64(i)] = true
		if blank(*field) {
			if delta := deltas.at(int64(i)); delta != "" {
				*field = delta
			}
		}
		present = present || !blank(*field)
	}
	from := declared
	if !present {
		from = math.MinInt64
	}
	return deltas.unclaimed(claimed, from)
}

// buildToolCall returns the function_call or custom_tool_call item of the slot,
// with the arguments or input a done item left empty taken from the deltas.
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
	if item.Type == "custom_tool_call" {
		item.Input = arguments
	} else {
		item.Arguments = arguments
	}
	return item, true
}
