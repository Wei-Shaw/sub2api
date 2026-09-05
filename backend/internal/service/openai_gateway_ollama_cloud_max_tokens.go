package service

import (
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

// OllamaCloudMaxTokensCapExtraKey 是账号 extra 中的可选配置键，表示该 Ollama Cloud
// 账号输出 token 的 provider 级硬上限。用户可通过 admin 账号更新 API 的 extra 字段
// 设置，覆盖默认值 ollamaCloudDefaultMaxTokensCap；0 或负数表示显式禁用 clamp。
const OllamaCloudMaxTokensCapExtraKey = "ollama_max_tokens_cap"

// ollamaCloudDefaultMaxTokensCap 是 Ollama Cloud 对输出 token 数的 provider 级硬上限
// （约 65535），max_tokens 超过该值会被上游直接 400 拒绝；该上限与模型无关，不做模型过滤。
const ollamaCloudDefaultMaxTokensCap = 65535

// 本文件的 clampOllamaCloudMaxTokens 被
// applyOllamaCloudRawChatCompletionsRequest（openai_gateway_ollama_cloud_cc_reasoning.go）
// 调用，账号检测（isOllamaCloudRawChatCompletionsAccount）由调用方完成，此处不再重复判断。

// ollamaCloudMaxTokensCap 返回账号配置的 max_tokens 上限。账号为 nil 或 extra 中
// 无该键时返回默认值；键值为数值类型（float64/int64/int/json.Number）时返回其整数
// 值（0 或负数表示显式禁用 clamp）；其它类型回退默认值。
func ollamaCloudMaxTokensCap(account *Account) int64 {
	if account == nil || account.Extra == nil {
		return ollamaCloudDefaultMaxTokensCap
	}
	value, ok := account.Extra[OllamaCloudMaxTokensCapExtraKey]
	if !ok {
		return ollamaCloudDefaultMaxTokensCap
	}
	switch number := value.(type) {
	case float64:
		return int64(number)
	case int64:
		return number
	case int:
		return int64(number)
	case json.Number:
		parsed, err := number.Int64()
		if err != nil {
			return ollamaCloudDefaultMaxTokensCap
		}
		return parsed
	default:
		return ollamaCloudDefaultMaxTokensCap
	}
}

// clampOllamaCloudMaxTokens 把 body 中超过 cap 的 max_tokens / max_completion_tokens
// 单向压到 cap。cap <= 0 或 body 不是合法 JSON 时原样返回；sjson 出错时返回原始 body。
// 有任一字段被 clamp 时记录一条 Debug 日志。
func clampOllamaCloudMaxTokens(account *Account, body []byte) []byte {
	cap := ollamaCloudMaxTokensCap(account)
	if cap <= 0 || !gjson.ValidBytes(body) {
		return body
	}
	clamped := false
	out := body
	for _, key := range []string{"max_tokens", "max_completion_tokens"} {
		result := gjson.GetBytes(out, key)
		if !result.Exists() || result.Type != gjson.Number || result.Int() <= cap {
			continue
		}
		updated, err := sjson.SetBytes(out, key, cap)
		if err != nil {
			return body
		}
		out = updated
		clamped = true
	}
	if clamped && account != nil {
		logger.L().Debug("openai chat_completions raw: clamped max_tokens for ollama cloud account",
			zap.Int64("account_id", account.ID),
			zap.Int64("cap", cap),
		)
	}
	return out
}
