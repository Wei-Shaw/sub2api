package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	openAICapabilityNotAllowedCode   = "capability_not_allowed"
	openAICapabilityNotAvailableCode = "capability_not_available"
)

// capabilityNotAllowedMessage 是分组白名单拒绝的客户端消息（HTTP 与 WS 共用）。
func capabilityNotAllowedMessage(blocked string) string {
	return fmt.Sprintf("Capability %q is not allowed for this group", blocked)
}

// capabilityUnavailableMessage 是分组内没有具备能力账号时的客户端消息。
func capabilityUnavailableMessage(req service.OpenAIAccountRequirement) string {
	names := req.Names()
	switch len(names) {
	case 0:
		return "No account in this group supports the requested capability"
	case 1:
		return fmt.Sprintf("No account in this group supports capability %q", names[0])
	default:
		quoted := make([]string, 0, len(names))
		for _, name := range names {
			quoted = append(quoted, fmt.Sprintf("%q", name))
		}
		return "No account in this group supports capabilities " + strings.Join(quoted, " and ")
	}
}

// markOpenAICapabilityNotAllowed 标记白名单拒绝为本地业务限制与 ingress 拒绝原因，供运维统计。
func markOpenAICapabilityNotAllowed(c *gin.Context) {
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalPolicyDenied)
	middleware2.MarkIngressRejected(c, middleware2.IngressRejectCapabilityNotAllowed)
}

// rejectOpenAICapabilityNotAllowed 以 OpenAI 错误格式返回 404 capability_not_allowed
// （Responses / Chat Completions 入口）。
func (h *OpenAIGatewayHandler) rejectOpenAICapabilityNotAllowed(c *gin.Context, blocked string, streamStarted bool) {
	markOpenAICapabilityNotAllowed(c)
	h.handleStreamingAwareErrorWithCode(c, http.StatusNotFound, "invalid_request_error", openAICapabilityNotAllowedCode, capabilityNotAllowedMessage(blocked), streamStarted, false)
}

// rejectAnthropicCapabilityNotAllowed 以 Anthropic 错误格式返回 404（Messages 入口）。
func (h *OpenAIGatewayHandler) rejectAnthropicCapabilityNotAllowed(c *gin.Context, blocked string, streamStarted bool) {
	markOpenAICapabilityNotAllowed(c)
	h.anthropicStreamingAwareError(c, http.StatusNotFound, "not_found_error", capabilityNotAllowedMessage(blocked)+" ("+openAICapabilityNotAllowedCode+")", streamStarted)
}

// openAICapabilityUnavailable 识别选号层的类型化错误：分组持久池内没有任何账号满足能力需求。
func openAICapabilityUnavailable(err error) (*service.OpenAICapabilityUnavailableError, bool) {
	var capErr *service.OpenAICapabilityUnavailableError
	if errors.As(err, &capErr) && capErr != nil {
		return capErr, true
	}
	return nil, false
}

// rejectOpenAICapabilityUnavailable 以 OpenAI 错误格式返回 404 capability_not_available。
// 能力缺失是权益或配置问题，归为配置类不可用（与 model_not_found 同类），不计入路由容量不足。
func (h *OpenAIGatewayHandler) rejectOpenAICapabilityUnavailable(c *gin.Context, capErr *service.OpenAICapabilityUnavailableError, streamStarted bool) {
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalModelConfiguration)
	h.handleStreamingAwareErrorWithCode(c, http.StatusNotFound, "invalid_request_error", openAICapabilityNotAvailableCode, capabilityUnavailableMessage(capErr.Requirement), streamStarted, false)
}

// rejectAnthropicCapabilityUnavailable 以 Anthropic 错误格式返回 404（Messages 入口）。
func (h *OpenAIGatewayHandler) rejectAnthropicCapabilityUnavailable(c *gin.Context, capErr *service.OpenAICapabilityUnavailableError, streamStarted bool) {
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalModelConfiguration)
	h.anthropicStreamingAwareError(c, http.StatusNotFound, "not_found_error", capabilityUnavailableMessage(capErr.Requirement)+" ("+openAICapabilityNotAvailableCode+")", streamStarted)
}

// writeOpenAICapabilityUnavailableWSError 在 WS 首帧选号失败时先向客户端写一条 Responses 风格
// 的 error 事件（连接已升级，无法再返回 HTTP 状态码），调用方随后以策略违规关闭连接。
func writeOpenAICapabilityUnavailableWSError(ctx context.Context, conn *coderws.Conn, capErr *service.OpenAICapabilityUnavailableError) {
	if conn == nil || capErr == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	message := capabilityUnavailableMessage(capErr.Requirement)
	payload, err := json.Marshal(gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "invalid_request_error",
			"code":    openAICapabilityNotAvailableCode,
			"message": message,
		},
	})
	if err != nil {
		payload = []byte(`{"type":"error","error":{"type":"invalid_request_error","code":"capability_not_available","message":"No account in this group supports the requested capability"}}`)
	}
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = conn.Write(writeCtx, coderws.MessageText, payload)
}
