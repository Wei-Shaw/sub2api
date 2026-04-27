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
	s.resetCountersForModeSwitches(ctx, id, oldLimiterModes, input.Limiters)
	if input.Enabled != nil && oldEnabled != nil && *oldEnabled != *input.Enabled {
		s.resetAllCountersForRule(ctx, id)
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
	s.resetAllCountersForRule(ctx, id)
	return nil
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

// IsPreCheckTwoPhase 返回 service_quota.precheck_two_phase setting 的当前值。
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
	if s.cache != nil {
		val, err := s.cache.GetEnabled(ctx)
		if err == nil && val != nil {
			return *val
		}
	}
	enabled, err := s.settings.GetBool(ctx, SettingKeyServiceQuotaEnabled)
	if err != nil {
		return false
	}
	if s.cache != nil {
		_ = s.cache.SetEnabled(ctx, enabled)
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
