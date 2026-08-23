//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAsyncVideoCompleteRefundCreditsBalanceExactlyOnce(t *testing.T) {
	ctx := context.Background()
	userID := newLedgerTestUser(t, 20)
	repo := NewAsyncVideoTaskRepository(integrationEntClient, integrationDB)
	task := &service.AsyncVideoTask{
		InternalRequestID: fmt.Sprintf("video-refund-%d", time.Now().UnixNano()),
		APIKeyID:          1,
		UserID:            userID,
		PayerUserID:       &userID,
		BalanceSource:     asyncVideoStringPtr(service.BalanceSourceSelf),
		Facade:            service.AsyncVideoFacadeFal,
		RequestedModel:    "test/video",
		Status:            service.AsyncVideoStatusRunning,
		BillingType:       service.BillingTypeBalance,
		HeldCost:          12.5,
		RateMultiplier:    1,
	}
	require.NoError(t, repo.Create(ctx, task))

	failed, err := repo.MarkFailed(ctx, task.ID, service.AsyncVideoStatusFailed, "upstream failed", time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.True(t, failed)

	applied, payerID, err := repo.CompleteRefund(ctx, task.ID, service.AsyncVideoStatusRefunded)
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, userID, payerID)

	replayed, _, err := repo.CompleteRefund(ctx, task.ID, service.AsyncVideoStatusRefunded)
	require.NoError(t, err)
	require.False(t, replayed)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1`, userID).Scan(&balance))
	require.InDelta(t, 32.5, balance, 1e-9)

	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, service.AsyncVideoStatusRefunded, stored.Status)
	require.Equal(t, service.AsyncVideoRefundStatusSucceeded, stored.RefundStatus)
	require.Zero(t, stored.FinalCost)
}

func asyncVideoStringPtr(value string) *string { return &value }
