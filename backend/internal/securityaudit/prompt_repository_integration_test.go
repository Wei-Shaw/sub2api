package securityaudit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

const promptAuditPostgresTestEnv = "PROMPT_AUDIT_TEST_POSTGRES_DSN"

func openPromptAuditIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(promptAuditPostgresTestEnv))
	if dsn == "" {
		t.Skip(promptAuditPostgresTestEnv + " is not set")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(16)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS users (id BIGSERIAL PRIMARY KEY);
		CREATE TABLE IF NOT EXISTS groups (id BIGSERIAL PRIMARY KEY);
		CREATE TABLE IF NOT EXISTS api_keys (id BIGSERIAL PRIMARY KEY);
		CREATE TABLE IF NOT EXISTS settings (
			key VARCHAR(255) PRIMARY KEY,
			value TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	require.NoError(t, err)
	for _, name := range []string{"181_prompt_audit.sql", "182_prompt_audit_full_prompt.sql"} {
		migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		require.NoError(t, err)
		// The migration runner can retry an interrupted deployment; the migration
		// must therefore be safe to execute more than once.
		_, err = db.ExecContext(ctx, string(migration))
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, string(migration))
		require.NoError(t, err)
	}
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	resetPromptAuditIntegrationDB(t, db)
	return db
}

func resetPromptAuditIntegrationDB(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`TRUNCATE TABLE prompt_audit_events, prompt_audit_jobs, api_keys, users, groups, settings RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
}

func insertIdentity(t *testing.T, db *sql.DB, table string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.QueryRow(`INSERT INTO `+table+` DEFAULT VALUES RETURNING id`).Scan(&id))
	return id
}

func integrationSnapshot(seed string) PromptSnapshot {
	return PromptSnapshot{
		RequestID: "request-" + seed, UsernameSnapshot: "user-" + seed,
		UserEmailSnapshot: "user-" + seed + "@example.test", APIKeyNameSnapshot: "key-" + seed,
		GroupName: "group-" + seed, Provider: "openai", Endpoint: "/v1/chat/completions",
		Protocol: "openai_chat", Model: "gpt-test", PromptHash: strings.Repeat(seed[:1], 64),
		RedactedPreview: "redacted-" + seed, PromptLength: len([]rune(seed)), MessageCount: 1,
	}
}

func integrationResult(decision EventDecision) *NormalizedResult {
	result := &NormalizedResult{
		Decision: decision, RiskLevel: RiskLow, Action: ActionAllow, Safety: "Safe",
		Categories: []string{}, MatchedScanners: []string{}, ScannerScores: map[string]float64{},
		ScannerEvidence: map[string]string{}, ScannerBackend: "qwen3guard-openai",
		ScannerVersion: "test", GuardEndpointID: "guard-1", PolicyID: "priority",
		PolicyVersion: 1, ChunkTotal: 1, LatencyMS: 2,
	}
	if decision != EventPass {
		result.RiskLevel = RiskCritical
		result.Action = ActionBlock
		result.Safety = "Unsafe"
		result.Categories = []string{"pii"}
		result.MatchedScanners = []string{"pii"}
		result.ScannerScores["pii"] = 1
		result.ScannerEvidence["pii"] = "redacted evidence"
	}
	return result
}

func TestPromptAuditMigrationSchemaAndLeakageGate(t *testing.T) {
	db := openPromptAuditIntegrationDB(t)
	ctx := context.Background()

	rows, err := db.QueryContext(ctx, `SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema='public' AND table_name IN ('prompt_audit_jobs','prompt_audit_events')`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	forbidden := []string{"raw_prompt", "raw_request", "payload", "token", "authorization", "credential", "ciphertext"}
	for rows.Next() {
		var tableName, columnName string
		require.NoError(t, rows.Scan(&tableName, &columnName))
		lower := strings.ToLower(columnName)
		for _, word := range forbidden {
			require.NotContainsf(t, lower, word, "%s.%s is a forbidden raw/credential column", tableName, columnName)
		}
	}
	require.NoError(t, rows.Err())

	indexRows, err := db.QueryContext(ctx, `SELECT indexname FROM pg_indexes
		WHERE schemaname='public' AND tablename IN ('prompt_audit_jobs','prompt_audit_events')`)
	require.NoError(t, err)
	defer func() { _ = indexRows.Close() }()
	indexes := map[string]bool{}
	for indexRows.Next() {
		var name string
		require.NoError(t, indexRows.Scan(&name))
		indexes[name] = true
	}
	for _, name := range []string{
		"idx_prompt_audit_jobs_schedule", "idx_prompt_audit_jobs_request", "idx_prompt_audit_jobs_user_created",
		"idx_prompt_audit_jobs_api_key_created", "idx_prompt_audit_jobs_group_created", "idx_prompt_audit_jobs_prompt_hash",
		"idx_prompt_audit_jobs_created", "idx_prompt_audit_events_job", "idx_prompt_audit_events_request",
		"idx_prompt_audit_events_decision_created", "idx_prompt_audit_events_risk_created",
		"idx_prompt_audit_events_user_created", "idx_prompt_audit_events_api_key_created",
		"idx_prompt_audit_events_group_created", "idx_prompt_audit_events_prompt_hash", "idx_prompt_audit_events_created",
	} {
		require.Truef(t, indexes[name], "missing index %s", name)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO prompt_audit_jobs(status) VALUES ('unknown')`)
	require.Error(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO prompt_audit_jobs(prompt_length) VALUES (-1)`)
	require.Error(t, err)
	var jobID int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO prompt_audit_jobs DEFAULT VALUES RETURNING id`).Scan(&jobID))
	_, err = db.ExecContext(ctx, `INSERT INTO prompt_audit_events(job_id,chunk_total) VALUES ($1,-1)`, jobID)
	require.Error(t, err)
}

func TestPromptAuditDatabasePersistsFullPromptOnEventsOnly(t *testing.T) {
	db := openPromptAuditIntegrationDB(t)
	repo := NewPostgreSQLRepository(db)
	ctx := context.Background()
	const promptCanary = "PROMPT_AUDIT_CANARY_SECRET_DO_NOT_PERSIST"
	request := Request{
		RequestID: "canary-request", Provider: "openai",
		Endpoint: "/v1/chat/completions", Protocol: "openai_chat", Model: "gpt-test", Stage: "http",
		Body: []byte(`{"messages":[{"role":"user","content":"` + promptCanary + `"}]}`),
	}
	snapshot, err := ExtractPromptSnapshot(request)
	require.NoError(t, err)
	require.NotContains(t, snapshot.RedactedPreview, promptCanary)
	require.Contains(t, snapshot.FullPrompt, promptCanary)
	event, err := repo.RecordBlocking(ctx, snapshot.Redacted(), 1, integrationResult(EventCritical), true)
	require.NoError(t, err)
	// The event intentionally retains the full prompt for admin review; the
	// redacted preview and transient job row still never contain it.
	adminJSON, err := json.Marshal(event)
	require.NoError(t, err)
	require.Contains(t, string(adminJSON), promptCanary)
	require.NotContains(t, event.Snapshot.RedactedPreview, promptCanary)

	var storedFullPrompt string
	require.NoError(t, db.QueryRow(`SELECT full_prompt FROM prompt_audit_events WHERE id=$1`, event.ID).Scan(&storedFullPrompt))
	require.Contains(t, storedFullPrompt, promptCanary)

	detail, err := repo.GetEvent(ctx, event.ID)
	require.NoError(t, err)
	require.Contains(t, detail.Snapshot.FullPrompt, promptCanary)

	var jobJSON string
	require.NoError(t, db.QueryRow(`SELECT row_to_json(j)::text FROM prompt_audit_jobs j WHERE id=$1`, event.JobID).Scan(&jobJSON))
	require.NotContains(t, jobJSON, promptCanary)

	failedJob, err := repo.CreateStagingWithCapacity(ctx, integrationSnapshot("error"), 1, 3, 10)
	require.NoError(t, err)
	const errorCanary = "GUARD_RAW_RESPONSE_CANARY_SECRET"
	require.NoError(t, repo.MarkStagingFailed(ctx, failedJob.ID, "payload_store_failed", "raw guard body: "+errorCanary))
	var code, message string
	require.NoError(t, db.QueryRow(`SELECT last_error_code,last_error_message FROM prompt_audit_jobs WHERE id=$1`, failedJob.ID).Scan(&code, &message))
	require.Equal(t, "payload_store_failed", code)
	require.Equal(t, stableErrorMessage(code), message)
	require.NotContains(t, message, errorCanary)
	require.LessOrEqual(t, len([]rune(message)), 160)
}

func TestPromptAuditRepositoryAdmissionClaimFencingAndEventTransaction(t *testing.T) {
	db := openPromptAuditIntegrationDB(t)
	repo := NewPostgreSQLRepository(db)
	ctx := context.Background()

	start := make(chan struct{})
	type admissionResult struct {
		job *Job
		err error
	}
	results := make(chan admissionResult, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			job, err := repo.CreateStagingWithCapacity(ctx, integrationSnapshot(string(rune('a'+index))), 1, 3, 1)
			results <- admissionResult{job: job, err: err}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	var accepted *Job
	rejected := 0
	for result := range results {
		if result.err == nil {
			require.Nil(t, accepted)
			accepted = result.job
			continue
		}
		require.True(t, errors.Is(result.err, ErrQueueFull) || errors.Is(result.err, ErrQueueAdmissionBusy))
		rejected++
	}
	require.NotNil(t, accepted)
	require.Equal(t, 1, rejected)
	stats, err := repo.QueueStats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.Active)
	require.NoError(t, repo.PublishQueued(ctx, accepted.ID))

	claimStart := make(chan struct{})
	claims := make(chan *Job, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-claimStart
			job, claimed, claimErr := repo.ClaimNextJob(ctx, time.Now().Add(time.Second))
			require.NoError(t, claimErr)
			if claimed {
				claims <- job
			}
		}()
	}
	close(claimStart)
	wg.Wait()
	close(claims)
	claimedJobs := make([]*Job, 0, 1)
	for job := range claims {
		claimedJobs = append(claimedJobs, job)
	}
	require.Len(t, claimedJobs, 1)
	firstClaim := claimedJobs[0]
	require.Equal(t, int64(1), firstClaim.ClaimVersion)

	reclaimed, err := repo.ReclaimStale(ctx, time.Now().Add(time.Hour), time.Now().Add(time.Hour), 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), reclaimed)
	secondClaim, claimed, err := repo.ClaimNextJob(ctx, time.Now().Add(time.Second))
	require.NoError(t, err)
	require.True(t, claimed)
	require.Greater(t, secondClaim.ClaimVersion, firstClaim.ClaimVersion)
	require.ErrorIs(t, repo.RefreshLease(ctx, firstClaim.ID, firstClaim.ClaimVersion, time.Now()), ErrLeaseLost)
	_, err = repo.Complete(ctx, firstClaim, integrationResult(EventCritical), true)
	require.ErrorIs(t, err, ErrLeaseLost)

	event, err := repo.Complete(ctx, secondClaim, integrationResult(EventCritical), true)
	require.NoError(t, err)
	require.NotNil(t, event)
	var status string
	var eventCount int
	require.NoError(t, db.QueryRow(`SELECT status FROM prompt_audit_jobs WHERE id=$1`, secondClaim.ID).Scan(&status))
	require.Equal(t, "done", status)
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM prompt_audit_events WHERE job_id=$1`, secondClaim.ID).Scan(&eventCount))
	require.Equal(t, 1, eventCount)

	staging, err := repo.CreateStagingWithCapacity(ctx, integrationSnapshot("stale"), 1, 3, 10)
	require.NoError(t, err)
	reclaimed, err = repo.ReclaimStale(ctx, time.Now().Add(time.Hour), time.Now().Add(time.Hour), 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), reclaimed)
	require.NoError(t, db.QueryRow(`SELECT status FROM prompt_audit_jobs WHERE id=$1`, staging.ID).Scan(&status))
	require.Equal(t, "failed", status)
}

func TestPromptAuditRepositoryForeignKeysFiltersAndStableIdentitySnapshots(t *testing.T) {
	db := openPromptAuditIntegrationDB(t)
	repo := NewPostgreSQLRepository(db)
	ctx := context.Background()
	userID := insertIdentity(t, db, "users")
	apiKeyID := insertIdentity(t, db, "api_keys")
	groupID := insertIdentity(t, db, "groups")
	snapshot := integrationSnapshot("identity")
	snapshot.UserID, snapshot.APIKeyID, snapshot.GroupID = userID, apiKeyID, &groupID
	event, err := repo.RecordBlocking(ctx, snapshot, 7, integrationResult(EventCritical), true)
	require.NoError(t, err)
	require.NotNil(t, event)

	start, end := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	page, err := repo.ListEvents(ctx, EventFilter{
		Decision: string(EventCritical), RiskLevel: string(RiskCritical), Endpoint: snapshot.Endpoint,
		GroupID: &groupID, UserID: &userID, APIKeyID: &apiKeyID, RequestID: snapshot.RequestID,
		PromptHash: snapshot.PromptHash, Keyword: snapshot.UsernameSnapshot, StartAt: &start, EndAt: &end,
	}, 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), page.Total)
	require.Len(t, page.Items, 1)
	require.NotEmpty(t, page.Items[0].IssueSummaries)
	require.Equal(t, snapshot.UsernameSnapshot, page.Items[0].Snapshot.UsernameSnapshot)
	require.Equal(t, snapshot.UserEmailSnapshot, page.Items[0].Snapshot.UserEmailSnapshot)
	require.Equal(t, snapshot.APIKeyNameSnapshot, page.Items[0].Snapshot.APIKeyNameSnapshot)

	_, err = db.Exec(`DELETE FROM users WHERE id=$1`, userID)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM api_keys WHERE id=$1`, apiKeyID)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM groups WHERE id=$1`, groupID)
	require.NoError(t, err)
	stored, err := repo.GetEvent(ctx, event.ID)
	require.NoError(t, err)
	require.Zero(t, stored.Snapshot.UserID)
	require.Zero(t, stored.Snapshot.APIKeyID)
	require.Nil(t, stored.Snapshot.GroupID)
	require.Equal(t, snapshot.UsernameSnapshot, stored.Snapshot.UsernameSnapshot)
	require.Equal(t, snapshot.UserEmailSnapshot, stored.Snapshot.UserEmailSnapshot)
	require.Equal(t, snapshot.APIKeyNameSnapshot, stored.Snapshot.APIKeyNameSnapshot)

	_, err = db.Exec(`DELETE FROM prompt_audit_jobs WHERE id=$1`, event.JobID)
	require.NoError(t, err)
	_, err = repo.GetEvent(ctx, event.ID)
	require.ErrorIs(t, err, ErrEventNotFound)
}

func TestPromptAuditRepositoryHighWaterAndSafeDeletion(t *testing.T) {
	db := openPromptAuditIntegrationDB(t)
	repo := NewPostgreSQLRepository(db)
	ctx := context.Background()
	first, err := repo.RecordBlocking(ctx, integrationSnapshot("first"), 1, integrationResult(EventCritical), true)
	require.NoError(t, err)
	second, err := repo.RecordBlocking(ctx, integrationSnapshot("second"), 1, integrationResult(EventCritical), true)
	require.NoError(t, err)
	start, end := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	filter := EventFilter{Decision: string(EventCritical), StartAt: &start, EndAt: &end}
	preview, err := repo.PreviewDelete(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, int64(2), preview.MatchedCount)
	require.Equal(t, second.ID, preview.SnapshotMaxID)
	require.Equal(t, FilterHash(preview.FilterSummary, preview.SnapshotMaxID), preview.FilterHash)

	newer, err := repo.RecordBlocking(ctx, integrationSnapshot("newer"), 1, integrationResult(EventCritical), true)
	require.NoError(t, err)
	result, err := repo.DeleteEventsByFilter(ctx, filter, preview.SnapshotMaxID, 1)
	require.NoError(t, err)
	require.Equal(t, int64(2), result.DeletedEvents)
	require.Equal(t, int64(2), result.DeletedJobs)
	_, err = repo.GetEvent(ctx, first.ID)
	require.ErrorIs(t, err, ErrEventNotFound)
	_, err = repo.GetEvent(ctx, second.ID)
	require.ErrorIs(t, err, ErrEventNotFound)
	_, err = repo.GetEvent(ctx, newer.ID)
	require.NoError(t, err, "an event created after preview must survive high-water deletion")

	processingEvent, err := repo.RecordBlocking(ctx, integrationSnapshot("processing"), 1, integrationResult(EventCritical), true)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE prompt_audit_jobs SET status='processing' WHERE id=$1`, processingEvent.JobID)
	require.NoError(t, err)
	deleteResult, err := repo.DeleteEvent(ctx, processingEvent.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleteResult.DeletedEvents)
	require.Zero(t, deleteResult.DeletedJobs)
	var remaining int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM prompt_audit_jobs WHERE id=$1`, processingEvent.JobID).Scan(&remaining))
	require.Equal(t, 1, remaining, "processing jobs must not be deleted as orphans")

	batchOne, err := repo.RecordBlocking(ctx, integrationSnapshot("batch-one"), 1, integrationResult(EventCritical), true)
	require.NoError(t, err)
	batchTwo, err := repo.RecordBlocking(ctx, integrationSnapshot("batch-two"), 1, integrationResult(EventCritical), true)
	require.NoError(t, err)
	ids := []int64{batchTwo.ID, batchOne.ID, batchOne.ID}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	batchResult, err := repo.DeleteEventsByIDs(ctx, ids)
	require.NoError(t, err)
	require.Equal(t, int64(2), batchResult.DeletedEvents)
}

func TestPromptAuditServiceConfirmationKeepsPostPreviewEventsAndConcurrentDeletesAreSafe(t *testing.T) {
	db := openPromptAuditIntegrationDB(t)
	repo := NewPostgreSQLRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	start, end := now.Add(-time.Hour), now.Add(time.Hour)
	filter := EventFilter{Decision: string(EventCritical), StartAt: &start, EndAt: &end}

	for i := 0; i < 12; i++ {
		_, err := repo.RecordBlocking(ctx, integrationSnapshot(fmt.Sprintf("event-%02d", i)), 1, integrationResult(EventCritical), true)
		require.NoError(t, err)
	}
	service := &PromptService{
		config: &fakeConfigStore{}, repo: repo, payload: NewRedisPayloadStore(nil), clock: fixedClock{now: now},
	}
	preview, err := service.PreviewDelete(ctx, filter, 77)
	require.NoError(t, err)
	require.Equal(t, int64(12), preview.MatchedCount)

	newer, err := repo.RecordBlocking(ctx, integrationSnapshot("post-preview"), 1, integrationResult(EventCritical), true)
	require.NoError(t, err)
	result, err := service.DeleteByFilter(ctx, DeleteByFilterRequest{
		Filter: filter, SnapshotMaxID: preview.SnapshotMaxID, FilterHash: preview.FilterHash,
		ConfirmationToken: preview.ConfirmationToken, Confirm: true,
	}, 77)
	require.NoError(t, err)
	require.Equal(t, int64(12), result.DeletedEvents)
	_, err = repo.GetEvent(ctx, newer.ID)
	require.NoError(t, err, "events created after delete-preview must survive")

	resetPromptAuditIntegrationDB(t, db)
	for i := 0; i < 24; i++ {
		_, err := repo.RecordBlocking(ctx, integrationSnapshot(fmt.Sprintf("race-%02d", i)), 1, integrationResult(EventCritical), true)
		require.NoError(t, err)
	}
	preview, err = repo.PreviewDelete(ctx, filter)
	require.NoError(t, err)

	type deleteOutcome struct {
		result *DeleteResult
		err    error
	}
	outcomes := make(chan deleteOutcome, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deleted, deleteErr := repo.DeleteEventsByFilter(ctx, filter, preview.SnapshotMaxID, 1)
			outcomes <- deleteOutcome{result: deleted, err: deleteErr}
		}()
	}
	wg.Wait()
	close(outcomes)
	var deletedTotal int64
	for outcome := range outcomes {
		require.NoError(t, outcome.err)
		require.NotNil(t, outcome.result)
		deletedTotal += outcome.result.DeletedEvents
	}
	require.Equal(t, int64(24), deletedTotal, "concurrent deleters must neither double-count nor strand matching events")
	remaining, err := repo.ListEvents(ctx, filter, 1, 100)
	require.NoError(t, err)
	require.Zero(t, remaining.Total)
}
