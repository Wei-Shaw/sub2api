package securityaudit

import (
	"context"
	"database/sql"
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The explicitly named test database must be disposable. No default DSN is
// inferred from application settings, so this cannot use a business database.
func TestRecordOptimizationPostgres(t *testing.T) {
	dsn := os.Getenv("PROMPT_RECORD_OPT_TEST_DSN")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL via PROMPT_RECORD_OPT_TEST_DSN")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var database string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT current_database()").Scan(&database))
	require.Equal(t, "prompt_record_optimization_test", database)
	var exists bool
	require.NoError(t, db.QueryRowContext(ctx, "SELECT to_regclass('public.prompt_records') IS NOT NULL").Scan(&exists))
	require.False(t, exists, "test requires a fresh disposable database")
	for _, name := range []string{"239_prompt_records.sql", "240_prompt_record_responses.sql", "241_prompt_record_requests.sql", "242_prompt_record_search_extension.sql", "243_prompt_record_search_cleanup_indexes_notx.sql", "244_prompt_record_session_id.sql"} {
		body, err := os.ReadFile(filepath.Join("../../migrations", name))
		require.NoError(t, err)
		if strings.HasSuffix(name, "_notx.sql") {
			for _, statement := range strings.Split(string(body), ";") {
				if strings.TrimSpace(statement) == "" {
					continue
				}
				_, err = db.ExecContext(ctx, statement)
				require.NoError(t, err)
			}
		} else {
			_, err = db.ExecContext(ctx, string(body))
			require.NoError(t, err)
		}
	}
	repo := NewPostgreSQLRepository(db)
	created := time.Now().UTC().Truncate(time.Microsecond)
	past, future := created.Add(-time.Hour), created.Add(time.Hour)
	for i := range 6 {
		record := &PromptRecord{SessionID: "session-" + strconv.Itoa(i), Stage: "http", PromptHash: strings.Repeat(strconv.Itoa(i), 64), Protocol: "responses", Model: "test-target-model", CreatedAt: created, RequestBody: `{"input":"canonical text"}`}
		if i == 0 {
			record.ExpiresAt = &past
		}
		if i == 1 {
			record.ExpiresAt = &future
		}
		_, err := repo.InsertPromptRecord(ctx, record)
		require.NoError(t, err)
	}
	seen := map[int64]bool{}
	filter := PromptRecordFilter{CursorMode: true, Model: "target"}
	wantTotal := int64(5)
	for {
		page, err := repo.ListPromptRecords(ctx, filter, 1, 2)
		require.NoError(t, err)
		require.Equal(t, wantTotal, page.Total)
		for _, item := range page.Items {
			require.False(t, seen[item.ID], "timestamp ties must not duplicate rows")
			seen[item.ID] = true
		}
		if !page.HasMore {
			break
		}
		decoded, err := base64.RawURLEncoding.DecodeString(page.NextCursor)
		require.NoError(t, err)
		parts := strings.Split(string(decoded), "|")
		at, err := time.Parse(time.RFC3339Nano, parts[0])
		require.NoError(t, err)
		id, err := strconv.ParseInt(parts[1], 10, 64)
		require.NoError(t, err)
		filter.CursorCreatedAt, filter.CursorID = &at, id
		// Insert above the cursor; later pages must stay stable.
		if len(seen) == 2 {
			_, err := repo.InsertPromptRecord(ctx, &PromptRecord{SessionID: "session-newer", Stage: "http", PromptHash: strings.Repeat("f", 64), Model: "test-target-model", CreatedAt: created.Add(time.Second)})
			require.NoError(t, err)
			wantTotal = 6
		}
	}
	require.Len(t, seen, 5)
	_, err = repo.GetPromptRecord(ctx, 1)
	require.ErrorIs(t, err, ErrPromptRecordNotFound)
	s := NewPromptRecordService(repo)
	detail, err := s.GetPromptRecord(ctx, 2)
	require.NoError(t, err)
	require.Equal(t, "canonical text", detail.PromptText)
	var storedText string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT prompt_text FROM prompt_records WHERE id=2").Scan(&storedText))
	require.Empty(t, storedText)
	// Legacy text stays readable without reconstructing or modifying it.
	_, err = db.ExecContext(ctx, "UPDATE prompt_records SET prompt_text='legacy text' WHERE id=2")
	require.NoError(t, err)
	detail, err = s.GetPromptRecord(ctx, 2)
	require.NoError(t, err)
	require.Equal(t, "legacy text", detail.PromptText)
	deleted, err := repo.DeleteExpiredPromptRecords(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	deleted, err = repo.DeleteExpiredPromptRecords(ctx, 1000)
	require.NoError(t, err)
	require.Zero(t, deleted)
	page, err := repo.ListPromptRecords(ctx, PromptRecordFilter{}, 1, 20)
	require.NoError(t, err)
	require.EqualValues(t, 6, page.Total)
	var indexes int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM pg_indexes WHERE tablename='prompt_records' AND indexname IN ('idx_prompt_records_model_trgm','idx_prompt_records_expires_at_id')`).Scan(&indexes))
	require.Equal(t, 2, indexes)
	deleted, err = repo.DeleteAllPromptRecords(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 6, deleted)
	page, err = repo.ListPromptRecords(ctx, PromptRecordFilter{}, 1, 20)
	require.NoError(t, err)
	require.Zero(t, page.Total)
	require.Empty(t, page.Items)
	t.Log("Verified migrations, counted cursors, tied-timestamp pagination, concurrent insertion, expiry visibility, full deletion and legacy detail compatibility.")
}
