package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type grokMediaImageCreateRepository struct {
	db *sql.DB
}

func NewGrokMediaImageCreateRepository(db *sql.DB) service.GrokMediaImageCreateRepository {
	return &grokMediaImageCreateRepository{db: db}
}

func (r *grokMediaImageCreateRepository) ClaimImageCreate(ctx context.Context, record service.GrokMediaImageCreateRecord) (*service.GrokMediaImageCreateRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("grok image idempotency database is unavailable")
	}
	if err := validateGrokImageCreateRecord(record, false); err != nil {
		return nil, err
	}
	claimed, err := scanGrokImageCreateRecord(r.db.QueryRowContext(ctx, `
		INSERT INTO grok_image_create_idempotency (
			user_id, api_key_id, group_id, endpoint, idempotency_key_hash,
			request_hash, upstream_idempotency_key, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, api_key_id, group_id, endpoint, idempotency_key_hash)
		DO UPDATE SET
			request_hash = EXCLUDED.request_hash,
			upstream_idempotency_key = EXCLUDED.upstream_idempotency_key,
			status = 'processing', account_id = NULL,
			response_status = NULL, response_content_type = NULL,
			response_body = NULL, expires_at = EXCLUDED.expires_at,
			updated_at = NOW()
		WHERE grok_image_create_idempotency.expires_at <= NOW()
		RETURNING user_id, api_key_id, group_id, endpoint, idempotency_key_hash,
			request_hash, upstream_idempotency_key, account_id, response_status,
			response_content_type, response_body, expires_at
	`, record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
		record.RequestHash, record.UpstreamIdempotencyKey, record.ExpiresAt.UTC()))
	if errors.Is(err, sql.ErrNoRows) {
		claimed, err = scanGrokImageCreateRecord(r.db.QueryRowContext(ctx, `
			SELECT user_id, api_key_id, group_id, endpoint, idempotency_key_hash,
				request_hash, upstream_idempotency_key, account_id, response_status,
				response_content_type, response_body, expires_at
			FROM grok_image_create_idempotency
			WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3
			  AND endpoint = $4 AND idempotency_key_hash = $5
			  AND expires_at > NOW()
		`, record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash))
	}
	if err != nil {
		return nil, fmt.Errorf("claim grok image create idempotency: %w", err)
	}
	if claimed.RequestHash != record.RequestHash || claimed.UpstreamIdempotencyKey != record.UpstreamIdempotencyKey {
		return nil, service.ErrGrokMediaImageIdempotencyConflict
	}
	return claimed, nil
}

func (r *grokMediaImageCreateRepository) BindImageCreateAccount(ctx context.Context, record service.GrokMediaImageCreateRecord, accountID int64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("grok image idempotency database is unavailable")
	}
	if err := validateGrokImageCreateRecord(record, false); err != nil {
		return 0, err
	}
	if accountID <= 0 {
		return 0, errors.New("grok image create account binding is invalid")
	}
	var boundID int64
	err := r.db.QueryRowContext(ctx, `
		UPDATE grok_image_create_idempotency
		SET account_id = COALESCE(account_id, $8), updated_at = NOW()
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3
		  AND endpoint = $4 AND idempotency_key_hash = $5
		  AND request_hash = $6 AND upstream_idempotency_key = $7
		  AND status = 'processing' AND expires_at > NOW()
		RETURNING account_id
	`, record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
		record.RequestHash, record.UpstreamIdempotencyKey, accountID).Scan(&boundID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, service.ErrGrokMediaImageIdempotencyConflict
	}
	if err != nil {
		return 0, fmt.Errorf("bind grok image create account: %w", err)
	}
	return boundID, nil
}

func (r *grokMediaImageCreateRepository) ReleaseImageCreateAccount(ctx context.Context, record service.GrokMediaImageCreateRecord, accountID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("grok image idempotency database is unavailable")
	}
	if err := validateGrokImageCreateRecord(record, false); err != nil {
		return false, err
	}
	if accountID <= 0 {
		return false, errors.New("grok image create account release is invalid")
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE grok_image_create_idempotency
		SET account_id = NULL, updated_at = NOW()
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3
		  AND endpoint = $4 AND idempotency_key_hash = $5
		  AND request_hash = $6 AND upstream_idempotency_key = $7
		  AND status = 'processing' AND account_id = $8 AND expires_at > NOW()
	`, record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
		record.RequestHash, record.UpstreamIdempotencyKey, accountID)
	if err != nil {
		return false, fmt.Errorf("release grok image create account: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read grok image create account release: %w", err)
	}
	return affected == 1, nil
}

func (r *grokMediaImageCreateRepository) CompleteImageCreate(ctx context.Context, record service.GrokMediaImageCreateRecord) error {
	if r == nil || r.db == nil {
		return errors.New("grok image idempotency database is unavailable")
	}
	if err := validateGrokImageCreateRecord(record, true); err != nil {
		return err
	}
	var completedAccountID int64
	err := r.db.QueryRowContext(ctx, `
		UPDATE grok_image_create_idempotency
		SET status = 'completed', response_status = $8,
			response_content_type = $9, response_body = $10,
			expires_at = GREATEST(expires_at, $11), updated_at = NOW()
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3
		  AND endpoint = $4 AND idempotency_key_hash = $5
		  AND request_hash = $6 AND upstream_idempotency_key = $7
		  AND account_id = $12 AND expires_at > NOW()
		  AND (status = 'processing' OR (
			status = 'completed' AND response_status = $8
			AND response_content_type IS NOT DISTINCT FROM $9
			AND response_body = $10
		  ))
		RETURNING account_id
	`, record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
		record.RequestHash, record.UpstreamIdempotencyKey, record.ResponseStatus,
		record.ResponseContentType, record.ResponseBody, record.ExpiresAt.UTC(), record.AccountID).Scan(&completedAccountID)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrGrokMediaImageIdempotencyConflict
	}
	if err != nil {
		return fmt.Errorf("persist grok image create response: %w", err)
	}
	if completedAccountID != record.AccountID {
		return service.ErrGrokMediaImageIdempotencyConflict
	}
	return nil
}

func (r *grokMediaImageCreateRepository) DeleteExpired(ctx context.Context, before time.Time, limit int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("grok image idempotency database is unavailable")
	}
	if before.IsZero() || limit <= 0 || limit > 1000 {
		return 0, errors.New("grok image idempotency cleanup request is invalid")
	}
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM grok_image_create_idempotency
		WHERE (user_id, api_key_id, group_id, endpoint, idempotency_key_hash) IN (
			SELECT user_id, api_key_id, group_id, endpoint, idempotency_key_hash
			FROM grok_image_create_idempotency
			WHERE expires_at <= $1
			ORDER BY expires_at, user_id, api_key_id, group_id, endpoint, idempotency_key_hash
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
	`, before.UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("delete expired grok image create idempotency records: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read expired grok image create cleanup: %w", err)
	}
	return affected, nil
}

type grokImageCreateScanner interface {
	Scan(dest ...any) error
}

func scanGrokImageCreateRecord(scanner grokImageCreateScanner) (*service.GrokMediaImageCreateRecord, error) {
	var record service.GrokMediaImageCreateRecord
	var endpoint string
	var accountID, responseStatus sql.NullInt64
	var responseContentType sql.NullString
	var responseBody []byte
	if err := scanner.Scan(
		&record.UserID, &record.APIKeyID, &record.GroupID, &endpoint,
		&record.IdempotencyKeyHash, &record.RequestHash, &record.UpstreamIdempotencyKey,
		&accountID, &responseStatus, &responseContentType, &responseBody, &record.ExpiresAt,
	); err != nil {
		return nil, err
	}
	record.Endpoint = service.GrokMediaEndpoint(endpoint)
	if accountID.Valid {
		record.AccountID = accountID.Int64
	}
	if responseStatus.Valid {
		record.ResponseStatus = int(responseStatus.Int64)
	}
	if responseContentType.Valid {
		record.ResponseContentType = responseContentType.String
	}
	if responseBody != nil {
		record.ResponseBody = append([]byte(nil), responseBody...)
	}
	record.ExpiresAt = record.ExpiresAt.In(time.UTC)
	return &record, nil
}

func validateGrokImageCreateRecord(record service.GrokMediaImageCreateRecord, completed bool) error {
	if record.UserID <= 0 || record.APIKeyID <= 0 || record.GroupID < 0 || !record.Endpoint.IsImageGenerationRequest() ||
		len(record.IdempotencyKeyHash) != 64 || len(record.RequestHash) != 64 || strings.TrimSpace(record.UpstreamIdempotencyKey) == "" || record.ExpiresAt.IsZero() {
		return errors.New("grok image create idempotency record is invalid")
	}
	if completed && (record.AccountID <= 0 || record.ResponseStatus < 200 || record.ResponseStatus >= 300 || record.ResponseBody == nil) {
		return errors.New("grok image create completion is invalid")
	}
	return nil
}
