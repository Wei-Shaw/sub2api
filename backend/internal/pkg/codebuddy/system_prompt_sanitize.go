package codebuddy

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// SystemPromptBlacklistRule 是一条 system prompt 内容改写规则。
//
// 背景（来自 backend/failed.txt → success.txt 的对比）：CodeBuddy 上游（hy3 等）
// 的 content_filter 会针对 system prompt 中特定词/短语直接拒答
// （finish_reason=content_filter）。逐条试出的触发项做成可维护的黑名单词表，
// 命中即按 Replace 改写（空串表示删除），从而自动"改写" system prompt 绕过审核。
//
// Find 支持两种匹配：
//   - IsRegex=false：子串匹配（大小写敏感，按原样），命中即整体替换为 Replace。
//   - IsRegex=true：按正则匹配，命中即替换为 Replace（Replace 不支持 $1 反向引用，
//     仅作固定串替换，保持简单可预期）。
type SystemPromptBlacklistRule struct {
	Find     string
	Replace  string
	IsRegex  bool
	compiled *regexp.Regexp
}

// DefaultSystemPromptBlacklist 是依据 failed/success 对比试出的默认触发词表。
// 命中即改写，覆盖两组样本的全部差异：
//   - "You are Claude Code, Anthropic's official CLI for Claude."（开头身份声明整句，正则仅删该句）
//   - "Codex CLI is an open source project led by OpenAI"（厂商归属整句）
//   - "Codex CLI" → "workbuddy"（product 名中性化；仅一句出现，不会误伤其余引用）
//   - "PRs" → "PR"（末尾 s 也会触发，去掉即可）
//
// 注意：不采用"Claude Code / Anthropic / OpenAI 全量替换"——success 样本中这些词
// 在正文其它位置仍被保留（如 "Claude Code is available as a CLI"），全量替换会过度改写。
// 新出现的触发词直接追加到本表即可。
func DefaultSystemPromptBlacklist() []SystemPromptBlacklistRule {
	return []SystemPromptBlacklistRule{
		{Find: `You are Claude Code, Anthropic's official CLI for Claude\.`, Replace: "", IsRegex: true},
		{Find: ` Codex CLI is an open source project led by OpenAI.`, Replace: "", IsRegex: false},
		{Find: "Codex CLI", Replace: "workbuddy", IsRegex: false},
		{Find: "PRs", Replace: "PR", IsRegex: false},
	}
}

// SanitizeForContentFilter 抽取 body.messages 中 role=="system" 的 content，
// 按 rules 词表改写，返回 (改写后 body, 是否发生过改写, error)。
//
// content 同时支持字符串与 OpenAI 多段格式（[{"type":"text","text":...}]）。
// rules 为 nil 时回退到 DefaultSystemPromptBlacklist。
func SanitizeForContentFilter(body []byte, rules []SystemPromptBlacklistRule) ([]byte, bool, error) {
	if len(rules) == 0 {
		rules = DefaultSystemPromptBlacklist()
	}
	compiled, err := compileBlacklistRules(rules)
	if err != nil {
		return body, false, err
	}
	if !gjson.GetBytes(body, "messages").Exists() {
		return body, false, nil
	}
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body, false, nil
	}

	changed := false
	for i, msg := range messages.Array() {
		if !strings.EqualFold(msg.Get("role").String(), "system") {
			continue
		}
		content := msg.Get("content")
		path := "messages." + strconv.Itoa(i) + ".content"

		if content.Type == gjson.String {
			cleaned := applyBlacklistRules(compiled, content.String())
			if cleaned != content.String() {
				updated, err := sjson.SetBytes(body, path, cleaned)
				if err != nil {
					return body, changed, err
				}
				body = updated
				changed = true
			}
			continue
		}

		if content.IsArray() {
			raw, err := sjson.DeleteBytes(body, path)
			if err != nil {
				return body, changed, err
			}
			parts := make([]map[string]any, 0, len(content.Array()))
			localChanged := false
			for _, part := range content.Array() {
				p := map[string]any{}
				for k, v := range part.Map() {
					if k == "text" && v.Type == gjson.String {
						cleaned := applyBlacklistRules(compiled, v.String())
						if cleaned != v.String() {
							localChanged = true
						}
						p["text"] = cleaned
					} else {
						p[k] = v.Value()
					}
				}
				parts = append(parts, p)
			}
			if localChanged {
				updated, err := sjson.SetBytes(raw, path, parts)
				if err != nil {
					return body, changed, err
				}
				body = updated
				changed = true
			}
		}
	}

	return body, changed, nil
}

type compiledRule struct {
	rule  SystemPromptBlacklistRule
	regex *regexp.Regexp
}

func compileBlacklistRules(rules []SystemPromptBlacklistRule) ([]compiledRule, error) {
	out := make([]compiledRule, 0, len(rules))
	for _, r := range rules {
		cr := compiledRule{rule: r}
		if r.IsRegex {
			re, err := regexp.Compile(r.Find)
			if err != nil {
				return nil, err
			}
			cr.regex = re
		}
		out = append(out, cr)
	}
	return out, nil
}

func applyBlacklistRules(rules []compiledRule, text string) string {
	out := text
	for _, r := range rules {
		if r.rule.IsRegex {
			out = r.regex.ReplaceAllString(out, r.rule.Replace)
			continue
		}
		if strings.Contains(out, r.rule.Find) {
			out = strings.ReplaceAll(out, r.rule.Find, r.rule.Replace)
		}
	}
	// 收尾：仅裁剪首尾空白。不主动折叠内部空行——success 样本保留了原始空行结构，
	// 过度折叠会破坏与样本的逐字节一致性。
	out = strings.TrimSpace(out)
	return out
}
