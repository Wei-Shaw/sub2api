package service

import (
	"fmt"
	"testing"
	"time"
)

func cacheWriteAdmissionFixture(t *testing.T, tracker *openAICacheWriteInferenceTracker, id string, at time.Time, input string) *OpenAICacheWriteRequest {
	t.Helper()
	prompt := buildOpenAICacheWritePromptEvidence([]byte(`{"model":"gpt-6-astra","input":` + input + `}`))
	if !prompt.Valid {
		t.Fatal("invalid test prompt")
	}
	return tracker.beginAdmission(OpenAICacheWriteRequest{ID: id, APIKeyID: 55, Epoch: tracker.settingEpoch, StartedAt: at, scopeKey: openAICacheWriteTrackerKey(7, "gpt-6-astra", "session", 55), prompt: prompt})
}
func cacheWriteAdmissionFinish(t *testing.T, tracker *openAICacheWriteInferenceTracker, request *OpenAICacheWriteRequest, at time.Time, input, read int, output string) (openAICacheWriteInference, bool) {
	t.Helper()
	if request == nil {
		t.Fatal("no admission")
	}
	state, ok := tracker.finishAdmission(request)
	if !ok {
		t.Fatal("missing admission")
	}
	evidence := buildOpenAICacheWriteOutputEvidence([]byte(`{"status":"completed","output":` + output + `}`))
	obs := openAICacheWriteObservation{AccountID: 7, APIKeyID: request.APIKeyID, Epoch: request.Epoch, Model: "gpt-6-astra", CacheIdentity: "session", scopeKey: request.scopeKey, admitted: !state.unsafe, StartedAt: request.StartedAt, ObservedAt: at, PredecessorID: request.predecessorID, Prompt: request.prompt, Output: evidence, InputTokens: input, CacheReadTokens: read}
	got, _, ok := tracker.observe(obs, request.ID)
	return got, ok
}
func TestOpenAICacheWriteAdmission_IdenticalSequentialPromptStillBills(t *testing.T) {
	tracker := &openAICacheWriteInferenceTracker{}
	at := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	input := `[{"role":"user","content":"same full prompt"}]`
	a := cacheWriteAdmissionFixture(t, tracker, "a", at, input)
	if _, ok := cacheWriteAdmissionFinish(t, tracker, a, at.Add(time.Second), 10818, 0, `[]`); ok {
		t.Fatal("first inferred")
	}
	b := cacheWriteAdmissionFixture(t, tracker, "b", at.Add(2*time.Second), input)
	got, ok := cacheWriteAdmissionFinish(t, tracker, b, at.Add(3*time.Second), 10818, 9984, `[]`)
	if !ok || got.Tokens != 9984 || got.ResidualInputTokens != 834 {
		t.Fatalf("lost supported inference: %+v %v", got, ok)
	}
}
func TestOpenAICacheWriteAdmission_DirectSuccessorAndMissingInstanceTurn(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			a, b := &openAICacheWriteInferenceTracker{}, &openAICacheWriteInferenceTracker{}
			at := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
			first := `[{"role":"user","content":"u1"}]`
			second := `[{"role":"user","content":"u1"},{"role":"assistant","content":"a1"},{"role":"user","content":"u2"}]`
			third := `[{"role":"user","content":"u1"},{"role":"assistant","content":"a1"},{"role":"user","content":"u2"},{"role":"assistant","content":"a2"},{"role":"user","content":"u3"}]`
			req := cacheWriteAdmissionFixture(t, a, "first", at, first)
			cacheWriteAdmissionFinish(t, a, req, at.Add(time.Second), 10000, 8000, `[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"a1"}]}]`)
			target := a
			if missing {
				target = b
			}
			req = cacheWriteAdmissionFixture(t, target, "second", at.Add(2*time.Second), second)
			got, ok := cacheWriteAdmissionFinish(t, target, req, at.Add(3*time.Second), 11000, 9000, `[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"a2"}]}]`)
			if !missing && (!ok || got.Tokens != 1000) {
				t.Fatalf("direct successor failed %+v", got)
			}
			req = cacheWriteAdmissionFixture(t, a, "third", at.Add(4*time.Second), third)
			got, ok = cacheWriteAdmissionFinish(t, a, req, at.Add(5*time.Second), 12000, 10000, `[]`)
			if missing && ok {
				t.Fatalf("unseen intermediate turn charged: %+v", got)
			}
			if !missing && (!ok || got.Tokens != 1000) {
				t.Fatalf("third successor failed %+v", got)
			}
		})
	}
}
func TestOpenAICacheWriteAdmission_OverlapInvalidatesBothReceipts(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			tracker := &openAICacheWriteInferenceTracker{}
			at := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
			input := `[{"role":"user","content":"same prompt"}]`
			a := cacheWriteAdmissionFixture(t, tracker, "a", at, input)
			b := cacheWriteAdmissionFixture(t, tracker, "b", at.Add(time.Second), input)
			if reverse {
				a, b = b, a
			}
			if _, ok := cacheWriteAdmissionFinish(t, tracker, a, at.Add(2*time.Second), 10000, 8000, `[]`); ok {
				t.Fatal("overlap inferred")
			}
			if _, ok := cacheWriteAdmissionFinish(t, tracker, b, at.Add(3*time.Second), 11000, 9000, `[]`); ok {
				t.Fatal("overlap inferred")
			}
		})
	}
}
func TestOpenAICacheWriteAdmission_ResetRejectsOldReceipt(t *testing.T) {
	tracker := &openAICacheWriteInferenceTracker{}
	at := time.Now()
	request := cacheWriteAdmissionFixture(t, tracker, "old", at, `[{"role":"user","content":"prompt"}]`)
	tracker.resetForSettingEpoch(1)
	state, ok := tracker.finishAdmission(request)
	if !ok || !state.unsafe || state.request.Epoch == tracker.settingEpoch {
		t.Fatal("old epoch live receipt was forgotten or remained safe")
	}
}
func TestOpenAICacheWriteAdmission_APIKeyIsolation(t *testing.T) {
	tracker := &openAICacheWriteInferenceTracker{}
	at := time.Now()
	input := `[{"role":"user","content":"prompt"}]`
	a := cacheWriteAdmissionFixture(t, tracker, "a", at, input)
	cacheWriteAdmissionFinish(t, tracker, a, at.Add(time.Second), 10000, 8000, `[]`)
	b := tracker.beginAdmission(OpenAICacheWriteRequest{ID: "b", APIKeyID: 56, StartedAt: at.Add(2 * time.Second), scopeKey: openAICacheWriteTrackerKey(7, "gpt-6-astra", "session", 56), prompt: buildOpenAICacheWritePromptEvidence([]byte(`{"model":"gpt-6-astra","input":` + input + `}`))})
	if b.predecessorID != "" {
		t.Fatal("cross API-key predecessor")
	}
	if _, ok := cacheWriteAdmissionFinish(t, tracker, b, at.Add(3*time.Second), 11000, 9000, `[]`); ok {
		t.Fatal("cross API-key inferred")
	}
}

func TestOpenAICacheWriteAdmission_IdenticalPromptChangedTotalSkips(t *testing.T) {
	tracker := &openAICacheWriteInferenceTracker{}
	at := time.Now()
	input := `[{"role":"user","content":"prompt"}]`
	a := cacheWriteAdmissionFixture(t, tracker, "a", at, input)
	cacheWriteAdmissionFinish(t, tracker, a, at.Add(time.Second), 10000, 8000, `[]`)
	b := cacheWriteAdmissionFixture(t, tracker, "b", at.Add(2*time.Second), input)
	if _, ok := cacheWriteAdmissionFinish(t, tracker, b, at.Add(3*time.Second), 11000, 9000, `[]`); ok {
		t.Fatal("same prompt with changed total is ambiguous")
	}
}
func TestOpenAICacheWriteAdmission_LiveRequestSurvivesTTLAndEpoch(t *testing.T) {
	tracker := &openAICacheWriteInferenceTracker{}
	at := time.Now()
	input := `[{"role":"user","content":"prompt"}]`
	a := cacheWriteAdmissionFixture(t, tracker, "long", at, input)
	tracker.mu.Lock()
	tracker.pruneLocked(at.Add(defaultOpenAICacheWriteInferenceWindow + time.Minute))
	tracker.mu.Unlock()
	if len(tracker.active) != 1 || tracker.inFlight[a.scopeKey] != 1 {
		t.Fatal("TTL forgot a live request")
	}
	tracker.resetForSettingEpoch(2)
	tracker.resetForSettingEpoch(1)
	if tracker.settingEpoch != 2 || len(tracker.active) != 1 {
		t.Fatal("epoch rollback/clear forgot live request")
	}
	b := cacheWriteAdmissionFixture(t, tracker, "new", at.Add(defaultOpenAICacheWriteInferenceWindow+2*time.Minute), input)
	state, ok := tracker.finishAdmission(b)
	if !ok || !state.unsafe {
		t.Fatal("overlap with long old-epoch request remained safe")
	}
	if _, ok := tracker.finishAdmission(a); !ok {
		t.Fatal("old receipt could not release concurrency")
	}
}
func TestOpenAICacheWriteAdmission_OverflowBarrierReleasedExactlyOnce(t *testing.T) {
	tracker := &openAICacheWriteInferenceTracker{}
	at := time.Now()
	tracker.active = make(map[string]openAICacheWriteAdmissionState)
	for i := 0; i < maxOpenAICacheWriteInferenceEntries; i++ {
		tracker.active[fmt.Sprint(i)] = openAICacheWriteAdmissionState{}
	}
	overflow := tracker.beginAdmission(OpenAICacheWriteRequest{ID: "overflow", APIKeyID: 5, StartedAt: at, scopeKey: "scope"})
	if overflow == nil || !overflow.overflow || tracker.overflow != 1 || len(tracker.active) != maxOpenAICacheWriteInferenceEntries {
		t.Fatal("overflow lost bounded barrier")
	}
	tracker.resetForSettingEpoch(2)
	if tracker.overflow != 1 {
		t.Fatal("epoch cleared live overflow barrier")
	}
	if _, ok := tracker.finishAdmission(overflow); !ok {
		t.Fatal("overflow not released")
	}
	if _, ok := tracker.finishAdmission(overflow); ok || tracker.overflow != 0 {
		t.Fatal("overflow released twice")
	}
}
func TestOpenAICacheWriteAdmission_ReusedCompletionIDBreaksNewAttempt(t *testing.T) {
	tracker := &openAICacheWriteInferenceTracker{}
	at := time.Now()
	obs := openAICacheWriteObservation{AccountID: 7, APIKeyID: 5, Model: "m", CacheIdentity: "s", CompletionID: "upstream", ObservedAt: at, InputTokens: 1000}
	tracker.observe(obs, "local-a")
	obs.ObservedAt = at.Add(time.Second)
	tracker.observe(obs, "local-b")
	if len(tracker.entries) != 0 {
		t.Fatal("new attempt with replayed upstream completion preserved lineage")
	}
}

func TestOpenAICacheWriteAdmission_StaleEpochBeginStillTracksConcurrency(t *testing.T) {
	tracker := &openAICacheWriteInferenceTracker{}
	at := time.Now()
	tracker.resetForSettingEpoch(2)
	request := tracker.beginAdmission(OpenAICacheWriteRequest{ID: "stale-begin", APIKeyID: 5, Epoch: 1, StartedAt: at, scopeKey: "scope"})
	if request == nil || tracker.inFlight["scope"] != 1 {
		t.Fatal("stale admission became an untracked upstream request")
	}
	state, ok := tracker.finishAdmission(request)
	if !ok || !state.unsafe {
		t.Fatal("stale admission was safe or did not release")
	}
}
