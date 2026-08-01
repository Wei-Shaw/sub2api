//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestScheduledTestRepositoriesErrorRecoveryRoundTrip(t *testing.T) {
	ctx := context.Background()
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "scheduled-recovery-roundtrip"})
	t.Cleanup(func() { _ = integrationEntClient.Account.DeleteOneID(account.ID).Exec(ctx) })

	planRepo := NewScheduledTestPlanRepository(integrationDB)
	resultRepo := NewScheduledTestResultRepository(integrationDB)
	interval := 5
	now := time.Now().UTC().Truncate(time.Millisecond)
	plan, err := planRepo.Create(ctx, &service.ScheduledTestPlan{
		AccountID: account.ID, ModelID: "gpt-5.6-luna", CronExpression: "*/30 * * * *",
		ModelIDs: []string{"gpt-5.6-luna", "gpt-5.6-sol"},
		TriggerMode: service.ScheduledTestTriggerErrorRecovery, RetryIntervalMinutes: &interval,
		Enabled: true, MaxResults: 100, AutoRecover: true, NextRunAt: &now,
	})
	require.NoError(t, err)
	require.Equal(t, service.ScheduledTestTriggerErrorRecovery, plan.TriggerMode)
	require.Equal(t, []string{"gpt-5.6-luna", "gpt-5.6-sol"}, plan.ModelIDs)
	require.Equal(t, 5, *plan.RetryIntervalMinutes)
	require.Nil(t, plan.RetryCronExpression)

	result, err := resultRepo.Create(ctx, &service.ScheduledTestResult{
		PlanID: plan.ID, ModelID: "gpt-5.6-sol", Status: "running",
		StartedAt: now, FinishedAt: now,
	})
	require.NoError(t, err)
	require.Equal(t, "gpt-5.6-sol", result.ModelID)
	result.Status = "success"
	result.ResponseText = "ok"
	result.FinishedAt = now.Add(time.Second)
	result.LatencyMs = 1000
	require.NoError(t, resultRepo.Update(ctx, result))

	results, err := resultRepo.ListByPlanID(ctx, plan.ID, 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "gpt-5.6-sol", results[0].ModelID)
	require.Equal(t, "success", results[0].Status)
	require.Equal(t, "ok", results[0].ResponseText)

	nextRun := now.Add(time.Minute)
	require.NoError(t, planRepo.UpdateNextRun(ctx, plan.ID, nextRun))
	updated, err := planRepo.GetByID(ctx, plan.ID)
	require.NoError(t, err)
	require.WithinDuration(t, nextRun, *updated.NextRunAt, time.Millisecond)
}

func TestScheduledTestRepositoriesEmptyModelIDsRoundTrip(t *testing.T) {
	ctx := context.Background()
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "scheduled-recovery-all-models"})
	t.Cleanup(func() { _ = integrationEntClient.Account.DeleteOneID(account.ID).Exec(ctx) })

	interval := 5
	plan, err := NewScheduledTestPlanRepository(integrationDB).Create(ctx, &service.ScheduledTestPlan{
		AccountID: account.ID, ModelID: "gpt-5.6-luna", CronExpression: "*/30 * * * *",
		TriggerMode: service.ScheduledTestTriggerErrorRecovery, RetryIntervalMinutes: &interval,
		Enabled: true, MaxResults: 100, AutoRecover: true,
	})
	require.NoError(t, err)
	require.Empty(t, plan.ModelIDs)

	plan.ModelIDs = nil
	updated, err := NewScheduledTestPlanRepository(integrationDB).Update(ctx, plan)
	require.NoError(t, err)
	require.Empty(t, updated.ModelIDs)
}
