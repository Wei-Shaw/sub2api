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
