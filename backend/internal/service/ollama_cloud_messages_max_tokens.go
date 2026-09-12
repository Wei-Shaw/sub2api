package service

import (
	"strings"
)

// clampOllamaCloudAnthropicMessagesMaxTokens 是 Anthropic Messages 出站
// （/v1/messages，含 CC / Responses 桥接与 passthrough）的 max_tokens clamp 钩子，
// 与 raw CC / Responses 路径共用 cap 配置与实现。判定与出站 URL 组装同源：调用方
// 传入本次实际选用的 Anthropic 上游 base_url（GetBaseURL 或
// GetAnthropicProtocolBaseURL，不用 GetOpenAIBaseURL——adaptive 时那是 CC/Responses
// 地址），与真实出站组装一样先 TrimRight "/"（urlvalidator 与
// nativeAnthropicTargetURL 同款归一化），命中 ollama.com 即按账号 cap 钳制，不看
// 模型家族（DeepSeek 系与非 DeepSeek 模型统一 65535），否则原样返回。
func clampOllamaCloudAnthropicMessagesMaxTokens(account *Account, baseURL string, body []byte) []byte {
	if account == nil || len(body) == 0 {
		return body
	}
	if !isOllamaCloudBaseURL(strings.TrimRight(strings.TrimSpace(baseURL), "/")) {
		return body
	}
	return clampOllamaCloudMaxTokens(account, body)
}
