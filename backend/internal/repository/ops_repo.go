package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type opsRepository struct {
	db *sql.DB
REDACTED

func NewOpsRepository(db *sql.DB) service.OpsRepository {
	return &opsRepository{db: dbREDACTED
REDACTED

func (r *opsRepository) InsertErrorLog(ctx context.Context, input *service.OpsInsertErrorLogInput) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("nil ops repository")
REDACTED
	if input == nil {
		return 0, fmt.Errorf("nil input")
REDACTED

	q := `
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
  duration_ms,
  time_to_first_token_ms,
  request_body,
  request_body_truncated,
  request_body_bytes,
  request_headers,
  is_retryable,
  retry_count,
  created_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35
) RETURNING id`

	var id int64
	err := r.db.QueryRowContext(
		ctx,
		q,
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
		opsNullInt(input.DurationMs),
		opsNullInt64(input.TimeToFirstTokenMs),
		opsNullString(input.RequestBodyJSON),
		input.RequestBodyTruncated,
		opsNullInt(input.RequestBodyBytes),
		opsNullString(input.RequestHeadersJSON),
		input.IsRetryable,
		input.RetryCount,
		input.CreatedAt,
	).Scan(&id)
	if err != nil {
		return 0, err
REDACTED
	return id, nil
REDACTED

func (r *opsRepository) ListErrorLogs(ctx context.Context, filter *service.OpsErrorLogFilter) (*service.OpsErrorLogList, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
REDACTED
	if filter == nil {
		filter = &service.OpsErrorLogFilter{REDACTED
REDACTED

	page := filter.Page
	if page <= 0 {
		page = 1
REDACTED
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
REDACTED
	if pageSize > 500 {
		pageSize = 500
REDACTED

	where, args := buildOpsErrorLogsWhere(filter)
	countSQL := "SELECT COUNT(*) FROM ops_error_logs " + where

	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, err
REDACTED

	offset := (page - 1) * pageSize
	argsWithLimit := append(args, pageSize, offset)
	selectSQL := `
SELECT
  id,
  created_at,
  error_phase,
  error_type,
  severity,
  COALESCE(upstream_status_code, status_code, 0),
  COALESCE(platform, ''),
  COALESCE(model, ''),
  duration_ms,
  COALESCE(client_request_id, ''),
  COALESCE(request_id, ''),
  COALESCE(error_message, ''),
  user_id,
  api_key_id,
  account_id,
  group_id,
  CASE WHEN client_ip IS NULL THEN NULL ELSE client_ip::text END,
  COALESCE(request_path, ''),
  stream
FROM ops_error_logs
` + where + `
ORDER BY created_at DESC
LIMIT $` + itoa(len(args)+1) + ` OFFSET $` + itoa(len(args)+2)

	rows, err := r.db.QueryContext(ctx, selectSQL, argsWithLimit...)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	out := make([]*service.OpsErrorLog, 0, pageSize)
	for rows.Next() {
		var item service.OpsErrorLog
		var latency sql.NullInt64
		var statusCode sql.NullInt64
		var clientIP sql.NullString
		var userID sql.NullInt64
		var apiKeyID sql.NullInt64
		var accountID sql.NullInt64
		var groupID sql.NullInt64
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.Phase,
			&item.Type,
			&item.Severity,
			&statusCode,
			&item.Platform,
			&item.Model,
			&latency,
			&item.ClientRequestID,
			&item.RequestID,
			&item.Message,
			&userID,
			&apiKeyID,
			&accountID,
			&groupID,
			&clientIP,
			&item.RequestPath,
			&item.Stream,
		); err != nil {
			return nil, err
	REDACTED
		if latency.Valid {
			v := int(latency.Int64)
			item.LatencyMs = &v
	REDACTED
		item.StatusCode = int(statusCode.Int64)
		if clientIP.Valid {
			s := clientIP.String
			item.ClientIP = &s
	REDACTED
		if userID.Valid {
			v := userID.Int64
			item.UserID = &v
	REDACTED
		if apiKeyID.Valid {
			v := apiKeyID.Int64
			item.APIKeyID = &v
	REDACTED
		if accountID.Valid {
			v := accountID.Int64
			item.AccountID = &v
	REDACTED
		if groupID.Valid {
			v := groupID.Int64
			item.GroupID = &v
	REDACTED
		out = append(out, &item)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED

	return &service.OpsErrorLogList{
		Errors:   out,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
REDACTED, nil
REDACTED

func (r *opsRepository) GetErrorLogByID(ctx context.Context, id int64) (*service.OpsErrorLogDetail, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
REDACTED
	if id <= 0 {
		return nil, fmt.Errorf("invalid id")
REDACTED

	q := `
SELECT
  id,
  created_at,
  error_phase,
  error_type,
  severity,
  COALESCE(upstream_status_code, status_code, 0),
  COALESCE(platform, ''),
  COALESCE(model, ''),
  duration_ms,
  COALESCE(client_request_id, ''),
  COALESCE(request_id, ''),
  COALESCE(error_message, ''),
  COALESCE(error_body, ''),
  upstream_status_code,
  COALESCE(upstream_error_message, ''),
  COALESCE(upstream_error_detail, ''),
  COALESCE(upstream_errors::text, ''),
  is_business_limited,
  user_id,
  api_key_id,
  account_id,
  group_id,
  CASE WHEN client_ip IS NULL THEN NULL ELSE client_ip::text END,
  COALESCE(request_path, ''),
  stream,
  COALESCE(user_agent, ''),
  auth_latency_ms,
  routing_latency_ms,
  upstream_latency_ms,
  response_latency_ms,
  time_to_first_token_ms,
  COALESCE(request_body::text, ''),
  request_body_truncated,
  request_body_bytes,
  COALESCE(request_headers::text, '')
FROM ops_error_logs
WHERE id = $1
LIMIT 1`

	var out service.OpsErrorLogDetail
	var latency sql.NullInt64
	var statusCode sql.NullInt64
	var upstreamStatusCode sql.NullInt64
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

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&out.ID,
		&out.CreatedAt,
		&out.Phase,
		&out.Type,
		&out.Severity,
		&statusCode,
		&out.Platform,
		&out.Model,
		&latency,
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
		&apiKeyID,
		&accountID,
		&groupID,
		&clientIP,
		&out.RequestPath,
		&out.Stream,
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
REDACTED

	out.StatusCode = int(statusCode.Int64)
	if latency.Valid {
		v := int(latency.Int64)
		out.LatencyMs = &v
REDACTED
	if clientIP.Valid {
		s := clientIP.String
		out.ClientIP = &s
REDACTED
	if upstreamStatusCode.Valid && upstreamStatusCode.Int64 > 0 {
		v := int(upstreamStatusCode.Int64)
		out.UpstreamStatusCode = &v
REDACTED
	if userID.Valid {
		v := userID.Int64
		out.UserID = &v
REDACTED
	if apiKeyID.Valid {
		v := apiKeyID.Int64
		out.APIKeyID = &v
REDACTED
	if accountID.Valid {
		v := accountID.Int64
		out.AccountID = &v
REDACTED
	if groupID.Valid {
		v := groupID.Int64
		out.GroupID = &v
REDACTED
	if authLatency.Valid {
		v := authLatency.Int64
		out.AuthLatencyMs = &v
REDACTED
	if routingLatency.Valid {
		v := routingLatency.Int64
		out.RoutingLatencyMs = &v
REDACTED
	if upstreamLatency.Valid {
		v := upstreamLatency.Int64
		out.UpstreamLatencyMs = &v
REDACTED
	if responseLatency.Valid {
		v := responseLatency.Int64
		out.ResponseLatencyMs = &v
REDACTED
	if ttft.Valid {
		v := ttft.Int64
		out.TimeToFirstTokenMs = &v
REDACTED
	if requestBodyBytes.Valid {
		v := int(requestBodyBytes.Int64)
		out.RequestBodyBytes = &v
REDACTED

	// Normalize request_body to empty string when stored as JSON null.
	out.RequestBody = strings.TrimSpace(out.RequestBody)
	if out.RequestBody == "null" {
		out.RequestBody = ""
REDACTED
	// Normalize request_headers to empty string when stored as JSON null.
	out.RequestHeaders = strings.TrimSpace(out.RequestHeaders)
	if out.RequestHeaders == "null" {
		out.RequestHeaders = ""
REDACTED
	// Normalize upstream_errors to empty string when stored as JSON null.
	out.UpstreamErrors = strings.TrimSpace(out.UpstreamErrors)
	if out.UpstreamErrors == "null" {
		out.UpstreamErrors = ""
REDACTED

	return &out, nil
REDACTED

func (r *opsRepository) InsertRetryAttempt(ctx context.Context, input *service.OpsInsertRetryAttemptInput) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("nil ops repository")
REDACTED
	if input == nil {
		return 0, fmt.Errorf("nil input")
REDACTED
	if input.SourceErrorID <= 0 {
		return 0, fmt.Errorf("invalid source_error_id")
REDACTED
	if strings.TrimSpace(input.Mode) == "" {
		return 0, fmt.Errorf("invalid mode")
REDACTED

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
REDACTED
	return id, nil
REDACTED

func (r *opsRepository) UpdateRetryAttempt(ctx context.Context, input *service.OpsUpdateRetryAttemptInput) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
REDACTED
	if input == nil {
		return fmt.Errorf("nil input")
REDACTED
	if input.ID <= 0 {
		return fmt.Errorf("invalid id")
REDACTED

	q := `
UPDATE ops_retry_attempts
SET
  status = $2,
  finished_at = $3,
  duration_ms = $4,
  result_request_id = $5,
  result_error_id = $6,
  error_message = $7
WHERE id = $1`

	_, err := r.db.ExecContext(
		ctx,
		q,
		input.ID,
		strings.TrimSpace(input.Status),
		nullTime(input.FinishedAt),
		input.DurationMs,
		opsNullString(input.ResultRequestID),
		opsNullInt64(input.ResultErrorID),
		opsNullString(input.ErrorMessage),
	)
	return err
REDACTED

func (r *opsRepository) GetLatestRetryAttemptForError(ctx context.Context, sourceErrorID int64) (*service.OpsRetryAttempt, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
REDACTED
	if sourceErrorID <= 0 {
		return nil, fmt.Errorf("invalid source_error_id")
REDACTED

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
		&resultRequestID,
		&resultErrorID,
		&errorMessage,
	)
	if err != nil {
		return nil, err
REDACTED
	out.RequestedByUserID = requestedBy.Int64
	if pinnedAccountID.Valid {
		v := pinnedAccountID.Int64
		out.PinnedAccountID = &v
REDACTED
	if startedAt.Valid {
		t := startedAt.Time
		out.StartedAt = &t
REDACTED
	if finishedAt.Valid {
		t := finishedAt.Time
		out.FinishedAt = &t
REDACTED
	if durationMs.Valid {
		v := durationMs.Int64
		out.DurationMs = &v
REDACTED
	if resultRequestID.Valid {
		s := resultRequestID.String
		out.ResultRequestID = &s
REDACTED
	if resultErrorID.Valid {
		v := resultErrorID.Int64
		out.ResultErrorID = &v
REDACTED
	if errorMessage.Valid {
		s := errorMessage.String
		out.ErrorMessage = &s
REDACTED

	return &out, nil
REDACTED

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{REDACTED
REDACTED
	return sql.NullTime{Time: t, Valid: trueREDACTED
REDACTED

func buildOpsErrorLogsWhere(filter *service.OpsErrorLogFilter) (string, []any) {
	clauses := make([]string, 0, 8)
	args := make([]any, 0, 8)
	clauses = append(clauses, "1=1")

	phaseFilter := ""
	if filter != nil {
		phaseFilter = strings.TrimSpace(strings.ToLower(filter.Phase))
REDACTED
	// ops_error_logs primarily stores client-visible error requests (status>=400),
	// but we also persist "recovered" upstream errors (status<400) for upstream health visibility.
	// By default, keep list endpoints scoped to client errors unless explicitly filtering upstream phase.
	if phaseFilter != "upstream" {
		clauses = append(clauses, "COALESCE(status_code, 0) >= 400")
REDACTED

	if filter.StartTime != nil && !filter.StartTime.IsZero() {
		args = append(args, filter.StartTime.UTC())
		clauses = append(clauses, "created_at >= $"+itoa(len(args)))
REDACTED
	if filter.EndTime != nil && !filter.EndTime.IsZero() {
		args = append(args, filter.EndTime.UTC())
		// Keep time-window semantics consistent with other ops queries: [start, end)
		clauses = append(clauses, "created_at < $"+itoa(len(args)))
REDACTED
	if p := strings.TrimSpace(filter.Platform); p != "" {
		args = append(args, p)
		clauses = append(clauses, "platform = $"+itoa(len(args)))
REDACTED
	if filter.GroupID != nil && *filter.GroupID > 0 {
		args = append(args, *filter.GroupID)
		clauses = append(clauses, "group_id = $"+itoa(len(args)))
REDACTED
	if filter.AccountID != nil && *filter.AccountID > 0 {
		args = append(args, *filter.AccountID)
		clauses = append(clauses, "account_id = $"+itoa(len(args)))
REDACTED
	if phase := phaseFilter; phase != "" {
		args = append(args, phase)
		clauses = append(clauses, "error_phase = $"+itoa(len(args)))
REDACTED
	if len(filter.StatusCodes) > 0 {
		args = append(args, pq.Array(filter.StatusCodes))
		clauses = append(clauses, "COALESCE(upstream_status_code, status_code, 0) = ANY($"+itoa(len(args))+")")
REDACTED
	if q := strings.TrimSpace(filter.Query); q != "" {
		like := "%" + q + "%"
		args = append(args, like)
		n := itoa(len(args))
		clauses = append(clauses, "(request_id ILIKE $"+n+" OR client_request_id ILIKE $"+n+" OR error_message ILIKE $"+n+")")
REDACTED

	return "WHERE " + strings.Join(clauses, " AND "), args
REDACTED

// Helpers for nullable args
func opsNullString(v any) any {
	switch s := v.(type) {
	case nil:
		return sql.NullString{REDACTED
	case *string:
		if s == nil || strings.TrimSpace(*s) == "" {
			return sql.NullString{REDACTED
	REDACTED
		return sql.NullString{String: strings.TrimSpace(*s), Valid: trueREDACTED
	case string:
		if strings.TrimSpace(s) == "" {
			return sql.NullString{REDACTED
	REDACTED
		return sql.NullString{String: strings.TrimSpace(s), Valid: trueREDACTED
	default:
		return sql.NullString{REDACTED
REDACTED
REDACTED

func opsNullInt64(v *int64) any {
	if v == nil || *v == 0 {
		return sql.NullInt64{REDACTED
REDACTED
	return sql.NullInt64{Int64: *v, Valid: trueREDACTED
REDACTED

func opsNullInt(v any) any {
	switch n := v.(type) {
	case nil:
		return sql.NullInt64{REDACTED
	case *int:
		if n == nil || *n == 0 {
			return sql.NullInt64{REDACTED
	REDACTED
		return sql.NullInt64{Int64: int64(*n), Valid: trueREDACTED
	case *int64:
		if n == nil || *n == 0 {
			return sql.NullInt64{REDACTED
	REDACTED
		return sql.NullInt64{Int64: *n, Valid: trueREDACTED
	case int:
		if n == 0 {
			return sql.NullInt64{REDACTED
	REDACTED
		return sql.NullInt64{Int64: int64(n), Valid: trueREDACTED
	default:
		return sql.NullInt64{REDACTED
REDACTED
REDACTED
