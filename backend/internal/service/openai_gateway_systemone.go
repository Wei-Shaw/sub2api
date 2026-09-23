package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// openAISystemOneUpstreamEndpoint 是 OpenCode Zen SystemOne (Jev) 模型的上游端点。
// 对齐 https://opencode.ai/docs/zh-cn/zen/#jev：POST {base}/v1/systemone，
// Body 为 {model, state, questions}，回答经 {answers, usage} 直接返回。
// Jev 不生成文本、无流式形态，透传即可，无需协议转换。
const openAISystemOneUpstreamEndpoint = "/v1/systemone"

// ForwardSystemOne 将 SystemOne (Jev) 请求直转到 OpenCode 上游
// `{base_url}/v1/systemone`，不做协议转换。
//
// 适用场景：account.platform=opencode_go，模型为 jev-*（jev-1.13 /
// jev-1.13-free）。base 取账号 OpenAI 协议基址：Zen 账号为
// https://opencode.ai/zen/v1，Go 账号为 https://opencode.ai/zen/go/v1。
//
// 与 forwardAsRawChatCompletions 的关键差异：
//
//   - 上游 URL 拼到 /v1/systemone 而非 /v1/chat/completions
//   - 无流式分支：Jev 只返回单次 JSON（{model, answers, usage}）
//   - 不改写响应体中的 model：上游返回的是解析后的版本（如 jev-1.13.0），
//     回写原名会丢失版本信息
//   - usage 直接取顶层 usage.{input_tokens,output_tokens}，供下游计费
func (s *OpenAIGatewayService) ForwardSystemOne(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	// 1. 最小字段解析：路由与计费只需要 model。
	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if originalModel == "" {
		writeSystemOneError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in systemone request")
	}

	// 2. 模型映射（与 ForwardAsChatCompletions 同口径）。
	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	SetOpsUpstreamModel(c, upstreamModel)

	// 3. 改写出站 model（无协议转换）。
	upstreamBody := body
	if upstreamModel != originalModel {
		upstreamBody = ReplaceModelInBody(body, upstreamModel)
	}

	token, tokenKind, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("account %d missing %s credential", account.ID, tokenKind)
	}

	logger.L().Debug("openai systemone: forwarding without protocol conversion",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
	)

	// 4. 上游端点与发送：复用 CC 出站管线（OC 会话身份头、规范 UA、
	// 账号 header 覆写、代理、传输层 failover 归一均在其中）。
	targetURL, err := s.systemOneTargetURL(account)
	if err != nil {
		return nil, err
	}
	resp, err := s.sendCCUpstreamRequest(ctx, c, account, targetURL, upstreamBody, false, token, "", "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	// sendCCUpstreamRequest 内把实际端点记为 /v1/chat/completions，纠正为 systemone，
	// 供用量记录与 ops 日志使用。
	SetActualOpenAIUpstreamEndpoint(c, openAISystemOneUpstreamEndpoint)

	// 5. 上游错误：failover 判定与各格式共享的错误回写。
	if resp.StatusCode >= 400 {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if foErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, upstreamModel); foErr != nil {
			return nil, foErr
		}
		return s.handleSystemOneErrorResponse(resp, c, account, billingModel)
	}

	// 6. 成功响应：透传 + 提取 usage。
	requestID := resp.Header.Get("x-request-id")
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			writeSystemOneError(c, http.StatusBadGateway, "api_error", "Failed to read upstream response")
		}
		return nil, fmt.Errorf("read upstream systemone body: %w", err)
	}
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	observer.ObserveOpenAI(respBody, "")

	var usage OpenAIUsage
	if parsedUsage, ok := extractOpenAIUsageFromJSONBytes(respBody); ok {
		usage = parsedUsage
	}

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Writer.Header().Set("Content-Type", ct)
	} else {
		c.Writer.Header().Set("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(respBody)

	return &OpenAIForwardResult{
		RequestID:                     requestID,
		UpstreamHeaders:               resp.Header,
		Usage:                         usage,
		Model:                         originalModel,
		BillingModel:                  billingModel,
		UpstreamModel:                 upstreamModel,
		UpstreamResponseModel:         observedUpstreamResponseModel(c),
		UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		UpstreamResponseServiceTier:   observedUpstreamResponseServiceTier(c),
		UpstreamEndpoint:              openAISystemOneUpstreamEndpoint,
		Stream:                        false,
		Duration:                      time.Since(startTime),
	}, nil
}

// systemOneTargetURL 解析账号的 SystemOne 上游端点。
func (s *OpenAIGatewayService) systemOneTargetURL(account *Account) (string, error) {
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		baseURL = DefaultOpenCodeZenBaseURL
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
	}
	return buildOpenAISystemOneURL(validatedURL), nil
}

// buildOpenAISystemOneURL 拼接上游 SystemOne 端点 URL。
//
//   - base 已是 /systemone：原样返回
//   - base 以 /v1 结尾：追加 /systemone
//   - 其他情况：追加 /v1/systemone
//
// 与 buildOpenAIChatCompletionsURL 是姐妹函数。
func buildOpenAISystemOneURL(base string) string {
	return buildOpenAIEndpointURL(base, openAISystemOneUpstreamEndpoint)
}

// handleSystemOneErrorResponse 读取上游错误并以 OpenAI 兼容错误格式返回。
func (s *OpenAIGatewayService) handleSystemOneErrorResponse(
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestedModel ...string,
) (*OpenAIForwardResult, error) {
	return s.handleCompatErrorResponse(resp, c, account, writeSystemOneError, requestedModel...)
}

// writeSystemOneError 以 OpenAI 兼容错误格式回写 SystemOne 错误。
func writeSystemOneError(c *gin.Context, statusCode int, errType, message string) {
	MarkResponseCommitted(c)
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}
