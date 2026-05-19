package kiro

import (
	"testing"
	"time"
)

func TestSessionStore_IdCRoundtrip(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()

	sess := &IdCSession{State: "s1", ExpiresAt: time.Now().Add(time.Minute)}
	store.SetIdC("sid", sess)
	got, ok := store.GetIdC("sid")
	if !ok || got != sess {
		t.Fatalf("get returned %v, %v", got, ok)
	}
	store.DeleteIdC("sid")
	if _, ok := store.GetIdC("sid"); ok {
		t.Fatal("session should be deleted")
	}
}

func TestSessionStore_BuilderIDInterval(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()

	store.SetBuilderID("b1", &BuilderIDSession{Interval: 5, ExpiresAt: time.Now().Add(time.Minute)})
	store.UpdateBuilderIDInterval("b1", 5)
	sess, _ := store.GetBuilderID("b1")
	if sess.Interval != 10 {
		t.Fatalf("interval=%d want 10", sess.Interval)
	}
}

func TestSessionStore_PurgeExpired(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()

	now := time.Now()
	store.SetIdC("fresh", &IdCSession{ExpiresAt: now.Add(time.Minute)})
	store.SetIdC("old", &IdCSession{ExpiresAt: now.Add(-time.Second)})
	store.SetBuilderID("fresh-b", &BuilderIDSession{ExpiresAt: now.Add(time.Minute)})
	store.SetBuilderID("old-b", &BuilderIDSession{ExpiresAt: now.Add(-time.Second)})

	store.purgeExpired(now)

	if _, ok := store.GetIdC("old"); ok {
		t.Fatal("expired idc not purged")
	}
	if _, ok := store.GetIdC("fresh"); !ok {
		t.Fatal("fresh idc purged")
	}
	if _, ok := store.GetBuilderID("old-b"); ok {
		t.Fatal("expired builderid not purged")
	}
	if _, ok := store.GetBuilderID("fresh-b"); !ok {
		t.Fatal("fresh builderid purged")
	}
}
