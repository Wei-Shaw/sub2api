package admin

import (
	"context"
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/plugin"
)

// ---------- OAuth plugin helpers (package-level for use by multiple handler types) ----------

// pluginClientForPlatform is a package-level helper that resolves an
// AccountPlatformClient from a PlatformRegistry. Returns nil if unavailable.
func pluginClientForPlatform(registry *plugin.PlatformRegistry, platform string) *plugin.AccountPlatformClient {
	if registry == nil {
		return nil
	}
	client, err := plugin.ClientForPlatform(registry, platform)
	if err != nil {
		return nil
	}
	return client
}

// tryPluginGenerateAuthURL delegates auth URL generation to the plugin.
// Returns (authURL, sessionID, handled). handled=false means fallback to core.
func tryPluginGenerateAuthURL(
	ctx context.Context,
	registry *plugin.PlatformRegistry,
	platform, oauthType string,
	proxyID int64,
	redirectURI string,
	params map[string]string,
) (authURL, sessionID string, handled bool) {
	client := pluginClientForPlatform(registry, platform)
	if client == nil {
		return "", "", false
	}
	resp, err := client.GenerateAuthURL(ctx, platform, oauthType, proxyID, redirectURI, params)
	if err != nil {
		if isUnimplemented(err) {
			return "", "", false
		}
		slog.WarnContext(ctx, "plugin generate auth URL failed, falling back to core",
			"platform", platform, "err", err)
		return "", "", false
	}
	return resp.AuthUrl, resp.SessionId, true
}

// tryPluginExchangeOAuthCode delegates code exchange to the plugin.
// Returns (credentialsJSON, extraJSON, accountName, tierID, handled).
func tryPluginExchangeOAuthCode(
	ctx context.Context,
	registry *plugin.PlatformRegistry,
	platform, oauthType, sessionID, code, state string,
	proxyID int64,
	redirectURI string,
	params map[string]string,
) (credentialsJSON, extraJSON []byte, accountName, tierID string, handled bool) {
	client := pluginClientForPlatform(registry, platform)
	if client == nil {
		return nil, nil, "", "", false
	}
	resp, err := client.ExchangeOAuthCode(
		ctx, platform, oauthType, sessionID, code, state,
		proxyID, redirectURI, params,
	)
	if err != nil {
		if isUnimplemented(err) {
			return nil, nil, "", "", false
		}
		slog.WarnContext(ctx, "plugin exchange OAuth code failed, falling back to core",
			"platform", platform, "err", err)
		return nil, nil, "", "", false
	}
	return resp.CredentialsJson, resp.ExtraJson, resp.AccountName, resp.TierId, true
}

// tryPluginValidateRefreshToken delegates refresh token validation to the plugin.
// Returns (credentialsJSON, extraJSON, accountName, tierID, handled).
func tryPluginValidateRefreshToken(
	ctx context.Context,
	registry *plugin.PlatformRegistry,
	platform, refreshToken string,
	proxyID int64,
	params map[string]string,
) (credentialsJSON, extraJSON []byte, accountName, tierID string, handled bool) {
	client := pluginClientForPlatform(registry, platform)
	if client == nil {
		return nil, nil, "", "", false
	}
	resp, err := client.ValidateRefreshToken(ctx, platform, refreshToken, proxyID, params)
	if err != nil {
		if isUnimplemented(err) {
			return nil, nil, "", "", false
		}
		slog.WarnContext(ctx, "plugin validate refresh token failed, falling back to core",
			"platform", platform, "err", err)
		return nil, nil, "", "", false
	}
	return resp.CredentialsJson, resp.ExtraJson, resp.AccountName, resp.TierId, true
}

// tryPluginCookieAuth delegates cookie authentication to the plugin.
// Returns (credentialsJSON, extraJSON, accountName, handled).
func tryPluginCookieAuth(
	ctx context.Context,
	registry *plugin.PlatformRegistry,
	platform, sessionKey string,
	proxyID int64,
	scope string,
) (credentialsJSON, extraJSON []byte, accountName string, handled bool) {
	client := pluginClientForPlatform(registry, platform)
	if client == nil {
		return nil, nil, "", false
	}
	resp, err := client.CookieAuth(ctx, sessionKey, proxyID, scope)
	if err != nil {
		if isUnimplemented(err) {
			return nil, nil, "", false
		}
		slog.WarnContext(ctx, "plugin cookie auth failed, falling back to core",
			"platform", platform, "err", err)
		return nil, nil, "", false
	}
	return resp.CredentialsJson, resp.ExtraJson, resp.AccountName, true
}
