package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type opsAvailabilityAccountRepo struct {
	accounts []Account
}

func (m *opsAvailabilityAccountRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	filtered := make([]Account, 0, len(m.accounts))
	for _, acc := range m.accounts {
		if platform != "" && acc.Platform != platform {
			continue
		}
		filtered = append(filtered, acc)
	}
	return filtered, &pagination.PaginationResult{Total: int64(len(filtered))}, nil
}

func (m *opsAvailabilityAccountRepo) Create(ctx context.Context, account *Account) error {
	panic("unexpected Create call")
}

func (m *opsAvailabilityAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	panic("unexpected GetByID call")
}

func (m *opsAvailabilityAccountRepo) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	panic("unexpected GetByIDs call")
}

func (m *opsAvailabilityAccountRepo) ExistsByID(ctx context.Context, id int64) (bool, error) {
	panic("unexpected ExistsByID call")
}

func (m *opsAvailabilityAccountRepo) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*Account, error) {
	panic("unexpected GetByCRSAccountID call")
}

func (m *opsAvailabilityAccountRepo) FindByExtraField(ctx context.Context, key string, value any) ([]Account, error) {
	panic("unexpected FindByExtraField call")
}

func (m *opsAvailabilityAccountRepo) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	panic("unexpected ListCRSAccountIDs call")
}

func (m *opsAvailabilityAccountRepo) Update(ctx context.Context, account *Account) error {
	panic("unexpected Update call")
}

func (m *opsAvailabilityAccountRepo) Delete(ctx context.Context, id int64) error {
	panic("unexpected Delete call")
}

func (m *opsAvailabilityAccountRepo) List(ctx context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (m *opsAvailabilityAccountRepo) ListByGroup(ctx context.Context, groupID int64) ([]Account, error) {
	panic("unexpected ListByGroup call")
}

func (m *opsAvailabilityAccountRepo) ListActive(ctx context.Context) ([]Account, error) {
	panic("unexpected ListActive call")
}

func (m *opsAvailabilityAccountRepo) ListByPlatform(ctx context.Context, platform string) ([]Account, error) {
	panic("unexpected ListByPlatform call")
}

func (m *opsAvailabilityAccountRepo) UpdateLastUsed(ctx context.Context, id int64) error {
	panic("unexpected UpdateLastUsed call")
}

func (m *opsAvailabilityAccountRepo) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	panic("unexpected BatchUpdateLastUsed call")
}

func (m *opsAvailabilityAccountRepo) SetError(ctx context.Context, id int64, errorMsg string) error {
	panic("unexpected SetError call")
}

func (m *opsAvailabilityAccountRepo) ClearError(ctx context.Context, id int64) error {
	panic("unexpected ClearError call")
}

func (m *opsAvailabilityAccountRepo) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	panic("unexpected SetSchedulable call")
}

func (m *opsAvailabilityAccountRepo) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	panic("unexpected AutoPauseExpiredAccounts call")
}

func (m *opsAvailabilityAccountRepo) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	panic("unexpected BindGroups call")
}

func (m *opsAvailabilityAccountRepo) ListSchedulable(ctx context.Context) ([]Account, error) {
	panic("unexpected ListSchedulable call")
}

func (m *opsAvailabilityAccountRepo) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error) {
	panic("unexpected ListSchedulableByGroupID call")
}

func (m *opsAvailabilityAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	panic("unexpected ListSchedulableByPlatform call")
}

func (m *opsAvailabilityAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	panic("unexpected ListSchedulableByGroupIDAndPlatform call")
}

func (m *opsAvailabilityAccountRepo) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	panic("unexpected ListSchedulableByPlatforms call")
}

func (m *opsAvailabilityAccountRepo) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]Account, error) {
	panic("unexpected ListSchedulableByGroupIDAndPlatforms call")
}

func (m *opsAvailabilityAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	panic("unexpected ListSchedulableUngroupedByPlatform call")
}

func (m *opsAvailabilityAccountRepo) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	panic("unexpected ListSchedulableUngroupedByPlatforms call")
}

func (m *opsAvailabilityAccountRepo) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	panic("unexpected SetRateLimited call")
}

func (m *opsAvailabilityAccountRepo) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time) error {
	panic("unexpected SetModelRateLimit call")
}

func (m *opsAvailabilityAccountRepo) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	panic("unexpected SetOverloaded call")
}

func (m *opsAvailabilityAccountRepo) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	panic("unexpected SetTempUnschedulable call")
}

func (m *opsAvailabilityAccountRepo) ClearTempUnschedulable(ctx context.Context, id int64) error {
	panic("unexpected ClearTempUnschedulable call")
}

func (m *opsAvailabilityAccountRepo) ClearRateLimit(ctx context.Context, id int64) error {
	panic("unexpected ClearRateLimit call")
}

func (m *opsAvailabilityAccountRepo) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	panic("unexpected ClearAntigravityQuotaScopes call")
}

func (m *opsAvailabilityAccountRepo) ClearModelRateLimits(ctx context.Context, id int64) error {
	panic("unexpected ClearModelRateLimits call")
}

func (m *opsAvailabilityAccountRepo) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	panic("unexpected UpdateSessionWindow call")
}

func (m *opsAvailabilityAccountRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	panic("unexpected UpdateExtra call")
}

func (m *opsAvailabilityAccountRepo) BulkUpdate(ctx context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	panic("unexpected BulkUpdate call")
}

func (m *opsAvailabilityAccountRepo) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	panic("unexpected IncrementQuotaUsed call")
}

func (m *opsAvailabilityAccountRepo) ResetQuotaUsed(ctx context.Context, id int64) error {
	panic("unexpected ResetQuotaUsed call")
}

func TestGetAccountAvailabilityStats_ExhaustedOpenAIAccountIsNotRateLimited(t *testing.T) {
	t.Parallel()

	resetAt := time.Now().Add(30 * time.Minute)
	repo := &opsAvailabilityAccountRepo{
		accounts: []Account{{
			ID:               65,
			Name:             "exhausted-openai",
			Platform:         PlatformOpenAI,
			Type:             AccountTypeOAuth,
			Status:           StatusActive,
			Schedulable:      true,
			RateLimitResetAt: &resetAt,
			Extra: map[string]any{
				"codex_7d_used_percent":      100.0,
				"codex_primary_used_percent": 100.0,
			},
		}},
	}
	svc := &OpsService{
		cfg:         &config.Config{Ops: config.OpsConfig{Enabled: true}},
		accountRepo: repo,
	}

	platformStats, _, accountStats, _, err := svc.GetAccountAvailabilityStats(context.Background(), PlatformOpenAI, nil)
	if err != nil {
		t.Fatalf("GetAccountAvailabilityStats returned error: %v", err)
	}

	account := accountStats[65]
	if account == nil {
		t.Fatalf("expected account availability entry")
	}
	if account.IsRateLimited {
		t.Fatalf("expected exhausted openai account not to be treated as rate limited")
	}
	if !account.IsAvailable {
		t.Fatalf("expected exhausted openai account to remain available for health/availability views")
	}
	if account.RateLimitResetAt != nil {
		t.Fatalf("expected exhausted openai account to omit rate limit reset in availability payload")
	}

	platform := platformStats[PlatformOpenAI]
	if platform == nil {
		t.Fatalf("expected platform availability entry")
	}
	if platform.RateLimitCount != 0 {
		t.Fatalf("expected platform rate limit count to ignore exhausted accounts, got %d", platform.RateLimitCount)
	}
	if platform.AvailableCount != 1 {
		t.Fatalf("expected platform available count to include exhausted account, got %d", platform.AvailableCount)
	}
}
