package kirocooldown

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestClearEarliestTransientCooldownEmptyKeysIsSafe(t *testing.T) {
	store := NewStore(redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"}))

	cleared, err := store.ClearEarliestTransientCooldown(context.Background(), nil)
	if err != nil {
		t.Fatalf("ClearEarliestTransientCooldown(nil) error = %v", err)
	}
	if cleared {
		t.Fatal("ClearEarliestTransientCooldown(nil) cleared = true, want false")
	}
}

func TestClearEarliestTransientCooldownUnavailableStore(t *testing.T) {
	store := NewStore(nil)

	cleared, err := store.ClearEarliestTransientCooldown(context.Background(), []string{"token"})
	if err == nil {
		t.Fatal("ClearEarliestTransientCooldown unavailable store error = nil")
	}
	if cleared {
		t.Fatal("ClearEarliestTransientCooldown unavailable store cleared = true, want false")
	}
}
