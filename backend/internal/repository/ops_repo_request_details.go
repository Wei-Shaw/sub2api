package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) ListRequestDetails(ctx context.Context, filter *service.OpsRequestDetailFilter) ([]*service.OpsRequestDetail, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, fmt.Errorf("nil ops repository")
	}

	page, pageSize, startTime, endTime := filter.Normalize()
	offset := (page - 1) * pageSize

	conditions := make([]string, 0, 16)
	args := make([]any, 0, 24)

	// Placeholders $1/$2 reserved for time window inside the CTE.
	args = append(args, startTime.UTC(), endTime.UTC())

	addCondition := func(condition string, values ...any) {
		conditions = append(conditions, condition)
		args = append(args, values...)
	}

	if filter != nil {
		if kind := strings.TrimSpace(strings.ToLower(filter.Kind)); kind != "" && kind != "all" {
			if kind != string(service.OpsRequestKindSuccess) && kind != string(service.OpsRequestKindError) {
				return nil, 0, fmt.Errorf("invalid kind")
			}
			addCondition(fmt.Sprintf("kind = $%d", len(args)+1), kind)
		}

		if platform := strings.TrimSpace(strings.ToLower(filter.Platform)); platform != "" {
			addCondition(fmt.Sprintf("platform = $%d", len(args)+1), platform)
		}
		if filter.GroupID != nil && *filter.GroupID > 0 {
			addCondition(fmt.Sprintf("group_id = $%d", len(args)+1), *filter.GroupID)
		}

		if filter.UserID != nil && *filter.UserID > 0 {
			addCondition(fmt.Sprintf("user_id = $%d", len(args)+1), *filter.UserID)
		}
		if filter.APIKeyID != nil && *filter.APIKeyID > 0 {
			addCondition(fmt.Sprintf("api_key_id = $%d", len(args)+1), *filter.APIKeyID)
		}
		if filter.AccountID != nil && *filter.AccountID > 0 {
			addCondition(fmt.Sprintf("account_id = $%d", len(args)+1), *filter.AccountID)
		}

		if model := strings.TrimSpace(filter.Model); model != "" {
			addCondition(fmt.Sprintf("model = $%d", len(args)+1), model)
		}
		if targetGroup := strings.TrimSpace(strings.ToLower(filter.RoutingTargetGroup)); targetGroup != "" {
			addCondition(fmt.Sprintf("routing_target_group = $%d", len(args)+1), targetGroup)
		}
		if scheduleLayer := strings.TrimSpace(filter.RoutingScheduleLayer); scheduleLayer != "" {
			addCondition(fmt.Sprintf("routing_schedule_layer = $%d", len(args)+1), scheduleLayer)
		}
		if requestID := strings.TrimSpace(filter.RequestID); requestID != "" {
			addCondition(fmt.Sprintf("request_id = $%d", len(args)+1), requestID)
		}
		if q := strings.TrimSpace(filter.Query); q != "" {
			like := "%" + strings.ToLower(q) + "%"
			startIdx := len(args) + 1
			addCondition(
				fmt.Sprintf("(LOWER(COALESCE(request_id,'')) LIKE $%d OR LOWER(COALESCE(model,'')) LIKE $%d OR LOWER(COALESCE(message,'')) LIKE $%d)",
					startIdx, startIdx+1, startIdx+2,
				),
				like, like, like,
			)
		}

		if filter.MinDurationMs != nil {
			addCondition(fmt.Sprintf("duration_ms >= $%d", len(args)+1), *filter.MinDurationMs)
		}
		if filter.MaxDurationMs != nil {
			addCondition(fmt.Sprintf("duration_ms <= $%d", len(args)+1), *filter.MaxDurationMs)
		}
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	cte := `
WITH combined AS (
  SELECT
    'success'::TEXT AS kind,
    ul.created_at AS created_at,
    ul.request_id AS request_id,
    COALESCE(NULLIF(g.platform, ''), NULLIF(a.platform, ''), '') AS platform,
    COALESCE(NULLIF(ul.requested_model, ''), ul.model) AS model,
    COALESCE(NULLIF(ul.routing_target_group, ''), '') AS routing_target_group,
    COALESCE(NULLIF(ul.routing_schedule_layer, ''), '') AS routing_schedule_layer,
    COALESCE(ul.routing_selected_account_id, ul.account_id) AS routing_selected_account_id,
    NULLIF(ul.routing_selected_account_name, '') AS routing_selected_account_name,
    COALESCE(NULLIF(ul.routing_effective_model, ''), NULLIF(ul.upstream_model, ''), '') AS routing_effective_model,
    ul.routing_failover_count AS routing_failover_count,
    COALESCE(NULLIF(ul.routing_failover_final_reason, ''), '') AS routing_failover_final_reason,
    ul.duration_ms AS duration_ms,
    NULL::INT AS status_code,
    NULL::BIGINT AS error_id,
    NULL::TEXT AS phase,
    NULL::TEXT AS severity,
    NULL::TEXT AS message,
    ul.user_id AS user_id,
    ul.api_key_id AS api_key_id,
    ul.account_id AS account_id,
    ul.group_id AS group_id,
    ul.stream AS stream
  FROM usage_logs ul
  LEFT JOIN groups g ON g.id = ul.group_id
  LEFT JOIN accounts a ON a.id = ul.account_id
  WHERE ul.created_at >= $1 AND ul.created_at < $2

  UNION ALL

  SELECT
    'error'::TEXT AS kind,
    o.created_at AS created_at,
    COALESCE(NULLIF(o.request_id,''), NULLIF(o.client_request_id,''), '') AS request_id,
    COALESCE(NULLIF(o.platform, ''), NULLIF(g.platform, ''), NULLIF(a.platform, ''), '') AS platform,
    COALESCE(NULLIF(o.routing_requested_model, ''), NULLIF(o.model, ''), '') AS model,
    COALESCE(NULLIF(o.routing_target_group, ''), '') AS routing_target_group,
    COALESCE(NULLIF(o.routing_schedule_layer, ''), '') AS routing_schedule_layer,
    COALESCE(o.routing_selected_account_id, o.account_id) AS routing_selected_account_id,
    NULLIF(o.routing_selected_account_name, '') AS routing_selected_account_name,
    COALESCE(NULLIF(o.routing_effective_model, ''), NULLIF(o.upstream_model, ''), '') AS routing_effective_model,
    o.routing_failover_count AS routing_failover_count,
    COALESCE(NULLIF(o.routing_failover_final_reason, ''), '') AS routing_failover_final_reason,
    o.duration_ms AS duration_ms,
    o.status_code AS status_code,
    o.id AS error_id,
    o.error_phase AS phase,
    o.severity AS severity,
    o.error_message AS message,
    o.user_id AS user_id,
    o.api_key_id AS api_key_id,
    o.account_id AS account_id,
    o.group_id AS group_id,
    o.stream AS stream
  FROM ops_error_logs o
  LEFT JOIN groups g ON g.id = o.group_id
  LEFT JOIN accounts a ON a.id = o.account_id
  WHERE o.created_at >= $1 AND o.created_at < $2
    AND COALESCE(o.status_code, 0) >= 400
)
`

	countQuery := fmt.Sprintf(`%s SELECT COUNT(1) FROM combined %s`, cte, where)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		if err == sql.ErrNoRows {
			total = 0
		} else {
			return nil, 0, err
		}
	}

	sort := "ORDER BY created_at DESC"
	if filter != nil {
		switch strings.TrimSpace(strings.ToLower(filter.Sort)) {
		case "", "created_at_desc":
			// default
		case "duration_desc":
			sort = "ORDER BY duration_ms DESC NULLS LAST, created_at DESC"
		default:
			return nil, 0, fmt.Errorf("invalid sort")
		}
	}

	listQuery := fmt.Sprintf(`
%s
SELECT
  kind,
  created_at,
  request_id,
  platform,
  model,
  routing_target_group,
  routing_schedule_layer,
  routing_selected_account_id,
  routing_selected_account_name,
  routing_effective_model,
  routing_failover_count,
  routing_failover_final_reason,
  duration_ms,
  status_code,
  error_id,
  phase,
  severity,
  message,
  user_id,
  api_key_id,
  account_id,
  group_id,
  stream
FROM combined
%s
%s
LIMIT $%d OFFSET $%d
`, cte, where, sort, len(args)+1, len(args)+2)

	listArgs := append(append([]any{}, args...), pageSize, offset)
	rows, err := r.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	toIntPtr := func(v sql.NullInt64) *int {
		if !v.Valid {
			return nil
		}
		i := int(v.Int64)
		return &i
	}
	toInt64Ptr := func(v sql.NullInt64) *int64 {
		if !v.Valid {
			return nil
		}
		i := v.Int64
		return &i
	}

	out := make([]*service.OpsRequestDetail, 0, pageSize)
	for rows.Next() {
		var (
			kind                       string
			createdAt                  time.Time
			requestID                  sql.NullString
			platform                   sql.NullString
			model                      sql.NullString
			routingTargetGroup         sql.NullString
			routingScheduleLayer       sql.NullString
			routingSelectedAccountID   sql.NullInt64
			routingSelectedAccountName sql.NullString
			routingEffectiveModel      sql.NullString
			routingFailoverCount       sql.NullInt64
			routingFailoverFinalReason sql.NullString

			durationMs sql.NullInt64
			statusCode sql.NullInt64
			errorID    sql.NullInt64

			phase    sql.NullString
			severity sql.NullString
			message  sql.NullString

			userID    sql.NullInt64
			apiKeyID  sql.NullInt64
			accountID sql.NullInt64
			groupID   sql.NullInt64

			stream bool
		)

		if err := rows.Scan(
			&kind,
			&createdAt,
			&requestID,
			&platform,
			&model,
			&routingTargetGroup,
			&routingScheduleLayer,
			&routingSelectedAccountID,
			&routingSelectedAccountName,
			&routingEffectiveModel,
			&routingFailoverCount,
			&routingFailoverFinalReason,
			&durationMs,
			&statusCode,
			&errorID,
			&phase,
			&severity,
			&message,
			&userID,
			&apiKeyID,
			&accountID,
			&groupID,
			&stream,
		); err != nil {
			return nil, 0, err
		}

		item := &service.OpsRequestDetail{
			Kind:                       service.OpsRequestKind(kind),
			CreatedAt:                  createdAt,
			RequestID:                  strings.TrimSpace(requestID.String),
			Platform:                   strings.TrimSpace(platform.String),
			Model:                      strings.TrimSpace(model.String),
			RoutingTargetGroup:         strings.TrimSpace(routingTargetGroup.String),
			RoutingScheduleLayer:       strings.TrimSpace(routingScheduleLayer.String),
			RoutingSelectedAccountID:   toInt64Ptr(routingSelectedAccountID),
			RoutingSelectedAccountName: nullStringPtr(routingSelectedAccountName),
			RoutingEffectiveModel:      strings.TrimSpace(routingEffectiveModel.String),
			RoutingFailoverCount:       toIntPtr(routingFailoverCount),
			RoutingFailoverFinalReason: strings.TrimSpace(routingFailoverFinalReason.String),

			DurationMs: toIntPtr(durationMs),
			StatusCode: toIntPtr(statusCode),
			ErrorID:    toInt64Ptr(errorID),
			Phase:      phase.String,
			Severity:   severity.String,
			Message:    message.String,

			UserID:    toInt64Ptr(userID),
			APIKeyID:  toInt64Ptr(apiKeyID),
			AccountID: toInt64Ptr(accountID),
			GroupID:   toInt64Ptr(groupID),

			Stream: stream,
		}

		if item.Platform == "" {
			item.Platform = "unknown"
		}

		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	trimmed := strings.TrimSpace(value.String)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
