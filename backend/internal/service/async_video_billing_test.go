//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type asyncVideoBillingTaskRepo struct {
	AsyncVideoTaskRepository
	markSucceededCalled   bool
	markSucceededErr      error
	markBillingFailedCall bool
	billingFailureReason  string
	usage                 *VideoTerminalUsageLogInput
}

func (r *asyncVideoBillingTaskRepo) MarkSucceeded(context.Context, int64, []string, []string, map[string]any, float64, int, float64) (bool, error) {
	r.markSucceededCalled = true
	return r.markSucceededErr == nil, r.markSucceededErr
}

func (r *asyncVideoBillingTaskRepo) MarkBillingFailed(_ context.Context, _ int64, _ []string, _ []string, _ map[string]any, _ float64, _ int, reason string) (bool, error) {
	r.markBillingFailedCall = true
	r.billingFailureReason = reason
	return true, nil
}

func (r *asyncVideoBillingTaskRepo) InsertTerminalUsageLog(_ context.Context, in *VideoTerminalUsageLogInput) (bool, error) {
	copy := *in
	r.usage = &copy
	return true, nil
}

type asyncVideoBillingUserRepo struct {
	UserRepository
	adjustErr   error
	refundErr   error
	adjustments []float64
	refunds     []float64
}

func (r *asyncVideoBillingUserRepo) AdjustBalance(_ context.Context, _ int64, delta float64) (BalanceChange, error) {
	r.adjustments = append(r.adjustments, delta)
	return BalanceChange{}, r.adjustErr
}

func (r *asyncVideoBillingUserRepo) UpdateBalance(_ context.Context, _ int64, amount float64) error {
	r.refunds = append(r.refunds, amount)
	return r.refundErr
}

func TestAsyncVideoMissingDurationRequiresManualBillingWithoutRefund(t *testing.T) {
	repo := &asyncVideoBillingTaskRepo{}
	users := &asyncVideoBillingUserRepo{}
	svc := NewAsyncVideoService(repo, users, nil)
	task := videoBillingTestTask()

	svc.markSucceeded(context.Background(), task, BillingTypeBalance, []string{"https://example.test/result.mp4"}, map[string]any{"video": map[string]any{"url": "https://example.test/result.mp4"}}, 0, 0)

	require.True(t, repo.markBillingFailedCall)
	require.False(t, repo.markSucceededCalled)
	require.Contains(t, repo.billingFailureReason, "duration")
	require.Empty(t, users.adjustments)
	require.Empty(t, users.refunds)
	require.NotNil(t, repo.usage)
	require.Equal(t, BillingStatusFailed, repo.usage.BillingStatus)
	require.Zero(t, repo.usage.ActualCost)
	require.Equal(t, AsyncVideoStatusSucceeded, task.Status)
}

func TestAsyncVideoExtraChargeFailureRequiresManualBillingWithoutRefund(t *testing.T) {
	repo := &asyncVideoBillingTaskRepo{}
	users := &asyncVideoBillingUserRepo{}
	repo.markSucceededErr = ErrInsufficientBalance
	svc := NewAsyncVideoService(repo, users, nil)
	task := videoBillingTestTask()

	svc.markSucceeded(context.Background(), task, BillingTypeBalance, nil, map[string]any{"duration": 20}, 0, 0)

	require.True(t, repo.markBillingFailedCall)
	require.Empty(t, users.adjustments)
	require.Empty(t, users.refunds)
	require.Equal(t, BillingStatusFailed, repo.usage.BillingStatus)
}

func TestAsyncVideoRefundFailureRequiresManualBillingAndKeepsHold(t *testing.T) {
	repo := &asyncVideoBillingTaskRepo{}
	users := &asyncVideoBillingUserRepo{}
	repo.markSucceededErr = errors.New("balance update failed")
	svc := NewAsyncVideoService(repo, users, nil)
	task := videoBillingTestTask()
	task.HeldCost = 3

	svc.markSucceeded(context.Background(), task, BillingTypeBalance, nil, map[string]any{"duration": 10}, 0, 0)

	require.True(t, repo.markBillingFailedCall)
	require.Empty(t, users.adjustments)
	require.Empty(t, users.refunds)
	require.Equal(t, BillingStatusFailed, repo.usage.BillingStatus)
}

func videoBillingTestTask() *AsyncVideoTask {
	payerID := int64(7)
	resolution := "720p"
	return &AsyncVideoTask{
		ID:                42,
		InternalRequestID: "video-billing-test",
		APIKeyID:          3,
		UserID:            payerID,
		PayerUserID:       &payerID,
		Status:            AsyncVideoStatusRunning,
		BillingType:       BillingTypeBalance,
		HeldCost:          1,
		RateMultiplier:    1,
		UnitPriceSnapshot: 0.1,
		Resolution:        &resolution,
		RequestedModel:    "test/video",
	}
}
