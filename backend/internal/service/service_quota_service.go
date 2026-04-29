package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/lib/pq"
	"golang.org/x/sync/singleflight"
)

type serviceQuotaService struct {
	repo     ServiceQuotaRuleRepository
	settings *SettingService
	limiter  ServiceQuotaLimiter
	cache    ServiceQuotaCache
	users    ServiceQuotaUserChecker
	sf       singleflight.Group
}

// NewServiceQuotaService 构造服务限额服务。
//
// users 用于在 Create/Update 时校验 target_user_ids 引用的用户是否存在；
// 允许传 nil（生产代码不会，但旧测试桩可能不提供），nil 时跳过用户存在性检查。
func NewServiceQuotaService(
	repo ServiceQuotaRuleRepository,
	settings *SettingService,
	limiter ServiceQuotaLimiter,
	cache ServiceQuotaCache,
	users ServiceQuotaUserChecker,
) ServiceQuotaService {
	return &serviceQuotaService{
		repo:     repo,
		settings: settings,
		limiter:  limiter,
		cache:    cache,
		users:    users,
	}
}

func (s *serviceQuotaService) ListRules(ctx context.Context, filter ServiceQuotaListFilter) ([]*ServiceQuotaRule, error) {
	return s.repo.List(ctx, filter)
}

func (s *serviceQuotaService) CreateRule(ctx context.Context, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	if err := s.normalizeAndValidate(ctx, &input); err != nil {
		return nil, err
	}
	rule, err := s.repo.Create(ctx, input)
	if err != nil {
		return nil, translateServiceQuotaPersistenceError(err)
	}
	s.reloadCache(ctx)
	return rule, nil
}

func (s *serviceQuotaService) UpdateRule(ctx context.Context, id int64, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	if err := s.normalizeAndValidate(ctx, &input); err != nil {
		return nil, err
	}
	// 取旧规则快照：用于事务后比对 limiter window_mode 是否切换 fixed↔rolling，
	// 这是 Bug #2 修复的关键——counterKey 公式不含 window_mode，同 key 切换 mode 后下次写命令
	// 会对旧 key 报 WRONGTYPE 被 fail-open 吞掉，必须主动 reset 旧 key。
	//
	// 同时 snapshot rule.enabled：Bug C 修复——admin 翻转 enabled 时（开→关 或 关→开）
	// 主动清掉该 rule 全部 counter key，避免旧窗口残留计数在重新启用后立即误命中超限。
	oldLimiterModes := s.snapshotRuleLimiterModes(ctx, id)
	oldEnabled := s.snapshotRuleEnabled(ctx, id)
	rule, err := s.repo.Update(ctx, id, input)
	if err != nil {
		return nil, translateServiceQuotaPersistenceError(err)
	}
	s.reloadCache(ctx)
	// reset 路径走 SCAN+DEL，规则越大批次越多 RTT 越长，会阻塞 admin 响应。
	// 改 fire-and-forget：用 context.WithoutCancel 让异步任务不被 admin 请求 ctx 取消打断；
	// 顶层 panic recover 保证单个 reset 失败不会拖垮整个进程。
	asyncLimiters := append([]ServiceQuotaLimiterInput(nil), input.Limiters...)
	s.goAsync(ctx, "reset_counters_mode_switch", id, func(asyncCtx context.Context) {
		s.resetCountersForModeSwitches(asyncCtx, id, oldLimiterModes, asyncLimiters)
	})
	if input.Enabled != nil && oldEnabled != nil && *oldEnabled != *input.Enabled {
		s.goAsync(ctx, "reset_counters_enabled_flip", id, func(asyncCtx context.Context) {
			s.resetAllCountersForRule(asyncCtx, id)
		})
	}
	return rule, nil
}

func (s *serviceQuotaService) DeleteRule(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return translateServiceQuotaPersistenceError(err)
	}
	s.reloadCache(ctx)
	// Bug #3：DB CASCADE 删干净了 rules / paths / limiters，但 Redis 计数 key 只能等 TTL（最长 48h）
	// 自然过期。监控页能查到 "幽灵 rule_id" 的旧 key 一直显示，且占内存。这里主动 SCAN+DEL
	// 该 rule 的所有 counter key（涵盖 path × limiter × scope_user 全笛卡尔积），失败仅 warn 不阻塞。
	//
	// 与 UpdateRule 同样 fire-and-forget：admin 删除请求立即返回，redis 清理在后台跑；
	// 失败仍仅 warn（与同步语义一致），TTL 自然兜底。
	s.goAsync(ctx, "reset_counters_rule_delete", id, func(asyncCtx context.Context) {
		s.resetAllCountersForRule(asyncCtx, id)
	})
	return nil
}

// goAsync 启动 fire-and-forget goroutine 跑 reset 类后台任务。
//
// 设计：
//   - 用 context.WithoutCancel(ctx)：保留 ctx 携带的 trace/log values，但解除 admin 请求的 cancel 联动，
//     避免 admin 响应返回后 ctx 被取消导致 SCAN 半路中止留下"已删一半"的脏 key。
//   - 顶层 defer recover：单个 reset goroutine panic（例如 redis client 内部 nil deref）不能让进程崩溃，
//     用 slog.Error 记录后继续；TTL 仍会兜底清理。
//   - op + ruleID 写到日志便于按规则 ID 关联问题。
//
// 不引 worker pool：admin 写路径调用频次低（人手操作 1Hz 量级），每次 reset 独立 goroutine 即可，
// 引 pool 反而增加生命周期管理复杂度。
func (s *serviceQuotaService) goAsync(ctx context.Context, op string, ruleID int64, fn func(context.Context)) {
	asyncCtx := context.WithoutCancel(ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("service quota async task panicked",
					"op", op,
					"rule_id", ruleID,
					"panic", r,
				)
			}
		}()
		fn(asyncCtx)
	}()
}

// translateServiceQuotaPersistenceError 把 repo 层透传的数据库错误翻译成业务层 ApplicationError，
// 让 handler 通过 response.ErrorFrom 自动得到正确的 HTTP 状态：
//   - sql.ErrNoRows → 404 ErrServiceQuotaRuleNotFound（Update/Delete 找不到规则）
//   - PostgreSQL 23505（unique_violation，例如 service_quota_paths 的 path 重复 / rule_users 重复）→ 409 Conflict
//   - 其他原样透传，由全局错误处理走 500
func translateServiceQuotaPersistenceError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrServiceQuotaRuleNotFound.WithCause(err)
	}
	var pgErr *pq.Error
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pkgerrors.Conflict(
			"SERVICE_QUOTA_CONFLICT",
			"service quota rule conflicts with an existing record",
		).WithMetadata(map[string]string{
			"constraint": pgErr.Constraint,
		}).WithCause(err)
	}
	return fmt.Errorf("service_quota: persistence error: %w", err)
}

func (s *serviceQuotaService) InvalidateEnabledCache(ctx context.Context) {
	if s.cache != nil {
		_ = s.cache.Invalidate(ctx)
	}
}

// IsPreCheckTwoPhase 返回 service_quota_precheck_two_phase setting 的当前值。
// 读取失败或 setting 缺失时一律返回 false（保守降级，沿用旧行为）。
func (s *serviceQuotaService) IsPreCheckTwoPhase(ctx context.Context) bool {
	if s == nil || s.settings == nil {
		return false
	}
	enabled, err := s.settings.GetBool(ctx, SettingKeyServiceQuotaPreCheckTwoPhase)
	if err != nil {
		return false
	}
	return enabled
}

func (s *serviceQuotaService) isEnabled(ctx context.Context) bool {
	return serviceQuotaIsEnabled(ctx, s.cache, s.settings)
}

// serviceQuotaIsEnabled 是 PreCheck/Record 与 Monitor 共享的"读总开关 + fail-open"实现。
//
// 读路径优先级：
//  1. cache 命中（GetEnabled 返回非 nil 且无 error）→ 直接用
//  2. cache 未命中或错误 → 走 SettingService 拉 DB；失败 fail-open 返回 false（与历史一致）
//  3. cache 非 nil 时回填 setting 值，让下次直接命中
//
// 抽出包级函数避免 isEnabled / snapshotEnabled 两份完全相同的实现漂移；
// 任何一边新增逻辑（例如 metrics、tracing）都必须在这里改一次而不是两次。
func serviceQuotaIsEnabled(ctx context.Context, cache ServiceQuotaCache, settings *SettingService) bool {
	if cache != nil {
		val, err := cache.GetEnabled(ctx)
		if err == nil && val != nil {
			return *val
		}
	}
	enabled, err := settings.GetBool(ctx, SettingKeyServiceQuotaEnabled)
	if err != nil {
		return false
	}
	if cache != nil {
		_ = cache.SetEnabled(ctx, enabled)
	}
	return enabled
}

func (s *serviceQuotaService) loadRulesWithCache(ctx context.Context) []*ServiceQuotaRule {
	if s.cache != nil {
		rules, hit, err := s.cache.GetRules(ctx)
		if err == nil && hit {
			return rules
		}
	}
	val, err, _ := s.sf.Do("load_rules", func() (any, error) {
		rules, err := s.repo.List(ctx, ServiceQuotaListFilter{})
		if err != nil {
			return nil, err
		}
		if s.cache != nil {
			_ = s.cache.SetRules(ctx, rules)
		}
		return rules, nil
	})
	if err != nil {
		return nil
	}
	rules, _ := val.([]*ServiceQuotaRule)
	return rules
}

// reloadCache 是写路径在写完 DB 后的缓存失效入口。多实例部署下只能 Del 缓存
// （而不是主动 SetRules），让其他实例下次读时通过 singleflight 重新拉取，
// 否则其他实例仍会持有最长达 serviceQuotaCacheTTL 的旧规则。
func (s *serviceQuotaService) reloadCache(ctx context.Context) {
	if s.cache == nil {
		return
	}
	if err := s.cache.InvalidateRules(ctx); err != nil {
		slog.Warn("invalidate service quota rules cache failed", "error", err)
	}
}

// ReloadCache 暴露给 handler 层在删除 channel/group/account/user 等
// 上游实体后主动失效服务限额规则缓存，下次读时会重新从 DB 拉取。
//
// 实现上仅 Del key，不重新 SELECT 数据库再 SetRules——后者会回到
// "实例 A 立即覆盖、实例 B 仍持旧值" 的旧语义，违背任务 #3 的修复初衷。
func (s *serviceQuotaService) ReloadCache(ctx context.Context) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Invalidate(ctx)
}
