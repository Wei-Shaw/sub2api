//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func TestAntigravityQuotaCredentialsIsolateCacheAndInflight(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var startedOnce, releaseOnce sync.Once
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1internal:retrieveUserQuotaSummary":
			calls.Add(1)
			remaining := 0.8
			if r.Header.Get("Authorization") == "Bearer old-token" {
				startedOnce.Do(func() { close(started) })
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				remaining = 0.25
			}
			_, _ = fmt.Fprintf(w, `{"groups":[{"displayName":"Gemini","buckets":[{"bucketId":"gemini-5h","window":"5h","remainingFraction":%g}]}]}`, remaining)
		case "/v1internal:fetchAvailableModels":
			_, _ = io.WriteString(w, `{"models":{}}`)
		case "/v1internal:loadCodeAssist":
			_, _ = io.WriteString(w, `{}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer func() {
		releaseOnce.Do(func() { close(release) })
		server.Close()
	}()
	oldURLs, oldAvailability := antigravity.BaseURLs, antigravity.DefaultURLAvailability
	antigravity.BaseURLs = []string{server.URL}
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)
	t.Cleanup(func() {
		antigravity.BaseURLs, antigravity.DefaultURLAvailability = oldURLs, oldAvailability
	})
	svc := &AccountUsageService{cache: NewUsageCache(), antigravityQuotaFetcher: NewAntigravityQuotaFetcher(nil, nil)}
	account := func(token string) *Account {
		return &Account{ID: 42, Platform: PlatformAntigravity, Credentials: map[string]any{
			"access_token": token, "project_id": "test-project",
		}}
	}
	type outcome struct {
		usage *UsageInfo
		err   error
	}
	oldResult := make(chan outcome, 1)
	go func() {
		u, err := svc.getAntigravityUsage(context.Background(), account("old-token"))
		oldResult <- outcome{u, err}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("old request did not start")
	}
	newResult := make(chan outcome, 1)
	go func() {
		u, err := svc.getAntigravityUsage(context.Background(), account("new-token"))
		newResult <- outcome{u, err}
	}()
	select {
	case result := <-newResult:
		require.NoError(t, result.err)
		require.InDelta(t, 20, *result.usage.AntigravityQuotaGroups[0].Buckets[0].Utilization, 1e-8)
	case <-time.After(3 * time.Second):
		t.Fatal("new credentials incorrectly joined the old in-flight query")
	}
	releaseOnce.Do(func() { close(release) })
	old := <-oldResult
	require.NoError(t, old.err)
	require.InDelta(t, 75, *old.usage.AntigravityQuotaGroups[0].Buckets[0].Utilization, 1e-8)
	// Even if the older request completes last, its snapshot cannot be reused
	// for the replacement credentials on the same database account row.
	current, err := svc.getAntigravityUsage(context.Background(), account("new-token"))
	require.NoError(t, err)
	require.InDelta(t, 20, *current.AntigravityQuotaGroups[0].Buckets[0].Utilization, 1e-8)
	before := calls.Load()
	_, err = svc.getAntigravityUsage(context.Background(), account("new-token"))
	require.NoError(t, err)
	require.Equal(t, before, calls.Load(), "unchanged credentials must still use the cache")
}

func TestAntigravityQuotaTierChangeRejectsOldSnapshot(t *testing.T) {
	previous := &UsageInfo{SubscriptionTierRaw: "g1-ultra-lite-tier", AntigravityQuotaGroups: []AntigravityQuotaGroup{{Name: "old pool"}}}
	current := &UsageInfo{SubscriptionTierRaw: "g1-ultra-tier", AntigravityQuotaError: "quota_summary_unavailable"}
	retainAntigravityQuotaSnapshot(previous, current)
	require.Equal(t, "g1-ultra-tier", current.SubscriptionTierRaw)
	require.Empty(t, current.AntigravityQuotaGroups)
	require.False(t, current.AntigravityQuotaStale)
}
