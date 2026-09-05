package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/tidwall/gjson"
)

const grokPassthroughModelsCacheTTL = 5 * time.Minute

type GrokPassthroughModelsResolution struct {
	RawData     []any
	FallbackIDs []string
	Enabled     bool
}

func grokPassthroughModelsCacheKey(groupID *int64) string {
	return "grok-pt-catalog|" + modelsListCacheKey(groupID, PlatformGrok)
}

// ResolveGrokPassthroughModels returns the live upstream /models catalog for a
// Grok passthrough group. Enabled is false when the group has no passthrough
// account. RawData is the upstream data[] when the fetch succeeds; otherwise
// FallbackIDs holds observed IDs (or empty so the handler can use defaults).
func (s *GatewayService) ResolveGrokPassthroughModels(ctx context.Context, groupID *int64) GrokPassthroughModelsResolution {
	if s == nil {
		return GrokPassthroughModelsResolution{}
	}
	accounts, err := s.listGrokPassthroughCatalogAccounts(ctx, groupID)
	if err != nil {
		return GrokPassthroughModelsResolution{}
	}
	account := firstGrokPassthroughAccount(accounts)
	if account == nil {
		return GrokPassthroughModelsResolution{}
	}

	cacheKey := grokPassthroughModelsCacheKey(groupID)
	if s.modelsListCache != nil {
		if cached, found := s.modelsListCache.Get(cacheKey); found {
			if resolution, ok := cached.(GrokPassthroughModelsResolution); ok {
				return resolution
			}
		}
	}

	resolution := GrokPassthroughModelsResolution{Enabled: true}
	if raw := s.fetchGrokUpstreamModelsCatalog(ctx, account); len(raw) > 0 {
		resolution.RawData = raw
	} else if snap := parseGrokObservedModels(account.Extra); snap != nil {
		resolution.FallbackIDs = append([]string(nil), snap.Models...)
	}

	if s.modelsListCache != nil {
		s.modelsListCache.Set(cacheKey, resolution, grokPassthroughModelsCacheTTL)
	}
	return resolution
}

func FilterGrokPassthroughCatalog(items []any, allowedIDs []string) []any {
	if len(items) == 0 || len(allowedIDs) == 0 {
		return items
	}
	allow := make(map[string]struct{}, len(allowedIDs))
	for _, id := range allowedIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		allow[id] = struct{}{}
	}
	if len(allow) == 0 {
		return items
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		id := grokPassthroughCatalogModelID(item)
		if id == "" {
			continue
		}
		if _, ok := allow[id]; ok {
			out = append(out, item)
		}
	}
	return out
}

func (s *GatewayService) listGrokPassthroughCatalogAccounts(ctx context.Context, groupID *int64) ([]Account, error) {
	if s.accountRepo == nil {
		return nil, nil
	}
	if groupID != nil {
		return s.accountRepo.ListSchedulableByGroupID(ctx, *groupID)
	}
	return s.accountRepo.ListSchedulable(ctx)
}

func firstGrokPassthroughAccount(accounts []Account) *Account {
	for i := range accounts {
		if accounts[i].IsGrokPassthroughEnabled() {
			return &accounts[i]
		}
	}
	return nil
}

func (s *GatewayService) fetchGrokUpstreamModelsCatalog(ctx context.Context, account *Account) []any {
	if s == nil || s.httpUpstream == nil || account == nil {
		return nil
	}
	fetchCtx, cancel := context.WithTimeout(ctx, grokObservedModelsTimeout)
	defer cancel()

	token, _, err := s.GetAccessToken(fetchCtx, account)
	if err != nil || strings.TrimSpace(token) == "" {
		return nil
	}
	baseURL := strings.TrimSpace(account.GetGrokBaseURL())
	if s.settingService != nil {
		baseURL = strings.TrimSpace(s.settingService.ResolveGrokBaseURL(fetchCtx, account))
	}
	if baseURL == "" {
		baseURL = xai.DefaultCLIBaseURL
	}
	validator, err := grokBaseURLValidator(account, s.cfg)
	if err != nil {
		return nil
	}
	validatedBaseURL, err := validator(baseURL)
	if err != nil {
		return nil
	}
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, buildOpenAIModelsURL(validatedBaseURL), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", defaultGrokUpstreamUserAgent())
	if account.IsGrokOAuth() {
		applyGrokCLIHeaders(req.Header)
		if isGrokCLIProxyTarget(req.URL.String()) {
			xai.EnsureCLIProxyAuthHeaders(req.Header)
		}
	}
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode >= 400 {
		return nil
	}
	return parseGrokUpstreamModelsCatalog(body)
}

func parseGrokUpstreamModelsCatalog(body []byte) []any {
	data := gjson.GetBytes(body, "data")
	if !data.IsArray() {
		parsed := gjson.ParseBytes(body)
		if parsed.IsArray() {
			data = parsed
		}
	}
	if !data.IsArray() || len(data.Array()) == 0 {
		return nil
	}
	var items []any
	if err := json.Unmarshal([]byte(data.Raw), &items); err != nil {
		return nil
	}
	return items
}

func grokPassthroughCatalogModelID(item any) string {
	obj, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"id", "model", "model_id", "modelId"} {
		if id, ok := obj[key].(string); ok {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}
