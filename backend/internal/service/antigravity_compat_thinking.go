package service

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	// antigravityThinkingHeadroom 是思考预算之外预留给可见答案的输出 token 余量。
	// Antigravity 上游把 thinking 与正文放在同一个 maxOutputTokens 预算里；余量不足时
	// 思考会吃满预算，最后只剩 thoughtSignature 帧，兼容层看到的就是空流。
	antigravityThinkingHeadroom = 8192
	// antigravityMaxOutputTokens 是上游允许的最大输出 token 数。
	antigravityMaxOutputTokens = 65535
	// antigravityTinyOutputLimit 是客户端显式给出的极小输出上限阈值。低于该值时
	// 用户显然只想拿少量 token，不应为了思考余量把上限抬高。
	antigravityTinyOutputLimit = 1024
)

// antigravityThinkingBudgetForModel 按 Antigravity 上游模型名返回固定思考预算。
// 与 Antigravity-Manager 的 model_specs 保持一致：档位后缀决定预算，flash 系列比
// 非 flash 低 1；Gemini 4 / argon 的 low 档最低 1024。ok=false 表示不该注入思考参数
// （非 Gemini 3/4、图像/语音/向量类模型）。
func antigravityThinkingBudgetForModel(model string) (int, bool) {
	name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(model), "models/"))
	if !strings.HasPrefix(name, "gemini-") {
		return 0, false
	}
	for _, kind := range []string{"image", "tts", "embedding", "live", "audio"} {
		if strings.Contains(name, kind) {
			return 0, false
		}
	}
	version, _, _ := strings.Cut(strings.TrimPrefix(name, "gemini-"), "-")
	majorText, _, _ := strings.Cut(version, ".")
	major, err := strconv.Atoi(majorText)
	if err != nil || major < 3 {
		return 0, false
	}

	isFlash := strings.Contains(name, "flash")
	isArgon := strings.Contains(name, "argon") || strings.HasPrefix(name, "gemini-4")
	switch {
	case strings.Contains(name, "-extra-low"):
		if isArgon {
			return 1024, true
		}
		return 1000, true
	case strings.Contains(name, "-low"), strings.Contains(name, "-minimal"):
		if isArgon {
			return 1024, true
		}
		return 1000, true
	case strings.Contains(name, "-medium"):
		return 4000, true
	case strings.Contains(name, "-high"), strings.Contains(name, "-xhigh"), strings.Contains(name, "-max"):
		if isFlash {
			return 10000, true
		}
		return 10001, true
	default:
		if isFlash {
			return 4000, true
		}
		return 10001, true
	}
}

// applyAntigravityGeminiThinkingConfig 给 Antigravity Gemini 3/4 请求注入固定思考预算，
// 并保证 maxOutputTokens 至少是「思考预算 + 8192」。客户端显式关闭思考时不注入。
func applyAntigravityGeminiThinkingConfig(geminiBody, claudeBody []byte, upstreamModel string) []byte {
	if parseClaudeThinkingIntent(claudeBody).thinkingType == "disabled" {
		return geminiBody
	}
	budget, ok := antigravityThinkingBudgetForModel(upstreamModel)
	if !ok {
		return geminiBody
	}
	out, err := sjson.SetBytes(geminiBody, "generationConfig.thinkingConfig.includeThoughts", true)
	if err != nil {
		return geminiBody
	}
	out, err = sjson.SetBytes(out, "generationConfig.thinkingConfig.thinkingBudget", budget)
	if err != nil {
		return geminiBody
	}
	return ensureAntigravityOutputHeadroom(out, budget)
}

// ensureAntigravityOutputHeadroom 把 maxOutputTokens 抬到思考预算 + 余量；
// 客户端已经给了更大的值则保留原值。
func ensureAntigravityOutputHeadroom(body []byte, budget int) []byte {
	want := budget + antigravityThinkingHeadroom
	if want > antigravityMaxOutputTokens {
		want = antigravityMaxOutputTokens
	}
	current := int(gjson.GetBytes(body, "generationConfig.maxOutputTokens").Int())
	if current > 0 && current < antigravityTinyOutputLimit {
		return body
	}
	if current >= want {
		return body
	}
	out, err := sjson.SetBytes(body, "generationConfig.maxOutputTokens", want)
	if err != nil {
		return body
	}
	return out
}
