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

type grokMediaVideoOwnerRepository struct {
	db *sql.DB
}

func NewGrokMediaVideoOwnerRepository(db *sql.DB) service.GrokMediaVideoRequestOwnerRepository {
	return &grokMediaVideoOwnerRepository{db: db}
}

func (r *grokMediaVideoOwnerRepository) Bind(ctx context.Context, owner service.GrokMediaVideoRequestOwner) error {
	owner.RequestID = strings.TrimSpace(owner.RequestID)
	if r == nil || r.db == nil {
		return errors.New("grok video owner database is unavailable")
	}
	if owner.RequestID == "" || owner.UserID <= 0 || owner.APIKeyID <= 0 || owner.GroupID < 0 || owner.AccountID <= 0 || owner.ExpiresAt.IsZero() {
		return errors.New("grok video owner binding is invalid")
	}

	var accountID int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO grok_video_request_owners (
			request_id, user_id, api_key_id, group_id, account_id, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (request_id, user_id, api_key_id, group_id)
		DO UPDATE SET
			expires_at = GREATEST(grok_video_request_owners.expires_at, EXCLUDED.expires_at),
			last_accessed_at = NOW(), updated_at = NOW()
		WHERE grok_video_request_owners.account_id = EXCLUDED.account_id
		RETURNING account_id
	`, owner.RequestID, owner.UserID, owner.APIKeyID, owner.GroupID, owner.AccountID, owner.ExpiresAt.UTC()).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrGrokMediaVideoRequestOwnerConflict
	}
	if err != nil {
		return fmt.Errorf("bind grok video request owner: %w", err)
	}
	if accountID != owner.AccountID {
		return service.ErrGrokMediaVideoRequestOwnerConflict
	}
	return nil
}

func (r *grokMediaVideoOwnerRepository) Resolve(
	ctx context.Context,
	requestID string,
	userID, apiKeyID, groupID int64,
	refreshUntil time.Time,
) (*service.GrokMediaVideoRequestOwner, error) {
	requestID = strings.TrimSpace(requestID)
	if r == nil || r.db == nil {
		return nil, errors.New("grok video owner database is unavailable")
	}
	if requestID == "" || userID <= 0 || apiKeyID <= 0 || groupID < 0 || refreshUntil.IsZero() {
		return nil, errors.New("grok video owner lookup is invalid")
	}

	owner := &service.GrokMediaVideoRequestOwner{
		RequestID: requestID,
		UserID:    userID,
		APIKeyID:  apiKeyID,
		GroupID:   groupID,
	}
	var terminalAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		UPDATE grok_video_request_owners
		SET expires_at = GREATEST(expires_at, $5),
			last_accessed_at = NOW(), updated_at = NOW()
		WHERE request_id = $1
		  AND user_id = $2
		  AND api_key_id = $3
		  AND group_id = $4
		  AND expires_at > NOW()
		RETURNING account_id, expires_at, terminal_at
	`, requestID, userID, apiKeyID, groupID, refreshUntil.UTC()).Scan(&owner.AccountID, &owner.ExpiresAt, &terminalAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGrokMediaVideoRequestOwnerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("resolve grok video request owner: %w", err)
	}
	owner.ExpiresAt = owner.ExpiresAt.In(time.UTC)
	if terminalAt.Valid {
		value := terminalAt.Time.In(time.UTC)
		owner.TerminalAt = &value
	}
	return owner, nil
}

func (r *grokMediaVideoOwnerRepository) MarkTerminal(
	ctx context.Context,
	requestID string,
	userID, apiKeyID, groupID int64,
	terminalAt, retainUntil time.Time,
) error {
	requestID = strings.TrimSpace(requestID)
	if r == nil || r.db == nil {
		return errors.New("grok video owner database is unavailable")
	}
	if requestID == "" || userID <= 0 || apiKeyID <= 0 || groupID < 0 || terminalAt.IsZero() || retainUntil.IsZero() {
		return errors.New("grok video terminal owner update is invalid")
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE grok_video_request_owners
		SET terminal_at = COALESCE(terminal_at, $5),
			expires_at = GREATEST(expires_at, $6),
			last_accessed_at = NOW(), updated_at = NOW()
		WHERE request_id = $1 AND user_id = $2 AND api_key_id = $3
		  AND group_id = $4 AND expires_at > NOW()
	`, requestID, userID, apiKeyID, groupID, terminalAt.UTC(), retainUntil.UTC())
	if err != nil {
		return fmt.Errorf("mark grok video request owner terminal: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read grok video terminal owner update: %w", err)
	}
	if affected != 1 {
		return service.ErrGrokMediaVideoRequestOwnerNotFound
	}
	return nil
}

func (r *grokMediaVideoOwnerRepository) DeleteExpired(ctx context.Context, before time.Time, limit int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("grok video owner database is unavailable")
	}
	if before.IsZero() || limit <= 0 || limit > 1000 {
		return 0, errors.New("grok video owner cleanup request is invalid")
	}
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM grok_video_request_owners
		WHERE (request_id, user_id, api_key_id, group_id) IN (
			SELECT request_id, user_id, api_key_id, group_id
			FROM grok_video_request_owners
			WHERE expires_at <= $1
			ORDER BY expires_at, request_id, user_id, api_key_id, group_id
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
	`, before.UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("delete expired grok video request owners: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read expired grok video owner cleanup: %w", err)
	}
	return affected, nil
}

func (r *grokMediaVideoOwnerRepository) DeleteExpiredVideoCreates(ctx context.Context, before time.Time, limit int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("grok video idempotency database is unavailable")
	}
	if before.IsZero() || limit <= 0 || limit > 1000 {
		return 0, errors.New("grok video idempotency cleanup request is invalid")
	}
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM grok_video_create_idempotency
		WHERE (user_id, api_key_id, group_id, endpoint, idempotency_key_hash) IN (
			SELECT user_id, api_key_id, group_id, endpoint, idempotency_key_hash
			FROM grok_video_create_idempotency
			WHERE expires_at <= $1
			ORDER BY expires_at, user_id, api_key_id, group_id, endpoint, idempotency_key_hash
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
	`, before.UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("delete expired grok video create idempotency records: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read expired grok video create cleanup: %w", err)
	}
	return affected, nil
}

func (r *grokMediaVideoOwnerRepository) ClaimVideoCreate(
	ctx context.Context,
	record service.GrokMediaVideoCreateRecord,
) (*service.GrokMediaVideoCreateRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("grok video owner database is unavailable")
	}
	if err := validateGrokVideoCreateRecord(record, false); err != nil {
		return nil, err
	}

	claimed, err := scanGrokVideoCreateRecord(r.db.QueryRowContext(ctx, `
		INSERT INTO grok_video_create_idempotency (
			user_id, api_key_id, group_id, endpoint, idempotency_key_hash,
			request_hash, upstream_idempotency_key, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, api_key_id, group_id, endpoint, idempotency_key_hash)
		DO UPDATE SET
			request_hash = EXCLUDED.request_hash,
			upstream_idempotency_key = EXCLUDED.upstream_idempotency_key,
			status = 'processing',
			account_id = NULL,
			request_id = NULL,
			response_status = NULL,
			response_content_type = NULL,
			response_body = NULL,
			expires_at = EXCLUDED.expires_at,
			updated_at = NOW()
		WHERE grok_video_create_idempotency.expires_at <= NOW()
		RETURNING user_id, api_key_id, group_id, endpoint, idempotency_key_hash,
			request_hash, upstream_idempotency_key, account_id, request_id,
			response_status, response_content_type, response_body, expires_at
	`, record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
		record.RequestHash, record.UpstreamIdempotencyKey, record.ExpiresAt.UTC()))
	if errors.Is(err, sql.ErrNoRows) {
		claimed, err = scanGrokVideoCreateRecord(r.db.QueryRowContext(ctx, `
			SELECT user_id, api_key_id, group_id, endpoint, idempotency_key_hash,
				request_hash, upstream_idempotency_key, account_id, request_id,
				response_status, response_content_type, response_body, expires_at
			FROM grok_video_create_idempotency
			WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3
			  AND endpoint = $4 AND idempotency_key_hash = $5
			  AND expires_at > NOW()
		`, record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash))
	}
	if err != nil {
		return nil, fmt.Errorf("claim grok video create idempotency: %w", err)
	}
	if claimed.RequestHash != record.RequestHash || claimed.UpstreamIdempotencyKey != record.UpstreamIdempotencyKey {
		return nil, service.ErrGrokMediaVideoIdempotencyConflict
	}
	return claimed, nil
}

func (r *grokMediaVideoOwnerRepository) BindVideoCreateAccount(
	ctx context.Context,
	record service.GrokMediaVideoCreateRecord,
	accountID int64,
) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("grok video owner database is unavailable")
	}
	if err := validateGrokVideoCreateRecord(record, false); err != nil || accountID <= 0 {
		if err != nil {
			return 0, err
		}
		return 0, errors.New("grok video create account binding is invalid")
	}
	var boundID int64
	err := r.db.QueryRowContext(ctx, `
		UPDATE grok_video_create_idempotency
		SET account_id = COALESCE(account_id, $8), updated_at = NOW()
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3
		  AND endpoint = $4 AND idempotency_key_hash = $5
		  AND request_hash = $6 AND upstream_idempotency_key = $7
		  AND status = 'processing' AND expires_at > NOW()
		RETURNING account_id
	`, record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
		record.RequestHash, record.UpstreamIdempotencyKey, accountID).Scan(&boundID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, service.ErrGrokMediaVideoIdempotencyConflict
	}
	if err != nil {
		return 0, fmt.Errorf("bind grok video create account: %w", err)
	}
	return boundID, nil
}

func (r *grokMediaVideoOwnerRepository) ReleaseVideoCreateAccount(
	ctx context.Context,
	record service.GrokMediaVideoCreateRecord,
	accountID int64,
) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("grok video owner database is unavailable")
	}
	if err := validateGrokVideoCreateRecord(record, false); err != nil || accountID <= 0 {
		if err != nil {
			return false, err
		}
		return false, errors.New("grok video create account release is invalid")
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE grok_video_create_idempotency
		SET account_id = NULL, updated_at = NOW()
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3
		  AND endpoint = $4 AND idempotency_key_hash = $5
		  AND request_hash = $6 AND upstream_idempotency_key = $7
		  AND status = 'processing' AND account_id = $8 AND expires_at > NOW()
	`, record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
		record.RequestHash, record.UpstreamIdempotencyKey, accountID)
	if err != nil {
		return false, fmt.Errorf("release grok video create account: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read grok video create account release: %w", err)
	}
	return affected == 1, nil
}

func (r *grokMediaVideoOwnerRepository) CompleteVideoCreate(
	ctx context.Context,
	record service.GrokMediaVideoCreateRecord,
	owner service.GrokMediaVideoRequestOwner,
) error {
	if r == nil || r.db == nil {
		return errors.New("grok video owner database is unavailable")
	}
	if err := validateGrokVideoCreateRecord(record, true); err != nil {
		return err
	}
	owner.RequestID = strings.TrimSpace(owner.RequestID)
	if owner.RequestID == "" || owner.UserID != record.UserID || owner.APIKeyID != record.APIKeyID || owner.GroupID != record.GroupID || owner.AccountID != record.AccountID || owner.ExpiresAt.IsZero() {
		return errors.New("grok video create completion owner is invalid")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin grok video create completion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var completedAccountID int64
	err = tx.QueryRowContext(ctx, `
		UPDATE grok_video_create_idempotency
		SET status = 'completed', request_id = $8, response_status = $9,
			response_content_type = $10, response_body = $11,
			expires_at = GREATEST(expires_at, $12), updated_at = NOW()
		WHERE user_id = $1 AND api_key_id = $2 AND group_id = $3
		  AND endpoint = $4 AND idempotency_key_hash = $5
		  AND request_hash = $6 AND upstream_idempotency_key = $7
		  AND account_id = $13 AND expires_at > NOW()
		  AND (status = 'processing' OR (status = 'completed' AND request_id = $8))
		RETURNING account_id
	`, record.UserID, record.APIKeyID, record.GroupID, string(record.Endpoint), record.IdempotencyKeyHash,
		record.RequestHash, record.UpstreamIdempotencyKey, record.RequestID, record.ResponseStatus,
		record.ResponseContentType, record.ResponseBody, record.ExpiresAt.UTC(), record.AccountID).Scan(&completedAccountID)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrGrokMediaVideoIdempotencyConflict
	}
	if err != nil {
		return fmt.Errorf("persist grok video create response: %w", err)
	}
	if completedAccountID != record.AccountID {
		return service.ErrGrokMediaVideoIdempotencyConflict
	}

	var ownerAccountID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO grok_video_request_owners (
			request_id, user_id, api_key_id, group_id, account_id, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (request_id, user_id, api_key_id, group_id)
		DO UPDATE SET
			expires_at = GREATEST(grok_video_request_owners.expires_at, EXCLUDED.expires_at),
			updated_at = NOW()
		WHERE grok_video_request_owners.account_id = EXCLUDED.account_id
		RETURNING account_id
	`, owner.RequestID, owner.UserID, owner.APIKeyID, owner.GroupID, owner.AccountID, owner.ExpiresAt.UTC()).Scan(&ownerAccountID)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrGrokMediaVideoRequestOwnerConflict
	}
	if err != nil {
		return fmt.Errorf("persist completed grok video request owner: %w", err)
	}
	if ownerAccountID != owner.AccountID {
		return service.ErrGrokMediaVideoRequestOwnerConflict
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit grok video create completion: %w", err)
	}
	return nil
}

type grokVideoCreateScanner interface {
	Scan(dest ...any) error
}

func scanGrokVideoCreateRecord(scanner grokVideoCreateScanner) (*service.GrokMediaVideoCreateRecord, error) {
	var record service.GrokMediaVideoCreateRecord
	var endpoint string
	var accountID sql.NullInt64
	var requestID, responseContentType sql.NullString
	var responseStatus sql.NullInt64
	var responseBody []byte
	if err := scanner.Scan(
		&record.UserID, &record.APIKeyID, &record.GroupID, &endpoint, &record.IdempotencyKeyHash,
		&record.RequestHash, &record.UpstreamIdempotencyKey, &accountID, &requestID,
		&responseStatus, &responseContentType, &responseBody, &record.ExpiresAt,
	); err != nil {
		return nil, err
	}
	record.Endpoint = service.GrokMediaEndpoint(endpoint)
	if accountID.Valid {
		record.AccountID = accountID.Int64
	}
	if requestID.Valid {
		record.RequestID = requestID.String
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

func validateGrokVideoCreateRecord(record service.GrokMediaVideoCreateRecord, completed bool) error {
	if record.UserID <= 0 || record.APIKeyID <= 0 || record.GroupID < 0 || !record.Endpoint.IsVideoGenerationRequest() ||
		len(record.IdempotencyKeyHash) != 64 || len(record.RequestHash) != 64 || strings.TrimSpace(record.UpstreamIdempotencyKey) == "" || record.ExpiresAt.IsZero() {
		return errors.New("grok video create idempotency record is invalid")
	}
	if completed && (record.AccountID <= 0 || strings.TrimSpace(record.RequestID) == "" || record.ResponseStatus < 200 || record.ResponseStatus >= 300 || record.ResponseBody == nil) {
		return errors.New("grok video create completion is invalid")
	}
	return nil
}
