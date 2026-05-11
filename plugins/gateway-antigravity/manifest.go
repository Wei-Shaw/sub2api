package main

import pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"

// buildManifest constructs the static Manifest for the Antigravity gateway plugin.
func buildManifest() *pluginsdk.Manifest {
	return &pluginsdk.Manifest{
		Name:        "gateway-antigravity",
		DisplayName: "Antigravity Gateway",
		Version:     pluginVersion,
		Description: "Antigravity gateway plugin — handles Antigravity (Google Cloud Code Assist) API forwarding and account management",
		Author:      "Sub2API",
		IconSVG:     antigravityIconSVG,
		Capabilities: []string{
			pluginsdk.CapabilityHTTPRegisterGateway,
		},
		Platforms: []pluginsdk.PlatformDecl{
			{
				Platform:    "antigravity",
				DisplayName: "Antigravity",
				IconSVG:     antigravityIconSVG,
				ThemeColor:         "#7c3aed",
				SortOrder:          3,
				CompatibleGateways: []string{"antigravity"},
				AccountTypes: []pluginsdk.AccountTypeDecl{
					{
						Type:        "oauth",
						DisplayName: "OAuth",
						Description: "Antigravity Google OAuth session (Code Assist)",
						SortOrder:   1,
						BadgeLabel:  "OAuth",
					},
					{
						Type:        "apikey",
						DisplayName: "API Key",
						Description: "Antigravity API key (supports Claude and Gemini models)",
						SortOrder:   2,
						BadgeLabel:  "Key",
					},
					{
						Type:        "upstream",
						DisplayName: "Upstream",
						Description: "Upstream pass-through (custom base URL + API key)",
						SortOrder:   3,
						BadgeLabel:  "Upstream",
					},
				},
				CapacityDisplay: &pluginsdk.CapacityDisplayConfig{
					ShowConcurrency: true,
				},
				UsageDisplay: &pluginsdk.UsageDisplayConfig{
					ShowReqCount: true,
					ShowCost:     true,
				},
				PrivacyStates: []pluginsdk.PrivacyStateDecl{
					{
						Value:       "privacy_set",
						DisplayName: "Privacy Set",
						BadgeColor:  "green",
						IsSet:       true,
					},
					{
						Value:       "privacy_set_failed",
						DisplayName: "Privacy Failed",
						BadgeColor:  "red",
						IsSet:       false,
					},
				},
				TestConfig: &pluginsdk.TestConnectionConfig{
					ModelSelector:    true,
					DefaultTestModel: "claude-sonnet-4-5",
				},
			},
		},
	}
}

// antigravityIconSVG is the Antigravity cloud logo SVG used for platform display.
const antigravityIconSVG = `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96z"/></svg>`