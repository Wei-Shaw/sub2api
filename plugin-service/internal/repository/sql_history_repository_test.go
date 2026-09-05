package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/model"
)

func TestSQLHistoryRepositoryCreateUpdateAndRead(t *testing.T) {
	db, mock := newSQLMock(t)
	defer db.Close()

	repo := NewSQLHistoryRepository(db)
	repo.now = func() time.Time {
		return time.Date(2026, 7, 9, 10, 11, 12, 0, time.UTC)
	}
	repo.newID = func() string {
		return "history-1"
	}

	ctx := context.Background()
	principal := model.CurrentPrincipal{
		UserID: 42,
		Email:  "user@example.com",
		Plugin: "image-generation",
	}
	request := map[string]any{"prompt": "cat", "size": "1024x1024", "conversation_id": "conversation-1"}
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO plugin_generation_history (
			id, conversation_id, user_id, user_email, plugin_key, prompt, status,
			request_payload, result_payload, error_message, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL, '', $9, $9)
	`)).
		WithArgs("history-1", "conversation-1", int64(42), "user@example.com", "image-generation", "cat", model.HistoryStatusPending, mustJSON(t, request), repo.now()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	record, err := repo.Create(ctx, principal, " cat ", request)
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != "history-1" {
		t.Fatalf("created id = %q", record.ID)
	}
	if record.ConversationID != "conversation-1" {
		t.Fatalf("created conversation id = %q, want %q", record.ConversationID, "conversation-1")
	}

	record.Status = model.HistoryStatusSucceeded
	record.Result = map[string]any{"type": "image_generation"}
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE plugin_generation_history
		SET conversation_id = $2,
			status = $3,
			request_payload = $4,
			result_payload = $5,
			error_message = $6,
			updated_at = $7
		WHERE id = $1
	`)).
		WithArgs("history-1", "conversation-1", model.HistoryStatusSucceeded, mustJSON(t, request), mustJSON(t, record.Result), "", repo.now()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Update(ctx, record); err != nil {
		t.Fatal(err)
	}

	rows := sqlmock.NewRows([]string{
		"id", "conversation_id", "user_id", "user_email", "plugin_key", "prompt", "status",
		"request_payload", "result_payload", "error_message", "created_at", "updated_at",
	}).AddRow(
		"history-1", "conversation-1", int64(42), "user@example.com", "image-generation", "cat", model.HistoryStatusSucceeded,
		mustJSON(t, request), mustJSON(t, record.Result), "", repo.now(), repo.now(),
	)
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, conversation_id, user_id, user_email, plugin_key, prompt, status,
			request_payload, result_payload, error_message, created_at, updated_at
		FROM plugin_generation_history
		WHERE id = $1
	`)).
		WithArgs("history-1").
		WillReturnRows(rows)

	got, ok, err := repo.Get(ctx, "history-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected record to be found")
	}
	if got.Result["type"] != "image_generation" {
		t.Fatalf("result = %#v", got.Result)
	}
	if got.ConversationID != "conversation-1" {
		t.Fatalf("read conversation id = %q, want %q", got.ConversationID, "conversation-1")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLHistoryRepositoryListByUserUsesDatabasePagination(t *testing.T) {
	db, mock := newSQLMock(t)
	defer db.Close()

	repo := NewSQLHistoryRepository(db)
	createdAt := time.Date(2026, 7, 9, 10, 11, 12, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "conversation_id", "user_id", "user_email", "plugin_key", "prompt", "status",
		"request_payload", "result_payload", "error_message", "created_at", "updated_at",
	}).AddRow(
		"history-2", "conversation-2", int64(7), "user@example.com", "image-generation", "dog", model.HistoryStatusPending,
		`{"prompt":"dog"}`, sql.NullString{}, "", createdAt, createdAt,
	)
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, conversation_id, user_id, user_email, plugin_key, prompt, status,
			request_payload, result_payload, error_message, created_at, updated_at
		FROM plugin_generation_history
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`)).
		WithArgs(int64(7), 20, 20).
		WillReturnRows(rows)

	records, err := repo.ListByUser(ctxWithCancel(t), 7, model.HistoryQuery{Page: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != "history-2" {
		t.Fatalf("records = %#v", records)
	}
	if records[0].ConversationID != "conversation-2" {
		t.Fatalf("conversation id = %q, want %q", records[0].ConversationID, "conversation-2")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLHistoryRepositoryDeleteRemovesRecord(t *testing.T) {
	db, mock := newSQLMock(t)
	defer db.Close()

	repo := NewSQLHistoryRepository(db)
	mock.ExpectExec(regexp.QuoteMeta(`
		DELETE FROM plugin_generation_history
		WHERE id = $1
	`)).
		WithArgs("history-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Delete(context.Background(), "history-1"); err != nil {
		t.Fatal(err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLHistoryRepositoryDeleteReturnsNotFound(t *testing.T) {
	db, mock := newSQLMock(t)
	defer db.Close()

	repo := NewSQLHistoryRepository(db)
	mock.ExpectExec(regexp.QuoteMeta(`
		DELETE FROM plugin_generation_history
		WHERE id = $1
	`)).
		WithArgs("missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := repo.Delete(context.Background(), "missing"); err != ErrNotFound {
		t.Fatalf("Delete() err = %v, want ErrNotFound", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLHistoryRepositoryEnsureSchema(t *testing.T) {
	db, mock := newSQLMock(t)
	defer db.Close()

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS plugin_generation_history").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE plugin_generation_history ADD COLUMN IF NOT EXISTS conversation_id TEXT NOT NULL DEFAULT ''").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_plugin_generation_history_user_created").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_plugin_generation_history_conversation").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_plugin_generation_history_created").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_plugin_generation_history_user_conversation_updated").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_plugin_generation_history_conversation_updated").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_plugin_generation_history_user_conversation_created").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := EnsureHistorySchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func newSQLMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

func mustJSON(t *testing.T, value map[string]any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func ctxWithCancel(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}
