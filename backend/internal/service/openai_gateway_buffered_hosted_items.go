package service

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
)

// openAICompatAccumulatedItemTypes are the output item types that
// apicompat.BufferedResponseAccumulator rebuilds from deltas. Every other type
// can only be recovered from its response.output_item.done event.
var openAICompatAccumulatedItemTypes = map[string]struct{}{
	"reasoning":        {},
	"message":          {},
	"function_call":    {},
	"custom_tool_call": {},
}

type openAICompatHostedItem struct {
	index   int64
	indexed bool
	raw     json.RawMessage
}

// openAICompatHostedItems collects the response.output_item.done items of types
// the delta accumulator does not produce, such as web_search_call and
// image_generation_call.
type openAICompatHostedItems struct {
	items []openAICompatHostedItem
}

func (h *openAICompatHostedItems) Observe(payload []byte) {
	if h == nil || strings.TrimSpace(gjson.GetBytes(payload, "type").String()) != "response.output_item.done" || !gjson.ValidBytes(payload) {
		return
	}
	item := gjson.GetBytes(payload, "item")
	if !item.IsObject() {
		return
	}
	itemType := strings.TrimSpace(item.Get("type").String())
	if _, ok := openAICompatAccumulatedItemTypes[itemType]; ok || itemType == "" {
		return
	}
	entry := openAICompatHostedItem{raw: json.RawMessage(item.Raw)}
	if index := gjson.GetBytes(payload, "output_index"); index.Exists() {
		entry.index, entry.indexed = index.Int(), true
		for i := range h.items {
			if h.items[i].indexed && h.items[i].index == entry.index {
				h.items[i] = entry
				return
			}
		}
	}
	h.items = append(h.items, entry)
}

// fillTerminalOutput gives an empty terminal output the collected hosted items
// ahead of what the accumulator rebuilds, after its leading reasoning item.
// Only terminals that carry a result are filled; the output of response.failed
// and error terminals never reaches the client and is left as the upstream sent it.
// Hosted items keep their output_index order; items reported without an
// output_index follow the indexed ones in arrival order.
func (h *openAICompatHostedItems) fillTerminalOutput(eventType string, response *apicompat.ResponsesResponse, acc *apicompat.BufferedResponseAccumulator) {
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done", "response.incomplete":
	default:
		return
	}
	if h == nil || len(h.items) == 0 || response == nil || len(response.Output) > 0 {
		return
	}
	sort.SliceStable(h.items, func(i, j int) bool {
		a, b := h.items[i], h.items[j]
		if a.indexed != b.indexed {
			return a.indexed
		}
		return a.indexed && a.index < b.index
	})
	hosted := make([]apicompat.ResponsesOutput, 0, len(h.items))
	for _, entry := range h.items {
		var item apicompat.ResponsesOutput
		if err := json.Unmarshal(entry.raw, &item); err == nil {
			hosted = append(hosted, item)
		}
	}
	if len(hosted) == 0 {
		return
	}
	var output []apicompat.ResponsesOutput
	if acc != nil && acc.HasContent() {
		output = acc.BuildOutput()
	}
	at := 0
	for at < len(output) && output[at].Type == "reasoning" {
		at++
	}
	response.Output = slices.Insert(output, at, hosted...)
}
