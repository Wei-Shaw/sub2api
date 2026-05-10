package main

import pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"

// buildManifest constructs the static Manifest for the Gemini gateway plugin.
func buildManifest() *pluginsdk.Manifest {
	return &pluginsdk.Manifest{
		Name:        "gateway-gemini",
		DisplayName: "Gemini Gateway",
		Version:     pluginVersion,
		Description: "Gemini gateway plugin — handles Google Gemini API forwarding and account management",
		Author:      "Sub2API",
		IconSVG:     geminiIconSVG,
		Capabilities: []string{
			pluginsdk.CapabilityHTTPRegisterGateway,
		},
		Platforms: []pluginsdk.PlatformDecl{
			{
				Platform:    "gemini",
				DisplayName: "Gemini",
				IconSVG:     geminiIconSVG,
				ThemeColor:  "#2563eb",
				SortOrder:   3,
				AccountTypes: []pluginsdk.AccountTypeDecl{
					{
						Type:        "oauth",
						DisplayName: "OAuth",
						Description: "Gemini OAuth session (Google One / AI Studio)",
						SortOrder:   1,
						BadgeLabel:  "OAuth",
						SubTypes: []pluginsdk.SubTypeOption{
							{Value: "google_one", Label: "Google One"},
							{Value: "code_assist", Label: "Code Assist"},
							{Value: "aistudio", Label: "AI Studio"},
						},
					},
					{
						Type:        "apikey",
						DisplayName: "API Key",
						Description: "Google AI Studio API key",
						SortOrder:   2,
						BadgeLabel:  "Key",
					},
					{
						Type:        "service_account",
						DisplayName: "Service Account",
						Description: "Google Service Account / Vertex AI credentials",
						SortOrder:   3,
						BadgeLabel:  "SA",
					},
				},
				CapacityDisplay: &pluginsdk.CapacityDisplayConfig{
					ShowConcurrency: true,
				},
				UsageDisplay: &pluginsdk.UsageDisplayConfig{
					WindowLabel:  "1d",
					ShowReqCount: true,
					ShowCost:     true,
				},
				TestConfig: &pluginsdk.TestConnectionConfig{
					ModelSelector:    true,
					DefaultTestModel: "gemini-2.5-flash",
					ImageModelPatterns: []string{"gemini-", "-image"},
					PrioritizedModels: []string{
						"gemini-3.1-flash-image", "gemini-2.5-flash-image",
						"gemini-2.5-flash", "gemini-2.5-pro",
						"gemini-3-flash-preview", "gemini-3-pro-preview", "gemini-2.0-flash",
					},
				},
			},
		},
	}
}

// geminiIconSVG is the Gemini logo SVG (star) used for platform display.
const geminiIconSVG = `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l1.89 7.2L21 12l-7.11 2.8L12 22l-1.89-7.2L3 12l7.11-2.8L12 2z"/></svg>`