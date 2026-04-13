package handler

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const gatewayCompatibilityMetricsLogInterval = 1024

var gatewayCompatibilityMetricsLogCounter atomic.Uint64

// GatewayHandler handles API gateway requests
type GatewayHandler struct {
	gatewayService            *service.GatewayService
	geminiCompatService       *service.GeminiMessagesCompatService
	antigravityGatewayService *service.AntigravityGatewayService
	userService               *service.UserService
	billingCacheService       *service.BillingCacheService
	usageService              *service.UsageService
	apiKeyService             *service.APIKeyService
	usageRecordWorkerPool     *service.UsageRecordWorkerPool
	errorPassthroughService   *service.ErrorPassthroughService
	concurrencyHelper         *ConcurrencyHelper
	userMsgQueueHelper        *UserMsgQueueHelper
	maxAccountSwitches        int
	maxAccountSwitchesGemini  int
	cfg                       *config.Config
	settingService            *service.SettingService
}

// NewGatewayHandler creates a new GatewayHandler
func NewGatewayHandler(
	gatewayService *service.GatewayService,
	geminiCompatService *service.GeminiMessagesCompatService,
	antigravityGatewayService *service.AntigravityGatewayService,
	userService *service.UserService,
	concurrencyService *service.ConcurrencyService,
	billingCacheService *service.BillingCacheService,
	usageService *service.UsageService,
	apiKeyService *service.APIKeyService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	errorPassthroughService *service.ErrorPassthroughService,
	userMsgQueueService *service.UserMessageQueueService,
	cfg *config.Config,
	settingService *service.SettingService,
) *GatewayHandler {
	pingInterval := time.Duration(0)
	maxAccountSwitches := 10
	maxAccountSwitchesGemini := 3
	if cfg != nil {
		pingInterval = time.Duration(cfg.Concurrency.PingInterval) * time.Second
		if cfg.Gateway.MaxAccountSwitches > 0 {
			maxAccountSwitches = cfg.Gateway.MaxAccountSwitches
		}
		if cfg.Gateway.MaxAccountSwitchesGemini > 0 {
			maxAccountSwitchesGemini = cfg.Gateway.MaxAccountSwitchesGemini
		}
	}

	// 初始化用户消息串行队列 helper
	var umqHelper *UserMsgQueueHelper
	if userMsgQueueService != nil && cfg != nil {
		umqHelper = NewUserMsgQueueHelper(userMsgQueueService, SSEPingFormatClaude, pingInterval)
	}

	return &GatewayHandler{
		gatewayService:            gatewayService,
		geminiCompatService:       geminiCompatService,
		antigravityGatewayService: antigravityGatewayService,
		userService:               userService,
		billingCacheService:       billingCacheService,
		usageService:              usageService,
		apiKeyService:             apiKeyService,
		usageRecordWorkerPool:     usageRecordWorkerPool,
		errorPassthroughService:   errorPassthroughService,
		concurrencyHelper:         NewConcurrencyHelper(concurrencyService, SSEPingFormatClaude, pingInterval),
		userMsgQueueHelper:        umqHelper,
		maxAccountSwitches:        maxAccountSwitches,
		maxAccountSwitchesGemini:  maxAccountSwitchesGemini,
		cfg:                       cfg,
		settingService:            settingService,
	}
}

// Messages handles Claude API compatible messages endpoint
// POST /v1/messages
func (h *GatewayHandler) Messages(c *gin.Context) {
	// 从context获取apiKey和user（ApiKeyAuth中间件已设置）
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.gateway.messages",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	defer h.maybeLogCompatibilityFallbackMetrics(reqLog)

	// 读取请求体
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	setOpsRequestContext(c, "", false, body)

	parsedReq, err := service.ParseGatewayRequest(body, domain.PlatformAnthropic)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}
	reqModel := parsedReq.Model
	reqStream := parsedReq.Stream
	reqLog = reqLog.With(zap.String("model", reqModel), zap.Bool("stream", reqStream))

	// 解析渠道级模型映射
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)

	// 设置 max_tokens=1 + haiku 探测请求标识到 context 中
	// 必须在 SetClaudeCodeClientContext 之前设置，因为 ClaudeCodeValidator 需要读取此标识进行绕过判断
	if isMaxTokensOneHaikuRequest(reqModel, parsedReq.MaxTokens, reqStream) {
		ctx := service.WithIsMaxTokensOneHaikuRequest(c.Request.Context(), true, h.metadataBridgeEnabled())
		c.Request = c.Request.WithContext(ctx)
	}

	// 检查是否为 Claude Code 客户端，设置到 context 中（复用已解析请求，避免二次反序列化）。
	SetClaudeCodeClientContext(c, body, parsedReq)
	isClaudeCodeClient := service.IsClaudeCodeClient(c.Request.Context())

	// 版本检查：仅对 Claude Code 客户端，拒绝低于最低版本的请求
	if !h.checkClaudeCodeVersion(c) {
		return
	}

	// 在请求上下文中记录 thinking 状态，供 Antigravity 最终模型 key 推导/模型维度限流使用
	c.Request = c.Request.WithContext(service.WithThinkingEnabled(c.Request.Context(), parsedReq.ThinkingEnabled, h.metadataBridgeEnabled()))

	setOpsRequestContext(c, reqModel, reqStream, body)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(reqStream, false)))

	// 验证 model 必填
	if reqModel == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}

	// Track if we've started streaming (for error handling)
	streamStarted := false

	// 绑定错误透传服务，允许 service 层在非 failover 错误场景复用规则。
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	// 获取订阅信息（可能为nil）- 提前获取用于后续检查
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	// 0. 检查wait队列是否已满
	maxWait := service.CalculateMaxWait(subject.Concurrency)
	canWait, err := h.concurrencyHelper.IncrementWaitCount(c.Request.Context(), subject.UserID, maxWait)
	waitCounted := false
	if err != nil {
		reqLog.Warn("gateway.user_wait_counter_increment_failed", zap.Error(err))
		// On error, allow request to proceed
	} else if !canWait {
		reqLog.Info("gateway.user_wait_queue_full", zap.Int("max_wait", maxWait))
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later")
		return
	}
	if err == nil && canWait {
		waitCounted = true
	}
	// Ensure we decrement if we exit before acquiring the user slot.
	defer func() {
		if waitCounted {
			h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), subject.UserID)
		}
	}()

	// 1. 首先获取用户并发槽位
	userReleaseFunc, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, reqStream, &streamStarted)
	if err != nil {
		reqLog.Warn("gateway.user_slot_acquire_failed", zap.Error(err))
		h.handleConcurrencyError(c, err, "user", streamStarted)
		return
	}
	// User slot acquired: no longer waiting in the queue.
	if waitCounted {
		h.concurrencyHelper.DecrementWaitCount(c.Request.Context(), subject.UserID)
		waitCounted = false
	}
	// 在请求结束或 Context 取消时确保释放槽位，避免客户端断开造成泄漏
	userReleaseFunc = wrapReleaseOnDone(c.Request.Context(), userReleaseFunc)
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	// 2. 【新增】Wait后二次检查余额/订阅
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		reqLog.Info("gateway.billing_eligibility_check_failed", zap.Error(err))
		status, code, message := billingErrorDetails(err)
		h.handleStreamingAwareError(c, status, code, message, streamStarted)
		return
	}

	// 设置请求所属分组 ID（用于渠道级功能判断，如 WebSearch 模拟）
	parsedReq.GroupID = apiKey.GroupID

	// 计算粘性会话hash
	parsedReq.SessionContext = &service.SessionContext{
		ClientIP:  ip.GetClientIP(c),
		UserAgent: c.GetHeader("User-Agent"),
		APIKeyID:  apiKey.ID,
	}
	sessionHash := h.gatewayService.GenerateSessionHash(parsedReq)

	// 获取平台：优先使用强制平台（/antigravity 路由，中间件已设置 request.Context），否则使用分组平台
	platform := ""
	if forcePlatform, ok := middleware2.GetForcePlatformFromContext(c); ok {
		platform = forcePlatform
	} else if apiKey.Group != nil {
		platform = apiKey.Group.Platform
	}
	sessionKey := sessionHash
	if platform == service.PlatformGemini && sessionHash != "" {
		sessionKey = "gemini:" + sessionHash
	}

	// 查询粘性会话绑定的账号 ID
	var sessionBoundAccountID int64
	if sessionKey != "" {
		sessionBoundAccountID, _ = h.gatewayService.GetCachedSessionAccountID(c.Request.Context(), apiKey.GroupID, sessionKey)
		if sessionBoundAccountID > 0 {
			prefetchedGroupID := int64(0)
			if apiKey.GroupID != nil {
				prefetchedGroupID = *apiKey.GroupID
			}
			ctx := service.WithPrefetchedStickySession(c.Request.Context(), sessionBoundAccountID, prefetchedGroupID, h.metadataBridgeEnabled())
			c.Request = c.Request.WithContext(ctx)
		}
	}
	// 判断是否真的绑定了粘性会话：有 sessionKey 且已经绑定到某个账号
	hasBoundSession := sessionKey != "" && sessionBoundAccountID > 0

	if platform == service.PlatformGemini {
		fs := NewFailoverState(h.maxAccountSwitchesGemini, hasBoundSession)

		// 单账号分组提前设置 SingleAccountRetry 标记，让 Service 层首次 503 就不设模型限流标记。
		// 避免单账号分组收到 503 (MODEL_CAPACITY_EXHAUSTED) 时设 29s 限流，导致后续请求连续快速失败。
		if h.gatewayService.IsSingleAntigravityAccountGroup(c.Request.Context(), apiKey.GroupID) {
			ctx := service.WithSingleAccountRetry(c.Request.Context(), true, h.metadataBridgeEnabled())
			c.Request = c.Request.WithContext(ctx)
		}

		for {
			selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), apiKey.GroupID, sessionKey, reqModel, fs.FailedAccountIDs, "", int64(0)) // Gemini 不使用会话限制
			if err != nil {
				if len(fs.FailedAccountIDs) == 0 {
					h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts: "+err.Error(), streamStarted)
					return
				}
				action := fs.HandleSelectionExhausted(c.Request.Context())
				switch action {
				case FailoverContinue:
					ctx := service.WithSingleAccountRetry(c.Request.Context(), true, h.metadataBridgeEnabled())
					c.Request = c.Request.WithContext(ctx)
					continue
				case FailoverCanceled:
					return
				default: // FailoverExhausted
					if fs.LastFailoverErr != nil {
						h.handleFailoverExhausted(c, fs.LastFailoverErr, service.PlatformGemini, streamStarted)
					} else {
						h.handleFailoverExhaustedSimple(c, 502, streamStarted)
					}
					return
				}
			}
			account := selection.Account
			setOpsSelectedAccount(c, account.ID, account.Platform)

			// 检查请求拦截（预热请求、SUGGESTION MODE等）
			if account.IsInterceptWarmupEnabled() {
				interceptType := detectInterceptType(body, reqModel, parsedReq.MaxTokens, reqStream, isClaudeCodeClient)
				if interceptType != InterceptTypeNone {
					if selection.Acquired && selection.ReleaseFunc != nil {
						selection.ReleaseFunc()
					}
					if reqStream {
						sendMockInterceptStream(c, reqModel, interceptType)
					} else {
						sendMockInterceptResponse(c, reqModel, interceptType)
					}
					return
				}
			}

			// 3. 获取账号并发槽位
			accountReleaseFunc := selection.ReleaseFunc
			if !selection.Acquired {
				if selection.WaitPlan == nil {
					h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", streamStarted)
					return
				}
				accountWaitCounted := false
				canWait, err := h.concurrencyHelper.IncrementAccountWaitCount(c.Request.Context(), account.ID, selection.WaitPlan.MaxWaiting)
				if err != nil {
					reqLog.Warn("gateway.account_wait_counter_increment_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				} else if !canWait {
					reqLog.Info("gateway.account_wait_queue_full",
						zap.Int64("account_id", account.ID),
						zap.Int("max_waiting", selection.WaitPlan.MaxWaiting),
					)
					h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later", streamStarted)
					return
				}
				if err == nil && canWait {
					accountWaitCounted = true
				}
				releaseWait := func() {
					if accountWaitCounted {
						h.concurrencyHelper.DecrementAccountWaitCount(c.Request.Context(), account.ID)
						accountWaitCounted = false
					}
				}

				accountReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
					c,
					account.ID,
					selection.WaitPlan.MaxConcurrency,
					selection.WaitPlan.Timeout,
					reqStream,
					&streamStarted,
				)
				if err != nil {
					reqLog.Warn("gateway.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
					releaseWait()
					h.handleConcurrencyError(c, err, "account", streamStarted)
					return
				}
				// Slot acquired: no longer waiting in queue.
				releaseWait()
				if err := h.gatewayService.BindStickySession(c.Request.Context(), apiKey.GroupID, sessionKey, account.ID); err != nil {
					reqLog.Warn("gateway.bind_sticky_session_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				}
			}
			// 账号槽位/等待计数需要在超时或断开时安全回收
			accountReleaseFunc = wrapReleaseOnDone(c.Request.Context(), accountReleaseFunc)

			// 转发请求 - 根据账号平台分流
			var result *service.ForwardResult
			requestCtx := c.Request.Context()
			if fs.SwitchCount > 0 {
				requestCtx = service.WithAccountSwitchCount(requestCtx, fs.SwitchCount, h.metadataBridgeEnabled())
			}
			// 记录 Forward 前已写入字节数，Forward 后若增加则说明 SSE 内容已发，禁止 failover
			writerSizeBeforeForward := c.Writer.Size()
			if account.Platform == service.PlatformAntigravity {
				result, err = h.antigravityGatewayService.ForwardGemini(requestCtx, c, account, reqModel, "generateContent", reqStream, body, hasBoundSession)
			} else {
				result, err = h.geminiCompatService.Forward(requestCtx, c, account, body)
			}
			if accountReleaseFunc != nil {
				accountReleaseFunc()
			}
			if err != nil {
				var failoverErr *service.UpstreamFailoverError
				if errors.As(err, &failoverErr) {
					// 流式内容已写入客户端，无法撤销，禁止 failover 以防止流拼接腐化
					if c.Writer.Size() != writerSizeBeforeForward {
						h.handleFailoverExhausted(c, failoverErr, service.PlatformGemini, true)
						return
					}
					action := fs.HandleFailoverError(c.Request.Context(), h.gatewayService, account.ID, account.Platform, failoverErr)
					switch action {
					case FailoverContinue:
						continue
					case FailoverExhausted:
						h.handleFailoverExhausted(c, fs.LastFailoverErr, service.PlatformGemini, streamStarted)
						return
					case FailoverCanceled:
						return
					}
				}
				wroteFallback := h.ensureForwardErrorResponse(c, streamStarted)
				forwardFailedFields := []zap.Field{
					zap.Int64("account_id", account.ID),
					zap.String("account_name", account.Name),
					zap.String("account_platform", account.Platform),
					zap.Bool("fallback_error_response_written", wroteFallback),
					zap.Error(err),
				}
				if account.Proxy != nil {
					forwardFailedFields = append(forwardFailedFields,
						zap.Int64("proxy_id", account.Proxy.ID),
						zap.String("proxy_name", account.Proxy.Name),
						zap.String("proxy_host", account.Proxy.Host),
						zap.Int("proxy_port", account.Proxy.Port),
					)
				} else if account.ProxyID != nil {
					forwardFailedFields = append(forwardFailedFields, zap.Int64p("proxy_id", account.ProxyID))
				}
				reqLog.Error("gateway.forward_failed", forwardFailedFields...)
				return
			}

			// RPM 计数递增（Forward 成功后）
			// 注意：TOCTOU 竞态是已知且可接受的设计权衡，与 WindowCost 一致的 soft-limit 模式。
			// 在高并发下可能短暂超出 RPM 限制，但不会导致请求失败。
			if account.IsAnthropicOAuthOrSetupToken() && account.GetBaseRPM() > 0 {
				if err := h.gatewayService.IncrementAccountRPM(c.Request.Context(), account.ID); err != nil {
					reqLog.Warn("gateway.rpm_increment_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				}
			}

			// 捕获请求信息（用于异步记录，避免在 goroutine 中访问 gin.Context）
			userAgent := c.GetHeader("User-Agent")
			clientIP := ip.GetClientIP(c)
			requestPayloadHash := service.HashUsageRequestPayload(body)
			inboundEndpoint := GetInboundEndpoint(c)
			upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)

			if result.ReasoningEffort == nil {
				result.ReasoningEffort = service.NormalizeClaudeOutputEffort(parsedReq.OutputEffort)
			}

			// 使用量记录通过有界 worker 池提交，避免请求热路径创建无界 goroutine。
			h.submitUsageRecordTask(func(ctx context.Context) {
				if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
					Result:             result,
					ParsedRequest:      parsedReq,
					APIKey:             apiKey,
					User:               apiKey.User,
					Account:            account,
					Subscription:       subscription,
					InboundEndpoint:    inboundEndpoint,
					UpstreamEndpoint:   upstreamEndpoint,
					UserAgent:          userAgent,
					IPAddress:          clientIP,
					RequestPayloadHash: requestPayloadHash,
					ForceCacheBilling:  fs.ForceCacheBilling,
					APIKeyService:      h.apiKeyService,
					ChannelUsageFields: channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
				}); err != nil {
					logger.L().With(
						zap.String("component", "handler.gateway.messages"),
						zap.Int64("user_id", subject.UserID),
						zap.Int64("api_key_id", apiKey.ID),
						zap.Any("group_id", apiKey.GroupID),
						zap.String("model", reqModel),
						zap.Int64("account_id", account.ID),
					).Error("gateway.record_usage_failed", zap.Error(err))
				}
			})
			return
		}
	}

	currentAPIKey := apiKey
	currentSubscription := subscription
	var fallbackGroupID *int64
	if apiKey.Group != nil {
		fallbackGroupID = apiKey.Group.FallbackGroupIDOnInvalidRequest
	}
	fallbackUsed := false

	// 单账号分组提前设置 SingleAccountRetry 标记，让 Service 层首次 503 就不设模型限流标记。
	// 避免单账号分组收到 503 (MODEL_CAPACITY_EXHAUSTED) 时设 29s 限流，导致后续请求连续快速失败。
	if h.gatewayService.IsSingleAntigravityAccountGroup(c.Request.Context(), currentAPIKey.GroupID) {
		ctx := service.WithSingleAccountRetry(c.Request.Context(), true, h.metadataBridgeEnabled())
		c.Request = c.Request.WithContext(ctx)
	}

	for {
		fs := NewFailoverState(h.maxAccountSwitches, hasBoundSession)
		retryWithFallback := false

		for {
			// 选择支持该模型的账号
			selection, err := h.gatewayService.SelectAccountWithLoadAwareness(c.Request.Context(), currentAPIKey.GroupID, sessionKey, reqModel, fs.FailedAccountIDs, parsedReq.MetadataUserID, subject.UserID)
			if err != nil {
				if len(fs.FailedAccountIDs) == 0 {
					h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts: "+err.Error(), streamStarted)
					return
				}
				action := fs.HandleSelectionExhausted(c.Request.Context())
				switch action {
				case FailoverContinue:
					ctx := service.WithSingleAccountRetry(c.Request.Context(), true, h.metadataBridgeEnabled())
					c.Request = c.Request.WithContext(ctx)
					continue
				case FailoverCanceled:
					return
				default: // FailoverExhausted
					if fs.LastFailoverErr != nil {
						h.handleFailoverExhausted(c, fs.LastFailoverErr, platform, streamStarted)
					} else {
						h.handleFailoverExhaustedSimple(c, 502, streamStarted)
					}
					return
				}
			}
			account := selection.Account
			setOpsSelectedAccount(c, account.ID, account.Platform)

			// 检查请求拦截（预热请求、SUGGESTION MODE等）
			if account.IsInterceptWarmupEnabled() {
				interceptType := detectInterceptType(body, reqModel, parsedReq.MaxTokens, reqStream, isClaudeCodeClient)
				if interceptType != InterceptTypeNone {
					if selection.Acquired && selection.ReleaseFunc != nil {
						selection.ReleaseFunc()
					}
					if reqStream {
						sendMockInterceptStream(c, reqModel, interceptType)
					} else {
						sendMockInterceptResponse(c, reqModel, interceptType)
					}
					return
				}
			}

			// 3. 获取账号并发槽位
			accountReleaseFunc := selection.ReleaseFunc
			if !selection.Acquired {
				if selection.WaitPlan == nil {
					h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", streamStarted)
					return
				}
				accountWaitCounted := false
				canWait, err := h.concurrencyHelper.IncrementAccountWaitCount(c.Request.Context(), account.ID, selection.WaitPlan.MaxWaiting)
				if err != nil {
					reqLog.Warn("gateway.account_wait_counter_increment_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				} else if !canWait {
					reqLog.Info("gateway.account_wait_queue_full",
						zap.Int64("account_id", account.ID),
						zap.Int("max_waiting", selection.WaitPlan.MaxWaiting),
					)
					h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later", streamStarted)
					return
				}
				if err == nil && canWait {
					accountWaitCounted = true
				}
				releaseWait := func() {
					if accountWaitCounted {
						h.concurrencyHelper.DecrementAccountWaitCount(c.Request.Context(), account.ID)
						accountWaitCounted = false
					}
				}

				accountReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
					c,
					account.ID,
					selection.WaitPlan.MaxConcurrency,
					selection.WaitPlan.Timeout,
					reqStream,
					&streamStarted,
				)
				if err != nil {
					reqLog.Warn("gateway.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
					releaseWait()
					h.handleConcurrencyError(c, err, "account", streamStarted)
					return
				}
				// Slot acquired: no longer waiting in queue.
				releaseWait()
				if err := h.gatewayService.BindStickySession(c.Request.Context(), currentAPIKey.GroupID, sessionKey, account.ID); err != nil {
					reqLog.Warn("gateway.bind_sticky_session_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				}
			}
			// 账号槽位/等待计数需要在超时或断开时安全回收
			accountReleaseFunc = wrapReleaseOnDone(c.Request.Context(), accountReleaseFunc)

			// ===== 用户消息串行队列 START =====
			var queueRelease func()
			umqMode := h.getUserMsgQueueMode(account, parsedReq)

			switch umqMode {
			case config.UMQModeSerialize:
				// 串行模式：获取锁 + RPM 延迟 + 释放（当前行为不变）
				baseRPM := account.GetBaseRPM()
				release, qErr := h.userMsgQueueHelper.AcquireWithWait(
					c, account.ID, baseRPM, reqStream, &streamStarted,
					h.cfg.Gateway.UserMessageQueue.WaitTimeout(),
					reqLog,
				)
				if qErr != nil {
					// fail-open: 记录 warn，不阻止请求
					reqLog.Warn("gateway.umq_acquire_failed",
						zap.Int64("account_id", account.ID),
						zap.Error(qErr),
					)
				} else {
					queueRelease = release
				}

			case config.UMQModeThrottle:
				// 软性限速：仅施加 RPM 自适应延迟，不阻塞并发
				baseRPM := account.GetBaseRPM()
				if tErr := h.userMsgQueueHelper.ThrottleWithPing(
					c, account.ID, baseRPM, reqStream, &streamStarted,
					h.cfg.Gateway.UserMessageQueue.WaitTimeout(),
					reqLog,
				); tErr != nil {
					reqLog.Warn("gateway.umq_throttle_failed",
						zap.Int64("account_id", account.ID),
						zap.Error(tErr),
					)
				}

			default:
				if umqMode != "" {
					reqLog.Warn("gateway.umq_unknown_mode",
						zap.String("mode", umqMode),
						zap.Int64("account_id", account.ID),
					)
				}
			}

			// 用 wrapReleaseOnDone 确保 context 取消时自动释放（仅 serialize 模式有 queueRelease）
			queueRelease = wrapReleaseOnDone(c.Request.Context(), queueRelease)
			// 注入回调到 ParsedRequest：使用外层 wrapper 以便提前清理 AfterFunc
			parsedReq.OnUpstreamAccepted = queueRelease
			// ===== 用户消息串行队列 END =====

			// 应用渠道模型映射到请求
			if channelMapping.Mapped {
				parsedReq.Model = channelMapping.MappedModel
				parsedReq.Body = h.gatewayService.ReplaceModelInBody(parsedReq.Body, channelMapping.MappedModel)
				body = h.gatewayService.ReplaceModelInBody(body, channelMapping.MappedModel)
			}

			// 转发请求 - 根据账号平台分流
			c.Set("parsed_request", parsedReq)
			var result *service.ForwardResult
			requestCtx := c.Request.Context()
			if fs.SwitchCount > 0 {
				requestCtx = service.WithAccountSwitchCount(requestCtx, fs.SwitchCount, h.metadataBridgeEnabled())
			}
			// 记录 Forward 前已写入字节数，Forward 后若增加则说明 SSE 内容已发，禁止 failover
			writerSizeBeforeForward := c.Writer.Size()
			if account.Platform == service.PlatformAntigravity && account.Type != service.AccountTypeAPIKey {
				result, err = h.antigravityGatewayService.Forward(requestCtx, c, account, body, hasBoundSession)
			} else {
				result, err = h.gatewayService.Forward(requestCtx, c, account, parsedReq)
			}

			// 兜底释放串行锁（正常情况已通过回调提前释放）
			if queueRelease != nil {
				queueRelease()
			}
			// 清理回调引用，防止 failover 重试时旧回调被错误调用
			parsedReq.OnUpstreamAccepted = nil

			if accountReleaseFunc != nil {
				accountReleaseFunc()
			}
			if err != nil {
				// Beta policy block: return 400 immediately, no failover
				var betaBlockedErr *service.BetaBlockedError
				if errors.As(err, &betaBlockedErr) {
					h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", betaBlockedErr.Message)
					return
				}

				var promptTooLongErr *service.PromptTooLongError
				if errors.As(err, &promptTooLongErr) {
					reqLog.Warn("gateway.prompt_too_long_from_antigravity",
						zap.Any("current_group_id", currentAPIKey.GroupID),
						zap.Any("fallback_group_id", fallbackGroupID),
						zap.Bool("fallback_used", fallbackUsed),
					)
					if !fallbackUsed && fallbackGroupID != nil && *fallbackGroupID > 0 {
						fallbackGroup, err := h.gatewayService.ResolveGroupByID(c.Request.Context(), *fallbackGroupID)
						if err != nil {
							reqLog.Warn("gateway.resolve_fallback_group_failed", zap.Int64("fallback_group_id", *fallbackGroupID), zap.Error(err))
							_ = h.antigravityGatewayService.WriteMappedClaudeError(c, account, promptTooLongErr.StatusCode, promptTooLongErr.RequestID, promptTooLongErr.Body)
							return
						}
						if fallbackGroup.Platform != service.PlatformAnthropic ||
							fallbackGroup.SubscriptionType == service.SubscriptionTypeSubscription ||
							fallbackGroup.FallbackGroupIDOnInvalidRequest != nil {
							reqLog.Warn("gateway.fallback_group_invalid",
								zap.Int64("fallback_group_id", fallbackGroup.ID),
								zap.String("fallback_platform", fallbackGroup.Platform),
								zap.String("fallback_subscription_type", fallbackGroup.SubscriptionType),
							)
							_ = h.antigravityGatewayService.WriteMappedClaudeError(c, account, promptTooLongErr.StatusCode, promptTooLongErr.RequestID, promptTooLongErr.Body)
							return
						}
						fallbackAPIKey := cloneAPIKeyWithGroup(apiKey, fallbackGroup)
						if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), fallbackAPIKey.User, fallbackAPIKey, fallbackGroup, nil); err != nil {
							status, code, message := billingErrorDetails(err)
							h.handleStreamingAwareError(c, status, code, message, streamStarted)
							return
						}
						// 兜底重试按"直接请求兜底分组"处理：清除强制平台，允许按分组平台调度
						ctx := context.WithValue(c.Request.Context(), ctxkey.ForcePlatform, "")
						c.Request = c.Request.WithContext(ctx)
						currentAPIKey = fallbackAPIKey
						currentSubscription = nil
						fallbackUsed = true
						retryWithFallback = true
						break
					}
					_ = h.antigravityGatewayService.WriteMappedClaudeError(c, account, promptTooLongErr.StatusCode, promptTooLongErr.RequestID, promptTooLongErr.Body)
					return
				}
				var failoverErr *service.UpstreamFailoverError
				if errors.As(err, &failoverErr) {
					// 流式内容已写入客户端，无法撤销，禁止 failover 以防止流拼接腐化
					if c.Writer.Size() != writerSizeBeforeForward {
						h.handleFailoverExhausted(c, failoverErr, account.Platform, true)
						return
					}
					action := fs.HandleFailoverError(c.Request.Context(), h.gatewayService, account.ID, account.Platform, failoverErr)
					switch action {
					case FailoverContinue:
						continue
					case FailoverExhausted:
						h.handleFailoverExhausted(c, fs.LastFailoverErr, account.Platform, streamStarted)
						return
					case FailoverCanceled:
						return
					}
				}
				wroteFallback := h.ensureForwardErrorResponse(c, streamStarted)
				forwardFailedFields := []zap.Field{
					zap.Int64("account_id", account.ID),
					zap.String("account_name", account.Name),
					zap.String("account_platform", account.Platform),
					zap.Bool("fallback_error_response_written", wroteFallback),
					zap.Error(err),
				}
				if account.Proxy != nil {
					forwardFailedFields = append(forwardFailedFields,
						zap.Int64("proxy_id", account.Proxy.ID),
						zap.String("proxy_name", account.Proxy.Name),
						zap.String("proxy_host", account.Proxy.Host),
						zap.Int("proxy_port", account.Proxy.Port),
					)
				} else if account.ProxyID != nil {
					forwardFailedFields = append(forwardFailedFields, zap.Int64p("proxy_id", account.ProxyID))
				}
				reqLog.Error("gateway.forward_failed", forwardFailedFields...)
				return
			}

			// RPM 计数递增（Forward 成功后）
			// 注意：TOCTOU 竞态是已知且可接受的设计权衡，与 WindowCost 一致的 soft-limit 模式。
			// 在高并发下可能短暂超出 RPM 限制，但不会导致请求失败。
			if account.IsAnthropicOAuthOrSetupToken() && account.GetBaseRPM() > 0 {
				if err := h.gatewayService.IncrementAccountRPM(c.Request.Context(), account.ID); err != nil {
					reqLog.Warn("gateway.rpm_increment_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				}
			}

			// 捕获请求信息（用于异步记录，避免在 goroutine 中访问 gin.Context）
			userAgent := c.GetHeader("User-Agent")
			clientIP := ip.GetClientIP(c)
			requestPayloadHash := service.HashUsageRequestPayload(body)
			inboundEndpoint := GetInboundEndpoint(c)
			upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)

			if result.ReasoningEffort == nil {
				result.ReasoningEffort = service.NormalizeClaudeOutputEffort(parsedReq.OutputEffort)
			}

			// 使用量记录通过有界 worker 池提交，避免请求热路径创建无界 goroutine。
			h.submitUsageRecordTask(func(ctx context.Context) {
				if err := h.gatewayService.RecordUsage(ctx, &service.RecordUsageInput{
					Result:             result,
					ParsedRequest:      parsedReq,
					APIKey:             currentAPIKey,
					User:               currentAPIKey.User,
					Account:            account,
					Subscription:       currentSubscription,
					InboundEndpoint:    inboundEndpoint,
					UpstreamEndpoint:   upstreamEndpoint,
					UserAgent:          userAgent,
					IPAddress:          clientIP,
					RequestPayloadHash: requestPayloadHash,
					ForceCacheBilling:  fs.ForceCacheBilling,
					APIKeyService:      h.apiKeyService,
					ChannelUsageFields: channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
				}); err != nil {
					logger.L().With(
						zap.String("component", "handler.gateway.messages"),
						zap.Int64("user_id", subject.UserID),
						zap.Int64("api_key_id", currentAPIKey.ID),
						zap.Any("group_id", currentAPIKey.GroupID),
						zap.String("model", reqModel),
						zap.Int64("account_id", account.ID),
					).Error("gateway.record_usage_failed", zap.Error(err))
				}
			})
			return
		}
		if !retryWithFallback {
			return
		}
	}
}

// Models handles listing available models
// GET /v1/models
// Returns models based on account configurations (model_mapping whitelist)
// Falls back to default models if no whitelist is configured
func (h *GatewayHandler) Models(c *gin.Context) {
	apiKey, _ := middleware2.GetAPIKeyFromContext(c)

	var groupID *int64
	var platform string

	if apiKey != nil && apiKey.Group != nil {
		groupID = &apiKey.Group.ID
		platform = apiKey.Group.Platform
	}
	if forcedPlatform, ok := middleware2.GetForcePlatformFromContext(c); ok && strings.TrimSpace(forcedPlatform) != "" {
		platform = forcedPlatform
	}

	// Get available models from account configurations (without platform filter)
	availableModels := h.gatewayService.GetAvailableModels(c.Request.Context(), groupID, "")

	if len(availableModels) > 0 {
		// Build model list from whitelist
		models := make([]claude.Model, 0, len(availableModels))
		for _, modelID := range availableModels {
			models = append(models, claude.Model{
				ID:          modelID,
				Type:        "model",
				DisplayName: modelID,
				CreatedAt:   "2024-01-01T00:00:00Z",
			})
		}
		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data":   models,
		})
		return
	}

	// Fallback to default models
	if platform == "openai" {
		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data":   openai.DefaultModels,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   claude.DefaultModels,
	})
}

// AntigravityModels 返回 Antigravity 支持的全部模型
// GET /antigravity/models
func (h *GatewayHandler) AntigravityModels(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   antigravity.DefaultModels(),
	})
}

func cloneAPIKeyWithGroup(apiKey *service.APIKey, group *service.Group) *service.APIKey {
	if apiKey == nil || group == nil {
		return apiKey
	}
	cloned := *apiKey
	groupID := group.ID
	cloned.GroupID = &groupID
	cloned.Group = group
	return &cloned
}

// Usage handles getting account balance and usage statistics for CC Switch integration
// GET /v1/usage
//
// Two modes:
//   - quota_limited: API Key has quota or rate limits configured. Returns key-level limits/usage.
//   - unrestricted:  No key-level limits. Returns subscription or wallet balance info.
func (h *GatewayHandler) Usage(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	ctx := c.Request.Context()

	// 解析可选的日期范围参数（用于 model_stats 查询）
	startTime, endTime := h.parseUsageDateRange(c)

	// Best-effort: 获取用量统计（按当前 API Key 过滤），失败不影响基础响应
	usageData := h.buildUsageData(ctx, apiKey.ID)

	// Best-effort: 获取模型统计
	var modelStats any
	if h.usageService != nil {
		if stats, err := h.usageService.GetAPIKeyModelStats(ctx, apiKey.ID, startTime, endTime); err == nil && len(stats) > 0 {
			modelStats = stats
		}
	}

	// 判断模式: key 有总额度或速率限制 → quota_limited，否则 → unrestricted
	isQuotaLimited := apiKey.Quota > 0 || apiKey.HasRateLimits()

	if isQuotaLimited {
		h.usageQuotaLimited(c, ctx, apiKey, usageData, modelStats)
		return
	}

	h.usageUnrestricted(c, ctx, apiKey, subject, usageData, modelStats)
}

// parseUsageDateRange 解析 start_date / end_date query params，默认返回近 30 天范围
func (h *GatewayHandler) parseUsageDateRange(c *gin.Context) (time.Time, time.Time) {
	now := timezone.Now()
	endTime := now
	startTime := now.AddDate(0, 0, -30)

	if s := c.Query("start_date"); s != "" {
		if t, err := timezone.ParseInLocation("2006-01-02", s); err == nil {
			startTime = t
		}
	}
	if s := c.Query("end_date"); s != "" {
		if t, err := timezone.ParseInLocation("2006-01-02", s); err == nil {
			endTime = t.AddDate(0, 0, 1) // half-open range upper bound
		}
	}
	return startTime, endTime
}

// buildUsageData 构建 today/total 用量摘要
func (h *GatewayHandler) buildUsageData(ctx context.Context, apiKeyID int64) gin.H {
	if h.usageService == nil {
		return nil
	}
	dashStats, err := h.usageService.GetAPIKeyDashboardStats(ctx, apiKeyID)
	if err != nil || dashStats == nil {
		return nil
	}
	return gin.H{
		"today": gin.H{
			"requests":              dashStats.TodayRequests,
			"input_tokens":          dashStats.TodayInputTokens,
			"output_tokens":         dashStats.TodayOutputTokens,
			"cache_creation_tokens": dashStats.TodayCacheCreationTokens,
			"cache_read_tokens":     dashStats.TodayCacheReadTokens,
			"total_tokens":          dashStats.TodayTokens,
			"cost":                  dashStats.TodayCost,
			"actual_cost":           dashStats.TodayActualCost,
		},
		"total": gin.H{
			"requests":              dashStats.TotalRequests,
			"input_tokens":          dashStats.TotalInputTokens,
			"output_tokens":         dashStats.TotalOutputTokens,
			"cache_creation_tokens": dashStats.TotalCacheCreationTokens,
			"cache_read_tokens":     dashStats.TotalCacheReadTokens,
			"total_tokens":          dashStats.TotalTokens,
			"cost":                  dashStats.TotalCost,
			"actual_cost":           dashStats.TotalActualCost,
		},
		"average_duration_ms": dashStats.AverageDurationMs,
		"rpm":                 dashStats.Rpm,
		"tpm":                 dashStats.Tpm,
	}
}

// usageQuotaLimited 处理 quota_limited 模式的响应
func (h *GatewayHandler) usageQuotaLimited(c *gin.Context, ctx context.Context, apiKey *service.APIKey, usageData gin.H, modelStats any) {
	resp := gin.H{
		"mode":    "quota_limited",
		"isValid": apiKey.Status == service.StatusAPIKeyActive || apiKey.Status == service.StatusAPIKeyQuotaExhausted || apiKey.Status == service.StatusAPIKeyExpired,
		"status":  apiKey.Status,
	}

	// 总额度信息
	if apiKey.Quota > 0 {
		remaining := apiKey.GetQuotaRemaining()
		resp["quota"] = gin.H{
			"limit":     apiKey.Quota,
			"used":      apiKey.QuotaUsed,
			"remaining": remaining,
			"unit":      "USD",
		}
		resp["remaining"] = remaining
		resp["unit"] = "USD"
	}

	// 速率限制信息（从 DB 获取实时用量）
	if apiKey.HasRateLimits() && h.apiKeyService != nil {
		rateLimitData, err := h.apiKeyService.GetRateLimitData(ctx, apiKey.ID)
		if err == nil && rateLimitData != nil {
			var rateLimits []gin.H
			if apiKey.RateLimit5h > 0 {
				used := rateLimitData.EffectiveUsage5h()
				entry := gin.H{
					"window":       "5h",
					"limit":        apiKey.RateLimit5h,
					"used":         used,
					"remaining":    max(0, apiKey.RateLimit5h-used),
					"window_start": rateLimitData.Window5hStart,
				}
				if rateLimitData.Window5hStart != nil && !service.IsWindowExpired(rateLimitData.Window5hStart, service.RateLimitWindow5h) {
					entry["reset_at"] = rateLimitData.Window5hStart.Add(service.RateLimitWindow5h)
				}
				rateLimits = append(rateLimits, entry)
			}
			if apiKey.RateLimit1d > 0 {
				used := rateLimitData.EffectiveUsage1d()
				entry := gin.H{
					"window":       "1d",
					"limit":        apiKey.RateLimit1d,
					"used":         used,
					"remaining":    max(0, apiKey.RateLimit1d-used),
					"window_start": rateLimitData.Window1dStart,
				}
				if rateLimitData.Window1dStart != nil && !service.IsWindowExpired(rateLimitData.Window1dStart, service.RateLimitWindow1d) {
					entry["reset_at"] = rateLimitData.Window1dStart.Add(service.RateLimitWindow1d)
				}
				rateLimits = append(rateLimits, entry)
			}
			if apiKey.RateLimit7d > 0 {
				used := rateLimitData.EffectiveUsage7d()
				entry := gin.H{
					"window":       "7d",
					"limit":        apiKey.RateLimit7d,
					"used":         used,
					"remaining":    max(0, apiKey.RateLimit7d-used),
					"window_start": rateLimitData.Window7dStart,
				}
				if rateLimitData.Window7dStart != nil && !service.IsWindowExpired(rateLimitData.Window7dStart, service.RateLimitWindow7d) {
					entry["reset_at"] = rateLimitData.Window7dStart.Add(service.RateLimitWindow7d)
				}
				rateLimits = append(rateLimits, entry)
			}
			if len(rateLimits) > 0 {
				resp["rate_limits"] = rateLimits
			}
		}
	}

	// 过期时间
	if apiKey.ExpiresAt != nil {
		resp["expires_at"] = apiKey.ExpiresAt
		resp["days_until_expiry"] = apiKey.GetDaysUntilExpiry()
	}

	if usageData != nil {
		resp["usage"] = usageData
	}
	if modelStats != nil {
		resp["model_stats"] = modelStats
	}

	c.JSON(http.StatusOK, resp)
}

// usageUnrestricted 处理 unrestricted 模式的响应（向后兼容）
func (h *GatewayHandler) usageUnrestricted(c *gin.Context, ctx context.Context, apiKey *service.APIKey, subject middleware2.AuthSubject, usageData gin.H, modelStats any) {
	// 订阅模式
	if apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
		resp := gin.H{
			"mode":     "unrestricted",
			"isValid":  true,
			"planName": apiKey.Group.Name,
			"unit":     "USD",
		}

		// 订阅信息可能不在 context 中（/v1/usage 路径跳过了中间件的计费检查）
		subscription, ok := middleware2.GetSubscriptionFromContext(c)
		if ok {
			remaining := h.calculateSubscriptionRemaining(apiKey.Group, subscription)
			resp["remaining"] = remaining
			resp["subscription"] = gin.H{
				"daily_usage_usd":   subscription.DailyUsageUSD,
				"weekly_usage_usd":  subscription.WeeklyUsageUSD,
				"monthly_usage_usd": subscription.MonthlyUsageUSD,
				"daily_limit_usd":   apiKey.Group.DailyLimitUSD,
				"weekly_limit_usd":  apiKey.Group.WeeklyLimitUSD,
				"monthly_limit_usd": apiKey.Group.MonthlyLimitUSD,
				"expires_at":        subscription.ExpiresAt,
			}
		}

		if usageData != nil {
			resp["usage"] = usageData
		}
		if modelStats != nil {
			resp["model_stats"] = modelStats
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	// 余额模式
	latestUser, err := h.userService.GetByID(ctx, subject.UserID)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to get user info")
		return
	}

	resp := gin.H{
		"mode":      "unrestricted",
		"isValid":   true,
		"planName":  "钱包余额",
		"remaining": latestUser.Balance,
		"unit":      "USD",
		"balance":   latestUser.Balance,
	}
	if usageData != nil {
		resp["usage"] = usageData
	}
	if modelStats != nil {
		resp["model_stats"] = modelStats
	}
	c.JSON(http.StatusOK, resp)
}

// calculateSubscriptionRemaining 计算订阅剩余可用额度
// 逻辑：
// 1. 如果日/周/月任一限额达到100%，返回0
// 2. 否则返回所有已配置周期中剩余额度的最小值
func (h *GatewayHandler) calculateSubscriptionRemaining(group *service.Group, sub *service.UserSubscription) float64 {
	var remainingValues []float64

	// 检查日限额
	if group.HasDailyLimit() {
		remaining := *group.DailyLimitUSD - sub.DailyUsageUSD
		if remaining <= 0 {
			return 0
		}
		remainingValues = append(remainingValues, remaining)
	}

	// 检查周限额
	if group.HasWeeklyLimit() {
		remaining := *group.WeeklyLimitUSD - sub.WeeklyUsageUSD
		if remaining <= 0 {
			return 0
		}
		remainingValues = append(remainingValues, remaining)
	}

	// 检查月限额
	if group.HasMonthlyLimit() {
		remaining := *group.MonthlyLimitUSD - sub.MonthlyUsageUSD
		if remaining <= 0 {
			return 0
		}
		remainingValues = append(remainingValues, remaining)
	}

	// 如果没有配置任何限额，返回-1表示无限制
	if len(remainingValues) == 0 {
		return -1
	}

	// 返回最小值
	min := remainingValues[0]
	for _, v := range remainingValues[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

// handleConcurrencyError handles concurrency-related errors with proper 429 response
func (h *GatewayHandler) handleConcurrencyError(c *gin.Context, err error, slotType string, streamStarted bool) {
	h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error",
		fmt.Sprintf("Concurrency limit exceeded for %s, please retry later", slotType), streamStarted)
}

func (h *GatewayHandler) handleFailoverExhausted(c *gin.Context, failoverErr *service.UpstreamFailoverError, platform string, streamStarted bool) {
	statusCode := failoverErr.StatusCode
	responseBody := failoverErr.ResponseBody

	// 先检查透传规则
	if h.errorPassthroughService != nil && len(responseBody) > 0 {
		if rule := h.errorPassthroughService.MatchRule(platform, statusCode, responseBody); rule != nil {
			// 确定响应状态码
			respCode := statusCode
			if !rule.PassthroughCode && rule.ResponseCode != nil {
				respCode = *rule.ResponseCode
			}

			// 确定响应消息
			msg := service.ExtractUpstreamErrorMessage(responseBody)
			if !rule.PassthroughBody && rule.CustomMessage != nil {
				msg = *rule.CustomMessage
			}

			if rule.SkipMonitoring {
				c.Set(service.OpsSkipPassthroughKey, true)
			}

			h.handleStreamingAwareError(c, respCode, "upstream_error", msg, streamStarted)
			return
		}
	}

	// 记录原始上游状态码，以便 ops 错误日志捕获真实的上游错误
	upstreamMsg := service.ExtractUpstreamErrorMessage(responseBody)
	service.SetOpsUpstreamError(c, statusCode, upstreamMsg, "")

	// 使用默认的错误映射
	status, errType, errMsg := h.mapUpstreamError(statusCode)
	h.handleStreamingAwareError(c, status, errType, errMsg, streamStarted)
}

// handleFailoverExhaustedSimple 简化版本，用于没有响应体的情况
func (h *GatewayHandler) handleFailoverExhaustedSimple(c *gin.Context, statusCode int, streamStarted bool) {
	status, errType, errMsg := h.mapUpstreamError(statusCode)
	service.SetOpsUpstreamError(c, statusCode, errMsg, "")
	h.handleStreamingAwareError(c, status, errType, errMsg, streamStarted)
}

func (h *GatewayHandler) mapUpstreamError(statusCode int) (int, string, string) {
	switch statusCode {
	case 401:
		return http.StatusBadGateway, "upstream_error", "Upstream authentication failed, please contact administrator"
	case 403:
		return http.StatusBadGateway, "upstream_error", "Upstream access forbidden, please contact administrator"
	case 429:
		return http.StatusTooManyRequests, "rate_limit_error", "Upstream rate limit exceeded, please retry later"
	case 529:
		return http.StatusServiceUnavailable, "overloaded_error", "Upstream service overloaded, please retry later"
	case 500, 502, 503, 504:
		return http.StatusBadGateway, "upstream_error", "Upstream service temporarily unavailable"
	default:
		return http.StatusBadGateway, "upstream_error", "Upstream request failed"
	}
}

// handleStreamingAwareError handles errors that may occur after streaming has started
func (h *GatewayHandler) handleStreamingAwareError(c *gin.Context, status int, errType, message string, streamStarted bool) {
	if streamStarted {
		// Stream already started, send error as SSE event then close
		flusher, ok := c.Writer.(http.Flusher)
		if ok {
			// SSE 错误事件固定 schema，使用 Quote 直拼可避免额外 Marshal 分配。
			errorEvent := `data: {"type":"error","error":{"type":` + strconv.Quote(errType) + `,"message":` + strconv.Quote(message) + `}}` + "\n\n"
			if _, err := fmt.Fprint(c.Writer, errorEvent); err != nil {
				_ = c.Error(err)
			}
			flusher.Flush()
		}
		return
	}

	// Normal case: return JSON response with proper status code
	h.errorResponse(c, status, errType, message)
}

// ensureForwardErrorResponse 在 Forward 返回错误但尚未写响应时补写统一错误响应。
func (h *GatewayHandler) ensureForwardErrorResponse(c *gin.Context, streamStarted bool) bool {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return false
	}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed", streamStarted)
	return true
}

// checkClaudeCodeVersion 检查 Claude Code 客户端版本是否满足版本要求
// 仅对已识别的 Claude Code 客户端执行，count_tokens 路径除外
func (h *GatewayHandler) checkClaudeCodeVersion(c *gin.Context) bool {
	ctx := c.Request.Context()
	if !service.IsClaudeCodeClient(ctx) {
		return true
	}

	// 排除 count_tokens 子路径
	if strings.HasSuffix(c.Request.URL.Path, "/count_tokens") {
		return true
	}

	minVersion, maxVersion := h.settingService.GetClaudeCodeVersionBounds(ctx)
	if minVersion == "" && maxVersion == "" {
		return true // 未设置，不检查
	}

	clientVersion := service.GetClaudeCodeVersion(ctx)
	if clientVersion == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error",
			"Unable to determine Claude Code version. Please update Claude Code: npm update -g @anthropic-ai/claude-code")
		return false
	}

	if minVersion != "" && service.CompareVersions(clientVersion, minVersion) < 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error",
			fmt.Sprintf("Your Claude Code version (%s) is below the minimum required version (%s). Please update: npm update -g @anthropic-ai/claude-code",
				clientVersion, minVersion))
		return false
	}

	if maxVersion != "" && service.CompareVersions(clientVersion, maxVersion) > 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error",
			fmt.Sprintf("Your Claude Code version (%s) exceeds the maximum allowed version (%s). "+
				"Please downgrade: npm install -g @anthropic-ai/claude-code@%s && "+
				"set CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 to prevent auto-upgrade",
				clientVersion, maxVersion, maxVersion))
		return false
	}

	return true
}

// errorResponse 返回Claude API格式的错误响应
func (h *GatewayHandler) errorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// CountTokens handles token counting endpoint
// POST /v1/messages/count_tokens
// 特点：校验订阅/余额，但不计算并发、不记录使用量
func (h *GatewayHandler) CountTokens(c *gin.Context) {
	// 从context获取apiKey和user（ApiKeyAuth中间件已设置）
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	_, ok = middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.gateway.count_tokens",
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	defer h.maybeLogCompatibilityFallbackMetrics(reqLog)

	// 读取请求体
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	setOpsRequestContext(c, "", false, body)

	parsedReq, err := service.ParseGatewayRequest(body, domain.PlatformAnthropic)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}
	// count_tokens 走 messages 严格校验时，复用已解析请求，避免二次反序列化。
	SetClaudeCodeClientContext(c, body, parsedReq)
	reqLog = reqLog.With(zap.String("model", parsedReq.Model), zap.Bool("stream", parsedReq.Stream))
	// 在请求上下文中记录 thinking 状态，供 Antigravity 最终模型 key 推导/模型维度限流使用
	c.Request = c.Request.WithContext(service.WithThinkingEnabled(c.Request.Context(), parsedReq.ThinkingEnabled, h.metadataBridgeEnabled()))

	// 验证 model 必填
	if parsedReq.Model == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}

	setOpsRequestContext(c, parsedReq.Model, parsedReq.Stream, body)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(parsedReq.Stream, false)))

	// 获取订阅信息（可能为nil）
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	// 校验 billing eligibility（订阅/余额）
	// 【注意】不计算并发，但需要校验订阅/余额
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		status, code, message := billingErrorDetails(err)
		h.errorResponse(c, status, code, message)
		return
	}

	// 计算粘性会话 hash
	parsedReq.SessionContext = &service.SessionContext{
		ClientIP:  ip.GetClientIP(c),
		UserAgent: c.GetHeader("User-Agent"),
		APIKeyID:  apiKey.ID,
	}
	sessionHash := h.gatewayService.GenerateSessionHash(parsedReq)

	// 选择支持该模型的账号
	account, err := h.gatewayService.SelectAccountForModel(c.Request.Context(), apiKey.GroupID, sessionHash, parsedReq.Model)
	if err != nil {
		reqLog.Warn("gateway.count_tokens_select_account_failed", zap.Error(err))
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable")
		return
	}
	setOpsSelectedAccount(c, account.ID, account.Platform)

	// 转发请求（不记录使用量）
	if err := h.gatewayService.ForwardCountTokens(c.Request.Context(), c, account, parsedReq); err != nil {
		reqLog.Error("gateway.count_tokens_forward_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		// 错误响应已在 ForwardCountTokens 中处理
		return
	}
}

// InterceptType 表示请求拦截类型
type InterceptType int

const (
	InterceptTypeNone              InterceptType = iota
	InterceptTypeWarmup                          // 预热请求（返回 "New Conversation"）
	InterceptTypeSuggestionMode                  // SUGGESTION MODE（返回空字符串）
	InterceptTypeMaxTokensOneHaiku               // max_tokens=1 + haiku 探测请求（返回 "#"）
)

// isHaikuModel 检查模型名称是否包含 "haiku"（大小写不敏感）
func isHaikuModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "haiku")
}

// isMaxTokensOneHaikuRequest 检查是否为 max_tokens=1 + haiku 模型的探测请求
// 这类请求用于 Claude Code 验证 API 连通性
// 条件：max_tokens == 1 且 model 包含 "haiku" 且非流式请求
func isMaxTokensOneHaikuRequest(model string, maxTokens int, isStream bool) bool {
	return maxTokens == 1 && isHaikuModel(model) && !isStream
}

// detectInterceptType 检测请求是否需要拦截，返回拦截类型
// 参数说明：
//   - body: 请求体字节
//   - model: 请求的模型名称
//   - maxTokens: max_tokens 值
//   - isStream: 是否为流式请求
//   - isClaudeCodeClient: 是否已通过 Claude Code 客户端校验
func detectInterceptType(body []byte, model string, maxTokens int, isStream bool, isClaudeCodeClient bool) InterceptType {
	// 优先检查 max_tokens=1 + haiku 探测请求（仅非流式）
	if isClaudeCodeClient && isMaxTokensOneHaikuRequest(model, maxTokens, isStream) {
		return InterceptTypeMaxTokensOneHaiku
	}

	// 快速检查：如果不包含任何关键字，直接返回
	bodyStr := string(body)
	hasSuggestionMode := strings.Contains(bodyStr, "[SUGGESTION MODE:")
	hasWarmupKeyword := strings.Contains(bodyStr, "title") || strings.Contains(bodyStr, "Warmup")

	if !hasSuggestionMode && !hasWarmupKeyword {
		return InterceptTypeNone
	}

	// 解析请求（只解析一次）
	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return InterceptTypeNone
	}

	// 检查 SUGGESTION MODE（最后一条 user 消息）
	if hasSuggestionMode && len(req.Messages) > 0 {
		lastMsg := req.Messages[len(req.Messages)-1]
		if lastMsg.Role == "user" && len(lastMsg.Content) > 0 &&
			lastMsg.Content[0].Type == "text" &&
			strings.HasPrefix(lastMsg.Content[0].Text, "[SUGGESTION MODE:") {
			return InterceptTypeSuggestionMode
		}
	}

	// 检查 Warmup 请求
	if hasWarmupKeyword {
		// 检查 messages 中的标题提示模式
		for _, msg := range req.Messages {
			for _, content := range msg.Content {
				if content.Type == "text" {
					if strings.Contains(content.Text, "Please write a 5-10 word title for the following conversation:") ||
						content.Text == "Warmup" {
						return InterceptTypeWarmup
					}
				}
			}
		}
		// 检查 system 中的标题提取模式
		for _, sys := range req.System {
			if strings.Contains(sys.Text, "nalyze if this message indicates a new conversation topic. If it does, extract a 2-3 word title") {
				return InterceptTypeWarmup
			}
		}
	}

	return InterceptTypeNone
}

// sendMockInterceptStream 发送流式 mock 响应（用于请求拦截）
func sendMockInterceptStream(c *gin.Context, model string, interceptType InterceptType) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 根据拦截类型决定响应内容
	var msgID string
	var outputTokens int
	var textDeltas []string

	switch interceptType {
	case InterceptTypeSuggestionMode:
		msgID = "msg_mock_suggestion"
		outputTokens = 1
		textDeltas = []string{""} // 空内容
	default: // InterceptTypeWarmup
		msgID = "msg_mock_warmup"
		outputTokens = 2
		textDeltas = []string{"New", " Conversation"}
	}

	// Build message_start event with fixed schema.
	messageStartJSON := `{"type":"message_start","message":{"id":` + strconv.Quote(msgID) + `,"type":"message","role":"assistant","model":` + strconv.Quote(model) + `,"content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`

	// Build events
	events := []string{
		`event: message_start` + "\n" + `data: ` + string(messageStartJSON),
		`event: content_block_start` + "\n" + `data: {"content_block":{"text":"","type":"text"},"index":0,"type":"content_block_start"}`,
	}

	// Add text deltas
	for _, text := range textDeltas {
		deltaJSON := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":` + strconv.Quote(text) + `}}`
		events = append(events, `event: content_block_delta`+"\n"+`data: `+string(deltaJSON))
	}

	// Add final events
	messageDeltaJSON := `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":10,"output_tokens":` + strconv.Itoa(outputTokens) + `}}`

	events = append(events,
		`event: content_block_stop`+"\n"+`data: {"index":0,"type":"content_block_stop"}`,
		`event: message_delta`+"\n"+`data: `+string(messageDeltaJSON),
		`event: message_stop`+"\n"+`data: {"type":"message_stop"}`,
	)

	for _, event := range events {
		_, _ = c.Writer.WriteString(event + "\n\n")
		c.Writer.Flush()
		time.Sleep(20 * time.Millisecond)
	}
}

// generateRealisticMsgID 生成仿真的消息 ID（msg_bdrk_XXXXXXX 格式）
// 格式与 Claude API 真实响应一致，24 位随机字母数字
func generateRealisticMsgID() string {
	const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	const idLen = 24
	randomBytes := make([]byte, idLen)
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Sprintf("msg_bdrk_%d", time.Now().UnixNano())
	}
	b := make([]byte, idLen)
	for i := range b {
		b[i] = charset[int(randomBytes[i])%len(charset)]
	}
	return "msg_bdrk_" + string(b)
}

// sendMockInterceptResponse 发送非流式 mock 响应（用于请求拦截）
func sendMockInterceptResponse(c *gin.Context, model string, interceptType InterceptType) {
	var msgID, text, stopReason string
	var outputTokens int

	switch interceptType {
	case InterceptTypeSuggestionMode:
		msgID = "msg_mock_suggestion"
		text = ""
		outputTokens = 1
		stopReason = "end_turn"
	case InterceptTypeMaxTokensOneHaiku:
		msgID = generateRealisticMsgID()
		text = "#"
		outputTokens = 1
		stopReason = "max_tokens" // max_tokens=1 探测请求的 stop_reason 应为 max_tokens
	default: // InterceptTypeWarmup
		msgID = "msg_mock_warmup"
		text = "New Conversation"
		outputTokens = 2
		stopReason = "end_turn"
	}

	// 构建完整的响应格式（与 Claude API 响应格式一致）
	response := gin.H{
		"model":         model,
		"id":            msgID,
		"type":          "message",
		"role":          "assistant",
		"content":       []gin.H{{"type": "text", "text": text}},
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": gin.H{
			"input_tokens":                10,
			"cache_creation_input_tokens": 0,
			"cache_read_input_tokens":     0,
			"cache_creation": gin.H{
				"ephemeral_5m_input_tokens": 0,
				"ephemeral_1h_input_tokens": 0,
			},
			"output_tokens": outputTokens,
			"total_tokens":  10 + outputTokens,
		},
	}

	c.JSON(http.StatusOK, response)
}

func billingErrorDetails(err error) (status int, code, message string) {
	if errors.Is(err, service.ErrBillingServiceUnavailable) {
		msg := pkgerrors.Message(err)
		if msg == "" {
			msg = "Billing service temporarily unavailable. Please retry later."
		}
		return http.StatusServiceUnavailable, "billing_service_error", msg
	}
	if errors.Is(err, service.ErrAPIKeyRateLimit5hExceeded) {
		msg := pkgerrors.Message(err)
		return http.StatusTooManyRequests, "rate_limit_exceeded", msg
	}
	if errors.Is(err, service.ErrAPIKeyRateLimit1dExceeded) {
		msg := pkgerrors.Message(err)
		return http.StatusTooManyRequests, "rate_limit_exceeded", msg
	}
	if errors.Is(err, service.ErrAPIKeyRateLimit7dExceeded) {
		msg := pkgerrors.Message(err)
		return http.StatusTooManyRequests, "rate_limit_exceeded", msg
	}
	msg := pkgerrors.Message(err)
	if msg == "" {
		logger.L().With(
			zap.String("component", "handler.gateway.billing"),
			zap.Error(err),
		).Warn("gateway.billing_error_missing_message")
		msg = "Billing error"
	}
	return http.StatusForbidden, "billing_error", msg
}

func (h *GatewayHandler) metadataBridgeEnabled() bool {
	if h == nil || h.cfg == nil {
		return true
	}
	return h.cfg.Gateway.OpenAIWS.MetadataBridgeEnabled
}

func (h *GatewayHandler) maybeLogCompatibilityFallbackMetrics(reqLog *zap.Logger) {
	if reqLog == nil {
		return
	}
	if gatewayCompatibilityMetricsLogCounter.Add(1)%gatewayCompatibilityMetricsLogInterval != 0 {
		return
	}
	metrics := service.SnapshotOpenAICompatibilityFallbackMetrics()
	reqLog.Info("gateway.compatibility_fallback_metrics",
		zap.Int64("session_hash_legacy_read_fallback_total", metrics.SessionHashLegacyReadFallbackTotal),
		zap.Int64("session_hash_legacy_read_fallback_hit", metrics.SessionHashLegacyReadFallbackHit),
		zap.Int64("session_hash_legacy_dual_write_total", metrics.SessionHashLegacyDualWriteTotal),
		zap.Float64("session_hash_legacy_read_hit_rate", metrics.SessionHashLegacyReadHitRate),
		zap.Int64("metadata_legacy_fallback_total", metrics.MetadataLegacyFallbackTotal),
	)
}

func (h *GatewayHandler) submitUsageRecordTask(task service.UsageRecordTask) {
	if task == nil {
		return
	}
	if h.usageRecordWorkerPool != nil {
		h.usageRecordWorkerPool.Submit(task)
		return
	}
	// 回退路径：worker 池未注入时同步执行，避免退回到无界 goroutine 模式。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.L().With(
				zap.String("component", "handler.gateway.messages"),
				zap.Any("panic", recovered),
			).Error("gateway.usage_record_task_panic_recovered")
		}
	}()
	task(ctx)
}

// getUserMsgQueueMode 获取当前请求的 UMQ 模式
// 返回 "serialize" | "throttle" | ""
func (h *GatewayHandler) getUserMsgQueueMode(account *service.Account, parsed *service.ParsedRequest) string {
	if h.userMsgQueueHelper == nil {
		return ""
	}
	// 仅适用于 Anthropic OAuth/SetupToken 账号
	if !account.IsAnthropicOAuthOrSetupToken() {
		return ""
	}
	if !service.IsRealUserMessage(parsed) {
		return ""
	}
	// 账号级模式优先，fallback 到全局配置
	mode := account.GetUserMsgQueueMode()
	if mode == "" {
		mode = h.cfg.Gateway.UserMessageQueue.GetEffectiveMode()
	}
	return mode
}
