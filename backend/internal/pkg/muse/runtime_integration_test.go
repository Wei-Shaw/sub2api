//go:build integration

package muse_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/muse"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestMuseRuntimePostgres(t *testing.T) {
	ctx := context.Background()
	db := museTestDatabase(t)
	newService := func() *service.MuseRuntimeService {
		return service.NewMuseRuntimeService(repository.NewMuseRuntimeRepository(db))
	}
	var accountSeq atomic.Int64
	accountSeq.Store(1000)
	bind := func(name string) *muse.Workspace {
		id := accountSeq.Add(2)
		_, err := db.ExecContext(ctx, `INSERT INTO accounts(id,platform) VALUES($1,'muse'),($2,'muse')`, id, id+1)
		require.NoError(t, err)
		var at time.Time
		require.NoError(t, db.QueryRowContext(ctx, `SELECT updated_at FROM accounts WHERE id=$1`, id).Scan(&at))
		identity := muse.Identity{PrincipalID: name, WorkspaceID: "vm-1", OwnerUserID: 1, AccountID: id, AccountUpdatedAt: at}
		w, err := newService().BindVerifiedWorkspace(ctx, identity)
		require.NoError(t, err)
		identity.AccountID = id + 1
		require.NoError(t, db.QueryRowContext(ctx, `SELECT updated_at FROM accounts WHERE id=$1`, id+1).Scan(&identity.AccountUpdatedAt))
		_, err = newService().BindVerifiedWorkspace(ctx, identity)
		require.NoError(t, err)
		return w
	}
	reserve := func(w *muse.Workspace) muse.Reservation {
		return muse.Reservation{WorkspaceID: w.ID, Generation: w.Generation,
			Actor: muse.Actor{UserID: 1, APIKeyID: 10}, AccountID: w.Identity.AccountID, AccountUpdatedAt: w.Identity.AccountUpdatedAt, LeaseOwner: "worker-a", LeaseDuration: time.Minute,
			Pricing: muse.Pricing{Mode: "flat_request", UnitPrice: "0.03", Multiplier: "1.25"}}
	}
	expire := func(id int64) {
		_, err := db.ExecContext(ctx, `UPDATE muse_workspace_bindings SET lease_until=clock_timestamp()-INTERVAL '1 second' WHERE id=$1`, id)
		require.NoError(t, err)
	}

	t.Run("CanonicalBindingAndOwner", func(t *testing.T) {
		var at time.Time
		require.NoError(t, db.QueryRowContext(ctx, `SELECT updated_at FROM accounts WHERE id=100`).Scan(&at))
		identity := muse.Identity{PrincipalID: t.Name(), WorkspaceID: "vm", OwnerUserID: 1, AccountID: 100, AccountUpdatedAt: at}
		var wg sync.WaitGroup
		ids := make(chan int64, 12)
		errs := make(chan error, 12)
		for range 12 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				w, err := newService().BindVerifiedWorkspace(ctx, identity)
				if err != nil {
					errs <- err
					return
				}
				ids <- w.ID
			}()
		}
		wg.Wait()
		close(ids)
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		var first int64
		for id := range ids {
			if first == 0 {
				first = id
			}
			require.Equal(t, first, id)
		}
		identity.OwnerUserID = 2
		_, err := newService().BindVerifiedWorkspace(ctx, identity)
		require.ErrorIs(t, err, muse.ErrOwner)
	})

	t.Run("TwoReplicasAndAccountAliasesAdmitOneTurn", func(t *testing.T) {
		w := bind(t.Name())
		var wg sync.WaitGroup
		accepted := make(chan muse.Lease, 20)
		errs := make(chan error, 20)
		for i := range 20 {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				input := reserve(w)
				input.AccountID += int64(i % 2)
				input.LeaseOwner = fmt.Sprintf("worker-%d", i)
				_, lease, err := newService().Reserve(ctx, input)
				if err != nil {
					errs <- err
				} else {
					accepted <- lease
				}
			}(i)
		}
		wg.Wait()
		close(accepted)
		close(errs)
		require.Len(t, accepted, 1)
		for err := range errs {
			require.ErrorIs(t, err, muse.ErrBusy)
		}
		expire(w.ID)
		_, _, err := newService().Reserve(ctx, reserve(w))
		require.ErrorIs(t, err, muse.ErrBusy, "expired local lease must not allow a new task")
	})

	t.Run("RestartRecoversOriginalOperationAndFencesOldWorker", func(t *testing.T) {
		w := bind(t.Name())
		s := newService()
		turn, lease, err := s.Reserve(ctx, reserve(w))
		require.NoError(t, err)
		require.NoError(t, s.BeginSubmission(ctx, lease, "provider-turn"))
		expire(w.ID)
		fresh := newService()
		recovered, next, err := fresh.ClaimRecovery(ctx, w.ID, "recovery-worker", time.Minute)
		require.NoError(t, err)
		require.Equal(t, turn.ID, recovered.ID)
		require.Equal(t, muse.Ambiguous, recovered.State)
		require.Equal(t, "provider-turn", recovered.ProviderTurnID, "the probe reference must survive a lost acknowledgement and process restart")
		require.Greater(t, next.Fence, lease.Fence)
		require.ErrorIs(t, s.Renew(ctx, lease, time.Minute), muse.ErrLease)
		require.ErrorIs(t, s.Advance(ctx, lease, muse.Submitting, muse.Accepted, "late-id"), muse.ErrLease)
		require.ErrorIs(t, fresh.Advance(ctx, next, muse.Ambiguous, muse.Submitting, ""), muse.ErrTransition)
		require.ErrorIs(t, fresh.Advance(ctx, next, muse.Ambiguous, muse.Completed, "different-task"), muse.ErrTransition)
		require.NoError(t, fresh.Advance(ctx, next, muse.Ambiguous, muse.Completed, "provider-turn"))
		stored, err := fresh.GetTurn(ctx, turn.ID, turn.Actor)
		require.NoError(t, err)
		require.Equal(t, muse.Completed, stored.State)
		require.Equal(t, "provider-turn", stored.ProviderTurnID)
		_, _, err = fresh.Reserve(ctx, reserve(w))
		require.NoError(t, err)
	})

	t.Run("ClientCancellationRetainsRemoteOccupancy", func(t *testing.T) {
		w := bind(t.Name())
		s := newService()
		requestCtx, cancel := context.WithCancel(ctx)
		turn, lease, err := s.Reserve(requestCtx, reserve(w))
		require.NoError(t, err)
		require.NoError(t, s.BeginSubmission(requestCtx, lease))
		require.NoError(t, s.Advance(requestCtx, lease, muse.Submitting, muse.Accepted, "accepted-id"))
		require.NoError(t, s.Advance(requestCtx, lease, muse.Accepted, muse.Running, ""))
		require.NoError(t, s.Advance(requestCtx, lease, muse.Running, muse.CancelPending, ""))
		cancel()
		_, _, err = newService().Reserve(ctx, reserve(w))
		require.ErrorIs(t, err, muse.ErrBusy)
		stored, err := newService().GetTurn(ctx, turn.ID, turn.Actor)
		require.NoError(t, err)
		require.Equal(t, muse.CancelPending, stored.State)
		require.NoError(t, s.Advance(ctx, lease, muse.CancelPending, muse.Completed, "accepted-id"), "completion may race with cancellation")
		_, _, err = s.Reserve(ctx, reserve(w))
		require.NoError(t, err)
	})

	t.Run("PricingOwnershipAndAdmissionFailClosed", func(t *testing.T) {
		w := bind(t.Name())
		s := newService()
		input := reserve(w)
		input.Pricing = muse.Pricing{}
		_, _, err := s.Reserve(ctx, input)
		require.ErrorIs(t, err, muse.ErrInvalid)
		input = reserve(w)
		input.Generation++
		_, _, err = s.Reserve(ctx, input)
		require.ErrorIs(t, err, muse.ErrGeneration)
		input = reserve(w)
		input.Actor.APIKeyID = 20
		_, _, err = s.Reserve(ctx, input)
		require.ErrorIs(t, err, muse.ErrOwner, "foreign key owner cannot be forged")
		input = reserve(w)
		input.AccountID = 200
		_, _, err = s.Reserve(ctx, input)
		require.ErrorIs(t, err, muse.ErrOwner, "other providers cannot enter Muse runtime")
		other := bind(t.Name() + "-other-workspace")
		input = reserve(w)
		input.AccountID = other.Identity.AccountID
		_, _, err = s.Reserve(ctx, input)
		require.ErrorIs(t, err, muse.ErrOwner, "native account cannot impersonate another verified workspace")
		input = reserve(w)
		turn, lease, err := s.Reserve(ctx, input)
		require.NoError(t, err)
		input.Pricing.UnitPrice = "999"
		stored, err := s.GetTurn(ctx, turn.ID, turn.Actor)
		require.NoError(t, err)
		require.Equal(t, "0.03", stored.Pricing.UnitPrice)
		_, err = s.GetTurn(ctx, turn.ID, muse.Actor{UserID: 1, APIKeyID: 11})
		require.ErrorIs(t, err, muse.ErrNotFound)
		_, err = s.GetTurn(ctx, turn.ID, muse.Actor{UserID: 2, APIKeyID: 20})
		require.ErrorIs(t, err, muse.ErrNotFound)
		require.NoError(t, s.Advance(ctx, lease, muse.Reserved, muse.Rejected, ""))
	})

	t.Run("ProviderCorrelationCannotBeReplaced", func(t *testing.T) {
		w := bind(t.Name())
		s := newService()
		_, lease, err := s.Reserve(ctx, reserve(w))
		require.NoError(t, err)
		require.NoError(t, s.BeginSubmission(ctx, lease))
		require.NoError(t, s.Advance(ctx, lease, muse.Submitting, muse.Accepted, "original"))
		require.ErrorIs(t, s.Advance(ctx, lease, muse.Accepted, muse.Completed, "other-task"), muse.ErrTransition)
		require.NoError(t, s.Advance(ctx, lease, muse.Accepted, muse.Completed, "original"))
	})

	t.Run("AccountChangesInvalidateVerifiedBindingBeforeSend", func(t *testing.T) {
		w := bind(t.Name())
		s := newService()
		_, lease, err := s.Reserve(ctx, reserve(w))
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `UPDATE accounts SET updated_at=clock_timestamp() WHERE id=$1`, w.Identity.AccountID)
		require.NoError(t, err)
		require.ErrorIs(t, s.BeginSubmission(ctx, lease), muse.ErrGeneration)
		_, err = s.BindVerifiedWorkspace(ctx, w.Identity)
		require.ErrorIs(t, err, muse.ErrGeneration, "stale auth observation must not rebind changed credentials")
		require.NoError(t, s.Advance(ctx, lease, muse.Reserved, muse.Rejected, ""))
		_, _, err = s.Reserve(ctx, reserve(w))
		require.ErrorIs(t, err, muse.ErrOwner)
	})

	t.Run("ReverificationCannotUpgradeQueuedCredentials", func(t *testing.T) {
		w := bind(t.Name())
		old := reserve(w)
		_, err := db.ExecContext(ctx, `UPDATE accounts SET updated_at=clock_timestamp()+INTERVAL '1 second' WHERE id=$1`, old.AccountID)
		require.NoError(t, err)
		fresh := w.Identity
		require.NoError(t, db.QueryRowContext(ctx, `SELECT updated_at FROM accounts WHERE id=$1`, old.AccountID).Scan(&fresh.AccountUpdatedAt))
		_, err = newService().BindVerifiedWorkspace(ctx, fresh)
		require.NoError(t, err)
		_, _, err = newService().Reserve(ctx, old)
		require.Error(t, err, "old credentials must not inherit a new verification")
	})
	t.Run("ProxyEditAfterReservationStopsDispatch", func(t *testing.T) {
		w := bind(t.Name())
		var at time.Time
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO proxies(id) VALUES($1) RETURNING updated_at`, w.ID).Scan(&at))
		_, err := db.ExecContext(ctx, `UPDATE accounts SET proxy_id=$2 WHERE id=$1`, w.Identity.AccountID, w.ID)
		require.NoError(t, err)
		input := reserve(w)
		input.ProxyUpdatedAt = &at
		_, lease, err := newService().Reserve(ctx, input)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `UPDATE proxies SET updated_at=clock_timestamp()+INTERVAL '1 second' WHERE id=$1`, w.ID)
		require.NoError(t, err)
		require.ErrorIs(t, newService().BeginSubmission(ctx, lease), muse.ErrGeneration)
	})
	t.Run("OwnerReviewDoesNotAutomaticallyResume", func(t *testing.T) {
		w := bind(t.Name())
		s := newService()
		_, lease, err := s.Reserve(ctx, reserve(w))
		require.NoError(t, err)
		require.NoError(t, s.BeginSubmission(ctx, lease))
		require.NoError(t, s.Advance(ctx, lease, muse.Submitting, muse.Ambiguous, ""))
		require.NoError(t, s.Advance(ctx, lease, muse.Ambiguous, muse.OwnerReview, ""))
		expire(w.ID)
		_, _, err = s.ClaimRecovery(ctx, w.ID, "automatic-worker", time.Minute)
		require.ErrorIs(t, err, muse.ErrOwnerReview)
		_, _, err = s.Reserve(ctx, reserve(w))
		require.ErrorIs(t, err, muse.ErrBusy)
	})
}

func museIsolatedTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	dsn := os.Getenv("SUB2API_MUSE_TEST_DSN")
	if dsn == "" {
		container, err := tcpostgres.Run(ctx, "postgres:18.1-alpine3.23", tcpostgres.WithDatabase("muse_test"), tcpostgres.WithUsername("muse_test"), tcpostgres.WithPassword("test-only"), tcpostgres.BasicWaitStrategies())
		require.NoError(t, err, "start Docker or set SUB2API_MUSE_TEST_DSN to an isolated test PostgreSQL")
		t.Cleanup(func() { _ = container.Terminate(ctx) })
		dsn, err = container.ConnectionString(ctx, "sslmode=disable")
		require.NoError(t, err)
	}
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })
	var nonce [8]byte
	_, err = rand.Read(nonce[:])
	require.NoError(t, err)
	schema := "muse_runtime_test_" + hex.EncodeToString(nonce[:])
	_, err = admin.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.ExecContext(ctx, "DROP SCHEMA "+pq.QuoteIdentifier(schema)+" CASCADE") })
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	require.True(t, u.Scheme == "postgres" || u.Scheme == "postgresql", "use a PostgreSQL URL for the isolated test DSN")
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db, err := sql.Open("postgres", u.String())
	require.NoError(t, err)
	db.SetMaxOpenConns(24)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func museTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	db := museIsolatedTestDatabase(t)
	_, err := db.ExecContext(ctx, `CREATE TABLE users(id BIGINT PRIMARY KEY,status TEXT NOT NULL DEFAULT 'active',deleted_at TIMESTAMPTZ);
		CREATE TABLE api_keys(id BIGINT PRIMARY KEY,user_id BIGINT REFERENCES users(id),status TEXT NOT NULL DEFAULT 'active',deleted_at TIMESTAMPTZ,expires_at TIMESTAMPTZ);
		CREATE TABLE proxies(id BIGINT PRIMARY KEY,updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),deleted_at TIMESTAMPTZ);
 CREATE TABLE accounts(id BIGINT PRIMARY KEY,proxy_id BIGINT REFERENCES proxies(id),platform TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'active',schedulable BOOLEAN NOT NULL DEFAULT TRUE,deleted_at TIMESTAMPTZ,updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
		INSERT INTO users(id) VALUES (1),(2);
		INSERT INTO api_keys(id,user_id) VALUES (10,1),(11,1),(20,2);
		INSERT INTO accounts(id,platform) VALUES (100,'muse'),(101,'muse'),(200,'openai');`)
	require.NoError(t, err)
	ddl, err := migrations.FS.ReadFile("242_muse_runtime.sql")
	require.NoError(t, err)
	for range 2 {
		_, err = db.ExecContext(ctx, string(ddl))
		require.NoError(t, err, "migration must be rerunnable")
	}
	// The registered provider migration adds the two settlement admission fields.
	_, err = db.ExecContext(ctx, `ALTER TABLE muse_turns ADD COLUMN billing_command JSONB;ALTER TABLE muse_turns ADD COLUMN settled_at TIMESTAMPTZ`)
	require.NoError(t, err)
	snapshotDDL, err := migrations.FS.ReadFile("244_muse_submission_snapshot.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(snapshotDDL))
	require.NoError(t, err)
	return db
}
