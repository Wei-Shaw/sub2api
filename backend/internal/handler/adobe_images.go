package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"

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

	// composite 分组在 middleware 已 resolve 出上游模型时，选号与渠道映射按它走（与 OpenAI 出图一致）。
	routingModel := parsed.Model
	if resolvedModel, ok := service.ResolvedUpstreamModelFromContext(c.Request.Context()); ok {
		routingModel = resolvedModel
	}

	reqLog = reqLog.With(
		zap.String("model", parsed.Model),
		zap.String("routing_model", routingModel),
		zap.String("size", parsed.Size),
		zap.Bool("stream", parsed.Stream),
	)
	setOpsRequestContext(c, parsed.Model, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))

	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject,
		service.ContentModerationProtocolOpenAIImages, parsed.Model, parsed.ModerationBody()); decision != nil &&
		!decision.AllowNextStage {
		h.openAISecurityAuditError(c, decision)
		return
	}

	releaseAdmission, admitted := h.admitAdobeImagesRequest(c, reqLog, apiKey, subject)
	if !admitted {
		return
	}
	defer releaseAdmission()

	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, routingModel)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	h.runAdobeImagesFailover(c, reqLog, apiKey, subject, subscription, &adobeImagesRequest{
		parsed:         parsed,
		body:           body,
		routingModel:   routingModel,
		channelMapping: channelMapping,
		call:           service.NewAdobeImageCall(parsed, channelMapping.MappedModel),
	})
}

// adobeImagesRequest 汇总一次 Adobe 出图请求在换号尝试之间共享的输入。
type adobeImagesRequest struct {
	parsed *service.OpenAIImagesRequest
	body   []byte
	// routingModel 用于选号；channelMapping.MappedModel 用于转发与账号模型映射。
	routingModel   string
	channelMapping service.ChannelMappingResult
	// call 在换号之间复用已抓取的输入图。
	call *service.AdobeImageCall
}

// admitAdobeImagesRequest 在选号前完成与 OpenAI/Grok 出图一致的准入：
// 全局出图并发槽 → 用户并发槽 → 计费资格（余额、user×platform 配额、API Key 限速、RPM）。
// 返回的 release 释放已占用的槽位；admitted=false 时错误响应已写出。
func (h *GatewayHandler) admitAdobeImagesRequest(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
) (release func(), admitted bool) {
	var releases []func()
	release = func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}

	imageRelease, acquired := h.acquireImageGenerationSlot(c)
	if !acquired {
		return nil, false
	}
	if imageRelease != nil {
		releases = append(releases, imageRelease)
	}

	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	streamStarted := false
	userRelease, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, false, &streamStarted)
	if err != nil {
		reqLog.Warn("adobe_images.user_slot_acquire_failed", zap.Error(err))
		release()
		status, errType, _, message := concurrencyErrorResponse(err, "user")
		adobeImagesError(c, status, errType, message)
		return nil, false
	}
	// 不用 wrapReleaseOnDone：提交之后上游与客户端连接脱钩，客户端断开时 Firefly 任务仍在跑。
	// 断开即释放会让用户靠「提交后立刻断开」绕过并发上限；槽位持有到 handler 返回，
	// 由 Redis 槽位 TTL（15 分钟，长于 adobeImageDetachedTimeout）兜底异常路径。
	if userRelease = adobeReleaseOnce(userRelease); userRelease != nil {
		releases = append(releases, userRelease)
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("adobe_images.billing_eligibility_check_failed", zap.Error(err))
		release()
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		adobeImagesError(c, status, code, message)
		return nil, false
	}
	return release, true
}

// acquireImageGenerationSlot 占用与 OpenAI/Grok 出图共享的全局出图并发槽。
func (h *GatewayHandler) acquireImageGenerationSlot(c *gin.Context) (func(), bool) {
	release, acquired := acquireImageConcurrencySlot(c.Request.Context(), h.cfg, h.imageLimiter)
	if acquired {
		return release, true
	}
	adobeImagesError(c, http.StatusTooManyRequests, "rate_limit_error", "Image generation concurrency limit exceeded, please retry later")
	return nil, false
}

// adobeImagesMaxSelectionRounds 是单个请求内选号轮数的硬上限（含不计入换号预算的跳过轮）。
const adobeImagesMaxSelectionRounds = 128

// runAdobeImagesFailover 逐个账号尝试出图，直到成功或没有可换的账号。
// gpt-image 带 mask 时先只选 API key 中转号；没有可用中转再忽略 mask 走 Cookie。
// 其它模型忽略 mask，走正常调度。
func (h *GatewayHandler) runAdobeImagesFailover(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	req *adobeImagesRequest,
) {
	parsed := req.parsed
	requestCtx := c.Request.Context()
	failedAccountIDs := make(map[int64]struct{})
	var lastFailover *service.UpstreamFailoverError
	skipAdobeNative := false
	// 仅 gpt-image 家族的 mask 才优先打 API key 中转；banana 等忽略 mask。
	preferRelayForMask := service.AdobePrefersMaskRelay(parsed)
	deferredNatives := make(map[int64]struct{})
	// busyAccountIDs 记录本请求内并发槽已满的账号：只从选号中排除，不算失败、不产生 failover 错误。
	busyAccountIDs := make(map[int64]struct{})

	// attempts 只统计真正打到账号上的尝试（中转转发、取 token 失败、Firefly 出图）。
	// busy / 推迟 / 内容拒绝后跳过的账号只加入排除集合，不消耗换号预算；排除集合单调增长，
	// 选号候选随之减少，round 上限只是防御调度器忽略排除集合时的死循环。
	attempts := 0
	for round := 0; attempts <= h.maxAccountSwitches && round < adobeImagesMaxSelectionRounds; round++ {
		if failoverClientGone(c) {
			return
		}

		unavailable := mergeAccountIDSets(failedAccountIDs, busyAccountIDs)
		excluded := service.AdobeMaskRelaySelectionExclusions(unavailable, preferRelayForMask, deferredNatives)
		if preferRelayForMask {
			h.gatewayService.ExcludeAdobeNativeAccounts(requestCtx, apiKey.GroupID, excluded)
		}

		account, err := h.gatewayService.SelectAccountForModelWithExclusions(
			requestCtx, apiKey.GroupID, "", req.routingModel, excluded)
		if err != nil || account == nil {
			if preferRelayForMask {
				preferRelayForMask = false
				reqLog.Warn("adobe_images.mask_fallback_native",
					zap.Int("failed_account_count", len(failedAccountIDs)),
					zap.Error(err),
				)
				account, err = h.gatewayService.SelectAccountForModelWithExclusions(
					requestCtx, apiKey.GroupID, "", req.routingModel, unavailable)
			}
			if err != nil || account == nil {
				if lastFailover == nil && len(busyAccountIDs) > 0 {
					adobeImagesAccountsBusyError(c, reqLog, busyAccountIDs)
					return
				}
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

		releaseAccount, acquired, err := h.concurrencyHelper.TryAcquireAccountSlot(requestCtx, account.ID, account.Concurrency)
		if err != nil {
			reqLog.Warn("adobe_images.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			status, errType, _, message := concurrencyErrorResponse(err, "account")
			adobeImagesError(c, status, errType, message)
			return
		}
		if !acquired {
			busyAccountIDs[account.ID] = struct{}{}
			continue
		}
		// 每轮尝试结束就释放账号槽，不能 defer 到函数返回，否则失败的账号会一直被占着。
		// 同样不用 wrapReleaseOnDone：客户端断开后该账号上的 Firefly 任务仍在跑，槽位要占到尝试结束。
		releaseAccount = adobeReleaseOnce(releaseAccount)
		if releaseAccount == nil {
			releaseAccount = func() {}
		}

		attempts++
		if service.IsAdobeRelayAccount(account) {
			_, done := h.tryAdobeImagesRelay(
				c, reqLog, apiKey, subject, subscription, account, req, failedAccountIDs, &lastFailover, attempts)
			releaseAccount()
			if done {
				return
			}
			continue
		}

		if parsed.Stream {
			// Firefly 原生出图没有流式协议；返回 JSON 会让按 SSE 解析的客户端出错，明确拒绝。
			releaseAccount()
			adobeImagesError(c, http.StatusBadRequest, "invalid_request_error",
				"stream is not supported by Adobe native image generation")
			return
		}

		token, _, err := h.gatewayService.GetAccessToken(requestCtx, account)
		if err != nil {
			releaseAccount()
			// 账号缺 token（刷新器还没跑到，或 cookie 已失效）：换下一个。
			reqLog.Warn("adobe_images.token_unavailable",
				zap.Int64("account_id", account.ID), zap.Error(err))
			failedAccountIDs[account.ID] = struct{}{}
			continue
		}

		// 提交之后的阶段在 service 内脱离客户端连接：拿到产物就一定记账，即便客户端已断开。
		result, err := h.adobeImageService.GenerateCall(requestCtx, account, token, req.call)
		releaseAccount()
		if err == nil {
			h.finishAdobeImagesSuccess(c, reqLog, apiKey, subject, subscription, account, result, req)
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
				zap.Int("attempts", attempts),
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
			zap.Int("attempts", attempts),
		)
		failedAccountIDs[account.ID] = struct{}{}
		lastFailover = failover
	}

	// 换号预算用尽。
	if lastFailover != nil {
		adobeImagesFailoverError(c, lastFailover)
		return
	}
	if len(busyAccountIDs) > 0 {
		adobeImagesAccountsBusyError(c, reqLog, busyAccountIDs)
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
	req *adobeImagesRequest,
	failedAccountIDs map[int64]struct{},
	lastFailover **service.UpstreamFailoverError,
	attempts int,
) (switched bool, done bool) {
	if h.openAIGatewayService == nil {
		reqLog.Error("adobe_images.relay_unavailable", zap.Int64("account_id", account.ID))
		adobeImagesError(c, http.StatusBadGateway, "api_error", "Adobe relay forwarding is not available")
		return false, true
	}

	result, err := h.openAIGatewayService.ForwardImages(
		c.Request.Context(), c, account, req.body, req.parsed, req.channelMapping.MappedModel)
	if err == nil {
		h.finishAdobeImagesRelaySuccess(c, reqLog, apiKey, subject, subscription, account, result, req)
		return false, true
	}

	// 中转流式中途断开时已经把部分图片交给了客户端：先按实际张数记账，再处理错误。
	partial := result != nil && result.ImageCount > 0
	if partial {
		reqLog.Warn("adobe_images.relay_partial_error_with_image_result",
			zap.Int64("account_id", account.ID),
			zap.Int("image_count", result.ImageCount),
			zap.Error(err))
		h.recordAdobeImagesUsage(c, apiKey, subject, subscription, account, result, req)
	}

	if errors.Is(err, context.Canceled) || failoverClientGone(c) {
		reqLog.Info("adobe_images.relay_aborted_client_disconnected", zap.Int64("account_id", account.ID))
		return false, true
	}

	if partial || service.IsResponseCommitted(c) {
		reqLog.Warn("adobe_images.relay_failed_after_flush",
			zap.Int64("account_id", account.ID), zap.Error(err))
		return false, true
	}

	var failover *service.UpstreamFailoverError
	if errors.As(err, &failover) && failover.ShouldRetryNextAccount() {
		reqLog.Warn("adobe_images.relay_failover",
			zap.Int64("account_id", account.ID),
			zap.Int("upstream_status", failover.StatusCode),
			zap.Int("attempts", attempts),
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
	req *adobeImagesRequest,
) {
	if result == nil {
		reqLog.Error("adobe_images.empty_result", zap.Int64("account_id", account.ID))
		adobeImagesError(c, http.StatusBadGateway, "api_error", "Upstream request failed")
		return
	}
	c.Data(http.StatusOK, "application/json", result.Body)
	h.recordAdobeImagesUsage(c, apiKey, subject, subscription, account, result.Forward, req)
	upstreamModel := ""
	if result.Forward != nil {
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
	req *adobeImagesRequest,
) {
	h.recordAdobeImagesUsage(c, apiKey, subject, subscription, account, result, req)
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
	req *adobeImagesRequest,
) {
	if result == nil {
		return
	}
	parsed := req.parsed
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	sessionID := service.ExtractClientSessionID(c)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	channelUsageFields := clientRequestedUsageFields(c, req.channelMapping, parsed.Model, result.UpstreamModel)
	requestPayloadHash := service.HashUsageRequestPayload(req.body)
	if parsed.Multipart {
		requestPayloadHash = service.HashUsageRequestPayload([]byte(parsed.StickySessionSeed()))
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
			RequestPayloadHash: requestPayloadHash,
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

// adobeImagesAccountsBusyError 处理「候选账号并发槽都满了」：这是可重试的容量问题，返回 429 而非 503。
func adobeImagesAccountsBusyError(c *gin.Context, reqLog *zap.Logger, busyAccountIDs map[int64]struct{}) {
	reqLog.Info("adobe_images.accounts_busy", zap.Int("busy_account_count", len(busyAccountIDs)))
	markOpsRoutingCapacityLimited(c)
	adobeImagesError(c, http.StatusTooManyRequests, "rate_limit_error",
		"Too many concurrent requests for available accounts, please retry later")
}

// adobeReleaseOnce 把槽位释放函数包成幂等的；与 wrapReleaseOnDone 不同，它不随请求 ctx 取消而提前释放。
func adobeReleaseOnce(release func()) func() {
	if release == nil {
		return nil
	}
	var once sync.Once
	return func() { once.Do(release) }
}

// mergeAccountIDSets 返回若干账号集合的并集副本，不改写入参。
func mergeAccountIDSets(sets ...map[int64]struct{}) map[int64]struct{} {
	size := 0
	for _, set := range sets {
		size += len(set)
	}
	out := make(map[int64]struct{}, size)
	for _, set := range sets {
		for id := range set {
			out[id] = struct{}{}
		}
	}
	return out
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
