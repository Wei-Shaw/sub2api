package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// CreateRequestPayload 留存 cyber_policy 命中的完整原始请求体。
// 请求体按需求原样写入（不截断、不脱敏），随 content_moderation_logs 级联删除。
func (r *contentModerationRepository) CreateRequestPayload(ctx context.Context, payload *service.CyberPolicyRequestPayload) error {
	if payload == nil {
		return nil
	}
	var userID any
	if payload.UserID != nil {
		userID = *payload.UserID
	}
	var apiKeyID any
	if payload.APIKeyID != nil {
		apiKeyID = *payload.APIKeyID
	}
	var groupID any
	if payload.GroupID != nil {
		groupID = *payload.GroupID
	}
	err := r.db.QueryRowContext(ctx, `
INSERT INTO cyber_policy_request_payloads (
    moderation_log_id, request_id, user_id, user_email, api_key_id, group_id,
    endpoint, protocol, model, request_body, body_bytes
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11
) RETURNING id`,
		payload.ModerationLogID, payload.RequestID, userID, payload.UserEmail, apiKeyID, groupID,
		payload.Endpoint, payload.Protocol, payload.Model, payload.RequestBody, payload.BodyBytes,
	).Scan(&payload.ID)
	if err != nil {
		return fmt.Errorf("insert cyber policy request payload: %w", err)
	}
	return nil
}

// GetRequestPayload 按风控记录 ID 取回请求体；未留存时返回 (nil, nil)。
func (r *contentModerationRepository) GetRequestPayload(ctx context.Context, moderationLogID int64) (*service.CyberPolicyRequestPayload, error) {
	var (
		payload  service.CyberPolicyRequestPayload
		userID   sql.NullInt64
		apiKeyID sql.NullInt64
		groupID  sql.NullInt64
	)
	err := r.db.QueryRowContext(ctx, `
SELECT id, moderation_log_id, request_id, user_id, user_email, api_key_id, group_id,
       endpoint, protocol, model, request_body, body_bytes, created_at
FROM cyber_policy_request_payloads
WHERE moderation_log_id = $1`, moderationLogID).Scan(
		&payload.ID, &payload.ModerationLogID, &payload.RequestID, &userID, &payload.UserEmail,
		&apiKeyID, &groupID, &payload.Endpoint, &payload.Protocol, &payload.Model,
		&payload.RequestBody, &payload.BodyBytes, &payload.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get cyber policy request payload: %w", err)
	}
	if userID.Valid {
		payload.UserID = &userID.Int64
	}
	if apiKeyID.Valid {
		payload.APIKeyID = &apiKeyID.Int64
	}
	if groupID.Valid {
		payload.GroupID = &groupID.Int64
	}
	return &payload, nil
}
