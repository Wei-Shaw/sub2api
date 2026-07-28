package service

import (
	"context"
	"errors"
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
	opsAlertEvaluatorJobName        = "ops_alert_evaluator"
	opsAlertEmailTransitionFiring   = "firing"
	opsAlertEmailTransitionResolved = "resolved"

	opsAlertEvaluatorTimeout         = 45 * time.Second
	opsAlertEvaluatorLeaderLockKey   = "ops:alert:evaluator:leader"
	opsAlertEvaluatorLeaderLockTTL   = 90 * time.Second
	opsAlertEvaluatorSkipLogInterval = 1 * time.Minute
	opsAlertSystemMetricsMaxAge      = 3 * time.Minute
	opsAlertEvaluatorVersion         = "v2"
)

var opsAlertEvaluatorReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type OpsAlertEvaluatorService struct {
	opsService             *OpsService
	opsRepo                OpsRepository
	notificationDispatcher *NotificationEmailDispatcher
	proxyRepo              ProxyRepository

	redisClient *redis.Client
	cfg         *config.Config
	instanceID  string

	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup

	skipLogMu sync.Mutex
	skipLogAt time.Time

	warnNoRedisOnce sync.Once
}

func NewOpsAlertEvaluatorService(
	opsService *OpsService,
	opsRepo OpsRepository,
	notificationDispatcher *NotificationEmailDispatcher,
	redisClient *redis.Client,
	cfg *config.Config,
	proxyRepo ProxyRepository,
) *OpsAlertEvaluatorService {
	return &OpsAlertEvaluatorService{
		opsService:             opsService,
		opsRepo:                opsRepo,
		notificationDispatcher: notificationDispatcher,
		proxyRepo:              proxyRepo,
		redisClient:            redisClient,
		cfg:                    cfg,
		instanceID:             uuid.NewString(),
	}
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
	ruleByID := make(map[int64]*OpsAlertRule, len(rules))
	for _, rule := range rules {
		if rule != nil && rule.ID > 0 {
			ruleByID[rule.ID] = rule
		}
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i] == nil {
			return false
		}
		if rules[j] == nil {
			return true
		}
		return opsAlertSeverityRank(rules[i].Severity) < opsAlertSeverityRank(rules[j].Severity)
	})
	activeIncidents := map[string]*OpsAlertEvent{}
	if activeEvents, listErr := s.opsRepo.ListAlertEvents(ctx, &OpsAlertEventFilter{Limit: 500, Status: OpsAlertStatusFiring}); listErr == nil {
		for _, event := range activeEvents {
			if event == nil {
				continue
			}
			rule := ruleByID[event.RuleID]
			key := opsAlertIncidentKeyFromEvent(rule, event)
			if key == "" {
				continue
			}
			if current := activeIncidents[key]; current == nil || opsAlertSeverityRank(event.Severity) < opsAlertSeverityRank(current.Severity) {
				activeIncidents[key] = event
			}
		}
	} else {
		logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] list active incidents failed: %v", listErr)
	}

	rulesTotal := len(rules)
	rulesEnabled := 0
	rulesEvaluated := 0
	eventsCreated := 0
	eventsResolved := 0
	emailsQueued := 0
	evaluationStatuses := map[string]int{}

	now := time.Now().UTC()
	safeEnd := now.Truncate(time.Minute)
	if safeEnd.IsZero() {
		safeEnd = now
	}

	systemMetrics, _ := s.opsRepo.GetLatestSystemMetrics(ctx, 1)

	for _, rule := range rules {
		if rule == nil || !rule.Enabled || rule.ID <= 0 {
			continue
		}
		rulesEnabled++

		scopePlatform, scopeGroupID, scopeRegion := parseOpsAlertRuleScope(rule.Filters)
		incidentKey := opsAlertIncidentKey(rule, scopePlatform, scopeGroupID, scopeRegion)

		windowMinutes := rule.WindowMinutes
		if windowMinutes <= 0 {
			windowMinutes = 1
		}
		windowStart := safeEnd.Add(-time.Duration(windowMinutes) * time.Minute)
		windowEnd := safeEnd

		metric := s.evaluateRuleMetric(ctx, rule, systemMetrics, windowStart, windowEnd, scopePlatform, scopeGroupID, now)
		evaluation := &OpsAlertRuleEvaluation{
			RuleID: rule.ID, EvaluatedAt: now, WindowStart: windowStart, WindowEnd: windowEnd,
			Status: metric.Status, Breached: metric.Breached, MetricValue: metric.Value,
			ThresholdValue: float64Ptr(rule.Threshold), SampleCount: metric.SampleCount,
			BadCount: metric.BadCount, DataAsOf: metric.DataAsOf,
			ErrorCode: metric.ErrorCode, ErrorMessage: truncateString(metric.ErrorMessage, 1024),
			EvaluatorVersion: opsAlertEvaluatorVersion,
		}
		if rule.ShadowMode && (metric.Status == OpsAlertEvaluationStatusOK || metric.Status == OpsAlertEvaluationStatusBreached) {
			evaluation.Status = OpsAlertEvaluationStatusShadow
		}
		if err := s.opsRepo.InsertAlertRuleEvaluation(ctx, evaluation); err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] persist evaluation failed (rule=%d): %v", rule.ID, err)
			continue
		}
		rulesEvaluated++
		evaluationStatuses[evaluation.Status]++

		state, err := s.loadAlertRuleState(ctx, rule.ID)
		if err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] load rule state failed (rule=%d): %v", rule.ID, err)
			continue
		}
		resetAlertRuleStateAfterGap(state, now, interval)
		state.LastEvaluatedAt = opsAlertTimePtr(now)

		if metric.Status != OpsAlertEvaluationStatusOK && metric.Status != OpsAlertEvaluationStatusBreached {
			state.ConsecutiveBreaches = 0
			state.ConsecutiveRecoveries = 0
			if err := s.opsRepo.UpsertAlertRuleState(ctx, state); err != nil {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] reset unavailable rule state failed (rule=%d): %v", rule.ID, err)
			}
			continue
		}

		metricValue := 0.0
		if metric.Value != nil {
			metricValue = *metric.Value
		}
		breachedNow := metric.Breached

		activeEvent, err := s.opsRepo.GetActiveAlertEvent(ctx, rule.ID)
		if err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] get active event failed (rule=%d): %v", rule.ID, err)
			continue
		}

		if activeEvent != nil {
			if familyEvent := activeIncidents[incidentKey]; familyEvent != nil && familyEvent.ID != activeEvent.ID &&
				opsAlertSeverityRank(familyEvent.Severity) <= opsAlertSeverityRank(activeEvent.Severity) {
				resolvedAt := now
				if err := s.opsRepo.UpdateAlertEventStatus(ctx, activeEvent.ID, OpsAlertStatusResolved, &resolvedAt); err == nil {
					eventsResolved++
					state.ConsecutiveBreaches = 0
					state.ConsecutiveRecoveries = 0
					_ = s.opsRepo.UpsertAlertRuleState(ctx, state)
				}
				continue
			}
			recoveredNow := !breachedNow
			if rule.RecoveryThreshold != nil && strings.TrimSpace(rule.RecoveryOperator) != "" {
				recoveredNow = compareMetric(metricValue, rule.RecoveryOperator, *rule.RecoveryThreshold)
			}
			if recoveredNow {
				state.ConsecutiveRecoveries++
			} else {
				state.ConsecutiveRecoveries = 0
			}
			state.ConsecutiveBreaches = 0
			if err := s.opsRepo.UpsertAlertRuleState(ctx, state); err != nil {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] persist recovery state failed (rule=%d): %v", rule.ID, err)
				continue
			}
			requiredRecovery := requiredSustainedBreaches(rule.RecoverySustainedMinutes, interval)
			if !recoveredNow || state.ConsecutiveRecoveries < requiredRecovery {
				continue
			}
			resolvedAt := now
			if err := s.opsRepo.UpdateAlertEventStatus(ctx, activeEvent.ID, OpsAlertStatusResolved, &resolvedAt); err != nil {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] resolve event failed (event=%d): %v", activeEvent.ID, err)
			} else {
				eventsResolved++
				if current := activeIncidents[incidentKey]; current != nil && current.ID == activeEvent.ID {
					delete(activeIncidents, incidentKey)
				}
				activeEvent.Status = OpsAlertStatusResolved
				activeEvent.ResolvedAt = &resolvedAt
				queued := s.maybeEnqueueAlertEmail(ctx, runtimeCfg, opsAlertEmailTransitionResolved, rule, activeEvent)
				emailsQueued += queued
				if queued > 0 {
					if err := s.opsRepo.UpdateAlertEventEmailQueued(ctx, activeEvent.ID, true); err != nil {
						logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] mark email queued failed (event=%d): %v", activeEvent.ID, err)
					}
				}
			}
			continue
		}

		state.ConsecutiveRecoveries = 0
		if breachedNow {
			state.ConsecutiveBreaches++
		} else {
			state.ConsecutiveBreaches = 0
		}
		if err := s.opsRepo.UpsertAlertRuleState(ctx, state); err != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] persist breach state failed (rule=%d): %v", rule.ID, err)
			continue
		}
		if rule.ShadowMode {
			continue
		}

		required := requiredSustainedBreaches(rule.SustainedMinutes, interval)
		if breachedNow && state.ConsecutiveBreaches >= required {
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

			if familyEvent := activeIncidents[incidentKey]; familyEvent != nil {
				if opsAlertSeverityRank(rule.Severity) >= opsAlertSeverityRank(familyEvent.Severity) {
					continue
				}
				// Escalation replaces the lower-severity event in the same incident family.
				resolvedAt := now
				if err := s.opsRepo.UpdateAlertEventStatus(ctx, familyEvent.ID, OpsAlertStatusResolved, &resolvedAt); err != nil {
					logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] resolve superseded event failed (event=%d): %v", familyEvent.ID, err)
					continue
				}
				eventsResolved++
				delete(activeIncidents, incidentKey)
			}

			firedEvent := &OpsAlertEvent{
				RuleID:         rule.ID,
				Severity:       strings.TrimSpace(rule.Severity),
				Status:         OpsAlertStatusFiring,
				Title:          fmt.Sprintf("%s: %s", strings.TrimSpace(rule.Severity), strings.TrimSpace(rule.Name)),
				Description:    buildOpsAlertDescription(rule, metricValue, metric.SampleCount, metric.BadCount, windowMinutes, scopePlatform, scopeGroupID),
				MetricValue:    float64Ptr(metricValue),
				ThresholdValue: float64Ptr(rule.Threshold),
				Dimensions:     buildOpsAlertDimensions(rule.IncidentFamily, scopePlatform, scopeGroupID, scopeRegion),
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
				activeIncidents[incidentKey] = created
				queued := s.maybeEnqueueAlertEmail(ctx, runtimeCfg, opsAlertEmailTransitionFiring, rule, created)
				emailsQueued += queued
				if queued > 0 {
					if err := s.opsRepo.UpdateAlertEventEmailQueued(ctx, created.ID, true); err != nil {
						logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] mark email queued failed (event=%d): %v", created.ID, err)
					}
				}
			}
			continue
		}

	}

	result := truncateString(fmt.Sprintf(
		"rules=%d enabled=%d evaluated=%d ok=%d breached=%d no_data=%d stale=%d error=%d unsupported=%d shadow=%d created=%d resolved=%d emails_queued=%d",
		rulesTotal, rulesEnabled, rulesEvaluated,
		evaluationStatuses[OpsAlertEvaluationStatusOK], evaluationStatuses[OpsAlertEvaluationStatusBreached],
		evaluationStatuses[OpsAlertEvaluationStatusNoData], evaluationStatuses[OpsAlertEvaluationStatusStale],
		evaluationStatuses[OpsAlertEvaluationStatusError], evaluationStatuses[OpsAlertEvaluationStatusUnsupported],
		evaluationStatuses[OpsAlertEvaluationStatusShadow], eventsCreated, eventsResolved, emailsQueued,
	), 2048)
	s.recordHeartbeatSuccess(runAt, time.Since(startedAt), result)
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

type opsAlertMetricEvaluation struct {
	Status       string
	Value        *float64
	SampleCount  int64
	BadCount     int64
	DataAsOf     *time.Time
	Breached     bool
	ErrorCode    string
	ErrorMessage string
}

func (s *OpsAlertEvaluatorService) evaluateRuleMetric(
	ctx context.Context,
	rule *OpsAlertRule,
	systemMetrics *OpsSystemMetricsSnapshot,
	start time.Time,
	end time.Time,
	platform string,
	groupID *int64,
	now time.Time,
) opsAlertMetricEvaluation {
	if rule == nil {
		return opsAlertMetricEvaluation{Status: OpsAlertEvaluationStatusError, ErrorCode: "invalid_rule", ErrorMessage: "rule is nil"}
	}
	metricType := strings.TrimSpace(rule.MetricType)
	if !isSupportedOpsAlertMetric(metricType) {
		return opsAlertMetricEvaluation{Status: OpsAlertEvaluationStatusUnsupported, ErrorCode: "unsupported_metric", ErrorMessage: "metric is not supported by evaluator"}
	}

	result := opsAlertMetricEvaluation{Status: OpsAlertEvaluationStatusOK}
	if isSystemOpsAlertMetric(metricType) {
		if systemMetrics == nil || systemMetrics.CreatedAt.IsZero() {
			result.Status = OpsAlertEvaluationStatusNoData
			result.ErrorCode = "system_metric_missing"
			return result
		}
		dataAsOf := systemMetrics.CreatedAt.UTC()
		result.DataAsOf = &dataAsOf
		if now.Sub(dataAsOf) > opsAlertSystemMetricsMaxAge {
			result.Status = OpsAlertEvaluationStatusStale
			result.ErrorCode = "system_metric_stale"
			result.ErrorMessage = fmt.Sprintf("latest system metric is %s old", now.Sub(dataAsOf).Round(time.Second))
			return result
		}
	}

	switch metricType {
	case "success_rate", "error_rate", "upstream_error_rate":
		overview, err := s.opsRepo.GetDashboardOverview(ctx, &OpsDashboardFilter{
			StartTime: start, EndTime: end, Platform: platform, GroupID: groupID, QueryMode: OpsQueryModeRaw,
		})
		if err != nil {
			return opsAlertMetricEvaluation{Status: OpsAlertEvaluationStatusError, ErrorCode: "metric_query_failed", ErrorMessage: err.Error()}
		}
		if overview == nil || overview.RequestCountSLA <= 0 {
			return opsAlertMetricEvaluation{Status: OpsAlertEvaluationStatusNoData, ErrorCode: "empty_window"}
		}
		result.SampleCount = overview.RequestCountSLA
		result.BadCount = overview.ErrorCountSLA
		value := overview.ErrorRate * 100
		if metricType == "success_rate" {
			value = overview.SLA * 100
		}
		if metricType == "upstream_error_rate" {
			value = overview.UpstreamErrorRate * 100
			result.BadCount = overview.UpstreamErrorCountExcl429529
		}
		result.Value = float64Ptr(value)
		dataAsOf := end.UTC()
		result.DataAsOf = &dataAsOf
	case "availability_failure_rate", "platform_failure_rate", "provider_failure_rate", "unknown_failure_rate",
		"platform_capacity_failure_count", "compatibility_error_count", "client_rejected_count",
		"business_limited_count", "cancelled_count", "security_blocked_count", "recovered_provider_error_count":
		stats, err := s.opsRepo.GetErrorClassificationStats(ctx, &OpsDashboardFilter{
			StartTime: start, EndTime: end, Platform: platform, GroupID: groupID, QueryMode: OpsQueryModeRaw,
		})
		if err != nil {
			return opsAlertMetricEvaluation{Status: OpsAlertEvaluationStatusError, ErrorCode: "classification_metric_query_failed", ErrorMessage: err.Error()}
		}
		if stats == nil {
			return opsAlertMetricEvaluation{Status: OpsAlertEvaluationStatusNoData, ErrorCode: "classification_metric_missing"}
		}
		value, samples, bad, ok := opsClassificationMetricValue(metricType, stats)
		if !ok {
			return opsAlertMetricEvaluation{Status: OpsAlertEvaluationStatusNoData, ErrorCode: "empty_window"}
		}
		result.Value = float64Ptr(value)
		result.SampleCount = samples
		result.BadCount = bad
		dataAsOf := stats.DataAsOf.UTC()
		result.DataAsOf = &dataAsOf
	default:
		value, ok := s.computeRuleMetric(ctx, rule, systemMetrics, start, end, platform, groupID)
		if !ok {
			return opsAlertMetricEvaluation{Status: OpsAlertEvaluationStatusNoData, ErrorCode: "metric_unavailable", DataAsOf: result.DataAsOf}
		}
		result.Value = float64Ptr(value)
		result.SampleCount = 1
	}

	if result.Value == nil {
		result.Status = OpsAlertEvaluationStatusNoData
		result.ErrorCode = "metric_value_missing"
		return result
	}
	rawBreached := compareMetric(*result.Value, rule.Operator, rule.Threshold)
	if result.BadCount == 0 && rawBreached && !isBusinessRateOpsAlertMetric(metricType) {
		result.BadCount = 1
	}
	meetsSamples := rule.MinimumSamples <= 0 || result.SampleCount >= int64(rule.MinimumSamples)
	meetsBadCount := rule.MinimumBadCount <= 0 || result.BadCount >= int64(rule.MinimumBadCount)
	result.Breached = rawBreached && meetsSamples && meetsBadCount
	if result.Breached {
		result.Status = OpsAlertEvaluationStatusBreached
	}
	return result
}

func isSupportedOpsAlertMetric(metricType string) bool {
	switch metricType {
	case "success_rate", "error_rate", "upstream_error_rate",
		"availability_failure_rate", "platform_failure_rate", "provider_failure_rate", "unknown_failure_rate",
		"platform_capacity_failure_count", "compatibility_error_count", "client_rejected_count",
		"business_limited_count", "cancelled_count", "security_blocked_count", "recovered_provider_error_count",
		"cpu_usage_percent", "memory_usage_percent", "concurrency_queue_depth",
		"group_available_accounts", "group_available_ratio", "account_rate_limited_count",
		"account_error_count", "account_temp_unscheduled_count", "group_rate_limit_ratio",
		"account_error_ratio", "overload_account_count", "proxy_expired_count",
		"proxy_expiring_soon_count":
		return true
	default:
		return false
	}
}

func isSystemOpsAlertMetric(metricType string) bool {
	switch metricType {
	case "cpu_usage_percent", "memory_usage_percent", "concurrency_queue_depth":
		return true
	default:
		return false
	}
}

func isBusinessRateOpsAlertMetric(metricType string) bool {
	switch metricType {
	case "success_rate", "error_rate", "upstream_error_rate", "availability_failure_rate",
		"platform_failure_rate", "provider_failure_rate", "unknown_failure_rate":
		return true
	default:
		return false
	}
}

func opsClassificationMetricValue(metricType string, stats *OpsErrorClassificationStats) (value float64, samples, bad int64, ok bool) {
	if stats == nil {
		return 0, 0, 0, false
	}
	eligible := stats.SuccessCount + stats.SLAFailureCount
	totalObserved := stats.SuccessCount + stats.FinalErrorCount
	switch metricType {
	case "availability_failure_rate":
		if eligible <= 0 {
			return 0, 0, 0, false
		}
		return float64(stats.SLAFailureCount) / float64(eligible) * 100, eligible, stats.SLAFailureCount, true
	case "platform_failure_rate":
		if eligible <= 0 {
			return 0, 0, 0, false
		}
		return float64(stats.PlatformFailureCount) / float64(eligible) * 100, eligible, stats.PlatformFailureCount, true
	case "provider_failure_rate":
		if eligible <= 0 {
			return 0, 0, 0, false
		}
		return float64(stats.ProviderFailureCount) / float64(eligible) * 100, eligible, stats.ProviderFailureCount, true
	case "unknown_failure_rate":
		if eligible <= 0 {
			return 0, 0, 0, false
		}
		return float64(stats.UnknownFailureCount) / float64(eligible) * 100, eligible, stats.UnknownFailureCount, true
	case "platform_capacity_failure_count":
		return float64(stats.PlatformCapacityCount), totalObserved, stats.PlatformCapacityCount, totalObserved > 0
	case "compatibility_error_count":
		return float64(stats.CompatibilityCount), totalObserved, stats.CompatibilityCount, totalObserved > 0
	case "client_rejected_count":
		return float64(stats.ClientRejectedCount), totalObserved, stats.ClientRejectedCount, totalObserved > 0
	case "business_limited_count":
		return float64(stats.BusinessLimitedCount), totalObserved, stats.BusinessLimitedCount, totalObserved > 0
	case "cancelled_count":
		return float64(stats.CancelledCount), totalObserved, stats.CancelledCount, totalObserved > 0
	case "security_blocked_count":
		return float64(stats.SecurityBlockedCount), totalObserved, stats.SecurityBlockedCount, totalObserved > 0
	case "recovered_provider_error_count":
		return float64(stats.RecoveredProviderCount), totalObserved, stats.RecoveredProviderCount, totalObserved > 0
	default:
		return 0, 0, 0, false
	}
}

func (s *OpsAlertEvaluatorService) loadAlertRuleState(ctx context.Context, ruleID int64) (*OpsAlertRuleState, error) {
	state, err := s.opsRepo.GetAlertRuleState(ctx, ruleID)
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &OpsAlertRuleState{RuleID: ruleID}
	}
	return state, nil
}

func resetAlertRuleStateAfterGap(state *OpsAlertRuleState, now time.Time, interval time.Duration) {
	if state == nil || state.LastEvaluatedAt == nil || interval <= 0 {
		return
	}
	if now.Sub(*state.LastEvaluatedAt) > interval*2 {
		state.ConsecutiveBreaches = 0
		state.ConsecutiveRecoveries = 0
	}
}

func opsAlertTimePtr(value time.Time) *time.Time {
	return &value
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

func buildOpsAlertDimensions(incidentFamily, platform string, groupID *int64, region *string) map[string]any {
	dims := map[string]any{}
	if family := strings.TrimSpace(incidentFamily); family != "" {
		dims["incident_family"] = family
	}
	if strings.TrimSpace(platform) != "" {
		dims["platform"] = strings.TrimSpace(platform)
	}
	if groupID != nil && *groupID > 0 {
		dims["group_id"] = *groupID
	}
	if region != nil && strings.TrimSpace(*region) != "" {
		dims["region"] = strings.TrimSpace(*region)
	}
	if len(dims) == 0 {
		return nil
	}
	return dims
}

func opsAlertSeverityRank(severity string) int {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	case "P3":
		return 3
	default:
		return 4
	}
}

func opsAlertIncidentKey(rule *OpsAlertRule, platform string, groupID *int64, region *string) string {
	if rule == nil || rule.ID <= 0 {
		return ""
	}
	family := strings.TrimSpace(rule.IncidentFamily)
	if family == "" || family == "custom" {
		family = fmt.Sprintf("custom_rule_%d", rule.ID)
	}
	group := int64(0)
	if groupID != nil {
		group = *groupID
	}
	regionValue := ""
	if region != nil {
		regionValue = strings.TrimSpace(*region)
	}
	return fmt.Sprintf("%s|%s|%d|%s", family, strings.TrimSpace(platform), group, regionValue)
}

func opsAlertIncidentKeyFromEvent(rule *OpsAlertRule, event *OpsAlertEvent) string {
	if rule == nil || event == nil {
		return ""
	}
	platform := ""
	var groupID *int64
	var region *string
	if event.Dimensions != nil {
		if value, ok := event.Dimensions["platform"].(string); ok {
			platform = value
		}
		switch value := event.Dimensions["group_id"].(type) {
		case float64:
			id := int64(value)
			groupID = &id
		case int64:
			id := value
			groupID = &id
		case int:
			id := int64(value)
			groupID = &id
		}
		if value, ok := event.Dimensions["region"].(string); ok && strings.TrimSpace(value) != "" {
			trimmed := strings.TrimSpace(value)
			region = &trimmed
		}
	}
	return opsAlertIncidentKey(rule, platform, groupID, region)
}

func buildOpsAlertDescription(rule *OpsAlertRule, value float64, sampleCount, badCount int64, windowMinutes int, platform string, groupID *int64) string {
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
	return fmt.Sprintf("%s %s %.2f (current %.2f, bad=%d, samples=%d) over last %dm (%s)",
		strings.TrimSpace(rule.MetricType),
		strings.TrimSpace(rule.Operator),
		rule.Threshold,
		value,
		badCount,
		sampleCount,
		windowMinutes,
		strings.TrimSpace(scope),
	)
}

func (s *OpsAlertEvaluatorService) maybeEnqueueAlertEmail(ctx context.Context, runtimeCfg *OpsAlertRuntimeSettings, transition string, rule *OpsAlertRule, event *OpsAlertEvent) int {
	if s == nil || s.notificationDispatcher == nil || s.opsService == nil || event == nil || rule == nil {
		return 0
	}
	if !rule.NotifyEmail {
		return 0
	}

	emailCfg, err := s.opsService.GetEmailNotificationConfig(ctx)
	if err != nil || emailCfg == nil {
		return 0
	}
	alertEnabled := emailCfg.Alert.Enabled
	recipients := normalizeEmails(emailCfg.Alert.Recipients)
	if notificationService := s.notificationDispatcher.emailService; notificationService != nil {
		channel, configured, channelErr := notificationService.GetChannelPolicyState(ctx, NotificationEmailChannelOpsAlert)
		if channelErr != nil {
			return 0
		}
		if configured {
			alertEnabled = channel.Enabled
			resolved, resolveErr := notificationService.ResolveGroupRecipients(ctx, NotificationEmailChannelOpsAlert)
			if resolveErr != nil {
				return 0
			}
			recipients = resolved
		}
	}
	if !alertEnabled || len(recipients) == 0 {
		return 0
	}
	if !shouldSendOpsAlertEmailByMinSeverity(strings.TrimSpace(emailCfg.Alert.MinSeverity), strings.TrimSpace(rule.Severity)) {
		return 0
	}

	transition = strings.ToLower(strings.TrimSpace(transition))
	switch transition {
	case opsAlertEmailTransitionFiring:
	case opsAlertEmailTransitionResolved:
		if !emailCfg.Alert.IncludeResolvedAlerts {
			return 0
		}
	default:
		return 0
	}

	if runtimeCfg != nil && runtimeCfg.Silencing.Enabled {
		if isOpsAlertSilenced(time.Now().UTC(), rule, event, runtimeCfg.Silencing) {
			return 0
		}
	}

	queued := 0
	incidentKey := opsAlertIncidentKeyFromEvent(rule, event)
	reminderKey := transition
	if incidentKey != "" {
		reminderKey += ":" + incidentKey
	}
	reminderKey = truncateString(reminderKey, 200)
	for _, recipient := range recipients {
		suppressed, suppressReason, suppressErr := s.shouldSuppressOpsAlertEmail(
			ctx,
			recipient,
			reminderKey,
			emailCfg.Alert.RateLimitPerHour,
			time.Duration(emailCfg.Alert.BatchingWindowSeconds)*time.Second,
			time.Now().UTC(),
		)
		if suppressErr != nil {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] inspect email limits failed (event=%d transition=%s): %v", event.ID, transition, suppressErr)
			continue
		}
		if suppressed {
			logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] email suppressed (event=%d transition=%s recipient=%s reason=%s)", event.ID, transition, maskNotificationEmail(recipient), suppressReason)
			continue
		}
		result, enqueueErr := s.notificationDispatcher.Enqueue(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventOpsAlert,
			RecipientEmail: recipient,
			RecipientName:  emailRecipientName(recipient),
			SourceType:     "ops_alert_event",
			SourceID:       fmt.Sprintf("%d", event.ID),
			ReminderKey:    reminderKey,
			Variables:      opsAlertEmailVariables(rule, event),
		})
		if enqueueErr != nil {
			if !errors.Is(enqueueErr, ErrNotificationEmailChannelDisabled) {
				logger.LegacyPrintf("service.ops_alert_evaluator", "[OpsAlertEvaluator] enqueue email failed (event=%d transition=%s): %v", event.ID, transition, enqueueErr)
			}
			continue
		}
		if result.Created {
			queued++
		}
	}
	return queued
}

func (s *OpsAlertEvaluatorService) shouldSuppressOpsAlertEmail(
	ctx context.Context,
	recipient string,
	reminderKey string,
	rateLimitPerHour int,
	batchingWindow time.Duration,
	now time.Time,
) (bool, string, error) {
	if s == nil || s.notificationDispatcher == nil || s.notificationDispatcher.repo == nil {
		return false, "", nil
	}
	if rateLimitPerHour <= 0 && batchingWindow <= 0 {
		return false, "", nil
	}
	recipientHash := notificationEmailHash(strings.ToLower(strings.TrimSpace(recipient)))
	hourStart := now.Add(-time.Hour)
	if batchingWindow > 0 {
		batchStart := now.Add(-batchingWindow)
		result, err := s.notificationDispatcher.repo.List(ctx, NotificationEmailDeliveryListFilter{
			Page:          1,
			PageSize:      1,
			Event:         NotificationEmailEventOpsAlert,
			SourceType:    "ops_alert_event",
			RecipientHash: recipientHash,
			ReminderKey:   strings.TrimSpace(reminderKey),
			CreatedAfter:  &batchStart,
		})
		if err != nil {
			return false, "", err
		}
		if result.Total > 0 {
			return true, "incident_merge_window", nil
		}
	}
	if rateLimitPerHour > 0 {
		result, err := s.notificationDispatcher.repo.List(ctx, NotificationEmailDeliveryListFilter{
			Page:          1,
			PageSize:      1,
			Event:         NotificationEmailEventOpsAlert,
			SourceType:    "ops_alert_event",
			RecipientHash: recipientHash,
			CreatedAfter:  &hourStart,
		})
		if err != nil {
			return false, "", err
		}
		if result.Total >= int64(rateLimitPerHour) {
			return true, "recipient_hourly_limit", nil
		}
	}
	return false, "", nil
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
