package airelay

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/pluginregistry"
	"github.com/Wei-Shaw/sub2api/plugin-service/plugins/ai-relay/backend"
	"github.com/Wei-Shaw/sub2api/plugin-service/plugins/ai-relay/manifest"
)

type Plugin struct{}

func New() Plugin {
	return Plugin{}
}

func (Plugin) Metadata() pluginregistry.Metadata {
	return manifest.Plugin().Metadata()
}

func (Plugin) RegisterRoutes(mux *http.ServeMux, deps pluginregistry.RouteDeps) {
	RegisterFrontend(mux)
	routes, err := backend.NewRouteRepository(deps.Config.Database)
	if err != nil {
		panic(err)
	}
	proxyResolver, err := backend.NewProxyResolver(deps.Config.Database)
	if err != nil {
		panic(err)
	}
	clients := backend.NewProxyClientProvider(nil, proxyResolver)
	backend.RegisterRoutes(mux, deps.Auth, backend.NewRelayHandlerWithClientProvider(routes, backend.NewDefaultPlatformRegistry(), clients))
}
