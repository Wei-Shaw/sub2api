package repository

import (
	"context"
	"database/sql"
	"errors"
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

func TestWeb3DepositRepositoryUpsertDetectedReturnsExistingWithoutChangingFacts(t *testing.T) {
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
	require.Equal(t, original.BlockNumber, stored.BlockNumber)
	require.Equal(t, original.BlockHash, stored.BlockHash)
	require.Equal(t, original.RawAmount, stored.RawAmount)
	require.Equal(t, original.TokenAmount, stored.TokenAmount)
	require.Equal(t, web3deposit.DepositStatusConfirming, stored.Status)
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
