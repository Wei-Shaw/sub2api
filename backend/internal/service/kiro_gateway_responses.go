package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
)

// ForwardAsResponses 把一次 OpenAI Responses API 请求（Codex 客户端专用，
// /backend-api/codex/responses）转发到 Kiro，并把响应以 Responses 协议形状
// 写回客户端。
//
// 入口转换照抄 AntigravityGatewayService.ForwardAsResponses 的模板
// （internal/service/antigravity_gateway_compat.go）：Responses 请求只需
// apicompat.ResponsesToAnthropicRequest 一步就能转成 Anthropic 形态（不像
// ChatCompletions 还要多经一次 apicompat.ChatCompletionsToResponses），因为
// Responses 本来就是这条转换链的中间格式。转成 Anthropic 形态之后，直接
// 复用 forwardUpstream 这同一套核心转发引擎（配额/重试/模型限流/会话全部
// 共享，不重新实现一遍）——Kiro 自己的上游协议本就是 Anthropic 形态，
// gpt-5.6-sol/terra/luna 走这条路径已经用真实账号验证过没有协议层差异。
func (s *KiroGatewayService) ForwardAsResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	_ *ParsedRequest,
) (*ForwardResult, error) {
	var request apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, s.writeKiroResponsesBadRequest(c, "Failed to parse request body")
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, s.writeKiroResponsesBadRequest(c, "model is required")
	}

	claudeRequest, err := apicompat.ResponsesToAnthropicRequest(&request)
	if err != nil {
		return nil, s.writeKiroResponsesBadRequest(c, err.Error())
	}
	claudeRequest.Stream = request.Stream

	claudeBody, err := json.Marshal(claudeRequest)
	if err != nil {
		return nil, fmt.Errorf("kiro: marshal anthropic request: %w", err)
	}

	return s.forwardUpstream(ctx, c, account, claudeBody, false, kiroOutputResponses)
}

// writeKiroResponsesBadRequest 用 Responses/OpenAI 风格的错误信封回复请求体
// 本身就解析失败的情况（还没到 forwardUpstream，无法复用其内部的模型白名单
// 错误路径）——信封形状与 AntigravityGatewayService.writeAntigravityCompatError
// 一致，两边都是给 OpenAI 系客户端读的错误响应，没有理由另起一套形状。
func (s *KiroGatewayService) writeKiroResponsesBadRequest(c *gin.Context, message string) error {
	MarkResponseCommitted(c)
	c.JSON(http.StatusBadRequest, map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "invalid_request_error",
			"param":   nil,
			"code":    nil,
		},
	})
	return fmt.Errorf("kiro: %s", message)
}
