//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type channelMonitorSingleflightRepo struct {
	ChannelMonitorRepository
	mu            sync.RWMutex
	monitor       ChannelMonitor
	getReads      atomic.Int32
	historyWrites atomic.Int32
	markWrites    atomic.Int32
}

func (r *channelMonitorSingleflightRepo) GetByID(context.Context, int64) (*ChannelMonitor, error) {
	r.getReads.Add(1)
	r.mu.RLock()
	defer r.mu.RUnlock()
	monitor := r.monitor
	monitor.ExtraModels = append([]string(nil), r.monitor.ExtraModels...)
	return &monitor, nil
}

func (r *channelMonitorSingleflightRepo) updateMonitor(update func(*ChannelMonitor)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	update(&r.monitor)
}

func (r *channelMonitorSingleflightRepo) InsertHistoryBatch(context.Context, []*ChannelMonitorHistoryRow) error {
	r.historyWrites.Add(1)
	return nil
}

func (r *channelMonitorSingleflightRepo) MarkChecked(context.Context, int64, time.Time) error {
	r.markWrites.Add(1)
	return nil
}

type channelMonitorTestEncryptor struct{}

func (channelMonitorTestEncryptor) Encrypt(plaintext string) (string, error) {
	return "ENC:" + plaintext, nil
}

func (channelMonitorTestEncryptor) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "ENC:"), nil
}

type blockingChannelMonitorUpstream struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
	calls       atomic.Int32
}

func (h *blockingChannelMonitorUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusOK)
		return
	}
	h.calls.Add(1)
	h.startedOnce.Do(func() { close(h.started) })
	<-h.release

	defer func() { _ = r.Body.Close() }()
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"content": answerFromOpenAIRequest(body)}}},
	})
}

func (h *blockingChannelMonitorUpstream) releaseNow() {
	h.releaseOnce.Do(func() { close(h.release) })
}

type recordingChannelMonitorUpstream struct {
	calls atomic.Int32
	mu    sync.Mutex
	model string
	auth  string
}

func (h *recordingChannelMonitorUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusOK)
		return
	}
	h.calls.Add(1)
	defer func() { _ = r.Body.Close() }()
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	h.mu.Lock()
	h.model, _ = body["model"].(string)
	h.auth = r.Header.Get("Authorization")
	h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"content": answerFromOpenAIRequest(body)}}},
	})
}

func (h *recordingChannelMonitorUpstream) request() (model, auth string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.model, h.auth
}

func TestChannelMonitorServiceRunCheckCoalescesManualAndScheduledCalls(t *testing.T) {
	swapMonitorHTTPClient(t)
	upstream := &blockingChannelMonitorUpstream{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	server := httptest.NewServer(upstream)
	t.Cleanup(func() {
		upstream.releaseNow()
		server.Close()
	})

	const monitorID = int64(42)
	repo := &channelMonitorSingleflightRepo{monitor: ChannelMonitor{
		ID:              monitorID,
		Provider:        MonitorProviderGrok,
		Endpoint:        server.URL,
		APIKey:          "ENC:test-key",
		PrimaryModel:    MonitorDefaultGrokModel,
		IntervalSeconds: 60,
	}}
	service := NewChannelMonitorService(repo, channelMonitorTestEncryptor{})

	firstResult := make(chan []*CheckResult, 1)
	firstErr := make(chan error, 1)
	go func() {
		results, err := service.RunCheck(context.Background(), monitorID)
		firstResult <- results
		firstErr <- err
	}()
	<-upstream.started

	go func() {
		time.Sleep(50 * time.Millisecond)
		upstream.releaseNow()
	}()
	second, err := service.RunCheck(context.Background(), monitorID)
	if err != nil {
		t.Fatalf("second RunCheck failed: %v", err)
	}
	if err := <-firstErr; err != nil {
		t.Fatalf("first RunCheck failed: %v", err)
	}
	first := <-firstResult

	for name, results := range map[string][]*CheckResult{"first": first, "second": second} {
		if len(results) != 1 || results[0].Status != MonitorStatusOperational {
			t.Fatalf("%s caller received unexpected results: %+v", name, results)
		}
	}
	if got := upstream.calls.Load(); got != 1 {
		t.Fatalf("concurrent checks should share one upstream request, got %d", got)
	}
	if got := repo.historyWrites.Load(); got != 1 {
		t.Fatalf("shared check should write history once, got %d", got)
	}
	if got := repo.markWrites.Load(); got != 1 {
		t.Fatalf("shared check should mark checked once, got %d", got)
	}
	if got := repo.getReads.Load(); got != 2 {
		t.Fatalf("each RunCheck should load its snapshot once, got %d reads", got)
	}
}

func TestChannelMonitorCheckFlightKeyTracksRequestConfig(t *testing.T) {
	base := ChannelMonitor{
		ID:               51,
		Name:             "display name",
		Provider:         MonitorProviderGrok,
		APIMode:          MonitorAPIModeChatCompletions,
		Endpoint:         "https://old.example.test",
		APIKey:           "old-key",
		PrimaryModel:     "grok-4.5",
		ExtraModels:      []string{"grok-4.3"},
		IntervalSeconds:  60,
		ExtraHeaders:     map[string]string{"X-Test": "old"},
		BodyOverrideMode: MonitorBodyOverrideModeMerge,
		BodyOverride:     map[string]any{"temperature": 0.2},
	}
	baseKey, err := channelMonitorCheckFlightKey(&base)
	if err != nil {
		t.Fatalf("base key: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ChannelMonitor)
	}{
		{name: "endpoint", mutate: func(m *ChannelMonitor) { m.Endpoint = "https://new.example.test" }},
		{name: "api key", mutate: func(m *ChannelMonitor) { m.APIKey = "new-key" }},
		{name: "primary model", mutate: func(m *ChannelMonitor) { m.PrimaryModel = "grok-4.3" }},
		{name: "extra models", mutate: func(m *ChannelMonitor) { m.ExtraModels = []string{"grok-4.20-0309-reasoning"} }},
		{name: "headers", mutate: func(m *ChannelMonitor) { m.ExtraHeaders = map[string]string{"X-Test": "new"} }},
		{name: "body override", mutate: func(m *ChannelMonitor) { m.BodyOverride = map[string]any{"temperature": 0.8} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated := base
			tt.mutate(&updated)
			updatedKey, keyErr := channelMonitorCheckFlightKey(&updated)
			if keyErr != nil {
				t.Fatalf("updated key: %v", keyErr)
			}
			if updatedKey == baseKey {
				t.Fatalf("%s change must start a new check flight", tt.name)
			}
		})
	}

	metadataOnly := base
	metadataOnly.Name = "renamed"
	metadataOnly.IntervalSeconds = 120
	metadataKey, err := channelMonitorCheckFlightKey(&metadataOnly)
	if err != nil {
		t.Fatalf("metadata key: %v", err)
	}
	if metadataKey != baseKey {
		t.Fatal("scheduler/display-only changes should keep the same check flight")
	}
}

func TestChannelMonitorServiceRunCheckDoesNotJoinStaleConfigFlight(t *testing.T) {
	swapMonitorHTTPClient(t)
	oldUpstream := &blockingChannelMonitorUpstream{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	oldServer := httptest.NewServer(oldUpstream)
	newUpstream := &recordingChannelMonitorUpstream{}
	newServer := httptest.NewServer(newUpstream)
	t.Cleanup(func() {
		oldUpstream.releaseNow()
		oldServer.Close()
		newServer.Close()
	})

	const monitorID = int64(52)
	repo := &channelMonitorSingleflightRepo{monitor: ChannelMonitor{
		ID:              monitorID,
		Provider:        MonitorProviderGrok,
		APIMode:         MonitorAPIModeChatCompletions,
		Endpoint:        oldServer.URL,
		APIKey:          "ENC:old-key",
		PrimaryModel:    "grok-4.5",
		IntervalSeconds: 60,
	}}
	service := NewChannelMonitorService(repo, channelMonitorTestEncryptor{})

	oldResult := make(chan []*CheckResult, 1)
	oldErr := make(chan error, 1)
	go func() {
		results, err := service.RunCheck(context.Background(), monitorID)
		oldResult <- results
		oldErr <- err
	}()
	<-oldUpstream.started

	repo.updateMonitor(func(m *ChannelMonitor) {
		m.Endpoint = newServer.URL
		m.APIKey = "ENC:new-key"
		m.PrimaryModel = "grok-4.3"
	})

	newCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	newResult, err := service.RunCheck(newCtx, monitorID)
	if err != nil {
		t.Fatalf("updated check joined the stale flight: %v", err)
	}
	if len(newResult) != 1 || newResult[0].Model != "grok-4.3" || newResult[0].Status != MonitorStatusOperational {
		t.Fatalf("updated check used stale config: %+v", newResult)
	}
	model, auth := newUpstream.request()
	if model != "grok-4.3" || auth != "Bearer new-key" {
		t.Fatalf("updated upstream request model=%q auth=%q", model, auth)
	}
	if got := repo.historyWrites.Load(); got != 1 {
		t.Fatalf("updated flight should persist independently before old flight completes, got %d writes", got)
	}

	oldUpstream.releaseNow()
	if err := <-oldErr; err != nil {
		t.Fatalf("old check failed: %v", err)
	}
	first := <-oldResult
	if len(first) != 1 || first[0].Model != "grok-4.5" || first[0].Status != MonitorStatusOperational {
		t.Fatalf("old flight snapshot changed underneath it: %+v", first)
	}
	if got := oldUpstream.calls.Load(); got != 1 {
		t.Fatalf("old flight upstream calls = %d, want 1", got)
	}
	if got := newUpstream.calls.Load(); got != 1 {
		t.Fatalf("new flight upstream calls = %d, want 1", got)
	}
	if got := repo.historyWrites.Load(); got != 2 {
		t.Fatalf("distinct config flights should each write history, got %d", got)
	}
	if got := repo.markWrites.Load(); got != 2 {
		t.Fatalf("distinct config flights should each mark checked, got %d", got)
	}
	if got := repo.getReads.Load(); got != 2 {
		t.Fatalf("each RunCheck should read one config snapshot, got %d reads", got)
	}
}
