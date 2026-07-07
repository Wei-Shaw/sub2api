package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

// openAIReasoningEffortAllowedValues 与 OpenAI Responses API 对齐的
// reasoning.effort 枚举：none/minimal/low/medium/high/xhigh。
var openAIReasoningEffortAllowedValues = map[string]struct{}{
	"none":    {},
	"minimal": {},
	"low":     {},
	"medium":  {},
	"high":    {},
	"xhigh":   {},
}

// ValidateOpenAIResponsesReasoningEffort 校验 /v1/responses 请求体里显式携带的
// reasoning.effort 枚举值，返回 (invalidValue, ok)。
//
// 非法枚举值（如拼写错误 "xhign"）属于客户端请求错误：转发只会消耗一次上游
// 调用并换回 400 invalid_value，且在网关侧被归类为 upstream error，误导排障
// （#3782）。在本地直接拒绝可以把错误留在正确的归属方。
//
// 保守边界：
//   - 字段缺失、非字符串类型不在本校验范围（维持原转发行为，语义交由上游）；
//   - 大小写变体（如 "High"）小写化后命中枚举的不拒绝，仍交由上游裁决，
//     避免对上游大小写宽容度做过强假设；只有小写化后仍不在枚举内的值
//     （任何上游都不可能接受）才本地拒绝。
func ValidateOpenAIResponsesReasoningEffort(body []byte) (invalidValue string, ok bool) {
	effort := gjson.GetBytes(body, "reasoning.effort")
	if !effort.Exists() || effort.Type != gjson.String {
		return "", true
	}
	value := effort.String()
	if _, allowed := openAIReasoningEffortAllowedValues[strings.ToLower(strings.TrimSpace(value))]; allowed {
		return "", true
	}
	return value, false
}

// OpenAIReasoningEffortInvalidValueMessage 构造与 OpenAI 上游一致的
// invalid_value 错误文案。回显值做长度截断，避免恶意超长值放大响应。
func OpenAIReasoningEffortInvalidValueMessage(invalidValue string) string {
	const maxEcho = 64
	if len(invalidValue) > maxEcho {
		invalidValue = invalidValue[:maxEcho] + "..."
	}
	return "Invalid value: '" + invalidValue + "'. Supported values are: 'none', 'minimal', 'low', 'medium', 'high', and 'xhigh'."
}
