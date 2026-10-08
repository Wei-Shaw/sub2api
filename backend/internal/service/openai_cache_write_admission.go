package service

import (
	"context"
	"strings"
	"sync/atomic"
	"time"
)

// OpenAICacheWriteRequest is a process-local admission receipt. It contains only
// hashes and identifiers, never the prompt or response body. The receipt is not
// a claim that the upstream admitted a cache write.
type OpenAICacheWriteRequest struct {
	ID            string
	Epoch         uint64
	APIKeyID      int64
	StartedAt     time.Time
	scopeKey      string
	predecessorID string
	prompt        openAICacheWritePromptEvidence
	overflow      bool
	completed     *atomic.Bool
}

type openAICacheWriteAdmissionState struct {
	request OpenAICacheWriteRequest
	unsafe  bool
}

func (t *openAICacheWriteInferenceTracker) beginAdmission(request OpenAICacheWriteRequest) *OpenAICacheWriteRequest {
	if t == nil || request.ID == "" || request.scopeKey == "" || request.APIKeyID <= 0 {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(request.StartedAt)
	request.completed = &atomic.Bool{}
	if len(t.active) >= maxOpenAICacheWriteInferenceEntries {
		request.overflow = true
		request.prompt = openAICacheWritePromptEvidence{}
		t.overflow++
		t.entries = nil
		for id, state := range t.active {
			state.unsafe = true
			t.active[id] = state
		}
		return &request
	}
	if t.active == nil {
		t.active = make(map[string]openAICacheWriteAdmissionState)
	}
	if t.inFlight == nil {
		t.inFlight = make(map[string]int)
	}
	unsafe := request.Epoch != t.settingEpoch || !request.prompt.Valid || t.inFlight[request.scopeKey] != 0 || t.overflow != 0
	if unsafe {
		delete(t.entries, request.scopeKey)
		for id, state := range t.active {
			if state.request.scopeKey == request.scopeKey {
				state.unsafe = true
				t.active[id] = state
			}
		}
	} else if previous, ok := t.entries[request.scopeKey]; ok {
		request.predecessorID = previous.observationID
	}
	t.inFlight[request.scopeKey]++
	t.active[request.ID] = openAICacheWriteAdmissionState{request: request, unsafe: unsafe}
	return &request
}

func (t *openAICacheWriteInferenceTracker) finishAdmission(request *OpenAICacheWriteRequest) (openAICacheWriteAdmissionState, bool) {
	if t == nil || request == nil {
		return openAICacheWriteAdmissionState{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if request.completed != nil && !request.completed.CompareAndSwap(false, true) {
		return openAICacheWriteAdmissionState{}, false
	}
	if request.overflow {
		if t.overflow > 0 {
			t.overflow--
		}
		t.entries = nil
		for id, state := range t.active {
			state.unsafe = true
			t.active[id] = state
		}
		return openAICacheWriteAdmissionState{request: *request, unsafe: true}, true
	}
	state, ok := t.active[request.ID]
	if !ok || state.request.Epoch != request.Epoch || state.request.scopeKey != request.scopeKey {
		return openAICacheWriteAdmissionState{}, false
	}
	delete(t.active, request.ID)
	if t.inFlight[request.scopeKey] > 1 {
		t.inFlight[request.scopeKey]--
	} else {
		delete(t.inFlight, request.scopeKey)
	}
	return state, true
}

// BeginOpenAICacheWriteObservation must run before the upstream request starts.
// Unsupported request shapes still receive an unsafe admission so they break
// lineage and invalidate any overlapping otherwise-supported request.
func (s *OpenAIGatewayService) BeginOpenAICacheWriteObservation(ctx context.Context, account *Account, apiKeyID int64, model, cacheIdentity string, body []byte) *OpenAICacheWriteRequest {
	if s == nil || account == nil || !account.IsOpenAIOAuthLike() || apiKeyID <= 0 {
		return nil
	}
	enabled := s.settingService != nil && s.settingService.IsOpenAICacheWriteInferenceEnabled(ctx)
	epoch := openAICacheWriteInferenceSettingEpoch.Load()
	if !enabled {
		s.openaiCacheWriteInferenceTracker.clearAtSettingEpoch(epoch)
	}
	s.openaiCacheWriteInferenceTracker.resetForSettingEpoch(epoch)
	model = strings.TrimSpace(model)
	cacheIdentity = strings.TrimSpace(cacheIdentity)
	if model == "" || cacheIdentity == "" {
		return nil
	}
	prompt := openAICacheWritePromptEvidence{}
	if enabled {
		prompt = buildOpenAICacheWritePromptEvidence(body)
	}
	return s.openaiCacheWriteInferenceTracker.beginAdmission(OpenAICacheWriteRequest{
		ID: "cacheobs:" + generateRequestID(), Epoch: epoch, APIKeyID: apiKeyID, StartedAt: time.Now(),
		scopeKey: openAICacheWriteTrackerKey(account.ID, "", cacheIdentity, apiKeyID),
		prompt:   prompt,
	})
}

// CancelOpenAICacheWriteObservation is safe to defer immediately after admission.
// It is a no-op after successful observation and invalidates failed/interrupted
// attempts instead of silently bridging across a missing turn.
func (s *OpenAIGatewayService) CancelOpenAICacheWriteObservation(request *OpenAICacheWriteRequest) {
	if s == nil || request == nil {
		return
	}
	t := &s.openaiCacheWriteInferenceTracker
	if _, ok := t.finishAdmission(request); !ok {
		return
	}
	t.invalidateScope(request.scopeKey)
}

func (t *openAICacheWriteInferenceTracker) invalidateScope(scopeKey string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, scopeKey)
	for id, state := range t.active {
		if state.request.scopeKey == scopeKey {
			state.unsafe = true
			t.active[id] = state
		}
	}
}
