//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type resetExecutionFixture struct {
	subscriptionQuotaFixture
	observer service.SubscriptionResetObserverRepository
	executor service.SubscriptionResetExecutionRepository
	policy   *service.SubscriptionResetPolicy
}

func newResetExecutionFixture(t *testing.T) resetExecutionFixture {
	t.Helper()
	f := newSubscriptionQuotaFixture(t)
	client := testEntClient(t)
	ids := []int64{}
	for i := 0; i < 2; i++ {
		a := mustCreateAccount(t, client, &service.Account{Name: "reset-execution-" + uuid.NewString(), Platform: service.PlatformOpenAI})
		mustBindAccountToGroup(t, client, a.ID, f.sub.GroupID, 50)
		ids = append(ids, a.ID)
	}
	t.Cleanup(func() {
		for _, id := range ids {
			_, err := integrationDB.Exec(`DELETE FROM accounts WHERE id=$1`, id)
			require.NoError(t, err)
		}
	})
	observer := ProvideSubscriptionResetObserverRepository(integrationDB, f.repo)
	p := service.DefaultSubscriptionResetPolicy(f.sub.GroupID)
	p.Mode = "auto"
	p.AccountIDs = ids
	p.ResetDimensions = []string{"daily", "monthly"}
	saved, err := observer.SavePolicy(context.Background(), p)
	require.NoError(t, err)
	return resetExecutionFixture{f, observer, NewSubscriptionResetExecutionRepository(integrationDB, f.repo), saved}
}

func (f resetExecutionFixture) event(t *testing.T, confirmedAt time.Time) string {
	t.Helper()
	id := uuid.NewString()
	e := &service.SubscriptionResetEvent{ID: id, GroupID: f.sub.GroupID, PolicyVersion: f.policy.Version, Source: "7d", Kind: "natural_reset", Status: "confirmed", OpenedAt: confirmedAt, ConfirmedAt: &confirmedAt, DeadlineAt: confirmedAt.Add(10 * time.Minute), UpdatedAt: confirmedAt, ResetDimensions: f.policy.ResetDimensions, Denominator: 2, RequiredCount: 2, ConfirmedCount: 2}
	payload, err := json.Marshal(e)
	require.NoError(t, err)
	_, err = integrationDB.Exec(`INSERT INTO subscription_reset_events(id,group_id,policy_version,opened_at,updated_at,payload) VALUES($1,$2,$3,$4,$4,$5::jsonb)`, id, f.sub.GroupID, f.policy.Version, confirmedAt, string(payload))
	require.NoError(t, err)
	return id
}

func TestSubscriptionResetExecutionAtomicAndIdempotent(t *testing.T) {
	f := newResetExecutionFixture(t)
	ctx := context.Background()
	before, err := f.repo.GetSubscriptionQuotaState(ctx, f.sub.ID)
	require.NoError(t, err)
	require.Equal(t, []float64{10, 20, 30}, []float64{before.DailyUsageUSD, before.WeeklyUsageUSD, before.MonthlyUsageUSD}, "saving auto mode only seeds accounting")
	id := f.event(t, time.Now().UTC())
	ids, err := f.executor.ListReadyResetEvents(ctx, 20)
	require.NoError(t, err)
	require.Contains(t, ids, id)
	var wg sync.WaitGroup
	results := make(chan *service.RotateGroupQuotaResult, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r, err := f.executor.ExecuteResetEvent(ctx, id); results <- r; errs <- err }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	applied := 0
	for result := range results {
		if result != nil {
			applied++
			require.EqualValues(t, 1, result.AffectedSubscriptions)
		}
	}
	require.Equal(t, 1, applied)
	after, err := f.repo.GetSubscriptionQuotaState(ctx, f.sub.ID)
	require.NoError(t, err)
	require.Equal(t, []float64{0, 20, 0}, []float64{after.DailyUsageUSD, after.WeeklyUsageUSD, after.MonthlyUsageUSD})
	require.Equal(t, before.WeeklyBucketID, after.WeeklyBucketID)
	require.True(t, before.ExpiresAt.Equal(after.ExpiresAt))
	var executions, outbox int
	require.NoError(t, integrationDB.QueryRow(`SELECT (SELECT COUNT(*) FROM subscription_reset_executions WHERE event_id=$1),(SELECT COUNT(*) FROM subscription_reset_outbox WHERE event_id=$1)`, id).Scan(&executions, &outbox))
	require.Equal(t, 1, executions)
	require.Equal(t, 1, outbox)
	status, err := f.observer.GetStatus(ctx, f.sub.GroupID, 20)
	require.NoError(t, err)
	require.Equal(t, "applied", status.Events[0].Status)
}

func TestSubscriptionResetExecutionRollsBackGrantWithPublication(t *testing.T) {
	f := newResetExecutionFixture(t)
	id := f.event(t, time.Now().UTC())
	constraint := "reset_outbox_test_" + fmt.Sprint(f.sub.GroupID)
	_, err := integrationDB.Exec(fmt.Sprintf(`ALTER TABLE subscription_reset_outbox ADD CONSTRAINT %s CHECK (group_id <> %d)`, constraint, f.sub.GroupID))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := integrationDB.Exec(`ALTER TABLE subscription_reset_outbox DROP CONSTRAINT ` + constraint)
		require.NoError(t, err)
	})
	_, err = f.executor.ExecuteResetEvent(context.Background(), id)
	require.Error(t, err)
	state, err := f.repo.GetSubscriptionQuotaState(context.Background(), f.sub.ID)
	require.NoError(t, err)
	require.Equal(t, []float64{10, 20, 30}, []float64{state.DailyUsageUSD, state.WeeklyUsageUSD, state.MonthlyUsageUSD})
	var count int
	require.NoError(t, integrationDB.QueryRow(`SELECT COUNT(*) FROM subscription_reset_executions WHERE event_id=$1`, id).Scan(&count))
	require.Zero(t, count)
	status, err := f.observer.GetStatus(context.Background(), f.sub.GroupID, 20)
	require.NoError(t, err)
	require.Equal(t, "confirmed", status.Events[0].Status)
}

func TestSubscriptionResetExecutionRejectsOldPolicyAndStaleConfirmation(t *testing.T) {
	f := newResetExecutionFixture(t)
	id := f.event(t, time.Now().UTC().Add(-20*time.Minute))
	result, err := f.executor.ExecuteResetEvent(context.Background(), id)
	require.NoError(t, err)
	require.Nil(t, result)
	status, err := f.observer.GetStatus(context.Background(), f.sub.GroupID, 20)
	require.NoError(t, err)
	require.Equal(t, "execution_expired", status.Events[0].Status)
	id = f.event(t, time.Now().UTC())
	f.policy.Mode = "off"
	_, err = f.observer.SavePolicy(context.Background(), f.policy)
	require.NoError(t, err)
	result, err = f.executor.ExecuteResetEvent(context.Background(), id)
	require.NoError(t, err)
	require.Nil(t, result)
	state, err := f.repo.GetSubscriptionQuotaState(context.Background(), f.sub.ID)
	require.NoError(t, err)
	require.Equal(t, 10.0, state.DailyUsageUSD)
	group, err := f.repo.GetGroupQuotaState(context.Background(), f.sub.GroupID)
	require.NoError(t, err)
	require.NotNil(t, group, "turning automation off preserves bucket accounting")
}

func TestSubscriptionResetExecutionConfirmedEarlyReplaysOneGeneration(t *testing.T) {
	f := newResetExecutionFixture(t)
	ctx := context.Background()
	f.policy.AllowEarlyResets = true
	var err error
	f.policy, err = f.observer.SavePolicy(ctx, f.policy)
	require.NoError(t, err)
	// Simulate an already-running policy so both fresh low observations can be
	// tested without sleeping. Only this empty fixture's activation is backdated.
	at := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)
	f.policy.UpdatedAt = at.Add(-time.Minute)
	encoded, err := json.Marshal(f.policy)
	require.NoError(t, err)
	_, err = integrationDB.Exec(`UPDATE subscription_reset_policies SET policy=$2::jsonb,updated_at=$3 WHERE group_id=$1`, f.sub.GroupID, string(encoded), f.policy.UpdatedAt)
	require.NoError(t, err)
	resetAt := at.Add(time.Hour)
	minutes := 10080
	record := func(accountID int64, observedAt time.Time, used float64) {
		t.Helper()
		err := f.observer.RecordSample(ctx, f.sub.GroupID, f.policy.Version, &service.SubscriptionResetSample{
			AccountID: accountID, ObservedAt: observedAt,
			Subject:     &service.SubscriptionResetQuotaSubject{UserID: fmt.Sprint(accountID), AccountID: "workspace", Scope: "codex:global"},
			UsedPercent: &used, ResetAt: &resetAt, WindowMinutes: &minutes, PlanType: "plus",
		})
		require.NoError(t, err)
	}
	for _, id := range f.policy.AccountIDs {
		record(id, at, 90)
	}
	for _, id := range f.policy.AccountIDs {
		record(id, at.Add(30*time.Second), 5)
	}
	status, err := f.observer.GetStatus(ctx, f.sub.GroupID, 20)
	require.NoError(t, err)
	require.Len(t, status.Events, 1)
	require.Equal(t, "pending", status.Events[0].Status)
	for _, id := range f.policy.AccountIDs {
		record(id, at.Add(61*time.Second), 4)
	}
	status, err = f.observer.GetStatus(ctx, f.sub.GroupID, 20)
	require.NoError(t, err)
	require.Equal(t, "confirmed", status.Events[0].Status)
	require.Equal(t, "early", status.Events[0].ConfirmedKind)
	eventID := status.Events[0].ID
	before, err := f.repo.GetSubscriptionQuotaState(ctx, f.sub.ID)
	require.NoError(t, err)
	secondReplica := NewSubscriptionResetExecutionRepository(integrationDB, f.repo)
	var wg sync.WaitGroup
	results := make(chan *service.RotateGroupQuotaResult, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			executor := f.executor
			if i%2 == 0 {
				executor = secondReplica
			}
			result, err := executor.ExecuteResetEvent(ctx, eventID)
			results <- result
			errs <- err
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	applied := 0
	for result := range results {
		if result != nil {
			applied++
		}
	}
	require.Equal(t, 1, applied)
	after, err := f.repo.GetSubscriptionQuotaState(ctx, f.sub.ID)
	require.NoError(t, err)
	require.Equal(t, before.GroupRevision+1, after.GroupRevision)
	require.NotEqual(t, before.DailyBucketID, after.DailyBucketID)
	require.Equal(t, before.WeeklyBucketID, after.WeeklyBucketID)
	require.NotEqual(t, before.MonthlyBucketID, after.MonthlyBucketID)
	require.True(t, before.ExpiresAt.Equal(after.ExpiresAt))
	// A later observer write must retain applied status in its persisted state,
	// so neither another sample nor a duplicate executor delivery can regrant.
	for _, id := range f.policy.AccountIDs {
		record(id, at.Add(90*time.Second), 4)
	}
	status, err = f.observer.GetStatus(ctx, f.sub.GroupID, 20)
	require.NoError(t, err)
	require.Equal(t, "applied", status.Events[0].Status)
	var storedStatus string
	require.NoError(t, integrationDB.QueryRow(`SELECT state->'events'->0->>'status' FROM subscription_reset_policies WHERE group_id=$1`, f.sub.GroupID).Scan(&storedStatus))
	require.Equal(t, "applied", storedStatus)
	result, err := f.executor.ExecuteResetEvent(ctx, eventID)
	require.NoError(t, err)
	require.Nil(t, result)
	var executions, outbox int
	require.NoError(t, integrationDB.QueryRow(`SELECT (SELECT COUNT(*) FROM subscription_reset_executions WHERE event_id=$1),(SELECT COUNT(*) FROM subscription_reset_outbox WHERE event_id=$1)`, eventID).Scan(&executions, &outbox))
	require.Equal(t, 1, executions)
	require.Equal(t, 1, outbox)
}
