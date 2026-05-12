package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const postUsageBillingTimeout = 15 * time.Second

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

// billingDeps 扣费逻辑依赖的服务（由各 gateway service 提供）
type billingDeps struct {
	accountRepo          AccountRepository
	userRepo             UserRepository
	userSubRepo          UserSubscriptionRepository
	billingCacheService  *BillingCacheService
	deferredService      *DeferredService
	balanceNotifyService *BalanceNotifyService
}

// postUsageBillingParams 统一扣费所需的参数
//
// 4 个 Token 字段分开传，便于 service quota TPM/TPD 限流器按各自 token_components
// 配置自由选择计入项；下游 Record 调用直接用这 4 个值组装 ServiceQuotaRecordRequest。
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
	ServiceQuotaRequest   ServiceQuotaCheckRequest
	InputTokens           int64
	OutputTokens          int64
	CacheCreationTokens   int64
	CacheReadTokens       int64
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

// buildServiceQuotaRecordRequest 从 postUsageBillingParams 装配 ServiceQuotaRecordRequest。
//
// 返回 ok=false 表示"本次请求不需要写服务限额计数"——目前唯一硬约束是 p.User == nil
// （未鉴权或异常路径），因为所有 limiter 计数 key 都需要 UserID 以做 per-user/shared 区分。
//
// 注意 ServiceQuotaRequest 已在 caller 路径就装填了 platform / channel / model 等字段，
// 这里只补上随响应才确定的 UserID/GroupID/Platform/AccountID 覆盖（同 APIKey 的 group 一致）。
func buildServiceQuotaRecordRequest(p *postUsageBillingParams) (ServiceQuotaRecordRequest, bool) {
	if p == nil || p.User == nil || p.Cost == nil {
		return ServiceQuotaRecordRequest{}, false
	}
	req := p.ServiceQuotaRequest
	req.UserID = p.User.ID
	if p.APIKey != nil && p.APIKey.GroupID != nil {
		req.GroupID = *p.APIKey.GroupID
	}
	if p.APIKey != nil && p.APIKey.Group != nil {
		req.Platform = p.APIKey.Group.Platform
	}
	if p.Account != nil {
		req.AccountID = p.Account.ID
	}
	return ServiceQuotaRecordRequest{
		ServiceQuotaCheckRequest: req,
		InputTokens:              p.InputTokens,
		OutputTokens:             p.OutputTokens,
		CacheCreationTokens:      p.CacheCreationTokens,
		CacheReadTokens:          p.CacheReadTokens,
		Cost:                     p.Cost.ActualCost,
	}, true
}

func detachedBillingContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, postUsageBillingTimeout)
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
			if IsUsageLogCreateDropped(err) {
				return
			}
			if _, syncErr := repo.Create(usageCtx, usageLog); syncErr != nil {
				logger.LegacyPrintf(logKey, "Create usage log sync fallback failed: %v", syncErr)
			}
		}
		return
	}

	if _, err := repo.Create(usageCtx, usageLog); err != nil {
		logger.LegacyPrintf(logKey, "Create usage log failed: %v", err)
	}
}

func resolveUsageBillingRequestID(ctx context.Context, upstreamRequestID string) string {
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

	finalizePostUsageBilling(p, deps, result)
	return true, nil
}

// postUsageBilling is the legacy fallback billing path used when the unified
// billing repo is unavailable (nil). Production uses applyUsageBilling -> repo.Apply
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

	// NOTE: finalizePostUsageBilling is NOT called here to avoid double-queuing
	// cache updates. The legacy path does DB writes directly; the finalize path
	// does cache queue + notifications. Notifications are dispatched separately
	// by the caller after recording the usage log.
}

func finalizePostUsageBilling(p *postUsageBillingParams, deps *billingDeps, result *UsageBillingApplyResult) {
	if p == nil || p.Cost == nil || deps == nil {
		return
	}

	if p.IsSubscriptionBill {
		if p.Cost.ActualCost > 0 && p.User != nil && p.APIKey != nil && p.APIKey.GroupID != nil {
			deps.billingCacheService.QueueUpdateSubscriptionUsage(p.User.ID, *p.APIKey.GroupID, p.Cost.ActualCost)
		}
	} else if p.Cost.ActualCost > 0 && p.User != nil {
		deps.billingCacheService.QueueDeductBalance(p.User.ID, p.Cost.ActualCost)
	}

	if p.Cost.ActualCost > 0 && p.APIKey != nil && p.APIKey.HasRateLimits() {
		deps.billingCacheService.QueueUpdateAPIKeyRateLimitUsage(p.APIKey.ID, p.Cost.ActualCost)
	}
	if record, ok := buildServiceQuotaRecordRequest(p); ok && deps.billingCacheService != nil {
		quotaCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		deps.billingCacheService.RecordServiceQuotaUsage(quotaCtx, record)
	}

	deps.deferredService.ScheduleLastUsedUpdate(p.Account.ID)

	// Notification checks run async -- all parameters are already captured,
	// no dependency on the request context or upstream connection.
	go notifyBalanceLow(p, deps, result)
	go notifyAccountQuota(p, deps, result)
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
