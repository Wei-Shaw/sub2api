package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type openAIGroupProbeGateway interface {
	SelectAccountWithSchedulerForCapability(
		ctx context.Context,
		groupID *int64,
		previousResponseID string,
		sessionHash string,
		requestedModel string,
		excludedIDs map[int64]struct{},
		requiredTransport OpenAIUpstreamTransport,
		requiredCapability OpenAIEndpointCapability,
		requireCompact bool,
		previousResponseCanMove bool,
		useUpstreamTokenCost bool,
		clientHints ...string,
	) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error)
	ForwardAsChatCompletions(ctx context.Context, c *gin.Context, account *Account, body []byte, promptCacheKey string, defaultMappedModel string) (*OpenAIForwardResult, error)
	ResolveChannelMappingAndRestrict(ctx context.Context, groupID *int64, model string) (ChannelMappingResult, bool)
	ReplaceModelInBody(body []byte, newModel string) []byte
	GenerateSessionHash(c *gin.Context, body []byte) string
	ExtractSessionID(c *gin.Context, body []byte) string
	ReportOpenAIAccountScheduleResult(accountID int64, model string, success bool, firstTokenMs *int)
	RecordOpenAIAccountSwitch()
}

type anthropicGroupProbeGateway interface {
	SelectAccountWithLoadAwareness(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, metadataUserID string, sub2apiUserID int64) (*AccountSelectionResult, error)
	Forward(ctx context.Context, c *gin.Context, account *Account, parsed *ParsedRequest) (*ForwardResult, error)
}

type monitorProbeGroup struct {
	id     int64
	apiKey *APIKey
}

func (s *ChannelMonitorService) resolveMonitorProbeGroup(ctx context.Context, m *ChannelMonitor) (*monitorProbeGroup, error) {
	if m == nil || (m.Provider != MonitorProviderOpenAI && m.Provider != MonitorProviderAnthropic) {
		return nil, nil
	}
	if s.apiKeyRepo == nil {
		return nil, nil
	}
	if m.Provider == MonitorProviderOpenAI && s.openAIGateway == nil {
		return nil, nil
	}
	if m.Provider == MonitorProviderAnthropic && s.gateway == nil {
		return nil, nil
	}

	apiKey, err := s.apiKeyRepo.GetByKey(ctx, normalizeMonitorProbeAPIKey(m.APIKey))
	if errors.Is(err, ErrAPIKeyNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load monitor api key: %w", err)
	}
	if apiKey == nil || apiKey.GroupID == nil || *apiKey.GroupID <= 0 {
		return nil, nil
	}
	groupID := *apiKey.GroupID
	return &monitorProbeGroup{id: groupID, apiKey: apiKey}, nil
}

func normalizeMonitorProbeAPIKey(key string) string {
	key = strings.TrimSpace(key)
	if parts := strings.SplitN(key, " ", 2); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return key
}

func (s *ChannelMonitorService) runGroupAccountProbeForModel(ctx context.Context, m *ChannelMonitor, model string, opts *CheckOptions, group *monitorProbeGroup) *CheckResult {
	start := time.Now()
	if m == nil || m.Provider != MonitorProviderOpenAI {
		return groupAccountProbeErrorResult(model, start, "group account probe supports openai provider only")
	}
	if group == nil || group.id <= 0 {
		return groupAccountProbeErrorResult(model, start, "monitor api key is not bound to a group")
	}
	if s.openAIGateway == nil {
		return groupAccountProbeErrorResult(model, start, "openai gateway is not configured")
	}
	groupID := group.id
	apiKey := group.apiKey
	if apiKey == nil {
		apiKey = &APIKey{Key: normalizeMonitorProbeAPIKey(m.APIKey), GroupID: &groupID}
	}

	challenge := generateChallenge()
	body, apiMode, err := buildOpenAIGroupProbeBody(model, challenge, opts)
	if err != nil {
		return groupAccountProbeErrorResult(model, start, err.Error())
	}

	hashCtx, _ := newMonitorOpenAIProbeContext(ctx, apiKey, body)
	sessionHash := s.openAIGateway.GenerateSessionHash(hashCtx, body)
	promptCacheKey := s.openAIGateway.ExtractSessionID(hashCtx, body)
	failedAccountIDs := make(map[int64]struct{})
	attempts := make([]string, 0)
	var bestDegraded *CheckResult

	for {
		if len(failedAccountIDs) > 0 && !sleepMonitorWithContext(ctx, monitorModelCheckGap) {
			if bestDegraded != nil {
				return bestDegraded
			}
			return groupAccountProbeErrorResult(model, start, "check cancelled during account probe")
		}

		selection, _, err := s.openAIGateway.SelectAccountWithSchedulerForCapability(
			ctx,
			&groupID,
			"",
			sessionHash,
			model,
			failedAccountIDs,
			OpenAIUpstreamTransportAny,
			OpenAIEndpointCapabilityChatCompletions,
			false,
			false,
			false,
		)
		if err != nil {
			if bestDegraded != nil {
				return bestDegraded
			}
			return groupAccountProbeExhaustedResult(model, start, attempts, fmt.Sprintf("select group account: %v", err))
		}
		if selection == nil || selection.Account == nil {
			if bestDegraded != nil {
				return bestDegraded
			}
			return groupAccountProbeExhaustedResult(model, start, attempts, "select group account: no available accounts")
		}

		account := selection.Account
		if _, seen := failedAccountIDs[account.ID]; seen {
			if bestDegraded != nil {
				return bestDegraded
			}
			return groupAccountProbeExhaustedResult(model, start, attempts, fmt.Sprintf("scheduler returned excluded account %d", account.ID))
		}
		if !selection.Acquired {
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			attempts = append(attempts, accountProbeAttemptSummary(account, MonitorStatusFailed, "account concurrency slot unavailable"))
			failedAccountIDs[account.ID] = struct{}{}
			continue
		}

		attemptStart := time.Now()
		result, attempt, passed, reportFailure := s.runOpenAIGroupProbeAttempt(ctx, m, apiKey, account, &groupID, model, body, apiMode, challenge, promptCacheKey, opts, attemptStart)
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		if passed {
			if acceptGroupProbeResult(result, &bestDegraded) {
				return result
			}
			if result == nil {
				return groupAccountProbeErrorResult(model, start, "group account probe passed without result")
			}
			attempts = append(attempts, accountProbeAttemptSummary(account, result.Status, result.Message))
			failedAccountIDs[account.ID] = struct{}{}
			continue
		}

		attempts = append(attempts, attempt)
		failedAccountIDs[account.ID] = struct{}{}
		if reportFailure {
			s.openAIGateway.ReportOpenAIAccountScheduleResult(account.ID, model, false, nil)
			s.openAIGateway.RecordOpenAIAccountSwitch()
		}
	}
}

func acceptGroupProbeResult(result *CheckResult, bestDegraded **CheckResult) bool {
	if result == nil {
		return false
	}
	if result.Status == MonitorStatusOperational {
		return true
	}
	if result.Status != MonitorStatusDegraded {
		return true
	}
	if bestDegraded == nil {
		return false
	}
	if *bestDegraded == nil || monitorResultLatencyLess(result, *bestDegraded) {
		*bestDegraded = result
	}
	return false
}

func monitorResultLatencyLess(a, b *CheckResult) bool {
	if a == nil || b == nil {
		return false
	}
	if a.LatencyMs == nil {
		return false
	}
	if b.LatencyMs == nil {
		return true
	}
	return *a.LatencyMs < *b.LatencyMs
}

func buildOpenAIGroupProbeBody(model string, challenge monitorChallenge, opts *CheckOptions) ([]byte, string, error) {
	requestedAPIMode := MonitorAPIModeChatCompletions
	if err := validateAPIMode(MonitorProviderOpenAI, requestedAPIMode); err != nil {
		return nil, "", err
	}
	adapter, apiMode, ok := providerAdapterFor(MonitorProviderOpenAI, requestedAPIMode)
	if !ok {
		return nil, "", fmt.Errorf("unsupported provider %q", MonitorProviderOpenAI)
	}
	body, err := buildRequestBody(adapter, MonitorProviderOpenAI, apiMode, model, challenge.Prompt, opts)
	if err != nil {
		return nil, "", err
	}
	return body, apiMode, nil
}

func (s *ChannelMonitorService) runOpenAIGroupProbeAttempt(
	ctx context.Context,
	m *ChannelMonitor,
	apiKey *APIKey,
	account *Account,
	groupID *int64,
	model string,
	body []byte,
	apiMode string,
	challenge monitorChallenge,
	promptCacheKey string,
	opts *CheckOptions,
	start time.Time,
) (*CheckResult, string, bool, bool) {
	forwardBody := body
	if mapping, _ := s.openAIGateway.ResolveChannelMappingAndRestrict(ctx, groupID, model); mapping.Mapped {
		forwardBody = s.openAIGateway.ReplaceModelInBody(body, mapping.MappedModel)
	}

	attempts := monitorTransientMaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		result, attemptSummary, passed, reportFailure, retrySameAccount := s.runOpenAIGroupProbeForwardOnce(
			ctx,
			m,
			apiKey,
			account,
			model,
			forwardBody,
			apiMode,
			challenge,
			promptCacheKey,
			opts,
			start,
		)
		if passed || !retrySameAccount || attempt == attempts {
			return result, attemptSummary, passed, reportFailure
		}
		if !sleepMonitorWithContext(ctx, monitorTransientRetryDelay) {
			return nil, accountProbeAttemptSummary(account, MonitorStatusError, "check cancelled during transient retry"), false, false
		}
	}

	return nil, accountProbeAttemptSummary(account, MonitorStatusError, "group account probe exhausted transient retries"), false, false
}

func (s *ChannelMonitorService) runOpenAIGroupProbeForwardOnce(
	ctx context.Context,
	m *ChannelMonitor,
	apiKey *APIKey,
	account *Account,
	model string,
	forwardBody []byte,
	apiMode string,
	challenge monitorChallenge,
	promptCacheKey string,
	opts *CheckOptions,
	start time.Time,
) (*CheckResult, string, bool, bool, bool) {
	c, recorder := newMonitorOpenAIProbeContext(ctx, apiKey, forwardBody)
	result, err := s.openAIGateway.ForwardAsChatCompletions(ctx, c, account, forwardBody, promptCacheKey, "")
	if err != nil {
		attemptSummary := accountProbeAttemptSummary(account, MonitorStatusError, monitorProbeForwardErrorMessage(err, recorder))
		if shouldRetryOpenAIGroupProbeForwardFailure(account, err, recorder) {
			return nil, attemptSummary, false, false, true
		}
		return nil, attemptSummary, false, true, false
	}
	if recorder.Code < http.StatusOK || recorder.Code >= http.StatusMultipleChoices {
		message := fmt.Sprintf("upstream HTTP %d: %s", recorder.Code, truncateForErrorBody(recorder.Body.String()))
		attemptSummary := accountProbeAttemptSummary(account, MonitorStatusError, message)
		if shouldRetryOpenAIGroupProbeHTTPStatus(account, recorder.Code) {
			return nil, attemptSummary, false, false, true
		}
		return nil, attemptSummary, false, true, false
	}

	respText := extractOpenAIChatText(recorder.Body.Bytes())
	if bodyOverrideMode(opts) == MonitorBodyOverrideModeReplace {
		if strings.TrimSpace(respText) == "" {
			return nil, accountProbeAttemptSummary(account, MonitorStatusFailed, "replace-mode: upstream returned 2xx with empty text"), false, false, false
		}
		res, attemptSummary, passed, reportFailure := s.groupAccountProbeSuccessResult(model, start, account, result)
		return res, attemptSummary, passed, reportFailure, false
	}
	if apiMode == MonitorAPIModeResponses && strings.TrimSpace(respText) == "" {
		respText = extractOpenAIResponsesText(recorder.Body.Bytes())
	}
	if !validateChallenge(respText, challenge.Expected) {
		message := fmt.Sprintf("challenge mismatch (expected %s, got %q)", challenge.Expected, respText)
		return nil, accountProbeAttemptSummary(account, MonitorStatusFailed, message), false, false, false
	}
	res, attemptSummary, passed, reportFailure := s.groupAccountProbeSuccessResult(model, start, account, result)
	return res, attemptSummary, passed, reportFailure, false
}

func newMonitorOpenAIProbeContext(ctx context.Context, apiKey *APIKey, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, providerOpenAIPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "sub2api-channel-monitor")
	c.Request = req
	if apiKey != nil {
		c.Set("api_key", apiKey)
	}
	return c, recorder
}

func (s *ChannelMonitorService) groupAccountProbeSuccessResult(model string, start time.Time, account *Account, forwardResult *OpenAIForwardResult) (*CheckResult, string, bool, bool) {
	latencyMs := int(time.Since(start).Milliseconds())
	res := &CheckResult{
		Model:     model,
		Status:    MonitorStatusOperational,
		LatencyMs: &latencyMs,
		CheckedAt: time.Now().UTC(),
		Message: appendMonitorMessage("", fmt.Sprintf(
			"group account probe passed account %d (%s)",
			account.ID,
			strings.TrimSpace(account.Name),
		)),
	}
	if time.Since(start) >= monitorDegradedThreshold {
		res.Status = MonitorStatusDegraded
		res.Message = appendMonitorMessage(res.Message, fmt.Sprintf("slow group account probe: %dms", latencyMs))
	}
	if forwardResult != nil {
		s.openAIGateway.ReportOpenAIAccountScheduleResult(account.ID, model, true, forwardResult.FirstTokenMs)
	} else {
		s.openAIGateway.ReportOpenAIAccountScheduleResult(account.ID, model, true, nil)
	}
	return res, "", true, false
}

func groupAccountProbeExhaustedResult(model string, start time.Time, attempts []string, terminal string) *CheckResult {
	if len(attempts) == 0 {
		return groupAccountProbeErrorResult(model, start, terminal)
	}
	status := MonitorStatusFailed
	for _, attempt := range attempts {
		if strings.Contains(attempt, MonitorStatusError+":") {
			status = MonitorStatusError
			break
		}
	}
	latencyMs := int(time.Since(start).Milliseconds())
	message := fmt.Sprintf("group account probe all accounts failed: %s", strings.Join(attempts, "; "))
	if strings.TrimSpace(terminal) != "" {
		message += "; " + terminal
	}
	return &CheckResult{
		Model:     model,
		Status:    status,
		LatencyMs: &latencyMs,
		Message:   truncateMessage(sanitizeErrorMessage(message)),
		CheckedAt: time.Now().UTC(),
	}
}

func groupAccountProbeErrorResult(model string, start time.Time, message string) *CheckResult {
	latencyMs := int(time.Since(start).Milliseconds())
	return &CheckResult{
		Model:     model,
		Status:    MonitorStatusError,
		LatencyMs: &latencyMs,
		Message:   truncateMessage(sanitizeErrorMessage(message)),
		CheckedAt: time.Now().UTC(),
	}
}

func accountProbeAttemptSummary(acc *Account, status, message string) string {
	accountID := int64(0)
	name := "unnamed"
	if acc != nil {
		accountID = acc.ID
		if trimmed := strings.TrimSpace(acc.Name); trimmed != "" {
			name = trimmed
		}
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "no detail"
	}
	return fmt.Sprintf("#%d %s %s: %s", accountID, name, status, message)
}

func appendMonitorMessage(existing, extra string) string {
	existing = strings.TrimSpace(existing)
	extra = strings.TrimSpace(extra)
	if existing == "" {
		return truncateMessage(sanitizeErrorMessage(extra))
	}
	if extra == "" {
		return truncateMessage(sanitizeErrorMessage(existing))
	}
	return truncateMessage(sanitizeErrorMessage(existing + "; " + extra))
}

func monitorProbeForwardErrorMessage(err error, recorder *httptest.ResponseRecorder) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if recorder != nil {
		body := strings.TrimSpace(recorder.Body.String())
		if body != "" {
			message += ": " + truncateForErrorBody(body)
		}
	}
	return message
}

func shouldRetryOpenAIGroupProbeForwardFailure(account *Account, err error, recorder *httptest.ResponseRecorder) bool {
	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		if failoverErr.RetryableOnSameAccount || shouldRetryOpenAIGroupProbeHTTPStatus(account, failoverErr.StatusCode) {
			return true
		}
	}
	if recorder != nil && shouldRetryOpenAIGroupProbeHTTPStatus(account, recorder.Code) {
		return true
	}
	lowerMessage := strings.ToLower(monitorProbeForwardErrorMessage(err, recorder))
	switch {
	case strings.Contains(lowerMessage, "unexpected eof"),
		strings.HasSuffix(lowerMessage, " eof"),
		strings.Contains(lowerMessage, " eof:"),
		strings.Contains(lowerMessage, "connection reset"),
		strings.Contains(lowerMessage, "gateway timeout"),
		strings.Contains(lowerMessage, "service unavailable"),
		strings.Contains(lowerMessage, "bad gateway"),
		strings.Contains(lowerMessage, "timeout"):
		return true
	}
	return strings.Contains(lowerMessage, "upstream request failed") && !classifyOpenAITransportError(err).Persistent
}

func shouldRetryOpenAIGroupProbeHTTPStatus(account *Account, statusCode int) bool {
	if isMonitorTransientHTTPStatus(statusCode) {
		return true
	}
	if account == nil || !account.IsPoolMode() {
		return false
	}
	return account.IsPoolModeRetryableStatus(statusCode)
}

func (m *ChannelMonitor) APIKeyObject() *APIKey {
	if m == nil {
		return nil
	}
	return &APIKey{ID: 0, Key: m.APIKey}
}
