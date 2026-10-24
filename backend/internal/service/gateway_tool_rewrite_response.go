package service

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

// restoreToolNamesInBytes accepts complete JSON responses, SSE events or SSE
// data lines. Only protocol-defined tool-name fields are restored: text,
// arguments, thinking, signatures and unknown payloads remain byte-identical.
func restoreToolNamesInBytes(data []byte, rw *ToolNameRewrite) []byte {
	if rw == nil || len(rw.Reverse) == 0 {
		return data
	}
	if gjson.ValidBytes(data) {
		return restoreToolNamesInJSON(data, rw.Reverse)
	}
	if !bytes.HasPrefix(data, []byte("data:")) && !bytes.Contains(data, []byte("\ndata:")) {
		return data
	}
	var out []byte
	for _, line := range bytes.SplitAfter(data, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("data:")) {
			out = append(out, line[:5]...)
			out = append(out, restoreToolNamesInJSON(line[5:], rw.Reverse)...)
		} else {
			out = append(out, line...)
		}
	}
	return out
}

func restoreToolNamesInJSON(data []byte, reverse map[string]string) []byte {
	if !gjson.ValidBytes(data) {
		return data
	}
	root := gjson.ParseBytes(data)
	var edits []toolNameSpan
	addName := func(name gjson.Result) {
		if name.Type != gjson.String {
			return
		}
		original, ok := reverse[name.String()]
		if !ok || original == name.String() {
			return
		}
		encoded, err := json.Marshal(original)
		if err != nil || name.Index < 0 || name.Index+len(name.Raw) > len(data) ||
			!bytes.Equal(data[name.Index:name.Index+len(name.Raw)], []byte(name.Raw)) {
			return
		}
		edits = append(edits, toolNameSpan{name.Index, name.Index + len(name.Raw), encoded})
	}
	forEach := func(array gjson.Result, visit func(gjson.Result)) {
		if array.IsArray() {
			array.ForEach(func(_, item gjson.Result) bool {
				visit(item)
				return true
			})
		}
	}
	addReference := func(ref gjson.Result) {
		if ref.Get("type").String() == "tool_reference" {
			addName(ref.Get("tool_name"))
		}
	}
	addBlock := func(block gjson.Result) {
		switch block.Get("type").String() {
		case "tool_use":
			addName(block.Get("name"))
		case "tool_result":
			forEach(block.Get("content"), addReference)
		case "tool_search_tool_result":
			if result := block.Get("content"); result.Get("type").String() == "tool_search_tool_search_result" {
				forEach(result.Get("tool_references"), addReference)
			}
		}
	}
	addOutputItem := func(item gjson.Result) {
		switch item.Get("type").String() {
		case "function_call", "custom_tool_call":
			addName(item.Get("name"))
		}
	}
	switch eventType := root.Get("type").String(); eventType {
	case "message":
		forEach(root.Get("content"), addBlock)
	case "message_start":
		forEach(root.Get("message.content"), addBlock)
	case "content_block_start":
		addBlock(root.Get("content_block"))
	case "response.output_item.added", "response.output_item.done":
		addOutputItem(root.Get("item"))
	case "response.function_call_arguments.delta", "response.function_call_arguments.done",
		"response.custom_tool_call_input.delta", "response.custom_tool_call_input.done":
		addName(root.Get("name"))
	default:
		if strings.HasPrefix(eventType, "response.") {
			forEach(root.Get("response.output"), addOutputItem)
		}
	}
	switch root.Get("object").String() {
	case "response":
		forEach(root.Get("output"), addOutputItem)
	case "chat.completion", "chat.completion.chunk":
		forEach(root.Get("choices"), func(choice gjson.Result) {
			for _, key := range []string{"message", "delta"} {
				message := choice.Get(key)
				addName(message.Get("function_call.name"))
				forEach(message.Get("tool_calls"), func(call gjson.Result) {
					addName(call.Get("function.name"))
					addName(call.Get("custom.name"))
				})
			}
		})
	}
	return applyToolNameSpans(data, edits)
}
