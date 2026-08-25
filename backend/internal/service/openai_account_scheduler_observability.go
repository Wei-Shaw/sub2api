package service

import (
	"context"
	"errors"
	"sort"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	openAIHAObservedCandidateLimit = 16
	openAIHAHealthCacheLogInterval = time.Minute
)

type openAIHAObservabilityMetrics struct {
	resultSuccessTotal          atomic.Int64
	resultFailureTotal          atomic.Int64
	rateLimitFailureTotal       atomic.Int64
	authFailureTotal            atomic.Int64
	serverFailureTotal          atomic.Int64
	timeoutFailureTotal         atomic.Int64
	networkFailureTotal         atomic.Int64
	requestFailureTotal         atomic.Int64
	requestScopedFailureTotal   atomic.Int64
	otherFailureTotal           atomic.Int64
	healthCircuitOpenedTotal    atomic.Int64
	healthCircuitRecoveredTotal atomic.Int64
	scheduledProbeSuccessTotal  atomic.Int64
	scheduledProbeFailureTotal  atomic.Int64
	lastHealthCacheLogUnixNano  atomic.Int64
}

type openAIHACandidateObservation struct {
	AccountID          int64   `json:"account_id"`
	Score              float64 `json:"score"`
	ErrorRate          float64 `json:"error_rate"`
	TTFTMs             float64 `json:"ttft_ms,omitempty"`
	TotalLatencyMs     float64 `json:"total_latency_ms,omitempty"`
	HasTotalLatency    bool    `json:"has_total_latency,omitempty"`
	HealthSampleSource string  `json:"health_sample_source"`
	Priority           int     `json:"priority"`
	LoadRate           int     `json:"load_rate"`
	WaitingCount       int     `json:"waiting_count"`
}

func (m *openAIHAObservabilityMetrics) recordResult(success bool, category openAIAccountFailureCategory) {
	if m == nil {
		return
	}
	if success {
		m.resultSuccessTotal.Add(1)
		return
	}
	m.resultFailureTotal.Add(1)
	switch category {
	case openAIAccountFailureRateLimit:
		m.rateLimitFailureTotal.Add(1)
	case openAIAccountFailureAuth:
		m.authFailureTotal.Add(1)
	case openAIAccountFailureServer:
		m.serverFailureTotal.Add(1)
	case openAIAccountFailureTimeout:
		m.timeoutFailureTotal.Add(1)
	case openAIAccountFailureNetwork:
		m.networkFailureTotal.Add(1)
	case openAIAccountFailureRequest:
		m.requestFailureTotal.Add(1)
	case openAIAccountFailureRequestScope:
		m.requestScopedFailureTotal.Add(1)
	default:
		m.otherFailureTotal.Add(1)
	}
}

func (m *openAIHAObservabilityMetrics) recordCircuitOpened() {
	if m != nil {
		m.healthCircuitOpenedTotal.Add(1)
	}
}

func (m *openAIHAObservabilityMetrics) recordCircuitRecovered() {
	if m != nil {
		m.healthCircuitRecoveredTotal.Add(1)
	}
}

func (m *openAIHAObservabilityMetrics) recordProbe(success bool) {
	if m == nil {
		return
	}
	if success {
		m.scheduledProbeSuccessTotal.Add(1)
	} else {
		m.scheduledProbeFailureTotal.Add(1)
	}
}

func (m *openAIHAObservabilityMetrics) addToSnapshot(snapshot *OpenAIAccountSchedulerMetricsSnapshot) {
	if m == nil || snapshot == nil {
		return
	}
	snapshot.ResultSuccessTotal = m.resultSuccessTotal.Load()
	snapshot.ResultFailureTotal = m.resultFailureTotal.Load()
	snapshot.RateLimitFailureTotal = m.rateLimitFailureTotal.Load()
	snapshot.AuthFailureTotal = m.authFailureTotal.Load()
	snapshot.ServerFailureTotal = m.serverFailureTotal.Load()
	snapshot.TimeoutFailureTotal = m.timeoutFailureTotal.Load()
	snapshot.NetworkFailureTotal = m.networkFailureTotal.Load()
	snapshot.RequestFailureTotal = m.requestFailureTotal.Load()
	snapshot.RequestScopedFailureTotal = m.requestScopedFailureTotal.Load()
	snapshot.OtherFailureTotal = m.otherFailureTotal.Load()
	snapshot.HealthCircuitOpenedTotal = m.healthCircuitOpenedTotal.Load()
	snapshot.HealthCircuitRecoveredTotal = m.healthCircuitRecoveredTotal.Load()
	snapshot.ScheduledProbeSuccessTotal = m.scheduledProbeSuccessTotal.Load()
	snapshot.ScheduledProbeFailureTotal = m.scheduledProbeFailureTotal.Load()
}

func (s *defaultOpenAIAccountScheduler) observeSelection(ctx context.Context, req OpenAIAccountScheduleRequest, decision OpenAIAccountScheduleDecision, selectErr error) {
	if s == nil || s.service == nil || !s.service.openAIHighAvailabilityEnabled() {
		return
	}
	requestLog := logger.FromContext(ctx)
	if !requestLog.Core().Enabled(zap.DebugLevel) {
		return
	}
	outcome := "selected"
	if selectErr != nil {
		outcome = openAIHASelectionErrorCategory(selectErr)
	} else if decision.SelectedAccountID <= 0 {
		outcome = "empty"
	}
	fields := []zap.Field{
		zap.String("strategy", "high_availability"),
		zap.String("selection_mode", normalizeGroupSelectionMode(req.SelectionMode)),
		zap.String("outcome", outcome),
		zap.String("platform", NormalizeOpenAICompatiblePlatform(req.Platform)),
		zap.String("model", req.RequestedModel),
		zap.String("layer", decision.Layer),
		zap.Int("candidate_count", decision.CandidateCount),
		zap.Int("top_k", decision.TopK),
		zap.Int("excluded_count", len(req.ExcludedIDs)),
		zap.Int64("selected_account_id", decision.SelectedAccountID),
		zap.String("selected_account_type", decision.SelectedAccountType),
		zap.Bool("sticky_previous_hit", decision.StickyPreviousHit),
		zap.Bool("sticky_session_hit", decision.StickySessionHit),
		zap.Int64("scheduler_latency_ms", decision.LatencyMs),
	}
	if req.GroupID != nil {
		fields = append(fields, zap.Int64("group_id", *req.GroupID))
	}
	requestLog.Debug("openai.ha_scheduler.selection", fields...)
}

func openAIHASelectionErrorCategory(err error) string {
	switch {
	case errors.Is(err, ErrNoAvailableCompactAccounts):
		return "no_available_compact_account"
	case errors.Is(err, ErrNoAvailableAccounts):
		return "no_available_account"
	case errors.Is(err, context.Canceled):
		return "request_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "selection_timeout"
	default:
		return "selection_error"
	}
}

func (s *defaultOpenAIAccountScheduler) observeCandidatePlan(ctx context.Context, req OpenAIAccountScheduleRequest, plan openAIAccountLoadPlan) {
	if s == nil || s.service == nil || !s.service.openAIHighAvailabilityEnabled() {
		return
	}
	requestLog := logger.FromContext(ctx)
	if !requestLog.Core().Enabled(zap.DebugLevel) {
		return
	}
	ranked := append([]openAIAccountCandidateScore(nil), plan.candidates...)
	sort.SliceStable(ranked, func(i, j int) bool {
		return isOpenAIAccountCandidateBetter(ranked[i], ranked[j])
	})
	limit := len(ranked)
	if limit > openAIHAObservedCandidateLimit {
		limit = openAIHAObservedCandidateLimit
	}
	now := time.Now()
	candidates := make([]openAIHACandidateObservation, 0, limit)
	for _, candidate := range ranked[:limit] {
		if candidate.account == nil || candidate.loadInfo == nil {
			continue
		}
		candidates = append(candidates, openAIHACandidateObservation{
			AccountID:          candidate.account.ID,
			Score:              candidate.score,
			ErrorRate:          candidate.errorRate,
			TTFTMs:             candidate.ttft,
			TotalLatencyMs:     candidate.totalLatencyMs,
			HasTotalLatency:    candidate.hasTotalLatency,
			HealthSampleSource: s.stats.healthSampleSourceAt(candidate.account.ID, now),
			Priority:           candidate.priority,
			LoadRate:           candidate.loadInfo.LoadRate,
			WaitingCount:       candidate.loadInfo.WaitingCount,
		})
	}
	fields := []zap.Field{
		zap.String("strategy", "high_availability"),
		zap.String("selection_mode", normalizeGroupSelectionMode(req.SelectionMode)),
		zap.String("platform", NormalizeOpenAICompatiblePlatform(req.Platform)),
		zap.String("model", req.RequestedModel),
		zap.Int("candidate_count", plan.candidateCount),
		zap.Int("top_k", plan.topK),
		zap.Int("candidate_log_limit", openAIHAObservedCandidateLimit),
		zap.Int("candidate_truncated_count", len(ranked)-limit),
		zap.Any("candidates", candidates),
	}
	if req.GroupID != nil {
		fields = append(fields, zap.Int64("group_id", *req.GroupID))
	}
	requestLog.Debug("openai.ha_scheduler.candidates", fields...)
}

func (s *openAIAccountRuntimeStats) healthSampleSourceAt(accountID int64, now time.Time) string {
	if s == nil || accountID <= 0 {
		return "cold"
	}
	value, ok := s.accounts.Load(accountID)
	if !ok {
		return "cold"
	}
	stat, _ := value.(*openAIAccountRuntimeStat)
	if stat == nil {
		return "cold"
	}
	realFresh := stat.samples.Load() > 0 && sampleTimestampFresh(stat.lastSampleUnixNano.Load(), now, openAIRealHealthSampleTTL)
	probeFresh := stat.probeSamples.Load() > 0 && sampleTimestampFresh(stat.lastProbeSampleUnixNano.Load(), now, openAIScheduledProbeSampleTTL)
	switch {
	case realFresh && probeFresh:
		return "real_and_probe"
	case realFresh:
		return "real"
	case probeFresh:
		return "probe"
	default:
		return "cold"
	}
}

func (s *defaultOpenAIAccountScheduler) observeAccountResult(ctx context.Context, accountID int64, success bool, firstTokenMs *int, category openAIAccountFailureCategory) {
	if s == nil || s.service == nil || !s.service.openAIHighAvailabilityEnabled() {
		return
	}
	requestLog := logger.FromContext(ctx)
	if !requestLog.Core().Enabled(zap.DebugLevel) {
		return
	}
	now := time.Now()
	errorRate, ttft, hasTTFT := s.stats.healthSnapshotAt(accountID, now)
	fields := []zap.Field{
		zap.Int64("account_id", accountID),
		zap.Bool("success", success),
		zap.Float64("health_error_rate", errorRate),
		zap.Float64("health_ttft_ms", ttft),
		zap.Bool("health_has_ttft", hasTTFT),
		zap.String("health_sample_source", s.stats.healthSampleSourceAt(accountID, now)),
		zap.Bool("local_circuit_open", s.stats.circuitOpen(accountID, now)),
	}
	if !success {
		fields = append(fields, zap.String("failure_category", string(category)))
	}
	if firstTokenMs != nil {
		fields = append(fields, zap.Int("first_token_ms", *firstTokenMs))
	}
	requestLog.Debug("openai.ha_scheduler.account_result", fields...)
}

func (s *defaultOpenAIAccountScheduler) observeCircuitOpened(ctx context.Context, accountID int64, category openAIAccountFailureCategory, until time.Time, distributed bool) {
	logger.FromContext(ctx).Warn("openai.ha_scheduler.circuit_opened",
		zap.Int64("account_id", accountID),
		zap.String("failure_category", string(category)),
		zap.Time("circuit_until", until),
		zap.Bool("distributed", distributed),
	)
}

func (s *defaultOpenAIAccountScheduler) observeCircuitRecovered(ctx context.Context, accountID int64) {
	logger.FromContext(ctx).Info("openai.ha_scheduler.circuit_recovered", zap.Int64("account_id", accountID))
}

func (s *defaultOpenAIAccountScheduler) observeHealthCacheError(ctx context.Context, operation string, err error) {
	if s == nil || err == nil {
		return
	}
	now := time.Now()
	last := s.observability.lastHealthCacheLogUnixNano.Load()
	if last > 0 && now.Sub(time.Unix(0, last)) < openAIHAHealthCacheLogInterval {
		return
	}
	if !s.observability.lastHealthCacheLogUnixNano.CompareAndSwap(last, now.UnixNano()) {
		return
	}
	logger.FromContext(ctx).Warn("openai.ha_scheduler.health_cache_fallback",
		zap.String("operation", operation),
		zap.String("fallback", "process_local"),
		zap.Error(err),
	)
}
