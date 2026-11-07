//go:build integration

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestOutboxClaimOrderingSurvivesBlockedHead establishes the ordering question
// deterministically: consumer A owns the head event of one account and its
// handler blocks, and only THEN does consumer B start claiming.
//
// If the claim enforces account affinity, B gets nothing for that account while
// A holds the head and the business completion order stays 0..9. If it does not,
// B takes 1..9, finishes them first, and the account's changes are replayed out
// of order -- which is what this test reports.
func TestOutboxClaimOrderingSurvivesBlockedHead(t *testing.T) {
	ctx := context.Background()
	q := claimQueue(t)

	const account = 7401
	const events = 10
	for seq := 0; seq < events; seq++ {
		_, err := integrationDB.ExecContext(ctx, `
			INSERT INTO scheduler_outbox (event_type, account_id, payload)
			VALUES ('account_changed', $1, jsonb_build_object('seq', $2::int))`, account, seq)
		require.NoError(t, err)
	}

	var (
		mu         sync.Mutex
		completed  []int
		claimedByB []int
		blockHead  = make(chan struct{})
		headHeld   = make(chan struct{})
	)

	// --- consumer A: takes exactly the head, then blocks inside its handler ---
	aDone := make(chan struct{})
	go func() {
		defer close(aDone)
		got, err := q.Claim(ctx, "consumer-A", 1, 30*time.Second)
		if err != nil || len(got) != 1 {
			t.Errorf("A claim: err=%v n=%d", err, len(got))
			close(headHeld)
			return
		}
		if seq := claimSeq(got[0].Payload); seq != 0 {
			t.Errorf("A claimed seq=%d, expected the head (0)", seq)
		}
		close(headHeld) // A now owns the head and is about to block
		<-blockHead
		mu.Lock()
		completed = append(completed, 0)
		mu.Unlock()
		if err := q.DeleteClaimed(ctx, got[0].ID, "consumer-A"); err != nil {
			t.Errorf("A ack: %v", err)
		}
	}()

	// A must be holding the head before B is allowed to compete.
	select {
	case <-headHeld:
	case <-time.After(10 * time.Second):
		t.Fatal("consumer A never claimed the head")
	}
	time.Sleep(200 * time.Millisecond)

	// --- consumer B: starts only now, and reports whatever it is allowed to take ---
	bStop := make(chan struct{})
	bDone := make(chan struct{})
	go func() {
		defer close(bDone)
		for {
			select {
			case <-bStop:
				return
			default:
			}
			got, err := q.Claim(ctx, "consumer-B", 10, 30*time.Second)
			if err != nil {
				t.Errorf("B claim: %v", err)
				return
			}
			if len(got) == 0 {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			for _, event := range got {
				seq := claimSeq(event.Payload)
				mu.Lock()
				claimedByB = append(claimedByB, seq)
				completed = append(completed, seq)
				mu.Unlock()
				if err := q.DeleteClaimed(ctx, event.ID, "consumer-B"); err != nil {
					t.Logf("B ack seq=%d rejected: %v", seq, err)
				}
			}
		}
	}()

	// Give B a real chance to overtake while the head is blocked.
	time.Sleep(2 * time.Second)
	mu.Lock()
	overtook := append([]int(nil), claimedByB...)
	mu.Unlock()

	close(blockHead) // release the head
	<-aDone
	time.Sleep(1500 * time.Millisecond)
	close(bStop)
	<-bDone
	claimDrain(t, q, "consumer-C")

	mu.Lock()
	order := append([]int(nil), completed...)
	mu.Unlock()

	t.Logf("while the head was blocked, the second consumer took: %v", overtook)
	t.Logf("business completion order: %v", order)

	require.Emptyf(t, overtook,
		"the second consumer overtook the blocked head of account %d: it took %v -- per-account ordering is NOT guaranteed across consumers", account, overtook)
	require.Lenf(t, order, events, "completion order: %v", order)
	for i, seq := range order {
		require.Equalf(t, i, seq, "completion order %v is not strictly 0..%d", order, events-1)
	}
	require.Equal(t, 0, claimCount(t))
}

// TestOutboxClaimStaleOwnerHasNoSideEffectsAfterTakeover lets a lease expire in
// the middle of handling and checks that the previous owner cannot change
// anything afterwards: its acknowledgement is refused and the row survives for
// the new owner instead of being deleted by both.
//
// Note on the takeover window: eligibility is decided with the *reclaimer's*
// lease, so both consumers must use the same lease for the expiry to be
// observable. The service uses a single constant, which is why that holds in
// production.
func TestOutboxClaimStaleOwnerHasNoSideEffectsAfterTakeover(t *testing.T) {
	ctx := context.Background()
	q := claimQueue(t)
	ids := claimInsert(t, 7402, 1)
	const lease = 2 * time.Second

	old, err := q.Claim(ctx, "stale-owner", 1, lease)
	require.NoError(t, err)
	require.Len(t, old, 1)

	// The handler outlives the lease.
	time.Sleep(lease + 700*time.Millisecond)

	fresh, err := q.Claim(ctx, "new-owner", 1, lease)
	require.NoError(t, err)
	require.Lenf(t, fresh, 1, "an expired claim was not handed to the new owner (lease=%s)", lease)
	require.Equal(t, ids[0], fresh[0].ID)

	// The stale owner finishes and tries to acknowledge: it must be refused and
	// must not remove the row the new owner is working on.
	require.Error(t, q.DeleteClaimed(ctx, ids[0], "stale-owner"),
		"a stale owner acknowledged a row it no longer holds")
	require.Equal(t, 1, claimCount(t), "the stale ack removed the row anyway")

	require.NoError(t, q.DeleteClaimed(ctx, ids[0], "new-owner"))
	require.Equal(t, 0, claimCount(t))
	t.Logf("stale owner refused after a %s lease; the row was deleted exactly once, by the new owner", lease)
}

// TestOutboxClaimBatchStaysFarBelowTheLease bounds the batch: a full batch of the
// production size must finish in a small fraction of the 5 minute lease.
func TestOutboxClaimBatchStaysFarBelowTheLease(t *testing.T) {
	ctx := context.Background()
	q := claimQueue(t)

	const batch = 200
	const lease = 5 * time.Minute
	claimInsert(t, 7403, batch)

	start := time.Now()
	events, err := q.Claim(ctx, "timed-consumer", batch, lease)
	require.NoError(t, err)
	require.Len(t, events, batch)
	for _, event := range events {
		require.NoError(t, q.DeleteClaimed(ctx, event.ID, "timed-consumer"))
	}
	elapsed := time.Since(start)

	t.Logf("full batch of %d events claimed and acknowledged in %s; lease is %s (%.2f%% used)",
		batch, elapsed.Round(time.Millisecond), lease, float64(elapsed)/float64(lease)*100)
	require.Lessf(t, elapsed, lease/10,
		"a full batch took %s, more than a tenth of the %s lease", elapsed, lease)
	require.Equal(t, 0, claimCount(t))
}
