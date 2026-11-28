//go:build unit

package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestIsTransientUsageLogWriteError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"attempt deadline", fmt.Errorf("wrapped: %w", context.DeadlineExceeded), true},
		{"bad conn", driver.ErrBadConn, true},
		{"eof", io.ErrUnexpectedEOF, true},
		{"net", &net.OpError{Op: "read", Err: errors.New("i/o timeout")}, true},
		{"query canceled while waiting for lock", &pq.Error{Code: "57014"}, true},
		{"connection failure", &pq.Error{Code: "08006"}, true},
		{"deadlock", &pq.Error{Code: "40P01"}, true},
		{"admin shutdown", &pq.Error{Code: "57P01"}, true},
		{"too many connections", &pq.Error{Code: "53300"}, true},
		{"lock not available", &pq.Error{Code: "55P03"}, true},
		{"foreign key", &pq.Error{Code: "23503"}, false},
		{"not null", &pq.Error{Code: "23502"}, false},
		{"numeric out of range", &pq.Error{Code: "22003"}, false},
		{"undefined column", &pq.Error{Code: "42703"}, false},
		{"database dropped", &pq.Error{Code: "57P04"}, false},
		{"plain", errors.New("sql: database is closed"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isTransientUsageLogWriteError(tc.err))
		})
	}
}

func TestUsageLogWriteRetryDelayIsFullJitterUnderCappedExponential(t *testing.T) {
	p := usageLogWriteRetry{baseDelay: 50 * time.Millisecond, maxDelay: 20 * time.Second}
	for retry, ceiling := range map[int]time.Duration{
		0:  50 * time.Millisecond,
		3:  400 * time.Millisecond,
		8:  12800 * time.Millisecond,
		9:  20 * time.Second,
		40: 20 * time.Second,
	} {
		var maxSeen time.Duration
		for i := 0; i < 2000; i++ {
			d := p.delay(retry)
			require.GreaterOrEqual(t, d, time.Duration(0))
			require.LessOrEqual(t, d, ceiling)
			maxSeen = max(maxSeen, d)
		}
		require.Greater(t, maxSeen, ceiling/2, "retry=%d", retry)
	}
}

// usageLogFakeStatement is one statement the fake driver received: the request_ids it carries and
// whether it was the single-row INSERT of the one-by-one path.
type usageLogFakeStatement struct {
	single bool
	ids    []string
}

// usageLogFakeDB is a database/sql driver that lets the test decide, per statement, whether the
// write succeeds. Rows of a successful statement are recorded in stored.
type usageLogFakeDB struct {
	mu         sync.Mutex
	handler    func(ctx context.Context, st usageLogFakeStatement) error
	rowArgs    int
	stored     map[string]int
	storedAt   map[string]time.Time
	statements []usageLogFakeStatement
}

func newUsageLogFakeRepo(t *testing.T, handler func(ctx context.Context, st usageLogFakeStatement) error) (*usageLogRepository, *sql.DB, *usageLogFakeDB) {
	t.Helper()
	fake := &usageLogFakeDB{
		handler:  handler,
		rowArgs:  len(prepareUsageLogInsert(&service.UsageLog{}).args),
		stored:   make(map[string]int),
		storedAt: make(map[string]time.Time),
	}
	db := sql.OpenDB(fake)
	t.Cleanup(func() { _ = db.Close() })
	repo := newUsageLogRepositoryWithSQL(nil, db)
	repo.bestEffortRetry = usageLogWriteRetry{
		attemptTimeout: time.Second,
		baseDelay:      5 * time.Millisecond,
		maxDelay:       20 * time.Millisecond,
		budget:         time.Hour,
	}
	return repo, db, fake
}

func (f *usageLogFakeDB) Connect(context.Context) (driver.Conn, error) {
	return &usageLogFakeConn{f: f}, nil
}

func (f *usageLogFakeDB) Driver() driver.Driver { return usageLogFakeDriver{f: f} }

func (f *usageLogFakeDB) counts() (batches, singles int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, st := range f.statements {
		if st.single {
			singles++
		} else {
			batches++
		}
	}
	return batches, singles
}

func (f *usageLogFakeDB) storedTime(id string) (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	at, ok := f.storedAt[id]
	return at, ok
}

func (f *usageLogFakeDB) storedCount(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stored[id]
}

type usageLogFakeDriver struct{ f *usageLogFakeDB }

func (d usageLogFakeDriver) Open(string) (driver.Conn, error) { return &usageLogFakeConn{f: d.f}, nil }

type usageLogFakeConn struct{ f *usageLogFakeDB }

func (c *usageLogFakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("usage log fake driver: prepare is not supported")
}
func (c *usageLogFakeConn) Close() error { return nil }
func (c *usageLogFakeConn) Begin() (driver.Tx, error) {
	return nil, errors.New("usage log fake driver: transactions are not supported")
}

func (c *usageLogFakeConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	f := c.f
	st := usageLogFakeStatement{single: strings.HasPrefix(strings.TrimSpace(query), "INSERT")}
	for i := 0; i+3 < len(args); i += f.rowArgs {
		id, _ := args[i+3].Value.(string) // request_id is the fourth column
		st.ids = append(st.ids, id)
	}
	f.mu.Lock()
	f.statements = append(f.statements, st)
	f.mu.Unlock()
	// The handler runs without the lock so that a statement can hang while others go on.
	if err := f.handler(ctx, st); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for _, id := range st.ids {
		f.stored[id]++
		if _, ok := f.storedAt[id]; !ok {
			f.storedAt[id] = now
		}
	}
	return driver.RowsAffected(len(st.ids)), nil
}

func newUsageLogFakeRequests(ids ...string) []usageLogBestEffortRequest {
	deadline := time.Now().Add(time.Hour)
	reqs := make([]usageLogBestEffortRequest, 0, len(ids))
	for _, id := range ids {
		reqs = append(reqs, usageLogBestEffortRequest{
			prepared: prepareUsageLogInsert(&service.UsageLog{UserID: 1, APIKeyID: 1, AccountID: 1, RequestID: id, Model: "m"}),
			apiKeyID: 1,
			deadline: deadline,
			resultCh: make(chan error, 1),
		})
	}
	return reqs
}

func usageLogFakeIDs(n int, prefix string) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s-%02d", prefix, i)
	}
	return ids
}

func requireUsageLogRequestIDs(t *testing.T, want []string, reqs []usageLogBestEffortRequest) {
	t.Helper()
	got := make([]string, 0, len(reqs))
	for _, req := range reqs {
		got = append(got, req.prepared.requestID)
	}
	require.Equal(t, want, got)
}

// One row of a batch keeps failing with a transient error (a lock that is not released). The rows
// before it land in the first round and the rest by the second; only that row is left to retry.
func TestFlushBestEffortBatch_TransientErrorFromOneRowDoesNotBlockTheOthers(t *testing.T) {
	for _, pos := range []int{0, 7, 19} {
		t.Run(fmt.Sprintf("poison at %d", pos), func(t *testing.T) {
			ids := usageLogFakeIDs(20, "row")
			ids[pos] = "poison"
			repo, db, fake := newUsageLogFakeRepo(t, func(_ context.Context, st usageLogFakeStatement) error {
				for _, id := range st.ids {
					if id == "poison" {
						return &pq.Error{Code: "55P03"}
					}
				}
				return nil
			})
			reqs := newUsageLogFakeRequests(ids...)

			pending := repo.flushBestEffortBatch(db, reqs)
			for i := 0; i < pos; i++ {
				require.Equal(t, 1, fake.storedCount(ids[i]), ids[i])
			}
			pending = repo.flushBestEffortBatch(db, pending)

			requireUsageLogRequestIDs(t, []string{"poison"}, pending)
			for i, id := range ids {
				if i != pos {
					require.Equal(t, 1, fake.storedCount(id), id)
					require.NoError(t, <-reqs[i].resultCh)
				}
			}
			require.Zero(t, fake.storedCount("poison"))
			require.Empty(t, reqs[pos].resultCh)
			require.Zero(t, repo.bestEffortLost.Load())

			// A single-row batch is retained as it is.
			again := repo.flushBestEffortBatch(db, pending)
			requireUsageLogRequestIDs(t, []string{"poison"}, again)
		})
	}
}

// The batch as a whole fails with a transient error because of its size (a statement that takes
// longer than statement_timeout or the attempt deadline) while single rows are fast.
func TestFlushBestEffortBatch_TransientErrorFromBatchSizeFallsBackToSingleRows(t *testing.T) {
	for name, transient := range map[string]error{
		"attempt deadline":  context.DeadlineExceeded,
		"statement timeout": &pq.Error{Code: "57014"},
	} {
		t.Run(name, func(t *testing.T) {
			repo, db, fake := newUsageLogFakeRepo(t, func(_ context.Context, st usageLogFakeStatement) error {
				if len(st.ids) > 5 {
					return transient
				}
				return nil
			})
			ids := usageLogFakeIDs(30, "row")
			reqs := newUsageLogFakeRequests(ids...)

			pending := repo.flushBestEffortBatch(db, reqs)

			require.Empty(t, pending)
			for i, id := range ids {
				require.Equal(t, 1, fake.storedCount(id), id)
				require.NoError(t, <-reqs[i].resultCh)
			}
			require.Zero(t, repo.bestEffortLost.Load())
		})
	}
}

// While the DB is down a round costs one batch attempt and one single-row attempt, however many
// rows wait; when it is back the next round writes them all.
func TestFlushBestEffortBatch_OutageCostsOneSingleRowAttemptPerRound(t *testing.T) {
	var down atomic.Bool
	down.Store(true)
	repo, db, fake := newUsageLogFakeRepo(t, func(context.Context, usageLogFakeStatement) error {
		if down.Load() {
			return &pq.Error{Code: "08006"}
		}
		return nil
	})
	ids := usageLogFakeIDs(30, "row")
	reqs := newUsageLogFakeRequests(ids...)

	pending := reqs
	const rounds = 20
	for i := 0; i < rounds; i++ {
		pending = repo.flushBestEffortBatch(db, pending)
		require.Len(t, pending, len(ids))
	}
	batches, singles := fake.counts()
	require.Equal(t, rounds, batches)
	require.Equal(t, rounds, singles)
	require.Zero(t, repo.bestEffortLost.Load())

	down.Store(false)
	pending = repo.flushBestEffortBatch(db, pending)
	require.Empty(t, pending)
	for i, id := range ids {
		require.Equal(t, 1, fake.storedCount(id), id)
		require.NoError(t, <-reqs[i].resultCh)
	}
}

// The same outage through the running writer: callers give up long before the DB is back, and every
// row still lands afterwards, exactly once.
func TestUsageLogBestEffort_OutageThenRecoveryLandsEveryRow(t *testing.T) {
	var down atomic.Bool
	down.Store(true)
	repo, _, fake := newUsageLogFakeRepo(t, func(context.Context, usageLogFakeStatement) error {
		if down.Load() {
			return &pq.Error{Code: "08006"}
		}
		return nil
	})
	ids := usageLogFakeIDs(30, "row")

	errs := make(chan error, len(ids))
	for _, id := range ids {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			errs <- repo.CreateBestEffort(ctx, &service.UsageLog{UserID: 1, APIKeyID: 1, AccountID: 1, RequestID: id, Model: "m"})
		}()
	}
	for range ids {
		require.NoError(t, <-errs)
	}

	time.Sleep(time.Second)
	batches, singles := fake.counts()
	require.Greater(t, batches, 5, "the writer keeps retrying while the DB is down")
	require.LessOrEqual(t, singles, batches, "at most one single-row attempt per round")
	for _, id := range ids {
		require.Zero(t, fake.storedCount(id), id)
	}

	down.Store(false)
	require.Eventually(t, func() bool {
		for _, id := range ids {
			if fake.storedCount(id) != 1 {
				return false
			}
		}
		return true
	}, 5*time.Second, 10*time.Millisecond)
	require.Zero(t, repo.bestEffortLost.Load())
}

// A row that keeps failing with a transient error must not delay the rows that arrive after it. They
// are written as they come, and the failing row is retried on its own schedule until its retry
// budget runs out and is then counted as lost. In the second case every attempt for that row hangs
// until the attempt deadline.
func TestUsageLogBestEffort_FailingRowDoesNotDelayNewRows(t *testing.T) {
	for _, tc := range []struct {
		name string
		hang bool
	}{
		{"fails at once", false},
		{"hangs until the attempt deadline", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, _, fake := newUsageLogFakeRepo(t, func(ctx context.Context, st usageLogFakeStatement) error {
				for _, id := range st.ids {
					if id != "poison" {
						continue
					}
					if tc.hang {
						<-ctx.Done()
						return ctx.Err()
					}
					return &pq.Error{Code: "55P03"}
				}
				return nil
			})
			repo.bestEffortRetry = usageLogWriteRetry{
				attemptTimeout: 600 * time.Millisecond,
				baseDelay:      5 * time.Millisecond,
				maxDelay:       time.Second,
				budget:         4 * time.Second,
			}
			newLog := func(id string) *service.UsageLog {
				return &service.UsageLog{UserID: 1, APIKeyID: 1, AccountID: 1, RequestID: id, Model: "m"}
			}

			poisonErr := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				poisonErr <- repo.CreateBestEffort(ctx, newLog("poison"))
			}()
			// Let the first attempt for the failing row finish, then send good rows at 100 per second.
			time.Sleep(repo.bestEffortRetry.attemptTimeout + 100*time.Millisecond)

			ids := usageLogFakeIDs(300, "row")
			enqueuedAt := make(map[string]time.Time, len(ids))
			errs := make(chan error, len(ids))
			for _, id := range ids {
				enqueuedAt[id] = time.Now()
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					errs <- repo.CreateBestEffort(ctx, newLog(id))
				}()
				time.Sleep(10 * time.Millisecond)
			}
			for range ids {
				require.NoError(t, <-errs)
			}
			require.Eventually(t, func() bool {
				for _, id := range ids {
					if _, ok := fake.storedTime(id); !ok {
						return false
					}
				}
				return true
			}, 3*time.Second, 10*time.Millisecond)

			var slowest time.Duration
			for _, id := range ids {
				at, _ := fake.storedTime(id)
				slowest = max(slowest, at.Sub(enqueuedAt[id]))
				require.Equal(t, 1, fake.storedCount(id), id)
			}
			require.Less(t, slowest, 300*time.Millisecond, "a row that arrived while another row keeps failing waited too long")

			err := <-poisonErr
			require.True(t, service.IsUsageLogCreateDropped(err), "err=%v", err)
			require.Zero(t, fake.storedCount("poison"))
			require.Equal(t, uint64(1), repo.bestEffortLost.Load())
		})
	}
}
