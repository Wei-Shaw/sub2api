//go:build unit

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func videoTaskRepoFixture() *service.VideoTask {
	group, sub := int64(3), int64(4)
	return &service.VideoTask{TaskID: "video-test", UserID: 1, APIKeyID: 2, AccountID: 313, GroupID: &group, SubscriptionID: &sub, LeaseToken: "lease-one", State: service.VideoTaskPending, RoutingFingerprint: strings.Repeat("a", 64), CostSnapshot: &service.VideoTaskCostSnapshot{Version: 1}, Pending: service.GrokVideoPendingBilling{Model: "video-model", VideoResolution: "720p", VideoDurationSeconds: 5, CreatedAt: "2026-09-15T06:00:00Z"}}
}

func videoTaskRows(t *testing.T, task *service.VideoTask) *sqlmock.Rows {
	pending, err := json.Marshal(task.Pending)
	require.NoError(t, err)
	cost, err := json.Marshal(task.CostSnapshot)
	require.NoError(t, err)
	var completion any
	if task.Completion != nil {
		b, e := json.Marshal(task.Completion)
		require.NoError(t, e)
		completion = b
	}
	return sqlmock.NewRows([]string{"task_id", "user_id", "api_key_id", "group_id", "account_id", "subscription_id", "pending", "completion", "state", "attempts", "next_attempt_at", "lease_token", "routing_fingerprint", "cost_snapshot", "last_error"}).AddRow(task.TaskID, task.UserID, task.APIKeyID, task.GroupID, task.AccountID, task.SubscriptionID, pending, completion, task.State, task.Attempts, time.Now(), task.LeaseToken, task.RoutingFingerprint, cost, task.LastError)
}

func TestVideoTaskRepositoryOwnedLookupAndMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := &usageBillingRepository{db: db}
	task := videoTaskRepoFixture()
	task.LastError = "poll_timeout"
	query := `SELECT .* FROM video_tasks WHERE task_id=\$1 AND user_id=\$2 AND api_key_id=\$3`
	mock.ExpectQuery(query).WithArgs(task.TaskID, int64(1), int64(2)).WillReturnRows(videoTaskRows(t, task))
	got, err := r.GetVideoTask(context.Background(), task.TaskID, 1, 2)
	require.NoError(t, err)
	require.Equal(t, task.Pending, got.Pending)
	require.Equal(t, task.SubscriptionID, got.SubscriptionID)
	require.Equal(t, task.LastError, got.LastError)
	mock.ExpectQuery(query).WithArgs(task.TaskID, int64(99), int64(2)).WillReturnError(sql.ErrNoRows)
	got, err = r.GetVideoTask(context.Background(), task.TaskID, 99, 2)
	require.NoError(t, err)
	require.Nil(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVideoTaskRepositoryCreateDoesNotOverwriteConflictingSnapshot(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "same snapshot", true: "conflict"}[conflict], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			r := &usageBillingRepository{db: db}
			task := videoTaskRepoFixture()
			query := `(?s)INSERT INTO video_tasks AS existing.*ON CONFLICT .*existing.account_id = EXCLUDED.account_id.*existing.group_id IS NOT DISTINCT FROM EXCLUDED.group_id.*existing.subscription_id IS NOT DISTINCT FROM EXCLUDED.subscription_id.*existing.pending = EXCLUDED.pending.*existing.routing_fingerprint = EXCLUDED.routing_fingerprint.*RETURNING task_id`
			expect := mock.ExpectQuery(query).WithArgs(task.TaskID, task.UserID, task.APIKeyID, *task.GroupID, task.AccountID, *task.SubscriptionID, sqlmock.AnyArg(), task.RoutingFingerprint, sqlmock.AnyArg())
			if conflict {
				expect.WillReturnError(sql.ErrNoRows)
			} else {
				expect.WillReturnRows(sqlmock.NewRows([]string{"task_id"}).AddRow(task.TaskID))
			}
			err = r.CreateVideoTask(context.Background(), task)
			if conflict {
				require.ErrorIs(t, err, service.ErrVideoTaskConflict)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestVideoTaskRepositoryClaimUsesDueLeaseAndOwnerFence(t *testing.T) {
	for _, owned := range []bool{false, true} {
		t.Run(map[bool]string{false: "worker", true: "owned"}[owned], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			r := &usageBillingRepository{db: db}
			task := videoTaskRepoFixture()
			query := `(?s)WITH due AS .*state IN \('pending','ready'\) AND \(next_attempt_at <= NOW\(\).*lease_until IS NULL OR lease_until <= NOW\(\).*task_id=\$1 AND user_id=\$2 AND api_key_id=\$3.*LIMIT 1 FOR UPDATE SKIP LOCKED.*UPDATE video_tasks t SET lease_token=\$4.*attempts=t.attempts\+1.*t.task_id=due.task_id AND t.user_id=due.user_id AND t.api_key_id=due.api_key_id.*RETURNING`
			id, user, key := "", int64(0), int64(0)
			if owned {
				id, user, key = task.TaskID, task.UserID, task.APIKeyID
			}
			mock.ExpectQuery(query).WithArgs(id, user, key, sqlmock.AnyArg(), int64(120000)).WillReturnRows(videoTaskRows(t, task))
			var got *service.VideoTask
			if owned {
				got, err = r.ClaimOwnedVideoTask(context.Background(), id, user, key, 2*time.Minute)
			} else {
				got, err = r.ClaimDueVideoTask(context.Background(), 2*time.Minute)
			}
			require.NoError(t, err)
			require.Equal(t, task.TaskID, got.TaskID)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestVideoTaskRepositoryExpiredLeaseCannotSaveOrFinish(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := &usageBillingRepository{db: db}
	task := videoTaskRepoFixture()
	mock.ExpectExec(`(?s)UPDATE video_tasks SET completion=.*WHERE task_id=\$1 AND user_id=\$2 AND api_key_id=\$3 AND lease_token=\$4.*lease_until > NOW\(\) AND state='pending'`).WithArgs(task.TaskID, task.UserID, task.APIKeyID, task.LeaseToken, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, r.SaveVideoTaskCompletion(context.Background(), task, &service.VideoTaskCompletion{}), service.ErrVideoTaskLeaseLost)
	mock.ExpectExec(`(?s)UPDATE video_tasks SET state=.*WHERE task_id=\$1 AND user_id=\$2 AND api_key_id=\$3 AND lease_token=\$4.*lease_until > NOW\(\).*state IN \('pending','ready'\)`).WithArgs(task.TaskID, task.UserID, task.APIKeyID, task.LeaseToken, service.VideoTaskCompleted, int64(0), "").WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, r.FinishVideoTaskAttempt(context.Background(), task, service.VideoTaskCompleted, 0, ""), service.ErrVideoTaskLeaseLost)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVideoTaskRepositoryReadyRetryPreservesCompletion(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := &usageBillingRepository{db: db}
	task := videoTaskRepoFixture()
	task.State = service.VideoTaskReady
	mock.ExpectExec(`(?s)UPDATE video_tasks SET state=\$5,next_attempt_at=CASE.*ELSE NOW\(\)\+\$6\*INTERVAL '1 millisecond' END,\s*last_error=\$7,lease_token=NULL,lease_until=NULL.*AND \(state='pending' OR \$5 IN \('ready','completed'\)\)`).WithArgs(task.TaskID, task.UserID, task.APIKeyID, task.LeaseToken, service.VideoTaskReady, int64(15000), "billing_failed").WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, r.FinishVideoTaskAttempt(context.Background(), task, service.VideoTaskReady, 15*time.Second, "billing_failed"))
	require.NoError(t, mock.ExpectationsWereMet())
}
