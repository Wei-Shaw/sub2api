package service

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// Local free-tier soft gate for Grok OAuth scheduling.
//
// Config keys (gateway.grok.*):
//   - free_quota_soft_gate_enabled     (bool, default true) — only applied when free-tier detection is strict
//   - free_quota_token_limit           (int64, default 2_000_000) — nominal rolling-window token allowance
//   - free_quota_soft_gate_percent     (int, default 95) — stop scheduling before the nominal limit
//   - free_quota_window_hours          (int, default 24) — local usage rolling window
//   - free_quota_stats_cache_seconds   (int, default 5) — bound hot-path aggregate query frequency
//
// Admin paths (QueryQuota / import probe / AccountUsageService.GetUsage) never call
// filterGrokFreeQuotaAccounts; only the OpenAI-compatible account scheduler filter does.

const (
	defaultGrokFreeQuotaTokenLimit      int64 = 2_000_000
	defaultGrokFreeQuotaSoftGatePercent       = 95
	defaultGrokFreeQuotaWindowHours           = 24
)

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

// isExplicitGrokFreeOAuthAccount is intentionally strict: only OAuth accounts with an
// explicit free marker (subscription_tier / plan_type on credentials or extra) are gated.
// Unknown/empty tier, paid tiers, and API-key accounts fail open (not gated).
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
// explicitly FREE Grok OAuth accounts on the scheduling hot path.
// Missing or failed statistics always fail open; upstream quota/rate-limit
// handling remains authoritative. Admin quota/import probes never call this.
func (s *defaultOpenAIAccountScheduler) filterGrokFreeQuotaAccounts(ctx context.Context, accounts []Account) []Account {
	if s == nil || s.service == nil {
		return accounts
REDACTED
	settings, enabled := resolveGrokFreeQuotaGateSettings(s.service.cfg)
	if !enabled || len(accounts) == 0 || s.service.usageLogRepo == nil {
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
		if cached, ok := s.grokFreeQuotaGateCache.Load(account.ID); ok {
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
		statsByID, err := s.queryGrokFreeQuotaWindowStats(ctx, missingIDs, now.Add(-settings.window))
		if err != nil {
			grokFreeQuotaGateQueryFailureTotal.Add(1)
			if settings.cacheTTL > 0 {
				for _, accountID := range missingIDs {
					s.grokFreeQuotaGateCache.Store(accountID, grokFreeQuotaGateCacheEntry{checkedAt: nowREDACTED)
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
				s.grokFreeQuotaGateCache.Store(accountID, grokFreeQuotaGateCacheEntry{tokens: tokens, checkedAt: now, known: trueREDACTED)
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

func (s *defaultOpenAIAccountScheduler) queryGrokFreeQuotaWindowStats(ctx context.Context, accountIDs []int64, start time.Time) (map[int64]*usagestats.AccountStats, error) {
	if batch, ok := s.service.usageLogRepo.(accountWindowStatsBatchReader); ok {
		return batch.GetAccountWindowStatsBatch(ctx, accountIDs, start)
REDACTED
	statsByID := make(map[int64]*usagestats.AccountStats, len(accountIDs))
	for _, accountID := range accountIDs {
		stats, err := s.service.usageLogRepo.GetAccountWindowStats(ctx, accountID, start)
		if err != nil {
			return nil, err
	REDACTED
		statsByID[accountID] = stats
REDACTED
	return statsByID, nil
REDACTED
