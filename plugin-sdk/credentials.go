package pluginsdk

import (
	"context"
	"errors"
	"time"
)

// CredentialManager provides unified credential management for gateway
// plugins. Plugins obtain an instance via PluginContext.Credentials().
//
// Three credential models are supported:
//
//  1. OAuth — SDK manages refresh / cache / distributed locks / DB
//     write-back internally. The plugin calls GetOAuthToken and
//     receives a short-lived access_token; long-lived refresh_tokens
//     never leave the host process.
//
//  2. API Key — a direct lookup for accounts whose credential is a
//     static key (e.g. Antigravity upstream api_key). No refresh
//     logic is involved.
//
//  3. Custom auth — the plugin registers a CustomAuthRefresher for
//     an arbitrary auth type (e.g. Bedrock SigV4, Vertex JWT). The
//     SDK handles cache and locking; the refresher only provides the
//     token-minting logic.
type CredentialManager interface {
	// GetOAuthToken returns a short-lived access_token for the given
	// account. The host manages refresh / cache / distributed locks /
	// DB write-back; the plugin never sees the refresh_token.
	//
	// Returns ErrCredentialNotFound when the account does not exist or
	// has no OAuth credentials configured.
	GetOAuthToken(ctx context.Context, accountID int64) (*OAuthToken, error)

	// GetAPIKey returns the stored API key for the given account.
	//
	// Returns ErrCredentialNotFound when the account does not exist or
	// has no API key configured.
	GetAPIKey(ctx context.Context, accountID int64) (string, error)

	// RegisterCustomAuth registers a refresher for the named auth
	// type. Subsequent GetCustomToken calls for the same authType
	// delegate refresh logic to this refresher while the SDK manages
	// cache and distributed locking.
	//
	// Calling RegisterCustomAuth with an authType that was already
	// registered silently replaces the previous refresher.
	RegisterCustomAuth(authType string, refresher CustomAuthRefresher)

	// GetCustomToken returns a cached or freshly-minted token for the
	// given account and auth type. If no refresher has been registered
	// for authType, returns ErrRefreshFailed.
	//
	// Returns ErrCredentialNotFound when the account does not exist.
	GetCustomToken(ctx context.Context, accountID int64, authType string) (*CustomToken, error)
}

// OAuthToken holds the result of a successful GetOAuthToken call.
// AccessToken is the short-lived bearer token the plugin should use
// for upstream requests; ExpiresAt indicates when the host considers
// it stale (the host may pre-emptively refresh before this time).
type OAuthToken struct {
	AccessToken string
	ExpiresAt   time.Time
	TokenType   string
	Scopes      []string
}

// CustomToken holds the result of a successful GetCustomToken call.
// Token is the opaque credential string; Metadata carries any extra
// key-value pairs the refresher chose to surface (e.g. region,
// endpoint URL).
type CustomToken struct {
	Token     string
	ExpiresAt time.Time
	Metadata  map[string]string
}

// CustomAuthRefresher is implemented by plugins that need non-OAuth
// credential minting (e.g. Bedrock SigV4 pre-signing, Vertex JWT
// exchange). The SDK calls Refresh when the cached token is missing
// or expired; the refresher must return a fresh token or an error.
//
// currentCreds contains the account's stored credential fields as
// provided by the host (sensitive long-lived keys are excluded).
type CustomAuthRefresher interface {
	Refresh(ctx context.Context, accountID int64, currentCreds map[string]string) (*CustomToken, error)
}

// Sentinel errors for CredentialManager. Callers can test with
// errors.Is to distinguish "no data" from "operation failed".
var (
	// ErrCredentialNotFound is returned when the requested account
	// does not exist or has no credentials of the requested type.
	ErrCredentialNotFound = errors.New("pluginsdk: credential not found")

	// ErrRefreshFailed is returned when a token refresh attempt
	// fails (network error, upstream rejection, or no refresher
	// registered for the requested auth type).
	ErrRefreshFailed = errors.New("pluginsdk: credential refresh failed")
)

// nilCredentialManager is returned when the host does not provide
// credential management (older hosts, or test rigs that wire only a
// subset of the SDK). Every method returns a clear error so debugging
// is straightforward; we do NOT silently no-op because that would
// mask real misconfiguration.
type nilCredentialManager struct{}

func (nilCredentialManager) GetOAuthToken(_ context.Context, _ int64) (*OAuthToken, error) {
	return nil, errors.New("pluginsdk: CredentialManager not available on this host")
}

func (nilCredentialManager) GetAPIKey(_ context.Context, _ int64) (string, error) {
	return "", errors.New("pluginsdk: CredentialManager not available on this host")
}

func (nilCredentialManager) RegisterCustomAuth(_ string, _ CustomAuthRefresher) {}

func (nilCredentialManager) GetCustomToken(_ context.Context, _ int64, _ string) (*CustomToken, error) {
	return nil, errors.New("pluginsdk: CredentialManager not available on this host")
}
