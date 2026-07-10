//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestReliabilityReconciliationRepositoryFindsDatabaseDriftReadOnly(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	expired := newVideoTaskFinalizationFixture(t, "3")
	_, err := integrationDB.ExecContext(ctx, "UPDATE billing_reservations SET expires_at = $2 WHERE id = $1", expired.reservation.ID, now.Add(-time.Minute))
	require.NoError(t, err)

	drift := newVideoTaskFinalizationFixture(t, "3")
	input := newVideoTaskFinalizationInput(drift.task, service.VideoStatusSucceeded, "1.25")
	_, err = newVideoTaskFinalizerForIntegration(t).Finalize(ctx, input)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "UPDATE users SET balance = balance + 0.01 WHERE id = $1", drift.user.ID)
	require.NoError(t, err)

	dead := newVideoTaskFinalizationFixture(t, "3")
	_, err = integrationDB.ExecContext(ctx, `
		UPDATE video_tasks SET status = 'succeeded', settlement_status = 'settled', result_url = '' WHERE id = $1;
		INSERT INTO domain_outbox (aggregate_type, aggregate_id, event_type, dedup_key, payload, status, next_attempt_at)
		VALUES ('video_task', $1, 'video.archive_asset', 'reconcile-dead-' || $1::text, '{}'::jsonb, 'dead', $2)
	`, dead.task.ID, now)
	require.NoError(t, err)

	source := NewReliabilityReconciliationRepository(integrationDB)
	findings, err := service.NewReliabilityReconciler(source).DryRun(ctx, now, 100)
	require.NoError(t, err)
	require.Contains(t, reliabilityFindingCodesForTask(findings, expired.task.ID), service.ReliabilityCodeExpiredQueuedUndispatched)
	require.Contains(t, reliabilityFindingCodesForTask(findings, drift.task.ID), service.ReliabilityCodeLedgerBalanceDrift)
	require.Contains(t, reliabilityFindingCodesForTask(findings, dead.task.ID), service.ReliabilityCodeDeadOutbox)
	require.Contains(t, reliabilityFindingCodesForTask(findings, dead.task.ID), service.ReliabilityCodeSuccessWithoutDeliverable)
}

func TestReliabilityReconciliationDoesNotCompareHistoricTaskLedgerToCurrentBalance(t *testing.T) {
	ctx := context.Background()
	user := newVideoTaskCreationUser(t, 10)
	providerID := newVideoTaskCreationProvider(t)
	createAndSettle := func() *service.VideoTask {
		created, err := NewVideoTaskCreationRepository(integrationDB).CreateWithReservation(ctx, newVideoTaskCreationInput(
			user.ID,
			providerID,
			service.HashIdempotencyKey("reconcile-history-create-"+uuid.NewString()),
			service.HashIdempotencyKey("reconcile-history-payload-"+uuid.NewString()),
			"1.25",
		))
		require.NoError(t, err)
		_, err = newVideoTaskFinalizerForIntegration(t).Finalize(ctx, newVideoTaskFinalizationInput(created.Task, service.VideoStatusSucceeded, "1.25"))
		require.NoError(t, err)
		return created.Task
	}
	first := createAndSettle()
	second := createAndSettle()

	source := NewReliabilityReconciliationRepository(integrationDB)
	snapshot, err := source.ReliabilitySnapshot(ctx, time.Now().UTC(), 10_000)
	require.NoError(t, err)
	seen := make(map[int64]bool, len(snapshot.Rows))
	for _, row := range snapshot.Rows {
		seen[row.TaskID] = true
	}
	require.True(t, seen[first.ID])
	require.True(t, seen[second.ID])

	findings, err := service.NewReliabilityReconciler(source).DryRun(ctx, time.Now().UTC(), 10_000)
	require.NoError(t, err)
	require.NotContains(t, reliabilityFindingCodesForTask(findings, first.ID), service.ReliabilityCodeLedgerBalanceDrift)
	require.NotContains(t, reliabilityFindingCodesForTask(findings, second.ID), service.ReliabilityCodeLedgerBalanceDrift)
}

func TestReliabilityReconciliationPrioritizesAnomaliesOverRecentHealthyRows(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	staleAnomaly := newVideoTaskFinalizationFixture(t, "3")
	healthyRecent := newVideoTaskFinalizationFixture(t, "3")
	_, err := integrationDB.ExecContext(ctx, `
		UPDATE video_tasks
		SET status = 'succeeded',
		    settlement_status = 'pending',
		    updated_at = $2
		WHERE id = $1;
		UPDATE video_tasks
		SET status = 'succeeded',
		    settlement_status = 'settled',
		    result_url = 'https://cdn.example.com/healthy.mp4',
		    local_asset_path = 'assets/video/healthy/out.mp4',
		    updated_at = $4
		WHERE id = $3
	`, staleAnomaly.task.ID, now.Add(-2*time.Hour), healthyRecent.task.ID, now.Add(time.Hour))
	require.NoError(t, err)

	snapshot, err := NewReliabilityReconciliationRepository(integrationDB).ReliabilitySnapshot(ctx, now, 1)
	require.NoError(t, err)
	require.Len(t, snapshot.Rows, 1)
	require.Equal(t, staleAnomaly.task.ID, snapshot.Rows[0].TaskID)
}

func TestReliabilityReconciliationPrioritizesRecentRowsWhenLimited(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	old := newVideoTaskFinalizationFixture(t, "3")
	recent := newVideoTaskFinalizationFixture(t, "3")
	_, err := integrationDB.ExecContext(ctx, `
		UPDATE video_tasks SET updated_at = $2 WHERE id = $1;
		UPDATE video_tasks SET updated_at = $4 WHERE id = $3
	`, old.task.ID, now.Add(-time.Hour), recent.task.ID, now.Add(time.Hour))
	require.NoError(t, err)

	snapshot, err := NewReliabilityReconciliationRepository(integrationDB).ReliabilitySnapshot(ctx, now, 1)
	require.NoError(t, err)
	require.Len(t, snapshot.Rows, 1)
	require.Equal(t, recent.task.ID, snapshot.Rows[0].TaskID)
}

func TestReliabilityReconciliationPrioritizesExpiredRemoteURLOverRecentHealthyRows(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	staleExpiredRemote := newVideoTaskFinalizationFixture(t, "3")
	healthyRecent := newVideoTaskFinalizationFixture(t, "3")
	expiredURL := "https://cdn.example.com/out.mp4?X-Amz-Date=20260701T010000Z&X-Amz-Expires=3600&X-Amz-Signature=abc"
	_, err := integrationDB.ExecContext(ctx, `
		UPDATE video_tasks
		SET status = 'succeeded',
		    settlement_status = 'settled',
		    result_url = $2,
		    local_asset_path = NULL,
		    completed_at = $3,
		    updated_at = $4
		WHERE id = $1;
		UPDATE video_tasks
		SET status = 'succeeded',
		    settlement_status = 'settled',
		    result_url = 'https://cdn.example.com/healthy.mp4',
		    local_asset_path = 'assets/video/healthy/out.mp4',
		    updated_at = $6
		WHERE id = $5
	`, staleExpiredRemote.task.ID, expiredURL, now.Add(-48*time.Hour), now.Add(-2*time.Hour), healthyRecent.task.ID, now.Add(time.Hour))
	require.NoError(t, err)

	snapshot, err := NewReliabilityReconciliationRepository(integrationDB).ReliabilitySnapshot(ctx, now, 1)
	require.NoError(t, err)
	require.Len(t, snapshot.Rows, 1)
	require.Equal(t, staleExpiredRemote.task.ID, snapshot.Rows[0].TaskID)
	require.False(t, snapshot.Rows[0].RemoteAssetAvailable)

	findings, err := service.NewReliabilityReconciler(NewReliabilityReconciliationRepository(integrationDB)).DryRun(ctx, now, 1)
	require.NoError(t, err)
	require.Contains(t, reliabilityFindingCodesForTask(findings, staleExpiredRemote.task.ID), service.ReliabilityCodeSuccessWithoutDeliverable)
}

func reliabilityFindingCodesForTask(findings []service.ReliabilityFinding, taskID int64) []string {
	codes := make([]string, 0)
	for _, finding := range findings {
		if finding.TaskID == taskID {
			codes = append(codes, finding.Code)
		}
	}
	return codes
}
