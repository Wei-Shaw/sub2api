// traceparent.go is the single source of truth for W3C trace-context
// propagation across the host ↔ plugin boundary. Both sides import the
// same symbols from here so a trace id minted on either end traverses
// the wire without translation.
//
// What lives here:
//
//   - format helpers (validation + minting) for W3C traceparent v00
//   - ctx key + WithTraceparent / TraceparentFromContext accessors
//   - gRPC client / server interceptor pairs that read traceparent from
//     metadata, stamp it onto ctx, and inject it on outbound calls
//
// Host-only concerns (e.g. caller-identity authorisation) are NOT here —
// they stay in backend/internal/plugin/interceptors.go because the SDK
// has no business enforcing host-side policy. By the same token the
// plugin SDK should NOT learn about caller authorisation; trace and
// auth are independent interceptor layers and must compose cleanly.
package pluginsdk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// MetadataTraceparentKey is the gRPC metadata header (and HTTP header) used
// to carry the W3C traceparent. gRPC normalises metadata keys to lower-case
// on the wire, so the constant stays lower-case to keep HTTP-side and
// gRPC-side code matching the same wire identifier.
const MetadataTraceparentKey = "traceparent"

// W3C traceparent format: version "-" trace-id "-" parent-id "-" trace-flags.
// version=2 hex, trace-id=32 hex (16 bytes), parent-id=16 hex (8 bytes),
// trace-flags=2 hex. Total 55 chars including dashes. Only version "00".
//
// Reference: https://www.w3.org/TR/trace-context/#traceparent-header
const (
	traceparentSupportedVersion = "00"
	traceparentLength           = 55 // 2 + 1 + 32 + 1 + 16 + 1 + 2
	traceIDHexLen               = 32
	spanIDHexLen                = 16
)

// invalidTraceID and invalidSpanID are the zero ids defined by W3C; values
// equal to them MUST be rejected as invalid.
var (
	invalidTraceID = strings.Repeat("0", traceIDHexLen)
	invalidSpanID  = strings.Repeat("0", spanIDHexLen)
)

// traceparentCtxKeyType is unexported so external packages cannot read or
// write the value directly — propagation only flows through WithTraceparent
// + TraceparentFromContext, keeping the host and gRPC interceptor layer in
// agreement about how the W3C traceparent travels across context boundaries.
type traceparentCtxKeyType struct{}

var traceparentCtxKey = traceparentCtxKeyType{}

// WithTraceparent attaches a W3C traceparent string to ctx. The value is
// not validated here — callers are expected to feed only output from
// NewTraceparent or a previously-validated header. Empty values short-circuit
// to avoid stamping a useless key onto the context tree.
func WithTraceparent(ctx context.Context, tp string) context.Context {
	if tp == "" {
		return ctx
	}
	return context.WithValue(ctx, traceparentCtxKey, tp)
}

// TraceparentFromContext returns the traceparent attached by WithTraceparent
// (or by the gRPC server interceptor that wraps inbound RPCs). Returns
// ("", false) when no value has been attached so callers can decide whether
// to generate a fresh one.
func TraceparentFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	tp, ok := ctx.Value(traceparentCtxKey).(string)
	return tp, ok && tp != ""
}

// IsValidTraceparent reports whether tp matches the W3C traceparent format
// for version 00 and contains non-zero trace/span ids.
//
// Anything else (wrong length, unknown version, all-zero ids, non-hex chars)
// is treated as invalid; callers should generate a fresh traceparent rather
// than propagate a malformed value.
func IsValidTraceparent(tp string) bool {
	if len(tp) != traceparentLength {
		return false
	}
	parts := strings.Split(tp, "-")
	if len(parts) != 4 {
		return false
	}
	version, traceID, spanID, flags := parts[0], parts[1], parts[2], parts[3]
	if version != traceparentSupportedVersion {
		return false
	}
	if len(traceID) != traceIDHexLen || len(spanID) != spanIDHexLen || len(flags) != 2 {
		return false
	}
	if traceID == invalidTraceID || spanID == invalidSpanID {
		return false
	}
	if !isLowerHex(traceID) || !isLowerHex(spanID) || !isLowerHex(flags) {
		return false
	}
	return true
}

// NewTraceparent creates a fresh W3C traceparent. trace-flags is fixed to 01
// (sampled=true) — we ask plugins to log/trace everything, sampling decisions
// happen at the host level.
//
// Returns "" only if crypto/rand fails, which is unrecoverable; callers
// already treat empty as "no traceparent" and skip propagation.
func NewTraceparent() string {
	var traceID [16]byte
	var spanID [8]byte
	if _, err := rand.Read(traceID[:]); err != nil {
		return ""
	}
	if _, err := rand.Read(spanID[:]); err != nil {
		return ""
	}
	return traceparentSupportedVersion + "-" +
		hex.EncodeToString(traceID[:]) + "-" +
		hex.EncodeToString(spanID[:]) + "-01"
}

func isLowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}

// resolveIncomingTraceparent reads a traceparent from gRPC incoming metadata
// and validates it against the W3C format. Missing or malformed values are
// replaced with a freshly generated traceparent so downstream handlers always
// observe a usable value. The boolean reports whether a fresh value was
// generated; callers may use it to log a debug entry when rewriting.
func resolveIncomingTraceparent(ctx context.Context) (string, bool) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get(MetadataTraceparentKey); len(vals) > 0 {
			if tp := vals[0]; IsValidTraceparent(tp) {
				return tp, false
			}
		}
	}
	return NewTraceparent(), true
}

// TraceparentUnaryServer returns a UnaryServerInterceptor that ensures every
// inbound RPC carries a valid traceparent on its context. Existing values in
// metadata pass through; missing or malformed values are replaced with a
// freshly-generated id so plugin → host (or host → plugin) calls cannot break
// the trace chain.
//
// The interceptor stamps the resolved traceparent onto ctx via WithTraceparent
// so handlers (and any outbound RPC chained from this ctx) observe the same
// id without re-parsing metadata.
func TraceparentUnaryServer() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		tp, _ := resolveIncomingTraceparent(ctx)
		return handler(WithTraceparent(ctx, tp), req)
	}
}

// TraceparentStreamServer is the streaming counterpart of TraceparentUnaryServer.
// It wraps the ServerStream so handler.Context() returns the trace-augmented
// context — required because grpc.ServerStream caches its own context internally
// and ignores any value attached after the stream is created.
func TraceparentStreamServer() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		tp, _ := resolveIncomingTraceparent(ss.Context())
		wrapped := &traceparentServerStream{ServerStream: ss, ctx: WithTraceparent(ss.Context(), tp)}
		return handler(srv, wrapped)
	}
}

// traceparentServerStream wraps grpc.ServerStream so the trace-augmented ctx
// is what handlers see via stream.Context(). Kept independent from any caller-
// identity wrapper so the chained interceptor pipeline composes cleanly: each
// concern owns its own wrapper without knowing about the other's context key.
type traceparentServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *traceparentServerStream) Context() context.Context { return s.ctx }

// TraceparentUnaryClient returns a UnaryClientInterceptor that propagates the
// caller's traceparent (or generates a new one) into the outgoing gRPC
// metadata. Calls that originate from a request-scoped ctx keep the upstream
// trace; background loops (e.g. watch / resync) get a fresh id so each
// iteration is independently traceable.
func TraceparentUnaryClient() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = injectOutgoingTraceparent(ctx)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// TraceparentStreamClient is the streaming counterpart used by Watch /
// Subscribe-style RPCs.
func TraceparentStreamClient() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx = injectOutgoingTraceparent(ctx)
		return streamer(ctx, desc, cc, method, opts...)
	}
}

// injectOutgoingTraceparent reads the traceparent attached to ctx (if any),
// generating a fresh one when absent, and appends it to the gRPC outgoing
// metadata. The value is also stamped back onto ctx so any nested RPC the
// caller fires off from the same ctx reuses the same id.
func injectOutgoingTraceparent(ctx context.Context) context.Context {
	tp, ok := TraceparentFromContext(ctx)
	if !ok {
		tp = NewTraceparent()
		if tp == "" {
			// crypto/rand failed; skip propagation rather than emit an empty
			// header. Background loops will retry on the next iteration.
			return ctx
		}
		ctx = WithTraceparent(ctx, tp)
	}
	return metadata.AppendToOutgoingContext(ctx, MetadataTraceparentKey, tp)
}
