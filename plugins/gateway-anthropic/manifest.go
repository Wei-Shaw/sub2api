package main

import (
	"encoding/json"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// buildManifest constructs a declarative manifest struct.
// Exceeds 30-line guideline: pure data literal, splitting would scatter the schema declaration.
func buildManifest() *pluginsdk.Manifest {
	return &pluginsdk.Manifest{
		Name:        "gateway-anthropic",
		DisplayName: "Anthropic Gateway",
		Version:     pluginVersion,
		Description: "Anthropic gateway plugin -- handles Claude API forwarding and account management",
		Author:      "Sub2API",
		IconSVG:     anthropicIconSVG,
		Capabilities: []string{
			pluginsdk.CapabilityHTTPRegisterGateway,
		},
		Frontend: &pluginsdk.FrontendManifest{
			EntryJS:  "dist/entry.js",
			EntryCSS: "dist/entry.css",
		},
		Platforms: []pluginsdk.PlatformDecl{
			{
				Platform:           "anthropic",
				DisplayName:        "Anthropic",
				IconSVG:            anthropicIconSVG,
				ThemeColor:         "#ea580c",
				SortOrder:          1,
				CompatibleGateways: []string{"anthropic"},
				FrontendMeta: json.RawMessage(`{
					"cli_config": {
						"base_url_suffix": "/anthropic",
						"claude_code_supported": true,
						"opencode_supported": true,
						"default_client_tab": "claude-code",
						"claude_code_env_base_url": "ANTHROPIC_BASE_URL",
						"claude_code_env_api_key": "ANTHROPIC_AUTH_TOKEN"
					},
					"cc_switch_config": {
						"app": "claude"
					},
					"preset_mappings": [
						{"label": "Sonnet 4", "from": "claude-sonnet-4-*", "to": "claude-sonnet-4-20250514"},
						{"label": "Opus 4.5", "from": "claude-opus-4-5-*", "to": "claude-opus-4-5-20250514"}
					],
					"supports_mixed_channel_check": true
				}`),
				AccountTypes: []pluginsdk.AccountTypeDecl{
					{
						Type:              "oauth",
						DisplayName:       "OAuth",
						FormComponentPath: "AnthropicForm",
						Description:       "Claude Code OAuth session (full scope: profile + inference)",
						SortOrder:         1,
						BadgeLabel:        "OAuth",
						FrontendMeta: json.RawMessage(`{
							"supports_rpm_limit": true,
							"default_usage_source": "passive"
						}`),
					},
					{
						Type:              "setup-token",
						DisplayName:       "Setup Token",
						FormComponentPath: "AnthropicForm",
						Description:       "Claude Code setup token (inference only)",
						SortOrder:         2,
						BadgeLabel:        "Setup",
						FrontendMeta: json.RawMessage(`{
							"supports_rpm_limit": true,
							"default_usage_source": "passive"
						}`),
					},
					{
						Type:              "apikey",
						DisplayName:       "API Key",
						FormComponentPath: "AnthropicForm",
						Description:       "Anthropic platform API key",
						SortOrder:         3,
						BadgeLabel:        "Key",
						FrontendMeta: json.RawMessage(`{
							"supports_advanced_quota_control": true
						}`),
					},
					{
						Type:              "bedrock",
						DisplayName:       "AWS Bedrock",
						FormComponentPath: "AnthropicForm",
						Description:       "AWS Bedrock access via SigV4 or cross-region API key",
						SortOrder:         4,
						BadgeLabel:        "Bedrock",
						FrontendMeta: json.RawMessage(`{
							"supports_advanced_quota_control": true
						}`),
					},
					{
						Type:              "service_account",
						DisplayName:       "Vertex AI",
						FormComponentPath: "AnthropicForm",
						Description:       "Vertex AI access via Google Service Account",
						SortOrder:         5,
						BadgeLabel:        "Vertex",
					},
				},
				CapacityDisplay: &pluginsdk.CapacityDisplayConfig{
					ShowConcurrency: true,
					ExtraRows: []pluginsdk.DisplayRow{
						{Label: "WindowCost", Source: "window_cost_limit", Format: "cost_progress"},
						{Label: "Sessions", Source: "max_sessions", Format: "count_progress"},
						{Label: "RPM", Source: "base_rpm", Format: "rpm_progress"},
					},
				},
				UsageDisplay: &pluginsdk.UsageDisplayConfig{
					WindowLabel:  "5h",
					ShowReqCount: true,
					ShowCost:     true,
				},
				TestConfig: &pluginsdk.TestConnectionConfig{
					ModelSelector:    true,
					DefaultTestModel: defaultTestModel,
				},
				GroupConfig: &pluginsdk.GroupConfigDecl{
					FormComponentPath: "AnthropicGroupConfig",
					FrontendMeta: json.RawMessage(`{
						"supports_fallback_on_invalid_request": true
					}`),
					GroupExtraSchema: json.RawMessage(`{
						"type": "object",
						"properties": {
							"claude_code_only": {
								"type": "boolean",
								"title": "Claude Code Only",
								"description": "Only allow Claude Code requests; non-CC traffic is rejected or falls back",
								"default": false
							},
							"fallback_group_id": {
								"type": ["integer", "null"],
								"title": "Fallback Group (Claude Code)",
								"description": "Group to forward non-Claude-Code requests when claude_code_only is enabled"
							},
							"fallback_group_id_on_invalid_request": {
								"type": ["integer", "null"],
								"title": "Fallback Group (Invalid Request)",
								"description": "Group to forward requests that fail validation (e.g. unsupported model)"
							},
							"model_routing_enabled": {
								"type": "boolean",
								"title": "Model Routing Enabled",
								"description": "Enable per-model routing rules to dispatch requests to specific groups",
								"default": false
							},
							"model_routing": {
								"type": "object",
								"title": "Model Routing Rules",
								"description": "Map of model pattern to target group IDs (e.g. {\"claude-opus-4*\": [1,2]})",
								"additionalProperties": {
									"type": "array",
									"items": { "type": "integer" }
								}
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

// anthropicIconSVG is the Anthropic/Claude logo SVG used for platform display.
// Copied from frontend/src/components/common/PlatformIcon.vue.
const anthropicIconSVG = `<svg viewBox="0 0 16 16" fill="currentColor"><path d="m3.127 10.604 3.135-1.76.053-.153-.053-.085H6.11l-.525-.032-1.791-.048-1.554-.065-1.505-.08-.38-.081L0 7.832l.036-.234.32-.214.455.04 1.009.069 1.513.105 1.097.064 1.626.17h.259l.036-.105-.089-.065-.068-.064-1.566-1.062-1.695-1.121-.887-.646-.48-.327-.243-.306-.104-.67.435-.48.585.04.15.04.593.456 1.267.981 1.654 1.218.242.202.097-.068.012-.049-.109-.181-.9-1.626-.96-1.655-.428-.686-.113-.411a2 2 0 0 1-.068-.484l.496-.674L4.446 0l.662.089.279.242.411.94.666 1.48 1.033 2.014.302.597.162.553.06.17h.105v-.097l.085-1.134.157-1.392.154-1.792.052-.504.25-.605.497-.327.387.186.319.456-.045.294-.19 1.23-.37 1.93-.243 1.29h.142l.161-.16.654-.868 1.097-1.372.484-.545.565-.601.363-.287h.686l.505.751-.226.775-.707.895-.585.759-.839 1.13-.524.904.048.072.125-.012 1.897-.403 1.024-.186 1.223-.21.553.258.06.263-.218.536-1.307.323-1.533.307-2.284.54-.028.02.032.04 1.029.098.44.024h1.077l2.005.15.525.346.315.424-.053.323-.807.411-3.631-.863-.872-.218h-.12v.073l.726.71 1.331 1.202 1.667 1.55.084.383-.214.302-.226-.032-1.464-1.101-.565-.497-1.28-1.077h-.084v.113l.295.432 1.557 2.34.08.718-.112.234-.404.141-.444-.08-.911-1.28-.94-1.44-.759-1.291-.093.053-.448 4.821-.21.246-.484.186-.403-.307-.214-.496.214-.98.258-1.28.21-1.016.19-1.263.112-.42-.008-.028-.092.012-.953 1.307-1.448 1.957-1.146 1.227-.274.109-.477-.247.045-.44.266-.39 1.586-2.018.956-1.25.617-.723-.004-.105h-.036l-4.212 2.736-.75.096-.324-.302.04-.496.154-.162 1.267-.871z"/></svg>`
