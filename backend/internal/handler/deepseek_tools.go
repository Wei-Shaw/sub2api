package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// deepseekToolsAudioMode 把 service 层提取的计费量种类映射到
// BillingService.CalculateAudioCost 的 mode。
func deepseekToolsAudioMode(kind string) string {
	switch kind {
	case service.DeepseekToolsUsageOCRPages:
		return "deepseek_ocr"
	case service.DeepseekToolsUsageASRSeconds:
		return "deepseek_asr"
	case service.DeepseekToolsUsageTTSChars:
		return "deepseek_tts"
	default:
		return ""
	}
}

// 在途预留的保守估值常量（真实用量上游成功后才落账，预留只防透支）：
// OCR 按 ~10 页文档、ASR 按 ~10 分钟单段音频、TTS 按 ~2000 计费字符
// （全宽字符算 2）估算。
const (
	deepseekToolsInflightOCRPages   = 10.0
	deepseekToolsInflightASRSeconds = 600.0
	deepseekToolsInflightTTSChars   = 2000.0
)

// deepseekToolsInflightEstimate 返回在途预留估算输入：
// search/fetch 按次（PerRequest）、ocr/asr/tts 按音频类（Audio）保守估值；
// files_* 是零成本辅助端点，不预留。
func deepseekToolsInflightEstimate(endpoint string) service.InflightEstimateRequest {
	switch endpoint {
	case "search", "fetch":
		return service.InflightEstimateRequest{
			Model:       "deepseek-tools-" + endpoint,
			Kind:        service.InflightEstimatePerRequest,
			SearchCalls: 1,
		}
	case "ocr":
		return service.InflightEstimateRequest{
			Model: "deepseek-tools-ocr", Kind: service.InflightEstimateAudio,
			AudioMode: "deepseek_ocr", AudioUnits: deepseekToolsInflightOCRPages,
		}
	case "asr":
		return service.InflightEstimateRequest{
			Model: "deepseek-tools-asr", Kind: service.InflightEstimateAudio,
			AudioMode: "deepseek_asr", AudioUnits: deepseekToolsInflightASRSeconds,
		}
	case "tts":
		return service.InflightEstimateRequest{
			Model: "deepseek-tools-tts", Kind: service.InflightEstimateAudio,
			AudioMode: "deepseek_tts", AudioUnits: deepseekToolsInflightTTSChars,
		}
	default:
		// files_create/files_list/files_get/files_delete：零成本，不预留。
		return service.InflightEstimateRequest{}
	}
}

// deepseekToolsBillingResult 把 service 提取的计费量折算成 usage 落账回执。
// usage.Kind 为空（files_* 或上游失败）返回 nil，不产生任何计费。
func deepseekToolsBillingResult(endpoint string, usage service.DeepseekToolsUsage, requestID string) *service.OpenAIForwardResult {
	if usage.Kind == "" {
		return nil
	}
	result := &service.OpenAIForwardResult{
		RequestID: requestID,
		Model:     "deepseek-tools-" + endpoint,
	}
	if usage.Kind == service.DeepseekToolsUsagePerRequest {
		// search/fetch 按次：复用 SearchCount → CalculateSearchCost 通道
		//（默认 ¥5/千次口径，分组 search_price_per_1k 可覆盖）。
		result.SearchCount = int(usage.Count)
		return result
	}
	if mode := deepseekToolsAudioMode(usage.Kind); mode != "" {
		result.AudioUsage = &service.AudioUsage{Mode: mode, DurationOrUnits: usage.Count}
		return result
	}
	return nil
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
	if !ok || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformDeepseek {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "DeepSeek tools API is not supported for this platform")
		return
	}
	if !h.ensureResponsesDependencies(c, nil) {
		return
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	// POST 系端点读入 body（multipart/JSON 同样字节读入，Content-Type 原样传
	// service）；GET/DELETE（files_list/files_get/files_delete）传 nil。
	body, err := readGrokVoiceGatewayBody(c)
	if err != nil {
		if errors.Is(err, errGrokVoiceBodyRequired) {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		bodyLog := requestLogger(c, "handler.openai_gateway.deepseek_tools", zap.String("endpoint", endpoint))
		if handleRequestBodyReadError(c, bodyLog, err) {
			return
		}
		status, message := requestBodyReadFailure(bodyLog, c.Request, err)
		h.errorResponse(c, status, "invalid_request_error", message)
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
	inflightDone, inflightErr := reserveInflightBalance(c, h.billingCacheService, h.gatewayService, apiKey, subscription, deepseekToolsInflightEstimate(endpoint))
	if inflightErr != nil {
		status, code, message, retryAfter := billingErrorDetails(inflightErr)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}
	defer inflightDone()

	contentType := c.GetHeader("Content-Type")
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/json"
	}

	reqLog := requestLogger(c, "handler.openai_gateway.deepseek_tools", zap.String("endpoint", endpoint))
	failed := map[int64]struct{}{}
	var last *service.UpstreamFailoverError
	for attempts := 0; attempts < 4; attempts++ {
		// 能力位复用 ChatCompletions（工具端点与对话同账号池），platform 强制 Deepseek。
		selection, _, selectErr := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(),
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
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available DeepSeek accounts")
			}
			return
		}
		account := selection.Account
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
			return h.gatewayService.ForwardDeepseekToolsRequest(c.Request.Context(), c, account, endpoint, body, contentType)
		}()
		if forwardErr == nil {
			h.recordDeepseekToolsUsage(c, apiKey, account, subscription, endpoint, body, result)
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
		return
	}
	if last != nil {
		h.handleFailoverExhausted(c, last, false)
	}
}

// recordDeepseekToolsUsage 对成功调用落账：search/fetch 走 SearchCount（按次）、
// ocr/asr/tts 走 AudioUsage（deepseek_ocr/deepseek_asr/deepseek_tts 默认价）；
// files_* 辅助端点零成本，不记 usage log。
func (h *OpenAIGatewayHandler) recordDeepseekToolsUsage(
	c *gin.Context,
	apiKey *service.APIKey,
	account *service.Account,
	subscription *service.UserSubscription,
	endpoint string,
	body []byte,
	result *service.DeepseekToolsForwardResult,
) {
	if h == nil || c == nil || apiKey == nil || account == nil || result == nil {
		return
	}
	payloadHash := service.HashUsageRequestPayload(body)
	if payloadHash == "" {
		payloadHash = service.HashUsageRequestPayload([]byte(endpoint))
	}
	// files_*：零成本辅助端点（usage.Kind 为空），不落账。
	billingResult := deepseekToolsBillingResult(endpoint, result.Usage, service.StableDeepseekToolsBillingRequestID(endpoint, payloadHash))
	if billingResult == nil {
		return
	}
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	sessionID := service.ExtractClientSessionID(c)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
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
			QuotaPlatform:      quotaPlatform,
			SessionID:          sessionID,
			ChannelUsageFields: channelUsageFields,
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.deepseek_tools"),
				zap.Int64("user_id", apiKey.User.ID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("endpoint", endpoint),
				zap.Int64("account_id", account.ID),
			).Error("deepseek_tools.record_usage_failed", zap.Error(err))
		}
	})
}
