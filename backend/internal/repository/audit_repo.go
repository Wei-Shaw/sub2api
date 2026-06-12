package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type auditRepository struct {
	db *sql.DB
}

func NewAuditRepository(db *sql.DB) service.AuditRepository {
	return &auditRepository{db: db}
}

func (r *auditRepository) Create(ctx context.Context, log *service.AuditLog) error {
	if r == nil || r.db == nil || log == nil {
		return nil
	}
	var userID any
	if log.UserID != nil {
		userID = *log.UserID
	}
	var apiKeyID any
	if log.APIKeyID != nil {
		apiKeyID = *log.APIKeyID
	}
	var groupID any
	if log.GroupID != nil {
		groupID = *log.GroupID
	}
	sessionID := strings.TrimSpace(log.SessionID)
	sessionScope := auditSessionScope(log, sessionID)
	requestBody := log.RequestBody
	responseBody := log.ResponseBody
	if sessionID != "" {
		requestTurns, requestErr := json.Marshal([]auditSessionContent{auditRequestContentFromLog(log)})
		responseTurns, responseErr := json.Marshal([]auditSessionContent{auditResponseContentFromLog(log)})
		if requestErr == nil && responseErr == nil {
			requestBody = string(requestTurns)
			responseBody = string(responseTurns)
		}
	}
	return r.db.QueryRowContext(ctx, `
INSERT INTO audit_logs (
    request_id, session_id, session_scope, request_count, user_id, user_email, api_key_id, api_key_name, group_id, group_name,
    platform, endpoint, method, model, status_code, request_body, response_body,
    request_truncated, response_truncated, duration_ms, ip_address, user_agent
) VALUES (
    $1, $2, $3, 1, $4, $5, $6, $7, $8, $9,
    $10, $11, $12, $13, $14, $15, $16,
    $17, $18, $19, $20, $21
)
ON CONFLICT (session_scope)
WHERE session_scope <> ''
DO UPDATE SET
    request_id = EXCLUDED.request_id,
    request_count = audit_logs.request_count + 1,
    user_email = EXCLUDED.user_email,
    api_key_name = EXCLUDED.api_key_name,
    group_name = EXCLUDED.group_name,
    platform = EXCLUDED.platform,
    endpoint = EXCLUDED.endpoint,
    method = EXCLUDED.method,
    model = EXCLUDED.model,
    status_code = EXCLUDED.status_code,
    request_body = append_audit_turns(audit_logs.request_body, EXCLUDED.request_body),
    response_body = append_audit_turns(audit_logs.response_body, EXCLUDED.response_body),
    request_truncated = audit_logs.request_truncated OR EXCLUDED.request_truncated,
    response_truncated = audit_logs.response_truncated OR EXCLUDED.response_truncated,
    duration_ms = audit_logs.duration_ms + EXCLUDED.duration_ms,
    ip_address = EXCLUDED.ip_address,
    user_agent = EXCLUDED.user_agent,
    updated_at = NOW()
RETURNING id, request_count, created_at, updated_at`,
		log.RequestID, sessionID, sessionScope, userID, log.UserEmail, apiKeyID, log.APIKeyName, groupID, log.GroupName,
		log.Platform, log.Endpoint, log.Method, log.Model, log.StatusCode, requestBody, responseBody,
		log.RequestTruncated, log.ResponseTruncated, log.DurationMS, log.IPAddress, log.UserAgent,
	).Scan(&log.ID, &log.RequestCount, &log.CreatedAt, &log.UpdatedAt)
}

func auditSessionScope(log *service.AuditLog, sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if log == nil || sessionID == "" {
		return ""
	}
	apiKeyID := int64(0)
	if log.APIKeyID != nil {
		apiKeyID = *log.APIKeyID
	}
	userID := int64(0)
	if log.UserID != nil {
		userID = *log.UserID
	}
	groupID := int64(0)
	if log.GroupID != nil {
		groupID = *log.GroupID
	}
	return fmt.Sprintf("api:%d|user:%d|group:%d|session:%s", apiKeyID, userID, groupID, sessionID)
}

func (r *auditRepository) List(ctx context.Context, filter service.AuditLogFilter) ([]service.AuditLog, *pagination.PaginationResult, error) {
	where, args := buildAuditWhere(filter)
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_logs l "+whereSQL, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count audit logs: %w", err)
	}

	params := filter.Pagination
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.Limit(), params.Offset())
	rows, err := r.db.QueryContext(ctx, `
SELECT
    l.id, l.request_id, l.session_id, l.request_count, l.user_id, l.user_email, l.api_key_id, l.api_key_name,
    l.group_id, l.group_name, l.platform, l.endpoint, l.method, l.model,
    l.status_code, l.request_body, l.response_body, l.request_truncated,
    l.response_truncated, l.duration_ms, l.ip_address, l.user_agent, l.created_at, l.updated_at
FROM audit_logs l `+whereSQL+`
ORDER BY l.updated_at DESC, l.id DESC
LIMIT $`+fmt.Sprint(len(queryArgs)-1)+` OFFSET $`+fmt.Sprint(len(queryArgs)),
		queryArgs...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("list audit logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.AuditLog, 0)
	for rows.Next() {
		var item service.AuditLog
		var userID, apiKeyID, groupID sql.NullInt64
		if err := rows.Scan(
			&item.ID,
			&item.RequestID,
			&item.SessionID,
			&item.RequestCount,
			&userID,
			&item.UserEmail,
			&apiKeyID,
			&item.APIKeyName,
			&groupID,
			&item.GroupName,
			&item.Platform,
			&item.Endpoint,
			&item.Method,
			&item.Model,
			&item.StatusCode,
			&item.RequestBody,
			&item.ResponseBody,
			&item.RequestTruncated,
			&item.ResponseTruncated,
			&item.DurationMS,
			&item.IPAddress,
			&item.UserAgent,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("scan audit log: %w", err)
		}
		if userID.Valid {
			v := userID.Int64
			item.UserID = &v
		}
		if apiKeyID.Valid {
			v := apiKeyID.Int64
			item.APIKeyID = &v
		}
		if groupID.Valid {
			v := groupID.Int64
			item.GroupID = &v
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate audit logs: %w", err)
	}
	return items, paginationResultFromTotal(total, params), nil
}

type auditSessionContent struct {
	RequestID  string    `json:"request_id"`
	Endpoint   string    `json:"endpoint"`
	Method     string    `json:"method"`
	Model      string    `json:"model"`
	StatusCode int       `json:"status_code"`
	Content    string    `json:"content"`
	Truncated  bool      `json:"truncated"`
	DurationMS int       `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

func auditRequestContentFromLog(log *service.AuditLog) auditSessionContent {
	return auditContentFromLog(log, log.RequestBody, log.RequestTruncated)
}

func auditResponseContentFromLog(log *service.AuditLog) auditSessionContent {
	return auditContentFromLog(log, log.ResponseBody, log.ResponseTruncated)
}

func auditContentFromLog(log *service.AuditLog, content string, truncated bool) auditSessionContent {
	return auditSessionContent{
		RequestID:  log.RequestID,
		Endpoint:   log.Endpoint,
		Method:     log.Method,
		Model:      log.Model,
		StatusCode: log.StatusCode,
		Content:    content,
		Truncated:  truncated,
		DurationMS: log.DurationMS,
		CreatedAt:  time.Now(),
	}
}

func buildAuditWhere(filter service.AuditLogFilter) ([]string, []any) {
	where := []string{"l.id IS NOT NULL"}
	args := make([]any, 0)
	add := func(sql string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(sql, len(args)))
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		args = append(args, "%"+search+"%")
		idx := len(args)
		where = append(where, fmt.Sprintf("(l.request_id ILIKE $%d OR l.session_id ILIKE $%d OR l.user_email ILIKE $%d OR l.api_key_name ILIKE $%d OR l.group_name ILIKE $%d)", idx, idx, idx, idx, idx))
	}
	if platform := strings.TrimSpace(filter.Platform); platform != "" && platform != "all" {
		add("l.platform = $%d", platform)
	}
	if model := strings.TrimSpace(filter.Model); model != "" {
		add("l.model ILIKE $%d", "%"+model+"%")
	}
	if endpoint := strings.TrimSpace(filter.Endpoint); endpoint != "" {
		add("l.endpoint ILIKE $%d", "%"+endpoint+"%")
	}
	if filter.From != nil {
		add("l.updated_at >= $%d", *filter.From)
	}
	if filter.To != nil {
		add("l.updated_at <= $%d", *filter.To)
	}
	return where, args
}
