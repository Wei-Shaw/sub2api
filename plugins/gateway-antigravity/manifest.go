package main

import (
	"encoding/json"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

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
				GroupConfig: &pluginsdk.GroupConfigDecl{
					FormComponentPath: "AntigravityGroupConfig",
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
							"supported_model_scopes": {
								"type": "array",
								"title": "Supported Model Scopes",
								"description": "Which model families this group supports (claude, gemini_text, gemini_image)",
								"items": {
									"type": "string",
									"enum": ["claude", "gemini_text", "gemini_image"]
								},
								"uniqueItems": true
							},
							"mcp_xml_inject": {
								"type": "boolean",
								"title": "MCP XML Inject",
								"description": "Enable MCP XML protocol injection for requests in this group",
								"default": false
							},
							"fallback_group_id_on_invalid_request": {
								"type": ["integer", "null"],
								"title": "Fallback Group (Invalid Request)",
								"description": "Group to forward requests that fail validation (e.g. unsupported model)"
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

// antigravityIconSVG is the Antigravity cloud logo SVG used for platform display.
const antigravityIconSVG = `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96z"/></svg>`