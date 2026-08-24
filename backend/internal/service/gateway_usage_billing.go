package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

func (s *GatewayService) getUserGroupRateMultiplier(ctx context.Context, userID, groupID int64, groupDefaultMultiplier float64) float64 {
	if s == nil {
		return groupDefaultMultiplier
	}
	resolver := s.userGroupRateResolver
	if resolver == nil {
		resolver = newUserGroupRateResolver(
			s.userGroupRateRepo,
			s.userGroupRateCache,
			resolveUserGroupRateCacheTTL(s.cfg),
			&s.userGroupRateSF,
			"service.gateway",
		)
	}
	return resolver.Resolve(ctx, userID, groupID, groupDefaultMultiplier)
}

// ResolveUserGroupRateMultiplier resolves the same cached multiplier used by usage billing.
func (s *GatewayService) ResolveUserGroupRateMultiplier(ctx context.Context, userID, groupID int64, groupDefaultMultiplier float64) float64 {
	return s.getUserGroupRateMultiplier(ctx, userID, groupID, groupDefaultMultiplier)
}

// RecordUsageInput 记录使用量的输入参数。
// 异步 worker 只接收计费所需快照，不能持有 ParsedRequest/RequestBodyRef 这类大请求体引用。
type RecordUsageInput struct {
	Result             *ForwardResult
	APIKey             *APIKey
	User               *User
	Account            *Account
	Subscription       *UserSubscription  // 可选：订阅信息
	PricingAt          time.Time          // token 售价固定时刻；零值保持既有的记录时刻语义
	InboundEndpoint    string             // 入站端点（客户端请求路径）
	UpstreamEndpoint   string             // 上游端点（标准化后的上游路径）
	UserAgent          string             // 请求的 User-Agent
	IPAddress          string             // 请求的客户端 IP 地址
	SessionID          string             // 客户端显式会话标识（session_id / X-Session-Id 等请求头），仅用于用量行会话关联
	RequestPayloadHash string             // 请求体语义哈希，用于降低 request_id 误复用时的静默误去重风险
	ForceCacheBilling  bool               // 强制缓存计费：将 input_tokens 转为 cache_read 计费（用于粘性会话切换）
	APIKeyService      APIKeyQuotaUpdater // 可选：用于更新API Key配额
	QuotaPlatform      string             // user×platform 配额计量平台：handler 在请求 ctx 内经 QuotaPlatform() 算定后传入（后扣运行在 worker 池 background ctx 上，取不到 ForcePlatform）

	ChannelUsageFields // 渠道映射信息（由 handler 在 Forward 前解析）
}

// APIKeyQuotaUpdater defines the interface for updating API Key quota and rate limit usage
type APIKeyQuotaUpdater interface {
	UpdateQuotaUsed(ctx context.Context, apiKeyID int64, cost float64) error
	UpdateRateLimitUsage(ctx context.Context, apiKeyID int64, cost float64) error
}

type apiKeyAuthCacheInvalidator interface {
	InvalidateAuthCacheByKey(ctx context.Context, key string)
}

type usageLogBestEffortWriter interface {
	CreateBestEffort(ctx context.Context, log *UsageLog) error
}

// postUsageBillingParams 统一扣费所需的参数
type postUsageBillingParams struct {
	Cost                  *CostBreakdown
	User                  *User
	APIKey                *APIKey
	Account               *Account
	Subscription          *UserSubscription
	RequestPayloadHash    string
	IsSubscriptionBill    bool
	AccountRateMultiplier float64
	APIKeyService         APIKeyQuotaUpdater
	Platform              string // 来自 APIKey 关联 Group 的平台标识
}

// PlatformFromAPIKey 从 APIKey 关联的 Group 推导 platform 名称。
// apiKey 为 nil 或 Group 信息缺失时返回空串（调用方据此 short-circuit quota 累加）。
// 导出供 handler 层调用。
func PlatformFromAPIKey(apiKey *APIKey) string {
	if apiKey == nil || apiKey.Group == nil {
		return ""
	}
	return apiKey.Group.Platform
}

// QuotaPlatform 返回 user×platform 配额计量使用的平台标识。
// 强制平台路由（如 /antigravity）优先按 ctx 中的 ForcePlatform 计量，否则回退到
// APIKey 关联 Group 的平台。
//
// 注意：必须用带 ForcePlatform 的请求 context 调用（如 handler 的 c.Request.Context()）。
// 后扣运行在 worker 池的 background ctx 上没有 ForcePlatform，因此后扣平台由 handler
// 预先算定、经 RecordUsageInput.QuotaPlatform 传入，不要在后扣链路用 worker ctx 调用本函数。
func QuotaPlatform(ctx context.Context, apiKey *APIKey) string {
	if ctx != nil {
		if fp, ok := ctx.Value(ctxkey.ForcePlatform).(string); ok && fp != "" {
			return fp
		}
	}
	if platform, ok := ResolvedTargetPlatformFromContext(ctx); ok {
		return platform
	}
	platform := PlatformFromAPIKey(apiKey)
	if platform == PlatformComposite {
		return ""
	}
	return platform
}

func (p *postUsageBillingParams) shouldDeductAPIKeyQuota() bool {
	return p.Cost.ActualCost > 0 && p.APIKey.Quota > 0 && p.APIKeyService != nil
}

func (p *postUsageBillingParams) shouldUpdateRateLimits() bool {
	return p.Cost.ActualCost > 0 && p.APIKey.HasRateLimits() && p.APIKeyService != nil
}

func (p *postUsageBillingParams) shouldUpdateAccountQuota() bool {
	return p.Cost.TotalCost > 0 && p.Account.IsAPIKeyOrBedrock() && p.Account.HasAnyQuotaLimit()
}

// postUsageBilling is the legacy fallback billing path used when the unified
// billing repo is unavailable (nil). Production uses applyUsageBilling → repo.Apply
// for atomic billing. This path only runs in tests or degraded mode.
func postUsageBilling(ctx context.Context, p *postUsageBillingParams, deps *billingDeps) {
	billingCtx, cancel := detachedBillingContext(ctx)
	defer cancel()

	cost := p.Cost

	if p.IsSubscriptionBill {
		// Subscription usage tracked by ActualCost so group rate multiplier
		// consumes the quota at the expected speed.
		if cost.ActualCost > 0 {
			if err := deps.userSubRepo.IncrementUsage(billingCtx, p.Subscription.ID, cost.ActualCost); err != nil {
				slog.Error("increment subscription usage failed", "subscription_id", p.Subscription.ID, "error", err)
			}
		}
	} else {
		if cost.ActualCost > 0 {
			if err := deps.userRepo.DeductBalance(billingCtx, p.User.ID, cost.ActualCost); err != nil {
				slog.Error("deduct balance failed", "user_id", p.User.ID, "error", err)
			} else if deps.billingCacheService != nil {
				if err := deps.billingCacheService.InvalidateUserBalance(billingCtx, p.User.ID); err != nil {
					slog.Warn("invalidate balance cache after legacy deduction failed", "user_id", p.User.ID, "error", err)
				}
			}
		}
	}

	if p.shouldDeductAPIKeyQuota() {
		if err := p.APIKeyService.UpdateQuotaUsed(billingCtx, p.APIKey.ID, cost.ActualCost); err != nil {
			slog.Error("update api key quota failed", "api_key_id", p.APIKey.ID, "error", err)
		}
	}

	if p.shouldUpdateRateLimits() {
		if err := p.APIKeyService.UpdateRateLimitUsage(billingCtx, p.APIKey.ID, cost.ActualCost); err != nil {
			slog.Error("update api key rate limit usage failed", "api_key_id", p.APIKey.ID, "error", err)
		}
	}

	if p.shouldUpdateAccountQuota() {
		accountCost := cost.TotalCost * p.AccountRateMultiplier
		if err := deps.accountRepo.IncrementQuotaUsed(billingCtx, p.Account.ID, accountCost); err != nil {
			slog.Error("increment account quota used failed", "account_id", p.Account.ID, "cost", accountCost, "error", err)
		}
	}

	// Platform quota 累加（legacy 兜底路径）：仅对 standard（余额）模式生效；订阅模式豁免；仅对有 limit 的用户写
	//   - HasUserPlatformQuotaLimit 守卫:与正常路径对齐，无 limit 公司跳过
	//   - 新增 Redis 同步写:enforcement 走 Redis，legacy 路径也必须同步写，否则 preflight 看不到消费
	//   - flusher_enabled=false（降级）:保留原有同步直写 DB
	//   - flusher_enabled=true:跳过直写 DB，由 flusher 异步批量刷（markDirty 在 IncrementUserPlatformQuotaUsage 内部完成）
	//   - 失败仅记 ALERT log + counter，不阻断主扣费流程
	if !p.IsSubscriptionBill && p.Platform != "" && cost.ActualCost > 0 && p.User != nil && deps.userPlatformQuotaRepo != nil {
		if deps.billingCacheService.HasUserPlatformQuotaLimit(billingCtx, p.User.ID, p.Platform) {
			deps.billingCacheService.IncrementUserPlatformQuotaUsage(p.User.ID, p.Platform, cost.ActualCost)
			if deps.cfg == nil || !deps.cfg.Database.UserPlatformQuotaFlusherEnabled {
				// 降级路径:flusher 未启用时保留原有同步直写 DB
				if err := deps.userPlatformQuotaRepo.IncrementUsageWithReset(billingCtx, p.User.ID, p.Platform, cost.ActualCost, time.Now().UTC()); err != nil {
					userPlatformQuotaDBIncrLegacyErrorTotal.Add(1)
					logger.LegacyPrintf("service.gateway", "ALERT: legacy incr user platform quota DB failed user=%d platform=%s cost=%f: %v", p.User.ID, p.Platform, cost.ActualCost, err)
				}
			}
			// flusher_enabled=true:不直写 DB，flusher 异步批量刷
		}
	}

	// NOTE: finalizePostUsageBilling is NOT called here to avoid double-queuing
	// cache updates. The legacy path does DB writes directly; the finalize path
	// does cache queue + notifications. Notifications are dispatched separately
	// by the caller after recording the usage log.
}

func resolveUsageBillingRequestID(ctx context.Context, upstreamRequestID string) string {
	// Forced durable money-event IDs must win over client/local context IDs so
	// standalone web_search / async video cannot collapse under a reused client id.
	if requestID := strings.TrimSpace(upstreamRequestID); requestID != "" {
		if isForcedUsageBillingRequestID(requestID) {
			return requestID
		}
	}
	if ctx != nil {
		if clientRequestID, _ := ctx.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(clientRequestID) != "" {
			return "client:" + strings.TrimSpace(clientRequestID)
		}
		if requestID, _ := ctx.Value(ctxkey.RequestID).(string); strings.TrimSpace(requestID) != "" {
			return "local:" + strings.TrimSpace(requestID)
		}
	}
	if requestID := strings.TrimSpace(upstreamRequestID); requestID != "" {
		return requestID
	}
	return "generated:" + generateRequestID()
}

func isForcedUsageBillingRequestID(requestID string) bool {
	id := strings.TrimSpace(requestID)
	return strings.HasPrefix(id, "web_search:") ||
		strings.HasPrefix(id, "grok-video:") ||
		strings.HasPrefix(id, "grok_audio:") ||
		strings.HasPrefix(id, "grok_realtime:")
}

// StableGrokAudioBillingRequestID is the durable usage_logs / dedup key for one
// voice HTTP call (TTS/STT). Prefer an upstream request id when present.
func StableGrokAudioBillingRequestID(upstreamRequestID string) string {
	upstreamRequestID = strings.TrimSpace(upstreamRequestID)
	if strings.HasPrefix(upstreamRequestID, "grok_audio:") {
		return upstreamRequestID
	}
	if upstreamRequestID == "" {
		upstreamRequestID = generateRequestID()
	}
	return "grok_audio:" + upstreamRequestID
}

// StableGrokRealtimeBillingRequestID is the durable usage_logs / dedup key for
// one realtime WebSocket session.
func StableGrokRealtimeBillingRequestID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if strings.HasPrefix(sessionID, "grok_realtime:") {
		return sessionID
	}
	if sessionID == "" {
		sessionID = generateRequestID()
	}
	return "grok_realtime:" + sessionID
}

func resolveUsageBillingPayloadFingerprint(ctx context.Context, requestPayloadHash string) string {
	if payloadHash := strings.TrimSpace(requestPayloadHash); payloadHash != "" {
		return payloadHash
	}
	if ctx != nil {
		if clientRequestID, _ := ctx.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(clientRequestID) != "" {
			return "client:" + strings.TrimSpace(clientRequestID)
		}
		if requestID, _ := ctx.Value(ctxkey.RequestID).(string); strings.TrimSpace(requestID) != "" {
			return "local:" + strings.TrimSpace(requestID)
		}
	}
	return ""
}

func buildUsageBillingCommand(requestID string, usageLog *UsageLog, p *postUsageBillingParams) *UsageBillingCommand {
	if p == nil || p.Cost == nil || p.APIKey == nil || p.User == nil || p.Account == nil {
		return nil
	}

	cmd := &UsageBillingCommand{
		RequestID:          requestID,
		APIKeyID:           p.APIKey.ID,
		UserID:             p.User.ID,
		AccountID:          p.Account.ID,
		AccountType:        p.Account.Type,
		RequestPayloadHash: strings.TrimSpace(p.RequestPayloadHash),
	}
	if usageLog != nil {
		cmd.Model = usageLog.Model
		cmd.BillingType = usageLog.BillingType
		cmd.InputTokens = usageLog.InputTokens
		cmd.OutputTokens = usageLog.OutputTokens
		cmd.CacheCreationTokens = usageLog.CacheCreationTokens
		cmd.CacheReadTokens = usageLog.CacheReadTokens
		cmd.ImageCount = usageLog.ImageCount
		if usageLog.ServiceTier != nil {
			cmd.ServiceTier = *usageLog.ServiceTier
		}
		if usageLog.ReasoningEffort != nil {
			cmd.ReasoningEffort = *usageLog.ReasoningEffort
		}
		if usageLog.SubscriptionID != nil {
			cmd.SubscriptionID = usageLog.SubscriptionID
		}
	}

	// Record subscription / balance cost using ActualCost so the group (and any
	// user-specific) rate multiplier consumes subscription quota at the expected
	// speed. TotalCost remains the raw (pre-multiplier) value; downstream guards
	// on "> 0" still correctly skip free subscriptions (RateMultiplier == 0).
	if p.IsSubscriptionBill && p.Subscription != nil && p.Cost.TotalCost > 0 {
		cmd.SubscriptionID = &p.Subscription.ID
		cmd.SubscriptionCost = p.Cost.ActualCost
	} else if p.Cost.ActualCost > 0 {
		cmd.BalanceCost = p.Cost.ActualCost
	}

	if p.shouldDeductAPIKeyQuota() {
		cmd.APIKeyQuotaCost = p.Cost.ActualCost
	}
	if p.shouldUpdateRateLimits() {
		cmd.APIKeyRateLimitCost = p.Cost.ActualCost
	}
	if p.shouldUpdateAccountQuota() {
		cmd.AccountQuotaCost = p.Cost.TotalCost * p.AccountRateMultiplier
	}

	cmd.Normalize()
	return cmd
}

func applyUsageBilling(ctx context.Context, requestID string, usageLog *UsageLog, p *postUsageBillingParams, deps *billingDeps, repo UsageBillingRepository) (bool, error) {
	if p == nil || deps == nil {
		return false, nil
	}

	cmd := buildUsageBillingCommand(requestID, usageLog, p)
	if cmd == nil || cmd.RequestID == "" || repo == nil {
		postUsageBilling(ctx, p, deps)
		return true, nil
	}

	billingCtx, cancel := detachedBillingContext(ctx)
	defer cancel()

	result, err := repo.Apply(billingCtx, cmd)
	if err != nil {
		return false, err
	}

	if result == nil || !result.Applied {
		deps.deferredService.ScheduleLastUsedUpdate(p.Account.ID)
		return false, nil
	}

	if result.APIKeyQuotaExhausted {
		if invalidator, ok := p.APIKeyService.(apiKeyAuthCacheInvalidator); ok && p.APIKey != nil && p.APIKey.Key != "" {
			invalidator.InvalidateAuthCacheByKey(billingCtx, p.APIKey.Key)
		}
	}

	finalizePostUsageBilling(billingCtx, p, deps, result)
	return true, nil
}

func finalizePostUsageBilling(ctx context.Context, p *postUsageBillingParams, deps *billingDeps, result *UsageBillingApplyResult) {
	if p == nil || p.Cost == nil || deps == nil {
		return
	}

	if p.IsSubscriptionBill {
		if p.Cost.ActualCost > 0 && p.User != nil && p.APIKey != nil && p.APIKey.GroupID != nil {
			deps.billingCacheService.QueueUpdateSubscriptionUsage(p.User.ID, *p.APIKey.GroupID, p.Cost.ActualCost)
		}
	} else if p.Cost.ActualCost > 0 && p.User != nil {
		syncBalanceCacheAfterDeduction(ctx, p, deps, result)
	}

	if p.Cost.ActualCost > 0 && p.APIKey != nil && p.APIKey.HasRateLimits() {
		deps.billingCacheService.QueueUpdateAPIKeyRateLimitUsage(p.APIKey.ID, p.Cost.ActualCost)
	}

	deps.deferredService.ScheduleLastUsedUpdate(p.Account.ID)

	// Platform quota 累加：仅在 standard（余额）模式生效；订阅模式豁免；仅对有 limit 的用户写
	// Redis 同步写 + DB 异步持久化（flag=false 降级）或 flusher 异步刷（flag=true）:
	//   - HasUserPlatformQuotaLimit 守卫:无 limit 的公司跳过,避免无效写入 + 浪费 Redis 容量
	//   - Redis 同步:确保下次 preflight 立即看到最新 usage,把 TOCTOU 超支窗口
	//     限制在并发 in-flight 请求数量内（旧实现的异步入队会让超支无限累积直到 worker 处理）
	//   - DB 异步(flusher_enabled=false):在独立 goroutine 中走 detached context,失败用 ALERT log 触发 oncall 对账
	//   - flusher_enabled=true:不直写 DB,由 flusher 异步批量刷（markDirty 已在 IncrementUserPlatformQuotaUsage 内部完成）
	if !p.IsSubscriptionBill && p.Platform != "" && p.Cost.ActualCost > 0 && p.User != nil && deps.userPlatformQuotaRepo != nil {
		if deps.billingCacheService.HasUserPlatformQuotaLimit(ctx, p.User.ID, p.Platform) {
			deps.billingCacheService.IncrementUserPlatformQuotaUsage(p.User.ID, p.Platform, p.Cost.ActualCost)
			if deps.cfg == nil || !deps.cfg.Database.UserPlatformQuotaFlusherEnabled {
				// 降级路径:flusher 未启用时保留原有异步直写 DB
				dbCtx, dbCancel := detachUpstreamContext(ctx)
				userID, platform, cost := p.User.ID, p.Platform, p.Cost.ActualCost
				go func() {
					defer func() {
						if r := recover(); r != nil {
							logger.LegacyPrintf("service.gateway", "ALERT: panic in user platform quota incr goroutine user=%d platform=%s: %v", userID, platform, r)
						}
					}()
					defer dbCancel()
					if err := deps.userPlatformQuotaRepo.IncrementUsageWithReset(dbCtx, userID, platform, cost, time.Now().UTC()); err != nil {
						// 失败计数器:暴露给 GatewayUserPlatformQuotaIncrStats(),由 ops 面板做斜率告警。
						userPlatformQuotaDBIncrErrorTotal.Add(1)
						// ALERT 级别:DB 持久化失败意味着 Redis cache 失效后该笔 cost 永久丢失,
						// 用户配额视图与实际消费会偏差,oncall 需要据此对账或人工补录。
						logger.LegacyPrintf("service.gateway", "ALERT: incr user platform quota DB failed user=%d platform=%s cost=%f: %v", userID, platform, cost, err)
					}
				}()
			}
			// flusher_enabled=true:不直写 DB,flusher 异步批量刷
		}
	}

	// Notification checks run async — all parameters are already captured,
	// no dependency on the request context or upstream connection.
	go notifyBalanceLow(p, deps, result)
	go notifyAccountQuota(p, deps, result)
}

func syncBalanceCacheAfterDeduction(ctx context.Context, p *postUsageBillingParams, deps *billingDeps, result *UsageBillingApplyResult) {
	if p == nil || p.Cost == nil || p.User == nil || deps == nil || deps.billingCacheService == nil {
		return
	}
	if result != nil && result.NewBalance != nil && deps.billingCacheService.balanceBelowEligibilityThreshold(*result.NewBalance) {
		if err := deps.billingCacheService.InvalidateUserBalance(ctx, p.User.ID); err != nil {
			slog.Warn("invalidate balance cache after exhausted deduction failed",
				"user_id", p.User.ID,
				"new_balance", *result.NewBalance,
				"balance_overdrafted", result.BalanceOverdrafted,
				"error", err,
			)
		}
		return
	}
	deps.billingCacheService.QueueDeductBalance(p.User.ID, p.Cost.ActualCost)
}

// notifyBalanceLow sends balance low notification after deduction.
// When result.NewBalance is available (from DB transaction RETURNING), it is used directly
// to reconstruct oldBalance, avoiding stale Redis reads and concurrent-deduction races.
func notifyBalanceLow(p *postUsageBillingParams, deps *billingDeps, result *UsageBillingApplyResult) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in notifyBalanceLow", "recover", r)
		}
	}()
	if p.IsSubscriptionBill || p.Cost.ActualCost <= 0 || p.User == nil || deps.balanceNotifyService == nil {
		slog.Debug("notifyBalanceLow: skipped",
			"is_subscription", p.IsSubscriptionBill,
			"actual_cost", p.Cost.ActualCost,
			"user_nil", p.User == nil,
			"service_nil", deps.balanceNotifyService == nil,
		)
		return
	}

	oldBalance := resolveOldBalance(p, result)
	slog.Debug("notifyBalanceLow: calling CheckBalanceAfterDeduction",
		"user_id", p.User.ID,
		"old_balance", oldBalance,
		"cost", p.Cost.ActualCost,
		"notify_enabled", p.User.BalanceNotifyEnabled,
		"threshold", p.User.BalanceNotifyThreshold,
		"result_has_new_balance", result != nil && result.NewBalance != nil,
	)
	deps.balanceNotifyService.CheckBalanceAfterDeduction(context.Background(), p.User, oldBalance, p.Cost.ActualCost)
}

// resolveOldBalance returns the pre-deduction balance.
// Prefers the DB transaction result (newBalance + cost) over snapshot.
func resolveOldBalance(p *postUsageBillingParams, result *UsageBillingApplyResult) float64 {
	if result != nil && result.NewBalance != nil {
		return *result.NewBalance + p.Cost.ActualCost
	}
	// Legacy fallback: snapshot balance from request context
	return p.User.Balance
}

// notifyAccountQuota sends account quota threshold notification after increment.
// When result.QuotaState is available (from DB transaction RETURNING), it is passed directly
// to avoid a separate DB read that may see stale or concurrently-modified data.
func notifyAccountQuota(p *postUsageBillingParams, deps *billingDeps, result *UsageBillingApplyResult) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in notifyAccountQuota", "recover", r)
		}
	}()
	if p.Cost.TotalCost <= 0 || p.Account == nil || !p.Account.IsAPIKeyOrBedrock() || deps.balanceNotifyService == nil {
		slog.Debug("notifyAccountQuota: skipped",
			"total_cost", p.Cost.TotalCost,
			"account_nil", p.Account == nil,
			"is_apikey_or_bedrock", p.Account != nil && p.Account.IsAPIKeyOrBedrock(),
			"service_nil", deps.balanceNotifyService == nil,
		)
		return
	}
	accountCost := p.Cost.TotalCost * p.AccountRateMultiplier
	var quotaState *AccountQuotaState
	if result != nil {
		quotaState = result.QuotaState
	}
	slog.Debug("notifyAccountQuota: calling CheckAccountQuotaAfterIncrement",
		"account_id", p.Account.ID,
		"account_cost", accountCost,
		"has_quota_state", quotaState != nil,
	)
	deps.balanceNotifyService.CheckAccountQuotaAfterIncrement(context.Background(), p.Account, accountCost, quotaState)
}

func detachedBillingContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, postUsageBillingTimeout)
}

func detachStreamUpstreamContext(ctx context.Context, stream bool) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.Background(), func() {}
	}
	if !stream {
		return ctx, func() {}
	}
	return context.WithoutCancel(ctx), func() {}
}

func detachUpstreamContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.Background(), func() {}
	}
	return context.WithoutCancel(ctx), func() {}
}

// billingDeps 扣费逻辑依赖的服务（由各 gateway service 提供）
type billingDeps struct {
	accountRepo           AccountRepository
	userRepo              UserRepository
	userSubRepo           UserSubscriptionRepository
	billingCacheService   *BillingCacheService
	deferredService       *DeferredService
	balanceNotifyService  *BalanceNotifyService
	userPlatformQuotaRepo UserPlatformQuotaRepository
	cfg                   *config.Config
}

func (s *GatewayService) billingDeps() *billingDeps {
	return &billingDeps{
		accountRepo:           s.accountRepo,
		userRepo:              s.userRepo,
		userSubRepo:           s.userSubRepo,
		billingCacheService:   s.billingCacheService,
		deferredService:       s.deferredService,
		balanceNotifyService:  s.balanceNotifyService,
		userPlatformQuotaRepo: s.userPlatformQuotaRepo,
		cfg:                   s.cfg,
	}
}

func writeUsageLogBestEffort(ctx context.Context, repo UsageLogRepository, usageLog *UsageLog, logKey string) {
	if repo == nil || usageLog == nil {
		return
	}
	usageCtx, cancel := detachedBillingContext(ctx)
	defer cancel()

	if writer, ok := repo.(usageLogBestEffortWriter); ok {
		if err := writer.CreateBestEffort(usageCtx, usageLog); err != nil {
			logger.LegacyPrintf(logKey, "Create usage log failed: %v", err)
			// 计费已在此前完成，日志必须落库：dropped（批处理队列超时）同样走同步兜底，
			// 否则会出现“已扣费但无 usage_log”的对账缺口（issue #3656）。
			// 重复写入由 usage_logs 的 ON CONFLICT (request_id, api_key_id) DO NOTHING 防护。
			fallbackCtx := usageCtx
			if usageCtx.Err() != nil {
				// usageCtx 已耗尽（best-effort 入队阻塞到期限）：换新的 detached 窗口，避免兜底必然失败。
				var fallbackCancel context.CancelFunc
				fallbackCtx, fallbackCancel = detachedBillingContext(context.Background())
				defer fallbackCancel()
			}
			if _, syncErr := repo.Create(fallbackCtx, usageLog); syncErr != nil {
				logger.LegacyPrintf(logKey, "Create usage log sync fallback failed: %v", syncErr)
			}
		}
		return
	}

	if _, err := repo.Create(usageCtx, usageLog); err != nil {
		logger.LegacyPrintf(logKey, "Create usage log failed: %v", err)
	}
}

// recordUsageOpts 内部选项，参数化普通计费与长上下文计费的差异点。
type recordUsageOpts struct {
	// 长上下文计费（仅 Gemini 路径需要）
	LongContextThreshold  int
	LongContextMultiplier float64
}

// RecordUsage 记录使用量并扣费（或更新订阅用量）
func (s *GatewayService) RecordUsage(ctx context.Context, input *RecordUsageInput) error {
	return s.recordUsageCore(ctx, &recordUsageCoreInput{
		Result:             input.Result,
		APIKey:             input.APIKey,
		User:               input.User,
		Account:            input.Account,
		Subscription:       input.Subscription,
		PricingAt:          input.PricingAt,
		InboundEndpoint:    input.InboundEndpoint,
		UpstreamEndpoint:   input.UpstreamEndpoint,
		UserAgent:          input.UserAgent,
		IPAddress:          input.IPAddress,
		SessionID:          input.SessionID,
		RequestPayloadHash: input.RequestPayloadHash,
		ForceCacheBilling:  input.ForceCacheBilling,
		APIKeyService:      input.APIKeyService,
		QuotaPlatform:      input.QuotaPlatform,
		ChannelUsageFields: input.ChannelUsageFields,
	}, &recordUsageOpts{})
}

// RecordUsageLongContextInput 记录使用量的输入参数（支持长上下文双倍计费）
type RecordUsageLongContextInput struct {
	Result                *ForwardResult
	APIKey                *APIKey
	User                  *User
	Account               *Account
	Subscription          *UserSubscription  // 可选：订阅信息
	PricingAt             time.Time          // token 售价固定时刻；零值保持既有的记录时刻语义
	InboundEndpoint       string             // 入站端点（客户端请求路径）
	UpstreamEndpoint      string             // 上游端点（标准化后的上游路径）
	UserAgent             string             // 请求的 User-Agent
	IPAddress             string             // 请求的客户端 IP 地址
	SessionID             string             // 客户端显式会话标识（session_id / X-Session-Id 等请求头），仅用于用量行会话关联
	RequestPayloadHash    string             // 请求体语义哈希，用于降低 request_id 误复用时的静默误去重风险
	LongContextThreshold  int                // 长上下文阈值（如 200000）
	LongContextMultiplier float64            // 超出阈值部分的倍率（如 2.0）
	ForceCacheBilling     bool               // 强制缓存计费：将 input_tokens 转为 cache_read 计费（用于粘性会话切换）
	APIKeyService         APIKeyQuotaUpdater // API Key 配额服务（可选）
	QuotaPlatform         string             // user×platform 配额计量平台：handler 在请求 ctx 内经 QuotaPlatform() 算定后传入（后扣运行在 worker 池 background ctx 上，取不到 ForcePlatform）

	ChannelUsageFields // 渠道映射信息（由 handler 在 Forward 前解析）
}

// RecordUsageWithLongContext 记录使用量并扣费，支持长上下文双倍计费（用于 Gemini）
func (s *GatewayService) RecordUsageWithLongContext(ctx context.Context, input *RecordUsageLongContextInput) error {
	return s.recordUsageCore(ctx, &recordUsageCoreInput{
		Result:             input.Result,
		APIKey:             input.APIKey,
		User:               input.User,
		Account:            input.Account,
		Subscription:       input.Subscription,
		PricingAt:          input.PricingAt,
		InboundEndpoint:    input.InboundEndpoint,
		UpstreamEndpoint:   input.UpstreamEndpoint,
		UserAgent:          input.UserAgent,
		IPAddress:          input.IPAddress,
		SessionID:          input.SessionID,
		RequestPayloadHash: input.RequestPayloadHash,
		ForceCacheBilling:  input.ForceCacheBilling,
		APIKeyService:      input.APIKeyService,
		QuotaPlatform:      input.QuotaPlatform,
		ChannelUsageFields: input.ChannelUsageFields,
	}, &recordUsageOpts{
		LongContextThreshold:  input.LongContextThreshold,
		LongContextMultiplier: input.LongContextMultiplier,
	})
}

// recordUsageCoreInput 是 recordUsageCore 的公共输入字段，从两种输入结构体中提取。
type recordUsageCoreInput struct {
	Result             *ForwardResult
	APIKey             *APIKey
	User               *User
	Account            *Account
	Subscription       *UserSubscription
	PricingAt          time.Time
	InboundEndpoint    string
	UpstreamEndpoint   string
	UserAgent          string
	IPAddress          string
	SessionID          string
	RequestPayloadHash string
	ForceCacheBilling  bool
	APIKeyService      APIKeyQuotaUpdater
	QuotaPlatform      string
	ChannelUsageFields
}

// responseModelBillingCostEpsilon 吸收两次成本计算之间的浮点末位误差，
// 避免同价模型因浮点误差被判成"更贵"而白白放弃采纳。
const responseModelBillingCostEpsilon = 1e-12

// responseModelBillingDeclaration 返回可用于计费的上游响应模型；返回空字符串表示
// 必须沿用基线计费模型。两条计费主干（Anthropic 系 / OpenAI 系）共用本准入判断。
//
// 渠道把 billing_model_source 设为 response_model，等于把"按哪个模型计价"的一部分
// 决定权交给上游，因此准入条件必须收紧：
//   - 只在渠道显式开启该模式时生效，其余模式一律不看响应模型；
//   - 一次请求内出现过互相冲突的模型声明时不采纳（无法确定上游究竟服务了哪个模型）；
//   - 图片 / 视频 / 网页搜索 / 语音 / 搜索附加费这类按次按量计费的请求不采纳：它们按张、
//     按秒、按次定价，与本模式的 token 定价准入检查不是同一套价格表，混用会让一个只验过
//     token 价的模型名去决定媒体单价。新增按次计费形态时必须同步扩这个入参。
//
// 调用方还必须额外满足两条：模型能被价格表确定性识别（见
// hasIdentifiedResponseModelPricing / hasIdentifiedOpenAIResponsePricing），以及通过
// responseModelBillingAdoptable 的成本准入。
func responseModelBillingDeclaration(source, responseModel string, conflict, mediaBilled bool) string {
	if source != BillingModelSourceResponse || conflict || mediaBilled {
		return ""
	}
	return strings.TrimSpace(responseModel)
}

// responseModelBillingAdoptable 判定按响应模型重算出的成本能否取代基线成本。
// 三条不变式，任一不满足都必须沿用基线（即开启本模式前的既有行为）：
//
//  1. 不得更贵——上游声明永远不能抬高用户费用；epsilon 吸收两次计算之间的浮点末位误差。
//  2. 不得把一笔本应计费的请求归零。价格表里存在把 token 价显式写成 0 的条目
//     （TokenPricingAbsent 只在 input/output 价**都缺失**时才为真，显式 0 算"有价"因而
//     能通过确定性识别那道门），放任归零等于让上游自报一个免费模型名就能白嫖。
//     基线本身就是 0 时不受影响，采纳与否都不改变金额。
//  3. 不得把计费从管理员显式配置的渠道定价切到全局价格表。渠道定价查表只做精确键与
//     前缀通配、**不剥日期后缀**，而全局价格表的确定性识别**会剥** 8 位日期后缀；上游
//     普遍自报带日期的模型 ID（如 claude-opus-4-5-20251101），若允许跨源比较，渠道加价
//     会被这类自报名字静默绕过。管理员若确实想让降级目标享受折扣，为它显式配一条渠道
//     定价即可——那是一次可审计的显式授权。
func responseModelBillingAdoptable(baseline, response *CostBreakdown, baselineChannelPriced, responseChannelPriced bool) bool {
	if baseline == nil || response == nil {
		return false
	}
	if response.TotalCost > baseline.TotalCost+responseModelBillingCostEpsilon {
		return false
	}
	if response.TotalCost <= 0 && baseline.TotalCost > 0 {
		return false
	}
	return !baselineChannelPriced || responseChannelPriced
}

// logResponseModelBillingApplied 记录一次实际生效的响应模型计费切换。
// 本模式下的少收由上游声明驱动，必须留下可审计痕迹；计费基准未变时不记录，避免刷屏。
func logResponseModelBillingApplied(component string, account *Account, requestID, baselineModel, responseModel string, baselineCost, responseCost *CostBreakdown) {
	baselineModel = strings.TrimSpace(baselineModel)
	responseModel = strings.TrimSpace(responseModel)
	if strings.EqualFold(baselineModel, responseModel) {
		return
	}
	attrs := []any{
		"component", component,
		"request_id", strings.TrimSpace(requestID),
		"baseline_model", baselineModel,
		"response_model", responseModel,
	}
	if baselineCost != nil && responseCost != nil {
		attrs = append(attrs, "baseline_cost", baselineCost.TotalCost, "billed_cost", responseCost.TotalCost)
	}
	if account != nil {
		attrs = append(attrs, "platform", account.Platform, "account_id", account.ID)
	}
	slog.Info("billing.response_model_applied", attrs...)
}

// recordUsageCore 是 RecordUsage 和 RecordUsageWithLongContext 的统一实现。
// LongContextThreshold > 0 时 Token 计费回退走 CalculateCostWithLongContext。
func (s *GatewayService) recordUsageCore(ctx context.Context, input *recordUsageCoreInput, opts *recordUsageOpts) error {
	result := input.Result
	apiKey := input.APIKey
	user := input.User
	account := input.Account
	subscription := input.Subscription
	ApplyForwardImageBillingResolution(result)

	// 强制缓存计费：将 input_tokens 转为 cache_read_input_tokens
	// 用于粘性会话切换时的特殊计费处理
	if input.ForceCacheBilling && result.Usage.InputTokens > 0 {
		logger.LegacyPrintf("service.gateway", "force_cache_billing: %d input_tokens → cache_read_input_tokens (account=%d)",
			result.Usage.InputTokens, account.ID)
		result.Usage.CacheReadInputTokens += result.Usage.InputTokens
		result.Usage.InputTokens = 0
	}

	// Cache TTL Override: 确保计费时 token 分类与账号设置一致。
	// 账号级设置优先；全局 1h 请求注入开启时，默认把 usage 计费归回 5m。
	cacheTTLOverridden := false
	if overrideTarget, ok := s.resolveCacheTTLUsageOverrideTarget(ctx, account); ok {
		applyCacheTTLOverride(&result.Usage, overrideTarget)
		cacheTTLOverridden = (result.Usage.CacheCreation5mTokens + result.Usage.CacheCreation1hTokens) > 0
	}

	// 获取费率倍数（优先级：用户专属 > 分组默认 > 系统默认）
	multiplier := 1.0
	if s.cfg != nil {
		multiplier = s.cfg.Default.RateMultiplier
	}
	if apiKey.GroupID != nil && apiKey.Group != nil {
		groupDefault := apiKey.Group.RateMultiplier
		multiplier = s.ResolveUserGroupRateMultiplier(ctx, user.ID, *apiKey.GroupID, groupDefault)
	}
	// token 倍率叠加高峰因子（token 计费含图片 token，图片按次倍率不受影响）。高峰因子按请求时刻现算，
	// 不并入上面的 getUserGroupRateMultiplier，以免污染 user:group 倍率缓存。
	pricingAt := input.PricingAt
	if pricingAt.IsZero() {
		pricingAt = timezone.Now()
	}
	multiplier, imageMultiplier := computePeakAwareMultipliers(apiKey, multiplier, pricingAt)

	// 确定计费模型
	concreteBillingModel := forwardResultBillingModel(result.Model, result.UpstreamModel)
	billingModel := concreteBillingModel
	if input.BillingModelSource == BillingModelSourceChannelMapped && input.ChannelMappedModel != "" {
		billingModel = input.ChannelMappedModel
	}
	if input.BillingModelSource == BillingModelSourceRequested && input.OriginalModel != "" {
		billingModel = input.OriginalModel
	}
	// composite 分组的公开别名（如 all/claude）会经 OriginalModel/ChannelMappedModel
	// 进入上面的来源覆盖：任意别名查无价会静默落 $0，含家族词的别名则被价格表的
	// 家族模糊匹配错计（如 Opus 流量按 Sonnet 兜底价）。除非管理员为别名显式配置了
	// 渠道定价（OpenRouter 式自定价），composite 请求一律按实际转发的具体模型计费。
	if apiKey.Group != nil && apiKey.Group.Platform == PlatformComposite {
		billingModel = s.compositeBillableModel(ctx, apiKey, billingModel, concreteBillingModel)
	}
	// 通用兜底（与 OpenAI 路径的 usageBillingModelCandidates 语义对齐）：
	// 选定模型查不到任何价格时回退到实际转发的具体模型。已定价流量不受影响。
	billingModel = s.billableModelWithFallback(ctx, apiKey, billingModel, result.UpstreamModel, result.Model)

	// 确定 RequestedModel（渠道映射前的原始模型）
	requestedModel := result.Model
	if input.OriginalModel != "" {
		requestedModel = input.OriginalModel
	}

	// 计算费用
	cost := s.calculateRecordUsageCost(ctx, result, apiKey, billingModel, multiplier, imageMultiplier, pricingAt, opts)
	// response_model：按上游成功响应自报的模型计费（渠道显式开启才生效）。
	// 采纳条件见 responseModelBillingDeclaration + hasIdentifiedResponseModelPricing
	// + responseModelBillingAdoptable。任一条件不满足都静默回落基线，即开启本模式前的
	// 既有行为。响应模型与基线同名时直接跳过：重算必然同价，白跑一次定价解析。
	if responseModel := responseModelBillingDeclaration(
		input.BillingModelSource,
		result.UpstreamResponseModel,
		result.UpstreamResponseModelConflict,
		result.ImageCount > 0 || result.AudioUsage != nil || result.SearchCount > 0,
	); responseModel != "" && !strings.EqualFold(responseModel, strings.TrimSpace(billingModel)) {
		if identified, responseChannelPriced := s.hasIdentifiedResponseModelPricing(ctx, responseModel, apiKey); identified {
			responseCost := s.calculateRecordUsageCost(ctx, result, apiKey, responseModel, multiplier, imageMultiplier, pricingAt, opts)
			baselineChannelPriced := s.resolveChannelPricing(ctx, billingModel, apiKey) != nil
			if responseModelBillingAdoptable(cost, responseCost, baselineChannelPriced, responseChannelPriced) {
				// billingModel 到此为止只是定价查表的入参，后续流程只消费 cost，
				// 因此这里不改写它，改由日志记录实际生效的计费基准。
				logResponseModelBillingApplied("service.gateway", account, result.RequestID, billingModel, responseModel, cost, responseCost)
				cost = responseCost
			}
		}
	}

	// 判断计费方式：订阅模式 vs 余额模式
	isSubscriptionBilling := subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
	}

	// 创建使用日志
	accountRateMultiplier := account.BillingRateMultiplier()
	usageLog := s.buildRecordUsageLog(ctx, input, result, apiKey, user, account, subscription,
		requestedModel, multiplier, imageMultiplier, accountRateMultiplier, billingType, cacheTTLOverridden, cost, opts)

	// 计算账号统计定价费用（使用最终上游模型匹配自定义规则）
	if apiKey.GroupID != nil {
		applyAccountStatsCost(ctx, usageLog, s.channelService, s.billingService,
			account.ID, *apiKey.GroupID, result.UpstreamModel, result.Model,
			// Anthropic's input_tokens excludes cache_read and cache_creation (billed separately);
			// OpenAI gateway uses actualInputTokens which also excludes cache_read for the same reason.
			UsageTokens{
				InputTokens:         result.Usage.InputTokens,
				OutputTokens:        result.Usage.OutputTokens,
				CacheCreationTokens: result.Usage.CacheCreationInputTokens,
				CacheReadTokens:     result.Usage.CacheReadInputTokens,
				ImageOutputTokens:   result.Usage.ImageOutputTokens,
			},
			cost.TotalCost,
		)
	}

	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.gateway")
		logger.LegacyPrintf("service.gateway", "[SIMPLE MODE] Usage recorded (not billed): user=%d, tokens=%d", usageLog.UserID, usageLog.TotalTokens())
		s.deferredService.ScheduleLastUsedUpdate(account.ID)
		return nil
	}

	// 配额平台由 handler 在请求 ctx 内经 QuotaPlatform() 算定并通过 input 传入；
	// 后扣运行在 worker 池的 background ctx 上，无法再从 ctx 取 ForcePlatform。
	// 缺省（未设置）时回退到分组平台，保持对其它调用方的兼容。
	quotaPlatform := input.QuotaPlatform
	if quotaPlatform == "" {
		quotaPlatform = PlatformFromAPIKey(apiKey)
		if quotaPlatform == PlatformComposite && account != nil {
			quotaPlatform = account.Platform
		}
	}
	requestID := usageLog.RequestID
	_, billingErr := applyUsageBilling(ctx, requestID, usageLog, &postUsageBillingParams{
		Cost:                  cost,
		User:                  user,
		APIKey:                apiKey,
		Account:               account,
		Subscription:          subscription,
		RequestPayloadHash:    resolveUsageBillingPayloadFingerprint(ctx, input.RequestPayloadHash),
		IsSubscriptionBill:    isSubscriptionBilling,
		AccountRateMultiplier: accountRateMultiplier,
		APIKeyService:         input.APIKeyService,
		Platform:              quotaPlatform,
	}, s.billingDeps(), s.usageBillingRepo)

	if billingErr != nil {
		usageLog.ActualCost = 0
		writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.gateway")
		return billingErr
	}
	writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.gateway")

	return nil
}

// calculateRecordUsageCost 根据请求类型和选项计算费用。
func (s *GatewayService) calculateRecordUsageCost(
	ctx context.Context,
	result *ForwardResult,
	apiKey *APIKey,
	billingModel string,
	multiplier float64,
	imageMultiplier float64,
	pricingAt time.Time,
	opts *recordUsageOpts,
) *CostBreakdown {
	// 图片生成：渠道定价为 token 计费时走 token 路径，否则走图片计费
	if result.ImageCount > 0 {
		if resolved := s.resolveChannelPricing(ctx, billingModel, apiKey); resolved != nil && resolved.Mode == BillingModeToken {
			return s.calculateTokenCost(ctx, result, apiKey, billingModel, multiplier, pricingAt, opts)
		}
		return s.calculateImageCost(ctx, result, apiKey, billingModel, imageMultiplier)
	}

	// Voice audio (TTS / STT / realtime) when present on the forward result.
	if result.AudioUsage != nil {
		if resolved := s.resolveChannelPricing(ctx, billingModel, apiKey); resolved != nil &&
			resolved.Mode == BillingModePerRequest {
			gid := apiKey.Group.ID
			cost, err := s.billingService.CalculateCostUnified(CostInput{
				Ctx: ctx, Model: billingModel, GroupID: &gid, Group: apiKey.Group,
				UsageUnits: result.AudioUsage.DurationOrUnits, SizeTier: result.AudioUsage.Mode,
				RateMultiplier: multiplier, Resolver: s.resolver, Resolved: resolved,
			})
			if err == nil {
				return cost
			}
		}
		cfg := groupAudioPriceConfigFromAPIKey(apiKey)
		return s.billingService.CalculateAudioCost(result.AudioUsage.Mode, result.AudioUsage.DurationOrUnits, cfg, multiplier)
	}

	// Token 计费；SearchCount 为叠加 surcharge（不替代 token）。
	tokenCost := s.calculateTokenCost(ctx, result, apiKey, billingModel, multiplier, pricingAt, opts)
	if result.SearchCount > 0 {
		price := groupSearchPricePer1kFromAPIKey(apiKey)
		if price != nil && *price == 0 {
			logger.LegacyPrintf("service.gateway", "[Billing] search_price_per_1k explicit 0; search free group_model=%s count=%d", billingModel, result.SearchCount)
		}
		searchCost := s.billingService.CalculateSearchCost(result.SearchCount, price, multiplier)
		if searchCost != nil && (searchCost.TotalCost > 0 || searchCost.ActualCost > 0) {
			if tokenCost == nil {
				return searchCost
			}
			tokenCost.TotalCost += searchCost.TotalCost
			tokenCost.ActualCost += searchCost.ActualCost
		}
	}
	return tokenCost
}

// compositeBillableModel 决定 composite 分组请求的计费模型：来源覆盖把计费模型
// 换成公开别名等非具体模型时，只有管理员为该名字显式配置了渠道定价才按其计费
// （OpenRouter 式自定价），否则回退到实际转发的具体模型，避免别名落入价格表的
// 家族模糊匹配（错价）或查无价（$0）。未发生来源覆盖时原样返回。
func (s *GatewayService) compositeBillableModel(ctx context.Context, apiKey *APIKey, billingModel, concreteBillingModel string) string {
	if concreteBillingModel == "" || billingModel == concreteBillingModel {
		return billingModel
	}
	if s.resolveChannelPricing(ctx, billingModel, apiKey) != nil {
		return billingModel
	}
	logger.LegacyPrintf("service.gateway", "[Billing] composite billing model %q has no explicit channel pricing, billing by concrete model %q", billingModel, concreteBillingModel)
	return concreteBillingModel
}

// billableModelWithFallback 在选定计费模型（可能是 composite 公开别名或未定价的映射名）
// 查不到任何价格（渠道价与全局价均无）时，按序回退到实际转发的具体模型，避免静默 $0 计费。
// 所有候选都无价时保持原值，走既有的 warn + 零成本路径。
func (s *GatewayService) billableModelWithFallback(ctx context.Context, apiKey *APIKey, billingModel string, fallbacks ...string) string {
	if s.hasResolvableTokenPricing(ctx, billingModel, apiKey) {
		return billingModel
	}
	for _, fallback := range fallbacks {
		fallback = strings.TrimSpace(fallback)
		if fallback == "" || fallback == billingModel {
			continue
		}
		if s.hasResolvableTokenPricing(ctx, fallback, apiKey) {
			logger.LegacyPrintf("service.gateway", "[Billing] billing model %q has no pricing, falling back to concrete model %q", billingModel, fallback)
			return fallback
		}
	}
	return billingModel
}

// hasResolvableTokenPricing 判断模型是否能在渠道定价或全局价格表中解析出 token 价格。
func (s *GatewayService) hasResolvableTokenPricing(ctx context.Context, model string, apiKey *APIKey) bool {
	if strings.TrimSpace(model) == "" {
		return false
	}
	if s.resolveChannelPricing(ctx, model, apiKey) != nil {
		return true
	}
	if s.billingService == nil {
		return false
	}
	_, err := s.billingService.GetModelPricing(model)
	return err == nil
}

// hasIdentifiedResponseModelPricing 判断上游自报的响应模型是否可以作为计费基准，
// 并回传它是否解析到了渠道级定价（供 responseModelBillingAdoptable 的跨定价源守卫使用，
// 避免为此再解析一次）。
// 与 hasResolvableTokenPricing 的区别是刻意更严：只接受管理员为该模型显式配置的
// 渠道定价，或价格表中能被确定性识别的条目；不接受按子串猜出来的系列兜底价。
// 详见 responseModelBillingDeclaration 的说明。
func (s *GatewayService) hasIdentifiedResponseModelPricing(ctx context.Context, model string, apiKey *APIKey) (identified bool, channelPriced bool) {
	if strings.TrimSpace(model) == "" {
		return false, false
	}
	if s.resolveChannelPricing(ctx, model, apiKey) != nil {
		return true, true
	}
	return s.billingService.HasIdentifiedTokenPricing(model), false
}

// resolveChannelPricing 检查指定模型是否存在渠道级别定价。
// 返回非 nil 的 ResolvedPricing 表示有渠道定价，nil 表示走默认定价路径。
func (s *GatewayService) resolveChannelPricing(ctx context.Context, billingModel string, apiKey *APIKey) *ResolvedPricing {
	if s.resolver == nil || apiKey.Group == nil {
		return nil
	}
	gid := apiKey.Group.ID
	resolved := s.resolver.Resolve(ctx, PricingInput{Model: billingModel, GroupID: &gid, Group: apiKey.Group})
	if resolved.Source == PricingSourceGroup || resolved.Source == PricingSourceChannel {
		return resolved
	}
	return nil
}

// calculateImageCost 计算图片生成费用：渠道级别定价优先，否则走按次计费。
func (s *GatewayService) calculateImageCost(
	ctx context.Context,
	result *ForwardResult,
	apiKey *APIKey,
	billingModel string,
	multiplier float64,
) *CostBreakdown {
	sizeTier := NormalizeImageBillingTierOrDefault(result.ImageSize)
	resolved := s.resolveChannelPricing(ctx, billingModel, apiKey)
	if resolved != nil && resolved.Source == PricingSourceGroup {
		gid := apiKey.Group.ID
		cost, err := s.billingService.CalculateCostUnified(CostInput{
			Ctx: ctx, Model: billingModel, GroupID: &gid, Group: apiKey.Group,
			RequestCount: result.ImageCount, SizeTier: sizeTier,
			RateMultiplier: multiplier, Resolver: s.resolver, Resolved: resolved,
		})
		if err == nil {
			return cost
		}
	}
	groupConfig := imagePriceConfigFromAPIKey(apiKey)
	if apiKeyHasConfiguredImagePrice(apiKey, sizeTier) {
		return s.billingService.CalculateImageCost(billingModel, sizeTier, result.ImageCount, groupConfig, multiplier)
	}
	if resolved != nil && resolved.Source == PricingSourceChannel {
		tokens := UsageTokens{
			InputTokens:       result.Usage.InputTokens,
			OutputTokens:      result.Usage.OutputTokens,
			ImageOutputTokens: result.Usage.ImageOutputTokens,
		}
		gid := apiKey.Group.ID
		cost, err := s.billingService.CalculateCostUnified(CostInput{
			Ctx:            ctx,
			Model:          billingModel,
			GroupID:        &gid,
			Group:          apiKey.Group,
			Tokens:         tokens,
			RequestCount:   result.ImageCount,
			SizeTier:       sizeTier,
			RateMultiplier: multiplier,
			Resolver:       s.resolver,
			Resolved:       resolved,
		})
		if err != nil {
			logger.LegacyPrintf("service.gateway", "Calculate image token cost failed: %v", err)
			return &CostBreakdown{ActualCost: 0}
		}
		return cost
	}

	return s.billingService.CalculateImageCost(billingModel, sizeTier, result.ImageCount, groupConfig, multiplier)
}

// calculateTokenCost 计算 Token 计费：根据 opts 决定走普通/长上下文/渠道统一计费。
func (s *GatewayService) calculateTokenCost(
	ctx context.Context,
	result *ForwardResult,
	apiKey *APIKey,
	billingModel string,
	multiplier float64,
	pricingAt time.Time,
	opts *recordUsageOpts,
) *CostBreakdown {
	tokens := UsageTokens{
		InputTokens:           result.Usage.InputTokens,
		OutputTokens:          result.Usage.OutputTokens,
		CacheCreationTokens:   result.Usage.CacheCreationInputTokens,
		CacheReadTokens:       result.Usage.CacheReadInputTokens,
		CacheCreation5mTokens: result.Usage.CacheCreation5mTokens,
		CacheCreation1hTokens: result.Usage.CacheCreation1hTokens,
		ImageOutputTokens:     result.Usage.ImageOutputTokens,
	}

	var cost *CostBreakdown
	var err error

	// Explicit group/channel pricing wins. Built-in pricing also uses the unified
	// resolver so the group long-context toggle can veto model-native tiers.
	if resolved := s.resolveChannelPricing(ctx, billingModel, apiKey); resolved != nil {
		gid := apiKey.Group.ID
		cost, err = s.billingService.CalculateCostUnified(CostInput{
			Ctx:            ctx,
			Model:          billingModel,
			GroupID:        &gid,
			Group:          apiKey.Group,
			Tokens:         tokens,
			RequestCount:   1,
			RateMultiplier: multiplier,
			PricingAt:      pricingAt,
			ServiceTier:    optionalStringValue(result.ServiceTier),
			Resolver:       s.resolver,
			Resolved:       resolved,
		})
	} else if opts.LongContextThreshold > 0 && (apiKey.Group == nil || apiKey.Group.LongContextPricingEnabled) {
		// 长上下文双倍计费（如 Gemini 200K 阈值）
		cost, err = s.billingService.CalculateCostWithLongContext(billingModel, tokens, multiplier, opts.LongContextThreshold, opts.LongContextMultiplier)
	} else if s.resolver != nil && apiKey.Group != nil {
		gid := apiKey.Group.ID
		cost, err = s.billingService.CalculateCostUnified(CostInput{
			Ctx: ctx, Model: billingModel, GroupID: &gid, Group: apiKey.Group,
			Tokens: tokens, RequestCount: 1, RateMultiplier: multiplier, PricingAt: pricingAt,
			ServiceTier: optionalStringValue(result.ServiceTier), Resolver: s.resolver,
		})
	} else {
		cost, err = s.billingService.CalculateCost(billingModel, tokens, multiplier)
	}
	if err != nil {
		logger.LegacyPrintf("service.gateway", "Calculate cost failed: %v", err)
		return &CostBreakdown{ActualCost: 0}
	}
	return cost
}

// buildRecordUsageLog 构建使用日志并设置计费模式。
func (s *GatewayService) buildRecordUsageLog(
	ctx context.Context,
	input *recordUsageCoreInput,
	result *ForwardResult,
	apiKey *APIKey,
	user *User,
	account *Account,
	subscription *UserSubscription,
	requestedModel string,
	multiplier float64,
	imageMultiplier float64,
	accountRateMultiplier float64,
	billingType int8,
	cacheTTLOverridden bool,
	cost *CostBreakdown,
	opts *recordUsageOpts,
) *UsageLog {
	durationMs := int(result.Duration.Milliseconds())
	requestID := resolveUsageBillingRequestID(ctx, result.RequestID)
	sentModel := upstreamSentModel(result.Model, result.UpstreamModel)
	if result.UpstreamResponseModelConflict {
		slog.Warn("upstream_response_model_conflict",
			"platform", account.Platform,
			"account_id", account.ID,
			"request_id", requestID,
			"sent_model", sentModel,
			"selected_response_model", strings.TrimSpace(result.UpstreamResponseModel),
		)
	}
	usageLog := &UsageLog{
		UserID:                user.ID,
		APIKeyID:              apiKey.ID,
		AccountID:             account.ID,
		RequestID:             requestID,
		Model:                 result.Model,
		RequestedModel:        requestedModel,
		UpstreamModel:         optionalTrimmedStringPtr(result.UpstreamModel),
		UpstreamResponseModel: optionalTrimmedStringPtr(result.UpstreamResponseModel),
		UpstreamModelMismatch: upstreamModelMismatch(sentModel, result.UpstreamResponseModel),
		ServiceTier:           result.ServiceTier,
		ReasoningEffort:       result.ReasoningEffort,
		InboundEndpoint:       optionalTrimmedStringPtr(input.InboundEndpoint),
		UpstreamEndpoint:      optionalTrimmedStringPtr(input.UpstreamEndpoint),
		InputTokens:           result.Usage.InputTokens,
		OutputTokens:          result.Usage.OutputTokens,
		CacheCreationTokens:   result.Usage.CacheCreationInputTokens,
		CacheReadTokens:       result.Usage.CacheReadInputTokens,
		CacheCreation5mTokens: result.Usage.CacheCreation5mTokens,
		CacheCreation1hTokens: result.Usage.CacheCreation1hTokens,
		ImageOutputTokens:     result.Usage.ImageOutputTokens,
		RateMultiplier:        multiplier,
		AccountRateMultiplier: &accountRateMultiplier,
		BillingType:           billingType,
		BillingMode:           resolveBillingMode(result, cost),
		Stream:                result.Stream,
		DurationMs:            &durationMs,
		FirstTokenMs:          result.FirstTokenMs,
		ImageCount:            result.ImageCount,
		ImageSize:             optionalTrimmedStringPtr(result.ImageSize),
		ImageInputSize:        optionalTrimmedStringPtr(result.ImageInputSize),
		ImageOutputSize:       optionalTrimmedStringPtr(result.ImageOutputSize),
		ImageSizeSource:       optionalTrimmedStringPtr(result.ImageSizeSource),
		ImageSizeBreakdown:    result.ImageSizeBreakdown,
		CacheTTLOverridden:    cacheTTLOverridden,
		ChannelID:             optionalInt64Ptr(input.ChannelID),
		ModelMappingChain:     optionalTrimmedStringPtr(input.ModelMappingChain),
		UserAgent:             optionalTrimmedStringPtr(input.UserAgent),
		IPAddress:             optionalTrimmedStringPtr(input.IPAddress),
		SessionID:             optionalTrimmedStringPtr(input.SessionID),
		GroupID:               apiKey.GroupID,
		SubscriptionID:        optionalSubscriptionID(subscription),
		CreatedAt:             time.Now(),
	}
	if result.ImageCount > 0 && (cost == nil || cost.BillingMode != string(BillingModeToken)) {
		usageLog.RateMultiplier = imageMultiplier
	}
	if cost != nil {
		usageLog.InputCost = cost.InputCost
		usageLog.OutputCost = cost.OutputCost
		usageLog.ImageOutputCost = cost.ImageOutputCost
		usageLog.CacheCreationCost = cost.CacheCreationCost
		usageLog.CacheReadCost = cost.CacheReadCost
		usageLog.TotalCost = cost.TotalCost
		usageLog.ActualCost = cost.ActualCost
		usageLog.LongContextBillingApplied = cost.LongContextBillingApplied
	}

	return usageLog
}

// resolveBillingMode 根据计费结果和请求类型确定计费模式。
func resolveBillingMode(result *ForwardResult, cost *CostBreakdown) *string {
	var mode string
	switch {
	case cost != nil && cost.BillingMode != "":
		mode = cost.BillingMode
	case result.ImageCount > 0:
		mode = string(BillingModeImage)
	default:
		mode = string(BillingModeToken)
	}
	return &mode
}

func optionalSubscriptionID(subscription *UserSubscription) *int64 {
	if subscription != nil {
		return &subscription.ID
	}
	return nil
}
