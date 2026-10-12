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

func TestVideoTaskRepositoryDurableLeaseRecovery(t *testing.T) {
	_ = testEntClient(t)
	ctx := context.Background()
	r := &usageBillingRepository{db: integrationDB}
	group, sub := int64(3), int64(4)
	task := &service.VideoTask{TaskID: "video-" + uuid.NewString(), UserID: 1, APIKeyID: 2, AccountID: 313, GroupID: &group, SubscriptionID: &sub, RoutingFingerprint: strings.Repeat("a", 64), CostSnapshot: &service.VideoTaskCostSnapshot{Version: 1, Cost: service.CostBreakdown{ActualCost: 0.25}}, Pending: service.GrokVideoPendingBilling{Model: "video-model", VideoResolution: "720p", VideoDurationSeconds: 5, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}}
	require.NoError(t, r.CreateVideoTask(ctx, task))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM video_tasks WHERE task_id=$1 AND user_id=$2 AND api_key_id=$3`, task.TaskID, task.UserID, task.APIKeyID)
	})
	require.NoError(t, r.CreateVideoTask(ctx, task), "same create snapshot is idempotent")
	changed := *task
	changed.AccountID++
	require.ErrorIs(t, r.CreateVideoTask(ctx, &changed), service.ErrVideoTaskConflict)
	missing, err := r.GetVideoTask(ctx, task.TaskID, 99, 2)
	require.NoError(t, err)
	require.Nil(t, missing)
	first, err := r.ClaimOwnedVideoTask(ctx, task.TaskID, 1, 2, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, first)
	second, err := r.ClaimOwnedVideoTask(ctx, task.TaskID, 1, 2, time.Minute)
	require.NoError(t, err)
	require.Nil(t, second, "live lease excludes another process")
	_, err = integrationDB.ExecContext(ctx, `UPDATE video_tasks SET lease_until=NOW()-INTERVAL '1 second' WHERE task_id=$1 AND user_id=$2 AND api_key_id=$3`, task.TaskID, task.UserID, task.APIKeyID)
	require.NoError(t, err)
	second, err = r.ClaimOwnedVideoTask(ctx, task.TaskID, 1, 2, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, second)
	require.NotEqual(t, first.LeaseToken, second.LeaseToken)
	completion := &service.VideoTaskCompletion{Model: "video-model", BillingModel: "video-model", VideoResolution: "720p", VideoDurationSeconds: 5}
	require.ErrorIs(t, r.SaveVideoTaskCompletion(ctx, first, completion), service.ErrVideoTaskLeaseLost)
	require.NoError(t, r.SaveVideoTaskCompletion(ctx, second, completion))
	require.NoError(t, r.FinishVideoTaskAttempt(ctx, second, service.VideoTaskReady, 0, "billing_failed"))
	// Simulate a fresh process/repository, without Redis or any in-memory state.
	restarted := &usageBillingRepository{db: integrationDB}
	third, err := restarted.ClaimOwnedVideoTask(ctx, task.TaskID, 1, 2, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, third)
	require.Equal(t, completion, third.Completion)
	require.Equal(t, task.Pending, third.Pending)
	require.Equal(t, sub, *third.SubscriptionID)
	require.Equal(t, task.CostSnapshot, third.CostSnapshot)
	require.NoError(t, restarted.FinishVideoTaskAttempt(ctx, third, service.VideoTaskCompleted, 0, ""))
	completed, err := restarted.GetVideoTask(ctx, task.TaskID, 1, 2)
	require.NoError(t, err)
	require.Equal(t, service.VideoTaskCompleted, completed.State)
	next, err := restarted.ClaimOwnedVideoTask(ctx, task.TaskID, 1, 2, time.Minute)
	require.NoError(t, err)
	require.Nil(t, next)
}
