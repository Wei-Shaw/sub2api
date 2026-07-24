package backgroundruntime

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestStandbyDefersTasksUntilActivationAndPersistsSlot(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "active-slot")
	t.Setenv("DEPLOYMENT_STANDBY", "true")
	t.Setenv("DEPLOYMENT_SLOT", "sub2api-green")
	t.Setenv("DEPLOYMENT_STATE_FILE", statePath)
	if err := ConfigureFromEnv(); err != nil {
		t.Fatal(err)
	}
	var starts atomic.Int32
	if err := Register("test", func() error { starts.Add(1); return nil }); err != nil {
		t.Fatal(err)
	}
	if starts.Load() != 0 || Status().State != StateStandby {
		t.Fatalf("starts=%d state=%s", starts.Load(), Status().State)
	}
	if IsActive() {
		t.Fatal("standby runtime reported active")
	}
	if err := os.WriteFile(statePath, []byte("sub2api-green\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Activate(); err != nil {
		t.Fatal(err)
	}
	if starts.Load() != 1 || Status().State != StateActive {
		t.Fatalf("starts=%d state=%s", starts.Load(), Status().State)
	}
	if !IsActive() {
		t.Fatal("promoted runtime did not report active")
	}
	data, err := os.ReadFile(statePath)
	if err != nil || string(data) != "sub2api-green\n" {
		t.Fatalf("marker=%q err=%v", data, err)
	}
	if err := Activate(); err != nil || starts.Load() != 1 {
		t.Fatalf("second activation starts=%d err=%v", starts.Load(), err)
	}
}

func TestPersistedSlotStartsActiveAfterRestart(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "active-slot")
	if err := os.WriteFile(statePath, []byte("blue\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEPLOYMENT_STANDBY", "1")
	t.Setenv("DEPLOYMENT_SLOT", "blue")
	t.Setenv("DEPLOYMENT_STATE_FILE", statePath)
	if err := ConfigureFromEnv(); err != nil {
		t.Fatal(err)
	}
	var starts atomic.Int32
	if err := Register("test", func() error { starts.Add(1); return nil }); err != nil {
		t.Fatal(err)
	}
	if starts.Load() != 1 || Status().State != StateActive {
		t.Fatalf("starts=%d state=%s", starts.Load(), Status().State)
	}
}

func TestActivationRequiresHostMarker(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "active-slot")
	t.Setenv("DEPLOYMENT_STANDBY", "true")
	t.Setenv("DEPLOYMENT_SLOT", "green")
	t.Setenv("DEPLOYMENT_STATE_FILE", statePath)
	if err := ConfigureFromEnv(); err != nil {
		t.Fatal(err)
	}
	var starts atomic.Int32
	if err := Register("test", func() error { starts.Add(1); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := Activate(); err == nil {
		t.Fatal("activation succeeded without host marker")
	}
	if starts.Load() != 0 || Status().State != StateFailed {
		t.Fatalf("starts=%d state=%s", starts.Load(), Status().State)
	}
	if err := os.WriteFile(statePath, []byte("green\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Activate(); err != nil || starts.Load() != 1 {
		t.Fatalf("retry starts=%d err=%v", starts.Load(), err)
	}
}

func TestInvalidStandbyConfigurationFailsClosed(t *testing.T) {
	t.Setenv("DEPLOYMENT_STANDBY", "true")
	t.Setenv("DEPLOYMENT_SLOT", "")
	t.Setenv("DEPLOYMENT_STATE_FILE", "relative")
	if err := ConfigureFromEnv(); err == nil {
		t.Fatal("expected invalid standby configuration")
	}
}

func TestBeginShutdownPreventsDeferredActivation(t *testing.T) {
	t.Cleanup(func() { global.configure(StateActive, "", "") })
	statePath := filepath.Join(t.TempDir(), "active-slot")
	t.Setenv("DEPLOYMENT_STANDBY", "true")
	t.Setenv("DEPLOYMENT_SLOT", "green")
	t.Setenv("DEPLOYMENT_STATE_FILE", statePath)
	if err := ConfigureFromEnv(); err != nil {
		t.Fatal(err)
	}
	var starts atomic.Int32
	if err := Register("test", func() error { starts.Add(1); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("green\n"), 0600); err != nil {
		t.Fatal(err)
	}
	BeginShutdown()
	if err := Activate(); err == nil {
		t.Fatal("activation succeeded after shutdown began")
	}
	if starts.Load() != 0 || Status().State != StateStopping {
		t.Fatalf("starts=%d state=%s", starts.Load(), Status().State)
	}
}

func TestBeginShutdownSerializesWithActivation(t *testing.T) {
	t.Cleanup(func() { global.configure(StateActive, "", "") })
	statePath := filepath.Join(t.TempDir(), "active-slot")
	t.Setenv("DEPLOYMENT_STANDBY", "true")
	t.Setenv("DEPLOYMENT_SLOT", "green")
	t.Setenv("DEPLOYMENT_STATE_FILE", statePath)
	if err := ConfigureFromEnv(); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	if err := Register("blocking", func() error {
		close(started)
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("green\n"), 0600); err != nil {
		t.Fatal(err)
	}
	activateDone := make(chan error, 1)
	go func() { activateDone <- Activate() }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("activation task did not start")
	}
	shutdownDone := make(chan struct{})
	go func() {
		BeginShutdown()
		close(shutdownDone)
	}()
	select {
	case <-shutdownDone:
		t.Fatal("shutdown barrier raced an in-progress activation")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-activateDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("shutdown barrier did not finish after activation")
	}
	if Status().State != StateStopping {
		t.Fatalf("state=%s, want stopping", Status().State)
	}
}
