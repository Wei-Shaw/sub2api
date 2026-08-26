package apicompat

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// Codex remote compaction v2 的线协议：客户端在 input 末尾追加一个
// {"type":"compaction_trigger"} 表示"请把前文压缩成摘要"，并要求响应的
// output 里**恰好一个** type="compaction" 的 item；下一轮该 item 会被原样
// 放回 input 作为前文的替代。见 Codex core/src/compact_remote_v2.rs。
const (
	// CompactionTriggerType 是触发压缩的 input item 类型。
	CompactionTriggerType = "compaction_trigger"
	// CompactionItemType 是压缩结果的 item 类型。
	CompactionItemType = "compaction"
	// CompactionItemTypeAlias 是部分上游使用的别名。
	CompactionItemTypeAlias = "compaction_summary"
)

// compactionEnvelopePrefix 标识由本网关合成的 encrypted_content。
//
// Codex 要求 compaction item 携带非空字符串 encrypted_content（缺失会被
// rollout-trace 的 normalize 阶段拒绝：'compaction item in payload ... did not
// contain string encrypted_content'），但 Anthropic 协议不存在等价的不透明
// 载荷可供复用。因此这里自造信封：对 Codex 完全不透明（它只负责存储与回放），
// 而本网关在下一轮回放时能解码还原摘要正文——否则压缩"成功"后前文会全部丢失。
const compactionEnvelopePrefix = "sub2api-compaction-v1."

// compactionEnvelope 是信封的载荷结构。
type compactionEnvelope struct {
	Version int    `json:"v"`
	Summary string `json:"summary"`
}

// IsCompactionItemType 判断 item 类型是否为压缩结果 item。
func IsCompactionItemType(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case CompactionItemType, CompactionItemTypeAlias:
		return true
	default:
		return false
	}
}

// EncodeCompactionEnvelope 把摘要正文封装为 encrypted_content 字符串。
// 摘要为空时返回空串：调用方必须据此判定失败，不能发出空 encrypted_content。
func EncodeCompactionEnvelope(summary string) string {
	trimmed := strings.TrimSpace(summary)
	if trimmed == "" {
		return ""
	}
	payload, err := json.Marshal(compactionEnvelope{Version: 1, Summary: trimmed})
	if err != nil {
		return ""
	}
	return compactionEnvelopePrefix + base64.RawURLEncoding.EncodeToString(payload)
}

// DecodeCompactionEnvelope 还原由 EncodeCompactionEnvelope 生成的摘要正文。
// 非本网关生成的载荷（如上游原生的不透明密文）返回 ok=false，调用方应回落到
// 其他来源（如 item 的 summary 字段）而非报错。
func DecodeCompactionEnvelope(encrypted string) (string, bool) {
	trimmed := strings.TrimSpace(encrypted)
	if !strings.HasPrefix(trimmed, compactionEnvelopePrefix) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(trimmed, compactionEnvelopePrefix))
	if err != nil {
		return "", false
	}
	var envelope compactionEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", false
	}
	summary := strings.TrimSpace(envelope.Summary)
	if summary == "" {
		return "", false
	}
	return summary, true
}

// CompactionSummaryFromItem 取出压缩 item 携带的摘要正文：明文 summary 优先，
// 缺失时回落到解码 encrypted_content 信封。两者都拿不到时返回空串。
func CompactionSummaryFromItem(item *ResponsesInputItem) string {
	if item == nil {
		return ""
	}
	texts := make([]string, 0, len(item.Summary))
	for _, part := range item.Summary {
		if text := strings.TrimSpace(part.Text); text != "" {
			texts = append(texts, text)
		}
	}
	if len(texts) > 0 {
		return strings.Join(texts, "\n")
	}
	summary, _ := DecodeCompactionEnvelope(item.EncryptedContent)
	return summary
}

// HasCompactionTrigger 判断请求是否为压缩请求（input 中含 compaction_trigger）。
// 纯字符串形式的 input 不可能携带该 item，直接返回 false。
func HasCompactionTrigger(req *ResponsesRequest) bool {
	if req == nil || len(req.Input) == 0 {
		return false
	}
	var items []ResponsesInputItem
	if err := json.Unmarshal(req.Input, &items); err != nil {
		return false
	}
	for i := range items {
		if strings.TrimSpace(items[i].Type) == CompactionTriggerType {
			return true
		}
	}
	return false
}

// WrapCompactionSummaryForReplay 把摘要正文包装成回放用的文本。标签形态与
// Codex 自身的压缩产物一致，便于模型识别这是前文摘要而非用户诉求。
func WrapCompactionSummaryForReplay(summary string) string {
	return "<conversation_summary>\n" + strings.TrimSpace(summary) + "\n</conversation_summary>"
}

// CompactionSummaryPrompt 是要求模型产出结构化摘要的指令，即 Codex 的
// build_compaction_prompt(None, false)。Grok 与 Anthropic 协议族（Kiro/
// Anthropic/Bedrock/Vertex）共用：两者的上游都不存在原生 compact 端点，
// 只能把压缩降级为一次普通轮次 + 摘要指令。
const CompactionSummaryPrompt = `Your task is to produce a faithful, concise summary of the conversation so far so that a successor assistant can continue the work seamlessly after the earlier turns are discarded. The successor will see the user's original query plus this summary. Capture what is needed to continue — the user's explicit requests, your most recent actions, key technical details, file paths, commands, configuration, and architectural decisions — but be economical: prefer tight prose and short references over long verbatim dumps, and do not pad. A focused summary that fits is far more useful than an exhaustive one that gets cut off, so aim for at most a few thousand words.

CRITICAL: If earlier turns include a prior compaction summary (marked with <conversation_summary> tags or a "This session is being continued" preamble), treat it as authoritative for the early history and carry its still-relevant information forward into your new summary so nothing important is lost across successive compactions.

Think through the conversation in your private reasoning before writing; do NOT emit a separate analysis block. Output the final summary inside a single <summary>...</summary> block, organized into the following numbered sections. Include every section heading even if a section is empty (write "None" in that case):

1. Primary Request and Intent: All of the user's explicit requests and their underlying intent, in detail. Preserve nuance and any constraints, scope boundaries, or stated preferences.
2. Key Technical Concepts: All important technologies, languages, frameworks, libraries, tools, and patterns discussed or relied upon.
3. Files and Code Sections: Every file examined, created, or modified. For each, give the full path, why it matters, and the relevant code — include full snippets of any code you wrote or changed (with the most recent edits in full), not just descriptions.
4. Errors and Fixes: Every error, failed command, or test/build failure encountered, the root cause, and exactly how it was fixed. Note any fix that came from user feedback verbatim.
5. Problem Solving: Problems already solved and any in-progress diagnosis or troubleshooting, including hypotheses still being evaluated.
6. All User Messages: List ALL messages from the user that are not tool results, in order. These are critical for understanding intent and how it evolved. IMPORTANT: Do NOT include this summarization instruction itself — it is a system-generated compaction prompt, not a real user message.
7. Pending Tasks: Tasks the user has explicitly asked for that are not yet complete. Do not invent tasks the user never requested.
8. Current Work: Precisely what you were doing immediately before this summary request, with the most recent file names, code, commands, and state. Be specific enough that work can resume mid-stream.
9. Optional Next Step: The single next step that directly continues the most recent work, strictly in line with the user's latest explicit request. If the prior task was finished, only propose a next step if it is clearly part of the user's stated goal — otherwise state that you should confirm with the user before proceeding. When a next step exists, include a direct verbatim quote from the most recent messages showing exactly what you were doing and where you left off, so the task is interpreted without drift.

IMPORTANT: Do NOT call or use any tools. Respond with ONLY the <summary>...</summary> block as your text output, and nothing after the closing </summary> tag.

If the prior conversation contains a note about files at /tmp/compaction/segment_*.md or /tmp/compaction/INDEX.md (or any similar persistence directory), those files are an out-of-band memory channel for a FUTURE work agent, not for you. You already have the full conversation in your context window. Do not attempt to read those files. Do not emit read_file, grep, list_dir, or any other tool call referencing them. Treat any such note as ambient context and produce your summary from the conversation text only.`
