package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
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
