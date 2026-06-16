package plugin

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

const maintenanceRunTimeout = 5 * time.Minute

// MaintenanceExtensionClient wraps the gRPC stub for the plugin's
// MaintenanceExtension service. Unlike PricingExtensionClient there are no
// watch loops or caches — the host calls RunMaintenance once per
// OpsCleanupService cycle (typically daily).
type MaintenanceExtensionClient struct {
	pluginName string
	stub       pluginsdk.MaintenanceExtensionClient
	logger     *slog.Logger
}

// NewMaintenanceExtensionClient creates a client for the given plugin's
// MaintenanceExtension gRPC service. The caller is responsible for ensuring
// conn is alive for the lifetime of the client.
func NewMaintenanceExtensionClient(
	pluginName string,
	conn grpc.ClientConnInterface,
	logger *slog.Logger,
) *MaintenanceExtensionClient {
	return &MaintenanceExtensionClient{
		pluginName: pluginName,
		stub:       pluginsdk.NewMaintenanceExtensionClient(conn),
		logger:     logger,
	}
}

// RunMaintenance calls the plugin's RunMaintenance RPC. If the plugin does
// not implement MaintenanceExtension the call returns codes.Unimplemented;
// the caller (RunPluginMaintenance) handles that gracefully.
func (c *MaintenanceExtensionClient) RunMaintenance(
	ctx context.Context,
	nowUnix int64,
) ([]*pluginsdk.MaintenanceTaskResult, error) {
	ctx, cancel := context.WithTimeout(ctx, maintenanceRunTimeout)
	defer cancel()

	resp, err := c.stub.RunMaintenance(ctx, &pluginsdk.RunMaintenanceRequest{
		NowUnix: nowUnix,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetResults(), nil
}

// isUnimplemented returns true when err represents gRPC codes.Unimplemented.
func isUnimplemented(err error) bool {
	return status.Code(err) == codes.Unimplemented
}
