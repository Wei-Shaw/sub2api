package imagegeneration

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/pluginregistry"
	"github.com/Wei-Shaw/sub2api/plugin-service/plugins/image-generation/backend"
	"github.com/Wei-Shaw/sub2api/plugin-service/plugins/image-generation/manifest"
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

	generation := backend.NewGenerationService(deps.History, backend.GenerationServiceOptions{MediaStorage: deps.Media})
	handler := backend.NewHandler(backend.HandlerDeps{
		PluginKey:  manifest.Key,
		History:    deps.History,
		Generation: generation,
	})
	backend.RegisterRoutes(mux, deps.Auth, handler)
}
