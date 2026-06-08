package main

import (
	"encoding/json"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// buildManifest constructs a declarative manifest struct.
// Exceeds 30-line guideline: pure data literal, splitting would scatter the schema declaration.
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
		Frontend: &pluginsdk.FrontendManifest{
			EntryJS: "dist/entry.js",
		},
		Platforms: []pluginsdk.PlatformDecl{
			{
				Platform:           "openai",
				DisplayName:        "OpenAI",
				IconSVG:            openaiIconSVG,
				ThemeColor:         "#10b981",
				SortOrder:          2,
				CompatibleGateways: []string{"openai", "chat_completions", "responses", "anthropic_via_openai", "images"},
				FrontendMeta: json.RawMessage(`{
					"cli_config": {
						"base_url_suffix": "/openai",
						"claude_code_supported": true,
						"codex_cli_supported": true,
						"codex_ws_supported": true,
						"opencode_supported": true,
						"claude_code_requires_dispatch": true,
						"codex_model": "gpt-5.4",
						"codex_wire_api": "responses"
					},
					"cc_switch_config": {
						"app": "codex",
						"model": "gpt-5.4"
					},
					"preset_mappings": [
						{"label": "GPT-4o", "from": "gpt-4o*", "to": "gpt-4o"},
						{"label": "GPT-5.2", "from": "gpt-5.2*", "to": "gpt-5.2-preview"}
					]
				}`),
				AccountTypes: []pluginsdk.AccountTypeDecl{
					{
						Type:              "oauth",
						DisplayName:       "OAuth",
						Description:       "ChatGPT Plus/Team/Enterprise OAuth session",
						SortOrder:         1,
						BadgeLabel:        "OAuth",
						FormComponentPath: "OpenAIForm",
						FrontendMeta: json.RawMessage(`{
							"supports_passthrough": true,
							"supports_ws_mode": true,
							"supports_compact_mode": true,
							"supports_codex_cli_only": true,
							"usage_refresh_extra_fields": ["codex_usage_updated_at", "codex_5h_used_percent", "codex_monthly_used_percent"]
						}`),
					},
					{
						Type:              "apikey",
						DisplayName:       "API Key",
						Description:       "OpenAI platform API key",
						SortOrder:         2,
						BadgeLabel:        "Key",
						FormComponentPath: "OpenAIForm",
						FrontendMeta: json.RawMessage(`{
							"supports_passthrough": true,
							"supports_ws_mode": true,
							"supports_compact_mode": true
						}`),
					},
				},
				CapacityDisplay: &pluginsdk.CapacityDisplayConfig{
					ShowConcurrency: true,
				},
				UsageDisplay: &pluginsdk.UsageDisplayConfig{
					WindowLabel:  "5h",
					ShowReqCount: true,
					ShowCost:     false,
				},
				PrivacyStates: []pluginsdk.PrivacyStateDecl{
					{
						Value:       "training_off",
						DisplayName: "Private",
						BadgeColor:  "green",
						IsSet:       true,
					},
					{
						Value:       "training_set_cf_blocked",
						DisplayName: "CF Blocked",
						BadgeColor:  "yellow",
						IsSet:       false,
					},
					{
						Value:       "training_set_failed",
						DisplayName: "Failed",
						BadgeColor:  "red",
						IsSet:       false,
					},
				},
				TestConfig: &pluginsdk.TestConnectionConfig{
					ModelSelector:    true,
					DefaultTestModel: "gpt-4o",
					TestModes: []pluginsdk.TestModeOption{
						{Value: "default", Label: "Default"},
						{Value: "compact", Label: "Compact"},
					},
					ImageModelPatterns: []string{"gpt-image-"},
				},
				GroupConfig: &pluginsdk.GroupConfigDecl{
					FormComponentPath: "OpenAIGroupConfig",
					FrontendMeta: json.RawMessage(`{
						"supports_messages_dispatch": true
					}`),
					GroupExtraSchema: json.RawMessage(`{
						"type": "object",
						"properties": {
							"allow_messages_dispatch": {
								"type": "boolean",
								"title": "Allow Messages Dispatch",
								"description": "Enable Anthropic-to-OpenAI messages dispatch with model mapping",
								"default": false
							},
							"messages_dispatch_model_config": {
								"type": "object",
								"title": "Messages Dispatch Model Config",
								"description": "Model mapping configuration for messages dispatch",
								"properties": {
									"opus_mapped_model": {
										"type": "string",
										"title": "Opus Mapped Model",
										"description": "OpenAI model to use for Claude Opus family requests"
									},
									"sonnet_mapped_model": {
										"type": "string",
										"title": "Sonnet Mapped Model",
										"description": "OpenAI model to use for Claude Sonnet family requests"
									},
									"haiku_mapped_model": {
										"type": "string",
										"title": "Haiku Mapped Model",
										"description": "OpenAI model to use for Claude Haiku family requests"
									},
									"exact_model_mappings": {
										"type": "object",
										"title": "Exact Model Mappings",
										"description": "Map of exact Claude model ID to target OpenAI model ID",
										"additionalProperties": { "type": "string" }
									}
								}
							},
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

// openaiIconSVG is the OpenAI logo SVG used for platform display.
const openaiIconSVG = `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M22.282 9.821a5.985 5.985 0 0 0-.516-4.91 6.046 6.046 0 0 0-6.51-2.9A6.065 6.065 0 0 0 4.981 4.18a5.985 5.985 0 0 0-3.998 2.9 6.046 6.046 0 0 0 .743 7.097 5.98 5.98 0 0 0 .51 4.911 6.051 6.051 0 0 0 6.515 2.9A5.985 5.985 0 0 0 13.26 24a6.056 6.056 0 0 0 5.772-4.206 5.99 5.99 0 0 0 3.997-2.9 6.056 6.056 0 0 0-.747-7.073z"/></svg>`
