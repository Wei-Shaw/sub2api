package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Excel / Basispoints (BPS) forwarding, ported from ranxi2001/sub2api
// (protocol package: hloolx/codex2api, MIT; see basispoints/NOTICE.md).
//
// ponytail: the fork's Mihomo session proxies, HTTPS image relay, image-limit
// auto-compaction and 403 group moves/recovery probes are not ported. BPS
// uses the account's own proxy and native attachment uploads only.

const excelBPSUpstreamEndpoint = "/basispoints/api/responses"

// Native attachment budget used by the fork when its relay is disabled.
const (
	excelBPSMaxImageMiB = 20
	excelBPSMaxTotalMiB = 32
)

var excelBPSReplay basispoints.ReplayCache
var excelBPSCatalog basispoints.CatalogCache

// AccountExcelBPSRepository is implemented by the account repository.
type AccountExcelBPSRepository interface {
	DisableExcelBPSOn403(ctx context.Context, account *Account) (bool, error)
}

type excelBPSForwardError struct{ code string }

func (e *excelBPSForwardError) Error() string { return "excel BPS: " + e.code }

// BPS uses the account's OAuth credentials, so authentication failures must
// update the same scheduling state as ordinary OpenAI requests. Keep arbitrary
// BPS error text (which may echo request data) out of persisted account reasons.
func (s *OpenAIGatewayService) handleExcelBPSUnauthorized(ctx context.Context, account *Account, status int, headers http.Header, raw []byte) {
	if status != http.StatusUnauthorized || s.rateLimitService == nil {
		return
	}
	fields := map[string]string{"message": "Excel BPS authentication failed"}
	code := extractUpstreamErrorCode(raw)
	if code == "token_invalidated" || code == "token_revoked" {
		fields["code"] = code
	}
	authError := map[string]any{"error": fields}
	if gjson.GetBytes(raw, "detail").String() == "Unauthorized" {
		authError["detail"] = "Unauthorized"
	}
	body, _ := json.Marshal(authError)
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	s.rateLimitService.HandleUpstreamError(stateCtx, account, status, headers, body)
}

func (s *OpenAIGatewayService) disableExcelBPSOn403(ctx context.Context, account *Account) bool {
	if !account.IsExcelBPSAutoDisableOn403Enabled() {
		return false
	}
	repo, ok := s.accountRepo.(AccountExcelBPSRepository)
	if !ok {
		return false
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	changed, err := repo.DisableExcelBPSOn403(stateCtx, account)
	if err != nil {
		// Do not log upstream bodies, credentials or database query arguments.
		logger.LegacyPrintf("service.openai_excel_bps", "auto-disable failed: account_id=%d error_type=%T", account.ID, err)
		return false
	}
	if changed {
		logger.LegacyPrintf("service.openai_excel_bps", "automatically disabled Excel BPS after upstream HTTP 403: account_id=%d", account.ID)
	}
	return changed
}

func excelBPSAccountID(account *Account, accessToken string) string {
	if accountID := strings.TrimSpace(account.GetChatGPTAccountID()); accountID != "" {
		return accountID
	}
	claims, err := openai.DecodeIDToken(accessToken)
	if err != nil || claims.OpenAIAuth == nil {
		return ""
	}
	return strings.TrimSpace(claims.OpenAIAuth.ChatGPTAccountID)
}

func newExcelBPSRequest(ctx context.Context, body []byte, token, accountID string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, basispoints.ResponsesURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = excelBPSHeaders(token, accountID, "text/event-stream")
	return req, nil
}

func excelBPSHeaders(token, accountID, accept string) http.Header {
	return http.Header{
		"Authorization": {"Bearer " + token}, "Chatgpt-Account-Id": {accountID}, "X-Openai-Account-Id": {accountID},
		"X-Basispoints-Auth-Mode": {"chatgpt"}, "Content-Type": {"application/json"}, "Accept": {accept},
		"Origin": {"https://bps.openai.com"}, "User-Agent": {"Mozilla/5.0"},
		"X-Openai-Internal-Basispoints-Client-Product":       {"basispoints-excel-plugin"},
		"X-Openai-Internal-Basispoints-Client-Agent-Profile": {"excel"},
	}
}

func excelBPSProxyURL(account *Account) string {
	if account.Proxy != nil {
		return account.Proxy.URL()
	}
	return ""
}

// BPS deliberately bypasses Codex ticket/cookie injection and OAuth plugins:
// only the selected account's bearer and ChatGPT account ID belong on this host.
func (s *OpenAIGatewayService) forwardExcelBPS(ctx context.Context, c *gin.Context, account *Account, body []byte, start time.Time) (*OpenAIForwardResult, error) {
	fail := func(status int, code, message string) (*OpenAIForwardResult, error) {
		// A compact keepalive may already have committed SSE headers. Otherwise
		// finish a single JSON response so the handler cannot append another error.
		committed := StopOpenAICompactSSEKeepaliveCommitted(c)
		MarkResponseCommitted(c)
		if committed {
			writeOpenAICompactSSEFailureMessage(c, status, code, message)
		} else {
			errorType := "invalid_request_error"
			if status >= 500 {
				errorType = "server_error"
			}
			c.JSON(status, gin.H{"error": gin.H{"type": errorType, "code": code, "message": message}})
		}
		return nil, &excelBPSForwardError{code: code}
	}
	clientCanceled := func() (*OpenAIForwardResult, error) {
		StopOpenAICompactSSEKeepaliveCommitted(c)
		MarkResponseCommitted(c)
		// No response or metered usage exists before headers; no usage row.
		return nil, context.Canceled
	}
	// No output exists yet, so the handler may replay the request on another
	// account within its switch budget unless the client is already gone.
	failoverRateLimited := func(retryAfter string) (*OpenAIForwardResult, error) {
		s.coolDownExcelBPS(ctx, account, retryAfter)
		if isExcelBPSClientCancellation(c, ctx.Err()) {
			return clientCanceled()
		}
		return nil, newExcelBPSRateLimitedFailoverError(retryAfter)
	}

	originalModel := gjson.GetBytes(body, "model").String()
	model := account.GetMappedModel(originalModel)
	stream := gjson.GetBytes(body, "stream").Bool()
	var err error
	if body, err = sjson.SetBytes(body, "model", model); err != nil {
		return fail(400, "basispoints_request_invalid", "Invalid model request")
	}
	identity, _ := resolveOpenAIWSExecutionScope(c, body, getAPIKeyIDFromContext(c))
	if identity != "" {
		if body, err = sjson.SetBytes(body, "prompt_cache_key", identity); err != nil {
			return nil, err
		}
	}
	if isOpenAIResponsesCompactPath(c) {
		if body, err = excelBPSCompactBody(body); err != nil {
			return fail(400, "basispoints_request_invalid", err.Error())
		}
	}
	scope := fmt.Sprintf("account:%d/key:%d/thread:%s", account.ID, getAPIKeyIDFromContext(c), identity)
	if account.IsExcelBPSIgnoreImagesEnabled() {
		if body, err = basispoints.StripInputImages(body); err != nil {
			return fail(400, "basispoints_request_invalid", err.Error())
		}
	}
	if account.IsExcelBPSIgnoreEncryptedContentEnabled() {
		if body, err = basispoints.StripEncryptedContent(body); err != nil {
			return fail(400, "basispoints_request_invalid", err.Error())
		}
	}
	if err = basispoints.ValidateImageBudget(body, excelBPSMaxImageMiB, excelBPSMaxTotalMiB); err != nil {
		return fail(400, "basispoints_request_invalid", err.Error())
	}
	images, err := basispoints.PrepareNativeImages(body)
	if err != nil {
		return fail(400, "basispoints_request_invalid", err.Error())
	}
	// Validate the complete request before uploading any attachments. The native
	// plan contains valid placeholder IDs until all local protocol checks pass.
	var replay *basispoints.ReplayCache
	var catalog *basispoints.CatalogCache
	if identity != "" {
		replay, catalog = &excelBPSReplay, &excelBPSCatalog
	}
	upstreamBody, bridge, err := images.PrepareWithCatalog(scope, replay, catalog)
	if err != nil {
		var contentErr *basispoints.ContentValidationError
		if errors.As(err, &contentErr) {
			return fail(400, "basispoints_request_invalid", err.Error()+" (param "+contentErr.Path+")")
		}
		return fail(400, "basispoints_request_invalid", err.Error())
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		if isExcelBPSClientCancellation(c, err) {
			return clientCanceled()
		}
		return fail(502, "basispoints_auth_unavailable", "Account OAuth credential is unavailable")
	}
	accountID := excelBPSAccountID(account, token)
	if accountID == "" {
		return fail(400, "basispoints_account_id_missing", "Excel BPS requires chatgpt_account_id")
	}
	proxyURL := excelBPSProxyURL(account)
	if images.HasImages() {
		attachmentScope := ""
		if identity != "" {
			attachmentScope = scope + "\x00" + accountID + "\x00" + token
		}
		body, err = images.Upload(ctx, &s.excelBPSAttachments, attachmentScope, func(uploadCtx context.Context, img basispoints.InlineAttachment) (string, error) {
			SetActualOpenAIUpstreamEndpoint(c, "/basispoints/api/attachments")
			return s.uploadExcelBPSAttachment(uploadCtx, account, token, accountID, proxyURL, img)
		})
		if err != nil {
			if isExcelBPSClientCancellation(c, err) {
				return clientCanceled()
			}
			status := http.StatusBadGateway
			var uploadError *excelBPSAttachmentError
			if errors.As(err, &uploadError) {
				status = uploadError.status
			}
			if errors.Is(err, basispoints.ErrAttachmentBusy) {
				status = http.StatusServiceUnavailable
			}
			if status == http.StatusTooManyRequests && uploadError != nil {
				return failoverRateLimited(uploadError.retryAfter)
			}
			logger.LegacyPrintf("service.openai_excel_bps", "attachment upload failed: account_id=%d status=%d", account.ID, status)
			return fail(status, "basispoints_attachment_error", "Excel BPS attachment upload failed; request was not replayed")
		}
		if upstreamBody, bridge, err = bridge.Reprepare(body); err != nil {
			return fail(400, "basispoints_request_invalid", err.Error())
		}
	}
	requestCtx := WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileLongStream))

	SetActualOpenAIUpstreamEndpoint(c, excelBPSUpstreamEndpoint)
	SetOpsUpstreamModel(c, model)
	sent := time.Now()
	req, err := newExcelBPSRequest(requestCtx, upstreamBody, token, accountID)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(sent).Milliseconds())
	if err != nil {
		if isExcelBPSClientCancellation(c, err) {
			return clientCanceled()
		}
		logger.LegacyPrintf("service.openai_excel_bps", "transport failed: account_id=%d error_type=%T", account.ID, err)
		return fail(502, "basispoints_transport_error", "Excel BPS connection failed; request was not replayed after sending")
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if resp.StatusCode == http.StatusBadRequest {
		// Read and close before retrying: the HTTP body owns the account's
		// concurrency slot.
		const maxRejectionBytes = 512 << 10
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxRejectionBytes+1))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(raw))
		if retryBody, retry := prepareExcelBPSInvalidEncryptedRetry(upstreamBody, raw); retry && readErr == nil && len(raw) <= maxRejectionBytes && ctx.Err() == nil {
			retryReq, retryErr := newExcelBPSRequest(requestCtx, retryBody, token, accountID)
			if retryErr != nil {
				return fail(502, "basispoints_transport_error", "Excel BPS recovery request could not be prepared")
			}
			logger.LegacyPrintf("service.openai_excel_bps", "retrying invalid encrypted reasoning once: account_id=%d", account.ID)
			resp, err = s.httpUpstream.Do(retryReq, proxyURL, account.ID, account.Concurrency)
			SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(sent).Milliseconds())
			if err != nil {
				if isExcelBPSClientCancellation(c, err) {
					return clientCanceled()
				}
				return fail(502, "basispoints_transport_error", "Excel BPS recovery connection failed; request was not replayed again")
			}
			upstreamBody = retryBody
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return s.handleExcelBPSErrorResponse(ctx, c, account, resp, token, fail, failoverRateLimited)
	}
	// BPS and Codex share quota. Refresh at the HTTP boundary even if the client
	// disconnects or a later stream/protocol error prevents normal completion.
	s.UpdateCodexUsageSnapshotFromHeaders(ctx, account.ID, resp.Header)
	converted := bridge.StreamWithRepairs(requestCtx, resp.Body,
		func(repairCtx context.Context, failed map[string]any, validation error) (map[string]any, error) {
			correctedBody, err := basispoints.BuildToolRepairRequest(upstreamBody, failed, validation)
			if err != nil {
				return nil, err
			}
			repairResp, err := s.doExcelBPSRepair(repairCtx, account, correctedBody, token, accountID, proxyURL)
			if err != nil {
				return nil, err
			}
			defer func() { _ = repairResp.Body.Close() }()
			stop := context.AfterFunc(repairCtx, func() { _ = repairResp.Body.Close() })
			defer stop()
			upstreamBody = correctedBody
			return basispoints.ReadToolRepairResponse(repairResp.Body)
		},
		func(repairCtx context.Context) (io.ReadCloser, error) {
			repairBody, err := basispoints.RepairRequest(upstreamBody)
			if err != nil {
				return nil, err
			}
			repaired, err := s.doExcelBPSRepair(repairCtx, account, repairBody, token, accountID, proxyURL)
			if err != nil {
				return nil, err
			}
			return repaired.Body, nil
		})
	defer func() { _ = converted.Close() }()
	return s.streamExcelBPSResponse(ctx, c, account, resp, converted, bridge, originalModel, model, stream, start)
}

// excelBPSCompactBody turns /responses/compact into a BPS compaction turn.
func excelBPSCompactBody(body []byte) ([]byte, error) {
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, errors.New("invalid compact request")
	}
	var input []any
	switch v := request["input"].(type) {
	case []any:
		input = v
	case string:
		input = []any{map[string]any{"role": "user", "content": v}}
	default:
		return nil, errors.New("compact requires input")
	}
	request["input"] = append(input, map[string]any{"type": "compaction_trigger"})
	request["tool_choice"] = "none"
	return json.Marshal(request)
}

// doExcelBPSRepair sends one tool-correction request. Output was already
// accepted, so a failure only cools the route; the request is never replayed.
func (s *OpenAIGatewayService) doExcelBPSRepair(ctx context.Context, account *Account, body []byte, token, accountID, proxyURL string) (*http.Response, error) {
	req, err := newExcelBPSRequest(ctx, body, token, accountID)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("excel BPS correction connection failed")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			s.coolDownExcelBPS(ctx, account, resp.Header.Get("Retry-After"))
		}
		s.handleExcelBPSUnauthorized(ctx, account, resp.StatusCode, resp.Header, raw)
		if resp.StatusCode == http.StatusForbidden && gjson.GetBytes(raw, "error.code").String() != "basispoints_model_access_changed" {
			s.disableExcelBPSOn403(ctx, account)
		}
		return nil, fmt.Errorf("excel BPS correction returned HTTP %d", resp.StatusCode)
	}
	s.UpdateCodexUsageSnapshotFromHeaders(ctx, account.ID, resp.Header)
	return resp, nil
}

func (s *OpenAIGatewayService) handleExcelBPSErrorResponse(
	ctx context.Context, c *gin.Context, account *Account, resp *http.Response, token string,
	fail func(int, string, string) (*OpenAIForwardResult, error),
	failoverRateLimited func(string) (*OpenAIForwardResult, error),
) (*OpenAIForwardResult, error) {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	// BPS errors can echo request fields, so redact before storing diagnostics.
	upstreamMessage := fmt.Sprintf("Excel BPS returned HTTP %d", resp.StatusCode)
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		safeBody := excelBPSSanitizeErrorBody(string(raw), token, account)
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail, _ = sanitizeErrorBodyForStorage(safeBody, maxBytes)
		if message := strings.TrimSpace(extractUpstreamErrorMessage([]byte(safeBody))); message != "" {
			upstreamMessage = truncateString(message, 2048)
		}
	}
	// A failover attempt is only an event; the handler records the final state.
	kind := "failover"
	if resp.StatusCode != http.StatusTooManyRequests {
		kind = "http_error"
		setOpsUpstreamError(c, resp.StatusCode, upstreamMessage, upstreamDetail)
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		UpstreamStatusCode: resp.StatusCode, UpstreamRequestID: resp.Header.Get("x-request-id"),
		UpstreamURL: basispoints.ResponsesURL, Kind: kind,
		Message: upstreamMessage, Detail: upstreamDetail, UpstreamResponseBody: upstreamDetail,
	})
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		// BPS throttles its own endpoint: cool only the BPS route and fail over.
		return failoverRateLimited(resp.Header.Get("Retry-After"))
	case http.StatusUnauthorized:
		s.handleExcelBPSUnauthorized(ctx, account, resp.StatusCode, resp.Header, raw)
		return fail(resp.StatusCode, "basispoints_upstream_error", "Excel BPS authentication failed; request was not replayed")
	}
	code := gjson.GetBytes(raw, "error.code").String()
	if code == "basispoints_model_access_changed" {
		return fail(resp.StatusCode, code, "This model is not available on the account's Excel BPS endpoint")
	}
	message := "Excel BPS rejected this request; account scheduling was not changed"
	errorCode := "basispoints_upstream_error"
	if resp.StatusCode == http.StatusBadRequest && isExcelBPSInvalidEncryptedContent(raw) {
		errorCode = "invalid_encrypted_content"
		message = "Excel BPS could not verify encrypted conversation state; resend the original plaintext history or start a new conversation"
	}
	if resp.StatusCode == http.StatusForbidden && s.disableExcelBPSOn403(ctx, account) {
		message = "Excel BPS rejected this request; Excel BPS was automatically disabled for this account; request was not replayed"
	}
	return fail(resp.StatusCode, errorCode, message)
}

func (s *OpenAIGatewayService) streamExcelBPSResponse(
	ctx context.Context, c *gin.Context, account *Account, resp *http.Response, converted io.ReadCloser,
	bridge *basispoints.Bridge, originalModel, model string, stream bool, start time.Time,
) (*OpenAIForwardResult, error) {
	// The bridge sees the body after group policy mapping. Keep the original
	// client effort for usage display, and the BPS-normalized effort for billing.
	requestedEffort := coalesceRequestedReasoningEffort(RequestedReasoningEffortFromContext(ctx), &bridge.RequestedEffort)
	result := &OpenAIForwardResult{Model: originalModel, UpstreamModel: model, UpstreamEndpoint: excelBPSUpstreamEndpoint, Stream: stream, ReasoningEffort: &bridge.Effort, RequestedReasoningEffort: requestedEffort, RequestID: resp.Header.Get("x-request-id")}
	if stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("X-Accel-Buffering", "no")
	}
	scanner := newOpenAISSEReadPump(converted, 16<<20)
	defer scanner.Close()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	keepalive := func() {
		if stream && ctx.Err() == nil {
			_, _ = c.Writer.WriteString(": keepalive\n\n")
			c.Writer.Flush()
		}
	}
	var completed []byte
	var upstreamFailure *basispoints.UpstreamFailure
	terminal := ""
	cacheCreationAsInput := account.IsExcelBPSCacheCreationAsInputEnabled()
	var err error
	for scanner.Next(ctx, 0, heartbeat.C, keepalive) {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			payload := []byte(strings.TrimPrefix(line, "data: "))
			kind := gjson.GetBytes(payload, "type").String()
			s.parseSSEUsageBytes(payload, &result.Usage)
			if cacheCreationAsInput {
				if payload, err = excelBPSDownstreamUsage(payload); err != nil {
					MarkResponseCommitted(c)
					c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"type": "server_error", "code": "basispoints_usage_invalid", "message": "Excel BPS usage could not be normalized"}})
					return nil, &excelBPSForwardError{code: "basispoints_usage_invalid"}
				}
				line = "data: " + string(payload)
			}
			if result.FirstTokenMs == nil && (kind == "response.output_text.delta" || kind == "response.output_item.added") {
				ms := int(time.Since(start).Milliseconds())
				result.FirstTokenMs = &ms
			}
			switch kind {
			case "response.completed", "response.failed", "response.cancelled", "response.incomplete", "error":
				terminal = kind
				completed = []byte(gjson.GetBytes(payload, "response").Raw)
				result.ResponseID = gjson.GetBytes(payload, "response.id").String()
				result.UpstreamResponseModel = gjson.GetBytes(payload, "response.model").String()
				upstreamFailure = basispoints.ParseUpstreamFailure(payload)
				if upstreamFailure != nil {
					// An accepted generation is never replayed.
					setOpsUpstreamError(c, resp.StatusCode, upstreamFailure.Message, "")
					MarkOpsStreamErrorValue(c, OpsStreamError{
						ErrType: upstreamFailure.Type, Code: upstreamFailure.Code, Message: upstreamFailure.Message,
						IntendedStatus: upstreamFailure.Status, CountTowardsSLA: true, NonStream: !stream,
					})
					if upstreamFailure.Status == http.StatusTooManyRequests {
						s.coolDownExcelBPS(ctx, account, resp.Header.Get("Retry-After"))
					}
				}
			}
		}
		if stream {
			if _, err = c.Writer.WriteString(line + "\n"); err != nil {
				result.ClientDisconnect = true
				result.Duration = time.Since(start)
				return result, err
			}
			if line == "" {
				c.Writer.Flush()
			}
		}
	}
	result.Duration = time.Since(start)
	result.UpstreamTerminalEvent = terminal
	if err = scanner.Err(); err != nil || terminal == "" {
		if ctx.Err() != nil {
			result.ClientDisconnect = true
			return result, ctx.Err()
		}
		// Do not replay an incomplete response.
		logger.LegacyPrintf("service.openai_excel_bps", "stream incomplete: account_id=%d error_type=%T", account.ID, err)
		MarkOpsStreamError(c, "basispoints_stream_incomplete", "Excel BPS stream ended before completion", http.StatusBadGateway)
		MarkResponseCommitted(c)
		if stream {
			_, _ = c.Writer.WriteString("event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"type\":\"server_error\",\"code\":\"basispoints_stream_incomplete\",\"message\":\"Upstream stream ended before completion\"}}}\n\n")
			c.Writer.Flush()
		} else {
			c.JSON(502, gin.H{"error": gin.H{"type": "server_error", "code": "basispoints_stream_incomplete", "message": "Excel BPS stream ended before completion"}})
		}
		return result, fmt.Errorf("excel BPS stream incomplete")
	}
	if terminal != "response.completed" {
		MarkResponseCommitted(c)
	}
	if !stream {
		switch {
		case upstreamFailure != nil:
			if StopOpenAICompactSSEKeepaliveCommitted(c) {
				writeOpenAICompactSSEFailureMessage(c, upstreamFailure.Status, upstreamFailure.Code, upstreamFailure.Message)
			} else {
				c.JSON(upstreamFailure.Status, gin.H{"error": upstreamFailure.Details()})
			}
		case terminal != "response.completed":
			c.JSON(502, gin.H{"error": gin.H{"code": "basispoints_protocol_error", "message": "Excel BPS did not complete the response"}})
		default:
			c.Data(200, "application/json", completed)
		}
	}
	if terminal != "response.completed" {
		return result, fmt.Errorf("excel BPS terminal: %s", terminal)
	}
	s.bindHTTPResponseAccount(ctx, c, account, result.ResponseID)
	return result, nil
}

var excelBPSBearerPattern = regexp.MustCompile(`(?i)\bBearer\s+[^\s"',;<>]+`)
var excelBPSURLCredentialsPattern = regexp.MustCompile(`(https?://)[^/\s@]+@`)
var excelBPSAttachmentIDPattern = regexp.MustCompile(`\bfile-[A-Za-z0-9_-]+`)

func excelBPSSanitizeErrorBody(raw, token string, account *Account) string {
	if !json.Valid([]byte(raw)) {
		return ""
	}
	secrets := append([]string{token}, excelBPSAccountSecrets(account)...)
	fields := make(map[string]string)
	for _, key := range []string{"message", "code", "type", "param"} {
		value := gjson.Get(raw, "error."+key)
		if value.Type != gjson.String {
			continue
		}
		clean := value.String()
		for _, secret := range secrets {
			if secret != "" {
				clean = strings.ReplaceAll(clean, secret, "[redacted]")
			}
		}
		clean = excelBPSBearerPattern.ReplaceAllString(clean, "Bearer [redacted]")
		clean = excelBPSURLCredentialsPattern.ReplaceAllString(clean, "${1}[redacted]@")
		clean = excelBPSAttachmentIDPattern.ReplaceAllString(clean, "file-[redacted]")
		clean = sanitizeUpstreamErrorMessage(clean)
		fields[key] = truncateString(logredact.RedactText(clean, "authorization", "api_key", "apikey", "token", "secret", "key", "cookie", "ticket", "recovery_ticket"), 2048)
	}
	encoded, _ := json.Marshal(map[string]any{"error": fields})
	return string(encoded)
}

func excelBPSAccountSecrets(account *Account) []string {
	var secrets []string
	for _, key := range []string{"access_token", "refresh_token", "id_token", "api_key", "session_key", "cookie"} {
		if value := account.GetCredential(key); value != "" {
			secrets = append(secrets, value)
		}
	}
	if account.Proxy != nil && account.Proxy.Password != "" {
		secrets = append(secrets, account.Proxy.Password)
	}
	return secrets
}
