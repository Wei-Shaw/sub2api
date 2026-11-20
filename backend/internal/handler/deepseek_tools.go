package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (h *OpenAIGatewayHandler) bindDeepseekFileReferences(c *gin.Context, apiKey *service.APIKey, protocol string, body []byte, write func(*gin.Context, int, string, string)) bool {
	ctx := c.Request.Context()
	next, err := h.gatewayService.WithDeepseekFileReferences(ctx, openAICompatibleRequestPlatform(ctx, apiKey), protocol, apiKey.UserID, body)
	if err != nil {
		e := infraerrors.FromError(err)
		write(c, int(e.Code), e.Reason, e.Message)
		return false
	}
	if next != ctx {
		c.Request = c.Request.WithContext(next)
	}
	return true
}

// Only the five standalone tools reserve balance. Files, including opt-in
// document parsing, have no gateway billing units.
func deepseekToolsInflightEstimate(endpoint string) service.InflightEstimateRequest {
	var units int
	switch endpoint {
	case "search", "fetch":
		units = 1
	case "ocr":
		units = 10 // pages
	case "asr":
		units = 600 // seconds
	case "tts":
		units = 2000 // billing characters
	default:
		return service.InflightEstimateRequest{}
	}
	return service.InflightEstimateRequest{DeepseekTool: &service.DeepseekToolsUsage{Endpoint: endpoint, Units: units}}
}

// deepseekToolsEndpointHasPrompt 报告端点请求体是否携带会被上游当作模型输入的
// 用户文本（需要内容审计）。
func deepseekToolsEndpointHasPrompt(endpoint string) bool {
	switch endpoint {
	case "search", "fetch", "tts":
		return true
	default:
		return false
	}
}

// extractDeepseekToolsPromptText 提取请求体里的主文本字段供内容审计归一化。
func extractDeepseekToolsPromptText(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"query", "q", "text", "input", "prompt", "url"} {
		if v, ok := payload[key]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// DeepseekTools 是 DeepSeek 独立工具接口的统一入口，endpoint 覆盖九端点：
// search/fetch/ocr/asr/tts/files_create/files_list/files_get/files_delete
// （files_get/files_delete 由路由层拼上 file id 后缀）。
// 上游 HTTP 响应（含错误）已由 service 写回客户端；handler 只负责门禁、
// 计费资格/在途预留、选号 failover 与落账。
func (h *OpenAIGatewayHandler) DeepseekTools(c *gin.Context, endpoint string) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	anthropicFiles := strings.HasPrefix(endpoint, "files_") && service.IsDeepseekFilesAnthropicRequest(c)
	write := h.errorResponse
	if anthropicFiles {
		write = h.anthropicErrorResponse
	}
	// composite 组放行：工具请求不带 model，无法经 compositeTarget 中间件解析，
	// 交给调度器按 platformOverride=deepseek 选号（对齐 Grok video 先例）。
	if !ok || apiKey.Group == nil || (apiKey.Group.Platform != service.PlatformDeepseek && apiKey.Group.Platform != service.PlatformComposite) {
		write(c, http.StatusNotFound, "not_found_error", "DeepSeek tools API is not supported for this platform")
		return
	}
	if !h.ensureResponsesDependencies(c, nil) {
		return
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.PlatformDeepseek); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		write(c, status, code, message)
		return
	}

	if endpoint == "files_list" {
		page, err := h.gatewayService.ListDeepseekFiles(c.Request.Context(), apiKey.UserID, c.Request.URL.RawQuery, anthropicFiles)
		if err != nil {
			e := infraerrors.FromError(err)
			write(c, int(e.Code), e.Reason, e.Message)
			return
		}
		c.JSON(http.StatusOK, page)
		return
	}

	// POST 系端点读入 body（multipart/JSON 同样字节读入，Content-Type 原样传
	// service）；GET/DELETE（files_list/files_get/files_delete）传 nil。
	body, err := readGrokVoiceGatewayBody(c)
	if err != nil {
		if errors.Is(err, errGrokVoiceBodyRequired) {
			write(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		bodyLog := requestLogger(c, "handler.openai_gateway.deepseek_tools", zap.String("endpoint", endpoint))
		if handleRequestBodyReadError(c, bodyLog, err) {
			return
		}
		status, message := requestBodyReadFailure(bodyLog, c.Request, err)
		write(c, status, "invalid_request_error", message)
		return
	}
	request, err := h.gatewayService.PrepareDeepseekToolsRequest(c.Request.Context(), c, apiKey.UserID, endpoint, body, c.GetHeader("Content-Type"))
	if err != nil {
		e := infraerrors.FromError(err)
		write(c, int(e.Code), e.Reason, e.Message)
		return
	}
	// 文本类端点（search/fetch/tts）在调用上游前做内容审计；ocr/asr 处理的是
	// 文档/音频（multipart，无模型文本提示），files_* 是纯文件管理，均不走审计。
	if deepseekToolsEndpointHasPrompt(endpoint) {
		subject, _ := middleware2.GetAuthSubjectFromContext(c)
		reqLog := requestLogger(c, "handler.openai_gateway.deepseek_tools", zap.String("endpoint", endpoint))
		auditBody := body
		if input := extractDeepseekToolsPromptText(body); input != "" {
			if b, marshalErr := json.Marshal(map[string]any{
				"messages": []map[string]any{{"role": "user", "content": input}},
			}); marshalErr == nil {
				auditBody = b
			}
		}
		if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIChat, "deepseek-tools-"+endpoint, auditBody); decision != nil && !decision.AllowNextStage {
			h.openAISecurityAuditError(c, decision)
			return
		}
	}
	// 余额模式在途预留（与计费同口径估算；计费任务扣减余额缓存后才释放）。
	if estimate := deepseekToolsInflightEstimate(endpoint); estimate.DeepseekTool != nil {
		inflightDone, inflightErr := reserveInflightBalance(c, h.billingCacheService, h.gatewayService, apiKey, subscription, estimate)
		if inflightErr != nil {
			status, code, message, retryAfter := billingErrorDetails(inflightErr)
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(retryAfter))
			}
			write(c, status, code, message)
			return
		}
		defer inflightDone()
	}

	ctx := request.SchedulingContext(c.Request.Context())
	reqLog := requestLogger(c, "handler.openai_gateway.deepseek_tools", zap.String("endpoint", endpoint))
	failed := map[int64]struct{}{}
	var last *service.UpstreamFailoverError
	for attempts := 0; attempts < 4; attempts++ {
		// 能力位复用 ChatCompletions（工具端点与对话同账号池），platform 强制 Deepseek。
		selection, _, selectErr := h.gatewayService.SelectAccountWithSchedulerForCapability(
			ctx,
			apiKey.GroupID,
			"",
			"",
			"",
			failed,
			service.OpenAIUpstreamTransportHTTPSSE,
			service.OpenAIEndpointCapabilityChatCompletions,
			false,
			false,
			false,
			service.PlatformDeepseek,
		)
		if selectErr != nil || selection == nil || selection.Account == nil {
			if last != nil {
				h.handleFailoverExhausted(c, last, false)
			} else {
				write(c, http.StatusServiceUnavailable, "api_error", "No available DeepSeek accounts")
			}
			return
		}
		account := selection.Account
		// 能力过滤：官方 DeepSeek 支持 /v1/tools/* 与 /v1/files*，ollama /
		// crazyhub 等第三方中转通常只有对话端点。已探测/兜底判定不支持的工具
		// 账号直接跳过，换下一个（避免把请求发到必然 404 的上游）。
		if !account.SupportsOpenAIEndpointCapability(service.OpenAIEndpointCapabilityDeepseekTools) {
			// 选号阶段已抢到该账号的并发槽；这条路径不进入 forward，必须在此
			// 归还，否则槽位滞留到 TTL 到期（每次换号泄漏一个）。
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			failed[account.ID] = struct{}{}
			continue
		}
		var started bool
		release, status := h.acquireResponsesAccountSlot(c, apiKey.GroupID, "", selection, false, &started, reqLog)
		if status == openAISlotAcquireProfitVetoed {
			failed[account.ID] = struct{}{}
			continue
		}
		if status != openAISlotAcquireOK {
			if status == openAISlotAcquireFailed && len(failed) == 0 {
				return
			}
			failed[account.ID] = struct{}{}
			continue
		}
		result, forwardErr := func() (*service.DeepseekToolsForwardResult, error) {
			defer release()
			return h.gatewayService.ForwardDeepseekToolsRequest(ctx, c, account, request)
		}()
		if forwardErr == nil {
			h.recordDeepseekToolsUsage(c, apiKey, account, subscription, body, result)
			return
		}
		// 上游 HTTP 4xx/5xx 已由 service 透传给客户端，不 failover；只有网络层
		// 错误（UpstreamFailoverError.ShouldRetryNextAccount）换号重试。
		var failoverErr *service.UpstreamFailoverError
		if errors.As(forwardErr, &failoverErr) && failoverErr.ShouldRetryNextAccount() {
			failed[account.ID] = struct{}{}
			last = failoverErr
			continue
		}
		if !c.Writer.Written() {
			e := infraerrors.FromError(forwardErr)
			code := e.Reason
			if code == "" {
				code = "api_error"
			}
			write(c, int(e.Code), code, e.Message)
		}
		return
	}
	if last != nil {
		h.handleFailoverExhausted(c, last, false)
	}
}

func (h *OpenAIGatewayHandler) recordDeepseekToolsUsage(
	c *gin.Context,
	apiKey *service.APIKey,
	account *service.Account,
	subscription *service.UserSubscription,
	body []byte,
	result *service.DeepseekToolsForwardResult,
) {
	if h == nil || c == nil || apiKey == nil || account == nil || result == nil || result.Usage.Units <= 0 {
		return
	}
	payloadHash := service.HashUsageRequestPayload(body)
	// One ID per invocation, retained if the resulting billing task is retried.
	billingResult := &service.OpenAIForwardResult{
		RequestID: "deepseek_tools:" + uuid.NewString(), Model: result.Usage.Model(),
		DeepseekTool: &result.Usage, UpstreamHeaders: result.UpstreamHeaders,
	}
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	sessionID := service.ExtractClientSessionID(c)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	channelUsageFields := clientRequestedUsageFields(c, service.ChannelMappingResult{}, billingResult.Model, "")
	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result:             billingResult,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: payloadHash,
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      service.PlatformDeepseek,
			SessionID:          sessionID,
			ChannelUsageFields: channelUsageFields,
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.deepseek_tools"),
				zap.Int64("user_id", apiKey.User.ID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("endpoint", result.Usage.Endpoint),
				zap.Int64("account_id", account.ID),
			).Error("deepseek_tools.record_usage_failed", zap.Error(err))
		}
	})
}
