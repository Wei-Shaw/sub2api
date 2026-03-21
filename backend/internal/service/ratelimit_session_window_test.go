package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// sessionWindowMockRepo is a minimal AccountRepository mock that records calls
// made by UpdateSessionWindow. Unrelated methods panic if invoked.
type sessionWindowMockRepo struct {
	// captured calls
	sessionWindowCalls []swCall
	updateExtraCalls   []ueCall
	clearRateLimitIDs  []int64
}

var _ AccountRepository = (*sessionWindowMockRepo)(nil)

type swCall struct {
	ID     int64
	Start  *time.Time
	End    *time.Time
	Status string
}

type ueCall struct {
	ID      int64
	Updates map[string]any
}

func (m *sessionWindowMockRepo) UpdateSessionWindow(_ context.Context, id int64, start, end *time.Time, status string) error {
	m.sessionWindowCalls = append(m.sessionWindowCalls, swCall{ID: id, Start: start, End: end, Status: status})
	return nil
}
func (m *sessionWindowMockRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	m.updateExtraCalls = append(m.updateExtraCalls, ueCall{ID: id, Updates: updates})
	return nil
}
func (m *sessionWindowMockRepo) ClearRateLimit(_ context.Context, id int64) error {
	m.clearRateLimitIDs = append(m.clearRateLimitIDs, id)
	return nil
}
func (m *sessionWindowMockRepo) ClearAntigravityQuotaScopes(_ context.Context, _ int64) error {
	return nil
}
func (m *sessionWindowMockRepo) ClearModelRateLimits(_ context.Context, _ int64) error {
	return nil
}
func (m *sessionWindowMockRepo) ClearTempUnschedulable(_ context.Context, _ int64) error {
	return nil
}

// --- Unused interface methods (panic on unexpected call) ---

func (m *sessionWindowMockRepo) Create(context.Context, *Account) error { panic("unexpected") }
func (m *sessionWindowMockRepo) GetByID(context.Context, int64) (*Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) GetByIDs(context.Context, []int64) ([]*Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ExistsByID(context.Context, int64) (bool, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) GetByCRSAccountID(context.Context, string) (*Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) FindByExtraField(context.Context, string, any) ([]Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ListCRSAccountIDs(context.Context) (map[string]int64, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) Update(context.Context, *Account) error { panic("unexpected") }
func (m *sessionWindowMockRepo) Delete(context.Context, int64) error    { panic("unexpected") }
func (m *sessionWindowMockRepo) List(context.Context, pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, string, int64) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ListByGroup(context.Context, int64) ([]Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ListActive(context.Context) ([]Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ListByPlatform(context.Context, string) ([]Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) UpdateLastUsed(context.Context, int64) error { panic("unexpected") }
func (m *sessionWindowMockRepo) BatchUpdateLastUsed(context.Context, map[int64]time.Time) error {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) SetError(context.Context, int64, string) error {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ClearError(context.Context, int64) error { panic("unexpected") }
func (m *sessionWindowMockRepo) SetSchedulable(context.Context, int64, bool) error {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) AutoPauseExpiredAccounts(context.Context, time.Time) (int64, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) BindGroups(context.Context, int64, []int64) error {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ListSchedulable(context.Context) ([]Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ListSchedulableByPlatform(context.Context, string) ([]Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ListSchedulableByPlatforms(context.Context, []string) ([]Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ListSchedulableByGroupIDAndPlatforms(context.Context, int64, []string) ([]Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ListSchedulableUngroupedByPlatform(context.Context, string) ([]Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ListSchedulableUngroupedByPlatforms(context.Context, []string) ([]Account, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) SetRateLimited(context.Context, int64, time.Time) error {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) SetModelRateLimit(context.Context, int64, string, time.Time) error {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) SetOverloaded(context.Context, int64, time.Time) error {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) BulkUpdate(context.Context, []int64, AccountBulkUpdate) (int64, error) {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) IncrementQuotaUsed(context.Context, int64, float64) error {
	panic("unexpected")
}
func (m *sessionWindowMockRepo) ResetQuotaUsed(context.Context, int64) error { panic("unexpected") }

// newRateLimitServiceForTest creates a RateLimitService with the given mock repo.
func newRateLimitServiceForTest(repo AccountRepository) *RateLimitService {
	return &RateLimitService{accountRepo: repo}
}

func TestUpdateSessionWindow_UsesResetHeader(t *testing.T) {
	// The reset header provides the real window end as a Unix timestamp.
	// UpdateSessionWindow should use it instead of the hour-truncated prediction.
	resetUnix := time.Now().Add(3 * time.Hour).Unix()
	wantEnd := time.Unix(resetUnix, 0)
	wantStart := wantEnd.Add(-5 * time.Hour)

	repo := &sessionWindowMockRepo{}
	svc := newRateLimitServiceForTest(repo)

	account := &Account{ID: 42} // no existing window → needInitWindow=true
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-status", "allowed")
	headers.Set("anthropic-ratelimit-unified-5h-reset", fmt.Sprintf("%d", resetUnix))

	svc.UpdateSessionWindow(context.Background(), account, headers)

	if len(repo.sessionWindowCalls) != 1 {
		t.Fatalf("expected 1 UpdateSessionWindow call, got %d", len(repo.sessionWindowCalls))
	}

	call := repo.sessionWindowCalls[0]
	if call.ID != 42 {
		t.Errorf("expected account ID 42, got %d", call.ID)
	}
	if call.End == nil || !call.End.Equal(wantEnd) {
		t.Errorf("expected window end %v, got %v", wantEnd, call.End)
	}
	if call.Start == nil || !call.Start.Equal(wantStart) {
		t.Errorf("expected window start %v, got %v", wantStart, call.Start)
	}
	if call.Status != "allowed" {
		t.Errorf("expected status 'allowed', got %q", call.Status)
	}
}

func TestUpdateSessionWindow_FallbackPredictionWhenNoResetHeader(t *testing.T) {
	// When the reset header is absent, should fall back to hour-truncated prediction.
	repo := &sessionWindowMockRepo{}
	svc := newRateLimitServiceForTest(repo)

	account := &Account{ID: 10} // no existing window
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-status", "allowed_warning")
	// No anthropic-ratelimit-unified-5h-reset header

	// Capture now before the call to avoid hour-boundary races
	now := time.Now()
	expectedStart := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
	expectedEnd := expectedStart.Add(5 * time.Hour)

	svc.UpdateSessionWindow(context.Background(), account, headers)

	if len(repo.sessionWindowCalls) != 1 {
		t.Fatalf("expected 1 UpdateSessionWindow call, got %d", len(repo.sessionWindowCalls))
	}

	call := repo.sessionWindowCalls[0]
	if call.End == nil {
		t.Fatal("expected window end to be set (fallback prediction)")
	}
	// Fallback: start = current hour truncated, end = start + 5h

	if !call.End.Equal(expectedEnd) {
		t.Errorf("expected fallback end %v, got %v", expectedEnd, *call.End)
	}
	if call.Start == nil || !call.Start.Equal(expectedStart) {
		t.Errorf("expected fallback start %v, got %v", expectedStart, call.Start)
	}
}

func TestUpdateSessionWindow_CorrectsStalePrediction(t *testing.T) {
	// When the stored SessionWindowEnd is wrong (from a previous prediction),
	// and the reset header provides the real time, it should update the window.
	staleEnd := time.Now().Add(2 * time.Hour)             // existing prediction: 2h from now
	realResetUnix := time.Now().Add(4 * time.Hour).Unix() // real reset: 4h from now
	wantEnd := time.Unix(realResetUnix, 0)

	repo := &sessionWindowMockRepo{}
	svc := newRateLimitServiceForTest(repo)

	account := &Account{
		ID:               55,
		SessionWindowEnd: &staleEnd,
	}
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-status", "allowed")
	headers.Set("anthropic-ratelimit-unified-5h-reset", fmt.Sprintf("%d", realResetUnix))

	svc.UpdateSessionWindow(context.Background(), account, headers)

	if len(repo.sessionWindowCalls) != 1 {
		t.Fatalf("expected 1 UpdateSessionWindow call, got %d", len(repo.sessionWindowCalls))
	}

	call := repo.sessionWindowCalls[0]
	if call.End == nil || !call.End.Equal(wantEnd) {
		t.Errorf("expected corrected end %v, got %v", wantEnd, call.End)
	}
}

func TestUpdateSessionWindow_NoUpdateWhenHeaderMatchesStored(t *testing.T) {
	// If the reset header matches the stored SessionWindowEnd, no window update needed.
	futureUnix := time.Now().Add(3 * time.Hour).Unix()
	existingEnd := time.Unix(futureUnix, 0)

	repo := &sessionWindowMockRepo{}
	svc := newRateLimitServiceForTest(repo)

	account := &Account{
		ID:               77,
		SessionWindowEnd: &existingEnd,
	}
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-status", "allowed")
	headers.Set("anthropic-ratelimit-unified-5h-reset", fmt.Sprintf("%d", futureUnix)) // same as stored

	svc.UpdateSessionWindow(context.Background(), account, headers)

	if len(repo.sessionWindowCalls) != 1 {
		t.Fatalf("expected 1 UpdateSessionWindow call, got %d", len(repo.sessionWindowCalls))
	}

	call := repo.sessionWindowCalls[0]
	// windowStart and windowEnd should be nil (no update needed)
	if call.Start != nil || call.End != nil {
		t.Errorf("expected nil start/end (no window change needed), got start=%v end=%v", call.Start, call.End)
	}
	// Status is still updated
	if call.Status != "allowed" {
		t.Errorf("expected status 'allowed', got %q", call.Status)
	}
}

func TestUpdateSessionWindow_ClearsUtilizationOnWindowReset(t *testing.T) {
	// When needInitWindow=true and window is set, utilization should be cleared.
	resetUnix := time.Now().Add(3 * time.Hour).Unix()

	repo := &sessionWindowMockRepo{}
	svc := newRateLimitServiceForTest(repo)

	account := &Account{ID: 33} // no existing window → needInitWindow=true
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-status", "allowed")
	headers.Set("anthropic-ratelimit-unified-5h-reset", fmt.Sprintf("%d", resetUnix))
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "0.15")

	svc.UpdateSessionWindow(context.Background(), account, headers)

	// Should have 2 UpdateExtra calls: one to clear utilization, one to store new utilization
	if len(repo.updateExtraCalls) != 2 {
		t.Fatalf("expected 2 UpdateExtra calls, got %d", len(repo.updateExtraCalls))
	}

	// First call: clear utilization (nil value)
	clearCall := repo.updateExtraCalls[0]
	if clearCall.Updates["session_window_utilization"] != nil {
		t.Errorf("expected utilization cleared to nil, got %v", clearCall.Updates["session_window_utilization"])
	}

	// Second call: store new utilization
	storeCall := repo.updateExtraCalls[1]
	if val, ok := storeCall.Updates["session_window_utilization"].(float64); !ok || val != 0.15 {
		t.Errorf("expected utilization stored as 0.15, got %v", storeCall.Updates["session_window_utilization"])
	}
}

func TestUpdateSessionWindow_NoClearUtilizationOnCorrection(t *testing.T) {
	// When correcting a stale prediction (needInitWindow=false), utilization should NOT be cleared.
	staleEnd := time.Now().Add(2 * time.Hour)
	realResetUnix := time.Now().Add(4 * time.Hour).Unix()

	repo := &sessionWindowMockRepo{}
	svc := newRateLimitServiceForTest(repo)

	account := &Account{
		ID:               66,
		SessionWindowEnd: &staleEnd,
	}
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-status", "allowed")
	headers.Set("anthropic-ratelimit-unified-5h-reset", fmt.Sprintf("%d", realResetUnix))
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "0.30")

	svc.UpdateSessionWindow(context.Background(), account, headers)

	// Only 1 UpdateExtra call (store utilization), no clear call
	if len(repo.updateExtraCalls) != 1 {
		t.Fatalf("expected 1 UpdateExtra call (no clear), got %d", len(repo.updateExtraCalls))
	}

	if val, ok := repo.updateExtraCalls[0].Updates["session_window_utilization"].(float64); !ok || val != 0.30 {
		t.Errorf("expected utilization 0.30, got %v", repo.updateExtraCalls[0].Updates["session_window_utilization"])
	}
}

func TestUpdateSessionWindow_NoStatusHeader(t *testing.T) {
	// Should return immediately if no status header.
	repo := &sessionWindowMockRepo{}
	svc := newRateLimitServiceForTest(repo)

	account := &Account{ID: 1}

	svc.UpdateSessionWindow(context.Background(), account, http.Header{})

	if len(repo.sessionWindowCalls) != 0 {
		t.Errorf("expected no calls when status header absent, got %d", len(repo.sessionWindowCalls))
	}
}
