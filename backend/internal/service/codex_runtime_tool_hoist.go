package service

// Codex 0.166 起不再在顶层 tools 声明运行时工具，而是放在
// input[].additional_tools 里，并把 exec 这类 custom 工具嵌进 namespace
// （如 namespace "functions" 下的 custom "exec"）。
//
// 两端兼容上游都吃不下这种形状：
//   - 只支持 Chat Completions 的上游：additional_tools 由 apicompat 提升成
//     顶层 tools，但 namespace 摊平只处理 function 子工具，custom 子工具被
//     静默丢弃（apicompat.namespaceChildrenToChatTools）。
//   - Responses 直通上游：additional_tools / namespace 都是 Codex 私有扩展，
//     上游不认识就整段忽略（DeepSeek /responses 对顶层 custom 还会直接 400）。
//
// 任一情况下模型都拿不到结构化工具声明，只能把工具调用写成上游原生的 DSML
// 文本（<｜｜DSML｜｜ calls>），客户端收到一堆无法执行的正文。
//
// 这里按账号开关把 additional_tools 提升到顶层 tools，并把 namespace 里的
// custom 子工具一并提到顶层。之后既有的「顶层 custom → function 降级 + 回程
// 还原 custom_tool_call」通道就能接管，WS 桥与 HTTP 回退两条路都生效。
//
// 开关默认关闭：未开启的账号请求体逐字节不变。

import (
	"bytes"
	"strings"
)

// ExtraKeyCodexNamespaceTools 是账号级开关：启用后对 Codex 0.166 形状的
// additional_tools / namespace custom 工具做提升。默认 false。
const ExtraKeyCodexNamespaceTools = "codex_namespace_tools"

// CodexNamespaceToolsEnabled 报告账号是否启用 Codex 运行时工具提升。
func (a *Account) CodexNamespaceToolsEnabled() bool {
	if a == nil || a.Extra == nil || !a.IsOpenAI() {
		return false
	}
	enabled, ok := a.Extra[ExtraKeyCodexNamespaceTools].(bool)
	return ok && enabled
}

// hoistCodexRuntimeTools 把 input[].additional_tools 里的工具提升到顶层
// tools，并把 namespace 内的 custom 子工具提到顶层。
//
// changed 表示请求体是否被改写；未改写时返回原切片，调用方无需替换。
// 解析失败时返回原 body 与 nil error —— 提升是尽力而为的兼容层，不应改变
// 上游对非法请求体的既有报错行为。
func hoistCodexRuntimeTools(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !bodyHasCodexRuntimeTools(body) {
		return body, false, nil
	}
	var root map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &root); err != nil {
		return body, false, nil
	}
	if root == nil {
		return body, false, nil
	}

	changed := false
	tools, _ := root["tools"].([]any)
	lifted, inputChanged := liftCodexAdditionalTools(root)
	if inputChanged {
		changed = true
	}
	if len(lifted) > 0 {
		tools = appendDedupeTools(tools, lifted)
		changed = true
	}
	hoistedTools, hoisted := hoistNamespaceCustomTools(tools)
	if hoisted {
		tools = hoistedTools
		changed = true
	}
	if !changed {
		return body, false, nil
	}
	if len(tools) > 0 {
		root["tools"] = tools
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(root)
	if err != nil {
		return body, false, nil
	}
	return rebuilt, true, nil
}

// bodyHasCodexRuntimeTools 用廉价字节判断挡掉绝大多数无关请求。
func bodyHasCodexRuntimeTools(body []byte) bool {
	return bytes.Contains(body, []byte(`"additional_tools"`)) ||
		bytes.Contains(body, []byte(`"namespace"`))
}

// liftCodexAdditionalTools 把 input 里的 additional_tools 项搬到顶层，
// 并从 input 中移除这些项（上游不认识它们，留着只会被当成无意义上下文）。
func liftCodexAdditionalTools(root map[string]any) ([]any, bool) {
	input, ok := root["input"].([]any)
	if !ok || len(input) == 0 {
		return nil, false
	}
	var lifted []any
	kept := make([]any, 0, len(input))
	changed := false
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok {
			kept = append(kept, raw)
			continue
		}
		if strings.TrimSpace(stringValue(item["type"])) != "additional_tools" {
			kept = append(kept, raw)
			continue
		}
		if children, ok := item["tools"].([]any); ok {
			lifted = append(lifted, children...)
		}
		changed = true
	}
	if changed {
		root["input"] = kept
	}
	return lifted, changed
}

// appendDedupeTools 按 (type, name) 去重追加，保留先出现的声明。
// 历史里重复出现的 additional_tools 不应产生重复工具声明。
func appendDedupeTools(tools, extra []any) []any {
	if len(extra) == 0 {
		return tools
	}
	seen := make(map[string]bool, len(tools)+len(extra))
	for _, raw := range tools {
		if key, ok := toolDeclKey(raw); ok {
			seen[key] = true
		}
	}
	out := tools
	for _, raw := range extra {
		key, ok := toolDeclKey(raw)
		if !ok {
			out = append(out, raw)
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, raw)
	}
	return out
}

func toolDeclKey(raw any) (string, bool) {
	tool, ok := raw.(map[string]any)
	if !ok {
		return "", false
	}
	name := strings.TrimSpace(stringValue(tool["name"]))
	if name == "" {
		return "", false
	}
	return strings.TrimSpace(stringValue(tool["type"])) + "\x00" + name, true
}

// hoistNamespaceCustomTools 把 namespace 里的 custom 子工具提到顶层，
// namespace 内其余子工具保持不变（交给既有的摊平逻辑处理）。
//
// 顶层已存在同名工具时跳过该子工具：两条声明无法区分，保持原样是最安全的
// 处理（上游会按原有路径拒绝或忽略，不会产生歧义调用）。
func hoistNamespaceCustomTools(tools []any) ([]any, bool) {
	if len(tools) == 0 {
		return tools, false
	}
	topLevel := make(map[string]bool, len(tools))
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ := strings.TrimSpace(stringValue(tool["type"]))
		name := strings.TrimSpace(stringValue(tool["name"]))
		if name != "" && (typ == "function" || typ == "custom") {
			topLevel[name] = true
		}
	}

	changed := false
	hoisted := make([]any, 0, 4)
	for _, raw := range tools {
		ns, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(ns["type"])) != "namespace" {
			continue
		}
		children, ok := ns["tools"].([]any)
		if !ok || len(children) == 0 {
			continue
		}
		kept := make([]any, 0, len(children))
		for _, rawChild := range children {
			child, ok := rawChild.(map[string]any)
			if !ok {
				kept = append(kept, rawChild)
				continue
			}
			name := strings.TrimSpace(stringValue(child["name"]))
			if strings.TrimSpace(stringValue(child["type"])) != "custom" || name == "" || topLevel[name] {
				kept = append(kept, rawChild)
				continue
			}
			topLevel[name] = true
			hoisted = append(hoisted, child)
			changed = true
		}
		if len(kept) != len(children) {
			ns["tools"] = kept
			changed = true
		}
	}
	if !changed {
		return tools, false
	}
	return append(tools, hoisted...), true
}
