package repository

import (
	"context"
	"database/sql"
	"math/big"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestWeb3ScannerBatchRepositoryCommitsDepositsAndCursorTogether(t *testing.T) {
	client := newWeb3ScannerBatchTestClient(t)
	cursorRepo := NewWeb3ScannerCursorRepository(client)
	batchRepo := NewWeb3ScannerBatchRepository(client)
	ctx := context.Background()
	now := time.Date(2026, time.August, 8, 10, 0, 0, 0, time.UTC)
	initializeScannerBatchCursor(t, cursorRepo, ctx, now)

	batch := testWeb3ScannerBatch(t, now,
		testWeb3ScannerBatchMatch(t, 7, 1_000_000),
		testWeb3ScannerBatchMatch(t, 8, 2_000_000),
	)
	deposits, err := batchRepo.CommitDetectedBatch(ctx, batch)
	require.NoError(t, err)
	require.Len(t, deposits, 2)
	require.Equal(t, []uint64{7, 8}, []uint64{deposits[0].LogIndex, deposits[1].LogIndex})

	cursor, err := cursorRepo.GetByKey(ctx, testWeb3ScannerKey)
	require.NoError(t, err)
	require.Equal(t, uint64(110), cursor.LastScannedBlock)
	require.NotNil(t, cursor.LastSuccessAt)

	stored, err := NewWeb3DepositRepository(client).ListByUser(ctx, 42)
	require.NoError(t, err)
	require.Len(t, stored, 2)
}

func TestWeb3ScannerBatchRepositoryAdvancesEmptyBatch(t *testing.T) {
	client := newWeb3ScannerBatchTestClient(t)
	cursorRepo := NewWeb3ScannerCursorRepository(client)
	batchRepo := NewWeb3ScannerBatchRepository(client)
	ctx := context.Background()
	now := time.Date(2026, time.August, 8, 10, 0, 0, 0, time.UTC)
	initializeScannerBatchCursor(t, cursorRepo, ctx, now)

	deposits, err := batchRepo.CommitDetectedBatch(ctx, testWeb3ScannerBatch(t, now))
	require.NoError(t, err)
	require.NotNil(t, deposits)
	require.Empty(t, deposits)
	cursor, err := cursorRepo.GetByKey(ctx, testWeb3ScannerKey)
	require.NoError(t, err)
	require.Equal(t, uint64(110), cursor.LastScannedBlock)
}

func TestWeb3ScannerBatchRepositoryRollsBackDepositsWhenBatchWriteFails(t *testing.T) {
	client := newWeb3ScannerBatchTestClient(t)
	cursorRepo := NewWeb3ScannerCursorRepository(client)
	batchRepo := NewWeb3ScannerBatchRepository(client)
	ctx := context.Background()
	now := time.Date(2026, time.August, 8, 10, 0, 0, 0, time.UTC)
	initializeScannerBatchCursor(t, cursorRepo, ctx, now)
	invalid := testWeb3ScannerBatchMatch(t, 8, 1_000_000)
	invalid.Event.ID.ChainID = 71

	deposits, err := batchRepo.CommitDetectedBatch(ctx, testWeb3ScannerBatch(t, now,
		testWeb3ScannerBatchMatch(t, 7, 1_000_000),
		invalid,
	))
	require.ErrorIs(t, err, web3deposit.ErrDepositEventChainMismatch)
	require.Nil(t, deposits)
	assertWeb3ScannerBatchRolledBack(t, client, cursorRepo, ctx)
}

func TestWeb3ScannerBatchRepositoryRollsBackDepositsWhenLeaseIsNotHeld(t *testing.T) {
	client := newWeb3ScannerBatchTestClient(t)
	cursorRepo := NewWeb3ScannerCursorRepository(client)
	batchRepo := NewWeb3ScannerBatchRepository(client)
	ctx := context.Background()
	now := time.Date(2026, time.August, 8, 10, 0, 0, 0, time.UTC)
	initializeScannerBatchCursor(t, cursorRepo, ctx, now)
	batch := testWeb3ScannerBatch(t, now, testWeb3ScannerBatchMatch(t, 7, 1_000_000))
	batch.LeaseToken = "stale-lease-token"

	deposits, err := batchRepo.CommitDetectedBatch(ctx, batch)
	require.ErrorIs(t, err, web3deposit.ErrLeaseNotHeld)
	require.Nil(t, deposits)
	assertWeb3ScannerBatchRolledBack(t, client, cursorRepo, ctx)
}

func TestWeb3ScannerBatchRepositoryRejectsCursorIdentityMismatch(t *testing.T) {
	client := newWeb3ScannerBatchTestClient(t)
	cursorRepo := NewWeb3ScannerCursorRepository(client)
	batchRepo := NewWeb3ScannerBatchRepository(client)
	ctx := context.Background()
	now := time.Date(2026, time.August, 8, 10, 0, 0, 0, time.UTC)
	initializeScannerBatchCursor(t, cursorRepo, ctx, now)
	batch := testWeb3ScannerBatch(t, now, testWeb3ScannerBatchMatch(t, 7, 1_000_000))
	batch.Config.TokenAddress = common.HexToAddress("0xffffffffffffffffffffffffffffffffffffffff")

	deposits, err := batchRepo.CommitDetectedBatch(ctx, batch)
	require.ErrorIs(t, err, web3deposit.ErrCursorIdentityConflict)
	require.Nil(t, deposits)
	assertWeb3ScannerBatchRolledBack(t, client, cursorRepo, ctx)
}

func TestWeb3ScannerBatchRepositoryRetriesIdempotently(t *testing.T) {
	client := newWeb3ScannerBatchTestClient(t)
	cursorRepo := NewWeb3ScannerCursorRepository(client)
	batchRepo := NewWeb3ScannerBatchRepository(client)
	ctx := context.Background()
	now := time.Date(2026, time.August, 8, 10, 0, 0, 0, time.UTC)
	initializeScannerBatchCursor(t, cursorRepo, ctx, now)
	batch := testWeb3ScannerBatch(t, now, testWeb3ScannerBatchMatch(t, 7, 1_000_000))

	first, err := batchRepo.CommitDetectedBatch(ctx, batch)
	require.NoError(t, err)
	second, err := batchRepo.CommitDetectedBatch(ctx, batch)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Len(t, second, 1)
	require.Equal(t, first[0].ID, second[0].ID)

	stored, err := NewWeb3DepositRepository(client).ListByUser(ctx, 42)
	require.NoError(t, err)
	require.Len(t, stored, 1)
}

func TestWeb3ScannerBatchRepositoryPersistsMaximumUint256AndAdvancesCursor(t *testing.T) {
	client := newWeb3ScannerBatchTestClient(t)
	cursorRepo := NewWeb3ScannerCursorRepository(client)
	batchRepo := NewWeb3ScannerBatchRepository(client)
	ctx := context.Background()
	now := time.Date(2026, time.August, 8, 10, 0, 0, 0, time.UTC)
	initializeScannerBatchCursor(t, cursorRepo, ctx, now)
	maximumUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	batch := testWeb3ScannerBatch(t, now, testWeb3ScannerBatchBigAmountMatch(t, 7, maximumUint256))

	deposits, err := batchRepo.CommitDetectedBatch(ctx, batch)
	require.NoError(t, err)
	require.Len(t, deposits, 1)
	require.Equal(t, maximumUint256.String(), deposits[0].RawAmount)
	require.Equal(t, "115792089237316195423570985008687907853269984665640564039457584007913129.639935", deposits[0].TokenAmount)

	cursor, err := cursorRepo.GetByKey(ctx, testWeb3ScannerKey)
	require.NoError(t, err)
	require.Equal(t, uint64(110), cursor.LastScannedBlock)
}

func newWeb3ScannerBatchTestClient(t *testing.T) *dbent.Client {
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

func initializeScannerBatchCursor(
	t *testing.T,
	repo *Web3ScannerCursorRepository,
	ctx context.Context,
	now time.Time,
) {
	t.Helper()
	_, err := repo.Initialize(ctx, testWeb3ScannerKey, 1030, testWeb3ScannerContract, 100)
	require.NoError(t, err)
	acquired, err := repo.AcquireLease(ctx, testWeb3ScannerKey, "scanner-01", "lease-token-01", now, time.Hour)
	require.NoError(t, err)
	require.True(t, acquired)
}

func testWeb3ScannerBatch(t *testing.T, now time.Time, matches ...web3deposit.MatchedTransferEvent) web3deposit.ScannerBatch {
	t.Helper()
	return web3deposit.ScannerBatch{
		ScannerKey:     testWeb3ScannerKey,
		LeaseToken:     "lease-token-01",
		ScannedThrough: 110,
		Now:            now,
		Config: web3deposit.ChainConfig{
			ChainID:       1030,
			TokenAddress:  common.HexToAddress(testWeb3ScannerContract),
			TokenDecimals: web3deposit.USDT0Decimals,
		},
		Matches: matches,
	}
}

func testWeb3ScannerBatchMatch(t *testing.T, logIndex uint64, rawAmount int64) web3deposit.MatchedTransferEvent {
	return testWeb3ScannerBatchBigAmountMatch(t, logIndex, big.NewInt(rawAmount))
}

func testWeb3ScannerBatchBigAmountMatch(t *testing.T, logIndex uint64, rawAmount *big.Int) web3deposit.MatchedTransferEvent {
	t.Helper()
	event, err := web3deposit.NewTransferEvent(
		web3deposit.DepositEventID{
			ChainID:  1030,
			TxHash:   common.BigToHash(new(big.Int).SetUint64(logIndex)),
			LogIndex: logIndex,
		},
		105,
		common.BigToHash(big.NewInt(105)),
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
		rawAmount,
	)
	require.NoError(t, err)
	return web3deposit.MatchedTransferEvent{Event: event, DepositAddressID: 9, UserID: 42}
}

func assertWeb3ScannerBatchRolledBack(
	t *testing.T,
	client *dbent.Client,
	cursorRepo *Web3ScannerCursorRepository,
	ctx context.Context,
) {
	t.Helper()
	stored, err := NewWeb3DepositRepository(client).ListByUser(ctx, 42)
	require.NoError(t, err)
	require.Empty(t, stored)
	cursor, err := cursorRepo.GetByKey(ctx, testWeb3ScannerKey)
	require.NoError(t, err)
	require.Equal(t, uint64(100), cursor.LastScannedBlock)
}
