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

// clampOllamaCloudUpstreamMaxTokens 是 raw CC 出站（forwardAsRawChatCompletions 与
// /v1/responses 降级 forwardResponsesViaRawChatCompletions 共用）的独立 token 钩子，
// 与 reasoning 钩子解耦：只看本次请求实际选用的 CC 上游 base_url（GetOpenAIBaseURL，
// 与 openAIChatCompletionsTargetURL 的取值一致）是否 ollama.com，不读 usage extra。
// DeepSeek 系模型（用模型映射后的真实出站 model，body 中 model 已改写）任意平台均
// clamp；非 DeepSeek 模型仅保留既有 openai 平台 Ollama 账号（force_chat_completions
// 判定）的 clamp，不扩展到其它平台。
func clampOllamaCloudUpstreamMaxTokens(account *Account, body []byte) []byte {
	if account == nil || len(body) == 0 {
		return body
	}
	if !isOllamaCloudBaseURL(account.GetOpenAIBaseURL()) {
		return body
	}
	if !isDeepSeekModel(gjson.GetBytes(body, "model").String()) && !isOllamaCloudRawChatCompletionsAccount(account) {
		return body
	}
	return clampOllamaCloudMaxTokens(account, body)
}

// ollamaCloudResponsesUpstreamBaseURL 返回原生 /v1/responses 本次实际选用的上游
// base_url，取值与 buildUpstreamRequest 一致：adaptive 原生 CN 账号用 api_base_urls
// 的 responses 地址，其余用 GetOpenAIBaseURL。
func ollamaCloudResponsesUpstreamBaseURL(account *Account) string {
	if account.UsesNativeCNResponses() && account.IsAdaptiveAPIProtocol() {
		return account.GetCNProtocolBaseURL(APIProtocolResponses)
	}
	return account.GetOpenAIBaseURL()
}

// ollamaCloudResponsesMaxOutputTokensClamp 是原生 /v1/responses 路径的 clamp：在
// 平台字段归一化之后独立调用，不改写原有的 max_output_tokens 平台 switch。判定：
// 实际 Responses 上游（ollamaCloudResponsesUpstreamBaseURL）为 ollama.com 的
// APIKey 账号 + 映射后的出站模型是 DeepSeek 系；openai 平台 max_tokens 刚被归一化
// patch 到 max_output_tokens 时取 max_tokens 为生效值。未命中返回 ok=false。
// 实测：ollama.com/v1/responses max_output_tokens=256000 被上游以 "max_tokens
// (256000) exceeds model's maximum output tokens (65536)" 400 拒绝（DeepSeek 模型）。
func ollamaCloudResponsesMaxOutputTokensClamp(account *Account, upstreamModel string, body []byte) (int64, bool) {
	if account == nil || account.Type != AccountTypeAPIKey || !isDeepSeekModel(upstreamModel) {
		return 0, false
	}
	if !isOllamaCloudBaseURL(ollamaCloudResponsesUpstreamBaseURL(account)) {
		return 0, false
	}
	value := gjson.GetBytes(body, "max_output_tokens")
	if !value.Exists() && account.Platform == PlatformOpenAI {
		value = gjson.GetBytes(body, "max_tokens")
	}
	cap := ollamaCloudMaxTokensCap(account)
	if cap <= 0 || !value.Exists() || value.Type != gjson.Number || value.Int() <= cap {
		return 0, false
	}
	return cap, true
}

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
