package service

import (
	"context"
	"fmt"
)

// PluginTokenRefreshResult contains the result of a plugin-based token refresh.
type PluginTokenRefreshResult struct {
	// UpdatedCredentials contains the new credentials to persist.
	// Already merged with existing credentials by the plugin.
	UpdatedCredentials map[string]any
	// UpdatedExtra contains updated extra fields (may be nil if unchanged).
	UpdatedExtra map[string]any
}

// PluginTokenRefresher delegates token refresh to a gateway plugin's
// RefreshToken RPC. The service layer defines this interface; the plugin
// layer provides the implementation backed by PlatformRegistry.
//
// When a platform has a registered plugin, the host calls RefreshToken
// instead of the legacy per-platform OAuthService. This eliminates the
// direct dependency on OpenAIOAuthService, GeminiOAuthService, etc.
type PluginTokenRefresher interface {
	// HasPlatform returns true if the given platform has a registered plugin
	// capable of handling token refresh.
	HasPlatform(platform string) bool

	// RefreshToken delegates token refresh to the plugin. Returns the
	// updated credentials (already merged by the plugin) or an error.
	RefreshToken(ctx context.Context, account *Account) (*PluginTokenRefreshResult, error)
}

// noopPluginTokenRefresher is a no-op implementation used when the plugin
// system is disabled. HasPlatform always returns false.
type noopPluginTokenRefresher struct{}

func (n *noopPluginTokenRefresher) HasPlatform(string) bool { return false }
func (n *noopPluginTokenRefresher) RefreshToken(_ context.Context, account *Account) (*PluginTokenRefreshResult, error) {
	return nil, fmt.Errorf("no plugin registered for platform %q", account.Platform)
}

// NoopPluginTokenRefresher returns a PluginTokenRefresher that never
// handles any platform. Used when the plugin system is disabled.
func NoopPluginTokenRefresher() PluginTokenRefresher {
	return &noopPluginTokenRefresher{}
}
