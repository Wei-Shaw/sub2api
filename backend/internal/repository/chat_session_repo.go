package repository

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type chatSessionRepository struct {
	sql          *sql.DB
	payloadStore *chatSessionPayloadStore
}

func NewChatSessionRepository(sqlDB *sql.DB, cfg *config.Config) service.ChatSessionRepository {
	return &chatSessionRepository{
		sql:          sqlDB,
		payloadStore: newChatSessionPayloadStore(cfg),
	}
}

func (r *chatSessionRepository) CreateSessionWithMessages(ctx context.Context, input *service.ChatSessionRecordInput) error {
	if r == nil || r.sql == nil || input == nil || (len(input.Messages) == 0 && len(input.Events) == 0) {
		return nil
	}

	tx, err := r.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	userPreview, assistantPreview := buildChatSessionPreviews(input.Messages)
	sessionID, err := r.ensureSession(ctx, tx, input, userPreview, assistantPreview)
	if err != nil {
		return err
	}

	messageBaseSeq, err := r.nextMessageSeq(ctx, tx, "chat_messages", sessionID)
	if err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO chat_messages (
			session_id, seq, role, direction, content_text, content_json, dedupe_hash, created_at
		) VALUES (
			$1, $2, $3::text, $4::text, $5::text, $6::jsonb,
			encode(
				digest(
					length(trim($3::text))::text || ':' || trim($3::text) || '|' ||
					length(trim($4::text))::text || ':' || trim($4::text) || '|' ||
					length(trim($5::text))::text || ':' || trim($5::text) || '|' ||
					length(
						CASE
							WHEN $6::jsonb ->> 'storage' = 'file' THEN COALESCE(NULLIF($6::jsonb ->> 'sha256', ''), $6::jsonb::text)
							ELSE COALESCE($6::jsonb::text, '')
						END
					)::text || ':' ||
					CASE
						WHEN $6::jsonb ->> 'storage' = 'file' THEN COALESCE(NULLIF($6::jsonb ->> 'sha256', ''), $6::jsonb::text)
						ELSE COALESCE($6::jsonb::text, '')
					END,
					'sha256'
				),
				'hex'
			),
			$7
		)
		ON CONFLICT (session_id, dedupe_hash)
		WHERE dedupe_hash IS NOT NULL
		DO NOTHING
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	insertedMessages := 0
	for _, msg := range input.Messages {
		role := strings.TrimSpace(msg.Role)
		direction := strings.TrimSpace(msg.Direction)
		contentText := strings.TrimSpace(msg.ContentText)
		var contentJSON any
		if len(msg.ContentJSON) > 0 {
			contentJSON = r.prepareContentJSON(ctx, msg.ContentJSON, input.CreatedAt, "message")
		}
		result, err := stmt.ExecContext(
			ctx,
			sessionID,
			messageBaseSeq+insertedMessages,
			role,
			direction,
			contentText,
			contentJSON,
			input.CreatedAt,
		)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected > 0 {
			insertedMessages++
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE chat_sessions
		SET message_count = (
			SELECT COUNT(*)
			FROM chat_messages
			WHERE session_id = $1
		)
		WHERE id = $1
	`, sessionID); err != nil {
		return err
	}

	if len(input.Events) > 0 {
		eventBaseSeq, err := r.nextMessageSeq(ctx, tx, "chat_message_events", sessionID)
		if err != nil {
			return err
		}
		eventStmt, err := tx.PrepareContext(ctx, `
			INSERT INTO chat_message_events (
				session_id, seq, kind, role, direction, content_text, content_json, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`)
		if err != nil {
			return err
		}
		defer eventStmt.Close()

		for i, ev := range input.Events {
			var contentJSON any
			if len(ev.ContentJSON) > 0 {
				contentJSON = r.prepareContentJSON(ctx, ev.ContentJSON, input.CreatedAt, "event")
			}
			if _, err := eventStmt.ExecContext(
				ctx,
				sessionID,
				eventBaseSeq+i,
				strings.TrimSpace(ev.Kind),
				strings.TrimSpace(ev.Role),
				strings.TrimSpace(ev.Direction),
				strings.TrimSpace(ev.ContentText),
				contentJSON,
				input.CreatedAt,
			); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func (r *chatSessionRepository) ensureSession(
	ctx context.Context,
	tx *sql.Tx,
	input *service.ChatSessionRecordInput,
	userPreview *string,
	assistantPreview *string,
) (int64, error) {
	sessionKey := strings.TrimSpace(input.SessionKey)
	if sessionKey != "" {
		if err := r.lockSessionKey(ctx, tx, input.UserID, input.APIKeyID, sessionKey); err != nil {
			return 0, err
		}
		var existingID int64
		err := tx.QueryRowContext(ctx, `
			SELECT id
			FROM chat_sessions
			WHERE user_id = $1 AND api_key_id = $2 AND session_key = $3
			ORDER BY id DESC
			LIMIT 1
		`, input.UserID, input.APIKeyID, sessionKey).Scan(&existingID)
		if err == nil {
			_, updateErr := tx.ExecContext(ctx, `
				UPDATE chat_sessions
				SET
					request_id = COALESCE(NULLIF($2, ''), request_id),
					account_id = COALESCE($3, account_id),
					group_id = COALESCE($4, group_id),
					platform = CASE WHEN $5 <> '' THEN $5 ELSE platform END,
					model = CASE WHEN $6 <> '' THEN $6 ELSE model END,
					requested_model = COALESCE($7, requested_model),
					upstream_model = COALESCE($8, upstream_model),
					inbound_endpoint = COALESCE($9, inbound_endpoint),
					upstream_endpoint = COALESCE($10, upstream_endpoint),
					request_type = $11,
					stream = $12,
					status = CASE WHEN $13 <> '' THEN $13 ELSE status END,
					http_status_code = CASE WHEN $14 > 0 THEN $14 ELSE http_status_code END,
					user_preview = COALESCE($15, user_preview),
					assistant_preview = COALESCE($16, assistant_preview),
					message_count = message_count + $17,
					created_at = GREATEST(created_at, $18)
				WHERE id = $1
			`,
				existingID,
				strings.TrimSpace(input.RequestID),
				nullableInt64(input.AccountID),
				nullableInt64(input.GroupID),
				strings.TrimSpace(input.Platform),
				strings.TrimSpace(input.Model),
				nullableString(input.RequestedModel),
				nullableString(input.UpstreamModel),
				nullableString(input.InboundEndpoint),
				nullableString(input.UpstreamEndpoint),
				int16(input.RequestType.Normalize()),
				input.Stream,
				strings.TrimSpace(input.Status),
				input.HTTPStatusCode,
				nullableString(userPreview),
				nullableString(assistantPreview),
				len(input.Messages),
				input.CreatedAt,
			)
			return existingID, updateErr
		}
		if err != nil && err != sql.ErrNoRows {
			return 0, err
		}
	}

	var sessionID int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO chat_sessions (
			session_key,
			request_id,
			user_id,
			api_key_id,
			account_id,
			group_id,
			platform,
			model,
			requested_model,
			upstream_model,
			inbound_endpoint,
			upstream_endpoint,
			request_type,
			stream,
			status,
			http_status_code,
			user_preview,
			assistant_preview,
			message_count,
			created_at
		) VALUES (
			NULLIF($1, ''),
			NULLIF($2, ''),
			$3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		)
		RETURNING id
	`,
		sessionKey,
		strings.TrimSpace(input.RequestID),
		input.UserID,
		input.APIKeyID,
		nullableInt64(input.AccountID),
		nullableInt64(input.GroupID),
		strings.TrimSpace(input.Platform),
		strings.TrimSpace(input.Model),
		nullableString(input.RequestedModel),
		nullableString(input.UpstreamModel),
		nullableString(input.InboundEndpoint),
		nullableString(input.UpstreamEndpoint),
		int16(input.RequestType.Normalize()),
		input.Stream,
		strings.TrimSpace(input.Status),
		input.HTTPStatusCode,
		nullableString(userPreview),
		nullableString(assistantPreview),
		len(input.Messages),
		input.CreatedAt,
	).Scan(&sessionID)
	return sessionID, err
}

func (r *chatSessionRepository) lockSessionKey(ctx context.Context, tx *sql.Tx, userID, apiKeyID int64, sessionKey string) error {
	_, err := tx.ExecContext(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
	`, strconv.FormatInt(userID, 10)+":"+strconv.FormatInt(apiKeyID, 10)+":"+sessionKey)
	return err
}

func (r *chatSessionRepository) nextMessageSeq(ctx context.Context, tx *sql.Tx, table string, sessionID int64) (int, error) {
	query := "SELECT COALESCE(MAX(seq), 0) FROM " + table + " WHERE session_id = $1"
	var maxSeq int
	if err := tx.QueryRowContext(ctx, query, sessionID).Scan(&maxSeq); err != nil {
		return 0, err
	}
	return maxSeq + 1, nil
}

func (r *chatSessionRepository) ListSessionsByAPIKey(ctx context.Context, userID, apiKeyID int64, params pagination.PaginationParams) ([]*service.ChatSession, int64, error) {
	if r == nil || r.sql == nil {
		return []*service.ChatSession{}, 0, nil
	}
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}

	var total int64
	if err := r.sql.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM chat_sessions
		WHERE user_id = $1 AND api_key_id = $2
	`, userID, apiKeyID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.sql.QueryContext(ctx, `
		SELECT
			id, session_key, request_id, user_id, api_key_id, account_id, group_id, platform, model,
			requested_model, upstream_model, inbound_endpoint, upstream_endpoint,
			request_type, stream, status, http_status_code, user_preview,
			assistant_preview, message_count, created_at
		FROM chat_sessions
		WHERE user_id = $1 AND api_key_id = $2
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4
	`, userID, apiKeyID, params.PageSize, (params.Page-1)*params.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]*service.ChatSession, 0, params.PageSize)
	for rows.Next() {
		item, scanErr := scanChatSession(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *chatSessionRepository) GetSessionDetail(ctx context.Context, userID, apiKeyID, sessionID int64, params pagination.PaginationParams) (*service.ChatSessionDetail, error) {
	if r == nil || r.sql == nil {
		return nil, sql.ErrNoRows
	}
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}

	row := r.sql.QueryRowContext(ctx, `
		SELECT
			id, session_key, request_id, user_id, api_key_id, account_id, group_id, platform, model,
			requested_model, upstream_model, inbound_endpoint, upstream_endpoint,
			request_type, stream, status, http_status_code, user_preview,
			assistant_preview, message_count, created_at
		FROM chat_sessions
		WHERE id = $1 AND user_id = $2 AND api_key_id = $3
	`, sessionID, userID, apiKeyID)
	session, err := scanChatSession(row)
	if err != nil {
		return nil, err
	}

	offset := (params.Page - 1) * params.PageSize
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, session_id, seq, role, direction, content_text, has_content_json, content_json_bytes, created_at
		FROM (
			SELECT
				id,
				session_id,
				seq,
				role,
				direction,
				content_text,
				content_json IS NOT NULL AS has_content_json,
				COALESCE(pg_column_size(content_json), 0)::BIGINT AS content_json_bytes,
				created_at
			FROM chat_messages
			WHERE session_id = $1
			ORDER BY seq DESC, id DESC
			LIMIT $2 OFFSET $3
		) m
		ORDER BY seq ASC, id ASC
	`, sessionID, params.PageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]service.ChatMessage, 0, params.PageSize)
	for rows.Next() {
		var msg service.ChatMessage
		if err := rows.Scan(
			&msg.ID,
			&msg.SessionID,
			&msg.Seq,
			&msg.Role,
			&msg.Direction,
			&msg.ContentText,
			&msg.HasContentJSON,
			&msg.ContentJSONBytes,
			&msg.CreatedAt,
		); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	pages := 1
	if params.PageSize > 0 {
		pages = int((int64(session.MessageCount) + int64(params.PageSize) - 1) / int64(params.PageSize))
		if pages < 1 {
			pages = 1
		}
	}
	return &service.ChatSessionDetail{
		ChatSession: *session,
		Messages:    messages,
		MessagesPage: service.ChatMessagePageData{
			Items:    messages,
			Total:    int64(session.MessageCount),
			Page:     params.Page,
			PageSize: params.PageSize,
			Pages:    pages,
		},
	}, nil
}

func (r *chatSessionRepository) GetChatMessageDetail(ctx context.Context, userID, apiKeyID, sessionID, messageID int64) (*service.ChatMessage, error) {
	if r == nil || r.sql == nil {
		return nil, sql.ErrNoRows
	}
	row := r.sql.QueryRowContext(ctx, `
		SELECT
			m.id,
			m.session_id,
			m.seq,
			m.role,
			m.direction,
			m.content_text,
			m.content_json,
			m.content_json IS NOT NULL AS has_content_json,
			COALESCE(pg_column_size(m.content_json), 0)::BIGINT AS content_json_bytes,
			m.created_at
		FROM chat_messages m
		INNER JOIN chat_sessions s ON s.id = m.session_id
		WHERE m.id = $1 AND m.session_id = $2 AND s.user_id = $3 AND s.api_key_id = $4
	`, messageID, sessionID, userID, apiKeyID)

	var msg service.ChatMessage
	var contentJSON []byte
	if err := row.Scan(
		&msg.ID,
		&msg.SessionID,
		&msg.Seq,
		&msg.Role,
		&msg.Direction,
		&msg.ContentText,
		&contentJSON,
		&msg.HasContentJSON,
		&msg.ContentJSONBytes,
		&msg.CreatedAt,
	); err != nil {
		return nil, err
	}
	msg.ContentJSON = r.resolveContentJSON(ctx, contentJSON)
	msg.HasContentJSON = len(msg.ContentJSON) > 0
	msg.ContentJSONBytes = int64(len(msg.ContentJSON))
	return &msg, nil
}

func (r *chatSessionRepository) ListRecentMessagesByAPIKey(ctx context.Context, userID, apiKeyID int64, limit int) ([]service.ChatMessage, error) {
	if r == nil || r.sql == nil {
		return []service.ChatMessage{}, nil
	}

	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, session_id, seq, role, direction, content_text, content_json, created_at
		FROM (
			SELECT
				m.id,
				m.session_id,
				m.seq,
				m.role,
				m.direction,
				m.content_text,
				m.content_json,
				m.created_at
			FROM chat_messages m
			INNER JOIN chat_sessions s ON s.id = m.session_id
			WHERE s.user_id = $1 AND s.api_key_id = $2
			ORDER BY m.created_at DESC, m.id DESC
			LIMIT $3
		) x
		ORDER BY created_at ASC, id ASC
	`, userID, apiKeyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]service.ChatMessage, 0, limit)
	for rows.Next() {
		var msg service.ChatMessage
		var contentJSON []byte
		if err := rows.Scan(
			&msg.ID,
			&msg.SessionID,
			&msg.Seq,
			&msg.Role,
			&msg.Direction,
			&msg.ContentText,
			&contentJSON,
			&msg.CreatedAt,
		); err != nil {
			return nil, err
		}
		msg.ContentJSON = r.resolveContentJSON(ctx, contentJSON)
		out = append(out, msg)
	}
	return out, rows.Err()
}

type chatSessionScanner interface {
	Scan(dest ...any) error
}

func scanChatSession(scanner chatSessionScanner) (*service.ChatSession, error) {
	var item service.ChatSession
	var sessionKey sql.NullString
	var requestID sql.NullString
	var accountID sql.NullInt64
	var groupID sql.NullInt64
	var requestedModel sql.NullString
	var upstreamModel sql.NullString
	var inboundEndpoint sql.NullString
	var upstreamEndpoint sql.NullString
	var userPreview sql.NullString
	var assistantPreview sql.NullString
	var requestType int16
	err := scanner.Scan(
		&item.ID,
		&sessionKey,
		&requestID,
		&item.UserID,
		&item.APIKeyID,
		&accountID,
		&groupID,
		&item.Platform,
		&item.Model,
		&requestedModel,
		&upstreamModel,
		&inboundEndpoint,
		&upstreamEndpoint,
		&requestType,
		&item.Stream,
		&item.Status,
		&item.HTTPStatusCode,
		&userPreview,
		&assistantPreview,
		&item.MessageCount,
		&item.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if requestID.Valid {
		v := requestID.String
		item.RequestID = &v
	}
	if sessionKey.Valid {
		v := sessionKey.String
		item.SessionKey = &v
	}
	if accountID.Valid {
		v := accountID.Int64
		item.AccountID = &v
	}
	if groupID.Valid {
		v := groupID.Int64
		item.GroupID = &v
	}
	if requestedModel.Valid {
		v := requestedModel.String
		item.RequestedModel = &v
	}
	if upstreamModel.Valid {
		v := upstreamModel.String
		item.UpstreamModel = &v
	}
	if inboundEndpoint.Valid {
		v := inboundEndpoint.String
		item.InboundEndpoint = &v
	}
	if upstreamEndpoint.Valid {
		v := upstreamEndpoint.String
		item.UpstreamEndpoint = &v
	}
	if userPreview.Valid {
		v := userPreview.String
		item.UserPreview = &v
	}
	if assistantPreview.Valid {
		v := assistantPreview.String
		item.AssistantPreview = &v
	}
	item.RequestType = service.RequestTypeFromInt16(requestType)
	return &item, nil
}

func buildChatSessionPreviews(messages []service.ChatMessageRecordInput) (*string, *string) {
	var userPreview *string
	var assistantPreview *string
	for _, msg := range messages {
		text := truncateChatPreview(strings.TrimSpace(msg.ContentText), 160)
		if text == "" {
			continue
		}
		switch strings.TrimSpace(msg.Direction) {
		case "inbound":
			userPreview = &text
		case "outbound":
			assistantPreview = &text
		}
	}
	return userPreview, assistantPreview
}

func truncateChatPreview(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxLen <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	return string(runes[:maxLen]) + "..."
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func (r *chatSessionRepository) DeleteSessionsBefore(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if r == nil || r.sql == nil || cutoff.IsZero() {
		return 0, nil
	}
	if limit <= 0 {
		limit = 1000
	}

	result, err := r.sql.ExecContext(ctx, `
		DELETE FROM chat_sessions
		WHERE id IN (
			SELECT id
			FROM chat_sessions
			WHERE created_at < $1
			ORDER BY created_at ASC, id ASC
			LIMIT $2
		)
	`, cutoff, limit)
	if err != nil {
		return 0, err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if deleted > 0 && r.payloadStore != nil {
		r.payloadStore.DeleteDateDirsBefore(cutoff)
	}
	return deleted, nil
}

type chatSessionPayloadStore struct {
	baseDir        string
	inlineMaxBytes int
}

type chatSessionPayloadRef struct {
	Storage          string `json:"storage"`
	Path             string `json:"path"`
	SHA256           string `json:"sha256"`
	Bytes            int64  `json:"bytes"`
	StoredBytes      int64  `json:"stored_bytes,omitempty"`
	Compression      string `json:"compression,omitempty"`
	CompressionLevel int    `json:"compression_level,omitempty"`
}

const (
	chatSessionPayloadStorageFile = "file"
	chatSessionPayloadCompression = "gzip"
)

func newChatSessionPayloadStore(cfg *config.Config) *chatSessionPayloadStore {
	retention := config.ChatSessionRetentionConfig{
		PayloadDir:            "./data/chat_session_payloads",
		PayloadInlineMaxBytes: 256 * 1024,
	}
	if cfg != nil {
		retention = cfg.ChatSessionRetention
	}
	baseDir := strings.TrimSpace(retention.PayloadDir)
	if baseDir == "" {
		baseDir = "./data/chat_session_payloads"
	}
	return &chatSessionPayloadStore{
		baseDir:        filepath.Clean(baseDir),
		inlineMaxBytes: retention.PayloadInlineMaxBytes,
	}
}

func (r *chatSessionRepository) prepareContentJSON(ctx context.Context, raw json.RawMessage, createdAt time.Time, kind string) any {
	if len(raw) == 0 || r == nil || r.payloadStore == nil {
		return raw
	}
	stored, err := r.payloadStore.Store(ctx, raw, createdAt, kind)
	if err != nil {
		return raw
	}
	return stored
}

func (r *chatSessionRepository) resolveContentJSON(ctx context.Context, raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	if r == nil || r.payloadStore == nil {
		return json.RawMessage(raw)
	}
	resolved, err := r.payloadStore.Load(ctx, raw)
	if err != nil {
		return json.RawMessage(raw)
	}
	return resolved
}

func (s *chatSessionPayloadStore) Store(ctx context.Context, raw json.RawMessage, createdAt time.Time, kind string) (any, error) {
	if s == nil || len(raw) == 0 || s.inlineMaxBytes < 0 || len(raw) <= s.inlineMaxBytes {
		return raw, nil
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	sum := sha256.Sum256(raw)
	sumHex := hex.EncodeToString(sum[:])
	compressed, err := gzipChatSessionPayload(raw)
	if err != nil {
		return nil, err
	}
	dateDir := createdAt.UTC().Format("2006-01-02")
	relPath := filepath.ToSlash(filepath.Join(dateDir, sumHex+"-"+randomHex(8)+"-"+sanitizeChatSessionPayloadKind(kind)+".json.gz"))
	fullPath, err := s.safePath(relPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0750); err != nil {
		return nil, err
	}
	if err := os.WriteFile(fullPath, compressed, 0640); err != nil {
		return nil, err
	}
	return chatSessionPayloadRef{
		Storage:          chatSessionPayloadStorageFile,
		Path:             relPath,
		SHA256:           sumHex,
		Bytes:            int64(len(raw)),
		StoredBytes:      int64(len(compressed)),
		Compression:      chatSessionPayloadCompression,
		CompressionLevel: gzip.BestSpeed,
	}, nil
}

func (s *chatSessionPayloadStore) Load(ctx context.Context, raw []byte) (json.RawMessage, error) {
	ref, ok := decodeChatSessionPayloadRef(raw)
	if !ok {
		return json.RawMessage(raw), nil
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	fullPath, err := s.safePath(ref.Path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}
	if ref.Compression == chatSessionPayloadCompression {
		data, err = gunzipChatSessionPayload(data)
		if err != nil {
			return nil, err
		}
	}
	if ref.SHA256 != "" {
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != ref.SHA256 {
			return nil, errors.New("chat session payload sha256 mismatch")
		}
	}
	return json.RawMessage(data), nil
}

func (s *chatSessionPayloadStore) DeleteDateDirsBefore(cutoff time.Time) {
	if s == nil || strings.TrimSpace(s.baseDir) == "" || cutoff.IsZero() {
		return
	}
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return
	}
	cutoffDay := time.Date(cutoff.UTC().Year(), cutoff.UTC().Month(), cutoff.UTC().Day(), 0, 0, 0, 0, time.UTC)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		day, err := time.Parse("2006-01-02", entry.Name())
		if err != nil || !day.Before(cutoffDay) {
			continue
		}
		fullPath, err := s.safePath(entry.Name())
		if err != nil {
			continue
		}
		_ = os.RemoveAll(fullPath)
	}
}

func (s *chatSessionPayloadStore) safePath(relPath string) (string, error) {
	if s == nil || strings.TrimSpace(s.baseDir) == "" {
		return "", errors.New("chat session payload store not configured")
	}
	cleanRel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(relPath)))
	if cleanRel == "." || filepath.IsAbs(cleanRel) || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
		return "", errors.New("invalid chat session payload path")
	}
	baseAbs, err := filepath.Abs(s.baseDir)
	if err != nil {
		return "", err
	}
	fullPath := filepath.Join(baseAbs, cleanRel)
	fullAbs, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	if fullAbs != baseAbs && !strings.HasPrefix(fullAbs, baseAbs+string(filepath.Separator)) {
		return "", errors.New("chat session payload path escapes base dir")
	}
	return fullAbs, nil
}

func decodeChatSessionPayloadRef(raw []byte) (chatSessionPayloadRef, bool) {
	var ref chatSessionPayloadRef
	if len(raw) == 0 || json.Unmarshal(raw, &ref) != nil {
		return chatSessionPayloadRef{}, false
	}
	if strings.TrimSpace(ref.Storage) != chatSessionPayloadStorageFile || strings.TrimSpace(ref.Path) == "" {
		return chatSessionPayloadRef{}, false
	}
	ref.Path = strings.TrimSpace(ref.Path)
	ref.SHA256 = strings.TrimSpace(ref.SHA256)
	return ref, true
}

func gzipChatSessionPayload(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzipChatSessionPayload(raw []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func sanitizeChatSessionPayloadKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "message", "event":
		return kind
	default:
		return "payload"
	}
}

func randomHex(n int) string {
	if n <= 0 {
		n = 8
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(buf)
}

var _ service.ChatSessionRepository = (*chatSessionRepository)(nil)
