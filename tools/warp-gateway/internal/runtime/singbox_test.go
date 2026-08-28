package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/store"
)

func TestSingBoxStartFailsAndCleansUpWhenProcessExitsEarly(t *testing.T) {
	m, inst := fakeSingBoxManager(t, "#!/bin/sh\nexit 7\n")
	_, err := m.Start(context.Background(), inst)
	if err == nil || !strings.Contains(err.Error(), "exited early") {
		t.Fatalf("error=%v, want early exit", err)
	}
	assertInstanceDirRemoved(t, m, inst.ID)
}

func TestSingBoxStartFailsAndCleansUpOnReadinessTimeout(t *testing.T) {
	m, inst := fakeSingBoxManager(t, "#!/bin/sh\nsleep 30\n")
	m.readinessTimeout = 150 * time.Millisecond
	started := time.Now()
	_, err := m.Start(context.Background(), inst)
	if err == nil || !strings.Contains(err.Error(), "readiness timeout") {
		t.Fatalf("error=%v, want readiness timeout", err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("cleanup took too long: %s", elapsed)
	}
	assertInstanceDirRemoved(t, m, inst.ID)
}

func fakeSingBoxManager(t *testing.T, script string) (*SingBoxManager, *store.Instance) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "sing-box")
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	m := NewSingBoxManager(bin, dir)
	inst := &store.Instance{
		ID: "test-instance", ListenHost: "127.0.0.1", ListenPort: 1,
		Profile: store.Profile{PrivateKey: "private", Peers: []store.PeerConfig{{PublicKey: "public", Endpoint: "127.0.0.1:2408"}}},
	}
	return m, inst
}

func assertInstanceDirRemoved(t *testing.T, m *SingBoxManager, id string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(m.dataDir, "instances", id))
	if !os.IsNotExist(err) {
		t.Fatalf("instance runtime directory still exists: %v", err)
	}
}
