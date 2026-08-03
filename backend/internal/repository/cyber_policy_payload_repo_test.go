package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCreateRequestPayloadInsertsRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO cyber_policy_request_payloads")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))

	payload := &service.CyberPolicyRequestPayload{
		ModerationLogID: 42,
		RequestBody:     `{"model":"gpt-5"}`,
		BodyBytes:       17,
	}
	err = repo.CreateRequestPayload(context.Background(), payload)

	require.NoError(t, err)
	require.Equal(t, int64(7), payload.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetRequestPayloadReturnsNilWhenMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	mock.ExpectQuery(regexp.QuoteMeta("FROM cyber_policy_request_payloads")).
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{}))

	payload, err := repo.GetRequestPayload(context.Background(), 99)

	require.NoError(t, err)
	require.Nil(t, payload)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetRequestPayloadScansRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	rows := sqlmock.NewRows([]string{
		"id", "moderation_log_id", "request_id", "user_id", "user_email",
		"api_key_id", "group_id", "endpoint", "protocol", "model",
		"request_body", "body_bytes", "created_at",
	}).AddRow(
		int64(3), int64(42), "req-1", int64(11), "u@x.com",
		int64(22), int64(33), "/v1/responses", "openai_responses", "gpt-5",
		`{"input":"hi"}`, 14, time.Now(),
	)
	mock.ExpectQuery(regexp.QuoteMeta("FROM cyber_policy_request_payloads")).
		WithArgs(int64(42)).
		WillReturnRows(rows)

	payload, err := repo.GetRequestPayload(context.Background(), 42)

	require.NoError(t, err)
	require.NotNil(t, payload)
	require.Equal(t, int64(42), payload.ModerationLogID)
	require.Equal(t, "openai_responses", payload.Protocol)
	require.Equal(t, `{"input":"hi"}`, payload.RequestBody)
	require.NotNil(t, payload.UserID)
	require.Equal(t, int64(11), *payload.UserID)
	require.NoError(t, mock.ExpectationsWereMet())
}
