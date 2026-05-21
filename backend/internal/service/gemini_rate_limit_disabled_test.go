//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type geminiRateLimitRepoStub struct {
	mockAccountRepoForGemini
	setRateLimitedCalls int
	lastRateLimitedID   int64
	lastResetAt         time.Time
}

func (s *geminiRateLimitRepoStub) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	s.setRateLimitedCalls++
	s.lastRateLimitedID = id
	s.lastResetAt = resetAt
	return nil
}

func TestRateLimitServicePreCheckUsage_GeminiAlwaysAllowed(t *testing.T) {
	svc := NewRateLimitService(&geminiRateLimitRepoStub{}, nil, nil, nil, nil)

	ok, err := svc.PreCheckUsage(context.Background(), &Account{
		ID:       1,
		Platform: PlatformGemini,
	}, "gemini-2.5-pro")
	if err != nil {
		t.Fatalf("PreCheckUsage returned error: %v", err)
	}
	if !ok {
		t.Fatal("PreCheckUsage should always allow Gemini when platform-side rate limiting is disabled")
	}
}

func TestRateLimitServicePreCheckUsageBatch_AllGeminiAlwaysAllowed(t *testing.T) {
	svc := NewRateLimitService(&geminiRateLimitRepoStub{}, nil, nil, nil, nil)

	result, err := svc.PreCheckUsageBatch(context.Background(), []*Account{
		{ID: 1, Platform: PlatformGemini},
		{ID: 2, Platform: PlatformGemini},
	}, "gemini-2.5-pro")
	if err != nil {
		t.Fatalf("PreCheckUsageBatch returned error: %v", err)
	}
	if !result[1] || !result[2] {
		t.Fatalf("PreCheckUsageBatch should allow all Gemini accounts, got %#v", result)
	}
}

func TestRateLimitServiceHandle429_GeminiDoesNotPersistLocalRateLimit(t *testing.T) {
	repo := &geminiRateLimitRepoStub{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{ID: 7, Platform: PlatformGemini, Type: AccountTypeOAuth}

	headers := http.Header{}
	headers.Set("retry-after", "120")

	svc.handle429(context.Background(), account, headers, []byte(`{"error":"rate limited"}`))

	if repo.setRateLimitedCalls != 0 {
		t.Fatalf("SetRateLimited called %d times, want 0", repo.setRateLimitedCalls)
	}
}
