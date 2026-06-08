package plugin

import (
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/gateway"
)

// tryStartContentInterceptExtension unconditionally creates a
// PluginContentInterceptor for the plugin and registers it with the
// GatewayPipeline. No probe RPC is issued; codes.Unimplemented is
// handled at call-time in PluginContentInterceptor.Check.
func (m *PluginManager) tryStartContentInterceptExtension(inst *PluginInstance) {
	if m.pipeline == nil {
		return
	}

	inst.mu.Lock()
	conn := inst.GRPCConn
	inst.mu.Unlock()
	if conn == nil {
		return
	}

	interceptor := gateway.NewPluginContentInterceptor(inst.Name, conn)
	m.pipeline.SetContentInterceptor(interceptor)

	slog.Info("plugin content intercept extension attached", "plugin", inst.Name)
}
