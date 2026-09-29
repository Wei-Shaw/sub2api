package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

// LiveBillingAPIKeySource 是 Live 结算按 ID 回查 API Key 并更新 Key 配额所需的最小接口，
// 由 *APIKeyService 实现。
type LiveBillingAPIKeySource interface {
	GetByID(ctx context.Context, id int64) (*APIKey, error)
	APIKeyQuotaUpdater
}

// SetLiveBillingAPIKeyService 注入 Live 结算使用的 API Key 来源。
// finalize 可能在另一实例上、以 Redis 中的 LiveCallRecord（只有 ID）触发，
// 因此计费所需的 API Key / 分组 / 用户须在结算时按 ID 回查。
func (s *OpenAIGatewayService) SetLiveBillingAPIKeyService(source LiveBillingAPIKeySource) {
	if s == nil {
		return
	}
	s.liveBillingAPIKeys = source
}

// livePricePerMinuteUSD 返回 gateway.live.price_per_minute_usd；未配置或非法时按 0（免费）处理。
func (s *OpenAIGatewayService) livePricePerMinuteUSD() float64 {
	if s == nil || s.cfg == nil {
		return 0
	}
	price := s.cfg.Gateway.Live.PricePerMinuteUSD
	// 配置校验已拒绝负数/NaN/Inf，这里是纵深防御。
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0
	}
	return price
}

// liveDurationCostUSD 按通话时长折算原始费用：单价 × 分钟数（毫秒折算的分钟小数，不向上取整）。
func liveDurationCostUSD(pricePerMinute float64, durationMs int) float64 {
	if pricePerMinute <= 0 || durationMs <= 0 {
		return 0
	}
	return pricePerMinute * float64(durationMs) / float64(time.Minute/time.Millisecond)
}

// liveBillingQuotaPlatform 对齐 handler 创建会话时的 QuotaPlatform：Composite 分组的
// Live 只路由到 OpenAI，结算时 ctx 中已没有路由结果，直接按 OpenAI 计量。
func liveBillingQuotaPlatform(apiKey *APIKey) string {
	platform := PlatformFromAPIKey(apiKey)
	if platform == PlatformComposite {
		return PlatformOpenAI
	}
	return platform
}

// recordLiveUsageWithBilling 按时长计费并写 Live 用量行，走与普通请求 RecordUsage 相同的
// applyUsageBilling 管道（余额/订阅/Key 配额与限速窗口/user×platform 配额），
// RequestID 固定为 CallHash，由 usage_billing_dedup 保证重复结算不重复扣费。
//
// 这是该会话唯一一次落库机会：无论回查或扣费是否失败，用量行都必须写出。失败时
// 与 RecordUsage 一致把 ActualCost 置 0（TotalCost 保留应收金额）并记错误日志，供对账。
func (s *OpenAIGatewayService) recordLiveUsageWithBilling(record *LiveCallRecord, usageLog *UsageLog, pricePerMinute float64, durationMs int) {
	ctx, cancel := detachedBillingContext(context.Background())
	defer cancel()

	usageLog.TotalCost = liveDurationCostUSD(pricePerMinute, durationMs)
	if err := s.applyLiveUsageBilling(ctx, record, usageLog); err != nil {
		usageLog.ActualCost = 0
		logger.L().With(
			zap.String("component", "service.openai_live"),
			zap.String("call_hash", record.CallHash),
			zap.Int64("user_id", record.UserID),
			zap.Int64("api_key_id", record.APIKeyID),
			zap.Int64("account_id", record.AccountID),
			zap.Int("duration_ms", durationMs),
			zap.Float64("total_cost", usageLog.TotalCost),
		).Error("openai_live.billing_failed", zap.Error(err))
	}
	writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.openai_live")
}

func (s *OpenAIGatewayService) applyLiveUsageBilling(ctx context.Context, record *LiveCallRecord, usageLog *UsageLog) error {
	if s.liveBillingAPIKeys == nil {
		return errors.New("live billing api key source unavailable")
	}
	apiKey, err := s.liveBillingAPIKeys.GetByID(ctx, record.APIKeyID)
	if err != nil {
		return fmt.Errorf("load api key: %w", err)
	}
	if apiKey == nil || apiKey.UserID != record.UserID {
		return errors.New("live billing api key owner mismatch")
	}
	user := apiKey.User
	if user == nil || user.ID != record.UserID {
		if s.userRepo == nil {
			return errors.New("live billing user unavailable")
		}
		if user, err = s.userRepo.GetByID(ctx, record.UserID); err != nil {
			return fmt.Errorf("load user: %w", err)
		}
		if user == nil {
			return errors.New("live billing user unavailable")
		}
	}
	account := &Account{ID: record.AccountID}
	if s.accountRepo != nil {
		// 账号只影响账号配额与账号倍率统计，回查失败不阻断向用户计费。
		loaded, loadErr := s.accountRepo.GetByID(ctx, record.AccountID)
		if loadErr != nil {
			logger.LegacyPrintf("service.openai_live", "live billing load account %d failed: %v", record.AccountID, loadErr)
		} else if loaded != nil {
			account = loaded
		}
	}
	var subscription *UserSubscription
	if record.SubscriptionID > 0 {
		// 订阅回查失败必须中止：否则订阅分组会被误按余额扣费。
		if s.userSubRepo == nil {
			return errors.New("live billing subscription repository unavailable")
		}
		if subscription, err = s.userSubRepo.GetByID(ctx, record.SubscriptionID); err != nil {
			return fmt.Errorf("load subscription: %w", err)
		}
	}

	// 倍率与普通请求 RecordUsage 同源：默认倍率 → 分组倍率 → 用户专属分组倍率。
	// 高峰因子只作用于 token 计费，按时长计费不叠加。
	multiplier := 1.0
	if s.cfg != nil {
		multiplier = s.cfg.Default.RateMultiplier
	}
	if apiKey.GroupID != nil && apiKey.Group != nil {
		multiplier = s.ResolveUserGroupRateMultiplier(ctx, user.ID, *apiKey.GroupID, apiKey.Group.RateMultiplier)
	}
	cost := &CostBreakdown{
		TotalCost:  usageLog.TotalCost,
		ActualCost: usageLog.TotalCost * multiplier,
	}

	isSubscriptionBilling := subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	usageLog.BillingType = BillingTypeBalance
	if isSubscriptionBilling {
		usageLog.BillingType = BillingTypeSubscription
	}
	accountRateMultiplier := account.BillingRateMultiplier()
	usageLog.RateMultiplier = multiplier
	usageLog.AccountRateMultiplier = &accountRateMultiplier
	usageLog.ActualCost = cost.ActualCost

	simpleModeKeyRateLimitOnly := simpleModeKeyRateLimitBillingEnabled(s.cfg, apiKey)
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple && !simpleModeKeyRateLimitOnly {
		// 与 RecordUsage 一致：简单模式只记录不扣费。
		return nil
	}

	_, err = applyUsageBilling(ctx, record.CallHash, usageLog, &postUsageBillingParams{
		Cost:                       cost,
		User:                       user,
		APIKey:                     apiKey,
		Account:                    account,
		Subscription:               subscription,
		IsSubscriptionBill:         isSubscriptionBilling && !simpleModeKeyRateLimitOnly,
		AccountRateMultiplier:      accountRateMultiplier,
		APIKeyService:              s.liveBillingAPIKeys,
		Platform:                   liveBillingQuotaPlatform(apiKey),
		SimpleModeKeyRateLimitOnly: simpleModeKeyRateLimitOnly,
	}, s.billingDeps(), s.usageBillingRepo)
	return err
}
