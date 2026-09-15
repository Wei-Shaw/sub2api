package handler

import (
	"context"
	"errors"
	"net/http"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AdobeImages 处理 Adobe 分组下的 /v1/images/generations 与 /v1/images/edits。
//
// 分组里可能同时有 Firefly Cookie 号和 OpenAI 形中转号。选到 oauth 时把请求翻译成
// Firefly payload；选到 apikey+base_url 时把同一份 OpenAI 请求转到 {base_url}。
// Adobe 账号不满足 account.IsOpenAICompatible()，走不了 OpenAI 调度器，故这里用
// 平台无关的 GatewayService.SelectAccountForModelWithExclusions 自建 failover。
func (h *GatewayHandler) AdobeImages(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		adobeImagesError(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		adobeImagesError(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	if h.adobeImageService == nil {
		adobeImagesError(c, http.StatusNotFound, "not_found_error", "Adobe image generation is not available")
		return
	}

	reqLog := requestLogger(
		c,
		"handler.gateway.adobe_images",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			adobeImagesError(c, http.StatusRequestEntityTooLarge, "invalid_request_error",
				buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		adobeImagesError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	parsed, err := h.openAIGatewayService.ParseOpenAIImagesRequest(c, body)
	if err != nil {
		adobeImagesError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	if !service.GroupAllowsImageGeneration(apiKey.Group) {
		adobeImagesError(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}

	reqLog = reqLog.With(zap.String("model", parsed.Model), zap.String("size", parsed.Size))
	setOpsRequestContext(c, parsed.Model, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))

	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject,
		service.ContentModerationProtocolOpenAIImages, parsed.Model, parsed.ModerationBody()); decision != nil &&
		!decision.AllowNextStage {
		h.anthropicSecurityAuditError(c, decision)
		return
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	h.runAdobeImagesFailover(c, reqLog, apiKey, subject, subscription, parsed, body)
}

// runAdobeImagesFailover 逐个账号尝试出图，直到成功或没有可换的账号。
// gpt-image 带 mask 时先只选 API key 中转号；没有可用中转再忽略 mask 走 Cookie。
// 其它模型忽略 mask，走正常调度。
func (h *GatewayHandler) runAdobeImagesFailover(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	parsed *service.OpenAIImagesRequest,
	body []byte,
) {
	requestCtx := c.Request.Context()
	failedAccountIDs := make(map[int64]struct{})
	var lastFailover *service.UpstreamFailoverError
	skipAdobeNative := false
	// 仅 gpt-image 家族的 mask 才优先打 API key 中转；banana 等忽略 mask。
	preferRelayForMask := service.AdobePrefersMaskRelay(parsed)
	deferredNatives := make(map[int64]struct{})

	for switchCount := 0; switchCount <= h.maxAccountSwitches; switchCount++ {
		if failoverClientGone(c) {
			return
		}

		excluded := service.AdobeMaskRelaySelectionExclusions(failedAccountIDs, preferRelayForMask, deferredNatives)
		if preferRelayForMask {
			h.gatewayService.ExcludeAdobeNativeAccounts(requestCtx, apiKey.GroupID, excluded)
		}

		account, err := h.gatewayService.SelectAccountForModelWithExclusions(
			requestCtx, apiKey.GroupID, "", parsed.Model, excluded)
		if err != nil || account == nil {
			if preferRelayForMask {
				preferRelayForMask = false
				reqLog.Warn("adobe_images.mask_fallback_native",
					zap.Int("failed_account_count", len(failedAccountIDs)),
					zap.Error(err),
				)
				account, err = h.gatewayService.SelectAccountForModelWithExclusions(
					requestCtx, apiKey.GroupID, "", parsed.Model, failedAccountIDs)
			}
			if err != nil || account == nil {
				h.finishAdobeImagesWithoutAccount(c, reqLog, apiKey, parsed, failedAccountIDs, lastFailover, err)
				return
			}
		}
		setOpsSelectedAccount(c, account.ID, account.Platform)

		if preferRelayForMask && !service.IsAdobeRelayAccount(account) {
			// listing 漏网的 Cookie 号：推迟到 mask fallback，不要记进 failed。
			deferredNatives[account.ID] = struct{}{}
			continue
		}

		if skipAdobeNative && !service.IsAdobeRelayAccount(account) {
			// 内容安全拒绝后绝不再把同一 prompt 打到其它 Firefly Cookie 号。
			failedAccountIDs[account.ID] = struct{}{}
			continue
		}

		if service.IsAdobeRelayAccount(account) {
			_, done := h.tryAdobeImagesRelay(
				c, reqLog, apiKey, subject, subscription, account, parsed, body, failedAccountIDs, &lastFailover, switchCount)
			if done {
				return
			}
			continue
		}

		token, _, err := h.gatewayService.GetAccessToken(requestCtx, account)
		if err != nil {
			// 账号缺 token（刷新器还没跑到，或 cookie 已失效）：换下一个。
			reqLog.Warn("adobe_images.token_unavailable",
				zap.Int64("account_id", account.ID), zap.Error(err))
			failedAccountIDs[account.ID] = struct{}{}
			continue
		}

		result, err := h.adobeImageService.Generate(requestCtx, account, token, parsed)
		if err == nil {
			h.finishAdobeImagesSuccess(c, reqLog, apiKey, subject, subscription, account, result, parsed, body)
			return
		}

		if errors.Is(err, context.Canceled) || failoverClientGone(c) {
			reqLog.Info("adobe_images.aborted_client_disconnected", zap.Int64("account_id", account.ID))
			return
		}

		failover := h.gatewayService.AdobeFailover(requestCtx, account.ID, token, err)
		if failover == nil {
			// 不属于 Adobe 上游语义（编解码错误等）：没有换号的依据，直接上抛。
			reqLog.Error("adobe_images.generate_failed",
				zap.Int64("account_id", account.ID), zap.Error(err))
			adobeImagesError(c, http.StatusBadGateway, "api_error", "Upstream request failed")
			return
		}
		if service.IsAdobeContentRejected(failover) && !service.IsAdobeRelayAccount(account) {
			skipAdobeNative = true
			h.gatewayService.ExcludeAdobeNativeAccounts(requestCtx, apiKey.GroupID, failedAccountIDs)
			reqLog.Warn("adobe_images.content_rejected_skip_native",
				zap.Int64("account_id", account.ID),
				zap.Int("switch_count", switchCount),
			)
			lastFailover = failover
			continue
		}
		if !failover.ShouldRetryNextAccount() {
			// 请求本身的问题，换号也救不了。
			adobeImagesFailoverError(c, failover)
			return
		}

		reqLog.Warn("adobe_images.account_failover",
			zap.Int64("account_id", account.ID),
			zap.String("reason", string(failover.Reason)),
			zap.Int("upstream_status", failover.StatusCode),
			zap.Int("switch_count", switchCount),
		)
		failedAccountIDs[account.ID] = struct{}{}
		lastFailover = failover
	}

	// 换号预算用尽。
	if lastFailover != nil {
		adobeImagesFailoverError(c, lastFailover)
		return
	}
	adobeImagesError(c, http.StatusServiceUnavailable, "api_error", "No available Adobe accounts")
}

// tryAdobeImagesRelay 把 OpenAI 形状的出图请求转到中转号的 base_url。
// done=true 表示已经给客户端写了成功或终态错误；switched=true 表示应换下一个账号。
func (h *GatewayHandler) tryAdobeImagesRelay(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	account *service.Account,
	parsed *service.OpenAIImagesRequest,
	body []byte,
	failedAccountIDs map[int64]struct{},
	lastFailover **service.UpstreamFailoverError,
	switchCount int,
) (switched bool, done bool) {
	if h.openAIGatewayService == nil {
		reqLog.Error("adobe_images.relay_unavailable", zap.Int64("account_id", account.ID))
		adobeImagesError(c, http.StatusBadGateway, "api_error", "Adobe relay forwarding is not available")
		return false, true
	}

	result, err := h.openAIGatewayService.ForwardImages(c.Request.Context(), c, account, body, parsed, "")
	if err == nil {
		h.finishAdobeImagesRelaySuccess(c, reqLog, apiKey, subject, subscription, account, result, parsed, body)
		return false, true
	}

	if errors.Is(err, context.Canceled) || failoverClientGone(c) {
		reqLog.Info("adobe_images.relay_aborted_client_disconnected", zap.Int64("account_id", account.ID))
		return false, true
	}

	if service.IsResponseCommitted(c) {
		reqLog.Warn("adobe_images.relay_failed_after_flush",
			zap.Int64("account_id", account.ID), zap.Error(err))
		return false, true
	}

	var failover *service.UpstreamFailoverError
	if errors.As(err, &failover) && failover.ShouldRetryNextAccount() {
		reqLog.Warn("adobe_images.relay_failover",
			zap.Int64("account_id", account.ID),
			zap.Int("upstream_status", failover.StatusCode),
			zap.Int("switch_count", switchCount),
		)
		failedAccountIDs[account.ID] = struct{}{}
		*lastFailover = failover
		return true, false
	}

	reqLog.Error("adobe_images.relay_failed",
		zap.Int64("account_id", account.ID), zap.Error(err))
	if failover != nil {
		adobeImagesFailoverError(c, failover)
		return false, true
	}
	adobeImagesError(c, http.StatusBadGateway, "api_error", "Upstream request failed")
	return false, true
}

// finishAdobeImagesWithoutAccount 处理「选不出账号」：首轮无候选说明分组本身没有可用
// 账号，后续轮次说明候选都已失败，此时应上报最后一次的上游错误而不是笼统的 503。
func (h *GatewayHandler) finishAdobeImagesWithoutAccount(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	parsed *service.OpenAIImagesRequest,
	failedAccountIDs map[int64]struct{},
	lastFailover *service.UpstreamFailoverError,
	selectErr error,
) {
	if failoverClientGone(c) {
		return
	}
	reqLog.Warn("adobe_images.account_select_failed",
		zap.Int("excluded_account_count", len(failedAccountIDs)),
		zap.Error(selectErr))

	if lastFailover != nil {
		adobeImagesFailoverError(c, lastFailover)
		return
	}
	markOpsRoutingCapacityLimitedIfNoAvailable(c, selectErr)
	cls := classifyNoAccountErrorFromGin(c, h.openAIGatewayService, apiKey, parsed.Model, parsed.Model, service.PlatformAdobe)
	adobeImagesError(c, cls.Status, cls.ErrType, cls.Message)
}

// finishAdobeImagesSuccess 写响应并记账。
func (h *GatewayHandler) finishAdobeImagesSuccess(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	account *service.Account,
	result *service.AdobeImageResult,
	parsed *service.OpenAIImagesRequest,
	body []byte,
) {
	c.Data(http.StatusOK, "application/json", result.Body)
	var forward *service.OpenAIForwardResult
	if result != nil {
		forward = result.Forward
	}
	h.recordAdobeImagesUsage(c, apiKey, subject, subscription, account, forward, parsed, body)
	upstreamModel := ""
	if result != nil && result.Forward != nil {
		upstreamModel = result.Forward.UpstreamModel
	}
	reqLog.Debug("adobe_images.request_completed",
		zap.Int64("account_id", account.ID),
		zap.String("upstream_model", upstreamModel),
	)
}

// finishAdobeImagesRelaySuccess 记账。响应体已由 ForwardImages 写入 gin，不能再 c.Data。
func (h *GatewayHandler) finishAdobeImagesRelaySuccess(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	account *service.Account,
	result *service.OpenAIForwardResult,
	parsed *service.OpenAIImagesRequest,
	body []byte,
) {
	h.recordAdobeImagesUsage(c, apiKey, subject, subscription, account, result, parsed, body)
	upstreamModel := ""
	if result != nil {
		upstreamModel = result.UpstreamModel
	}
	reqLog.Debug("adobe_images.relay_completed",
		zap.Int64("account_id", account.ID),
		zap.String("upstream_model", upstreamModel),
	)
}

func (h *GatewayHandler) recordAdobeImagesUsage(
	c *gin.Context,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	account *service.Account,
	result *service.OpenAIForwardResult,
	parsed *service.OpenAIImagesRequest,
	body []byte,
) {
	if result == nil {
		return
	}
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	sessionID := service.ExtractClientSessionID(c)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	channelUsageFields := service.ChannelUsageFields{
		OriginalModel:      clientRequestedModel(c, parsed.Model),
		ChannelMappedModel: parsed.Model,
	}

	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.openAIGatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result:             result,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: service.HashUsageRequestPayload(body),
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
			SessionID:          sessionID,
			ChannelUsageFields: channelUsageFields,
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.gateway.adobe_images"),
				zap.Int64("user_id", subject.UserID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Int64("account_id", account.ID),
			).Error("adobe_images.record_usage_failed", zap.Error(err))
		}
	})
}

// adobeImagesFailoverError 把 failover 错误渲染给客户端，优先用错误自带的对外状态码与文案。
func adobeImagesFailoverError(c *gin.Context, failover *service.UpstreamFailoverError) {
	status := failover.ClientStatusCode
	if status <= 0 {
		status = failover.StatusCode
	}
	if status <= 0 {
		status = http.StatusBadGateway
	}
	message := failover.ClientMessage
	if message == "" {
		message = "Upstream request failed"
	}
	adobeImagesError(c, status, string(failover.Reason), message)
}

// adobeImagesError 渲染 OpenAI 形状的错误体——images 端点的客户端按 OpenAI 协议解析。
func adobeImagesError(c *gin.Context, status int, errType, message string) {
	if service.IsResponseCommitted(c) {
		return
	}
	c.JSON(status, gin.H{"error": gin.H{"type": errType, "message": message}})
}
