package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"unsafe"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	blockTypeServerToolUse       = "server_tool_use"
	blockTypeWebSearchToolResult = "web_search_tool_result"
)

// Fast-path byte patterns: both block types only ever appear as quoted JSON
// string values, so a raw substring check is a safe pre-filter regardless of
// key/value spacing.
var (
	patternServerToolUse       = []byte(`"server_tool_use"`)
	patternWebSearchToolResult = []byte(`"web_search_tool_result"`)
)

// FilterWebSearchHistoryBlocks removes web-search content blocks from
// historical messages when the upstream cannot accept them:
//
//  1. Emulation-synthesized blocks — server_tool_use / web_search_tool_result
//     whose tool-use ID carries webSearchToolUseIDPrefix — are fabricated
//     locally by the web-search emulation (gateway_websearch_emulation.go).
//     No upstream ever issued them, so clients replaying the conversation
//     (e.g. Claude Code) poison every follow-up request. They are stripped
//     for all upstreams.
//  2. For passback-required upstreams (DeepSeek/Kimi/GLM …, see
//     ResolveThinkingProtocol) all server_tool_use / web_search_tool_result
//     blocks are stripped: these upstreams only accept
//     text/thinking/image/tool_use/tool_result and reject anything else with
//     400 "invalid value: `server_tool_use`". anthropic-strict and unknown
//     upstreams keep genuine blocks untouched.
//
// The emulated assistant turn always carries a trailing text summary, so the
// search context survives the strip. A message whose content would become
// empty gets a placeholder text block (mirroring FilterThinkingBlocksForRetry).
// Returns the original body unchanged when nothing needs stripping.
func FilterWebSearchHistoryBlocks(body []byte, mappedModel string) []byte {
	if !bytes.Contains(body, patternServerToolUse) && !bytes.Contains(body, patternWebSearchToolResult) {
		return body
	}

	stripAll := ResolveThinkingProtocol(mappedModel) == ThinkingProtocolPassbackRequired

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

		// 延迟分配：只有命中需剥离的块才构建新 slice。
		var newContent []any
		for i, block := range content {
			blockMap, isMap := block.(map[string]any)
			if isMap && shouldStripWebSearchBlock(blockMap, stripAll) {
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

func shouldStripWebSearchBlock(block map[string]any, stripAll bool) bool {
	blockType, _ := block["type"].(string)
	switch blockType {
	case blockTypeServerToolUse:
		if stripAll {
			return true
		}
		id, _ := block["id"].(string)
		return strings.HasPrefix(id, webSearchToolUseIDPrefix)
	case blockTypeWebSearchToolResult:
		if stripAll {
			return true
		}
		id, _ := block["tool_use_id"].(string)
		return strings.HasPrefix(id, webSearchToolUseIDPrefix)
	default:
		return false
	}
}

// StripAnthropicWebSearchServerTools removes web_search/google_search server
// tool definitions from the Anthropic request when the upstream model does not
// properly support them. Kimi, DeepSeek, GLM and similar third-party
// Anthropic-compatible models activate built-in search when they see
// web_search_20250305, producing "Search results for query:" text prefixes
// that pollute output and accumulate across turns.
//
// This is distinct from FilterWebSearchHistoryBlocks which strips content
// blocks from history messages — here we strip the tool DEFINITION from the
// request tools array so the upstream never sees the capability.
//
// Returns true if any tools were removed.
func StripAnthropicWebSearchServerTools(req *apicompat.AnthropicRequest, mappedModel string) bool {
	if !shouldStripAnthropicWebSearch(mappedModel) || len(req.Tools) == 0 {
		return false
	}

	kept := req.Tools[:0]
	removed := false
	var removedNames []string

	for _, tool := range req.Tools {
		if isAnthropicWebSearchServerTool(tool) {
			removed = true
			removedNames = append(removedNames, tool.Name)
			continue
		}
		kept = append(kept, tool)
	}
	req.Tools = kept

	if removed {
		req.ToolChoice = sanitizeToolChoiceAfterWebSearchRemoval(req.ToolChoice, req.Tools, removedNames)
	}
	return removed
}

// shouldStripAnthropicWebSearch determines whether web_search server tools
// should be removed for this model. Currently uses thinking protocol as a
// proxy for server tool capability — passback-required models (Kimi, DeepSeek,
// GLM, etc.) are known to not properly support Anthropic server tools.
//
// NOTE: This is a capability heuristic, not a thinking-protocol implication.
// If a passback-required model gains proper web_search support in the future,
// this function should be refined into an independent capability check.
func shouldStripAnthropicWebSearch(mappedModel string) bool {
	return ResolveThinkingProtocol(mappedModel) == ThinkingProtocolPassbackRequired
}

// isAnthropicWebSearchServerTool identifies web search server tools by their
// Type field. Regular function tools named "web_search" are NOT affected —
// only tools declared as server tools via the Type field.
func isAnthropicWebSearchServerTool(tool apicompat.AnthropicTool) bool {
	typ := strings.ToLower(strings.TrimSpace(tool.Type))
	if typ == "" {
		return false
	}
	return strings.HasPrefix(typ, "web_search") || typ == "google_search"
}

// sanitizeToolChoiceAfterWebSearchRemoval ensures tool_choice doesn't reference
// a removed web_search tool. If tool_choice forces a removed tool, it's cleared.
// If no tools remain at all, tool_choice is also cleared.
func sanitizeToolChoiceAfterWebSearchRemoval(
	toolChoice json.RawMessage,
	remainingTools []apicompat.AnthropicTool,
	removedNames []string,
) json.RawMessage {
	if len(toolChoice) == 0 {
		return toolChoice
	}

	if len(remainingTools) == 0 {
		return nil
	}

	var choice map[string]json.RawMessage
	if err := json.Unmarshal(toolChoice, &choice); err != nil {
		return toolChoice
	}
	choiceType := strings.Trim(string(choice["type"]), `"`)
	if choiceType != "tool" {
		return toolChoice
	}

	var choiceName string
	if raw, ok := choice["name"]; ok {
		_ = json.Unmarshal(raw, &choiceName)
	}

	for _, name := range removedNames {
		if choiceName == name {
			return nil
		}
	}
	return toolChoice
}
