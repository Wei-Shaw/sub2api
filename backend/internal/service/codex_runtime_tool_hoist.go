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
// 这里按账号开关把 additional_tools 提升到顶层 tools，并把默认命名空间
// "functions" 里的 custom 子工具一并提到顶层。之后既有的「顶层 custom →
// function 降级 + 回程还原 custom_tool_call」通道就能接管。接入点：WS→HTTP
// 桥、Chat Completions 回退、HTTP /responses 透传（OpenAI API Key 账号）。
//
// 开关默认关闭，且只对 OpenAI API Key 账号生效：未开启的账号请求体逐字节不变。

import (
	"bytes"
	"strings"
)

// ExtraKeyCodexNamespaceTools 是账号级开关：启用后对 Codex 0.166 形状的
// additional_tools / namespace custom 工具做提升。默认 false。
const ExtraKeyCodexNamespaceTools = "codex_namespace_tools"

// CodexNamespaceToolsEnabled 报告账号是否启用 Codex 运行时工具提升。
// 只对 OpenAI API Key 账号生效：只有它们的上游路径会把顶层 custom 工具降级为
// function 并在回程还原；OAuth 上游原生支持 additional_tools / namespace，
// 提升反而会改写发往官方上游的请求体。
func (a *Account) CodexNamespaceToolsEnabled() bool {
	if a == nil || a.Extra == nil || !a.IsOpenAIApiKey() {
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
	var additional []map[string]any
	kept := make([]any, 0, len(input))
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(item["type"])) != "additional_tools" {
			kept = append(kept, raw)
			continue
		}
		if children, ok := item["tools"].([]any); ok {
			lifted = append(lifted, children...)
		}
		additional = append(additional, item)
	}
	if len(additional) == 0 {
		return nil, false
	}
	if len(kept) > 0 {
		root["input"] = kept
		return lifted, true
	}
	// Codex 的首包有时只带 additional_tools（~1.4KB），全部移除会让 input 变成
	// 空数组，上游 /responses 直接 400 "Input items array must not be empty"。
	// 此时保留 item 但清空其 tools：工具已提升到顶层，留着会被
	// apicompat.EffectiveResponsesTools 再合并一次，同名工具重复声明被 Chat
	// Completions 转换拒绝。
	changed := false
	for _, item := range additional {
		if children, ok := item["tools"].([]any); ok && len(children) == 0 {
			continue
		}
		item["tools"] = []any{}
		changed = true
	}
	return lifted, changed
}

// appendDedupeTools 按 (type, name) 去重追加；同名 namespace 不整体丢弃，而是
// 按子工具身份合并——顶层 tools 与 input[].additional_tools 各自声明同一个
// namespace 是 Codex 增量下发的常见形态，整体去重会把后声明的子工具一起丢掉。
func appendDedupeTools(tools, extra []any) []any {
	if len(extra) == 0 {
		return tools
	}
	out := tools
	index := make(map[string]int, len(tools)+len(extra))
	for i, raw := range out {
		if key, ok := toolDeclKey(raw); ok {
			index[key] = i
		}
	}
	for _, raw := range extra {
		key, ok := toolDeclKey(raw)
		if !ok {
			out = append(out, raw)
			continue
		}
		if at, seen := index[key]; seen {
			if merged, mergedOK := mergeNamespaceChildren(out[at], raw); mergedOK {
				out[at] = merged
			}
			continue
		}
		index[key] = len(out)
		out = append(out, raw)
	}
	return out
}

// mergeNamespaceChildren 合并同名 namespace 的子工具（按 (type, name) 去重），
// 保留两侧新增的声明。非 namespace 或没有新增子工具时返回 false，调用方按原逻辑
// 保留先出现的声明。
func mergeNamespaceChildren(dst, src any) (any, bool) {
	d, ok := dst.(map[string]any)
	if !ok || strings.TrimSpace(stringValue(d["type"])) != "namespace" {
		return nil, false
	}
	s, ok := src.(map[string]any)
	if !ok || strings.TrimSpace(stringValue(s["type"])) != "namespace" {
		return nil, false
	}
	srcChildren, _ := s["tools"].([]any)
	if len(srcChildren) == 0 {
		return nil, false
	}
	dstChildren, _ := d["tools"].([]any)
	seen := make(map[string]bool, len(dstChildren)+len(srcChildren))
	for _, raw := range dstChildren {
		if key, ok := toolDeclKey(raw); ok {
			seen[key] = true
		}
	}
	merged := append([]any(nil), dstChildren...)
	added := false
	for _, raw := range srcChildren {
		if key, ok := toolDeclKey(raw); ok {
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		merged = append(merged, raw)
		added = true
	}
	if !added {
		return nil, false
	}
	out := make(map[string]any, len(d)+1)
	for key, value := range d {
		out[key] = value
	}
	out["tools"] = merged
	return out, true
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

// hoistNamespaceCustomTools 把默认命名空间 "functions" 里的 custom 子工具提到
// 顶层，namespace 内其余子工具保持不变（交给既有的摊平逻辑处理）。
//
// 只处理 "functions"：降级通道回程还原出的 custom_tool_call 只带裸名，Codex 按
// ToolName::with_default_namespace 把缺省 namespace 补成 "functions" 再查注册
// 表（codex-rs protocol/src/tool_name.rs、core/src/tools/router.rs）。其它命名
// 空间的 custom 工具提升后会被路由到 functions/<name>，找不到处理器或与默认命名
// 空间的同名工具串线，因此保持原样。
//
// 顶层已存在同名工具时跳过该子工具：两条声明无法区分，保持原样是最安全的
// 处理（上游会按原有路径拒绝或忽略，不会产生歧义调用）。子工具全部被提走的
// namespace 一并移除，避免向上游发送空的 namespace 声明。
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
	out := make([]any, 0, len(tools))
	var hoisted []any
	for _, raw := range tools {
		ns, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(ns["type"])) != "namespace" ||
			strings.TrimSpace(stringValue(ns["name"])) != codexDefaultToolNamespace {
			out = append(out, raw)
			continue
		}
		children, ok := ns["tools"].([]any)
		if !ok || len(children) == 0 {
			out = append(out, raw)
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
		}
		if len(kept) == len(children) {
			out = append(out, raw)
			continue
		}
		changed = true
		if len(kept) == 0 {
			continue
		}
		// 复制而非原地修改：同一个 namespace 对象可能仍被 input 里的
		// additional_tools 项引用。
		rewritten := make(map[string]any, len(ns))
		for key, value := range ns {
			rewritten[key] = value
		}
		rewritten["tools"] = kept
		out = append(out, rewritten)
	}
	if !changed {
		return tools, false
	}
	return append(out, hoisted...), true
}

// codexDefaultToolNamespace 是 Codex 的默认工具命名空间（DEFAULT_FUNCTION_NAMESPACE），
// 与无 namespace 的顶层工具等价。
const codexDefaultToolNamespace = "functions"
