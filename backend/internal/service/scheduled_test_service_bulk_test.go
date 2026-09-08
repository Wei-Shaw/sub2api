package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type scheduledTestPlanRepoBulkStub struct {
	defaults ScheduledTestBulkDefaults
	created  int64
	err      error
}

func (r *scheduledTestPlanRepoBulkStub) Create(context.Context, *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	panic("unexpected Create call")
}

func (r *scheduledTestPlanRepoBulkStub) CreateMissingForAllAccounts(
	_ context.Context,
	defaults ScheduledTestBulkDefaults,
) (int64, error) {
	r.defaults = defaults
	return r.created, r.err
}

func (r *scheduledTestPlanRepoBulkStub) GetByID(context.Context, int64) (*ScheduledTestPlan, error) {
	panic("unexpected GetByID call")
}

func (r *scheduledTestPlanRepoBulkStub) ListByAccountID(context.Context, int64) ([]*ScheduledTestPlan, error) {
	panic("unexpected ListByAccountID call")
}

func (r *scheduledTestPlanRepoBulkStub) ListDue(context.Context, time.Time) ([]*ScheduledTestPlan, error) {
	panic("unexpected ListDue call")
}

func (r *scheduledTestPlanRepoBulkStub) Update(context.Context, *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	panic("unexpected Update call")
}

func (r *scheduledTestPlanRepoBulkStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (r *scheduledTestPlanRepoBulkStub) UpdateAfterRun(context.Context, int64, time.Time, time.Time) error {
	panic("unexpected UpdateAfterRun call")
}

func TestScheduledTestServiceCreateMissingPlansForAllAccountsUsesLowCostDefaults(t *testing.T) {
	repo := &scheduledTestPlanRepoBulkStub{created: 7}
	svc := NewScheduledTestService(repo, nil)

	created, err := svc.CreateMissingPlansForAllAccounts(context.Background())

	require.NoError(t, err)
	require.Equal(t, int64(7), created)
	require.Equal(t, "*/30 * * * *", repo.defaults.CronExpression)
	require.Equal(t, 50, repo.defaults.MaxResults)
	require.True(t, repo.defaults.AutoRecover)
	require.Equal(t, 30, repo.defaults.SpreadMinutes)
	require.Equal(t, map[string]string{
		PlatformAnthropic:   "claude-haiku-4-5-20251001",
		PlatformOpenAI:      "gpt-5.4-mini",
		PlatformGemini:      "gemini-2.0-flash",
		PlatformAntigravity: "gemini-2.5-flash-lite",
		PlatformGrok:        "grok-composer-2.5-fast",
	}, repo.defaults.ModelByPlatform)
}
