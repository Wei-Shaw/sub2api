//go:build darwin || linux

package deployer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProcessLockIsExclusiveAndReusableAfterRelease(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	first, err := AcquireProcessLock(statePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })

	if _, err := AcquireProcessLock(statePath); !errors.Is(err, ErrProcessLocked) {
		t.Fatalf("second lock error=%v, want ErrProcessLocked", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireProcessLock(statePath)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	if _, err := os.Stat(filepath.Join(filepath.Dir(statePath), "deployer.lock")); err != nil {
		t.Fatalf("stat fixed process lock: %v", err)
	}
}
