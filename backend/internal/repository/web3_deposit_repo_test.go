package repository

import (
	"context"
	"database/sql"
	"errors"
	"math"
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
