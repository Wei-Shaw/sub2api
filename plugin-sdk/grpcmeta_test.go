package pluginsdk

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// fakeUnaryInvoker captures the outgoing metadata so tests can assert what
// the interceptor appended.
type fakeUnaryInvoker struct {
	got metadata.MD
}

func (f *fakeUnaryInvoker) invoke(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		f.got = md
	}
	return nil
}

func TestAppendOutgoingMetadataUnary_Appends(t *testing.T) {
	f := &fakeUnaryInvoker{}
	ic := AppendOutgoingMetadataUnary(CallerMetadataKey, "channel-management")
	if err := ic(context.Background(), "/svc/Method", nil, nil, nil, f.invoke); err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
	if got := f.got.Get(CallerMetadataKey); len(got) != 1 || got[0] != "channel-management" {
		t.Errorf("metadata mismatch: got=%v", got)
	}
}

func TestAppendOutgoingMetadataUnary_EmptyValueIsNoOp(t *testing.T) {
	// Caller wiring may not have the plugin name yet; the factory must not
	// crash and must not stamp an empty header (which the host would
	// reject as an anonymous caller).
	f := &fakeUnaryInvoker{}
	ic := AppendOutgoingMetadataUnary(CallerMetadataKey, "")
	if err := ic(context.Background(), "/svc/Method", nil, nil, nil, f.invoke); err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
	if got := f.got.Get(CallerMetadataKey); len(got) != 0 {
		t.Errorf("empty value must not stamp metadata, got=%v", got)
	}
}

func TestAppendOutgoingMetadataUnary_PreservesOtherMetadata(t *testing.T) {
	// Pre-existing metadata (from a previously chained interceptor like
	// the traceparent one) must survive this layer untouched.
	f := &fakeUnaryInvoker{}
	parent := metadata.AppendToOutgoingContext(context.Background(), "traceparent", "00-aabb-ccdd-01")

	ic := AppendOutgoingMetadataUnary(CallerMetadataKey, "p1")
	if err := ic(parent, "/svc/Method", nil, nil, nil, f.invoke); err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
	if got := f.got.Get("traceparent"); len(got) != 1 || got[0] != "00-aabb-ccdd-01" {
		t.Errorf("traceparent stripped: %v", got)
	}
	if got := f.got.Get(CallerMetadataKey); len(got) != 1 || got[0] != "p1" {
		t.Errorf("caller metadata missing: %v", got)
	}
}

func TestAppendOutgoingMetadataUnary_PropagatesInvokerError(t *testing.T) {
	want := errors.New("rpc failed")
	ic := AppendOutgoingMetadataUnary("x-test", "v")
	got := ic(context.Background(), "/svc/M", nil, nil, nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			return want
		})
	if !errors.Is(got, want) {
		t.Errorf("invoker error must propagate: got=%v want=%v", got, want)
	}
}

// fakeStreamer captures the outgoing metadata for streaming-side assertions.
type fakeStreamer struct {
	got metadata.MD
}

func (f *fakeStreamer) call(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		f.got = md
	}
	return nil, nil
}

func TestAppendOutgoingMetadataStream_Appends(t *testing.T) {
	f := &fakeStreamer{}
	ic := AppendOutgoingMetadataStream(CallerMetadataKey, "monitor")
	if _, err := ic(context.Background(), &grpc.StreamDesc{}, nil, "/svc/Stream", f.call); err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
	if got := f.got.Get(CallerMetadataKey); len(got) != 1 || got[0] != "monitor" {
		t.Errorf("metadata mismatch: got=%v", got)
	}
}

func TestAppendOutgoingMetadataStream_EmptyKeyIsNoOp(t *testing.T) {
	f := &fakeStreamer{}
	ic := AppendOutgoingMetadataStream("", "ignored")
	if _, err := ic(context.Background(), &grpc.StreamDesc{}, nil, "/svc/Stream", f.call); err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
	if len(f.got) != 0 {
		t.Errorf("empty key must not stamp anything, got=%v", f.got)
	}
}
