package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"golang.org/x/sync/singleflight"
)

// WebSearchEmulationConfig holds the global web search emulation configuration.
type WebSearchEmulationConfig struct {
	Enabled   bool                      `json:"enabled"`
	Providers []WebSearchProviderConfig `json:"providers"`
REDACTED

// WebSearchProviderConfig describes a single search provider (Brave or Tavily).
type WebSearchProviderConfig struct {
	Type             string `json:"type"`                    // websearch.ProviderTypeBrave | Tavily
	APIKey           string `json:"api_key,omitempty"`       // secret — omitted in API responses
	APIKeyConfigured bool   `json:"api_key_configured"`      // read-only mask
	QuotaLimit       *int64 `json:"quota_limit"`             // nil = unlimited, >0 = limited
	SubscribedAt     *int64 `json:"subscribed_at,omitempty"` // subscription start (unix seconds); quota resets monthly
	QuotaUsed        int64  `json:"quota_used,omitempty"`    // read-only: current usage from Redis
	ProxyID          *int64 `json:"proxy_id"`                // optional proxy association
	ExpiresAt        *int64 `json:"expires_at,omitempty"`    // optional expiration timestamp
REDACTED

// --- Validation ---

const maxWebSearchProviders = 10

var validProviderTypes = map[string]bool{
	websearch.ProviderTypeBrave:  true,
	websearch.ProviderTypeTavily: true,
REDACTED

func validateWebSearchConfig(cfg *WebSearchEmulationConfig) error {
	if cfg == nil {
		return nil
REDACTED
	if len(cfg.Providers) > maxWebSearchProviders {
		return fmt.Errorf("too many providers (max %d)", maxWebSearchProviders)
REDACTED
	seen := make(map[string]bool, len(cfg.Providers))
	for i, p := range cfg.Providers {
		if !validProviderTypes[p.Type] {
			return fmt.Errorf("provider[%d]: invalid type %q", i, p.Type)
	REDACTED
		if p.QuotaLimit != nil && *p.QuotaLimit < 0 {
			return fmt.Errorf("provider[%d]: quota_limit must be > 0 or null", i)
	REDACTED
		if seen[p.Type] {
			return fmt.Errorf("provider[%d]: duplicate type %q", i, p.Type)
	REDACTED
		seen[p.Type] = true
REDACTED
	return nil
REDACTED

// --- In-process cache (same pattern as gateway forwarding settings) ---

const sfKeyWebSearchConfig = "web_search_emulation_config"

type cachedWebSearchEmulationConfig struct {
	config    *WebSearchEmulationConfig
	expiresAt int64 // unix nano
REDACTED

var webSearchEmulationCache atomic.Value // *cachedWebSearchEmulationConfig
var webSearchEmulationSF singleflight.Group

const (
	webSearchEmulationCacheTTL  = 60 * time.Second
	webSearchEmulationErrorTTL  = 5 * time.Second
	webSearchEmulationDBTimeout = 5 * time.Second
)

// GetWebSearchEmulationConfig returns the configuration with in-process cache + singleflight.
func (s *SettingService) GetWebSearchEmulationConfig(ctx context.Context) (*WebSearchEmulationConfig, error) {
	if cached := webSearchEmulationCache.Load(); cached != nil {
		if c, ok := cached.(*cachedWebSearchEmulationConfig); ok && time.Now().UnixNano() < c.expiresAt {
			return c.config, nil
	REDACTED
REDACTED
	result, err, _ := webSearchEmulationSF.Do(sfKeyWebSearchConfig, func() (any, error) {
		return s.loadWebSearchConfigFromDB()
REDACTED)
	if err != nil {
		return &WebSearchEmulationConfig{REDACTED, err
REDACTED
	if cfg, ok := result.(*WebSearchEmulationConfig); ok {
		return cfg, nil
REDACTED
	return &WebSearchEmulationConfig{REDACTED, nil
REDACTED

func (s *SettingService) loadWebSearchConfigFromDB() (*WebSearchEmulationConfig, error) {
	dbCtx, cancel := context.WithTimeout(context.Background(), webSearchEmulationDBTimeout)
	defer cancel()

	raw, err := s.settingRepo.GetValue(dbCtx, SettingKeyWebSearchEmulationConfig)
	if err != nil {
		// Missing key is the normal first-boot state: return empty disabled config.
		if errors.Is(err, ErrSettingNotFound) {
			cfg := &WebSearchEmulationConfig{REDACTED
			webSearchEmulationCache.Store(&cachedWebSearchEmulationConfig{
				config:    cfg,
				expiresAt: time.Now().Add(webSearchEmulationCacheTTL).UnixNano(),
		REDACTED)
			return cfg, nil
	REDACTED
		webSearchEmulationCache.Store(&cachedWebSearchEmulationConfig{
			config:    &WebSearchEmulationConfig{REDACTED,
			expiresAt: time.Now().Add(webSearchEmulationErrorTTL).UnixNano(),
	REDACTED)
		return &WebSearchEmulationConfig{REDACTED, err
REDACTED
	cfg := parseWebSearchConfigJSON(raw)
	webSearchEmulationCache.Store(&cachedWebSearchEmulationConfig{
		config:    cfg,
		expiresAt: time.Now().Add(webSearchEmulationCacheTTL).UnixNano(),
REDACTED)
	return cfg, nil
REDACTED

func parseWebSearchConfigJSON(raw string) *WebSearchEmulationConfig {
	cfg := &WebSearchEmulationConfig{REDACTED
	if raw == "" {
		return cfg
REDACTED
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		slog.Warn("websearch: failed to parse config JSON", "error", err)
		return &WebSearchEmulationConfig{REDACTED
REDACTED
	return cfg
REDACTED

// SaveWebSearchEmulationConfig validates and persists the configuration.
// Empty API keys in the input are preserved from the existing config.
func (s *SettingService) SaveWebSearchEmulationConfig(ctx context.Context, cfg *WebSearchEmulationConfig) error {
	if err := validateWebSearchConfig(cfg); err != nil {
		return infraerrors.BadRequest("INVALID_WEB_SEARCH_CONFIG", err.Error())
REDACTED
	s.mergeExistingAPIKeys(ctx, cfg)

	// After merge, validate all enabled providers have API keys
	if cfg.Enabled {
		for _, p := range cfg.Providers {
			if p.APIKey == "" {
				return infraerrors.BadRequest("MISSING_API_KEY",
					fmt.Sprintf("provider %s has no API key configured", p.Type))
		REDACTED
	REDACTED
REDACTED

	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("websearch: marshal config: %w", err)
REDACTED
	if err := s.settingRepo.Set(ctx, SettingKeyWebSearchEmulationConfig, string(data)); err != nil {
		return fmt.Errorf("websearch: save config: %w", err)
REDACTED
	// Invalidate: forget singleflight first, then store new value
	webSearchEmulationSF.Forget(sfKeyWebSearchConfig)
	webSearchEmulationCache.Store(&cachedWebSearchEmulationConfig{
		config:    cfg,
		expiresAt: time.Now().Add(webSearchEmulationCacheTTL).UnixNano(),
REDACTED)

	// Hot-reload: rebuild the global Manager with new config
	s.rebuildWebSearchManager(ctx)
	return nil
REDACTED

// mergeExistingAPIKeys preserves API keys from the current config when incoming value is empty.
func (s *SettingService) mergeExistingAPIKeys(ctx context.Context, cfg *WebSearchEmulationConfig) {
	existing, _ := s.getWebSearchEmulationConfigRaw(ctx)
	if existing == nil || cfg == nil {
		return
REDACTED
	existingByType := make(map[string]string, len(existing.Providers))
	for _, p := range existing.Providers {
		if p.APIKey != "" {
			existingByType[p.Type] = p.APIKey
	REDACTED
REDACTED
	for i := range cfg.Providers {
		if cfg.Providers[i].APIKey == "" {
			if key, ok := existingByType[cfg.Providers[i].Type]; ok {
				cfg.Providers[i].APIKey = key
		REDACTED
	REDACTED
REDACTED
REDACTED

func (s *SettingService) getWebSearchEmulationConfigRaw(ctx context.Context) (*WebSearchEmulationConfig, error) {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyWebSearchEmulationConfig)
	if err != nil {
		return nil, err
REDACTED
	return parseWebSearchConfigJSON(raw), nil
REDACTED

// IsWebSearchEmulationEnabled is a quick check for whether the global switch is on.
func (s *SettingService) IsWebSearchEmulationEnabled(ctx context.Context) bool {
	cfg, err := s.GetWebSearchEmulationConfig(ctx)
	if err != nil {
		return false
REDACTED
	return cfg.Enabled && len(cfg.Providers) > 0
REDACTED

// SetWebSearchManagerBuilder injects a callback that creates and wires a websearch.Manager.
// The infra layer (main/wire) provides this builder, keeping redis out of the service layer.
// Triggers initial build.
func (s *SettingService) SetWebSearchManagerBuilder(ctx context.Context, builder WebSearchManagerBuilder) {
	s.webSearchManagerBuilder = builder
	s.rebuildWebSearchManager(ctx)
REDACTED

// rebuildWebSearchManager reads the current config, resolves proxy URLs, and invokes the builder.
func (s *SettingService) rebuildWebSearchManager(ctx context.Context) {
	if s.webSearchManagerBuilder == nil {
		return
REDACTED
	cfg, err := s.GetWebSearchEmulationConfig(ctx)
	if err != nil {
		SetWebSearchManager(nil)
		return
REDACTED
	proxyURLs := s.resolveProviderProxyURLs(ctx, cfg)
	s.webSearchManagerBuilder(cfg, proxyURLs)
REDACTED

// resolveProviderProxyURLs collects proxy IDs from providers and resolves them to URLs.
func (s *SettingService) resolveProviderProxyURLs(ctx context.Context, cfg *WebSearchEmulationConfig) map[int64]string {
	if cfg == nil || s.proxyRepo == nil {
		return nil
REDACTED
	var ids []int64
	for _, p := range cfg.Providers {
		if p.ProxyID != nil && *p.ProxyID > 0 {
			ids = append(ids, *p.ProxyID)
	REDACTED
REDACTED
	if len(ids) == 0 {
		return nil
REDACTED
	proxies, err := s.proxyRepo.ListByIDs(ctx, ids)
	if err != nil {
		slog.Warn("websearch: failed to resolve proxy URLs", "error", err)
		return nil
REDACTED
	result := make(map[int64]string, len(proxies))
	for _, px := range proxies {
		result[px.ID] = px.URL()
REDACTED
	return result
REDACTED

// WebSearchTestResult holds the result of a search test.
type WebSearchTestResult struct {
	Provider string                   `json:"provider"`
	Results  []websearch.SearchResult `json:"results"`
	Query    string                   `json:"query"`
REDACTED

// TestWebSearch executes a test search using the currently configured Manager.
// Uses Manager.TestSearch which bypasses quota tracking.
const testSearchTimeout = 15 * time.Second

func TestWebSearch(ctx context.Context, query string) (*WebSearchTestResult, error) {
	mgr := getWebSearchManager()
	if mgr == nil {
		return nil, fmt.Errorf("web search: manager not initialized, save config first")
REDACTED
	testCtx, cancel := context.WithTimeout(ctx, testSearchTimeout)
	defer cancel()
	resp, providerName, err := mgr.TestSearch(testCtx, websearch.SearchRequest{
		Query:      query,
		MaxResults: webSearchDefaultMaxResults,
REDACTED)
	if err != nil {
		return nil, err
REDACTED
	return &WebSearchTestResult{
		Provider: providerName,
		Results:  resp.Results,
		Query:    resp.Query,
REDACTED, nil
REDACTED

// PopulateWebSearchUsage returns a copy with quota usage populated from Redis (api_key kept as-is).
func PopulateWebSearchUsage(ctx context.Context, cfg *WebSearchEmulationConfig) *WebSearchEmulationConfig {
	if cfg == nil {
		return nil
REDACTED
	out := *cfg
	out.Providers = make([]WebSearchProviderConfig, len(cfg.Providers))

	mgr := getWebSearchManager()

	for i, p := range cfg.Providers {
		out.Providers[i] = p
		out.Providers[i].APIKeyConfigured = p.APIKey != ""

		if mgr != nil {
			used, _ := mgr.GetUsage(ctx, p.Type)
			out.Providers[i].QuotaUsed = used
	REDACTED
REDACTED
	return &out
REDACTED

// ResetWebSearchUsage deletes the Redis quota key for the given provider type.
func ResetWebSearchUsage(ctx context.Context, providerType string) error {
	mgr := getWebSearchManager()
	if mgr == nil {
		return fmt.Errorf("web search manager not initialized")
REDACTED
	return mgr.ResetUsage(ctx, providerType)
REDACTED

// SanitizeWebSearchConfig returns a copy with api_key fields masked and quota usage populated.
func SanitizeWebSearchConfig(ctx context.Context, cfg *WebSearchEmulationConfig) *WebSearchEmulationConfig {
	if cfg == nil {
		return nil
REDACTED
	out := *cfg
	out.Providers = make([]WebSearchProviderConfig, len(cfg.Providers))

	// Load usage from the global Manager (reads from Redis)
	mgr := getWebSearchManager()

	for i, p := range cfg.Providers {
		out.Providers[i] = p
		out.Providers[i].APIKeyConfigured = p.APIKey != ""
		out.Providers[i].APIKey = "" // never return the secret

		// Populate quota usage from Redis
		if mgr != nil {
			used, _ := mgr.GetUsage(ctx, p.Type)
			out.Providers[i].QuotaUsed = used
	REDACTED
REDACTED
	return &out
REDACTED
