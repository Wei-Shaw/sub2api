package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// TestContentModerationQueueStartsSmallAndGrowsToQueueSize pins the lazy
// allocation shape: the first enqueue creates min(initial, QueueSize) slots,
// growth doubles the capacity, and capacity never exceeds the configured
// QueueSize drop threshold.
func TestContentModerationQueueStartsSmallAndGrowsToQueueSize(t *testing.T) {
	small := defaultContentModerationConfig()
	small.QueueSize = 8
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	if svc.asyncQueueOf() != nil {
		t.Fatal("new service must start without an allocated queue")
	}
	svc.enqueueAsync(ContentModerationCheckInput{}, small, ContentModerationInput{}, "small")
	if got := cap(svc.asyncQueueOf()); got != 8 {
		t.Fatalf("first queue capacity for QueueSize=8 = %d, want 8", got)
	}

	cfg := defaultContentModerationConfig()
	cfg.QueueSize = 100
	grown := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
	for i := 0; i < contentModerationInitialQueueCapacity; i++ {
		grown.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{}, fmt.Sprintf("fill-%d", i))
	}
	if got := cap(grown.asyncQueueOf()); got != contentModerationInitialQueueCapacity {
		t.Fatalf("queue capacity before growth = %d, want %d", got, contentModerationInitialQueueCapacity)
	}
	grown.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{}, "grow")
	if got := cap(grown.asyncQueueOf()); got != 100 {
		t.Fatalf("queue capacity after growth = %d, want doubling capped at QueueSize=100", got)
	}
	if got := len(grown.asyncQueueOf()); got != contentModerationInitialQueueCapacity+1 {
		t.Fatalf("pending tasks after growth = %d, want %d (growth must keep queued tasks)", got, contentModerationInitialQueueCapacity+1)
	}
}

// newConcurrentQueueTestService returns a constructor-built service with no
// setting repository, so it has the lazy queue but starts no worker goroutines.
// The queue tests drive dequeueAsyncTask directly, which is the exact call the
// worker loop makes. A real worker() cannot be used inside synctest because it
// has no stop handle; TestContentModerationWorkerConsumesConcurrentQueue covers
// the real worker loop separately.
func newConcurrentQueueTestService() *ContentModerationService {
	return NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
}

// TestContentModerationQueueConcurrentProducersAndConsumersExactlyOnce forces
// the lazy queue through several growth steps while four consumers are already
// parked, then releases them. It proves the properties the old single-producer
// test could not see: every unique task is consumed exactly once, growth does
// not drop or duplicate a task, no task is stranded after the consumers drain,
// and the queue actually grew beyond its initial 64-slot allocation.
func TestContentModerationQueueConcurrentProducersAndConsumersExactlyOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newConcurrentQueueTestService()
		cfg := defaultContentModerationConfig()
		cfg.QueueSize = 4096

		const producerCount = 8
		const perProducer = 200
		const consumerCount = 4
		const total = producerCount * perProducer

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var mu sync.Mutex
		received := make(map[string]int, total)
		consumersDone := make(chan struct{}, consumerCount)
		release := make(chan struct{})

		for i := 0; i < consumerCount; i++ {
			go func() {
				defer func() { consumersDone <- struct{}{} }()
				<-release
				for {
					task, ok := svc.dequeueAsyncTask(ctx, time.Minute)
					if !ok {
						return
					}
					mu.Lock()
					received[task.inputHash]++
					mu.Unlock()
				}
			}()
		}
		synctest.Wait() // every consumer is durably parked on release

		var producers sync.WaitGroup
		for p := 0; p < producerCount; p++ {
			producers.Add(1)
			go func(producer int) {
				defer producers.Done()
				for i := 0; i < perProducer; i++ {
					svc.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{}, fmt.Sprintf("p%d-%d", producer, i))
				}
			}(p)
		}
		producers.Wait()

		if got := svc.asyncEnqueued.Load(); got != total {
			t.Fatalf("asyncEnqueued = %d, want %d", got, total)
		}
		if got := svc.asyncDropped.Load(); got != 0 {
			t.Fatalf("asyncDropped = %d, want 0 below the configured threshold", got)
		}
		if got := cap(svc.asyncQueueOf()); got <= contentModerationInitialQueueCapacity {
			t.Fatalf("queue capacity = %d, want growth beyond the initial %d", got, contentModerationInitialQueueCapacity)
		}
		if got := len(svc.asyncQueueOf()); got != total {
			t.Fatalf("queued before consumers released = %d, want %d", got, total)
		}

		close(release)
		synctest.Wait()

		mu.Lock()
		unique := 0
		duplicates := 0
		emptyID := false
		for id, count := range received {
			if id == "" {
				emptyID = true
			}
			unique++
			if count > 1 {
				duplicates += count - 1
			}
		}
		mu.Unlock()
		if emptyID {
			t.Fatal("consumer received a task without an input hash")
		}
		if unique != total {
			t.Fatalf("unique task ids consumed = %d, want %d", unique, total)
		}
		if duplicates != 0 {
			t.Fatalf("duplicate deliveries = %d, want 0", duplicates)
		}
		if got := len(svc.asyncQueueOf()); got != 0 {
			t.Fatalf("queue length after consumers drained = %d, want 0 (stranded tasks)", got)
		}

		cancel()
		for i := 0; i < consumerCount; i++ {
			<-consumersDone
		}
	})
}

// TestContentModerationQueueConsumerOverlapsGrowth proves the missing overlap
// property directly, using the same dequeueAsyncTask call the real worker loop
// makes:
//
//  1. A seed phase consumes 20 tasks, proving the consumer entered its loop and
//     made progress before any growth.
//  2. The consumer then blocks INSIDE dequeueAsyncTask on the empty 64-slot
//     queue. A producer burst of 500 tasks grows the channel 64->128->256->512
//     while that read is in flight; growAsyncQueueLocked replaces the channel
//     and wakes the parked read. Capacity growth, the in-flight read, and the
//     consumer's +1 progress are all asserted explicitly.
//  3. A sustained phase keeps both loops progressing in the same window: each
//     round writes 50 tasks, releases 50 permits, and waits for only 25 reads,
//     so permits stay outstanding while the next round is being written. Reads
//     taken while the producer has not finished writing are counted, which
//     proves reads and writes interleave instead of running in two disjoint
//     phases.
//
// A final drain proves every task is consumed exactly once with no stranded
// queue entries. synctest makes every phase deterministic and instant.
func TestContentModerationQueueConsumerOverlapsGrowth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newConcurrentQueueTestService()
		cfg := defaultContentModerationConfig()
		cfg.QueueSize = 4096

		const seedTasks = 20
		const growthBurst = 500
		const overlapRounds = 6
		const overlapWritePerHalf = 25
		const overlapPermitsPerHalf = 10
		const overlapMinReadsPerHalf = 5
		const overlapTasks = overlapRounds * 2 * overlapWritePerHalf
		const total = seedTasks + growthBurst + overlapTasks

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		permits := make(chan struct{}, total+1)
		consumedSignal := make(chan struct{}, total+1)
		consumerDone := make(chan struct{})
		producerDone := make(chan struct{})
		var producerActive atomic.Bool
		var phaseWritten atomic.Int64
		var consumerInDequeue atomic.Int64
		var consumed atomic.Int64
		var overlapReads atomic.Int64
		var midPhaseReads atomic.Int64
		var mu sync.Mutex
		received := make(map[string]int, total)

		go func() {
			defer close(consumerDone)
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-permits:
					if !ok {
						return
					}
				}
				consumerInDequeue.Add(1)
				task, ok := svc.dequeueAsyncTask(ctx, time.Minute)
				consumerInDequeue.Add(-1)
				if !ok {
					return
				}
				mu.Lock()
				received[task.inputHash]++
				mu.Unlock()
				if producerActive.Load() {
					overlapReads.Add(1)
					if phaseWritten.Load() < overlapTasks {
						midPhaseReads.Add(1)
					}
				}
				consumed.Add(1)
				consumedSignal <- struct{}{}
			}
		}()

		// Phase 1: progress before growth.
		for i := 0; i < seedTasks; i++ {
			svc.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{}, fmt.Sprintf("seed-%d", i))
		}
		for i := 0; i < seedTasks; i++ {
			permits <- struct{}{}
		}
		for i := 0; i < seedTasks; i++ {
			<-consumedSignal
		}
		if got := consumed.Load(); got != seedTasks {
			t.Fatalf("consumer progress before growth = %d, want %d", got, seedTasks)
		}
		if got := len(svc.asyncQueueOf()); got != 0 {
			t.Fatalf("queue length after seed drain = %d, want 0", got)
		}
		capBefore := cap(svc.asyncQueueOf())
		if capBefore != contentModerationInitialQueueCapacity {
			t.Fatalf("initial queue capacity = %d, want %d", capBefore, contentModerationInitialQueueCapacity)
		}
		consumedBeforeGrowth := consumed.Load()

		// Phase 2: one read is in flight while the producer burst grows the queue.
		permits <- struct{}{}
		synctest.Wait() // consumer is durably blocked inside dequeueAsyncTask
		if got := consumerInDequeue.Load(); got != 1 {
			t.Fatalf("consumer in-flight dequeue count = %d before growth burst, want 1", got)
		}
		for i := 0; i < growthBurst; i++ {
			svc.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{}, fmt.Sprintf("grow-%d", i))
		}
		synctest.Wait()
		capAfterGrowth := cap(svc.asyncQueueOf())
		if capAfterGrowth <= contentModerationInitialQueueCapacity {
			t.Fatalf("queue capacity after growth burst = %d, want more than initial %d", capAfterGrowth, contentModerationInitialQueueCapacity)
		}
		if capAfterGrowth < 512 {
			t.Fatalf("queue capacity after %d growth tasks = %d, want at least 512", growthBurst, capAfterGrowth)
		}
		if got := consumed.Load(); got != consumedBeforeGrowth+1 {
			t.Fatalf("consumer progress across queue growth = %d, want %d", got, consumedBeforeGrowth+1)
		}
		if got := svc.asyncDropped.Load(); got != 0 {
			t.Fatalf("asyncDropped during growth = %d, want 0", got)
		}

		// Phase 3: sustained interleaving. Every half-round writes 25 tasks and
		// then releases 10 permits, waiting for 5 reads before writing the next
		// 25. The producer therefore yields mid-round with reads still
		// outstanding, so reads land between writes instead of after the whole
		// burst. The queue is above 512 in round 0, proving a second observed
		// growth while the consumer loops.
		producerActive.Store(true)
		go func() {
			defer close(producerDone)
			for round := 0; round < overlapRounds; round++ {
				for half := 0; half < 2; half++ {
					target := consumed.Load() + overlapMinReadsPerHalf
					for i := 0; i < overlapWritePerHalf; i++ {
						svc.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{},
							fmt.Sprintf("overlap-%d-%d-%d", round, half, i))
						phaseWritten.Add(1)
					}
					for i := 0; i < overlapPermitsPerHalf; i++ {
						permits <- struct{}{}
					}
					for consumed.Load() < target {
						<-consumedSignal
					}
				}
			}
			producerActive.Store(false)
		}()

		<-producerDone
		capAfterOverlap := cap(svc.asyncQueueOf())
		if capAfterOverlap < 1024 {
			t.Fatalf("queue capacity after overlap phase = %d, want at least 1024", capAfterOverlap)
		}
		if got := overlapReads.Load(); got < int64(overlapRounds*2*overlapMinReadsPerHalf) {
			t.Fatalf("consumer reads while producer phase was active = %d, want at least %d", got, overlapRounds*2*overlapMinReadsPerHalf)
		}
		if got := midPhaseReads.Load(); got < int64(overlapRounds*2*overlapMinReadsPerHalf) {
			t.Fatalf("consumer reads before the producer finished writing = %d, want at least %d", got, overlapRounds*2*overlapMinReadsPerHalf)
		}
		if got := consumed.Load(); got <= consumedBeforeGrowth {
			t.Fatalf("consumer progress across growth = %d, want more than %d", got, consumedBeforeGrowth)
		}
		if got := svc.asyncDropped.Load(); got != 0 {
			t.Fatalf("asyncDropped during overlap = %d, want 0", got)
		}

		// Drain the remaining backlog; every task is already queued, so the
		// consumer never waits on an empty queue during this pass.
		remaining := total - int(consumed.Load())
		for i := 0; i < remaining; i++ {
			permits <- struct{}{}
		}
		for consumed.Load() < total {
			<-consumedSignal
		}
		close(permits)
		<-consumerDone

		if got := svc.asyncEnqueued.Load(); got != total {
			t.Fatalf("asyncEnqueued = %d, want %d", got, total)
		}
		if got := svc.asyncDropped.Load(); got != 0 {
			t.Fatalf("asyncDropped = %d, want 0 below the configured threshold", got)
		}
		if got := len(svc.asyncQueueOf()); got != 0 {
			t.Fatalf("queue length after all consumers stopped = %d, want 0 (stranded tasks)", got)
		}
		if got := consumed.Load(); got != total {
			t.Fatalf("consumer progress = %d, want %d", got, total)
		}

		mu.Lock()
		unique := 0
		duplicates := 0
		emptyID := false
		for id, count := range received {
			if id == "" {
				emptyID = true
			}
			unique++
			if count > 1 {
				duplicates += count - 1
			}
		}
		mu.Unlock()
		if emptyID {
			t.Fatal("consumer received a task without an input hash")
		}
		if unique != total {
			t.Fatalf("unique task ids consumed = %d, want %d", unique, total)
		}
		if duplicates != 0 {
			t.Fatalf("duplicate deliveries = %d, want 0", duplicates)
		}
	})
}

// TestContentModerationDequeueWakesOnFirstLazyQueue pins the first-enqueue wake:
// a consumer parked while the queue is still nil must be released by the
// enqueue that lazily creates the channel, not by its idle timer.
func TestContentModerationDequeueWakesOnFirstLazyQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newConcurrentQueueTestService()
		if svc.asyncQueueOf() != nil {
			t.Fatal("new service must start without an allocated queue")
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		result := make(chan contentModerationTask, 1)
		go func() {
			task, ok := svc.dequeueAsyncTask(ctx, time.Hour)
			if ok {
				result <- task
			}
			close(result)
		}()
		synctest.Wait() // consumer is durably parked on the nil queue

		cfg := defaultContentModerationConfig()
		cfg.QueueSize = 8
		svc.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{}, "first")
		synctest.Wait()

		task, ok := <-result
		if !ok {
			t.Fatal("consumer did not receive the first task")
		}
		if task.inputHash != "first" {
			t.Fatalf("consumer received %q, want the first task", task.inputHash)
		}
	})
}

// TestContentModerationDequeueWakesWhenQueueReplaced pins the old-waiting-queue
// to new-queue hand-off. growAsyncQueueLocked swaps the channel and calls
// wakeWorkersLocked; a consumer already blocked on the old channel must re-read
// the new channel instead of waiting for its one-second idle timer. The swap is
// performed directly under asyncQueueMu because a real grow can only happen
// while the old queue is full, which would let the consumer receive from the old
// channel instead of exercising the blocked hand-off.
func TestContentModerationDequeueWakesWhenQueueReplaced(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newConcurrentQueueTestService()
		cfg := defaultContentModerationConfig()
		cfg.QueueSize = 8
		svc.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{}, "seed")
		seed, ok := svc.dequeueAsyncTask(context.Background(), time.Second)
		if !ok || seed.inputHash != "seed" {
			t.Fatalf("seed dequeue = %q, %v", seed.inputHash, ok)
		}
		if got := len(svc.asyncQueueOf()); got != 0 {
			t.Fatalf("queue length before replacement = %d, want 0", got)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan contentModerationTask, 1)
		go func() {
			task, ok := svc.dequeueAsyncTask(ctx, time.Hour)
			if ok {
				result <- task
			}
			close(result)
		}()
		synctest.Wait() // consumer is durably parked on the old empty channel

		svc.asyncQueueMu.Lock()
		grown := make(chan contentModerationTask, 128)
		grown <- contentModerationTask{inputHash: "replacement"}
		svc.asyncQueue = grown
		svc.wakeWorkersLocked()
		svc.asyncQueueMu.Unlock()

		synctest.Wait()
		task, ok := <-result
		if !ok {
			t.Fatal("consumer did not wake after the queue was replaced")
		}
		if task.inputHash != "replacement" {
			t.Fatalf("consumer received %q, want the replacement task", task.inputHash)
		}
	})
}

// TestContentModerationDequeueHonorsCancellationAndIdleTimeout covers both exit
// paths of dequeueAsyncTask: a cancelled context returns immediately and an
// empty idle queue returns false when the idle timer fires. It must not spin or
// return a zero-value task as if it were real work.
func TestContentModerationDequeueHonorsCancellationAndIdleTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newConcurrentQueueTestService()

		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		if _, ok := svc.dequeueAsyncTask(cancelled, time.Second); ok {
			t.Fatal("cancelled context must not dequeue a task")
		}

		start := time.Now()
		if _, ok := svc.dequeueAsyncTask(context.Background(), 10*time.Second); ok {
			t.Fatal("idle queue must not dequeue a task")
		}
		if elapsed := time.Since(start); elapsed != 10*time.Second {
			t.Fatalf("idle wait = %s, want 10s", elapsed)
		}
	})
}
