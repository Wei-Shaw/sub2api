package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

// tokenRefreshTempUnschedDuration token 刷新重试耗尽后临时不可调度的持续时间
const tokenRefreshTempUnschedDuration = 10 * time.Minute

const (
	defaultTokenRefreshCandidatePageSize        = 200
	maxTokenRefreshCandidatePageSize            = 1000
	defaultTokenRefreshProviderConcurrency      = 4
	maxTokenRefreshProviderConcurrency          = 32
	defaultTokenRefreshProviderQPS              = 2
	maxTokenRefreshProviderQPS                  = 100
	defaultTokenRefreshProviderFailureThreshold = 3
	maxTokenRefreshProviderFailureThreshold     = 100
	defaultTokenRefreshMaxRetries               = 1
	maxTokenRefreshMaxRetries                   = 10
	maxTokenRefreshRetryBackoff                 = 30 * time.Second
	defaultTokenRefreshAttemptTimeout           = 15 * time.Second
	maxTokenRefreshAttemptTimeout               = 5 * time.Minute
	maxTokenRefreshLockSafetyMargin             = 5 * time.Second
	defaultTokenRefreshCycleTimeout             = 4 * time.Minute
	maxTokenRefreshCycleTimeout                 = time.Hour
	defaultTokenRefreshCleanupTimeout           = 2 * time.Second
)

type tokenRefreshRegistration struct {
	platform  string
	refresher TokenRefresher
	executor  OAuthRefreshExecutor
REDACTED

// GrokOAuthRefreshMutationRepository protects background refresh failure
// mutations with the exact credential document used by the upstream attempt.
// This contract is intentionally Grok-only; existing provider behavior remains
// unchanged.
type GrokOAuthRefreshMutationRepository interface {
	SetGrokOAuthRefreshErrorIfCredentialsUnchanged(ctx context.Context, id int64, expectedCredentials map[string]any, expectedProxyID *int64, errorMsg string) (bool, error)
	SetGrokOAuthRefreshTempUnschedulableIfCredentialsUnchanged(ctx context.Context, id int64, expectedCredentials map[string]any, expectedProxyID *int64, until time.Time, reason string) (bool, error)
REDACTED

// TokenRefreshService OAuth token自动刷新服务
// 定期检查并刷新即将过期的token
type TokenRefreshService struct {
	accountRepo      AccountRepository
	candidatePager   OAuthRefreshCandidatePager
	registrations    []tokenRefreshRegistration
	refreshPolicy    BackgroundRefreshPolicy
	cfg              *config.TokenRefreshConfig
	cacheInvalidator TokenCacheInvalidator
	schedulerCache   SchedulerCache   // 用于同步更新调度器缓存，解决 token 刷新后缓存不一致问题
	tempUnschedCache TempUnschedCache // 用于清除 Redis 中的临时不可调度缓存
	refreshAPI       *OAuthRefreshAPI // 统一刷新 API
	runtimeBlocker   AccountRuntimeBlocker

	// OpenAI privacy: 刷新成功后检查并设置 training opt-out
	privacyClientFactory PrivacyClientFactory
	proxyRepo            ProxyRepository

	stopCh        chan struct{REDACTED
	stopOnce      sync.Once
	wg            sync.WaitGroup
	runCtx        context.Context
	runCancel     context.CancelFunc
	candidateMu   sync.Mutex
	afterID       int64
	providerMu    sync.Mutex
	providerGates map[string]*tokenRefreshRateGate
	providerPools map[string]*tokenRefreshConcurrencyGate

	// Test-only duration seam; production uses TokenRefreshConfig seconds.
	attemptTimeoutOverride time.Duration
REDACTED

// NewTokenRefreshService 创建token刷新服务
func NewTokenRefreshService(
	accountRepo AccountRepository,
	oauthService *OAuthService,
	openaiOAuthService *OpenAIOAuthService,
	geminiOAuthService *GeminiOAuthService,
	antigravityOAuthService *AntigravityOAuthService,
	cacheInvalidator TokenCacheInvalidator,
	schedulerCache SchedulerCache,
	cfg *config.Config,
	tempUnschedCache TempUnschedCache,
	grokOAuthServices ...*GrokOAuthService,
) *TokenRefreshService {
	refreshCfg := &config.TokenRefreshConfig{REDACTED
	if cfg != nil {
		refreshCfg = &cfg.TokenRefresh
REDACTED
	runCtx, runCancel := context.WithCancel(context.Background())
	s := &TokenRefreshService{
		accountRepo:      accountRepo,
		refreshPolicy:    DefaultBackgroundRefreshPolicy(),
		cfg:              refreshCfg,
		cacheInvalidator: cacheInvalidator,
		schedulerCache:   schedulerCache,
		tempUnschedCache: tempUnschedCache,
		stopCh:           make(chan struct{REDACTED),
		runCtx:           runCtx,
		runCancel:        runCancel,
REDACTED
	if pager, ok := accountRepo.(OAuthRefreshCandidatePager); ok {
		s.candidatePager = pager
REDACTED

	openAIRefresher := NewOpenAITokenRefresher(openaiOAuthService, accountRepo)

	claudeRefresher := NewClaudeTokenRefresher(oauthService)
	geminiRefresher := NewGeminiTokenRefresher(geminiOAuthService)
	agRefresher := NewAntigravityTokenRefresher(antigravityOAuthService)
	var grokOAuthService *GrokOAuthService
	if len(grokOAuthServices) > 0 {
		grokOAuthService = grokOAuthServices[0]
REDACTED
	grokRefresher := NewGrokTokenRefresher(grokOAuthService)

	// Each provider is registered exactly once. The same registry supplies both
	// execution and repository eligibility, preventing future platform drift.
	s.registrations = []tokenRefreshRegistration{
		{platform: PlatformAnthropic, refresher: claudeRefresher, executor: claudeRefresherREDACTED,
		{platform: PlatformOpenAI, refresher: openAIRefresher, executor: openAIRefresherREDACTED,
		{platform: PlatformGemini, refresher: geminiRefresher, executor: geminiRefresherREDACTED,
		{platform: PlatformAntigravity, refresher: agRefresher, executor: agRefresherREDACTED,
		{platform: PlatformGrok, refresher: grokRefresher, executor: grokRefresherREDACTED,
REDACTED

	return s
REDACTED

func (s *TokenRefreshService) eligiblePlatforms() []string {
	platforms := make([]string, 0, len(s.registrations))
	for _, registration := range s.registrations {
		if registration.platform != "" && registration.refresher != nil {
			platforms = append(platforms, registration.platform)
	REDACTED
REDACTED
	return platforms
REDACTED

func (s *TokenRefreshService) candidateAfterID() int64 {
	s.candidateMu.Lock()
	defer s.candidateMu.Unlock()
	return s.afterID
REDACTED

func (s *TokenRefreshService) setCandidateAfterID(afterID int64) {
	s.candidateMu.Lock()
	s.afterID = afterID
	s.candidateMu.Unlock()
REDACTED

// SetPrivacyDeps 注入 OpenAI privacy opt-out 所需依赖
func (s *TokenRefreshService) SetPrivacyDeps(factory PrivacyClientFactory, proxyRepo ProxyRepository) {
	s.privacyClientFactory = factory
	s.proxyRepo = proxyRepo
REDACTED

// SetRefreshAPI 注入统一的 OAuth 刷新 API
func (s *TokenRefreshService) SetRefreshAPI(api *OAuthRefreshAPI) {
	s.refreshAPI = api
REDACTED

// SetRefreshPolicy 注入后台刷新调用侧策略（用于显式化平台/场景差异行为）。
func (s *TokenRefreshService) SetRefreshPolicy(policy BackgroundRefreshPolicy) {
	s.refreshPolicy = policy
REDACTED

func (s *TokenRefreshService) SetAccountRuntimeBlocker(blocker AccountRuntimeBlocker) {
	s.runtimeBlocker = blocker
REDACTED

func (s *TokenRefreshService) notifyAccountSchedulingBlocked(account *Account, until time.Time, reason string) {
	if s == nil || s.runtimeBlocker == nil || account == nil {
		return
REDACTED
	s.runtimeBlocker.BlockAccountScheduling(account, until, reason)
REDACTED

func (s *TokenRefreshService) notifyAccountSchedulingBlockCleared(accountID int64) {
	if s == nil || s.runtimeBlocker == nil || accountID <= 0 {
		return
REDACTED
	s.runtimeBlocker.ClearAccountSchedulingBlock(accountID)
REDACTED

// Start 启动后台刷新服务
func (s *TokenRefreshService) Start() {
	if s.cfg == nil || !s.cfg.Enabled {
		slog.Info("token_refresh.service_disabled")
		return
REDACTED

	s.wg.Add(1)
	go s.refreshLoop()

	slog.Info("token_refresh.service_started",
		"check_interval_minutes", s.cfg.CheckIntervalMinutes,
		"refresh_before_expiry_hours", s.cfg.RefreshBeforeExpiryHours,
	)
REDACTED

// Stop 停止刷新服务（可安全多次调用）
func (s *TokenRefreshService) Stop() {
	s.stopOnce.Do(func() {
		if s.runCancel != nil {
			s.runCancel()
	REDACTED
		close(s.stopCh)
REDACTED)
	s.wg.Wait()
	slog.Info("token_refresh.service_stopped")
REDACTED

// refreshLoop 刷新循环
func (s *TokenRefreshService) refreshLoop() {
	defer s.wg.Done()
	ctx := s.runCtx
	if ctx == nil {
		ctx = context.Background()
REDACTED

	// 计算检查间隔
	checkInterval := time.Duration(s.cfg.CheckIntervalMinutes) * time.Minute
	if checkInterval < time.Minute {
		checkInterval = 5 * time.Minute
REDACTED

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// 启动时立即执行一次检查
	s.processRefreshContext(ctx)

	for {
		select {
		case <-ticker.C:
			s.processRefreshContext(ctx)
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
	REDACTED
REDACTED
REDACTED

type tokenRefreshPageStats struct {
	total        int
	oauth        int
	needsRefresh int
	refreshed    int
	skipped      int
	failed       int
REDACTED

type tokenRefreshProviderState struct {
	service      *TokenRefreshService
	registration tokenRefreshRegistration
	rateGate     refreshAttemptGate
	poolGate     *tokenRefreshConcurrencyGate

	mu                  sync.Mutex
	consecutiveFailures int
	tripped             bool
REDACTED

type tokenRefreshRateGate struct {
	mu       sync.Mutex
	next     time.Time
	interval time.Duration
REDACTED

type tokenRefreshConcurrencyGate struct {
	slots chan struct{REDACTED
REDACTED

type refreshAttemptGate interface {
	acquire(ctx context.Context) (release func(), err error)
REDACTED

type providerRefreshAttemptGate interface {
	refreshAttemptGate
	acquireRate(ctx context.Context) (release func(), err error)
REDACTED

type rateLimitedOAuthRefreshExecutor struct {
	OAuthRefreshExecutor
	acquireRate func(context.Context) (func(), error)
REDACTED

func (e *rateLimitedOAuthRefreshExecutor) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if e == nil || e.OAuthRefreshExecutor == nil {
		return nil, errors.New("OAuth refresh executor is not configured")
REDACTED
	release := func() {REDACTED
	if e.acquireRate != nil {
		var err error
		release, err = e.acquireRate(ctx)
		if err != nil {
			return nil, err
	REDACTED
REDACTED
	defer release()
	return e.OAuthRefreshExecutor.Refresh(ctx, account)
REDACTED

func newTokenRefreshRateGate(qps int) *tokenRefreshRateGate {
	if qps <= 0 {
		return &tokenRefreshRateGate{REDACTED
REDACTED
	return newTokenRefreshRateGateWithInterval(time.Second / time.Duration(qps))
REDACTED

// newTokenRefreshRateGateWithInterval is a narrow duration seam used to test
// slot reservation and cancellation without waiting on production-scale QPS.
func newTokenRefreshRateGateWithInterval(interval time.Duration) *tokenRefreshRateGate {
	return &tokenRefreshRateGate{interval: intervalREDACTED
REDACTED

func (g *tokenRefreshRateGate) reserveSlot(now time.Time) time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.next.Before(now) {
		g.next = now
REDACTED
	slot := g.next
	g.next = g.next.Add(g.interval)
	return slot
REDACTED

func (g *tokenRefreshRateGate) wait(ctx context.Context) error {
	if g == nil || g.interval <= 0 {
		return nil
REDACTED
	slot := g.reserveSlot(time.Now())

	wait := time.Until(slot)
	if wait <= 0 {
		return nil
REDACTED
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
REDACTED
REDACTED

func (g *tokenRefreshRateGate) acquire(ctx context.Context) (func(), error) {
	if err := g.wait(ctx); err != nil {
		return nil, err
REDACTED
	return func() {REDACTED, nil
REDACTED

func newTokenRefreshConcurrencyGate(concurrency int) *tokenRefreshConcurrencyGate {
	if concurrency < 1 {
		concurrency = 1
REDACTED
	return &tokenRefreshConcurrencyGate{slots: make(chan struct{REDACTED, concurrency)REDACTED
REDACTED

func (g *tokenRefreshConcurrencyGate) acquire(ctx context.Context) (func(), error) {
	if g == nil {
		return func() {REDACTED, nil
REDACTED
	select {
	case g.slots <- struct{REDACTED{REDACTED:
		return func() { <-g.slots REDACTED, nil
	case <-ctx.Done():
		return nil, ctx.Err()
REDACTED
REDACTED

func (p *tokenRefreshProviderState) isTripped() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tripped
REDACTED

func (p *tokenRefreshProviderState) acquire(ctx context.Context) (func(), error) {
	if p == nil || p.isTripped() {
		return nil, errRefreshSkipped
REDACTED
	release, err := p.poolGate.acquire(ctx)
	if err != nil {
		return nil, err
REDACTED
	if p.isTripped() {
		release()
		return nil, errRefreshSkipped
REDACTED
	return release, nil
REDACTED

func (p *tokenRefreshProviderState) acquireRate(ctx context.Context) (func(), error) {
	if p == nil || p.isTripped() {
		return nil, errRefreshSkipped
REDACTED
	release := func() {REDACTED
	if p.rateGate != nil {
		var err error
		release, err = p.rateGate.acquire(ctx)
		if err != nil {
			return nil, err
	REDACTED
REDACTED
	if p.isTripped() {
		release()
		return nil, errRefreshSkipped
REDACTED
	return release, nil
REDACTED

func (p *tokenRefreshProviderState) recordResult(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err == nil {
		p.consecutiveFailures = 0
		return
REDACTED
	if errors.Is(err, errRefreshSkipped) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		var attemptTimeoutErr *refreshAttemptTimeoutError
		if !errors.As(err, &attemptTimeoutErr) {
			return
	REDACTED
REDACTED
	var attemptTimeoutErr *refreshAttemptTimeoutError
	if errors.As(err, &attemptTimeoutErr) {
		p.consecutiveFailures++
		if p.consecutiveFailures >= p.service.providerFailureThreshold() {
			p.tripped = true
	REDACTED
		return
REDACTED
	var providerErr *providerConfigurationRefreshError
	if errors.As(err, &providerErr) {
		p.tripped = true
		return
REDACTED
	var containmentErr *providerCycleContainmentRefreshError
	if errors.As(err, &containmentErr) {
		p.tripped = true
		return
REDACTED
	var permanentErr *accountPermanentRefreshError
	if errors.As(err, &permanentErr) {
		p.consecutiveFailures = 0
		return
REDACTED
	if isNonRetryableRefreshError(err) {
		// A permanent account credential failure is isolated to that account and
		// does not imply the provider is unhealthy.
		p.consecutiveFailures = 0
		return
REDACTED
	p.consecutiveFailures++
	if p.consecutiveFailures >= p.service.providerFailureThreshold() {
		p.tripped = true
REDACTED
REDACTED

// processRefresh preserves the existing test/internal call surface while the
// production loop supplies a cancelable parent context.
func (s *TokenRefreshService) processRefresh() {
	s.processRefreshContext(context.Background())
REDACTED

// processRefreshContext executes one bounded, cursor-resumable refresh cycle.
func (s *TokenRefreshService) processRefreshContext(parent context.Context) {
	if parent == nil {
		parent = context.Background()
REDACTED
	ctx, cancel := context.WithTimeout(parent, s.cycleTimeout())
	defer cancel()

	pager := s.candidatePager
	if pager == nil {
		pager, _ = s.accountRepo.(OAuthRefreshCandidatePager)
REDACTED
	if pager == nil {
		slog.Error("token_refresh.candidate_pager_missing")
		return
REDACTED
	platforms := s.eligiblePlatforms()
	if len(platforms) == 0 {
		slog.Error("token_refresh.provider_registry_empty")
		return
REDACTED

	refreshWindow := time.Duration(s.cfg.RefreshBeforeExpiryHours * float64(time.Hour))
	pageSize := s.candidatePageSize()
	providerStates := make(map[string]*tokenRefreshProviderState, len(s.registrations))
	for i := range s.registrations {
		registration := s.registrations[i]
		providerStates[registration.platform] = &tokenRefreshProviderState{
			service:      s,
			registration: registration,
			rateGate:     s.providerRateGate(registration.platform),
			poolGate:     s.providerConcurrencyGate(registration.platform),
	REDACTED
REDACTED

	stats := tokenRefreshPageStats{REDACTED
	afterID := s.candidateAfterID()
	for {
		if ctx.Err() != nil {
			slog.Warn("token_refresh.cycle_stopped", "error", ctx.Err(), "resume_after_id", afterID)
			break
	REDACTED
		page, err := pager.ListOAuthRefreshCandidatePage(ctx, OAuthRefreshPageOptions{
			Platforms:            platforms,
			AfterID:              afterID,
			Limit:                pageSize,
			ActiveOnly:           true,
			IncludeSetupToken:    true,
			RequireRefreshToken:  true,
			ExcludeRetryCooldown: true,
	REDACTED)
		if err != nil {
			slog.Error("token_refresh.list_accounts_failed", "error", err, "after_id", afterID)
			break
	REDACTED
		if page == nil {
			slog.Error("token_refresh.nil_candidate_page", "after_id", afterID)
			break
	REDACTED
		accounts := page.Accounts
		if !page.HasMore && page.NextAfterID == 0 && len(accounts) == 0 {
			s.setCandidateAfterID(0)
			break
	REDACTED
		if page.NextAfterID <= afterID {
			slog.Error("token_refresh.invalid_candidate_page_metadata", "after_id", afterID)
			break
	REDACTED
		if !isStrictlyIncreasingAccountPage(accounts, afterID) {
			slog.Error("token_refresh.invalid_candidate_page", "after_id", afterID, "count", len(accounts))
			break
	REDACTED

		pageStats := s.processCandidatePage(ctx, accounts, providerStates, refreshWindow)
		stats.total += pageStats.total
		stats.oauth += pageStats.oauth
		stats.needsRefresh += pageStats.needsRefresh
		stats.refreshed += pageStats.refreshed
		stats.skipped += pageStats.skipped
		stats.failed += pageStats.failed

		// Never advance past a partially processed page. Re-reading a page is
		// safe because OAuthRefreshAPI re-reads DB state and checks expiry again.
		if ctx.Err() != nil {
			break
	REDACTED
		afterID = page.NextAfterID
		s.setCandidateAfterID(afterID)
		if !page.HasMore {
			s.setCandidateAfterID(0)
			break
	REDACTED
REDACTED

	if stats.needsRefresh == 0 && stats.failed == 0 {
		slog.Debug("token_refresh.cycle_completed",
			"total", stats.total, "oauth", stats.oauth,
			"needs_refresh", stats.needsRefresh, "refreshed", stats.refreshed,
			"skipped", stats.skipped, "failed", stats.failed)
REDACTED else {
		slog.Info("token_refresh.cycle_completed",
			"total", stats.total, "oauth", stats.oauth,
			"needs_refresh", stats.needsRefresh, "refreshed", stats.refreshed,
			"skipped", stats.skipped, "failed", stats.failed)
REDACTED
REDACTED

func isStrictlyIncreasingAccountPage(accounts []Account, afterID int64) bool {
	previous := afterID
	for i := range accounts {
		if accounts[i].ID <= previous {
			return false
	REDACTED
		previous = accounts[i].ID
REDACTED
	return true
REDACTED

func (s *TokenRefreshService) processCandidatePage(
	ctx context.Context,
	accounts []Account,
	providerStates map[string]*tokenRefreshProviderState,
	refreshWindow time.Duration,
) tokenRefreshPageStats {
	stats := tokenRefreshPageStats{total: len(accounts)REDACTED
	groups := make(map[string][]*Account)
	for i := range accounts {
		account := &accounts[i]
		state := providerStates[account.Platform]
		if state == nil || state.registration.refresher == nil || !state.registration.refresher.CanRefresh(account) {
			continue
	REDACTED
		stats.oauth++
		if !state.registration.refresher.NeedsRefresh(account, refreshWindow) {
			continue
	REDACTED
		stats.needsRefresh++
		groups[account.Platform] = append(groups[account.Platform], account)
REDACTED

	type providerResult struct {
		refreshed int
		skipped   int
		failed    int
REDACTED
	results := make(chan providerResult, len(groups))
	var wg sync.WaitGroup
	for platform, group := range groups {
		state := providerStates[platform]
		wg.Add(1)
		go func() {
			defer wg.Done()
			refreshed, skipped, failed := s.processProviderAccounts(ctx, state, group, refreshWindow)
			results <- providerResult{refreshed: refreshed, skipped: skipped, failed: failedREDACTED
	REDACTED()
REDACTED
	wg.Wait()
	close(results)
	for result := range results {
		stats.refreshed += result.refreshed
		stats.skipped += result.skipped
		stats.failed += result.failed
REDACTED
	return stats
REDACTED

func (s *TokenRefreshService) processProviderAccounts(
	ctx context.Context,
	state *tokenRefreshProviderState,
	accounts []*Account,
	refreshWindow time.Duration,
) (refreshed, skipped, failed int) {
	if state == nil || len(accounts) == 0 {
		return 0, 0, 0
REDACTED
	type refreshResult struct {
		accountID int64
		err       error
REDACTED
	jobs := make(chan *Account, len(accounts))
	results := make(chan refreshResult, len(accounts))
	workerCount := s.providerConcurrency()
	if workerCount > len(accounts) {
		workerCount = len(accounts)
REDACTED
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for account := range jobs {
				if ctx.Err() != nil || state.isTripped() {
					results <- refreshResult{accountID: account.ID, err: errRefreshSkippedREDACTED
					continue
			REDACTED
				if state.isTripped() {
					results <- refreshResult{accountID: account.ID, err: errRefreshSkippedREDACTED
					continue
			REDACTED
				err := s.refreshWithRetryWithRateGate(ctx, account, state.registration.refresher, state.registration.executor, refreshWindow, state)
				state.recordResult(err)
				results <- refreshResult{accountID: account.ID, err: errREDACTED
		REDACTED
	REDACTED()
REDACTED
	for _, account := range accounts {
		jobs <- account
REDACTED
	close(jobs)
	wg.Wait()
	close(results)

	for result := range results {
		switch {
		case result.err == nil:
			refreshed++
			slog.Info("token_refresh.account_refreshed", "account_id", result.accountID, "platform", state.registration.platform)
		case errors.Is(result.err, errRefreshSkipped):
			skipped++
		default:
			failed++
			slog.Warn("token_refresh.account_refresh_failed", "account_id", result.accountID, "platform", state.registration.platform, "error", logredact.RedactText(result.err.Error()))
	REDACTED
REDACTED
	return refreshed, skipped, failed
REDACTED

func (s *TokenRefreshService) candidatePageSize() int {
	if s.cfg != nil && s.cfg.CandidatePageSize > 0 {
		return min(s.cfg.CandidatePageSize, maxTokenRefreshCandidatePageSize)
REDACTED
	return defaultTokenRefreshCandidatePageSize
REDACTED

func (s *TokenRefreshService) providerConcurrency() int {
	if s.cfg != nil && s.cfg.ProviderConcurrency > 0 {
		return min(s.cfg.ProviderConcurrency, maxTokenRefreshProviderConcurrency)
REDACTED
	return defaultTokenRefreshProviderConcurrency
REDACTED

func (s *TokenRefreshService) providerQPS() int {
	if s.cfg != nil && s.cfg.ProviderQPS > 0 {
		return min(s.cfg.ProviderQPS, maxTokenRefreshProviderQPS)
REDACTED
	return defaultTokenRefreshProviderQPS
REDACTED

// providerRateGate returns the process-local limiter shared by background
// cycles and admin reconciliation. Sharing it prevents concurrent entry points
// or retries from multiplying the configured per-provider request rate.
func (s *TokenRefreshService) providerRateGate(platform string) *tokenRefreshRateGate {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	if s.providerGates == nil {
		s.providerGates = make(map[string]*tokenRefreshRateGate)
REDACTED
	if gate := s.providerGates[platform]; gate != nil {
		return gate
REDACTED
	gate := newTokenRefreshRateGate(s.providerQPS())
	s.providerGates[platform] = gate
	return gate
REDACTED

// providerConcurrencyGate returns the process-local semaphore shared by every
// background cycle and admin reconciliation call for a provider. It is
// acquired and released around each upstream retry attempt, so parallel entry
// points cannot multiply ProviderConcurrency.
func (s *TokenRefreshService) providerConcurrencyGate(platform string) *tokenRefreshConcurrencyGate {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	if s.providerPools == nil {
		s.providerPools = make(map[string]*tokenRefreshConcurrencyGate)
REDACTED
	if gate := s.providerPools[platform]; gate != nil {
		return gate
REDACTED
	gate := newTokenRefreshConcurrencyGate(s.providerConcurrency())
	s.providerPools[platform] = gate
	return gate
REDACTED

func (s *TokenRefreshService) providerFailureThreshold() int {
	if s.cfg != nil && s.cfg.ProviderFailureThreshold > 0 {
		return min(s.cfg.ProviderFailureThreshold, maxTokenRefreshProviderFailureThreshold)
REDACTED
	return defaultTokenRefreshProviderFailureThreshold
REDACTED

func (s *TokenRefreshService) attemptTimeout() time.Duration {
	timeout := defaultTokenRefreshAttemptTimeout
	if s.attemptTimeoutOverride > 0 {
		timeout = s.attemptTimeoutOverride
REDACTED else if s.cfg != nil && s.cfg.AttemptTimeoutSeconds > 0 {
		seconds := min(s.cfg.AttemptTimeoutSeconds, int(maxTokenRefreshAttemptTimeout/time.Second))
		timeout = time.Duration(seconds) * time.Second
REDACTED
	if s.refreshAPI != nil && s.refreshAPI.tokenCache != nil {
		timeout = clampRefreshAttemptToLockLease(timeout, s.refreshAPI.lockTTL)
REDACTED
	return timeout
REDACTED

func clampRefreshAttemptToLockLease(timeout, lease time.Duration) time.Duration {
	if timeout <= 0 || lease <= 0 {
		return timeout
REDACTED
	margin := lease / 10
	if margin > maxTokenRefreshLockSafetyMargin {
		margin = maxTokenRefreshLockSafetyMargin
REDACTED
	if margin <= 0 {
		margin = time.Nanosecond
REDACTED
	leaseBudget := lease - margin
	if leaseBudget <= 0 {
		leaseBudget = lease / 2
REDACTED
	if leaseBudget > 0 && timeout > leaseBudget {
		return leaseBudget
REDACTED
	return timeout
REDACTED

func (s *TokenRefreshService) cycleTimeout() time.Duration {
	if s.cfg != nil && s.cfg.CycleTimeoutSeconds > 0 {
		seconds := min(s.cfg.CycleTimeoutSeconds, int(maxTokenRefreshCycleTimeout/time.Second))
		return time.Duration(seconds) * time.Second
REDACTED
	return defaultTokenRefreshCycleTimeout
REDACTED

func (s *TokenRefreshService) maxRetries() int {
	if s.cfg != nil && s.cfg.MaxRetries > 0 {
		return min(s.cfg.MaxRetries, maxTokenRefreshMaxRetries)
REDACTED
	return defaultTokenRefreshMaxRetries
REDACTED

// refreshWithRetry 带重试的刷新
func (s *TokenRefreshService) refreshWithRetry(ctx context.Context, account *Account, refresher TokenRefresher, executor OAuthRefreshExecutor, refreshWindow time.Duration) error {
	return s.refreshWithRetryWithRateGate(ctx, account, refresher, executor, refreshWindow, nil)
REDACTED

func (s *TokenRefreshService) refreshWithRetryWithRateGate(
	ctx context.Context,
	account *Account,
	refresher TokenRefresher,
	executor OAuthRefreshExecutor,
	refreshWindow time.Duration,
	gate refreshAttemptGate,
) error {
	var lastErr error
	maxRetries := s.maxRetries()

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
	REDACTED
		releaseAttempt := func() {REDACTED
		var acquireRate func(context.Context) (func(), error)
		if gate != nil {
			if providerGate, ok := gate.(providerRefreshAttemptGate); ok {
				var err error
				releaseAttempt, err = providerGate.acquire(ctx)
				if err != nil {
					return err
			REDACTED
				acquireRate = providerGate.acquireRate
		REDACTED else {
				// Compatibility gates are rate-admission gates. Acquire them only
				// when an upstream Refresh call is actually about to start.
				acquireRate = gate.acquire
		REDACTED
	REDACTED
		attemptCtx, cancelAttempt := context.WithTimeout(ctx, s.attemptTimeout())
		var newCredentials map[string]any
		var err error
		shortCircuit := false
		credentialsPersisted := false

		// 优先使用统一 API（带分布式锁 + DB 重读保护）
		if s.refreshAPI != nil && executor != nil {
			actualExecutor := executor
			if acquireRate != nil {
				actualExecutor = &rateLimitedOAuthRefreshExecutor{
					OAuthRefreshExecutor: executor,
					acquireRate:          acquireRate,
			REDACTED
		REDACTED
			result, refreshErr := s.refreshAPI.RefreshIfNeeded(attemptCtx, account, actualExecutor, refreshWindow)
			if result != nil && result.Account != nil {
				account = result.Account
		REDACTED
			if refreshErr != nil {
				err = refreshErr
		REDACTED else if result.LockHeld {
				// 锁被其他 worker 持有，由调用侧策略决定如何计数
				err = s.refreshPolicy.handleLockHeld()
				shortCircuit = true
		REDACTED else if !result.Refreshed {
				// 已被其他路径刷新，由调用侧策略决定如何计数
				err = s.refreshPolicy.handleAlreadyRefreshed()
				shortCircuit = true
		REDACTED else {
				credentialsPersisted = result.NewCredentials != nil
				_ = result.NewCredentials // 统一 API 已设置 _token_version 并更新 DB，无需重复操作
		REDACTED
	REDACTED else {
			// 降级：直接调用 refresher（兼容旧路径）
			releaseRate := func() {REDACTED
			if acquireRate != nil {
				releaseRate, err = acquireRate(attemptCtx)
		REDACTED
			if err == nil {
				newCredentials, err = refresher.Refresh(attemptCtx, account)
		REDACTED
			if releaseRate != nil {
				releaseRate()
		REDACTED
			attemptTimedOut := errors.Is(attemptCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil
			if err == nil && newCredentials != nil && !attemptTimedOut {
				newCredentials["_token_version"] = time.Now().UnixMilli()
				if saveErr := persistAccountCredentials(attemptCtx, s.accountRepo, account, newCredentials); saveErr != nil {
					err = fmt.Errorf("failed to save credentials: %w", saveErr)
			REDACTED else {
					credentialsPersisted = true
			REDACTED
		REDACTED
	REDACTED
		attemptTimedOut := errors.Is(attemptCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil
		cancelAttempt()
		releaseAttempt()
		persistedAfterAttemptDeadline := attemptTimedOut && credentialsPersisted && err == nil
		if attemptTimedOut && !persistedAfterAttemptDeadline && !isProviderScopedTerminalRefreshError(err) {
			cause := err
			if cause == nil {
				cause = context.DeadlineExceeded
		REDACTED
			err = &refreshAttemptTimeoutError{err: causeREDACTED
			shortCircuit = false
			if credentialsPersisted {
				s.postRefreshStateSyncWithCleanup(ctx, account)
		REDACTED
	REDACTED
		if shortCircuit {
			return err
	REDACTED

		if err == nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				if credentialsPersisted {
					s.postRefreshStateSyncWithCleanup(ctx, account)
			REDACTED
				return ctxErr
		REDACTED
			if persistedAfterAttemptDeadline {
				// The provider result and exact-state CAS are already durable. Only
				// the internal attempt budget elapsed while bounded detached cleanup
				// completed; do not convert that success into retry/cooldown/breaker
				// evidence. Publish cache state with a fresh cleanup context and stop.
				s.postRefreshStateSyncWithCleanup(ctx, account)
				return nil
		REDACTED
			s.postRefreshActions(ctx, account)
			return nil
	REDACTED
		if ctxErr := ctx.Err(); ctxErr != nil {
			if credentialsPersisted {
				s.postRefreshStateSyncWithCleanup(ctx, account)
		REDACTED
			return ctxErr
	REDACTED
		if errors.Is(err, errRefreshSkipped) {
			return errRefreshSkipped
	REDACTED
		if isProviderScopedTerminalRefreshError(err) {
			return err
	REDACTED
		var stateUnavailableErr *oauthRefreshStateUnavailableError
		if errors.As(err, &stateUnavailableErr) {
			return &providerCycleContainmentRefreshError{err: errREDACTED
	REDACTED
		if isAmbiguousGrokEntitlementRefreshError(account, err) {
			// The current Grok client labels every token-endpoint 403 as an
			// entitlement denial. Without explicit entitlement evidence, contain
			// the provider for this cycle instead of disabling an account on a
			// possible WAF or shared provider failure.
			return &providerCycleContainmentRefreshError{err: errREDACTED
	REDACTED

		// Provider-wide OAuth client/scope failures are not evidence that every
		// account is invalid. Return a typed internal signal so the cycle contains
		// the provider without mutating account state.
		if isSharedProviderRefreshError(err) {
			return &providerConfigurationRefreshError{err: errREDACTED
	REDACTED

		// 不可重试错误（invalid_grant/invalid_client 等）直接标记 error 状态并返回
		if isNonRetryableRefreshError(err) {
			errorMsg := "Token refresh failed (non-retryable): " + logredact.RedactText(err.Error())
			isGrokOAuth := account.IsGrokOAuth()
			if !isGrokOAuth {
				s.notifyAccountSchedulingBlocked(account, time.Time{REDACTED, "token_refresh_non_retryable")
		REDACTED
			s.clearAntigravityForceTokenRefresh(ctx, account, "non_retryable")
			persistentlyBlocked := false
			var setErr error
			if isGrokOAuth {
				conditionalRepo, ok := s.accountRepo.(GrokOAuthRefreshMutationRepository)
				if !ok {
					return &providerConfigurationRefreshError{
						err: errors.New("Grok OAuth conditional refresh mutation repository is not configured"),
				REDACTED
			REDACTED else {
					persistentlyBlocked, setErr = conditionalRepo.SetGrokOAuthRefreshErrorIfCredentialsUnchanged(
						ctx,
						account.ID,
						account.Credentials,
						account.ProxyID,
						errorMsg,
					)
					if setErr == nil && !persistentlyBlocked {
						slog.Info("token_refresh.grok_error_status_skipped_stale_credentials", "account_id", account.ID)
						return errRefreshSkipped
				REDACTED
			REDACTED
		REDACTED else {
				setErr = s.accountRepo.SetError(ctx, account.ID, errorMsg)
				persistentlyBlocked = setErr == nil
		REDACTED
			if setErr != nil {
				slog.Error("token_refresh.set_error_status_failed",
					"account_id", account.ID,
					"error", setErr,
				)
				if isGrokOAuth {
					return &providerCycleContainmentRefreshError{
						err: fmt.Errorf("failed to conditionally persist Grok OAuth refresh failure: %w", setErr),
				REDACTED
			REDACTED
		REDACTED else if isGrokOAuth && persistentlyBlocked {
				s.notifyAccountSchedulingBlocked(account, time.Time{REDACTED, "token_refresh_non_retryable")
		REDACTED
			cacheInvalidationFailed := false
			if account.Type == AccountTypeOAuth && (!isGrokOAuth || persistentlyBlocked) {
				if s.cacheInvalidator == nil {
					cacheInvalidationFailed = true
			REDACTED else if invalidateErr := s.cacheInvalidator.InvalidateToken(ctx, account); invalidateErr != nil {
					cacheInvalidationFailed = true
					slog.Warn("token_refresh.invalidate_failed_token_cache_failed",
						"account_id", account.ID,
						"error", logredact.RedactText(invalidateErr.Error()),
					)
			REDACTED
		REDACTED
			return &accountPermanentRefreshError{
				err:                     err,
				persistentlyBlocked:     persistentlyBlocked,
				cacheInvalidationFailed: cacheInvalidationFailed,
		REDACTED
	REDACTED

		lastErr = err
		slog.Warn("token_refresh.retry_attempt_failed",
			"account_id", account.ID,
			"attempt", attempt,
			"max_retries", maxRetries,
			"error", logredact.RedactText(err.Error()),
		)

		// 如果还有重试机会，等待后重试
		if attempt < maxRetries {
			backoff := s.retryBackoff(account.ID, attempt)
			if backoff > 0 {
				timer := time.NewTimer(backoff)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
			REDACTED
		REDACTED
	REDACTED
REDACTED
	if err := ctx.Err(); err != nil {
		return err
REDACTED

	// 可重试错误耗尽：临时标记账号不可调度，避免请求路径反复命中已知失败的账号
	slog.Warn("token_refresh.retry_exhausted",
		"account_id", account.ID,
		"platform", account.Platform,
		"max_retries", maxRetries,
		"error", logredact.RedactText(lastErr.Error()),
	)

	// 设置临时不可调度 10 分钟（不标记 error，保持 status=active 让下个刷新周期能继续尝试）
	until := time.Now().Add(tokenRefreshTempUnschedDuration)
	reason := "token refresh retry exhausted"
	if lastErr != nil {
		reason += ": " + logredact.RedactText(lastErr.Error())
REDACTED
	if account.IsGrokOAuth() {
		conditionalRepo, ok := s.accountRepo.(GrokOAuthRefreshMutationRepository)
		if !ok {
			return &providerConfigurationRefreshError{
				err: errors.New("Grok OAuth conditional refresh mutation repository is not configured"),
		REDACTED
	REDACTED
		applied, setErr := conditionalRepo.SetGrokOAuthRefreshTempUnschedulableIfCredentialsUnchanged(
			ctx,
			account.ID,
			account.Credentials,
			account.ProxyID,
			until,
			reason,
		)
		if setErr != nil {
			slog.Warn("token_refresh.set_temp_unschedulable_failed",
				"account_id", account.ID,
				"error", setErr,
			)
			return &providerCycleContainmentRefreshError{
				err: fmt.Errorf("failed to conditionally persist Grok OAuth refresh cooldown: %w", setErr),
		REDACTED
	REDACTED else if !applied {
			slog.Info("token_refresh.grok_temp_unschedulable_skipped_stale_credentials", "account_id", account.ID)
			return errRefreshSkipped
	REDACTED else {
			s.notifyAccountSchedulingBlocked(account, until, "token_refresh_retry_exhausted")
			slog.Info("token_refresh.temp_unschedulable_set",
				"account_id", account.ID,
				"until", until.Format(time.RFC3339),
			)
	REDACTED
		return lastErr
REDACTED

	s.notifyAccountSchedulingBlocked(account, until, "token_refresh_retry_exhausted")
	if setErr := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, reason); setErr != nil {
		slog.Warn("token_refresh.set_temp_unschedulable_failed",
			"account_id", account.ID,
			"error", setErr,
		)
REDACTED else {
		slog.Info("token_refresh.temp_unschedulable_set",
			"account_id", account.ID,
			"until", until.Format(time.RFC3339),
		)
REDACTED

	return lastErr
REDACTED

func (s *TokenRefreshService) retryBackoff(accountID int64, attempt int) time.Duration {
	if s.cfg == nil || s.cfg.RetryBackoffSeconds <= 0 {
		return 0
REDACTED
	shift := attempt - 1
	if shift > 10 {
		shift = 10
REDACTED
	baseSeconds := min(s.cfg.RetryBackoffSeconds, int(maxTokenRefreshRetryBackoff/time.Second))
	base := time.Duration(baseSeconds) * time.Second * time.Duration(1<<shift)
	// Stable 75-125% jitter prevents synchronized replicas from retrying on the
	// same boundaries without making tests or operations nondeterministic.
	jitterPercent := int64(75) + (accountID+int64(attempt*17))%51
	backoff := base * time.Duration(jitterPercent) / 100
	return min(backoff, maxTokenRefreshRetryBackoff)
REDACTED

// postRefreshActions 刷新成功后的后续动作（清除错误状态、缓存失效、调度器同步等）
func (s *TokenRefreshService) postRefreshActions(ctx context.Context, account *Account) {
	s.clearAntigravityForceTokenRefresh(ctx, account, "success")

	// Antigravity 账户：如果之前是因为缺少 project_id 而标记为 error，现在成功获取到了，清除错误状态
	if account.Platform == PlatformAntigravity &&
		account.Status == StatusError &&
		strings.Contains(account.ErrorMessage, "missing_project_id:") {
		if clearErr := s.accountRepo.ClearError(ctx, account.ID); clearErr != nil {
			slog.Warn("token_refresh.clear_account_error_failed",
				"account_id", account.ID,
				"error", clearErr,
			)
	REDACTED else {
			slog.Info("token_refresh.cleared_missing_project_id_error", "account_id", account.ID)
			s.notifyAccountSchedulingBlockCleared(account.ID)
	REDACTED
REDACTED
	// 刷新成功后清除临时不可调度状态（处理 OAuth 401 恢复场景）
	if account.TempUnschedulableUntil != nil && time.Now().Before(*account.TempUnschedulableUntil) {
		if clearErr := s.accountRepo.ClearTempUnschedulable(ctx, account.ID); clearErr != nil {
			slog.Warn("token_refresh.clear_temp_unschedulable_failed",
				"account_id", account.ID,
				"error", clearErr,
			)
	REDACTED else {
			slog.Info("token_refresh.cleared_temp_unschedulable", "account_id", account.ID)
			s.notifyAccountSchedulingBlockCleared(account.ID)
	REDACTED
		// 同步清除 Redis 缓存，避免调度器读到过期的临时不可调度状态
		if s.tempUnschedCache != nil {
			if clearErr := s.tempUnschedCache.DeleteTempUnsched(ctx, account.ID); clearErr != nil {
				slog.Warn("token_refresh.clear_temp_unsched_cache_failed",
					"account_id", account.ID,
					"error", clearErr,
				)
		REDACTED
	REDACTED
REDACTED
	s.postRefreshStateSync(ctx, account)
	// OpenAI OAuth: 刷新成功后，检查是否已设置 privacy_mode，未设置则尝试关闭训练数据共享
	s.ensureOpenAIPrivacy(ctx, account)
	// Antigravity OAuth: 刷新成功后，检查是否已设置 privacy_mode，未设置则调用 setUserSettings
	s.ensureAntigravityPrivacy(ctx, account)
REDACTED

func (s *TokenRefreshService) postRefreshStateSyncWithCleanup(parent context.Context, account *Account) {
	cleanupParent := context.Background()
	if parent != nil {
		cleanupParent = context.WithoutCancel(parent)
REDACTED
	ctx, cancel := context.WithTimeout(cleanupParent, defaultTokenRefreshCleanupTimeout)
	defer cancel()
	s.postRefreshStateSync(ctx, account)
REDACTED

func (s *TokenRefreshService) postRefreshStateSync(ctx context.Context, account *Account) {
	// 对所有 OAuth 账号调用缓存失效（InvalidateToken 内部根据平台判断是否需要处理）
	if s.cacheInvalidator != nil && account.Type == AccountTypeOAuth {
		if err := s.cacheInvalidator.InvalidateToken(ctx, account); err != nil {
			slog.Warn("token_refresh.invalidate_token_cache_failed",
				"account_id", account.ID,
				"error", err,
			)
	REDACTED else {
			slog.Debug("token_refresh.token_cache_invalidated", "account_id", account.ID)
	REDACTED
REDACTED
	// 同步更新调度器缓存，确保调度获取的 Account 对象包含最新的 credentials
	if s.schedulerCache != nil {
		if err := s.schedulerCache.SetAccount(ctx, account); err != nil {
			slog.Warn("token_refresh.sync_scheduler_cache_failed",
				"account_id", account.ID,
				"error", err,
			)
	REDACTED else {
			slog.Debug("token_refresh.scheduler_cache_synced", "account_id", account.ID)
	REDACTED
REDACTED
REDACTED

func (s *TokenRefreshService) clearAntigravityForceTokenRefresh(ctx context.Context, account *Account, outcome string) {
	if s == nil || account == nil || !accountNeedsAntigravityForceTokenRefresh(account) {
		return
REDACTED
	updates := clearAntigravityForceTokenRefreshExtra()
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("token_refresh.clear_antigravity_force_refresh_failed",
			"account_id", account.ID,
			"outcome", outcome,
			"error", err,
		)
		return
REDACTED
	if account.Extra != nil {
		for k, v := range updates {
			account.Extra[k] = v
	REDACTED
REDACTED
	slog.Info("token_refresh.cleared_antigravity_force_refresh",
		"account_id", account.ID,
		"outcome", outcome,
	)
REDACTED

// errRefreshSkipped 表示刷新被跳过（锁竞争或已被其他路径刷新），不计入 failed 或 refreshed
var errRefreshSkipped = fmt.Errorf("refresh skipped")

type providerConfigurationRefreshError struct {
	err error
REDACTED

type providerCycleContainmentRefreshError struct {
	err error
REDACTED

type accountPermanentRefreshError struct {
	err                     error
	persistentlyBlocked     bool
	cacheInvalidationFailed bool
REDACTED

type refreshAttemptTimeoutError struct {
	err error
REDACTED

func isProviderScopedTerminalRefreshError(err error) bool {
	if err == nil {
		return false
REDACTED
	var containmentErr *providerCycleContainmentRefreshError
	if errors.As(err, &containmentErr) {
		return true
REDACTED
	var configurationErr *providerConfigurationRefreshError
	return errors.As(err, &configurationErr)
REDACTED

func (e *refreshAttemptTimeoutError) Error() string {
	return "OAuth refresh attempt timed out"
REDACTED

func (e *refreshAttemptTimeoutError) Unwrap() error {
	if e == nil || e.err == nil {
		return context.DeadlineExceeded
REDACTED
	return e.err
REDACTED

func (e *providerConfigurationRefreshError) Error() string {
	return "provider OAuth configuration rejected"
REDACTED

func (e *providerCycleContainmentRefreshError) Error() string {
	return "provider OAuth failure contained for this cycle"
REDACTED

func (e *providerCycleContainmentRefreshError) Unwrap() error {
	if e == nil {
		return nil
REDACTED
	return e.err
REDACTED

func (e *accountPermanentRefreshError) Error() string {
	return "account OAuth credentials permanently rejected"
REDACTED

func (e *accountPermanentRefreshError) Unwrap() error {
	if e == nil {
		return nil
REDACTED
	return e.err
REDACTED

func isAmbiguousGrokEntitlementRefreshError(account *Account, err error) bool {
	if account == nil || !account.IsGrokOAuth() || err == nil {
		return false
REDACTED
	msg := strings.ToLower(err.Error())
	coarseEntitlementLabel := strings.EqualFold(infraerrors.Reason(err), "GROK_OAUTH_ENTITLEMENT_DENIED") ||
		strings.Contains(msg, "grok_oauth_entitlement_denied")
	if !coarseEntitlementLabel {
		return false
REDACTED

	for _, evidence := range []string{
		"subscription required",
		"no active grok subscription",
		"no active subscription",
		"grok subscription required",
		"account is not entitled",
		"not entitled",
		"entitlement required",
		"subscription inactive",
		"subscription expired",
		"upgrade your plan",
REDACTED {
		if strings.Contains(msg, evidence) {
			return false
	REDACTED
REDACTED
	if bodyIndex := strings.Index(msg, "body:"); bodyIndex >= 0 {
		body := msg[bodyIndex+len("body:"):]
		for _, evidence := range []string{
			"entitlement_denied",
			"entitlement denied",
			"subscription_required",
			"no_active_subscription",
	REDACTED {
			if strings.Contains(body, evidence) {
				return false
		REDACTED
	REDACTED
REDACTED
	return true
REDACTED

func (e *providerConfigurationRefreshError) Unwrap() error {
	if e == nil {
		return nil
REDACTED
	return e.err
REDACTED

func isSharedProviderRefreshError(err error) bool {
	if err == nil {
		return false
REDACTED
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"invalid_client",
		"unauthorized_client",
		"invalid_scope",
		"unknown scope",
REDACTED {
		if strings.Contains(msg, needle) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// isNonRetryableRefreshError 判断是否为不可重试的刷新错误
// 这些错误通常表示凭证已失效或配置确实缺失，需要用户重新授权
// 注意：missing_project_id 错误只在真正缺失（从未获取过）时返回，临时获取失败不会返回此错误
func isNonRetryableRefreshError(err error) bool {
	if err == nil {
		return false
REDACTED
	msg := strings.ToLower(err.Error())
	nonRetryable := []string{
		"invalid_grant",             // refresh_token 已失效
		"invalid_refresh_token",     // refresh_token 无效, team 账号工作区被删除会出现
		"token_expired",             // OpenAI refresh_token 已过期，需要重新授权
		"app_session_terminated",    // refresh_token team 账号工作区被删除
		"refresh_token_reused",      // OpenAI refresh_token 已被使用，必须重新授权
		"refresh_token_invalidated", // OpenAI session ended; refresh token invalidated
		"invalid_client",            // 客户端配置错误
		"unauthorized_client",       // 客户端未授权
		"access_denied",             // 访问被拒绝
		"missing_project_id",        // 缺少 project_id
		"no refresh token available",
		"grok_oauth_entitlement_denied",
		"entitlement_denied",
		"invalid_scope",
		"unknown scope",
		"subscription required",
		"no active grok subscription",
REDACTED
	for _, needle := range nonRetryable {
		if strings.Contains(msg, needle) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// ensureOpenAIPrivacy 检查 OpenAI OAuth 账号是否已设置 privacy_mode，
// 未设置则调用 disableOpenAITraining 并持久化结果到 Extra。
func (s *TokenRefreshService) ensureOpenAIPrivacy(ctx context.Context, account *Account) {
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return
REDACTED
	if s.privacyClientFactory == nil {
		return
REDACTED
	if shouldSkipOpenAIPrivacyEnsure(account.Extra) {
		return
REDACTED

	token, _ := account.Credentials["access_token"].(string)
	if token == "" {
		return
REDACTED

	var proxyURL string
	if account.ProxyID != nil && s.proxyRepo != nil {
		if p, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && p != nil {
			proxyURL = p.URL()
	REDACTED
REDACTED

	mode := disableOpenAITraining(ctx, s.privacyClientFactory, token, proxyURL)
	if mode == "" {
		return
REDACTED

	if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{"privacy_mode": modeREDACTED); err != nil {
		slog.Warn("token_refresh.update_privacy_mode_failed",
			"account_id", account.ID,
			"error", err,
		)
REDACTED else {
		slog.Info("token_refresh.privacy_mode_set",
			"account_id", account.ID,
			"privacy_mode", mode,
		)
REDACTED
REDACTED

// ensureAntigravityPrivacy 后台刷新中检查 Antigravity OAuth 账号隐私状态。
// 仅当 privacy_mode 已成功设置（"privacy_set"）时跳过；
// 未设置或之前失败（"privacy_set_failed"）均会重试。
func (s *TokenRefreshService) ensureAntigravityPrivacy(ctx context.Context, account *Account) {
	if account.Platform != PlatformAntigravity || account.Type != AccountTypeOAuth {
		return
REDACTED
	if account.Extra != nil {
		if mode, ok := account.Extra["privacy_mode"].(string); ok && mode == AntigravityPrivacySet {
			return
	REDACTED
REDACTED

	token, _ := account.Credentials["access_token"].(string)
	if token == "" {
		return
REDACTED

	projectID, _ := account.Credentials["project_id"].(string)

	var proxyURL string
	if account.ProxyID != nil && s.proxyRepo != nil {
		if p, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && p != nil {
			proxyURL = p.URL()
	REDACTED
REDACTED

	mode := setAntigravityPrivacy(ctx, token, projectID, proxyURL)
	if mode == "" {
		return
REDACTED

	if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{"privacy_mode": modeREDACTED); err != nil {
		slog.Warn("token_refresh.update_antigravity_privacy_mode_failed",
			"account_id", account.ID,
			"error", err,
		)
REDACTED else {
		applyAntigravityPrivacyMode(account, mode)
		slog.Info("token_refresh.antigravity_privacy_mode_set",
			"account_id", account.ID,
			"privacy_mode", mode,
		)
REDACTED
REDACTED
