package websearch

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
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
	ProxyID              int64  `json:"-"`                      // resolved proxy ID for unavailability tracking
	ExpiresAt            *int64 `json:"expires_at,omitempty"`   // optional expiration (unix seconds)
}

// Manager selects providers by priority and tracks quota via Redis.
type Manager struct {
	configs []ProviderConfig
	redis   *redis.Client

	clientMu    sync.Mutex
	clientCache map[string]*http.Client
}

// Timeout constants for proxy and search operations.
const (
	proxyDialTimeout     = 3 * time.Second  // proxy TCP connection timeout
	proxyTLSTimeout      = 3 * time.Second  // TLS handshake timeout
	searchDataTimeout    = 60 * time.Second // response data transfer timeout
	searchRequestTimeout = searchDataTimeout + proxyDialTimeout

	quotaKeyPrefix       = "websearch:quota:"
	proxyUnavailableKey  = "websearch:proxy_unavailable:%d"
	proxyUnavailableTTL  = 5 * time.Minute
	quotaTTLBuffer       = 24 * time.Hour
	maxCachedClients     = 100
)

// ErrProxyUnavailable indicates the search failed due to a proxy connectivity issue.
// Callers may use this to trigger account switching instead of direct fallback.
var ErrProxyUnavailable = errors.New("websearch: proxy unavailable")

// quotaIncrScript atomically increments the counter and sets TTL on first creation.
var quotaIncrScript = redis.NewScript(`
local val = redis.call('INCR', KEYS[1])
if val == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
else
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

// SearchWithBestProvider selects a provider using quota-weighted load balancing,
// reserves quota, executes the search, and rolls back quota on failure.
// If the search fails due to a proxy error, the proxy is marked unavailable for 5 minutes.
func (m *Manager) SearchWithBestProvider(ctx context.Context, req SearchRequest) (*SearchResponse, string, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, "", fmt.Errorf("websearch: empty search query")
	}

	candidates := m.filterAvailableProviders(ctx, req.ProxyURL)
	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("websearch: no available provider (all exhausted, expired, or proxy unavailable)")
	}

	selected := m.selectByQuotaWeight(ctx, candidates)

	for _, cfg := range selected {
		allowed, incremented := m.tryReserveQuota(ctx, cfg)
		if !allowed {
			continue
		}
		resp, err := m.executeSearch(ctx, cfg, req)
		if err != nil {
			if incremented {
				m.rollbackQuota(ctx, cfg)
			}
			if isProxyError(err) {
				m.markProxyUnavailable(ctx, cfg, req.ProxyURL)
				slog.Warn("websearch: proxy error, marking unavailable",
					"provider", cfg.Type, "error", err)
				return nil, "", fmt.Errorf("%w: %s", ErrProxyUnavailable, err.Error())
			}
			slog.Warn("websearch: provider search failed",
				"provider", cfg.Type, "error", err)
			continue
		}
		return resp, cfg.Type, nil
	}
	return nil, "", fmt.Errorf("websearch: no available provider (all exhausted or failed)")
}

// filterAvailableProviders returns providers that have API keys, are not expired,
// and whose proxies are not marked unavailable.
func (m *Manager) filterAvailableProviders(ctx context.Context, accountProxyURL string) []ProviderConfig {
	var out []ProviderConfig
	for _, cfg := range m.configs {
		if !m.isProviderAvailable(cfg) {
			continue
		}
		proxyID := resolveProxyID(cfg, accountProxyURL)
		if proxyID > 0 && !m.isProxyAvailable(ctx, proxyID) {
			slog.Debug("websearch: proxy marked unavailable, skipping",
				"provider", cfg.Type, "proxy_id", proxyID)
			continue
		}
		out = append(out, cfg)
	}
	return out
}

// selectByQuotaWeight orders candidates by remaining quota weight.
// Providers with quota_limit=0 (no limit set) get weight 0 and are placed last.
// Among providers with quota, higher remaining quota = higher priority.
func (m *Manager) selectByQuotaWeight(ctx context.Context, candidates []ProviderConfig) []ProviderConfig {
	type weighted struct {
		cfg    ProviderConfig
		weight int64
	}
	items := make([]weighted, 0, len(candidates))
	for _, cfg := range candidates {
		w := int64(0)
		if cfg.QuotaLimit > 0 {
			used, _ := m.GetUsage(ctx, cfg.Type, cfg.QuotaRefreshInterval)
			remaining := cfg.QuotaLimit - used
			if remaining > 0 {
				w = remaining
			}
		}
		items = append(items, weighted{cfg: cfg, weight: w})
	}

	// Separate providers with quota (weight > 0) from those without (weight == 0)
	var withQuota, withoutQuota []weighted
	for _, item := range items {
		if item.weight > 0 {
			withQuota = append(withQuota, item)
		} else {
			withoutQuota = append(withoutQuota, item)
		}
	}

	// Within quota group: weighted random sort (higher remaining = more likely first)
	if len(withQuota) > 1 {
		sort.Slice(withQuota, func(i, j int) bool {
			wi := float64(withQuota[i].weight) * (0.5 + rand.Float64())
			wj := float64(withQuota[j].weight) * (0.5 + rand.Float64())
			return wi > wj
		})
	}

	// Build final order: quota providers first, then no-quota providers (original priority order)
	result := make([]ProviderConfig, 0, len(candidates))
	for _, item := range withQuota {
		result = append(result, item.cfg)
	}
	for _, item := range withoutQuota {
		result = append(result, item.cfg)
	}
	return result
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

// --- Proxy availability tracking ---

// markProxyUnavailable marks the effective proxy as unavailable for proxyUnavailableTTL.
func (m *Manager) markProxyUnavailable(ctx context.Context, cfg ProviderConfig, accountProxyURL string) {
	proxyID := resolveProxyID(cfg, accountProxyURL)
	if proxyID <= 0 || m.redis == nil {
		return
	}
	key := fmt.Sprintf(proxyUnavailableKey, proxyID)
	if err := m.redis.Set(ctx, key, "1", proxyUnavailableTTL).Err(); err != nil {
		slog.Warn("websearch: failed to mark proxy unavailable",
			"proxy_id", proxyID, "error", err)
	}
}

// isProxyAvailable checks whether a proxy is currently marked as unavailable.
func (m *Manager) isProxyAvailable(ctx context.Context, proxyID int64) bool {
	if m.redis == nil || proxyID <= 0 {
		return true
	}
	key := fmt.Sprintf(proxyUnavailableKey, proxyID)
	val, err := m.redis.Get(ctx, key).Result()
	if err != nil {
		return true // Redis error → assume available
	}
	return val == ""
}

// resolveProxyID determines the effective proxy ID for a provider+account combination.
func resolveProxyID(cfg ProviderConfig, accountProxyURL string) int64 {
	if accountProxyURL != "" {
		return 0 // account proxy has no ID in provider config
	}
	return cfg.ProxyID
}

// isProxyError checks whether the error is likely caused by proxy connectivity.
func isProxyError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "proxy") ||
		strings.Contains(msg, "SOCKS") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout")
}

// --- Quota management ---

func (m *Manager) tryReserveQuota(ctx context.Context, cfg ProviderConfig) (bool, bool) {
	if cfg.QuotaLimit <= 0 {
		return true, false
	}
	if m.redis == nil {
		slog.Warn("websearch: Redis unavailable, quota check skipped", "provider", cfg.Type)
		return true, false
	}
	key := quotaRedisKey(cfg.Type, cfg.QuotaRefreshInterval)
	ttlSec := int(quotaTTL(cfg.QuotaRefreshInterval).Seconds())
	newVal, err := quotaIncrScript.Run(ctx, m.redis, []string{key}, ttlSec).Int64()
	if err != nil {
		slog.Warn("websearch: quota Lua INCR failed, allowing request",
			"provider", cfg.Type, "error", err)
		return true, false
	}
	if newVal > cfg.QuotaLimit {
		if decrErr := m.redis.Decr(ctx, key).Err(); decrErr != nil {
			slog.Warn("websearch: quota over-limit DECR failed",
				"provider", cfg.Type, "error", decrErr)
		}
		slog.Info("websearch: provider quota exhausted",
			"provider", cfg.Type, "used", newVal, "limit", cfg.QuotaLimit)
		return false, false
	}
	return true, true
}

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

// --- Search execution ---

func (m *Manager) executeSearch(ctx context.Context, cfg ProviderConfig, req SearchRequest) (*SearchResponse, error) {
	proxyURL := cfg.ProxyURL
	if req.ProxyURL != "" {
		proxyURL = req.ProxyURL
	}
	client, err := m.getOrCreateHTTPClient(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("websearch: %w", err)
	}
	provider := m.buildProvider(cfg, client)
	return provider.Search(ctx, req)
}

// --- HTTP client cache ---

func (m *Manager) getOrCreateHTTPClient(proxyURL string) (*http.Client, error) {
	m.clientMu.Lock()
	defer m.clientMu.Unlock()

	if c, ok := m.clientCache[proxyURL]; ok {
		return c, nil
	}
	if len(m.clientCache) >= maxCachedClients {
		m.clientCache = make(map[string]*http.Client)
	}
	c, err := newHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	m.clientCache[proxyURL] = c
	return c, nil
}

// newHTTPClient creates an HTTP client with proper timeout settings.
// Uses proxyutil.ConfigureTransportProxy for unified proxy protocol support
// (HTTP/HTTPS/SOCKS5/SOCKS5H).
// Returns error if proxyURL is invalid — never falls back to direct connection.
func newHTTPClient(proxyURL string) (*http.Client, error) {
	transport := &http.Transport{
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext:           (&net.Dialer{Timeout: proxyDialTimeout}).DialContext,
		TLSHandshakeTimeout:   proxyTLSTimeout,
		ResponseHeaderTimeout: searchDataTimeout,
	}
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL %q: %w", proxyURL, err)
		}
		if err := proxyutil.ConfigureTransportProxy(transport, parsed); err != nil {
			return nil, fmt.Errorf("configure proxy: %w", err)
		}
	}
	return &http.Client{Transport: transport, Timeout: searchRequestTimeout}, nil
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
	default:
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
