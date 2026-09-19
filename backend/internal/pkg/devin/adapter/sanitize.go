// 本文件处理上游内容策略拦截的已知文案。
//
// Devin 上游有两道门：competitor-fingerprint（竞品身份指纹）和
// content-policy（permission_denied）。指纹匹配按完整特征句/段落触发，
// 单独出现 "Claude Code"、"Anthropic"、"FREEFORM" 之外的普通词不拦截。
// 规则集对齐 WindsurfAPI identity-neutralize.js（a1–a7 均为线上 A/B 验证），
// 另补一条本项目实测发现的 tool-call 冒号句。改写为等义中性文案，
// 保持系统提示语义不变。
//
// 注意：上游策略非确定性（同一 prompt 可能先封后放），规则只覆盖已实证触发器，
// 不做猜测性改写，避免误伤用户正文。
package adapter

import (
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

// withinParagraph 匹配可跨单换行但不跨空行的文本（RE2 无 lookahead，
// 用「换行后必须跟非空行内容」近似 WindsurfAPI 的 \n(?!\s*\n)）。
const withinParagraph = `(?:[^\n]|\n[ \t]*[^\n \t])*?`

const securityBenign = "Decline requests that facilitate clearly malicious or harmful activity, and otherwise help the user with their software engineering task."

// upstreamSanitizeRule 是一条已实证的上游拦截文案改写规则。
type upstreamSanitizeRule struct {
	id          string
	pattern     *regexp.Regexp
	replacement string
	// promptOnly 为 true 的裸词规则只应用于 system prompt 与工具描述，
	// 不改写用户消息正文，避免破坏用户数据（如代码里的 FREEFORM 字面量）。
	promptOnly bool
	// trigger 是该 pattern 任何匹配必然包含的小写字面子串，供
	// strings.Contains 预筛：文本不含 trigger 则规则不可能命中。
	// 新增规则必须填写且宁短勿泛——写错会让规则静默失效。
	trigger string
}

func rule(id, pattern, replacement, trigger string) upstreamSanitizeRule {
	return upstreamSanitizeRule{id: id, pattern: regexp.MustCompile(pattern), replacement: replacement, trigger: trigger}
}

// upstreamSanitizeRules 顺序与参考实现一致：先整段/整句，再单行兜底。
var upstreamSanitizeRules = []upstreamSanitizeRule{
	// (a1) Claude Code 自我身份句（竞品指纹门），直/弯撇号、完整句/名词短语两形态。
	rule("a1-cc-full", `(?i)You are Claude Code,\s*Anthropic['’]?s official CLI for Claude\.?`, "You are an AI coding assistant.", "claude code"),
	rule("a1-cc-noun", `(?i)Claude Code,\s*Anthropic['’]?s official CLI for Claude\.?`, "an AI coding assistant.", "claude code"),
	// (a2) Claude Agent SDK 自我身份句（content-policy）。
	rule("a2-sdk-full", `(?i)You are a Claude agent, built on Anthropic['’]?s Claude Agent SDK\.?`, "You are an AI coding assistant.", "claude agent"),
	rule("a2-sdk-noun", `(?i)\ba Claude agent, built on Anthropic['’]?s Claude Agent SDK\.?`, "an AI coding assistant.", "claude agent"),
	// (a3) Claude Code 注入系统提示的计费头行（竞品指纹），整行剥除。
	rule("a3-billing", `(?im)^\s*x-anthropic-billing-header:[^\n]*\n?`, "", "x-anthropic-billing-header"),
	// (b) 安全策略整段（abuse gate）：跨段失败时由 b-security-line 单行兜底。
	rule("b-security", `(?i)IMPORTANT:\s*Assist with authorized security testing`+withinParagraph+`(?:defensive use cases\.|security research[^.]*\.)`, securityBenign, "authorized security testing"),
	rule("b-security-line", `(?i)IMPORTANT:\s*Assist with authorized security testing[^\n]*`, securityBenign, "authorized security testing"),
	// dual-use 句本身是独立指纹（本项目实测）；无 IMPORTANT 前缀时兜底改写。
	rule("b-dualluse", `(?i)Dual-use security tools \(C2 frameworks, credential testing, exploit development\) require clear authorization context:[^\n]*`, "Dual-use security tooling (e.g. C2 frameworks, credential testing, exploit development) needs explicit authorization context such as pentesting engagements, CTF competitions, security research, or defensive use cases.", "dual-use security tools"),
	// (a4) Claude Code Environment 品牌块与型号目录，均为段落级指纹。
	rule("a4-brand-span", `(?i)Claude Code is available as a CLI`+withinParagraph+`available on Opus [\d./]+\.`, "This coding assistant runs in a terminal.", "claude code is available"),
	rule("a4-fastmode", `(?im)(?:^|\n)\s*-?\s*Fast mode for Claude Code[^\n]*\n?`, "\n", "fast mode for claude code"),
	rule("a4-cli-line", `(?i)Claude Code is available as a CLI[^\n]*\n?`, "This coding assistant runs in a terminal.\n", "claude code is available"),
	rule("a4-catalogue", `(?i)The most recent Claude models are`+withinParagraph+`most capable Claude models\.`, "", "the most recent claude models"),
	rule("a4-catalogue-line", `(?im)The most recent Claude models are[^\n]*\n?`, "", "the most recent claude models"),
	// 「You are powered by the model …」与「The exact model ID is …」自我型号指纹：
	// 句点需跟空白或行尾（RE2 无 lookahead，用 (?:\.(\s|$)|$) 实现，(?m) 使 $ 匹配行尾）。
	rule("a4-poweredby", `(?im)You are powered by the model[^\n]*?(?:\.(?:\s|$)|$)\n?`, "", "powered by the model"),
	rule("a4-modelid", `(?im)The exact model ID is[^\n]*?(?:\.(?:\s|$)|$)\n?`, "", "the exact model id is"),
	// (a5) Cline 能力吹嘘句：触发点是句式而非名字，名字保留。
	rule("a5-cline-boast", `You are ([A-Z][\w.-]*), a highly skilled software engineer with extensive knowledge in many programming languages, frameworks, design patterns,? and best practices\.`, "You are $1, a software engineer.", "a highly skilled software engineer"),
	// (a6) Grok/xAI 自我身份句 + executing_actions_with_care 整块。
	rule("a6-grok-full", `(?i)You are Grok[\w .-]* released by xAI\.?`, "You are an AI coding assistant.", "released by xai"),
	rule("a6-grok-noun", `(?i)\bGrok[\w .-]* released by xAI\.?`, "an AI coding assistant.", "released by xai"),
	rule("a6-grok2-care", `(?is)<executing_actions_with_care>.*?</executing_actions_with_care>`, "", "executing_actions_with_care"),
	// (a7) codex apply_patch 工具描述里的 FREEFORM 裸词与 JSON 包裹句。
	// 裸词可能命中用户正文（SQL/代码标识符），只作用于 prompt/工具描述。
	{id: "a7-freeform", pattern: regexp.MustCompile(`FREEFORM`), replacement: "free-form", promptOnly: true, trigger: "freeform"},
	{id: "a7-json-wrap", pattern: regexp.MustCompile(`do not wrap the patch in JSON\.`), replacement: "provide the patch as plain text.", promptOnly: true, trigger: "do not wrap the patch in json"},
	// 本项目实测：Claude Code 提示词的 tool-call 冒号句也是指纹。
	rule("cc-colon-toolcall", `(?i)Do not use a colon before tool calls\.[^\n]*?with a period\.`, "Never put a colon before a tool call; write text like \"Let me read the file.\" ending with a period instead of a colon before the call.", "colon before tool calls"),
	// CC 2.1.x 提示词新增指纹行（本项目逐行 bisect 实证）：
	// 自动压缩句、/help 品牌行、Agent 工具引导句、CLAUDE.md 行、memory 强制句。
	rule("cc-autocompact", `(?i)The system will automatically compress prior messages in your conversation as it approaches context limits\.[^\n]*`, "Earlier messages may be automatically summarized as the conversation grows long, so the conversation is not bounded by the context window.", "automatically compress prior messages"),
	// cc-help-line/cc-feedback 的指纹是裸句本身（上游实测：无行首/列表
	// 前缀也拦），故不做行首锚定；列表形态的 "- " 前缀得以保留，替换结果不变。
	rule("cc-help-line", `(?i)/help:\s*Get help with using Claude Code[^\n]*`, "/help: Get help with using this CLI", "/help:"),
	rule("cc-agent-tool", `(?i)Use the Agent tool with specialized agents when the task at hand matches the agent's description\.[^\n]*`, "Use the Agent tool with specialized agents when the task matches the agent's description. Delegation is useful for parallelizing independent queries and for keeping the main context window free of excessive results, but avoid using it when not needed, and do not repeat work already delegated to a subagent.", "use the agent tool with specialized agents"),
	rule("cc-claudemd", `(?i)Anything already documented in CLAUDE\.md files\.`, "Anything already documented in project instruction files.", "claude.md"),
	rule("cc-memory-must", `(?i)You MUST access memory when the user explicitly asks you to check, recall, or remember\.`, "Always consult memory when the user explicitly asks you to check, recall, or remember.", "must access memory"),
	rule("cc-feedback", `(?i)To give feedback, users should report the issue at https://github\.com/anthropics/claude-code/issues[^\n]*`, "To give feedback, users should report issues to the maintainers of this CLI.", "claude-code"),
	rule("cc-blast-radius", `(?i)Carefully consider the reversibility and blast radius of actions\.`, "Carefully consider the reversibility and impact of actions.", "blast radius"),
	rule("cc-claudemd-2", `(?i)durable instructions like CLAUDE\.md files`, "durable instructions like project instruction files", "claude.md"),
	// CC 2.1.x subagent 系统提示的 emoji 禁令行（本项目逐行 bisect 实证：
	// 指纹是整句，"For clear communication…" 前缀与 "MUST avoid" 缺一不可）。
	rule("cc-subagent-emojis", `(?i)For clear communication with the user the assistant MUST avoid using emojis\.`, "Keep communication with the user clear and free of emojis.", "avoid using emojis"),
	// Codex CLI 系提示词指纹（codex 0.153.x 模板逐句/逐组合 bisect 实证）：
	// "Codex refers to" 定义句整句触发，去掉 "Codex" 或缩短尾部即放行。
	rule("codex-opensource-def", `(?i)Codex refers to the open-source agentic coding interface`, "Codex is the open-source coding interface", "codex refers to the open-source"),
	// plan 状态句对：仅「batch-complete」紧跟「Finish with all items…」
	// 该顺序相邻时触发，单独任一句、倒序或中间插句均放行。
	rule("codex-plan-statuses", `(?i)Do not batch-complete multiple items after the fact\. Finish with all items completed or explicitly canceled/deferred before ending the turn\.`, "Do not batch-complete multiple items after the fact. Before ending the turn, leave all items completed or explicitly canceled/deferred.", "do not batch-complete multiple items"),
	// ANSI 转义句：「Don't output ANSI escape codes directly」与
	// 「the CLI renderer applies them」须同句共现，缺一或换主语即放行。
	rule("codex-ansi-escapes", `Don['’]t output ANSI escape codes directly — the CLI renderer applies them\.`, "Never output ANSI escape codes directly — the CLI renderer applies them.", "ansi escape codes directly"),
}

// sanitizeRequest 改写请求中所有会被上游策略拦截的已知文案，
// 返回按规则 id 统计的命中数——改写本身是静默的，命中计数
// 随请求日志落盘让「代理动过什么」可查。
func sanitizeRequest(request llm.RequestMessages) (llm.RequestMessages, map[string]int) {
	hits := make(map[string]int)
	request.SystemPrompt = sanitizeUpstreamText(request.SystemPrompt, true, hits)
	for index, message := range request.Messages {
		switch typed := message.(type) {
		case llm.UserMessage:
			typed.Content = sanitizeContents(typed.Content, hits)
			request.Messages[index] = typed
		case llm.AssistantMessage:
			typed.Content = sanitizeContents(typed.Content, hits)
			request.Messages[index] = typed
		case llm.ToolResultMessage:
			typed.Content = sanitizeContents(typed.Content, hits)
			request.Messages[index] = typed
		}
	}
	for index, tool := range request.Tools {
		request.Tools[index].Description = sanitizeUpstreamText(tool.Description, true, hits)
	}
	// 上游会拒绝「声明了 tools 但 system prompt 为空」的请求（实测触发
	// permission_denied）；注入最小中性身份句兜底。
	if strings.TrimSpace(request.SystemPrompt) == "" && len(request.Tools) > 0 {
		request.SystemPrompt = "You are an AI coding assistant."
		hits["inject-empty-system"]++
	}
	return request, hits
}

func sanitizeContents(content []llm.Content, hits map[string]int) []llm.Content {
	for index, block := range content {
		switch typed := block.(type) {
		case llm.TextContent:
			typed.Text = sanitizeUpstreamText(typed.Text, false, hits)
			content[index] = typed
		case llm.ThinkingContent:
			typed.Thinking = sanitizeUpstreamText(typed.Thinking, false, hits)
			content[index] = typed
		}
	}
	return content
}

// sanitizeBucketsAll / sanitizeBucketsMessages 把全部（或仅非 promptOnly）
// trigger 按首字节折小写（b | 0x20）分桶：预筛逐字节查桶，桶命中再做
// EqualFold 前缀比对。干净文本（绝大多数块）单趟扫描、零分配返回，
// 免去逐规则 strings.Contains 全扫和 ToLower 整文拷贝。
var (
	sanitizeBucketsAll      = triggerBuckets(true)
	sanitizeBucketsMessages = triggerBuckets(false)
)

func triggerBuckets(includePromptOnly bool) [256][]string {
	var buckets [256][]string
	for _, rule := range upstreamSanitizeRules {
		if rule.promptOnly && !includePromptOnly {
			continue
		}
		buckets[rule.trigger[0]|0x20] = append(buckets[rule.trigger[0]|0x20], rule.trigger)
	}
	return buckets
}

func hasSanitizeTrigger(text string, buckets *[256][]string) bool {
	for i := 0; i < len(text); i++ {
		for _, trigger := range buckets[text[i]|0x20] {
			if len(text)-i >= len(trigger) && strings.EqualFold(text[i:i+len(trigger)], trigger) {
				return true
			}
		}
	}
	return false
}

// sanitizeUpstreamText 按规则集改写文本并把命中数累加进 hits（由
// sanitizeRequest 统一分配）；替换串允许含 $ 捕获组引用，故命中数
// 用 FindAll 先数一遍而非 ReplaceAllStringFunc 统计。
func sanitizeUpstreamText(text string, includePromptOnly bool, hits map[string]int) string {
	if text == "" {
		return text
	}
	buckets := &sanitizeBucketsMessages
	if includePromptOnly {
		buckets = &sanitizeBucketsAll
	}
	if !hasSanitizeTrigger(text, buckets) {
		return text
	}
	// 每条规则的 trigger 是该 pattern 任何匹配必然包含的字面词；
	// 预筛命中后仍按 trigger 逐规则跳过，再做正则改写。
	lower := strings.ToLower(text)
	for _, rule := range upstreamSanitizeRules {
		if rule.promptOnly && !includePromptOnly {
			continue
		}
		if !strings.Contains(lower, rule.trigger) {
			continue
		}
		if matches := rule.pattern.FindAllStringIndex(text, -1); len(matches) > 0 {
			hits[rule.id] += len(matches)
			text = rule.pattern.ReplaceAllString(text, rule.replacement)
		}
	}
	return text
}
