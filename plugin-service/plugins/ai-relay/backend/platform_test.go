package backend

import (
	"context"
	"net/http"
	"testing"
)

type testPlatformHandler struct{}

func (testPlatformHandler) Platform() string               { return "test" }
func (testPlatformHandler) Descriptor() PlatformDescriptor { return PlatformDescriptor{Key: "test"} }
func (testPlatformHandler) Handle(context.Context, PlatformRequest) (PlatformResponse, error) {
	return PlatformResponse{StatusCode: http.StatusNoContent}, nil
}

func TestPlatformRegistryLooksUpRegisteredHandler(t *testing.T) {
	registry := NewPlatformRegistry()
	registry.Register(testPlatformHandler{})
	handler, ok := registry.Get("TEST")
	if !ok || handler.Platform() != "test" {
		t.Fatalf("Get() = %#v, %v", handler, ok)
	}
}
