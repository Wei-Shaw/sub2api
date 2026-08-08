package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *ChannelMonitorService) runAnthropicGroupAccountProbeForModel(ctx context.Context, m *ChannelMonitor, model string, opts *CheckOptions, group *monitorProbeGroup) *CheckResult {
	start := time.Now()
	if m == nil || m.Provider != MonitorProviderAnthropic {
		return groupAccountProbeErrorResult(model, start, "anthropic group account probe supports anthropic provider only")
	}
	if group == nil || group.id <= 0 {
		return groupAccountProbeErrorResult(model, start, "monitor api key is not bound to a group")
	}
	if s.gateway == nil {
		return groupAccountProbeErrorResult(model, start, "gateway is not configured")
	}

	challenge := generateChallenge()
	body, err := buildRequestBody(providerAdapters[MonitorProviderAnthropic], MonitorProviderAnthropic, MonitorAPIModeChatCompletions, model, challenge.Prompt, opts)
	if err != nil {
		return groupAccountProbeErrorResult(model, start, err.Error())
	}

	failedAccountIDs := make(map[int64]struct{})
	attempts := make([]string, 0)
	sessionHash := fmt.Sprintf("channel-monitor:%d:%s:%s", group.id, model, challenge.Expected)
	var bestDegraded *CheckResult

	for {
		if len(failedAccountIDs) > 0 && !sleepMonitorWithContext(ctx, monitorModelCheckGap) {
			if bestDegraded != nil {
				return bestDegraded
			}
			return groupAccountProbeErrorResult(model, start, "check cancelled during anthropic account probe")
		}

		selection, err := s.gateway.SelectAccountWithLoadAwareness(ctx, &group.id, sessionHash, model, failedAccountIDs, "", 0)
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
		result, attempt, passed := s.runAnthropicGroupProbeAttempt(ctx, group, account, model, body, challenge, opts, attemptStart)
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		if passed {
			if acceptGroupProbeResult(result, &bestDegraded) {
				return result
			}
			if result == nil {
				return groupAccountProbeErrorResult(model, start, "anthropic group account probe passed without result")
			}
			attempts = append(attempts, accountProbeAttemptSummary(account, result.Status, result.Message))
			failedAccountIDs[account.ID] = struct{}{}
			continue
		}

		attempts = append(attempts, attempt)
		failedAccountIDs[account.ID] = struct{}{}
	}
}

func (s *ChannelMonitorService) runAnthropicGroupProbeAttempt(
	ctx context.Context,
	group *monitorProbeGroup,
	account *Account,
	model string,
	body []byte,
	challenge monitorChallenge,
	opts *CheckOptions,
	start time.Time,
) (*CheckResult, string, bool) {
	c, recorder, err := newMonitorAnthropicProbeContext(ctx, group, body)
	if err != nil {
		return nil, accountProbeAttemptSummary(account, MonitorStatusError, err.Error()), false
	}
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(append([]byte(nil), body...)), PlatformAnthropic)
	if err != nil {
		return nil, accountProbeAttemptSummary(account, MonitorStatusError, fmt.Sprintf("parse monitor request: %v", err)), false
	}

	result, err := s.gateway.Forward(ctx, c, account, parsed)
	if err != nil {
		return nil, accountProbeAttemptSummary(account, MonitorStatusError, monitorProbeForwardErrorMessage(err, recorder)), false
	}
	if result == nil {
		return nil, accountProbeAttemptSummary(account, MonitorStatusError, "gateway returned empty result"), false
	}

	status := recorder.Code
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, accountProbeAttemptSummary(account, MonitorStatusError, fmt.Sprintf("upstream HTTP %d: %s", status, truncateForErrorBody(recorder.Body.String()))), false
	}

	respText := extractMonitorResponseText(providerAdapters[MonitorProviderAnthropic], recorder.Body.Bytes())
	if bodyOverrideMode(opts) == MonitorBodyOverrideModeReplace {
		if strings.TrimSpace(respText) == "" {
			return nil, accountProbeAttemptSummary(account, MonitorStatusFailed, "replace-mode: upstream returned 2xx with empty text"), false
		}
		return anthropicGroupProbeSuccessResult(model, start, account), "", true
	}
	if !validateChallenge(respText, challenge.Expected) {
		message := fmt.Sprintf("challenge mismatch (expected %s, got %q)", challenge.Expected, respText)
		return nil, accountProbeAttemptSummary(account, MonitorStatusFailed, message), false
	}
	return anthropicGroupProbeSuccessResult(model, start, account), "", true
}

func newMonitorAnthropicProbeContext(ctx context.Context, group *monitorProbeGroup, body []byte) (*gin.Context, *httptest.ResponseRecorder, error) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, providerAnthropicPath, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "sub2api-channel-monitor")
	req.Header.Set("anthropic-version", monitorAnthropicAPIVersion)
	c.Request = req
	if group != nil && group.apiKey != nil {
		c.Set("api_key", group.apiKey)
	}
	return c, recorder, nil
}

func anthropicGroupProbeSuccessResult(model string, start time.Time, account *Account) *CheckResult {
	latencyMs := int(time.Since(start).Milliseconds())
	res := &CheckResult{
		Model:     model,
		Status:    MonitorStatusOperational,
		LatencyMs: &latencyMs,
		CheckedAt: time.Now().UTC(),
		Message: appendMonitorMessage("", fmt.Sprintf(
			"anthropic group account probe passed account %d (%s)",
			account.ID,
			strings.TrimSpace(account.Name),
		)),
	}
	if time.Since(start) >= monitorDegradedThreshold {
		res.Status = MonitorStatusDegraded
		res.Message = appendMonitorMessage(res.Message, fmt.Sprintf("slow anthropic group account probe: %dms", latencyMs))
	}
	return res
}
