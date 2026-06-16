package plugin

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
)

// =============================================================
// Maintenance extension — spawn-side wiring + cleanup-time dispatch
//
// Unlike PricingExtension there is no probe at spawn time: we
// unconditionally create the client for every plugin that has a gRPC
// connection. If the plugin does not implement MaintenanceExtension the
// RunMaintenance RPC returns codes.Unimplemented at cleanup time, which
// RunPluginMaintenance handles by silently skipping — no warn log, no
// retry. This avoids running actual maintenance during the spawn path.
//
// Lifecycle pairing:
//   - tryStartMaintenanceExtension (spawn) → detachMaintenanceClient (stop)
//   - RunPluginMaintenance is called by OpsCleanupService at the end of
//     its cleanup cycle (not during spawn).
// =============================================================

// tryStartMaintenanceExtension unconditionally creates a
// MaintenanceExtensionClient for the plugin. No probe RPC is issued;
// codes.Unimplemented is handled at call-time in RunPluginMaintenance.
func (m *PluginManager) tryStartMaintenanceExtension(_ context.Context, inst *PluginInstance) {
	inst.mu.Lock()
	conn := inst.GRPCConn
	inst.mu.Unlock()
	if conn == nil {
		return
	}

	client := NewMaintenanceExtensionClient(
		inst.Name,
		grpc.ClientConnInterface(conn),
		slog.Default().With("plugin", inst.Name, "extension", "maintenance"),
	)

	inst.mu.Lock()
	inst.maintenanceClient = client
	inst.mu.Unlock()

	slog.Info("plugin maintenance extension attached", "plugin", inst.Name)
}

// detachMaintenanceClient releases the maintenance client reference.
// Called from detachExtensions during plugin stop.
func (m *PluginManager) detachMaintenanceClient(client *MaintenanceExtensionClient) {
	if client == nil {
		return
	}
	slog.Info("plugin maintenance extension detached", "plugin", client.pluginName)
}

// RunPluginMaintenance iterates all active plugin instances that have a
// maintenance client and calls RunMaintenance on each. Called by
// OpsCleanupService at the end of its cleanup cycle.
//
// codes.Unimplemented is silently skipped (plugin does not implement the
// service). Any other error is logged as a warning but does not fail the
// overall cleanup.
func (m *PluginManager) RunPluginMaintenance(ctx context.Context, nowUnix int64) {
	clients := m.collectMaintenanceClients()
	for _, c := range clients {
		m.runOnePluginMaintenance(ctx, c, nowUnix)
	}
}

// collectMaintenanceClients snapshots the set of non-nil maintenance
// clients under the manager and instance locks.
func (m *PluginManager) collectMaintenanceClients() []*MaintenanceExtensionClient {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var clients []*MaintenanceExtensionClient
	for _, inst := range m.plugins {
		inst.mu.Lock()
		c := inst.maintenanceClient
		inst.mu.Unlock()
		if c != nil {
			clients = append(clients, c)
		}
	}
	return clients
}

// runOnePluginMaintenance calls RunMaintenance on a single plugin and
// logs results. Unimplemented is silently skipped.
func (m *PluginManager) runOnePluginMaintenance(
	ctx context.Context,
	c *MaintenanceExtensionClient,
	nowUnix int64,
) {
	results, err := c.RunMaintenance(ctx, nowUnix)
	if err != nil {
		if isUnimplemented(err) {
			return
		}
		slog.Warn("plugin maintenance failed",
			"plugin", c.pluginName, "error", err)
		return
	}
	for _, r := range results {
		if r.GetError() != "" {
			slog.Warn("plugin maintenance task failed",
				"plugin", c.pluginName,
				"task", r.GetTask(),
				"error", r.GetError())
		} else {
			slog.Info("plugin maintenance task completed",
				"plugin", c.pluginName,
				"task", r.GetTask(),
				"deleted", r.GetDeletedCount())
		}
	}
}
