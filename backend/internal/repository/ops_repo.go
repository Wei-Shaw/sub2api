package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type opsRepository struct {
	db *sql.DB
}

const insertOpsErrorLogSQL = `
INSERT INTO ops_error_logs (
  request_id,
  client_request_id,
  user_id,
  api_key_id,
  account_id,
  group_id,
  client_ip,
  platform,
  model,
  request_path,
  stream,
  inbound_endpoint,
  upstream_endpoint,
  requested_model,
  upstream_model,
  request_type,
  user_agent,
  error_phase,
  error_type,
  severity,
  status_code,
  is_business_limited,
  is_count_tokens,
  error_message,
  error_body,
  error_source,
  error_owner,
  upstream_status_code,
  upstream_error_message,
  upstream_error_detail,
  upstream_errors,
  auth_latency_ms,
  routing_latency_ms,
  upstream_latency_ms,
  response_latency_ms,
  time_to_first_token_ms,
  request_body,
  request_body_truncated,
  request_body_bytes,
  request_headers,
  is_retryable,
  retry_count,
  created_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38,$39,$40,$41,$42,$43
)`

func NewOpsRepository(db *sql.DB) service.OpsRepository {
	return &opsRepository{db: db}
}

func (r *opsRepository) InsertErrorLog(ctx context.Context, input *service.OpsInsertErrorLogInput) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("nil ops repository")
	}
	if input == nil {
		return 0, fmt.Errorf("nil input")
	}

	var id int64
	err := r.db.QueryRowContext(
		ctx,
		insertOpsErrorLogSQL+" RETURNING id",
		opsInsertErrorLogArgs(input)...,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *opsRepository) BatchInsertErrorLogs(ctx context.Context, inputs []*service.OpsInsertErrorLogInput) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("nil ops repository")
	}
	if len(inputs) == 0 {
		return 0, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, insertOpsErrorLogSQL)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = stmt.Close()
	}()

	var inserted int64
	for _, input := range inputs {
		if input == nil {
			continue
		}
		if _, err = stmt.ExecContext(ctx, opsInsertErrorLogArgs(input)...); err != nil {
			return inserted, err
		}
		inserted++
	}

	if err = tx.Commit(); err != nil {
		return inserted, err
	}
	return inserted, nil
}

func opsInsertErrorLogArgs(input *service.OpsInsertErrorLogInput) []any {
	return []any{
		opsNullString(input.RequestID),
		opsNullString(input.ClientRequestID),
		opsNullInt64(input.UserID),
		opsNullInt64(input.APIKeyID),
		opsNullInt64(input.AccountID),
		opsNullInt64(input.GroupID),
		opsNullString(input.ClientIP),
		opsNullString(input.Platform),
		opsNullString(input.Model),
		opsNullString(input.RequestPath),
		input.Stream,
		opsNullString(input.InboundEndpoint),
		opsNullString(input.UpstreamEndpoint),
		opsNullString(input.RequestedModel),
		opsNullString(input.UpstreamModel),
		opsNullInt16(input.RequestType),
		opsNullString(input.UserAgent),
		input.ErrorPhase,
		input.ErrorType,
		opsNullString(input.Severity),
		opsNullInt(input.StatusCode),
		input.IsBusinessLimited,
		input.IsCountTokens,
		opsNullString(input.ErrorMessage),
		opsNullString(input.ErrorBody),
		opsNullString(input.ErrorSource),
		opsNullString(input.ErrorOwner),
		opsNullInt(input.UpstreamStatusCode),
		opsNullString(input.UpstreamErrorMessage),
		opsNullString(input.UpstreamErrorDetail),
		opsNullString(input.UpstreamErrorsJSON),
		opsNullInt64(input.AuthLatencyMs),
		opsNullInt64(input.RoutingLatencyMs),
		opsNullInt64(input.UpstreamLatencyMs),
		opsNullInt64(input.ResponseLatencyMs),
		opsNullInt64(input.TimeToFirstTokenMs),
		opsNullString(input.RequestBodyJSON),
		input.RequestBodyTruncated,
		opsNullInt(input.RequestBodyBytes),
		opsNullString(input.RequestHeadersJSON),
		input.IsRetryable,
		input.RetryCount,
		input.CreatedAt,
	}
}

func (r *opsRepository) ListErrorLogs(ctx context.Context, filter *service.OpsErrorLogFilter) (*service.OpsErrorLogList, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		filter = &service.OpsErrorLogFilter{}
	}

	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 500 {
		pageSize = 500
	}

	where, args := buildOpsErrorLogsWhere(filter)
	countSQL := "SELECT COUNT(*) FROM ops_error_logs e " + where

	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, err
	}

	offset := (page - 1) * pageSize
	argsWithLimit := append(args, pageSize, offset)
	selectSQL := `
SELECT
  e.id,
  e.created_at,
  e.error_phase,
  e.error_type,
  COALESCE(e.error_owner, ''),
  COALESCE(e.error_source, ''),
  e.severity,
  COALESCE(e.upstream_status_code, e.status_code, 0),
  COALESCE(e.platform, ''),
  COALESCE(e.model, ''),
  COALESCE(e.is_retryable, false),
  COALESCE(e.retry_count, 0),
  COALESCE(e.resolved, false),
  e.resolved_at,
  e.resolved_by_user_id,
  COALESCE(u2.email, ''),
  e.resolved_retry_id,
  COALESCE(e.client_request_id, ''),
  COALESCE(e.request_id, ''),
  COALESCE(e.error_message, ''),
  e.user_id,
  COALESCE(u.email, ''),
  e.api_key_id,
  e.account_id,
  COALESCE(a.name, ''),
  e.group_id,
  COALESCE(g.name, ''),
  CASE WHEN e.client_ip IS NULL THEN NULL ELSE e.client_ip::text END,
  COALESCE(e.request_path, ''),
  e.stream,
  COALESCE(e.inbound_endpoint, ''),
  COALESCE(e.upstream_endpoint, ''),
  COALESCE(e.requested_model, ''),
  COALESCE(e.upstream_model, ''),
  e.request_type
FROM ops_error_logs e
LEFT JOIN accounts a ON e.account_id = a.id
LEFT JOIN groups g ON e.group_id = g.id
LEFT JOIN users u ON e.user_id = u.id
LEFT JOIN users u2 ON e.resolved_by_user_id = u2.id
` + where + `
ORDER BY e.created_at DESC
LIMIT $` + itoa(len(args)+1) + ` OFFSET $` + itoa(len(args)+2)

	rows, err := r.db.QueryContext(ctx, selectSQL, argsWithLimit...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.OpsErrorLog, 0, pageSize)
	for rows.Next() {
		var item service.OpsErrorLog
		var statusCode sql.NullInt64
		var clientIP sql.NullString
		var userID sql.NullInt64
		var apiKeyID sql.NullInt64
		var accountID sql.NullInt64
		var accountName string
		var groupID sql.NullInt64
		var groupName string
		var userEmail string
		var resolvedAt sql.NullTime
		var resolvedBy sql.NullInt64
		var resolvedByName string
		var resolvedRetryID sql.NullInt64
		var requestType sql.NullInt64
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.Phase,
			&item.Type,
			&item.Owner,
			&item.Source,
			&item.Severity,
			&statusCode,
			&item.Platform,
			&item.Model,
			&item.IsRetryable,
			&item.RetryCount,
			&item.Resolved,
			&resolvedAt,
			&resolvedBy,
			&resolvedByName,
			&resolvedRetryID,
			&item.ClientRequestID,
			&item.RequestID,
			&item.Message,
			&userID,
			&userEmail,
			&apiKeyID,
			&accountID,
			&accountName,
			&groupID,
			&groupName,
			&clientIP,
			&item.RequestPath,
			&item.Stream,
			&item.InboundEndpoint,
			&item.UpstreamEndpoint,
			&item.RequestedModel,
			&item.UpstreamModel,
			&requestType,
		); err != nil {
			return nil, err
		}
		if resolvedAt.Valid {
			t := resolvedAt.Time
			item.ResolvedAt = &t
		}
		if resolvedBy.Valid {
			v := resolvedBy.Int64
			item.ResolvedByUserID = &v
		}
		item.ResolvedByUserName = resolvedByName
		if resolvedRetryID.Valid {
			v := resolvedRetryID.Int64
			item.ResolvedRetryID = &v
		}
		item.StatusCode = int(statusCode.Int64)
		if clientIP.Valid {
			s := clientIP.String
			item.ClientIP = &s
		}
		if userID.Valid {
			v := userID.Int64
			item.UserID = &v
		}
		item.UserEmail = userEmail
		if apiKeyID.Valid {
			v := apiKeyID.Int64
			item.APIKeyID = &v
		}
		if accountID.Valid {
			v := accountID.Int64
			item.AccountID = &v
		}
		item.AccountName = accountName
		if groupID.Valid {
			v := groupID.Int64
			item.GroupID = &v
		}
		item.GroupName = groupName
		if requestType.Valid {
			v := int16(requestType.Int64)
			item.RequestType = &v
		}
		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &service.OpsErrorLogList{
		Errors:   out,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (r *opsRepository) GetErrorLogByID(ctx context.Context, id int64) (*service.OpsErrorLogDetail, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if id <= 0 {
		return nil, fmt.Errorf("invalid id")
	}

	q := `
SELECT
  e.id,
  e.created_at,
  e.error_phase,
  e.error_type,
  COALESCE(e.error_owner, ''),
  COALESCE(e.error_source, ''),
  e.severity,
  COALESCE(e.upstream_status_code, e.status_code, 0),
  COALESCE(e.platform, ''),
  COALESCE(e.model, ''),
  COALESCE(e.is_retryable, false),
  COALESCE(e.retry_count, 0),
  COALESCE(e.resolved, false),
  e.resolved_at,
  e.resolved_by_user_id,
  e.resolved_retry_id,
  COALESCE(e.client_request_id, ''),
  COALESCE(e.request_id, ''),
  COALESCE(e.error_message, ''),
  COALESCE(e.error_body, ''),
  e.upstream_status_code,
  COALESCE(e.upstream_error_message, ''),
  COALESCE(e.upstream_error_detail, ''),
  COALESCE(e.upstream_errors::text, ''),
  e.is_business_limited,
  e.user_id,
  COALESCE(u.email, ''),
  e.api_key_id,
  e.account_id,
  COALESCE(a.name, ''),
  e.group_id,
  COALESCE(g.name, ''),
  CASE WHEN e.client_ip IS NULL THEN NULL ELSE e.client_ip::text END,
  COALESCE(e.request_path, ''),
  e.stream,
  COALESCE(e.inbound_endpoint, ''),
  COALESCE(e.upstream_endpoint, ''),
  COALESCE(e.requested_model, ''),
  COALESCE(e.upstream_model, ''),
  e.request_type,
  COALESCE(e.user_agent, ''),
  e.auth_latency_ms,
  e.routing_latency_ms,
  e.upstream_latency_ms,
  e.response_latency_ms,
  e.time_to_first_token_ms,
  COALESCE(e.request_body::text, ''),
  e.request_body_truncated,
  e.request_body_bytes,
  COALESCE(e.request_headers::text, '')
FROM ops_error_logs e
LEFT JOIN users u ON e.user_id = u.id
LEFT JOIN accounts a ON e.account_id = a.id
LEFT JOIN groups g ON e.group_id = g.id
WHERE e.id = $1
LIMIT 1`

	var out service.OpsErrorLogDetail
	var statusCode sql.NullInt64
	var upstreamStatusCode sql.NullInt64
	var resolvedAt sql.NullTime
	var resolvedBy sql.NullInt64
	var resolvedRetryID sql.NullInt64
	var clientIP sql.NullString
	var userID sql.NullInt64
	var apiKeyID sql.NullInt64
	var accountID sql.NullInt64
	var groupID sql.NullInt64
	var authLatency sql.NullInt64
	var routingLatency sql.NullInt64
	var upstreamLatency sql.NullInt64
	var responseLatency sql.NullInt64
	var ttft sql.NullInt64
	var requestBodyBytes sql.NullInt64
	var requestType sql.NullInt64

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&out.ID,
		&out.CreatedAt,
		&out.Phase,
		&out.Type,
		&out.Owner,
		&out.Source,
		&out.Severity,
		&statusCode,
		&out.Platform,
		&out.Model,
		&out.IsRetryable,
		&out.RetryCount,
		&out.Resolved,
		&resolvedAt,
		&resolvedBy,
		&resolvedRetryID,
		&out.ClientRequestID,
		&out.RequestID,
		&out.Message,
		&out.ErrorBody,
		&upstreamStatusCode,
		&out.UpstreamErrorMessage,
		&out.UpstreamErrorDetail,
		&out.UpstreamErrors,
		&out.IsBusinessLimited,
		&userID,
		&out.UserEmail,
		&apiKeyID,
		&accountID,
		&out.AccountName,
		&groupID,
		&out.GroupName,
		&clientIP,
		&out.RequestPath,
		&out.Stream,
		&out.InboundEndpoint,
		&out.UpstreamEndpoint,
		&out.RequestedModel,
		&out.UpstreamModel,
		&requestType,
		&out.UserAgent,
		&authLatency,
		&routingLatency,
		&upstreamLatency,
		&responseLatency,
		&ttft,
		&out.RequestBody,
		&out.RequestBodyTruncated,
		&requestBodyBytes,
		&out.RequestHeaders,
	)
	if err != nil {
		return nil, err
	}

	out.StatusCode = int(statusCode.Int64)
	if resolvedAt.Valid {
		t := resolvedAt.Time
		out.ResolvedAt = &t
	}
	if resolvedBy.Valid {
		v := resolvedBy.Int64
		out.ResolvedByUserID = &v
	}
	if resolvedRetryID.Valid {
		v := resolvedRetryID.Int64
		out.ResolvedRetryID = &v
	}
	if clientIP.Valid {
		s := clientIP.String
		out.ClientIP = &s
	}
	if upstreamStatusCode.Valid && upstreamStatusCode.Int64 > 0 {
		v := int(upstreamStatusCode.Int64)
		out.UpstreamStatusCode = &v
	}
	if userID.Valid {
		v := userID.Int64
		out.UserID = &v
	}
	if apiKeyID.Valid {
		v := apiKeyID.Int64
		out.APIKeyID = &v
	}
	if accountID.Valid {
		v := accountID.Int64
		out.AccountID = &v
	}
	if groupID.Valid {
		v := groupID.Int64
		out.GroupID = &v
	}
	if authLatency.Valid {
		v := authLatency.Int64
		out.AuthLatencyMs = &v
	}
	if routingLatency.Valid {
		v := routingLatency.Int64
		out.RoutingLatencyMs = &v
	}
	if upstreamLatency.Valid {
		v := upstreamLatency.Int64
		out.UpstreamLatencyMs = &v
	}
	if responseLatency.Valid {
		v := responseLatency.Int64
		out.ResponseLatencyMs = &v
	}
	if ttft.Valid {
		v := ttft.Int64
		out.TimeToFirstTokenMs = &v
	}
	if requestBodyBytes.Valid {
		v := int(requestBodyBytes.Int64)
		out.RequestBodyBytes = &v
	}
	if requestType.Valid {
		v := int16(requestType.Int64)
		out.RequestType = &v
	}

	// Normalize request_body to empty string when stored as JSON null.
	out.RequestBody = strings.TrimSpace(out.RequestBody)
	if out.RequestBody == "null" {
		out.RequestBody = ""
	}
	// Normalize request_headers to empty string when stored as JSON null.
	out.RequestHeaders = strings.TrimSpace(out.RequestHeaders)
	if out.RequestHeaders == "null" {
		out.RequestHeaders = ""
	}
	// Normalize upstream_errors to empty string when stored as JSON null.
	out.UpstreamErrors = strings.TrimSpace(out.UpstreamErrors)
	if out.UpstreamErrors == "null" {
		out.UpstreamErrors = ""
	}

	return &out, nil
}

func (r *opsRepository) InsertRetryAttempt(ctx context.Context, input *service.OpsInsertRetryAttemptInput) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("nil ops repository")
	}
	if input == nil {
		return 0, fmt.Errorf("nil input")
	}
	if input.SourceErrorID <= 0 {
		return 0, fmt.Errorf("invalid source_error_id")
	}
	if strings.TrimSpace(input.Mode) == "" {
		return 0, fmt.Errorf("invalid mode")
	}

	q := `
INSERT INTO ops_retry_attempts (
  requested_by_user_id,
  source_error_id,
  mode,
  pinned_account_id,
  status,
  started_at
) VALUES (
  $1,$2,$3,$4,$5,$6
) RETURNING id`

	var id int64
	err := r.db.QueryRowContext(
		ctx,
		q,
		opsNullInt64(&input.RequestedByUserID),
		input.SourceErrorID,
		strings.TrimSpace(input.Mode),
		opsNullInt64(input.PinnedAccountID),
		strings.TrimSpace(input.Status),
		input.StartedAt,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *opsRepository) UpdateRetryAttempt(ctx context.Context, input *service.OpsUpdateRetryAttemptInput) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if input == nil {
		return fmt.Errorf("nil input")
	}
	if input.ID <= 0 {
		return fmt.Errorf("invalid id")
	}

	q := `
UPDATE ops_retry_attempts
SET
  status = $2,
  finished_at = $3,
  duration_ms = $4,
  success = $5,
  http_status_code = $6,
  upstream_request_id = $7,
  used_account_id = $8,
  response_preview = $9,
  response_truncated = $10,
  result_request_id = $11,
  result_error_id = $12,
  error_message = $13
WHERE id = $1`

	_, err := r.db.ExecContext(
		ctx,
		q,
		input.ID,
		strings.TrimSpace(input.Status),
		nullTime(input.FinishedAt),
		input.DurationMs,
		nullBool(input.Success),
		nullInt(input.HTTPStatusCode),
		opsNullString(input.UpstreamRequestID),
		nullInt64(input.UsedAccountID),
		opsNullString(input.ResponsePreview),
		nullBool(input.ResponseTruncated),
		opsNullString(input.ResultRequestID),
		nullInt64(input.ResultErrorID),
		opsNullString(input.ErrorMessage),
	)
	return err
}

func (r *opsRepository) GetLatestRetryAttemptForError(ctx context.Context, sourceErrorID int64) (*service.OpsRetryAttempt, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if sourceErrorID <= 0 {
		return nil, fmt.Errorf("invalid source_error_id")
	}

	q := `
SELECT
  id,
  created_at,
  COALESCE(requested_by_user_id, 0),
  source_error_id,
  COALESCE(mode, ''),
  pinned_account_id,
  COALESCE(status, ''),
  started_at,
  finished_at,
  duration_ms,
  success,
  http_status_code,
  upstream_request_id,
  used_account_id,
  response_preview,
  response_truncated,
  result_request_id,
  result_error_id,
  error_message
FROM ops_retry_attempts
WHERE source_error_id = $1
ORDER BY created_at DESC
LIMIT 1`

	var out service.OpsRetryAttempt
	var pinnedAccountID sql.NullInt64
	var requestedBy sql.NullInt64
	var startedAt sql.NullTime
	var finishedAt sql.NullTime
	var durationMs sql.NullInt64
	var success sql.NullBool
	var httpStatusCode sql.NullInt64
	var upstreamRequestID sql.NullString
	var usedAccountID sql.NullInt64
	var responsePreview sql.NullString
	var responseTruncated sql.NullBool
	var resultRequestID sql.NullString
	var resultErrorID sql.NullInt64
	var errorMessage sql.NullString

	err := r.db.QueryRowContext(ctx, q, sourceErrorID).Scan(
		&out.ID,
		&out.CreatedAt,
		&requestedBy,
		&out.SourceErrorID,
		&out.Mode,
		&pinnedAccountID,
		&out.Status,
		&startedAt,
		&finishedAt,
		&durationMs,
		&success,
		&httpStatusCode,
		&upstreamRequestID,
		&usedAccountID,
		&responsePreview,
		&responseTruncated,
		&resultRequestID,
		&resultErrorID,
		&errorMessage,
	)
	if err != nil {
		return nil, err
	}
	out.RequestedByUserID = requestedBy.Int64
	if pinnedAccountID.Valid {
		v := pinnedAccountID.Int64
		out.PinnedAccountID = &v
	}
	if startedAt.Valid {
		t := startedAt.Time
		out.StartedAt = &t
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		out.FinishedAt = &t
	}
	if durationMs.Valid {
		v := durationMs.Int64
		out.DurationMs = &v
	}
	if success.Valid {
		v := success.Bool
		out.Success = &v
	}
	if httpStatusCode.Valid {
		v := int(httpStatusCode.Int64)
		out.HTTPStatusCode = &v
	}
	if upstreamRequestID.Valid {
		s := upstreamRequestID.String
		out.UpstreamRequestID = &s
	}
	if usedAccountID.Valid {
		v := usedAccountID.Int64
		out.UsedAccountID = &v
	}
	if responsePreview.Valid {
		s := responsePreview.String
		out.ResponsePreview = &s
	}
	if responseTruncated.Valid {
		v := responseTruncated.Bool
		out.ResponseTruncated = &v
	}
	if resultRequestID.Valid {
		s := resultRequestID.String
		out.ResultRequestID = &s
	}
	if resultErrorID.Valid {
		v := resultErrorID.Int64
		out.ResultErrorID = &v
	}
	if errorMessage.Valid {
		s := errorMessage.String
		out.ErrorMessage = &s
	}

	return &out, nil
}

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

func nullBool(v *bool) sql.NullBool {
	if v == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *v, Valid: true}
}

func (r *opsRepository) ListRetryAttemptsByErrorID(ctx context.Context, sourceErrorID int64, limit int) ([]*service.OpsRetryAttempt, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if sourceErrorID <= 0 {
		return nil, fmt.Errorf("invalid source_error_id")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	q := `
SELECT
  r.id,
  r.created_at,
  COALESCE(r.requested_by_user_id, 0),
  r.source_error_id,
  COALESCE(r.mode, ''),
  r.pinned_account_id,
  COALESCE(pa.name, ''),
  COALESCE(r.status, ''),
  r.started_at,
  r.finished_at,
  r.duration_ms,
  r.success,
  r.http_status_code,
  r.upstream_request_id,
  r.used_account_id,
  COALESCE(ua.name, ''),
  r.response_preview,
  r.response_truncated,
  r.result_request_id,
  r.result_error_id,
  r.error_message
FROM ops_retry_attempts r
LEFT JOIN accounts pa ON r.pinned_account_id = pa.id
LEFT JOIN accounts ua ON r.used_account_id = ua.id
WHERE r.source_error_id = $1
ORDER BY r.created_at DESC
LIMIT $2`

	rows, err := r.db.QueryContext(ctx, q, sourceErrorID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.OpsRetryAttempt, 0, 16)
	for rows.Next() {
		var item service.OpsRetryAttempt
		var pinnedAccountID sql.NullInt64
		var pinnedAccountName string
		var requestedBy sql.NullInt64
		var startedAt sql.NullTime
		var finishedAt sql.NullTime
		var durationMs sql.NullInt64
		var success sql.NullBool
		var httpStatusCode sql.NullInt64
		var upstreamRequestID sql.NullString
		var usedAccountID sql.NullInt64
		var usedAccountName string
		var responsePreview sql.NullString
		var responseTruncated sql.NullBool
		var resultRequestID sql.NullString
		var resultErrorID sql.NullInt64
		var errorMessage sql.NullString

		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&requestedBy,
			&item.SourceErrorID,
			&item.Mode,
			&pinnedAccountID,
			&pinnedAccountName,
			&item.Status,
			&startedAt,
			&finishedAt,
			&durationMs,
			&success,
			&httpStatusCode,
			&upstreamRequestID,
			&usedAccountID,
			&usedAccountName,
			&responsePreview,
			&responseTruncated,
			&resultRequestID,
			&resultErrorID,
			&errorMessage,
		); err != nil {
			return nil, err
		}

		item.RequestedByUserID = requestedBy.Int64
		if pinnedAccountID.Valid {
			v := pinnedAccountID.Int64
			item.PinnedAccountID = &v
		}
		item.PinnedAccountName = pinnedAccountName
		if startedAt.Valid {
			t := startedAt.Time
			item.StartedAt = &t
		}
		if finishedAt.Valid {
			t := finishedAt.Time
			item.FinishedAt = &t
		}
		if durationMs.Valid {
			v := durationMs.Int64
			item.DurationMs = &v
		}
		if success.Valid {
			v := success.Bool
			item.Success = &v
		}
		if httpStatusCode.Valid {
			v := int(httpStatusCode.Int64)
			item.HTTPStatusCode = &v
		}
		if upstreamRequestID.Valid {
			item.UpstreamRequestID = &upstreamRequestID.String
		}
		if usedAccountID.Valid {
			v := usedAccountID.Int64
			item.UsedAccountID = &v
		}
		item.UsedAccountName = usedAccountName
		if responsePreview.Valid {
			item.ResponsePreview = &responsePreview.String
		}
		if responseTruncated.Valid {
			v := responseTruncated.Bool
			item.ResponseTruncated = &v
		}
		if resultRequestID.Valid {
			item.ResultRequestID = &resultRequestID.String
		}
		if resultErrorID.Valid {
			v := resultErrorID.Int64
			item.ResultErrorID = &v
		}
		if errorMessage.Valid {
			item.ErrorMessage = &errorMessage.String
		}
		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *opsRepository) UpdateErrorResolution(ctx context.Context, errorID int64, resolved bool, resolvedByUserID *int64, resolvedRetryID *int64, resolvedAt *time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if errorID <= 0 {
		return fmt.Errorf("invalid error id")
	}

	q := `
UPDATE ops_error_logs
SET
  resolved = $2,
  resolved_at = $3,
  resolved_by_user_id = $4,
  resolved_retry_id = $5
WHERE id = $1`

	at := sql.NullTime{}
	if resolvedAt != nil && !resolvedAt.IsZero() {
		at = sql.NullTime{Time: resolvedAt.UTC(), Valid: true}
	} else if resolved {
		now := time.Now().UTC()
		at = sql.NullTime{Time: now, Valid: true}
	}

	_, err := r.db.ExecContext(
		ctx,
		q,
		errorID,
		resolved,
		at,
		nullInt64(resolvedByUserID),
		nullInt64(resolvedRetryID),
	)
	return err
}

func (r *opsRepository) BatchInsertSystemLogs(ctx context.Context, inputs []*service.OpsInsertSystemLogInput) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("nil ops repository")
	}
	if len(inputs) == 0 {
		return 0, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, pq.CopyIn(
		"ops_system_logs",
		"created_at",
		"level",
		"component",
		"message",
		"request_id",
		"client_request_id",
		"user_id",
		"account_id",
		"platform",
		"model",
		"extra",
	))
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}

	var inserted int64
	for _, input := range inputs {
		if input == nil {
			continue
		}
		createdAt := input.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		component := strings.TrimSpace(input.Component)
		level := strings.ToLower(strings.TrimSpace(input.Level))
		message := strings.TrimSpace(input.Message)
		if level == "" || message == "" {
			continue
		}
		if component == "" {
			component = "app"
		}
		extra := strings.TrimSpace(input.ExtraJSON)
		if extra == "" {
			extra = "{}"
		}
		if _, err := stmt.ExecContext(
			ctx,
			createdAt.UTC(),
			level,
			component,
			message,
			opsNullString(input.RequestID),
			opsNullString(input.ClientRequestID),
			opsNullInt64(input.UserID),
			opsNullInt64(input.AccountID),
			opsNullString(input.Platform),
			opsNullString(input.Model),
			extra,
		); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return inserted, err
		}
		inserted++
	}

	if _, err := stmt.ExecContext(ctx); err != nil {
		_ = stmt.Close()
		_ = tx.Rollback()
		return inserted, err
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return inserted, err
	}
	if err := tx.Commit(); err != nil {
		return inserted, err
	}
	return inserted, nil
}

func (r *opsRepository) ListSystemLogs(ctx context.Context, filter *service.OpsSystemLogFilter) (*service.OpsSystemLogList, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		filter = &service.OpsSystemLogFilter{}
	}

	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	where, args, _ := buildOpsSystemLogsWhere(filter)
	countSQL := "SELECT COUNT(*) FROM ops_system_logs l " + where
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, err
	}

	offset := (page - 1) * pageSize
	argsWithLimit := append(args, pageSize, offset)
	query := `
SELECT
  l.id,
  l.created_at,
  l.level,
  COALESCE(l.component, ''),
  COALESCE(l.message, ''),
  COALESCE(l.request_id, ''),
  COALESCE(l.client_request_id, ''),
  l.user_id,
  l.account_id,
  COALESCE(l.platform, ''),
  COALESCE(l.model, ''),
  COALESCE(l.extra::text, '{}')
FROM ops_system_logs l
` + where + `
ORDER BY l.created_at DESC, l.id DESC
LIMIT $` + itoa(len(args)+1) + ` OFFSET $` + itoa(len(args)+2)

	rows, err := r.db.QueryContext(ctx, query, argsWithLimit...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	logs := make([]*service.OpsSystemLog, 0, pageSize)
	for rows.Next() {
		item := &service.OpsSystemLog{}
		var userID sql.NullInt64
		var accountID sql.NullInt64
		var extraRaw string
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.Level,
			&item.Component,
			&item.Message,
			&item.RequestID,
			&item.ClientRequestID,
			&userID,
			&accountID,
			&item.Platform,
			&item.Model,
			&extraRaw,
		); err != nil {
			return nil, err
		}
		if userID.Valid {
			v := userID.Int64
			item.UserID = &v
		}
		if accountID.Valid {
			v := accountID.Int64
			item.AccountID = &v
		}
		extraRaw = strings.TrimSpace(extraRaw)
		if extraRaw != "" && extraRaw != "null" && extraRaw != "{}" {
			extra := make(map[string]any)
			if err := json.Unmarshal([]byte(extraRaw), &extra); err == nil {
				item.Extra = extra
			}
		}
		logs = append(logs, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &service.OpsSystemLogList{
		Logs:     logs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (r *opsRepository) DeleteSystemLogs(ctx context.Context, filter *service.OpsSystemLogCleanupFilter) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		filter = &service.OpsSystemLogCleanupFilter{}
	}

	where, args, hasConstraint := buildOpsSystemLogsCleanupWhere(filter)
	if !hasConstraint {
		return 0, fmt.Errorf("cleanup requires at least one filter condition")
	}

	query := "DELETE FROM ops_system_logs l " + where
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *opsRepository) InsertSystemLogCleanupAudit(ctx context.Context, input *service.OpsSystemLogCleanupAudit) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if input == nil {
		return fmt.Errorf("nil input")
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO ops_system_log_cleanup_audits (
  created_at,
  operator_id,
  conditions,
  deleted_rows
) VALUES ($1,$2,$3,$4)
`, createdAt.UTC(), input.OperatorID, input.Conditions, input.DeletedRows)
	return err
}

func buildOpsErrorLogsWhere(filter *service.OpsErrorLogFilter) (string, []any) {
	clauses := make([]string, 0, 12)
	args := make([]any, 0, 12)
	clauses = append(clauses, "1=1")

	phaseFilter := ""
	if filter != nil {
		phaseFilter = strings.TrimSpace(strings.ToLower(filter.Phase))
	}
	// ops_error_logs stores client-visible error requests (status>=400),
	// but we also persist "recovered" upstream errors (status<400) for upstream health visibility.
	// If Resolved is not specified, do not filter by resolved state (backward-compatible).
	resolvedFilter := (*bool)(nil)
	if filter != nil {
		resolvedFilter = filter.Resolved
	}
	// Keep list endpoints scoped to client errors unless explicitly filtering upstream phase.
	if phaseFilter != "upstream" {
		clauses = append(clauses, "COALESCE(e.status_code, 0) >= 400")
	}

	if filter.StartTime != nil && !filter.StartTime.IsZero() {
		args = append(args, filter.StartTime.UTC())
		clauses = append(clauses, "e.created_at >= $"+itoa(len(args)))
	}
	if filter.EndTime != nil && !filter.EndTime.IsZero() {
		args = append(args, filter.EndTime.UTC())
		// Keep time-window semantics consistent with other ops queries: [start, end)
		clauses = append(clauses, "e.created_at < $"+itoa(len(args)))
	}
	if p := strings.TrimSpace(filter.Platform); p != "" {
		args = append(args, p)
		clauses = append(clauses, "e.platform = $"+itoa(len(args)))
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		args = append(args, *filter.GroupID)
		clauses = append(clauses, "e.group_id = $"+itoa(len(args)))
	}
	if filter.AccountID != nil && *filter.AccountID > 0 {
		args = append(args, *filter.AccountID)
		clauses = append(clauses, "e.account_id = $"+itoa(len(args)))
	}
	if phase := phaseFilter; phase != "" {
		args = append(args, phase)
		clauses = append(clauses, "e.error_phase = $"+itoa(len(args)))
	}
	if filter != nil {
		if owner := strings.TrimSpace(strings.ToLower(filter.Owner)); owner != "" {
			args = append(args, owner)
			clauses = append(clauses, "LOWER(COALESCE(e.error_owner,'')) = $"+itoa(len(args)))
		}
		if source := strings.TrimSpace(strings.ToLower(filter.Source)); source != "" {
			args = append(args, source)
			clauses = append(clauses, "LOWER(COALESCE(e.error_source,'')) = $"+itoa(len(args)))
		}
	}
	if resolvedFilter != nil {
		args = append(args, *resolvedFilter)
		clauses = append(clauses, "COALESCE(e.resolved,false) = $"+itoa(len(args)))
	}

	// View filter: errors vs excluded vs all.
	// Excluded = business-limited errors (quota/concurrency/billing).
	// Upstream 429/529 are included in errors view to match SLA calculation.
	view := ""
	if filter != nil {
		view = strings.ToLower(strings.TrimSpace(filter.View))
	}
	switch view {
	case "", "errors":
		clauses = append(clauses, "COALESCE(e.is_business_limited,false) = false")
	case "excluded":
		clauses = append(clauses, "COALESCE(e.is_business_limited,false) = true")
	case "all":
		// no-op
	default:
		// treat unknown as default 'errors'
		clauses = append(clauses, "COALESCE(e.is_business_limited,false) = false")
	}
	if len(filter.StatusCodes) > 0 {
		args = append(args, pq.Array(filter.StatusCodes))
		clauses = append(clauses, "COALESCE(e.upstream_status_code, e.status_code, 0) = ANY($"+itoa(len(args))+")")
	} else if filter.StatusCodesOther {
		// "Other" means: status codes not in the common list.
		known := []int{400, 401, 403, 404, 409, 422, 429, 500, 502, 503, 504, 529}
		args = append(args, pq.Array(known))
		clauses = append(clauses, "NOT (COALESCE(e.upstream_status_code, e.status_code, 0) = ANY($"+itoa(len(args))+"))")
	}
	// Exact correlation keys (preferred for request↔upstream linkage).
	if rid := strings.TrimSpace(filter.RequestID); rid != "" {
		args = append(args, rid)
		clauses = append(clauses, "COALESCE(e.request_id,'') = $"+itoa(len(args)))
	}
	if crid := strings.TrimSpace(filter.ClientRequestID); crid != "" {
		args = append(args, crid)
		clauses = append(clauses, "COALESCE(e.client_request_id,'') = $"+itoa(len(args)))
	}

	if q := strings.TrimSpace(filter.Query); q != "" {
		like := "%" + q + "%"
		args = append(args, like)
		n := itoa(len(args))
		clauses = append(clauses, "(e.request_id ILIKE $"+n+" OR e.client_request_id ILIKE $"+n+" OR e.error_message ILIKE $"+n+")")
	}

	if userQuery := strings.TrimSpace(filter.UserQuery); userQuery != "" {
		like := "%" + userQuery + "%"
		args = append(args, like)
		n := itoa(len(args))
		clauses = append(clauses, "EXISTS (SELECT 1 FROM users u WHERE u.id = e.user_id AND u.email ILIKE $"+n+")")
	}

	return "WHERE " + strings.Join(clauses, " AND "), args
}

func buildOpsSystemLogsWhere(filter *service.OpsSystemLogFilter) (string, []any, bool) {
	clauses := make([]string, 0, 10)
	args := make([]any, 0, 10)
	clauses = append(clauses, "1=1")
	hasConstraint := false

	if filter != nil && filter.StartTime != nil && !filter.StartTime.IsZero() {
		args = append(args, filter.StartTime.UTC())
		clauses = append(clauses, "l.created_at >= $"+itoa(len(args)))
		hasConstraint = true
	}
	if filter != nil && filter.EndTime != nil && !filter.EndTime.IsZero() {
		args = append(args, filter.EndTime.UTC())
		clauses = append(clauses, "l.created_at < $"+itoa(len(args)))
		hasConstraint = true
	}
	if filter != nil {
		if v := strings.ToLower(strings.TrimSpace(filter.Level)); v != "" {
			args = append(args, v)
			clauses = append(clauses, "LOWER(COALESCE(l.level,'')) = $"+itoa(len(args)))
			hasConstraint = true
		}
		if v := strings.TrimSpace(filter.Component); v != "" {
			args = append(args, v)
			clauses = append(clauses, "COALESCE(l.component,'') = $"+itoa(len(args)))
			hasConstraint = true
		}
		if v := strings.TrimSpace(filter.RequestID); v != "" {
			args = append(args, v)
			clauses = append(clauses, "COALESCE(l.request_id,'') = $"+itoa(len(args)))
			hasConstraint = true
		}
		if v := strings.TrimSpace(filter.ClientRequestID); v != "" {
			args = append(args, v)
			clauses = append(clauses, "COALESCE(l.client_request_id,'') = $"+itoa(len(args)))
			hasConstraint = true
		}
		if filter.UserID != nil && *filter.UserID > 0 {
			args = append(args, *filter.UserID)
			clauses = append(clauses, "l.user_id = $"+itoa(len(args)))
			hasConstraint = true
		}
		if filter.AccountID != nil && *filter.AccountID > 0 {
			args = append(args, *filter.AccountID)
			clauses = append(clauses, "l.account_id = $"+itoa(len(args)))
			hasConstraint = true
		}
		if v := strings.TrimSpace(filter.Platform); v != "" {
			args = append(args, v)
			clauses = append(clauses, "COALESCE(l.platform,'') = $"+itoa(len(args)))
			hasConstraint = true
		}
		if v := strings.TrimSpace(filter.Model); v != "" {
			args = append(args, v)
			clauses = append(clauses, "COALESCE(l.model,'') = $"+itoa(len(args)))
			hasConstraint = true
		}
		if v := strings.TrimSpace(filter.Query); v != "" {
			like := "%" + v + "%"
			args = append(args, like)
			n := itoa(len(args))
			clauses = append(clauses, "(l.message ILIKE $"+n+" OR COALESCE(l.request_id,'') ILIKE $"+n+" OR COALESCE(l.client_request_id,'') ILIKE $"+n+" OR COALESCE(l.extra::text,'') ILIKE $"+n+")")
			hasConstraint = true
		}
	}

	return "WHERE " + strings.Join(clauses, " AND "), args, hasConstraint
}

func buildOpsSystemLogsCleanupWhere(filter *service.OpsSystemLogCleanupFilter) (string, []any, bool) {
	if filter == nil {
		filter = &service.OpsSystemLogCleanupFilter{}
	}
	listFilter := &service.OpsSystemLogFilter{
		StartTime:       filter.StartTime,
		EndTime:         filter.EndTime,
		Level:           filter.Level,
		Component:       filter.Component,
		RequestID:       filter.RequestID,
		ClientRequestID: filter.ClientRequestID,
		UserID:          filter.UserID,
		AccountID:       filter.AccountID,
		Platform:        filter.Platform,
		Model:           filter.Model,
		Query:           filter.Query,
	}
	return buildOpsSystemLogsWhere(listFilter)
}

// Helpers for nullable args
func opsNullString(v any) any {
	switch s := v.(type) {
	case nil:
		return sql.NullString{}
	case *string:
		if s == nil || strings.TrimSpace(*s) == "" {
			return sql.NullString{}
		}
		return sql.NullString{String: strings.TrimSpace(*s), Valid: true}
	case string:
		if strings.TrimSpace(s) == "" {
			return sql.NullString{}
		}
		return sql.NullString{String: strings.TrimSpace(s), Valid: true}
	default:
		return sql.NullString{}
	}
}

func opsNullInt64(v *int64) any {
	if v == nil || *v == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

func opsNullInt(v any) any {
	switch n := v.(type) {
	case nil:
		return sql.NullInt64{}
	case *int:
		if n == nil || *n == 0 {
			return sql.NullInt64{}
		}
		return sql.NullInt64{Int64: int64(*n), Valid: true}
	case *int64:
		if n == nil || *n == 0 {
			return sql.NullInt64{}
		}
		return sql.NullInt64{Int64: *n, Valid: true}
	case int:
		if n == 0 {
			return sql.NullInt64{}
		}
		return sql.NullInt64{Int64: int64(n), Valid: true}
	default:
		return sql.NullInt64{}
	}
}

func opsNullInt16(v *int16) any {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}
