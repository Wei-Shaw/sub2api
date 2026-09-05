package manifest

import "github.com/Wei-Shaw/sub2api/plugin-service/internal/pluginregistry"

const Key = "ai-relay"

func Plugin() pluginregistry.StaticPlugin {
	return pluginregistry.StaticPlugin{
		Meta: pluginregistry.Metadata{
			Key:              Key,
			Name:             "AI Relay",
			Description:      "Internal protocol adapters",
			Enabled:          true,
			FrontendMode:     pluginregistry.FrontendModeHosted,
			DefaultEntryPath: "/plugins/ai-relay",
		},
	}
}
