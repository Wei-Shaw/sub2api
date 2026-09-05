package backend

import (
	"context"
	"net/http"
	"sort"
	"strings"
)

// PlatformRequest is the raw relay request handed to a single platform.
// Platform implementations own all endpoint and protocol interpretation.
type PlatformRequest struct {
	Route    RouteConfig
	Endpoint string
	Method   string
	Query    string
	Headers  http.Header
	Body     []byte
	Client   *http.Client
}

type PlatformResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Stream     func(http.ResponseWriter) error
}

type PlatformHandler interface {
	Platform() string
	Descriptor() PlatformDescriptor
	Handle(context.Context, PlatformRequest) (PlatformResponse, error)
}

type PlatformRegistry struct{ handlers map[string]PlatformHandler }

func NewPlatformRegistry() *PlatformRegistry {
	return &PlatformRegistry{handlers: make(map[string]PlatformHandler)}
}

func (r *PlatformRegistry) Register(handler PlatformHandler) {
	if r == nil || handler == nil {
		return
	}
	r.handlers[strings.ToLower(strings.TrimSpace(handler.Platform()))] = handler
}

func (r *PlatformRegistry) Get(platform string) (PlatformHandler, bool) {
	if r == nil {
		return nil, false
	}
	handler, ok := r.handlers[strings.ToLower(strings.TrimSpace(platform))]
	return handler, ok
}

func (r *PlatformRegistry) Platforms() []PlatformDescriptor {
	if r == nil {
		return []PlatformDescriptor{}
	}
	platforms := make([]PlatformDescriptor, 0, len(r.handlers))
	for _, handler := range r.handlers {
		platforms = append(platforms, handler.Descriptor())
	}
	sort.Slice(platforms, func(i, j int) bool { return platforms[i].Key < platforms[j].Key })
	return platforms
}

func NewDefaultPlatformRegistry() *PlatformRegistry {
	registry := NewPlatformRegistry()
	registry.Register(NewAgnesAdapter())
	registry.Register(NewOpenAIAdapter())
	registry.Register(NewOpenCodeAdapter())
	return registry
}
