package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

const (
	accountValidateTimeout = 10 * time.Second
	accountRefreshTimeout  = 30 * time.Second
	accountModelsTimeout   = 15 * time.Second
	accountActionTimeout   = 30 * time.Second
)

// AccountPlatformClient wraps gRPC calls to a plugin's
// AccountPlatformExtension service.
type AccountPlatformClient struct {
	stub pb.AccountPlatformExtensionClient
}

func NewAccountPlatformClient(conn *grpc.ClientConn) *AccountPlatformClient {
	return &AccountPlatformClient{
		stub: pb.NewAccountPlatformExtensionClient(conn),
	}
}

// ValidateAccountData delegates credential/extra validation to the plugin.
func (c *AccountPlatformClient) ValidateAccountData(
	ctx context.Context,
	platform, accountType string,
	credentials, extra json.RawMessage,
	isUpdate bool,
	accountID int64,
) (*pb.ValidateAccountDataResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, accountValidateTimeout)
	defer cancel()
	return c.stub.ValidateAccountData(ctx, &pb.ValidateAccountDataRequest{
		Platform:        platform,
		AccountType:     accountType,
		CredentialsJson: credentials,
		ExtraJson:       extra,
		IsUpdate:        isUpdate,
		AccountId:       accountID,
	})
}

// TestConnection opens a streaming test connection to the plugin.
// The caller reads events from the returned stream.
func (c *AccountPlatformClient) TestConnection(
	ctx context.Context,
	accountID int64,
	platform, accountType string,
	credentials, extra json.RawMessage,
	modelID string,
) (pb.AccountPlatformExtension_TestConnectionClient, error) {
	return c.stub.TestConnection(ctx, &pb.TestConnectionRequest{
		AccountId:       accountID,
		Platform:        platform,
		AccountType:     accountType,
		CredentialsJson: credentials,
		ExtraJson:       extra,
		ModelId:         modelID,
	})
}

// RefreshToken delegates token refresh to the plugin.
func (c *AccountPlatformClient) RefreshToken(
	ctx context.Context,
	accountID int64,
	platform, accountType string,
	credentials, extra json.RawMessage,
) (*pb.RefreshTokenResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, accountRefreshTimeout)
	defer cancel()
	return c.stub.RefreshToken(ctx, &pb.RefreshTokenRequest{
		AccountId:       accountID,
		Platform:        platform,
		AccountType:     accountType,
		CredentialsJson: credentials,
		ExtraJson:       extra,
	})
}

// RefreshTier delegates tier refresh to the plugin.
func (c *AccountPlatformClient) RefreshTier(
	ctx context.Context,
	accountID int64,
	platform, accountType string,
	credentials, extra json.RawMessage,
) (*pb.RefreshTierResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, accountRefreshTimeout)
	defer cancel()
	return c.stub.RefreshTier(ctx, &pb.RefreshTierRequest{
		AccountId:       accountID,
		Platform:        platform,
		AccountType:     accountType,
		CredentialsJson: credentials,
		ExtraJson:       extra,
	})
}

// GetAvailableModels retrieves the models available for an account.
func (c *AccountPlatformClient) GetAvailableModels(
	ctx context.Context,
	accountID int64,
	platform, accountType string,
	credentials, extra json.RawMessage,
) (*pb.GetAvailableModelsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, accountModelsTimeout)
	defer cancel()
	return c.stub.GetAvailableModels(ctx, &pb.GetAvailableModelsRequest{
		AccountId:       accountID,
		Platform:        platform,
		AccountType:     accountType,
		CredentialsJson: credentials,
		ExtraJson:       extra,
	})
}

// ExecuteCustomAction delegates a custom action to the plugin.
func (c *AccountPlatformClient) ExecuteCustomAction(
	ctx context.Context,
	actionID string,
	accountID int64,
	platform string,
	credentials, extra, payload json.RawMessage,
) (*pb.ExecuteCustomActionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, accountActionTimeout)
	defer cancel()
	return c.stub.ExecuteCustomAction(ctx, &pb.ExecuteCustomActionRequest{
		ActionId:        actionID,
		AccountId:       accountID,
		Platform:        platform,
		CredentialsJson: credentials,
		ExtraJson:       extra,
		PayloadJson:     payload,
	})
}

// ClientForPlatform creates a new AccountPlatformClient for the given
// platform from the PlatformRegistry. Returns an error if the platform
// is not registered.
func ClientForPlatform(registry *PlatformRegistry, platform string) (*AccountPlatformClient, error) {
	rp, ok := registry.Get(platform)
	if !ok {
		return nil, fmt.Errorf("platform %q not registered", platform)
	}
	return NewAccountPlatformClient(rp.Conn), nil
}

const accountModelSupportTimeout = 3 * time.Second

// IsModelSupported asks the plugin whether the given account supports
// the requested model. Returns (supported, mappedModel, error).
// A codes.Unimplemented error from the plugin signals "not implemented"
// and the caller should fall back to the static model_mapping check.
func (c *AccountPlatformClient) IsModelSupported(
	ctx context.Context,
	accountID int64,
	platform string,
	credentials, extra json.RawMessage,
	requestedModel string,
) (*pb.IsModelSupportedResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, accountModelSupportTimeout)
	defer cancel()
	return c.stub.IsModelSupported(ctx, &pb.IsModelSupportedRequest{
		AccountId:       accountID,
		AccountPlatform: platform,
		RequestedModel:  requestedModel,
		CredentialsJson: credentials,
		ExtraJson:       extra,
	})
}

const accountSchedulingHintsTimeout = 3 * time.Second

// GetSchedulingHints asks the plugin to provide dynamic scheduling hints
// for a batch of candidate accounts. Returns hints keyed by account ID.
// A codes.Unimplemented error signals the plugin does not implement this
// RPC; the caller should silently skip hint application.
func (c *AccountPlatformClient) GetSchedulingHints(
	ctx context.Context,
	accounts []*pb.SchedulingHintAccount,
	gatewayProtocol string,
	requestedModel string,
) (*pb.GetSchedulingHintsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, accountSchedulingHintsTimeout)
	defer cancel()
	return c.stub.GetSchedulingHints(ctx, &pb.GetSchedulingHintsRequest{
		Accounts:        accounts,
		GatewayProtocol: gatewayProtocol,
		RequestedModel:  requestedModel,
	})
}

const accountSchedulabilityTimeout = 3 * time.Second

// CheckSchedulability asks the plugin whether the given account is
// schedulable. Returns the response or an error (including
// codes.Unimplemented when the plugin does not implement this check).
func (c *AccountPlatformClient) CheckSchedulability(
	ctx context.Context,
	accountID int64,
	platform string,
	credentials, extra json.RawMessage,
	requestedModel, gatewayProtocol string,
) (*pb.CheckSchedulabilityResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, accountSchedulabilityTimeout)
	defer cancel()
	return c.stub.CheckSchedulability(ctx, &pb.CheckSchedulabilityRequest{
		AccountId:       accountID,
		AccountPlatform: platform,
		CredentialsJson: credentials,
		ExtraJson:       extra,
		RequestedModel:  requestedModel,
		GatewayProtocol: gatewayProtocol,
	})
}
