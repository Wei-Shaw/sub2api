// Grok credential BC: the GatewayFailureReason type, the credential mutation
// snapshot value type, and the typed GrokCredentialReason* sentinels used to
// classify Grok OAuth credential failures. Lifted from internal/service so
// account_repo can depend solely on domain.
//
// Service re-exports all symbols as aliases:
//   - GatewayFailureReason                          (internal/service/gateway_service.go)
//   - GrokCredentialReason* (10 sentinels)          (internal/service/grok_credential_failure.go)
//   - GrokCredentialMutationSnapshot                (internal/service/grok_credential_failure.go)
//
// The shared GatewayFailureReason name is intentionally preserved in domain so
// consumers like openai_gateway_upstream_errors.go compile unchanged.
package domain

// GatewayFailureReason classifies why an upstream/credential request failed.
type GatewayFailureReason string

// GrokCredentialReason* values are GatewayFailureReason-typed sentinels for
// the Grok OAuth credential failure classification.
const (
	GrokCredentialReasonRevoked          GatewayFailureReason = "grok_oauth_credential_revoked"
	GrokCredentialReasonMissing          GatewayFailureReason = "grok_oauth_credentials_missing"
	GrokCredentialReasonEntitlement      GatewayFailureReason = "grok_oauth_entitlement_action_required"
	GrokCredentialReasonProxyInvalid     GatewayFailureReason = "grok_oauth_proxy_invalid"
	GrokCredentialReasonRefreshTransient GatewayFailureReason = "grok_oauth_refresh_transient"
	GrokCredentialReasonProviderConfig   GatewayFailureReason = "grok_oauth_provider_config"
	GrokCredentialReasonProviderDown     GatewayFailureReason = "grok_oauth_provider_unavailable"
	GrokCredentialReasonAccountChanged   GatewayFailureReason = "grok_oauth_account_state_changed"
	GrokCredentialReasonStateUpdate      GatewayFailureReason = "grok_oauth_account_state_update_failed"
	GrokCredentialReasonFailoverTimeout  GatewayFailureReason = "grok_oauth_failover_timeout"
)

// GrokCredentialMutationSnapshot is the credential identity observed when the
// request selected an account. Repository mutations compare all fields before
// quarantining that account so a concurrent refresh cannot be overwritten.
type GrokCredentialMutationSnapshot struct {
	CredentialsJSON string
	AccessToken     string
	RefreshToken    string
	TokenVersion    int64
	ProxyID         *int64
}
