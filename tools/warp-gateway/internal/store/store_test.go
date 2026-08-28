package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistenceFailureDoesNotCommitMemory(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		s := newFailureStore(t)
		inst := &Instance{ID: "two", Name: "two", ListenPort: 41002}
		if err := s.Create(inst); err == nil {
			t.Fatal("expected persistence error")
		}
		if _, err := s.Get("two"); err != ErrNotFound {
			t.Fatalf("failed create committed to memory: %v", err)
		}
		if _, used := s.ports[41002]; used {
			t.Fatal("failed create reserved port")
		}
	})

	t.Run("update", func(t *testing.T) {
		s := newFailureStore(t)
		s.byID["one"].Profile.Address = []string{"original"}
		if _, err := s.Update("one", func(i *Instance) {
			i.Name = "changed"
			i.Profile.Address[0] = "changed"
		}); err == nil {
			t.Fatal("expected persistence error")
		}
		got, _ := s.Get("one")
		if got.Name != "one" || got.Profile.Address[0] != "original" {
			t.Fatalf("failed update committed state: %#v", got)
		}
	})

	t.Run("delete", func(t *testing.T) {
		s := newFailureStore(t)
		if err := s.Delete("one"); err == nil {
			t.Fatal("expected persistence error")
		}
		if _, err := s.Get("one"); err != nil {
			t.Fatalf("failed delete removed instance: %v", err)
		}
		if owner := s.ports[41001]; owner != "one" {
			t.Fatalf("failed delete released port, owner=%q", owner)
		}
	})
}

func newFailureStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(dir, 41001, 41010)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Create(&Instance{ID: "one", Name: "one", ListenPort: 41001}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "instances.json.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	return s
}
