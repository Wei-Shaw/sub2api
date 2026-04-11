package websearch

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Quota refresh interval constants.
const (
	QuotaRefreshDaily   = "daily"
	QuotaRefreshWeekly  = "weekly"
	QuotaRefreshMonthly = "monthly"
)

// ProviderConfig holds the configuration for a single search provider.
type ProviderConfig struct {
	Type                 string `json:"type"`                   // ProviderTypeBrave | ProviderTypeTavily
	APIKey               string `json:"api_key"`                // secret
	Priority             int    `json:"priority"`               // lower = higher priority
	QuotaLimit           int64  `json:"quota_limit"`            // 0 = unlimited
	QuotaRefreshInterval string `json:"quota_refresh_interval"` // QuotaRefreshDaily / Weekly / Monthly
	ProxyURL             string `json:"-"`                      // resolved proxy URL (not persisted)
	ExpiresAt            *int64 `json:"expires_at,omitempty"`   // optional expiration (unix seconds)
}

// Manager selects providers by priority and tracks quota via Redis.
type Manager struct {
	configs []ProviderConfig
	redis   *redis.Client

	clientMu    sync.Mutex
	clientCache map[string]*http.Client
}

const (
	quotaKeyPrefix       = "websearch:quota:"
	searchRequestTimeout = 30 * time.Second
	quotaTTLBuffer       = 24 * time.Hour
	maxCachedClients     = 100
)

// quotaIncrScript atomically increments the counter and sets TTL on first creation.
// KEYS[1] = quota key, ARGV[1] = TTL in seconds.
// Returns the new counter value.
var quotaIncrScript = redis.NewScript(`
local val = redis.call('INCR', KEYS[1])
if val == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
else
  -- Defensive: ensure TTL exists even if a prior EXPIRE failed
  local ttl = redis.call('TTL', KEYS[1])
  if ttl == -1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
  end
end
return val
`)

// NewManager creates a Manager with the given provider configs and Redis client.
func NewManager(configs []ProviderConfig, redisClient *redis.Client) *Manager {
	sorted := make([]ProviderConfig, len(configs))
	copy(sorted, configs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})
	return &Manager{
		configs:     sorted,
		redis:       redisClient,
		clientCache: make(map[string]*http.Client),
	}
}

// SearchWithBestProvider selects the highest-priority available provider,
// reserves quota, executes the search, and rolls back quota on failure.
func (m *Manager) SearchWithBestProvider(ctx context.Context, req SearchRequest) (*SearchResponse, string, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, "", fmt.Errorf("websearch: empty search query")
	}
	for _, cfg := range m.configs {
		if !m.isProviderAvailable(cfg) {
			continue
		}
		allowed, incremented := m.tryReserveQuota(ctx, cfg)
		if !allowed {
			continue
		}
		resp, err := m.executeSearch(ctx, cfg, req)
		if err != nil {
			if incremented {
				m.rollbackQuota(ctx, cfg)
			}
			slog.Warn("websearch: provider search failed",
				"provider", cfg.Type, "error", err)
			continue
		}
		return resp, cfg.Type, nil
	}
	return nil, "", fmt.Errorf("websearch: no available provider (all exhausted or failed)")
}

func (m *Manager) isProviderAvailable(cfg ProviderConfig) bool {
	if cfg.APIKey == "" {
		return false
	}
	if cfg.ExpiresAt != nil && time.Now().Unix() > *cfg.ExpiresAt {
		slog.Info("websearch: provider expired, skipping",
			"provider", cfg.Type, "expires_at", *cfg.ExpiresAt)
		return false
	}
	return true
}

// tryReserveQuota atomically increments the counter via Lua script and checks limit.
// Returns (allowed, incremented): allowed=true means the request may proceed;
// incremented=true means the Redis counter was actually incremented (so rollback is needed on failure).
func (m *Manager) tryReserveQuota(ctx context.Context, cfg ProviderConfig) (bool, bool) {
	if cfg.QuotaLimit <= 0 {
		return true, false // unlimited, no INCR
	}
	if m.redis == nil {
		slog.Warn("websearch: Redis unavailable, quota check skipped",
			"provider", cfg.Type)
		return true, false // allowed but not incremented
	}
	key := quotaRedisKey(cfg.Type, cfg.QuotaRefreshInterval)
	ttlSec := int(quotaTTL(cfg.QuotaRefreshInterval).Seconds())

	newVal, err := quotaIncrScript.Run(ctx, m.redis, []string{key}, ttlSec).Int64()
	if err != nil {
		slog.Warn("websearch: quota Lua INCR failed, allowing request",
			"provider", cfg.Type, "error", err)
		return true, false // allowed but not incremented
	}
	if newVal > cfg.QuotaLimit {
		if decrErr := m.redis.Decr(ctx, key).Err(); decrErr != nil {
			slog.Warn("websearch: quota over-limit DECR failed",
				"provider", cfg.Type, "error", decrErr)
		}
		slog.Info("websearch: provider quota exhausted",
			"provider", cfg.Type, "used", newVal, "limit", cfg.QuotaLimit)
		return false, false // rejected, already rolled back
	}
	return true, true // allowed and incremented
}

// rollbackQuota decrements the counter after a search failure.
func (m *Manager) rollbackQuota(ctx context.Context, cfg ProviderConfig) {
	if cfg.QuotaLimit <= 0 || m.redis == nil {
		return
	}
	key := quotaRedisKey(cfg.Type, cfg.QuotaRefreshInterval)
	if err := m.redis.Decr(ctx, key).Err(); err != nil {
		slog.Warn("websearch: quota rollback DECR failed",
			"provider", cfg.Type, "error", err)
	}
}

func (m *Manager) executeSearch(ctx context.Context, cfg ProviderConfig, req SearchRequest) (*SearchResponse, error) {
	proxyURL := cfg.ProxyURL
	if req.ProxyURL != "" {
		proxyURL = req.ProxyURL
	}
	client := m.getOrCreateHTTPClient(proxyURL)
	provider := m.buildProvider(cfg, client)
	return provider.Search(ctx, req)
}

// GetUsage returns the current usage count for the given provider.
func (m *Manager) GetUsage(ctx context.Context, providerType, refreshInterval string) (int64, error) {
	if m.redis == nil {
		return 0, nil
	}
	key := quotaRedisKey(providerType, refreshInterval)
	val, err := m.redis.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

// GetAllUsage returns usage for every configured provider.
func (m *Manager) GetAllUsage(ctx context.Context) map[string]int64 {
	result := make(map[string]int64, len(m.configs))
	for _, cfg := range m.configs {
		used, _ := m.GetUsage(ctx, cfg.Type, cfg.QuotaRefreshInterval)
		result[cfg.Type] = used
	}
	return result
}

// --- HTTP client cache (bounded) ---

func (m *Manager) getOrCreateHTTPClient(proxyURL string) *http.Client {
	m.clientMu.Lock()
	defer m.clientMu.Unlock()

	if c, ok := m.clientCache[proxyURL]; ok {
		return c
	}
	if len(m.clientCache) >= maxCachedClients {
		m.clientCache = make(map[string]*http.Client) // evict all
	}
	c := newHTTPClient(proxyURL)
	m.clientCache[proxyURL] = c
	return c
}

func newHTTPClient(proxyURL string) *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Transport: transport, Timeout: searchRequestTimeout}
}

// --- Provider factory ---

func (m *Manager) buildProvider(cfg ProviderConfig, client *http.Client) Provider {
	switch cfg.Type {
	case braveProviderName:
		return NewBraveProvider(cfg.APIKey, client)
	case tavilyProviderName:
		return NewTavilyProvider(cfg.APIKey, client)
	default:
		slog.Warn("websearch: unknown provider type, falling back to brave",
			"type", cfg.Type)
		return NewBraveProvider(cfg.APIKey, client)
	}
}

// --- Redis key helpers ---

func quotaRedisKey(providerType, refreshInterval string) string {
	return quotaKeyPrefix + providerType + ":" + periodKey(refreshInterval)
}

func periodKey(refreshInterval string) string {
	now := time.Now().UTC()
	switch refreshInterval {
	case QuotaRefreshDaily:
		return now.Format("2006-01-02")
	case QuotaRefreshWeekly:
		year, week := now.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	default: // QuotaRefreshMonthly
		return now.Format("2006-01")
	}
}

func quotaTTL(refreshInterval string) time.Duration {
	switch refreshInterval {
	case QuotaRefreshDaily:
		return 24*time.Hour + quotaTTLBuffer
	case QuotaRefreshWeekly:
		return 7*24*time.Hour + quotaTTLBuffer
	default:
		return 31*24*time.Hour + quotaTTLBuffer
	}
}
