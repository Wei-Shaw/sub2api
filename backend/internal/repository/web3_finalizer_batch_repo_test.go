package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/stretchr/testify/require"
)

func TestWeb3FinalizerBatchRepositoryUpdatesDepositsAndCursorTogether(t *testing.T) {
	client := newWeb3ScannerBatchTestClient(t)
	cursorRepo := NewWeb3ScannerCursorRepository(client)
	depositRepo := NewWeb3DepositRepository(client)
	batchRepo := NewWeb3FinalizerBatchRepository(client)
	ctx := context.Background()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	initializeScannerBatchCursor(t, cursorRepo, ctx, now)
	require.NoError(t, cursorRepo.AdvanceScanner(ctx, testWeb3ScannerKey, "lease-token-01", 110, now))
	ready, err := depositRepo.Create(ctx, testWeb3DepositRecord(7))
	require.NoError(t, err)
	orphanedRecord := testWeb3DepositRecord(8)
	orphanedRecord.TxHash = "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	orphaned, err := depositRepo.Create(ctx, orphanedRecord)
	require.NoError(t, err)
	failedRecord := testWeb3DepositRecord(9)
	failedRecord.TxHash = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	failed, err := depositRepo.Create(ctx, failedRecord)
	require.NoError(t, err)
	reviewRecord := testWeb3DepositRecord(10)
	reviewRecord.TxHash = "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	reviewed, err := depositRepo.Create(ctx, reviewRecord)
	require.NoError(t, err)

	updated, err := batchRepo.CommitFinalizedBatch(ctx, web3deposit.FinalizerBatch{
		ScannerKey:       testWeb3ScannerKey,
		LeaseToken:       "lease-token-01",
		FinalizedThrough: 110,
		Now:              now,
		Decisions: []web3deposit.FinalizedDepositDecision{
			{DepositID: ready.ID, Status: web3deposit.DepositStatusReadyToCredit},
			{DepositID: orphaned.ID, Status: web3deposit.DepositStatusOrphaned, FailureReason: string(web3deposit.CanonicalMismatchBlockHash)},
			{DepositID: failed.ID, Status: web3deposit.DepositStatusFailed, FailureReason: web3deposit.FailureReasonAmountExceedsPlatformBalance},
			{DepositID: reviewed.ID, Status: web3deposit.DepositStatusManualReview, ReviewReason: web3deposit.ReviewReasonAboveAutoCreditLimit},
		},
	})

	require.NoError(t, err)
	require.Equal(t, 4, updated)
	readyStored, err := depositRepo.GetByEvent(ctx, ready.ChainID, ready.TxHash, ready.LogIndex)
	require.NoError(t, err)
	require.Equal(t, web3deposit.DepositStatusReadyToCredit, readyStored.Status)
	require.Equal(t, &now, readyStored.FinalizedAt)
	orphanedStored, err := depositRepo.GetByEvent(ctx, orphaned.ChainID, orphaned.TxHash, orphaned.LogIndex)
	require.NoError(t, err)
	require.Equal(t, web3deposit.DepositStatusOrphaned, orphanedStored.Status)
	require.Equal(t, string(web3deposit.CanonicalMismatchBlockHash), *orphanedStored.FailureReason)
	failedStored, err := depositRepo.GetByEvent(ctx, failed.ChainID, failed.TxHash, failed.LogIndex)
	require.NoError(t, err)
	require.Equal(t, web3deposit.DepositStatusFailed, failedStored.Status)
	require.Equal(t, web3deposit.FailureReasonAmountExceedsPlatformBalance, *failedStored.FailureReason)
	require.Equal(t, &now, failedStored.FinalizedAt)
	reviewedStored, err := depositRepo.GetByEvent(ctx, reviewed.ChainID, reviewed.TxHash, reviewed.LogIndex)
	require.NoError(t, err)
	require.Equal(t, web3deposit.DepositStatusManualReview, reviewedStored.Status)
	require.Equal(t, web3deposit.ReviewReasonAboveAutoCreditLimit, *reviewedStored.ReviewReason)
	require.Equal(t, &now, reviewedStored.FinalizedAt)
	cursor, err := cursorRepo.GetByKey(ctx, testWeb3ScannerKey)
	require.NoError(t, err)
	require.Equal(t, uint64(110), cursor.LastFinalizedBlock)
}

func TestWeb3FinalizerBatchRepositoryRollsBackWhenLeaseIsLost(t *testing.T) {
	client := newWeb3ScannerBatchTestClient(t)
	cursorRepo := NewWeb3ScannerCursorRepository(client)
	depositRepo := NewWeb3DepositRepository(client)
	batchRepo := NewWeb3FinalizerBatchRepository(client)
	ctx := context.Background()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	initializeScannerBatchCursor(t, cursorRepo, ctx, now)
	require.NoError(t, cursorRepo.AdvanceScanner(ctx, testWeb3ScannerKey, "lease-token-01", 110, now))
	deposit, err := depositRepo.Create(ctx, testWeb3DepositRecord(7))
	require.NoError(t, err)

	_, err = batchRepo.CommitFinalizedBatch(ctx, web3deposit.FinalizerBatch{
		ScannerKey:       testWeb3ScannerKey,
		LeaseToken:       "stale-token",
		FinalizedThrough: 110,
		Now:              now,
		Decisions: []web3deposit.FinalizedDepositDecision{
			{DepositID: deposit.ID, Status: web3deposit.DepositStatusReadyToCredit},
		},
	})

	require.ErrorIs(t, err, web3deposit.ErrLeaseNotHeld)
	stored, err := depositRepo.GetByEvent(ctx, deposit.ChainID, deposit.TxHash, deposit.LogIndex)
	require.NoError(t, err)
	require.Equal(t, web3deposit.DepositStatusDetected, stored.Status)
	require.Nil(t, stored.FinalizedAt)
}

func TestWeb3DepositRepositoryListsPendingFinalizationForTargetThroughUpperBound(t *testing.T) {
	repo := newWeb3DepositRepository(t)
	ctx := context.Background()
	historical := testWeb3DepositRecord(6)
	historical.TxHash = "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccd"
	historical.BlockNumber = 90
	_, err := repo.Create(ctx, historical)
	require.NoError(t, err)
	inside := testWeb3DepositRecord(7)
	inside.BlockNumber = 105
	_, err = repo.Create(ctx, inside)
	require.NoError(t, err)
	terminal := testWeb3DepositRecord(8)
	terminal.TxHash = "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	terminal.BlockNumber = 106
	terminal.Status = web3deposit.DepositStatusReadyToCredit
	_, err = repo.Create(ctx, terminal)
	require.NoError(t, err)
	outOfRange := testWeb3DepositRecord(9)
	outOfRange.TxHash = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	outOfRange.BlockNumber = 120
	_, err = repo.Create(ctx, outOfRange)
	require.NoError(t, err)
	otherChain := testWeb3DepositRecord(10)
	otherChain.ChainID = 71
	otherChain.TokenContract = "0x1111111111111111111111111111111111111111"
	otherChain.TxHash = "0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe"
	otherChain.BlockNumber = 105
	_, err = repo.Create(ctx, otherChain)
	require.NoError(t, err)
	otherToken := testWeb3DepositRecord(11)
	otherToken.TokenContract = "0x1111111111111111111111111111111111111111"
	otherToken.TxHash = "0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffd"
	otherToken.BlockNumber = 105
	_, err = repo.Create(ctx, otherToken)
	require.NoError(t, err)

	deposits, err := repo.ListPendingFinalization(ctx, 1030, testWeb3DepositTokenContract, 110)

	require.NoError(t, err)
	require.Len(t, deposits, 2)
	require.Equal(t, []uint64{6, 7}, []uint64{deposits[0].LogIndex, deposits[1].LogIndex})
}
