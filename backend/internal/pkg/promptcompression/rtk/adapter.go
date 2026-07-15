package rtk

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

var shellToolNames = map[string]bool{
	"bash": true, "shell": true, "terminal": true, "exec": true,
	"exec_command": true, "run_command": true, "execute_command": true,
	"local_shell": true,
}

type toolInfo struct{ name, command string }

// discoverTargets performs protocol-specific, fail-closed target discovery.
// It intentionally returns no target for malformed JSON or unsupported shapes.
func discoverTargets(body []byte, protocol Protocol, protected map[string]bool) ([]Target, error) {
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("invalid JSON")
	}
	switch protocol {
	case ProtocolAnthropic:
		return discoverAnthropic(body, protected), nil
	case ProtocolChat:
		return discoverChat(body, protected), nil
	case ProtocolResponses:
		return discoverResponses(body, protected), nil
	case ProtocolGemini:
		return discoverGemini(body, protected), nil
	default:
		return nil, nil
	}
}

func discoverAnthropic(body []byte, protected map[string]bool) []Target {
	uses := map[string]toolInfo{}
	ambiguous := map[string]bool{}
	var targets []Target
	gjson.GetBytes(body, "messages").ForEach(func(mi, msg gjson.Result) bool {
		msg.Get("content").ForEach(func(ci, block gjson.Result) bool {
			if block.Get("type").String() == "tool_use" {
				id := block.Get("id").String()
				if id != "" {
					if _, exists := uses[id]; exists {
						ambiguous[id] = true
						delete(uses, id)
						return true
					}
					uses[id] = toolInfo{name: block.Get("name").String(), command: commandFrom(block.Get("input"))}
				}
			}
			return true
		})
		_ = mi
		return true
	})
	gjson.GetBytes(body, "messages").ForEach(func(mi, msg gjson.Result) bool {
		msg.Get("content").ForEach(func(ci, block gjson.Result) bool {
			if block.Get("type").String() != "tool_result" {
				return true
			}
			id := block.Get("tool_use_id").String()
			base := fmt.Sprintf("messages.%d.content.%d", mi.Int(), ci.Int())
			if ambiguous[id] {
				targets = append(targets, Target{Protocol: ProtocolAnthropic, JSONPath: base + ".content", ToolCallID: id, Eligible: false, SkipReason: "ambiguous_tool_call"})
				return true
			}
			info, ok := uses[id]
			cache := protected[base] || block.Get("cache_control").Exists() || block.Get("content.#(cache_control)").Exists()
			if !ok {
				targets = append(targets, Target{Protocol: ProtocolAnthropic, JSONPath: base + ".content", ToolCallID: id, Eligible: false, SkipReason: "orphan_tool_result"})
				return true
			}
			appendTextTargets(&targets, ProtocolAnthropic, base+".content", block.Get("content"), id, info.name, info.command, block.Get("is_error").Bool(), cache)
			return true
		})
		return true
	})
	return targets
}

func discoverChat(body []byte, protected map[string]bool) []Target {
	uses := map[string]toolInfo{}
	ambiguous := map[string]bool{}
	var targets []Target
	gjson.GetBytes(body, "messages").ForEach(func(_, msg gjson.Result) bool {
		if msg.Get("role").String() != "assistant" {
			return true
		}
		msg.Get("tool_calls").ForEach(func(_, call gjson.Result) bool {
			id := call.Get("id").String()
			name := call.Get("function.name").String()
			if name == "" {
				name = call.Get("name").String()
			}
			args := call.Get("function.arguments")
			if !args.Exists() {
				args = call.Get("arguments")
			}
			if id != "" {
				if _, exists := uses[id]; exists {
					ambiguous[id] = true
					delete(uses, id)
					return true
				}
				uses[id] = toolInfo{name: name, command: commandFrom(args)}
			}
			return true
		})
		return true
	})
	gjson.GetBytes(body, "messages").ForEach(func(mi, msg gjson.Result) bool {
		if msg.Get("role").String() != "tool" {
			return true
		}
		id := msg.Get("tool_call_id").String()
		base := fmt.Sprintf("messages.%d.content", mi.Int())
		if ambiguous[id] {
			targets = append(targets, Target{Protocol: ProtocolChat, JSONPath: base, ToolCallID: id, Eligible: false, SkipReason: "ambiguous_tool_call"})
			return true
		}
		info, ok := uses[id]
		cache := protected[base] || msg.Get("cache_control").Exists() || msg.Get("content.#(cache_control)").Exists()
		if !ok {
			targets = append(targets, Target{Protocol: ProtocolChat, JSONPath: base, ToolCallID: id, Eligible: false, SkipReason: "orphan_tool_result"})
			return true
		}
		appendTextTargets(&targets, ProtocolChat, base, msg.Get("content"), id, info.name, info.command, msg.Get("is_error").Bool(), cache)
		return true
	})
	return targets
}

func discoverResponses(body []byte, protected map[string]bool) []Target {
	uses := map[string]toolInfo{}
	ambiguous := map[string]bool{}
	var targets []Target
	gjson.GetBytes(body, "input").ForEach(func(_, item gjson.Result) bool {
		typ := item.Get("type").String()
		if typ != "function_call" && typ != "custom_tool_call" {
			return true
		}
		id := item.Get("call_id").String()
		if id == "" {
			id = item.Get("id").String()
		}
		if id != "" {
			if _, exists := uses[id]; exists {
				ambiguous[id] = true
				delete(uses, id)
				return true
			}
			uses[id] = toolInfo{name: item.Get("name").String(), command: commandFrom(item.Get("arguments"))}
		}
		return true
	})
	gjson.GetBytes(body, "input").ForEach(func(i, item gjson.Result) bool {
		typ := item.Get("type").String()
		if typ != "function_call_output" && typ != "custom_tool_call_output" {
			return true
		}
		id := item.Get("call_id").String()
		base := fmt.Sprintf("input.%d.output", i.Int())
		if ambiguous[id] {
			targets = append(targets, Target{Protocol: ProtocolResponses, JSONPath: base, ToolCallID: id, Eligible: false, SkipReason: "ambiguous_tool_call"})
			return true
		}
		info, ok := uses[id]
		cache := protected[base] || item.Get("cache_control").Exists()
		if !ok {
			targets = append(targets, Target{Protocol: ProtocolResponses, JSONPath: base, ToolCallID: id, Eligible: false, SkipReason: "orphan_tool_result"})
			return true
		}
		appendTextTargets(&targets, ProtocolResponses, base, item.Get("output"), id, info.name, info.command, item.Get("is_error").Bool(), cache)
		return true
	})
	return targets
}

func discoverGemini(body []byte, protected map[string]bool) []Target {
	uses := map[string]toolInfo{}
	ambiguous := map[string]bool{}
	var targets []Target
	// First collect all function calls so responses can be correlated even
	// when the provider sends them out of order.
	gjson.GetBytes(body, "contents").ForEach(func(_, content gjson.Result) bool {
		content.Get("parts").ForEach(func(_, part gjson.Result) bool {
			call := part.Get("functionCall")
			if !call.Exists() {
				return true
			}
			name := call.Get("name").String()
			if name == "" {
				return true
			}
			if _, exists := uses[name]; exists {
				ambiguous[name] = true
				delete(uses, name)
				return true
			}
			uses[name] = toolInfo{name: name, command: commandFrom(call.Get("args"))}
			return true
		})
		return true
	})
	gjson.GetBytes(body, "contents").ForEach(func(ci, content gjson.Result) bool {
		content.Get("parts").ForEach(func(pi, part gjson.Result) bool {
			response := part.Get("functionResponse")
			if !response.Exists() {
				return true
			}
			name := response.Get("name").String()
			base := fmt.Sprintf("contents.%d.parts.%d.functionResponse.response", ci.Int(), pi.Int())
			cache := response.Get("cache_control").Exists() || response.Get("response.cache_control").Exists()
			if ambiguous[name] {
				targets = append(targets, Target{Protocol: ProtocolGemini, JSONPath: base, ToolName: name, Eligible: false, SkipReason: "ambiguous_tool_call"})
				return true
			}
			info, ok := uses[name]
			if !ok {
				targets = append(targets, Target{Protocol: ProtocolGemini, JSONPath: base, ToolName: name, Eligible: false, SkipReason: "orphan_tool_result"})
				return true
			}
			// Never stringify Gemini's response object. Only explicitly textual
			// fields can be patched in place; all other structured data is kept.
			for _, key := range []string{"text", "output", "stdout"} {
				value := response.Get("response." + key)
				if value.Type != gjson.String {
					continue
				}
				path := base + "." + key
				eligible, reason := eligibleTool(info.name, info.command, response.Get("is_error").Bool(), cache)
				targets = append(targets, Target{Protocol: ProtocolGemini, JSONPath: path, ToolCallID: name, ToolName: info.name, Command: info.command, Text: value.String(), IsError: response.Get("is_error").Bool(), CacheProtected: cache, Eligible: eligible, SkipReason: reason})
			}
			return true
		})
		return true
	})
	return targets
}

func appendTextTargets(dst *[]Target, protocol Protocol, base string, value gjson.Result, id, name, command string, isError, cache bool) {
	if value.Type == gjson.String {
		eligible, reason := eligibleTool(name, command, isError, cache)
		*dst = append(*dst, Target{Protocol: protocol, JSONPath: base, ToolCallID: id, ToolName: name, Command: command, Text: value.String(), IsError: isError, CacheProtected: cache, Eligible: eligible, SkipReason: reason})
		return
	}
	if !value.IsArray() {
		*dst = append(*dst, Target{Protocol: protocol, JSONPath: base, ToolCallID: id, ToolName: name, Command: command, IsError: isError, CacheProtected: cache, Eligible: false, SkipReason: "non_text_content"})
		return
	}
	value.ForEach(func(i, child gjson.Result) bool {
		if child.Get("type").String() != "text" && child.Get("type").String() != "input_text" && child.Get("type").String() != "output_text" {
			return true
		}
		text := child.Get("text")
		if text.Type != gjson.String {
			return true
		}
		path := fmt.Sprintf("%s.%d.text", base, i.Int())
		eligible, reason := eligibleTool(name, command, isError, cache)
		*dst = append(*dst, Target{Protocol: protocol, JSONPath: path, ToolCallID: id, ToolName: name, Command: command, Text: text.String(), IsError: isError, CacheProtected: cache, Eligible: eligible, SkipReason: reason})
		return true
	})
}

func commandFrom(v gjson.Result) string {
	if v.Type == gjson.String {
		text := strings.TrimSpace(v.String())
		if strings.HasPrefix(text, "{") {
			if parsed := gjson.Parse(text); parsed.IsObject() {
				return commandFrom(parsed)
			}
		}
		return text
	}
	for _, path := range []string{"command", "cmd", "script", "input", "action.command"} {
		if value := v.Get(path); value.Type == gjson.String {
			return value.String()
		}
	}
	return ""
}

func eligibleTool(name, command string, isError, cache bool) (bool, string) {
	if isError {
		return false, "error_result"
	}
	if cache {
		return false, "cache_protected"
	}
	if !shellToolNames[strings.ToLower(strings.TrimSpace(name))] {
		return false, "non_shell_tool"
	}
	if strings.TrimSpace(command) == "" {
		return false, "missing_command"
	}
	return true, ""
}
