//go:build unit

package repository

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGrokMediaVideoOwnerRepositoryBindAndResolve(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewGrokMediaVideoOwnerRepository(db)
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour).Truncate(time.Microsecond)
	refreshUntil := expiresAt.Add(24 * time.Hour)

	mock.ExpectQuery(`INSERT INTO grok_video_request_owners`).
		WithArgs("video-123", int64(41), int64(51), int64(7), int64(63), expiresAt).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(63)))
	require.NoError(t, repo.Bind(context.Background(), service.GrokMediaVideoRequestOwner{
		RequestID: "video-123",
		UserID:    41,
		APIKeyID:  51,
		GroupID:   7,
		AccountID: 63,
		ExpiresAt: expiresAt,
	}))

	mock.ExpectQuery(`UPDATE grok_video_request_owners\s+SET expires_at = GREATEST\(expires_at, \$5\)`).
		WithArgs("video-123", int64(41), int64(51), int64(7), refreshUntil).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "expires_at", "terminal_at"}).AddRow(int64(63), refreshUntil, nil))
	owner, err := repo.Resolve(context.Background(), "video-123", 41, 51, 7, refreshUntil)
	require.NoError(t, err)
	require.Equal(t, int64(63), owner.AccountID)
	require.Equal(t, refreshUntil, owner.ExpiresAt)
	require.Nil(t, owner.TerminalAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrokMediaVideoOwnerRepositoryConflictMissingAndUnavailable(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewGrokMediaVideoOwnerRepository(db)
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)
	refreshUntil := expiresAt.Add(24 * time.Hour)

	mock.ExpectQuery(`INSERT INTO grok_video_request_owners`).
		WillReturnError(sql.ErrNoRows)
	err = repo.Bind(context.Background(), service.GrokMediaVideoRequestOwner{
		RequestID: "collision", UserID: 1, APIKeyID: 2, GroupID: 3, AccountID: 4, ExpiresAt: expiresAt,
	})
	require.ErrorIs(t, err, service.ErrGrokMediaVideoRequestOwnerConflict)

	mock.ExpectQuery(`UPDATE grok_video_request_owners\s+SET expires_at = GREATEST\(expires_at, \$5\)`).
		WithArgs("missing", int64(1), int64(2), int64(3), refreshUntil).
		WillReturnError(sql.ErrNoRows)
	_, err = repo.Resolve(context.Background(), "missing", 1, 2, 3, refreshUntil)
	require.ErrorIs(t, err, service.ErrGrokMediaVideoRequestOwnerNotFound)

	dbErr := errors.New("postgres unavailable")
	mock.ExpectQuery(`UPDATE grok_video_request_owners\s+SET expires_at = GREATEST\(expires_at, \$5\)`).
		WithArgs("present", int64(1), int64(2), int64(3), refreshUntil).
		WillReturnError(dbErr)
	_, err = repo.Resolve(context.Background(), "present", 1, 2, 3, refreshUntil)
	require.ErrorIs(t, err, dbErr)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrokMediaVideoOwnerRepositoryTerminalAndBoundedCleanup(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewGrokMediaVideoOwnerRepository(db)
	terminalAt := time.Now().UTC().Truncate(time.Microsecond)
	retainUntil := terminalAt.Add(7 * 24 * time.Hour)

	mock.ExpectExec(`UPDATE grok_video_request_owners\s+SET terminal_at = COALESCE\(terminal_at, \$5\)`).
		WithArgs("video-terminal", int64(41), int64(51), int64(7), terminalAt, retainUntil).
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.MarkTerminal(context.Background(), "video-terminal", 41, 51, 7, terminalAt, retainUntil))

	mock.ExpectExec(`DELETE FROM grok_video_request_owners[\s\S]+LIMIT \$2[\s\S]+FOR UPDATE SKIP LOCKED`).
		WithArgs(terminalAt, 100).
		WillReturnResult(sqlmock.NewResult(0, 37))
	deleted, err := repo.DeleteExpired(context.Background(), terminalAt, 100)
	require.NoError(t, err)
	require.Equal(t, int64(37), deleted)

	for _, count := range []int64{100, 100, 37, 0} {
		mock.ExpectExec(`DELETE FROM grok_video_create_idempotency[\s\S]+WHERE expires_at <= \$1[\s\S]+ORDER BY expires_at[\s\S]+LIMIT \$2[\s\S]+FOR UPDATE SKIP LOCKED`).
			WithArgs(terminalAt, 100).
			WillReturnResult(sqlmock.NewResult(0, count))
		deleted, err = repo.DeleteExpiredVideoCreates(context.Background(), terminalAt, 100)
		require.NoError(t, err)
		require.Equal(t, count, deleted)
	}
	_, err = repo.DeleteExpiredVideoCreates(context.Background(), terminalAt, 1001)
	require.ErrorContains(t, err, "cleanup request is invalid")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrokMediaVideoOwnerRepositoryCreateIdempotencyCrashSafeLifecycle(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewGrokMediaVideoOwnerRepository(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	record := service.GrokMediaVideoCreateRecord{
		UserID:                 41,
		APIKeyID:               51,
		GroupID:                7,
		Endpoint:               service.GrokMediaEndpointVideosGenerations,
		IdempotencyKeyHash:     strings.Repeat("a", 64),
		RequestHash:            strings.Repeat("b", 64),
		UpstreamIdempotencyKey: "sub2api-grok-video-" + strings.Repeat("c", 64),
		ExpiresAt:              now.Add(24 * time.Hour),
	}
	columns := []string{
		"user_id", "api_key_id", "group_id", "endpoint", "idempotency_key_hash",
		"request_hash", "upstream_idempotency_key", "account_id", "request_id",
		"response_status", "response_content_type", "response_body", "expires_at",
	}
	mock.ExpectQuery(`INSERT INTO grok_video_create_idempotency`).
		WithArgs(record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
			record.RequestHash, record.UpstreamIdempotencyKey, record.ExpiresAt).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
			record.RequestHash, record.UpstreamIdempotencyKey, nil, nil, nil, nil, nil, record.ExpiresAt,
		))
	claimed, err := repo.ClaimVideoCreate(context.Background(), record)
	require.NoError(t, err)
	require.Zero(t, claimed.AccountID)

	mock.ExpectQuery(`UPDATE grok_video_create_idempotency\s+SET account_id = COALESCE`).
		WithArgs(record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
			record.RequestHash, record.UpstreamIdempotencyKey, int64(63)).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(63)))
	boundID, err := repo.BindVideoCreateAccount(context.Background(), *claimed, 63)
	require.NoError(t, err)
	require.Equal(t, int64(63), boundID)
	mock.ExpectExec(`UPDATE grok_video_create_idempotency\s+SET account_id = NULL`).
		WithArgs(record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
			record.RequestHash, record.UpstreamIdempotencyKey, int64(63)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	released, err := repo.ReleaseVideoCreateAccount(context.Background(), *claimed, 63)
	require.NoError(t, err)
	require.True(t, released)
	mock.ExpectQuery(`UPDATE grok_video_create_idempotency\s+SET account_id = COALESCE`).
		WithArgs(record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
			record.RequestHash, record.UpstreamIdempotencyKey, int64(63)).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(63)))
	boundID, err = repo.BindVideoCreateAccount(context.Background(), *claimed, 63)
	require.NoError(t, err)
	require.Equal(t, int64(63), boundID)

	claimed.AccountID = 63
	claimed.RequestID = "video-123"
	claimed.ResponseStatus = http.StatusAccepted
	claimed.ResponseContentType = "application/json"
	claimed.ResponseBody = []byte(`{"request_id":"video-123"}`)
	owner := service.GrokMediaVideoRequestOwner{
		RequestID: "video-123", UserID: 41, APIKeyID: 51, GroupID: 7, AccountID: 63,
		ExpiresAt: now.Add(2 * time.Hour),
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE grok_video_create_idempotency\s+SET status = 'completed'`).
		WithArgs(record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
			record.RequestHash, record.UpstreamIdempotencyKey, claimed.RequestID, claimed.ResponseStatus,
			claimed.ResponseContentType, claimed.ResponseBody, claimed.ExpiresAt, claimed.AccountID).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(63)))
	mock.ExpectQuery(`INSERT INTO grok_video_request_owners`).
		WithArgs(owner.RequestID, owner.UserID, owner.APIKeyID, owner.GroupID, owner.AccountID, owner.ExpiresAt).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(63)))
	mock.ExpectCommit()
	require.NoError(t, repo.CompleteVideoCreate(context.Background(), *claimed, owner))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrokMediaVideoOwnerRepositoryCreateIdempotencyPayloadConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewGrokMediaVideoOwnerRepository(db)
	expiresAt := time.Now().UTC().Add(time.Hour)
	record := service.GrokMediaVideoCreateRecord{
		UserID: 1, APIKeyID: 2, GroupID: 3, Endpoint: service.GrokMediaEndpointVideosGenerations,
		IdempotencyKeyHash: strings.Repeat("a", 64), RequestHash: strings.Repeat("b", 64),
		UpstreamIdempotencyKey: "sub2api-grok-video-" + strings.Repeat("c", 64), ExpiresAt: expiresAt,
	}
	columns := []string{
		"user_id", "api_key_id", "group_id", "endpoint", "idempotency_key_hash",
		"request_hash", "upstream_idempotency_key", "account_id", "request_id",
		"response_status", "response_content_type", "response_body", "expires_at",
	}
	mock.ExpectQuery(`INSERT INTO grok_video_create_idempotency`).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT user_id, api_key_id, group_id, endpoint, idempotency_key_hash`).
		WithArgs(record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
			strings.Repeat("d", 64), record.UpstreamIdempotencyKey, nil, nil, nil, nil, nil, expiresAt,
		))
	_, err = repo.ClaimVideoCreate(context.Background(), record)
	require.ErrorIs(t, err, service.ErrGrokMediaVideoIdempotencyConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}
