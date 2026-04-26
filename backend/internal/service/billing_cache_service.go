package service

// BillingCacheService 计费缓存服务：余额 / 订阅 / 限速 / 配额 的统一缓存入口。
// 职责拆分参见兄弟文件：
//   - billing_cache_service_worker.go      任务类型、异步写入工作池
//   - billing_cache_service_balance.go     余额缓存读写
//   - billing_cache_service_subscription.go 订阅缓存读写
//   - billing_cache_service_rate_limit.go  API Key 限速窗口
//   - billing_cache_service_quota.go       用户每日配额 (feature issue #1750)
//   - billing_cache_service_circuit_breaker.go 计费熔断器

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"golang.org/x/sync/singleflight"
)

// 错误定义
// 注：ErrInsufficientBalance在redeem_service.go中定义
// 注：ErrDailyLimitExceeded/ErrWeeklyLimitExceeded/ErrMonthlyLimitExceeded在subscription_service.go中定义
var (
	ErrSubscriptionInvalid       = infraerrors.Forbidden("SUBSCRIPTION_INVALID", "subscription is invalid or expired")
	ErrBillingServiceUnavailable = infraerrors.ServiceUnavailable("BILLING_SERVICE_ERROR", "Billing service temporarily unavailable. Please retry later.")
	// RPM 超限错误。gateway_handler 负责映射为 HTTP 429。
	ErrGroupRPMExceeded = infraerrors.TooManyRequests("GROUP_RPM_EXCEEDED", "group requests-per-minute limit exceeded")
	ErrUserRPMExceeded  = infraerrors.TooManyRequests("USER_RPM_EXCEEDED", "user requests-per-minute limit exceeded")
)

// billingCacheLogComponent 是 billing_cache_service 所有子文件（余额/订阅/限速/worker）
// 异步写入告警日志共用的 component 标签，与 quotaLogComponent (billing_cache_service_quota.go)
// 平行。集中一处避免各方法散落相同字面量；消息本身（如 "set balance cache failed"）
// 已足够区分具体场景。
const billingCacheLogComponent = "service.billing_cache"

// BillingCacheService 计费缓存服务
// 负责余额和订阅数据的缓存管理，提供高性能的计费资格检查
type BillingCacheService struct {
	cache                 BillingCache
	userRepo              UserRepository
	subRepo               UserSubscriptionRepository
	apiKeyRateLimitLoader apiKeyRateLimitLoader
	userRPMCache          UserRPMCache
	userGroupRateRepo     UserGroupRateRepository
	cfg                   *config.Config
	circuitBreaker        *billingCircuitBreaker

	serviceQuota ServiceQuotaService

	cacheWriteChan     chan cacheWriteTask
	cacheWriteWg       sync.WaitGroup
	cacheWriteStopOnce sync.Once
	cacheWriteMu       sync.RWMutex
	stopped            atomic.Bool
	balanceLoadSF      singleflight.Group
	// 丢弃日志节流计数器（减少高负载下日志噪音）
	cacheWriteDropFullCount     uint64
	cacheWriteDropFullLastLog   int64
	cacheWriteDropClosedCount   uint64
	cacheWriteDropClosedLastLog int64
}

// NewBillingCacheService 创建计费缓存服务
func NewBillingCacheService(
	cache BillingCache,
	userRepo UserRepository,
	subRepo UserSubscriptionRepository,
	apiKeyRepo APIKeyRepository,
	userRPMCache UserRPMCache,
	userGroupRateRepo UserGroupRateRepository,
	cfg *config.Config,
	serviceQuotas ...ServiceQuotaService,
) *BillingCacheService {
	var serviceQuota ServiceQuotaService
	if len(serviceQuotas) > 0 {
		serviceQuota = serviceQuotas[0]
	}
	svc := &BillingCacheService{
		cache:                 cache,
		userRepo:              userRepo,
		subRepo:               subRepo,
		apiKeyRateLimitLoader: apiKeyRepo,
		userRPMCache:          userRPMCache,
		userGroupRateRepo:     userGroupRateRepo,
		cfg:                   cfg,
		serviceQuota:          serviceQuota,
	}
	svc.circuitBreaker = newBillingCircuitBreaker(cfg.Billing.CircuitBreaker)
	svc.startCacheWriteWorkers()
	return svc
}

// CheckBillingEligibility 检查用户是否有资格发起请求。
//
// 余额模式：检查缓存余额 > 0
// 订阅模式：检查缓存用量未超过限额（Group限额从参数传入）
//
// 返回的 *ServiceQuotaLease 持有服务限额（concurrency 类）已抢占的槽位，
// 调用方必须在请求结束（包括 error / panic 路径）时调用 lease.Release() 释放，
// 否则槽位将累积直到 ZSET 5min 兜底回收，期间 limit 会被陈旧 member 撑爆。
//
// lease 可能为 nil（无规则匹配 / 服务限额未启用 / 仅命中非 concurrency 限流器），
// 返回的 lease.Release 已用 sync.Once 保护，重复调用安全。
//
// 推荐用法：
//
//	lease, err := svc.CheckBillingEligibility(ctx, user, apiKey, group, sub)
//	defer ReleaseQuotaLease(lease)
//	if err != nil { return err }
func (s *BillingCacheService) CheckBillingEligibility(ctx context.Context, user *User, apiKey *APIKey, group *Group, subscription *UserSubscription) (*ServiceQuotaLease, error) {
	return s.CheckBillingEligibilityForRequest(ctx, user, apiKey, group, subscription, ServiceQuotaCheckRequest{})
}

// CheckBillingEligibilityForRequest 同 CheckBillingEligibility，但额外接受 ServiceQuotaCheckRequest
// 以便携带 model、channel 等服务限额匹配维度。返回 lease 的生命周期约定与
// CheckBillingEligibility 一致，调用方必须 defer ReleaseQuotaLease(lease)。
func (s *BillingCacheService) CheckBillingEligibilityForRequest(ctx context.Context, user *User, apiKey *APIKey, group *Group, subscription *UserSubscription, quotaReq ServiceQuotaCheckRequest) (lease *ServiceQuotaLease, err error) {
	// 简易模式：跳过所有计费检查
	if s.cfg.RunMode == config.RunModeSimple {
		return nil, nil
	}
	if s.circuitBreaker != nil && !s.circuitBreaker.Allow() {
		return nil, ErrBillingServiceUnavailable
	}
	if s.serviceQuota != nil && user != nil {
		quotaReq.UserID = user.ID
		if group != nil {
			quotaReq.GroupID = group.ID
			quotaReq.Platform = group.Platform
		}
		acquired, perr := s.serviceQuota.PreCheck(ctx, quotaReq)
		if perr != nil {
			return nil, perr
		}
		// 把原始 Release 包成 once-safe，防止上层重复 defer 或 panic 路径双释放。
		lease = wrapServiceQuotaLeaseOnce(acquired)
	}

	// 任何后续检查失败都要释放已抢占的 concurrency 槽位，
	// 避免 PreCheck 通过、后续 RPM/余额校验失败时 lease 永远漏。
	defer func() {
		if err != nil && lease != nil {
			lease.Release()
			lease = nil
		}
	}()

	// 判断计费模式
	isSubscriptionMode := group != nil && group.IsSubscriptionType() && subscription != nil

	if isSubscriptionMode {
		if err = s.checkSubscriptionEligibility(ctx, user.ID, group, subscription); err != nil {
			return nil, err
		}
	} else {
		if err = s.checkBalanceEligibility(ctx, user.ID); err != nil {
			return nil, err
		}
	}

	// Check API Key rate limits (applies to both billing modes)
	if apiKey != nil && apiKey.HasRateLimits() {
		if err = s.checkAPIKeyRateLimits(ctx, apiKey); err != nil {
			return nil, err
		}
	}

	// RPM 限流：级联回落（Override → Group → User），放在最后以避免为注定失败的请求增加计数。
	if err = s.checkRPM(ctx, user, group); err != nil {
		return nil, err
	}

	return lease, nil
}

// wrapServiceQuotaLeaseOnce 把 PreCheck 返回的 lease.Release 包成 sync.Once，
// 调用方可放心在 defer 路径多次执行。lease 为 nil 或 Release 为 nil 时透传。
func wrapServiceQuotaLeaseOnce(lease *ServiceQuotaLease) *ServiceQuotaLease {
	if lease == nil || lease.Release == nil {
		return lease
	}
	original := lease.Release
	var once sync.Once
	lease.Release = func() {
		once.Do(original)
	}
	return lease
}

// ReleaseQuotaLease 安全释放 *ServiceQuotaLease，nil 友好。
// 用于 caller 端 defer ReleaseQuotaLease(lease) 的便捷写法。
func ReleaseQuotaLease(lease *ServiceQuotaLease) {
	if lease == nil || lease.Release == nil {
		return
	}
	lease.Release()
}

// checkRPM 执行并行 RPM 限流，所有适用的限制同时生效，任一超限即拒绝：
//
//  1. (用户, 分组) rpm_override       — 最细粒度：管理员为特定用户在特定分组设定的专属限额。
//     override=0 表示该用户在该分组免检（绿灯），但 user 级全局上限仍然生效。
//  2. group.rpm_limit                 — 分组级：该分组的统一 RPM 容量（仅当无 override 时生效）。
//  3. user.rpm_limit                  — 用户级全局硬上限：无论 override/group 如何配置，始终生效。
//
// 与旧版"级联互斥"设计不同，新版确保 user.rpm_limit 作为全局天花板不会被 group 或 override 覆盖。
// Redis 故障一律 fail-open（打 warning，不阻塞业务）。
func (s *BillingCacheService) checkRPM(ctx context.Context, user *User, group *Group) error {
	if s == nil || s.userRPMCache == nil || user == nil {
		return nil
	}

	// ── 第一层：分组级检查（override 或 group.rpm_limit） ──
	if group != nil {
		// 解析 override：优先从 auth cache snapshot，nil 时回退 DB。
		var override *int
		if user.UserGroupRPMOverride != nil {
			override = user.UserGroupRPMOverride
		} else if s.userGroupRateRepo != nil {
			dbOverride, err := s.userGroupRateRepo.GetRPMOverrideByUserAndGroup(ctx, user.ID, group.ID)
			if err != nil {
				logger.LegacyPrintf(
					"service.billing_cache",
					"Warning: rpm override lookup failed for user=%d group=%d: %v",
					user.ID, group.ID, err,
				)
			} else {
				override = dbOverride
			}
		}

		if override != nil {
			// override=0 → 该用户在该分组免检（但 user 级仍会在下面检查）。
			if *override > 0 {
				count, incErr := s.userRPMCache.IncrementUserGroupRPM(ctx, user.ID, group.ID)
				if incErr != nil {
					logger.LegacyPrintf(
						"service.billing_cache",
						"Warning: rpm increment (override) failed for user=%d group=%d: %v",
						user.ID, group.ID, incErr,
					)
					// fail-open
				} else if count > *override {
					return ErrGroupRPMExceeded
				}
			}
			// override 命中后跳过 group.rpm_limit（override 替代 group），但不 return——继续检查 user 级。
		} else if group.RPMLimit > 0 {
			// 无 override，检查 group.rpm_limit。
			count, err := s.userRPMCache.IncrementUserGroupRPM(ctx, user.ID, group.ID)
			if err != nil {
				logger.LegacyPrintf(
					"service.billing_cache",
					"Warning: rpm increment (group) failed for user=%d group=%d: %v",
					user.ID, group.ID, err,
				)
				// fail-open
			} else if count > group.RPMLimit {
				return ErrGroupRPMExceeded
			}
		}
	}

	// ── 第二层：用户级全局硬上限（始终生效） ──
	if user.RPMLimit > 0 {
		count, err := s.userRPMCache.IncrementUserRPM(ctx, user.ID)
		if err != nil {
			logger.LegacyPrintf(
				"service.billing_cache",
				"Warning: rpm increment (user) failed for user=%d: %v",
				user.ID, err,
			)
			return nil // fail-open
		}
		if count > user.RPMLimit {
			return ErrUserRPMExceeded
		}
	}

	return nil
}

// checkBalanceEligibility 检查余额模式资格
func (s *BillingCacheService) checkBalanceEligibility(ctx context.Context, userID int64) error {
	balance, err := s.GetUserBalance(ctx, userID)
	if err != nil {
		if s.circuitBreaker != nil {
			s.circuitBreaker.OnFailure(err)
		}
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing balance check failed for user %d: %v", userID, err)
		return ErrBillingServiceUnavailable.WithCause(err)
	}
	if s.circuitBreaker != nil {
		s.circuitBreaker.OnSuccess()
	}

	if balance <= 0 {
		return ErrInsufficientBalance
	}

	return nil
}

// checkSubscriptionEligibility 检查订阅模式资格
func (s *BillingCacheService) checkSubscriptionEligibility(ctx context.Context, userID int64, group *Group, subscription *UserSubscription) error {
	// 获取订阅缓存数据
	subData, err := s.GetSubscriptionStatus(ctx, userID, group.ID)
	if err != nil {
		if s.circuitBreaker != nil {
			s.circuitBreaker.OnFailure(err)
		}
		logger.LegacyPrintf("service.billing_cache", "ALERT: billing subscription check failed for user %d group %d: %v", userID, group.ID, err)
		return ErrBillingServiceUnavailable.WithCause(err)
	}
	if s.circuitBreaker != nil {
		s.circuitBreaker.OnSuccess()
	}

	// 检查订阅状态
	if subData.Status != SubscriptionStatusActive {
		return ErrSubscriptionInvalid
	}

	// 检查是否过期
	if time.Now().After(subData.ExpiresAt) {
		return ErrSubscriptionInvalid
	}

	// 检查限额（使用传入的Group限额配置）
	if group.HasDailyLimit() && subData.DailyUsage >= *group.DailyLimitUSD {
		return ErrDailyLimitExceeded
	}

	if group.HasWeeklyLimit() && subData.WeeklyUsage >= *group.WeeklyLimitUSD {
		return ErrWeeklyLimitExceeded
	}

	if group.HasMonthlyLimit() && subData.MonthlyUsage >= *group.MonthlyLimitUSD {
		return ErrMonthlyLimitExceeded
	}

	return nil
}
