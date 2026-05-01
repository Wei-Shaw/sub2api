package pluginsdk

// CallerMetadataKey is the gRPC metadata header the SDK client attaches to
// every outgoing call so the host server can identify which plugin issued it.
//
// Both the SDK runner (plugin-sdk/runner.go) and the host's plugin manager
// (backend/internal/plugin/grpc_server_redis_do.go, interceptors.go) MUST use
// the same string. Exposing the constant from the SDK package makes it the
// single source of truth — host code imports this symbol rather than
// duplicating the literal.
//
// The trust model: the operator installed the plugin binary; we trust it to
// honour this metadata convention. Capability enforcement still applies
// because the host registry only contains plugins the operator has
// explicitly approved at Init time.
const CallerMetadataKey = "x-sub2api-plugin"
