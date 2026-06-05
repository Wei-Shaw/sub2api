package service

import (
	"context"
	"strconv"
	"time"
)

// PluginBasedTokenRefresher implements TokenRefresher and OAuthRefreshExecutor
// by delegating to a PluginTokenRefresher. It replaces the per-platform
// refreshers (ClaudeTokenRefresher, OpenAITokenRefresher, etc.) when a
// plugin is registered for the account's platform.
//
// CanRefresh returns true for any OAuth account whose platform is handled
// by a plugin, so a single instance covers all plugin-backed platforms.
type PluginBasedTokenRefresher struct {
	plugin PluginTokenRefresher
}

// NewPluginBasedTokenRefresher creates a unified token refresher backed by
// plugin RPCs.
func NewPluginBasedTokenRefresher(plugin PluginTokenRefresher) *PluginBasedTokenRefresher {
	return &PluginBasedTokenRefresher{plugin: plugin}
}

// CacheKey returns a cache key for the distributed lock. Uses a generic
// format since the plugin handles all platforms.
func (r *PluginBasedTokenRefresher) CacheKey(account *Account) string {
	return "plugin:refresh:" + account.Platform + ":" + strconv.FormatInt(account.ID, 10)
}

// CanRefresh returns true for OAuth accounts whose platform has a registered
// plugin.
func (r *PluginBasedTokenRefresher) CanRefresh(account *Account) bool {
	return account.Type == AccountTypeOAuth && r.plugin.HasPlatform(account.Platform)
}

// NeedsRefresh checks if the token needs refreshing based on expires_at.
func (r *PluginBasedTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		// No expiry info; for rate-limited accounts, refresh proactively
		return account.IsRateLimited()
	}
	return time.Until(*expiresAt) < refreshWindow
}

// Refresh delegates to the plugin's RefreshToken RPC and returns the
// updated credentials.
func (r *PluginBasedTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	result, err := r.plugin.RefreshToken(ctx, account)
	if err != nil {
		return nil, err
	}
	if result.UpdatedCredentials == nil {
		return nil, nil
	}
	// Merge with existing credentials to preserve fields the plugin didn't return
	merged := MergeCredentials(account.Credentials, result.UpdatedCredentials)
	return merged, nil
}
