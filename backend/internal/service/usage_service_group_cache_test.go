//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
)

type stubUsageRepoForGroupCache struct {
	UsageLogRepository
	rows  []GroupCacheHitRate7d
	err   error
	calls int
}

func (s *stubUsageRepoForGroupCache) GetGroupCacheHitRates7d(ctx context.Context) ([]GroupCacheHitRate7d, error) {
	s.calls++
	return s.rows, s.err
}

func TestUsageService_GetGroupCacheHitRates7d_NoCache(t *testing.T) {
	t.Parallel()
	repo := &stubUsageRepoForGroupCache{
		rows: []GroupCacheHitRate7d{
			{GroupID: 1, CacheRead: 500, Denom: 1000},
			{GroupID: 2, CacheRead: 0, Denom: 1},
			{GroupID: 3, CacheRead: 100, Denom: 0}, // defensively filtered
		},
	}
	svc := &UsageService{usageRepo: repo} // redisClient nil → cache path skipped

	got, err := svc.GetGroupCacheHitRates7d(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got[1] != 0.5 {
		t.Errorf("group 1: want 0.5, got %v", got[1])
	}
	if got[2] != 0 {
		t.Errorf("group 2: want 0, got %v", got[2])
	}
	if _, ok := got[3]; ok {
		t.Errorf("group 3 should be filtered (denom=0)")
	}
	if repo.calls != 1 {
		t.Errorf("want 1 repo call, got %d", repo.calls)
	}
}

func TestUsageService_GetGroupCacheHitRates7d_RepoError(t *testing.T) {
	t.Parallel()
	repo := &stubUsageRepoForGroupCache{err: errors.New("db down")}
	svc := &UsageService{usageRepo: repo}

	if _, err := svc.GetGroupCacheHitRates7d(context.Background()); err == nil {
		t.Fatalf("want error, got nil")
	}
}
