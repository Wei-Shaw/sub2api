package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestOpenAIHAObservabilityMetricsSnapshot(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIScheduler.Strategy = "high_availability"
	cfg.Gateway.OpenAIScheduler.HealthCircuitEnabled = true
	cfg.Gateway.OpenAIScheduler.HealthCircuitFailureThreshold = 1
	cfg.Gateway.OpenAIScheduler.HealthCircuitWindowSeconds = 60
	cfg.Gateway.OpenAIScheduler.HealthCircuitCooldownSeconds = 30
	scheduler := &defaultOpenAIAccountScheduler{
		service: &OpenAIGatewayService{cfg: cfg},
		stats:   newOpenAIAccountRuntimeStats(),
	}

	rateLimit := &UpstreamFailoverError{StatusCode: 429, Scope: GatewayFailureScopeAccount}
	scheduler.ReportResultWithErrorContext(context.Background(), 501, false, nil, rateLimit)
	scheduler.ReportResultWithErrorContext(context.Background(), 501, true, intPtrForTest(80), nil)
	scheduler.ReportProbeResult(501, true)
	scheduler.ReportProbeResult(501, false)

	snapshot := scheduler.SnapshotMetrics()
	require.Equal(t, int64(1), snapshot.ResultSuccessTotal)
	require.Equal(t, int64(1), snapshot.ResultFailureTotal)
	require.Equal(t, int64(1), snapshot.RateLimitFailureTotal)
	require.Equal(t, int64(1), snapshot.HealthCircuitOpenedTotal)
	require.Equal(t, int64(1), snapshot.HealthCircuitRecoveredTotal)
	require.Equal(t, int64(1), snapshot.ScheduledProbeSuccessTotal)
	require.Equal(t, int64(1), snapshot.ScheduledProbeFailureTotal)
}

func TestOpenAIHAObservabilityLogsCorrelatedSelectionAndHealthState(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()

	cfg := &config.Config{}
	cfg.Gateway.OpenAIScheduler.Strategy = "high_availability"
	cfg.Gateway.OpenAIScheduler.HealthCircuitEnabled = true
	cfg.Gateway.OpenAIScheduler.HealthCircuitFailureThreshold = 1
	cfg.Gateway.OpenAIScheduler.HealthCircuitWindowSeconds = 60
	cfg.Gateway.OpenAIScheduler.HealthCircuitCooldownSeconds = 30
	scheduler := &defaultOpenAIAccountScheduler{
		service: &OpenAIGatewayService{cfg: cfg},
		stats:   newOpenAIAccountRuntimeStats(),
	}
	ctx := logger.IntoContext(context.Background(), logger.With(zap.String("request_id", "ha-observe-1")))
	groupID := int64(77)
	req := OpenAIAccountScheduleRequest{GroupID: &groupID, Platform: PlatformOpenAI, RequestedModel: "gpt-5"}
	scheduler.observeCandidatePlan(ctx, req, openAIAccountLoadPlan{
		candidates: []openAIAccountCandidateScore{{
			account:   &Account{ID: 502, Priority: 3},
			loadInfo:  &AccountLoadInfo{AccountID: 502, LoadRate: 20, WaitingCount: 1},
			score:     4.25,
			errorRate: 0.1,
			ttft:      120,
			hasTTFT:   true,
			priority:  3,
		}},
		candidateCount: 1,
		topK:           1,
	})
	scheduler.observeSelection(ctx, req, OpenAIAccountScheduleDecision{
		Layer:             openAIAccountScheduleLayerLoadBalance,
		CandidateCount:    1,
		TopK:              1,
		SelectedAccountID: 502,
		LatencyMs:         2,
	}, nil)
	scheduler.ReportResultWithErrorContext(ctx, 502, false, nil, &UpstreamFailoverError{StatusCode: 503, Scope: GatewayFailureScopeAccount})

	require.True(t, logSink.ContainsMessageAtLevel("openai.ha_scheduler.candidates", "debug"))
	require.True(t, logSink.ContainsMessageAtLevel("openai.ha_scheduler.selection", "debug"))
	require.True(t, logSink.ContainsMessageAtLevel("openai.ha_scheduler.account_result", "debug"))
	require.True(t, logSink.ContainsMessageAtLevel("openai.ha_scheduler.circuit_opened", "warn"))
	require.True(t, logSink.ContainsFieldValue("request_id", "ha-observe-1"))
	require.True(t, logSink.ContainsFieldValue("selected_account_id", "502"))
	require.True(t, logSink.ContainsFieldValue("failure_category", string(openAIAccountFailureServer)))
}

func TestOpenAIHAHealthSampleSource(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	now := time.Now()
	require.Equal(t, "cold", stats.healthSampleSourceAt(503, now))
	stats.reportProbe(503, true, now)
	require.Equal(t, "probe", stats.healthSampleSourceAt(503, now))
	stats.report(503, true, nil)
	require.Equal(t, "real_and_probe", stats.healthSampleSourceAt(503, time.Now()))
}
