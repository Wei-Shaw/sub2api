package plugins

import (
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/pluginregistry"
	airelay "github.com/Wei-Shaw/sub2api/plugin-service/plugins/ai-relay"
	imagegeneration "github.com/Wei-Shaw/sub2api/plugin-service/plugins/image-generation"
)

func RegisterAll(registry *pluginregistry.Registry) error {
	for _, plugin := range []pluginregistry.Plugin{imagegeneration.New(), airelay.New()} {
		if err := registry.Register(plugin); err != nil {
			return err
		}
	}
	return nil
}
