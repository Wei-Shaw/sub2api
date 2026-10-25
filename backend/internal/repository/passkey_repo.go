package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/lib/pq"
)

type passkeyRepository struct {
	db *sql.DB
}

func NewPasskeyRepository(db *sql.DB) service.PasskeyRepository {
	return &passkeyRepository{db: db}
}

func (r *passkeyRepository) EnsureUserHandle(
	ctx context.Context,
	userID int64,
	candidate []byte,
) ([]byte, error) {
	if len(candidate) < 16 || len(candidate) > 64 {
		return nil, fmt.Errorf("passkey user handle must contain 16-64 bytes")
	}
	ensureSQL := `
		INSERT INTO passkey_user_handles (user_id, user_handle)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO NOTHING
	`
	if IsMySQL() {
		// PG 的 ON CONFLICT (user_id) DO NOTHING 只吞 user_id 主键冲突；
		// user_handle 唯一冲突仍应报错（PG 报 23505）。MySQL 拆为裸 INSERT +
		// 主键冲突静默（对齐 DO NOTHING），非主键冲突原样返回。
		ensureSQL = `
		INSERT INTO passkey_user_handles (user_id, user_handle)
		VALUES (?, ?)
	`
	}
	if _, err := r.db.ExecContext(ctx, ensureSQL, userID, candidate); err != nil {
		if IsMySQL() && isMySQLUniqueViolation(err) && isMySQLPrimaryKeyDuplicate(err) {
			// user_id 已存在：等同 DO NOTHING，继续读取既有 handle。
		} else {
			return nil, fmt.Errorf("ensure passkey user handle: %w", err)
		}
	}
	return r.GetUserHandle(ctx, userID)
}

func (r *passkeyRepository) GetUserHandle(ctx context.Context, userID int64) ([]byte, error) {
	query := `
		SELECT user_handle
		FROM passkey_user_handles
		WHERE user_id = $1
	`
	if IsMySQL() {
		query = `
		SELECT user_handle
		FROM passkey_user_handles
		WHERE user_id = ?
		`
	}
	var handle []byte
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&handle)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrPasskeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get passkey user handle: %w", err)
	}
	return handle, nil
}

func (r *passkeyRepository) GetByCredentialID(
	ctx context.Context,
	credentialID []byte,
) (*service.PasskeyCredentialRecord, error) {
	credentialQuery := `
		SELECT c.id, c.user_id, h.user_handle, c.name, c.credential_data,
		       c.last_used_at, c.created_at, c.updated_at
		FROM passkey_credentials c
		JOIN passkey_user_handles h ON h.user_id = c.user_id
		WHERE c.credential_id = $1
	`
	if IsMySQL() {
		credentialQuery = `
		SELECT c.id, c.user_id, h.user_handle, c.name, c.credential_data,
		       c.last_used_at, c.created_at, c.updated_at
		FROM passkey_credentials c
		JOIN passkey_user_handles h ON h.user_id = c.user_id
		WHERE c.credential_id = ?
		`
	}
	row := r.db.QueryRowContext(ctx, credentialQuery, credentialID)
	record, err := scanPasskeyCredential(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrPasskeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get passkey credential: %w", err)
	}
	return record, nil
}

func (r *passkeyRepository) ListByUserID(
	ctx context.Context,
	userID int64,
) ([]service.PasskeyCredentialRecord, error) {
	listQuery := `
		SELECT c.id, c.user_id, h.user_handle, c.name, c.credential_data,
		       c.last_used_at, c.created_at, c.updated_at
		FROM passkey_credentials c
		JOIN passkey_user_handles h ON h.user_id = c.user_id
		WHERE c.user_id = $1
		ORDER BY c.created_at DESC, c.id DESC
	`
	if IsMySQL() {
		listQuery = `
		SELECT c.id, c.user_id, h.user_handle, c.name, c.credential_data,
		       c.last_used_at, c.created_at, c.updated_at
		FROM passkey_credentials c
		JOIN passkey_user_handles h ON h.user_id = c.user_id
		WHERE c.user_id = ?
		ORDER BY c.created_at DESC, c.id DESC
		`
	}
	rows, err := r.db.QueryContext(ctx, listQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("list passkey credentials: %w", err)
	}
	defer func() { _ = rows.Close() }()

	records := make([]service.PasskeyCredentialRecord, 0)
	for rows.Next() {
		record, scanErr := scanPasskeyCredential(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan passkey credential: %w", scanErr)
		}
		records = append(records, *record)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("list passkey credentials: %w", err)
	}
	return records, nil
}

func (r *passkeyRepository) Create(
	ctx context.Context,
	record *service.PasskeyCredentialRecord,
) (*service.PasskeyCredentialRecord, error) {
	if record == nil || len(record.Credential.ID) == 0 {
		return nil, fmt.Errorf("passkey credential is required")
	}
	credentialJSON, err := json.Marshal(record.Credential)
	if err != nil {
		return nil, fmt.Errorf("encode passkey credential: %w", err)
	}
	name := strings.TrimSpace(record.Name)
	if name == "" {
		name = "Passkey"
	}
	created := &service.PasskeyCredentialRecord{
		UserID:     record.UserID,
		UserHandle: append([]byte(nil), record.UserHandle...),
		Name:       name,
		Credential: record.Credential,
	}
	if IsMySQL() {
		return r.createMySQL(ctx, record, created, name, credentialJSON)
	}
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO passkey_credentials
		    (user_id, credential_id, name, credential_data)
		VALUES ($1, $2, $3, $4::jsonb)
		RETURNING id, name, created_at, updated_at
	`, record.UserID, record.Credential.ID, name, string(credentialJSON)).
		Scan(&created.ID, &created.Name, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return nil, service.ErrPasskeyExists
		}
		return nil, fmt.Errorf("create passkey credential: %w", err)
	}
	return created, nil
}

// createMySQL 是 Create 的 MySQL 实现：PG 版的 `$4::jsonb` → CAST(? AS JSON)、
// `RETURNING id, name, created_at, updated_at` → LastInsertId（name 应用侧已知）+
// 回查 created_at/updated_at（两列由数据库 CURRENT_TIMESTAMP(3) 生成）；
// pq.Error 23505 判定 → isMySQLUniqueViolation（1062）。
func (r *passkeyRepository) createMySQL(ctx context.Context, record *service.PasskeyCredentialRecord, created *service.PasskeyCredentialRecord, name string, credentialJSON []byte) (*service.PasskeyCredentialRecord, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO passkey_credentials
		    (user_id, credential_id, name, credential_data)
		VALUES (?, ?, ?, CAST(? AS JSON))
	`, record.UserID, record.Credential.ID, name, string(credentialJSON))
	if err != nil {
		if isMySQLUniqueViolation(err) {
			return nil, service.ErrPasskeyExists
		}
		return nil, fmt.Errorf("create passkey credential: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create passkey credential: %w", err)
	}
	created.ID = id
	created.Name = name
	if err := r.db.QueryRowContext(ctx, `
		SELECT created_at, updated_at FROM passkey_credentials WHERE id = ?
	`, id).Scan(&created.CreatedAt, &created.UpdatedAt); err != nil {
		return nil, fmt.Errorf("create passkey credential: %w", err)
	}
	return created, nil
}

func (r *passkeyRepository) UpdateCredential(
	ctx context.Context,
	userID int64,
	credential *webauthn.Credential,
	usedAt time.Time,
) error {
	if credential == nil || len(credential.ID) == 0 {
		return fmt.Errorf("passkey credential is required")
	}
	credentialJSON, err := json.Marshal(credential)
	if err != nil {
		return fmt.Errorf("encode passkey credential: %w", err)
	}
	updateQuery := `
		UPDATE passkey_credentials
		SET credential_data = $3::jsonb, last_used_at = $4, updated_at = NOW()
		WHERE user_id = $1 AND credential_id = $2
	`
	if IsMySQL() {
		updateQuery = `
		UPDATE passkey_credentials
		SET credential_data = CAST(? AS JSON), last_used_at = ?, updated_at = NOW(3)
		WHERE user_id = ? AND credential_id = ?
		`
	}
	result, err := r.db.ExecContext(ctx, updateQuery, userID, credential.ID, string(credentialJSON), usedAt.UTC())
	if err != nil {
		return fmt.Errorf("update passkey credential: %w", err)
	}
	return requirePasskeyAffected(result)
}

func (r *passkeyRepository) Rename(
	ctx context.Context,
	userID, credentialID int64,
	name string,
) error {
	renameQuery := `
		UPDATE passkey_credentials
		SET name = $3, updated_at = NOW()
		WHERE user_id = $1 AND id = $2
	`
	if IsMySQL() {
		renameQuery = `
		UPDATE passkey_credentials
		SET name = ?, updated_at = NOW(3)
		WHERE user_id = ? AND id = ?
		`
	}
	result, err := r.db.ExecContext(ctx, renameQuery, userID, credentialID, name)
	if err != nil {
		return fmt.Errorf("rename passkey credential: %w", err)
	}
	return requirePasskeyAffected(result)
}

func (r *passkeyRepository) Delete(
	ctx context.Context,
	userID, credentialID int64,
) error {
	deleteQuery := `
		DELETE FROM passkey_credentials
		WHERE user_id = $1 AND id = $2
	`
	if IsMySQL() {
		deleteQuery = `
		DELETE FROM passkey_credentials
		WHERE user_id = ? AND id = ?
		`
	}
	result, err := r.db.ExecContext(ctx, deleteQuery, userID, credentialID)
	if err != nil {
		return fmt.Errorf("delete passkey credential: %w", err)
	}
	return requirePasskeyAffected(result)
}

type passkeyScanner interface {
	Scan(dest ...any) error
}

func scanPasskeyCredential(scanner passkeyScanner) (*service.PasskeyCredentialRecord, error) {
	var (
		record         service.PasskeyCredentialRecord
		credentialJSON []byte
	)
	if err := scanner.Scan(
		&record.ID,
		&record.UserID,
		&record.UserHandle,
		&record.Name,
		&credentialJSON,
		&record.LastUsedAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(credentialJSON, &record.Credential); err != nil {
		return nil, fmt.Errorf("decode passkey credential: %w", err)
	}
	return &record, nil
}

func requirePasskeyAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrPasskeyNotFound
	}
	return nil
}
