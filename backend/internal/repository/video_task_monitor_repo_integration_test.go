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

func TestVideoMonitorIntegrationUsageSyncWithoutCharges(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	r := &usageBillingRepository{db: integrationDB}
	prefix := "monitor-" + uuid.NewString()[:8]
	user := mustCreateUser(t, client, &service.User{Email: prefix + "@example.com", Balance: 100})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-" + prefix})
	account := mustCreateAccount(t, client, &service.Account{Name: prefix, Platform: service.PlatformGrok})
	other := mustCreateAccount(t, client, &service.Account{Name: prefix + "-other", Platform: service.PlatformGrok})
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, `DELETE FROM video_tasks WHERE user_id=$1`, user.ID) })
	create := func(suffix string) *service.VideoTask {
		t.Helper()
		task := &service.VideoTask{TaskID: prefix + suffix, UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID,
			RoutingFingerprint: strings.Repeat("a", 64), Pending: service.GrokVideoPendingBilling{TrackingOnly: true, Platform: service.PlatformGrok, Model: "grok-imagine-video", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}}
		require.NoError(t, r.BeginVideoTaskRecord(ctx, task))
		require.NoError(t, r.AcceptVideoTaskRecord(ctx, task, task.TaskID+"-accepted", service.VideoTaskPending))
		task.TaskID += "-accepted"
		_, err := integrationDB.ExecContext(ctx, `UPDATE video_tasks SET updated_at=NOW()-INTERVAL '10 minutes' WHERE task_id=$1`, task.TaskID)
		require.NoError(t, err)
		return task
	}
	addUsage := func(task *service.VideoTask, accountID int64) {
		t.Helper()
		_, err := NewUsageLogRepository(client, integrationDB).Create(ctx, &service.UsageLog{RequestID: "grok-video:" + task.TaskID, UserID: user.ID, APIKeyID: key.ID, AccountID: accountID, Model: "video", ActualCost: 0.75, TotalCost: 0.75, CreatedAt: time.Now()})
		require.NoError(t, err)
	}
	assertNoCharge := func(expectedLogs int) {
		t.Helper()
		var balance float64
		var count int
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1`, user.ID).Scan(&balance))
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM usage_logs WHERE api_key_id=$1`, key.ID).Scan(&count))
		require.Equal(t, float64(100), balance)
		require.Equal(t, expectedLogs, count)
	}
	task := create("-billed")
	addUsage(task, other.ID)
	claimed, err := r.ClaimDueVideoTaskMonitor(ctx, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, task.TaskID, claimed.TaskID)
	second, err := r.ClaimDueVideoTaskMonitor(ctx, time.Minute)
	require.NoError(t, err)
	require.Nil(t, second, "concurrent monitors cannot claim the same record")
	done, err := r.SyncVideoTaskMonitorUsage(ctx, claimed)
	require.NoError(t, err)
	require.False(t, done, "same task ID on a different upstream account is not a match")
	// Replace only this deliberately mismatched test fixture. Production usage
	// records have a unique (request_id, api_key_id) constraint.
	_, err = integrationDB.ExecContext(ctx, `DELETE FROM usage_logs WHERE request_id=$1 AND api_key_id=$2`, "grok-video:"+task.TaskID, key.ID)
	require.NoError(t, err)
	addUsage(task, account.ID)
	assertNoCharge(1)
	done, err = r.SyncVideoTaskMonitorUsage(ctx, claimed)
	require.NoError(t, err)
	require.True(t, done)
	assertNoCharge(1)
	items, _, err := r.ListVideoTasks(ctx, service.VideoTaskAdminFilter{Page: 1, PageSize: 10, Days: 1, Search: task.TaskID})
	require.NoError(t, err)
	require.Equal(t, "completed", items[0].DisplayStatus)
	require.Equal(t, "charged", items[0].BillingStatus)
	require.Equal(t, 0.75, *items[0].ChargedCost)
	require.ErrorIs(t, r.FinishVideoTaskMonitor(ctx, claimed, service.VideoTaskFailed, "upstream_failed"), service.ErrVideoTaskLeaseLost)

	// An upstream completion with no usage is just an observation, never a charge.
	task = create("-unbilled")
	claimed, err = r.ClaimDueVideoTaskMonitor(ctx, time.Minute)
	require.NoError(t, err)
	require.Equal(t, task.TaskID, claimed.TaskID)
	done, err = r.SyncVideoTaskMonitorUsage(ctx, claimed)
	require.NoError(t, err)
	require.False(t, done)
	var oldWorkerDue bool
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT next_attempt_at<=NOW() FROM video_tasks WHERE task_id=$1`, task.TaskID).Scan(&oldWorkerDue))
	require.False(t, oldWorkerDue, "even old billing workers must not claim monitoring records")
	require.NoError(t, r.FinishVideoTaskMonitor(ctx, claimed, service.VideoTaskCompleted, ""))
	assertNoCharge(1)
	items, _, err = r.ListVideoTasks(ctx, service.VideoTaskAdminFilter{Page: 1, PageSize: 10, Days: 1, Search: task.TaskID})
	require.NoError(t, err)
	require.Equal(t, "completed", items[0].DisplayStatus)
	require.Equal(t, "review", items[0].BillingStatus)
	require.Nil(t, items[0].ChargedCost)

	// Bounded retries preserve an unknown outcome, with no fictitious failure.
	task = create("-review")
	claimed, err = r.ClaimDueVideoTaskMonitor(ctx, time.Minute)
	require.NoError(t, err)
	require.Equal(t, task.TaskID, claimed.TaskID)
	require.NoError(t, r.FinishVideoTaskMonitor(ctx, claimed, service.VideoTaskPending, "poll_exhausted"))
	_, err = integrationDB.ExecContext(ctx, `UPDATE video_tasks SET updated_at=NOW()-INTERVAL '2 days' WHERE task_id=$1`, task.TaskID)
	require.NoError(t, err)
	second, err = r.ClaimDueVideoTaskMonitor(ctx, time.Minute)
	require.NoError(t, err)
	require.Nil(t, second)
	items, _, err = r.ListVideoTasks(ctx, service.VideoTaskAdminFilter{Page: 1, PageSize: 10, Days: 1, Search: task.TaskID, Status: "review_required"})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "pending", items[0].State)
	require.Nil(t, items[0].NextAttemptAt)
	assertNoCharge(1)
}

func TestVideoMonitorExhaustedDurableQueryIsNotClaimedByOldWorkers(t *testing.T) {
	r := &usageBillingRepository{db: integrationDB}
	ctx := context.Background()
	task := &service.VideoTask{TaskID: "exhausted-" + uuid.NewString(), UserID: 1, APIKeyID: 2, AccountID: 3, RoutingFingerprint: strings.Repeat("a", 64), Pending: service.GrokVideoPendingBilling{Model: "video"}, CostSnapshot: &service.VideoTaskCostSnapshot{Version: 1}}
	require.NoError(t, r.CreateVideoTask(ctx, task))
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, `DELETE FROM video_tasks WHERE task_id=$1`, task.TaskID) })
	claimed, err := r.ClaimOwnedVideoTask(ctx, task.TaskID, 1, 2, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.NoError(t, r.FinishVideoTaskAttempt(ctx, claimed, service.VideoTaskPending, 4*time.Minute, "poll_exhausted"))
	var eligible bool
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT state IN ('pending','ready') AND next_attempt_at<=NOW() FROM video_tasks WHERE task_id=$1`, task.TaskID).Scan(&eligible))
	require.False(t, eligible)
	// Stop background retries, but retain bounded explicit client lookups so a
	// recovered upstream can still return the original task through its old flow.
	_, err = integrationDB.ExecContext(ctx, `UPDATE video_tasks SET updated_at=NOW()-INTERVAL '5 minutes' WHERE task_id=$1`, task.TaskID)
	require.NoError(t, err)
	background, err := r.ClaimDueVideoTask(ctx, time.Minute)
	require.NoError(t, err)
	if background != nil {
		require.NotEqual(t, task.TaskID, background.TaskID)
	}
	owned, err := r.ClaimOwnedVideoTask(ctx, task.TaskID, 1, 2, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, owned)
	require.Equal(t, task.TaskID, owned.TaskID)
	concurrent, err := r.ClaimOwnedVideoTask(ctx, task.TaskID, 1, 2, time.Minute)
	require.NoError(t, err)
	require.Nil(t, concurrent)
}
