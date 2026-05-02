// interceptors.go centralises the gRPC caller-identity gate that polices the
// SDK service surface. Before this file every server implementation rolled
// its own resolveCaller + reject-empty + error-mapping snippet, with three
// independent failure modes:
//
//   - the legacy RedisProxy / EventBus methods accepted anonymous callers
//     because their handlers never bothered to check;
//   - LogProxy.PushLogs degraded an unauthenticated stream into a "plugin.unknown"
//     logger instead of refusing it;
//   - error codes drifted between PermissionDenied / Unauthenticated / raw errors.
//
// We collapse all three concerns into a pair of grpc interceptors that run
// once per RPC, write the resolved name into context, and let downstream
// handlers retrieve it through CallerFromContext. Empty / unknown callers
// short-circuit with PermissionDenied so individual handlers no longer carry
// the rejection responsibility.
//
// The design also exposes errService / errInternal helpers so handlers stop
// returning bare errors.New / fmt.Errorf wrappers — every plugin-facing error
// now carries a stable gRPC code.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// CallerResolver maps an incoming gRPC context to the plugin name. The
// production resolver (SDKServer.resolveCaller) reads the x-sub2api-plugin
// metadata header and validates the value against the registered plugin set.
// Tests inject stubs that return a constant name.
type CallerResolver func(context.Context) string

// callerCtxKeyType is unexported so external packages cannot read or write
// the caller key directly — the only way in is through WithCaller, which the
// interceptor below owns.
type callerCtxKeyType struct{}

var callerCtxKey = callerCtxKeyType{}

// WithCaller attaches a resolved plugin name to ctx. Exported so testing
// helpers and the streaming wrapper below can build a child context once the
// interceptor has authorised the caller.
func WithCaller(ctx context.Context, name string) context.Context {
	if name == "" {
		return ctx
	}
	return context.WithValue(ctx, callerCtxKey, name)
}

// CallerFromContext returns the plugin name attached by the identity
// interceptor. Returns ("", false) when no caller has been authorised.
//
// As a defensive fallback for code paths that bypass the interceptor (e.g.
// in-process unit tests that call handlers directly on a freshly constructed
// server), the function reads the raw metadata header when no value has been
// stamped onto ctx. The fallback never validates against knownPluginsRegistry
// — only the interceptor performs that check, so anything skipping it must
// run with a controlled set of callers.
func CallerFromContext(ctx context.Context) (string, bool) {
	if v, ok := ctx.Value(callerCtxKey).(string); ok && v != "" {
		return v, true
	}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get(callerMetadataKey); len(vals) > 0 && vals[0] != "" {
			return vals[0], true
		}
	}
	return "", false
}

// RequirePluginIdentityUnary returns a UnaryServerInterceptor that resolves
// the caller via the supplied resolver and rejects anonymous calls with
// PermissionDenied. The authorised name is stamped onto ctx so handlers can
// retrieve it through CallerFromContext without re-running the resolver.
func RequirePluginIdentityUnary(resolver CallerResolver) grpc.UnaryServerInterceptor {
	if resolver == nil {
		// Defensive: a nil resolver would deny every call. Surface the
		// configuration bug to operators by logging once and rejecting at
		// runtime — preferable to silently allowing every caller through.
		resolver = func(context.Context) string { return "" }
	}
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		name := resolver(ctx)
		if name == "" {
			return nil, status.Error(codes.PermissionDenied, "anonymous caller rejected")
		}
		return handler(WithCaller(ctx, name), req)
	}
}

// RequirePluginIdentityStream is the streaming counterpart of
// RequirePluginIdentityUnary. The wrapped ServerStream serves the augmented
// context to the handler so any nested call to CallerFromContext resolves to
// the validated plugin name.
func RequirePluginIdentityStream(resolver CallerResolver) grpc.StreamServerInterceptor {
	if resolver == nil {
		resolver = func(context.Context) string { return "" }
	}
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		name := resolver(ctx)
		if name == "" {
			return status.Error(codes.PermissionDenied, "anonymous caller rejected")
		}
		return handler(srv, &callerServerStream{ServerStream: ss, ctx: WithCaller(ctx, name)})
	}
}

// callerServerStream wraps grpc.ServerStream so handler.Context() returns the
// caller-augmented context. Patterned after the wrappedServerStream helper in
// grpc-ecosystem/go-grpc-middleware — duplicated rather than imported because
// pulling that whole module just for a 10-line wrapper is overkill.
type callerServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *callerServerStream) Context() context.Context { return s.ctx }

// errService returns codes.Unavailable for "host resource not configured"
// failures (nil DB, nil Redis client, etc.). The message names the resource
// so operators see "redis service not configured" rather than a generic
// "service not configured".
func errService(name string) error {
	return status.Errorf(codes.Unavailable, "%s service not configured", name)
}

// errInternal wraps an arbitrary host-side error as codes.Internal with the
// operation name as a prefix. Use this for things that should never happen in
// a healthy host (DB connection drop mid-query, scan failures, etc.).
//
// When err is a *pq.Error the SQLSTATE code, constraint name and column name
// are preserved in a structured prefix "pq[CODE,constraint=...,column=...]:"
// so the plugin can classify the error without depending on fragile message
// text parsing. Sensitive fields (Detail, Where, InternalQuery) are omitted.
func errInternal(err error, op string) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return status.Errorf(codes.Internal, "%s: %s", op, formatPQForGRPC(pqErr))
	}
	return status.Errorf(codes.Internal, "%s: %v", op, err)
}

// formatPQForGRPC builds a structured-but-parseable error string from a pq.Error.
// Format: "pq[SQLSTATE,constraint=name,column=name]: message"
// Only non-empty metadata fields are included in the bracket section.
func formatPQForGRPC(e *pq.Error) string {
	bracket := string(e.Code)
	if e.Constraint != "" {
		bracket += ",constraint=" + e.Constraint
	}
	if e.Column != "" {
		bracket += ",column=" + e.Column
	}
	if e.Table != "" {
		bracket += ",table=" + e.Table
	}
	return fmt.Sprintf("pq[%s]: %s", bracket, e.Message)
}

// loggerOrDefault collapses the "if logger == nil { logger = slog.Default() }"
// boilerplate that was duplicated across every server constructor. Keeping
// it here means a single place owns the fallback policy.
func loggerOrDefault(l *slog.Logger) *slog.Logger {
	if l != nil {
		return l
	}
	return slog.Default()
}
