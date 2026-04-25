package service

import (
	"context"
	"strings"

	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/sync/singleflight"
)

type serviceQuotaService struct {
	repo     ServiceQuotaRuleRepository
	settings *SettingService
	limiter  ServiceQuotaLimiter
	cache    ServiceQuotaCache
	sf       singleflight.Group
}

func NewServiceQuotaService(repo ServiceQuotaRuleRepository, settings *SettingService, limiter ServiceQuotaLimiter, cache ServiceQuotaCache) ServiceQuotaService {
	return &serviceQuotaService{repo: repo, settings: settings, limiter: limiter, cache: cache}
}

func (s *serviceQuotaService) ListRules(ctx context.Context, filter ServiceQuotaListFilter) ([]*ServiceQuotaRule, error) {
	return s.repo.List(ctx, filter)
}

func (s *serviceQuotaService) CreateRule(ctx context.Context, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	if err := s.normalizeAndValidate(&input); err != nil {
		return nil, err
	}
	rule, err := s.repo.Create(ctx, input)
	if err != nil {
		return nil, err
	}
	s.reloadCache(ctx)
	return rule, nil
}

func (s *serviceQuotaService) UpdateRule(ctx context.Context, id int64, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	if err := s.normalizeAndValidate(&input); err != nil {
		return nil, err
	}
	rule, err := s.repo.Update(ctx, id, input)
	if err != nil {
		return nil, err
	}
	s.reloadCache(ctx)
	return rule, nil
}

func (s *serviceQuotaService) DeleteRule(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.reloadCache(ctx)
	return nil
}

func (s *serviceQuotaService) InvalidateEnabledCache(ctx context.Context) {
	if s.cache != nil {
		_ = s.cache.Invalidate(ctx)
	}
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
	val, err, _ := s.sf.Do("load_rules", func() (interface{}, error) {
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
	return val.([]*ServiceQuotaRule)
}

func (s *serviceQuotaService) reloadCache(ctx context.Context) {
	if s.cache == nil {
		return
	}
	rules, err := s.repo.List(ctx, ServiceQuotaListFilter{})
	if err != nil {
		_ = s.cache.Invalidate(ctx)
		return
	}
	_ = s.cache.SetRules(ctx, rules)
}

func (s *serviceQuotaService) normalizeAndValidate(input *ServiceQuotaRuleInput) error {
	if input == nil {
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_RULE", "invalid service quota rule")
	}
	if input.CounterMode == "" {
		input.CounterMode = ServiceQuotaCounterModePerUser
	}
	switch input.CounterMode {
	case ServiceQuotaCounterModeUser, ServiceQuotaCounterModePerUser, ServiceQuotaCounterModeShared:
	default:
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_COUNTER_MODE", "invalid counter_mode")
	}
	if input.Enabled == nil {
		enabled := true
		input.Enabled = &enabled
	}
	input.TargetUserIDs = sanitizeTargetUserIDs(input.TargetUserIDs)
	if input.CounterMode == ServiceQuotaCounterModeUser && len(input.TargetUserIDs) == 0 {
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_TARGET", "target_user_ids is required when counter_mode=user")
	}
	if input.CounterMode != ServiceQuotaCounterModeUser {
		input.TargetUserIDs = nil
	}
	if err := normalizeLimiters(input); err != nil {
		return err
	}
	normalizeDimensions(input)
	return nil
}

func normalizeLimiters(input *ServiceQuotaRuleInput) error {
	if len(input.Limiters) == 0 {
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_LIMITERS", "at least one limiter is required")
	}
	seen := make(map[string]struct{}, len(input.Limiters))
	for i := range input.Limiters {
		if err := normalizeOneLimiter(&input.Limiters[i], seen); err != nil {
			return err
		}
	}
	return nil
}

func normalizeOneLimiter(l *ServiceQuotaLimiterInput, seen map[string]struct{}) error {
	switch l.LimiterType {
	case ServiceQuotaLimiterRPM, ServiceQuotaLimiterTPM, ServiceQuotaLimiterTPD,
		ServiceQuotaLimiterDailyUSD, ServiceQuotaLimiterConcurrency:
	default:
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_LIMITER_TYPE", "invalid limiter_type").WithMetadata(map[string]string{
			"limiter_type": l.LimiterType,
		})
	}
	if l.LimitValue <= 0 {
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_LIMIT_VALUE", "limit_value must be > 0").WithMetadata(map[string]string{
			"limiter_type": l.LimiterType,
		})
	}
	if l.WindowMode == "" || l.LimiterType == ServiceQuotaLimiterConcurrency {
		l.WindowMode = ServiceQuotaWindowFixed
	}
	switch l.WindowMode {
	case ServiceQuotaWindowFixed, ServiceQuotaWindowRolling:
	default:
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_WINDOW_MODE", "invalid window_mode").WithMetadata(map[string]string{
			"window_mode": l.WindowMode,
		})
	}
	if _, dup := seen[l.LimiterType]; dup {
		return pkgerrors.BadRequest("SERVICE_QUOTA_DUPLICATE_LIMITER", "duplicate limiter_type in rule").WithMetadata(map[string]string{
			"limiter_type": l.LimiterType,
		})
	}
	seen[l.LimiterType] = struct{}{}
	return nil
}

// normalizeDimensions 把 5 个维度集合归一化：trim、平台小写、去重、去空。
// 不做跨维度一致性校验：维度采用 AND-of-sets 语义，不一致的组合自然不会命中真实请求，
// 不需要在写入时拦截（DB FK 已保证 channel/group/account 的存在性）。
func normalizeDimensions(input *ServiceQuotaRuleInput) {
	input.Platforms = dedupStrings(input.Platforms, true)
	input.ModelPatterns = dedupStrings(input.ModelPatterns, false)
	input.ChannelIDs = dedupInt64s(input.ChannelIDs)
	input.GroupIDs = dedupInt64s(input.GroupIDs)
	input.AccountIDs = dedupInt64s(input.AccountIDs)
}

func dedupStrings(values []string, lower bool) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if lower {
			v = strings.ToLower(v)
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func dedupInt64s(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, v := range values {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func sanitizeTargetUserIDs(ids []int64) []int64 {
	return dedupInt64s(ids)
}