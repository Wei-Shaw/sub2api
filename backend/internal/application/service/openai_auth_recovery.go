package service

import (
	"net/http"
	"strings"
)

// shouldRefreshOpenAIOAuth401 identifies an authentication response that may
// be caused by an early access-token expiry. ChatGPT Team workspaces commonly
// return a generic 401/Unauthorized body instead of the structured
// token_expired code, so the request path must not treat every unrecognised
// 401 as permanent before giving the refresh token one bounded attempt.
//
// Explicit revocation, disabled-account and API-key-style codes remain
// terminal. A missing refresh token also remains terminal because the account
// cannot recover without user interaction.
func shouldRefreshOpenAIOAuth401(account *Account, statusCode int, responseBody []byte) bool {
	if statusCode != http.StatusUnauthorized || account == nil ||
		!account.IsOpenAIOAuth() || account.IsOpenAIPersonalAccessToken() ||
		strings.TrimSpace(account.GetOpenAIRefreshToken()) == "" {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(extractUpstreamErrorCode(responseBody))) {
	case "token_revoked", "token_invalidated", "invalid_api_key", "account_deactivated", "account_disabled", "organization_deactivated", "invalid_grant", "invalid_refresh_token", "refresh_token_reused", "refresh_token_invalidated":
		return false
	default:
		return true
	}
}
