package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

// queueRepoFake implements both the legacy SchedulerOutboxRepository (by embedding
// the existing test fake) and the durable-claim SchedulerOutboxQueue, so these
// tests drive the new pollOutbox path without a database.
type queueRepoFake struct {
	*outboxCleanupRepo

	batches     [][]SchedulerOutboxEvent
	claimCalled int

	acked      []int64
	ackedBy    []string
	ackErrFor  map[int64]error
	retried    []int64
	released   []int64
	statsCalls int
}

func newQueueRepoFake(batches ...[]SchedulerOutboxEvent) *queueRepoFake {
	return &queueRepoFake{
		outboxCleanupRepo: &outboxCleanupRepo{},
		batches:           batches,
		ackErrFor:         map[int64]error{},
	}
}

func (r *queueRepoFake) Claim(_ context.Context, workerID string, _ int, _ time.Duration) ([]SchedulerOutboxEvent, error) {
	r.claimCalled++
	if len(r.batches) == 0 {
		return nil, nil
	}
	batch := r.batches[0]
	r.batches = r.batches[1:]
	out := make([]SchedulerOutboxEvent, 0, len(batch))
	for _, event := range batch {
		clone := event
		out = append(out, clone)
	}
	return out, nil
}

func (r *queueRepoFake) DeleteClaimed(_ context.Context, id int64, workerID string) error {
	if err, ok := r.ackErrFor[id]; ok {
		return err
	}
	r.acked = append(r.acked, id)
	r.ackedBy = append(r.ackedBy, workerID)
	return nil
}

func (r *queueRepoFake) RetryClaimed(_ context.Context, id int64, _ string, _ time.Time, _ string) error {
	r.retried = append(r.retried, id)
	return nil
}

func (r *queueRepoFake) ReleaseClaimed(_ context.Context, ids []int64, _ time.Time) error {
	r.released = append(r.released, ids...)
	return nil
}

func (r *queueRepoFake) PendingStats(context.Context) (SchedulerOutboxPendingStats, error) {
	r.statsCalls++
	return SchedulerOutboxPendingStats{Pending: 0}, nil
}

// watermarkFailingCache promotes the whole SchedulerCache interface and overrides
// only the watermark write, so the metric failure cannot be confused with a
// consumption failure.
type watermarkFailingCache struct {
	SchedulerCache
	attempts []int64
}

func (c *watermarkFailingCache) SetOutboxWatermark(ctx context.Context, id int64) error {
	c.attempts = append(c.attempts, id)
	return errors.New("redis: simulated watermark write failure")
}

func lastUsedEvents(ids ...int64) []SchedulerOutboxEvent {
	out := make([]SchedulerOutboxEvent, 0, len(ids))
	for _, id := range ids {
		out = append(out, SchedulerOutboxEvent{
			ID:        id,
			EventType: SchedulerOutboxEventAccountLastUsed,
			Payload:   map[string]any{"last_used": map[string]any{"7": time.Now().Unix()}},
		})
	}
	return out
}

func TestPollOutboxQueueAcksEveryEventInOrder(t *testing.T) {
	cache := &outboxCleanupCache{}
	repo := newQueueRepoFake(lastUsedEvents(1, 2, 3), nil)
	svc := NewSchedulerSnapshotService(cache, repo, nil, nil, nil)

	svc.pollOutbox()

	if repo.claimCalled == 0 {
		t.Fatal("the durable-claim path was not used")
	}
	if len(repo.acked) != 3 || repo.acked[0] != 1 || repo.acked[2] != 3 {
		t.Fatalf("acked=%v want [1 2 3] in id order", repo.acked)
	}
	if len(repo.released) != 0 || len(repo.retried) != 0 {
		t.Fatalf("nothing should be released or retried on a clean run: released=%v retried=%v", repo.released, repo.retried)
	}
	if len(cache.setWatermarks) != 1 || cache.setWatermarks[0] != 3 {
		t.Fatalf("watermark writes=%v want the last acked id only", cache.setWatermarks)
	}
	for _, worker := range repo.ackedBy {
		if worker != svc.outboxWorkerID || worker == "" {
			t.Fatalf("ack from %q, expected the service worker id %q", worker, svc.outboxWorkerID)
		}
	}
}

func TestPollOutboxQueueStopsAtFailureAndReleasesTheTail(t *testing.T) {
	cache := &outboxCleanupCache{updateErr: errors.New("boom")}
	repo := newQueueRepoFake(lastUsedEvents(10, 11, 12), nil)
	svc := NewSchedulerSnapshotService(cache, repo, nil, nil, nil)

	svc.pollOutbox()

	if len(repo.acked) != 0 {
		t.Fatalf("no event should be acknowledged when the handler fails, got %v", repo.acked)
	}
	if len(repo.retried) != 1 || repo.retried[0] != 10 {
		t.Fatalf("retried=%v want exactly the failed event [10]", repo.retried)
	}
	if len(repo.released) != 2 || repo.released[0] != 11 || repo.released[1] != 12 {
		t.Fatalf("released=%v want the untouched tail [11 12]", repo.released)
	}
	if len(cache.setWatermarks) != 0 {
		t.Fatalf("the watermark must not advance over unprocessed events, got %v", cache.setWatermarks)
	}
}

func TestPollOutboxQueueContinuesWhenOwnershipIsLost(t *testing.T) {
	cache := &outboxCleanupCache{}
	repo := newQueueRepoFake(lastUsedEvents(20, 21, 22), nil)
	repo.ackErrFor[21] = errors.New("scheduler outbox claim 21 is no longer owned by worker")

	svc := NewSchedulerSnapshotService(cache, repo, nil, nil, nil)
	svc.pollOutbox()

	if len(repo.acked) != 2 || repo.acked[0] != 20 || repo.acked[1] != 22 {
		t.Fatalf("acked=%v want the two rows this consumer still owns [20 22]", repo.acked)
	}
	if len(repo.released) != 0 {
		t.Fatalf("a lost claim is not a batch failure, released=%v", repo.released)
	}
	if len(cache.setWatermarks) != 1 || cache.setWatermarks[0] != 22 {
		t.Fatalf("watermark=%v want the last successfully acked id (22)", cache.setWatermarks)
	}
}

func TestPollOutboxQueueWatermarkWriteFailureDoesNotBlockConsumption(t *testing.T) {
	cache := &watermarkFailingCache{SchedulerCache: &outboxCleanupCache{}}
	repo := newQueueRepoFake(lastUsedEvents(30, 31, 32), nil)
	svc := NewSchedulerSnapshotService(cache, repo, nil, nil, nil)

	svc.pollOutbox()

	if len(repo.acked) != 3 {
		t.Fatalf("acked=%v want all three events acknowledged despite the metric failure", repo.acked)
	}
	if len(repo.released) != 0 || len(repo.retried) != 0 {
		t.Fatalf("nothing should be released, released=%v retried=%v", repo.released, repo.retried)
	}
	if len(cache.attempts) != 1 || cache.attempts[0] != 32 {
		t.Fatalf("the watermark attempt should still have been made, got %v", cache.attempts)
	}
}

// TestPollOutboxQueueEmptyQueueClearsDegradedEpisode keeps the healthy fast path
// behaviour: an empty claim must not touch the watermark.
func TestPollOutboxQueueEmptyQueueClearsDegradedEpisode(t *testing.T) {
	cache := &outboxCleanupCache{}
	repo := newQueueRepoFake(nil)
	svc := NewSchedulerSnapshotService(cache, repo, nil, nil, nil)

	svc.pollOutbox()

	if repo.claimCalled != 1 {
		t.Fatalf("claimCalled=%d want 1", repo.claimCalled)
	}
	if len(repo.acked) != 0 || len(cache.setWatermarks) != 0 {
		t.Fatalf("an empty queue must not ack or advance the watermark: acked=%v wm=%v", repo.acked, cache.setWatermarks)
	}
}

// TestPollOutboxFallsBackToLegacyPathWithoutQueue documents the compatibility
// shim: a repository that does not implement SchedulerOutboxQueue keeps the old
// watermark-driven behaviour, which is what the existing cleanup tests cover.
func TestPollOutboxFallsBackToLegacyPathWithoutQueue(t *testing.T) {
	cache := &outboxCleanupCache{}
	repo := &outboxCleanupRepo{
		events: []SchedulerOutboxEvent{
			{ID: 5, EventType: SchedulerOutboxEventAccountLastUsed},
		},
		rows:         []int64{5, 6},
		lockAcquired: true,
	}
	svc := NewSchedulerSnapshotService(cache, repo, nil, nil, nil)

	svc.pollOutbox()

	if len(cache.setWatermarks) != 1 || cache.setWatermarks[0] != 5 {
		t.Fatalf("legacy path should still advance the watermark, got %v", cache.setWatermarks)
	}
	if len(repo.deleteCalls) == 0 {
		t.Fatal("legacy path should still run the watermark cleanup")
	}
}
