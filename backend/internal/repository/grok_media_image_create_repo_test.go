//go:build unit

package repository

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGrokMediaImageCreateRepositoryLifecycleNeverWritesVideoOwner(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewGrokMediaImageCreateRepository(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	record := service.GrokMediaImageCreateRecord{
		UserID: 41, APIKeyID: 51, GroupID: 7,
		Endpoint:               service.GrokMediaEndpointImagesGenerations,
		IdempotencyKeyHash:     strings.Repeat("a", 64),
		RequestHash:            strings.Repeat("b", 64),
		UpstreamIdempotencyKey: "sub2api-grok-image-" + strings.Repeat("c", 64),
		ExpiresAt:              now.Add(24 * time.Hour),
	}
	columns := []string{
		"user_id", "api_key_id", "group_id", "endpoint", "idempotency_key_hash",
		"request_hash", "upstream_idempotency_key", "account_id", "response_status",
		"response_content_type", "response_body", "expires_at",
	}
	mock.ExpectQuery(`INSERT INTO grok_image_create_idempotency`).
		WithArgs(record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
			record.RequestHash, record.UpstreamIdempotencyKey, record.ExpiresAt).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
			record.RequestHash, record.UpstreamIdempotencyKey, nil, nil, nil, nil, record.ExpiresAt,
		))
	claimed, err := repo.ClaimImageCreate(context.Background(), record)
	require.NoError(t, err)

	mock.ExpectQuery(`UPDATE grok_image_create_idempotency\s+SET account_id = COALESCE`).
		WithArgs(record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
			record.RequestHash, record.UpstreamIdempotencyKey, int64(63)).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(63)))
	boundID, err := repo.BindImageCreateAccount(context.Background(), *claimed, 63)
	require.NoError(t, err)
	require.Equal(t, int64(63), boundID)

	claimed.AccountID = boundID
	claimed.ResponseStatus = http.StatusOK
	claimed.ResponseContentType = "application/json"
	claimed.ResponseBody = []byte(`{"data":[{"url":"https://images.test/1.png"}]}`)
	mock.ExpectQuery(`UPDATE grok_image_create_idempotency\s+SET status = 'completed'`).
		WithArgs(record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
			record.RequestHash, record.UpstreamIdempotencyKey, claimed.ResponseStatus,
			claimed.ResponseContentType, claimed.ResponseBody, claimed.ExpiresAt, claimed.AccountID).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(63)))
	require.NoError(t, repo.CompleteImageCreate(context.Background(), *claimed))
	require.NoError(t, mock.ExpectationsWereMet(), "no unexpected grok_video_request_owners write is allowed")
}

func TestGrokMediaImageCreateRepositoryAccountRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewGrokMediaImageCreateRepository(db)
	record := service.GrokMediaImageCreateRecord{
		UserID: 1, APIKeyID: 2, GroupID: 3, Endpoint: service.GrokMediaEndpointImagesEdits,
		IdempotencyKeyHash: strings.Repeat("a", 64), RequestHash: strings.Repeat("b", 64),
		UpstreamIdempotencyKey: "sub2api-grok-image-" + strings.Repeat("c", 64), ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	mock.ExpectExec(`UPDATE grok_image_create_idempotency\s+SET account_id = NULL`).
		WithArgs(record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
			record.RequestHash, record.UpstreamIdempotencyKey, int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	released, err := repo.ReleaseImageCreateAccount(context.Background(), record, 9)
	require.NoError(t, err)
	require.True(t, released)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrokMediaImageCreateRepositoryCleanupIsOrderedBoundedAndConcurrentSafe(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewGrokMediaImageCreateRepository(db)
	before := time.Now().UTC().Truncate(time.Microsecond)

	for _, count := range []int64{100, 100, 37, 0} {
		mock.ExpectExec(`DELETE FROM grok_image_create_idempotency[\s\S]+WHERE expires_at <= \$1[\s\S]+ORDER BY expires_at[\s\S]+LIMIT \$2[\s\S]+FOR UPDATE SKIP LOCKED`).
			WithArgs(before, 100).
			WillReturnResult(sqlmock.NewResult(0, count))
		deleted, cleanupErr := repo.DeleteExpired(context.Background(), before, 100)
		require.NoError(t, cleanupErr)
		require.Equal(t, count, deleted)
	}
	_, err = repo.DeleteExpired(context.Background(), before, 1001)
	require.ErrorContains(t, err, "cleanup request is invalid")
	require.NoError(t, mock.ExpectationsWereMet())
}
