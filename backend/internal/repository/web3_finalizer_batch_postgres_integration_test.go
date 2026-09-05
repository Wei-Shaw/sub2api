//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/stretchr/testify/require"
)

func TestWeb3FinalizerBatchRepositorySetsReviewAndFailureReasonsOnPostgres(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	walletID := fmt.Sprintf("evm_finalizer_%d", suffix)
	_, err := integrationEntClient.Web3DepositWallet.Create().
		SetWalletID(walletID).
		SetAccountPath("m/44'/60'/0'").
		SetXpubFingerprint(fmt.Sprintf("%064x", suffix)).
		Save(ctx)
	require.NoError(t, err)
	user, err := integrationEntClient.User.Create().
		SetEmail(fmt.Sprintf("web3-finalizer-%d@example.com", suffix)).
		SetPasswordHash("password-hash").
		Save(ctx)
	require.NoError(t, err)
	addressValue := fmt.Sprintf("0x%040x", suffix)
	address, err := integrationEntClient.Web3DepositAddress.Create().
		SetUserID(user.ID).
		SetWalletID(walletID).
		SetDerivationIndex(suffix % 2_000_000_000).
		SetAddress(addressValue).
		SetNormalizedAddress(addressValue).
		Save(ctx)
	require.NoError(t, err)

	now := time.Now().UTC()
	scannerKey := fmt.Sprintf("postgres-finalizer-%d", suffix)
	leaseToken := fmt.Sprintf("lease-%d", suffix)
	tokenContract := fmt.Sprintf("0x%040x", suffix+1)
	cursorRepo := NewWeb3ScannerCursorRepository(integrationEntClient)
	_, err = cursorRepo.Initialize(ctx, scannerKey, 1030, tokenContract, 100)
	require.NoError(t, err)
	acquired, err := cursorRepo.AcquireLease(ctx, scannerKey, "integration-test", leaseToken, now, time.Hour)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, cursorRepo.AdvanceScanner(ctx, scannerKey, leaseToken, 110, now))

	depositRepo := NewWeb3DepositRepository(integrationEntClient)
	deposits := make([]web3deposit.Deposit, 3)
	for index := range deposits {
		deposit := testWeb3DepositRecord(uint64(index + 1))
		deposit.UserID = user.ID
		deposit.DepositAddressID = address.ID
		deposit.TokenContract = tokenContract
		deposit.TxHash = fmt.Sprintf("0x%064x", suffix+int64(index))
		deposit.BlockNumber = 105
		deposit.ToAddress = addressValue
		deposits[index], err = depositRepo.Create(ctx, deposit)
		require.NoError(t, err)
	}

	updated, err := NewWeb3FinalizerBatchRepository(integrationEntClient).CommitFinalizedBatch(ctx, web3deposit.FinalizerBatch{
		ScannerKey:       scannerKey,
		LeaseToken:       leaseToken,
		FinalizedThrough: 110,
		Now:              now,
		Decisions: []web3deposit.FinalizedDepositDecision{
			{DepositID: deposits[0].ID, Status: web3deposit.DepositStatusManualReview, ReviewReason: web3deposit.ReviewReasonAboveAutoCreditLimit},
			{DepositID: deposits[1].ID, Status: web3deposit.DepositStatusOrphaned, FailureReason: string(web3deposit.CanonicalMismatchBlockHash)},
			{DepositID: deposits[2].ID, Status: web3deposit.DepositStatusFailed, FailureReason: web3deposit.FailureReasonAmountExceedsPlatformBalance},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 3, updated)

	reviewed, err := depositRepo.GetByEvent(ctx, deposits[0].ChainID, deposits[0].TxHash, deposits[0].LogIndex)
	require.NoError(t, err)
	require.Equal(t, web3deposit.ReviewReasonAboveAutoCreditLimit, *reviewed.ReviewReason)
	orphaned, err := depositRepo.GetByEvent(ctx, deposits[1].ChainID, deposits[1].TxHash, deposits[1].LogIndex)
	require.NoError(t, err)
	require.Equal(t, string(web3deposit.CanonicalMismatchBlockHash), *orphaned.FailureReason)
	failed, err := depositRepo.GetByEvent(ctx, deposits[2].ChainID, deposits[2].TxHash, deposits[2].LogIndex)
	require.NoError(t, err)
	require.Equal(t, web3deposit.FailureReasonAmountExceedsPlatformBalance, *failed.FailureReason)
}
