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
//   - free_quota_stats_cache_seconds   (int, default 60) — stats cache TTL; hot path never waits on DB
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
}

type grokFreeQuotaGateSettings struct {
	limitTokens int64
	gateTokens  int64
	window      time.Duration
	cacheTTL    time.Duration
}

type grokFreeQuotaGateCacheEntry struct {
	tokens    int64
	checkedAt time.Time
	known     bool
}

var grokFreeQuotaGateQueryFailureTotal atomic.Int64
var grokFreeQuotaGateBlockedTotal atomic.Int64

func resolveGrokFreeQuotaGateSettings(cfg *config.Config) (grokFreeQuotaGateSettings, bool) {
	if cfg == nil || !cfg.Gateway.Grok.FreeQuotaSoftGateEnabled {
		return grokFreeQuotaGateSettings{}, false
	}
	limit := cfg.Gateway.Grok.FreeQuotaTokenLimit
	percent := cfg.Gateway.Grok.FreeQuotaSoftGatePercent
	windowHours := cfg.Gateway.Grok.FreeQuotaWindowHours
	cacheSeconds := cfg.Gateway.Grok.FreeQuotaStatsCacheSeconds
	if limit <= 0 || percent < 1 || percent > 100 || windowHours <= 0 || cacheSeconds < 0 {
		return grokFreeQuotaGateSettings{}, false
	}
	gate := calculateGrokFreeQuotaSoftGateTokens(limit, percent)
	if gate <= 0 {
		return grokFreeQuotaGateSettings{}, false
	}
	return grokFreeQuotaGateSettings{
		limitTokens: limit,
		gateTokens:  gate,
		window:      time.Duration(windowHours) * time.Hour,
		cacheTTL:    time.Duration(cacheSeconds) * time.Second,
	}, true
}

func calculateGrokFreeQuotaSoftGateTokens(limit int64, percent int) int64 {
	if limit <= 0 || percent <= 0 {
		return 0
	}
	return (limit/100)*int64(percent) + (limit%100)*int64(percent)/100
}

// isExplicitGrokFreeOAuthAccount decides whether the free soft-gate applies.
// Contract: only OAuth accounts with credentials/extra
// subscription_tier or plan_type exactly "free" (case-insensitive). Inferred
// free / basic / blank plan do not soft-gate.
func isExplicitGrokFreeOAuthAccount(account *Account) bool {
	if account == nil || !account.IsGrokOAuth() {
		return false
	}
	for _, tier := range []string{
		account.GetCredential("subscription_tier"),
		account.GetCredential("plan_type"),
		account.GetExtraString("subscription_tier"),
		account.GetExtraString("plan_type"),
	} {
		if strings.EqualFold(strings.TrimSpace(tier), "free") {
			return true
		}
	}
	return false
}

// filterGrokFreeQuotaAccounts applies a local, rolling soft gate only to
// FREE Grok OAuth accounts on the OpenAI scheduling hot path.
// Missing or failed statistics always fail open; upstream quota/rate-limit
// handling remains authoritative. Admin quota/import probes never call this.
func (s *defaultOpenAIAccountScheduler) filterGrokFreeQuotaAccounts(ctx context.Context, accounts []Account) []Account {
	if s == nil || s.service == nil {
		return accounts
	}
	return filterGrokFreeQuotaAccountsCore(ctx, s.service.cfg, s.service.usageLogRepo, &s.grokFreeQuotaGateCache, accounts)
}

// filterGrokFreeQuotaAccountsForGateway applies the same soft gate on Gateway
// scheduling (e.g. /v1/web_search) so free accounts near local 95%/1M are not
// still selected for native search while Responses soft-gates them out.
func (s *GatewayService) filterGrokFreeQuotaAccountsForGateway(ctx context.Context, accounts []Account) []Account {
	if s == nil {
		return accounts
	}
	return filterGrokFreeQuotaAccountsCore(ctx, s.cfg, s.usageLogRepo, &gatewayGrokFreeQuotaGateCache, accounts)
}

// Shared caches for non-advanced-scheduler selection paths.
// Advanced scheduler keeps per-instance sync.Map on defaultOpenAIAccountScheduler.
var gatewayGrokFreeQuotaGateCache sync.Map
var openaiGrokFreeQuotaGateCache sync.Map

// freeQuotaRefreshInFlight coalesces concurrent background refreshes per cache map.
var freeQuotaRefreshInFlight sync.Map // *sync.Map -> *sync.Map (accountID -> struct{})

func filterGrokFreeQuotaAccountsCore(
	ctx context.Context,
	cfg *config.Config,
	usageLogRepo UsageLogRepository,
	cache *sync.Map,
	accounts []Account,
) []Account {
	if cache == nil {
		return accounts
	}
	settings, enabled := resolveGrokFreeQuotaGateSettings(cfg)
	if !enabled || len(accounts) == 0 || usageLogRepo == nil {
		return accounts
	}
	now := time.Now().UTC()
	tokensByID := make(map[int64]int64)
	missingIDs := make([]int64, 0, len(accounts))
	seenMissing := make(map[int64]struct{})
	for i := range accounts {
		account := &accounts[i]
		if !isExplicitGrokFreeOAuthAccount(account) || account.ID <= 0 {
			continue
		}
		if cached, ok := cache.Load(account.ID); ok {
			entry, valid := cached.(grokFreeQuotaGateCacheEntry)
			if valid {
				age := now.Sub(entry.checkedAt)
				// cacheTTL == 0 means "no expiry" for known entries (still fail-open
				// on first miss; refresh is only scheduled when missing/stale).
				fresh := settings.cacheTTL <= 0 || (age >= 0 && age < settings.cacheTTL)
				if fresh {
					if entry.known {
						tokensByID[account.ID] = entry.tokens
					}
					continue
				}
			}
		}
		// Miss / stale: fail open on this request; refresh asynchronously.
		if _, exists := seenMissing[account.ID]; !exists {
			seenMissing[account.ID] = struct{}{}
			missingIDs = append(missingIDs, account.ID)
		}
	}

	if len(missingIDs) > 0 {
		scheduleGrokFreeQuotaStatsRefresh(usageLogRepo, cache, settings, missingIDs)
	}

	filtered := make([]Account, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if isExplicitGrokFreeOAuthAccount(account) {
			if tokens, known := tokensByID[account.ID]; known && tokens >= settings.gateTokens {
				continue
			}
		}
		filtered = append(filtered, *account)
	}
	return filtered
}

// scheduleGrokFreeQuotaStatsRefresh loads usage stats off the request path.
// Concurrent callers for the same accountID are coalesced via in-flight markers.
func scheduleGrokFreeQuotaStatsRefresh(
	usageLogRepo UsageLogRepository,
	cache *sync.Map,
	settings grokFreeQuotaGateSettings,
	accountIDs []int64,
) {
	if usageLogRepo == nil || cache == nil || len(accountIDs) == 0 {
		return
	}
	inFlightRoot, _ := freeQuotaRefreshInFlight.LoadOrStore(cache, &sync.Map{})
	inFlight, ok := inFlightRoot.(*sync.Map)
	if !ok || inFlight == nil {
		return
	}

	toFetch := make([]int64, 0, len(accountIDs))
	for _, id := range accountIDs {
		if _, loaded := inFlight.LoadOrStore(id, struct{}{}); !loaded {
			toFetch = append(toFetch, id)
		}
	}
	if len(toFetch) == 0 {
		return
	}

	window := settings.window
	gateTokens := settings.gateTokens
	limitTokens := settings.limitTokens
	cacheTTL := settings.cacheTTL
	go func() {
		defer func() {
			for _, id := range toFetch {
				inFlight.Delete(id)
			}
		}()
		now := time.Now().UTC()
		statsByID, err := queryGrokFreeQuotaWindowStats(context.Background(), usageLogRepo, toFetch, now.Add(-window))
		if err != nil {
			grokFreeQuotaGateQueryFailureTotal.Add(1)
			// Store a negative entry so subsequent hot-path calls do not thrash.
			// known=false → still fail open until a successful refresh lands.
			for _, accountID := range toFetch {
				cache.Store(accountID, grokFreeQuotaGateCacheEntry{checkedAt: now})
			}
			slog.Warn("grok_free_quota_soft_gate_stats_failed",
				"account_count", len(toFetch),
				"window_hours", window.Hours(),
				"error", err)
			sweepGrokFreeQuotaGateCache(cache, now, cacheTTL)
			return
		}
		for _, accountID := range toFetch {
			tokens := int64(0)
			if stats := statsByID[accountID]; stats != nil && stats.Tokens > 0 {
				tokens = stats.Tokens
			}
			cache.Store(accountID, grokFreeQuotaGateCacheEntry{tokens: tokens, checkedAt: now, known: true})
			if tokens >= gateTokens {
				grokFreeQuotaGateBlockedTotal.Add(1)
				slog.Info("grok_free_quota_soft_gate_blocked",
					"account_id", accountID,
					"tokens", tokens,
					"gate_tokens", gateTokens,
					"limit_tokens", limitTokens,
					"window_hours", window.Hours())
			}
		}
		sweepGrokFreeQuotaGateCache(cache, now, cacheTTL)
	}()
}

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
	}
	maxAge := cacheTTL * 20
	if maxAge < grokFreeQuotaGateCacheMinSweepAge {
		maxAge = grokFreeQuotaGateCacheMinSweepAge
	}
	cache.Range(func(key, value any) bool {
		entry, ok := value.(grokFreeQuotaGateCacheEntry)
		if !ok || now.Sub(entry.checkedAt) > maxAge {
			cache.Delete(key)
		}
		return true
	})
}

func queryGrokFreeQuotaWindowStats(ctx context.Context, usageLogRepo UsageLogRepository, accountIDs []int64, start time.Time) (map[int64]*usagestats.AccountStats, error) {
	if usageLogRepo == nil {
		return nil, nil
	}
	if batch, ok := usageLogRepo.(accountWindowStatsBatchReader); ok {
		return batch.GetAccountWindowStatsBatch(ctx, accountIDs, start)
	}
	statsByID := make(map[int64]*usagestats.AccountStats, len(accountIDs))
	for _, accountID := range accountIDs {
		stats, err := usageLogRepo.GetAccountWindowStats(ctx, accountID, start)
		if err != nil {
			return nil, err
		}
		statsByID[accountID] = stats
	}
	return statsByID, nil
}
