package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// Local free-tier soft gate for Grok OAuth scheduling.
//
// Config keys (gateway.grok.*):
//   - free_quota_soft_gate_enabled     (bool, default true)
//   - free_quota_token_limit           (int64, default 500_000)
//   - free_quota_soft_gate_percent     (int, default 95) — stop scheduling before the nominal limit
//   - free_quota_window_hours          (int, default 24) — local usage rolling window
//   - free_quota_stats_cache_seconds   (int, default 5) — bound hot-path aggregate query frequency
//
// Soft-gate applies only to *explicit* free OAuth (subscription_tier/plan_type ==
// "free"). Media/cache free detection uses isKnownGrokFreeAccount instead.
// Admin paths (QueryQuota / import probe) never call this filter.
// Defaults live on config.Gateway.Grok (see config load defaults / tests).

type GrokFreeQuotaPolicy struct {
	Enabled         bool  `json:"enabled"`
	TokenLimit      int64 `json:"token_limit"`
	SoftGatePercent int   `json:"soft_gate_percent"`
	SoftGateTokens  int64 `json:"soft_gate_tokens"`
	WindowHours     int   `json:"window_hours"`
REDACTED

type grokFreeQuotaGateSettings struct {
	limitTokens int64
	gateTokens  int64
	window      time.Duration
	cacheTTL    time.Duration
REDACTED

type grokFreeQuotaGateCacheEntry struct {
	tokens    int64
	checkedAt time.Time
	known     bool
REDACTED

var grokFreeQuotaGateQueryFailureTotal atomic.Int64
var grokFreeQuotaGateBlockedTotal atomic.Int64

func resolveGrokFreeQuotaGateSettings(cfg *config.Config) (grokFreeQuotaGateSettings, bool) {
	if cfg == nil || !cfg.Gateway.Grok.FreeQuotaSoftGateEnabled {
		return grokFreeQuotaGateSettings{REDACTED, false
REDACTED
	limit := cfg.Gateway.Grok.FreeQuotaTokenLimit
	percent := cfg.Gateway.Grok.FreeQuotaSoftGatePercent
	windowHours := cfg.Gateway.Grok.FreeQuotaWindowHours
	cacheSeconds := cfg.Gateway.Grok.FreeQuotaStatsCacheSeconds
	if limit <= 0 || percent < 1 || percent > 100 || windowHours <= 0 || cacheSeconds < 0 {
		return grokFreeQuotaGateSettings{REDACTED, false
REDACTED
	gate := calculateGrokFreeQuotaSoftGateTokens(limit, percent)
	if gate <= 0 {
		return grokFreeQuotaGateSettings{REDACTED, false
REDACTED
	return grokFreeQuotaGateSettings{
		limitTokens: limit,
		gateTokens:  gate,
		window:      time.Duration(windowHours) * time.Hour,
		cacheTTL:    time.Duration(cacheSeconds) * time.Second,
REDACTED, true
REDACTED

func calculateGrokFreeQuotaSoftGateTokens(limit int64, percent int) int64 {
	if limit <= 0 || percent <= 0 {
		return 0
REDACTED
	return (limit/100)*int64(percent) + (limit%100)*int64(percent)/100
REDACTED

// isExplicitGrokFreeOAuthAccount decides whether the free soft-gate applies.
// Contract: only OAuth accounts with credentials/extra
// subscription_tier or plan_type exactly "free" (case-insensitive). Inferred
// free / basic / blank plan do not soft-gate.
func isExplicitGrokFreeOAuthAccount(account *Account) bool {
	if account == nil || !account.IsGrokOAuth() {
		return false
REDACTED
	for _, tier := range []string{
		account.GetCredential("subscription_tier"),
		account.GetCredential("plan_type"),
		account.GetExtraString("subscription_tier"),
		account.GetExtraString("plan_type"),
REDACTED {
		if strings.EqualFold(strings.TrimSpace(tier), "free") {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// filterGrokFreeQuotaAccounts applies a local, rolling soft gate only to
// FREE Grok OAuth accounts on the OpenAI scheduling hot path.
// Missing or failed statistics always fail open; upstream quota/rate-limit
// handling remains authoritative. Admin quota/import probes never call this.
func (s *defaultOpenAIAccountScheduler) filterGrokFreeQuotaAccounts(ctx context.Context, accounts []Account) []Account {
	if s == nil || s.service == nil {
		return accounts
REDACTED
	return filterGrokFreeQuotaAccountsCore(ctx, s.service.cfg, s.service.usageLogRepo, &s.grokFreeQuotaGateCache, accounts)
REDACTED

// filterGrokFreeQuotaAccountsForGateway applies the same soft gate on Gateway
// scheduling (e.g. /v1/web_search) so free accounts near local 95%/1M are not
// still selected for native search while Responses soft-gates them out.
func (s *GatewayService) filterGrokFreeQuotaAccountsForGateway(ctx context.Context, accounts []Account) []Account {
	if s == nil {
		return accounts
REDACTED
	return filterGrokFreeQuotaAccountsCore(ctx, s.cfg, s.usageLogRepo, &gatewayGrokFreeQuotaGateCache, accounts)
REDACTED

// Shared caches for non-advanced-scheduler selection paths.
// Advanced scheduler keeps per-instance sync.Map on defaultOpenAIAccountScheduler.
var gatewayGrokFreeQuotaGateCache sync.Map
var openaiGrokFreeQuotaGateCache sync.Map

func filterGrokFreeQuotaAccountsCore(
	ctx context.Context,
	cfg *config.Config,
	usageLogRepo UsageLogRepository,
	cache *sync.Map,
	accounts []Account,
) []Account {
	if cache == nil {
		return accounts
REDACTED
	settings, enabled := resolveGrokFreeQuotaGateSettings(cfg)
	if !enabled || len(accounts) == 0 || usageLogRepo == nil {
		return accounts
REDACTED
	now := time.Now().UTC()
	tokensByID := make(map[int64]int64)
	missingIDs := make([]int64, 0, len(accounts))
	seenMissing := make(map[int64]struct{REDACTED)
	for i := range accounts {
		account := &accounts[i]
		if !isExplicitGrokFreeOAuthAccount(account) || account.ID <= 0 {
			continue
	REDACTED
		if cached, ok := cache.Load(account.ID); ok {
			entry, valid := cached.(grokFreeQuotaGateCacheEntry)
			age := now.Sub(entry.checkedAt)
			if valid && settings.cacheTTL > 0 && age >= 0 && age < settings.cacheTTL {
				if entry.known {
					tokensByID[account.ID] = entry.tokens
			REDACTED
				continue
		REDACTED
	REDACTED
		if _, exists := seenMissing[account.ID]; !exists {
			seenMissing[account.ID] = struct{REDACTED{REDACTED
			missingIDs = append(missingIDs, account.ID)
	REDACTED
REDACTED

	if len(missingIDs) > 0 {
		statsByID, err := queryGrokFreeQuotaWindowStats(ctx, usageLogRepo, missingIDs, now.Add(-settings.window))
		if err != nil {
			grokFreeQuotaGateQueryFailureTotal.Add(1)
			if settings.cacheTTL > 0 {
				for _, accountID := range missingIDs {
					cache.Store(accountID, grokFreeQuotaGateCacheEntry{checkedAt: nowREDACTED)
			REDACTED
		REDACTED
			slog.Warn("grok_free_quota_soft_gate_stats_failed",
				"account_count", len(missingIDs),
				"window_hours", settings.window.Hours(),
				"error", err)
	REDACTED else {
			for _, accountID := range missingIDs {
				tokens := int64(0)
				if stats := statsByID[accountID]; stats != nil && stats.Tokens > 0 {
					tokens = stats.Tokens
			REDACTED
				tokensByID[accountID] = tokens
				cache.Store(accountID, grokFreeQuotaGateCacheEntry{tokens: tokens, checkedAt: now, known: trueREDACTED)
				if tokens >= settings.gateTokens {
					grokFreeQuotaGateBlockedTotal.Add(1)
					slog.Info("grok_free_quota_soft_gate_blocked",
						"account_id", accountID,
						"tokens", tokens,
						"gate_tokens", settings.gateTokens,
						"limit_tokens", settings.limitTokens,
						"window_hours", settings.window.Hours())
			REDACTED
		REDACTED
	REDACTED
		// Only sweep on the query path: it is already TTL-bounded, so the Range
		// cost stays off the cache-hit hot path.
		sweepGrokFreeQuotaGateCache(cache, now, settings.cacheTTL)
REDACTED

	filtered := make([]Account, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if isExplicitGrokFreeOAuthAccount(account) {
			if tokens, known := tokensByID[account.ID]; known && tokens >= settings.gateTokens {
				continue
		REDACTED
	REDACTED
		filtered = append(filtered, *account)
REDACTED
	return filtered
REDACTED

// grokFreeQuotaGateCacheMinSweepAge floors the eviction age so a tiny cacheTTL
// does not turn the cache into a per-call re-query.
const grokFreeQuotaGateCacheMinSweepAge = 5 * time.Minute

// sweepGrokFreeQuotaGateCache drops entries far past their TTL.
//
// Entries are keyed by account ID and only ever overwritten, so an account that
// stops being scheduled (deleted, or moved off the free tier) would otherwise
// sit in the map for the process lifetime. A still-live account simply
// re-populates its entry on the next miss.
func sweepGrokFreeQuotaGateCache(cache *sync.Map, now time.Time, cacheTTL time.Duration) {
	if cache == nil || cacheTTL <= 0 {
		return
REDACTED
	maxAge := cacheTTL * 20
	if maxAge < grokFreeQuotaGateCacheMinSweepAge {
		maxAge = grokFreeQuotaGateCacheMinSweepAge
REDACTED
	cache.Range(func(key, value any) bool {
		entry, ok := value.(grokFreeQuotaGateCacheEntry)
		if !ok || now.Sub(entry.checkedAt) > maxAge {
			cache.Delete(key)
	REDACTED
		return true
REDACTED)
REDACTED

func queryGrokFreeQuotaWindowStats(ctx context.Context, usageLogRepo UsageLogRepository, accountIDs []int64, start time.Time) (map[int64]*usagestats.AccountStats, error) {
	if usageLogRepo == nil {
		return nil, nil
REDACTED
	if batch, ok := usageLogRepo.(accountWindowStatsBatchReader); ok {
		return batch.GetAccountWindowStatsBatch(ctx, accountIDs, start)
REDACTED
	statsByID := make(map[int64]*usagestats.AccountStats, len(accountIDs))
	for _, accountID := range accountIDs {
		stats, err := usageLogRepo.GetAccountWindowStats(ctx, accountID, start)
		if err != nil {
			return nil, err
	REDACTED
		statsByID[accountID] = stats
REDACTED
	return statsByID, nil
REDACTED
