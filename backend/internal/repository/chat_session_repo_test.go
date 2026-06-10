package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChatSessionRepositoryAppendRefreshesSessionCreatedAt(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &chatSessionRepository{sql: db}
	createdAt := time.Date(2026, 6, 1, 12, 34, 0, 0, time.UTC)
	accountID := int64(33)
	groupID := int64(44)
	requestedModel := "gpt-5.5"
	upstreamModel := "gpt-5.4"
	inboundEndpoint := "/v1/chat/completions"
	upstreamEndpoint := "/v1/responses"
	input := &service.ChatSessionRecordInput{
		SessionKey:       "session-key",
		RequestID:        "req-123",
		UserID:           11,
		APIKeyID:         22,
		AccountID:        &accountID,
		GroupID:          &groupID,
		Platform:         "openai",
		Model:            "gpt-5.5",
		RequestedModel:   &requestedModel,
		UpstreamModel:    &upstreamModel,
		InboundEndpoint:  &inboundEndpoint,
		UpstreamEndpoint: &upstreamEndpoint,
		RequestType:      service.RequestTypeStream,
		Stream:           true,
		Status:           "OK",
		HTTPStatusCode:   200,
		CreatedAt:        createdAt,
		Messages: []service.ChatMessageRecordInput{
			{Role: "user", Direction: "inbound", ContentText: "new question"},
			{Role: "assistant", Direction: "outbound", ContentText: "new answer"},
		},
	}

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs("11:22:" + input.SessionKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM chat_sessions\\s+WHERE user_id = \\$1 AND api_key_id = \\$2 AND session_key = \\$3").
		WithArgs(input.UserID, input.APIKeyID, input.SessionKey).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(99)))
	mock.ExpectExec("UPDATE chat_sessions").
		WithArgs(
			int64(99),
			input.RequestID,
			sql.NullInt64{Int64: accountID, Valid: true},
			sql.NullInt64{Int64: groupID, Valid: true},
			input.Platform,
			input.Model,
			sql.NullString{String: requestedModel, Valid: true},
			sql.NullString{String: upstreamModel, Valid: true},
			sql.NullString{String: inboundEndpoint, Valid: true},
			sql.NullString{String: upstreamEndpoint, Valid: true},
			int16(input.RequestType.Normalize()),
			input.Stream,
			input.Status,
			input.HTTPStatusCode,
			sql.NullString{String: "new question", Valid: true},
			sql.NullString{String: "new answer", Valid: true},
			len(input.Messages),
			createdAt,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(seq\\), 0\\) FROM chat_messages WHERE session_id = \\$1").
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(2))
	mock.ExpectPrepare("INSERT INTO chat_messages").
		ExpectExec().
		WithArgs(int64(99), 3, "user", "inbound", "new question", nil, createdAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO chat_messages").
		WithArgs(int64(99), 4, "assistant", "outbound", "new answer", nil, createdAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE chat_sessions\\s+SET message_count =").
		WithArgs(int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = repo.CreateSessionWithMessages(context.Background(), input)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChatSessionRepositoryDeleteSessionsBefore(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &chatSessionRepository{sql: db}
	cutoff := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)

	mock.ExpectExec("DELETE FROM chat_sessions").
		WithArgs(cutoff, 500).
		WillReturnResult(sqlmock.NewResult(0, 12))

	deleted, err := repo.DeleteSessionsBefore(context.Background(), cutoff, 500)
	require.NoError(t, err)
	require.Equal(t, int64(12), deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChatSessionPayloadStoreStoresLargePayloadsByDay(t *testing.T) {
	baseDir := t.TempDir()
	store := newChatSessionPayloadStore(&config.Config{
		ChatSessionRetention: config.ChatSessionRetentionConfig{
			PayloadDir:            baseDir,
			PayloadInlineMaxBytes: 8,
		},
	})
	raw := json.RawMessage(`{"text":"large payload"}`)
	createdAt := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	stored, err := store.Store(context.Background(), raw, createdAt, "message")
	require.NoError(t, err)

	ref, ok := stored.(chatSessionPayloadRef)
	require.True(t, ok)
	require.Equal(t, chatSessionPayloadStorageFile, ref.Storage)
	require.Equal(t, chatSessionPayloadCompression, ref.Compression)
	require.Contains(t, ref.Path, "2026-06-10/")
	require.True(t, strings.HasSuffix(ref.Path, ".json.gz"), "path = %s", ref.Path)
	require.Equal(t, int64(len(raw)), ref.Bytes)
	require.Positive(t, ref.StoredBytes)
	require.FileExists(t, filepath.Join(baseDir, filepath.FromSlash(ref.Path)))

	refJSON, err := json.Marshal(ref)
	require.NoError(t, err)
	resolved, err := store.Load(context.Background(), refJSON)
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(resolved))
}

func TestChatSessionPayloadStoreKeepsSmallPayloadInline(t *testing.T) {
	store := newChatSessionPayloadStore(&config.Config{
		ChatSessionRetention: config.ChatSessionRetentionConfig{
			PayloadDir:            t.TempDir(),
			PayloadInlineMaxBytes: 1024,
		},
	})
	raw := json.RawMessage(`{"ok":true}`)

	stored, err := store.Store(context.Background(), raw, time.Now(), "message")
	require.NoError(t, err)
	require.Equal(t, raw, stored)
}

func TestChatSessionPayloadStoreDeleteDateDirsBefore(t *testing.T) {
	baseDir := t.TempDir()
	store := &chatSessionPayloadStore{baseDir: baseDir, inlineMaxBytes: 0}
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "2026-05-01"), 0750))
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "2026-06-09"), 0750))
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "2026-06-10"), 0750))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "2026-05-01", "payload.json"), []byte(`{}`), 0640))

	store.DeleteDateDirsBefore(time.Date(2026, 6, 10, 15, 0, 0, 0, time.UTC))

	require.NoDirExists(t, filepath.Join(baseDir, "2026-05-01"))
	require.NoDirExists(t, filepath.Join(baseDir, "2026-06-09"))
	require.DirExists(t, filepath.Join(baseDir, "2026-06-10"))
}
