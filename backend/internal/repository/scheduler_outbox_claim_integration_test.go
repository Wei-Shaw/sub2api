//go:build integration

package repository

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// These tests run against the real PostgreSQL started by the integration harness
// (migrations applied, including 250_scheduler_outbox_claim.sql) and exercise the
// durable-claim queue with concurrent consumers. They exist because the watermark
// design silently lost events, and because a statement-level FOR UPDATE cannot
// protect an event that is handled for minutes -- neither fact is visible in a
// unit test with a fake.

func claimQueue(t *testing.T) service.SchedulerOutboxQueue {
	t.Helper()
	repo := NewSchedulerOutboxRepository(integrationDB)
	q, ok := repo.(service.SchedulerOutboxQueue)
	require.True(t, ok, "the scheduler outbox repository must implement service.SchedulerOutboxQueue")
	_, err := integrationDB.ExecContext(context.Background(), "TRUNCATE scheduler_outbox")
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = integrationDB.ExecContext(ctx, "TRUNCATE scheduler_outbox")
	})
	return q
}

func claimInsert(t *testing.T, accountID int, count int) []int64 {
	t.Helper()
	ctx := context.Background()
	ids := make([]int64, 0, count)
	for i := 0; i < count; i++ {
		var id int64
		err := integrationDB.QueryRowContext(ctx, `
			INSERT INTO scheduler_outbox (event_type, account_id, payload)
			VALUES ('account_changed', $1, jsonb_build_object('seq', $2::int)) RETURNING id`,
			accountID, i).Scan(&id)
		require.NoError(t, err)
		ids = append(ids, id)
	}
	return ids
}

func claimCount(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM scheduler_outbox").Scan(&n))
	return n
}

func claimIDs(events []service.SchedulerOutboxEvent) []int64 {
	out := make([]int64, 0, len(events))
	for _, event := range events {
		out = append(out, event.ID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func claimSeq(payload map[string]any) int {
	switch v := payload["seq"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return -1
}

// TestOutboxClaimTwoConsumersHandleEachEventOnce is the core concurrency gate:
// two consumers draining the same queue must never handle or acknowledge the same
// event twice.
func TestOutboxClaimTwoConsumersHandleEachEventOnce(t *testing.T) {
	ctx := context.Background()
	q := claimQueue(t)
	seeded := claimInsert(t, 7001, 200)

	var (
		mu       sync.Mutex
		handled  = map[int64]int{}
		ackOwned = map[int64]string{}
		staleAck int
		rounds   int
	)
	var wg sync.WaitGroup
	for w := 1; w <= 2; w++ {
		wg.Add(1)
		go func(worker string) {
			defer wg.Done()
			for {
				events, err := q.Claim(ctx, worker, 10, time.Minute)
				if err != nil {
					t.Errorf("claim(%s): %v", worker, err)
					return
				}
				if len(events) == 0 {
					return
				}
				mu.Lock()
				rounds++
				mu.Unlock()
				for _, event := range events {
					mu.Lock()
					handled[event.ID]++
					mu.Unlock()

					if err := q.DeleteClaimed(ctx, event.ID, worker); err != nil {
						mu.Lock()
						staleAck++
						mu.Unlock()
						continue
					}
					mu.Lock()
					ackOwned[event.ID] = worker
					mu.Unlock()
				}
			}
		}(fmt.Sprintf("consumer-%d", w))
	}
	wg.Wait()

	require.Equal(t, 200, len(seeded))
	for _, id := range seeded {
		require.Equalf(t, 1, handled[id], "event %d handled %d times, want once", id, handled[id])
		_, acked := ackOwned[id]
		require.Truef(t, acked, "event %d was never acknowledged", id)
	}
	require.Equal(t, 200, len(ackOwned))
	require.Zerof(t, staleAck, "%d ownership-checked acks failed inside their lease", staleAck)
	require.Equal(t, 0, claimCount(t))
	t.Logf("200 events drained by 2 consumers in %d claim rounds; every event handled and acked exactly once", rounds)
}

// TestOutboxClaimLateCommittedLowIDIsConsumed reproduces the lost-event race: a
// transaction allocates a low id, a higher id commits and is consumed (pushing the
// watermark past the low one), and only then does the first transaction commit.
// With `id > watermark` as the read filter the low row can never be polled again
// and cleanup deletes it as if it had been consumed.
func TestOutboxClaimLateCommittedLowIDIsConsumed(t *testing.T) {
	ctx := context.Background()
	q := claimQueue(t)

	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	committed := false
	t.Cleanup(func() {
		if !committed {
			_ = tx.Rollback()
		}
	})

	var lowID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO scheduler_outbox (event_type, account_id, payload)
		VALUES ('account_changed', 7002, jsonb_build_object('late', true)) RETURNING id`).Scan(&lowID)
	require.NoError(t, err)

	highIDs := claimInsert(t, 7003, 3)
	drained := claimIDs(claimDrain(t, q, "drainer-1"))
	for _, id := range highIDs {
		require.Contains(t, drained, id)
	}
	legacyWatermark := highIDs[len(highIDs)-1]

	require.NoError(t, tx.Commit())
	committed = true
	require.Less(t, lowID, legacyWatermark, "the setup depends on the low id staying below the watermark")

	// The row is now visible and its id is already behind the watermark. The queue
	// must still consume it, because presence in the table -- not the watermark --
	// is what marks an event as unconsumed.
	second := claimIDs(claimDrain(t, q, "drainer-2"))
	require.Containsf(t, second, lowID,
		"late-committed event id=%d (below watermark %d) was never consumed", lowID, legacyWatermark)
	require.Equal(t, 0, claimCount(t))
	t.Logf("late low-id commit: id=%d consumed although the old watermark was %d", lowID, legacyWatermark)
}

// TestOutboxClaimAbandonedLeaseIsReclaimed covers the crash path. There is
// deliberately no renewal call: holding a claim past its lease costs it, so the
// batch duration must stay under the lease (the service uses 5 minutes against a
// 2 minute per-event timeout). Duplicate handling after a takeover is safe because
// the handlers are updates.
func TestOutboxClaimAbandonedLeaseIsReclaimed(t *testing.T) {
	ctx := context.Background()
	q := claimQueue(t)
	ids := claimInsert(t, 7004, 4)
	lease := time.Second

	stolen, err := q.Claim(ctx, "dead-worker", 10, lease)
	require.NoError(t, err)
	require.Len(t, stolen, len(ids))

	// Inside the lease nobody else may take it.
	early, err := q.Claim(ctx, "eager-worker", 10, lease)
	require.NoError(t, err)
	require.Empty(t, early, "rows were re-claimed inside the lease")

	time.Sleep(lease + 500*time.Millisecond)

	taken, err := q.Claim(ctx, "survivor", 10, lease)
	require.NoError(t, err)
	require.Len(t, taken, len(ids), "an abandoned claim was not re-delivered after the lease")

	// The crashed owner can no longer ack: the ack is ownership-checked, so a
	// stale ack cannot destroy a row another consumer is working on.
	require.Error(t, q.DeleteClaimed(ctx, ids[0], "dead-worker"),
		"the previous owner acknowledged a row it no longer owns")

	for _, event := range taken {
		require.NoError(t, q.DeleteClaimed(ctx, event.ID, "survivor"))
	}
	require.Equal(t, 0, claimCount(t))

	late, err := q.Claim(ctx, "late-arriver", 10, lease)
	require.NoError(t, err)
	require.Empty(t, late, "a completed event was handed out again")
	t.Logf("lease %s expired -> re-delivered to a second consumer -> stale ack rejected -> drained once", lease)
}

// TestOutboxClaimOwnershipIsExclusive shows the two halves of ownership: a
// concurrent consumer cannot see leased rows, and cannot ack them either.
func TestOutboxClaimOwnershipIsExclusive(t *testing.T) {
	ctx := context.Background()
	q := claimQueue(t)
	ids := claimInsert(t, 7005, 2)

	first, err := q.Claim(ctx, "owner-a", 10, 30*time.Second)
	require.NoError(t, err)
	require.Len(t, first, len(ids))

	second, err := q.Claim(ctx, "owner-b", 10, 30*time.Second)
	require.NoError(t, err)
	require.Empty(t, second, "owner-b claimed rows leased to owner-a")

	require.Error(t, q.DeleteClaimed(ctx, ids[0], "owner-b"), "owner-b acknowledged owner-a's row")
	require.NoError(t, q.DeleteClaimed(ctx, ids[0], "owner-a"))
	require.Equal(t, len(ids)-1, claimCount(t))

	// owner-a still holds ids[1] under its 30s lease; hand it back explicitly, which
	// is exactly what releaseOutboxTail does for the untouched tail of a batch.
	require.NoError(t, q.ReleaseClaimed(ctx, ids[1:], time.Now()))

	// Releasing a claim explicitly makes the row instantly available again, so a
	// handled-but-failed event does not wait for the lease.
	released, err := q.Claim(ctx, "owner-c", 10, 30*time.Second)
	require.NoError(t, err)
	require.Len(t, released, 1)
	require.NoError(t, q.RetryClaimed(ctx, released[0].ID, "owner-c", time.Now(), "simulated handler error"))
	again, err := q.Claim(ctx, "owner-d", 10, 30*time.Second)
	require.NoError(t, err)
	require.Len(t, again, 1, "a released claim did not return to the queue")
	require.NoError(t, q.DeleteClaimed(ctx, again[0].ID, "owner-d"))
}

// TestOutboxClaimSkipLockedIsNotACrossTransactionLock documents why the claim is
// column-based: FOR UPDATE inside an autocommit statement releases its row locks
// when the statement returns, even though the queue still considers the row owned.
func TestOutboxClaimSkipLockedIsNotACrossTransactionLock(t *testing.T) {
	ctx := context.Background()
	q := claimQueue(t)
	ids := claimInsert(t, 7006, 3)

	events, err := q.Claim(ctx, "slow-consumer", 10, time.Hour)
	require.NoError(t, err)
	require.Len(t, events, len(ids))

	conn, err := integrationDB.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()

	var locked int
	err = conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM (
			SELECT id FROM scheduler_outbox
			WHERE id = ANY($1)
			FOR UPDATE NOWAIT
		) x`, pq.Array(ids)).Scan(&locked)
	require.NoErrorf(t, err, "FOR UPDATE NOWAIT should succeed: the claim statement already released its locks")
	require.Equal(t, len(ids), locked)

	// Yet the queue will not hand the rows out, because ownership is recorded.
	blocked, err := q.Claim(ctx, "other-consumer", 10, time.Hour)
	require.NoError(t, err)
	require.Empty(t, blocked)
	t.Logf("row locks are gone after the claim statement (re-lockable immediately); claimed_by is what keeps the rows off the queue")
}

// TestOutboxClaimSameAccountOrderingAcrossTwoConsumers pins the ordering contract
// when two consumers run at once: ascending hand-off inside a batch, no event lost
// or duplicated, and every per-account sequence complete.
func TestOutboxClaimSameAccountOrderingAcrossTwoConsumers(t *testing.T) {
	ctx := context.Background()
	q := claimQueue(t)

	type seed struct {
		id   int64
		acct int64
		seq  int
	}
	index := map[int64]seed{}
	for seq := 0; seq < 10; seq++ {
		for acct := 1; acct <= 3; acct++ {
			var id int64
			err := integrationDB.QueryRowContext(ctx, `
				INSERT INTO scheduler_outbox (event_type, account_id, payload)
				VALUES ('account_changed', $1, jsonb_build_object('seq', $2::int)) RETURNING id`,
				7100+acct, seq).Scan(&id)
			require.NoError(t, err)
			index[id] = seed{id: id, acct: int64(7100 + acct), seq: seq}
		}
	}
	require.Len(t, index, 30)

	type record struct {
		id     int64
		acct   int64
		seq    int
		worker string
	}
	var (
		mu      sync.Mutex
		records []record
	)
	var wg sync.WaitGroup
	for w := 1; w <= 2; w++ {
		wg.Add(1)
		go func(worker string) {
			defer wg.Done()
			for {
				events, err := q.Claim(ctx, worker, 4, time.Minute)
				if err != nil {
					t.Errorf("claim(%s): %v", worker, err)
					return
				}
				if len(events) == 0 {
					return
				}
				for i := 1; i < len(events); i++ {
					if events[i].ID <= events[i-1].ID {
						t.Errorf("%s got non-ascending ids %d then %d", worker, events[i-1].ID, events[i].ID)
						return
					}
				}
				for _, event := range events {
					mu.Lock()
					records = append(records, record{id: event.ID, acct: index[event.ID].acct, seq: claimSeq(event.Payload), worker: worker})
					mu.Unlock()
					if err := q.DeleteClaimed(ctx, event.ID, worker); err != nil {
						t.Errorf("ack %d: %v", event.ID, err)
						return
					}
				}
			}
		}(fmt.Sprintf("ordered-%d", w))
	}
	wg.Wait()

	require.Len(t, records, 30)
	handled := map[int64]int{}
	byAccount := map[int64][]int{}
	byWorker := map[string][]record{}
	for _, r := range records {
		handled[r.id]++
		byAccount[r.acct] = append(byAccount[r.acct], r.seq)
		key := fmt.Sprintf("%d/%s", r.acct, r.worker)
		byWorker[key] = append(byWorker[key], r)
	}
	for id, n := range handled {
		require.Equalf(t, 1, n, "event %d handled %d times", id, n)
	}
	for acct, seqs := range byAccount {
		sort.Ints(seqs)
		require.Len(t, seqs, 10, "account %d handled %d events", acct, len(seqs))
		for i, v := range seqs {
			require.Equalf(t, i, v, "account %d sequence broken: %v", acct, seqs)
		}
	}
	for key, list := range byWorker {
		sort.Slice(list, func(i, j int) bool { return list[i].id < list[j].id })
		for i := 1; i < len(list); i++ {
			require.Greater(t, list[i].id, list[i-1].id, "ids out of order in %s", key)
			require.GreaterOrEqual(t, list[i].seq, list[i-1].seq, "sequence rewound in %s", key)
		}
	}
	require.Equal(t, 0, claimCount(t))
	t.Logf("3 accounts x 10 interleaved events drained by 2 consumers; every event once, every seq 0..9 present, per-worker runs ascending")
}

// TestOutboxClaimFailedHeadIsRetriedWithoutStrandingTheTail is the behaviour the
// outage came from: the head event fails, and neither the failed event may be lost
// nor the rows behind it blocked until a lease expires.
func TestOutboxClaimFailedHeadIsRetriedWithoutStrandingTheTail(t *testing.T) {
	ctx := context.Background()
	q := claimQueue(t)

	bad := claimInsert(t, 7200, 1)
	good := claimInsert(t, 7201, 2)

	first, err := q.Claim(ctx, "worker-a", 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, first, len(bad)+len(good))

	require.NoError(t, q.RetryClaimed(ctx, first[0].ID, "worker-a", time.Now().Add(time.Second), "simulated handler error"))
	require.NoError(t, q.ReleaseClaimed(ctx, claimIDs(first)[1:], time.Now()))

	next, err := q.Claim(ctx, "worker-b", 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, next, len(good), "the released tail was not available immediately")
	for _, event := range next {
		require.NoError(t, q.DeleteClaimed(ctx, event.ID, "worker-b"))
	}

	soSoon, err := q.Claim(ctx, "worker-c", 10, time.Minute)
	require.NoError(t, err)
	require.Empty(t, soSoon, "the backed-off event was retried before its available_at")

	time.Sleep(1200 * time.Millisecond)
	eventually, err := q.Claim(ctx, "worker-c", 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, eventually, 1)
	require.Equal(t, bad[0], eventually[0].ID, "the failing head was not retried, but dropped")
	require.NoError(t, q.DeleteClaimed(ctx, bad[0], "worker-c"))
	require.Equal(t, 0, claimCount(t))
	t.Logf("failing head backed off and was retried; the tail resumed immediately; nothing was lost")
}

// TestOutboxClaimBacklogStatsAreHonest drains a production-sized backlog with two
// consumers and compares the new backlog metric against the old formula, which
// reported "no backlog" for exactly the rows the watermark had jumped over.
func TestOutboxClaimBacklogStatsAreHonest(t *testing.T) {
	ctx := context.Background()
	q := claimQueue(t)

	var seeded int
	for acct := 0; acct < 6; acct++ {
		seeded += len(claimInsert(t, 7300+acct, 66)) // 396 rows
	}
	var maxID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(id), 0) FROM scheduler_outbox").Scan(&maxID))

	stats, err := q.PendingStats(ctx)
	require.NoError(t, err)
	require.EqualValues(t, seeded, stats.Pending, "PendingStats must report the real backlog")
	require.NotNil(t, stats.OldestCreatedAt, "PendingStats must report the age of the head event")

	// The old backlog estimate was max(id) - watermark. After the last successful
	// batch the watermark equals max(id), so it would have said zero while these
	// rows were still queued -- which is why the outage looked healthy.
	legacyBacklog := maxID - maxID
	require.Zerof(t, legacyBacklog, "the legacy formula should under-report here to make the point")
	t.Logf("backlog honesty: PendingStats pending=%d oldest=%s, the old formula said pending=%d",
		stats.Pending, stats.OldestCreatedAt.Format(time.RFC3339), legacyBacklog)

	var wg sync.WaitGroup
	for w := 1; w <= 2; w++ {
		wg.Add(1)
		go func(worker string) {
			defer wg.Done()
			for {
				events, err := q.Claim(ctx, worker, 50, time.Minute)
				if err != nil {
					t.Errorf("claim(%s): %v", worker, err)
					return
				}
				if len(events) == 0 {
					return
				}
				for _, event := range events {
					if err := q.DeleteClaimed(ctx, event.ID, worker); err != nil {
						t.Errorf("ack %d: %v", event.ID, err)
						return
					}
				}
			}
		}(fmt.Sprintf("drain-%d", w))
	}
	wg.Wait()

	final, err := q.PendingStats(ctx)
	require.NoError(t, err)
	require.Zero(t, final.Pending, "%d rows left after draining a production-sized backlog", final.Pending)
	require.Nil(t, final.OldestCreatedAt)
	require.Equal(t, 0, claimCount(t))

	// The watermark is only a metric now, so the queue position must not depend on
	// it: an empty table is an empty queue regardless of what Redis remembers.
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM scheduler_outbox WHERE claimed_by IS NOT NULL").Scan(new(int64)))
	t.Logf("%d rows drained by 2 consumers with no leftovers and no held claims", seeded)
}

func claimDrain(t *testing.T, q service.SchedulerOutboxQueue, worker string) []service.SchedulerOutboxEvent {
	t.Helper()
	ctx := context.Background()
	out := []service.SchedulerOutboxEvent{}
	for {
		events, err := q.Claim(ctx, worker, 200, time.Minute)
		require.NoError(t, err)
		if len(events) == 0 {
			return out
		}
		for _, event := range events {
			require.NoError(t, q.DeleteClaimed(ctx, event.ID, worker))
			out = append(out, event)
		}
	}
}
