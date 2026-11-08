package service

import "encoding/json"

// ProbeUsageTokens 是一次内部探针（智力检测 / 渠道监控直连）从上游响应里解析出的 token 用量。
//
// 与网关计费口径对齐：OpenAI 的 input/prompt tokens 含缓存读取，这里把缓存拆出去
// 分别落在 CacheRead 上；Anthropic 的 input_tokens 本来就不含缓存，直接取。
type ProbeUsageTokens struct {
	Input         int
	Output        int
	CacheCreation int
	CacheRead     int
}

// IsZero 报告是否一个 token 都没解析到（此时记账行 token 列保持 0）。
func (u ProbeUsageTokens) IsZero() bool {
	return u.Input == 0 && u.Output == 0 && u.CacheCreation == 0 && u.CacheRead == 0
}

// mergeMax 把一组用量并入：同一字段取最大值。
//
// 为什么是「取最大」而不是累加：流式响应的 usage 可能分散在多个事件里
// （Anthropic 的 input 在 message_start、output 在 message_delta，
// 有些 OpenAI 兼容上游每个 chunk 都带一份 usage），累加会翻倍，取最大既能汇总
// 又不会把重复事件算两遍。
func (u *ProbeUsageTokens) mergeMax(input, output, cacheCreation, cacheRead int) {
	if u == nil {
		return
	}
	if input > u.Input {
		u.Input = input
	}
	if output > u.Output {
		u.Output = output
	}
	if cacheCreation > u.CacheCreation {
		u.CacheCreation = cacheCreation
	}
	if cacheRead > u.CacheRead {
		u.CacheRead = cacheRead
	}
}

// captureProbeUsage 从一段已解析的 JSON 对象里提取 token 用量并合并进 dst。
// 覆盖四种形态：
//   - OpenAI Responses：顶层 usage / 流式 response.completed 的 response.usage
//   - OpenAI Chat Completions：顶层 usage 或流式尾部 usage chunk
//   - Anthropic Messages：message_start 的 message.usage、message_delta 的 usage
//   - Gemini generateContent：usageMetadata
func captureProbeUsage(dst *ProbeUsageTokens, obj map[string]any) {
	if dst == nil || len(obj) == 0 {
		return
	}
	if usage := objectValue(obj["usage"]); usage != nil {
		applyProbeUsageObject(dst, usage)
	}
	if response := objectValue(obj["response"]); response != nil {
		if usage := objectValue(response["usage"]); usage != nil {
			applyProbeUsageObject(dst, usage)
		}
	}
	if message := objectValue(obj["message"]); message != nil {
		if usage := objectValue(message["usage"]); usage != nil {
			applyProbeUsageObject(dst, usage)
		}
	}
	if meta := objectValue(obj["usageMetadata"]); meta != nil {
		applyGeminiProbeUsage(dst, meta)
	}
}

// applyProbeUsageObject 解析单个 usage 对象（OpenAI 或 Anthropic 口径）。
func applyProbeUsageObject(dst *ProbeUsageTokens, usage map[string]any) {
	if dst == nil || len(usage) == 0 {
		return
	}
	// Anthropic：缓存分列，input_tokens 不含缓存。
	if hasObjectKey(usage, "cache_creation_input_tokens") || hasObjectKey(usage, "cache_read_input_tokens") {
		cacheRead := intValue(usage["cache_read_input_tokens"])
		if cacheRead == 0 {
			cacheRead = intValue(usage["cached_tokens"])
		}
		dst.mergeMax(
			intValue(usage["input_tokens"]),
			intValue(usage["output_tokens"]),
			intValue(usage["cache_creation_input_tokens"]),
			cacheRead,
		)
		return
	}

	// OpenAI（Responses / Chat）：input/prompt 含缓存读取，存储口径要把缓存拆出去。
	input := intValue(usage["input_tokens"])
	if input == 0 {
		input = intValue(usage["prompt_tokens"])
	}
	output := intValue(usage["output_tokens"])
	if output == 0 {
		output = intValue(usage["completion_tokens"])
	}
	cached := intValue(usage["cached_tokens"])
	if details := objectValue(usage["input_tokens_details"]); details != nil {
		if value := intValue(details["cached_tokens"]); value > cached {
			cached = value
		}
	}
	if details := objectValue(usage["prompt_tokens_details"]); details != nil {
		if value := intValue(details["cached_tokens"]); value > cached {
			cached = value
		}
	}
	if cached > 0 {
		input -= cached
		if input < 0 {
			input = 0
		}
	}
	dst.mergeMax(input, output, 0, cached)
}

// applyGeminiProbeUsage 解析 Gemini usageMetadata。
func applyGeminiProbeUsage(dst *ProbeUsageTokens, meta map[string]any) {
	if dst == nil || len(meta) == 0 {
		return
	}
	input := intValue(meta["promptTokenCount"])
	cached := intValue(meta["cachedContentTokenCount"])
	if cached > input {
		cached = input
	}
	dst.mergeMax(input-cached, intValue(meta["candidatesTokenCount"]), 0, cached)
}

// parseProbeJSONObject 把一个 SSE data 载荷或响应体解析成对象；失败返回 nil。
func parseProbeJSONObject(raw string) map[string]any {
	if raw == "" {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil
	}
	return obj
}

func objectValue(value any) map[string]any {
	obj, _ := value.(map[string]any)
	return obj
}

func hasObjectKey(obj map[string]any, key string) bool {
	if obj == nil {
		return false
	}
	_, ok := obj[key]
	return ok
}

func intValue(value any) int {
	switch n := value.(type) {
	case float64:
		return int(n)
	case float32:
		return int(n)
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case json.Number:
		parsed, err := n.Int64()
		if err != nil {
			return 0
		}
		return int(parsed)
	default:
		return 0
	}
}
