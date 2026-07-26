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
REDACTED

func NewPasskeyRepository(db *sql.DB) service.PasskeyRepository {
	return &passkeyRepository{db: dbREDACTED
REDACTED

func (r *passkeyRepository) EnsureUserHandle(
	ctx context.Context,
	userID int64,
	candidate []byte,
) ([]byte, error) {
	if len(candidate) < 16 || len(candidate) > 64 {
		return nil, fmt.Errorf("passkey user handle must contain 16-64 bytes")
REDACTED
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO passkey_user_handles (user_id, user_handle)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO NOTHING
	`, userID, candidate); err != nil {
		return nil, fmt.Errorf("ensure passkey user handle: %w", err)
REDACTED
	return r.GetUserHandle(ctx, userID)
REDACTED

func (r *passkeyRepository) GetUserHandle(ctx context.Context, userID int64) ([]byte, error) {
	var handle []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT user_handle
		FROM passkey_user_handles
		WHERE user_id = $1
	`, userID).Scan(&handle)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrPasskeyNotFound
REDACTED
	if err != nil {
		return nil, fmt.Errorf("get passkey user handle: %w", err)
REDACTED
	return handle, nil
REDACTED

func (r *passkeyRepository) GetByCredentialID(
	ctx context.Context,
	credentialID []byte,
) (*service.PasskeyCredentialRecord, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT c.id, c.user_id, h.user_handle, c.name, c.credential_data,
		       c.last_used_at, c.created_at, c.updated_at
		FROM passkey_credentials c
		JOIN passkey_user_handles h ON h.user_id = c.user_id
		WHERE c.credential_id = $1
	`, credentialID)
	record, err := scanPasskeyCredential(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrPasskeyNotFound
REDACTED
	if err != nil {
		return nil, fmt.Errorf("get passkey credential: %w", err)
REDACTED
	return record, nil
REDACTED

func (r *passkeyRepository) ListByUserID(
	ctx context.Context,
	userID int64,
) ([]service.PasskeyCredentialRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.user_id, h.user_handle, c.name, c.credential_data,
		       c.last_used_at, c.created_at, c.updated_at
		FROM passkey_credentials c
		JOIN passkey_user_handles h ON h.user_id = c.user_id
		WHERE c.user_id = $1
		ORDER BY c.created_at DESC, c.id DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list passkey credentials: %w", err)
REDACTED
	defer rows.Close()

	records := make([]service.PasskeyCredentialRecord, 0)
	for rows.Next() {
		record, scanErr := scanPasskeyCredential(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan passkey credential: %w", scanErr)
	REDACTED
		records = append(records, *record)
REDACTED
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("list passkey credentials: %w", err)
REDACTED
	return records, nil
REDACTED

func (r *passkeyRepository) Create(
	ctx context.Context,
	record *service.PasskeyCredentialRecord,
) (*service.PasskeyCredentialRecord, error) {
	if record == nil || len(record.Credential.ID) == 0 {
		return nil, fmt.Errorf("passkey credential is required")
REDACTED
	credentialJSON, err := json.Marshal(record.Credential)
	if err != nil {
		return nil, fmt.Errorf("encode passkey credential: %w", err)
REDACTED
	name := strings.TrimSpace(record.Name)
	if name == "" {
		name = "Passkey"
REDACTED
	created := &service.PasskeyCredentialRecord{
		UserID:     record.UserID,
		UserHandle: append([]byte(nil), record.UserHandle...),
		Name:       name,
		Credential: record.Credential,
REDACTED
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
	REDACTED
		return nil, fmt.Errorf("create passkey credential: %w", err)
REDACTED
	return created, nil
REDACTED

func (r *passkeyRepository) UpdateCredential(
	ctx context.Context,
	userID int64,
	credential *webauthn.Credential,
	usedAt time.Time,
) error {
	if credential == nil || len(credential.ID) == 0 {
		return fmt.Errorf("passkey credential is required")
REDACTED
	credentialJSON, err := json.Marshal(credential)
	if err != nil {
		return fmt.Errorf("encode passkey credential: %w", err)
REDACTED
	result, err := r.db.ExecContext(ctx, `
		UPDATE passkey_credentials
		SET credential_data = $3::jsonb, last_used_at = $4, updated_at = NOW()
		WHERE user_id = $1 AND credential_id = $2
	`, userID, credential.ID, string(credentialJSON), usedAt.UTC())
	if err != nil {
		return fmt.Errorf("update passkey credential: %w", err)
REDACTED
	return requirePasskeyAffected(result)
REDACTED

func (r *passkeyRepository) Rename(
	ctx context.Context,
	userID, credentialID int64,
	name string,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE passkey_credentials
		SET name = $3, updated_at = NOW()
		WHERE user_id = $1 AND id = $2
	`, userID, credentialID, name)
	if err != nil {
		return fmt.Errorf("rename passkey credential: %w", err)
REDACTED
	return requirePasskeyAffected(result)
REDACTED

func (r *passkeyRepository) Delete(
	ctx context.Context,
	userID, credentialID int64,
) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM passkey_credentials
		WHERE user_id = $1 AND id = $2
	`, userID, credentialID)
	if err != nil {
		return fmt.Errorf("delete passkey credential: %w", err)
REDACTED
	return requirePasskeyAffected(result)
REDACTED

type passkeyScanner interface {
	Scan(dest ...any) error
REDACTED

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
REDACTED
	if err := json.Unmarshal(credentialJSON, &record.Credential); err != nil {
		return nil, fmt.Errorf("decode passkey credential: %w", err)
REDACTED
	return &record, nil
REDACTED

func requirePasskeyAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
REDACTED
	if affected == 0 {
		return service.ErrPasskeyNotFound
REDACTED
	return nil
REDACTED
