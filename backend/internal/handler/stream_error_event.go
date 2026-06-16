package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// responsesFailedError 对齐 OpenAI Responses 协议 error 子对象。
type responsesFailedError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// responsesFailedBody 对齐 apicompat.makeResponsesCompletedEvent 输出的 response 子对象字段集。
// Output 用空 slice（不是 nil）确保 marshal 为 `[]` 而非 `null`。
type responsesFailedBody struct {
	ID     string               `json:"id"`
	Object string               `json:"object"`
	Model  string               `json:"model,omitempty"`
	Status string               `json:"status"`
	Output []any                `json:"output"`
	Error  responsesFailedError `json:"error"`
}

// responsesFailedEvent 是写入 SSE data 行的顶层结构。
// 故意不带 sequence_number：spec 标记可选，且本函数被调用时无法可靠拿到 last seq。
type responsesFailedEvent struct {
	Type     string              `json:"type"`
	Response responsesFailedBody `json:"response"`
}

// writeResponsesFailedSSE emits a `response.failed` SSE event in the OpenAI
// Responses API protocol after the stream has already started.
//
// 必要性：一旦 SSE 头和任意数据（例如等待槽位时的 ping comment）已经 flush，
// HTTP 200 状态码就被固化。此后若网关需要回报错误，只能继续通过 SSE 事件传达。
// 通用的 `event: error` 帧不是 Responses 协议规定的终止事件，
// Codex CLI 等严格 SDK 会因为没收到 `response.completed/failed/incomplete/cancelled`
// 而抛出 "stream closed before response.completed"。
//
// 字段集对齐 apicompat.makeResponsesCompletedEvent：id/object/model/status/output/error。
// 故意不写 sequence_number：本函数被调用时无法可靠拿到当前流的 last sequence，
// 而 OpenAI spec 将 sequence_number 设为可选；省略避免破坏单调性约束。
//
// 返回 true 表示已尝试 SSE 写出（不论 Write 是否成功，caller 都应直接 return）。
// 返回 false 表示 writer 不支持 Flusher，无法以 SSE 形式回报错误；
// 此时 caller 也无法回退到 JSON（HTTP 200 已固化），通常意味着连接已经损坏，
// 应当让请求处理函数 return，由上层关闭连接。
func writeResponsesFailedSSE(c *gin.Context, errType, message string) bool {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return false
	}

	payload, err := json.Marshal(responsesFailedEvent{
		Type: "response.failed",
		Response: responsesFailedBody{
			ID:     synthesizeResponseID(c),
			Object: "response",
			Model:  requestModel(c),
			Status: "failed",
			Output: []any{},
			Error: responsesFailedError{
				Code:    mapResponsesErrorCode(errType),
				Message: message,
			},
		},
	})
	if err != nil {
		_ = c.Error(err)
		return true
	}

	if _, err := fmt.Fprintf(c.Writer, "event: response.failed\ndata: %s\n\n", payload); err != nil {
		_ = c.Error(err)
		return true
	}
	flusher.Flush()
	return true
}

// inboundIsResponses 判断当前请求是否落在任何 /responses 路由上。
//
// 不能直接用 GetInboundEndpoint(c) == EndpointResponses 比较，因为
// NormalizeInboundEndpoint 只识别包含 "/v1/responses" 子串的路径；
// 项目里实际注册了多组路由（gateway_v1、top-level bare、codex direct），
// 其中 r.POST("/responses", ...) 和 codexDirect.POST("/responses", ...)
// 的 c.FullPath() 不含 "/v1/" 前缀，会被归一化为原始路径，
// 导致协议合规终止事件没法发出去。
//
// 这里用 FullPath 的后缀判断，覆盖所有变体：
//   - /v1/responses
//   - /v1/responses/compact
//   - /responses
//   - /responses/compact
//   - /backend-api/codex/responses
//   - /backend-api/codex/responses/compact
func inboundIsResponses(c *gin.Context) bool {
	if c == nil {
		return false
	}
	p := strings.TrimRight(c.FullPath(), "/")
	if p == "" && c.Request != nil && c.Request.URL != nil {
		p = strings.TrimRight(c.Request.URL.Path, "/")
	}
	if p == "" {
		return false
	}
	return strings.HasSuffix(p, "/responses") || strings.Contains(p, "/responses/")
}

// synthesizeResponseID 为合成的 response.failed 事件生成一个稳定的 id。
// 优先复用 server 端生成的 request_id（存在 request.Context 里，由 request_logger 写入），
// 以便客户端报错能与 server 日志关联；缺失时回退 uuid。
func synthesizeResponseID(c *gin.Context) string {
	if c != nil && c.Request != nil {
		if rid, ok := c.Request.Context().Value(ctxkey.RequestID).(string); ok {
			if rid = strings.TrimSpace(rid); rid != "" {
				return "resp_" + strings.ReplaceAll(rid, "-", "")
			}
		}
	}
	return "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

// requestModel 取当前请求的 inbound model（由 setOpsRequestContext 写入）。
// 缺失时返回 ""；caller 据此决定是否忽略该字段。
func requestModel(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get(opsModelKey); ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// mapResponsesErrorCode 把内部 errType 映射为 Responses 协议常见的 error.code。
// 无明确映射时原样返回，保证至少可读。
func mapResponsesErrorCode(errType string) string {
	switch errType {
	case "rate_limit_error":
		return "rate_limit_exceeded"
	case "invalid_request_error":
		return "invalid_request"
	case "permission_error":
		return "permission_denied"
	case "authentication_error":
		return "authentication_failed"
	case "upstream_error":
		return "upstream_error"
	case "server_error", "api_error", "":
		return "server_error"
	default:
		return errType
	}
}
