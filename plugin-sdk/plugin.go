// Package pluginsdk provides the SDK that out-of-process plugins use to
// integrate with the Sub2API core. A plugin implements the Plugin interface
// and starts itself with pluginsdk.Run, which handles the gRPC handshake
// with the core, exposes the plugin's HTTP endpoints, and wires up SQL/Redis
// proxies so the plugin can use the core's connection pools transparently.
package pluginsdk

import "google.golang.org/grpc"

// Plugin is the contract every plugin binary implements.
//
// The lifecycle is:
//  1. The core process spawns the plugin binary with --core-sdk-addr=<addr>.
//  2. The plugin's main calls pluginsdk.Run(p), which serves a gRPC
//     PluginLifecycle service.
//  3. The core calls Init via gRPC; the SDK builds a PluginContext backed by
//     the core's SDK address and invokes Plugin.Init(ctx).
//  4. The core may call GetManifest, HealthCheck, GetFrontendBundle, then
//     finally Shutdown.
//  5. On Shutdown the SDK calls Plugin.Shutdown and exits the process.
type Plugin interface {
	// Manifest returns the static description of the plugin: routes, menu
	// items, frontend assets, etc. It is called both before Init (for
	// validation by the core) and may be called again after Init.
	Manifest() *Manifest

	// Init is invoked once after the gRPC handshake completes. The provided
	// PluginContext gives access to the core DB, Redis, logger, and config.
	// Returning an error causes Init to fail at the gRPC layer; the core may
	// then terminate the plugin.
	Init(ctx PluginContext) error

	// Shutdown is invoked when the core asks the plugin to terminate. Plugins
	// should release resources, flush state, and return promptly.
	Shutdown() error
}

// HealthChecker is an optional interface a Plugin may implement to provide
// custom health information. If not implemented, the SDK reports healthy=true.
type HealthChecker interface {
	HealthCheck() (healthy bool, message string)
}

// HTTPRegistrar is an optional interface a Plugin may implement to register
// HTTP handlers on the SDK-managed HTTP server. The SDK passes the underlying
// *http.ServeMux so plugins can mount handlers without pulling in a specific
// web framework.
//
// Routes registered here MUST match the paths declared in Manifest().
type HTTPRegistrar interface {
	RegisterHTTP(mux HTTPMux)
}

// HTTPMux is the minimal mux interface the SDK exposes to plugins. It is
// intentionally narrow so plugins are not coupled to net/http internals.
type HTTPMux interface {
	Handle(pattern string, handler interface{})
}

// FrontendBundleProvider is an optional interface a Plugin may implement to
// serve static frontend assets (compiled JS/CSS) over the GetFrontendBundle
// gRPC stream. The provider is typically backed by an embed.FS.
type FrontendBundleProvider interface {
	OpenFrontendFile(path string) (data []byte, err error)
}

// GRPCServiceRegistrar is an optional interface a Plugin may implement to
// register additional gRPC services on the SDK-managed grpc.Server. The SDK
// invokes RegisterGRPCServices once, immediately after registering its own
// PluginLifecycle service and before the gRPC listener starts serving — so
// the plugin's services share the same TCP listener / handshake address the
// host already opens for the lifecycle RPCs.
//
// Use this to expose host-callable extensions such as PricingExtension. The
// plugin's gRPC server embeds the SDK's lifecycle methods plus whatever the
// plugin registers here; the host opens a single ClientConn to the plugin
// and dispatches by service name. Plugins that do not need to expose any
// extra services can simply omit this interface (zero overhead).
//
// The host detects whether a plugin implements a particular extension by
// invoking it lazily — gRPC returns codes.Unimplemented when the plugin
// does not register the service, which the host treats as "not available"
// rather than an error.
type GRPCServiceRegistrar interface {
	RegisterGRPCServices(server *grpc.Server)
}

// MigrationProvider is an optional interface a Plugin may implement to ship
// SQL migration files. The host fetches each declared file body via
// PluginLifecycle.GetMigration; the SDK runner dispatches that RPC to
// OpenMigration. The provider is typically backed by an embed.FS rooted at
// the plugin's migrations/ directory.
//
// `filename` is the same string the plugin declared in
// Manifest.Migrations[*].Filename and matches the SQL file name on disk
// (e.g. "001_create_x.sql"). Implementations should return fs.ErrNotExist
// for unknown filenames so callers can distinguish a missing file from a
// generic read error.
//
// A plugin that declares Manifest.Migrations MUST implement this interface;
// the SDK returns codes.FailedPrecondition for GetMigration calls that
// arrive at a plugin without a MigrationProvider, surfacing the wiring bug
// instead of letting the host see codes.Unimplemented and skip the
// migrations silently.
type MigrationProvider interface {
	OpenMigration(filename string) (data []byte, err error)
}
