package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *OpsService) ListSystemLogs(ctx context.Context, filter *OpsSystemLogFilter) (*OpsSystemLogList, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return &OpsSystemLogList{
			Logs:     []*OpsSystemLog{REDACTED,
			Total:    0,
			Page:     1,
			PageSize: 50,
	REDACTED, nil
REDACTED
	if filter == nil {
		filter = &OpsSystemLogFilter{REDACTED
REDACTED
	if filter.Page <= 0 {
		filter.Page = 1
REDACTED
	if filter.PageSize <= 0 {
		filter.PageSize = 50
REDACTED
	if filter.PageSize > 200 {
		filter.PageSize = 200
REDACTED

	result, err := s.opsRepo.ListSystemLogs(ctx, filter)
	if err != nil {
		return nil, infraerrors.InternalServer("OPS_SYSTEM_LOG_LIST_FAILED", "Failed to list system logs").WithCause(err)
REDACTED
	return result, nil
REDACTED

func (s *OpsService) CleanupSystemLogs(ctx context.Context, filter *OpsSystemLogCleanupFilter, operatorID int64) (int64, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return 0, err
REDACTED
	if s.opsRepo == nil {
		return 0, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if operatorID <= 0 {
		return 0, infraerrors.BadRequest("OPS_SYSTEM_LOG_CLEANUP_INVALID_OPERATOR", "invalid operator")
REDACTED
	if filter == nil {
		filter = &OpsSystemLogCleanupFilter{REDACTED
REDACTED
	if filter.EndTime != nil && filter.StartTime != nil && filter.StartTime.After(*filter.EndTime) {
		return 0, infraerrors.BadRequest("OPS_SYSTEM_LOG_CLEANUP_INVALID_RANGE", "invalid time range")
REDACTED

	deletedRows, err := s.opsRepo.DeleteSystemLogs(ctx, filter)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
	REDACTED
		if strings.Contains(strings.ToLower(err.Error()), "requires at least one filter") {
			return 0, infraerrors.BadRequest("OPS_SYSTEM_LOG_CLEANUP_FILTER_REQUIRED", "cleanup requires at least one filter condition")
	REDACTED
		return 0, infraerrors.InternalServer("OPS_SYSTEM_LOG_CLEANUP_FAILED", "Failed to cleanup system logs").WithCause(err)
REDACTED

	if auditErr := s.opsRepo.InsertSystemLogCleanupAudit(ctx, &OpsSystemLogCleanupAudit{
		CreatedAt:   time.Now().UTC(),
		OperatorID:  operatorID,
		Conditions:  marshalSystemLogCleanupConditions(filter),
		DeletedRows: deletedRows,
REDACTED); auditErr != nil {
		// 审计失败不影响主流程，避免运维清理被阻塞。
		log.Printf("[OpsSystemLog] cleanup audit failed: %v", auditErr)
REDACTED
	return deletedRows, nil
REDACTED

func marshalSystemLogCleanupConditions(filter *OpsSystemLogCleanupFilter) string {
	if filter == nil {
		return "{REDACTED"
REDACTED
	payload := map[string]any{
		"level":             strings.TrimSpace(filter.Level),
		"component":         strings.TrimSpace(filter.Component),
		"request_id":        strings.TrimSpace(filter.RequestID),
		"client_request_id": strings.TrimSpace(filter.ClientRequestID),
		"platform":          strings.TrimSpace(filter.Platform),
		"model":             strings.TrimSpace(filter.Model),
		"query":             strings.TrimSpace(filter.Query),
REDACTED
	if filter.UserID != nil {
		payload["user_id"] = *filter.UserID
REDACTED
	if filter.APIKeyID != nil {
		payload["api_key_id"] = *filter.APIKeyID
REDACTED
	if filter.AccountID != nil {
		payload["account_id"] = *filter.AccountID
REDACTED
	if filter.StartTime != nil && !filter.StartTime.IsZero() {
		payload["start_time"] = filter.StartTime.UTC().Format(time.RFC3339Nano)
REDACTED
	if filter.EndTime != nil && !filter.EndTime.IsZero() {
		payload["end_time"] = filter.EndTime.UTC().Format(time.RFC3339Nano)
REDACTED
	raw, err := json.Marshal(payload)
	if err != nil {
		return "{REDACTED"
REDACTED
	return string(raw)
REDACTED

func (s *OpsService) GetSystemLogSinkHealth() OpsSystemLogSinkHealth {
	if s == nil || s.systemLogSink == nil {
		return OpsSystemLogSinkHealth{REDACTED
REDACTED
	return s.systemLogSink.Health()
REDACTED
