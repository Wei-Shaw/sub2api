package service

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// GetAnthropicAPIKeyAuthScheme moved to domain in Phase 3 (Account BC hybrid).
// Consts re-exported so callers and setAnthropicAPIKeyAuthHeader compile.
const (
	AnthropicAPIKeyAuthSchemeXAPIKey             = domain.AnthropicAPIKeyAuthSchemeXAPIKey
	AnthropicAPIKeyAuthSchemeAuthorizationBearer = domain.AnthropicAPIKeyAuthSchemeAuthorizationBearer
)

func setAnthropicAPIKeyAuthHeader(header http.Header, account *Account, token string) {
	if account.GetAnthropicAPIKeyAuthScheme() == AnthropicAPIKeyAuthSchemeAuthorizationBearer {
		header.Set("Authorization", "Bearer "+token)
		return
	}
	header.Set("x-api-key", token)
}
