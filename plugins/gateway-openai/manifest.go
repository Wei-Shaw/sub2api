package main

import pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"

// buildManifest constructs the static Manifest for the OpenAI gateway plugin.
func buildManifest() *pluginsdk.Manifest {
	return &pluginsdk.Manifest{
		Name:        "gateway-openai",
		DisplayName: "OpenAI Gateway",
		Version:     pluginVersion,
		Description: "OpenAI gateway plugin — handles OpenAI/ChatGPT API forwarding and account management",
		Author:      "Sub2API",
		IconSVG:     openaiIconSVG,
		Capabilities: []string{
			pluginsdk.CapabilityHTTPRegisterGateway,
		},
		Platforms: []pluginsdk.PlatformDecl{
			{
				Platform:    "openai",
				DisplayName: "OpenAI",
				IconSVG:     openaiIconSVG,
				ThemeColor:  "#10b981",
				SortOrder:   2,
				AccountTypes: []pluginsdk.AccountTypeDecl{
					{
						Type:        "oauth",
						DisplayName: "OAuth",
						Description: "ChatGPT Plus/Team/Enterprise OAuth session",
						SortOrder:   1,
						BadgeLabel:  "OAuth",
					},
					{
						Type:        "apikey",
						DisplayName: "API Key",
						Description: "OpenAI platform API key",
						SortOrder:   2,
						BadgeLabel:  "Key",
					},
				},
				TestConfig: &pluginsdk.TestConnectionConfig{
					ModelSelector:    true,
					DefaultTestModel: "gpt-4o",
				},
			},
		},
	}
}

// openaiIconSVG is the OpenAI logo SVG used for platform display.
const openaiIconSVG = `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M22.282 9.821a5.985 5.985 0 0 0-.516-4.91 6.046 6.046 0 0 0-6.51-2.9A6.065 6.065 0 0 0 4.981 4.18a5.985 5.985 0 0 0-3.998 2.9 6.046 6.046 0 0 0 .743 7.097 5.98 5.98 0 0 0 .51 4.911 6.051 6.051 0 0 0 6.515 2.9A5.985 5.985 0 0 0 13.26 24a6.056 6.056 0 0 0 5.772-4.206 5.99 5.99 0 0 0 3.997-2.9 6.056 6.056 0 0 0-.747-7.073z"/></svg>`
