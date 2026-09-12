package service

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Ollama Cloud 的 OpenAI 兼容 /v1/chat/completions 把思维放在 reasoning / thinking，
// 而 DeepSeek/OpenAI 客户端只认 reasoning_content。仅在 raw CC 直转路径上做 wire JSON
// 双向补齐，不改 CC↔Responses / Anthropic / Grok 桥。

// isOllamaCloudDeepSeekUpstream 判断本次出站是否命中 Ollama Cloud 上托管的
// DeepSeek 模型：出站 base_url 是 Ollama Cloud（GetOpenAIBaseURL，与
// clampOllamaCloudUpstreamMaxTokens 的取值一致）且映射后的出站模型是 DeepSeek 系。
// 不看 platform / account.Type / responses-mode，不读 usage extra；同一分组内
// 官方 DeepSeek 账号（api.deepseek.com）不命中，保持字节级透传。后续其它出站
// 路径复用同一判定。nil account / 空模型返回 false。
func isOllamaCloudDeepSeekUpstream(account *Account, upstreamModel string) bool {
	if account == nil {
		return false
	}
	if !isOllamaCloudBaseURL(account.GetOpenAIBaseURL()) {
		return false
	}
	return isDeepSeekModel(upstreamModel)
}

// applyOllamaCloudRawChatCompletionsRequest 只做 Ollama Cloud reasoning 归一化；
// max_tokens clamp 已解耦到独立钩子 clampOllamaCloudUpstreamMaxTokens，由出站方依次调用。
func applyOllamaCloudRawChatCompletionsRequest(account *Account, upstreamModel string, body []byte) []byte {
	if !isOllamaCloudDeepSeekUpstream(account, upstreamModel) || len(body) == 0 {
		return body
	}
	return normalizeOllamaCloudChatCompletionsRequest(body)
}

func applyOllamaCloudRawChatCompletionsResponse(account *Account, upstreamModel string, body []byte) []byte {
	if !isOllamaCloudDeepSeekUpstream(account, upstreamModel) || len(body) == 0 {
		return body
	}
	return normalizeOllamaCloudChatCompletionsResponseJSON(body)
}

func applyOllamaCloudRawChatCompletionsSSELine(account *Account, upstreamModel string, line string) string {
	if !isOllamaCloudDeepSeekUpstream(account, upstreamModel) || line == "" {
		return line
	}
	return normalizeOllamaCloudChatCompletionsSSELine(line)
}

func normalizeOllamaCloudChatCompletionsRequest(body []byte) []byte {
	if !gjson.ValidBytes(body) {
		return body
	}
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}
	updated := body
	changed := false
	for i, msg := range messages.Array() {
		if msg.Get("role").String() != "assistant" {
			continue
		}
		reasoningContent, ok := jsonNonEmptyString(msg.Get("reasoning_content"))
		if !ok {
			continue
		}
		if _, has := jsonNonEmptyString(msg.Get("reasoning")); has {
			continue
		}
		if _, has := jsonNonEmptyString(msg.Get("thinking")); has {
			continue
		}
		next, err := sjson.SetBytes(updated, "messages."+strconv.Itoa(i)+".reasoning", reasoningContent)
		if err != nil {
			return body
		}
		updated = next
		changed = true
	}
	if !changed {
		return body
	}
	return updated
}

func normalizeOllamaCloudChatCompletionsResponseJSON(body []byte) []byte {
	if !gjson.ValidBytes(body) {
		return body
	}
	choices := gjson.GetBytes(body, "choices")
	if !choices.IsArray() {
		return body
	}
	updated := body
	changed := false
	for i, choice := range choices.Array() {
		for _, container := range []string{"message", "delta"} {
			obj := choice.Get(container)
			if !obj.Exists() || !obj.IsObject() {
				continue
			}
			if obj.Get("reasoning_content").Exists() {
				continue
			}
			src, ok := jsonNonEmptyString(obj.Get("reasoning"))
			if !ok {
				src, ok = jsonNonEmptyString(obj.Get("thinking"))
			}
			if !ok {
				continue
			}
			next, err := sjson.SetBytes(updated, "choices."+strconv.Itoa(i)+"."+container+".reasoning_content", src)
			if err != nil {
				return body
			}
			updated = next
			changed = true
		}
	}
	if !changed {
		return body
	}
	return updated
}

func normalizeOllamaCloudChatCompletionsSSELine(line string) string {
	payload, ok := extractOpenAISSEDataLine(line)
	if !ok {
		return line
	}
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" || trimmed == "[DONE]" {
		return line
	}
	rewritten := normalizeOllamaCloudChatCompletionsResponseJSON([]byte(payload))
	if string(rewritten) == payload {
		return line
	}
	prefixLen := len(line) - len(payload)
	if prefixLen < 0 {
		return line
	}
	return line[:prefixLen] + string(rewritten)
}

func jsonNonEmptyString(v gjson.Result) (string, bool) {
	if v.Type != gjson.String || v.Str == "" {
		return "", false
	}
	return v.Str, true
}
