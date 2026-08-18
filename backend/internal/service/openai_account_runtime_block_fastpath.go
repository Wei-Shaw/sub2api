package service

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	openAIAccountStateUpdateTimeout       = 5 * time.Second
	openAIOAuth429RetryWindow             = 2 * time.Minute
	openAIOAuth429RetryDelay              = 0
	openAIStopSchedulingBridgeCooldown    = 2 * time.Minute
	openAIOAuth429MaxAccountAttempts      = 3
	openAIOAuth429StormWindow             = 10 * time.Second
	openAIOAuth429StormThreshold          = 20
	openAIOAuth429StormMaxAccountSwitches = 1
)

func (s *OpenAIGatewayService) rateLimit429StrategySettings() RateLimit429CooldownSettings {
	defaults := DefaultRateLimit429CooldownSettings()
	if s == nil || s.settingService == nil {
		return *defaults
REDACTED
	s.openai429StrategyMu.Lock()
	defer s.openai429StrategyMu.Unlock()
	if time.Since(s.openai429StrategyCachedAt) < 5*time.Second {
		return s.openai429StrategyCached
REDACTED
	settings := *defaults
	if loaded, err := s.settingService.GetRateLimit429CooldownSettings(context.Background()); err == nil && loaded != nil {
		settings = *loaded
REDACTED
	s.openai429StrategyCached = settings
	s.openai429StrategyCachedAt = time.Now()
	return settings
REDACTED

// OpenAIOAuth429FailoverState tracks the request-local follow-up budget after
// the first Grok OAuth 429. Once that 429 occurs, exactly one different account
// may be attempted; any failure from that follow-up account ends failover.
type OpenAIOAuth429FailoverState struct {
	grokOAuth429FollowupPending bool
REDACTED

func openAIAccountStateContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
REDACTED
	return context.WithTimeout(base, openAIAccountStateUpdateTimeout)
REDACTED

func isOpenAIOAuthAccount(account *Account) bool {
	return account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeOAuth
REDACTED

func isGrokOAuthAccount(account *Account) bool {
	return account != nil && account.Platform == PlatformGrok && account.Type == AccountTypeOAuth
REDACTED

func isOpenAIAccount(account *Account) bool {
	return account != nil && (account.Platform == PlatformOpenAI || account.Platform == PlatformGrok)
REDACTED

// handleOpenAIAccountUpstreamError expects canonicalModel to be the model used
// for scheduling after applying account mapping exactly once.
func (s *OpenAIGatewayService) handleOpenAIAccountUpstreamError(ctx context.Context, account *Account, statusCode int, headers http.Header, responseBody []byte, canonicalModel ...string) bool {
	if account != nil && account.Platform == PlatformGrok && isGrokContentPolicyRejection(statusCode, responseBody) {
		return false
REDACTED
	// Any non-2xx upstream HTTP response means the model request was actually sent.
	if s != nil {
		scheduleOllamaCloudUsageActivity(s.deferredService, account)
REDACTED
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()

	if account != nil && account.Platform == PlatformOpenAI && isOpenAIContextWindowError("", responseBody) {
		return false
REDACTED

	if isOpenAIImageRateLimitError(statusCode, responseBody) {
		if s != nil && s.rateLimitService != nil {
			_ = s.rateLimitService.HandleOpenAIImageRateLimit(stateCtx, account, statusCode, headers, responseBody)
	REDACTED
		return false
REDACTED

	if s == nil || account == nil {
		return false
REDACTED
	stateCtx = withTempUnschedulableModel(stateCtx, canonicalModel)
	if s.rateLimitService != nil && len(canonicalModel) > 0 && s.rateLimitService.HandleUpstreamModelNotFound(stateCtx, account, canonicalModel[0], statusCode, responseBody) {
		return true
REDACTED
	// Isolate a custom temporary-unschedulable match to the known upstream
	// model before entering the generic account error path. This keeps the
	// account available to other models and avoids the account runtime blocker.
	if s.rateLimitService != nil && statusCode != http.StatusUnauthorized && len(canonicalModel) > 0 && strings.TrimSpace(canonicalModel[0]) != "" &&
		s.rateLimitService.HandleTempUnschedulable(stateCtx, account, statusCode, responseBody, canonicalModel[0]) {
		return true
REDACTED
	if statusCode == http.StatusTooManyRequests {
		s.markOpenAIOAuth429RateLimited(stateCtx, account, headers, responseBody)
REDACTED
	if s.rateLimitService == nil {
		return false
REDACTED
	shouldDisable := s.rateLimitService.HandleUpstreamError(stateCtx, account, statusCode, headers, responseBody)
	modelTempMatched := statusCode != http.StatusUnauthorized && tempUnschedulableModel(stateCtx, nil) != "" &&
		len(matchTempUnschedulableRules(account, statusCode, responseBody)) > 0
	if shouldDisable && !modelTempMatched {
		s.BlockAccountScheduling(account, time.Time{REDACTED, "upstream_disable")
REDACTED
	// Pool-mode retryable upstream errors are already bounded by the request-local
	// same-account retry budget. Recording the generic account+model transient
	// cooldown here would block the next approved retry before that budget is used.
	poolModeRetryable := account.IsPoolMode() && account.IsPoolModeRetryableStatus(statusCode)
	if !shouldDisable && account.Platform == PlatformOpenAI && account.Type == AccountTypeAPIKey &&
		shouldCooldownOpenAITransientUpstreamError(statusCode, responseBody) && !poolModeRetryable {
		model := ""
		if len(canonicalModel) > 0 {
			model = canonicalModel[0]
	REDACTED
		decision := s.recordOpenAIAccountModelTransientFailure(account, model, time.Now())
		if decision.FailureStreak > 0 {
			slog.Warn("openai_model_transient_state",
				"account_id", account.ID,
				"model", openAIAccountModelTransientModel(model),
				"failure_streak", decision.FailureStreak,
				"cooldown_ms", decision.Cooldown.Milliseconds(),
				"block_scope", "account_model",
			)
	REDACTED
REDACTED
	return shouldDisable
REDACTED

func shouldCooldownOpenAITransientUpstreamError(statusCode int, responseBody []byte) bool {
	switch statusCode {
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, 520, 521, 522, 523, 524:
		return true
	case http.StatusBadRequest:
		return isOpenAITransientProcessingError(statusCode, "", responseBody)
	default:
		return false
REDACTED
REDACTED

func (s *OpenAIGatewayService) markOpenAIOAuth429RateLimited(ctx context.Context, account *Account, headers http.Header, responseBody []byte) {
	if s == nil || !isOpenAIOAuthAccount(account) {
		return
REDACTED
	// Spark 影子：不按 /responses 429 的 global x-codex-* 信号做内存运行时熔断(同 handle429,外审第8轮 P1)。
	// 同时避免把 spark 的 429 计入全局 429 storm 计数(recordOpenAIOAuth429),否则会误伤母账号 failover 决策。
	if account.IsShadow() {
		return
REDACTED
	s.recordOpenAIOAuth429()
	if s.ShouldRetryOpenAIOAuth429(account, headers, responseBody) {
		return
REDACTED

	cooldownUntil := time.Time{REDACTED
	hasCooldown := false
	if s.rateLimitService != nil {
		if resetAt := s.rateLimitService.calculateOpenAI429ResetTime(headers); resetAt != nil && resetAt.After(time.Now()) {
			cooldownUntil = *resetAt
			hasCooldown = true
	REDACTED else if resetUnix := parseOpenAIRateLimitResetTime(responseBody); resetUnix != nil {
			if resetAt := time.Unix(*resetUnix, 0); resetAt.After(time.Now()) {
				cooldownUntil = resetAt
				hasCooldown = true
		REDACTED
	REDACTED else if cooldown, ok := s.rateLimitService.get429FallbackCooldown(ctx, account); ok && cooldown > 0 {
			cooldownUntil = time.Now().Add(cooldown)
			hasCooldown = true
	REDACTED
REDACTED
	if !hasCooldown {
		// The request-local retry window has expired without an upstream reset
		// signal. Keep the account out of new selections while this request
		// switches to another candidate, rather than immediately selecting it
		// again on a concurrent request.
		cooldownUntil = time.Now().Add(openAIStopSchedulingBridgeCooldown)
REDACTED
	s.BlockAccountScheduling(account, cooldownUntil, "429")
	s.openaiOAuth429RetryStartedAt.Delete(account.ID)
REDACTED

// shouldRetryOpenAIOAuth429OnSameAccount keeps an OAuth account pinned while
// a transient 429 is still inside its retry window. API-key accounts keep the
// existing pool-mode behavior.
func (s *OpenAIGatewayService) shouldRetryOpenAIOAuth429OnSameAccount(account *Account, statusCode int, shouldDisable bool) bool {
	if shouldDisable || account == nil {
		return false
REDACTED
	if statusCode == http.StatusTooManyRequests && isOpenAIOAuthAccount(account) && !account.IsShadow() {
		if s.settingService != nil && s.rateLimit429StrategySettings().Strategy != "same_account_retry" {
			return false
	REDACTED
		// A prior retry window may already have expired and parked this account.
		// Do not create a fresh window while that runtime block is active.
		if s.isOpenAIAccountRuntimeBlocked(account) {
			return false
	REDACTED
		return s.openAIOAuth429RetryWindowActive(account)
REDACTED
	return account.IsPoolMode() && account.IsPoolModeRetryableStatus(statusCode)
REDACTED

// ShouldRetryOpenAIOAuth429 is used before persisting a scheduler block. An
// upstream-provided reset takes precedence; only temporary 429s without one
// stay on the same OAuth account during the retry window.
func (s *OpenAIGatewayService) ShouldRetryOpenAIOAuth429(account *Account, headers http.Header, responseBody []byte) bool {
	if s == nil || !isOpenAIOAuthAccount(account) || account.IsShadow() {
		return false
REDACTED
	if s.isOpenAIAccountRuntimeBlocked(account) {
		return false
REDACTED
	if s.settingService != nil && s.rateLimit429StrategySettings().Strategy != "same_account_retry" {
		return false
REDACTED
	if s.rateLimitService != nil && s.rateLimitService.calculateOpenAI429ResetTime(headers) != nil {
		return false
REDACTED
	if parseOpenAIRateLimitResetTime(responseBody) != nil {
		return false
REDACTED
	return s.openAIOAuth429RetryWindowActive(account)
REDACTED

func (s *OpenAIGatewayService) openAIOAuth429RetryWindowActive(account *Account) bool {
	if s == nil || !isOpenAIOAuthAccount(account) || account.IsShadow() {
		return false
REDACTED
	now := time.Now()
	value, _ := s.openaiOAuth429RetryStartedAt.LoadOrStore(account.ID, now)
	startedAt, ok := value.(time.Time)
	if !ok {
		s.openaiOAuth429RetryStartedAt.Store(account.ID, now)
		startedAt = now
REDACTED
	window := openAIOAuth429RetryWindow
	if s.settingService != nil {
		window = time.Duration(s.rateLimit429StrategySettings().RetryMaxDurationSeconds) * time.Second
REDACTED
	return now.Sub(startedAt) < window
REDACTED

func openAIOAuth429SameAccountRetryDelay(statusCode int, account *Account) time.Duration {
	if statusCode == http.StatusTooManyRequests && isOpenAIOAuthAccount(account) && !account.IsShadow() {
		return openAIOAuth429RetryDelay
REDACTED
	return 0
REDACTED

func (s *OpenAIGatewayService) openAIOAuth429SameAccountRetryDelay(statusCode int, account *Account) time.Duration {
	if statusCode == http.StatusTooManyRequests && isOpenAIOAuthAccount(account) && !account.IsShadow() && s != nil && s.settingService != nil {
		return time.Duration(s.rateLimit429StrategySettings().RetryIntervalMs) * time.Millisecond
REDACTED
	return openAIOAuth429SameAccountRetryDelay(statusCode, account)
REDACTED

// openAIOAuth429RetryDeadline returns the request-local retry window end that
// was established when the account first saw a temporary OAuth 429.
func (s *OpenAIGatewayService) openAIOAuth429RetryDeadline(account *Account) time.Time {
	if s == nil || !isOpenAIOAuthAccount(account) || account.IsShadow() {
		return time.Time{REDACTED
REDACTED
	value, ok := s.openaiOAuth429RetryStartedAt.Load(account.ID)
	if !ok {
		return time.Time{REDACTED
REDACTED
	startedAt, ok := value.(time.Time)
	if !ok {
		return time.Time{REDACTED
REDACTED
	window := openAIOAuth429RetryWindow
	if s.settingService != nil {
		window = time.Duration(s.rateLimit429StrategySettings().RetryMaxDurationSeconds) * time.Second
REDACTED
	return startedAt.Add(window)
REDACTED

// SameAccountRetryLimit returns the request-local retry budget. OAuth 429s
// deliberately use a time-derived budget rather than an account pool setting.
func SameAccountRetryLimit(account *Account, failoverErr *UpstreamFailoverError) int {
	if failoverErr != nil && failoverErr.StatusCode == http.StatusTooManyRequests &&
		isOpenAIOAuthAccount(account) && !account.IsShadow() {
		if failoverErr.SameAccountRetryMax > 0 {
			return failoverErr.SameAccountRetryMax
	REDACTED
		return 24
REDACTED
	if account == nil {
		return 0
REDACTED
	return account.GetPoolModeRetryCount()
REDACTED

func (s *OpenAIGatewayService) openAIOAuth429SameAccountRetryMax() int {
	if s == nil || s.settingService == nil {
		return 24
REDACTED
	settings := s.rateLimit429StrategySettings()
	interval := time.Duration(settings.RetryIntervalMs) * time.Millisecond
	window := time.Duration(settings.RetryMaxDurationSeconds) * time.Second
	max := int(window / interval)
	if max < 1 {
		max = 1
REDACTED
	if max > 240 {
		max = 240
REDACTED
	return max
REDACTED

func (s *OpenAIGatewayService) BlockAccountScheduling(account *Account, until time.Time, reason string) {
	if s == nil || !isOpenAIAccount(account) {
		return
REDACTED
	mu := s.openAIAccountRuntimeBlockLock(account.ID)
	mu.Lock()
	defer mu.Unlock()
	_, _ = s.blockAccountSchedulingLocked(account, until, reason)
REDACTED

func (s *OpenAIGatewayService) openAIAccountRuntimeBlockLock(accountID int64) *sync.Mutex {
	actual, _ := s.openaiAccountRuntimeBlockLocks.LoadOrStore(accountID, &sync.Mutex{REDACTED)
	mu, ok := actual.(*sync.Mutex)
	if !ok {
		mu = &sync.Mutex{REDACTED
		s.openaiAccountRuntimeBlockLocks.Store(accountID, mu)
REDACTED
	return mu
REDACTED

func (s *OpenAIGatewayService) blockAccountSchedulingLocked(account *Account, until time.Time, _ string) (uint64, bool) {
	generation := s.openaiAccountRuntimeBlockSequence.Add(1)
	s.openaiAccountRuntimeBlockGeneration.Store(account.ID, generation)
	now := time.Now()
	blockUntil := until
	if blockUntil.IsZero() || !blockUntil.After(now) {
		blockUntil = now.Add(openAIStopSchedulingBridgeCooldown)
REDACTED

	for {
		current, loaded := s.openaiAccountRuntimeBlockUntil.Load(account.ID)
		if !loaded {
			actual, stored := s.openaiAccountRuntimeBlockUntil.LoadOrStore(account.ID, blockUntil)
			if !stored {
				return generation, true
		REDACTED
			current = actual
	REDACTED

		currentUntil, ok := current.(time.Time)
		if !ok || currentUntil.IsZero() {
			if s.openaiAccountRuntimeBlockUntil.CompareAndSwap(account.ID, current, blockUntil) {
				return generation, true
		REDACTED
			continue
	REDACTED
		if !blockUntil.After(currentUntil) {
			return generation, false
	REDACTED
		if s.openaiAccountRuntimeBlockUntil.CompareAndSwap(account.ID, current, blockUntil) {
			return generation, true
	REDACTED
REDACTED
REDACTED

func (s *OpenAIGatewayService) ClearAccountSchedulingBlock(accountID int64) {
	if s == nil || accountID <= 0 {
		return
REDACTED
	mu := s.openAIAccountRuntimeBlockLock(accountID)
	mu.Lock()
	defer mu.Unlock()
	s.openaiAccountRuntimeBlockUntil.Delete(accountID)
	s.openaiAccountRuntimeBlockGeneration.Store(accountID, s.openaiAccountRuntimeBlockSequence.Add(1))
REDACTED

func (s *OpenAIGatewayService) isOpenAIAccountRuntimeBlocked(account *Account) bool {
	if s == nil || !isOpenAIAccount(account) {
		return false
REDACTED
	mu := s.openAIAccountRuntimeBlockLock(account.ID)
	mu.Lock()
	defer mu.Unlock()
	value, ok := s.openaiAccountRuntimeBlockUntil.Load(account.ID)
	if !ok {
		return false
REDACTED
	cooldownUntil, ok := value.(time.Time)
	if !ok || cooldownUntil.IsZero() {
		s.openaiAccountRuntimeBlockUntil.Delete(account.ID)
		s.openaiAccountRuntimeBlockGeneration.Store(account.ID, s.openaiAccountRuntimeBlockSequence.Add(1))
		return false
REDACTED
	if time.Now().Before(cooldownUntil) {
		return true
REDACTED
	s.openaiAccountRuntimeBlockUntil.Delete(account.ID)
	s.openaiAccountRuntimeBlockGeneration.Store(account.ID, s.openaiAccountRuntimeBlockSequence.Add(1))
	return false
REDACTED

func (s *OpenAIGatewayService) getOpenAIAccountModelTransientState() *openAIAccountModelTransientState {
	if s == nil {
		return nil
REDACTED
	s.openaiModelTransientOnce.Do(func() {
		if s.openaiModelTransient == nil {
			s.openaiModelTransient = newOpenAIAccountModelTransientState(openAIModelTransientDefaultMax)
	REDACTED
REDACTED)
	return s.openaiModelTransient
REDACTED

func canonicalOpenAIAccountSchedulingModel(account *Account, requestedModel string) string {
	model := strings.TrimSpace(requestedModel)
	if account == nil || model == "" {
		return model
REDACTED
	if mapped := strings.TrimSpace(account.GetMappedModel(model)); mapped != "" {
		return mapped
REDACTED
	return model
REDACTED

func openAIAccountModelTransientModel(canonicalModel string) string {
	return normalizeOpenAIAccountModelTransientModel(canonicalModel)
REDACTED

func (s *OpenAIGatewayService) recordOpenAIAccountModelTransientFailure(account *Account, canonicalModel string, now time.Time) openAIAccountModelTransientDecision {
	if s == nil || account == nil {
		return openAIAccountModelTransientDecision{REDACTED
REDACTED
	state := s.getOpenAIAccountModelTransientState()
	if state == nil {
		return openAIAccountModelTransientDecision{REDACTED
REDACTED
	return state.recordFailure(account.ID, openAIAccountModelTransientModel(canonicalModel), now)
REDACTED

func (s *OpenAIGatewayService) clearOpenAIAccountModelTransientState(accountID int64, model string) {
	state := s.getOpenAIAccountModelTransientState()
	if state == nil {
		return
REDACTED
	state.recordSuccess(accountID, model)
REDACTED

func (s *OpenAIGatewayService) isOpenAIAccountModelRuntimeBlocked(account *Account, requestedModel string) bool {
	if s == nil || account == nil {
		return false
REDACTED
	state := s.getOpenAIAccountModelTransientState()
	if state == nil {
		return false
REDACTED
	canonicalModel := canonicalOpenAIAccountSchedulingModel(account, requestedModel)
	return state.isBlocked(account.ID, openAIAccountModelTransientModel(canonicalModel), time.Now())
REDACTED

func (s *OpenAIGatewayService) isOpenAIAccountRequestRuntimeBlocked(account *Account, requestedModel string) bool {
	return s != nil && (s.isOpenAIAccountRuntimeBlocked(account) || s.isOpenAIAccountModelRuntimeBlocked(account, requestedModel))
REDACTED

func (s *OpenAIGatewayService) recordOpenAIOAuth429() {
	if s == nil {
		return
REDACTED
	now := time.Now()
	windowStart := s.openaiOAuth429WindowStartUnixNano.Load()
	if windowStart == 0 || now.Sub(time.Unix(0, windowStart)) >= openAIOAuth429StormWindow {
		if s.openaiOAuth429WindowStartUnixNano.CompareAndSwap(windowStart, now.UnixNano()) {
			s.openaiOAuth429WindowCount.Store(1)
			return
	REDACTED
REDACTED
	s.openaiOAuth429WindowCount.Add(1)
REDACTED

func (s *OpenAIGatewayService) isOpenAIOAuth429Storm() bool {
	if s == nil {
		return false
REDACTED
	windowStart := s.openaiOAuth429WindowStartUnixNano.Load()
	if windowStart == 0 || time.Since(time.Unix(0, windowStart)) >= openAIOAuth429StormWindow {
		return false
REDACTED
	return s.openaiOAuth429WindowCount.Load() >= openAIOAuth429StormThreshold
REDACTED

func (s *OpenAIGatewayService) ShouldStopOpenAIOAuth429Failover(account *Account, statusCode int, failedSwitches int, state *OpenAIOAuth429FailoverState) bool {
	maxSwitches := openAIOAuth429StormMaxAccountSwitches
	if s != nil && s.settingService != nil {
		maxSwitches = s.rateLimit429StrategySettings().MaxAccountSwitches
REDACTED
	if failedSwitches < maxSwitches {
		return false
REDACTED
	if state != nil && state.grokOAuth429FollowupPending {
		// The follow-up budget was armed by a Grok OAuth 429. Consume it on
		// any failing follow-up account, even if a mixed pool selected an API-key
		// account next.
		return true
REDACTED
	if isGrokOAuthAccount(account) {
		if state == nil {
			// Preserve the old threshold for callers that have not adopted the
			// request-local state contract yet.
			return statusCode == http.StatusTooManyRequests && failedSwitches >= 2
	REDACTED
		if statusCode == http.StatusTooManyRequests {
			state.grokOAuth429FollowupPending = true
	REDACTED
		return false
REDACTED
	if statusCode != http.StatusTooManyRequests || !isOpenAIOAuthAccount(account) {
		return false
REDACTED
	// failedSwitches is incremented after each exhausted candidate. Therefore,
	// a value of three means this request has already given three distinct OAuth
	// accounts their full same-account retry window. A 429 storm is diagnostic
	// only; it must not skip those candidates and return a client 429 early.
	return failedSwitches >= maxSwitches+1
REDACTED
