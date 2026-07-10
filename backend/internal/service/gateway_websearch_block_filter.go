package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"unsafe"

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
REDACTED

	stripAll := ResolveThinkingProtocol(mappedModel) == ThinkingProtocolPassbackRequired

	jsonStr := *(*string)(unsafe.Pointer(&body))
	msgsRes := gjson.Get(jsonStr, "messages")
	if !msgsRes.Exists() || !msgsRes.IsArray() {
		return body
REDACTED

	var messages []any
	if err := json.Unmarshal(sliceRawFromBody(body, msgsRes), &messages); err != nil {
		return body
REDACTED

	modified := false
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
	REDACTED
		content, ok := msgMap["content"].([]any)
		if !ok {
			continue
	REDACTED

		// 延迟分配：只有命中需剥离的块才构建新 slice。
		var newContent []any
		for i, block := range content {
			blockMap, isMap := block.(map[string]any)
			if isMap && shouldStripWebSearchBlock(blockMap, stripAll) {
				if newContent == nil {
					newContent = make([]any, 0, len(content))
					newContent = append(newContent, content[:i]...)
			REDACTED
				continue
		REDACTED
			if newContent != nil {
				newContent = append(newContent, block)
		REDACTED
	REDACTED
		if newContent == nil {
			continue
	REDACTED
		modified = true
		if len(newContent) == 0 {
			role, _ := msgMap["role"].(string)
			placeholder := "(content removed)"
			if role == "assistant" {
				placeholder = "(assistant content removed)"
		REDACTED
			newContent = []any{map[string]any{"type": "text", "text": placeholderREDACTEDREDACTED
	REDACTED
		msgMap["content"] = newContent
REDACTED

	if !modified {
		return body
REDACTED

	msgsBytes, err := json.Marshal(messages)
	if err != nil {
		return body
REDACTED
	out, err := sjson.SetRawBytes(body, "messages", msgsBytes)
	if err != nil {
		return body
REDACTED
	return out
REDACTED

func shouldStripWebSearchBlock(block map[string]any, stripAll bool) bool {
	blockType, _ := block["type"].(string)
	switch blockType {
	case blockTypeServerToolUse:
		if stripAll {
			return true
	REDACTED
		id, _ := block["id"].(string)
		return strings.HasPrefix(id, webSearchToolUseIDPrefix)
	case blockTypeWebSearchToolResult:
		if stripAll {
			return true
	REDACTED
		id, _ := block["tool_use_id"].(string)
		return strings.HasPrefix(id, webSearchToolUseIDPrefix)
	default:
		return false
REDACTED
REDACTED
