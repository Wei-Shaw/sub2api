package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// geminiCachedContentCreateMaxAttempts 限制创建缓存时实际发往上游的账号数。
const geminiCachedContentCreateMaxAttempts = 3

// geminiCachedContentPersistTimeout 是上游操作完成后落库的超时；落库与客户端连接脱钩。
const geminiCachedContentPersistTimeout = 15 * time.Second

const geminiCachedContentUnavailableMessage = "The account holding this cached content is temporarily unavailable"

// geminiCachedContentsPreamble 校验显式缓存管理接口的公共前置条件：gemini 分组、非 composite 分组、功能已装配。
func (h *GatewayHandler) geminiCachedContentsPreamble(c *gin.Context, component string) (*service.APIKey, middleware.AuthSubject, *zap.Logger, bool) {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		googleError(c, http.StatusUnauthorized, "Invalid API key")
		return nil, middleware.AuthSubject{}, nil, false
	}
	authSubject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		googleError(c, http.StatusInternalServerError, "User context not found")
		return nil, middleware.AuthSubject{}, nil, false
	}
	if h.geminiCachedContentService == nil {
		googlePlatformError(c, http.StatusNotFound, "Explicit context caching is not enabled")
		return nil, middleware.AuthSubject{}, nil, false
	}
	if apiKey.Group == nil || apiKey.GroupID == nil || apiKey.Group.Platform == service.PlatformComposite {
		googleError(c, http.StatusBadRequest, "Explicit context caching is not supported for this API key group")
		return nil, middleware.AuthSubject{}, nil, false
	}
	if effectiveAPIKeyPlatform(c, apiKey) != service.PlatformGemini {
		googleError(c, http.StatusBadRequest, "API key group platform is not gemini")
		return nil, middleware.AuthSubject{}, nil, false
	}
	reqLog := requestLogger(
		c,
		component,
		zap.Int64("user_id", authSubject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	return apiKey, authSubject, reqLog, true
}

func readGeminiCachedContentBody(c *gin.Context) ([]byte, bool) {
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			googleError(c, http.StatusRequestEntityTooLarge, buildBodyTooLargeMessage(maxErr.Limit))
			return nil, false
		}
		googleError(c, http.StatusBadRequest, "Failed to read request body")
		return nil, false
	}
	if len(body) == 0 {
		googleError(c, http.StatusBadRequest, "Request body is empty")
		return nil, false
	}
	return body, true
}

func (h *GatewayHandler) checkGeminiCachedContentBilling(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subscription *service.UserSubscription) bool {
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("gemini.cached_content.billing_eligibility_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		googleErrorWithType(c, status, code, "", message)
		return false
	}
	return true
}

// GeminiCachedContentsCreate POST /v1beta/cachedContents
// 创建请求内一次性收取缓存 token 的输入费与整个有效期的存储费。
func (h *GatewayHandler) GeminiCachedContentsCreate(c *gin.Context) {
	apiKey, authSubject, reqLog, ok := h.geminiCachedContentsPreamble(c, "handler.gemini_v1beta.cached_contents.create")
	if !ok {
		return
	}
	svc := h.geminiCachedContentService

	body, ok := readGeminiCachedContentBody(c)
	if !ok {
		return
	}
	req, err := service.ParseGeminiCachedContentCreateRequest(body, svc.Now(), svc.MaxTTL())
	if err != nil {
		googleError(c, http.StatusBadRequest, err.Error())
		return
	}
	modelName := req.Model
	reqLog = reqLog.With(zap.String("model", modelName))

	setOpsRequestContext(c, modelName, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(false, false)))
	pricingCtx, pricingAt := service.WithGatewayTokenRequestPricing(c.Request.Context())
	c.Request = c.Request.WithContext(pricingCtx)

	if decision := h.checkSecurityAudit(c, reqLog, apiKey, authSubject, service.ContentModerationProtocolGemini, modelName, body); decision != nil && !decision.AllowNextStage {
		googleSecurityAuditError(c, decision)
		return
	}

	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, modelName)
	reqModel := modelName
	if channelMapping.Mapped {
		modelName = channelMapping.MappedModel
	}

	subscription, _ := middleware.GetSubscriptionFromContext(c)
	geminiConcurrency := NewConcurrencyHelper(h.concurrencyHelper.concurrencyService, SSEPingFormatNone, 0)
	streamStarted := false
	userReleaseFunc, err := geminiConcurrency.AcquireUserSlotWithWait(c, authSubject.UserID, authSubject.Concurrency, false, &streamStarted)
	if err != nil {
		reqLog.Warn("gemini.cached_content.user_slot_acquire_failed", zap.Error(err))
		googleConcurrencyError(c, err, "user")
		return
	}
	userReleaseFunc = wrapReleaseOnDone(c.Request.Context(), userReleaseFunc)
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	if !h.checkGeminiCachedContentBilling(c, reqLog, apiKey, subscription) {
		return
	}
	inflightRelease, err := reserveInflightBalance(c, h.billingCacheService, h.gatewayService, apiKey, subscription, tokenInflightEstimate(modelName, body))
	if err != nil {
		reqLog.Info("gemini.cached_content.inflight_reservation_rejected", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		googleErrorWithType(c, status, code, "", message)
		return
	}
	defer inflightRelease()

	excluded, err := h.gatewayService.GeminiExplicitCacheExclusions(c.Request.Context(), apiKey.GroupID, 0)
	if err != nil {
		reqLog.Warn("gemini.cached_content.list_accounts_failed", zap.Error(err))
		googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts support explicit context caching")
		return
	}

	var lastUpstreamErr *service.GeminiCachedContentUpstreamError
	var ttlRejectedLimit time.Duration
	for attempts := 0; attempts < geminiCachedContentCreateMaxAttempts; {
		selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), apiKey.GroupID, "", modelName, excluded, "", int64(0))
		if err != nil {
			if failoverClientGone(c) {
				return
			}
			reqLog.Info("gemini.cached_content.account_select_exhausted", zap.Error(err))
			break
		}
		account := selection.Account
		if !service.IsGeminiExplicitCacheAccount(account) {
			if selection.Acquired && selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			excluded[account.ID] = struct{}{}
			continue
		}
		ttl, ttlAllowed := req.TTLForAccount(account)
		if !ttlAllowed {
			if selection.Acquired && selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			excluded[account.ID] = struct{}{}
			ttlRejectedLimit = max(ttlRejectedLimit, account.GeminiExplicitCacheUpstreamMaxTTL())
			continue
		}
		accountReleaseFunc, ok := h.acquireGeminiCachedContentAccountSlot(c, reqLog, geminiConcurrency, selection)
		if !ok {
			return
		}
		admissionCtx := service.ContextWithSelectionProfitGate(c.Request.Context(), selection)
		latest, vetoed, reason := h.gatewayService.GatewayProfitControlVetoLatest(admissionCtx, account)
		if vetoed {
			accountReleaseFunc()
			reqLog.Debug("gemini.cached_content.account_profit_vetoed", zap.Int64("account_id", account.ID), zap.String("reason", reason))
			excluded[account.ID] = struct{}{}
			continue
		}
		account = latest
		attempts++
		setOpsSelectedAccount(c, account.ID, account.Platform)

		mappedModel := account.GetMappedModel(modelName)
		usageFields := clientRequestedUsageFields(c, channelMapping, reqModel, mappedModel)
		if !h.gatewayService.CacheStoragePriced(c.Request.Context(), &service.RecordUsageInput{
			Result:             &service.ForwardResult{Model: modelName, UpstreamModel: mappedModel, CacheStorageTokenHours: 1},
			APIKey:             apiKey,
			ChannelUsageFields: usageFields,
		}) {
			accountReleaseFunc()
			googlePlatformError(c, http.StatusBadRequest, fmt.Sprintf("Explicit context caching is not available for model %s: storage pricing is not configured", modelName))
			return
		}
		upstreamModel, err := service.GeminiCachedContentUpstreamModelResource(account, mappedModel)
		if err != nil {
			accountReleaseFunc()
			reqLog.Warn("gemini.cached_content.upstream_model_invalid", zap.Int64("account_id", account.ID), zap.Error(err))
			excluded[account.ID] = struct{}{}
			continue
		}
		publicID, err := service.NewGeminiCachedContentPublicID()
		if err != nil {
			accountReleaseFunc()
			googleError(c, http.StatusInternalServerError, "Failed to allocate cached content id")
			return
		}

		upstream, err := svc.CreateUpstream(c.Request.Context(), account, req, mappedModel, publicID, ttl)
		accountReleaseFunc()
		if err != nil {
			var upErr *service.GeminiCachedContentUpstreamError
			if errors.As(err, &upErr) {
				reqLog.Info("gemini.cached_content.upstream_create_failed", zap.Int64("account_id", account.ID), zap.Int("status", upErr.StatusCode))
				if upErr.RetryableOnOtherAccount() {
					lastUpstreamErr = upErr
					excluded[account.ID] = struct{}{}
					continue
				}
				h.writeGeminiCachedContentUpstreamError(c, upErr)
				return
			}
			// 上游已返回成功但响应不可用：不换号重建，能识别出资源名时删除上游缓存。
			var malformed *service.GeminiCachedContentMalformedResponseError
			if errors.As(err, &malformed) {
				reqLog.Error("gemini.cached_content.upstream_create_malformed", zap.Int64("account_id", account.ID), zap.Error(err))
				if malformed.Name != "" {
					h.cleanupGeminiCachedContentUpstream(reqLog, account, malformed.Name)
				}
				googleError(c, http.StatusBadGateway, "Upstream returned an invalid cached content response")
				return
			}
			if failoverClientGone(c) {
				return
			}
			reqLog.Warn("gemini.cached_content.upstream_create_error", zap.Int64("account_id", account.ID), zap.Error(err))
			excluded[account.ID] = struct{}{}
			continue
		}

		// 缓存 token 数在上游创建后才确定：按确定金额校验余额 / 额度并预留，不足时删除上游缓存，
		// 只收取已发生的缓存 token 输入费。
		tokens := upstream.TotalTokenCount
		storageTokenHours := service.GeminiCachedContentStorageTokenHours(tokens, ttl)
		amount := h.gatewayService.QuoteUsageCost(c.Request.Context(), &service.RecordUsageInput{
			Result: &service.ForwardResult{
				Model: modelName, UpstreamModel: mappedModel,
				Usage: service.ClaudeUsage{InputTokens: int(tokens)}, CacheStorageTokenHours: storageTokenHours,
			},
			APIKey: apiKey, User: apiKey.User, Subscription: subscription, PricingAt: pricingAt, ChannelUsageFields: usageFields,
		})
		inflightRelease()
		chargeDone, err := h.reserveGeminiCachedContentCharge(c, apiKey, subscription, amount)
		if err != nil {
			reqLog.Info("gemini.cached_content.storage_charge_rejected", zap.Float64("amount", amount), zap.Error(err))
			h.cleanupGeminiCachedContentUpstream(reqLog, account, upstream.Name)
			h.submitGeminiCachedContentUsage(c, apiKey, account, subscription, pricingAt,
				service.GeminiCachedContentCreateUsageRequestID(publicID), modelName, mappedModel, int(tokens), 0, usageFields, body)
			writeGeminiCachedContentBillingError(c, err)
			return
		}
		defer chargeDone()

		expireTime := service.ClampGeminiCachedContentExpire(upstream.ExpireTime, svc.Now().Add(ttl))
		record := &service.GeminiCachedContent{
			PublicID:        publicID,
			UserID:          authSubject.UserID,
			APIKeyID:        apiKey.ID,
			GroupID:         *apiKey.GroupID,
			AccountID:       account.ID,
			UpstreamName:    upstream.Name,
			Model:           modelName,
			UpstreamModel:   upstreamModel,
			DisplayName:     req.DisplayName,
			TotalTokenCount: tokens,
			ExpireTime:      expireTime,
			ChannelUsage:    usageFields,
		}
		persistCtx, cancelPersist := geminiCachedContentPersistContext(c)
		err = svc.Save(persistCtx, record)
		cancelPersist()
		if err != nil {
			reqLog.Error("gemini.cached_content.save_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			h.cleanupGeminiCachedContentUpstream(reqLog, account, record.UpstreamName)
			googleError(c, http.StatusInternalServerError, "Failed to persist cached content")
			return
		}

		h.submitGeminiCachedContentUsage(c, apiKey, account, subscription, pricingAt,
			service.GeminiCachedContentCreateUsageRequestID(publicID), modelName, mappedModel, int(tokens), storageTokenHours, usageFields, body)
		c.JSON(http.StatusOK, service.GeminiCachedContentView(record))
		return
	}

	if lastUpstreamErr != nil {
		h.writeGeminiCachedContentUpstreamError(c, lastUpstreamErr)
		return
	}
	if ttlRejectedLimit > 0 {
		googleError(c, http.StatusBadRequest, fmt.Sprintf("ttl exceeds the maximum allowed %ds by the upstream accounts", int64(ttlRejectedLimit.Seconds())))
		return
	}
	markOpsRoutingCapacityLimited(c)
	googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts support explicit context caching")
}

// GeminiCachedContentsList GET /v1beta/cachedContents
// 只返回当前 API Key 在当前分组下仍有效的缓存，不透传上游 list（同一账号上其他租户的缓存不可见）。
func (h *GatewayHandler) GeminiCachedContentsList(c *gin.Context) {
	apiKey, _, reqLog, ok := h.geminiCachedContentsPreamble(c, "handler.gemini_v1beta.cached_contents.list")
	if !ok {
		return
	}
	pageSize := service.GeminiCachedContentListDefaultPageSize
	if raw := strings.TrimSpace(c.Query("pageSize")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			googleError(c, http.StatusBadRequest, "pageSize must be a non-negative integer")
			return
		}
		if n > 0 {
			pageSize = min(n, service.GeminiCachedContentListMaxPageSize)
		}
	}
	beforeID := int64(0)
	if raw := strings.TrimSpace(c.Query("pageToken")); raw != "" {
		id, ok := decodeGeminiCachedContentPageToken(raw)
		if !ok {
			googleError(c, http.StatusBadRequest, "Invalid pageToken")
			return
		}
		beforeID = id
	}
	records, err := h.geminiCachedContentService.List(c.Request.Context(), apiKey.ID, *apiKey.GroupID, beforeID, pageSize+1)
	if err != nil {
		reqLog.Error("gemini.cached_content.list_failed", zap.Error(err))
		googleError(c, http.StatusInternalServerError, "Failed to list cached contents")
		return
	}
	resp := gin.H{}
	if len(records) > pageSize {
		records = records[:pageSize]
		resp["nextPageToken"] = encodeGeminiCachedContentPageToken(records[len(records)-1].ID)
	}
	if len(records) > 0 {
		views := make([]map[string]any, 0, len(records))
		for _, record := range records {
			views = append(views, service.GeminiCachedContentView(record))
		}
		resp["cachedContents"] = views
	}
	c.JSON(http.StatusOK, resp)
}

// GeminiCachedContentsGet GET /v1beta/cachedContents/:cacheID
func (h *GatewayHandler) GeminiCachedContentsGet(c *gin.Context) {
	apiKey, _, reqLog, ok := h.geminiCachedContentsPreamble(c, "handler.gemini_v1beta.cached_contents.get")
	if !ok {
		return
	}
	record, ok := h.resolveGeminiCachedContentOrWrite(c, reqLog, apiKey, c.Param("cacheID"))
	if !ok {
		return
	}
	c.JSON(http.StatusOK, service.GeminiCachedContentView(record))
}

// GeminiCachedContentsPatch PATCH /v1beta/cachedContents/:cacheID
// 只允许修改有效期；延长部分在本次请求内按增量时长收取存储费，缩短不退费。
func (h *GatewayHandler) GeminiCachedContentsPatch(c *gin.Context) {
	apiKey, _, reqLog, ok := h.geminiCachedContentsPreamble(c, "handler.gemini_v1beta.cached_contents.patch")
	if !ok {
		return
	}
	svc := h.geminiCachedContentService
	body, ok := readGeminiCachedContentBody(c)
	if !ok {
		return
	}
	record, ok := h.resolveGeminiCachedContentOrWrite(c, reqLog, apiKey, c.Param("cacheID"))
	if !ok {
		return
	}
	account, err := svc.BoundAccount(c.Request.Context(), record)
	if err != nil {
		reqLog.Error("gemini.cached_content.load_account_failed", zap.Int64("account_id", record.AccountID), zap.Error(err))
		googleError(c, http.StatusInternalServerError, "Failed to load cached content account")
		return
	}
	if account == nil {
		googleError(c, http.StatusServiceUnavailable, geminiCachedContentUnavailableMessage)
		return
	}
	ttl, err := service.ParseGeminiCachedContentPatchRequest(body, c.Query("updateMask"), svc.Now(), svc.MaxTTLForAccount(account))
	if err != nil {
		googleError(c, http.StatusBadRequest, err.Error())
		return
	}
	setOpsRequestContext(c, record.Model, false)
	pricingCtx, pricingAt := service.WithGatewayTokenRequestPricing(c.Request.Context())
	c.Request = c.Request.WithContext(pricingCtx)

	subscription, _ := middleware.GetSubscriptionFromContext(c)
	if extension := svc.Now().Add(ttl).Sub(record.ExpireTime); extension > 0 {
		usage := &service.RecordUsageInput{
			Result: &service.ForwardResult{
				Model: record.Model, UpstreamModel: record.UpstreamModelID(),
				CacheStorageTokenHours: service.GeminiCachedContentStorageTokenHours(record.TotalTokenCount, extension),
			},
			APIKey: apiKey, User: apiKey.User, Subscription: subscription, PricingAt: pricingAt,
			ChannelUsageFields: record.ChannelUsage,
		}
		if !h.gatewayService.CacheStoragePriced(c.Request.Context(), usage) {
			googlePlatformError(c, http.StatusBadRequest, fmt.Sprintf("Explicit context caching is not available for model %s: storage pricing is not configured", record.Model))
			return
		}
		if !h.checkGeminiCachedContentBilling(c, reqLog, apiKey, subscription) {
			return
		}
		chargeDone, err := h.reserveGeminiCachedContentCharge(c, apiKey, subscription, h.gatewayService.QuoteUsageCost(c.Request.Context(), usage))
		if err != nil {
			reqLog.Info("gemini.cached_content.storage_charge_rejected", zap.Error(err))
			writeGeminiCachedContentBillingError(c, err)
			return
		}
		defer chargeDone()
	}

	setOpsSelectedAccount(c, account.ID, account.Platform)
	upstream, err := svc.PatchUpstream(c.Request.Context(), account, record, ttl)
	if err != nil {
		var upErr *service.GeminiCachedContentUpstreamError
		if errors.As(err, &upErr) {
			if upErr.NotFound() {
				if forgetErr := svc.Forget(c.Request.Context(), record); forgetErr != nil {
					reqLog.Warn("gemini.cached_content.forget_failed", zap.Error(forgetErr))
				}
			}
			h.writeGeminiCachedContentUpstreamError(c, upErr)
			return
		}
		reqLog.Warn("gemini.cached_content.upstream_patch_error", zap.Int64("account_id", account.ID), zap.Error(err))
		googleError(c, http.StatusBadGateway, "Upstream request failed")
		return
	}
	expireTime := service.ClampGeminiCachedContentExpire(upstream.ExpireTime, svc.Now().Add(ttl))
	persistCtx, cancelPersist := geminiCachedContentPersistContext(c)
	previousExpire, err := svc.UpdateExpireTime(persistCtx, record, expireTime)
	cancelPersist()
	if err != nil {
		reqLog.Error("gemini.cached_content.update_expire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		if errors.Is(err, service.ErrGeminiCachedContentNotFound) {
			googleError(c, http.StatusForbidden, service.GeminiCachedContentNotFoundMessage)
			return
		}
		googleError(c, http.StatusInternalServerError, "Failed to persist cached content expiration")
		return
	}
	// 按库中实际推进的时长收费：并发延长各自只为自己推进的部分付费。
	if extended := expireTime.Sub(previousExpire); extended > 0 {
		h.submitGeminiCachedContentUsage(c, apiKey, account, subscription, pricingAt,
			service.GeminiCachedContentPatchUsageRequestID(record.PublicID), record.Model, record.UpstreamModelID(), 0,
			service.GeminiCachedContentStorageTokenHours(record.TotalTokenCount, extended), record.ChannelUsage, body)
	}
	c.JSON(http.StatusOK, service.GeminiCachedContentView(record))
}

// GeminiCachedContentsDelete DELETE /v1beta/cachedContents/:cacheID
// 删除不退还已收取的存储费。
func (h *GatewayHandler) GeminiCachedContentsDelete(c *gin.Context) {
	apiKey, _, reqLog, ok := h.geminiCachedContentsPreamble(c, "handler.gemini_v1beta.cached_contents.delete")
	if !ok {
		return
	}
	svc := h.geminiCachedContentService
	record, ok := h.resolveGeminiCachedContentOrWrite(c, reqLog, apiKey, c.Param("cacheID"))
	if !ok {
		return
	}
	account, err := svc.BoundAccount(c.Request.Context(), record)
	if err != nil {
		reqLog.Error("gemini.cached_content.load_account_failed", zap.Int64("account_id", record.AccountID), zap.Error(err))
		googleError(c, http.StatusInternalServerError, "Failed to load cached content account")
		return
	}
	if account != nil {
		setOpsSelectedAccount(c, account.ID, account.Platform)
		if err := svc.DeleteUpstream(c.Request.Context(), account, record); err != nil {
			var upErr *service.GeminiCachedContentUpstreamError
			if !errors.As(err, &upErr) {
				reqLog.Warn("gemini.cached_content.upstream_delete_error", zap.Int64("account_id", account.ID), zap.Error(err))
				googleError(c, http.StatusBadGateway, "Upstream request failed")
				return
			}
			if !upErr.NotFound() {
				h.writeGeminiCachedContentUpstreamError(c, upErr)
				return
			}
		}
	}
	persistCtx, cancelPersist := geminiCachedContentPersistContext(c)
	err = svc.Forget(persistCtx, record)
	cancelPersist()
	if err != nil {
		reqLog.Error("gemini.cached_content.forget_failed", zap.Error(err))
		googleError(c, http.StatusInternalServerError, "Failed to delete cached content")
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

func (h *GatewayHandler) resolveGeminiCachedContentOrWrite(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, cacheID string) (*service.GeminiCachedContent, bool) {
	record, err := h.geminiCachedContentService.Resolve(c.Request.Context(), apiKey.ID, apiKey.GroupID, cacheID)
	if err != nil {
		if errors.Is(err, service.ErrGeminiCachedContentNotFound) {
			googleError(c, http.StatusForbidden, service.GeminiCachedContentNotFoundMessage)
			return nil, false
		}
		reqLog.Error("gemini.cached_content.lookup_failed", zap.Error(err))
		googleError(c, http.StatusInternalServerError, "Failed to load cached content")
		return nil, false
	}
	return record, true
}

// resolveGeminiCachedContentBinding 处理 generateContent / countTokens 请求对显式缓存的引用：
// 校验归属与模型后把引用改写为上游资源名，返回绑定记录；请求未引用缓存时返回 nil。
// 返回 false 表示已写出错误响应。
func (h *GatewayHandler) resolveGeminiCachedContentBinding(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, modelName string, body []byte) (*service.GeminiCachedContent, []byte, bool) {
	ref, err := service.FindGeminiCachedContentReference(body)
	if err != nil {
		googleError(c, http.StatusBadRequest, err.Error())
		return nil, nil, false
	}
	if ref == nil {
		return nil, body, true
	}
	if middleware.HasForcePlatform(c) || h.geminiCachedContentService == nil ||
		apiKey.Group == nil || apiKey.Group.Platform == service.PlatformComposite {
		googleError(c, http.StatusBadRequest, "cachedContent is not supported on this route")
		return nil, nil, false
	}
	record, err := h.geminiCachedContentService.Resolve(c.Request.Context(), apiKey.ID, apiKey.GroupID, ref.Name)
	if err != nil {
		if errors.Is(err, service.ErrGeminiCachedContentNotFound) {
			googleError(c, http.StatusForbidden, service.GeminiCachedContentNotFoundMessage)
			return nil, nil, false
		}
		reqLog.Error("gemini.cached_content.lookup_failed", zap.Error(err))
		googleError(c, http.StatusInternalServerError, "Failed to load cached content")
		return nil, nil, false
	}
	if record.Model != modelName {
		googleError(c, http.StatusBadRequest, fmt.Sprintf(
			"Model used by GenerateContent request (models/%s) and CachedContent (models/%s) has to be the same.", modelName, record.Model))
		return nil, nil, false
	}
	rewritten, err := service.RewriteGeminiCachedContentReference(body, ref, record.UpstreamName)
	if err != nil {
		googleError(c, http.StatusBadRequest, "Invalid cachedContent reference")
		return nil, nil, false
	}
	return record, rewritten, true
}

// acquireGeminiCachedContentAccountSlot 获取选中账号的并发槽（含等待计划）；返回 false 表示已写出错误响应。
func (h *GatewayHandler) acquireGeminiCachedContentAccountSlot(c *gin.Context, reqLog *zap.Logger, helper *ConcurrencyHelper, selection *service.AccountSelectionResult) (func(), bool) {
	if selection.Acquired {
		release := selection.ReleaseFunc
		if release == nil {
			release = func() {}
		}
		return wrapReleaseOnDone(c.Request.Context(), release), true
	}
	account := selection.Account
	if selection.WaitPlan == nil {
		markOpsRoutingCapacityLimited(c)
		googleError(c, http.StatusServiceUnavailable, "No available Gemini accounts")
		return nil, false
	}
	canWait, err := helper.IncrementAccountWaitCount(c.Request.Context(), account.ID, selection.WaitPlan.MaxWaiting)
	if err != nil {
		reqLog.Warn("gemini.cached_content.account_wait_counter_increment_failed", zap.Int64("account_id", account.ID), zap.Error(err))
	} else if !canWait {
		googleError(c, http.StatusTooManyRequests, "Too many pending requests, please retry later")
		return nil, false
	}
	streamStarted := false
	release, acquireErr := helper.AcquireAccountSlotWithWaitTimeout(c, account.ID, selection.WaitPlan.MaxConcurrency, selection.WaitPlan.Timeout, false, &streamStarted)
	if err == nil {
		helper.DecrementAccountWaitCount(c.Request.Context(), account.ID)
	}
	if acquireErr != nil {
		reqLog.Warn("gemini.cached_content.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(acquireErr))
		googleConcurrencyError(c, acquireErr, "account")
		return nil, false
	}
	if release == nil {
		release = func() {}
	}
	return wrapReleaseOnDone(c.Request.Context(), release), true
}

// submitGeminiCachedContentUsage 记录缓存创建 / 延长的计费：inputTokens 按输入价计，storageTokenHours 按计费模型的存储单价计；
// requestID 是带固定前缀的费用行标识（见 GeminiCachedContentUsageRequestIDPrefix），落库时不被请求 id 覆盖。
func (h *GatewayHandler) submitGeminiCachedContentUsage(
	c *gin.Context,
	apiKey *service.APIKey,
	account *service.Account,
	subscription *service.UserSubscription,
	pricingAt time.Time,
	requestID string,
	model string,
	upstreamModel string,
	inputTokens int,
	storageTokenHours float64,
	usageFields service.ChannelUsageFields,
	payload []byte,
) {
	if inputTokens <= 0 && storageTokenHours <= 0 {
		return
	}
	if upstreamModel == model {
		upstreamModel = ""
	}
	result := &service.ForwardResult{
		RequestID:              requestID,
		Model:                  model,
		UpstreamModel:          upstreamModel,
		Usage:                  service.ClaudeUsage{InputTokens: inputTokens},
		CacheStorageTokenHours: storageTokenHours,
	}
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	requestPayloadHash := service.HashUsageRequestPayload(payload)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
			Result:             result,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			PricingAt:          pricingAt,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: requestPayloadHash,
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
			ChannelUsageFields: usageFields,
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.gemini_v1beta.cached_contents"),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Int64("account_id", account.ID),
				zap.String("request_id", result.RequestID),
			).Error("gemini.cached_content.record_usage_failed", zap.Error(err))
		}
	})
}

// geminiCachedContentPersistContext 返回与客户端连接脱钩的落库 context：上游操作已生效时，客户端断开不应让本地记录缺失。
func geminiCachedContentPersistContext(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(c.Request.Context()), geminiCachedContentPersistTimeout)
}

// cleanupGeminiCachedContentUpstream 尽力删除已在上游创建、但网关不会登记的缓存，避免其持续产生存储费。
func (h *GatewayHandler) cleanupGeminiCachedContentUpstream(reqLog *zap.Logger, account *service.Account, upstreamName string) {
	ctx, cancel := context.WithTimeout(context.Background(), geminiCachedContentPersistTimeout)
	defer cancel()
	if err := h.geminiCachedContentService.DeleteUpstream(ctx, account, &service.GeminiCachedContent{UpstreamName: upstreamName}); err != nil {
		reqLog.Error("gemini.cached_content.orphan_cleanup_failed", zap.Int64("account_id", account.ID), zap.String("upstream_name", upstreamName), zap.Error(err))
	}
}

// reserveGeminiCachedContentCharge 校验余额 / 订阅额度 / API Key 额度足以支付确定金额 amount（倍率后），
// 并按该金额登记在途预留（不受预留上限封顶）。返回的 done 须在 handler 结束时调用。
func (h *GatewayHandler) reserveGeminiCachedContentCharge(c *gin.Context, apiKey *service.APIKey, subscription *service.UserSubscription, amount float64) (func(), error) {
	ctx := c.Request.Context()
	if err := h.billingCacheService.CheckChargeAffordable(ctx, apiKey.User, apiKey, apiKey.Group, subscription, amount); err != nil {
		return inflightNoop, err
	}
	res, err := h.billingCacheService.ReserveInflightCharge(ctx, apiKey.User, apiKey.Group, subscription, amount)
	if err != nil {
		return inflightNoop, err
	}
	if res == nil {
		return inflightNoop, nil
	}
	c.Request = c.Request.WithContext(service.WithInflightReservation(ctx, res))
	return res.HandlerDone, nil
}

func writeGeminiCachedContentBillingError(c *gin.Context, err error) {
	status, code, message, retryAfter := billingErrorDetails(err)
	if retryAfter > 0 {
		c.Header("Retry-After", strconv.Itoa(retryAfter))
	}
	googleErrorWithType(c, status, code, "", message)
}

// writeGeminiCachedContentUpstreamError 回写上游错误：缓存不存在统一返回标准 403（各上游形态不同且可能含上游资源 ID）；
// 账号凭据或权限类错误不是客户端问题，统一改为 502；限流 / 不可用按普通 Gemini 路径处理（透传规则、通用映射），
// 只有上游网关账号的这类响应反映持有缓存的上游账号状态，原样回写。
func (h *GatewayHandler) writeGeminiCachedContentUpstreamError(c *gin.Context, upErr *service.GeminiCachedContentUpstreamError) {
	status := upErr.StatusCode
	message := strings.TrimSpace(extractGoogleErrorMessage(upErr.Body))
	service.SetOpsUpstreamError(c, status, message, "")
	if upErr.NotFound() {
		service.MarkOpsRequestScopedError(c, "permission_error")
		c.Data(http.StatusForbidden, "application/json", []byte(service.GeminiCachedContentNotFoundResponse))
		return
	}
	if (status == http.StatusTooManyRequests || status >= http.StatusInternalServerError) && !upErr.UpstreamGateway {
		h.handleGeminiFailoverExhausted(c, &service.UpstreamFailoverError{StatusCode: status, ResponseBody: upErr.Body})
		return
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		googleError(c, http.StatusBadGateway, "Upstream account rejected the cached content request")
		return
	}
	if len(upErr.Body) == 0 || !json.Valid(upErr.Body) {
		if message == "" {
			message = http.StatusText(status)
		}
		googleError(c, status, message)
		return
	}
	c.Data(status, "application/json", upErr.Body)
}

func extractGoogleErrorMessage(body []byte) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	return envelope.Error.Message
}

func encodeGeminiCachedContentPageToken(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}

func decodeGeminiCachedContentPageToken(raw string) (int64, bool) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return 0, false
	}
	id, err := strconv.ParseInt(string(decoded), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// mergeAccountIDSets 返回两个账号 ID 集合的并集（不修改入参）。
func mergeAccountIDSets(a, b map[int64]struct{}) map[int64]struct{} {
	out := make(map[int64]struct{}, len(a)+len(b))
	for id := range a {
		out[id] = struct{}{}
	}
	for id := range b {
		out[id] = struct{}{}
	}
	return out
}

// handleGeminiCachedContentFailoverExhausted 处理绑定缓存的请求在持有账号上重试耗尽：没有其他账号可换。
// 上游网关账号的限流 / 不可用响应反映持有缓存的上游账号状态，原样返回供客户端判断；
// 其余（含官方账号）仍按普通 Gemini 路径处理：透传规则、通用映射。
func (h *GatewayHandler) handleGeminiCachedContentFailoverExhausted(c *gin.Context, failoverErr *service.UpstreamFailoverError) {
	if failoverErr == nil || !failoverErr.BoundUpstreamPassthrough || len(failoverErr.ResponseBody) == 0 || !json.Valid(failoverErr.ResponseBody) {
		h.handleGeminiFailoverExhausted(c, failoverErr)
		return
	}
	service.SetOpsUpstreamError(c, failoverErr.StatusCode, service.ExtractUpstreamErrorMessage(failoverErr.ResponseBody), "")
	c.Data(failoverErr.StatusCode, "application/json", failoverErr.ResponseBody)
}
