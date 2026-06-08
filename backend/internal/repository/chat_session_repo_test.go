package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
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
	mock.ExpectQuery("SELECT role, direction, content_text, content_json\\s+FROM chat_messages").
		WithArgs(int64(99)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT role, direction, content_text, content_json\\s+FROM chat_messages").
		WithArgs(int64(99), 1000).
		WillReturnRows(sqlmock.NewRows([]string{"role", "direction", "content_text", "content_json"}))
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
