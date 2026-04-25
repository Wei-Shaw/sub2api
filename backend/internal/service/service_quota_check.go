package service

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *serviceQuotaService) PreCheck(ctx context.Context, req ServiceQuotaCheckRequest) (*ServiceQuotaLease, error) {
	matched, ok := s.matchedRules(ctx, req)
	if !ok || len(matched) == 0 {
		return nil, nil
	}
	matched = applyServiceQuotaFallbackOverride(matched)

	lease := &ServiceQuotaLease{}

	// 三轮：concurrency → RPM → deferred(tpm/tpd/daily_usd)。先抢并发额度避免后续判断时占用未释放。
	if err := s.runLimiterRound(ctx, req, matched, lease, ServiceQuotaLimiterConcurrency); err != nil {
		lease.release()
		return nil, err
	}
	if err := s.runLimiterRound(ctx, req, matched, lease, ServiceQuotaLimiterRPM); err != nil {
		lease.release()
		return nil, err
	}
	if err := s.runLimiterRound(ctx, req, matched, lease, ""); err != nil {
		lease.release()
		return nil, err
	}
	return lease, nil
}

// runLimiterRound 跑某一轮限流检查。kind 为空表示 deferred 轮（TPM/TPD/DailyUSD）。
func (s *serviceQuotaService) runLimiterRound(ctx context.Context, req ServiceQuotaCheckRequest, matched []*ServiceQuotaRule, lease *ServiceQuotaLease, kind string) error {
	for _, rule := range matched {
		for _, lim := range rule.Limiters {
			if !roundMatches(kind, lim.LimiterType) {
				continue
			}
			if err := s.runOneLimiter(ctx, req, rule, lim, lease); err != nil {
				return err
			}
		}
	}
	return nil
}

func roundMatches(kind, limiterType string) bool {
	if kind == "" {
		return isDeferredServiceQuotaLimiter(limiterType)
	}
	return limiterType == kind
}

func (s *serviceQuotaService) runOneLimiter(ctx context.Context, req ServiceQuotaCheckRequest, rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef, lease *ServiceQuotaLease) error {
	switch lim.LimiterType {
	case ServiceQuotaLimiterConcurrency:
		return s.acquireConcurrency(ctx, req, rule, lim, lease)
	case ServiceQuotaLimiterRPM:
		return s.checkRPM(ctx, req, rule, lim)
	default:
		return s.checkDeferred(ctx, req, rule, lim)
	}
}

func (s *serviceQuotaService) Record(ctx context.Context, req ServiceQuotaRecordRequest) {
	matched, ok := s.matchedRules(ctx, req.ServiceQuotaCheckRequest)
	if !ok || len(matched) == 0 {
		return
	}
	for _, rule := range applyServiceQuotaFallbackOverride(matched) {
		for _, lim := range rule.Limiters {
			delta := serviceQuotaRecordDelta(lim.LimiterType, req)
			if delta <= 0 {
				continue
			}
			key := s.counterKey(req.ServiceQuotaCheckRequest, rule, lim)
			if _, err := s.limiter.Increment(ctx, key, delta, serviceQuotaWindow(lim), lim.WindowMode); err != nil {
				s.logLimiterFailure("record", rule, lim, key, err)
			}
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
		if !rule.Enabled || !serviceQuotaTargetMatches(rule, req.UserID) {
			continue
		}
		if !dimensionMatches(rule, req) {
			continue
		}
		out = append(out, rule)
	}
	return out, true
}

func (s *serviceQuotaService) acquireConcurrency(ctx context.Context, req ServiceQuotaCheckRequest, rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef, lease *ServiceQuotaLease) error {
	member := fmt.Sprintf("%d:%d", req.UserID, time.Now().UnixNano())
	key := s.counterKey(req, rule, lim)
	ok, err := s.limiter.Acquire(ctx, key, member, int64(lim.LimitValue))
	if err != nil {
		s.logLimiterFailure("acquire_concurrency", rule, lim, key, err)
		return nil
	}
	if !ok {
		return serviceQuotaExceeded(rule, lim, 0)
	}
	prev := lease.Release
	lease.Release = func() {
		if prev != nil {
			prev()
		}
		if rerr := s.limiter.Release(context.Background(), key, member); rerr != nil {
			s.logLimiterFailure("release_concurrency", rule, lim, key, rerr)
		}
	}
	return nil
}

func (s *serviceQuotaService) checkRPM(ctx context.Context, req ServiceQuotaCheckRequest, rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef) error {
	key := s.counterKey(req, rule, lim)
	used, err := s.limiter.Increment(ctx, key, 1, time.Minute, lim.WindowMode)
	if err != nil {
		s.logLimiterFailure("check_rpm", rule, lim, key, err)
		return nil
	}
	if used > lim.LimitValue {
		return serviceQuotaExceeded(rule, lim, used)
	}
	return nil
}

func (s *serviceQuotaService) checkDeferred(ctx context.Context, req ServiceQuotaCheckRequest, rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef) error {
	key := s.counterKey(req, rule, lim)
	used, err := s.limiter.Current(ctx, key, serviceQuotaWindow(lim), lim.WindowMode)
	if err != nil {
		s.logLimiterFailure("check_deferred", rule, lim, key, err)
		return nil
	}
	if used >= lim.LimitValue {
		return serviceQuotaExceeded(rule, lim, used)
	}
	return nil
}

// Fail-open is intentional: when Redis is unreachable we allow the request
// through rather than return 5xx, but we must surface the degradation so the
// operator knows service quota rules are silently inert.
func (s *serviceQuotaService) logLimiterFailure(op string, rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef, key string, err error) {
	slog.Warn("service quota limiter unavailable, rule not enforced",
		"op", op,
		"rule_id", rule.ID,
		"limiter_id", lim.ID,
		"limiter_type", lim.LimiterType,
		"window_mode", lim.WindowMode,
		"key", key,
		"error", err,
	)
}

// counterKey 返回 rule × limiter 的独立计数 key。v3 前缀使旧 key 自然失效（v2 含 path_id 已不再适用）。
func (s *serviceQuotaService) counterKey(req ServiceQuotaCheckRequest, rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef) string {
	target := "shared"
	if rule.CounterMode == ServiceQuotaCounterModeUser || rule.CounterMode == ServiceQuotaCounterModePerUser {
		target = strconv.FormatInt(req.UserID, 10)
	}
	return fmt.Sprintf("svcquota:v3:%d:%s:%s", rule.ID, lim.LimiterType, target)
}

func (l *ServiceQuotaLease) release() {
	if l != nil && l.Release != nil {
		l.Release()
	}
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

// dimensionMatches 判断规则的 5 个维度集合是否命中本次请求。
// 每个维度独立判断：集合为空 = 不限制；非空 = 必须包含请求的对应字段。
func dimensionMatches(rule *ServiceQuotaRule, req ServiceQuotaCheckRequest) bool {
	if len(rule.Platforms) > 0 && !containsStringFold(rule.Platforms, req.Platform) {
		return false
	}
	if len(rule.ChannelIDs) > 0 && !containsInt64(rule.ChannelIDs, req.ChannelID) {
		return false
	}
	if len(rule.GroupIDs) > 0 && !containsInt64(rule.GroupIDs, req.GroupID) {
		return false
	}
	if len(rule.AccountIDs) > 0 && !containsInt64(rule.AccountIDs, req.AccountID) {
		return false
	}
	if len(rule.ModelPatterns) > 0 && !anyModelPatternMatches(rule.ModelPatterns, req.Model) {
		return false
	}
	return true
}

func containsStringFold(values []string, target string) bool {
	for _, v := range values {
		if strings.EqualFold(v, target) {
			return true
		}
	}
	return false
}


// anyModelPatternMatches 命中规则的任一 model_pattern 即视为匹配。空 pattern 视为通配。
func anyModelPatternMatches(patterns []string, model string) bool {
	for _, p := range patterns {
		if serviceQuotaModelMatches(p, model) {
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

// applyServiceQuotaFallbackOverride 在限流器层级处理 is_fallback 语义：
// 同一 limiter_type 若有任意非 fallback 规则命中并配置了该类型限流器，则丢弃 fallback 规则中该类型的限流器
// （而不是整条 fallback 规则）。如果丢空了 fallback 规则的所有限流器，则该 fallback 规则整条被忽略。
func applyServiceQuotaFallbackOverride(matched []*ServiceQuotaRule) []*ServiceQuotaRule {
	covered := map[string]bool{}
	for _, rule := range matched {
		if rule.IsFallback {
			continue
		}
		for _, lim := range rule.Limiters {
			covered[lim.LimiterType] = true
		}
	}
	out := make([]*ServiceQuotaRule, 0, len(matched))
	for _, rule := range matched {
		if !rule.IsFallback {
			out = append(out, rule)
			continue
		}
		kept := make([]ServiceQuotaLimiterDef, 0, len(rule.Limiters))
		for _, lim := range rule.Limiters {
			if covered[lim.LimiterType] {
				continue
			}
			kept = append(kept, lim)
		}
		if len(kept) == 0 {
			continue
		}
		clone := *rule
		clone.Limiters = kept
		out = append(out, &clone)
	}
	return out
}

func isDeferredServiceQuotaLimiter(kind string) bool {
	return kind == ServiceQuotaLimiterTPM || kind == ServiceQuotaLimiterTPD || kind == ServiceQuotaLimiterDailyUSD
}

func serviceQuotaWindow(lim ServiceQuotaLimiterDef) time.Duration {
	switch lim.LimiterType {
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

func serviceQuotaExceeded(rule *ServiceQuotaRule, lim ServiceQuotaLimiterDef, used float64) error {
	return ErrServiceQuotaExceeded.WithMetadata(map[string]string{
		"rule_id":      strconv.FormatInt(rule.ID, 10),
		"limiter_type": lim.LimiterType,
		"limit":        strconv.FormatFloat(lim.LimitValue, 'f', -1, 64),
		"used":         strconv.FormatFloat(used, 'f', -1, 64),
	})
}