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
  created_at,
  attempted_key_prefix,
  deleted_key_owner_user_id,
  deleted_key_name,
  api_key_prefix
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38,$39,$40,$41
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
		opsNullableIntPointer(input.UpstreamStatusCode),
		opsNullString(input.UpstreamErrorMessage),
		opsNullString(input.UpstreamErrorDetail),
		opsNullString(input.UpstreamErrorsJSON),
		opsNullInt64(input.AuthLatencyMs),
		opsNullInt64(input.RoutingLatencyMs),
		opsNullInt64(input.UpstreamLatencyMs),
		opsNullInt64(input.ResponseLatencyMs),
		opsNullInt64(input.TimeToFirstTokenMs),
		input.CreatedAt,
		opsNullString(input.AttemptedKeyPrefix),
		opsNullInt64(input.DeletedKeyOwnerUserID),
		opsNullString(input.DeletedKeyName),
		opsNullString(input.APIKeyPrefix),
	}
}

// opsErrorLogsOrderBy builds the ORDER BY clause from a whitelist, mirroring
// usageLogOrderBy semantics. Unknown SortBy falls back to created_at; e.id is
// always appended as tiebreaker for stable pagination.
func opsErrorLogsOrderBy(filter *service.OpsErrorLogFilter) string {
	sortBy := ""
	sortOrder := ""
	if filter != nil {
		sortBy = strings.ToLower(strings.TrimSpace(filter.SortBy))
		sortOrder = strings.ToLower(strings.TrimSpace(filter.SortOrder))
	}

	var column string
	switch sortBy {
	case "model":
		column = "COALESCE(NULLIF(TRIM(e.requested_model), ''), e.model)"
	case "status_code":
		// 与展示列/过滤保持同义:列表展示 COALESCE(upstream_status_code, status_code, 0),
		// status_code 过滤也用同一表达式,故排序必须一致——否则 recovered upstream 行
		//（status_code<400 但展示上游 5xx）排序键与显示值/分页切分不符。
		column = "COALESCE(e.upstream_status_code, e.status_code, 0)"
	default:
		column = "e.created_at"
	}

	dir := "DESC"
	if sortOrder == "asc" {
		dir = "ASC"
	}
	return fmt.Sprintf("%s %s, e.id %s", column, dir, dir)
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
  COALESCE(e.resolved, false),
  e.resolved_at,
  e.resolved_by_user_id,
  COALESCE(u2.email, ''),
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
  CASE WHEN e.client_ip IS NULL THEN NULL ELSE host(e.client_ip) END,
  COALESCE(e.request_path, ''),
  e.stream,
  COALESCE(e.inbound_endpoint, ''),
  COALESCE(e.upstream_endpoint, ''),
  COALESCE(e.requested_model, ''),
  COALESCE(e.upstream_model, ''),
  COALESCE(e.user_agent, ''),
  e.request_type,
  COALESCE(ak.name, ''),
  ak.deleted_at,
  COALESCE(e.deleted_key_name, ''),
  e.deleted_key_owner_user_id,
  COALESCE(du.email, '')
FROM ops_error_logs e
LEFT JOIN accounts a ON e.account_id = a.id
LEFT JOIN groups g ON e.group_id = g.id
LEFT JOIN users u ON e.user_id = u.id
LEFT JOIN users u2 ON e.resolved_by_user_id = u2.id
LEFT JOIN users du ON e.deleted_key_owner_user_id = du.id
LEFT JOIN api_keys ak ON ak.id = e.api_key_id
` + where + `
ORDER BY ` + opsErrorLogsOrderBy(filter) + `
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
		var requestType sql.NullInt64
		var apiKeyName string
		var apiKeyDeletedAt sql.NullTime
		var deletedKeyName string
		var deletedKeyOwnerID sql.NullInt64
		var deletedKeyOwnerEmail string
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
			&item.Resolved,
			&resolvedAt,
			&resolvedBy,
			&resolvedByName,
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
			&item.UserAgent,
			&requestType,
			&apiKeyName,
			&apiKeyDeletedAt,
			&deletedKeyName,
			&deletedKeyOwnerID,
			&deletedKeyOwnerEmail,
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
		// Key 名称：优先关联到的 ak.name（已软删的 key name 仍保留）；
		// 关联不到（api_key_id 为空 / 历史硬删）时回退错误记录里快照的 deleted_key_name。
		if apiKeyName != "" {
			item.APIKeyName = apiKeyName
		} else {
			item.APIKeyName = deletedKeyName
		}
		// 已删除：ak.deleted_at 非空（软删），或仅命中 deleted_key_name 兜底。
		item.APIKeyDeleted = apiKeyDeletedAt.Valid || (apiKeyName == "" && deletedKeyName != "")
		// 已删除 KEY 所有者快照:认证失败行 user_id 为空,列表用户列以此回退。
		if deletedKeyOwnerID.Valid {
			v := deletedKeyOwnerID.Int64
			item.DeletedKeyOwnerUserID = &v
			item.DeletedKeyOwnerEmail = deletedKeyOwnerEmail
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
  COALESCE(e.resolved, false),
  e.resolved_at,
  e.resolved_by_user_id,
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
  CASE WHEN e.client_ip IS NULL THEN NULL ELSE host(e.client_ip) END,
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
  COALESCE(e.attempted_key_prefix, ''),
  e.deleted_key_owner_user_id,
  COALESCE(du.email, ''),
  COALESCE(e.deleted_key_name, ''),
  COALESCE(e.api_key_prefix, ''),
  COALESCE(ak.name, ''),
  ak.deleted_at
FROM ops_error_logs e
LEFT JOIN users u ON e.user_id = u.id
LEFT JOIN accounts a ON e.account_id = a.id
LEFT JOIN groups g ON e.group_id = g.id
LEFT JOIN users du ON e.deleted_key_owner_user_id = du.id
LEFT JOIN api_keys ak ON ak.id = e.api_key_id
WHERE e.id = $1
LIMIT 1`

	var out service.OpsErrorLogDetail
	var statusCode sql.NullInt64
	var upstreamStatusCode sql.NullInt64
	var resolvedAt sql.NullTime
	var resolvedBy sql.NullInt64
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
	var requestType sql.NullInt64
	var deletedKeyOwnerUserID sql.NullInt64
	var detailAPIKeyName string
	var detailAPIKeyDeletedAt sql.NullTime

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
		&out.Resolved,
		&resolvedAt,
		&resolvedBy,
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
		&out.AttemptedKeyPrefix,
		&deletedKeyOwnerUserID,
		&out.DeletedKeyOwnerEmail,
		&out.DeletedKeyName,
		&out.APIKeyPrefix,
		&detailAPIKeyName,
		&detailAPIKeyDeletedAt,
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
	if clientIP.Valid {
		s := clientIP.String
		out.ClientIP = &s
	}
	if upstreamStatusCode.Valid {
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
	if requestType.Valid {
		v := int16(requestType.Int64)
		out.RequestType = &v
	}
	if deletedKeyOwnerUserID.Valid {
		v := deletedKeyOwnerUserID.Int64
		out.DeletedKeyOwnerUserID = &v
	}
	// Key 名称：优先关联到的 ak.name；关联不到时回退快照的 deleted_key_name。
	if detailAPIKeyName != "" {
		out.APIKeyName = detailAPIKeyName
	} else {
		out.APIKeyName = out.DeletedKeyName
	}
	// 已删除：ak.deleted_at 非空（软删），或仅命中 deleted_key_name 兜底。
	out.APIKeyDeleted = detailAPIKeyDeletedAt.Valid || (detailAPIKeyName == "" && out.DeletedKeyName != "")

	// Normalize upstream_errors to empty string when stored as JSON null.
	out.UpstreamErrors = strings.TrimSpace(out.UpstreamErrors)
	if out.UpstreamErrors == "null" {
		out.UpstreamErrors = ""
	}

	return &out, nil
}

// LookupDeletedKeyAudit 按明文 key 反查最近一条已删除 key 审计。
// 同一 key 可能有多条历史(反复创建/删除),取 deleted_at 最近一条(id 作同毫秒 tiebreaker)。
// 未命中返回 (nil, nil)。
func (r *opsRepository) LookupDeletedKeyAudit(ctx context.Context, key string) (*service.DeletedKeyAuditResult, error) {
	var res service.DeletedKeyAuditResult
	err := r.db.QueryRowContext(ctx, `
		SELECT user_id, key_name
		FROM deleted_api_key_audits
		WHERE key = $1
		ORDER BY deleted_at DESC, id DESC
		LIMIT 1`, key).Scan(&res.UserID, &res.KeyName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &res, nil
}

func (r *opsRepository) UpdateErrorResolution(ctx context.Context, errorID int64, resolved bool, resolvedByUserID *int64, resolvedAt *time.Time) error {
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
  resolved_by_user_id = $4
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
		"host",
		"level",
		"component",
		"message",
		"request_id",
		"client_request_id",
		"user_id",
		"api_key_id",
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
			opsNullString(input.Host),
			level,
			component,
			message,
			opsNullString(input.RequestID),
			opsNullString(input.ClientRequestID),
			opsNullInt64(input.UserID),
			opsNullInt64(input.APIKeyID),
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
  COALESCE(l.host, ''),
  l.level,
  COALESCE(l.component, ''),
  COALESCE(l.message, ''),
  COALESCE(l.request_id, ''),
  COALESCE(l.client_request_id, ''),
  l.user_id,
  l.api_key_id,
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
		var apiKeyID sql.NullInt64
		var accountID sql.NullInt64
		var extraRaw string
		if err := rows.Scan(
			&item.ID,
			&item.CreatedAt,
			&item.Host,
			&item.Level,
			&item.Component,
			&item.Message,
			&item.RequestID,
			&item.ClientRequestID,
			&userID,
			&apiKeyID,
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
		if apiKeyID.Valid {
			v := apiKeyID.Int64
			item.APIKeyID = &v
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

var likePatternReplacer = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// escapeLikePattern 转义 LIKE/ILIKE 通配符（\ % _），避免用户输入被当作通配符。
// Postgres 默认以反斜杠为转义符，无需额外 ESCAPE 子句。
func escapeLikePattern(s string) string {
	return likePatternReplacer.Replace(s)
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
	// Keep list endpoints scoped to client errors unless the caller explicitly opts
	// into recovered provider-health rows (upstream/account_auth). Request-error
	// endpoints never set the opt-in and retain this guard.
	// cyber_policy is exempt from the status >= 400 guard: streaming cyber hits arrive with
	// status 200 (the SSE stream opened successfully before upstream returned response.failed),
	// but they are always client-visible blocked requests that belong in admin + user error
	// lists.  Without the exemption the entire streaming-path cyber sink would be invisible.
	if !opsFilterIncludesRecoveredProviderRows(filter, phaseFilter) {
		clauses = append(clauses, "(COALESCE(e.status_code, 0) >= 400 OR e.error_type = 'cyber_policy')")
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

	if filter.UserID != nil && *filter.UserID > 0 {
		args = append(args, *filter.UserID)
		n := itoa(len(args))
		if filter.MatchDeletedKeyOwner {
			// 用户侧:把「删 key 后认证失败」(user_id=NULL,靠 deleted_key_owner 归因)的记录也纳入。
			clauses = append(clauses, "(e.user_id = $"+n+" OR e.deleted_key_owner_user_id = $"+n+")")
		} else {
			clauses = append(clauses, "e.user_id = $"+n)
		}
	}
	if filter.APIKeyID != nil && *filter.APIKeyID > 0 {
		args = append(args, *filter.APIKeyID)
		clauses = append(clauses, "e.api_key_id = $"+itoa(len(args)))
	}
	if m := strings.TrimSpace(filter.Model); m != "" {
		if filter.ModelFuzzy {
			args = append(args, "%"+escapeLikePattern(m)+"%")
			clauses = append(clauses, "COALESCE(e.requested_model, e.model, '') ILIKE $"+itoa(len(args)))
		} else {
			args = append(args, m)
			clauses = append(clauses, "COALESCE(e.requested_model, e.model, '') = $"+itoa(len(args)))
		}
	}
	if filter.ExcludeCountTokens {
		clauses = append(clauses, "COALESCE(e.is_count_tokens, false) = false")
	}
	if len(filter.ErrorPhasesAny) > 0 {
		args = append(args, pq.Array(filter.ErrorPhasesAny))
		clauses = append(clauses, "e.error_phase = ANY($"+itoa(len(args))+")")
	}
	if len(filter.ErrorTypesAny) > 0 {
		args = append(args, pq.Array(filter.ErrorTypesAny))
		clauses = append(clauses, "e.error_type = ANY($"+itoa(len(args))+")")
	}

	return "WHERE " + strings.Join(clauses, " AND "), args
}

func opsFilterIncludesRecoveredProviderRows(filter *service.OpsErrorLogFilter, phaseFilter string) bool {
	if filter == nil || !filter.IncludeRecoveredUpstream {
		return false
	}
	if phaseFilter != "" {
		return phaseFilter == "upstream" || phaseFilter == "account_auth"
	}
	if len(filter.ErrorPhasesAny) == 0 {
		return false
	}
	sawProviderPhase := false
	for _, rawPhase := range filter.ErrorPhasesAny {
		switch strings.TrimSpace(strings.ToLower(rawPhase)) {
		case "upstream", "account_auth":
			sawProviderPhase = true
		default:
			return false
		}
	}
	return sawProviderPhase
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
		if v := strings.TrimSpace(filter.Host); v != "" {
			args = append(args, v)
			clauses = append(clauses, "l.host = $"+itoa(len(args)))
			hasConstraint = true
		}
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
		if filter.APIKeyID != nil && *filter.APIKeyID > 0 {
			args = append(args, *filter.APIKeyID)
			clauses = append(clauses, "l.api_key_id = $"+itoa(len(args)))
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
		Host:            filter.Host,
		Level:           filter.Level,
		Component:       filter.Component,
		RequestID:       filter.RequestID,
		ClientRequestID: filter.ClientRequestID,
		UserID:          filter.UserID,
		APIKeyID:        filter.APIKeyID,
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

// opsNullableIntPointer distinguishes an absent value from an explicitly
// observed zero. Credential-stage failures intentionally persist upstream
// status 0 because no inference request was sent.
func opsNullableIntPointer(v *int) any {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

func opsNullInt16(v *int16) any {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}
