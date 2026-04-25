package service

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
	rules, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		s.fillCurrentUsage(ctx, rule)
	}
	return rules, nil
}

func (s *serviceQuotaService) CreateRule(ctx context.Context, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	if err := normalizeServiceQuotaRule(&input); err != nil {
		return nil, err
	}
	if err := s.validateScopeLinkage(ctx, input); err != nil {
		return nil, err
	}
	rule, err := s.repo.Create(ctx, input)
	if err != nil {
		return nil, err
	}
	s.reloadCache(ctx)
	return rule, nil
}

func (s *serviceQuotaService) CreateBatch(ctx context.Context, inputs []ServiceQuotaRuleInput) ([]*ServiceQuotaRule, error) {
	for i := range inputs {
		if err := normalizeServiceQuotaRule(&inputs[i]); err != nil {
			return nil, err
		}
		if err := s.validateScopeLinkage(ctx, inputs[i]); err != nil {
			return nil, err
		}
	}
	rules, err := s.repo.CreateBatch(ctx, inputs)
	if err != nil {
		return nil, err
	}
	s.reloadCache(ctx)
	return rules, nil
}

func (s *serviceQuotaService) UpdateRule(ctx context.Context, id int64, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	if err := normalizeServiceQuotaRule(&input); err != nil {
		return nil, err
	}
	if err := s.validateScopeLinkage(ctx, input); err != nil {
		return nil, err
	}
	rule, err := s.repo.Update(ctx, id, input)
	if err != nil {
		return nil, err
	}
	s.reloadCache(ctx)
	return rule, nil
}

func (s *serviceQuotaService) UpdateBatch(ctx context.Context, batchID string, patch ServiceQuotaBatchPatch) error {
	if _, err := s.repo.UpdateBatch(ctx, batchID, patch); err != nil {
		return err
	}
	s.reloadCache(ctx)
	return nil
}

// validateScopeLinkage 校验 scope 字段之间的链路一致性：
//   - 同时填 account_id 和 group_id：account 必须属于 group（account_groups 中间表命中）
//   - 同时填 account_id 和 platform：account.platform 必须与 platform 一致
//   - 同时填 group_id 和 platform：group.platform 必须与 platform 一致
func (s *serviceQuotaService) validateScopeLinkage(ctx context.Context, input ServiceQuotaRuleInput) error {
	platform := trimPlatform(input.Platform)

	var accountInfo *AccountScopeInfo
	if input.AccountID != nil && *input.AccountID > 0 {
		info, err := s.repo.FetchAccountScope(ctx, *input.AccountID)
		if err != nil {
			return err
		}
		if info == nil {
			return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_ACCOUNT_NOT_FOUND", "account not found").WithMetadata(map[string]string{
				"account_id": strconv.FormatInt(*input.AccountID, 10),
			})
		}
		accountInfo = info
	}

	var groupInfo *GroupScopeInfo
	if input.GroupID != nil && *input.GroupID > 0 {
		info, err := s.repo.FetchGroupScope(ctx, *input.GroupID)
		if err != nil {
			return err
		}
		if info == nil {
			return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_GROUP_NOT_FOUND", "group not found").WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(*input.GroupID, 10),
			})
		}
		groupInfo = info
	}

	if accountInfo != nil && input.GroupID != nil && *input.GroupID > 0 {
		found := false
		for _, gid := range accountInfo.GroupIDs {
			if gid == *input.GroupID {
				found = true
				break
			}
		}
		if !found {
			return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_MISMATCH", "account is not a member of the specified group").WithMetadata(map[string]string{
				"field":      "account_id",
				"account_id": strconv.FormatInt(*input.AccountID, 10),
				"group_id":   strconv.FormatInt(*input.GroupID, 10),
			})
		}
	}

	if accountInfo != nil && platform != "" && !strings.EqualFold(accountInfo.Platform, platform) {
		return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_MISMATCH", "account platform does not match the specified platform").WithMetadata(map[string]string{
			"field":             "account_id",
			"account_id":        strconv.FormatInt(*input.AccountID, 10),
			"account_platform":  accountInfo.Platform,
			"expected_platform": platform,
		})
	}

	if groupInfo != nil && platform != "" && !strings.EqualFold(groupInfo.Platform, platform) {
		return pkgerrors.BadRequest("SERVICE_QUOTA_SCOPE_MISMATCH", "group platform does not match the specified platform").WithMetadata(map[string]string{
			"field":             "group_id",
			"group_id":          strconv.FormatInt(*input.GroupID, 10),
			"group_platform":    groupInfo.Platform,
			"expected_platform": platform,
		})
	}

	return nil
}

func trimPlatform(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func (s *serviceQuotaService) DeleteRule(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.reloadCache(ctx)
	return nil
}

func (s *serviceQuotaService) DeleteBatch(ctx context.Context, batchID string) error {
	if _, err := s.repo.DeleteBatch(ctx, batchID); err != nil {
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

func (s *serviceQuotaService) PreCheck(ctx context.Context, req ServiceQuotaCheckRequest) (*ServiceQuotaLease, error) {
	rules, ok := s.matchedRules(ctx, req)
	if !ok || len(rules) == 0 {
		return nil, nil
	}
	rules = applyServiceQuotaFallbackOverride(rules)
	lease := &ServiceQuotaLease{}
	for _, rule := range rules {
		if rule.LimiterType == ServiceQuotaLimiterConcurrency {
			if err := s.acquireConcurrency(ctx, req, rule, lease); err != nil {
				lease.release()
				return nil, err
			}
		}
	}
	for _, rule := range rules {
		if rule.LimiterType == ServiceQuotaLimiterRPM {
			if err := s.checkRPM(ctx, req, rule); err != nil {
				lease.release()
				return nil, err
			}
		}
	}
	for _, rule := range rules {
		if isDeferredServiceQuotaLimiter(rule.LimiterType) {
			if err := s.checkDeferred(ctx, req, rule); err != nil {
				lease.release()
				return nil, err
			}
		}
	}
	return lease, nil
}

func (s *serviceQuotaService) Record(ctx context.Context, req ServiceQuotaRecordRequest) {
	rules, ok := s.matchedRules(ctx, req.ServiceQuotaCheckRequest)
	if !ok || len(rules) == 0 {
		return
	}
	for _, rule := range applyServiceQuotaFallbackOverride(rules) {
		delta := serviceQuotaRecordDelta(rule.LimiterType, req)
		if delta <= 0 {
			continue
		}
		key := s.counterKey(req.ServiceQuotaCheckRequest, rule)
		if _, err := s.limiter.Increment(ctx, key, delta, serviceQuotaWindow(rule), rule.WindowMode); err != nil {
			s.logLimiterFailure("record", rule, key, err)
		}
	}
}

func (s *serviceQuotaService) matchedRules(ctx context.Context, req ServiceQuotaCheckRequest) ([]*ServiceQuotaRule, bool) {
	if s == nil || s.repo == nil || s.settings == nil || s.limiter == nil || req.UserID <= 0 {
		return nil, false
	}
	if !s.isEnabled(ctx) {
		return nil, false
	}
	rules := s.loadRulesWithCache(ctx)
	if rules == nil {
		return nil, false
	}
	out := make([]*ServiceQuotaRule, 0, len(rules))
	for _, rule := range rules {
		if serviceQuotaRuleMatches(rule, req) {
			out = append(out, rule)
		}
	}
	return out, true
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

func (s *serviceQuotaService) acquireConcurrency(ctx context.Context, req ServiceQuotaCheckRequest, rule *ServiceQuotaRule, lease *ServiceQuotaLease) error {
	member := fmt.Sprintf("%d:%d", req.UserID, time.Now().UnixNano())
	key := s.counterKey(req, rule)
	ok, err := s.limiter.Acquire(ctx, key, member, int64(rule.LimitValue))
	if err != nil {
		s.logLimiterFailure("acquire_concurrency", rule, key, err)
		return nil
	}
	if !ok {
		return serviceQuotaExceeded(rule, 0)
	}
	prev := lease.Release
	lease.Release = func() {
		if prev != nil {
			prev()
		}
		if rerr := s.limiter.Release(context.Background(), key, member); rerr != nil {
			s.logLimiterFailure("release_concurrency", rule, key, rerr)
		}
	}
	return nil
}

func (s *serviceQuotaService) checkRPM(ctx context.Context, req ServiceQuotaCheckRequest, rule *ServiceQuotaRule) error {
	key := s.counterKey(req, rule)
	used, err := s.limiter.Increment(ctx, key, 1, time.Minute, rule.WindowMode)
	if err != nil {
		s.logLimiterFailure("check_rpm", rule, key, err)
		return nil
	}
	if used > rule.LimitValue {
		return serviceQuotaExceeded(rule, used)
	}
	return nil
}

func (s *serviceQuotaService) checkDeferred(ctx context.Context, req ServiceQuotaCheckRequest, rule *ServiceQuotaRule) error {
	key := s.counterKey(req, rule)
	used, err := s.limiter.Current(ctx, key, serviceQuotaWindow(rule), rule.WindowMode)
	if err != nil {
		s.logLimiterFailure("check_deferred", rule, key, err)
		return nil
	}
	if used >= rule.LimitValue {
		return serviceQuotaExceeded(rule, used)
	}
	return nil
}

// fillCurrentUsage best-effort annotates a rule with its current usage for the
// admin list response. Only shared-counter rules resolve with a single lookup;
// per-user and multi-user sharded rules are left as nil rather than scanning
// every per-user key (N lookups per rule × M rules).
func (s *serviceQuotaService) fillCurrentUsage(ctx context.Context, rule *ServiceQuotaRule) {
	if rule == nil || s.limiter == nil {
		return
	}
	if rule.LimiterType == ServiceQuotaLimiterConcurrency {
		return
	}
	if rule.CounterMode != ServiceQuotaCounterModeShared {
		return
	}
	key := fmt.Sprintf("svcquota:%d:%s:shared", rule.ID, rule.LimiterType)
	used, err := s.limiter.Current(ctx, key, serviceQuotaWindow(rule), rule.WindowMode)
	if err != nil {
		s.logLimiterFailure("current_usage", rule, key, err)
		return
	}
	rule.CurrentUsage = &used
}

// Fail-open is intentional: when Redis is unreachable we allow the request
// through rather than return 5xx, but we must surface the degradation so the
// operator knows service quota rules are silently inert.
func (s *serviceQuotaService) logLimiterFailure(op string, rule *ServiceQuotaRule, key string, err error) {
	slog.Warn("service quota limiter unavailable, rule not enforced",
		"op", op,
		"rule_id", rule.ID,
		"limiter_type", rule.LimiterType,
		"window_mode", rule.WindowMode,
		"key", key,
		"error", err,
	)
}

func (s *serviceQuotaService) counterKey(req ServiceQuotaCheckRequest, rule *ServiceQuotaRule) string {
	target := "shared"
	if rule.CounterMode == ServiceQuotaCounterModeUser || rule.CounterMode == ServiceQuotaCounterModePerUser {
		target = strconv.FormatInt(req.UserID, 10)
	}
	return fmt.Sprintf("svcquota:%d:%s:%s", rule.ID, rule.LimiterType, target)
}

func (l *ServiceQuotaLease) release() {
	if l != nil && l.Release != nil {
		l.Release()
	}
}

func normalizeServiceQuotaRule(input *ServiceQuotaRuleInput) error {
	if input == nil || input.LimitValue <= 0 {
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
	if input.WindowMode == "" || input.LimiterType == ServiceQuotaLimiterConcurrency {
		input.WindowMode = ServiceQuotaWindowFixed
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
	return nil
}

func sanitizeTargetUserIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func serviceQuotaRuleMatches(rule *ServiceQuotaRule, req ServiceQuotaCheckRequest) bool {
	if rule == nil || !rule.Enabled || !serviceQuotaTargetMatches(rule, req.UserID) {
		return false
	}
	if rule.Platform != nil && !strings.EqualFold(*rule.Platform, req.Platform) {
		return false
	}
	if rule.GroupID != nil && *rule.GroupID != req.GroupID {
		return false
	}
	if rule.AccountID != nil && *rule.AccountID != req.AccountID {
		return false
	}
	if rule.ModelPattern != nil && !serviceQuotaModelMatches(*rule.ModelPattern, req.Model) {
		return false
	}
	return true
}

func serviceQuotaTargetMatches(rule *ServiceQuotaRule, userID int64) bool {
	if rule.CounterMode != ServiceQuotaCounterModeUser {
		return true
	}
	for _, id := range rule.TargetUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}

func serviceQuotaModelMatches(pattern, model string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return true
	}
	ok, err := filepath.Match(pattern, model)
	return err == nil && ok
}

// applyServiceQuotaFallbackOverride 在已匹配的规则集中处理 is_fallback 语义：
// 若同一 limiter_type 存在任何非 fallback 规则，则丢弃该 limiter_type 的所有 fallback 规则。
// 匹配时的 scope 层级判断已由 serviceQuotaRuleMatches 完成，这里只看是否有更具体（非 fallback）规则生效。
func applyServiceQuotaFallbackOverride(rules []*ServiceQuotaRule) []*ServiceQuotaRule {
	hasConcrete := map[string]bool{}
	for _, rule := range rules {
		if !rule.IsFallback {
			hasConcrete[rule.LimiterType] = true
		}
	}
	out := rules[:0]
	for _, rule := range rules {
		if rule.IsFallback && hasConcrete[rule.LimiterType] {
			continue
		}
		out = append(out, rule)
	}
	return out
}

func isDeferredServiceQuotaLimiter(kind string) bool {
	return kind == ServiceQuotaLimiterTPM || kind == ServiceQuotaLimiterTPD || kind == ServiceQuotaLimiterDailyUSD
}

func serviceQuotaWindow(rule *ServiceQuotaRule) time.Duration {
	switch rule.LimiterType {
	case ServiceQuotaLimiterRPM, ServiceQuotaLimiterTPM:
		return time.Minute
	case ServiceQuotaLimiterTPD, ServiceQuotaLimiterDailyUSD:
		return 24 * time.Hour
	default:
		return time.Minute
	}
}

func serviceQuotaRecordDelta(kind string, req ServiceQuotaRecordRequest) float64 {
	switch kind {
	case ServiceQuotaLimiterTPM, ServiceQuotaLimiterTPD:
		return float64(req.Tokens)
	case ServiceQuotaLimiterDailyUSD:
		return req.Cost
	default:
		return 0
	}
}

func serviceQuotaExceeded(rule *ServiceQuotaRule, used float64) error {
	return ErrServiceQuotaExceeded.WithMetadata(map[string]string{
		"rule_id":      strconv.FormatInt(rule.ID, 10),
		"limiter_type": rule.LimiterType,
		"limit":        strconv.FormatFloat(rule.LimitValue, 'f', -1, 64),
		"used":         strconv.FormatFloat(used, 'f', -1, 64),
	})
}
