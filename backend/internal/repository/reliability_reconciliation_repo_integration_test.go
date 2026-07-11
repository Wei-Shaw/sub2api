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
	drift := newVideoTaskFinalizationFixture(t, "3")
	dead := newVideoTaskFinalizationFixture(t, "3")
	trackReliabilityOwnedIDs(t, expired.task.ID, drift.task.ID, dead.task.ID)

	_, err := integrationDB.ExecContext(ctx, "UPDATE billing_reservations SET expires_at = $2 WHERE id = $1", expired.reservation.ID, now.Add(-time.Minute))
	require.NoError(t, err)

	input := newVideoTaskFinalizationInput(drift.task, service.VideoStatusSucceeded, "1.25")
	_, err = newVideoTaskFinalizerForIntegration(t).Finalize(ctx, input)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "UPDATE users SET balance = balance + 0.01 WHERE id = $1", drift.user.ID)
	require.NoError(t, err)

	_, err = integrationDB.ExecContext(ctx, "UPDATE video_tasks SET status = 'succeeded', settlement_status = 'settled', result_url = '' WHERE id = $1", dead.task.ID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `
		INSERT INTO domain_outbox (aggregate_type, aggregate_id, event_type, dedup_key, payload, status, next_attempt_at)
		VALUES ('video_task', $1::bigint, 'video.archive_asset', 'reconcile-dead-' || $1::bigint::text, '{}'::jsonb, 'dead', $2)
	`, dead.task.ID, now)
	require.NoError(t, err)

	source := NewReliabilityReconciliationRepository(integrationDB)
	findings, err := service.NewReliabilityReconciler(source).DryRun(ctx, now, 10_000)
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
	trackReliabilityOwnedIDs(t, first.ID, second.ID)

	source := NewReliabilityReconciliationRepository(integrationDB)
	snapshot, err := source.ReliabilitySnapshot(ctx, time.Now().UTC(), 10_000)
	require.NoError(t, err)
	seen := reliabilitySnapshotTaskIDs(snapshot.Rows, first.ID, second.ID)
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
	trackReliabilityOwnedIDs(t, staleAnomaly.task.ID, healthyRecent.task.ID)
	_, err := integrationDB.ExecContext(ctx, `
		UPDATE video_tasks
		SET status = 'succeeded',
		    settlement_status = 'pending',
		    updated_at = $2
		WHERE id = $1
	`, staleAnomaly.task.ID, now.Add(-2*time.Hour))
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `
		UPDATE video_tasks
		SET status = 'succeeded',
		    settlement_status = 'settled',
		    result_url = 'https://cdn.example.com/healthy.mp4',
		    local_asset_path = 'assets/video/healthy/out.mp4',
		    updated_at = $2
		WHERE id = $1
	`, healthyRecent.task.ID, now.Add(time.Hour))
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "UPDATE billing_reservations SET status = 'settled', settled_at = $2 WHERE id = $1", healthyRecent.reservation.ID, now)
	require.NoError(t, err)

	snapshot, err := NewReliabilityReconciliationRepository(integrationDB).ReliabilitySnapshot(ctx, now, 10_000)
	require.NoError(t, err)
	owned := reliabilitySnapshotRowsForTasks(snapshot.Rows, staleAnomaly.task.ID, healthyRecent.task.ID)
	require.NotEmpty(t, owned)
	require.Equal(t, staleAnomaly.task.ID, owned[0].TaskID)
}

func TestReliabilityReconciliationPrioritizesRecentRowsWhenLimited(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	old := newVideoTaskFinalizationFixture(t, "3")
	recent := newVideoTaskFinalizationFixture(t, "3")
	trackReliabilityOwnedIDs(t, old.task.ID, recent.task.ID)
	_, err := integrationDB.ExecContext(ctx, "UPDATE video_tasks SET updated_at = $2 WHERE id = $1", old.task.ID, now.Add(-time.Hour))
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "UPDATE video_tasks SET updated_at = $2 WHERE id = $1", recent.task.ID, now.Add(time.Hour))
	require.NoError(t, err)

	snapshot, err := NewReliabilityReconciliationRepository(integrationDB).ReliabilitySnapshot(ctx, now, 10_000)
	require.NoError(t, err)
	owned := reliabilitySnapshotRowsForTasks(snapshot.Rows, old.task.ID, recent.task.ID)
	require.NotEmpty(t, owned)
	require.Equal(t, recent.task.ID, owned[0].TaskID)
}

func TestReliabilityReconciliationPrioritizesExpiredRemoteURLOverRecentHealthyRows(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	staleExpiredRemote := newVideoTaskFinalizationFixture(t, "3")
	healthyRecent := newVideoTaskFinalizationFixture(t, "3")
	trackReliabilityOwnedIDs(t, staleExpiredRemote.task.ID, healthyRecent.task.ID)
	expiredURL := "https://cdn.example.com/out.mp4?X-Amz-Date=20260701T010000Z&X-Amz-Expires=3600&X-Amz-Signature=abc"
	_, err := integrationDB.ExecContext(ctx, `
		UPDATE video_tasks
		SET status = 'succeeded',
		    settlement_status = 'settled',
		    result_url = $2,
		    local_asset_path = NULL,
		    completed_at = $3,
		    updated_at = $4
		WHERE id = $1
	`, staleExpiredRemote.task.ID, expiredURL, now.Add(-48*time.Hour), now.Add(-2*time.Hour))
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `
		UPDATE video_tasks
		SET status = 'succeeded',
		    settlement_status = 'settled',
		    result_url = 'https://cdn.example.com/healthy.mp4',
		    local_asset_path = 'assets/video/healthy/out.mp4',
		    updated_at = $2
		WHERE id = $1
	`, healthyRecent.task.ID, now.Add(time.Hour))
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "UPDATE billing_reservations SET status = 'settled', settled_at = $2 WHERE id = $1", healthyRecent.reservation.ID, now)
	require.NoError(t, err)

	snapshot, err := NewReliabilityReconciliationRepository(integrationDB).ReliabilitySnapshot(ctx, now, 10_000)
	require.NoError(t, err)
	owned := reliabilitySnapshotRowsForTasks(snapshot.Rows, staleExpiredRemote.task.ID, healthyRecent.task.ID)
	require.NotEmpty(t, owned)
	require.Equal(t, staleExpiredRemote.task.ID, owned[0].TaskID)
	require.False(t, owned[0].RemoteAssetAvailable)

	findings, err := service.NewReliabilityReconciler(NewReliabilityReconciliationRepository(integrationDB)).DryRun(ctx, now, 10_000)
	require.NoError(t, err)
	require.Contains(t, reliabilityFindingCodesForTask(findings, staleExpiredRemote.task.ID), service.ReliabilityCodeSuccessWithoutDeliverable)
}

func trackReliabilityOwnedIDs(t *testing.T, taskIDs ...int64) {
	t.Helper()
	owned := append([]int64(nil), taskIDs...)
	t.Cleanup(func() {
		cleanupReliabilityOwnedIDs(t, owned...)
	})
}

func cleanupReliabilityOwnedIDs(t *testing.T, taskIDs ...int64) {
	t.Helper()
	if len(taskIDs) == 0 {
		return
	}
	ctx := context.Background()
	for _, taskID := range taskIDs {
		_, err := integrationDB.ExecContext(ctx, "DELETE FROM domain_outbox WHERE aggregate_type = 'video_task' AND aggregate_id = $1", taskID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM video_task_events WHERE video_task_id = $1", taskID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM video_usage_logs WHERE video_task_id = $1", taskID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM billing_transactions WHERE source_type = 'video_task' AND source_id = $1", taskID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM billing_reservations WHERE source_type = 'video_task' AND source_id = $1", taskID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM video_tasks WHERE id = $1", taskID)
		require.NoError(t, err)
	}
}

func reliabilitySnapshotRowsForTasks(rows []service.ReliabilityReconciliationRow, taskIDs ...int64) []service.ReliabilityReconciliationRow {
	wanted := make(map[int64]struct{}, len(taskIDs))
	for _, id := range taskIDs {
		wanted[id] = struct{}{}
	}
	out := make([]service.ReliabilityReconciliationRow, 0, len(taskIDs))
	for _, row := range rows {
		if _, ok := wanted[row.TaskID]; ok {
			out = append(out, row)
		}
	}
	return out
}

func reliabilitySnapshotTaskIDs(rows []service.ReliabilityReconciliationRow, taskIDs ...int64) map[int64]bool {
	wanted := make(map[int64]struct{}, len(taskIDs))
	for _, id := range taskIDs {
		wanted[id] = struct{}{}
	}
	seen := make(map[int64]bool, len(taskIDs))
	for _, row := range rows {
		if _, ok := wanted[row.TaskID]; ok {
			seen[row.TaskID] = true
		}
	}
	return seen
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
