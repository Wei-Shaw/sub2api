package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func (h *OpenAIGatewayHandler) bindDeepseekFileReferences(c *gin.Context, apiKey *service.APIKey, protocol string, body *[]byte, write func(*gin.Context, int, string, string)) bool {
	ctx := c.Request.Context()
	next, normalized, err := h.gatewayService.PrepareDeepseekFileIngress(ctx, openAICompatibleRequestPlatform(ctx, apiKey), protocol, apiKey.UserID, *body)
	if err != nil {
		e := infraerrors.FromError(err)
		write(c, int(e.Code), e.Reason, e.Message)
		return false
	}
	*body = normalized
	if next != ctx {
		c.Request = c.Request.WithContext(next)
	}
	return true
}

// Only the five standalone tools reserve balance. Files, including opt-in
// document parsing, have no gateway billing units.
func deepseekToolsInflightEstimate(endpoint string, body []byte) service.InflightEstimateRequest {
	var units int
	switch endpoint {
	case "search", "fetch":
		units = 1
	case "ocr":
		units = 10 // pages
	case "asr":
		units = 120 // maximum supported audio duration in seconds
	case "tts":
		units = service.DeepseekTTSBillingChars(body)
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

// extractDeepseekToolsPromptText 提取请求体里的文本字段供内容审计归一化。
func extractDeepseekToolsPromptText(endpoint string, body []byte) string {
	if !gjson.ValidBytes(body) {
		return ""
	}
	var keys []string
	switch endpoint {
	case "search":
		keys = []string{"query", "ori_question", "q"}
	case "tts":
		keys = []string{"text", "input"}
	case "fetch":
		keys = []string{"url"}
	default:
		keys = []string{"query", "q", "text", "input", "prompt", "url"}
	}
	var parts []string
	for _, key := range keys {
		if v := gjson.GetBytes(body, key); v.Type == gjson.String {
			if s := strings.TrimSpace(v.Str); s != "" {
				parts = append(parts, s)
			}
		}
	}
	return strings.Join(parts, "\n")
}

const deepseekUploadSlotKey = "deepseek_upload_user_slot"

// AdmitDeepseekUpload holds the user slot across all middleware that may buffer the upload.
func (h *OpenAIGatewayHandler) AdmitDeepseekUpload(c *gin.Context) {
	if c.Request.Method != http.MethodPost {
		return
	}
	var endpoint string
	switch c.FullPath() {
	case "/v1/files":
		endpoint = "files_create"
	case "/v1/tools/ocr":
		endpoint = "ocr"
	case "/v1/tools/asr":
		endpoint = "asr"
	default:
		return
	}
	release, acquired := h.acquireDeepseekToolsUserSlot(c, endpoint)
	if !acquired {
		c.Abort()
		return
	}
	if release != nil {
		defer release()
	}
	c.Set(deepseekUploadSlotKey, true)
	c.Next()
}

func (h *OpenAIGatewayHandler) deepseekToolsErrorWriters(c *gin.Context, endpoint string) (func(*gin.Context, int, string, string), openAISlotErrorWriter) {
	write := h.errorResponse
	if strings.HasPrefix(endpoint, "files_") && service.IsDeepseekFilesAnthropicRequest(c) {
		write = h.anthropicErrorResponse
		return write, func(status int, errType, code, message string) { write(c, status, errType, message) }
	}
	return write, func(status int, errType, code, message string) {
		h.handleStreamingAwareErrorWithCode(c, status, errType, code, message, false, false)
	}
}

func (h *OpenAIGatewayHandler) acquireDeepseekToolsUserSlot(c *gin.Context, endpoint string) (func(), bool) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	write, writeSlotError := h.deepseekToolsErrorWriters(c, endpoint)
	// composite 组放行：工具请求不带 model，无法经 compositeTarget 中间件解析，
	// 交给调度器按 platformOverride=deepseek 选号（对齐 Grok video 先例）。
	if !ok || apiKey == nil || apiKey.Group == nil || (apiKey.Group.Platform != service.PlatformDeepseek && apiKey.Group.Platform != service.PlatformComposite) {
		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
		write(c, http.StatusNotFound, "not_found_error", "DeepSeek tools API is not supported for this platform")
		return nil, false
	}
	reqLog := requestLogger(c, "handler.openai_gateway.deepseek_tools", zap.String("endpoint", endpoint))
	if missing := h.missingResponsesDependencies(); len(missing) > 0 {
		reqLog.Error("openai.handler_dependencies_missing", zap.Strings("missing_dependencies", missing))
		write(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable")
		return nil, false
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		write(c, http.StatusInternalServerError, "api_error", "User context not found")
		return nil, false
	}
	if (endpoint == "tts" || endpoint == "ocr") && h.cfg != nil {
		c.Request = c.Request.WithContext(service.WithDrainingSlotTTL(c.Request.Context(), time.Duration(h.cfg.Gateway.ConcurrencySlotTTLMinutes)*time.Minute))
	}
	var started bool
	return h.acquireOpenAIUserSlot(c, subject.UserID, subject.Concurrency, false, &started, reqLog, writeSlotError, releaseSlotOnReturn)
}

// DeepseekTools serves the standalone tools and Files endpoints.
func (h *OpenAIGatewayHandler) DeepseekTools(c *gin.Context, endpoint string) {
	if c.Writer.Written() || service.IsResponseCommitted(c) {
		return
	}
	if !c.GetBool(deepseekUploadSlotKey) {
		release, acquired := h.acquireDeepseekToolsUserSlot(c, endpoint)
		if !acquired {
			return
		}
		if release != nil {
			defer release()
		}
	}
	apiKey, _ := middleware2.GetAPIKeyFromContext(c)
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	write, writeSlotError := h.deepseekToolsErrorWriters(c, endpoint)
	reqLog := requestLogger(c, "handler.openai_gateway.deepseek_tools", zap.String("endpoint", endpoint))
	anthropicFiles := strings.HasPrefix(endpoint, "files_") && service.IsDeepseekFilesAnthropicRequest(c)
	var started bool
	service.BindErrorPassthroughService(c, h.errorPassthroughService)
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
		if middleware2.RejectRequestBodyReadError(c, "handler.openai_gateway.deepseek_tools", err) {
			return
		}
		middleware2.AbortWithRequestBodyReadFailure(c, "handler.openai_gateway.deepseek_tools", err)
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
		auditBody := body
		if input := extractDeepseekToolsPromptText(endpoint, body); input != "" {
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
	if estimate := deepseekToolsInflightEstimate(endpoint, body); estimate.DeepseekTool != nil {
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
	failed := map[int64]struct{}{}
	var last *service.UpstreamFailoverError
	for attempts := 0; attempts < 4; attempts++ {
		selection, _, selectErr := h.gatewayService.SelectAccountWithSchedulerForCapability(
			ctx,
			apiKey.GroupID,
			"",
			"",
			"",
			failed,
			service.OpenAIUpstreamTransportHTTPSSE,
			service.OpenAIEndpointCapabilityDeepseekTools,
			false,
			false,
			false,
			service.PlatformDeepseek,
		)
		if selectErr != nil || selection == nil || selection.Account == nil {
			break
		}
		release, status := h.acquireOpenAIAccountSlot(c, apiKey.GroupID, "", selection, false, &started, reqLog, writeSlotError, releaseSlotOnReturn)
		if status == openAISlotAcquireProfitVetoed {
			failed[selection.Account.ID] = struct{}{}
			continue
		}
		if status != openAISlotAcquireOK {
			return
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)
		result, forwardErr := func() (*service.DeepseekToolsForwardResult, error) {
			if release != nil {
				defer release()
			}
			return h.gatewayService.ForwardDeepseekToolsRequest(ctx, c, account, request)
		}()
		if forwardErr == nil {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account, "", true, nil)
			h.recordDeepseekToolsUsage(c, apiKey, account, subscription, body, result)
			return
		}
		if c.Writer.Written() || service.IsResponseCommitted(c) {
			return
		}
		if !errors.As(forwardErr, &last) {
			e := infraerrors.FromError(forwardErr)
			if e.Reason == "" {
				write(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
			} else {
				write(c, int(e.Code), e.Reason, e.Message)
			}
			return
		}
		h.gatewayService.ReportOpenAIAccountScheduleResult(account, "", false, nil, forwardErr)
		if !last.ShouldRetryNextAccount() || ctx.Err() != nil {
			break
		}
		h.gatewayService.RecordOpenAIAccountSwitch()
		failed[account.ID] = struct{}{}
	}
	if last == nil {
		markOpsRoutingCapacityLimited(c)
		write(c, http.StatusServiceUnavailable, "api_error", "No available DeepSeek accounts")
		return
	}
	copyFailoverRetryAfter(c, last.ResponseHeaders)
	status, errType, message := h.mapUpstreamError(last.StatusCode)
	if last.ClientStatusCode > 0 && last.ClientMessage != "" {
		status, message = last.ClientStatusCode, last.ClientMessage
	}
	write(c, status, errType, message)
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
