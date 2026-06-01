package pluginsdk

import (
	"context"
	"encoding/json"
)

// AccountPlatformExtension is the Go interface gateway plugins implement
// to handle platform-specific account operations. The host discovers it
// via lazy gRPC invocation — codes.Unimplemented means "not available".
//
// Plugins that declare Manifest.Platforms SHOULD implement this interface
// (via GRPCServiceRegistrar) so the host can delegate validation, testing,
// and token refresh to the plugin.
type AccountPlatformExtension interface {
	ValidateAccountData(ctx context.Context, req *ValidateAccountDataReq) (*ValidateAccountDataResp, error)
	TestConnection(ctx context.Context, req *TestConnectionReq, stream TestConnectionStream) error
	RefreshToken(ctx context.Context, req *RefreshTokenReq) (*RefreshTokenResp, error)
	RefreshTier(ctx context.Context, req *RefreshTierReq) (*RefreshTierResp, error)
	GetAvailableModels(ctx context.Context, req *GetAvailableModelsReq) ([]AvailableModel, error)
	ExecuteCustomAction(ctx context.Context, req *ExecuteCustomActionReq) (*ExecuteCustomActionResp, error)
	// IsModelSupported checks whether the account supports the requested model
	// at runtime. Return an error to signal that the plugin does not implement
	// this check, causing the host to fall back to static model_mapping.
	IsModelSupported(ctx context.Context, req *IsModelSupportedReq) (*IsModelSupportedResp, error)
	// GetSchedulingHints lets the plugin dynamically adjust account priority
	// and availability during scheduling. Return an error (including
	// codes.Unimplemented) to signal the plugin does not implement this.
	GetSchedulingHints(ctx context.Context, req *GetSchedulingHintsReq) (*GetSchedulingHintsResp, error)
	// CheckSchedulability performs a plugin-defined schedulability check on a
	// single account during the host's scheduling filter loop. Return an error
	// to signal the plugin does not implement this check (default: schedulable).
	CheckSchedulability(ctx context.Context, req *CheckSchedulabilityReq) (*CheckSchedulabilityResp, error)
	// GenerateAuthURL generates an OAuth authorization URL for the platform.
	// Return an error to signal the plugin does not support OAuth.
	GenerateAuthURL(ctx context.Context, req *GenerateAuthURLReq) (*GenerateAuthURLResp, error)
	// ExchangeOAuthCode exchanges an OAuth code for credentials.
	// Return an error to signal the plugin does not support OAuth.
	ExchangeOAuthCode(ctx context.Context, req *ExchangeOAuthCodeReq) (*ExchangeOAuthCodeResp, error)
	// ValidateRefreshToken validates a refresh token and returns credentials.
	// Return an error to signal the plugin does not support this flow.
	ValidateRefreshToken(ctx context.Context, req *ValidateRefreshTokenReq) (*ValidateRefreshTokenResp, error)
	// CookieAuth authenticates using a session cookie (e.g. Claude session_key).
	// Return an error to signal the plugin does not support cookie auth.
	CookieAuth(ctx context.Context, req *CookieAuthReq) (*CookieAuthResp, error)
	// SetPrivacy sets the privacy/data-collection mode for an account.
	// Return an error to signal the plugin does not support privacy management.
	SetPrivacy(ctx context.Context, req *SetPrivacyReq) (*SetPrivacyResp, error)
	// PostAccountCreate is called after the host persists a new account.
	// Return an error to signal the plugin does not implement this hook.
	PostAccountCreate(ctx context.Context, req *PostAccountCreateReq) (*PostAccountCreateResp, error)
	// ValidateGroupConfig validates group_extra data before a group is
	// created or updated. Return an error to signal the plugin does not
	// implement this check (host silently skips validation).
	ValidateGroupConfig(ctx context.Context, req *ValidateGroupConfigReq) (*ValidateGroupConfigResp, error)
}

// ValidateAccountDataReq is the input for ValidateAccountData.
type ValidateAccountDataReq struct {
	Platform    string
	AccountType string
	Credentials json.RawMessage
	Extra       json.RawMessage
	IsUpdate    bool
	AccountID   int64
}

// ValidateAccountDataResp is the output of ValidateAccountData.
type ValidateAccountDataResp struct {
	Valid                bool
	FieldErrors          map[string]string
	ProcessedCredentials json.RawMessage
	ProcessedExtra       json.RawMessage
}

// TestConnectionReq is the input for TestConnection.
type TestConnectionReq struct {
	AccountID   int64
	Platform    string
	AccountType string
	Credentials json.RawMessage
	Extra       json.RawMessage
	ModelID     string
}

// TestConnectionEvent is a single event in the test connection stream.
type TestConnectionEvent struct {
	Type    string          `json:"type"`
	Text    string          `json:"text,omitempty"`
	Model   string          `json:"model,omitempty"`
	Success bool            `json:"success,omitempty"`
	Error   string          `json:"error,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// TestConnectionStream is the server-side stream for test events.
type TestConnectionStream interface {
	Send(event *TestConnectionEvent) error
}

// RefreshTokenReq is the input for RefreshToken.
type RefreshTokenReq struct {
	AccountID   int64
	Platform    string
	AccountType string
	Credentials json.RawMessage
	Extra       json.RawMessage
}

// RefreshTokenResp is the output of RefreshToken.
type RefreshTokenResp struct {
	Success            bool
	Error              string
	UpdatedCredentials json.RawMessage
	UpdatedExtra       json.RawMessage
}

// RefreshTierReq is the input for RefreshTier.
type RefreshTierReq struct {
	AccountID   int64
	Platform    string
	AccountType string
	Credentials json.RawMessage
	Extra       json.RawMessage
}

// RefreshTierResp is the output of RefreshTier.
type RefreshTierResp struct {
	Success      bool
	Error        string
	UpdatedExtra json.RawMessage
}

// GetAvailableModelsReq is the input for GetAvailableModels.
type GetAvailableModelsReq struct {
	AccountID   int64
	Platform    string
	AccountType string
	Credentials json.RawMessage
}

// AvailableModel describes a model available for an account.
type AvailableModel struct {
	ModelID     string
	DisplayName string
	Available   bool
}

// ExecuteCustomActionReq is the input for ExecuteCustomAction.
type ExecuteCustomActionReq struct {
	ActionID    string
	AccountID   int64
	Platform    string
	Credentials json.RawMessage
	Extra       json.RawMessage
	Payload     json.RawMessage
}

// ExecuteCustomActionResp is the output of ExecuteCustomAction.
type ExecuteCustomActionResp struct {
	Success            bool
	Error              string
	Result             json.RawMessage
	UpdatedCredentials json.RawMessage
	UpdatedExtra       json.RawMessage
}

// IsModelSupportedReq is the input for IsModelSupported.
type IsModelSupportedReq struct {
	AccountID   int64
	Platform    string
	Model       string
	Credentials json.RawMessage
	Extra       json.RawMessage
}

// IsModelSupportedResp is the output of IsModelSupported.
type IsModelSupportedResp struct {
	Supported   bool
	MappedModel string // optional: plugin can return the upstream model name
}

// GetSchedulingHintsReq is the input for GetSchedulingHints.
type GetSchedulingHintsReq struct {
	Accounts        []SchedulingHintAccountInfo
	GatewayProtocol string
	RequestedModel  string
}

// SchedulingHintAccountInfo carries per-account data for the scheduling
// hints batch request.
type SchedulingHintAccountInfo struct {
	AccountID   int64
	Credentials json.RawMessage
	Extra       json.RawMessage
}

// GetSchedulingHintsResp is the output of GetSchedulingHints.
type GetSchedulingHintsResp struct {
	Hints map[int64]SchedulingHint
}

// SchedulingHint is the per-account hint returned by GetSchedulingHints.
type SchedulingHint struct {
	PriorityModifier       int32
	TemporarilyUnavailable bool
	Reason                 string
}

// CheckSchedulabilityReq is the input for CheckSchedulability.
type CheckSchedulabilityReq struct {
	AccountID       int64
	Platform        string
	Credentials     json.RawMessage
	Extra           json.RawMessage
	RequestedModel  string
	GatewayProtocol string
}

// CheckSchedulabilityResp is the output of CheckSchedulability.
type CheckSchedulabilityResp struct {
	Schedulable bool
	Reason      string // reason when not schedulable (for logging)
}

// GenerateAuthURLReq is the input for GenerateAuthURL.
type GenerateAuthURLReq struct {
	Platform    string
	OAuthType   string // "oauth"/"setup-token"/"cookie" etc.
	ProxyID     int64
	RedirectURI string
	Params      map[string]string // platform-specific params
}

// GenerateAuthURLResp is the output of GenerateAuthURL.
type GenerateAuthURLResp struct {
	AuthURL   string
	SessionID string // plugin-managed session ID
}

// ExchangeOAuthCodeReq is the input for ExchangeOAuthCode.
type ExchangeOAuthCodeReq struct {
	Platform    string
	OAuthType   string
	SessionID   string
	Code        string
	State       string
	ProxyID     int64
	RedirectURI string
	Params      map[string]string
}

// ExchangeOAuthCodeResp is the output of ExchangeOAuthCode.
type ExchangeOAuthCodeResp struct {
	Credentials json.RawMessage // ready-to-persist credentials
	Extra       json.RawMessage // ready-to-persist extra
	AccountName string          // suggested account name
	TierID      string          // detected tier
}

// ValidateRefreshTokenReq is the input for ValidateRefreshToken.
type ValidateRefreshTokenReq struct {
	Platform     string
	RefreshToken string
	ProxyID      int64
	Params       map[string]string
}

// ValidateRefreshTokenResp is the output of ValidateRefreshToken.
type ValidateRefreshTokenResp struct {
	Credentials json.RawMessage
	Extra       json.RawMessage
	AccountName string
	TierID      string
}

// CookieAuthReq is the input for CookieAuth.
type CookieAuthReq struct {
	SessionKey string
	ProxyID    int64
	Scope      string
}

// CookieAuthResp is the output of CookieAuth.
type CookieAuthResp struct {
	Credentials json.RawMessage
	Extra       json.RawMessage
	AccountName string
}

// SetPrivacyReq is the input for SetPrivacy.
type SetPrivacyReq struct {
	AccountID   int64
	Platform    string
	Credentials json.RawMessage
	Extra       json.RawMessage
	Force       bool
}

// SetPrivacyResp is the output of SetPrivacy.
type SetPrivacyResp struct {
	Success     bool
	PrivacyMode string
	Error       string
}

// PostAccountCreateReq is the input for PostAccountCreate.
type PostAccountCreateReq struct {
	AccountID   int64
	Platform    string
	AccountType string
	Credentials json.RawMessage
	Extra       json.RawMessage
}

// PostAccountCreateResp is the output of PostAccountCreate.
type PostAccountCreateResp struct {
	UpdatedCredentials json.RawMessage // optional, nil = no update
	UpdatedExtra       json.RawMessage // optional
}

// ValidateGroupConfigReq is the input for ValidateGroupConfig.
type ValidateGroupConfigReq struct {
	Platform       string
	GroupExtraJSON json.RawMessage
	IsUpdate       bool
}

// ValidateGroupConfigResp is the output of ValidateGroupConfig.
type ValidateGroupConfigResp struct {
	Valid                   bool
	FieldErrors             map[string]string
	ProcessedGroupExtraJSON json.RawMessage // optional: processed group_extra with defaults filled
}
