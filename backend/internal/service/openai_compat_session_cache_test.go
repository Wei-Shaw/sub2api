package service

import (
	"strconv"
	"testing"
	"time"
)

func TestOpenAICompatSessionCacheBoundsAndExpiresEntries(t *testing.T) {
	cache := &boundedOpenAICompatSessionCache{}
	expiresAt := time.Now().Add(time.Hour)
	for i := 0; i < openAICompatSessionCacheMaxEntries+1; i++ {
		cache.Store(strconv.Itoa(i), openAICompatSessionResponseBinding{ExpiresAt: expiresAt})
	}
	if got := cache.Len(); got > openAICompatSessionCacheMaxEntries {
		t.Fatalf("expected at most %d entries, got %d", openAICompatSessionCacheMaxEntries, got)
	}

	cache.Store("expired", openAICompatSessionResponseBinding{ExpiresAt: time.Now().Add(-time.Second)})
	if _, ok := cache.Load("expired"); ok {
		t.Fatal("expired compatibility session was retained")
	}
}

func TestOpenAICompatSessionCacheDeletePrefix(t *testing.T) {
	cache := &boundedOpenAICompatSessionCache{}
	expiresAt := time.Now().Add(time.Hour)
	cache.Store("42\x001", openAICompatSessionResponseBinding{ExpiresAt: expiresAt})
	cache.Store("42\x002", openAICompatSessionResponseBinding{ExpiresAt: expiresAt})
	cache.Store("43\x001", openAICompatSessionResponseBinding{ExpiresAt: expiresAt})

	cache.DeletePrefix("42\x00")
	if got := cache.Len(); got != 1 {
		t.Fatalf("expected one unrelated entry, got %d", got)
	}
}
