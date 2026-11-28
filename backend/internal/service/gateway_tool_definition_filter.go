package service

import (
	"bytes"
	"encoding/json"
	"unsafe"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	blockTypeToolAddition   = "tool_addition"
	blockTypeToolDefinition = "tool_definition"
)

// Fast-path byte pattern: the block/tool type only ever appears as a quoted
// JSON string value, so a raw substring check is a safe pre-filter regardless
// of key/value spacing.
var patternToolDefinition = []byte(`"tool_definition"`)

// FilterInlineToolDefinitionBlocks strips inline-tool-definition content
// blocks (mid-conversation-tool-changes / inline-tools beta) from messages
// when the upstream Anthropic-compatible endpoint does not support them.
//
// DeepSeek and similar providers expose an Anthropic-compatible endpoint that
// accepts tool_reference (a reference to an already-declared tool) but rejects
// tool_definition (an inline tool schema) with 422 "unknown variant
// `tool_definition`, expected `tool_reference`". Because these providers do
// not implement the inline-tools beta, an inline tool definition carries no
// upstream meaning and is stripped so the request body can be deserialized.
// tool_reference blocks are kept untouched.
//
// A message whose content would become empty gets a placeholder text block
// (mirroring FilterWebSearchHistoryBlocks). Returns the original body
// unchanged when nothing needs stripping.
func FilterInlineToolDefinitionBlocks(body []byte) []byte {
	if !bytes.Contains(body, patternToolDefinition) {
		return body
	}

	jsonStr := *(*string)(unsafe.Pointer(&body))
	msgsRes := gjson.Get(jsonStr, "messages")
	if !msgsRes.Exists() || !msgsRes.IsArray() {
		return body
	}

	var messages []any
	if err := json.Unmarshal(sliceRawFromBody(body, msgsRes), &messages); err != nil {
		return body
	}

	modified := false
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		content, ok := msgMap["content"].([]any)
		if !ok {
			continue
		}

		// Lazy allocation: only build a new slice once a block is actually
		// stripped, mirroring FilterWebSearchHistoryBlocks.
		var newContent []any
		for i, block := range content {
			blockMap, isMap := block.(map[string]any)
			if isMap && shouldStripToolDefinitionBlock(blockMap) {
				if newContent == nil {
					newContent = make([]any, 0, len(content))
					newContent = append(newContent, content[:i]...)
				}
				continue
			}
			if newContent != nil {
				newContent = append(newContent, block)
			}
		}
		if newContent == nil {
			continue
		}
		modified = true
		if len(newContent) == 0 {
			role, _ := msgMap["role"].(string)
			placeholder := "(content removed)"
			if role == "assistant" {
				placeholder = "(assistant content removed)"
			}
			newContent = []any{map[string]any{"type": "text", "text": placeholder}}
		}
		msgMap["content"] = newContent
	}

	if !modified {
		return body
	}

	msgsBytes, err := json.Marshal(messages)
	if err != nil {
		return body
	}
	out, err := sjson.SetRawBytes(body, "messages", msgsBytes)
	if err != nil {
		return body
	}
	return out
}

func shouldStripToolDefinitionBlock(block map[string]any) bool {
	blockType, _ := block["type"].(string)
	switch blockType {
	case blockTypeToolDefinition:
		return true
	case blockTypeToolAddition:
		tool, ok := block["tool"].(map[string]any)
		if !ok {
			return false
		}
		toolType, _ := tool["type"].(string)
		return toolType == blockTypeToolDefinition
	default:
		return false
	}
}
