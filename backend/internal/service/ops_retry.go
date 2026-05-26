package service

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/lib/pq"
)

const (
	OpsRetryModeClient   = "client"
	OpsRetryModeUpstream = "upstream"
)

const (
	opsRetryStatusRunning   = "running"
	opsRetryStatusSucceeded = "succeeded"
	opsRetryStatusFailed    = "failed"
)

const (
	opsRetryTimeout             = 60 * time.Second
	opsRetryCaptureBytesLimit   = 64 * 1024
	opsRetryResponsePreviewMax  = 8 * 1024
	opsRetryMinIntervalPerError = 10 * time.Second
	opsRetryMaxAccountSwitches  = 3
)

// Protocol constants for ops retry forwarding. These mirror the gateway
// package protocol constants to avoid a circular import.
const (
	opsRetryProtocolAnthropic       = "anthropic"
	opsRetryProtocolResponses       = "responses"
	opsRetryProtocolGemini          = "gemini"
	opsRetryProtocolChatCompletions = "chat_completions"
	opsRetryProtocolImages          = "images"
)

var opsRetryRequestHeaderAllowlist = map[string]bool{
	"anthropic-beta":    true,
	"anthropic-version": true,
}

type opsRetryRequestType string

const (
	opsRetryTypeMessages  opsRetryRequestType = "messages"
	opsRetryTypeOpenAI    opsRetryRequestType = "openai_responses"
	opsRetryTypeGeminiV1B opsRetryRequestType = "gemini_v1beta"
)

const (
	OpsRetryModeUpstreamEvent = "upstream_event"
)

func (s *OpsService) RetryError(ctx context.Context, requestedByUserID int64, errorID int64, mode string, pinnedAccountID *int64) (*OpsRetryResult, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}

	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case OpsRetryModeClient, OpsRetryModeUpstream:
	default:
		return nil, infraerrors.BadRequest("OPS_RETRY_INVALID_MODE", "mode must be client or upstream")
	}

	errorLog, err := s.GetErrorLogByID(ctx, errorID)
	if err != nil {
		return nil, err
	}
	if errorLog == nil {
		return nil, infraerrors.NotFound("OPS_ERROR_NOT_FOUND", "ops error log not found")
	}
	if strings.TrimSpace(errorLog.RequestBody) == "" {
		return nil, infraerrors.BadRequest("OPS_RETRY_NO_REQUEST_BODY", "No request body found to retry")
	}

	var pinned *int64
	if mode == OpsRetryModeUpstream {
		if pinnedAccountID != nil && *pinnedAccountID > 0 {
			pinned = pinnedAccountID
		} else if errorLog.AccountID != nil && *errorLog.AccountID > 0 {
			pinned = errorLog.AccountID
		} else {
			return nil, infraerrors.BadRequest("OPS_RETRY_PINNED_ACCOUNT_REQUIRED", "pinned_account_id is required for upstream retry")
		}
	}

	return s.retryWithErrorLog(ctx, requestedByUserID, errorID, mode, mode, pinned, errorLog)
}

// RetryUpstreamEvent retries a specific upstream attempt captured inside ops_error_logs.upstream_errors.
// idx is 0-based. It always pins the original event account_id.
func (s *OpsService) RetryUpstreamEvent(ctx context.Context, requestedByUserID int64, errorID int64, idx int) (*OpsRetryResult, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if idx < 0 {
		return nil, infraerrors.BadRequest("OPS_RETRY_INVALID_UPSTREAM_IDX", "invalid upstream idx")
	}

	errorLog, err := s.GetErrorLogByID(ctx, errorID)
	if err != nil {
		return nil, err
	}
	if errorLog == nil {
		return nil, infraerrors.NotFound("OPS_ERROR_NOT_FOUND", "ops error log not found")
	}

	ev, err := s.extractUpstreamEvent(errorLog, idx)
	if err != nil {
		return nil, err
	}

	override := *errorLog
	override.RequestBody = strings.TrimSpace(ev.UpstreamRequestBody)
	pinned := ev.AccountID

	return s.retryWithErrorLog(ctx, requestedByUserID, errorID, OpsRetryModeUpstreamEvent, OpsRetryModeUpstream, &pinned, &override)
}

func (s *OpsService) extractUpstreamEvent(errorLog *OpsErrorLogDetail, idx int) (*OpsUpstreamErrorEvent, error) {
	events, err := ParseOpsUpstreamErrors(errorLog.UpstreamErrors)
	if err != nil {
		return nil, infraerrors.BadRequest("OPS_RETRY_UPSTREAM_EVENTS_INVALID", "invalid upstream_errors")
	}
	if idx >= len(events) {
		return nil, infraerrors.BadRequest("OPS_RETRY_UPSTREAM_IDX_OOB", "upstream idx out of range")
	}
	ev := events[idx]
	if ev == nil {
		return nil, infraerrors.BadRequest("OPS_RETRY_UPSTREAM_EVENT_MISSING", "upstream event missing")
	}
	if ev.AccountID <= 0 {
		return nil, infraerrors.BadRequest("OPS_RETRY_PINNED_ACCOUNT_REQUIRED", "account_id is required for upstream retry")
	}
	if strings.TrimSpace(ev.UpstreamRequestBody) == "" {
		return nil, infraerrors.BadRequest("OPS_RETRY_UPSTREAM_NO_REQUEST_BODY", "No upstream request body found to retry")
	}
	return ev, nil
}

func (s *OpsService) retryWithErrorLog(ctx context.Context, requestedByUserID int64, errorID int64, mode string, execMode string, pinnedAccountID *int64, errorLog *OpsErrorLogDetail) (*OpsRetryResult, error) {
	if err := s.checkRetryThrottle(ctx, errorID); err != nil {
		return nil, err
	}

	if errorLog == nil || strings.TrimSpace(errorLog.RequestBody) == "" {
		return nil, infraerrors.BadRequest("OPS_RETRY_NO_REQUEST_BODY", "No request body found to retry")
	}

	pinned := resolvePinnedAccount(execMode, pinnedAccountID, errorLog)
	if execMode == OpsRetryModeUpstream && pinned == nil {
		return nil, infraerrors.BadRequest("OPS_RETRY_PINNED_ACCOUNT_REQUIRED", "account_id is required for upstream retry")
	}

	startedAt := time.Now()
	attemptID, err := s.insertRetryAttempt(ctx, requestedByUserID, errorID, mode, pinned, startedAt)
	if err != nil {
		return nil, err
	}

	result := s.buildInitialRetryResult(attemptID, mode, pinned, startedAt)

	execCtx, cancel := context.WithTimeout(ctx, opsRetryTimeout)
	defer cancel()

	execRes := s.executeRetry(execCtx, errorLog, execMode, pinned)
	s.finalizeRetryResult(result, execRes, startedAt)
	s.persistRetryOutcome(ctx, result, attemptID, errorID, requestedByUserID)

	return result, nil
}

func (s *OpsService) checkRetryThrottle(ctx context.Context, errorID int64) error {
	latest, err := s.opsRepo.GetLatestRetryAttemptForError(ctx, errorID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return infraerrors.InternalServer("OPS_RETRY_LOAD_LATEST_FAILED", "Failed to check retry status").WithCause(err)
	}
	if latest == nil {
		return nil
	}

	if strings.EqualFold(latest.Status, opsRetryStatusRunning) || strings.EqualFold(latest.Status, "queued") {
		return infraerrors.Conflict("OPS_RETRY_IN_PROGRESS", "A retry is already in progress for this error")
	}

	lastAttemptAt := resolveLastAttemptTime(latest)
	if time.Since(lastAttemptAt) < opsRetryMinIntervalPerError {
		return infraerrors.Conflict("OPS_RETRY_TOO_FREQUENT", "Please wait before retrying this error again")
	}
	return nil
}

func resolveLastAttemptTime(latest *OpsRetryAttempt) time.Time {
	if latest.FinishedAt != nil && !latest.FinishedAt.IsZero() {
		return *latest.FinishedAt
	}
	if latest.StartedAt != nil && !latest.StartedAt.IsZero() {
		return *latest.StartedAt
	}
	return latest.CreatedAt
}

func resolvePinnedAccount(execMode string, pinnedAccountID *int64, errorLog *OpsErrorLogDetail) *int64 {
	if execMode != OpsRetryModeUpstream {
		return nil
	}
	if pinnedAccountID != nil && *pinnedAccountID > 0 {
		return pinnedAccountID
	}
	if errorLog.AccountID != nil && *errorLog.AccountID > 0 {
		return errorLog.AccountID
	}
	return nil
}

func (s *OpsService) insertRetryAttempt(ctx context.Context, requestedByUserID int64, errorID int64, mode string, pinned *int64, startedAt time.Time) (int64, error) {
	attemptID, err := s.opsRepo.InsertRetryAttempt(ctx, &OpsInsertRetryAttemptInput{
		RequestedByUserID: requestedByUserID,
		SourceErrorID:     errorID,
		Mode:              mode,
		PinnedAccountID:   pinned,
		Status:            opsRetryStatusRunning,
		StartedAt:         startedAt,
	})
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == "23505" {
			return 0, infraerrors.Conflict("OPS_RETRY_IN_PROGRESS", "A retry is already in progress for this error")
		}
		return 0, infraerrors.InternalServer("OPS_RETRY_CREATE_ATTEMPT_FAILED", "Failed to create retry attempt").WithCause(err)
	}
	return attemptID, nil
}

func (s *OpsService) buildInitialRetryResult(attemptID int64, mode string, pinned *int64, startedAt time.Time) *OpsRetryResult {
	return &OpsRetryResult{
		AttemptID:       attemptID,
		Mode:            mode,
		Status:          opsRetryStatusFailed,
		PinnedAccountID: pinned,
		StartedAt:       startedAt,
	}
}

func (s *OpsService) finalizeRetryResult(result *OpsRetryResult, execRes *opsRetryExecution, startedAt time.Time) {
	finishedAt := time.Now()
	result.FinishedAt = finishedAt
	result.DurationMs = finishedAt.Sub(startedAt).Milliseconds()

	if execRes == nil {
		return
	}
	result.Status = execRes.status
	result.UsedAccountID = execRes.usedAccountID
	result.HTTPStatusCode = execRes.httpStatusCode
	result.UpstreamRequestID = execRes.upstreamRequestID
	result.ResponsePreview = execRes.responsePreview
	result.ResponseTruncated = execRes.responseTruncated
	result.ErrorMessage = execRes.errorMessage
}

func (s *OpsService) persistRetryOutcome(ctx context.Context, result *OpsRetryResult, attemptID int64, errorID int64, requestedByUserID int64) {
	updateCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	input := buildRetryUpdateInput(result, attemptID)
	if err := s.opsRepo.UpdateRetryAttempt(updateCtx, input); err != nil {
		log.Printf("[Ops] UpdateRetryAttempt failed: %v", err)
		return
	}
	if strings.EqualFold(input.Status, opsRetryStatusSucceeded) {
		finishedAt := result.FinishedAt
		if err := s.opsRepo.UpdateErrorResolution(updateCtx, errorID, true, &requestedByUserID, &attemptID, &finishedAt); err != nil {
			log.Printf("[Ops] UpdateErrorResolution failed: %v", err)
		}
	}
}

func buildRetryUpdateInput(result *OpsRetryResult, attemptID int64) *OpsUpdateRetryAttemptInput {
	finalStatus := result.Status
	if strings.TrimSpace(finalStatus) == "" {
		finalStatus = opsRetryStatusFailed
	}
	success := strings.EqualFold(finalStatus, opsRetryStatusSucceeded)

	var updateErrMsg *string
	if msg := strings.TrimSpace(result.ErrorMessage); msg != "" {
		updateErrMsg = &msg
	}

	httpStatus := result.HTTPStatusCode
	upstreamReqID := result.UpstreamRequestID
	preview := result.ResponsePreview
	truncated := result.ResponseTruncated

	return &OpsUpdateRetryAttemptInput{
		ID:                attemptID,
		Status:            finalStatus,
		FinishedAt:        result.FinishedAt,
		DurationMs:        result.DurationMs,
		Success:           &success,
		HTTPStatusCode:    &httpStatus,
		UpstreamRequestID: &upstreamReqID,
		UsedAccountID:     result.UsedAccountID,
		ResponsePreview:   &preview,
		ResponseTruncated: &truncated,
		ErrorMessage:      updateErrMsg,
	}
}
