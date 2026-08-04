package apicompat

import "encoding/json"

// MarshalJSON renders a ResponsesStreamEvent into its wire form.
//
// The OpenAI Responses streaming protocol requires several fields to be present
// even when they hold a zero value: output_index/content_index/summary_index are
// meaningful at 0, a function_call item must always carry call_id/name/arguments
// (arguments may be ""), a message item must carry content:[] and an output_text
// part must carry text/annotations/logprobs. Go's `omitempty` drops exactly those
// zero values, and strict clients (Codex CLI / Grok) reject items/deltas whose
// required fields are missing.
//
// Rather than marshalling with omitempty and patching the JSON afterwards, every
// streamed event type is constructed explicitly here — the Go analogue of the
// reference gateways' (cc-switch, CCX) per-event object construction. This is the
// single source of truth for Responses SSE field presence and applies uniformly
// to every emitter (Chat→Responses bridge and Anthropic→Responses converter).
//
// Event types not listed fall back to the default struct marshalling, which
// bounds the blast radius of this method to the streamed item/part/text/tool
// and terminal response events.
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
		m["part"] = responsesContentPartWireValue(e.Part)
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
		defaultStatus := "in_progress"
		if e.Type == "response.output_item.done" {
			defaultStatus = "completed"
		}
		m["item"] = responsesOutputItemWireValue(e.Item, defaultStatus)
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

	case "response.created", "response.in_progress", "response.completed", "response.done",
		"response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		// Terminal/lifecycle events must carry a fully-shaped response.output so
		// strict clients can deserialize message/output_text items (Grok requires
		// annotations; Codex requires content arrays and item ids).
		m := e.wireBase()
		if e.Response != nil {
			m["response"] = responsesResponseWire(e.Response, responseItemDefaultStatus(e.Type, e.Response.Status))
		}
		if e.Usage != nil {
			m["usage"] = e.Usage
		}
		return json.Marshal(m)

	default:
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
	annotations := []json.RawMessage{}
	logprobs := []json.RawMessage{}
	if part != nil {
		text = part.Text
		if part.Annotations != nil {
			annotations = part.Annotations
		}
		if part.Logprobs != nil {
			logprobs = part.Logprobs
		}
	}
	return map[string]any{
		"type":        "output_text",
		"text":        text,
		"annotations": annotations,
		"logprobs":    logprobs,
	}
}

func responsesContentPartWireValue(part *ResponsesContentPart) any {
	if part == nil {
		return map[string]any{}
	}
	if part.Type == "" || part.Type == "text" || part.Type == "output_text" {
		return outputTextPartWire(part)
	}
	return part
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

// responseItemDefaultStatus picks the default status for output items nested
// under a lifecycle/terminal response event when the item itself has no status.
func responseItemDefaultStatus(eventType, responseStatus string) string {
	switch eventType {
	case "response.completed", "response.done":
		return "completed"
	case "response.incomplete":
		return "incomplete"
	case "response.failed", "response.cancelled", "response.canceled":
		if responseStatus != "" {
			return responseStatus
		}
		return "failed"
	case "response.created", "response.in_progress":
		return "in_progress"
	default:
		if responseStatus != "" {
			return responseStatus
		}
		return "completed"
	}
}

// responsesResponseWire renders a Response object with every nested item/part
// field strict clients require. response.id is preserved as-is; message items
// get msg_… ids and completed status when missing; output_text parts always
// carry annotations/logprobs (even empty).
func responsesResponseWire(resp *ResponsesResponse, defaultItemStatus string) map[string]any {
	if resp == nil {
		return map[string]any{}
	}
	object := resp.Object
	if object == "" {
		object = "response"
	}
	m := map[string]any{
		"id":     resp.ID,
		"object": object,
		"model":  resp.Model,
		"status": resp.Status,
		"output": responsesOutputListWire(resp.Output, defaultItemStatus),
	}
	if resp.Usage != nil {
		m["usage"] = resp.Usage
	}
	if resp.IncompleteDetails != nil {
		m["incomplete_details"] = resp.IncompleteDetails
	}
	if resp.Error != nil {
		m["error"] = resp.Error
	}
	return m
}

func responsesOutputListWire(items []ResponsesOutput, defaultStatus string) []any {
	out := make([]any, 0, len(items))
	for i := range items {
		out = append(out, responsesOutputItemWireValue(&items[i], defaultStatus))
	}
	return out
}

func responsesItemUsesStrictWire(itemType string) bool {
	switch itemType {
	case "tool_search_call", "message", "function_call", "custom_tool_call", "reasoning":
		return true
	default:
		return false
	}
}

// responsesOutputItemWireValue applies the strict builder only to item types it
// fully models. Other item types retain their default JSON representation so
// fields such as web_search_call.action are not discarded.
func responsesOutputItemWireValue(item *ResponsesOutput, defaultStatus string) any {
	if item == nil {
		return map[string]any{}
	}
	if responsesItemUsesStrictWire(item.Type) {
		return responsesItemWire(item, defaultStatus)
	}
	copyItem := *item
	if copyItem.Status == "" {
		copyItem.Status = defaultStatus
	}
	return copyItem
}

// responsesItemWire renders an output_item with every field the item's type
// requires to be present, including the empty arrays/strings that omitempty
// would otherwise drop. Mirrors cc-switch's response_function_call_item and the
// message/reasoning item shapes codex/Grok expect.
//
// defaultStatus is applied when item.Status is empty (e.g. "completed" on
// output_item.done / response.completed, "in_progress" on output_item.added).
func responsesItemWire(item *ResponsesOutput, defaultStatus string) map[string]any {
	if item == nil {
		return map[string]any{}
	}
	id := item.ID
	if id == "" && item.Type == "message" {
		id = generateMessageItemID()
	}
	m := map[string]any{
		"type": item.Type,
		"id":   id,
	}
	status := item.Status
	if status == "" {
		status = defaultStatus
	}
	if status != "" {
		m["status"] = status
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
// (never null). output_text parts always carry text/annotations/logprobs so
// strict deserializers (Grok ResponseStreamEvent) accept the payload.
func messageContentWire(parts []ResponsesContentPart) []map[string]any {
	out := make([]map[string]any, 0, len(parts))
	for i := range parts {
		p := parts[i]
		typ := p.Type
		if typ == "" || typ == "text" {
			typ = "output_text"
		}
		if typ == "output_text" {
			part := p
			part.Type = "output_text"
			out = append(out, outputTextPartWire(&part))
			continue
		}
		m := map[string]any{"type": typ}
		if p.Text != "" {
			m["text"] = p.Text
		}
		if p.ImageURL != "" {
			m["image_url"] = p.ImageURL
		}
		out = append(out, m)
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
