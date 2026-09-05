package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

const (
	testWeb3DepositTxHash        = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testWeb3DepositBlockHash     = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testWeb3DepositTokenContract = "0xcccccccccccccccccccccccccccccccccccccccc"
	testWeb3DepositFromAddress   = "0xdddddddddddddddddddddddddddddddddddddddd"
	testWeb3DepositToAddress     = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
)

func newWeb3DepositRepository(t *testing.T) *Web3DepositRepository {
	t.Helper()

	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared&_fk=1"
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	driver := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(driver)))
	t.Cleanup(func() { _ = client.Close() })

	return NewWeb3DepositRepository(client)
}

func TestWeb3DepositRepositoryCreateAndGetByEvent(t *testing.T) {
	repo := newWeb3DepositRepository(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, testWeb3DepositRecord(7))
	require.NoError(t, err)
	require.Positive(t, created.ID)
	require.Equal(t, web3deposit.DepositStatusDetected, created.Status)
	require.Equal(t, "1234567", created.RawAmount)
	require.Equal(t, "1.234567", created.TokenAmount)
	require.False(t, created.DetectedAt.IsZero())

	loaded, err := repo.GetByEvent(ctx, 1030, testWeb3DepositTxHash, 7)
	require.NoError(t, err)
	require.Equal(t, created.ID, loaded.ID)
	require.Equal(t, created.TxHash, loaded.TxHash)
}

func TestWeb3DepositRepositoryEventIdentityIncludesLogIndex(t *testing.T) {
	repo := newWeb3DepositRepository(t)
	ctx := context.Background()

	_, err := repo.Create(ctx, testWeb3DepositRecord(7))
	require.NoError(t, err)
	_, err = repo.Create(ctx, testWeb3DepositRecord(8))
	require.NoError(t, err)
	differentChain := testWeb3DepositRecord(7)
	differentChain.ChainID = 71
	_, err = repo.Create(ctx, differentChain)
	require.NoError(t, err)

	_, err = repo.Create(ctx, testWeb3DepositRecord(7))
	require.ErrorIs(t, err, web3deposit.ErrDepositAlreadyExists)

	deposits, err := repo.ListByUser(ctx, 42)
	require.NoError(t, err)
	require.Len(t, deposits, 3)
}

func TestWeb3DepositRepositoryUpsertDetectedRefreshesPreFinalizationCanonicalFacts(t *testing.T) {
	repo := newWeb3DepositRepository(t)
	ctx := context.Background()
	original := testWeb3DepositRecord(7)
	original.Status = web3deposit.DepositStatusConfirming
	created, err := repo.Create(ctx, original)
	require.NoError(t, err)

	duplicate := testWeb3DepositRecord(7)
	duplicate.UserID = 99
	duplicate.DepositAddressID = 88
	duplicate.BlockNumber = 54321
	duplicate.BlockHash = "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	duplicate.RawAmount = "7654321"
	duplicate.TokenAmount = "7.654321"

	stored, err := repo.UpsertDetected(ctx, duplicate)
	require.NoError(t, err)
	require.Equal(t, created.ID, stored.ID)
	require.Equal(t, original.UserID, stored.UserID)
	require.Equal(t, original.DepositAddressID, stored.DepositAddressID)
	require.Equal(t, duplicate.BlockNumber, stored.BlockNumber)
	require.Equal(t, duplicate.BlockHash, stored.BlockHash)
	require.Equal(t, duplicate.RawAmount, stored.RawAmount)
	require.Equal(t, duplicate.TokenAmount, stored.TokenAmount)
	require.Equal(t, web3deposit.DepositStatusConfirming, stored.Status)
}

func TestWeb3DepositRepositoryUpsertDetectedDoesNotRewriteFinalizedFacts(t *testing.T) {
	repo := newWeb3DepositRepository(t)
	ctx := context.Background()
	original := testWeb3DepositRecord(7)
	original.Status = web3deposit.DepositStatusReadyToCredit
	created, err := repo.Create(ctx, original)
	require.NoError(t, err)

	duplicate := original
	duplicate.BlockNumber = 54321
	duplicate.BlockHash = "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	duplicate.RawAmount = "7654321"
	duplicate.TokenAmount = "7.654321"

	stored, err := repo.UpsertDetected(ctx, duplicate)

	require.NoError(t, err)
	require.Equal(t, created.BlockNumber, stored.BlockNumber)
	require.Equal(t, created.BlockHash, stored.BlockHash)
	require.Equal(t, created.RawAmount, stored.RawAmount)
	require.Equal(t, created.TokenAmount, stored.TokenAmount)
}

func TestWeb3DepositRepositoryUpsertDetectedCreatesDifferentLogIndexes(t *testing.T) {
	repo := newWeb3DepositRepository(t)
	ctx := context.Background()

	first, err := repo.UpsertDetected(ctx, testWeb3DepositRecord(7))
	require.NoError(t, err)
	second, err := repo.UpsertDetected(ctx, testWeb3DepositRecord(8))
	require.NoError(t, err)

	require.NotEqual(t, first.ID, second.ID)
	require.Equal(t, uint64(7), first.LogIndex)
	require.Equal(t, uint64(8), second.LogIndex)
}

func TestWeb3DepositRepositoryReturnsNotFound(t *testing.T) {
	repo := newWeb3DepositRepository(t)

	_, err := repo.GetByEvent(context.Background(), 1030, testWeb3DepositTxHash, 7)
	require.True(t, errors.Is(err, web3deposit.ErrDepositNotFound))
}

func TestWeb3DepositRepositoryPaginatesAndScopesUserDeposits(t *testing.T) {
	repo := newWeb3DepositRepository(t)
	ctx := context.Background()
	for logIndex := uint64(1); logIndex <= 3; logIndex++ {
		deposit := testWeb3DepositRecord(logIndex)
		deposit.TxHash = fmt.Sprintf("0x%064x", logIndex)
		_, err := repo.Create(ctx, deposit)
		require.NoError(t, err)
	}
	other := testWeb3DepositRecord(4)
	other.UserID = 99
	other.TxHash = fmt.Sprintf("0x%064x", 4)
	_, err := repo.Create(ctx, other)
	require.NoError(t, err)
	testnet := testWeb3DepositRecord(5)
	testnet.ChainID = 71
	testnet.TokenContract = "0x1111111111111111111111111111111111111111"
	testnet.TxHash = fmt.Sprintf("0x%064x", 5)
	_, err = repo.Create(ctx, testnet)
	require.NoError(t, err)

	page, total, err := repo.ListUserDeposits(ctx, 42, web3deposit.UserDepositFilter{Page: 1, PageSize: 2})
	require.NoError(t, err)
	require.Equal(t, int64(4), total)
	require.Len(t, page, 2)
	require.Greater(t, page[0].ID, page[1].ID)

	owned, err := repo.GetUserDeposit(ctx, 42, page[0].ID)
	require.NoError(t, err)
	require.Equal(t, int64(42), owned.UserID)

	_, err = repo.GetUserDeposit(ctx, 99, page[0].ID)
	require.ErrorIs(t, err, web3deposit.ErrDepositNotFound)

	targeted, targetedTotal, err := repo.ListUserDeposits(ctx, 42, web3deposit.UserDepositFilter{
		ChainID: 71, TokenContract: testnet.TokenContract, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), targetedTotal)
	require.Len(t, targeted, 1)
	require.Equal(t, uint64(71), targeted[0].ChainID)
	require.Equal(t, testnet.TokenContract, targeted[0].TokenContract)
}

func TestWeb3DepositRepositoryCountsStatusesInDatabaseAndScopesTarget(t *testing.T) {
	repo := newWeb3DepositRepository(t)
	ctx := context.Background()
	for index, status := range []web3deposit.DepositStatus{
		web3deposit.DepositStatusDetected,
		web3deposit.DepositStatusDetected,
		web3deposit.DepositStatusManualReview,
	} {
		deposit := testWeb3DepositRecord(uint64(index + 1))
		deposit.TxHash = fmt.Sprintf("0x%064x", index+1)
		deposit.Status = status
		_, err := repo.Create(ctx, deposit)
		require.NoError(t, err)
	}
	otherTarget := testWeb3DepositRecord(9)
	otherTarget.ChainID = 71
	otherTarget.TokenContract = "0x1111111111111111111111111111111111111111"
	otherTarget.TxHash = fmt.Sprintf("0x%064x", 9)
	otherTarget.Status = web3deposit.DepositStatusDetected
	_, err := repo.Create(ctx, otherTarget)
	require.NoError(t, err)

	all, err := repo.CountAdminDepositsByStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(3), all[web3deposit.DepositStatusDetected])
	require.Equal(t, int64(1), all[web3deposit.DepositStatusManualReview])
	targeted, err := repo.CountAdminDepositsByStatusForTarget(ctx, 1030, testWeb3DepositTokenContract)
	require.NoError(t, err)
	require.Equal(t, int64(2), targeted[web3deposit.DepositStatusDetected])
	require.Equal(t, int64(1), targeted[web3deposit.DepositStatusManualReview])
}

func TestWeb3DepositRepositoryAdminStateTransitionsAreConditional(t *testing.T) {
	repo := newWeb3DepositRepository(t)
	ctx := context.Background()

	reviewed := testWeb3DepositRecord(21)
	reviewed.TxHash = fmt.Sprintf("0x%064x", 21)
	reviewed.Status = web3deposit.DepositStatusManualReview
	reviewedID, err := repo.Create(ctx, reviewed)
	require.NoError(t, err)
	require.NoError(t, repo.ApproveReviewedDeposit(ctx, reviewedID.ID))
	approved, err := repo.GetAdminDeposit(ctx, reviewedID.ID)
	require.NoError(t, err)
	require.Equal(t, web3deposit.DepositStatusReadyToCredit, approved.Status)
	require.ErrorIs(t, repo.ApproveReviewedDeposit(ctx, reviewedID.ID), web3deposit.ErrAdminDepositStateConflict)

	ignored := testWeb3DepositRecord(22)
	ignored.TxHash = fmt.Sprintf("0x%064x", 22)
	ignored.Status = web3deposit.DepositStatusManualReview
	ignoredID, err := repo.Create(ctx, ignored)
	require.NoError(t, err)
	require.NoError(t, repo.IgnoreReviewedDeposit(ctx, ignoredID.ID, " duplicate external settlement "))
	ignoredStored, err := repo.GetAdminDeposit(ctx, ignoredID.ID)
	require.NoError(t, err)
	require.Equal(t, web3deposit.DepositStatusIgnored, ignoredStored.Status)
	require.Equal(t, "duplicate external settlement", *ignoredStored.ReviewReason)

	failed := testWeb3DepositRecord(23)
	failed.TxHash = fmt.Sprintf("0x%064x", 23)
	failed.Status = web3deposit.DepositStatusFailed
	failure := "temporary database failure"
	failed.FailureReason = &failure
	failedID, err := repo.Create(ctx, failed)
	require.NoError(t, err)
	require.NoError(t, repo.RetryFailedDeposit(ctx, failedID.ID))
	retried, err := repo.GetAdminDeposit(ctx, failedID.ID)
	require.NoError(t, err)
	require.Equal(t, web3deposit.DepositStatusReadyToCredit, retried.Status)
	require.Nil(t, retried.FailureReason)

	permanentFailure := testWeb3DepositRecord(24)
	permanentFailure.TxHash = fmt.Sprintf("0x%064x", 24)
	permanentFailure.Status = web3deposit.DepositStatusFailed
	overflowReason := web3deposit.FailureReasonAmountExceedsPlatformBalance
	permanentFailure.FailureReason = &overflowReason
	permanentFailureID, err := repo.Create(ctx, permanentFailure)
	require.NoError(t, err)
	require.ErrorIs(t, repo.RetryFailedDeposit(ctx, permanentFailureID.ID), web3deposit.ErrAdminDepositStateConflict)
	permanentStored, err := repo.GetAdminDeposit(ctx, permanentFailureID.ID)
	require.NoError(t, err)
	require.Equal(t, web3deposit.DepositStatusFailed, permanentStored.Status)
	require.Equal(t, overflowReason, *permanentStored.FailureReason)
}

func TestWeb3DepositRepositoryRejectsInvalidValues(t *testing.T) {
	repo := newWeb3DepositRepository(t)
	ctx := context.Background()

	invalidRawAmount := testWeb3DepositRecord(7)
	invalidRawAmount.RawAmount = "0"
	_, err := repo.Create(ctx, invalidRawAmount)
	require.Error(t, err)

	overflowChainID := testWeb3DepositRecord(7)
	overflowChainID.ChainID = uint64(math.MaxInt64) + 1
	_, err = repo.Create(ctx, overflowChainID)
	require.ErrorContains(t, err, "exceeds PostgreSQL BIGINT")
}

func TestWeb3DepositRepositoryChecksCreditEligibility(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(context.Context, *Web3DepositRepository, *dbent.User, *dbent.Web3DepositAddress, *web3deposit.Deposit)
		wantReason string
	}{
		{name: "eligible"},
		{
			name: "missing user",
			configure: func(_ context.Context, _ *Web3DepositRepository, _ *dbent.User, _ *dbent.Web3DepositAddress, deposit *web3deposit.Deposit) {
				deposit.UserID = 999
			},
			wantReason: web3deposit.ReviewReasonUserMissing,
		},
		{
			name: "deleted user",
			configure: func(ctx context.Context, repo *Web3DepositRepository, user *dbent.User, _ *dbent.Web3DepositAddress, _ *web3deposit.Deposit) {
				require.NoError(t, repo.client.User.UpdateOneID(user.ID).SetDeletedAt(time.Now()).Exec(ctx))
			},
			wantReason: web3deposit.ReviewReasonUserDeleted,
		},
		{
			name: "disabled user",
			configure: func(ctx context.Context, repo *Web3DepositRepository, user *dbent.User, _ *dbent.Web3DepositAddress, _ *web3deposit.Deposit) {
				require.NoError(t, repo.client.User.UpdateOneID(user.ID).SetStatus("disabled").Exec(ctx))
			},
			wantReason: web3deposit.ReviewReasonUserInactive,
		},
		{
			name: "missing address",
			configure: func(_ context.Context, _ *Web3DepositRepository, _ *dbent.User, _ *dbent.Web3DepositAddress, deposit *web3deposit.Deposit) {
				deposit.DepositAddressID = 999
			},
			wantReason: web3deposit.ReviewReasonAddressMissing,
		},
		{
			name: "disabled address",
			configure: func(ctx context.Context, repo *Web3DepositRepository, _ *dbent.User, address *dbent.Web3DepositAddress, _ *web3deposit.Deposit) {
				require.NoError(t, repo.client.Web3DepositAddress.UpdateOneID(address.ID).SetStatus(string(web3deposit.AddressStatusDisabled)).Exec(ctx))
			},
			wantReason: web3deposit.ReviewReasonAddressDisabled,
		},
		{
			name: "address user mismatch",
			configure: func(ctx context.Context, repo *Web3DepositRepository, _ *dbent.User, _ *dbent.Web3DepositAddress, deposit *web3deposit.Deposit) {
				otherUser, err := repo.client.User.Create().
					SetEmail("address-owner-mismatch@example.com").
					SetPasswordHash("password-hash").
					Save(ctx)
				require.NoError(t, err)
				deposit.UserID = otherUser.ID
			},
			wantReason: web3deposit.ReviewReasonAddressUserMismatch,
		},
		{
			name: "address mismatch",
			configure: func(_ context.Context, _ *Web3DepositRepository, _ *dbent.User, _ *dbent.Web3DepositAddress, deposit *web3deposit.Deposit) {
				deposit.ToAddress = "0xffffffffffffffffffffffffffffffffffffffff"
			},
			wantReason: web3deposit.ReviewReasonAddressMismatch,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newWeb3DepositRepository(t)
			ctx := context.Background()
			userEntity, err := repo.client.User.Create().
				SetEmail(test.name + "@example.com").
				SetPasswordHash("password-hash").
				Save(ctx)
			require.NoError(t, err)
			addressEntity, err := repo.client.Web3DepositAddress.Create().
				SetUserID(userEntity.ID).
				SetWalletID("evm_deposit_v1").
				SetDerivationIndex(1).
				SetAddress(testWeb3DepositToAddress).
				SetNormalizedAddress(testWeb3DepositToAddress).
				Save(ctx)
			require.NoError(t, err)
			deposit := web3deposit.Deposit{
				UserID:           userEntity.ID,
				DepositAddressID: addressEntity.ID,
				ToAddress:        testWeb3DepositToAddress,
			}
			if test.configure != nil {
				test.configure(ctx, repo, userEntity, addressEntity, &deposit)
			}

			eligibility, err := repo.CheckCreditEligibility(ctx, deposit)

			require.NoError(t, err)
			require.Equal(t, test.wantReason == "", eligibility.Eligible)
			require.Equal(t, test.wantReason, eligibility.ReviewReason)
		})
	}
}

func testWeb3DepositRecord(logIndex uint64) web3deposit.Deposit {
	return web3deposit.Deposit{
		UserID:           42,
		DepositAddressID: 9,
		ChainID:          1030,
		TokenContract:    testWeb3DepositTokenContract,
		TxHash:           testWeb3DepositTxHash,
		LogIndex:         logIndex,
		BlockNumber:      12345,
		BlockHash:        testWeb3DepositBlockHash,
		FromAddress:      testWeb3DepositFromAddress,
		ToAddress:        testWeb3DepositToAddress,
		RawAmount:        "1234567",
		TokenDecimals:    6,
		TokenAmount:      "1.234567",
	}
}
