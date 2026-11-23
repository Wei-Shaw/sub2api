package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

const (
	// AntigravityModelCatalogExtraKey stores the account's callable upstream model IDs
	// as reported by fetchAvailableModels: {"models": [sorted ids], "synced_at": RFC3339}.
	AntigravityModelCatalogExtraKey = "antigravity_model_catalog"

	// AntigravityModelMappingModeCredentialKey selects how an Antigravity account builds
	// its model mapping. AntigravityModelMappingModeLive derives it from the saved catalog.
	AntigravityModelMappingModeCredentialKey = "model_mapping_mode"
	AntigravityModelMappingModeLive          = "live"

	// antigravityModelCatalogMaxAge is how old a saved catalog may get before the
	// background token refresh fetches a new one.
	antigravityModelCatalogMaxAge       = 6 * time.Hour
	antigravityModelCatalogFetchTimeout = 10 * time.Second
	antigravityDisabledModelSuffix      = "-disabled"
)

// AntigravityModelCatalog is the persisted form of an account's live model catalog.
type AntigravityModelCatalog struct {
	Models   []string `json:"models"`
	SyncedAt string   `json:"synced_at"`
}

// normalizeAntigravityCatalogIDs trims, drops empty IDs and "-disabled" placeholders,
// dedupes and sorts. Both the save path and the mapping derivation use it, so a
// catalog written by an older build cannot reintroduce a placeholder.
func normalizeAntigravityCatalogIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || strings.HasSuffix(id, antigravityDisabledModelSuffix) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// newAntigravityModelCatalog builds the catalog from a fetchAvailableModels response.
// It reports false when the response holds no callable model, so an empty answer
// never replaces a saved catalog.
func newAntigravityModelCatalog(resp *antigravity.FetchAvailableModelsResponse, now time.Time) (AntigravityModelCatalog, bool) {
	if resp == nil {
		return AntigravityModelCatalog{}, false
	}
	ids := make([]string, 0, len(resp.Models))
	for id, info := range resp.Models {
		if info.Callable() {
			ids = append(ids, id)
		}
	}
	ids = normalizeAntigravityCatalogIDs(ids)
	if len(ids) == 0 {
		return AntigravityModelCatalog{}, false
	}
	return AntigravityModelCatalog{Models: ids, SyncedAt: now.UTC().Format(time.RFC3339)}, true
}

// IsAntigravityLiveModelMapping reports whether the account derives its model mapping
// from the saved upstream catalog.
func (a *Account) IsAntigravityLiveModelMapping() bool {
	if a == nil || a.Platform != domain.PlatformAntigravity || a.Credentials == nil {
		return false
	}
	mode, _ := a.Credentials[AntigravityModelMappingModeCredentialKey].(string)
	return strings.EqualFold(strings.TrimSpace(mode), AntigravityModelMappingModeLive)
}

// antigravityModelCatalogParts returns the stored model list and synced_at without
// copying. Extra holds map[string]any after a DB or cache decode and the struct after
// an in-process save.
func antigravityModelCatalogParts(raw any) (models any, syncedAt string) {
	switch catalog := raw.(type) {
	case AntigravityModelCatalog:
		return catalog.Models, catalog.SyncedAt
	case *AntigravityModelCatalog:
		if catalog == nil {
			return nil, ""
		}
		return catalog.Models, catalog.SyncedAt
	case map[string]any:
		syncedAt, _ := catalog["synced_at"].(string)
		return catalog["models"], syncedAt
	}
	return nil, ""
}

// antigravityModelCatalogIDs returns the account's saved catalog IDs, normalized.
func (a *Account) antigravityModelCatalogIDs() []string {
	if a == nil || a.Extra == nil {
		return nil
	}
	models, _ := antigravityModelCatalogParts(a.Extra[AntigravityModelCatalogExtraKey])
	switch list := models.(type) {
	case []string:
		return normalizeAntigravityCatalogIDs(list)
	case []any:
		ids := make([]string, 0, len(list))
		for _, item := range list {
			if id, ok := item.(string); ok {
				ids = append(ids, id)
			}
		}
		return normalizeAntigravityCatalogIDs(ids)
	}
	return nil
}

// antigravityModelCatalogCacheKey identifies the stored catalog value for the model
// mapping cache: the backing array pointer and length of its model list.
func (a *Account) antigravityModelCatalogCacheKey() (uintptr, int) {
	if a == nil || a.Extra == nil {
		return 0, 0
	}
	models, _ := antigravityModelCatalogParts(a.Extra[AntigravityModelCatalogExtraKey])
	value := reflect.ValueOf(models)
	if value.Kind() != reflect.Slice || value.Len() == 0 {
		return 0, 0
	}
	return value.Pointer(), value.Len()
}

// antigravityModelCatalogSignature hashes the stored catalog IDs in order, so an
// in-place change of the list invalidates the cached mapping.
func (a *Account) antigravityModelCatalogSignature() uint64 {
	if a == nil || a.Extra == nil {
		return 0
	}
	models, _ := antigravityModelCatalogParts(a.Extra[AntigravityModelCatalogExtraKey])
	h := uint64(modelMappingSignatureFNVOffset)
	write := func(s string) {
		for i := 0; i < len(s); i++ {
			h ^= uint64(s[i])
			h *= modelMappingSignatureFNVPrime
		}
		// Separator byte 0 between IDs.
		h *= modelMappingSignatureFNVPrime
	}
	switch list := models.(type) {
	case []string:
		for _, id := range list {
			write(id)
		}
	case []any:
		for _, item := range list {
			if id, ok := item.(string); ok {
				write(id)
			} else {
				write("\x01")
			}
		}
	default:
		return 0
	}
	return h
}

// antigravityModelCatalogSyncedAt returns when the saved catalog was fetched.
func (a *Account) antigravityModelCatalogSyncedAt() (time.Time, bool) {
	if a == nil || a.Extra == nil {
		return time.Time{}, false
	}
	_, syncedAt := antigravityModelCatalogParts(a.Extra[AntigravityModelCatalogExtraKey])
	synced, err := time.Parse(time.RFC3339, strings.TrimSpace(syncedAt))
	if err != nil {
		return time.Time{}, false
	}
	return synced, true
}

// antigravityModelCatalogStale reports whether the saved catalog is missing,
// unreadable or older than antigravityModelCatalogMaxAge.
func (a *Account) antigravityModelCatalogStale(now time.Time) bool {
	if len(a.antigravityModelCatalogIDs()) == 0 {
		return true
	}
	synced, ok := a.antigravityModelCatalogSyncedAt()
	return !ok || now.Sub(synced) >= antigravityModelCatalogMaxAge
}

// resolveAntigravityAccountProxyURL returns the proxy URL of an account's bound
// proxy, or "" for an account without one. A bound proxy that cannot be loaded is
// an error: deployments bind proxies without a direct fallback, so the request must
// not leave the host directly.
func resolveAntigravityAccountProxyURL(ctx context.Context, proxyRepo ProxyRepository, account *Account) (string, error) {
	if account == nil || account.ProxyID == nil {
		return "", nil
	}
	if account.Proxy != nil && account.Proxy.ID == *account.ProxyID {
		return account.Proxy.URL(), nil
	}
	if proxyRepo == nil {
		return "", fmt.Errorf("proxy %d of account %d is not loaded", *account.ProxyID, account.ID)
	}
	proxy, err := proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil {
		return "", fmt.Errorf("load proxy %d of account %d: %w", *account.ProxyID, account.ID, err)
	}
	if proxy == nil {
		return "", fmt.Errorf("proxy %d of account %d not found", *account.ProxyID, account.ID)
	}
	return proxy.URL(), nil
}

// fetchAntigravityAvailableModels queries fetchAvailableModels on the endpoint that
// receives this account's forwarded requests, since catalogs differ per endpoint.
func fetchAntigravityAvailableModels(ctx context.Context, client *antigravity.Client, account *Account, accessToken string, bodyLimit int64) (*antigravity.FetchAvailableModelsResponse, map[string]any, error) {
	baseURL := resolveAntigravityForwardBaseURL(account)
	if baseURL == "" {
		return nil, nil, errors.New("no antigravity base url configured")
	}
	return client.FetchAvailableModelsFromURL(ctx, baseURL, accessToken, strings.TrimSpace(account.GetCredential("project_id")), bodyLimit)
}

// persistAntigravityModelCatalog saves the catalog on a persisted account and mirrors
// it into account.Extra. A failed write keeps the previous catalog and only logs,
// because the request path never depends on this background refresh.
// The catalog key triggers a scheduler rebuild, so an unchanged list younger than
// half the refresh age is not written again by the frequent quota fetches.
func persistAntigravityModelCatalog(ctx context.Context, repo AccountRepository, account *Account, catalog AntigravityModelCatalog) {
	if account == nil || account.ID <= 0 || repo == nil || len(catalog.Models) == 0 {
		return
	}
	if slices.Equal(account.antigravityModelCatalogIDs(), catalog.Models) {
		if synced, ok := account.antigravityModelCatalogSyncedAt(); ok {
			if now, err := time.Parse(time.RFC3339, catalog.SyncedAt); err == nil && now.Sub(synced) < antigravityModelCatalogMaxAge/2 {
				return
			}
		}
	}
	if err := repo.UpdateExtra(ctx, account.ID, map[string]any{AntigravityModelCatalogExtraKey: catalog}); err != nil {
		slog.Warn("antigravity_model_catalog.save_failed", "account_id", account.ID, "error", err)
		return
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	account.Extra[AntigravityModelCatalogExtraKey] = catalog
}
