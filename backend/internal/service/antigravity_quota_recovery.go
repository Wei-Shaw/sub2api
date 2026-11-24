package service

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	antigravityQuotaGroupGemini     = "gemini"
	antigravityQuotaGroupThirdParty = "3p"

	antigravityQuotaRecoveryInterval = 10 * time.Minute
	antigravityQuotaRecoveryTick     = time.Minute
	antigravityQuotaRecoveryTimeout  = 30 * time.Second

	// Limits that reset sooner than this are short rate or capacity cooldowns. They expire by
	// themselves, and lifting them early only earns another 429.
	antigravityQuotaRecoveryMinRemaining = 15 * time.Minute
)

// antigravityQuotaGroupForRateLimitKey maps a model_rate_limits key of an Antigravity account
// to the quota summary group whose windows it is charged against. Gemini image models are
// excluded from the shared Gemini cooldown scope, so they are not assumed to share the Gemini
// group's windows either.
func antigravityQuotaGroupForRateLimitKey(key string) (string, bool) {
	switch strings.TrimSpace(key) {
	case antigravityGeminiModelRateLimitKey, antigravityArgonModelRateLimitKey:
		return antigravityQuotaGroupGemini, true
	}
	model := normalizeAntigravityModelName(key)
	switch {
	case strings.HasPrefix(model, "gemini-"):
		if isAntigravityGeminiImageModel(model) {
			return "", false
		}
		return antigravityQuotaGroupGemini, true
	case strings.HasPrefix(model, "claude-"), strings.HasPrefix(model, "gpt-oss-"):
		return antigravityQuotaGroupThirdParty, true
	}
	return "", false
}

type antigravityActiveModelRateLimit struct {
	key     string
	resetAt time.Time
}

// activeAntigravityModelRateLimits lists, in key order, the model-level limits a quota fetch
// can decide: keys that map to a quota group and reset more than
// antigravityQuotaRecoveryMinRemaining from now. The AICredits marker maps to no group and is
// never listed: it is written for causes a credit balance does not show.
func activeAntigravityModelRateLimits(account *Account, now time.Time) []antigravityActiveModelRateLimit {
	if account == nil || account.Platform != PlatformAntigravity {
		return nil
	}
	rawLimits, _ := account.Extra[modelRateLimitsKey].(map[string]any)
	var limits []antigravityActiveModelRateLimit
	for key := range rawLimits {
		if _, ok := antigravityQuotaGroupForRateLimitKey(key); !ok {
			continue
		}
		resetAt := account.modelRateLimitResetAt(key)
		if resetAt == nil || resetAt.Sub(now) <= antigravityQuotaRecoveryMinRemaining {
			continue
		}
		limits = append(limits, antigravityActiveModelRateLimit{key: key, resetAt: *resetAt})
	}
	sort.Slice(limits, func(i, j int) bool { return limits[i].key < limits[j].key })
	return limits
}

// antigravityQuotaShowsRecovered decides whether a fresh quota result proves that the limit
// stored under key no longer applies. Everything that is missing counts as "still limited":
// the group summary is fetched on a non-critical path, so an absent group or window list says
// nothing about the quota.
func antigravityQuotaShowsRecovered(usage *UsageInfo, key string) bool {
	if usage == nil || usage.IsForbidden || usage.ErrorCode != "" || usage.Error != "" {
		return false
	}
	groupKey, ok := antigravityQuotaGroupForRateLimitKey(key)
	if !ok {
		return false
	}
	var group *AntigravityQuotaGroup
	for _, candidate := range usage.AntigravityQuotaGroups {
		if candidate != nil && candidate.Key == groupKey {
			group = candidate
			break
		}
	}
	if group == nil || len(group.Windows) == 0 {
		return false
	}
	for _, window := range group.Windows {
		if window == nil || window.Utilization >= 100 {
			return false
		}
	}
	// fetchAvailableModels reports the tighter window per model; a model at 100% stays limited
	// whatever its group says.
	if quota, known := usage.AntigravityQuota[strings.TrimSpace(key)]; known && (quota == nil || quota.Utilization >= 100) {
		return false
	}
	return true
}

// recoverAntigravityModelRateLimitsFromQuota lifts the account's model-level limits that a
// fresh quota result shows as recovered, one scope at a time, and returns the cleared keys.
// Account-level rate limits, error status and the schedulable flag are never touched.
//
// The repository clear already enqueues the scheduler outbox event and resyncs the scheduler
// snapshot from the database, so only this account object's own Extra is updated here.
func recoverAntigravityModelRateLimitsFromQuota(ctx context.Context, repo AccountRepository, account *Account, usage *UsageInfo, now time.Time) []string {
	if repo == nil || account == nil || account.ID <= 0 || usage == nil {
		return nil
	}
	var cleared []string
	for _, limit := range activeAntigravityModelRateLimits(account, now) {
		if !antigravityQuotaShowsRecovered(usage, limit.key) {
			continue
		}
		if err := repo.ClearModelRateLimit(ctx, account.ID, limit.key); err != nil {
			slog.Warn("antigravity_quota_recovery_clear_failed",
				"account_id", account.ID,
				"key", limit.key,
				"error", err)
			continue
		}
		clearAccountModelRateLimitSnapshot(account, limit.key)
		slog.Info("antigravity_model_rate_limit_cleared_on_quota_reset",
			"account_id", account.ID,
			"key", limit.key,
			"old_reset_at", limit.resetAt.UTC().Format(time.RFC3339))
		cleared = append(cleared, limit.key)
	}
	return cleared
}

// AntigravityQuotaRecoveryService re-reads the upstream quota of Antigravity accounts that hold
// a long-running model-level rate limit and lifts the limits whose quota has come back, so an
// early upstream reset does not leave the account parked until the stored reset time.
//
// Each tick costs one account list from the database. Upstream is called only for limited
// accounts, each at most once per interval, with first checks spread over the interval by
// account ID.
type AntigravityQuotaRecoveryService struct {
	accountRepo AccountRepository
	proxyRepo   ProxyRepository
	fetcher     *AntigravityQuotaFetcher
	interval    time.Duration
	tick        time.Duration
	// nextDue is only touched by the single worker goroutine (or by a test calling runOnce).
	nextDue  map[int64]time.Time
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewAntigravityQuotaRecoveryService builds the service; interval <= 0 keeps Start from running.
func NewAntigravityQuotaRecoveryService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	fetcher *AntigravityQuotaFetcher,
	interval time.Duration,
) *AntigravityQuotaRecoveryService {
	tick := antigravityQuotaRecoveryTick
	if interval > 0 && interval < tick {
		tick = interval
	}
	return &AntigravityQuotaRecoveryService{
		accountRepo: accountRepo,
		proxyRepo:   proxyRepo,
		fetcher:     fetcher,
		interval:    interval,
		tick:        tick,
		nextDue:     make(map[int64]time.Time),
		stopCh:      make(chan struct{}),
	}
}

func (s *AntigravityQuotaRecoveryService) Start() {
	if s == nil || s.accountRepo == nil || s.fetcher == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.tick)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				s.runOnce(now)
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *AntigravityQuotaRecoveryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

// firstCheckDelay spreads the first check of each account over one interval. It is never zero:
// right after a limit is written the quota is still exhausted.
func (s *AntigravityQuotaRecoveryService) firstCheckDelay(accountID int64) time.Duration {
	slots := int64(s.interval / s.tick)
	if slots < 1 {
		slots = 1
	}
	return time.Duration(accountID%slots+1) * s.tick
}

func (s *AntigravityQuotaRecoveryService) runOnce(now time.Time) {
	listCtx, cancel := context.WithTimeout(context.Background(), antigravityQuotaRecoveryTimeout)
	accounts, err := s.accountRepo.ListByPlatform(listCtx, PlatformAntigravity)
	cancel()
	if err != nil {
		slog.Warn("antigravity_quota_recovery_list_failed", "error", err)
		return
	}

	limited := make(map[int64]struct{})
	for i := range accounts {
		account := &accounts[i]
		if !account.IsActive() || !s.fetcher.CanFetch(account) {
			continue
		}
		if len(activeAntigravityModelRateLimits(account, now)) == 0 {
			continue
		}
		limited[account.ID] = struct{}{}
		due, scheduled := s.nextDue[account.ID]
		if !scheduled {
			due = now.Add(s.firstCheckDelay(account.ID))
			s.nextDue[account.ID] = due
		}
		if now.Before(due) {
			continue
		}
		// Half a tick of slack keeps ticker jitter from pushing the next check one tick late.
		s.nextDue[account.ID] = now.Add(s.interval - s.tick/2)
		s.checkAccount(account, now)
	}
	for id := range s.nextDue {
		if _, ok := limited[id]; !ok {
			delete(s.nextDue, id)
		}
	}
}

func (s *AntigravityQuotaRecoveryService) checkAccount(account *Account, now time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), antigravityQuotaRecoveryTimeout)
	defer cancel()

	proxyURL, ok := s.boundProxyURL(ctx, account)
	if !ok {
		slog.Warn("antigravity_quota_recovery_proxy_unavailable",
			"account_id", account.ID,
			"proxy_id", *account.ProxyID)
		return
	}
	result, err := s.fetcher.FetchQuota(ctx, account, proxyURL)
	if err != nil {
		slog.Warn("antigravity_quota_recovery_fetch_failed", "account_id", account.ID, "error", err)
		return
	}
	if result == nil {
		return
	}
	recoverAntigravityModelRateLimitsFromQuota(ctx, s.accountRepo, account, result.UsageInfo, now)
}

// boundProxyURL returns the account's proxy URL. An account bound to a proxy that cannot be
// loaded reports ok=false: a background check must not reach upstream from the host's own IP.
func (s *AntigravityQuotaRecoveryService) boundProxyURL(ctx context.Context, account *Account) (string, bool) {
	if account.ProxyID == nil {
		return "", true
	}
	if s.proxyRepo == nil {
		return "", false
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil || proxy == nil {
		return "", false
	}
	proxyURL := strings.TrimSpace(proxy.URL())
	return proxyURL, proxyURL != ""
}
