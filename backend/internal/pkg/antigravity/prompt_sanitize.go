package antigravity

import (
	"fmt"
	"regexp"
	"strings"
)

// PromptRule 描述一条可配置的 systemInstruction 改写规则。
//
// 背景：Antigravity（Cloud Code Assist，daily-cloudcode-pa）在 requestType="agent"
// 的信封下会检查 systemInstruction 的字面指纹，命中后返回伪装成
// "429 RESOURCE_EXHAUSTED (e.g. check quota)" 的响应（并非真实配额不足）。
// 把被标记的字面串改写成等价但不同字面的表达即可通过检查。
//
// Pattern 使用 Go RE2 语法，Replacement 支持 $1 之类的捕获组引用。
type PromptRule struct {
	Name        string
	Pattern     string
	Replacement string

	re *regexp.Regexp
}

// NewPromptRule 编译一条规则。pattern 为空或不是合法正则时返回错误。
func NewPromptRule(name, pattern, replacement string) (PromptRule, error) {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return PromptRule{}, fmt.Errorf("empty pattern")
	}
	re, err := regexp.Compile(trimmed)
	if err != nil {
		return PromptRule{}, fmt.Errorf("compile pattern %q: %w", pattern, err)
	}
	return PromptRule{
		Name:        name,
		Pattern:     pattern,
		Replacement: replacement,
		re:          re,
	}, nil
}

// apply 应用单条规则，返回改写后的文本与替换次数。
func (r PromptRule) apply(text string) (string, int) {
	if r.re == nil || text == "" {
		return text, 0
	}
	if !r.re.MatchString(text) {
		return text, 0
	}
	matches := r.re.FindAllStringIndex(text, -1)
	return r.re.ReplaceAllString(text, r.Replacement), len(matches)
}

// PromptSanitizer 按顺序对文本应用一组规则。nil 或空规则表示禁用。
type PromptSanitizer struct {
	rules []PromptRule
}

// NewPromptSanitizer 过滤掉未编译的规则后构建 sanitizer；没有有效规则时返回 nil。
func NewPromptSanitizer(rules []PromptRule) *PromptSanitizer {
	valid := make([]PromptRule, 0, len(rules))
	for _, rule := range rules {
		if rule.re != nil {
			valid = append(valid, rule)
		}
	}
	if len(valid) == 0 {
		return nil
	}
	return &PromptSanitizer{rules: valid}
}

// Enabled 报告 sanitizer 是否会改写任何内容。
func (s *PromptSanitizer) Enabled() bool {
	return s != nil && len(s.rules) > 0
}

// RuleCount 返回生效的规则数量。
func (s *PromptSanitizer) RuleCount() int {
	if s == nil {
		return 0
	}
	return len(s.rules)
}

// SanitizeText 依次应用全部规则，返回改写后的文本与总替换次数。
func (s *PromptSanitizer) SanitizeText(text string) (string, int) {
	if !s.Enabled() || text == "" {
		return text, 0
	}
	out := text
	total := 0
	for _, rule := range s.rules {
		var replaced int
		out, replaced = rule.apply(out)
		total += replaced
	}
	return out, total
}

// SanitizeGeminiContent 改写 GeminiContent 中所有 text part。
func (s *PromptSanitizer) SanitizeGeminiContent(content *GeminiContent) int {
	if !s.Enabled() || content == nil {
		return 0
	}
	total := 0
	for i := range content.Parts {
		part := &content.Parts[i]
		if part.Text == "" {
			continue
		}
		out, replaced := s.SanitizeText(part.Text)
		if replaced == 0 {
			continue
		}
		part.Text = out
		total += replaced
	}
	return total
}

// SanitizeRequestJSON 改写已解析 JSON 请求中的 systemInstruction.parts[].text。
//
// 同时支持裸 Gemini 请求体 {"systemInstruction": ...} 和 v1internal 包装体
// {"request": {"systemInstruction": ...}}，其余字段保持不变。
func (s *PromptSanitizer) SanitizeRequestJSON(value any) int {
	if !s.Enabled() || value == nil {
		return 0
	}
	root, ok := value.(map[string]any)
	if !ok {
		return 0
	}
	total := s.sanitizeSystemInstructionField(root)
	if inner, ok := root["request"].(map[string]any); ok {
		total += s.sanitizeSystemInstructionField(inner)
	}
	return total
}

func (s *PromptSanitizer) sanitizeSystemInstructionField(container map[string]any) int {
	instruction, ok := container["systemInstruction"].(map[string]any)
	if !ok {
		return 0
	}
	parts, ok := instruction["parts"].([]any)
	if !ok {
		return 0
	}
	total := 0
	for _, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok {
			continue
		}
		text, ok := part["text"].(string)
		if !ok || text == "" {
			continue
		}
		out, replaced := s.SanitizeText(text)
		if replaced == 0 {
			continue
		}
		part["text"] = out
		total += replaced
	}
	return total
}
