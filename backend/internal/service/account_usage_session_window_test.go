package service

import (
	"context"
	"sync"
	"testing"
	"time"
)

// sessionWindowSyncRepo 记录 syncActiveToPassive 触发的所有写操作。
type sessionWindowSyncRepo struct {
	AccountRepository

	mu                sync.Mutex
	extraUpdates      []map[string]any
	sessionWindowEnds []sessionWindowEndCall
}

type sessionWindowEndCall struct {
	AccountID int64
	End       time.Time
}

func (r *sessionWindowSyncRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := make(map[string]any, len(updates))
	for k, v := range updates {
		copied[k] = v
	}
	r.extraUpdates = append(r.extraUpdates, copied)
	return nil
}

func (r *sessionWindowSyncRepo) UpdateSessionWindowEnd(_ context.Context, id int64, end time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionWindowEnds = append(r.sessionWindowEnds, sessionWindowEndCall{AccountID: id, End: end})
	return nil
}

func TestEstimateSetupTokenUsage_ExpiredWindowZeroes(t *testing.T) {
	t.Parallel()

	past := time.Now().Add(-2 * time.Hour)
	svc := &AccountUsageService{}
	info := svc.estimateSetupTokenUsage(&Account{
		SessionWindowEnd: &past,
		Extra: map[string]any{
			"session_window_utilization": 0.53,
		},
	})

	if info.FiveHour == nil {
		t.Fatal("expected non-nil FiveHour info")
	}
	if info.FiveHour.Utilization != 0 {
		t.Fatalf("expected Utilization=0 for expired window, got %v", info.FiveHour.Utilization)
	}
	if info.FiveHour.ResetsAt != nil {
		t.Fatalf("expected ResetsAt=nil for expired window, got %v", info.FiveHour.ResetsAt)
	}
	if info.FiveHour.RemainingSeconds != 0 {
		t.Fatalf("expected RemainingSeconds=0 for expired window, got %v", info.FiveHour.RemainingSeconds)
	}
}

func TestEstimateSetupTokenUsage_ActiveWindowPreservesUtilization(t *testing.T) {
	t.Parallel()

	future := time.Now().Add(3 * time.Hour)
	svc := &AccountUsageService{}
	info := svc.estimateSetupTokenUsage(&Account{
		SessionWindowEnd: &future,
		Extra: map[string]any{
			"session_window_utilization": 0.53,
		},
	})

	if info.FiveHour == nil {
		t.Fatal("expected non-nil FiveHour info")
	}
	if info.FiveHour.Utilization != 53 {
		t.Fatalf("expected Utilization=53, got %v", info.FiveHour.Utilization)
	}
	if info.FiveHour.ResetsAt == nil || !info.FiveHour.ResetsAt.Equal(future) {
		t.Fatalf("expected ResetsAt=%v, got %v", future, info.FiveHour.ResetsAt)
	}
	if info.FiveHour.RemainingSeconds <= 0 {
		t.Fatalf("expected positive RemainingSeconds, got %v", info.FiveHour.RemainingSeconds)
	}
}

func TestSyncActiveToPassive_WritesFiveHourSessionWindowEnd(t *testing.T) {
	t.Parallel()

	repo := &sessionWindowSyncRepo{}
	svc := &AccountUsageService{accountRepo: repo}
	resetsAt := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	svc.syncActiveToPassive(context.Background(), 42, &UsageInfo{
		FiveHour: &UsageProgress{
			Utilization: 53,
			ResetsAt:    &resetsAt,
		},
	})

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.sessionWindowEnds) != 1 {
		t.Fatalf("expected 1 UpdateSessionWindowEnd call, got %d", len(repo.sessionWindowEnds))
	}
	call := repo.sessionWindowEnds[0]
	if call.AccountID != 42 {
		t.Fatalf("expected AccountID=42, got %d", call.AccountID)
	}
	if !call.End.Equal(resetsAt) {
		t.Fatalf("expected End=%v, got %v", resetsAt, call.End)
	}
}

func TestSyncActiveToPassive_SkipsSessionWindowEndWhenResetMissing(t *testing.T) {
	t.Parallel()

	repo := &sessionWindowSyncRepo{}
	svc := &AccountUsageService{accountRepo: repo}
	svc.syncActiveToPassive(context.Background(), 99, &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 10},
	})

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.sessionWindowEnds) != 0 {
		t.Fatalf("expected no UpdateSessionWindowEnd calls when ResetsAt is nil, got %d", len(repo.sessionWindowEnds))
	}
}
