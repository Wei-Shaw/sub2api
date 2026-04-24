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
)

type serviceQuotaService struct {
	repo     ServiceQuotaRuleRepository
	settings quotaSettingsProvider
	limiter  ServiceQuotaLimiter
}

func NewServiceQuotaService(repo ServiceQuotaRuleRepository, settings quotaSettingsProvider, limiter ServiceQuotaLimiter) ServiceQuotaService {
	return &serviceQuotaService{repo: repo, settings: settings, limiter: limiter}
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
	return s.repo.Create(ctx, input)
}

func (s *serviceQuotaService) UpdateRule(ctx context.Context, id int64, input ServiceQuotaRuleInput) (*ServiceQuotaRule, error) {
	if err := normalizeServiceQuotaRule(&input); err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, id, input)
}

func (s *serviceQuotaService) DeleteRule(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *serviceQuotaService) PreCheck(ctx context.Context, req ServiceQuotaCheckRequest) (*ServiceQuotaLease, error) {
	rules, ok := s.matchedRules(ctx, req)
	if !ok || len(rules) == 0 {
		return nil, nil
	}
	rules = applyServiceQuotaDefaultOverride(rules)
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
	for _, rule := range applyServiceQuotaDefaultOverride(rules) {
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
	enabled, err := s.settings.GetBool(ctx, SettingKeyServiceQuotaEnabled)
	if err != nil || !enabled {
		return nil, false
	}
	rules, err := s.repo.List(ctx, ServiceQuotaListFilter{})
	if err != nil {
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
// admin list response. Only global-scope counters (target_mode=shared or a
// specific user) and non-concurrency limiters are resolvable with a single
// lookup; per_user/default rules are sharded per request user and are left as
// nil rather than paying the cost of scanning every per-user key.
func (s *serviceQuotaService) fillCurrentUsage(ctx context.Context, rule *ServiceQuotaRule) {
	if rule == nil || s.limiter == nil {
		return
	}
	if rule.LimiterType == ServiceQuotaLimiterConcurrency {
		return
	}
	var target string
	switch rule.TargetMode {
	case ServiceQuotaTargetShared:
		target = "shared"
	case ServiceQuotaTargetUser:
		if rule.TargetUserID == nil {
			return
		}
		target = strconv.FormatInt(*rule.TargetUserID, 10)
	default:
		return
	}
	key := fmt.Sprintf("svcquota:%d:%s:%s", rule.ID, rule.LimiterType, target)
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
	if rule.TargetMode == ServiceQuotaTargetUser || rule.TargetMode == ServiceQuotaTargetPerUser || rule.TargetMode == ServiceQuotaTargetDefault {
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
	if input.ScopeLevel == "" {
		input.ScopeLevel = ServiceQuotaScopeGlobal
	}
	if input.TargetMode == "" {
		input.TargetMode = ServiceQuotaTargetPerUser
	}
	if input.WindowMode == "" || input.LimiterType == ServiceQuotaLimiterConcurrency {
		input.WindowMode = ServiceQuotaWindowFixed
	}
	if input.Enabled == nil {
		enabled := true
		input.Enabled = &enabled
	}
	if input.TargetMode == ServiceQuotaTargetUser && input.TargetUserID == nil {
		return pkgerrors.BadRequest("SERVICE_QUOTA_INVALID_TARGET", "target_user_id is required")
	}
	return nil
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
	return rule.TargetMode != ServiceQuotaTargetUser || (rule.TargetUserID != nil && *rule.TargetUserID == userID)
}

func serviceQuotaModelMatches(pattern, model string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return true
	}
	ok, err := filepath.Match(pattern, model)
	return err == nil && ok
}

func applyServiceQuotaDefaultOverride(rules []*ServiceQuotaRule) []*ServiceQuotaRule {
	nonDefault := map[string]bool{}
	for _, rule := range rules {
		if rule.TargetMode != ServiceQuotaTargetDefault {
			nonDefault[rule.LimiterType] = true
		}
	}
	out := rules[:0]
	for _, rule := range rules {
		if rule.TargetMode == ServiceQuotaTargetDefault && nonDefault[rule.LimiterType] {
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
