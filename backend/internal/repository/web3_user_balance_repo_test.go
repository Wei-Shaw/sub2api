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

func newWeb3BalanceTestClient(t *testing.T) *dbent.Client {
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
	return client
}

func TestWeb3UserBalanceRepositoryCreateOrGet(t *testing.T) {
	client := newWeb3BalanceTestClient(t)
	repo := NewWeb3UserBalanceRepository(client)
	ctx := context.Background()

	created, err := repo.CreateOrGet(ctx, 42, web3deposit.AssetKeyUSDT)
	require.NoError(t, err)
	require.Positive(t, created.ID)
	require.Equal(t, "0", created.AvailableAmount)
	require.Equal(t, "0", created.TotalDeposited)
	require.Equal(t, "0", created.TotalTransferred)
	require.Zero(t, created.BalanceVersion)

	loaded, err := repo.CreateOrGet(ctx, 42, web3deposit.AssetKeyUSDT)
	require.NoError(t, err)
	require.Equal(t, created.ID, loaded.ID)

	count, err := client.Web3UserBalance.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestWeb3UserBalanceRepositoryGetReturnsNotFound(t *testing.T) {
	repo := NewWeb3UserBalanceRepository(newWeb3BalanceTestClient(t))

	_, err := repo.GetByUserAndAsset(context.Background(), 42, web3deposit.AssetKeyUSDT)
	require.True(t, errors.Is(err, web3deposit.ErrBalanceNotFound))
}

func TestWeb3UserBalanceRepositoryRejectsInvalidAssetKey(t *testing.T) {
	repo := NewWeb3UserBalanceRepository(newWeb3BalanceTestClient(t))

	_, err := repo.CreateOrGet(context.Background(), 42, "USDT0")
	require.Error(t, err)
}
