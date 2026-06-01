package main

import (
	"encoding/json"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// buildManifest constructs a declarative manifest struct.
// Exceeds 30-line guideline: pure data literal, splitting would scatter the schema declaration.
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
		Frontend: &pluginsdk.FrontendManifest{
			EntryJS: "dist/entry.js",
		},
		Platforms: []pluginsdk.PlatformDecl{
			{
				Platform:           "gemini",
				DisplayName:        "Gemini",
				IconSVG:            geminiIconSVG,
				ThemeColor:         "#2563eb",
				SortOrder:          3,
				CompatibleGateways: []string{"gemini"},
				FrontendMeta: json.RawMessage(`{
					"cli_config": {
						"base_url_suffix": "/gemini",
						"gemini_cli_supported": true,
						"opencode_supported": true,
						"default_client_tab": "gemini-cli",
						"gemini_cli_env_base_url": "GOOGLE_GEMINI_BASE_URL",
						"gemini_cli_env_api_key": "GEMINI_API_KEY"
					},
					"cc_switch_config": {
						"app": "gemini",
						"defaultClientType": "gemini"
					},
					"preset_mappings": [
						{"label": "Gemini 2.5 Pro", "from": "gemini-2.5-pro*", "to": "gemini-2.5-pro-preview-05-06"}
					]
				}`),
				AccountTypes: []pluginsdk.AccountTypeDecl{
					{
						Type:              "oauth",
						DisplayName:       "OAuth",
						Description:       "Gemini OAuth session (Google One / AI Studio)",
						SortOrder:         1,
						BadgeLabel:        "OAuth",
						FormComponentPath: "GeminiForm",
						SubTypes: []pluginsdk.SubTypeOption{
							{Value: "google_one", Label: "Google One"},
							{Value: "code_assist", Label: "Code Assist"},
							{Value: "aistudio", Label: "AI Studio"},
						},
					},
					{
						Type:              "apikey",
						DisplayName:       "API Key",
						Description:       "Google AI Studio API key",
						SortOrder:         2,
						BadgeLabel:        "Key",
						FormComponentPath: "GeminiForm",
					},
					{
						Type:              "service_account",
						DisplayName:       "Service Account",
						Description:       "Google Service Account / Vertex AI credentials",
						SortOrder:         3,
						BadgeLabel:        "SA",
						FormComponentPath: "GeminiForm",
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
					ModelSelector:      true,
					DefaultTestModel:   "gemini-2.5-flash",
					ImageModelPatterns: []string{"gemini-", "-image"},
					PrioritizedModels: []string{
						"gemini-3.1-flash-image", "gemini-2.5-flash-image",
						"gemini-2.5-flash", "gemini-2.5-pro",
						"gemini-3-flash-preview", "gemini-3-pro-preview", "gemini-2.0-flash",
					},
				},
				GroupConfig: &pluginsdk.GroupConfigDecl{
					FormComponentPath: "GeminiGroupConfig",
					GroupExtraSchema: json.RawMessage(`{
						"type": "object",
						"properties": {
							"image_price_1k": {
								"type": "number",
								"title": "Image Price 1K ($)",
								"description": "Cost per 1K-resolution generated image in USD",
								"minimum": 0
							},
							"image_price_2k": {
								"type": "number",
								"title": "Image Price 2K ($)",
								"description": "Cost per 2K-resolution generated image in USD",
								"minimum": 0
							},
							"image_price_4k": {
								"type": "number",
								"title": "Image Price 4K ($)",
								"description": "Cost per 4K-resolution generated image in USD",
								"minimum": 0
							},
							"require_oauth_only": {
								"type": "boolean",
								"title": "Require OAuth Only",
								"description": "Only dispatch to OAuth-type accounts in this group",
								"default": false
							},
							"require_privacy_set": {
								"type": "boolean",
								"title": "Require Privacy Set",
								"description": "Only dispatch to accounts that have privacy/training opt-out confirmed",
								"default": false
							}
						}
					}`),
				},
			},
		},
	}
}

// geminiIconSVG is the Gemini logo SVG (star) used for platform display.
const geminiIconSVG = `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l1.89 7.2L21 12l-7.11 2.8L12 22l-1.89-7.2L3 12l7.11-2.8L12 2z"/></svg>`
