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
	testWeb3ScannerKey      = "conflux_espace_mainnet:usdt0"
	testWeb3ScannerContract = "0xcccccccccccccccccccccccccccccccccccccccc"
)

func newWeb3ScannerCursorRepository(t *testing.T) *Web3ScannerCursorRepository {
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

	return NewWeb3ScannerCursorRepository(client)
}

func TestWeb3ScannerCursorRepositoryInitializesOnce(t *testing.T) {
	repo := newWeb3ScannerCursorRepository(t)
	ctx := context.Background()

	created, err := repo.Initialize(ctx, testWeb3ScannerKey, 1030, testWeb3ScannerContract, 100)
	require.NoError(t, err)
	require.Positive(t, created.ID)
	require.EqualValues(t, 100, created.LastScannedBlock)
	require.EqualValues(t, 100, created.LastFinalizedBlock)

	lowerStart, err := repo.Initialize(ctx, testWeb3ScannerKey, 1030, testWeb3ScannerContract, 50)
	require.NoError(t, err)
	require.EqualValues(t, 100, lowerStart.ScanStartBlock)

	higherStart, err := repo.Initialize(ctx, testWeb3ScannerKey, 1030, testWeb3ScannerContract, 200)
	require.NoError(t, err)
	require.EqualValues(t, 100, higherStart.ScanStartBlock)

	_, err = repo.Initialize(ctx, testWeb3ScannerKey, 71, testWeb3ScannerContract, 100)
	require.ErrorIs(t, err, web3deposit.ErrCursorIdentityConflict)
	_, err = repo.Initialize(ctx, "duplicate:asset", 1030, testWeb3ScannerContract, 100)
	require.ErrorIs(t, err, web3deposit.ErrCursorIdentityConflict)
}

func TestWeb3ScannerCursorRepositoryLeaseLifecycle(t *testing.T) {
	repo := newWeb3ScannerCursorRepository(t)
	ctx := context.Background()
	_, err := repo.Initialize(ctx, testWeb3ScannerKey, 1030, testWeb3ScannerContract, 100)
	require.NoError(t, err)
	now := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)

	acquired, err := repo.AcquireLease(ctx, testWeb3ScannerKey, "scanner-01", "lease-token-01", now, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)

	acquired, err = repo.AcquireLease(ctx, testWeb3ScannerKey, "scanner-02", "lease-token-02", now, time.Minute)
	require.NoError(t, err)
	require.False(t, acquired)

	renewed, err := repo.RenewLease(ctx, testWeb3ScannerKey, "scanner-01", "lease-token-01", now.Add(30*time.Second), time.Minute)
	require.NoError(t, err)
	require.True(t, renewed)

	released, err := repo.ReleaseLease(ctx, testWeb3ScannerKey, "scanner-01", "stale-token")
	require.NoError(t, err)
	require.False(t, released)
	released, err = repo.ReleaseLease(ctx, testWeb3ScannerKey, "scanner-01", "lease-token-01")
	require.NoError(t, err)
	require.True(t, released)
}

func TestWeb3ScannerCursorRepositoryLeaseTakeoverRejectsStaleWorker(t *testing.T) {
	repo := newWeb3ScannerCursorRepository(t)
	ctx := context.Background()
	_, err := repo.Initialize(ctx, testWeb3ScannerKey, 1030, testWeb3ScannerContract, 100)
	require.NoError(t, err)
	now := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)

	acquired, err := repo.AcquireLease(ctx, testWeb3ScannerKey, "scanner-01", "lease-token-01", now, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = repo.AcquireLease(ctx, testWeb3ScannerKey, "scanner-02", "lease-token-02", now.Add(2*time.Minute), time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)

	err = repo.AdvanceScanner(ctx, testWeb3ScannerKey, "lease-token-01", 110, now.Add(2*time.Minute))
	require.ErrorIs(t, err, web3deposit.ErrLeaseNotHeld)
	require.NoError(t, repo.AdvanceScanner(ctx, testWeb3ScannerKey, "lease-token-02", 110, now.Add(2*time.Minute)))
}

func TestWeb3ScannerCursorRepositoryAdvancesMonotonically(t *testing.T) {
	repo := newWeb3ScannerCursorRepository(t)
	ctx := context.Background()
	_, err := repo.Initialize(ctx, testWeb3ScannerKey, 1030, testWeb3ScannerContract, 100)
	require.NoError(t, err)
	now := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	acquired, err := repo.AcquireLease(ctx, testWeb3ScannerKey, "scanner-01", "lease-token-01", now, time.Hour)
	require.NoError(t, err)
	require.True(t, acquired)

	require.NoError(t, repo.RecordError(ctx, testWeb3ScannerKey, "lease-token-01", now, "temporary RPC failure"))
	require.NoError(t, repo.AdvanceScanner(ctx, testWeb3ScannerKey, "lease-token-01", 120, now))
	err = repo.AdvanceScanner(ctx, testWeb3ScannerKey, "lease-token-01", 119, now)
	require.ErrorIs(t, err, web3deposit.ErrCursorWouldRegress)

	require.NoError(t, repo.AdvanceFinalizer(ctx, testWeb3ScannerKey, "lease-token-01", 110, now))
	err = repo.AdvanceFinalizer(ctx, testWeb3ScannerKey, "lease-token-01", 109, now)
	require.ErrorIs(t, err, web3deposit.ErrCursorWouldRegress)
	err = repo.AdvanceFinalizer(ctx, testWeb3ScannerKey, "lease-token-01", 121, now)
	require.ErrorIs(t, err, web3deposit.ErrFinalizerAheadOfScanner)

	loaded, err := repo.GetByKey(ctx, testWeb3ScannerKey)
	require.NoError(t, err)
	require.EqualValues(t, 120, loaded.LastScannedBlock)
	require.EqualValues(t, 110, loaded.LastFinalizedBlock)
	require.Nil(t, loaded.LastError)
	require.NotNil(t, loaded.LastSuccessAt)
}

func TestWeb3ScannerCursorRepositoryReturnsNotFound(t *testing.T) {
	repo := newWeb3ScannerCursorRepository(t)

	_, err := repo.GetByKey(context.Background(), "missing:cursor")
	require.True(t, errors.Is(err, web3deposit.ErrCursorNotFound))
}
