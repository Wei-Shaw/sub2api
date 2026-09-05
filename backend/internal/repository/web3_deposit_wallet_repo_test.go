package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

const testWeb3DepositWalletFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func newWeb3DepositWalletRepository(t *testing.T) (*Web3DepositWalletRepository, *dbent.Client) {
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

	return NewWeb3DepositWalletRepository(client), client
}

func TestWeb3DepositWalletRepositoryCreateAndGet(t *testing.T) {
	repo, _ := newWeb3DepositWalletRepository(t)

	created, err := repo.Create(context.Background(), web3deposit.WalletMetadata{
		WalletID:        "evm_deposit_v1",
		AccountPath:     "m/44'/60'/0'",
		XPubFingerprint: testWeb3DepositWalletFingerprint,
	})
	require.NoError(t, err)
	require.Positive(t, created.ID)
	require.Equal(t, "evm_deposit_v1", created.WalletID)
	require.Equal(t, web3deposit.WalletStatusActive, created.Status)
	require.Zero(t, created.NextDerivationIndex)
	require.False(t, created.CreatedAt.IsZero())
	require.False(t, created.UpdatedAt.IsZero())

	loaded, err := repo.GetByWalletID(context.Background(), "evm_deposit_v1")
	require.NoError(t, err)
	require.Equal(t, created.ID, loaded.ID)
	require.Equal(t, created.WalletID, loaded.WalletID)
	require.Equal(t, created.AccountPath, loaded.AccountPath)
	require.Equal(t, created.XPubFingerprint, loaded.XPubFingerprint)
	require.Equal(t, created.NextDerivationIndex, loaded.NextDerivationIndex)
	require.Equal(t, created.Status, loaded.Status)
	require.True(t, created.CreatedAt.Equal(loaded.CreatedAt))
	require.True(t, created.UpdatedAt.Equal(loaded.UpdatedAt))
}

func TestWeb3DepositWalletRepositoryRejectsDuplicateWalletID(t *testing.T) {
	repo, _ := newWeb3DepositWalletRepository(t)
	wallet := web3deposit.WalletMetadata{
		WalletID:        "evm_deposit_v1",
		AccountPath:     "m/44'/60'/0'",
		XPubFingerprint: testWeb3DepositWalletFingerprint,
	}

	_, err := repo.Create(context.Background(), wallet)
	require.NoError(t, err)
	_, err = repo.Create(context.Background(), wallet)
	require.True(t, errors.Is(err, web3deposit.ErrWalletAlreadyExists))
}

func TestWeb3DepositWalletRepositoryReturnsNotFound(t *testing.T) {
	repo, _ := newWeb3DepositWalletRepository(t)

	_, err := repo.GetByWalletID(context.Background(), "missing_wallet")
	require.True(t, errors.Is(err, web3deposit.ErrWalletNotFound))
}

func TestWeb3DepositWalletRepositoryValidatesMetadata(t *testing.T) {
	repo, _ := newWeb3DepositWalletRepository(t)

	_, err := repo.Create(context.Background(), web3deposit.WalletMetadata{
		WalletID:            "Invalid-Wallet",
		AccountPath:         "m/44'/60'/0'",
		XPubFingerprint:     testWeb3DepositWalletFingerprint,
		NextDerivationIndex: web3deposit.MaxDerivationIndexExclusive,
		Status:              web3deposit.WalletStatus("unknown"),
	})
	require.Error(t, err)
	require.False(t, errors.Is(err, web3deposit.ErrWalletAlreadyExists))
}

func TestWeb3DepositWalletRepositoryReservesDerivationIndexes(t *testing.T) {
	repo, _ := newWeb3DepositWalletRepository(t)
	ctx := context.Background()
	wallet := createWeb3DepositWallet(t, repo, web3deposit.WalletMetadata{
		WalletID:        "evm_deposit_v1",
		AccountPath:     "m/44'/60'/0'",
		XPubFingerprint: testWeb3DepositWalletFingerprint,
	})

	first, err := repo.ReserveDerivationIndex(ctx, wallet)
	require.NoError(t, err)
	require.EqualValues(t, 0, first)

	second, err := repo.ReserveDerivationIndex(ctx, wallet)
	require.NoError(t, err)
	require.EqualValues(t, 1, second)

	stored, err := repo.GetByWalletID(ctx, wallet.WalletID)
	require.NoError(t, err)
	require.EqualValues(t, 2, stored.NextDerivationIndex)
}

func TestWeb3DepositWalletRepositoryReservesUniqueIndexesConcurrently(t *testing.T) {
	repo, _ := newWeb3DepositWalletRepository(t)
	ctx := context.Background()
	wallet := createWeb3DepositWallet(t, repo, web3deposit.WalletMetadata{
		WalletID:        "evm_deposit_v1",
		AccountPath:     "m/44'/60'/0'",
		XPubFingerprint: testWeb3DepositWalletFingerprint,
	})

	const reservations = 100
	indexes := make(chan int64, reservations)
	errorsCh := make(chan error, reservations)
	var waitGroup sync.WaitGroup
	for range reservations {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			index, err := repo.ReserveDerivationIndex(ctx, wallet)
			if err != nil {
				errorsCh <- err
				return
			}
			indexes <- index
		}()
	}
	waitGroup.Wait()
	close(indexes)
	close(errorsCh)

	for err := range errorsCh {
		require.NoError(t, err)
	}
	reserved := make([]int64, 0, reservations)
	for index := range indexes {
		reserved = append(reserved, index)
	}
	require.Len(t, reserved, reservations)
	sort.Slice(reserved, func(left, right int) bool { return reserved[left] < reserved[right] })
	for index, reservedIndex := range reserved {
		require.EqualValues(t, index, reservedIndex)
	}

	stored, err := repo.GetByWalletID(ctx, wallet.WalletID)
	require.NoError(t, err)
	require.EqualValues(t, reservations, stored.NextDerivationIndex)
}

func TestWeb3DepositWalletRepositoryDoesNotReuseConsumedIndex(t *testing.T) {
	repo, _ := newWeb3DepositWalletRepository(t)
	wallet := createWeb3DepositWallet(t, repo, web3deposit.WalletMetadata{
		WalletID:        "evm_deposit_v1",
		AccountPath:     "m/44'/60'/0'",
		XPubFingerprint: testWeb3DepositWalletFingerprint,
	})

	consumed, err := repo.ReserveDerivationIndex(context.Background(), wallet)
	require.NoError(t, err)
	require.EqualValues(t, 0, consumed)

	next, err := repo.ReserveDerivationIndex(context.Background(), wallet)
	require.NoError(t, err)
	require.EqualValues(t, 1, next)
}

func TestWeb3DepositWalletRepositoryRejectsUnavailableWallet(t *testing.T) {
	tests := []struct {
		name      string
		wallet    web3deposit.WalletMetadata
		mutate    func(*web3deposit.WalletMetadata)
		wantError error
	}{
		{
			name: "disabled",
			wallet: web3deposit.WalletMetadata{
				WalletID:        "disabled_wallet",
				AccountPath:     "m/44'/60'/0'",
				XPubFingerprint: testWeb3DepositWalletFingerprint,
				Status:          web3deposit.WalletStatusDisabled,
			},
			wantError: web3deposit.ErrWalletDisabled,
		},
		{
			name: "account path changed",
			wallet: web3deposit.WalletMetadata{
				WalletID:        "path_wallet",
				AccountPath:     "m/44'/60'/0'",
				XPubFingerprint: testWeb3DepositWalletFingerprint,
			},
			mutate: func(wallet *web3deposit.WalletMetadata) {
				wallet.AccountPath = "m/44'/60'/1'"
			},
			wantError: web3deposit.ErrWalletAccountPathMismatch,
		},
		{
			name: "fingerprint changed",
			wallet: web3deposit.WalletMetadata{
				WalletID:        "fingerprint_wallet",
				AccountPath:     "m/44'/60'/0'",
				XPubFingerprint: testWeb3DepositWalletFingerprint,
			},
			mutate: func(wallet *web3deposit.WalletMetadata) {
				wallet.XPubFingerprint = strings.Repeat("f", 64)
			},
			wantError: web3deposit.ErrWalletFingerprintMismatch,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, _ := newWeb3DepositWalletRepository(t)
			verified := createWeb3DepositWallet(t, repo, test.wallet)
			if test.mutate != nil {
				test.mutate(&verified)
			}

			_, err := repo.ReserveDerivationIndex(context.Background(), verified)
			require.ErrorIs(t, err, test.wantError)
		})
	}
}

func TestWeb3DepositWalletRepositoryRejectsExhaustedIndexes(t *testing.T) {
	repo, _ := newWeb3DepositWalletRepository(t)
	wallet := createWeb3DepositWallet(t, repo, web3deposit.WalletMetadata{
		WalletID:            "evm_deposit_v1",
		AccountPath:         "m/44'/60'/0'",
		XPubFingerprint:     testWeb3DepositWalletFingerprint,
		NextDerivationIndex: web3deposit.MaxDerivationIndexExclusive - 1,
	})

	last, err := repo.ReserveDerivationIndex(context.Background(), wallet)
	require.NoError(t, err)
	require.Equal(t, web3deposit.MaxDerivationIndexExclusive-1, last)

	_, err = repo.ReserveDerivationIndex(context.Background(), wallet)
	require.ErrorIs(t, err, web3deposit.ErrDerivationIndexExhausted)
}

func TestWeb3DepositWalletRepositoryReserveReturnsNotFound(t *testing.T) {
	repo, _ := newWeb3DepositWalletRepository(t)

	_, err := repo.ReserveDerivationIndex(context.Background(), web3deposit.WalletMetadata{WalletID: "missing_wallet"})
	require.ErrorIs(t, err, web3deposit.ErrWalletNotFound)
}

func createWeb3DepositWallet(
	t *testing.T,
	repo *Web3DepositWalletRepository,
	wallet web3deposit.WalletMetadata,
) web3deposit.WalletMetadata {
	t.Helper()
	created, err := repo.Create(context.Background(), wallet)
	require.NoError(t, err)
	return created
}
