package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// GetRequestTrace 按 request_id 聚合四张表的追溯视图。
//
// 四张表的写入时机不同：usage_logs 只有成功请求、ops_error_logs 只有失败请求、
// upstream_attempts 每个失败请求的每一跳、account_status_events 只在账号状态变化时。
// 所以这里对每段查询都按"可能不存在"处理，最终由 service 判断是否整体 NotFound。
func (r *opsRepository) GetRequestTrace(ctx context.Context, requestID string) (*service.OpsRequestTrace, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ops repository is not configured")
	}

	trace := &service.OpsRequestTrace{RequestID: requestID}

	if err := r.loadTraceUsage(ctx, requestID, trace); err != nil {
		return nil, err
	}
	if err := r.loadTraceAttempts(ctx, requestID, trace); err != nil {
		return nil, err
	}
	if err := r.loadTraceStatusEvents(ctx, requestID, trace); err != nil {
		return nil, err
	}
	if err := r.loadTraceError(ctx, requestID, trace); err != nil {
		return nil, err
	}
	return trace, nil
}

func (r *opsRepository) loadTraceUsage(ctx context.Context, requestID string, trace *service.OpsRequestTrace) error {
	const query = `
		SELECT user_id, api_key_id, account_id, model,
		       COALESCE(requested_model, ''), COALESCE(upstream_model, ''),
		       input_tokens, output_tokens, total_cost, duration_ms,
		       COALESCE(first_token_ms, 0), created_at
		FROM usage_logs
		WHERE request_id = $1
		ORDER BY created_at DESC
		LIMIT 1`

	var usage service.OpsTraceUsage
	err := r.db.QueryRowContext(ctx, query, requestID).Scan(
		&usage.UserID, &usage.APIKeyID, &usage.AccountID, &usage.Model,
		&usage.RequestedModel, &usage.UpstreamModel,
		&usage.InputTokens, &usage.OutputTokens, &usage.TotalCost, &usage.DurationMs,
		&usage.FirstTokenMs, &usage.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load trace usage: %w", err)
	}
	trace.Usage = &usage
	return nil
}

func (r *opsRepository) loadTraceAttempts(ctx context.Context, requestID string, trace *service.OpsRequestTrace) error {
	const query = `
		SELECT ` + traceAttemptColumns + `
		FROM upstream_attempts
		WHERE request_id = $1
		ORDER BY attempt_no, id`

	items, err := queryTraceAttempts(ctx, r.db, query, requestID)
	if err != nil {
		return err
	}
	trace.Attempts = append(trace.Attempts, items...)
	return nil
}

func (r *opsRepository) loadTraceStatusEvents(ctx context.Context, requestID string, trace *service.OpsRequestTrace) error {
	const query = `
		SELECT ` + traceStatusEventColumns + `
		FROM account_status_events
		WHERE request_id = $1
		ORDER BY created_at, id`

	items, err := queryTraceStatusEvents(ctx, r.db, query, requestID)
	if err != nil {
		return err
	}
	trace.StatusEvents = append(trace.StatusEvents, items...)
	return nil
}

func (r *opsRepository) loadTraceError(ctx context.Context, requestID string, trace *service.OpsRequestTrace) error {
	const query = `
		SELECT id, error_phase, error_type, status_code, upstream_status_code, error_message, created_at
		FROM ops_error_logs
		WHERE request_id = $1
		ORDER BY created_at DESC
		LIMIT 1`

	var (
		item         service.OpsTraceError
		statusCode   sql.NullInt64
		upstreamCode sql.NullInt64
		errorMessage sql.NullString
	)
	err := r.db.QueryRowContext(ctx, query, requestID).Scan(
		&item.ID, &item.ErrorPhase, &item.ErrorType,
		&statusCode, &upstreamCode, &errorMessage, &item.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load trace error: %w", err)
	}
	item.StatusCode = traceNullInt64ToIntPtr(statusCode)
	item.UpstreamStatusCode = traceNullInt64ToIntPtr(upstreamCode)
	item.ErrorMessage = traceNullStringPtr(errorMessage)
	trace.Error = &item
	return nil
}

func traceNullStringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func traceNullInt64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}

func traceNullInt64ToIntPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

func traceNullTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}
