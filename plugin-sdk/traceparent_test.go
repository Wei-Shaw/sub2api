package pluginsdk

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const validTraceparentFixture = "00-0123456789abcdef0123456789abcdef-fedcba9876543210-01"

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context { return s.ctx }

func TestIsValidTraceparent(t *testing.T) {
	cases := []struct {
		name string
		tp   string
		want bool
	}{
		{name: "valid", tp: "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01", want: true},
		{name: "valid with mixed flags", tp: "00-0123456789abcdef0123456789abcdef-fedcba9876543210-00", want: true},
		{name: "empty", tp: "", want: false},
		{name: "too short", tp: "00-aaa-bbb-01", want: false},
		{name: "wrong version", tp: "ff-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01", want: false},
		{name: "uppercase hex (W3C requires lowercase)", tp: "00-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA-bbbbbbbbbbbbbbbb-01", want: false},
		{name: "all-zero trace id", tp: "00-00000000000000000000000000000000-bbbbbbbbbbbbbbbb-01", want: false},
		{name: "all-zero span id", tp: "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-0000000000000000-01", want: false},
		{name: "non-hex char", tp: "00-zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz-bbbbbbbbbbbbbbbb-01", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidTraceparent(tc.tp); got != tc.want {
				t.Fatalf("IsValidTraceparent(%q) = %v, want %v", tc.tp, got, tc.want)
			}
		})
	}
}

func TestNewTraceparentShape(t *testing.T) {
	tp := NewTraceparent()
	if !IsValidTraceparent(tp) {
		t.Fatalf("NewTraceparent produced invalid value %q", tp)
	}
	parts := strings.Split(tp, "-")
	if parts[0] != traceparentSupportedVersion {
		t.Fatalf("version = %q, want %q", parts[0], traceparentSupportedVersion)
	}
	if parts[3] != "01" {
		t.Fatalf("flags = %q, want sampled=01", parts[3])
	}
}

func TestNewTraceparentUniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		tp := NewTraceparent()
		if _, dup := seen[tp]; dup {
			t.Fatalf("duplicate traceparent: %s", tp)
		}
		seen[tp] = struct{}{}
	}
}

func TestTraceparentUnaryServer_ValidPassThrough(t *testing.T) {
	md := metadata.Pairs(MetadataTraceparentKey, validTraceparentFixture)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var observed string
	handler := func(ctx context.Context, _ any) (any, error) {
		tp, ok := TraceparentFromContext(ctx)
		if !ok {
			t.Fatalf("traceparent not stamped onto handler ctx")
		}
		observed = tp
		return nil, nil
	}

	if _, err := TraceparentUnaryServer()(ctx, nil, &grpc.UnaryServerInfo{}, handler); err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
	if observed != validTraceparentFixture {
		t.Fatalf("traceparent = %q, want %q", observed, validTraceparentFixture)
	}
}

func TestTraceparentUnaryServer_InvalidGetsRewritten(t *testing.T) {
	md := metadata.Pairs(MetadataTraceparentKey, "garbage")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var observed string
	handler := func(ctx context.Context, _ any) (any, error) {
		tp, _ := TraceparentFromContext(ctx)
		observed = tp
		return nil, nil
	}
	if _, err := TraceparentUnaryServer()(ctx, nil, &grpc.UnaryServerInfo{}, handler); err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
	if observed == "" {
		t.Fatalf("expected fresh traceparent, got empty")
	}
	if observed == "garbage" {
		t.Fatalf("invalid traceparent leaked through")
	}
	if !IsValidTraceparent(observed) {
		t.Fatalf("rewritten traceparent invalid: %q", observed)
	}
}

func TestTraceparentUnaryServer_MissingGetsGenerated(t *testing.T) {
	var observed string
	handler := func(ctx context.Context, _ any) (any, error) {
		tp, _ := TraceparentFromContext(ctx)
		observed = tp
		return nil, nil
	}
	if _, err := TraceparentUnaryServer()(context.Background(), nil, &grpc.UnaryServerInfo{}, handler); err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
	if !IsValidTraceparent(observed) {
		t.Fatalf("generated traceparent invalid: %q", observed)
	}
}

func TestTraceparentStreamServer_StampsCtx(t *testing.T) {
	md := metadata.Pairs(MetadataTraceparentKey, validTraceparentFixture)
	ss := &fakeServerStream{ctx: metadata.NewIncomingContext(context.Background(), md)}

	var observed string
	handler := func(_ any, stream grpc.ServerStream) error {
		tp, ok := TraceparentFromContext(stream.Context())
		if !ok {
			return errors.New("traceparent missing on stream ctx")
		}
		observed = tp
		return nil
	}
	if err := TraceparentStreamServer()(nil, ss, &grpc.StreamServerInfo{}, handler); err != nil {
		t.Fatalf("stream interceptor: %v", err)
	}
	if observed != validTraceparentFixture {
		t.Fatalf("traceparent = %q, want %q", observed, validTraceparentFixture)
	}
}

func TestTraceparentUnaryClient_InjectsFromCtx(t *testing.T) {
	ctx := WithTraceparent(context.Background(), validTraceparentFixture)

	var seen []string
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			return errors.New("no outgoing metadata")
		}
		seen = md.Get(MetadataTraceparentKey)
		return nil
	}

	if err := TraceparentUnaryClient()(ctx, "/svc/Method", nil, nil, nil, invoker); err != nil {
		t.Fatalf("client interceptor: %v", err)
	}
	if len(seen) != 1 || seen[0] != validTraceparentFixture {
		t.Fatalf("outgoing traceparent = %v, want [%s]", seen, validTraceparentFixture)
	}
}

func TestTraceparentUnaryClient_GeneratesWhenAbsent(t *testing.T) {
	var seen []string
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		seen = md.Get(MetadataTraceparentKey)
		return nil
	}
	if err := TraceparentUnaryClient()(context.Background(), "/svc/Method", nil, nil, nil, invoker); err != nil {
		t.Fatalf("client interceptor: %v", err)
	}
	if len(seen) != 1 {
		t.Fatalf("expected exactly one outgoing traceparent, got %v", seen)
	}
	if !IsValidTraceparent(seen[0]) {
		t.Fatalf("generated traceparent invalid: %q", seen[0])
	}
}

func TestTraceparentStreamClient_InjectsFromCtx(t *testing.T) {
	ctx := WithTraceparent(context.Background(), validTraceparentFixture)

	var seen []string
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		md, _ := metadata.FromOutgoingContext(ctx)
		seen = md.Get(MetadataTraceparentKey)
		return nil, nil
	}
	if _, err := TraceparentStreamClient()(ctx, &grpc.StreamDesc{}, nil, "/svc/Stream", streamer); err != nil {
		t.Fatalf("stream client interceptor: %v", err)
	}
	if len(seen) != 1 || seen[0] != validTraceparentFixture {
		t.Fatalf("outgoing traceparent = %v, want [%s]", seen, validTraceparentFixture)
	}
}

func TestTraceparentRoundTripServerToClient(t *testing.T) {
	md := metadata.Pairs(MetadataTraceparentKey, validTraceparentFixture)
	inbound := metadata.NewIncomingContext(context.Background(), md)

	var outbound []string
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		outbound = md.Get(MetadataTraceparentKey)
		return nil
	}
	handler := func(ctx context.Context, _ any) (any, error) {
		return nil, TraceparentUnaryClient()(ctx, "/svc/Out", nil, nil, nil, invoker)
	}
	if _, err := TraceparentUnaryServer()(inbound, nil, &grpc.UnaryServerInfo{}, handler); err != nil {
		t.Fatalf("interceptor chain: %v", err)
	}
	if len(outbound) != 1 || outbound[0] != validTraceparentFixture {
		t.Fatalf("trace did not round-trip: outbound=%v", outbound)
	}
}

func TestWithTraceparent_EmptyShortCircuits(t *testing.T) {
	ctx := WithTraceparent(context.Background(), "")
	if _, ok := TraceparentFromContext(ctx); ok {
		t.Fatalf("empty traceparent should not be stamped")
	}
}
