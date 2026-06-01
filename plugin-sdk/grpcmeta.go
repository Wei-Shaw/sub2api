package pluginsdk

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

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

// AppendOutgoingMetadataUnary returns a UnaryClientInterceptor that appends
// (key, value) onto every outbound RPC's metadata. The value is captured at
// factory time so all subsequent calls send the same constant — useful for
// plugin identity, build tags, or any other client-side metadata that does
// not change per call.
//
// The interceptor is the simple inverse of the bundled traceparent client
// interceptors (which read from ctx and inject), exposed as a reusable
// factory so the SDK avoids hand-rolling the same closure for every metadata
// key it needs to stamp on outbound traffic.
//
// Empty value short-circuits to a no-op so callers don't have to guard
// against "I haven't resolved the plugin name yet" cases at the wiring site.
func AppendOutgoingMetadataUnary(key, value string) grpc.UnaryClientInterceptor {
	if key == "" || value == "" {
		return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
	}
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(metadata.AppendToOutgoingContext(ctx, key, value), method, req, reply, cc, opts...)
	}
}

// AppendOutgoingMetadataStream is the streaming counterpart of
// AppendOutgoingMetadataUnary. Both interceptors must be installed when a
// chained client uses both unary and streaming RPCs; otherwise streaming
// RPCs would arrive without the metadata header and the host's identity
// gate would reject them.
func AppendOutgoingMetadataStream(key, value string) grpc.StreamClientInterceptor {
	if key == "" || value == "" {
		return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return streamer(ctx, desc, cc, method, opts...)
		}
	}
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return streamer(metadata.AppendToOutgoingContext(ctx, key, value), desc, cc, method, opts...)
	}
}
