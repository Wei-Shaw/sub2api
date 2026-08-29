package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestApplyVideoTaskTotalDurationPrefersUpstreamTimestamps(t *testing.T) {
	result := &OpenAIForwardResult{
		VideoCreatedAtUnix:   1712697600,
		VideoCompletedAtUnix: 1712697725,
	}
	pending := &GrokVideoPendingBilling{
		CreatedAt: time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}

	applyVideoTaskTotalDuration(result, pending, time.Date(2026, 8, 30, 0, 10, 0, 0, time.UTC))

	require.Equal(t, 125*time.Second, result.Duration)
}

func TestApplyVideoTaskTotalDurationUsesPersistedUpstreamCreatedAt(t *testing.T) {
	result := &OpenAIForwardResult{VideoCompletedAtUnix: 1712697725}
	pending := &GrokVideoPendingBilling{UpstreamCreatedAtUnix: 1712697600}

	applyVideoTaskTotalDuration(result, pending, time.Now())

	require.Equal(t, 125*time.Second, result.Duration)
}

func TestApplyVideoTaskTotalDurationFallsBackToLocalWallClock(t *testing.T) {
	created := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	discovered := created.Add(3*time.Minute + 17*time.Second)
	result := &OpenAIForwardResult{}
	pending := &GrokVideoPendingBilling{CreatedAt: created.Format(time.RFC3339Nano)}

	applyVideoTaskTotalDuration(result, pending, discovered)

	require.Equal(t, 3*time.Minute+17*time.Second, result.Duration)
}

func TestApplyVideoTaskTotalDurationRejectsInvalidUpstreamRange(t *testing.T) {
	created := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	result := &OpenAIForwardResult{
		VideoCreatedAtUnix:   1712697800,
		VideoCompletedAtUnix: 1712697725,
	}
	pending := &GrokVideoPendingBilling{CreatedAt: created.Format(time.RFC3339Nano)}

	applyVideoTaskTotalDuration(result, pending, created.Add(2*time.Minute))

	require.Equal(t, 2*time.Minute, result.Duration)
}

func TestApplySettledBalanceCostUsesCapturedActualAmount(t *testing.T) {
	settled := 1.6080000000000002
	cost := &CostBreakdown{TotalCost: 1.608, ActualCost: 1.608}

	applySettledBalanceCost(cost, &OpenAIRecordUsageInput{
		BalanceAlreadyReserved: true,
		SettledBalanceCost:     &settled,
	})

	require.Equal(t, 1.608, cost.ActualCost)
	require.Equal(t, 1.608, cost.TotalCost,
		"上游原始成本统计与最终实际结算金额保持一致")
}

func TestApplySettledBalanceCostDoesNotOverrideNormalRequests(t *testing.T) {
	settled := 2.01
	cost := &CostBreakdown{ActualCost: 1.608}

	applySettledBalanceCost(cost, &OpenAIRecordUsageInput{
		SettledBalanceCost: &settled,
	})

	require.Equal(t, 1.608, cost.ActualCost)
}

func TestMergeVideoTaskBillingResultPrefersCompletionMetadata(t *testing.T) {
	result := &OpenAIForwardResult{
		Model:                "minimax-h3-768p",
		VideoDurationSeconds: 8,
		VideoResolution:      VideoBillingResolution720P,
	}
	pending := &GrokVideoPendingBilling{
		Model:                "minimax-h3-768p",
		VideoDurationSeconds: 10,
		VideoResolution:      VideoBillingResolution480P,
	}

	require.NoError(t, mergeVideoTaskBillingResult(result, pending))
	require.Equal(t, 8, result.VideoDurationSeconds)
	require.Equal(t, VideoBillingResolution720P, result.VideoResolution)
}

func TestMergeVideoTaskBillingResultFallsBackToCreateSnapshot(t *testing.T) {
	result := &OpenAIForwardResult{}
	pending := &GrokVideoPendingBilling{
		Model:                "minimax-h3-768p",
		VideoDurationSeconds: 10,
		VideoResolution:      VideoBillingResolution720P,
	}

	require.NoError(t, mergeVideoTaskBillingResult(result, pending))
	require.Equal(t, 10, result.VideoDurationSeconds)
	require.Equal(t, VideoBillingResolution720P, result.VideoResolution)
}

func TestMergeVideoTaskBillingResultRejectsMissingDuration(t *testing.T) {
	result := &OpenAIForwardResult{VideoResolution: VideoBillingResolution720P}
	pending := &GrokVideoPendingBilling{Model: "minimax-h3-768p"}

	require.ErrorContains(t, mergeVideoTaskBillingResult(result, pending), "duration is unavailable")
}

func TestVideoTaskCaptureCommandKeepsHoldAndActualAmountsSeparate(t *testing.T) {
	cmd := videoTaskHoldCommand("capture", "video_task:test", 1, 1, 2.01, 1.608, "payload")
	cmd.Normalize()

	require.Equal(t, 2.01, cmd.HoldAmount)
	require.Equal(t, 1.608, cmd.ActualAmount)
}
