package service

import (
	"encoding/json"
	"testing"
)

func TestContentModerationServiceUsesConfiguredWorkersAndStops(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.WorkerCount = 2
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	service := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyContentModerationConfig: string(raw),
		}},
		&contentModerationTestRepo{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	service.workerMu.Lock()
	workerCount := len(service.workerCancels)
	service.workerMu.Unlock()
	if workerCount != 2 {
		t.Fatalf("expected 2 configured workers, got %d", workerCount)
	}

	service.Stop()
	service.Stop()
	service.workerMu.Lock()
	workerCount = len(service.workerCancels)
	service.workerMu.Unlock()
	if workerCount != 0 {
		t.Fatalf("expected all workers to stop, got %d", workerCount)
	}
	if !service.stopped.Load() {
		t.Fatal("service did not record stopped lifecycle state")
	}
}

func TestContentModerationServicePrunesRemovedAPIKeyHealth(t *testing.T) {
	service := &ContentModerationService{keyHealth: make(map[string]*contentModerationKeyHealth)}
	service.beginModerationAPIKeyCall("keep")
	service.beginModerationAPIKeyCall("remove")
	service.pruneAPIKeyHealth([]string{"keep"})

	service.keyHealthMu.Lock()
	defer service.keyHealthMu.Unlock()
	if len(service.keyHealth) != 1 || service.keyHealth[moderationAPIKeyHash("keep")] == nil {
		t.Fatalf("unexpected retained key health: %#v", service.keyHealth)
	}
}
