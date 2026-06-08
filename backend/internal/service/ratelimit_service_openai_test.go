//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCalculateOpenAI429ResetTime_7dExhausted(t *testing.T) {
	svc := &RateLimitService{}

	// Simulate headers when 7d limit is exhausted (100% used)
	// Primary = 7d (10080 minutes), Secondary = 5h (300 minutes)
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "384607") // ~4.5 days
	headers.Set("x-codex-primary-window-minutes", "10080")       // 7 days
	headers.Set("x-codex-secondary-used-percent", "3")
	headers.Set("x-codex-secondary-reset-after-seconds", "17369") // ~4.8 hours
	headers.Set("x-codex-secondary-window-minutes", "300")        // 5 hours

	before := time.Now()
	resetAt := svc.calculateOpenAI429ResetTime(headers)
	after := time.Now()

	if resetAt == nil {
		t.Fatal("expected non-nil resetAt")
	}

	// Should be approximately 384607 seconds from now
	expectedDuration := 384607 * time.Second
	minExpected := before.Add(expectedDuration)
	maxExpected := after.Add(expectedDuration)

	if resetAt.Before(minExpected) || resetAt.After(maxExpected) {
		t.Errorf("resetAt %v not in expected range [%v, %v]", resetAt, minExpected, maxExpected)
	}
}

func TestCalculateOpenAI429ResetTime_5hExhausted(t *testing.T) {
	svc := &RateLimitService{}

	// Simulate headers when 5h limit is exhausted (100% used)
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "50")
	headers.Set("x-codex-primary-reset-after-seconds", "500000")
	headers.Set("x-codex-primary-window-minutes", "10080") // 7 days
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "3600") // 1 hour
	headers.Set("x-codex-secondary-window-minutes", "300")       // 5 hours

	before := time.Now()
	resetAt := svc.calculateOpenAI429ResetTime(headers)
	after := time.Now()

	if resetAt == nil {
		t.Fatal("expected non-nil resetAt")
	}

	// Should be approximately 3600 seconds from now
	expectedDuration := 3600 * time.Second
	minExpected := before.Add(expectedDuration)
	maxExpected := after.Add(expectedDuration)

	if resetAt.Before(minExpected) || resetAt.After(maxExpected) {
		t.Errorf("resetAt %v not in expected range [%v, %v]", resetAt, minExpected, maxExpected)
	}
}

func TestCalculateOpenAI429ResetTime_NeitherExhausted_UsesMax(t *testing.T) {
	svc := &RateLimitService{}

	// Neither limit at 100%, should use the longer reset time
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "80")
	headers.Set("x-codex-primary-reset-after-seconds", "100000")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "90")
	headers.Set("x-codex-secondary-reset-after-seconds", "5000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	before := time.Now()
	resetAt := svc.calculateOpenAI429ResetTime(headers)
	after := time.Now()

	if resetAt == nil {
		t.Fatal("expected non-nil resetAt")
	}

	// Should use the max (100000 seconds from 7d window)
	expectedDuration := 100000 * time.Second
	minExpected := before.Add(expectedDuration)
	maxExpected := after.Add(expectedDuration)

	if resetAt.Before(minExpected) || resetAt.After(maxExpected) {
		t.Errorf("resetAt %v not in expected range [%v, %v]", resetAt, minExpected, maxExpected)
	}
}

func TestCalculateOpenAI429ResetTime_NoCodexHeaders(t *testing.T) {
	svc := &RateLimitService{}

	// No codex headers at all
	headers := http.Header{}
	headers.Set("content-type", "application/json")

	resetAt := svc.calculateOpenAI429ResetTime(headers)

	if resetAt != nil {
		t.Errorf("expected nil resetAt when no codex headers, got %v", resetAt)
	}
}

func TestCalculateOpenAI429ResetTime_ReversedWindowOrder(t *testing.T) {
	svc := &RateLimitService{}

	// Test when OpenAI sends primary as 5h and secondary as 7d (reversed)
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")         // This is 5h
	headers.Set("x-codex-primary-reset-after-seconds", "3600") // 1 hour
	headers.Set("x-codex-primary-window-minutes", "300")       // 5 hours - smaller!
	headers.Set("x-codex-secondary-used-percent", "50")
	headers.Set("x-codex-secondary-reset-after-seconds", "500000")
	headers.Set("x-codex-secondary-window-minutes", "10080") // 7 days - larger!

	before := time.Now()
	resetAt := svc.calculateOpenAI429ResetTime(headers)
	after := time.Now()

	if resetAt == nil {
		t.Fatal("expected non-nil resetAt")
	}

	// Should correctly identify that primary is 5h (smaller window) and use its reset time
	expectedDuration := 3600 * time.Second
	minExpected := before.Add(expectedDuration)
	maxExpected := after.Add(expectedDuration)

	if resetAt.Before(minExpected) || resetAt.After(maxExpected) {
		t.Errorf("resetAt %v not in expected range [%v, %v]", resetAt, minExpected, maxExpected)
	}
}

type openAI429SnapshotRepo struct {
	mockAccountRepoForGemini
	rateLimitedID      int64
	updatedExtra       map[string]any
	bulkUpdatedIDs     []int64
	bulkUpdatedPayload AccountBulkUpdate
}

func (r *openAI429SnapshotRepo) SetRateLimited(_ context.Context, id int64, _ time.Time) error {
	r.rateLimitedID = id
	return nil
}

func (r *openAI429SnapshotRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updatedExtra = updates
	return nil
}

func (r *openAI429SnapshotRepo) BulkUpdate(_ context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	r.bulkUpdatedIDs = append([]int64(nil), ids...)
	r.bulkUpdatedPayload = updates
	return int64(len(ids)), nil
}

func TestHandle429_OpenAIPersistsCodexSnapshotImmediately(t *testing.T) {
	repo := &openAI429SnapshotRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{ID: 123, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "604800")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "18000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	svc.handle429(context.Background(), account, headers, nil)

	if repo.rateLimitedID != account.ID {
		t.Fatalf("rateLimitedID = %d, want %d", repo.rateLimitedID, account.ID)
	}
	if len(repo.updatedExtra) == 0 {
		t.Fatal("expected codex snapshot to be persisted on 429")
	}
	if got := repo.updatedExtra["codex_5h_used_percent"]; got != 100.0 {
		t.Fatalf("codex_5h_used_percent = %v, want 100", got)
	}
	if got := repo.updatedExtra["codex_7d_used_percent"]; got != 100.0 {
		t.Fatalf("codex_7d_used_percent = %v, want 100", got)
	}
}

func TestHandle429_OpenAISyncsObservedPlanType(t *testing.T) {
	repo := &openAI429SnapshotRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{
		ID:          124,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"plan_type": "plus"},
	}
	body := []byte(`{"error":{"type":"usage_limit_reached","message":"limit reached","plan_type":"free","resets_at":1777283883}}`)

	svc.handle429(context.Background(), account, http.Header{}, body)

	require.Equal(t, []int64{account.ID}, repo.bulkUpdatedIDs)
	require.Equal(t, "free", repo.bulkUpdatedPayload.Credentials["plan_type"])
	require.Equal(t, "free", account.Credentials["plan_type"])
	require.Equal(t, account.ID, repo.rateLimitedID)
}

func TestNormalizedCodexLimits(t *testing.T) {
	// Test the Normalize() method directly
	pUsed := 100.0
	pReset := 384607
	pWindow := 10080
	sUsed := 3.0
	sReset := 17369
	sWindow := 300

	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:         &pUsed,
		PrimaryResetAfterSeconds:   &pReset,
		PrimaryWindowMinutes:       &pWindow,
		SecondaryUsedPercent:       &sUsed,
		SecondaryResetAfterSeconds: &sReset,
		SecondaryWindowMinutes:     &sWindow,
	}

	normalized := snapshot.Normalize()
	if normalized == nil {
		t.Fatal("expected non-nil normalized")
	}

	// Primary has larger window (10080 > 300), so primary should be 7d
	if normalized.Used7dPercent == nil || *normalized.Used7dPercent != 100.0 {
		t.Errorf("expected Used7dPercent=100, got %v", normalized.Used7dPercent)
	}
	if normalized.Reset7dSeconds == nil || *normalized.Reset7dSeconds != 384607 {
		t.Errorf("expected Reset7dSeconds=384607, got %v", normalized.Reset7dSeconds)
	}
	if normalized.Used5hPercent == nil || *normalized.Used5hPercent != 3.0 {
		t.Errorf("expected Used5hPercent=3, got %v", normalized.Used5hPercent)
	}
	if normalized.Reset5hSeconds == nil || *normalized.Reset5hSeconds != 17369 {
		t.Errorf("expected Reset5hSeconds=17369, got %v", normalized.Reset5hSeconds)
	}
}

func TestNormalizedCodexLimits_OnlyPrimaryData(t *testing.T) {
	// Test when only primary has data, no window_minutes
	pUsed := 80.0
	pReset := 50000

	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:       &pUsed,
		PrimaryResetAfterSeconds: &pReset,
		// No window_minutes, no secondary data
	}

	normalized := snapshot.Normalize()
	if normalized == nil {
		t.Fatal("expected non-nil normalized")
	}

	// Legacy assumption: primary=7d, secondary=5h
	if normalized.Used7dPercent == nil || *normalized.Used7dPercent != 80.0 {
		t.Errorf("expected Used7dPercent=80, got %v", normalized.Used7dPercent)
	}
	if normalized.Reset7dSeconds == nil || *normalized.Reset7dSeconds != 50000 {
		t.Errorf("expected Reset7dSeconds=50000, got %v", normalized.Reset7dSeconds)
	}
	// Secondary (5h) should be nil
	if normalized.Used5hPercent != nil {
		t.Errorf("expected Used5hPercent=nil, got %v", *normalized.Used5hPercent)
	}
	if normalized.Reset5hSeconds != nil {
		t.Errorf("expected Reset5hSeconds=nil, got %v", *normalized.Reset5hSeconds)
	}
}

func TestRateLimitService_HandleUpstreamError_403PreservesOriginalUpstreamMessage(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       201,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		403,
		http.Header{},
		[]byte(`{"error":{"message":"workspace forbidden by policy","type":"invalid_request_error"}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls)
	require.Contains(t, repo.lastErrorMsg, "workspace forbidden by policy")
	require.NotContains(t, repo.lastErrorMsg, "account may be suspended or lack permissions")
}

func TestRateLimitService_HandleUpstreamError_403FallsBackToRawBody(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       202,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		403,
		http.Header{},
		[]byte(`{"error":{"type":"access_denied","details":{"reason":"ip_blocked"}}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls)
	require.Contains(t, repo.lastErrorMsg, `"access_denied"`)
	require.Contains(t, repo.lastErrorMsg, `"ip_blocked"`)
	require.NotContains(t, repo.lastErrorMsg, "account may be suspended or lack permissions")
}

func TestNormalizedCodexLimits_OnlySecondaryData(t *testing.T) {
	// Test when only secondary has data, no window_minutes
	sUsed := 60.0
	sReset := 3000

	snapshot := &OpenAICodexUsageSnapshot{
		SecondaryUsedPercent:       &sUsed,
		SecondaryResetAfterSeconds: &sReset,
		// No window_minutes, no primary data
	}

	normalized := snapshot.Normalize()
	if normalized == nil {
		t.Fatal("expected non-nil normalized")
	}

	// Legacy assumption: primary=7d, secondary=5h
	// So secondary goes to 5h
	if normalized.Used5hPercent == nil || *normalized.Used5hPercent != 60.0 {
		t.Errorf("expected Used5hPercent=60, got %v", normalized.Used5hPercent)
	}
	if normalized.Reset5hSeconds == nil || *normalized.Reset5hSeconds != 3000 {
		t.Errorf("expected Reset5hSeconds=3000, got %v", normalized.Reset5hSeconds)
	}
	// Primary (7d) should be nil
	if normalized.Used7dPercent != nil {
		t.Errorf("expected Used7dPercent=nil, got %v", *normalized.Used7dPercent)
	}
}

func TestNormalizedCodexLimits_BothDataNoWindowMinutes(t *testing.T) {
	// Test when both have data but no window_minutes
	pUsed := 100.0
	pReset := 400000
	sUsed := 50.0
	sReset := 10000

	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:         &pUsed,
		PrimaryResetAfterSeconds:   &pReset,
		SecondaryUsedPercent:       &sUsed,
		SecondaryResetAfterSeconds: &sReset,
		// No window_minutes
	}

	normalized := snapshot.Normalize()
	if normalized == nil {
		t.Fatal("expected non-nil normalized")
	}

	// Legacy assumption: primary=7d, secondary=5h
	if normalized.Used7dPercent == nil || *normalized.Used7dPercent != 100.0 {
		t.Errorf("expected Used7dPercent=100, got %v", normalized.Used7dPercent)
	}
	if normalized.Reset7dSeconds == nil || *normalized.Reset7dSeconds != 400000 {
		t.Errorf("expected Reset7dSeconds=400000, got %v", normalized.Reset7dSeconds)
	}
	if normalized.Used5hPercent == nil || *normalized.Used5hPercent != 50.0 {
		t.Errorf("expected Used5hPercent=50, got %v", normalized.Used5hPercent)
	}
	if normalized.Reset5hSeconds == nil || *normalized.Reset5hSeconds != 10000 {
		t.Errorf("expected Reset5hSeconds=10000, got %v", normalized.Reset5hSeconds)
	}
}

func TestHandle429_AnthropicPlatformUnaffected(t *testing.T) {
	// Verify that Anthropic platform accounts still use the original logic
	// This test ensures we don't break existing Claude account rate limiting

	svc := &RateLimitService{}

	// Simulate Anthropic 429 headers
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-reset", "1737820800") // A future Unix timestamp

	// For Anthropic platform, calculateOpenAI429ResetTime should return nil
	// because it only handles OpenAI platform
	resetAt := svc.calculateOpenAI429ResetTime(headers)

	// Should return nil since there are no x-codex-* headers
	if resetAt != nil {
		t.Errorf("expected nil for Anthropic headers, got %v", resetAt)
	}
}

func TestCalculateOpenAI429ResetTime_UserProvidedScenario(t *testing.T) {
	// This is the exact scenario from the user:
	// codex_7d_used_percent: 100
	// codex_7d_reset_after_seconds: 384607 (约4.5天后重置)
	// codex_5h_used_percent: 3
	// codex_5h_reset_after_seconds: 17369 (约4.8小时后重置)

	svc := &RateLimitService{}

	// Simulate headers matching user's data
	// Note: We need to map the canonical 5h/7d back to primary/secondary
	// Based on typical OpenAI behavior: primary=7d (larger window), secondary=5h (smaller window)
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "384607")
	headers.Set("x-codex-primary-window-minutes", "10080") // 7 days = 10080 minutes
	headers.Set("x-codex-secondary-used-percent", "3")
	headers.Set("x-codex-secondary-reset-after-seconds", "17369")
	headers.Set("x-codex-secondary-window-minutes", "300") // 5 hours = 300 minutes

	before := time.Now()
	resetAt := svc.calculateOpenAI429ResetTime(headers)
	after := time.Now()

	if resetAt == nil {
		t.Fatal("expected non-nil resetAt for user scenario")
	}

	// Should use the 7d reset time (384607 seconds) since 7d limit is exhausted (100%)
	expectedDuration := 384607 * time.Second
	minExpected := before.Add(expectedDuration)
	maxExpected := after.Add(expectedDuration)

	if resetAt.Before(minExpected) || resetAt.After(maxExpected) {
		t.Errorf("resetAt %v not in expected range [%v, %v]", resetAt, minExpected, maxExpected)
	}

	// Verify it's approximately 4.45 days (384607 seconds)
	duration := resetAt.Sub(before)
	actualDays := duration.Hours() / 24.0

	// 384607 / 86400 = ~4.45 days
	if actualDays < 4.4 || actualDays > 4.5 {
		t.Errorf("expected ~4.45 days, got %.2f days", actualDays)
	}

	t.Logf("User scenario: reset_at=%v, duration=%.2f days", resetAt, actualDays)
}

func TestCalculateOpenAI429ResetTime_5MinFallbackWhenNoReset(t *testing.T) {
	// Test that we return nil when there's used_percent but no reset_after_seconds
	// This should cause the caller to use the default 5-minute fallback

	svc := &RateLimitService{}

	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "100")
	// No reset_after_seconds!

	resetAt := svc.calculateOpenAI429ResetTime(headers)

	// Should return nil since there's no reset time available
	if resetAt != nil {
		t.Errorf("expected nil when no reset_after_seconds, got %v", resetAt)
	}
}
