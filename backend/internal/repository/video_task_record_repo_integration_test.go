//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestVideoTaskRecordsIntegration(t *testing.T) {
	ctx := context.Background()
	r := &usageBillingRepository{db: integrationDB}
	prefix := "record-" + uuid.NewString()
	task := &service.VideoTask{TaskID: prefix, UserID: 1, APIKeyID: 2, AccountID: 3, RoutingFingerprint: strings.Repeat("a", 64), Pending: service.GrokVideoPendingBilling{TrackingOnly: true, Model: "grok-video", VideoDurationSeconds: 6, VideoResolution: "480p"}}
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM video_tasks WHERE user_id=1 AND api_key_id=2 AND task_id LIKE $1`, prefix+"%")
	})
	require.NoError(t, r.BeginVideoTaskRecord(ctx, task))
	items, total, err := r.ListVideoTasks(ctx, service.VideoTaskAdminFilter{Page: 1, PageSize: 10, Days: 1, Search: prefix})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "submission_unknown", items[0].DisplayStatus)
	require.Nil(t, items[0].NextAttemptAt)
	// Old workers also respect next_attempt_at, before their code is upgraded.
	var oldWorkerEligible bool
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT next_attempt_at<=NOW() FROM video_tasks WHERE task_id=$1`, prefix).Scan(&oldWorkerEligible))
	require.False(t, oldWorkerEligible)
	claimed, err := r.ClaimOwnedVideoTask(ctx, prefix, 1, 2, time.Minute)
	require.NoError(t, err)
	require.Nil(t, claimed)
	require.NoError(t, r.AcceptVideoTaskRecord(ctx, task, prefix+"-accepted", service.VideoTaskPending))
	old, err := r.GetVideoTask(ctx, prefix, 1, 2)
	require.NoError(t, err)
	require.Nil(t, old)
	for i := 0; i < 2; i++ {
		require.NoError(t, r.ObserveVideoTaskRecord(ctx, prefix+"-accepted", 1, 2, 3, service.VideoTaskCompleted))
	}
	got, err := r.GetVideoTask(ctx, prefix+"-accepted", 1, 2)
	require.NoError(t, err)
	require.Equal(t, service.VideoTaskCompleted, got.State)
	require.NoError(t, r.ObserveVideoTaskRecord(ctx, prefix+"-accepted", 1, 2, 3, service.VideoTaskFailed))
	got, err = r.GetVideoTask(ctx, prefix+"-accepted", 1, 2)
	require.NoError(t, err)
	require.Equal(t, service.VideoTaskCompleted, got.State)
	items, _, err = r.ListVideoTasks(ctx, service.VideoTaskAdminFilter{Page: 1, PageSize: 10, Days: 1, Search: prefix})
	require.NoError(t, err)
	require.Equal(t, "completed", items[0].DisplayStatus)
	require.Equal(t, "review", items[0].BillingStatus)

	// An already-persisted OpenAI task keeps its billing snapshot and state.
	_, err = integrationDB.ExecContext(ctx, `UPDATE video_tasks SET pending=pending-'tracking_only',
		cost_snapshot='{"version":1,"cost":{"ActualCost":1.2}}' WHERE task_id=$1`, prefix+"-accepted")
	require.NoError(t, err)
	task.TaskID = prefix + "-duplicate"
	require.NoError(t, r.BeginVideoTaskRecord(ctx, task))
	require.NoError(t, r.AcceptVideoTaskRecord(ctx, task, prefix+"-accepted", service.VideoTaskPending))
	items, total, err = r.ListVideoTasks(ctx, service.VideoTaskAdminFilter{Page: 1, PageSize: 10, Days: 1, Search: prefix})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "completed", items[0].DisplayStatus)
	require.NotNil(t, items[0].ExpectedCost)
	require.Equal(t, 1.2, *items[0].ExpectedCost)

	task.TaskID = prefix + "-conflict"
	task.AccountID = 4
	require.NoError(t, r.BeginVideoTaskRecord(ctx, task))
	require.ErrorIs(t, r.AcceptVideoTaskRecord(ctx, task, prefix+"-accepted", service.VideoTaskPending), service.ErrVideoTaskConflict)
	wrong, err := r.GetVideoTask(ctx, prefix+"-conflict", 1, 2)
	require.NoError(t, err)
	require.NotNil(t, wrong)
}
