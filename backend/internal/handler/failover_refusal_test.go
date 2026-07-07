package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type noopTempUnscheduler struct{}

func (noopTempUnscheduler) TempUnscheduleRetryableError(context.Context, int64, *service.UpstreamFailoverError) {
}

func refusalErr() *service.UpstreamFailoverError {
	return &service.UpstreamFailoverError{
		StatusCode:             http.StatusBadGateway,
		ResponseBody:           []byte(`{"error":"upstream_silent_refusal"}`),
		RetryableOnSameAccount: false,
		SilentRefusal:          true,
	}
}

func TestIsSilentRefusal(t *testing.T) {
	if !isSilentRefusal(refusalErr()) {
		t.Fatal("refusal error should be detected")
	}
	emptyStream := &service.UpstreamFailoverError{
		StatusCode:             http.StatusBadGateway,
		ResponseBody:           []byte(`{"error":"empty stream response from upstream"}`),
		RetryableOnSameAccount: true,
	}
	if isSilentRefusal(emptyStream) {
		t.Fatal("empty-stream error must not be classified as silent refusal")
	}
	if isSilentRefusal(nil) {
		t.Fatal("nil must not be a silent refusal")
	}
}

func TestRefusalSwitchCap(t *testing.T) {
	fs := NewFailoverState(10, false).WithRefusalCap(2)
	ctx := context.Background()
	g := noopTempUnscheduler{}

	// Two refusal switches are allowed.
	for i := 0; i < 2; i++ {
		act := fs.HandleFailoverError(ctx, g, int64(i+1), service.PlatformAntigravity, refusalErr())
		if act != FailoverContinue {
			t.Fatalf("refusal switch %d: got %v, want FailoverContinue", i+1, act)
		}
	}
	// Third refusal exceeds the cap even though MaxSwitches=10 is not reached.
	act := fs.HandleFailoverError(ctx, g, 3, service.PlatformAntigravity, refusalErr())
	if act != FailoverExhausted {
		t.Fatalf("refusal switch over cap: got %v, want FailoverExhausted", act)
	}
}

func TestRefusalCapZeroDisables(t *testing.T) {
	fs := NewFailoverState(10, false).WithRefusalCap(0)
	ctx := context.Background()
	g := noopTempUnscheduler{}
	for i := 0; i < 5; i++ {
		act := fs.HandleFailoverError(ctx, g, int64(i+1), service.PlatformAntigravity, refusalErr())
		if act != FailoverContinue {
			t.Fatalf("cap=0 switch %d: got %v, want FailoverContinue", i+1, act)
		}
	}
}
