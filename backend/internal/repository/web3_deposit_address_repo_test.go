package repository

import (
	"context"
	"database/sql"
	"errors"
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
	testWeb3DepositAddress           = "0x1234567890abcdef1234567890ABCDEF12345678"
	testWeb3DepositNormalizedAddress = "0x1234567890abcdef1234567890abcdef12345678"
)

func newWeb3DepositAddressRepository(t *testing.T) *Web3DepositAddressRepository {
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

	return NewWeb3DepositAddressRepository(client)
}

func TestWeb3DepositAddressRepositoryCreateAndQuery(t *testing.T) {
	repo := newWeb3DepositAddressRepository(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, web3deposit.DepositAddress{
		UserID:            42,
		WalletID:          "evm_deposit_v1",
		DerivationIndex:   7,
		Address:           testWeb3DepositAddress,
		NormalizedAddress: testWeb3DepositNormalizedAddress,
	})
	require.NoError(t, err)
	require.Positive(t, created.ID)
	require.Equal(t, web3deposit.AddressStatusActive, created.Status)
	require.False(t, created.AllocatedAt.IsZero())

	byUser, err := repo.GetByUserAndWallet(ctx, 42, "evm_deposit_v1")
	require.NoError(t, err)
	require.Equal(t, created.ID, byUser.ID)

	byAddress, err := repo.GetByNormalizedAddress(ctx, testWeb3DepositNormalizedAddress)
	require.NoError(t, err)
	require.Equal(t, created.ID, byAddress.ID)
}

func TestWeb3DepositAddressRepositoryRejectsUniqueConflicts(t *testing.T) {
	tests := []struct {
		name        string
		conflicting web3deposit.DepositAddress
	}{
		{
			name: "user and wallet",
			conflicting: web3deposit.DepositAddress{
				UserID:            42,
				WalletID:          "evm_deposit_v1",
				DerivationIndex:   8,
				Address:           "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
				NormalizedAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
			},
		},
		{
			name: "wallet and derivation index",
			conflicting: web3deposit.DepositAddress{
				UserID:            43,
				WalletID:          "evm_deposit_v1",
				DerivationIndex:   7,
				Address:           "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
				NormalizedAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
			},
		},
		{
			name: "normalized address",
			conflicting: web3deposit.DepositAddress{
				UserID:            43,
				WalletID:          "evm_deposit_v2",
				DerivationIndex:   8,
				Address:           testWeb3DepositAddress,
				NormalizedAddress: testWeb3DepositNormalizedAddress,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newWeb3DepositAddressRepository(t)
			ctx := context.Background()
			_, err := repo.Create(ctx, web3deposit.DepositAddress{
				UserID:            42,
				WalletID:          "evm_deposit_v1",
				DerivationIndex:   7,
				Address:           testWeb3DepositAddress,
				NormalizedAddress: testWeb3DepositNormalizedAddress,
			})
			require.NoError(t, err)

			_, err = repo.Create(ctx, tt.conflicting)
			require.ErrorIs(t, err, web3deposit.ErrAddressAlreadyExists)
		})
	}
}

func TestWeb3DepositAddressRepositoryReturnsNotFound(t *testing.T) {
	repo := newWeb3DepositAddressRepository(t)

	_, err := repo.GetByUserAndWallet(context.Background(), 42, "evm_deposit_v1")
	require.True(t, errors.Is(err, web3deposit.ErrAddressNotFound))
}

func TestWeb3DepositAddressRepositoryListsUserHistory(t *testing.T) {
	repo := newWeb3DepositAddressRepository(t)
	ctx := context.Background()
	firstAllocatedAt := time.Now().Add(-time.Hour)
	secondAllocatedAt := time.Now()

	_, err := repo.Create(ctx, web3deposit.DepositAddress{
		UserID:            42,
		WalletID:          "evm_deposit_v1",
		DerivationIndex:   7,
		Address:           testWeb3DepositAddress,
		NormalizedAddress: testWeb3DepositNormalizedAddress,
		AllocatedAt:       firstAllocatedAt,
	})
	require.NoError(t, err)
	_, err = repo.Create(ctx, web3deposit.DepositAddress{
		UserID:            42,
		WalletID:          "evm_deposit_v2",
		DerivationIndex:   8,
		Address:           "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		NormalizedAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		AllocatedAt:       secondAllocatedAt,
	})
	require.NoError(t, err)

	addresses, err := repo.ListByUser(ctx, 42)
	require.NoError(t, err)
	require.Len(t, addresses, 2)
	require.Equal(t, "evm_deposit_v2", addresses[0].WalletID)
	require.Equal(t, "evm_deposit_v1", addresses[1].WalletID)
}

func TestWeb3DepositAddressRepositoryListsActiveByNormalizedAddresses(t *testing.T) {
	repo := newWeb3DepositAddressRepository(t)
	ctx := context.Background()
	active, err := repo.Create(ctx, web3deposit.DepositAddress{
		UserID:            42,
		WalletID:          "evm_deposit_v1",
		DerivationIndex:   7,
		Address:           testWeb3DepositAddress,
		NormalizedAddress: testWeb3DepositNormalizedAddress,
	})
	require.NoError(t, err)
	_, err = repo.Create(ctx, web3deposit.DepositAddress{
		UserID:            43,
		WalletID:          "evm_deposit_v1",
		DerivationIndex:   8,
		Address:           "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		NormalizedAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		Status:            web3deposit.AddressStatusDisabled,
	})
	require.NoError(t, err)

	addresses, err := repo.ListActiveByNormalizedAddresses(ctx, []string{
		testWeb3DepositNormalizedAddress,
		"0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		"0x9999999999999999999999999999999999999999",
	})
	require.NoError(t, err)
	require.Len(t, addresses, 1)
	require.Equal(t, active.ID, addresses[0].ID)
	require.Equal(t, active.UserID, addresses[0].UserID)
	require.Equal(t, active.NormalizedAddress, addresses[0].NormalizedAddress)
	require.Equal(t, web3deposit.AddressStatusActive, addresses[0].Status)
}

func TestWeb3DepositAddressRepositoryListsNoActiveAddressesForEmptyInput(t *testing.T) {
	repo := newWeb3DepositAddressRepository(t)

	addresses, err := repo.ListActiveByNormalizedAddresses(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, addresses)
	require.Empty(t, addresses)
}
