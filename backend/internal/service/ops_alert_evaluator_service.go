package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	opsAlertEvaluatorJobName = "ops_alert_evaluator"

	opsAlertEvaluatorTimeout         = 45 * time.Second
	opsAlertEvaluatorLeaderLockKey   = "ops:alert:evaluator:leader"
	opsAlertEvaluatorLeaderLockTTL   = 90 * time.Second
	opsAlertEvaluatorSkipLogInterval = 1 * time.Minute
)

var opsAlertEvaluatorReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type OpsAlertEvaluatorService struct {
	opsService   *OpsService
	opsRepo      OpsRepository
	emailService *EmailService
	proxyRepo    ProxyRepository

	redisClient *redis.Client
	cfg         *config.Config
	instanceID  string

	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup

	mu         sync.Mutex
	ruleStates map[int64]*opsAlertRuleState

	emailLimiter *slidingWindowLimiter

	skipLogMu sync.Mutex
	skipLogAt time.Time

	warnNoRedisOnce   sync.Once
	telegramNotifySvc *TelegramNotifyService
}

type opsAlertRuleState struct {
	LastEvaluatedAt     time.Time
	ConsecutiveBreaches int
}

type opsAlertAccountBreakdownRepository interface {
	ListAlertAccountBreakdown(ctx context.Context, filter *OpsAlertAccountBreakdownFilter) ([]*OpsAlertAccountBreakdown, error)
}

func NewOpsAlertEvaluatorService(
	opsService *OpsService,
	opsRepo OpsRepository,
	emailService *EmailService,
	redisClient *redis.Client,
	cfg *config.Config,
	proxyRepo ProxyRepository,
) *OpsAlertEvaluatorService {
	return &OpsAlertEvaluatorService{
		opsService:   opsService,
		opsRepo:      opsRepo,
		emailService: emailService,
		proxyRepo:    proxyRepo,
		redisClient:  redisClient,
		cfg:          cfg,
		instanceID:   uuid.NewString(),
		ruleStates:   map[int64]*opsAlertRuleState{},
		emailLimiter: newSlidingWindowLimiter(0, time.Hour),
	}
}

func (s *OpsAlertEvaluatorService) SetTelegramNotifyService(svc *TelegramNotifyService) {
	s.telegramNotifySvc = svc
}

func (s *OpsAlertEvaluatorService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		if s.stopCh == nil {
			s.stopCh = make(chan struct{})
		}
		s.wg.Add(1)
		go s.run()
	})
}

func (s *OpsAlertEvaluatorService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})
	s.wg.Wait()
}

func (s *OpsAlertEvaluatorService) run() {
	defer s.wg.Done()

	// Start immediately to produce early feedback in ops dashboard.
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			interval := s.getInterval()
			s.evaluateOnce(interval)
			timer.Reset(interval)
		case <-s.stopCh:
			return
		}
	}
}

func (s *OpsAlertEvaluatorService) getInterval() time.Duration {
	// Default.
	interval := 60 * time.Second

	if s == nil || s.opsService == nil {
		return interval
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cfg, err := s.opsService.GetOpsAlertRuntimeSettings(ctx)
	if err != nil || cfg == nil {
		return interval
	}
	if cfg.EvaluationIntervalSeconds <= 0 {
		return interval
	}
	if cfg.EvaluationIntervalSeconds < 1 {
		return interval
	}
	if cfg.EvaluationIntervalSeconds > int((24 * time.Hour).Seconds()) {
		return interval
	}
	return time.Duration(cfg.EvaluationIntervalSeconds) * time.Second
}

func (s *OpsAlertEvaluatorService) evaluateOnce(interval time.Duration) {
	if s == nil || s.opsRepo == nil {
		return
	}
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), opsAlertEvaluatorTimeout)
	defer cancel()

	if s.opsService != nil && !s.opsService.IsMonitoringEnabled(ctx) {
		return
	}

	runtimeCfg := defaultOpsAlertRuntimeSettings()
	if s.opsService != nil {
		if loaded, err := s.opsService.GetOpsAlertRuntimeSettings(ctx); err == nil && loaded != nil {
			runtimeCfg = loaded
		}
	}

	release, ok := s.tryAcquireLeaderLock(ctx, runtimeCfg.DistributedLock)
	if !ok {
		return
	}
	if release != nil {
		defer release()
	}

	startedAt := time.Now().UTC()
	runAt := startedAt

	rules, err := s.opsRepo.ListAlertRules(ctx)
	if err != nil {
		s.recordHeartbeatError(runAt, time.Since(startedAt), err)
		logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] list rules failed: %v", err)
		return
	}
	rules = normalizeDefaultOpsAlertRules(rules)

	rulesTotal := len(rules)
	rulesEnabled := 0
	rulesEvaluated := 0
	eventsCreated := 0
	eventsResolved := 0
	emailsSent := 0

	now := time.Now().UTC()
	safeEnd := now.Truncate(time.Minute)
	if safeEnd.IsZero() {
		safeEnd = now
	}

	systemMetrics, _ := s.opsRepo.GetLatestSystemMetrics(ctx, 1)

	// Cleanup stale state for removed rules.
	s.pruneRuleStates(rules)

	for _, rule := range rules {
		if rule == nil || !rule.Enabled || rule.ID <= 0 {
			continue
		}
		rulesEnabled++

		scopePlatform, scopeGroupID, scopeRegion := parseOpsAlertRuleScope(rule.Filters)

		windowMinutes := rule.WindowMinutes
		if windowMinutes <= 0 {
			windowMinutes = 1
		}
		windowStart := safeEnd.Add(-time.Duration(windowMinutes) * time.Minute)
		windowEnd := safeEnd

		metricValue, ok := s.computeRuleMetric(ctx, rule, systemMetrics, windowStart, windowEnd, scopePlatform, scopeGroupID)
		if !ok {
			s.resetRuleState(rule.ID, now)
			continue
		}
		rulesEvaluated++

		breachedNow := compareMetric(metricValue, rule.Operator, rule.Threshold)
		required := requiredSustainedBreaches(rule.SustainedMinutes, interval)
		consecutive := s.updateRuleBreaches(rule.ID, now, interval, breachedNow)

		activeEvent, err := s.opsRepo.GetActiveAlertEvent(ctx, rule.ID)
		if err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] get active event failed (rule=%d): %v", rule.ID, err)
			continue
		}

		if breachedNow && consecutive >= required {
			if activeEvent != nil {
				continue
			}

			// Scoped silencing: if a matching silence exists, skip creating a firing event.
			if s.opsService != nil {
				platform := strings.TrimSpace(scopePlatform)
				region := scopeRegion
				if platform != "" {
					if ok, err := s.opsService.IsAlertSilenced(ctx, rule.ID, platform, scopeGroupID, region, now); err == nil && ok {
						continue
					}
				}
			}

			latestEvent, err := s.opsRepo.GetLatestAlertEvent(ctx, rule.ID)
			if err != nil {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] get latest event failed (rule=%d): %v", rule.ID, err)
				continue
			}
			if latestEvent != nil && rule.CooldownMinutes > 0 {
				cooldown := time.Duration(rule.CooldownMinutes) * time.Minute
				if now.Sub(latestEvent.FiredAt) < cooldown {
					continue
				}
			}

			baseDescription := buildOpsAlertDescription(rule, metricValue, windowMinutes, scopePlatform, scopeGroupID)
			accountContext := s.buildOpsAlertAccountContext(ctx, rule, windowStart, windowEnd, scopePlatform, scopeGroupID)
			eventDescription := appendOpsAlertAccountContext(baseDescription, accountContext)

			firedEvent := &OpsAlertEvent{
				RuleID:         rule.ID,
				Severity:       strings.TrimSpace(rule.Severity),
				Status:         OpsAlertStatusFiring,
				Title:          buildOpsAlertTitle(rule),
				Description:    eventDescription,
				MetricValue:    float64Ptr(metricValue),
				ThresholdValue: float64Ptr(rule.Threshold),
				Dimensions:     buildOpsAlertDimensions(scopePlatform, scopeGroupID),
				FiredAt:        now,
				CreatedAt:      now,
			}

			created, err := s.opsRepo.CreateAlertEvent(ctx, firedEvent)
			if err != nil {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] create event failed (rule=%d): %v", rule.ID, err)
				continue
			}

			eventsCreated++
			if created != nil && created.ID > 0 {
				if s.maybeSendAlertEmail(ctx, runtimeCfg, rule, created) {
					emailsSent++
				}
				// Telegram notification
				if s.telegramNotifySvc != nil {
					metricVal := float64(0)
					if created.MetricValue != nil {
						metricVal = *created.MetricValue
					}
					go s.telegramNotifySvc.NotifyOpsAlert(context.Background(), rule.Name, rule.Severity, created.Description, metricVal)
				}
			}
			continue
		}

		// Not breached: resolve active event if present.
		if activeEvent != nil {
			resolvedAt := now
			if err := s.opsRepo.UpdateAlertEventStatus(ctx, activeEvent.ID, OpsAlertStatusResolved, &resolvedAt); err != nil {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] resolve event failed (event=%d): %v", activeEvent.ID, err)
			} else {
				eventsResolved++
			}
		}
	}

	result := truncateString(fmt.Sprintf("rules=%d enabled=%d evaluated=%d created=%d resolved=%d emails_sent=%d", rulesTotal, rulesEnabled, rulesEvaluated, eventsCreated, eventsResolved, emailsSent), 2048)
	s.recordHeartbeatSuccess(runAt, time.Since(startedAt), result)
}

func (s *OpsAlertEvaluatorService) pruneRuleStates(rules []*OpsAlertRule) {
	s.mu.Lock()
	defer s.mu.Unlock()

	live := map[int64]struct{}{}
	for _, r := range rules {
		if r != nil && r.ID > 0 {
			live[r.ID] = struct{}{}
		}
	}
	for id := range s.ruleStates {
		if _, ok := live[id]; !ok {
			delete(s.ruleStates, id)
		}
	}
}

func (s *OpsAlertEvaluatorService) resetRuleState(ruleID int64, now time.Time) {
	if ruleID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.ruleStates[ruleID]
	if !ok {
		state = &opsAlertRuleState{}
		s.ruleStates[ruleID] = state
	}
	state.LastEvaluatedAt = now
	state.ConsecutiveBreaches = 0
}

func (s *OpsAlertEvaluatorService) updateRuleBreaches(ruleID int64, now time.Time, interval time.Duration, breached bool) int {
	if ruleID <= 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.ruleStates[ruleID]
	if !ok {
		state = &opsAlertRuleState{}
		s.ruleStates[ruleID] = state
	}

	if !state.LastEvaluatedAt.IsZero() && interval > 0 {
		if now.Sub(state.LastEvaluatedAt) > interval*2 {
			state.ConsecutiveBreaches = 0
		}
	}

	state.LastEvaluatedAt = now
	if breached {
		state.ConsecutiveBreaches++
	} else {
		state.ConsecutiveBreaches = 0
	}
	return state.ConsecutiveBreaches
}

func requiredSustainedBreaches(sustainedMinutes int, interval time.Duration) int {
	if sustainedMinutes <= 0 {
		return 1
	}
	if interval <= 0 {
		return sustainedMinutes
	}
	required := int(math.Ceil(float64(sustainedMinutes*60) / interval.Seconds()))
	if required < 1 {
		return 1
	}
	return required
}

func parseOpsAlertRuleScope(filters map[string]any) (platform string, groupID *int64, region *string) {
	if filters == nil {
		return "", nil, nil
	}
	if v, ok := filters["platform"]; ok {
		if s, ok := v.(string); ok {
			platform = strings.TrimSpace(s)
		}
	}
	if v, ok := filters["group_id"]; ok {
		switch t := v.(type) {
		case float64:
			if t > 0 {
				id := int64(t)
				groupID = &id
			}
		case int64:
			if t > 0 {
				id := t
				groupID = &id
			}
		case int:
			if t > 0 {
				id := int64(t)
				groupID = &id
			}
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
			if err == nil && n > 0 {
				groupID = &n
			}
		}
	}
	if v, ok := filters["region"]; ok {
		if s, ok := v.(string); ok {
			vv := strings.TrimSpace(s)
			if vv != "" {
				region = &vv
			}
		}
	}
	return platform, groupID, region
}

func (s *OpsAlertEvaluatorService) computeRuleMetric(
	ctx context.Context,
	rule *OpsAlertRule,
	systemMetrics *OpsSystemMetricsSnapshot,
	start time.Time,
	end time.Time,
	platform string,
	groupID *int64,
) (float64, bool) {
	if rule == nil {
		return 0, false
	}
	switch strings.TrimSpace(rule.MetricType) {
	case "cpu_usage_percent":
		if systemMetrics != nil && systemMetrics.CPUUsagePercent != nil {
			return *systemMetrics.CPUUsagePercent, true
		}
		return 0, false
	case "memory_usage_percent":
		if systemMetrics != nil && systemMetrics.MemoryUsagePercent != nil {
			return *systemMetrics.MemoryUsagePercent, true
		}
		return 0, false
	case "concurrency_queue_depth":
		if systemMetrics != nil && systemMetrics.ConcurrencyQueueDepth != nil {
			return float64(*systemMetrics.ConcurrencyQueueDepth), true
		}
		return 0, false
	case "group_available_accounts":
		if groupID == nil || *groupID <= 0 {
			return 0, false
		}
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		if availability.Group == nil {
			return 0, true
		}
		return float64(availability.Group.AvailableCount), true
	case "group_available_ratio":
		if groupID == nil || *groupID <= 0 {
			return 0, false
		}
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return computeGroupAvailableRatio(availability.Group), true
	case "account_rate_limited_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.IsRateLimited
		})), true
	case "account_error_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.HasError && acc.TempUnschedulableUntil == nil
		})), true
	case "account_temp_unscheduled_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		now := time.Now().UTC()
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.TempUnschedulableUntil != nil && now.Before(*acc.TempUnschedulableUntil)
		})), true
	case "group_rate_limit_ratio":
		if groupID == nil || *groupID <= 0 {
			return 0, false
		}
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		if availability.Group == nil || availability.Group.TotalAccounts <= 0 {
			return 0, true
		}
		return (float64(availability.Group.RateLimitCount) / float64(availability.Group.TotalAccounts)) * 100, true
	case "account_error_ratio":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		total := int64(len(availability.Accounts))
		if total <= 0 {
			return 0, true
		}
		errorCount := countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.HasError && acc.TempUnschedulableUntil == nil
		})
		return (float64(errorCount) / float64(total)) * 100, true
	case "overload_account_count":
		if s == nil || s.opsService == nil {
			return 0, false
		}
		availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
		if err != nil || availability == nil {
			return 0, false
		}
		return float64(countAccountsByCondition(availability.Accounts, func(acc *AccountAvailability) bool {
			return acc.IsOverloaded
		})), true
	case "proxy_expired_count":
		if s == nil || s.proxyRepo == nil {
			return 0, false
		}
		n, err := s.proxyRepo.CountExpired(ctx)
		if err != nil {
			return 0, false
		}
		return float64(n), true
	case "proxy_expiring_soon_count":
		if s == nil || s.proxyRepo == nil {
			return 0, false
		}
		n, err := s.proxyRepo.CountExpiringSoon(ctx, time.Now())
		if err != nil {
			return 0, false
		}
		return float64(n), true
	case "account_available_count":
		accounts, ok := s.listAccountsForAlertMetric(ctx, platform, groupID)
		if !ok {
			return 0, false
		}
		return float64(countAccounts(accounts, func(acc Account) bool { return acc.IsSchedulable() })), true
	case "account_available_ratio":
		accounts, ok := s.listAccountsForAlertMetric(ctx, platform, groupID)
		if !ok {
			return 0, false
		}
		if len(accounts) == 0 {
			return 0, true
		}
		available := countAccounts(accounts, func(acc Account) bool { return acc.IsSchedulable() })
		return (float64(available) / float64(len(accounts))) * 100, true
	case "account_quota_usage_ratio":
		accounts, ok := s.listAccountsForAlertMetric(ctx, platform, groupID)
		if !ok {
			return 0, false
		}
		return maxAccountQuotaUsagePercent(accounts), true
	case "account_quota_exhausted_count":
		accounts, ok := s.listAccountsForAlertMetric(ctx, platform, groupID)
		if !ok {
			return 0, false
		}
		return float64(countAccounts(accounts, func(acc Account) bool {
			return acc.IsAPIKeyOrBedrock() && acc.IsQuotaExceeded()
		})), true
	}

	overview, err := s.opsRepo.GetDashboardOverview(ctx, &OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  platform,
		GroupID:   groupID,
		QueryMode: OpsQueryModeRaw,
	})
	if err != nil {
		return 0, false
	}
	if overview == nil {
		return 0, false
	}

	switch strings.TrimSpace(rule.MetricType) {
	case "success_rate":
		if overview.RequestCountSLA <= 0 {
			return 0, false
		}
		return overview.SLA * 100, true
	case "error_rate":
		if overview.RequestCountSLA <= 0 {
			return 0, false
		}
		return overview.ErrorRate * 100, true
	case "upstream_error_rate":
		if overview.RequestCountSLA <= 0 {
			return 0, false
		}
		return overview.UpstreamErrorRate * 100, true
	case "p95_latency_ms":
		if overview.Duration.P95 == nil {
			return 0, false
		}
		return float64(*overview.Duration.P95), true
	case "p99_latency_ms":
		if overview.Duration.P99 == nil {
			return 0, false
		}
		return float64(*overview.Duration.P99), true
	default:
		return 0, false
	}
}

func compareMetric(value float64, operator string, threshold float64) bool {
	switch strings.TrimSpace(operator) {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	default:
		return false
	}
}

func buildOpsAlertDimensions(platform string, groupID *int64) map[string]any {
	dims := map[string]any{}
	if strings.TrimSpace(platform) != "" {
		dims["platform"] = strings.TrimSpace(platform)
	}
	if groupID != nil && *groupID > 0 {
		dims["group_id"] = *groupID
	}
	if len(dims) == 0 {
		return nil
	}
	return dims
}

func buildOpsAlertDescription(rule *OpsAlertRule, value float64, windowMinutes int, platform string, groupID *int64) string {
	if rule == nil {
		return ""
	}
	scope := "overall"
	if strings.TrimSpace(platform) != "" {
		scope = fmt.Sprintf("platform=%s", strings.TrimSpace(platform))
	}
	if groupID != nil && *groupID > 0 {
		scope = fmt.Sprintf("%s group_id=%d", scope, *groupID)
	}
	if windowMinutes <= 0 {
		windowMinutes = 1
	}
	return fmt.Sprintf("%s %s %.2f (current %.2f) over last %dm (%s)",
		strings.TrimSpace(rule.MetricType),
		strings.TrimSpace(rule.Operator),
		rule.Threshold,
		value,
		windowMinutes,
		strings.TrimSpace(scope),
	)
}

func (s *OpsAlertEvaluatorService) buildOpsAlertAccountContext(ctx context.Context, rule *OpsAlertRule, start, end time.Time, platform string, groupID *int64) string {
	if s == nil || rule == nil || s.opsRepo == nil {
		return ""
	}
	metricType := strings.TrimSpace(rule.MetricType)
	if opsAlertMetricNeedsAvailabilityBreakdown(metricType) {
		return s.buildOpsAlertAvailabilityContext(ctx, metricType, platform, groupID)
	}
	if !opsAlertMetricNeedsRequestAccountBreakdown(metricType) {
		return ""
	}
	repo, ok := s.opsRepo.(opsAlertAccountBreakdownRepository)
	if !ok {
		return ""
	}
	items, err := repo.ListAlertAccountBreakdown(ctx, &OpsAlertAccountBreakdownFilter{
		StartTime:  start,
		EndTime:    end,
		Platform:   platform,
		GroupID:    groupID,
		MetricType: metricType,
		Limit:      5,
	})
	if err != nil {
		logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] list alert account breakdown failed (metric=%s): %v", metricType, err)
		return ""
	}
	return formatOpsAlertAccountContext(metricType, items)
}

func (s *OpsAlertEvaluatorService) buildOpsAlertAvailabilityContext(ctx context.Context, metricType string, platform string, groupID *int64) string {
	if s == nil || s.opsService == nil {
		return ""
	}
	availability, err := s.opsService.GetAccountAvailability(ctx, platform, groupID)
	if err != nil || availability == nil {
		if err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] get account availability failed (metric=%s): %v", metricType, err)
		}
		return ""
	}
	return formatOpsAlertAvailabilityContext(metricType, availability)
}

func opsAlertMetricNeedsAvailabilityBreakdown(metricType string) bool {
	switch strings.TrimSpace(metricType) {
	case "account_rate_limited_count", "account_error_count", "account_error_ratio", "account_temp_unscheduled_count", "overload_account_count", "account_available_count", "account_available_ratio":
		return true
	default:
		return false
	}
}

func opsAlertMetricNeedsRequestAccountBreakdown(metricType string) bool {
	switch strings.TrimSpace(metricType) {
	case "success_rate", "error_rate", "upstream_error_rate", "p95_latency_ms", "p99_latency_ms":
		return true
	default:
		return false
	}
}

func appendOpsAlertAccountContext(description, accountContext string) string {
	description = strings.TrimSpace(description)
	accountContext = strings.TrimSpace(accountContext)
	if accountContext == "" {
		return description
	}
	if description == "" {
		return accountContext
	}
	return description + "\n\n" + accountContext
}

func formatOpsAlertAvailabilityContext(metricType string, availability *OpsAccountAvailability) string {
	if availability == nil || len(availability.Accounts) == 0 {
		return "Tài khoản cần kiểm tra: không có account trong phạm vi rule."
	}
	accounts := make([]*AccountAvailability, 0, len(availability.Accounts))
	for _, acc := range availability.Accounts {
		if acc == nil || !opsAlertAvailabilityAccountMatches(metricType, acc) {
			continue
		}
		accounts = append(accounts, acc)
	}
	if len(accounts) == 0 {
		return "Tài khoản cần kiểm tra: không tìm thấy account khớp trạng thái cảnh báo trong snapshot hiện tại."
	}
	sort.SliceStable(accounts, func(i, j int) bool {
		return accounts[i].AccountID < accounts[j].AccountID
	})
	if len(accounts) > 5 {
		accounts = accounts[:5]
	}

	var b strings.Builder
	_, _ = b.WriteString("Tài khoản cần kiểm tra:")
	for i, acc := range accounts {
		_, _ = b.WriteString("\n")
		fmt.Fprintf(&b, "%d) %s", i+1, formatOpsAlertAvailabilityAccountLabel(acc))
		reasons := opsAlertAvailabilityReasons(acc)
		if len(reasons) > 0 {
			fmt.Fprintf(&b, " — %s", strings.Join(reasons, ", "))
		}
		if strings.TrimSpace(acc.ErrorMessage) != "" {
			fmt.Fprintf(&b, ", lỗi: %s", truncateString(strings.TrimSpace(acc.ErrorMessage), 80))
		}
	}
	return b.String()
}

func opsAlertAvailabilityAccountMatches(metricType string, acc *AccountAvailability) bool {
	if acc == nil {
		return false
	}
	switch strings.TrimSpace(metricType) {
	case "account_rate_limited_count":
		return acc.IsRateLimited
	case "account_error_count", "account_error_ratio":
		return acc.HasError && acc.TempUnschedulableUntil == nil
	case "account_temp_unscheduled_count":
		return acc.TempUnschedulableUntil != nil && time.Now().UTC().Before(*acc.TempUnschedulableUntil)
	case "overload_account_count":
		return acc.IsOverloaded
	case "account_available_count", "account_available_ratio":
		return !acc.IsAvailable
	default:
		return false
	}
}

func opsAlertAvailabilityReasons(acc *AccountAvailability) []string {
	if acc == nil {
		return nil
	}
	reasons := make([]string, 0, 4)
	if strings.TrimSpace(acc.Status) != "" {
		reasons = append(reasons, "status="+strings.TrimSpace(acc.Status))
	}
	if acc.HasError {
		reasons = append(reasons, "has_error")
	}
	if acc.IsRateLimited {
		reason := "rate_limited"
		if acc.RateLimitRemainingSec != nil && *acc.RateLimitRemainingSec > 0 {
			reason = fmt.Sprintf("%s còn %ds", reason, *acc.RateLimitRemainingSec)
		}
		reasons = append(reasons, reason)
	}
	if acc.IsOverloaded {
		reason := "overloaded"
		if acc.OverloadRemainingSec != nil && *acc.OverloadRemainingSec > 0 {
			reason = fmt.Sprintf("%s còn %ds", reason, *acc.OverloadRemainingSec)
		}
		reasons = append(reasons, reason)
	}
	if acc.TempUnschedulableUntil != nil && time.Now().UTC().Before(*acc.TempUnschedulableUntil) {
		reasons = append(reasons, "temp_unscheduled đến "+acc.TempUnschedulableUntil.UTC().Format(time.RFC3339))
	}
	if !acc.IsAvailable {
		reasons = append(reasons, "unavailable")
	}
	return reasons
}

func formatOpsAlertAvailabilityAccountLabel(acc *AccountAvailability) string {
	if acc == nil {
		return "Không rõ account"
	}
	name := strings.TrimSpace(acc.AccountName)
	if name == "" {
		name = "Không rõ account"
	}
	if acc.AccountID > 0 {
		name = fmt.Sprintf("%s (#%d)", name, acc.AccountID)
	}
	scope := make([]string, 0, 2)
	if platform := strings.TrimSpace(acc.Platform); platform != "" {
		scope = append(scope, platform)
	}
	groupLabel := strings.TrimSpace(acc.GroupName)
	if acc.GroupID > 0 {
		if groupLabel != "" {
			groupLabel = fmt.Sprintf("%s #%d", groupLabel, acc.GroupID)
		} else {
			groupLabel = fmt.Sprintf("group #%d", acc.GroupID)
		}
	}
	if groupLabel != "" {
		scope = append(scope, groupLabel)
	}
	if len(scope) > 0 {
		name = fmt.Sprintf("%s [%s]", name, strings.Join(scope, ", "))
	}
	return name
}

func formatOpsAlertAccountContext(metricType string, items []*OpsAlertAccountBreakdown) string {
	metricType = strings.TrimSpace(metricType)
	if len(items) == 0 {
		return "Tài khoản cần kiểm tra: không tìm thấy account_id trong log của window này (có thể lỗi xảy ra trước khi chọn account, ở auth, hoặc runtime nội bộ)."
	}

	var b strings.Builder
	if metricType == "p95_latency_ms" || metricType == "p99_latency_ms" {
		_, _ = b.WriteString("Tài khoản độ trễ cao cần kiểm tra:")
	} else {
		_, _ = b.WriteString("Tài khoản lỗi cần kiểm tra:")
	}
	for i, item := range items {
		if item == nil {
			continue
		}
		_, _ = b.WriteString("\n")
		fmt.Fprintf(&b, "%d) %s — ", i+1, formatOpsAlertAccountLabel(item))
		if metricType == "p95_latency_ms" || metricType == "p99_latency_ms" {
			fmt.Fprintf(&b, "success %d req, p95 %s, p99 %s, avg %s, max %s",
				item.SuccessCount,
				formatOpsAlertMs(item.DurationP95Ms),
				formatOpsAlertMs(item.DurationP99Ms),
				formatOpsAlertMs(item.DurationAvgMs),
				formatOpsAlertMs(item.DurationMaxMs),
			)
			if item.ErrorCountSLA > 0 {
				fmt.Fprintf(&b, ", lỗi SLA %d", item.ErrorCountSLA)
			}
		} else {
			fmt.Fprintf(&b, "lỗi SLA %d/%d (%.2f%%), success %d",
				item.ErrorCountSLA,
				item.RequestCountSLA,
				item.ErrorRate,
				item.SuccessCount,
			)
			if item.UpstreamErrorCount > 0 {
				fmt.Fprintf(&b, ", upstream %d", item.UpstreamErrorCount)
			}
			if item.BusinessLimitedCount > 0 {
				fmt.Fprintf(&b, ", business-limited %d", item.BusinessLimitedCount)
			}
		}
		if item.LastErrorStatusCode != nil || strings.TrimSpace(item.LastErrorType) != "" || strings.TrimSpace(item.LastErrorMessage) != "" {
			_, _ = b.WriteString(", lỗi gần nhất: ")
			parts := make([]string, 0, 3)
			if item.LastErrorStatusCode != nil {
				parts = append(parts, fmt.Sprintf("HTTP %d", *item.LastErrorStatusCode))
			}
			if strings.TrimSpace(item.LastErrorType) != "" {
				parts = append(parts, strings.TrimSpace(item.LastErrorType))
			}
			if strings.TrimSpace(item.LastErrorMessage) != "" {
				parts = append(parts, truncateString(strings.TrimSpace(item.LastErrorMessage), 80))
			}
			_, _ = b.WriteString(strings.Join(parts, " | "))
		}
	}
	return b.String()
}

func formatOpsAlertAccountLabel(item *OpsAlertAccountBreakdown) string {
	if item == nil {
		return "Không rõ account"
	}
	name := strings.TrimSpace(item.AccountName)
	if name == "" {
		name = "Không rõ account"
	}
	if item.AccountID != nil && *item.AccountID > 0 {
		name = fmt.Sprintf("%s (#%d)", name, *item.AccountID)
	}
	scope := make([]string, 0, 2)
	if platform := strings.TrimSpace(item.Platform); platform != "" {
		scope = append(scope, platform)
	}
	groupLabel := strings.TrimSpace(item.GroupName)
	if item.GroupID != nil && *item.GroupID > 0 {
		if groupLabel != "" {
			groupLabel = fmt.Sprintf("%s #%d", groupLabel, *item.GroupID)
		} else {
			groupLabel = fmt.Sprintf("group #%d", *item.GroupID)
		}
	}
	if groupLabel != "" {
		scope = append(scope, groupLabel)
	}
	if len(scope) > 0 {
		name = fmt.Sprintf("%s [%s]", name, strings.Join(scope, ", "))
	}
	return name
}

func formatOpsAlertMs(value *int) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%dms", *value)
}

func (s *OpsAlertEvaluatorService) maybeSendAlertEmail(ctx context.Context, runtimeCfg *OpsAlertRuntimeSettings, rule *OpsAlertRule, event *OpsAlertEvent) bool {
	if s == nil || s.emailService == nil || s.opsService == nil || event == nil || rule == nil {
		return false
	}
	if event.EmailSent {
		return false
	}
	if !rule.NotifyEmail {
		return false
	}

	emailCfg, err := s.opsService.GetEmailNotificationConfig(ctx)
	if err != nil || emailCfg == nil || !emailCfg.Alert.Enabled {
		return false
	}

	if len(emailCfg.Alert.Recipients) == 0 {
		return false
	}
	if !shouldSendOpsAlertEmailByMinSeverity(strings.TrimSpace(emailCfg.Alert.MinSeverity), strings.TrimSpace(rule.Severity)) {
		return false
	}

	if runtimeCfg != nil && runtimeCfg.Silencing.Enabled {
		if isOpsAlertSilenced(time.Now().UTC(), rule, event, runtimeCfg.Silencing) {
			return false
		}
	}

	// Apply/update rate limiter.
	s.emailLimiter.SetLimit(emailCfg.Alert.RateLimitPerHour)

	subject := fmt.Sprintf("[Ops Alert][%s] %s", strings.TrimSpace(rule.Severity), strings.TrimSpace(rule.Name))
	body := buildOpsAlertEmailBody(rule, event)

	anySent := false
	for _, to := range emailCfg.Alert.Recipients {
		addr := strings.TrimSpace(to)
		if addr == "" {
			continue
		}
		if !s.emailLimiter.Allow(time.Now().UTC()) {
			continue
		}
		if s.emailService.notificationEmailService != nil {
			if err := s.emailService.notificationEmailService.Send(ctx, NotificationEmailSendInput{
				Event:          NotificationEmailEventOpsAlert,
				RecipientEmail: addr,
				RecipientName:  emailRecipientName(addr),
				SourceType:     "ops_alert",
				SourceID:       fmt.Sprintf("%d", event.ID),
				Variables:      opsAlertEmailVariables(rule, event),
			}); err == nil {
				anySent = true
				continue
			} else if !shouldFallbackNotificationEmail(err) {
				continue
			}
		}
		if err := s.emailService.SendEmail(ctx, addr, subject, body); err != nil {
			// Ignore per-recipient failures; continue best-effort.
			continue
		}
		anySent = true
	}

	if anySent {
		_ = s.opsRepo.UpdateAlertEventEmailSent(context.Background(), event.ID, true)
	}
	return anySent
}

func opsAlertEmailVariables(rule *OpsAlertRule, event *OpsAlertEvent) map[string]string {
	variables := map[string]string{
		"rule_name":         "-",
		"severity":          "-",
		"alert_status":      "-",
		"metric_type":       "-",
		"operator":          "-",
		"metric_value":      "-",
		"threshold_value":   "-",
		"triggered_at":      time.Now().UTC().Format(time.RFC3339),
		"alert_description": "-",
	}
	if rule != nil {
		variables["rule_name"] = strings.TrimSpace(rule.Name)
		variables["severity"] = strings.TrimSpace(rule.Severity)
		variables["metric_type"] = strings.TrimSpace(rule.MetricType)
		variables["operator"] = strings.TrimSpace(rule.Operator)
		variables["threshold_value"] = fmt.Sprintf("%.2f", rule.Threshold)
		if strings.TrimSpace(rule.Description) != "" {
			variables["alert_description"] = strings.TrimSpace(rule.Description)
		}
	}
	if event != nil {
		variables["alert_status"] = strings.TrimSpace(event.Status)
		if event.MetricValue != nil {
			variables["metric_value"] = fmt.Sprintf("%.2f", *event.MetricValue)
		}
		if event.ThresholdValue != nil {
			variables["threshold_value"] = fmt.Sprintf("%.2f", *event.ThresholdValue)
		}
		if !event.FiredAt.IsZero() {
			variables["triggered_at"] = event.FiredAt.UTC().Format(time.RFC3339)
		}
		if strings.TrimSpace(event.Description) != "" {
			variables["alert_description"] = strings.TrimSpace(event.Description)
		}
	}
	return variables
}

func buildOpsAlertEmailBody(rule *OpsAlertRule, event *OpsAlertEvent) string {
	if rule == nil || event == nil {
		return ""
	}
	metric := strings.TrimSpace(rule.MetricType)
	value := "-"
	threshold := fmt.Sprintf("%.2f", rule.Threshold)
	if event.MetricValue != nil {
		value = fmt.Sprintf("%.2f", *event.MetricValue)
	}
	if event.ThresholdValue != nil {
		threshold = fmt.Sprintf("%.2f", *event.ThresholdValue)
	}
	return fmt.Sprintf(`
<h2>Ops Alert</h2>
<p><b>Rule</b>: %s</p>
<p><b>Severity</b>: %s</p>
<p><b>Status</b>: %s</p>
<p><b>Metric</b>: %s %s %s</p>
<p><b>Fired at</b>: %s</p>
<p><b>Description</b>: %s</p>
`,
		htmlEscape(rule.Name),
		htmlEscape(rule.Severity),
		htmlEscape(event.Status),
		htmlEscape(metric),
		htmlEscape(rule.Operator),
		htmlEscape(fmt.Sprintf("%s (threshold %s)", value, threshold)),
		event.FiredAt.Format(time.RFC3339),
		htmlEscape(event.Description),
	)
}

func shouldSendOpsAlertEmailByMinSeverity(minSeverity string, ruleSeverity string) bool {
	minSeverity = strings.ToLower(strings.TrimSpace(minSeverity))
	if minSeverity == "" {
		return true
	}

	eventLevel := opsEmailSeverityForOps(ruleSeverity)
	minLevel := strings.ToLower(minSeverity)

	rank := func(level string) int {
		switch level {
		case "critical":
			return 3
		case "warning":
			return 2
		case "info":
			return 1
		default:
			return 0
		}
	}
	return rank(eventLevel) >= rank(minLevel)
}

func opsEmailSeverityForOps(severity string) string {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "P0":
		return "critical"
	case "P1":
		return "warning"
	default:
		return "info"
	}
}

func isOpsAlertSilenced(now time.Time, rule *OpsAlertRule, event *OpsAlertEvent, silencing OpsAlertSilencingSettings) bool {
	if !silencing.Enabled {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(silencing.GlobalUntilRFC3339) != "" {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(silencing.GlobalUntilRFC3339)); err == nil {
			if now.Before(t) {
				return true
			}
		}
	}

	for _, entry := range silencing.Entries {
		untilRaw := strings.TrimSpace(entry.UntilRFC3339)
		if untilRaw == "" {
			continue
		}
		until, err := time.Parse(time.RFC3339, untilRaw)
		if err != nil {
			continue
		}
		if now.After(until) {
			continue
		}
		if entry.RuleID != nil && rule != nil && rule.ID > 0 && *entry.RuleID != rule.ID {
			continue
		}
		if len(entry.Severities) > 0 {
			match := false
			for _, s := range entry.Severities {
				if strings.EqualFold(strings.TrimSpace(s), strings.TrimSpace(event.Severity)) || strings.EqualFold(strings.TrimSpace(s), strings.TrimSpace(rule.Severity)) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		return true
	}

	return false
}

func (s *OpsAlertEvaluatorService) tryAcquireLeaderLock(ctx context.Context, lock OpsDistributedLockSettings) (func(), bool) {
	if !lock.Enabled {
		return nil, true
	}
	if s.redisClient == nil {
		s.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] redis not configured; running without distributed lock")
		})
		return nil, true
	}
	key := strings.TrimSpace(lock.Key)
	if key == "" {
		key = opsAlertEvaluatorLeaderLockKey
	}
	ttl := time.Duration(lock.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = opsAlertEvaluatorLeaderLockTTL
	}

	ok, err := s.redisClient.SetNX(ctx, key, s.instanceID, ttl).Result()
	if err != nil {
		// Prefer fail-closed to avoid duplicate evaluators stampeding the DB when Redis is flaky.
		// Single-node deployments can disable the distributed lock via runtime settings.
		s.warnNoRedisOnce.Do(func() {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] leader lock SetNX failed; skipping this cycle: %v", err)
		})
		return nil, false
	}
	if !ok {
		s.maybeLogSkip(key)
		return nil, false
	}
	return func() {
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer releaseCancel()
		_, _ = opsAlertEvaluatorReleaseScript.Run(releaseCtx, s.redisClient, []string{key}, s.instanceID).Result()
	}, true
}

func (s *OpsAlertEvaluatorService) maybeLogSkip(key string) {
	s.skipLogMu.Lock()
	defer s.skipLogMu.Unlock()

	now := time.Now()
	if !s.skipLogAt.IsZero() && now.Sub(s.skipLogAt) < opsAlertEvaluatorSkipLogInterval {
		return
	}
	s.skipLogAt = now
	logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] leader lock held by another instance; skipping (key=%q)", key)
}

func (s *OpsAlertEvaluatorService) recordHeartbeatSuccess(runAt time.Time, duration time.Duration, result string) {
	if s == nil || s.opsRepo == nil {
		return
	}
	now := time.Now().UTC()
	durMs := duration.Milliseconds()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msg := strings.TrimSpace(result)
	if msg == "" {
		msg = "ok"
	}
	msg = truncateString(msg, 2048)
	_ = s.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsAlertEvaluatorJobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &now,
		LastDurationMs: &durMs,
		LastResult:     &msg,
	})
}

func (s *OpsAlertEvaluatorService) recordHeartbeatError(runAt time.Time, duration time.Duration, err error) {
	if s == nil || s.opsRepo == nil || err == nil {
		return
	}
	now := time.Now().UTC()
	durMs := duration.Milliseconds()
	msg := truncateString(err.Error(), 2048)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsAlertEvaluatorJobName,
		LastRunAt:      &runAt,
		LastErrorAt:    &now,
		LastError:      &msg,
		LastDurationMs: &durMs,
	})
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}

type slidingWindowLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	sent   []time.Time
}

func newSlidingWindowLimiter(limit int, window time.Duration) *slidingWindowLimiter {
	if window <= 0 {
		window = time.Hour
	}
	return &slidingWindowLimiter{
		limit:  limit,
		window: window,
		sent:   []time.Time{},
	}
}

func (l *slidingWindowLimiter) SetLimit(limit int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limit = limit
}

func (l *slidingWindowLimiter) Allow(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.limit <= 0 {
		return true
	}
	cutoff := now.Add(-l.window)
	keep := l.sent[:0]
	for _, t := range l.sent {
		if t.After(cutoff) {
			keep = append(keep, t)
		}
	}
	l.sent = keep
	if len(l.sent) >= l.limit {
		return false
	}
	l.sent = append(l.sent, now)
	return true
}

func (s *OpsAlertEvaluatorService) listAccountsForAlertMetric(ctx context.Context, platform string, groupID *int64) ([]Account, bool) {
	if s == nil || s.opsService == nil {
		return nil, false
	}
	accounts, err := s.opsService.listAllAccountsForOps(ctx, platform, groupID)
	if err != nil {
		return nil, false
	}
	if groupID == nil || *groupID <= 0 {
		return accounts, true
	}
	filtered := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if opsAlertAccountBelongsToGroup(account, *groupID) {
			filtered = append(filtered, account)
		}
	}
	return filtered, true
}

func countAccounts(accounts []Account, condition func(Account) bool) int64 {
	if len(accounts) == 0 || condition == nil {
		return 0
	}
	var count int64
	for _, account := range accounts {
		if condition(account) {
			count++
		}
	}
	return count
}

func opsAlertAccountBelongsToGroup(account Account, groupID int64) bool {
	if groupID <= 0 {
		return true
	}
	for _, id := range account.GroupIDs {
		if id == groupID {
			return true
		}
	}
	for _, group := range account.Groups {
		if group != nil && group.ID == groupID {
			return true
		}
	}
	for _, accountGroup := range account.AccountGroups {
		if accountGroup.GroupID == groupID {
			return true
		}
	}
	return false
}

func maxAccountQuotaUsagePercent(accounts []Account) float64 {
	var maxUsage float64
	for _, account := range accounts {
		if !account.IsAPIKeyOrBedrock() {
			continue
		}
		if percent, ok := accountMaxQuotaUsagePercent(account); ok && percent > maxUsage {
			maxUsage = percent
		}
	}
	return maxUsage
}

func accountMaxQuotaUsagePercent(account Account) (float64, bool) {
	maxUsage := 0.0
	hasLimit := false
	if limit := account.GetQuotaLimit(); limit > 0 {
		hasLimit = true
		maxUsage = math.Max(maxUsage, quotaUsagePercent(account.GetQuotaUsed(), limit))
	}
	if limit := account.GetQuotaDailyLimit(); limit > 0 {
		hasLimit = true
		used := account.GetQuotaDailyUsed()
		if account.IsDailyQuotaPeriodExpired() {
			used = 0
		}
		maxUsage = math.Max(maxUsage, quotaUsagePercent(used, limit))
	}
	if limit := account.GetQuotaWeeklyLimit(); limit > 0 {
		hasLimit = true
		used := account.GetQuotaWeeklyUsed()
		if account.IsWeeklyQuotaPeriodExpired() {
			used = 0
		}
		maxUsage = math.Max(maxUsage, quotaUsagePercent(used, limit))
	}
	return maxUsage, hasLimit
}

func quotaUsagePercent(used, limit float64) float64 {
	if limit <= 0 || used <= 0 {
		return 0
	}
	return (used / limit) * 100
}

// computeGroupAvailableRatio returns the available percentage for a group.
// Formula: (AvailableCount / TotalAccounts) * 100.
// Returns 0 when TotalAccounts is 0.
func computeGroupAvailableRatio(group *GroupAvailability) float64 {
	if group == nil || group.TotalAccounts <= 0 {
		return 0
	}
	return (float64(group.AvailableCount) / float64(group.TotalAccounts)) * 100
}

// countAccountsByCondition counts accounts that satisfy the given condition.
func countAccountsByCondition(accounts map[int64]*AccountAvailability, condition func(*AccountAvailability) bool) int64 {
	if len(accounts) == 0 || condition == nil {
		return 0
	}
	var count int64
	for _, account := range accounts {
		if account != nil && condition(account) {
			count++
		}
	}
	return count
}
