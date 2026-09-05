package pluginregistry

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/config"
	hostprincipal "github.com/Wei-Shaw/sub2api/plugin-service/internal/host/principal"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/media"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/service"
)

type FrontendMode string

const (
	FrontendModeHosted FrontendMode = "hosted"
	FrontendModeRemote FrontendMode = "remote"
)

type Metadata struct {
	Key              string
	Name             string
	Description      string
	Enabled          bool
	FrontendMode     FrontendMode
	DefaultEntryPath string
	RemoteEntryURL   string
}

type Plugin interface {
	Metadata() Metadata
}

type RouteDeps struct {
	Config  config.Config
	Auth    *hostprincipal.Middleware
	History *service.HistoryService
	Media   media.Storage
}

type RoutablePlugin interface {
	Plugin
	RegisterRoutes(mux *http.ServeMux, deps RouteDeps)
}

type StaticPlugin struct {
	Meta Metadata
}

func (p StaticPlugin) Metadata() Metadata {
	return p.Meta
}
