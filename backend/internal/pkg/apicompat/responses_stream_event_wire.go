package apicompat

import "encoding/json"

// MarshalJSON renders a ResponsesStreamEvent into its wire form.
//
// The OpenAI Responses streaming protocol requires several fields to be present
// even when they hold a zero value: output_index/content_index/summary_index are
// meaningful at 0, a function_call item must always carry call_id/name/arguments
// (arguments may be ""), a message item must carry content:[] and an output_text
// part must carry text/annotations/logprobs. Go's `omitempty` drops exactly those
// zero values, and strict clients (Codex CLI) reject items/deltas whose required
// fields are missing.
//
// Rather than marshalling with omitempty and patching the JSON afterwards, every
// streamed event type is constructed explicitly here — the Go analogue of the
// reference gateways' (cc-switch, CCX) per-event object construction. This is the
// single source of truth for Responses SSE field presence and applies uniformly
// to every emitter (Chat→Responses bridge and Anthropic→Responses converter).
//
// Event types not listed fall back to the default struct marshalling, which
// bounds the blast radius of this method to the streamed item/part/text/tool
// events.
func (e ResponsesStreamEvent) MarshalJSON() ([]byte, error) {
	switch e.Type {
	case "response.output_text.delta", "response.output_text.done":
		m := e.wireBase()
		e.putItemID(m)
		m["output_index"] = e.OutputIndex
		m["content_index"] = e.ContentIndex
		if e.Type == "response.output_text.done" {
			m["text"] = e.Text
		} else {
			m["delta"] = e.Delta
		}
		return json.Marshal(m)

	case "response.content_part.added", "response.content_part.done":
		m := e.wireBase()
		e.putItemID(m)
		m["output_index"] = e.OutputIndex
		m["content_index"] = e.ContentIndex
		m["part"] = outputTextPartWire(e.Part)
		return json.Marshal(m)

	case "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done":
		m := e.wireBase()
		e.putItemID(m)
		m["output_index"] = e.OutputIndex
		m["summary_index"] = e.SummaryIndex
		if e.Type == "response.reasoning_summary_text.done" {
			m["text"] = e.Text
		} else {
			m["delta"] = e.Delta
		}
		return json.Marshal(m)

	case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		m := e.wireBase()
		e.putItemID(m)
		m["output_index"] = e.OutputIndex
		m["summary_index"] = e.SummaryIndex
		m["part"] = summaryTextPartWire(e.Part)
		return json.Marshal(m)

	case "response.output_item.added", "response.output_item.done":
		m := e.wireBase()
		m["output_index"] = e.OutputIndex
		m["item"] = responsesItemWire(e.Item)
		return json.Marshal(m)

	case "response.function_call_arguments.delta", "response.function_call_arguments.done":
		m := e.wireBase()
		e.putItemID(m)
		m["output_index"] = e.OutputIndex
		if e.CallID != "" {
			m["call_id"] = e.CallID
		}
		if e.Name != "" {
			m["name"] = e.Name
		}
		if e.Type == "response.function_call_arguments.done" {
			m["arguments"] = e.Arguments
		} else {
			m["delta"] = e.Delta
		}
		return json.Marshal(m)

	case "response.custom_tool_call_input.delta", "response.custom_tool_call_input.done":
		m := e.wireBase()
		e.putItemID(m)
		m["output_index"] = e.OutputIndex
		if e.CallID != "" {
			m["call_id"] = e.CallID
		}
		if e.Name != "" {
			m["name"] = e.Name
		}
		if e.Type == "response.custom_tool_call_input.done" {
			m["input"] = e.Input
		} else {
			m["delta"] = e.Delta
		}
		return json.Marshal(m)

	default:
		// response.created / completed / done / failed / incomplete and any
		// event type not shaped above keep the default struct marshalling.
		type alias ResponsesStreamEvent
		return json.Marshal(alias(e))
	}
}

func (e ResponsesStreamEvent) wireBase() map[string]any {
	m := map[string]any{
		"type":            e.Type,
		"sequence_number": e.SequenceNumber,
	}
	return m
}

func (e ResponsesStreamEvent) putItemID(m map[string]any) {
	if e.ItemID != "" {
		m["item_id"] = e.ItemID
	}
}

// outputTextPartWire renders a content part for a message's output_text, always
// carrying text/annotations/logprobs (matching cc-switch's push_text_delta).
func outputTextPartWire(part *ResponsesContentPart) map[string]any {
	text := ""
	if part != nil {
		text = part.Text
	}
	return map[string]any{
		"type":        "output_text",
		"text":        text,
		"annotations": []any{},
		"logprobs":    []any{},
	}
}

// summaryTextPartWire renders a reasoning summary part.
func summaryTextPartWire(part *ResponsesContentPart) map[string]any {
	text := ""
	if part != nil {
		text = part.Text
	}
	return map[string]any{
		"type": "summary_text",
		"text": text,
	}
}

// responsesItemWire renders an output_item with every field the item's type
// requires to be present, including the empty arrays/strings that omitempty
// would otherwise drop. Mirrors cc-switch's response_function_call_item and the
// message/reasoning item shapes codex expects.
func responsesItemWire(item *ResponsesOutput) map[string]any {
	if item == nil {
		return map[string]any{}
	}
	m := map[string]any{
		"type": item.Type,
		"id":   item.ID,
	}
	if item.Status != "" {
		m["status"] = item.Status
	}
	switch item.Type {
	case "message":
		role := item.Role
		if role == "" {
			role = "assistant"
		}
		m["role"] = role
		m["content"] = messageContentWire(item.Content)
	case "reasoning":
		m["summary"] = reasoningSummaryWire(item.Summary)
		if item.EncryptedContent != "" {
			m["encrypted_content"] = item.EncryptedContent
		}
	case "function_call":
		m["call_id"] = item.CallID
		m["name"] = item.Name
		m["arguments"] = item.Arguments
		// namespace 子工具的还原调用：codex 按 namespace+name 路由，缺少该字段
		// 会被判为 unsupported call。
		if item.Namespace != "" {
			m["namespace"] = item.Namespace
		}
	case "custom_tool_call":
		// custom/freeform 工具调用（如 codex 的 exec）：input 为自由文本。缺少
		// call_id/name 时 codex 无法路由该调用（表现为 unsupported call）。
		m["call_id"] = item.CallID
		m["name"] = item.Name
		m["input"] = item.Input
	case "tool_search_call":
		// tool_search 调用还原项：execution 必须为 "client"（否则 codex 忽略该
		// 调用），arguments 在线上是 JSON 对象而非字符串。
		m["call_id"] = item.CallID
		m["execution"] = "client"
		m["arguments"] = toolSearchCallArgumentsJSON(item.Arguments)
	}
	return m
}

// messageContentWire renders a message item's content array; always an array
// (never null), with each output_text part carrying its text.
func messageContentWire(parts []ResponsesContentPart) []map[string]any {
	out := make([]map[string]any, 0, len(parts))
	for _, p := range parts {
		typ := p.Type
		if typ == "" {
			typ = "output_text"
		}
		out = append(out, map[string]any{"type": typ, "text": p.Text})
	}
	return out
}

// reasoningSummaryWire renders a reasoning item's summary array; always an array.
func reasoningSummaryWire(summary []ResponsesSummary) []map[string]any {
	out := make([]map[string]any, 0, len(summary))
	for _, s := range summary {
		typ := s.Type
		if typ == "" {
			typ = "summary_text"
		}
		out = append(out, map[string]any{"type": typ, "text": s.Text})
	}
	return out
}
