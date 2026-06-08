// Package maintenance implements the MaintenanceExtension gRPC server for
// the channel-management plugin. The host's OpsCleanupService calls
// RunMaintenance once per day on the leader replica; the plugin delegates
// to ChannelMonitorService.RunDailyMaintenanceWithResults and reports
// per-task outcomes back to the host for unified logging.
//
// The server is constructed at process startup (before plugin.Init) so it
// can register on the SDK's grpc.Server. The monitor service dependency is
// wired lazily via SetMonitorService once Init has built the service. RPCs
// that arrive before wiring return an empty response (no tasks) so the host
// treats the plugin as "nothing to clean up yet".
package maintenance

import (
	"context"

	pluginsdkpb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	monitorservice "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"
	"google.golang.org/grpc"
)

// Server implements pluginsdkpb.MaintenanceExtensionServer for the
// channel-management plugin.
type Server struct {
	pluginsdkpb.UnimplementedMaintenanceExtensionServer
	monitorSvc *monitorservice.ChannelMonitorService
}

// NewServer returns a Server with no monitor service wired. Call
// SetMonitorService from Plugin.Init once the service is available.
func NewServer() *Server {
	return &Server{}
}

// SetMonitorService wires the monitor service that owns the daily
// maintenance logic. Must be called from Init before the first
// RunMaintenance RPC lands.
func (s *Server) SetMonitorService(svc *monitorservice.ChannelMonitorService) {
	s.monitorSvc = svc
}

// Register attaches this server to the plugin's gRPC server so the host
// can invoke RunMaintenance.
func (s *Server) Register(srv *grpc.Server) {
	pluginsdkpb.RegisterMaintenanceExtensionServer(srv, s)
}

// RunMaintenance delegates to the monitor service's structured daily
// maintenance and translates the results into the proto response. If the
// monitor service is not yet wired, returns an empty response.
func (s *Server) RunMaintenance(
	ctx context.Context,
	_ *pluginsdkpb.RunMaintenanceRequest,
) (*pluginsdkpb.RunMaintenanceResponse, error) {
	if s.monitorSvc == nil {
		return &pluginsdkpb.RunMaintenanceResponse{}, nil
	}
	results := s.monitorSvc.RunDailyMaintenanceWithResults(ctx)
	pbResults := make([]*pluginsdkpb.MaintenanceTaskResult, 0, len(results))
	for _, r := range results {
		pbResults = append(pbResults, &pluginsdkpb.MaintenanceTaskResult{
			Task:         r.Task,
			DeletedCount: r.DeletedCount,
			Error:        r.Error,
		})
	}
	return &pluginsdkpb.RunMaintenanceResponse{Results: pbResults}, nil
}
