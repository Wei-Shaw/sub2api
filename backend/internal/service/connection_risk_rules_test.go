package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScoreConnectionRiskR1(t *testing.T) {
	settings := *DefaultConnectionRiskSettings()
	m := &ConnectionRiskSubjectMetrics{
		APIKeyID: 1, UserID: 2,
		DistinctIP5m: 8, ReqCount5m: 20,
	}
	res := ScoreConnectionRisk(m, settings)
	require.True(t, res.ShouldOpen)
	require.Contains(t, ruleIDs(res), "R1")
	// R1 alone: 30 * 0.85 = 25.5 → low band [15, 30)
	require.Equal(t, connectionRiskSeverityLow, res.Severity)
	require.InDelta(t, 25.5, res.Score, 0.01)
}

func TestScoreConnectionRiskR2SlidingWindowInput(t *testing.T) {
	settings := *DefaultConnectionRiskSettings()
	// R2 alone: 15*0.7=10.5 < 15 → no event; pair with R3_abs (9) → 19.5 opens.
	m := &ConnectionRiskSubjectMetrics{
		APIKeyID: 1, UserID: 2,
		UACount1h: 6,
		IPHll24h:  40, IPHll1h: 15,
	}
	res := ScoreConnectionRisk(m, settings)
	require.True(t, res.ShouldOpen)
	require.Contains(t, ruleIDs(res), "R2")
	require.Contains(t, ruleIDs(res), "R3_abs")
}

func TestScoreConnectionRiskR3Abs(t *testing.T) {
	settings := *DefaultConnectionRiskSettings()
	m := &ConnectionRiskSubjectMetrics{
		APIKeyID: 1, UserID: 2,
		IPHll24h: 40, IPHll1h: 15,
		UACount1h: 6,
	}
	res := ScoreConnectionRisk(m, settings)
	require.Contains(t, ruleIDs(res), "R3_abs")
	require.True(t, res.ShouldOpen)
}

func TestScoreConnectionRiskR7Critical(t *testing.T) {
	settings := *DefaultConnectionRiskSettings()
	m := &ConnectionRiskSubjectMetrics{
		APIKeyID: 1, UserID: 2,
		SBMismatch15m: 1, DistinctIP5m: 3,
	}
	res := ScoreConnectionRisk(m, settings)
	require.True(t, res.ShouldOpen)
	require.Equal(t, connectionRiskSeverityCritical, res.Severity)
	require.Contains(t, ruleIDs(res), "R7")
}

func TestScoreConnectionRiskR5NeedsConcurrencyLimit(t *testing.T) {
	settings := *DefaultConnectionRiskSettings()
	m := &ConnectionRiskSubjectMetrics{
		APIKeyID: 1, UserID: 2,
		UserConcurrency: 9, UserConcurrencyLimit: 10,
		DistinctIP5m: 5,
	}
	res := ScoreConnectionRisk(m, settings)
	require.Contains(t, ruleIDs(res), "R5")

	m.UserConcurrencyLimit = 0
	res = ScoreConnectionRisk(m, settings)
	require.NotContains(t, ruleIDs(res), "R5")
}

func TestScoreConnectionRiskDisabledRule(t *testing.T) {
	settings := *DefaultConnectionRiskSettings()
	off := false
	settings.Rules.R2.Enabled = &off
	m := &ConnectionRiskSubjectMetrics{APIKeyID: 1, UserID: 2, UACount1h: 100}
	res := ScoreConnectionRisk(m, settings)
	require.NotContains(t, ruleIDs(res), "R2")
}

func TestScoreConnectionRiskNoHits(t *testing.T) {
	settings := *DefaultConnectionRiskSettings()
	m := &ConnectionRiskSubjectMetrics{APIKeyID: 1, UserID: 2, DistinctIP5m: 1, ReqCount5m: 1}
	res := ScoreConnectionRisk(m, settings)
	require.False(t, res.ShouldOpen)
	require.Empty(t, res.RulesFired)
}

func TestScoreConnectionRiskR3Baseline(t *testing.T) {
	settings := *DefaultConnectionRiskSettings()
	on := true
	settings.Rules.R3.Enabled = &on
	m := &ConnectionRiskSubjectMetrics{
		APIKeyID: 1, UserID: 2,
		IPHll24h: 100, BaselineP95: 20, BaselineSampleDays: 5, BaselineFactor: 3,
	}
	res := ScoreConnectionRisk(m, settings)
	require.Contains(t, ruleIDs(res), "R3")
	require.True(t, res.ShouldOpen)
}

func TestPercentileApprox(t *testing.T) {
	require.Equal(t, 40.0, percentileApprox([]int64{10, 20, 30, 40}, 95))
	require.Equal(t, 0.0, percentileApprox(nil, 95))
}

func ruleIDs(res ConnectionRiskScoreResult) []string {
	out := make([]string, 0, len(res.RulesFired))
	for _, h := range res.RulesFired {
		out = append(out, h.RuleID)
	}
	return out
}
