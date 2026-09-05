package manifest

import "github.com/Wei-Shaw/sub2api/plugin-service/internal/pluginregistry"

const Key = "image-generation"

func Plugin() pluginregistry.StaticPlugin {
	return pluginregistry.StaticPlugin{
		Meta: pluginregistry.Metadata{
			Key:              Key,
			Name:             "Image Generation",
			Description:      "Generate and edit images in the plugin host",
			Enabled:          true,
			FrontendMode:     pluginregistry.FrontendModeHosted,
			DefaultEntryPath: "/plugins/image-generation",
		},
	}
}
