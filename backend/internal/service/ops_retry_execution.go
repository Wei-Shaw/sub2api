package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type opsRetryExecution struct {
	status string

	usedAccountID     *int64
	httpStatusCode    int
	upstreamRequestID string

	responsePreview   string
	responseTruncated bool

	errorMessage string
}

func (s *OpsService) executeRetry(ctx context.Context, errorLog *OpsErrorLogDetail, mode string, pinnedAccountID *int64) *opsRetryExecution {
	if errorLog == nil {
		return &opsRetryExecution{
			status:       opsRetryStatusFailed,
			errorMessage: "missing error log",
		}
	}

	reqType := detectOpsRetryType(errorLog.RequestPath)
	bodyBytes := []byte(errorLog.RequestBody)

	if reqType == opsRetryTypeMessages {
		bodyBytes = FilterThinkingBlocksForRetry(bodyBytes)
	}

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case OpsRetryModeUpstream:
		if pinnedAccountID == nil || *pinnedAccountID <= 0 {
			return &opsRetryExecution{
				status:       opsRetryStatusFailed,
				errorMessage: "pinned_account_id required for upstream retry",
			}
		}
		return s.executePinnedRetry(ctx, reqType, errorLog, bodyBytes, *pinnedAccountID)
	case OpsRetryModeClient:
		return s.executeClientRetry(ctx, reqType, errorLog, bodyBytes)
	default:
		return &opsRetryExecution{
			status:       opsRetryStatusFailed,
			errorMessage: "invalid retry mode",
		}
	}
}

func detectOpsRetryType(path string) opsRetryRequestType {
	p := strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.Contains(p, "/responses"), strings.Contains(p, "/images/"):
		return opsRetryTypeOpenAI
	case strings.Contains(p, "/v1beta/"):
		return opsRetryTypeGeminiV1B
	default:
		return opsRetryTypeMessages
	}
}

// opsRetryProtocol maps a retry request type + request path to the
// protocol string expected by the gateway provider.
func opsRetryProtocol(reqType opsRetryRequestType, path string) string {
	switch reqType {
	case opsRetryTypeOpenAI:
		p := strings.ToLower(strings.TrimSpace(path))
		if strings.Contains(p, "/images/") {
			return opsRetryProtocolImages
		}
		if strings.Contains(p, "/chat/completions") {
			return opsRetryProtocolChatCompletions
		}
		return opsRetryProtocolResponses
	case opsRetryTypeGeminiV1B:
		return opsRetryProtocolGemini
	default:
		return opsRetryProtocolAnthropic
	}
}

func (s *OpsService) executePinnedRetry(ctx context.Context, reqType opsRetryRequestType, errorLog *OpsErrorLogDetail, body []byte, pinnedAccountID int64) *opsRetryExecution {
	if s.accountRepo == nil {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "account repository not available"}
	}

	account, err := s.accountRepo.GetByID(ctx, pinnedAccountID)
	if err != nil {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: fmt.Sprintf("account not found: %v", err)}
	}
	if account == nil {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "account not found"}
	}
	if err := s.validatePinnedAccount(account, errorLog); err != nil {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: err.Error()}
	}

	release := s.acquireAccountSlot(ctx, account)
	if release != nil {
		defer release()
	}

	usedID := account.ID
	exec := s.executeWithAccount(ctx, reqType, errorLog, body, account)
	exec.usedAccountID = &usedID
	if exec.status == "" {
		exec.status = opsRetryStatusFailed
	}
	return exec
}

func (s *OpsService) validatePinnedAccount(account *Account, errorLog *OpsErrorLogDetail) error {
	if !account.IsSchedulable() {
		return fmt.Errorf("account is not schedulable")
	}
	if errorLog.GroupID != nil && *errorLog.GroupID > 0 {
		if !containsInt64(account.GroupIDs, *errorLog.GroupID) {
			return fmt.Errorf("pinned account is not in the same group as the original request")
		}
	}
	return nil
}

// acquireAccountSlot attempts to acquire a concurrency slot for the account.
// Returns the release function, or nil if concurrency limiting is disabled
// or acquisition failed (retry will proceed without slot protection).
func (s *OpsService) acquireAccountSlot(ctx context.Context, account *Account) func() {
	if s.concurrencyService == nil {
		return nil
	}
	acq, err := s.concurrencyService.AcquireAccountSlot(ctx, account.ID, account.Concurrency)
	if err != nil || acq == nil || !acq.Acquired {
		return nil
	}
	return acq.ReleaseFunc
}

func (s *OpsService) executeClientRetry(ctx context.Context, reqType opsRetryRequestType, errorLog *OpsErrorLogDetail, body []byte) *opsRetryExecution {
	groupID := errorLog.GroupID
	if groupID == nil || *groupID <= 0 {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "group_id missing; cannot reselect account"}
	}

	model, _, parsedErr := extractRetryModelAndStream(reqType, errorLog, body)
	if parsedErr != nil {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: parsedErr.Error()}
	}

	return s.clientRetryLoop(ctx, reqType, errorLog, body, groupID, model)
}

func (s *OpsService) clientRetryLoop(ctx context.Context, reqType opsRetryRequestType, errorLog *OpsErrorLogDetail, body []byte, groupID *int64, model string) *opsRetryExecution {
	excluded := make(map[int64]struct{})
	switches := 0

	for {
		if switches >= opsRetryMaxAccountSwitches {
			return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "retry failed after exhausting account failovers"}
		}

		exec := s.tryClientRetryAccount(ctx, reqType, errorLog, body, groupID, model, excluded, &switches)
		if exec != nil {
			return exec
		}
	}
}

func (s *OpsService) tryClientRetryAccount(ctx context.Context, reqType opsRetryRequestType, errorLog *OpsErrorLogDetail, body []byte, groupID *int64, model string, excluded map[int64]struct{}, switches *int) *opsRetryExecution {
	selection, selErr := s.selectAccountForRetry(ctx, groupID, model, excluded)
	if selErr != nil {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: selErr.Error()}
	}
	if selection == nil || selection.Account == nil {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: ErrNoAvailableAccounts.Error()}
	}

	account := selection.Account
	if !selection.Acquired || selection.ReleaseFunc == nil {
		excluded[account.ID] = struct{}{}
		*switches++
		return nil // signal: try next account
	}

	attemptCtx := ctx
	if *switches > 0 {
		attemptCtx = WithAccountSwitchCount(attemptCtx, *switches, false)
	}
	exec := func() *opsRetryExecution {
		defer selection.ReleaseFunc()
		return s.executeWithAccount(attemptCtx, reqType, errorLog, body, account)
	}()

	return s.classifyClientRetryResult(exec, account, excluded, switches)
}

func (s *OpsService) classifyClientRetryResult(exec *opsRetryExecution, account *Account, excluded map[int64]struct{}, switches *int) *opsRetryExecution {
	if exec == nil {
		excluded[account.ID] = struct{}{}
		*switches++
		return nil
	}
	if exec.status == opsRetryStatusSucceeded {
		usedID := account.ID
		exec.usedAccountID = &usedID
		return exec
	}
	if s.isFailoverError(exec.errorMessage) {
		excluded[account.ID] = struct{}{}
		*switches++
		return nil
	}
	usedID := account.ID
	exec.usedAccountID = &usedID
	return exec
}

// selectAccountForRetry uses the unified GatewayService scheduler for all
// platforms. The scheduler automatically filters by model compatibility
// and group membership.
func (s *OpsService) selectAccountForRetry(ctx context.Context, groupID *int64, model string, excludedIDs map[int64]struct{}) (*AccountSelectionResult, error) {
	if s.gatewayService == nil {
		return nil, fmt.Errorf("gateway service not available")
	}
	return s.gatewayService.SelectAccountWithLoadAwareness(
		ctx, groupID, "", model, excludedIDs, "", int64(0),
	)
}

func extractRetryModelAndStream(reqType opsRetryRequestType, errorLog *OpsErrorLogDetail, body []byte) (model string, stream bool, err error) {
	switch reqType {
	case opsRetryTypeMessages:
		parsed, parseErr := ParseGatewayRequest(body, PlatformAnthropic)
		if parseErr != nil {
			return "", false, fmt.Errorf("failed to parse messages request body: %w", parseErr)
		}
		return parsed.Model, parsed.Stream, nil
	case opsRetryTypeOpenAI:
		return extractOpenAIRetryModel(body)
	case opsRetryTypeGeminiV1B:
		if strings.TrimSpace(errorLog.Model) == "" {
			return "", false, fmt.Errorf("missing model for gemini v1beta retry")
		}
		return strings.TrimSpace(errorLog.Model), errorLog.Stream, nil
	default:
		return "", false, fmt.Errorf("unsupported retry type: %s", reqType)
	}
}

func extractOpenAIRetryModel(body []byte) (string, bool, error) {
	var v struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", false, fmt.Errorf("failed to parse openai request body: %w", err)
	}
	return strings.TrimSpace(v.Model), v.Stream, nil
}

// executeWithAccount dispatches the retry request to the upstream provider
// via OpsRetryForwarder. The provider is resolved from the account's
// platform through the gateway ProviderRegistry.
func (s *OpsService) executeWithAccount(ctx context.Context, reqType opsRetryRequestType, errorLog *OpsErrorLogDetail, body []byte, account *Account) *opsRetryExecution {
	if account == nil {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "missing account"}
	}
	if s.retryForwarder == nil {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: "retry forwarder not available"}
	}
	if !s.retryForwarder.HasProvider(account.Platform) {
		return &opsRetryExecution{status: opsRetryStatusFailed, errorMessage: fmt.Sprintf("no provider for platform %q", account.Platform)}
	}

	c, w := newOpsRetryContext(ctx, errorLog)
	params := buildRetryForwardParams(reqType, errorLog, body, account, c)

	err := s.retryForwarder.Forward(ctx, w, params)
	return buildRetryExecution(c, w, err)
}

func buildRetryForwardParams(reqType opsRetryRequestType, errorLog *OpsErrorLogDetail, body []byte, account *Account, c *gin.Context) *OpsRetryForwardParams {
	return &OpsRetryForwardParams{
		Account:      account,
		RawBody:      body,
		Model:        strings.TrimSpace(errorLog.Model),
		Stream:       errorLog.Stream,
		Protocol:     opsRetryProtocol(reqType, errorLog.RequestPath),
		GeminiAction: resolveGeminiAction(reqType, errorLog),
		GroupID:      errorLog.GroupID,
		GinContext:   c,
	}
}

// resolveGeminiAction extracts the Gemini action from the error log
// metadata for Gemini protocol retries.
func resolveGeminiAction(reqType opsRetryRequestType, errorLog *OpsErrorLogDetail) string {
	if reqType != opsRetryTypeGeminiV1B {
		return ""
	}
	if errorLog.Stream {
		return "streamGenerateContent"
	}
	return "generateContent"
}

func buildRetryExecution(c *gin.Context, w *limitedResponseWriter, err error) *opsRetryExecution {
	statusCode := http.StatusOK
	if c != nil && c.Writer != nil {
		statusCode = c.Writer.Status()
	}

	upstreamReqID := extractUpstreamRequestID(c)
	preview, truncated := extractResponsePreview(w)

	exec := &opsRetryExecution{
		status:            opsRetryStatusFailed,
		httpStatusCode:    statusCode,
		upstreamRequestID: upstreamReqID,
		responsePreview:   preview,
		responseTruncated: truncated,
	}

	if err == nil && statusCode < 400 {
		exec.status = opsRetryStatusSucceeded
		return exec
	}

	if err != nil {
		exec.errorMessage = err.Error()
	} else {
		exec.errorMessage = fmt.Sprintf("upstream returned status %d", statusCode)
	}
	return exec
}

func (s *OpsService) isFailoverError(message string) bool {
	msg := strings.ToLower(strings.TrimSpace(message))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "upstream error:") && strings.Contains(msg, "failover")
}
