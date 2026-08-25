package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

const (
	CodexQuotaOverdraftProbeExtraKey = "codex_quota_overdraft_probe"

	codexQuotaOverdraftProbePending      = "pending"
	codexQuotaOverdraftProbePassed       = "passed"
	codexQuotaOverdraftProbeFailed       = "failed"
	codexQuotaOverdraftProbeInconclusive = "inconclusive"
	codexQuotaOverdraftProbeRecovered    = "recovered"

	codexQuotaOverdraftProbeAttemptLimit   = 1
	codexQuotaOverdraftProbeAttemptTimeout = 20 * time.Second
	codexQuotaOverdraftProbePlanTimeout    = codexQuotaOverdraftProbeAttemptLimit * codexQuotaOverdraftProbeAttemptTimeout
	codexQuotaOverdraftProbeBodyLimit      = 256 << 10
	codexQuotaOverdraftPauseSource         = "codex_quota_overdraft"
	codexQuotaOverdraftPersistAttempts     = 3

	codexQuotaOverdraftFallbackModel      = "gpt-5.5"
	codexQuotaOverdraftCompatibilityModel = "gpt-5.4-mini"
)

// CodexQuotaOverdraftProbeState is persisted in accounts.extra so quota-cycle
// decisions survive restarts and are visible through the account usage API.
type CodexQuotaOverdraftProbeState struct {
	Status             string     `json:"status"`
	QuotaWindow        string     `json:"quota_window"`
	CycleKey           string     `json:"cycle_key"`
	Attempts           int        `json:"attempts"`
	Limit              int        `json:"limit"`
	Model              string     `json:"model,omitempty"`
	ReasonCode         string     `json:"reason_code,omitempty"`
	StartedAt          time.Time  `json:"started_at"`
	TestedAt           *time.Time `json:"tested_at,omitempty"`
	RetryAt            *time.Time `json:"retry_at,omitempty"`
	RetryCount         int        `json:"retry_count,omitempty"`
	RecoverAt          *time.Time `json:"recover_at,omitempty"`
	FiveHourRecoverAt  *time.Time `json:"five_hour_recover_at,omitempty"`
	SevenDayRecoverAt  *time.Time `json:"seven_day_recover_at,omitempty"`
	OverdraftStartedAt *time.Time `json:"overdraft_started_at,omitempty"`
	FiveHourStartedAt  *time.Time `json:"five_hour_overdraft_started_at,omitempty"`
	SevenDayStartedAt  *time.Time `json:"seven_day_overdraft_started_at,omitempty"`
	ObservedRateLimit  *time.Time `json:"observed_rate_limit_reset_at,omitempty"`
}

type codexQuotaOverdraftSignal struct {
	Window            string
	CycleKey          string
	RecoverAt         time.Time
	FiveHourRecoverAt *time.Time
	SevenDayRecoverAt *time.Time
}

type codexQuotaOverdraftProbeResult struct {
	Status     string
	ReasonCode string
	StatusCode int
	Model      string
	Headers    http.Header
	Body       []byte
}

type codexQuotaOverdraftProbeClaimer interface {
	ClaimCodexQuotaOverdraftProbe(context.Context, int64, *CodexQuotaOverdraftProbeState) (bool, error)
}

type codexQuotaOverdraftFailedFinalizer interface {
	FinalizeCodexQuotaOverdraftProbeFailed(context.Context, int64, *CodexQuotaOverdraftProbeState, time.Time, string) (bool, error)
}

type codexQuotaOverdraftNonFailedPersister interface {
	PersistCodexQuotaOverdraftProbeUnlessFailed(context.Context, int64, *CodexQuotaOverdraftProbeState) (bool, error)
}

type codexQuotaOverdraftPauseClearer interface {
	ClearCodexQuotaOverdraftPauseIfState(context.Context, int64, string, string, string) (bool, bool, error)
}

// CodexQuotaOverdraftCoordinator implements the bounded real-request gate used
// by cpa-account-config-manager before it disables a quota-exhausted account.
type CodexQuotaOverdraftCoordinator struct {
	accountRepo         AccountRepository
	httpUpstream        HTTPUpstream
	openAITokenProvider *OpenAITokenProvider
	tlsFPProfileService *TLSFingerprintProfileService
	cfg                 *config.Config
	tempUnschedCache    TempUnschedCache
	runtimeBlocker      AccountRuntimeBlocker
	rateLimitService    *RateLimitService
	agentIdentityWS     agentIdentityWSConnectionInvalidator
	agentIdentityTaskMu sync.Mutex
	running             sync.Map
	now                 func() time.Time
	probeAttemptForTest func(context.Context, *Account, string) codexQuotaOverdraftProbeResult
}

func NewCodexQuotaOverdraftCoordinator(
	accountRepo AccountRepository,
	httpUpstream HTTPUpstream,
	openAITokenProvider *OpenAITokenProvider,
	tlsFPProfileService *TLSFingerprintProfileService,
	cfg *config.Config,
	tempUnschedCache TempUnschedCache,
	runtimeBlocker AccountRuntimeBlocker,
	rateLimitService *RateLimitService,
) *CodexQuotaOverdraftCoordinator {
	coordinator := &CodexQuotaOverdraftCoordinator{
		accountRepo:         accountRepo,
		httpUpstream:        httpUpstream,
		openAITokenProvider: openAITokenProvider,
		tlsFPProfileService: tlsFPProfileService,
		cfg:                 cfg,
		tempUnschedCache:    tempUnschedCache,
		runtimeBlocker:      runtimeBlocker,
		rateLimitService:    rateLimitService,
		now:                 time.Now,
	}
	coordinator.agentIdentityWS, _ = runtimeBlocker.(agentIdentityWSConnectionInvalidator)
	return coordinator
}
func (c *CodexQuotaOverdraftCoordinator) enabled() bool {
	return c != nil && c.cfg != nil && c.cfg.Gateway.CodexQuotaOverdraftEnabled &&
		c.accountRepo != nil && c.httpUpstream != nil
}

// ObserveAccount starts a probe when a persisted 5h/7d snapshot first reaches
// 100%, and closes an old overdraft cycle after the quota window recovers.
func (c *CodexQuotaOverdraftCoordinator) ObserveAccount(account *Account, preferredModel string) {
	if !c.enabled() || !isCodexQuotaOverdraftAccount(account) || account.ID <= 0 {
		return
	}
	accountCopy := cloneCodexQuotaOverdraftAccount(account)
	go c.observeAccount(accountCopy, preferredModel)
}

// ObserveBusinessSuccess treats a successful request that actually carried the
// overdraft payload as stronger evidence than a synthetic probe.
func (c *CodexQuotaOverdraftCoordinator) ObserveBusinessSuccess(account *Account, preferredModel string) {
	if !c.enabled() || !isCodexQuotaOverdraftAccount(account) || account.ID <= 0 {
		return
	}
	accountCopy := cloneCodexQuotaOverdraftAccount(account)
	go c.observeBusinessSuccess(accountCopy, preferredModel)
}

func (c *CodexQuotaOverdraftCoordinator) ObserveBusinessSuccessByID(accountID int64, preferredModel string) {
	if !c.enabled() || accountID <= 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		account, err := c.accountRepo.GetByID(ctx, accountID)
		cancel()
		if err != nil || !isCodexQuotaOverdraftAccount(account) {
			return
		}
		c.observeBusinessSuccess(account, preferredModel)
	}()
}

func (c *CodexQuotaOverdraftCoordinator) observeBusinessSuccess(account *Account, preferredModel string) {
	if account == nil {
		return
	}
	now := c.currentTime()
	current, _ := codexQuotaOverdraftStateFromAccount(account)
	signal, exhausted := codexQuotaOverdraftSignalFromAccount(account, current, now)
	if !exhausted {
		return
	}
	if current != nil && current.Status == codexQuotaOverdraftProbePassed && codexQuotaOverdraftStateCoversSignal(current, signal) {
		return
	}
	if current != nil && current.Status == codexQuotaOverdraftProbeFailed && codexQuotaOverdraftStateCoversSignal(current, signal) {
		return
	}
	state := &CodexQuotaOverdraftProbeState{
		Status:            codexQuotaOverdraftProbePassed,
		QuotaWindow:       signal.Window,
		CycleKey:          signal.CycleKey,
		Attempts:          1,
		Limit:             codexQuotaOverdraftProbeAttemptLimit,
		Model:             strings.TrimSpace(preferredModel),
		ReasonCode:        "business_request_ok",
		StartedAt:         now,
		TestedAt:          codexQuotaOverdraftTimePtr(now),
		RecoverAt:         codexQuotaOverdraftTimePtr(signal.RecoverAt),
		FiveHourRecoverAt: cloneTimePtr(signal.FiveHourRecoverAt),
		SevenDayRecoverAt: cloneTimePtr(signal.SevenDayRecoverAt),
		ObservedRateLimit: cloneTimePtr(account.RateLimitResetAt),
	}
	carryCodexQuotaOverdraftWindowStarts(state, current, signal, now)
	startCodexQuotaOverdraftWindows(state, signal, now)
	if !c.persistNonFailedState(account.ID, state) {
		return
	}
	mergeAccountExtra(account, map[string]any{CodexQuotaOverdraftProbeExtraKey: state})
	c.clearQuotaPause(account.ID, state)
	slog.Info("codex_quota_overdraft_business_passed",
		"account_id", account.ID,
		"model", state.Model,
		"quota_window", signal.Window,
		"cycle_key", signal.CycleKey,
	)
}

func (c *CodexQuotaOverdraftCoordinator) observeAccount(account *Account, preferredModel string) {
	if account == nil {
		return
	}
	now := c.currentTime()
	state, _ := codexQuotaOverdraftStateFromAccount(account)
	if state != nil && clearRecoveredCodexQuotaOverdraftWindows(state, account, now) {
		c.persistState(account.ID, state)
		mergeAccountExtra(account, map[string]any{CodexQuotaOverdraftProbeExtraKey: state})
	}
	signal, exhausted := codexQuotaOverdraftSignalFromAccount(account, state, now)
	if !exhausted {
		c.recoverCycle(account, state, now)
		return
	}
	c.startProbe(account, signal, preferredModel)
}

// HandleQuota429 intercepts only definite subscription-quota 429s. Ordinary
// transient 429s continue through the existing rate-limit policy unchanged.
func (c *CodexQuotaOverdraftCoordinator) HandleQuota429(
	ctx context.Context,
	account *Account,
	headers http.Header,
	body []byte,
	preferredModel string,
) bool {
	if !c.enabled() || !isCodexQuotaOverdraftAccount(account) || account.ID <= 0 ||
		!codexQuotaOverdraftResponseIsQuotaLimited(headers, body) {
		return false
	}

	accountCopy := cloneCodexQuotaOverdraftAccount(account)
	if snapshot := ParseCodexRateLimitHeaders(headers); snapshot != nil {
		updates := buildCodexUsageExtraUpdates(snapshot, c.currentTime())
		mergeAccountExtra(accountCopy, updates)
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		_ = c.accountRepo.UpdateExtra(persistCtx, account.ID, updates)
		cancel()
	}
	state, _ := codexQuotaOverdraftStateFromAccount(accountCopy)
	signal, exhausted := codexQuotaOverdraftSignalFromAccount(accountCopy, state, c.currentTime())
	if !exhausted {
		signal = codexQuotaOverdraftFallbackSignal(headers, body, state, c.currentTime())
	}
	if signal.CycleKey == "" {
		return false
	}
	if codexQuotaOverdraftWasInjected(ctx, account.ID) {
		return c.finishBusinessQuotaFailure(accountCopy, signal, preferredModel)
	}
	c.startProbe(accountCopy, signal, preferredModel)
	return true
}

func (c *CodexQuotaOverdraftCoordinator) finishBusinessQuotaFailure(
	account *Account,
	signal codexQuotaOverdraftSignal,
	preferredModel string,
) bool {
	if account == nil || signal.CycleKey == "" {
		return false
	}
	now := c.currentTime()
	current, hasCurrent := codexQuotaOverdraftStateFromAccount(account)
	if hasCurrent && codexQuotaOverdraftStateCoversSignal(current, signal) && current.Status == codexQuotaOverdraftProbeFailed {
		return c.ensureFailedPause(account, current)
	}
	if !hasCurrent || !codexQuotaOverdraftStateCoversSignal(current, signal) {
		current = &CodexQuotaOverdraftProbeState{
			Status:            codexQuotaOverdraftProbePending,
			QuotaWindow:       signal.Window,
			CycleKey:          signal.CycleKey,
			Limit:             codexQuotaOverdraftProbeAttemptLimit,
			StartedAt:         now,
			RecoverAt:         codexQuotaOverdraftTimePtr(signal.RecoverAt),
			FiveHourRecoverAt: cloneTimePtr(signal.FiveHourRecoverAt),
			SevenDayRecoverAt: cloneTimePtr(signal.SevenDayRecoverAt),
			ObservedRateLimit: cloneTimePtr(account.RateLimitResetAt),
		}
		claimed, err := c.claimProbe(account.ID, current)
		if err != nil {
			slog.Warn("codex_quota_overdraft_business_failure_claim_failed", "account_id", account.ID, "error", err)
			return false
		}
		if !claimed {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			latest, loadErr := c.accountRepo.GetByID(ctx, account.ID)
			cancel()
			latestState, ok := codexQuotaOverdraftStateFromAccount(latest)
			if loadErr != nil || !ok || !codexQuotaOverdraftStateCoversSignal(latestState, signal) {
				return false
			}
			account = latest
			current = latestState
			if current.Status == codexQuotaOverdraftProbeFailed {
				return c.ensureFailedPause(account, current)
			}
		}
	}
	state := *current
	state.Status = codexQuotaOverdraftProbeFailed
	if state.Attempts < 1 {
		state.Attempts = 1
	}
	state.Model = strings.TrimSpace(preferredModel)
	state.ReasonCode = "business_quota_limited"
	state.TestedAt = codexQuotaOverdraftTimePtr(now)
	state.RetryAt = nil
	state.RecoverAt = codexQuotaOverdraftTimePtr(signal.RecoverAt)
	state.FiveHourRecoverAt = cloneTimePtr(signal.FiveHourRecoverAt)
	state.SevenDayRecoverAt = cloneTimePtr(signal.SevenDayRecoverAt)
	if !c.ensureFailedPause(account, &state) {
		return false
	}
	slog.Warn("codex_quota_overdraft_business_exhausted",
		"account_id", account.ID,
		"model", state.Model,
		"quota_window", signal.Window,
		"recover_at", state.RecoverAt,
	)
	return true
}

func (c *CodexQuotaOverdraftCoordinator) startProbe(account *Account, signal codexQuotaOverdraftSignal, preferredModel string) {
	if account == nil || signal.CycleKey == "" {
		return
	}
	current, hasCurrent := codexQuotaOverdraftStateFromAccount(account)
	if hasCurrent && codexQuotaOverdraftStateCoversSignal(current, signal) {
		switch current.Status {
		case codexQuotaOverdraftProbePassed:
			return
		case codexQuotaOverdraftProbeFailed:
			c.ensureFailedPause(account, current)
			return
		case codexQuotaOverdraftProbePending, codexQuotaOverdraftProbeInconclusive:
			return
		}
	}

	startedAt := c.currentTime().UTC()
	state := &CodexQuotaOverdraftProbeState{
		Status:            codexQuotaOverdraftProbePending,
		QuotaWindow:       signal.Window,
		CycleKey:          signal.CycleKey,
		Limit:             codexQuotaOverdraftProbeAttemptLimit,
		StartedAt:         startedAt,
		RecoverAt:         codexQuotaOverdraftTimePtr(signal.RecoverAt),
		FiveHourRecoverAt: cloneTimePtr(signal.FiveHourRecoverAt),
		SevenDayRecoverAt: cloneTimePtr(signal.SevenDayRecoverAt),
		ObservedRateLimit: cloneTimePtr(account.RateLimitResetAt),
	}
	carryCodexQuotaOverdraftWindowStarts(state, current, signal, startedAt)
	claimed, err := c.claimProbe(account.ID, state)
	if err != nil {
		slog.Warn("codex_quota_overdraft_probe_claim_failed", "account_id", account.ID, "error", err)
		return
	}
	if !claimed {
		return
	}
	mergeAccountExtra(account, map[string]any{CodexQuotaOverdraftProbeExtraKey: state})

	taskKey := fmt.Sprintf("%d:%s", account.ID, signal.CycleKey)
	if _, loaded := c.running.LoadOrStore(taskKey, struct{}{}); loaded {
		return
	}
	go func() {
		defer c.running.Delete(taskKey)
		c.runProbePlan(account.ID, signal, preferredModel, state)
	}()
}

func (c *CodexQuotaOverdraftCoordinator) claimProbe(accountID int64, state *CodexQuotaOverdraftProbeState) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if claimer, ok := c.accountRepo.(codexQuotaOverdraftProbeClaimer); ok {
		return claimer.ClaimCodexQuotaOverdraftProbe(ctx, accountID, state)
	}
	if err := c.accountRepo.UpdateExtra(ctx, accountID, map[string]any{CodexQuotaOverdraftProbeExtraKey: state}); err != nil {
		return false, err
	}
	return true, nil
}

func (c *CodexQuotaOverdraftCoordinator) runProbePlan(
	accountID int64,
	signal codexQuotaOverdraftSignal,
	preferredModel string,
	state *CodexQuotaOverdraftProbeState,
) {
	planCtx, cancel := context.WithTimeout(context.Background(), codexQuotaOverdraftProbePlanTimeout)
	defer cancel()

	account, err := c.accountRepo.GetByID(planCtx, accountID)
	if err != nil || !isCodexQuotaOverdraftAccount(account) {
		c.finishInconclusive(accountID, state, "upstream_unavailable")
		return
	}
	models := codexQuotaOverdraftProbeModels(preferredModel)
	lastReason := "invalid_response"
	quotaLimitedAttempts := 0
	for attempt := 0; attempt < codexQuotaOverdraftProbeAttemptLimit; attempt++ {
		if planCtx.Err() != nil {
			c.finishInconclusive(accountID, state, "request_timeout")
			return
		}
		model := models[attempt%len(models)]
		attemptCtx, attemptCancel := context.WithTimeout(planCtx, codexQuotaOverdraftProbeAttemptTimeout)
		result := c.runProbeAttempt(attemptCtx, account, model)
		attemptCancel()
		state.Attempts = attempt + 1
		state.Model = result.Model
		state.ReasonCode = result.ReasonCode
		testedAt := c.currentTime().UTC()
		state.TestedAt = &testedAt
		lastReason = result.ReasonCode
		if result.ReasonCode == "quota_limited" {
			quotaLimitedAttempts++
		}
		slog.Info("codex_quota_overdraft_probe_attempt",
			"account_id", accountID,
			"attempt", state.Attempts,
			"limit", codexQuotaOverdraftProbeAttemptLimit,
			"model", state.Model,
			"status_code", result.StatusCode,
			"result_status", result.Status,
			"reason", result.ReasonCode,
			"quota_window", signal.Window,
		)
		if result.Status == "available" {
			state.Status = codexQuotaOverdraftProbePassed
			state.ReasonCode = "model_response_ok"
			state.RetryAt = nil
			state.RetryCount = 0
			startCodexQuotaOverdraftWindows(state, signal, testedAt)
			if !c.persistNonFailedState(accountID, state) {
				return
			}
			c.clearQuotaPause(accountID, state)
			slog.Info("codex_quota_overdraft_probe_passed", "account_id", accountID, "attempts", state.Attempts, "model", state.Model, "quota_window", signal.Window)
			return
		}
		if result.Status == "inconclusive" {
			c.finishInconclusive(accountID, state, result.ReasonCode)
			return
		}
		if result.Status == "authentication_failed" {
			if c.rateLimitService != nil {
				c.rateLimitService.HandleUpstreamError(context.Background(), account, result.StatusCode, result.Headers, result.Body)
			}
			c.finishInconclusive(accountID, state, result.ReasonCode)
			return
		}
	}
	if quotaLimitedAttempts < codexQuotaOverdraftProbeAttemptLimit {
		c.finishInconclusive(accountID, state, lastReason)
		return
	}

	state.Status = codexQuotaOverdraftProbeFailed
	state.ReasonCode = lastReason
	state.RetryAt = nil
	state.RetryCount = 0
	if !c.ensureFailedPause(account, state) {
		return
	}
	slog.Warn("codex_quota_overdraft_probe_failed", "account_id", accountID, "attempts", state.Attempts, "quota_window", signal.Window, "recover_at", state.RecoverAt)
}

func (c *CodexQuotaOverdraftCoordinator) finishInconclusive(
	accountID int64,
	state *CodexQuotaOverdraftProbeState,
	reason string,
) {
	now := c.currentTime().UTC()
	state.Status = codexQuotaOverdraftProbeInconclusive
	state.ReasonCode = reason
	state.TestedAt = &now
	state.RetryAt = nil
	state.RetryCount = 0
	if !c.persistNonFailedState(accountID, state) {
		return
	}
	slog.Warn("codex_quota_overdraft_probe_inconclusive",
		"account_id", accountID,
		"attempts", state.Attempts,
		"reason", reason,
	)
}

func (c *CodexQuotaOverdraftCoordinator) runProbeAttempt(ctx context.Context, account *Account, model string) codexQuotaOverdraftProbeResult {
	if c.probeAttemptForTest != nil {
		return c.probeAttemptForTest(ctx, account, model)
	}
	result := codexQuotaOverdraftProbeResult{Model: model}
	upstreamModel := normalizeOpenAIModelForUpstream(account, account.GetMappedModel(model))
	payload := map[string]any{
		"model": upstreamModel,
		"input": []map[string]any{{
			"type": "message",
			"role": "user",
			"content": []map[string]string{{
				"type": "input_text",
				"text": "hi",
			}},
		}},
		"instructions": "Reply with OK only.",
		"stream":       true,
		"store":        false,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		result.Status, result.ReasonCode = "inconclusive", "experimental_probe_unavailable"
		return result
	}
	payloadBytes, changed, err := injectCodexQuotaOverdraft(payloadBytes)
	if err != nil || !changed {
		result.Status, result.ReasonCode = "inconclusive", "experimental_probe_unavailable"
		return result
	}

	req, err := http.NewRequestWithContext(WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileOpenAI), http.MethodPost, chatgptCodexAPIURL, bytes.NewReader(payloadBytes))
	if err != nil {
		result.Status, result.ReasonCode = "inconclusive", "upstream_unavailable"
		return result
	}
	req.Host = "chatgpt.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	// Keep the overdraft probe aligned with the official outbound identity
	// resolver so automatic Codex client-version updates apply to probes too.
	canonical := resolveCodexOutboundIdentity("")
	req.Header.Set("Originator", canonical.originator)
	req.Header.Set("Version", canonical.version)
	req.Header.Set("User-Agent", canonical.userAgent)

	if account.IsOpenAIAgentIdentity() {
		authHeaders, authErr := buildAgentIdentityAuthenticationHeaders(ctx, c.accountRepo, c.agentIdentityWS, &c.agentIdentityTaskMu, account)
		if authErr != nil {
			result.Status, result.ReasonCode = "inconclusive", "authentication_unavailable"
			return result
		}
		for key, values := range authHeaders {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	} else {
		token := account.GetOpenAIAccessToken()
		if c.openAITokenProvider != nil {
			if refreshed, tokenErr := c.openAITokenProvider.GetAccessToken(ctx, account); tokenErr == nil {
				token = refreshed
			} else if strings.TrimSpace(token) == "" {
				result.Status, result.ReasonCode = "inconclusive", "authentication_unavailable"
				return result
			}
		}
		if strings.TrimSpace(token) == "" {
			result.Status, result.ReasonCode = "inconclusive", "authentication_unavailable"
			return result
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	setOpenAIChatGPTAccountHeaders(req.Header, account)
	enforceCodexIdentityHeadersWithUA(req.Header, account.GetOpenAIUserAgent())
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	var tlsProfile = (*tlsfingerprint.Profile)(nil)
	if c.tlsFPProfileService != nil {
		tlsProfile = c.tlsFPProfileService.ResolveTLSProfile(account)
	}
	resp, err := c.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, tlsProfile)
	if err != nil {
		result.Status = "inconclusive"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.ReasonCode = "request_timeout"
		} else {
			result.ReasonCode = "upstream_unavailable"
		}
		return result
	}
	defer func() { _ = resp.Body.Close() }()
	result.StatusCode = resp.StatusCode
	result.Headers = resp.Header.Clone()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, codexQuotaOverdraftProbeBodyLimit+1))
	if readErr != nil || len(body) > codexQuotaOverdraftProbeBodyLimit {
		result.Status, result.ReasonCode = "inconclusive", "invalid_response"
		return result
	}
	result.Body = body
	result.Status, result.ReasonCode = classifyCodexQuotaOverdraftProbe(resp.StatusCode, resp.Header, body)
	return result
}

func classifyCodexQuotaOverdraftProbe(statusCode int, headers http.Header, body []byte) (string, string) {
	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		lower := bytes.ToLower(body)
		if bytes.Contains(lower, []byte(`"type":"error"`)) || bytes.Contains(lower, []byte(`"type": "error"`)) ||
			bytes.Contains(lower, []byte(`"response.failed"`)) {
			if codexQuotaOverdraftResponseIsQuotaLimited(headers, body) {
				return "retry", "quota_limited"
			}
			return "retry", "invalid_response"
		}
		if bytes.Contains(lower, []byte(`"response.completed"`)) || bytes.Contains(lower, []byte(`"response.output_item.done"`)) {
			return "available", "model_response_ok"
		}
		return "retry", "invalid_response"
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "authentication_failed", "authentication_failed"
	case http.StatusPaymentRequired:
		lower := strings.ToLower(string(body))
		if strings.Contains(lower, "deactivated_workspace") || strings.Contains(lower, "account_deactivated") {
			return "authentication_failed", "account_deactivated"
		}
		if codexQuotaOverdraftResponseIsQuotaLimited(headers, body) {
			return "retry", "quota_limited"
		}
		return "retry", "invalid_response"
	case http.StatusTooManyRequests:
		if codexQuotaOverdraftResponseIsQuotaLimited(headers, body) {
			return "retry", "quota_limited"
		}
		return "inconclusive", "transient_failure"
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return "inconclusive", "request_timeout"
	default:
		if codexQuotaOverdraftResponseIsQuotaLimited(headers, body) {
			return "retry", "quota_limited"
		}
		if statusCode >= http.StatusInternalServerError {
			return "inconclusive", "upstream_unavailable"
		}
		if statusCode == http.StatusBadRequest || statusCode == http.StatusNotFound {
			return "retry", "model_not_found"
		}
		return "inconclusive", "invalid_response"
	}
}

func (c *CodexQuotaOverdraftCoordinator) persistState(accountID int64, state *CodexQuotaOverdraftProbeState) bool {
	var lastErr error
	for attempt := 1; attempt <= codexQuotaOverdraftPersistAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		lastErr = c.accountRepo.UpdateExtra(ctx, accountID, map[string]any{CodexQuotaOverdraftProbeExtraKey: state})
		cancel()
		if lastErr == nil {
			return true
		}
		if attempt < codexQuotaOverdraftPersistAttempts {
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
		}
	}
	slog.Warn("codex_quota_overdraft_state_persist_failed",
		"account_id", accountID,
		"attempts", codexQuotaOverdraftPersistAttempts,
		"error", lastErr,
	)
	return false
}

// persistNonFailedState keeps an already-confirmed quota failure terminal for
// the current cycle, even when a concurrent success or probe finishes later.
func (c *CodexQuotaOverdraftCoordinator) persistNonFailedState(accountID int64, state *CodexQuotaOverdraftProbeState) bool {
	if state == nil || state.Status == codexQuotaOverdraftProbeFailed {
		return false
	}
	persister, ok := c.accountRepo.(codexQuotaOverdraftNonFailedPersister)
	if !ok {
		return c.persistState(accountID, state)
	}
	var (
		persisted bool
		lastErr   error
	)
	for attempt := 1; attempt <= codexQuotaOverdraftPersistAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		persisted, lastErr = persister.PersistCodexQuotaOverdraftProbeUnlessFailed(ctx, accountID, state)
		cancel()
		if lastErr == nil {
			return persisted
		}
		if attempt < codexQuotaOverdraftPersistAttempts {
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
		}
	}
	slog.Warn("codex_quota_overdraft_state_persist_failed",
		"account_id", accountID,
		"attempts", codexQuotaOverdraftPersistAttempts,
		"error", lastErr,
	)
	return false
}

func (c *CodexQuotaOverdraftCoordinator) ensureFailedPause(account *Account, state *CodexQuotaOverdraftProbeState) bool {
	if account == nil || state == nil || state.RecoverAt == nil || !state.RecoverAt.After(c.currentTime()) {
		return false
	}
	if persisted, ok := codexQuotaOverdraftStateFromAccount(account); ok &&
		persisted.Status == codexQuotaOverdraftProbeFailed && persisted.CycleKey == state.CycleKey &&
		account.TempUnschedulableUntil != nil && !account.TempUnschedulableUntil.Before(*state.RecoverAt) {
		return true
	}
	reason := BuildTempUnschedReasonPayload(codexQuotaOverdraftPauseSource, "injected overdraft request confirmed quota exhaustion")
	if finalizer, ok := c.accountRepo.(codexQuotaOverdraftFailedFinalizer); ok {
		finalized := false
		var lastErr error
		for attempt := 1; attempt <= codexQuotaOverdraftPersistAttempts; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			finalized, lastErr = finalizer.FinalizeCodexQuotaOverdraftProbeFailed(ctx, account.ID, state, *state.RecoverAt, reason)
			cancel()
			if lastErr == nil {
				break
			}
			if attempt < codexQuotaOverdraftPersistAttempts {
				time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
			}
		}
		if lastErr != nil {
			slog.Warn("codex_quota_overdraft_pause_failed",
				"account_id", account.ID,
				"attempts", codexQuotaOverdraftPersistAttempts,
				"error", lastErr,
			)
			return false
		}
		if !finalized {
			slog.Warn("codex_quota_overdraft_pause_stale", "account_id", account.ID, "cycle_key", state.CycleKey)
			return false
		}
	} else {
		if !c.persistState(account.ID, state) {
			return false
		}
		var lastErr error
		for attempt := 1; attempt <= codexQuotaOverdraftPersistAttempts; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			lastErr = c.accountRepo.SetTempUnschedulable(ctx, account.ID, *state.RecoverAt, reason)
			cancel()
			if lastErr == nil {
				break
			}
			if attempt < codexQuotaOverdraftPersistAttempts {
				time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
			}
		}
		if lastErr != nil {
			slog.Warn("codex_quota_overdraft_pause_failed",
				"account_id", account.ID,
				"attempts", codexQuotaOverdraftPersistAttempts,
				"error", lastErr,
			)
			return false
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if c.tempUnschedCache != nil {
		_ = c.tempUnschedCache.SetTempUnsched(ctx, account.ID, &TempUnschedState{
			UntilUnix:       state.RecoverAt.Unix(),
			TriggeredAtUnix: c.currentTime().Unix(),
			StatusCode:      http.StatusTooManyRequests,
			ErrorMessage:    "injected overdraft request confirmed quota exhaustion",
		})
	}
	if c.runtimeBlocker != nil {
		c.runtimeBlocker.BlockAccountScheduling(account, *state.RecoverAt, codexQuotaOverdraftPauseSource)
	}
	slog.Info("codex_quota_overdraft_pause_applied", "account_id", account.ID, "until", state.RecoverAt, "cycle_key", state.CycleKey)
	return true
}

func (c *CodexQuotaOverdraftCoordinator) clearQuotaPause(accountID int64, state *CodexQuotaOverdraftProbeState) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	account, err := c.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return
	}
	persisted, ok := codexQuotaOverdraftStateFromAccount(account)
	if !ok || state == nil || persisted.CycleKey != state.CycleKey || persisted.Status != state.Status ||
		(state.Status != codexQuotaOverdraftProbePassed && state.Status != codexQuotaOverdraftProbeRecovered) {
		return
	}
	expectedTempReason := ""
	if IsAccountSchedulingThresholdReason(account.TempUnschedulableReason) || codexQuotaOverdraftPauseReason(account.TempUnschedulableReason) {
		expectedTempReason = account.TempUnschedulableReason
	}
	if clearer, ok := c.accountRepo.(codexQuotaOverdraftPauseClearer); ok {
		clearedRateLimit, clearedTemp, clearErr := clearer.ClearCodexQuotaOverdraftPauseIfState(
			ctx,
			accountID,
			state.CycleKey,
			state.Status,
			expectedTempReason,
		)
		if clearErr != nil {
			slog.Warn("codex_quota_overdraft_stale_rate_limit_clear_failed", "account_id", accountID, "error", clearErr)
			return
		}
		if clearedRateLimit {
			slog.Info("codex_quota_overdraft_stale_rate_limit_cleared", "account_id", accountID, "probe_status", state.Status)
		}
		if clearedTemp && c.tempUnschedCache != nil {
			_ = c.tempUnschedCache.DeleteTempUnsched(ctx, accountID)
		}
		if (clearedRateLimit || clearedTemp) && c.runtimeBlocker != nil {
			c.runtimeBlocker.ClearAccountSchedulingBlock(accountID)
		}
		return
	}
	clearedSchedulingState := false
	if account.RateLimitResetAt != nil {
		staleResetAt := account.RateLimitResetAt.UTC()
		if err := c.accountRepo.ClearRateLimit(ctx, accountID); err != nil {
			slog.Warn("codex_quota_overdraft_stale_rate_limit_clear_failed", "account_id", accountID, "reset_at", staleResetAt, "error", err)
		} else {
			clearedSchedulingState = true
			slog.Info("codex_quota_overdraft_stale_rate_limit_cleared", "account_id", accountID, "reset_at", staleResetAt, "probe_status", state.Status)
		}
	}
	if expectedTempReason != "" {
		_ = c.accountRepo.ClearTempUnschedulable(ctx, accountID)
		clearedSchedulingState = true
		if c.tempUnschedCache != nil {
			_ = c.tempUnschedCache.DeleteTempUnsched(ctx, accountID)
		}
	}
	if clearedSchedulingState && c.runtimeBlocker != nil {
		c.runtimeBlocker.ClearAccountSchedulingBlock(accountID)
	}
}

func (c *CodexQuotaOverdraftCoordinator) recoverCycle(account *Account, state *CodexQuotaOverdraftProbeState, now time.Time) {
	if account == nil || state == nil || state.Status == "" || state.Status == codexQuotaOverdraftProbeRecovered {
		return
	}
	state.Status = codexQuotaOverdraftProbeRecovered
	state.ReasonCode = "quota_recovered"
	state.RetryAt = nil
	state.OverdraftStartedAt = nil
	state.FiveHourStartedAt = nil
	state.SevenDayStartedAt = nil
	testedAt := now.UTC()
	state.TestedAt = &testedAt
	c.persistState(account.ID, state)
	c.clearQuotaPause(account.ID, state)
}

func (c *CodexQuotaOverdraftCoordinator) currentTime() time.Time {
	if c != nil && c.now != nil {
		return c.now().UTC()
	}
	return time.Now().UTC()
}

func codexQuotaOverdraftProbeModels(preferred string) []string {
	models := []string{strings.TrimSpace(preferred), codexQuotaOverdraftFallbackModel, codexQuotaOverdraftCompatibilityModel}
	if models[0] == "" {
		models[0] = openai.DefaultTestModel
	}
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		key := strings.ToLower(strings.TrimSpace(model))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, model)
	}
	return out
}

func codexQuotaOverdraftResponseIsQuotaLimited(headers http.Header, body []byte) bool {
	var payload any
	parsedPayload := len(bytes.TrimSpace(body)) > 0 && json.Unmarshal(body, &payload) == nil
	if parsedPayload && codexQuotaOverdraftJSONHasQuotaEvidence(payload, 0) {
		return true
	}
	text := strings.ToLower(strings.Join(strings.Fields(string(body)), " "))
	for _, marker := range []string{
		"usage_limit_reached",
		"usage limit has been reached",
		"you have reached your usage limit",
		"quota exhausted",
		"insufficient quota",
		"weekly limit reached",
		"weekly_limit_reached",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	if parsedPayload && codexQuotaOverdraftJSONHasTransientRateLimitEvidence(payload, 0) {
		return false
	}
	if snapshot := ParseCodexRateLimitHeaders(headers); snapshot != nil {
		if normalized := snapshot.Normalize(); normalized != nil &&
			(normalized.Used5hPercent != nil && *normalized.Used5hPercent >= 100 ||
				normalized.Used7dPercent != nil && *normalized.Used7dPercent >= 100) {
			return true
		}
	}
	return false
}

func codexQuotaOverdraftJSONHasQuotaEvidence(value any, depth int) bool {
	if depth > 6 {
		return false
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, raw := range typed {
			normalizedKey := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
			switch normalizedKey {
			case "type", "code", "reason", "error_code":
				if marker, ok := raw.(string); ok && codexQuotaOverdraftQuotaCode(marker) {
					return true
				}
			case "limit_reached", "limitreached":
				if reached, ok := raw.(bool); ok && reached {
					return true
				}
			case "used_percent", "usedpercent":
				if used, ok := raw.(float64); ok && used >= 100 {
					return true
				}
			}
			if codexQuotaOverdraftJSONHasQuotaEvidence(raw, depth+1) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if codexQuotaOverdraftJSONHasQuotaEvidence(item, depth+1) {
				return true
			}
		}
	case string:
		if codexQuotaOverdraftQuotaCode(typed) {
			return true
		}
		text := strings.ToLower(strings.Join(strings.Fields(typed), " "))
		for _, marker := range []string{
			"usage limit has been reached",
			"you have reached your usage limit",
			"quota exhausted",
			"insufficient quota",
			"weekly limit reached",
		} {
			if strings.Contains(text, marker) {
				return true
			}
		}
	}
	return false
}

func codexQuotaOverdraftQuotaCode(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("-", "_", " ", "_").Replace(value)
	switch value {
	case "usage_limit_reached", "weekly_limit_reached", "monthly_limit_reached", "quota_exhausted", "insufficient_quota", "billing_hard_limit_reached":
		return true
	default:
		return false
	}
}

func codexQuotaOverdraftJSONHasTransientRateLimitEvidence(value any, depth int) bool {
	if depth > 6 {
		return false
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, raw := range typed {
			normalizedKey := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
			if normalizedKey == "type" || normalizedKey == "code" || normalizedKey == "reason" || normalizedKey == "error_code" {
				if marker, ok := raw.(string); ok {
					marker = strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(strings.TrimSpace(marker)))
					switch marker {
					case "rate_limit_exceeded", "too_many_requests", "request_rate_limited", "token_rate_limited":
						return true
					}
				}
			}
			if codexQuotaOverdraftJSONHasTransientRateLimitEvidence(raw, depth+1) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if codexQuotaOverdraftJSONHasTransientRateLimitEvidence(item, depth+1) {
				return true
			}
		}
	}
	return false
}

func codexQuotaOverdraftSignalFromAccount(account *Account, state *CodexQuotaOverdraftProbeState, now time.Time) (codexQuotaOverdraftSignal, bool) {
	if account == nil || len(account.Extra) == 0 {
		return codexQuotaOverdraftSignal{}, false
	}
	fiveUsed := parseExtraFloat64(account.Extra["codex_5h_used_percent"])
	sevenUsed := parseExtraFloat64(account.Extra["codex_7d_used_percent"])
	fiveReset := codexQuotaOverdraftResetAt(account.Extra["codex_5h_reset_at"], now)
	sevenReset := codexQuotaOverdraftResetAt(account.Extra["codex_7d_reset_at"], now)
	if state != nil {
		fiveReset = stabilizeCodexQuotaOverdraftReset(fiveReset, state.FiveHourRecoverAt, now)
		sevenReset = stabilizeCodexQuotaOverdraftReset(sevenReset, state.SevenDayRecoverAt, now)
	}
	fiveExhausted := fiveUsed >= 100 && (fiveReset == nil || fiveReset.After(now))
	sevenExhausted := sevenUsed >= 100 && (sevenReset == nil || sevenReset.After(now))
	if !fiveExhausted && !sevenExhausted {
		return codexQuotaOverdraftSignal{}, false
	}
	if fiveExhausted && fiveReset == nil {
		fallback := now.Add(5 * time.Hour)
		if state != nil && state.FiveHourRecoverAt != nil && state.FiveHourRecoverAt.After(now) {
			fallback = *state.FiveHourRecoverAt
		}
		fiveReset = &fallback
	}
	if sevenExhausted && sevenReset == nil {
		fallback := now.Add(7 * 24 * time.Hour)
		if state != nil && state.SevenDayRecoverAt != nil && state.SevenDayRecoverAt.After(now) {
			fallback = *state.SevenDayRecoverAt
		}
		sevenReset = &fallback
	}

	signal := codexQuotaOverdraftSignal{FiveHourRecoverAt: fiveReset, SevenDayRecoverAt: sevenReset}
	switch {
	case fiveExhausted && sevenExhausted:
		signal.Window = "multiple"
		signal.CycleKey = fmt.Sprintf("5h:%d|7d:%d", fiveReset.Unix(), sevenReset.Unix())
		signal.RecoverAt = laterTime(*fiveReset, *sevenReset)
	case fiveExhausted:
		signal.Window = "5h"
		signal.CycleKey = fmt.Sprintf("5h:%d", fiveReset.Unix())
		signal.RecoverAt = *fiveReset
		signal.SevenDayRecoverAt = nil
	default:
		signal.Window = "7d"
		signal.CycleKey = fmt.Sprintf("7d:%d", sevenReset.Unix())
		signal.RecoverAt = *sevenReset
		signal.FiveHourRecoverAt = nil
	}
	return signal, true
}

func clearRecoveredCodexQuotaOverdraftWindows(state *CodexQuotaOverdraftProbeState, account *Account, now time.Time) bool {
	if state == nil || account == nil {
		return false
	}
	changed := false
	fiveUsed := parseExtraFloat64(account.Extra["codex_5h_used_percent"])
	sevenUsed := parseExtraFloat64(account.Extra["codex_7d_used_percent"])
	if state.FiveHourStartedAt != nil && (fiveUsed < 100 || state.FiveHourRecoverAt == nil || !state.FiveHourRecoverAt.After(now)) {
		state.FiveHourStartedAt = nil
		state.FiveHourRecoverAt = nil
		changed = true
	}
	if state.SevenDayStartedAt != nil && (sevenUsed < 100 || state.SevenDayRecoverAt == nil || !state.SevenDayRecoverAt.After(now)) {
		state.SevenDayStartedAt = nil
		state.SevenDayRecoverAt = nil
		changed = true
	}
	state.OverdraftStartedAt = earlierCodexQuotaOverdraftStart(state.FiveHourStartedAt, state.SevenDayStartedAt)
	return changed
}

func codexQuotaOverdraftFallbackSignal(headers http.Header, body []byte, state *CodexQuotaOverdraftProbeState, now time.Time) codexQuotaOverdraftSignal {
	recoverAt := now.Add(5 * time.Hour)
	if resetAt := calculateOpenAI429ResetTime(headers); resetAt != nil && resetAt.After(now) {
		recoverAt = *resetAt
	} else if resetUnix := parseOpenAIRateLimitResetTime(body); resetUnix != nil {
		if parsed := time.Unix(*resetUnix, 0).UTC(); parsed.After(now) {
			recoverAt = parsed
		}
	} else if state != nil && state.RecoverAt != nil && state.RecoverAt.After(now) {
		recoverAt = *state.RecoverAt
	}
	return codexQuotaOverdraftSignal{
		Window:            "multiple",
		CycleKey:          fmt.Sprintf("multiple:%d", recoverAt.Unix()),
		RecoverAt:         recoverAt,
		FiveHourRecoverAt: codexQuotaOverdraftTimePtr(recoverAt),
		SevenDayRecoverAt: codexQuotaOverdraftTimePtr(recoverAt),
	}
}

func codexQuotaOverdraftStateFromAccount(account *Account) (*CodexQuotaOverdraftProbeState, bool) {
	if account == nil || account.Extra == nil {
		return nil, false
	}
	raw, ok := account.Extra[CodexQuotaOverdraftProbeExtraKey]
	if !ok || raw == nil {
		return nil, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var state CodexQuotaOverdraftProbeState
	if err := json.Unmarshal(encoded, &state); err != nil || state.CycleKey == "" {
		return nil, false
	}
	return &state, true
}

func codexQuotaOverdraftSchedulingAllowed(account *Account, now time.Time) bool {
	if !codexQuotaOverdraftInjectionEligible(account, now) {
		return false
	}
	state, _ := codexQuotaOverdraftStateFromAccount(account)
	signal, exhausted := codexQuotaOverdraftSignalFromAccount(account, state, now)
	if !exhausted || state == nil || !codexQuotaOverdraftStateCoversSignal(state, signal) {
		return true
	}
	return state.Status != codexQuotaOverdraftProbeFailed
}

func codexQuotaOverdraftSnapshotPrearmReached(updates map[string]any) bool {
	if len(updates) == 0 {
		return false
	}
	return parseExtraFloat64(updates["codex_5h_used_percent"]) >= codexQuotaOverdraftPrearmPercent ||
		parseExtraFloat64(updates["codex_7d_used_percent"]) >= codexQuotaOverdraftPrearmPercent
}

func applyCodexQuotaOverdraftUsage(
	ctx context.Context,
	repo UsageLogRepository,
	account *Account,
	usage *UsageInfo,
	now time.Time,
) {
	if repo == nil || account == nil || usage == nil {
		return
	}
	state, ok := codexQuotaOverdraftStateFromAccount(account)
	if !ok || state.Status == codexQuotaOverdraftProbeRecovered {
		return
	}
	apply := func(progress *UsageProgress, startedAt, recoverAt *time.Time) {
		if progress == nil || progress.Utilization < 100 || startedAt == nil || recoverAt == nil || !recoverAt.After(now) {
			return
		}
		stats, err := repo.GetAccountWindowStats(ctx, account.ID, *startedAt)
		if err != nil {
			return
		}
		progress.OverdraftActive = true
		progress.OverdraftStats = windowStatsFromAccountStats(stats)
		progress.OverdraftStarted = cloneTimePtr(startedAt)
		progress.OverdraftRecover = cloneTimePtr(recoverAt)
	}
	fiveStarted, sevenStarted := codexQuotaOverdraftWindowStarts(state)
	apply(usage.FiveHour, fiveStarted, state.FiveHourRecoverAt)
	apply(usage.SevenDay, sevenStarted, state.SevenDayRecoverAt)
}

func stabilizeCodexQuotaOverdraftReset(current, persisted *time.Time, now time.Time) *time.Time {
	if current == nil || persisted == nil || !persisted.After(now) {
		return current
	}
	delta := current.Sub(*persisted)
	if delta < 0 {
		delta = -delta
	}
	if delta <= 2*time.Minute {
		return cloneTimePtr(persisted)
	}
	return current
}

func codexQuotaOverdraftStateCoversSignal(state *CodexQuotaOverdraftProbeState, signal codexQuotaOverdraftSignal) bool {
	if state == nil || signal.CycleKey == "" {
		return false
	}
	if state.CycleKey == signal.CycleKey {
		return true
	}
	resetMatches := func(current, persisted *time.Time) bool {
		if current == nil || persisted == nil {
			return false
		}
		delta := current.Sub(*persisted)
		if delta < 0 {
			delta = -delta
		}
		return delta <= 2*time.Minute
	}
	switch signal.Window {
	case "5h":
		return resetMatches(signal.FiveHourRecoverAt, state.FiveHourRecoverAt)
	case "7d":
		return resetMatches(signal.SevenDayRecoverAt, state.SevenDayRecoverAt)
	case "multiple":
		return resetMatches(signal.FiveHourRecoverAt, state.FiveHourRecoverAt) &&
			resetMatches(signal.SevenDayRecoverAt, state.SevenDayRecoverAt)
	default:
		return false
	}
}

func carryCodexQuotaOverdraftWindowStarts(target, current *CodexQuotaOverdraftProbeState, signal codexQuotaOverdraftSignal, now time.Time) {
	if target == nil || current == nil {
		return
	}
	if current.FiveHourRecoverAt != nil && current.FiveHourRecoverAt.After(now) && signal.FiveHourRecoverAt != nil {
		delta := signal.FiveHourRecoverAt.Sub(*current.FiveHourRecoverAt)
		if delta >= -2*time.Minute && delta <= 2*time.Minute {
			target.FiveHourStartedAt = cloneTimePtr(current.FiveHourStartedAt)
		}
	}
	if current.SevenDayRecoverAt != nil && current.SevenDayRecoverAt.After(now) && signal.SevenDayRecoverAt != nil {
		delta := signal.SevenDayRecoverAt.Sub(*current.SevenDayRecoverAt)
		if delta >= -2*time.Minute && delta <= 2*time.Minute {
			target.SevenDayStartedAt = cloneTimePtr(current.SevenDayStartedAt)
		}
	}
	target.OverdraftStartedAt = earlierCodexQuotaOverdraftStart(target.FiveHourStartedAt, target.SevenDayStartedAt)
}

func startCodexQuotaOverdraftWindows(state *CodexQuotaOverdraftProbeState, signal codexQuotaOverdraftSignal, testedAt time.Time) {
	if state == nil {
		return
	}
	switch signal.Window {
	case "5h":
		if state.FiveHourStartedAt == nil {
			state.FiveHourStartedAt = codexQuotaOverdraftTimePtr(testedAt)
		}
	case "7d":
		if state.SevenDayStartedAt == nil {
			state.SevenDayStartedAt = codexQuotaOverdraftTimePtr(testedAt)
		}
	default:
		if state.FiveHourStartedAt == nil {
			state.FiveHourStartedAt = codexQuotaOverdraftTimePtr(testedAt)
		}
		if state.SevenDayStartedAt == nil {
			state.SevenDayStartedAt = codexQuotaOverdraftTimePtr(testedAt)
		}
	}
	state.OverdraftStartedAt = earlierCodexQuotaOverdraftStart(state.FiveHourStartedAt, state.SevenDayStartedAt)
}

func codexQuotaOverdraftWindowStarts(state *CodexQuotaOverdraftProbeState) (*time.Time, *time.Time) {
	if state == nil {
		return nil, nil
	}
	fiveStarted := cloneTimePtr(state.FiveHourStartedAt)
	sevenStarted := cloneTimePtr(state.SevenDayStartedAt)
	if fiveStarted == nil && sevenStarted == nil && state.OverdraftStartedAt != nil {
		switch state.QuotaWindow {
		case "5h":
			fiveStarted = cloneTimePtr(state.OverdraftStartedAt)
		case "7d":
			sevenStarted = cloneTimePtr(state.OverdraftStartedAt)
		default:
			fiveStarted = cloneTimePtr(state.OverdraftStartedAt)
			sevenStarted = cloneTimePtr(state.OverdraftStartedAt)
		}
	}
	return fiveStarted, sevenStarted
}

func earlierCodexQuotaOverdraftStart(left, right *time.Time) *time.Time {
	switch {
	case left == nil:
		return cloneTimePtr(right)
	case right == nil:
		return cloneTimePtr(left)
	case left.Before(*right):
		return cloneTimePtr(left)
	default:
		return cloneTimePtr(right)
	}
}

func codexQuotaOverdraftPauseReason(reason string) bool {
	payload, ok := parseTempUnschedReasonPayload(reason)
	return ok && payload.Source == codexQuotaOverdraftPauseSource
}

func codexQuotaOverdraftResetAt(raw any, now time.Time) *time.Time {
	if raw == nil {
		return nil
	}
	parsed, err := parseTime(strings.TrimSpace(fmt.Sprint(raw)))
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func cloneCodexQuotaOverdraftAccount(account *Account) *Account {
	if account == nil {
		return nil
	}
	clone := *account
	clone.Extra = shallowCopyMap(account.Extra)
	clone.Credentials = shallowCopyMap(account.Credentials)
	return &clone
}

func laterTime(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}

func codexQuotaOverdraftTimePtr(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}
