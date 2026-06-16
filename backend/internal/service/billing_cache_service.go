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
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
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

	ErrUserPlatformDailyQuotaExhausted   = infraerrors.TooManyRequests("USER_PLATFORM_DAILY_QUOTA_EXHAUSTED", "Daily usage quota exhausted for this platform.")
	ErrUserPlatformWeeklyQuotaExhausted  = infraerrors.TooManyRequests("USER_PLATFORM_WEEKLY_QUOTA_EXHAUSTED", "Weekly usage quota exhausted for this platform.")
	ErrUserPlatformMonthlyQuotaExhausted = infraerrors.TooManyRequests("USER_PLATFORM_MONTHLY_QUOTA_EXHAUSTED", "Monthly usage quota exhausted for this platform.")
)

// errBillingCacheUnavailable 内部哨兵：用于 quota 校验路径在 cache==nil 时
// 与"Redis 故障"走同一条 fail-open + DB 一次性检查的分支。
var errBillingCacheUnavailable = fmt.Errorf("billing cache unavailable")

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
	userPlatformQuotaRepo UserPlatformQuotaRepository

	serviceQuota ServiceQuotaService

	cacheWriteChan     chan cacheWriteTask
	cacheWriteWg       sync.WaitGroup
	cacheWriteStopOnce sync.Once
	cacheWriteMu       sync.RWMutex
	stopped            atomic.Bool
	balanceLoadSF      singleflight.Group
	quotaLoadSF        singleflight.Group
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
	userPlatformQuotaRepo UserPlatformQuotaRepository,
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
		userPlatformQuotaRepo: userPlatformQuotaRepo,
		serviceQuota:          serviceQuota,
	}
	svc.circuitBreaker = newBillingCircuitBreaker(cfg.Billing.CircuitBreaker)
	svc.startCacheWriteWorkers()
	return svc
}

// PrepareBillingCheck 是两阶段 PreCheck 的第一阶段入口。
//
// 在 caller 路由前调用：
//   - 执行余额 / 订阅 / API Key 限速 / RPM 等不依赖 channel/account 的检查；
//   - 通过 ServiceQuotaService.PreCheckSelect 选出候选规则但不抢 concurrency；
//   - 返回的 *BillingTicket 必须由 caller 持有并在所有返回路径上 defer ticket.Close()。
//
// Redis 写失败用 ALERT 级 log；DB 持久化由 caller 单独 goroutine 兜底（gateway_service.go）。
func (s *BillingCacheService) IncrementUserPlatformQuotaUsage(userID int64, platform string, cost float64) {
	if s.cache == nil {
		return
	}
	if platform == "" || cost <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
	defer cancel()
	ttl := time.Duration(s.cfg.Billing.UserPlatformQuotaCacheTTLSeconds) * time.Second
	markDirty := s.cfg.Database.UserPlatformQuotaFlusherEnabled
	if err := s.cache.IncrUserPlatformQuotaUsageCache(ctx, userID, platform, cost, ttl, markDirty); err != nil {
		logger.LegacyPrintf("service.billing_cache",
			"ALERT: incr user platform quota cache failed user=%d platform=%s cost=%f: %v",
			userID, platform, cost, err)
	}
}

// 后续 caller 选定 channel + account（路由完成）后，必须调用 ticket.Consume(ctx, channelID, accountID)
// 才会真正按 channel/account scope 抢 concurrency / 增 RPM。
//
// 失败语义：返回 error 时 ticket==nil，caller 不需要也不应该 Close。
//
// 推荐用法：
//
//	ticket, err := billingCache.PrepareBillingCheck(ctx, user, apiKey, group, sub)
//	if err != nil { return err }
//	defer ticket.Close()
//	// ... 选定 account / channel ...
//	if err := ticket.Consume(ctx, channelID, accountID); err != nil { return err }
func (s *BillingCacheService) PrepareBillingCheck(ctx context.Context, user *User, apiKey *APIKey, group *Group, subscription *UserSubscription) (*BillingTicket, error) {
	return s.PrepareBillingCheckForRequest(ctx, user, apiKey, group, subscription, ServiceQuotaCheckRequest{})
}

// PrepareBillingCheckForRequest 同 PrepareBillingCheck，但 caller 可以预先填好 model / 已知字段。
// quotaReq 中 ChannelID / AccountID 可留 0（caller 通常路由前还不知道），后续由 ticket.Consume 补全。
//
// 内部统一走两阶段路径：Prepare 阶段调 PreCheckSelect 选候选规则（不抢 concurrency / 不增 RPM），
// 真正按 path 维度抢占由 caller 后续调 ticket.Consume 完成。
func (s *BillingCacheService) PrepareBillingCheckForRequest(ctx context.Context, user *User, apiKey *APIKey, group *Group, subscription *UserSubscription, quotaReq ServiceQuotaCheckRequest) (ticket *BillingTicket, err error) {
	// 简易模式：跳过所有计费检查
	if s.cfg.RunMode == config.RunModeSimple {
		return nil, nil
	}
	if s.circuitBreaker != nil && !s.circuitBreaker.Allow() {
		return nil, ErrBillingServiceUnavailable
	}

	if user != nil {
		quotaReq.UserID = user.ID
		if group != nil {
			quotaReq.GroupID = group.ID
			quotaReq.Platform = group.Platform
		}
	}

	t := &BillingTicket{
		svc:      s,
		quotaReq: quotaReq,
	}

	if s.serviceQuota != nil && user != nil {
		plan, perr := s.serviceQuota.PreCheckSelect(ctx, quotaReq)
		if perr != nil {
			return nil, perr
		}
		t.plan = plan
	}

	// 任何后续检查失败都要释放已抢占的 concurrency 槽位，
	// 避免 PreCheck 通过、后续 RPM/余额校验失败时 lease 永远漏。
	defer func() {
		if err != nil && t != nil {
			t.Close()
		}
	}()

	// 判断计费模式
	isSubscriptionMode := group != nil && group.IsSubscriptionType() && subscription != nil

	if isSubscriptionMode {
		if err = s.checkSubscriptionEligibility(ctx, user.ID, group, subscription); err != nil {
			return nil, err
		}
	} else if user != nil {
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

	return t, nil
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

// RecordServiceQuotaUsage 收敛 ServiceQuota.Record 调用，让 gateway / 任何其他 caller
// 不必再穿透 BillingCacheService.serviceQuota 这个内部字段。
//
// 入口 nil-guard：BillingCacheService 自身可能为 nil（早期初始化路径）；serviceQuota
// 也可能未注入（旧测试桩或部署不开启）。两种情况都视作"无规则"静默 no-op，与 Record
// 内部一直以来的语义一致。
//
// Record 本身不返回 error（fail-open 设计：限流器不可用时落日志 + metrics，但请求继续），
// 因此这里也不返回 error。
func (s *BillingCacheService) RecordServiceQuotaUsage(ctx context.Context, req ServiceQuotaRecordRequest) {
	if s == nil || s.serviceQuota == nil {
		return
	}
	s.serviceQuota.Record(ctx, req)
}

// FilterAccountsByServiceQuotaSchedulability 是 GatewayService 调度阶段调用的 facade：
// 把 ticket 的 PreCheckPlan + 全局 ServiceQuotaService 引用收敛进 BillingCacheService，
// 让网关代码不直接看到 ServiceQuotaService 接口（封装边界）。
//
// 行为分支：
//   - BillingCacheService 为 nil / serviceQuota 未注入 / ticket nil / plan 空 → 返回原 accounts
//   - 其余走 service.FilterAccountsByServiceQuotaSchedulability，命中 account-/channel-scope
//     的候选被剔除，调用方据空切片返回 ErrNoAvailableAccounts。
//
// fail-open：底层 SnapshotForAccounts 内部已实现 Redis 错误吞错，本 facade 同样不抛 error。
func (s *BillingCacheService) FilterAccountsByServiceQuotaSchedulability(
	ctx context.Context,
	ticket *BillingTicket,
	base ServiceQuotaCheckRequest,
	accounts []Account,
) []Account {
	if s == nil || s.serviceQuota == nil || ticket == nil {
		return accounts
	}
	plan := ticket.PreCheckPlan()
	if plan == nil {
		return accounts
	}
	return FilterAccountsByServiceQuotaSchedulability(ctx, s.serviceQuota, plan, base, accounts)
}

func (s *BillingCacheService) checkUserPlatformQuotaEligibility(
	ctx context.Context,
	userID int64,
	platform string,
) error {
	if platform == "" || s.userPlatformQuotaRepo == nil {
		return nil
	}

	// cache 未配置（如简化部署 / 单测路径）→ 直接走 DB 查询，避免 nil panic。
	// 其他 check* 方法（balance/subscription/rate-limit）也有类似守卫。
	var (
		entry    *UserPlatformQuotaCacheEntry
		ok       bool
		cacheErr error
	)
	if s.cache != nil {
		entry, ok, cacheErr = s.cache.GetUserPlatformQuotaCache(ctx, userID, platform)
	} else {
		// 标记为"cache 故障"分支：跳过 HIT 路径、不回填、走 DB 一次性检查
		cacheErr = errBillingCacheUnavailable
	}

	// --- cache HIT with current schema → 直接用 entry，不查 DB ---
	if cacheErr == nil && ok && entry != nil && entry.SchemaVersion == UserPlatformQuotaCacheSchemaV1 {
		now := time.Now()
		dailyUsage := entry.DailyUsageUSD
		weeklyUsage := entry.WeeklyUsageUSD
		monthlyUsage := entry.MonthlyUsageUSD
		windowExpired := false
		newDailyStart := entry.DailyWindowStart
		newWeeklyStart := entry.WeeklyWindowStart
		newMonthlyStart := entry.MonthlyWindowStart
		if quotaWindowExpired(entry.DailyWindowStart, timezone.StartOfDay(now)) {
			dailyUsage = 0
			windowExpired = true
			dayStart := timezone.StartOfDay(now)
			newDailyStart = &dayStart
		}
		if quotaWindowExpired(entry.WeeklyWindowStart, timezone.StartOfWeek(now)) {
			weeklyUsage = 0
			windowExpired = true
			weekStart := timezone.StartOfWeek(now)
			newWeeklyStart = &weekStart
		}
		if monthlyQuotaWindowExpired(entry.MonthlyWindowStart, now) {
			monthlyUsage = 0
			windowExpired = true
			monthStart := now
			newMonthlyStart = &monthStart
		}
		isSentinel := entry.DailyLimitUSD == nil && entry.WeeklyLimitUSD == nil && entry.MonthlyLimitUSD == nil
		if windowExpired && s.cache != nil && !isSentinel {
			refreshed := &UserPlatformQuotaCacheEntry{
				DailyUsageUSD:      dailyUsage,
				WeeklyUsageUSD:     weeklyUsage,
				MonthlyUsageUSD:    monthlyUsage,
				SchemaVersion:      UserPlatformQuotaCacheSchemaV1,
				DailyLimitUSD:      entry.DailyLimitUSD,
				WeeklyLimitUSD:     entry.WeeklyLimitUSD,
				MonthlyLimitUSD:    entry.MonthlyLimitUSD,
				DailyWindowStart:   newDailyStart,
				WeeklyWindowStart:  newWeeklyStart,
				MonthlyWindowStart: newMonthlyStart,
			}
			ttl := time.Duration(s.cfg.Billing.UserPlatformQuotaCacheTTLSeconds) * time.Second
			setCtx, setCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			if setErr := s.cache.SetUserPlatformQuotaCache(setCtx, userID, platform, refreshed, ttl); setErr != nil {
				logger.LegacyPrintf("service.billing_cache",
					"Warning: refresh expired user platform quota cache failed user=%d platform=%s: %v",
					userID, platform, setErr)
			}
			setCancel()
		}
		if entry.DailyLimitUSD != nil && dailyUsage >= *entry.DailyLimitUSD {
			return withWindowResetsMetadata(ErrUserPlatformDailyQuotaExhausted, nextDailyReset(now))
		}
		if entry.WeeklyLimitUSD != nil && weeklyUsage >= *entry.WeeklyLimitUSD {
			return withWindowResetsMetadata(ErrUserPlatformWeeklyQuotaExhausted, nextWeeklyReset(now))
		}
		if entry.MonthlyLimitUSD != nil && monthlyUsage >= *entry.MonthlyLimitUSD {
			return withWindowResetsMetadata(ErrUserPlatformMonthlyQuotaExhausted, nextMonthlyResetFrom(entry.MonthlyWindowStart, now))
		}
		return nil
	}

	// --- cache MISS、旧版 entry 或 Redis 故障 → 查 DB（singleflight 合并并发回源）---
	sfKey := strconv.FormatInt(userID, 10) + ":" + platform
	ch := s.quotaLoadSF.DoChan(sfKey, func() (any, error) {
		bgCtx, bgCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer bgCancel()
		return s.userPlatformQuotaRepo.GetByUserPlatform(bgCtx, userID, platform)
	})
	var (
		v     any
		dbErr error
	)
	select {
	case res := <-ch:
		v, dbErr = res.Val, res.Err
	case <-ctx.Done():
		logger.LegacyPrintf("service.billing_cache", "Warning: user platform quota check ctx cancelled user=%d platform=%s: %v (fail-open)", userID, platform, ctx.Err())
		return nil
	}
	if dbErr != nil {
		logger.LegacyPrintf("service.billing_cache", "Warning: load user platform quota failed user=%d platform=%s: %v (fail-open)", userID, platform, dbErr)
		return nil
	}
	rec, _ := v.(*UserPlatformQuotaRecord)
	if rec == nil {
		if s.cache != nil && cacheErr == nil {
			now := time.Now()
			startOfDay := timezone.StartOfDay(now)
			startOfWeek := timezone.StartOfWeek(now)
			sentinel := &UserPlatformQuotaCacheEntry{
				SchemaVersion:      UserPlatformQuotaCacheSchemaV1,
				DailyWindowStart:   &startOfDay,
				WeeklyWindowStart:  &startOfWeek,
				MonthlyWindowStart: &now,
			}
			sentinelTTL := time.Duration(s.cfg.Billing.UserPlatformQuotaSentinelTTLSeconds) * time.Second
			if sentinelTTL <= 0 {
				sentinelTTL = time.Hour
			}
			setCtx, setCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			if setErr := s.cache.SetUserPlatformQuotaCache(setCtx, userID, platform, sentinel, sentinelTTL); setErr != nil {
				userPlatformQuotaSentinelSetCacheErrorTotal.Add(1)
				logger.LegacyPrintf("service.billing_cache", "Warning: set sentinel quota cache failed user=%d platform=%s: %v", userID, platform, setErr)
			}
			setCancel()
		}
		return nil
	}

	now := time.Now()
	dailyUsage := rec.DailyUsageUSD
	weeklyUsage := rec.WeeklyUsageUSD
	monthlyUsage := rec.MonthlyUsageUSD
	if quotaWindowExpired(rec.DailyWindowStart, timezone.StartOfDay(now)) {
		dailyUsage = 0
	}
	if quotaWindowExpired(rec.WeeklyWindowStart, timezone.StartOfWeek(now)) {
		weeklyUsage = 0
	}
	if monthlyQuotaWindowExpired(rec.MonthlyWindowStart, now) {
		monthlyUsage = 0
	}

	if cacheErr != nil {
		if rec.DailyLimitUSD != nil && dailyUsage >= *rec.DailyLimitUSD {
			return withWindowResetsMetadata(ErrUserPlatformDailyQuotaExhausted, nextDailyReset(now))
		}
		if rec.WeeklyLimitUSD != nil && weeklyUsage >= *rec.WeeklyLimitUSD {
			return withWindowResetsMetadata(ErrUserPlatformWeeklyQuotaExhausted, nextWeeklyReset(now))
		}
		if rec.MonthlyLimitUSD != nil && monthlyUsage >= *rec.MonthlyLimitUSD {
			return withWindowResetsMetadata(ErrUserPlatformMonthlyQuotaExhausted, nextMonthlyResetFrom(rec.MonthlyWindowStart, now))
		}
		return nil
	}

	newEntry := &UserPlatformQuotaCacheEntry{
		DailyUsageUSD:      dailyUsage,
		WeeklyUsageUSD:     weeklyUsage,
		MonthlyUsageUSD:    monthlyUsage,
		SchemaVersion:      UserPlatformQuotaCacheSchemaV1,
		DailyLimitUSD:      rec.DailyLimitUSD,
		WeeklyLimitUSD:     rec.WeeklyLimitUSD,
		MonthlyLimitUSD:    rec.MonthlyLimitUSD,
		DailyWindowStart:   rec.DailyWindowStart,
		WeeklyWindowStart:  rec.WeeklyWindowStart,
		MonthlyWindowStart: rec.MonthlyWindowStart,
	}
	if s.cache != nil {
		ttl := time.Duration(s.cfg.Billing.UserPlatformQuotaCacheTTLSeconds) * time.Second
		setCtx, setCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		if setErr := s.cache.SetUserPlatformQuotaCache(setCtx, userID, platform, newEntry, ttl); setErr != nil {
			logger.LegacyPrintf("service.billing_cache", "Warning: set user platform quota cache failed user=%d platform=%s: %v", userID, platform, setErr)
		}
		setCancel()
	}

	if rec.DailyLimitUSD != nil && dailyUsage >= *rec.DailyLimitUSD {
		return withWindowResetsMetadata(ErrUserPlatformDailyQuotaExhausted, nextDailyReset(now))
	}
	if rec.WeeklyLimitUSD != nil && weeklyUsage >= *rec.WeeklyLimitUSD {
		return withWindowResetsMetadata(ErrUserPlatformWeeklyQuotaExhausted, nextWeeklyReset(now))
	}
	if rec.MonthlyLimitUSD != nil && monthlyUsage >= *rec.MonthlyLimitUSD {
		return withWindowResetsMetadata(ErrUserPlatformMonthlyQuotaExhausted, nextMonthlyResetFrom(rec.MonthlyWindowStart, now))
	}
	return nil
}

// withWindowResetsMetadata 给 quota error 附加 window_resets_at metadata（RFC3339）。
func withWindowResetsMetadata(err error, resetAt time.Time) error {
	appErr, ok := err.(*infraerrors.ApplicationError)
	if !ok || appErr == nil {
		return err
	}
	return appErr.WithMetadata(map[string]string{
		"window_resets_at": resetAt.Format(time.RFC3339),
	})
}

func nextDailyReset(now time.Time) time.Time {
	return timezone.StartOfDay(now).AddDate(0, 0, 1)
}

func nextWeeklyReset(now time.Time) time.Time {
	return timezone.StartOfWeek(now).AddDate(0, 0, 7)
}

func nextMonthlyResetFrom(start *time.Time, now time.Time) time.Time {
	if start == nil || now.Sub(*start) >= 30*24*time.Hour {
		return now.Add(30 * 24 * time.Hour)
	}
	return start.Add(30 * 24 * time.Hour)
}

func quotaWindowExpired(start *time.Time, currWindowStart time.Time) bool {
	if start == nil {
		return true
	}
	return start.Before(currWindowStart)
}

func monthlyQuotaWindowExpired(start *time.Time, now time.Time) bool {
	if start == nil {
		return true
	}
	return now.Sub(*start) >= 30*24*time.Hour
}

// HasUserPlatformQuotaLimit 判断该 user×platform 是否设了任一非 nil limit。
// 写入点守卫:无 limit 直接跳过 Redis 写 + 脏集标记,消除无谓写入。
// fail-safe:任何不确定(simple 模式除外)都返回 true 维持写入。
// DeleteUserPlatformQuotaCache invalidates the cached quota entry for a user+platform.
func (s *BillingCacheService) DeleteUserPlatformQuotaCache(ctx context.Context, userID int64, platform string) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.DeleteUserPlatformQuotaCache(ctx, userID, platform)
}

func (s *BillingCacheService) HasUserPlatformQuotaLimit(ctx context.Context, userID int64, platform string) bool {
	if s.cfg.RunMode == config.RunModeSimple {
		return false
	}
	if s.cache == nil {
		return true
	}
	entry, ok, err := s.cache.GetUserPlatformQuotaCache(ctx, userID, platform)
	if err != nil || !ok || entry == nil {
		return true
	}
	return entry.DailyLimitUSD != nil || entry.WeeklyLimitUSD != nil || entry.MonthlyLimitUSD != nil
}
