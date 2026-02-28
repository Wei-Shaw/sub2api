package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
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

var opsRetryRequestHeaderAllowlist = map[string]bool{
	"anthropic-beta":    true,
	"anthropic-version": true,
REDACTED

type opsRetryRequestType string

const (
	opsRetryTypeMessages  opsRetryRequestType = "messages"
	opsRetryTypeOpenAI    opsRetryRequestType = "openai_responses"
	opsRetryTypeGeminiV1B opsRetryRequestType = "gemini_v1beta"
)

type limitedResponseWriter struct {
	header      http.Header
	wroteHeader bool

	limit        int
	totalWritten int64
	buf          bytes.Buffer
REDACTED

func newLimitedResponseWriter(limit int) *limitedResponseWriter {
	if limit <= 0 {
		limit = 1
REDACTED
	return &limitedResponseWriter{
		header: make(http.Header),
		limit:  limit,
REDACTED
REDACTED

func (w *limitedResponseWriter) Header() http.Header {
	return w.header
REDACTED

func (w *limitedResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
REDACTED
	w.wroteHeader = true
REDACTED

func (w *limitedResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
REDACTED
	w.totalWritten += int64(len(p))

	if w.buf.Len() < w.limit {
		remaining := w.limit - w.buf.Len()
		if len(p) > remaining {
			_, _ = w.buf.Write(p[:remaining])
	REDACTED else {
			_, _ = w.buf.Write(p)
	REDACTED
REDACTED

	// Pretend we wrote everything to avoid upstream/client code treating it as an error.
	return len(p), nil
REDACTED

func (w *limitedResponseWriter) Flush() {REDACTED

func (w *limitedResponseWriter) bodyBytes() []byte {
	return w.buf.Bytes()
REDACTED

func (w *limitedResponseWriter) truncated() bool {
	return w.totalWritten > int64(w.limit)
REDACTED

const (
	OpsRetryModeUpstreamEvent = "upstream_event"
)

func (s *OpsService) RetryError(ctx context.Context, requestedByUserID int64, errorID int64, mode string, pinnedAccountID *int64) (*OpsRetryResult, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED

	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case OpsRetryModeClient, OpsRetryModeUpstream:
	default:
		return nil, infraerrors.BadRequest("OPS_RETRY_INVALID_MODE", "mode must be client or upstream")
REDACTED

	errorLog, err := s.GetErrorLogByID(ctx, errorID)
	if err != nil {
		return nil, err
REDACTED
	if errorLog == nil {
		return nil, infraerrors.NotFound("OPS_ERROR_NOT_FOUND", "ops error log not found")
REDACTED
	if strings.TrimSpace(errorLog.RequestBody) == "" {
		return nil, infraerrors.BadRequest("OPS_RETRY_NO_REQUEST_BODY", "No request body found to retry")
REDACTED

	var pinned *int64
	if mode == OpsRetryModeUpstream {
		if pinnedAccountID != nil && *pinnedAccountID > 0 {
			pinned = pinnedAccountID
	REDACTED else if errorLog.AccountID != nil && *errorLog.AccountID > 0 {
			pinned = errorLog.AccountID
	REDACTED else {
			return nil, infraerrors.BadRequest("OPS_RETRY_PINNED_ACCOUNT_REQUIRED", "pinned_account_id is required for upstream retry")
	REDACTED
REDACTED

	return s.retryWithErrorLog(ctx, requestedByUserID, errorID, mode, mode, pinned, errorLog)
REDACTED

// RetryUpstreamEvent retries a specific upstream attempt captured inside ops_error_logs.upstream_errors.
// idx is 0-based. It always pins the original event account_id.
func (s *OpsService) RetryUpstreamEvent(ctx context.Context, requestedByUserID int64, errorID int64, idx int) (*OpsRetryResult, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if idx < 0 {
		return nil, infraerrors.BadRequest("OPS_RETRY_INVALID_UPSTREAM_IDX", "invalid upstream idx")
REDACTED

	errorLog, err := s.GetErrorLogByID(ctx, errorID)
	if err != nil {
		return nil, err
REDACTED
	if errorLog == nil {
		return nil, infraerrors.NotFound("OPS_ERROR_NOT_FOUND", "ops error log not found")
REDACTED

	events, err := ParseOpsUpstreamErrors(errorLog.UpstreamErrors)
	if err != nil {
		return nil, infraerrors.BadRequest("OPS_RETRY_UPSTREAM_EVENTS_INVALID", "invalid upstream_errors")
REDACTED
	if idx >= len(events) {
		return nil, infraerrors.BadRequest("OPS_RETRY_UPSTREAM_IDX_OOB", "upstream idx out of range")
REDACTED
	ev := events[idx]
	if ev == nil {
		return nil, infraerrors.BadRequest("OPS_RETRY_UPSTREAM_EVENT_MISSING", "upstream event missing")
REDACTED
	if ev.AccountID <= 0 {
		return nil, infraerrors.BadRequest("OPS_RETRY_PINNED_ACCOUNT_REQUIRED", "account_id is required for upstream retry")
REDACTED

	upstreamBody := strings.TrimSpace(ev.UpstreamRequestBody)
	if upstreamBody == "" {
		return nil, infraerrors.BadRequest("OPS_RETRY_UPSTREAM_NO_REQUEST_BODY", "No upstream request body found to retry")
REDACTED

	override := *errorLog
	override.RequestBody = upstreamBody
	pinned := ev.AccountID

	// Persist as upstream_event, execute as upstream pinned retry.
	return s.retryWithErrorLog(ctx, requestedByUserID, errorID, OpsRetryModeUpstreamEvent, OpsRetryModeUpstream, &pinned, &override)
REDACTED

func (s *OpsService) retryWithErrorLog(ctx context.Context, requestedByUserID int64, errorID int64, mode string, execMode string, pinnedAccountID *int64, errorLog *OpsErrorLogDetail) (*OpsRetryResult, error) {
	latest, err := s.opsRepo.GetLatestRetryAttemptForError(ctx, errorID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, infraerrors.InternalServer("OPS_RETRY_LOAD_LATEST_FAILED", "Failed to check retry status").WithCause(err)
REDACTED
	if latest != nil {
		if strings.EqualFold(latest.Status, opsRetryStatusRunning) || strings.EqualFold(latest.Status, "queued") {
			return nil, infraerrors.Conflict("OPS_RETRY_IN_PROGRESS", "A retry is already in progress for this error")
	REDACTED

		lastAttemptAt := latest.CreatedAt
		if latest.FinishedAt != nil && !latest.FinishedAt.IsZero() {
			lastAttemptAt = *latest.FinishedAt
	REDACTED else if latest.StartedAt != nil && !latest.StartedAt.IsZero() {
			lastAttemptAt = *latest.StartedAt
	REDACTED

		if time.Since(lastAttemptAt) < opsRetryMinIntervalPerError {
			return nil, infraerrors.Conflict("OPS_RETRY_TOO_FREQUENT", "Please wait before retrying this error again")
	REDACTED
REDACTED

	if errorLog == nil || strings.TrimSpace(errorLog.RequestBody) == "" {
		return nil, infraerrors.BadRequest("OPS_RETRY_NO_REQUEST_BODY", "No request body found to retry")
REDACTED

	var pinned *int64
	if execMode == OpsRetryModeUpstream {
		if pinnedAccountID != nil && *pinnedAccountID > 0 {
			pinned = pinnedAccountID
	REDACTED else if errorLog.AccountID != nil && *errorLog.AccountID > 0 {
			pinned = errorLog.AccountID
	REDACTED else {
			return nil, infraerrors.BadRequest("OPS_RETRY_PINNED_ACCOUNT_REQUIRED", "account_id is required for upstream retry")
	REDACTED
REDACTED

	startedAt := time.Now()
	attemptID, err := s.opsRepo.InsertRetryAttempt(ctx, &OpsInsertRetryAttemptInput{
		RequestedByUserID: requestedByUserID,
		SourceErrorID:     errorID,
		Mode:              mode,
		PinnedAccountID:   pinned,
		Status:            opsRetryStatusRunning,
		StartedAt:         startedAt,
REDACTED)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == "23505" {
			return nil, infraerrors.Conflict("OPS_RETRY_IN_PROGRESS", "A retry is already in progress for this error")
	REDACTED
		return nil, infraerrors.InternalServer("OPS_RETRY_CREATE_ATTEMPT_FAILED", "Failed to create retry attempt").WithCause(err)
REDACTED

	result := &OpsRetryResult{
		AttemptID:         attemptID,
		Mode:              mode,
		Status:            opsRetryStatusFailed,
		PinnedAccountID:   pinned,
		HTTPStatusCode:    0,
		UpstreamRequestID: "",
		ResponsePreview:   "",
		ResponseTruncated: false,
		ErrorMessage:      "",
		StartedAt:         startedAt,
REDACTED

	execCtx, cancel := context.WithTimeout(ctx, opsRetryTimeout)
	defer cancel()

	execRes := s.executeRetry(execCtx, errorLog, execMode, pinned)

	finishedAt := time.Now()
	result.FinishedAt = finishedAt
	result.DurationMs = finishedAt.Sub(startedAt).Milliseconds()

	if execRes != nil {
		result.Status = execRes.status
		result.UsedAccountID = execRes.usedAccountID
		result.HTTPStatusCode = execRes.httpStatusCode
		result.UpstreamRequestID = execRes.upstreamRequestID
		result.ResponsePreview = execRes.responsePreview
		result.ResponseTruncated = execRes.responseTruncated
		result.ErrorMessage = execRes.errorMessage
REDACTED

	updateCtx, updateCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer updateCancel()

	var updateErrMsg *string
	if strings.TrimSpace(result.ErrorMessage) != "" {
		msg := result.ErrorMessage
		updateErrMsg = &msg
REDACTED
	// Keep legacy result_request_id empty; use upstream_request_id instead.
	var resultRequestID *string

	finalStatus := result.Status
	if strings.TrimSpace(finalStatus) == "" {
		finalStatus = opsRetryStatusFailed
REDACTED

	success := strings.EqualFold(finalStatus, opsRetryStatusSucceeded)
	httpStatus := result.HTTPStatusCode
	upstreamReqID := result.UpstreamRequestID
	usedAccountID := result.UsedAccountID
	preview := result.ResponsePreview
	truncated := result.ResponseTruncated

	if err := s.opsRepo.UpdateRetryAttempt(updateCtx, &OpsUpdateRetryAttemptInput{
		ID:                attemptID,
		Status:            finalStatus,
		FinishedAt:        finishedAt,
		DurationMs:        result.DurationMs,
		Success:           &success,
		HTTPStatusCode:    &httpStatus,
		UpstreamRequestID: &upstreamReqID,
		UsedAccountID:     usedAccountID,
		ResponsePreview:   &preview,
		ResponseTruncated: &truncated,
		ResultRequestID:   resultRequestID,
		ErrorMessage:      updateErrMsg,
REDACTED); err != nil {
		log.Printf("[Ops] UpdateRetryAttempt failed: %v", err)
REDACTED else if success {
		if err := s.opsRepo.UpdateErrorResolution(updateCtx, errorID, true, &requestedByUserID, &attemptID, &finishedAt); err != nil {
			log.Printf("[Ops] UpdateErrorResolution failed: %v", err)
	REDACTED
REDACTED

	return result, nil
REDACTED

type opsRetryExecution struct {
	status string

	usedAccountID     *int64
	httpStatusCode    int
	upstreamRequestID string

	responsePreview   string
	responseTruncated bool

	errorMessage string
REDACTED

func (s *OpsService) executeRetry(ctx context.Context, errorLog *OpsErrorLogDetail, mode string, pinnedAccountID *int64) *opsRetryExecution {
	if errorLog == nil {
		return &opsRetryExecution{
			status:       opsRetryStatusFailed,
			errorMessage: "missing error log",
	REDACTED
REDACTED

	reqType := detectOpsRetryType(errorLog.RequestPath)
	bodyBytes := []byte(errorLog.RequestBody)

	switch reqType {
	case opsRetryTypeMessages:
		bodyBytes = FilterThinkingBlocksForRetry(bodyBytes)
	case opsRetryTypeOpenAI, opsRetryTypeGeminiV1B:
		// No-op
REDACTED

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case OpsRetryModeUpstream:
		if pinnedAccountID == nil || *pinnedAccountID <= 0 {
			return &opsRetryExecution{
				status:       opsRetryStatusFailed,
				errorMessage: "pinned_account_id required for upstream retry",
		REDACTED
	REDACTED
		return s.executePinnedRetry(ctx, reqType, errorLog, bodyBytes, *pinnedAccountID)
	case OpsRetryModeClient:
		return s.executeClientRetry(ctx, reqType, errorLog, bodyBytes)
	default:
		return &opsRetryExecution{
			status:       opsRetryStatusFailed,
			errorMessage: "invalid retry mode",
	REDACTED
REDACTED
REDACTED

func detectOpsRetryType(path string) opsRetryRequestType {
	p := strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.Contains(p, "/responses"):
		return opsRetryTypeOpenAI
	case strings.Contains(p, "/v1beta/"):
		return opsRetryTypeGeminiV1B
	default:
		return opsRetryTypeMessages
REDACTED
REDACTED

func (s *OpsService) executePinnedRetry(ctx context.Context, reqType opsRetryRequestType, errorLog *OpsErrorLogDetail, body []byte, pinnedAccountID int64) *opsRetryExecution {
	if s.accountRepo == nil {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "account repository not available"REDACTED
REDACTED

	account, err := s.accountRepo.GetByID(ctx, pinnedAccountID)
	if err != nil {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: fmt.Sprintf("account not found: %v", err)REDACTED
REDACTED
	if account == nil {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "account not found"REDACTED
REDACTED
	if !account.IsSchedulable() {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "account is not schedulable"REDACTED
REDACTED
	if errorLog.GroupID != nil && *errorLog.GroupID > 0 {
		if !containsInt64(account.GroupIDs, *errorLog.GroupID) {
			return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "pinned account is not in the same group as the original request"REDACTED
	REDACTED
REDACTED

	var release func()
	if s.concurrencyService != nil {
		acq, err := s.concurrencyService.AcquireAccountSlot(ctx, account.ID, account.Concurrency)
		if err != nil {
			return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: fmt.Sprintf("acquire account slot failed: %v", err)REDACTED
	REDACTED
		if acq == nil || !acq.Acquired {
			return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "account concurrency limit reached"REDACTED
	REDACTED
		release = acq.ReleaseFunc
REDACTED
	if release != nil {
		defer release()
REDACTED

	usedID := account.ID
	exec := s.executeWithAccount(ctx, reqType, errorLog, body, account)
	exec.usedAccountID = &usedID
	if exec.status == "" {
		exec.status = opsRetryStatusFailed
REDACTED
	return exec
REDACTED

func (s *OpsService) executeClientRetry(ctx context.Context, reqType opsRetryRequestType, errorLog *OpsErrorLogDetail, body []byte) *opsRetryExecution {
	groupID := errorLog.GroupID
	if groupID == nil || *groupID <= 0 {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "group_id missing; cannot reselect account"REDACTED
REDACTED

	model, stream, parsedErr := extractRetryModelAndStream(reqType, errorLog, body)
	if parsedErr != nil {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: parsedErr.Error()REDACTED
REDACTED
	_ = stream

	excluded := make(map[int64]struct{REDACTED)
	switches := 0

	for {
		if switches >= opsRetryMaxAccountSwitches {
			return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "retry failed after exhausting account failovers"REDACTED
	REDACTED

		selection, selErr := s.selectAccountForRetry(ctx, reqType, groupID, model, excluded)
		if selErr != nil {
			return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: selErr.Error()REDACTED
	REDACTED
		if selection == nil || selection.Account == nil {
			return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "no available accounts"REDACTED
	REDACTED

		account := selection.Account
		if !selection.Acquired || selection.ReleaseFunc == nil {
			excluded[account.ID] = struct{REDACTED{REDACTED
			switches++
			continue
	REDACTED

		attemptCtx := ctx
		if switches > 0 {
			attemptCtx = WithAccountSwitchCount(attemptCtx, switches, false)
	REDACTED
		exec := func() *opsRetryExecution {
			defer selection.ReleaseFunc()
			return s.executeWithAccount(attemptCtx, reqType, errorLog, body, account)
	REDACTED()

		if exec != nil {
			if exec.status == opsRetryStatusSucceeded {
				usedID := account.ID
				exec.usedAccountID = &usedID
				return exec
		REDACTED
			// If the gateway services ask for failover, try another account.
			if s.isFailoverError(exec.errorMessage) {
				excluded[account.ID] = struct{REDACTED{REDACTED
				switches++
				continue
		REDACTED
			usedID := account.ID
			exec.usedAccountID = &usedID
			return exec
	REDACTED

		excluded[account.ID] = struct{REDACTED{REDACTED
		switches++
REDACTED
REDACTED

func (s *OpsService) selectAccountForRetry(ctx context.Context, reqType opsRetryRequestType, groupID *int64, model string, excludedIDs map[int64]struct{REDACTED) (*AccountSelectionResult, error) {
	switch reqType {
	case opsRetryTypeOpenAI:
		if s.openAIGatewayService == nil {
			return nil, fmt.Errorf("openai gateway service not available")
	REDACTED
		return s.openAIGatewayService.SelectAccountWithLoadAwareness(ctx, groupID, "", model, excludedIDs)
	case opsRetryTypeGeminiV1B, opsRetryTypeMessages:
		if s.gatewayService == nil {
			return nil, fmt.Errorf("gateway service not available")
	REDACTED
		return s.gatewayService.SelectAccountWithLoadAwareness(ctx, groupID, "", model, excludedIDs, "") // 重试不使用会话限制
	default:
		return nil, fmt.Errorf("unsupported retry type: %s", reqType)
REDACTED
REDACTED

func extractRetryModelAndStream(reqType opsRetryRequestType, errorLog *OpsErrorLogDetail, body []byte) (model string, stream bool, err error) {
	switch reqType {
	case opsRetryTypeMessages:
		parsed, parseErr := ParseGatewayRequest(body, domain.PlatformAnthropic)
		if parseErr != nil {
			return "", false, fmt.Errorf("failed to parse messages request body: %w", parseErr)
	REDACTED
		return parsed.Model, parsed.Stream, nil
	case opsRetryTypeOpenAI:
		var v struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
	REDACTED
		if err := json.Unmarshal(body, &v); err != nil {
			return "", false, fmt.Errorf("failed to parse openai request body: %w", err)
	REDACTED
		return strings.TrimSpace(v.Model), v.Stream, nil
	case opsRetryTypeGeminiV1B:
		if strings.TrimSpace(errorLog.Model) == "" {
			return "", false, fmt.Errorf("missing model for gemini v1beta retry")
	REDACTED
		return strings.TrimSpace(errorLog.Model), errorLog.Stream, nil
	default:
		return "", false, fmt.Errorf("unsupported retry type: %s", reqType)
REDACTED
REDACTED

func (s *OpsService) executeWithAccount(ctx context.Context, reqType opsRetryRequestType, errorLog *OpsErrorLogDetail, body []byte, account *Account) *opsRetryExecution {
	if account == nil {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "missing account"REDACTED
REDACTED

	c, w := newOpsRetryContext(ctx, errorLog)

	var err error
	switch reqType {
	case opsRetryTypeOpenAI:
		if s.openAIGatewayService == nil {
			return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "openai gateway service not available"REDACTED
	REDACTED
		_, err = s.openAIGatewayService.Forward(ctx, c, account, body)
	case opsRetryTypeGeminiV1B:
		if s.geminiCompatService == nil || s.antigravityGatewayService == nil {
			return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "gemini services not available"REDACTED
	REDACTED
		modelName := strings.TrimSpace(errorLog.Model)
		action := "generateContent"
		if errorLog.Stream {
			action = "streamGenerateContent"
	REDACTED
		if account.Platform == PlatformAntigravity {
			_, err = s.antigravityGatewayService.ForwardGemini(ctx, c, account, modelName, action, errorLog.Stream, body, false)
	REDACTED else {
			_, err = s.geminiCompatService.ForwardNative(ctx, c, account, modelName, action, errorLog.Stream, body)
	REDACTED
	case opsRetryTypeMessages:
		switch account.Platform {
		case PlatformAntigravity:
			if s.antigravityGatewayService == nil {
				return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "antigravity gateway service not available"REDACTED
		REDACTED
			_, err = s.antigravityGatewayService.Forward(ctx, c, account, body, false)
		case PlatformGemini:
			if s.geminiCompatService == nil {
				return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "gemini gateway service not available"REDACTED
		REDACTED
			_, err = s.geminiCompatService.Forward(ctx, c, account, body)
		default:
			if s.gatewayService == nil {
				return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "gateway service not available"REDACTED
		REDACTED
			parsedReq, parseErr := ParseGatewayRequest(body, domain.PlatformAnthropic)
			if parseErr != nil {
				return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "failed to parse request body"REDACTED
		REDACTED
			_, err = s.gatewayService.Forward(ctx, c, account, parsedReq)
	REDACTED
	default:
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "unsupported retry type"REDACTED
REDACTED

	statusCode := http.StatusOK
	if c != nil && c.Writer != nil {
		statusCode = c.Writer.Status()
REDACTED

	upstreamReqID := extractUpstreamRequestID(c)
	preview, truncated := extractResponsePreview(w)

	exec := &opsRetryExecution{
		status:            opsRetryStatusFailed,
		httpStatusCode:    statusCode,
		upstreamRequestID: upstreamReqID,
		responsePreview:   preview,
		responseTruncated: truncated,
		errorMessage:      "",
REDACTED

	if err == nil && statusCode < 400 {
		exec.status = opsRetryStatusSucceeded
		return exec
REDACTED

	if err != nil {
		exec.errorMessage = err.Error()
REDACTED else {
		exec.errorMessage = fmt.Sprintf("upstream returned status %d", statusCode)
REDACTED

	return exec
REDACTED

func newOpsRetryContext(ctx context.Context, errorLog *OpsErrorLogDetail) (*gin.Context, *limitedResponseWriter) {
	w := newLimitedResponseWriter(opsRetryCaptureBytesLimit)
	c, _ := gin.CreateTestContext(w)

	path := "/"
	if errorLog != nil && strings.TrimSpace(errorLog.RequestPath) != "" {
		path = errorLog.RequestPath
REDACTED

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost"+path, bytes.NewReader(nil))
	req.Header.Set("content-type", "application/json")
	if errorLog != nil && strings.TrimSpace(errorLog.UserAgent) != "" {
		req.Header.Set("user-agent", errorLog.UserAgent)
REDACTED
	// Restore a minimal, whitelisted subset of request headers to improve retry fidelity
	// (e.g. anthropic-beta / anthropic-version). Never replay auth credentials.
	if errorLog != nil && strings.TrimSpace(errorLog.RequestHeaders) != "" {
		var stored map[string]string
		if err := json.Unmarshal([]byte(errorLog.RequestHeaders), &stored); err == nil {
			for k, v := range stored {
				key := strings.TrimSpace(k)
				if key == "" {
					continue
			REDACTED
				if !opsRetryRequestHeaderAllowlist[strings.ToLower(key)] {
					continue
			REDACTED
				val := strings.TrimSpace(v)
				if val == "" {
					continue
			REDACTED
				req.Header.Set(key, val)
		REDACTED
	REDACTED
REDACTED

	c.Request = req
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	return c, w
REDACTED

func extractUpstreamRequestID(c *gin.Context) string {
	if c == nil || c.Writer == nil {
		return ""
REDACTED
	h := c.Writer.Header()
	if h == nil {
		return ""
REDACTED
	for _, key := range []string{"x-request-id", "X-Request-Id", "X-Request-ID"REDACTED {
		if v := strings.TrimSpace(h.Get(key)); v != "" {
			return v
	REDACTED
REDACTED
	return ""
REDACTED

func extractResponsePreview(w *limitedResponseWriter) (preview string, truncated bool) {
	if w == nil {
		return "", false
REDACTED
	b := bytes.TrimSpace(w.bodyBytes())
	if len(b) == 0 {
		return "", w.truncated()
REDACTED
	if len(b) > opsRetryResponsePreviewMax {
		return string(b[:opsRetryResponsePreviewMax]), true
REDACTED
	return string(b), w.truncated()
REDACTED

func containsInt64(items []int64, needle int64) bool {
	for _, v := range items {
		if v == needle {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func (s *OpsService) isFailoverError(message string) bool {
	msg := strings.ToLower(strings.TrimSpace(message))
	if msg == "" {
		return false
REDACTED
	return strings.Contains(msg, "upstream error:") && strings.Contains(msg, "failover")
REDACTED
