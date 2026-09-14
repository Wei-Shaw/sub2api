// devin_gateway_handler.go 处理 Devin 平台分组的三种入站协议：
// Anthropic /v1/messages、OpenAI /v1/chat/completions、OpenAI /v1/responses。
// 每种协议经 pkg/devin/api 编解码为 llm.RequestMessages / ResponseEvent，
// 上游走 pkg/devin/adapter 的 Connect-RPC 通道。
//
// 编排语义对齐 GatewayHandler：粘性会话选号、并发槽位与等待队列、
// failover 换号、后扣计费。差异点：上游协议翻译是固定三件套，
// 不存在按账号分协议的 channel 映射。
package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	devinanthropic "github.com/Wei-Shaw/sub2api/internal/pkg/devin/api/anthropic/messages"
	devincommon "github.com/Wei-Shaw/sub2api/internal/pkg/devin/api/common"
	devinchat "github.com/Wei-Shaw/sub2api/internal/pkg/devin/api/openai/chat"
	devinresponses "github.com/Wei-Shaw/sub2api/internal/pkg/devin/api/openai/responses"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// devinProtocol 标识入站协议，决定编解码与错误形态。
type devinProtocol int

const (
	devinProtocolMessages devinProtocol = iota
	devinProtocolChatCompletions
	devinProtocolResponses
)

// devinAdapted 是解码后的统一请求视图。
type devinAdapted struct {
	context llm.RequestMessages
	stream  bool
	// includeUsage 仅 chat 协议：stream_options.include_usage。
	includeUsage bool
}

// devinStreamEncoder 抽象三协议的事件编码器。
type devinStreamEncoder interface {
	Encode(event llm.ResponseEvent) ([]devincommon.SSEEvent, error)
}

// DevinGatewayHandler 处理 Devin 平台分组的网关请求。
type DevinGatewayHandler struct {
	devinGateway          *service.DevinGatewayService
	gatewayService        *service.GatewayService
	rateLimitService      *service.RateLimitService
	billingCacheService   *service.BillingCacheService
	apiKeyService         *service.APIKeyService
	usageRecordWorkerPool *service.UsageRecordWorkerPool
	concurrencyHelper     *ConcurrencyHelper
	maxAccountSwitches    int
	cfg                   *config.Config
}

// NewDevinGatewayHandler 创建 handler。
func NewDevinGatewayHandler(
	devinGateway *service.DevinGatewayService,
	gatewayService *service.GatewayService,
	rateLimitService *service.RateLimitService,
	concurrencyService *service.ConcurrencyService,
	billingCacheService *service.BillingCacheService,
	apiKeyService *service.APIKeyService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	cfg *config.Config,
) *DevinGatewayHandler {
	maxAccountSwitches := 10
	if cfg != nil && cfg.Gateway.MaxAccountSwitches > 0 {
		maxAccountSwitches = cfg.Gateway.MaxAccountSwitches
	}
	return &DevinGatewayHandler{
		devinGateway:          devinGateway,
		gatewayService:        gatewayService,
		rateLimitService:      rateLimitService,
		billingCacheService:   billingCacheService,
		apiKeyService:         apiKeyService,
		usageRecordWorkerPool: usageRecordWorkerPool,
		concurrencyHelper:     NewConcurrencyHelper(concurrencyService, SSEPingFormatClaude, 0),
		maxAccountSwitches:    maxAccountSwitches,
		cfg:                   cfg,
	}
}

// Messages 处理 Anthropic /v1/messages。
func (h *DevinGatewayHandler) Messages(c *gin.Context) {
	h.forward(c, devinProtocolMessages)
}

// ChatCompletions 处理 OpenAI /v1/chat/completions。
func (h *DevinGatewayHandler) ChatCompletions(c *gin.Context) {
	h.forward(c, devinProtocolChatCompletions)
}

// Responses 处理 OpenAI /v1/responses（HTTP POST；WS 走 ResponsesWebSocket）。
func (h *DevinGatewayHandler) Responses(c *gin.Context) {
	h.forward(c, devinProtocolResponses)
}

// decodeRequest 按协议解码入站 JSON。
func decodeDevinRequest(protocol devinProtocol, body []byte) (devinAdapted, error) {
	switch protocol {
	case devinProtocolMessages:
		adapted, err := devinanthropic.DecodeRequest(body)
		if err != nil {
			return devinAdapted{}, err
		}
		return devinAdapted{context: adapted.Context, stream: adapted.Options.Stream}, nil
	case devinProtocolChatCompletions:
		adapted, err := devinchat.DecodeRequest(body)
		if err != nil {
			return devinAdapted{}, err
		}
		return devinAdapted{
			context:      adapted.Context,
			stream:       adapted.Options.Stream,
			includeUsage: adapted.Options.IncludeUsage,
		}, nil
	default:
		adapted, err := devinresponses.DecodeRequest(body)
		if err != nil {
			return devinAdapted{}, err
		}
		return devinAdapted{context: adapted.Context, stream: adapted.Options.Stream}, nil
	}
}

// newDevinEncoder 按协议构造流编码器。
func newDevinEncoder(protocol devinProtocol, model string, includeUsage bool) devinStreamEncoder {
	switch protocol {
	case devinProtocolMessages:
		return devinanthropic.NewStreamEncoder(model)
	case devinProtocolChatCompletions:
		return devinchat.NewStreamEncoder(model, includeUsage)
	default:
		return devinresponses.NewStreamEncoder(model)
	}
}

// encodeDevinFinal 按协议把最终消息编码为非流式响应体。
func encodeDevinFinal(protocol devinProtocol, message *llm.AssistantMessage) ([]byte, error) {
	switch protocol {
	case devinProtocolMessages:
		return devinanthropic.EncodeResponse(message)
	case devinProtocolChatCompletions:
		return devinchat.EncodeResponse(message)
	default:
		return devinresponses.EncodeResponse(message)
	}
}

// devinSessionHash 计算粘性会话 hash：显式 SessionKey 优先，
// 否则用模型+首条用户文本的短哈希保证同会话落同账号
// （Devin cascade/trajectory 身份由 SessionKey 派生，粘性键保持一致即可）。
func devinSessionHash(request *llm.RequestMessages) string {
	if request == nil {
		return ""
	}
	if key := strings.TrimSpace(request.SessionKey); key != "" {
		return "devin:" + key
	}
	h := sha256.New()
	h.Write([]byte(request.Model))
	h.Write([]byte{0})
	h.Write([]byte(request.SystemPrompt))
	if len(request.Messages) > 0 {
		// 只取首条消息，与 pkg/devin/adapter 的会话派生一致。
		for _, content := range devinMessageContents(request.Messages[0]) {
			if text, ok := content.(llm.TextContent); ok {
				h.Write([]byte{0})
				h.Write([]byte(text.Text))
			}
		}
	}
	return "devin:auto:" + hex.EncodeToString(h.Sum(nil))[:32]
}

// devinMessageContents 提取消息的内容块（Message 是接口类型）。
func devinMessageContents(message llm.Message) []llm.Content {
	switch m := message.(type) {
	case llm.UserMessage:
		return m.Content
	case *llm.UserMessage:
		return m.Content
	case llm.AssistantMessage:
		return m.Content
	case *llm.AssistantMessage:
		return m.Content
	case llm.ToolResultMessage:
		return m.Content
	case *llm.ToolResultMessage:
		return m.Content
	default:
		return nil
	}
}

// forward 是三协议共用的转发主流程。
func (h *DevinGatewayHandler) forward(c *gin.Context, protocol devinProtocol) {
	reqLog := logger.L().With(zap.String("component", "handler.gateway.devin"))

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		h.devinError(c, protocol, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
		return
	}
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.devinError(c, protocol, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	adapted, err := decodeDevinRequest(protocol, body)
	if err != nil {
		h.devinError(c, protocol, http.StatusBadRequest, "invalid_request_error", "Invalid request: "+err.Error())
		return
	}
	reqModel := adapted.context.Model
	reqStream := adapted.stream
	streamStarted := false

	// 余额/订阅资格检查（与其他平台一致，在拿槽位之前）。
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
		}
		h.devinError(c, protocol, status, code, message)
		return
	}

	// 用户并发槽位。
	userReleaseFunc, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, reqStream, &streamStarted)
	if err != nil {
		h.devinError(c, protocol, http.StatusTooManyRequests, "rate_limit_error", "Too many concurrent requests")
		return
	}
	userReleaseFunc = wrapReleaseOnDone(c.Request.Context(), userReleaseFunc)
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	sessionKey := devinSessionHash(&adapted.context)
	fs := NewFailoverState(h.maxAccountSwitches, false)

	for {
		selection, selErr := h.gatewayService.SelectAccountWithLoadAwareness(
			c.Request.Context(), apiKey.GroupID, sessionKey, reqModel,
			fs.FailedAccountIDs, "", subject.UserID)
		if selErr != nil {
			if len(fs.FailedAccountIDs) == 0 {
				markOpsRoutingCapacityLimitedIfNoAvailable(c, selErr)
				reqLog.Warn("devin.select_account_no_available",
					zap.String("model", reqModel),
					zap.Int64p("group_id", apiKey.GroupID),
					zap.Error(selErr))
				h.devinError(c, protocol, http.StatusServiceUnavailable, "api_error",
					"No available accounts: "+selErr.Error())
				return
			}
			switch fs.HandleSelectionExhausted(c.Request.Context()) {
			case FailoverContinue:
				continue
			case FailoverCanceled:
				return
			default:
				h.devinFailoverExhausted(c, protocol, fs, streamStarted)
				return
			}
		}
		account := selection.Account

		// 账号级 model_mapping：客户端模型 → Devin 目录模型（命中即改写上游
		// 模型；调度与计费仍用客户端请求模型 reqModel）。
		upstreamCtx := adapted.context
		if mapped, matched := account.ResolveMappedModel(upstreamCtx.Model); matched && strings.TrimSpace(mapped) != "" {
			upstreamCtx.Model = mapped
		}

		accountReleaseFunc := selection.ReleaseFunc
		if !selection.Acquired {
			if selection.WaitPlan == nil {
				markOpsRoutingCapacityLimited(c)
				h.devinError(c, protocol, http.StatusServiceUnavailable, "api_error", "No available accounts")
				return
			}
			accountReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
				c, account.ID, selection.WaitPlan.MaxConcurrency,
				selection.WaitPlan.Timeout, reqStream, &streamStarted)
			if err != nil {
				h.devinError(c, protocol, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests")
				return
			}
		}
		accountReleaseFunc = wrapReleaseOnDone(c.Request.Context(), accountReleaseFunc)

		// 打开上游流。
		upstream, streamErr := h.devinGateway.Stream(c.Request.Context(), account, upstreamCtx)
		if streamErr != nil {
			if accountReleaseFunc != nil {
				accountReleaseFunc()
			}
			failoverErr := service.DevinFailoverError(streamErr)
			h.rateLimitService.HandleUpstreamError(c.Request.Context(), account, failoverErr.StatusCode, nil, failoverErr.ResponseBody, reqModel)
			switch fs.HandleFailoverError(c.Request.Context(), h.gatewayService, account.ID, service.PlatformDevin, 0, failoverErr) {
			case FailoverContinue:
				continue
			case FailoverCanceled:
				return
			default:
				h.devinFailoverExhausted(c, protocol, fs, streamStarted)
				return
			}
		}

		// 泵事件流 → 客户端。
		result, pumpErr := h.pumpDevinStream(c, protocol, upstream, adapted, reqStream, &streamStarted)
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
		if pumpErr != nil {
			// 流已开始/结束后的失败不再换号——下游已收到字节，
			// 错误以协议内 error 事件下发（pump 内部已写）。
			if streamStarted {
				return
			}
			failoverErr := service.DevinFailoverError(pumpErr)
			h.rateLimitService.HandleUpstreamError(c.Request.Context(), account, failoverErr.StatusCode, nil, failoverErr.ResponseBody, reqModel)
			switch fs.HandleFailoverError(c.Request.Context(), h.gatewayService, account.ID, service.PlatformDevin, 0, failoverErr) {
			case FailoverContinue:
				continue
			case FailoverCanceled:
				return
			default:
				h.devinFailoverExhausted(c, protocol, fs, streamStarted)
				return
			}
		}

		// 成功后扣费（worker 池异步）。
		h.submitDevinUsage(c, protocol, apiKey, subscription, account, reqModel, result, body)
		return
	}
}

// pumpDevinStream 把上游事件泵到客户端；返回最终消息与泵错误。
// 非流式请求聚合到 done/error 后写一次性 JSON。
func (h *DevinGatewayHandler) pumpDevinStream(
	c *gin.Context,
	protocol devinProtocol,
	upstream llm.ResponseStream,
	adapted devinAdapted,
	reqStream bool,
	streamStarted *bool,
) (*llm.AssistantMessage, error) {
	ctx := c.Request.Context()
	startedAt := time.Now()
	var firstTokenMs *int
	// adapted.context.Model 仍是客户端请求模型（上游模型走 upstreamCtx 副本），
	// 编码器直接用它回填响应中的 model 字段。
	encoder := newDevinEncoder(protocol, adapted.context.Model, adapted.includeUsage)

	if reqStream {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.Flush()
		*streamStarted = true
	}

	var final *llm.AssistantMessage
	for {
		event, err := upstream.Recv(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return final, err
		}
		if firstTokenMs == nil &&
			(event.Type == llm.ResponseEventTextDelta ||
				event.Type == llm.ResponseEventThinkingDelta ||
				event.Type == llm.ResponseEventToolCallDelta) {
			ms := int(time.Since(startedAt).Milliseconds())
			firstTokenMs = &ms
		}
		if event.Type == llm.ResponseEventDone && event.Message != nil {
			final = event.Message
		}
		if event.Type == llm.ResponseEventError && event.Error != nil {
			final = event.Error
		}

		if !reqStream {
			continue // 非流式：只聚合，不逐事件下发
		}
		events, encErr := encoder.Encode(event)
		if encErr != nil {
			return final, fmt.Errorf("encode %s event: %w", event.Type, encErr)
		}
		for _, sse := range events {
			if !writeDevinSSE(c, sse) {
				return final, errors.New("client disconnected")
			}
		}
		c.Writer.Flush()
	}

	if reqStream {
		return final, nil
	}
	if final == nil {
		return nil, errors.New("upstream stream ended without a final message")
	}
	if final.ErrorMessage != "" {
		return final, errors.New(final.ErrorMessage)
	}
	payload, err := encodeDevinFinal(protocol, final)
	if err != nil {
		return final, err
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	if _, err := c.Writer.Write(payload); err != nil {
		return final, err
	}
	_ = firstTokenMs // 非流式不计首字延迟到客户端，仅计入用量
	return final, nil
}

// writeDevinSSE 写一个 SSE 事件；写失败（客户端断开）返回 false。
func writeDevinSSE(c *gin.Context, event devincommon.SSEEvent) bool {
	if event.Name != "" && event.Name != "[DONE]" {
		if _, err := fmt.Fprintf(c.Writer, "event: %s\n", event.Name); err != nil {
			return false
		}
	}
	if _, err := c.Writer.Write([]byte("data: ")); err != nil {
		return false
	}
	if _, err := c.Writer.Write(event.Data); err != nil {
		return false
	}
	if _, err := c.Writer.Write([]byte("\n\n")); err != nil {
		return false
	}
	return true
}

// submitDevinUsage 把最终消息折叠为 ForwardResult 交给统一计费链。
func (h *DevinGatewayHandler) submitDevinUsage(
	c *gin.Context,
	protocol devinProtocol,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	account *service.Account,
	reqModel string,
	message *llm.AssistantMessage,
	requestBody []byte,
) {
	if message == nil {
		return
	}
	result := &service.ForwardResult{
		RequestID: message.UpstreamRequestID,
		Model:     reqModel,
		Stream:    true,
		Usage: service.ClaudeUsage{
			InputTokens:              int(message.Usage.Input),
			OutputTokens:             int(message.Usage.Output),
			CacheCreationInputTokens: int(message.Usage.CacheWrite),
			CacheReadInputTokens:     int(message.Usage.CacheRead),
		},
	}
	// 上游回报的实际模型（actual_model_uid）≠ 请求模型时记入 UpstreamModel，
	// usage_log 据此显示真实结算模型。
	if upstreamModel := strings.TrimSpace(message.ResponseModel); upstreamModel != "" && upstreamModel != reqModel {
		result.UpstreamModel = upstreamModel
	}
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	payloadHash := service.HashUsageRequestPayload(requestBody)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	sessionID := service.ExtractClientSessionID(c)

	submit := func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
			Result:             result,
			QuotaPlatform:      quotaPlatform,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			SessionID:          sessionID,
			RequestPayloadHash: payloadHash,
			APIKeyService:      h.apiKeyService,
		}); err != nil {
			logger.L().With(zap.String("component", "handler.gateway.devin")).Error(
				"devin.record_usage_failed",
				zap.Int64("account_id", account.ID),
				zap.String("model", reqModel),
				zap.Error(err))
		}
	}
	if h.usageRecordWorkerPool != nil {
		if mode := h.usageRecordWorkerPool.Submit(submit); !mode.Dropped() {
			return
		}
	}
	submit(context.Background())
}

// devinFailoverExhausted 在换号耗尽时按协议写错误。
func (h *DevinGatewayHandler) devinFailoverExhausted(c *gin.Context, protocol devinProtocol, fs *FailoverState, streamStarted bool) {
	if fs.LastFailoverErr != nil {
		status := fs.LastFailoverErr.ClientStatusCode
		if status == 0 {
			status = http.StatusBadGateway
		}
		message := fs.LastFailoverErr.ClientMessage
		if message == "" {
			message = fs.LastFailoverErr.Error()
		}
		h.devinErrorStreamingAware(c, protocol, status, message, streamStarted)
		return
	}
	h.devinErrorStreamingAware(c, protocol, http.StatusBadGateway, "upstream error", streamStarted)
}

// devinErrorStreamingAware 流已开始时发协议内 error 事件，否则 JSON 错误。
func (h *DevinGatewayHandler) devinErrorStreamingAware(c *gin.Context, protocol devinProtocol, status int, message string, streamStarted bool) {
	if streamStarted {
		event := llm.ResponseEvent{
			Type:  llm.ResponseEventError,
			Error: &llm.AssistantMessage{ErrorMessage: message},
		}
		encoder := newDevinEncoder(protocol, "", false)
		if events, err := encoder.Encode(event); err == nil {
			for _, sse := range events {
				writeDevinSSE(c, sse)
			}
		}
		c.Writer.Flush()
		return
	}
	h.devinError(c, protocol, status, devinErrorType(protocol, status), message)
}

// devinErrorType 按协议+状态码给出 error.type。
func devinErrorType(protocol devinProtocol, status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return "authentication_error"
	case status == http.StatusTooManyRequests:
		return "rate_limit_error"
	case status == http.StatusBadRequest:
		return "invalid_request_error"
	case status >= 500:
		if protocol == devinProtocolMessages {
			return "api_error"
		}
		return "server_error"
	default:
		if protocol == devinProtocolMessages {
			return "api_error"
		}
		return "server_error"
	}
}

// devinError 写协议形态的 JSON 错误（非流式路径）。
func (h *DevinGatewayHandler) devinError(c *gin.Context, protocol devinProtocol, status int, errType, message string) {
	if protocol == devinProtocolMessages {
		c.JSON(status, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    errType,
				"message": message,
			},
		})
		return
	}
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errType,
		},
	})
}

// CountTokens 处理 POST /v1/messages/count_tokens（Devin 分组）：
// 上游无 token 计数端点，本地估算（与 Grok 同一估算器）。
func (h *DevinGatewayHandler) CountTokens(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || len(body) == 0 {
		h.devinError(c, devinProtocolMessages, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}
	estimated, err := service.EstimateGrokCountTokens(body)
	if err != nil {
		h.devinError(c, devinProtocolMessages, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}
	c.JSON(http.StatusOK, gin.H{"input_tokens": estimated})
}

// ResponsesWebSocket 处理 GET /v1/responses 的 WebSocket 传输形态，
// 实现在 devin_gateway_ws.go。

// Models 处理 GET /v1/models（Devin 分组）：拉取上游分组目录。
// 选任一可调度 devin 账号；无账号时返回空目录而非报错——目录拉取
// 只是模型展示，不应被当作推理失败。
func (h *DevinGatewayHandler) Models(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		h.devinError(c, devinProtocolResponses, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
		return
	}
	selection, err := h.gatewayService.SelectAccountWithLoadAwareness(
		c.Request.Context(), apiKey.GroupID, "", "", nil, "", 0)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": []any{}})
		return
	}
	if selection.ReleaseFunc != nil {
		defer selection.ReleaseFunc()
	}
	groups, err := h.devinGateway.ListModels(c.Request.Context(), selection.Account)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": []any{}})
		return
	}
	// 只暴露分组 id（swe-2/claude-fable-5-1/…）：thinking 档位经
	// effort 参数或 "model:level" 语法解析，不把档位 uid 铺平
	// 成独立模型（与插件 catalog 语义一致）。
	data := make([]any, 0, len(groups))
	seen := make(map[string]bool)
	for _, group := range groups {
		if group.ID == "" || seen[group.ID] {
			continue
		}
		seen[group.ID] = true
		data = append(data, gin.H{
			"id":       group.ID,
			"object":   "model",
			"created":  0,
			"owned_by": "devin",
			"name":     group.Name,
		})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}
