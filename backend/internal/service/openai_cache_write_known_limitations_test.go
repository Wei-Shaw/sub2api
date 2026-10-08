package service

import (
	"testing"
	"time"
)

func TestOpenAICacheWriteKnownLimitation_SingleLocalInstanceCanMisattributeExternalWrite(t *testing.T) {
	t.Parallel()
	a := &openAICacheWriteInferenceTracker{}
	at := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	prompt := buildOpenAICacheWritePromptEvidence([]byte(`{"model":"gpt-6-astra","input":[{"role":"user","content":"full prompt P"}]}`))
	output := buildOpenAICacheWriteOutputEvidence([]byte(`{"status":"completed","output":[]}`))
	if !prompt.Valid || !output.Valid {
		t.Fatal("invalid completed-response fixture")
	}
	scope := openAICacheWriteTrackerKey(7, "", "same-prefix", 55)
	observeOnA := func(id string, started time.Time, read int) (openAICacheWriteInference, string, bool) {
		t.Helper()
		request := a.beginAdmission(OpenAICacheWriteRequest{
			ID: id, APIKeyID: 55, Epoch: a.settingEpoch, StartedAt: started,
			scopeKey: scope, prompt: prompt,
		})
		state, ok := a.finishAdmission(request)
		if !ok || state.unsafe {
			t.Fatalf("%s must have locally valid non-overlapping admission", id)
		}
		return a.observe(openAICacheWriteObservation{
			AccountID: 7, APIKeyID: request.APIKeyID, Epoch: request.Epoch,
			Model: "gpt-6-astra", CacheIdentity: "same-prefix", scopeKey: request.scopeKey,
			StartedAt: request.StartedAt, ObservedAt: started.Add(time.Second),
			PredecessorID: request.predecessorID, admitted: !state.unsafe,
			Prompt: request.prompt, Output: output, InputTokens: 1000,
			CacheReadTokens: read, CacheWriteState: openAICacheWriteFieldAbsent,
		}, id)
	}

	// Synthetic hidden ground truth, never supplied to A: R1 writes nothing;
	// then R2 on invisible replica B (or an external writer) fills P's 1000-token
	// prefix before R3. No actual distributed network or upstream is simulated.
	actualWrites := map[string]int{"R1": 0, "R2-on-B": 1000}
	hitSource := "R2-on-B"
	// Counts are artificial, chosen to satisfy the existing inference checks;
	// they make no assertion about a provider's minimum cacheable prefix size.
	if _, _, inferred := observeOnA("R1", at, 0); inferred {
		t.Fatal("R1 should only establish a baseline")
	}
	// A receives only R1 and R3: identical full prompt, missing write fields,
	// and input/read counters of 1000/0 followed by 1000/1000.
	got, attributedTo, inferred := observeOnA("R3", at.Add(3*time.Second), 1000)
	if len(a.seen) != 2 {
		t.Fatalf("A observed %d completions; want only R1 and R3", len(a.seen))
	}
	for _, seen := range a.seen {
		if seen.observationID != "R1" && seen.observationID != "R3" {
			t.Fatalf("hidden writer observation was delivered to A: %q", seen.observationID)
		}
	}
	// LIMITATION: this deliberately asserts misattribution, not billing correctness.
	// Experimental estimates cannot identify the writer, even with one local
	// service instance when an external writer can populate the upstream cache.
	if !inferred || attributedTo != "R1" || got.Tokens != 1000 || got.Source != "inferred_next_hit" {
		t.Fatalf("known-limitation behavior changed: inferred=%v attributed=%q estimate=%+v", inferred, attributedTo, got)
	}
	if actualWrites[attributedTo] == got.Tokens || attributedTo == hitSource || actualWrites[hitSource] != got.Tokens {
		t.Fatal("fixture must demonstrate R1 was attributed an estimate for B's actual write")
	}
}
