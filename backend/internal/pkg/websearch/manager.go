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

// ProviderConfig holds the configuration for a single search provider.
type ProviderConfig struct {
	Type         string `json:"type"`                    // ProviderTypeBrave | ProviderTypeTavily
	APIKey       string `json:"api_key"`                 // secret
	QuotaLimit   int64  `json:"quota_limit"`             // 0 = unlimited
	SubscribedAt *int64 `json:"subscribed_at,omitempty"` // subscription start (unix seconds); quota resets monthly from this date
	ProxyURL     string `json:"-"`                       // resolved proxy URL (not persisted)
	ProxyID      int64  `json:"-"`                       // resolved proxy ID for unavailability tracking
	ExpiresAt    *int64 `json:"expires_at,omitempty"`    // optional expiration (unix seconds)
REDACTED

// Manager selects providers by quota-weighted load balancing and tracks quota via Redis.
type Manager struct {
	configs []ProviderConfig
	redis   *redis.Client

	clientMu    sync.Mutex
	clientCache map[string]*http.Client
REDACTED

// Timeout constants for proxy and search operations.
const (
	proxyDialTimeout     = 3 * time.Second  // proxy TCP connection timeout
	proxyTLSTimeout      = 3 * time.Second  // TLS handshake timeout
	searchDataTimeout    = 60 * time.Second // response data transfer timeout
	searchRequestTimeout = searchDataTimeout + proxyDialTimeout

	quotaKeyPrefix      = "websearch:quota:"
	proxyUnavailableKey = "websearch:proxy_unavailable:%d"
	proxyUnavailableTTL = 5 * time.Minute
	quotaTTLBuffer      = 24 * time.Hour
	defaultQuotaTTL     = 31*24*time.Hour + quotaTTLBuffer // fallback when no subscription date
	maxCachedClients    = 100
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
// Provider order is preserved as-is; selectByQuotaWeight handles load balancing.
func NewManager(configs []ProviderConfig, redisClient *redis.Client) *Manager {
	copied := make([]ProviderConfig, len(configs))
	copy(copied, configs)
	return &Manager{
		configs:     copied,
		redis:       redisClient,
		clientCache: make(map[string]*http.Client),
REDACTED
REDACTED

// SearchWithBestProvider selects a provider using quota-weighted load balancing,
// reserves quota, executes the search, and rolls back quota on failure.
// If the search fails due to a proxy error, the proxy is marked unavailable for 5 minutes.
func (m *Manager) SearchWithBestProvider(ctx context.Context, req SearchRequest) (*SearchResponse, string, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, "", fmt.Errorf("websearch: empty search query")
REDACTED

	candidates := m.filterAvailableProviders(ctx, req.ProxyURL)
	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("websearch: no available provider (all exhausted, expired, or proxy unavailable)")
REDACTED

	selected := m.selectByQuotaWeight(ctx, candidates)

	for _, cfg := range selected {
		allowed, incremented := m.tryReserveQuota(ctx, cfg)
		if !allowed {
			continue
	REDACTED
		resp, err := m.executeSearch(ctx, cfg, req)
		if err != nil {
			if incremented {
				m.rollbackQuota(ctx, cfg)
		REDACTED
			if isProxyError(err) {
				m.markProxyUnavailable(ctx, cfg, req.ProxyURL)
				slog.Warn("websearch: proxy error, marking unavailable",
					"provider", cfg.Type, "error", err)
				return nil, "", fmt.Errorf("%w: %s", ErrProxyUnavailable, err.Error())
		REDACTED
			slog.Warn("websearch: provider search failed",
				"provider", cfg.Type, "error", err)
			continue
	REDACTED
		return resp, cfg.Type, nil
REDACTED
	return nil, "", fmt.Errorf("websearch: no available provider (all exhausted or failed)")
REDACTED

// filterAvailableProviders returns providers that have API keys, are not expired,
// and whose proxies are not marked unavailable.
func (m *Manager) filterAvailableProviders(ctx context.Context, accountProxyURL string) []ProviderConfig {
	var out []ProviderConfig
	for _, cfg := range m.configs {
		if !m.isProviderAvailable(cfg) {
			continue
	REDACTED
		proxyID := resolveProxyID(cfg, accountProxyURL)
		if proxyID > 0 && !m.isProxyAvailable(ctx, proxyID) {
			slog.Debug("websearch: proxy marked unavailable, skipping",
				"provider", cfg.Type, "proxy_id", proxyID)
			continue
	REDACTED
		out = append(out, cfg)
REDACTED
	return out
REDACTED

// weighted is a provider candidate with computed quota weight.
type weighted struct {
	cfg    ProviderConfig
	weight int64
REDACTED

// selectByQuotaWeight orders candidates by remaining quota weight.
// Providers with quota_limit=0 (no limit set) get weight 0 and are placed last.
// Among providers with quota, higher remaining quota = higher priority.
func (m *Manager) selectByQuotaWeight(ctx context.Context, candidates []ProviderConfig) []ProviderConfig {
	items := m.computeWeights(ctx, candidates)
	withQuota, withoutQuota := partitionByQuota(items)
	sortByStableRandomWeight(withQuota)
	return mergeWeightedResults(withQuota, withoutQuota, len(candidates))
REDACTED

func (m *Manager) computeWeights(ctx context.Context, candidates []ProviderConfig) []weighted {
	items := make([]weighted, 0, len(candidates))
	for _, cfg := range candidates {
		w := int64(0)
		if cfg.QuotaLimit > 0 {
			used, _ := m.GetUsage(ctx, cfg.Type)
			if remaining := cfg.QuotaLimit - used; remaining > 0 {
				w = remaining
		REDACTED
	REDACTED
		items = append(items, weighted{cfg: cfg, weight: wREDACTED)
REDACTED
	return items
REDACTED

func partitionByQuota(items []weighted) (withQuota, withoutQuota []weighted) {
	for _, item := range items {
		if item.weight > 0 {
			withQuota = append(withQuota, item)
	REDACTED else {
			withoutQuota = append(withoutQuota, item)
	REDACTED
REDACTED
	return
REDACTED

// sortByStableRandomWeight assigns a fixed random factor to each item before sorting,
// ensuring deterministic sort behavior (transitivity) within a single call.
func sortByStableRandomWeight(items []weighted) {
	if len(items) <= 1 {
		return
REDACTED
	factors := make([]float64, len(items))
	for i, item := range items {
		factors[i] = float64(item.weight) * (0.5 + rand.Float64())
REDACTED
	sort.Slice(items, func(i, j int) bool {
		return factors[i] > factors[j]
REDACTED)
REDACTED

func mergeWeightedResults(withQuota, withoutQuota []weighted, capacity int) []ProviderConfig {
	result := make([]ProviderConfig, 0, capacity)
	for _, item := range withQuota {
		result = append(result, item.cfg)
REDACTED
	for _, item := range withoutQuota {
		result = append(result, item.cfg)
REDACTED
	return result
REDACTED

func (m *Manager) isProviderAvailable(cfg ProviderConfig) bool {
	if cfg.APIKey == "" {
		return false
REDACTED
	if cfg.ExpiresAt != nil && time.Now().Unix() > *cfg.ExpiresAt {
		slog.Info("websearch: provider expired, skipping",
			"provider", cfg.Type, "expires_at", *cfg.ExpiresAt)
		return false
REDACTED
	return true
REDACTED

// --- Proxy availability tracking ---

// markProxyUnavailable marks the effective proxy as unavailable for proxyUnavailableTTL.
func (m *Manager) markProxyUnavailable(ctx context.Context, cfg ProviderConfig, accountProxyURL string) {
	proxyID := resolveProxyID(cfg, accountProxyURL)
	if proxyID <= 0 || m.redis == nil {
		return
REDACTED
	key := fmt.Sprintf(proxyUnavailableKey, proxyID)
	if err := m.redis.Set(ctx, key, "1", proxyUnavailableTTL).Err(); err != nil {
		slog.Warn("websearch: failed to mark proxy unavailable",
			"proxy_id", proxyID, "error", err)
REDACTED
REDACTED

// isProxyAvailable checks whether a proxy is currently marked as unavailable.
func (m *Manager) isProxyAvailable(ctx context.Context, proxyID int64) bool {
	if m.redis == nil || proxyID <= 0 {
		return true
REDACTED
	key := fmt.Sprintf(proxyUnavailableKey, proxyID)
	val, err := m.redis.Get(ctx, key).Result()
	if err != nil {
		return true // Redis error → assume available
REDACTED
	return val == ""
REDACTED

// resolveProxyID determines the effective proxy ID for a provider+account combination.
func resolveProxyID(cfg ProviderConfig, accountProxyURL string) int64 {
	if accountProxyURL != "" {
		return 0 // account proxy has no ID in provider config
REDACTED
	return cfg.ProxyID
REDACTED

// isProxyError checks whether the error is likely caused by proxy or network connectivity
// (as opposed to an API-level error from the search provider).
func isProxyError(err error) bool {
	if err == nil {
		return false
REDACTED
	// Network-level errors (timeout, connection refused, DNS failure)
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
REDACTED
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
REDACTED
	// TLS handshake failures (often caused by proxy intercepting/blocking)
	var tlsErr *tls.RecordHeaderError
	if errors.As(err, &tlsErr) {
		return true
REDACTED
	// String-based detection for wrapped errors
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "proxy") ||
		strings.Contains(msg, "socks") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "tls handshake") ||
		strings.Contains(msg, "certificate")
REDACTED

// --- Quota management ---

func (m *Manager) tryReserveQuota(ctx context.Context, cfg ProviderConfig) (bool, bool) {
	if cfg.QuotaLimit <= 0 {
		return true, false
REDACTED
	if m.redis == nil {
		slog.Warn("websearch: Redis unavailable, quota check skipped", "provider", cfg.Type)
		return true, false
REDACTED
	key := quotaRedisKey(cfg.Type)
	ttlSec := int(quotaTTLFromSubscription(cfg.SubscribedAt).Seconds())
	newVal, err := quotaIncrScript.Run(ctx, m.redis, []string{keyREDACTED, ttlSec).Int64()
	if err != nil {
		slog.Warn("websearch: quota Lua INCR failed, allowing request",
			"provider", cfg.Type, "error", err)
		return true, false
REDACTED
	if newVal > cfg.QuotaLimit {
		if decrErr := m.redis.Decr(ctx, key).Err(); decrErr != nil {
			slog.Warn("websearch: quota over-limit DECR failed",
				"provider", cfg.Type, "error", decrErr)
	REDACTED
		slog.Info("websearch: provider quota exhausted",
			"provider", cfg.Type, "used", newVal, "limit", cfg.QuotaLimit)
		return false, false
REDACTED
	return true, true
REDACTED

func (m *Manager) rollbackQuota(ctx context.Context, cfg ProviderConfig) {
	if cfg.QuotaLimit <= 0 || m.redis == nil {
		return
REDACTED
	key := quotaRedisKey(cfg.Type)
	if err := m.redis.Decr(ctx, key).Err(); err != nil {
		slog.Warn("websearch: quota rollback DECR failed",
			"provider", cfg.Type, "error", err)
REDACTED
REDACTED

// --- Search execution ---

// TestSearch executes a search using the first available provider without reserving quota.
// Intended for admin test functionality only.
func (m *Manager) TestSearch(ctx context.Context, req SearchRequest) (*SearchResponse, string, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, "", fmt.Errorf("websearch: empty search query")
REDACTED
	for _, cfg := range m.configs {
		if !m.isProviderAvailable(cfg) {
			continue
	REDACTED
		resp, err := m.executeSearch(ctx, cfg, req)
		if err != nil {
			continue
	REDACTED
		return resp, cfg.Type, nil
REDACTED
	return nil, "", fmt.Errorf("websearch: no available provider")
REDACTED

func (m *Manager) executeSearch(ctx context.Context, cfg ProviderConfig, req SearchRequest) (*SearchResponse, error) {
	proxyURL := cfg.ProxyURL
	if req.ProxyURL != "" {
		proxyURL = req.ProxyURL
REDACTED
	client, err := m.getOrCreateHTTPClient(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("websearch: %w", err)
REDACTED
	provider := m.buildProvider(cfg, client)
	return provider.Search(ctx, req)
REDACTED

// --- HTTP client cache ---

func (m *Manager) getOrCreateHTTPClient(proxyURL string) (*http.Client, error) {
	m.clientMu.Lock()
	defer m.clientMu.Unlock()

	if c, ok := m.clientCache[proxyURL]; ok {
		return c, nil
REDACTED
	if len(m.clientCache) >= maxCachedClients {
		m.clientCache = make(map[string]*http.Client)
REDACTED
	c, err := newHTTPClient(proxyURL)
	if err != nil {
		return nil, err
REDACTED
	m.clientCache[proxyURL] = c
	return c, nil
REDACTED

// newHTTPClient creates an HTTP client with proper timeout settings.
// Uses proxyutil.ConfigureTransportProxy for unified proxy protocol support
// (HTTP/HTTPS/SOCKS5/SOCKS5H).
// Returns error if proxyURL is invalid — never falls back to direct connection.
func newHTTPClient(proxyURL string) (*http.Client, error) {
	transport := &http.Transport{
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12REDACTED,
		DialContext:           (&net.Dialer{Timeout: proxyDialTimeoutREDACTED).DialContext,
		TLSHandshakeTimeout:   proxyTLSTimeout,
		ResponseHeaderTimeout: searchDataTimeout,
REDACTED
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL %q: %w", proxyURL, err)
	REDACTED
		if err := proxyutil.ConfigureTransportProxy(transport, parsed); err != nil {
			return nil, fmt.Errorf("configure proxy: %w", err)
	REDACTED
REDACTED
	return &http.Client{Transport: transport, Timeout: searchRequestTimeoutREDACTED, nil
REDACTED

// GetUsage returns the current usage count for the given provider.
func (m *Manager) GetUsage(ctx context.Context, providerType string) (int64, error) {
	if m.redis == nil {
		return 0, nil
REDACTED
	key := quotaRedisKey(providerType)
	val, err := m.redis.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
REDACTED
	return val, err
REDACTED

// GetAllUsage returns usage for every configured provider.
func (m *Manager) GetAllUsage(ctx context.Context) map[string]int64 {
	result := make(map[string]int64, len(m.configs))
	for _, cfg := range m.configs {
		used, _ := m.GetUsage(ctx, cfg.Type)
		result[cfg.Type] = used
REDACTED
	return result
REDACTED

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
REDACTED
REDACTED

// --- Redis key helpers ---

func quotaRedisKey(providerType string) string {
	return quotaKeyPrefix + providerType
REDACTED

// quotaTTLFromSubscription calculates the TTL for the quota counter based on
// the provider's subscription start date. Quota resets monthly from that date.
// When the Redis key expires naturally, the next INCR creates a fresh counter (lazy refresh).
func quotaTTLFromSubscription(subscribedAt *int64) time.Duration {
	if subscribedAt == nil || *subscribedAt == 0 {
		return defaultQuotaTTL
REDACTED
	next := nextMonthlyReset(time.Unix(*subscribedAt, 0).UTC())
	ttl := time.Until(next) + quotaTTLBuffer
	if ttl <= quotaTTLBuffer {
		// Already past the reset — next cycle
		ttl = defaultQuotaTTL
REDACTED
	return ttl
REDACTED

// nextMonthlyReset returns the next monthly reset time based on the subscription start date.
// E.g., subscribed on Jan 15 → resets on Feb 15, Mar 15, etc.
// Handles day-of-month overflow: Jan 31 → Feb 28 (not Mar 3).
func nextMonthlyReset(subscribedAt time.Time) time.Time {
	now := time.Now().UTC()
	if subscribedAt.IsZero() {
		return now.AddDate(0, 1, 0)
REDACTED
	months := (now.Year()-subscribedAt.Year())*12 + int(now.Month()-subscribedAt.Month())
	if months < 0 {
		months = 0
REDACTED
	candidate := addMonthsClamped(subscribedAt, months)
	if candidate.After(now) {
		return candidate
REDACTED
	return addMonthsClamped(subscribedAt, months+1)
REDACTED

// addMonthsClamped adds N months to a date, clamping the day to the last day of the target month.
// E.g., Jan 31 + 1 month = Feb 28 (not Mar 3).
func addMonthsClamped(t time.Time, months int) time.Time {
	y, m, d := t.Date()
	targetMonth := time.Month(int(m) + months)
	targetYear := y + int(targetMonth-1)/12
	targetMonth = (targetMonth-1)%12 + 1
	// Last day of the target month
	lastDay := time.Date(targetYear, targetMonth+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if d > lastDay {
		d = lastDay
REDACTED
	return time.Date(targetYear, targetMonth, d, 0, 0, 0, 0, time.UTC)
REDACTED
