//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Uses the existing disposable-DB integration harness, never a production DSN.
func TestVideoTaskAdminRepositoryIntegration(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	r := &usageBillingRepository{db: integrationDB}
	prefix := "vta-" + uuid.NewString()[:8]
	user := mustCreateUser(t, client, &service.User{Email: prefix + "@example.com", Username: prefix + "-用户", Balance: 100})
	group := mustCreateGroup(t, client, &service.Group{Name: prefix + "-group"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, GroupID: &group.ID, Key: "sk-" + prefix, Name: prefix + "-key"})
	otherKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-other-" + prefix, Name: "other"})
	account := mustCreateAccount(t, client, &service.Account{Name: prefix + "-账号", Platform: service.PlatformOpenAI})
	otherAccount := mustCreateAccount(t, client, &service.Account{Name: prefix + "-other"})
	otherUser := mustCreateUser(t, client, &service.User{Email: prefix + "-other@example.com"})
	logs := NewUsageLogRepository(client, integrationDB)
	newLog := func(taskID string, userID, keyID, accountID int64, cost float64) *service.UsageLog {
		t.Helper()
		l := &service.UsageLog{RequestID: "grok-video:" + taskID, UserID: userID, APIKeyID: keyID, AccountID: accountID, Model: "video", ActualCost: cost, TotalCost: cost, CreatedAt: time.Now()}
		_, err := logs.Create(ctx, l)
		require.NoError(t, err)
		return l
	}
	type scenario struct {
		name, state, reason, display, billing string
		age                                   int
		logged                                bool
	}
	cases := []scenario{
		{"active", "pending", "", "in_progress", "not_charged", 0, false},
		{"late", "pending", "", "delayed", "not_charged", 3, false},
		{"retry", "pending", "poll_timeout", "query_retry", "not_charged", 3, false},
		{"ready", "ready", "billing_failed", "billing_pending", "pending", 0, false},
		{"ready-charged", "ready", "billing_failed", "billing_pending", "charged", 0, true},
		{"done", "completed", "", "completed", "charged", 0, true},
		{"done-missing", "completed", "", "completed", "review", 0, false},
		{"failed", "failed", "upstream_failed", "failed", "not_charged", 0, false},
		{"failed-charged", "failed", "upstream_cancelled", "failed", "review", 0, true},
		{"unsafe", "pending", "https://secret.invalid/token?prompt=hidden", "query_retry", "not_charged", 3, false},
		{"expired", "pending", "upstream_expired", "query_retry", "not_charged", 3, false},
		{"quote-null", "pending", "", "in_progress", "not_charged", 0, false},
		{"quote-zero", "ready", "", "billing_pending", "charged", 0, true},
		{"old", "pending", "", "delayed", "not_charged", 8 * 24, false},
		{"ancient", "pending", "", "delayed", "not_charged", 31 * 24, false},
	}
	for _, tc := range cases {
		task := &service.VideoTask{TaskID: prefix + "-" + tc.name, UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, GroupID: &group.ID,
			RoutingFingerprint: strings.Repeat("a", 64), Pending: service.GrokVideoPendingBilling{Model: prefix + "-模型%_\\", VideoDurationSeconds: 5, VideoResolution: "720p"},
			CostSnapshot: &service.VideoTaskCostSnapshot{Version: 1, Cost: service.CostBreakdown{ActualCost: 0.25}},
		}
		require.NoError(t, r.CreateVideoTask(ctx, task))
		_, err := integrationDB.ExecContext(ctx, `UPDATE video_tasks SET state=$2::varchar,last_error=$3,
			created_at=NOW()-$4::int*INTERVAL '1 hour', completion='{}',
			completed_at=CASE WHEN $2::varchar IN ('completed','failed') THEN NOW() END
			WHERE task_id=$1`, task.TaskID, tc.state, tc.reason, tc.age)
		require.NoError(t, err)
		if tc.name == "quote-null" {
			// Missing is not a free quote; also reject a string that resembles money.
			_, err = integrationDB.ExecContext(ctx, `UPDATE video_tasks SET cost_snapshot='{"cost":{"ActualCost":"0.9"}}',pending='{}' WHERE task_id=$1`, task.TaskID)
			require.NoError(t, err)
		}
		if tc.name == "quote-zero" {
			_, err = integrationDB.ExecContext(ctx, `UPDATE video_tasks SET cost_snapshot='{"cost":{"ActualCost":0}}' WHERE task_id=$1`, task.TaskID)
			require.NoError(t, err)
		}
		if tc.logged {
			cost := 0.4
			if tc.name == "quote-zero" {
				cost = 0
			}
			newLog(task.TaskID, user.ID, key.ID, account.ID, cost)
		}
	}
	// Each identity predicate must match, as must the entire prefixed task ID.
	newLog(prefix+"-active", user.ID, otherKey.ID, account.ID, 9)
	newLog(prefix+"-late", user.ID, key.ID, otherAccount.ID, 9)
	newLog(prefix+"-retry", otherUser.ID, key.ID, account.ID, 9)
	newLog(prefix+"-ready-extra", user.ID, key.ID, account.ID, 9)
	for i, hash := range []string{"private-key-hash-a", "private-key-hash-b", "resolved"} {
		taskID := ""
		if i == 2 {
			taskID = prefix + "-active"
		}
		_, err := integrationDB.ExecContext(ctx, `INSERT INTO video_submissions(api_key_id,key_hash,payload_hash,task_id) VALUES($1,$2,'private-payload',$3)`, key.ID, hash, taskID)
		require.NoError(t, err)
	}
	// Soft deletions must retain historical identity and must not drop rows.
	require.NoError(t, client.APIKey.DeleteOneID(key.ID).Exec(ctx))
	require.NoError(t, client.User.DeleteOneID(user.ID).Exec(ctx))
	f := service.VideoTaskAdminFilter{Page: 1, PageSize: 100, Days: 7, Search: prefix}
	items, total, err := r.ListVideoTasks(ctx, f)
	require.NoError(t, err)
	require.EqualValues(t, len(cases), total, "13 recent tasks plus two unresolved submissions")
	require.Len(t, items, int(total))
	byID := map[string]service.VideoTaskAdminItem{}
	for _, item := range items {
		require.Equal(t, user.ID, *item.UserID)
		require.Equal(t, user.Email, item.UserEmail)
		require.Equal(t, key.Name, item.APIKeyName)
		if item.Kind == "submission" {
			require.Nil(t, item.Model)
			require.Nil(t, item.AccountID)
			require.Nil(t, item.GroupID, "do not infer submission group from current key")
			require.Nil(t, item.ExpectedCost)
			require.Nil(t, item.ChargedCost)
			require.Nil(t, item.UsageLogID)
			require.Nil(t, item.NextAttemptAt)
			require.Equal(t, item.CreatedAt, item.UpdatedAt)
			require.Equal(t, "review", item.BillingStatus)
		} else {
			byID[item.TaskID] = item
		}
	}
	for _, tc := range cases[:len(cases)-2] {
		item := byID[prefix+"-"+tc.name]
		require.Equal(t, tc.display, item.DisplayStatus, tc.name)
		require.Equal(t, tc.billing, item.BillingStatus, tc.name)
		if tc.name == "quote-null" {
			require.Nil(t, item.ExpectedCost)
			require.Nil(t, item.VideoDurationSeconds)
		} else {
			expected := 0.25
			if tc.name == "quote-zero" {
				expected = 0
			}
			require.Equal(t, expected, *item.ExpectedCost, "uses real cost.ActualCost JSON casing")
			require.Equal(t, 5, *item.VideoDurationSeconds)
			require.Equal(t, "720p", item.VideoResolution)
		}
		if tc.logged {
			require.NotNil(t, item.UsageLogID)
			charged := 0.4
			if tc.name == "quote-zero" {
				charged = 0
			}
			require.Equal(t, charged, *item.ChargedCost)
		} else {
			require.Nil(t, item.UsageLogID, "mismatched logs must not imply a charge")
			require.Nil(t, item.ChargedCost)
		}
		if tc.state == "completed" || tc.state == "failed" {
			require.Nil(t, item.NextAttemptAt)
			require.NotNil(t, item.CompletedAt)
		} else {
			require.NotNil(t, item.NextAttemptAt)
		}
		reason := tc.reason
		if tc.name == "unsafe" {
			reason = ""
		}
		require.Equal(t, reason, item.ReasonCode)
	}
	body, err := json.Marshal(items)
	require.NoError(t, err)
	for _, secret := range []string{"private-key-hash", "private-payload", "secret.invalid", "cost_snapshot", "routing_fingerprint", "sort_key"} {
		require.NotContains(t, string(body), secret)
	}
	for _, status := range strings.Fields("in_progress delayed query_retry billing_pending completed failed submission_unknown") {
		filter := f
		filter.Status = status
		got, count, err := r.ListVideoTasks(ctx, filter)
		require.NoError(t, err)
		want := 0
		for _, item := range items {
			if item.DisplayStatus == status {
				want++
			}
		}
		require.EqualValues(t, want, count)
		require.Len(t, got, want)
	}
	for _, search := range []string{prefix + "-模型%_\\", prefix + "-账号", strings.ToUpper(user.Email), prefix + "-用户"} {
		filter := f
		filter.Search = search
		got, _, err := r.ListVideoTasks(ctx, filter)
		require.NoError(t, err)
		require.NotEmpty(t, got, search)
	}
	for _, search := range []string{prefix + "-模型%_x", `%' OR TRUE --`} {
		filter := f
		filter.Search = search
		got, count, err := r.ListVideoTasks(ctx, filter)
		require.NoError(t, err)
		require.Zero(t, count)
		require.Empty(t, got)
	}
	filter := f
	filter.Days = 30
	_, count, err := r.ListVideoTasks(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, total+1, count)
	filter = f
	filter.Page = 999
	got, count, err := r.ListVideoTasks(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, total, count)
	require.Empty(t, got)
	// Stable order across page boundaries, including same-key submissions.
	var paged []service.VideoTaskAdminItem
	for page := 1; page <= (int(total)+1)/2; page++ {
		filter = f
		filter.PageSize = 2
		filter.Page = page
		got, count, err := r.ListVideoTasks(ctx, filter)
		require.NoError(t, err)
		require.Equal(t, total, count)
		paged = append(paged, got...)
	}
	require.Equal(t, items, paged)

	t.Run("historical duplicate usage cannot multiply rows", func(t *testing.T) {
		tx, err := integrationDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer tx.Rollback()
		// Session-local shadow retains actual column types/defaults but omits
		// unique indexes, simulating a historical/partitioned duplicate safely.
		_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE usage_logs (LIKE public.usage_logs INCLUDING DEFAULTS) ON COMMIT DROP`)
		require.NoError(t, err)
		for _, cost := range []float64{0.4, 9} {
			_, err = tx.ExecContext(ctx, `INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,actual_cost) VALUES($1,$2,$3,$4,'video',$5)`, user.ID, key.ID, account.ID, "grok-video:"+prefix+"-ready-charged", cost)
			require.NoError(t, err)
		}
		var payload []byte
		var count int64
		err = tx.QueryRowContext(ctx, videoTaskAdminQuery, 7, prefix+"-ready-charged", "", 100, int64(0)).Scan(&count, &payload)
		require.NoError(t, err)
		var got []service.VideoTaskAdminItem
		require.NoError(t, json.Unmarshal(payload, &got))
		require.EqualValues(t, 1, count)
		require.Len(t, got, 1)
		require.Equal(t, 0.4, *got[0].ChargedCost, "first exact row, never sum duplicates")
	})

	t.Run("physically deleted submission key", func(t *testing.T) {
		deletedKey := mustCreateApiKey(t, client, &service.APIKey{UserID: otherUser.ID, Key: "sk-gone-" + prefix, Name: "gone"})
		_, err := integrationDB.ExecContext(ctx, `INSERT INTO video_submissions(api_key_id,key_hash,payload_hash) VALUES($1,'secret-gone','secret-gone')`, deletedKey.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM api_keys WHERE id=$1`, deletedKey.ID)
		require.NoError(t, err)
		got, _, err := r.ListVideoTasks(ctx, service.VideoTaskAdminFilter{Page: 1, PageSize: 100, Days: 1, Status: "submission_unknown"})
		require.NoError(t, err)
		found := false
		for _, item := range got {
			if item.APIKeyID != deletedKey.ID {
				continue
			}
			found = true
			require.Nil(t, item.UserID)
			require.Empty(t, item.UserEmail)
			require.Empty(t, item.APIKeyName)
			require.Nil(t, item.Model)
			require.Nil(t, item.AccountID)
			require.Nil(t, item.ChargedCost)
			require.Equal(t, "review", item.BillingStatus)
		}
		require.True(t, found, "missing identity must not suppress unresolved submission")
	})
}
