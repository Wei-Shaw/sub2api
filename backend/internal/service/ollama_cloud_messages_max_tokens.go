package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

// clampOllamaCloudAnthropicMessagesMaxTokens 是 Anthropic Messages 出站
// （/v1/messages，含 CC / Responses 桥接与 passthrough）的 max_tokens clamp 钩子，
// 与 raw CC / Responses 路径共用 cap 配置与实现。判定与出站 URL 组装同源：调用方
// 传入本次实际选用的 Anthropic 上游 base_url（GetBaseURL 或
// GetAnthropicProtocolBaseURL，不用 GetOpenAIBaseURL——adaptive 时那是 CC/Responses
// 地址），与真实出站组装一样先 TrimRight "/"（urlvalidator 与
// nativeAnthropicTargetURL 同款归一化），仅当其为 ollama.com 且 body 中映射后出站
// model 为 DeepSeek 系时压到 cap，否则原样返回。
func clampOllamaCloudAnthropicMessagesMaxTokens(account *Account, baseURL string, body []byte) []byte {
	if account == nil || len(body) == 0 {
		return body
	}
	if !isOllamaCloudBaseURL(strings.TrimRight(strings.TrimSpace(baseURL), "/")) {
		return body
	}
	if !isDeepSeekModel(gjson.GetBytes(body, "model").String()) {
		return body
	}
	return clampOllamaCloudMaxTokens(account, body)
}
