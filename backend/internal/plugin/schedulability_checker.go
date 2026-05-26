package plugin

import (
	"context"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pluginSchedulabilityChecker implements service.PluginSchedulabilityChecker
// by delegating to the plugin's CheckSchedulability gRPC RPC via the
// PlatformRegistry.
type pluginSchedulabilityChecker struct {
	registry *PlatformRegistry
}

// NewPluginSchedulabilityChecker creates a PluginSchedulabilityChecker
// backed by the given PlatformRegistry.
func NewPluginSchedulabilityChecker(registry *PlatformRegistry) service.PluginSchedulabilityChecker {
	return &pluginSchedulabilityChecker{registry: registry}
}

// Check delegates to the plugin's CheckSchedulability RPC.
func (c *pluginSchedulabilityChecker) Check(
	ctx context.Context,
	account *service.Account,
	requestedModel, gatewayProtocol string,
) (bool, string, error) {
	client, err := ClientForPlatform(c.registry, account.Platform)
	if err != nil {
		return true, "", err
	}
	creds, _ := json.Marshal(account.Credentials)
	extra, _ := json.Marshal(account.Extra)
	resp, err := client.CheckSchedulability(
		ctx, account.ID, account.Platform,
		creds, extra, requestedModel, gatewayProtocol,
	)
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.Unimplemented {
			return true, "", err
		}
		return true, "", err
	}
	return resp.GetSchedulable(), resp.GetReason(), nil
}
