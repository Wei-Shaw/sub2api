package main

import (
	"encoding/json"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// buildManifest constructs a declarative manifest struct.
// Exceeds 30-line guideline: pure data literal, splitting would scatter the schema declaration.
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
		Frontend: &pluginsdk.FrontendManifest{
			EntryJS: "dist/entry.js",
		},
		Platforms: []pluginsdk.PlatformDecl{
			{
				Platform:           "antigravity",
				DisplayName:        "Antigravity",
				IconSVG:            antigravityIconSVG,
				ThemeColor:         "#7c3aed",
				SortOrder:          4,
				CompatibleGateways: []string{"antigravity"},
				FrontendMeta: json.RawMessage(`{
					"cli_config": {
						"base_url_suffix": "/antigravity",
						"claude_code_supported": true,
						"gemini_cli_supported": true,
						"opencode_supported": true,
						"default_client_tab": "claude-code",
						"claude_code_env_base_url": "ANTHROPIC_BASE_URL",
						"claude_code_env_api_key": "ANTHROPIC_AUTH_TOKEN",
						"gemini_cli_env_base_url": "GOOGLE_GEMINI_BASE_URL",
						"gemini_cli_env_api_key": "GEMINI_API_KEY"
					},
					"cc_switch_config": {
						"app": "claude",
						"endpoint_suffix": "/antigravity",
						"client_type_aware": true
					},
					"preset_mappings": [
						{"label": "Claude Sonnet 4", "from": "claude-sonnet-4-*", "to": "claude-sonnet-4-20250514"},
						{"label": "Gemini 2.5 Pro", "from": "gemini-2.5-pro*", "to": "gemini-2.5-pro-preview-05-06"}
					],
					"supports_mixed_channel_check": true
				}`),
				AccountTypes: []pluginsdk.AccountTypeDecl{
					{
						Type:              "oauth",
						DisplayName:       "OAuth",
						Description:       "Antigravity Google OAuth session (Code Assist)",
						SortOrder:         1,
						BadgeLabel:        "OAuth",
						FormComponentPath: "AntigravityForm",
						// TODO(plugin-grpc): AI Credits / overages injection
						// (enabledCreditTypes=GOOGLE_ONE_AI) is NOT yet implemented in
						// the plugin Forward path; the legacy in-service loop that did it
						// (antigravity_credits_overages.go) is bypassed. The allow_overages
						// toggle still drives quota-scope resolution + admin clear-on-toggle,
						// so the UI control is retained, but overage forwarding is a no-op
						// until reimplemented here. Remove this flag if/when the product
						// decision is to drop overages from the plugin gateway entirely.
						FrontendMeta: json.RawMessage(`{
							"supports_allow_overages": true
						}`),
					},
					{
						Type:              "apikey",
						DisplayName:       "API Key",
						Description:       "Antigravity API key (supports Claude and Gemini models)",
						SortOrder:         2,
						BadgeLabel:        "Key",
						FormComponentPath: "AntigravityForm",
					},
					{
						Type:              "upstream",
						DisplayName:       "Upstream",
						Description:       "Upstream pass-through (custom base URL + API key)",
						SortOrder:         3,
						BadgeLabel:        "Upstream",
						FormComponentPath: "AntigravityForm",
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
					FrontendMeta: json.RawMessage(`{
						"supports_fallback_on_invalid_request": true
					}`),
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
