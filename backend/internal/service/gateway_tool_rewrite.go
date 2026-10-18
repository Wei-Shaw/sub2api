package service

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/big"
	"sort"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// mcpServerNameWords is a built-in pool of ordinary ASCII words that can be
// used as the synthetic MCP server name. A process selects one word once and
// reuses it so tool names remain stable across requests handled by that process.
var mcpServerNameWords = []string{
	"agent", "assist", "bridge", "builder", "code", "command", "console", "context",
	"data", "debug", "design", "dev", "editor", "engine", "files", "format",
	"gateway", "helper", "index", "input", "kit", "memory", "model", "notes",
	"output", "parser", "patch", "project", "query", "reader", "render", "request",
	"response", "runner", "schema", "script", "search", "service", "shell", "store",
	"stream", "system", "task", "text", "toolbox", "tools", "transform", "utility",
	"view", "worker", "workflow", "writer",
}

var (
	mcpServerNameOnce sync.Once
	mcpServerName     string
)

// toolNameRewriteKey is the gin.Context key for the request-scoped mapping.
const toolNameRewriteKey = "claude_tool_name_rewrite"

func defaultMCPServerName() string {
	mcpServerNameOnce.Do(func() {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(mcpServerNameWords))))
		if err != nil {
			mcpServerName = "tools"
			return
		}
		mcpServerName = mcpServerNameWords[index.Int64()]
	})
	return mcpServerName
}

func mcpToolPrefix(serverName string) string {
	return "mcp__" + serverName + "__"
}

func isMCPToolName(name string) bool {
	if len(name) == 0 || len(name) > 64 || !strings.HasPrefix(name, "mcp__") {
		return false
	}
	rest := strings.TrimPrefix(name, "mcp__")
	separator := strings.Index(rest, "__")
	if separator <= 0 || separator+2 >= len(rest) {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func buildMCPToolMap(toolNames []string, serverName string, reservedNames ...string) map[string]string {
	reserved := make(map[string]bool, len(toolNames)*2+len(reservedNames))
	for _, name := range reservedNames {
		reserved[name] = true
	}
	for _, name := range toolNames {
		reserved[name] = true
	}

	// Resolve sanitized-name collisions deterministically, independent of tools[] order.
	ordered := append([]string(nil), toolNames...)
	sort.Strings(ordered)
	mapping := make(map[string]string, len(toolNames))
	for _, name := range ordered {
		if name == "" || isMCPToolName(name) {
			continue
		}
		if _, exists := mapping[name]; exists {
			continue
		}
		alias, ok := allocateMCPToolName(serverName, name, reserved)
		if !ok {
			continue
		}
		mapping[name] = alias
		reserved[alias] = true
	}
	return mapping
}

func allocateMCPToolName(serverName, original string, reserved map[string]bool) (string, bool) {
	prefix := mcpToolPrefix(serverName)
	semanticMaxLength := 64 - len(prefix)
	if !isMCPToolName(prefix + "tool") {
		return "", false
	}

	semantic, truncated := sanitizeMCPToolNameSuffix(original, semanticMaxLength)
	if !truncated {
		alias := prefix + semantic
		if alias != original && !reserved[alias] && isMCPToolName(alias) {
			return alias, true
		}
	}

	const hashLength = 8
	semanticMaxLength -= 1 + hashLength
	if semanticMaxLength < len("tool") {
		return "", false
	}
	semantic, _ = sanitizeMCPToolNameSuffix(original, semanticMaxLength)
	for attempt := uint32(0); attempt < 1<<16; attempt++ {
		alias := prefix + semantic + "_" + mcpToolHash(original, attempt)
		if !reserved[alias] && isMCPToolName(alias) {
			return alias, true
		}
	}
	return "", false
}

func sanitizeMCPToolNameSuffix(original string, maxLength int) (string, bool) {
	if maxLength < 1 {
		return "tool", true
	}
	var semantic strings.Builder
	semantic.Grow(maxLength)
	pendingSeparator := false
	truncated := false
	for _, r := range original {
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-'
		if !valid {
			pendingSeparator = semantic.Len() > 0
			continue
		}
		if pendingSeparator && semantic.Len() > 0 && semantic.Len() < maxLength {
			_ = semantic.WriteByte('_')
		}
		pendingSeparator = false
		if semantic.Len() >= maxLength {
			truncated = true
			break
		}
		_, _ = semantic.WriteRune(r)
	}
	result := strings.Trim(semantic.String(), "_-")
	if result == "" {
		result = "tool"
	}
	return result, truncated
}

func mcpToolHash(original string, attempt uint32) string {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s\x00%d", original, attempt)
	return fmt.Sprintf("%08x", h.Sum32())
}

// ToolNameRewrite is a request-scoped tool-name mapping.
//   - Forward maps client names to MCP-style names for the upstream request.
//   - Reverse maps MCP-style names back to client names in the response.
type ToolNameRewrite struct {
	Forward map[string]string
	Reverse map[string]string
}

// shouldMimicToolName 指示某个 tool 是否需要重命名。
// server tool（type != "" 且不是 "function" / "custom"）是 Anthropic 协议语义的一部分，
// 比如 "web_search_20250305" / "computer_20250124"；误改会导致上游拒绝。
func shouldMimicToolName(toolType string) bool {
	if toolType == "" || toolType == "function" || toolType == "custom" {
		return true
	}
	return false
}

// buildToolNameRewriteFromBody builds an MCP-style tool-name mapping using
// the process-stable synthetic server name.
func buildToolNameRewriteFromBody(body []byte) *ToolNameRewrite {
	return buildToolNameRewriteFromBodyWithServerName(body, defaultMCPServerName())
}

func buildToolNameRewriteFromBodyWithServerName(body []byte, serverName string) *ToolNameRewrite {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return nil
	}

	mimicableNames := make([]string, 0)
	reservedNames := make([]string, 0)
	for _, tool := range tools.Array() {
		name := tool.Get("name")
		if name.Type != gjson.String || name.String() == "" {
			continue
		}
		reservedNames = append(reservedNames, name.String())
		if shouldMimicToolName(tool.Get("type").String()) {
			mimicableNames = append(mimicableNames, name.String())
		}
	}

	mapping := buildMCPToolMap(mimicableNames, serverName, reservedNames...)
	if len(mapping) == 0 {
		return nil
	}

	rw := &ToolNameRewrite{
		Forward: mapping,
		Reverse: make(map[string]string, len(mapping)),
	}
	for original, alias := range mapping {
		rw.Reverse[alias] = original
	}

	return rw
}

type toolNameSpan struct {
	start, end int
	value      []byte
}

// applyToolNameRewriteToBody 把已构造的 ToolNameRewrite 应用到 body 上：
//
//   - 改写 $.tools[*].name（仅对 shouldMimicToolName 通过的 tool）
//   - 改写 $.tool_choice.name（仅当 $.tool_choice.type == "tool"）
//   - Rewrite context_management.edits[].exclude_tools for clear_tool_uses_20250919.
//   - 改写 $.messages[*].content[*].name（仅当 type == "tool_use"）
//   - 改写协议定义的 tool_reference.tool_name：
//     $.messages[*].content[*].content[*]（type == "tool_result"），以及
//     $.messages[*].content[*].content.tool_references[*]
//     （type == "tool_search_tool_result" 且 content.type == "tool_search_tool_search_result"）
//   - 在 $.tools[last].cache_control 上打 ephemeral 缓存断点
//
// Response bytes are restored through the request-scoped alias mapping.
func applyToolNameRewriteToBody(body []byte, rw *ToolNameRewrite) []byte {
	if rw == nil || len(rw.Forward) == 0 {
		body = applyToolsLastCacheBreakpoint(body)
		return body
	}

	// gjson child indexes are absolute offsets into the original body. Gather all
	// replacements before changing its length, then copy the body only once.
	var edits []toolNameSpan
	addName := func(name gjson.Result) {
		if !name.Exists() {
			return
		}
		fake, ok := rw.Forward[name.String()]
		if !ok {
			return
		}
		encoded, err := json.Marshal(fake)
		if err != nil || name.Index < 0 || name.Index+len(name.Raw) > len(body) ||
			!bytes.Equal(body[name.Index:name.Index+len(name.Raw)], []byte(name.Raw)) {
			return
		}
		edits = append(edits, toolNameSpan{name.Index, name.Index + len(name.Raw), encoded})
	}
	// Only the direct items of a protocol-defined tool_reference container are
	// references; tool inputs, outputs and nested payloads stay opaque.
	addToolReferences := func(refs gjson.Result) {
		if !refs.IsArray() {
			return
		}
		refs.ForEach(func(_, ref gjson.Result) bool {
			if ref.Get("type").String() == "tool_reference" {
				addName(ref.Get("tool_name"))
			}
			return true
		})
	}

	tools := gjson.GetBytes(body, "tools")
	if tools.IsArray() {
		tools.ForEach(func(_, tool gjson.Result) bool {
			if shouldMimicToolName(tool.Get("type").String()) {
				addName(tool.Get("name"))
			}
			return true
		})
	}
	if choice := gjson.GetBytes(body, "tool_choice"); choice.Get("type").String() == "tool" {
		addName(choice.Get("name"))
	}
	if contextEdits := gjson.GetBytes(body, "context_management.edits"); contextEdits.IsArray() {
		contextEdits.ForEach(func(_, edit gjson.Result) bool {
			if edit.Get("type").String() == "clear_tool_uses_20250919" {
				if excluded := edit.Get("exclude_tools"); excluded.IsArray() {
					excluded.ForEach(func(_, name gjson.Result) bool {
						if name.Type == gjson.String {
							addName(name)
						}
						return true
					})
				}
			}
			return true
		})
	}
	if messages := gjson.GetBytes(body, "messages"); messages.IsArray() {
		messages.ForEach(func(_, msg gjson.Result) bool {
			content := msg.Get("content")
			if content.IsArray() {
				content.ForEach(func(_, block gjson.Result) bool {
					switch block.Get("type").String() {
					case "tool_use":
						addName(block.Get("name"))
					case "tool_result":
						addToolReferences(block.Get("content"))
					case "tool_search_tool_result":
						if result := block.Get("content"); result.Get("type").String() == "tool_search_tool_search_result" {
							addToolReferences(result.Get("tool_references"))
						}
					}
					return true
				})
			}
			return true
		})
	}
	return applyToolsLastCacheBreakpoint(applyToolNameSpans(body, edits))
}

func applyToolNameSpans(body []byte, edits []toolNameSpan) []byte {
	if len(edits) == 0 {
		return body
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	out := make([]byte, 0, len(body))
	pos := 0
	for _, edit := range edits {
		if edit.start < pos { // Ignore any overlapping result from malformed JSON.
			continue
		}
		out = append(out, body[pos:edit.start]...)
		out = append(out, edit.value...)
		pos = edit.end
	}
	return append(out, body[pos:]...)
}

// applyToolsLastCacheBreakpoint 在最后一个非延迟加载工具上注入 cache_control
// 断点。Anthropic 不允许 defer_loading=true 的工具携带 cache_control，
// 因此会先清理所有延迟加载工具上的客户端断点。兼容官方顶层字段和
// Claude Code 使用的 custom.defer_loading 字段。其余行为对齐 Parrot
// `tools[-1]["cache_control"] = {"type":"ephemeral","ttl":"1h"}`，
// 但 ttl 按本仓规则：
//   - 客户端已为该 tool 显式设置 cache_control.ttl → 完全透传不覆盖
//   - 否则注入 {"type":"ephemeral","ttl": claude.DefaultCacheControlTTL}
//
// 纯副作用函数，tools 不存在或为空数组时 no-op。
func applyToolsLastCacheBreakpoint(body []byte) []byte {
	body = stripDeferredToolCacheControl(body)
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return body
	}
	arr := tools.Array()
	if len(arr) == 0 {
		return body
	}
	lastIdx := -1
	for idx, tool := range arr {
		if isDeferredLoadingTool(tool) {
			continue
		}
		lastIdx = idx
	}
	if lastIdx == -1 {
		return body
	}

	existingCC := arr[lastIdx].Get("cache_control")

	if existingCC.Exists() && existingCC.Get("ttl").String() != "" {
		return body
	}

	if existingCC.Exists() {
		if next, err := sjson.SetBytes(body, fmt.Sprintf("tools.%d.cache_control.ttl", lastIdx), claude.DefaultCacheControlTTL); err == nil {
			body = next
		}
		return body
	}

	raw := fmt.Sprintf(`{"type":"ephemeral","ttl":%q}`, claude.DefaultCacheControlTTL)
	if next, err := sjson.SetRawBytes(body, fmt.Sprintf("tools.%d.cache_control", lastIdx), []byte(raw)); err == nil {
		body = next
	}
	return body
}

func isDeferredLoadingTool(tool gjson.Result) bool {
	return tool.Get("defer_loading").Type == gjson.True ||
		tool.Get("custom.defer_loading").Type == gjson.True
}

// stripDeferredToolCacheControl removes the cache marker Anthropic rejects on
// deferred tools. Only the literal JSON boolean true enables deferred loading.
func stripDeferredToolCacheControl(body []byte) []byte {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return body
	}
	for idx, tool := range tools.Array() {
		if !isDeferredLoadingTool(tool) || !tool.Get("cache_control").Exists() {
			continue
		}
		if next, err := sjson.DeleteBytes(body, fmt.Sprintf("tools.%d.cache_control", idx)); err == nil {
			body = next
		}
	}
	return body
}

// toolNameRewriteFromContext 从 gin.Context 取出请求阶段保存的工具名映射。
// 找不到（c==nil 或 key 不存在或类型不对）时返回 nil；调用方必须能处理 nil。
func toolNameRewriteFromContext(c interface {
	Get(string) (any, bool)
}) *ToolNameRewrite {
	if c == nil {
		return nil
	}
	raw, ok := c.Get(toolNameRewriteKey)
	if !ok || raw == nil {
		return nil
	}
	rw, _ := raw.(*ToolNameRewrite)
	return rw
}

// reverseToolNamesIfPresent restores aliases only when the request stored a mapping.
func reverseToolNamesIfPresent(c interface {
	Get(string) (any, bool)
}, chunk []byte) []byte {
	rw := toolNameRewriteFromContext(c)
	return restoreToolNamesInBytes(chunk, rw)
}
