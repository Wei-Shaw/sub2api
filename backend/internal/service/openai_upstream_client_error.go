package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// openAIUpstreamClientErrorFallbackType 是上游没给 error.type 时的兜底值。
// 与 handleCompatErrorResponse 对 400 的取值保持一致。
const openAIUpstreamClientErrorFallbackType = "invalid_request_error"

// openAIUpstreamClientErrorFallbackMessage 是上游连 message 都没给时的兜底文案。
// 仍然比 "Upstream request failed" 明确：它说明拒绝来自请求本身，而不是链路故障。
const openAIUpstreamClientErrorFallbackMessage = "Upstream rejected the request"

// isOpenAIDeterministicClientError 判断上游状态码是否表示「请求本身非法」。
//
// 只认 400：同一份请求体换任何账号、重试多少次都会得到同样的结果。
//   - 401/402/403 是网关运营方的凭据/账单问题，继续包成 502，不向客户端暴露上游账号状态。
//   - 404/405 既可能是模型不存在，也可能是上游 base_url 配错，同属运营方问题，保持现状。
//   - 429 已有独立分支映射成 429。
//   - 413 在更上面就按 request-body-too-large 走 failover 了。
func isOpenAIDeterministicClientError(statusCode int) bool {
	return statusCode == http.StatusBadRequest
}

// writeOpenAIUpstreamClientError 以 OpenAI 错误体形状回写确定性客户端错误。
//
// 保留上游的 type/code/param：客户端靠 param 定位是哪个字段非法（上游会给出形如
// input[8].tools[1].tools[2].parameters 的路径），靠 code 判断是否值得重试。归一成
// {type:"upstream_error", message:"Upstream request failed"} 会把这些信息全部抹掉。
//
// upstreamMsg 由调用方传入，调用方已做过 sanitizeUpstreamErrorMessage 与
// redactAgentIdentitySensitiveBody；这里不重复清洗，也不回落读取原始 body 的
// message，避免绕开那两道脱敏。
func writeOpenAIUpstreamClientError(c *gin.Context, statusCode int, body []byte, upstreamMsg string) {
	c.JSON(statusCode, gin.H{"error": openAIUpstreamErrorPayload(
		body, upstreamMsg, openAIUpstreamClientErrorFallbackType, openAIUpstreamClientErrorFallbackMessage,
	)})
}

// writeOpenAIPassthroughRuleBodyError 回写命中 passthrough_body=true 规则的上游错误。
//
// 规则的意图是让上游错误原样到达客户端，因此与确定性 400 分支一样保留 type/code/param
// （例如 previous_response_not_found 依赖 code 才能被客户端识别为"需要重发完整历史"）；
// 上游缺 type/message 时沿用规则路径原有的 upstream_error 与规则消息兜底。
// upstreamMsg 约定同 writeOpenAIUpstreamClientError：调用方已脱敏。
func writeOpenAIPassthroughRuleBodyError(c *gin.Context, statusCode int, body []byte, upstreamMsg, fallbackMsg string) {
	c.JSON(statusCode, gin.H{"error": openAIUpstreamErrorPayload(body, upstreamMsg, "upstream_error", fallbackMsg)})
}

func openAIUpstreamErrorPayload(body []byte, upstreamMsg, fallbackType, fallbackMsg string) gin.H {
	errorPayload := gin.H{"type": fallbackType}
	if errType := strings.TrimSpace(gjson.GetBytes(body, "error.type").String()); errType != "" {
		errorPayload["type"] = errType
	}
	if code := strings.TrimSpace(extractUpstreamErrorCode(body)); code != "" {
		errorPayload["code"] = code
	}
	if param := strings.TrimSpace(gjson.GetBytes(body, "error.param").String()); param != "" {
		errorPayload["param"] = param
	}
	message := strings.TrimSpace(upstreamMsg)
	if message == "" {
		message = fallbackMsg
	}
	errorPayload["message"] = message
	return errorPayload
}

// WriteOpenAIUpstreamClientError preserves a structured deterministic upstream
// client error when the handler has exhausted all eligible accounts.
func WriteOpenAIUpstreamClientError(c *gin.Context, statusCode int, body []byte, upstreamMsg string) {
	writeOpenAIUpstreamClientError(c, statusCode, body, upstreamMsg)
}

// WriteOpenAIPassthroughRuleBodyError exposes writeOpenAIPassthroughRuleBodyError
// for the handler's failover-exhausted path.
func WriteOpenAIPassthroughRuleBodyError(c *gin.Context, statusCode int, body []byte, upstreamMsg, fallbackMsg string) {
	writeOpenAIPassthroughRuleBodyError(c, statusCode, body, upstreamMsg, fallbackMsg)
}

// ExtractUpstreamErrorCode returns the upstream error.code, if any.
func ExtractUpstreamErrorCode(body []byte) string {
	return strings.TrimSpace(extractUpstreamErrorCode(body))
}
