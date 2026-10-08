package service

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// These simulations exercise the production tracker directly. No inference
// implementation is copied into the tests. Times are synthetic and fixed; no
// sleeps, wall-clock timing, network calls, settings globals, or billing writes
// are involved.
//
// A fixture's branch, request-start time, source request, and actual write count
// describe a deliberately constructed upstream world. They are NOT derivable
// from usage counters and are NOT supplied to observe. The safe-invariant tests
// intentionally fail when the tracker attributes a write contradicted by that
// fixture ground truth. Passing arithmetic tests alone does not establish real
// prompt-prefix identity or prove that a cache hit came from the prior request.

func cacheWriteSimulationStart() time.Time {
	return time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC)
}

func cacheWriteSimulationObservation(input, read int, at time.Time) openAICacheWriteObservation {
	return openAICacheWriteObservation{
		AccountID:       501,
		Output:          openAICacheWriteOutputEvidence{Valid: true},
		APIKeyID:        601,
		admitted:        true,
		StartedAt:       at,
		Prompt:          buildOpenAICacheWritePromptEvidence([]byte(`{"model":"gpt-6-astra","input":[{"role":"user","content":"fixture prompt"}]}`)),
		Model:           "gpt-6-astra",
		CacheIdentity:   "simulation-session",
		InputTokens:     input,
		CacheReadTokens: read,
		CacheWriteState: openAICacheWriteFieldAbsent,
		ObservedAt:      at,
	}
}

func cacheWriteSimulationRequireInference(t *testing.T, tracker *openAICacheWriteInferenceTracker, observation openAICacheWriteObservation, id, previousID string, tokens, residual int) {
	t.Helper()
	observation.PredecessorID = previousID
	observation.scopeKey = openAICacheWriteTrackerKey(observation.AccountID, observation.Model, observation.CacheIdentity)
	got, previous, ok := cacheWriteSimulationObserve(tracker, observation, id)
	if !ok || previous != previousID || got.Tokens != tokens || got.ResidualInputTokens != residual || got.Source != "inferred_next_hit" {
		t.Fatalf("observe(%q): accepted=%v previous=%q inference=%+v; want previous=%q tokens=%d residual=%d source=inferred_next_hit", id, ok, previous, got, previousID, tokens, residual)
	}
}

func cacheWriteSimulationRequireNoInference(t *testing.T, tracker *openAICacheWriteInferenceTracker, observation openAICacheWriteObservation, id string) {
	t.Helper()
	observation.scopeKey = openAICacheWriteTrackerKey(observation.AccountID, observation.Model, observation.CacheIdentity)
	got, previous, ok := cacheWriteSimulationObserve(tracker, observation, id)
	if ok {
		t.Fatalf("observe(%q) unexpectedly attributed %d tokens to %q", id, got.Tokens, previous)
	}
}

func TestOpenAICacheWriteSimulationTracker_SequentialTwoAndThreeTurns(t *testing.T) {
	t.Parallel()
	for _, count := range []int{2, 3} {
		t.Run(fmt.Sprintf("%d_turns", count), func(t *testing.T) {
			tracker := &openAICacheWriteInferenceTracker{}
			start := cacheWriteSimulationStart()
			cacheWriteSimulationRequireNoInference(t, tracker, cacheWriteSimulationObservation(10000, 8000, start), "turn-1")
			for turn := 2; turn <= count; turn++ {
				observation := cacheWriteSimulationObservation(10000+(turn-1)*1000, 8000+(turn-1)*1000, start.Add(time.Duration(turn-1)*time.Second))
				cacheWriteSimulationRequireInference(t, tracker, observation, fmt.Sprintf("turn-%d", turn), fmt.Sprintf("turn-%d", turn-1), 1000, 1000)
			}
			key := openAICacheWriteTrackerKey(501, "gpt-6-astra", "simulation-session")
			if got := tracker.entries[key].observation.Sequence; got != uint64(count) {
				t.Fatalf("sequence=%d; want %d", got, count)
			}
		})
	}
}

func TestOpenAICacheWriteSimulationTracker_IsolatesSessionAccountAndModel(t *testing.T) {
	t.Parallel()
	for _, dimension := range []string{"session", "account", "model"} {
		t.Run(dimension, func(t *testing.T) {
			tracker := &openAICacheWriteInferenceTracker{}
			start := cacheWriteSimulationStart()
			a := cacheWriteSimulationObservation(10000, 8000, start)
			b := cacheWriteSimulationObservation(12000, 8500, start.Add(time.Second))
			switch dimension {
			case "session":
				b.CacheIdentity = "second-session"
			case "account":
				b.AccountID++
			case "model":
				b.Model = "gpt-6-sol"
			}
			cacheWriteSimulationRequireNoInference(t, tracker, a, "a-1")
			cacheWriteSimulationRequireNoInference(t, tracker, b, "b-1")
			a.InputTokens, a.CacheReadTokens, a.ObservedAt = 11000, 9000, start.Add(2*time.Second)
			b.InputTokens, b.CacheReadTokens, b.ObservedAt = 13000, 9500, start.Add(3*time.Second)
			cacheWriteSimulationRequireInference(t, tracker, a, "a-2", "a-1", 1000, 1000)
			cacheWriteSimulationRequireInference(t, tracker, b, "b-2", "b-1", 1000, 2500)
			if len(tracker.entries) != 2 {
				t.Fatalf("tracker entries=%d; want two isolated lineages", len(tracker.entries))
			}
		})
	}
}

func TestOpenAICacheWriteSimulationTracker_OfficialFieldAuthority(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		present   bool
		tokens    int
		wantState openAICacheWriteFieldState
		wantInfer bool
	}{
		{"absent", false, 0, openAICacheWriteFieldAbsent, true},
		{"explicit_zero", true, 0, openAICacheWriteFieldExplicitZero, false},
		{"explicit_positive", true, 750, openAICacheWriteFieldPositive, false},
		{"positive_without_presence_bit", false, 750, openAICacheWriteFieldPositive, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tracker := &openAICacheWriteInferenceTracker{}
			start := cacheWriteSimulationStart()
			first := cacheWriteSimulationObservation(10000, 8000, start)
			first.CacheWriteState = classifyOpenAICacheWriteField(tc.present, tc.tokens)
			if first.CacheWriteState != tc.wantState {
				t.Fatalf("classification=%v; want %v", first.CacheWriteState, tc.wantState)
			}
			cacheWriteSimulationRequireNoInference(t, tracker, first, "official-first")
			next := cacheWriteSimulationObservation(11000, 9000, start.Add(time.Second))
			if tc.wantInfer {
				cacheWriteSimulationRequireInference(t, tracker, next, "next", "official-first", 1000, 1000)
			} else {
				cacheWriteSimulationRequireNoInference(t, tracker, next, "next")
			}
		})
	}
}

func TestOpenAICacheWriteSimulationTracker_CurrentOfficialFieldDoesNotOverridePreviousAbsent(t *testing.T) {
	t.Parallel()
	for _, state := range []openAICacheWriteFieldState{openAICacheWriteFieldExplicitZero, openAICacheWriteFieldPositive} {
		t.Run(fmt.Sprintf("current_state_%d", state), func(t *testing.T) {
			tracker := &openAICacheWriteInferenceTracker{}
			start := cacheWriteSimulationStart()
			cacheWriteSimulationRequireNoInference(t, tracker, cacheWriteSimulationObservation(10000, 8000, start), "missing-field")
			current := cacheWriteSimulationObservation(11000, 9000, start.Add(time.Second))
			current.CacheWriteState = state
			cacheWriteSimulationRequireInference(t, tracker, current, "official-field", "missing-field", 1000, 1000)
			cacheWriteSimulationRequireNoInference(t, tracker, cacheWriteSimulationObservation(12000, 10000, start.Add(2*time.Second)), "following")
		})
	}
}

func TestOpenAICacheWriteSimulationTracker_DivergentHistoryMustNotAttribute(t *testing.T) {
	t.Parallel()
	// Both worlds give the tracker exactly the same observations. Only the test
	// oracle knows whether the later hit came from this request's prompt branch.
	for _, divergent := range []bool{false, true} {
		name := "append_only_fixture"
		if divergent {
			name = "different_prompt_branch_same_session_and_counts"
		}
		t.Run(name, func(t *testing.T) {
			tracker := &openAICacheWriteInferenceTracker{}
			start := cacheWriteSimulationStart()
			actualPreviousWrite := 1000
			hitSourceRequest := "previous"
			if divergent {
				actualPreviousWrite = 0
				hitSourceRequest = "unobserved-request-on-another-branch"
			}
			cacheWriteSimulationRequireNoInference(t, tracker, cacheWriteSimulationObservation(10000, 8000, start), "previous")
			next := cacheWriteSimulationObservation(11000, 9000, start.Add(time.Second))
			next.PredecessorID = "previous"
			if divergent {
				next.Prompt = buildOpenAICacheWritePromptEvidence([]byte(`{"model":"gpt-6-astra","input":[{"role":"user","content":"different branch"}]}`))
			}
			got, previous, ok := cacheWriteSimulationObserve(tracker, next, "next")
			if divergent {
				if ok {
					t.Fatalf("unsafe attribution: tracker assigned %d tokens to %q, but fixture actual write=%d and next-hit source=%q; identical counters/session do not identify a prompt prefix", got.Tokens, previous, actualPreviousWrite, hitSourceRequest)
				}
				return
			}
			if !ok || previous != hitSourceRequest || got.Tokens != actualPreviousWrite {
				t.Fatalf("append-only control: accepted=%v previous=%q inference=%+v", ok, previous, got)
			}
		})
	}
}

func TestOpenAICacheWriteSimulationTracker_ConcurrentCompletionMustNotAttribute(t *testing.T) {
	t.Parallel()
	for _, outOfOrder := range []bool{false, true} {
		name := "overlapping_requests_complete_in_start_order"
		if outOfOrder {
			name = "overlapping_requests_complete_out_of_start_order"
		}
		t.Run(name, func(t *testing.T) {
			tracker := &openAICacheWriteInferenceTracker{}
			start := cacheWriteSimulationStart()
			firstStarted, secondStarted := start, start.Add(time.Second)
			if outOfOrder {
				firstStarted, secondStarted = secondStarted, firstStarted
			}
			first := cacheWriteSimulationObservation(10000, 8000, start.Add(3*time.Second))
			second := cacheWriteSimulationObservation(11000, 9000, start.Add(4*time.Second))
			first.StartedAt, second.StartedAt = firstStarted, secondStarted
			second.PredecessorID = "first-completion"
			if !firstStarted.Before(first.ObservedAt) || !secondStarted.Before(first.ObservedAt) {
				t.Fatal("invalid simulation: both requests must be in flight before the first completes")
			}
			// Fixture ground truth: neither overlapping request admitted a new
			// cache prefix. Their read counters came from separate, pre-existing
			// prefixes. The counters do not encode that fact, or request starts.
			actualFirstWrite := 0
			hitSourceRequest := "pre-existing-cache-populator"
			type outcome struct {
				inference openAICacheWriteInference
				previous  string
				accepted  bool
			}
			outcomes := make(chan outcome, 2)
			firstCompleted := make(chan struct{})
			// The channel schedules completion deterministically. This is a
			// causal-attribution simulation, not a probabilistic mutex-race test.
			go func() {
				got, previous, ok := cacheWriteSimulationObserve(tracker, first, "first-completion")
				outcomes <- outcome{got, previous, ok}
				close(firstCompleted)
			}()
			go func() {
				<-firstCompleted
				got, previous, ok := cacheWriteSimulationObserve(tracker, second, "second-completion")
				outcomes <- outcome{got, previous, ok}
			}()
			baseline, candidate := <-outcomes, <-outcomes
			if baseline.accepted {
				t.Fatal("first completion must only establish a baseline")
			}
			if candidate.accepted {
				t.Fatalf("unsafe overlapping-completion attribution: %d tokens assigned to %q; fixture actual write=%d, hit source=%q, first started=%s, second started=%s", candidate.inference.Tokens, candidate.previous, actualFirstWrite, hitSourceRequest, firstStarted.Format(time.RFC3339), secondStarted.Format(time.RFC3339))
			}
		})
	}
}

func TestOpenAICacheWriteSimulationTracker_AdjacentDuplicateIsIgnored(t *testing.T) {
	t.Parallel()
	tracker := &openAICacheWriteInferenceTracker{}
	start := cacheWriteSimulationStart()
	first := cacheWriteSimulationObservation(10000, 8000, start)
	cacheWriteSimulationRequireNoInference(t, tracker, first, "request-1")
	duplicate := first
	duplicate.CacheReadTokens = 8500
	duplicate.ObservedAt = start.Add(time.Second)
	cacheWriteSimulationRequireNoInference(t, tracker, duplicate, "request-1")
	key := openAICacheWriteTrackerKey(first.AccountID, first.Model, first.CacheIdentity)
	if entry := tracker.entries[key]; entry.observation.Sequence != 1 || entry.observation.CacheReadTokens != 8000 {
		t.Fatalf("adjacent duplicate changed baseline: %+v", entry)
	}
	next := cacheWriteSimulationObservation(11000, 9000, start.Add(2*time.Second))
	cacheWriteSimulationRequireInference(t, tracker, next, "request-2", "request-1", 1000, 1000)
	next.ObservedAt = start.Add(3 * time.Second)
	cacheWriteSimulationRequireNoInference(t, tracker, next, "request-2")
}

func TestOpenAICacheWriteSimulationTracker_NonAdjacentDuplicateMustNotReplaceLineage(t *testing.T) {
	t.Parallel()
	for _, retainOriginalTime := range []bool{false, true} {
		name := "fresh_observation_timestamp"
		if retainOriginalTime {
			name = "original_observation_timestamp"
		}
		t.Run(name, func(t *testing.T) {
			tracker := &openAICacheWriteInferenceTracker{}
			start := cacheWriteSimulationStart()
			first := cacheWriteSimulationObservation(10000, 8000, start)
			cacheWriteSimulationRequireNoInference(t, tracker, first, "request-1")
			cacheWriteSimulationRequireInference(t, tracker, cacheWriteSimulationObservation(11000, 9000, start.Add(time.Second)), "request-2", "request-1", 1000, 1000)
			replay := first
			if !retainOriginalTime {
				replay.ObservedAt = start.Add(2 * time.Second)
			}
			cacheWriteSimulationRequireNoInference(t, tracker, replay, "request-1")
			key := openAICacheWriteTrackerKey(first.AccountID, first.Model, first.CacheIdentity)
			entry := tracker.entries[key]
			if entry.observationID != "request-2" || entry.observation.Sequence != 2 {
				t.Errorf("nonadjacent duplicate replaced live lineage: id=%q sequence=%d; want request-2 sequence=2", entry.observationID, entry.observation.Sequence)
			}
			next := cacheWriteSimulationObservation(12000, 10000, start.Add(3*time.Second))
			next.PredecessorID = "request-2"
			got, previous, ok := cacheWriteSimulationObserve(tracker, next, "request-3")
			if !ok || previous != "request-2" || got.Tokens != 1000 {
				t.Errorf("replay changed next attribution: accepted=%v previous=%q tokens=%d; want request-2/1000 without re-attributing request-1", ok, previous, got.Tokens)
			}
		})
	}
}

func TestOpenAICacheWriteSimulationTracker_TTLBoundaryAndRecovery(t *testing.T) {
	t.Parallel()
	for _, expired := range []bool{false, true} {
		name := "exact_window_still_eligible"
		if expired {
			name = "past_window_restarts_lineage"
		}
		t.Run(name, func(t *testing.T) {
			tracker := &openAICacheWriteInferenceTracker{}
			start := cacheWriteSimulationStart()
			cacheWriteSimulationRequireNoInference(t, tracker, cacheWriteSimulationObservation(10000, 8000, start), "before-gap")
			nextTime := start.Add(defaultOpenAICacheWriteInferenceWindow)
			if expired {
				nextTime = nextTime.Add(time.Nanosecond)
			}
			next := cacheWriteSimulationObservation(11000, 9000, nextTime)
			if expired {
				cacheWriteSimulationRequireNoInference(t, tracker, next, "after-gap")
			} else {
				cacheWriteSimulationRequireInference(t, tracker, next, "after-gap", "before-gap", 1000, 1000)
			}
			cacheWriteSimulationRequireInference(t, tracker, cacheWriteSimulationObservation(12000, 10000, nextTime.Add(time.Second)), "after-rebaseline", "after-gap", 1000, 1000)
		})
	}
}

func cacheWriteSimulationSeedTransientState(tracker *openAICacheWriteInferenceTracker, at time.Time) {
	cacheWriteSimulationObserve(tracker, cacheWriteSimulationObservation(10000, 8000, at), "lineage")
	tracker.attachBillingSnapshot("snapshot-only", openAICacheWriteBillingSnapshot{ObservedAt: at, Epoch: tracker.settingEpoch})
	inference := openAICacheWriteInference{Tokens: 1000, Source: "inferred_next_hit", Epoch: tracker.settingEpoch}
	tracker.registerInference("inference-only", inference, at)
	tracker.attachBillingSnapshot("ready", openAICacheWriteBillingSnapshot{ObservedAt: at, Epoch: tracker.settingEpoch})
	tracker.registerInference("ready", inference, at)
}

func cacheWriteSimulationRequireEmptyState(t *testing.T, tracker *openAICacheWriteInferenceTracker) {
	t.Helper()
	if len(tracker.entries) != 0 || len(tracker.billingSnapshots) != 0 || len(tracker.pendingInferences) != 0 || len(tracker.ready) != 0 {
		t.Fatalf("state not empty: entries=%d snapshots=%d pending=%d ready=%d", len(tracker.entries), len(tracker.billingSnapshots), len(tracker.pendingInferences), len(tracker.ready))
	}
}

func TestOpenAICacheWriteSimulationTracker_TTLPrunesUnreconciledMemory(t *testing.T) {
	t.Parallel()
	tracker := &openAICacheWriteInferenceTracker{}
	start := cacheWriteSimulationStart()
	cacheWriteSimulationSeedTransientState(tracker, start)
	if len(tracker.billingSnapshots) != 1 || len(tracker.pendingInferences) != 1 || len(tracker.ready) != 1 {
		t.Fatal("fixture must contain a snapshot, pending inference, and ready reconciliation")
	}
	// Known limit: unreconciled information is not durable and is discarded by
	// pruning. This tests the tracker queues, not a database or billing worker.
	tracker.mu.Lock()
	tracker.pruneLocked(start.Add(defaultOpenAICacheWriteInferenceWindow + time.Nanosecond))
	tracker.mu.Unlock()
	cacheWriteSimulationRequireEmptyState(t, tracker)
}

func TestOpenAICacheWriteSimulationTracker_ResetAndRestartMemory(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"same_epoch_preserves", "new_epoch_clears", "disable_same_epoch_clears", "process_restart_loses_state"} {
		t.Run(mode, func(t *testing.T) {
			tracker := &openAICacheWriteInferenceTracker{}
			tracker.resetForSettingEpoch(10)
			start := cacheWriteSimulationStart()
			cacheWriteSimulationSeedTransientState(tracker, start)
			switch mode {
			case "same_epoch_preserves":
				tracker.resetForSettingEpoch(10)
				if len(tracker.entries) != 1 || len(tracker.billingSnapshots) != 1 || len(tracker.pendingInferences) != 1 || len(tracker.ready) != 1 {
					t.Fatal("reset at unchanged epoch lost pending state")
				}
				cacheWriteSimulationRequireInference(t, tracker, cacheWriteSimulationObservation(11000, 9000, start.Add(time.Second)), "after", "lineage", 1000, 1000)
				return
			case "new_epoch_clears":
				tracker.resetForSettingEpoch(11)
				if tracker.settingEpoch != 11 {
					t.Fatal("reset did not store the new setting epoch")
				}
			case "disable_same_epoch_clears":
				tracker.clearAtSettingEpoch(10)
			case "process_restart_loses_state":
				tracker = &openAICacheWriteInferenceTracker{}
			}
			cacheWriteSimulationRequireEmptyState(t, tracker)
			cacheWriteSimulationRequireNoInference(t, tracker, cacheWriteSimulationObservation(11000, 9000, start.Add(time.Second)), "after")
			cacheWriteSimulationRequireInference(t, tracker, cacheWriteSimulationObservation(12000, 10000, start.Add(2*time.Second)), "rolling", "after", 1000, 1000)
		})
	}
}

func TestOpenAICacheWriteSimulationTracker_SeparateInstancesHaveNoSharedMemory(t *testing.T) {
	t.Parallel()
	a, b := &openAICacheWriteInferenceTracker{}, &openAICacheWriteInferenceTracker{}
	start := cacheWriteSimulationStart()
	cacheWriteSimulationRequireNoInference(t, a, cacheWriteSimulationObservation(10000, 8000, start), "on-a")
	cacheWriteSimulationRequireNoInference(t, b, cacheWriteSimulationObservation(11000, 9000, start.Add(time.Second)), "on-b")
	// Even if matching snapshot/inference identifiers are delivered separately,
	// in-process queues cannot join information across two service instances.
	a.attachBillingSnapshot("split-request", openAICacheWriteBillingSnapshot{ObservedAt: start})
	b.registerInference("split-request", openAICacheWriteInference{Tokens: 1000}, start.Add(time.Second))
	if len(a.billingSnapshots) != 1 || len(b.pendingInferences) != 1 || len(a.ready) != 0 || len(b.ready) != 0 {
		t.Fatal("unexpected cross-instance reconciliation state")
	}
	if _, ready := a.claimReadyReconciliation(); ready != nil {
		t.Fatal("instance A cannot reconcile a snapshot using instance B's inference")
	}
	if _, ready := b.claimReadyReconciliation(); ready != nil {
		t.Fatal("instance B cannot reconcile an inference using instance A's snapshot")
	}
}

func TestOpenAICacheWriteSimulationTracker_MissingInstanceTurnMustNotOverAttribute(t *testing.T) {
	t.Parallel()
	a, b := &openAICacheWriteInferenceTracker{}, &openAICacheWriteInferenceTracker{}
	start := cacheWriteSimulationStart()
	// Fixture: one append-only conversation; each of turns 1 and 2 actually
	// writes 1000 tokens. Turn 2 is routed to another instance. Instance A never
	// sees it, so its locally assigned "adjacent" sequence is not adjacency in
	// the upstream conversation. No counter reveals which admission wrote what.
	actualTurn1Write := 1000
	cacheWriteSimulationRequireNoInference(t, a, cacheWriteSimulationObservation(10000, 8000, start), "turn-1")
	cacheWriteSimulationRequireNoInference(t, b, cacheWriteSimulationObservation(11000, 9000, start.Add(time.Second)), "turn-2")
	next := cacheWriteSimulationObservation(12000, 10000, start.Add(2*time.Second))
	next.PredecessorID = "turn-2"
	got, previous, ok := cacheWriteSimulationObserve(a, next, "turn-3")
	if ok && (previous != "turn-1" || got.Tokens != actualTurn1Write) {
		t.Fatalf("unsafe missing-turn attribution: instance A assigned %d tokens to %q; fixture turn-1 actually wrote %d and turn-2 on instance B wrote the other 1000", got.Tokens, previous, actualTurn1Write)
	}
}

// This checks actual goroutine access to the same tracker. It is a memory-safety
// check, not evidence that a completion belongs to the prior prompt lineage.
func TestOpenAICacheWriteSimulationTracker_ConcurrentAccessStress(t *testing.T) {
	tracker := &openAICacheWriteInferenceTracker{}
	start := time.Now()
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for turn := 0; turn < 20; turn++ {
				id := fmt.Sprintf("stress-%d-%d", worker, turn)
				// Fixed counters/timestamp avoid manufacturing a billing inference while
				// concurrent map/queue operations run against shared state.
				cacheWriteSimulationObserve(tracker, cacheWriteSimulationObservation(10000, 8000, start), id)
				tracker.attachBillingSnapshot(id, openAICacheWriteBillingSnapshot{ObservedAt: start})
				tracker.registerInference(id, openAICacheWriteInference{Tokens: 1000}, start)
				if key, ready := tracker.claimReadyReconciliation(); ready != nil {
					tracker.finishReadyReconciliation(key, true)
				}
			}
		}(worker)
	}
	wg.Wait()
	for {
		key, ready := tracker.claimReadyReconciliation()
		if ready == nil {
			break
		}
		tracker.finishReadyReconciliation(key, true)
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if len(tracker.entries) != 1 || len(tracker.ready) != 0 || len(tracker.billingSnapshots) != 0 || len(tracker.pendingInferences) != 0 {
		t.Fatalf("unexpected stress state: entries=%d ready=%d snapshots=%d pending=%d", len(tracker.entries), len(tracker.ready), len(tracker.billingSnapshots), len(tracker.pendingInferences))
	}
}

func cacheWriteSimulationObserve(tracker *openAICacheWriteInferenceTracker, observation openAICacheWriteObservation, id string) (openAICacheWriteInference, string, bool) {
	tracker.mu.Lock()
	observation.Epoch = tracker.settingEpoch
	tracker.mu.Unlock()
	if observation.scopeKey == "" {
		observation.scopeKey = openAICacheWriteTrackerKey(observation.AccountID, observation.Model, observation.CacheIdentity)
	}
	tracker.mu.Lock()
	prev, ok := tracker.entries[observation.scopeKey]
	tracker.mu.Unlock()
	if ok && observation.PredecessorID == prev.observationID {
		// Fixture sequential counters represent a direct extra user item, not
		// an identical wire prompt whose total token count mysteriously changed.
		if observation.Prompt.ConfigHash == prev.observation.Prompt.ConfigHash && len(observation.Prompt.InputHashes) == 1 && observation.Prompt.InputHashes[0] == buildOpenAICacheWritePromptEvidence([]byte(`{"model":"gpt-6-astra","input":[{"role":"user","content":"fixture prompt"}]}`)).InputHashes[0] {
			observation.Prompt = cacheWriteFixtureSuccessorPrompt(prev.observation, observation, id)
		}
	}
	return tracker.observe(observation, id)
}
