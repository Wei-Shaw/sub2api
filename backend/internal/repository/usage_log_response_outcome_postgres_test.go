//go:build integration

package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/usagelog"
	"github.com/Wei-Shaw/sub2api/internal/service"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// The standard integration harness owns this disposable PostgreSQL database.
func TestResponseOutcomePostgresPersistenceAndAccounting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db := integrationDB
	// A legacy-shaped insert leaves the additive outcome column NULL.
	// Synthetic fixtures use no upstream credentials.
	var userID, keyID, secondKeyID, accountID, groupID int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO users(email,password_hash) VALUES ('delivery-timeout-fixture@example.invalid','fixture-only') RETURNING id`).Scan(&userID))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO groups(name,platform) VALUES ('delivery-timeout-fixture','openai') RETURNING id`).Scan(&groupID))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO accounts(name,platform,type) VALUES ('delivery-timeout-fixture','openai','apikey') RETURNING id`).Scan(&accountID))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO api_keys(user_id,key,name,group_id) VALUES ($1,'delivery-fixture-key-one','fixture-one',$2) RETURNING id`, userID, groupID).Scan(&keyID))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO api_keys(user_id,key,name,group_id) VALUES ($1,'delivery-fixture-key-two','fixture-two',$2) RETURNING id`, userID, groupID).Scan(&secondKeyID))
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM ops_error_logs WHERE user_id=$1`, userID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id=$1`, userID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM accounts WHERE id=$1`, accountID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM groups WHERE id=$1`, groupID)
	})
	at := time.Now().UTC().Add(-time.Minute)
	var legacyID int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO usage_logs(user_id,api_key_id,account_id,group_id,request_id,model,input_tokens,output_tokens,total_cost,actual_cost,created_at) VALUES($1,$2,$3,$4,'client:legacy-fixture','delivery-fixture-gpt',7,3,0.01,0.01,$5) RETURNING id`, userID, keyID, accountID, groupID, at).Scan(&legacyID))
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	repo := newUsageLogRepositoryWithSQL(client, db)
	ops := &opsRepository{db: db}
	legacy, err := repo.GetByID(ctx, legacyID)
	require.NoError(t, err)
	require.Nil(t, legacy.ResponseOutcome)
	entLegacy, err := client.UsageLog.Get(ctx, legacyID)
	require.NoError(t, err)
	require.Nil(t, entLegacy.ResponseOutcome)
	// The explicit old column list and old insert shape still work after upgrade.
	oldColumns := strings.Replace(usageLogSelectColumns, ", response_outcome", "", 1)
	rows, err := db.QueryContext(ctx, "SELECT "+oldColumns+" FROM usage_logs WHERE id=$1", legacyID)
	require.NoError(t, err)
	columns, err := rows.Columns()
	require.NoError(t, err)
	require.Len(t, columns, len(strings.Split(usageLogSelectColumns, ","))-1)
	require.True(t, rows.Next())
	oldValues := make([]any, len(columns))
	targets := make([]any, len(columns))
	for i := range oldValues {
		targets[i] = &oldValues[i]
	}
	require.NoError(t, rows.Scan(targets...))
	require.NoError(t, rows.Close())

	failedAt := at.Add(10 * time.Second)
	makeLog := func(request string, outcome service.ResponseOutcome) *service.UsageLog {
		createdAt := at
		if outcome != service.ResponseOutcomeWritten {
			createdAt = failedAt
		}
		return &service.UsageLog{UserID: userID, APIKeyID: keyID, AccountID: accountID, GroupID: &groupID, RequestID: "client:" + request, Model: "delivery-fixture-gpt", RequestedModel: "delivery-fixture-gpt", InputTokens: 20, OutputTokens: 10, TotalCost: 0.03, ActualCost: 0.03, RateMultiplier: 1, RequestType: service.RequestTypeSync, CreatedAt: createdAt, ResponseOutcome: service.ResponseOutcomePtr(outcome)}
	}
	failed := makeLog("failed-fixture", service.ResponseOutcomeUpstreamFailed)
	inserted, err := repo.createSingle(ctx, db, failed)
	require.NoError(t, err)
	require.True(t, inserted)
	duplicate := *failed
	duplicate.TotalCost = 99
	duplicate.ActualCost = 99
	duplicate.ResponseOutcome = service.ResponseOutcomePtr(service.ResponseOutcomeWritten)
	inserted, err = repo.createSingle(ctx, db, &duplicate)
	require.NoError(t, err)
	require.False(t, inserted)
	stored, err := repo.GetByID(ctx, failed.ID)
	require.NoError(t, err)
	require.Equal(t, "upstream_failed", *stored.ResponseOutcome)
	require.InDelta(t, 0.03, stored.ActualCost, 1e-9)
	entFailed, err := client.UsageLog.Get(ctx, failed.ID)
	require.NoError(t, err)
	require.Equal(t, "upstream_failed", *entFailed.ResponseOutcome)
	_, err = client.UsageLog.Query().Where(usagelog.ResponseOutcomeEQ("upstream_failed")).All(ctx)
	require.NoError(t, err)
	// Guard new values, without backfilling or inventing old outcomes.
	_, err = db.ExecContext(ctx, "UPDATE usage_logs SET response_outcome='invalid' WHERE id=$1", failed.ID)
	require.Error(t, err)

	cancelled := makeLog("cancelled-fixture", service.ResponseOutcomeClientCancelled)
	require.NoError(t, execUsageLogInsertNoResult(ctx, db, prepareUsageLogInsert(cancelled)))
	written := makeLog("written-fixture", service.ResponseOutcomeWritten)
	prepared := prepareUsageLogInsert(written)
	batchQuery, batchArgs := buildUsageLogBatchInsertQuery([]string{"written-fixture"}, map[string]usageLogInsertPrepared{"written-fixture": prepared})
	var batchResult []byte
	require.NoError(t, db.QueryRowContext(ctx, batchQuery, batchArgs...).Scan(&batchResult))
	require.Contains(t, string(batchResult), "written-fixture")
	writeFailed := makeLog("write-failed-fixture", service.ResponseOutcomeWriteFailed)
	bestQuery, bestArgs := buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepareUsageLogInsert(writeFailed)})
	_, err = db.ExecContext(ctx, bestQuery, bestArgs...)
	require.NoError(t, err)
	// Check all four persisted outcome paths through the native SELECT mapper.
	for _, want := range []string{"upstream_failed", "client_cancelled", "response_written", "write_failed"} {
		var id int64
		require.NoError(t, db.QueryRowContext(ctx, "SELECT id FROM usage_logs WHERE user_id=$1 AND response_outcome=$2", userID, want).Scan(&id))
		row, err := repo.GetByID(ctx, id)
		require.NoError(t, err)
		require.Equal(t, want, *row.ResponseOutcome)
	}
	start, end := at.Add(-time.Minute), at.Add(time.Minute)
	success, tokens, err := ops.queryUsageCounts(ctx, &service.OpsDashboardFilter{GroupID: &groupID}, start, end)
	require.NoError(t, err)
	require.EqualValues(t, 2, success)
	require.EqualValues(t, 130, tokens)
	failedStart, failedEnd := failedAt.Add(-time.Second), failedAt.Add(time.Second)
	failureSuccess, failureTokens, err := ops.queryUsageCounts(ctx, &service.OpsDashboardFilter{GroupID: &groupID}, failedStart, failedEnd)
	require.NoError(t, err)
	require.Zero(t, failureSuccess)
	require.EqualValues(t, 90, failureTokens)
	var cost float64
	require.NoError(t, db.QueryRowContext(ctx, "SELECT SUM(actual_cost) FROM usage_logs WHERE user_id=$1", userID).Scan(&cost))
	require.InDelta(t, 0.13, cost, 1e-9)
	// A matching ops error and failed consumption row represent one request.
	_, err = ops.InsertErrorLog(ctx, &service.OpsInsertErrorLogInput{RequestID: "external-correlated-id", ClientRequestID: "failed-fixture", UserID: &userID, APIKeyID: &keyID, AccountID: &accountID, GroupID: &groupID, Platform: "openai", Model: "delivery-fixture-gpt", StatusCode: 502, ErrorPhase: "upstream", ErrorType: "upstream_error", Severity: "error", ErrorMessage: "synthetic terminal failure", ErrorOwner: "provider", CreatedAt: at})
	require.NoError(t, err)
	filter := &service.OpsRequestDetailFilter{StartTime: &start, EndTime: &end, UserID: &userID, PageSize: 100}
	details, total, err := ops.ListRequestDetails(ctx, filter)
	require.NoError(t, err)
	require.EqualValues(t, 5, total)
	failures := 0
	for _, detail := range details {
		if detail.Kind == service.OpsRequestKindError {
			failures++
		}
	}
	require.Equal(t, 3, failures)
	// Identical internal ID on another API key cannot swallow its consumption.
	crossKey := makeLog("failed-fixture", service.ResponseOutcomeUpstreamFailed)
	crossKey.APIKeyID = secondKeyID
	inserted, err = repo.createSingle(ctx, db, crossKey)
	require.NoError(t, err)
	require.True(t, inserted)
	_, total, err = ops.ListRequestDetails(ctx, filter)
	require.NoError(t, err)
	require.EqualValues(t, 6, total)
}
