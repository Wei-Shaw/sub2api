package plugin

import (
	"context"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pluginModelSupportChecker implements service.PluginModelSupportChecker
// by delegating to the plugin's IsModelSupported gRPC RPC via the
// PlatformRegistry.
type pluginModelSupportChecker struct {
	registry *PlatformRegistry
}

// NewPluginModelSupportChecker creates a PluginModelSupportChecker backed
// by the given PlatformRegistry. The checker looks up the plugin connection
// for the account's platform and calls the IsModelSupported RPC.
func NewPluginModelSupportChecker(registry *PlatformRegistry) service.PluginModelSupportChecker {
	return &pluginModelSupportChecker{registry: registry}
}

// Check delegates to the plugin's IsModelSupported RPC. Returns an error
// if the platform is not registered or the plugin returns Unimplemented.
func (c *pluginModelSupportChecker) Check(
	ctx context.Context,
	account *service.Account,
	requestedModel string,
) (bool, error) {
	client, err := ClientForPlatform(c.registry, account.Platform)
	if err != nil {
		return false, err
	}
	creds, _ := json.Marshal(account.Credentials)
	extra, _ := json.Marshal(account.Extra)
	resp, err := client.IsModelSupported(
		ctx, account.ID, account.Platform,
		creds, extra, requestedModel,
	)
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.Unimplemented {
			return false, err
		}
		return false, err
	}
	return resp.GetSupported(), nil
}
