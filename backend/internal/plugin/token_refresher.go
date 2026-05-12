package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// pluginTokenRefresher implements service.PluginTokenRefresher by
// delegating token refresh to the plugin's RefreshToken gRPC RPC via
// the PlatformRegistry.
type pluginTokenRefresher struct {
	registry *PlatformRegistry
}

// NewPluginTokenRefresher creates a PluginTokenRefresher backed by the
// given PlatformRegistry.
func NewPluginTokenRefresher(registry *PlatformRegistry) service.PluginTokenRefresher {
	return &pluginTokenRefresher{registry: registry}
}

// HasPlatform returns true if the given platform has a registered plugin.
func (r *pluginTokenRefresher) HasPlatform(platform string) bool {
	return r.registry.HasPlatform(platform)
}

// RefreshToken delegates to the plugin's RefreshToken RPC.
func (r *pluginTokenRefresher) RefreshToken(
	ctx context.Context,
	account *service.Account,
) (*service.PluginTokenRefreshResult, error) {
	client, err := ClientForPlatform(r.registry, account.Platform)
	if err != nil {
		return nil, err
	}

	credsJSON, _ := json.Marshal(account.Credentials)
	extraJSON, _ := json.Marshal(account.Extra)

	resp, err := client.RefreshToken(
		ctx,
		account.ID,
		account.Platform,
		account.Type,
		credsJSON,
		extraJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("plugin RefreshToken RPC failed: %w", err)
	}

	if !resp.GetSuccess() {
		return nil, fmt.Errorf("%s", resp.GetError())
	}

	result := &service.PluginTokenRefreshResult{}

	if len(resp.GetUpdatedCredentialsJson()) > 0 {
		var creds map[string]any
		if err := json.Unmarshal(resp.GetUpdatedCredentialsJson(), &creds); err != nil {
			return nil, fmt.Errorf("failed to unmarshal updated credentials: %w", err)
		}
		result.UpdatedCredentials = creds
	}

	if len(resp.GetUpdatedExtraJson()) > 0 {
		var extra map[string]any
		if err := json.Unmarshal(resp.GetUpdatedExtraJson(), &extra); err != nil {
			return nil, fmt.Errorf("failed to unmarshal updated extra: %w", err)
		}
		result.UpdatedExtra = extra
	}

	return result, nil
}
