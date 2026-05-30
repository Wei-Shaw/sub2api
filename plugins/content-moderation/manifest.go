package main

import (
	"net/http"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

const (
	pluginName    = "content-moderation"
	pluginVersion = "0.1.0"

	// pluginRoutePrefix is the path prefix the core gateway uses when proxying
	// this plugin's admin endpoints. Per the channel-management convention the
	// manifest declares full paths including this prefix; the core matches them
	// verbatim and does NOT strip it before forwarding to the plugin's HTTP
	// server, so RegisterHTTP must mount the same full paths.
	pluginRoutePrefix = "/api/v1/plugin/content-moderation"
)

// contentModerationSettingsSchema is the V5/W3 SettingsExtension schema for
// the moderation runtime config. This is the skeleton subset (operating mode);
// the full schema (API keys, thresholds, keyword lists, protocol toggles, …)
// migrates from the core alongside the service layer.
var (
	contentModerationSettingsSchema = []byte(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Content Moderation Settings",
  "type": "object",
  "properties": {
    "mode": {
      "type": "string",
      "title": "Operating Mode",
      "description": "off: never check; observe: check async, never block; pre_block: check sync, block on violation.",
      "enum": ["off", "observe", "pre_block"],
      "default": "off"
    }
  }
}`)
	contentModerationSettingsDefaults = []byte(`{"mode":"off"}`)
)

func (p *ContentModerationPlugin) Manifest() *pluginsdk.Manifest {
	return &pluginsdk.Manifest{
		Name:        pluginName,
		DisplayName: "Content Moderation",
		Version:     pluginVersion,
		Description: "Inspects gateway request content and blocks or flags policy violations.",
		Author:      "Sub2API",
		IconSVG:     pluginsdk.IconPuzzle,

		// Admin management API served by the plugin's own HTTP server.
		PluginEndpoints: []pluginsdk.EndpointDecl{
			{Path: pluginRoutePrefix + "/config", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeAdmin},
			{Path: pluginRoutePrefix + "/config", Methods: []string{http.MethodPut}, AuthType: pluginsdk.AuthTypeAdmin},
			{Path: pluginRoutePrefix + "/test-api-keys", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeAdmin},
			{Path: pluginRoutePrefix + "/status", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeAdmin},
			{Path: pluginRoutePrefix + "/logs", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeAdmin},
			{Path: pluginRoutePrefix + "/unban-user", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeAdmin},
			{Path: pluginRoutePrefix + "/flagged-hashes/:hash", Methods: []string{http.MethodDelete}, AuthType: pluginsdk.AuthTypeAdmin},
			{Path: pluginRoutePrefix + "/flagged-hashes", Methods: []string{http.MethodDelete}, AuthType: pluginsdk.AuthTypeAdmin},
		},

		// OwnedTables: the plugin creates content_moderation_logs via its
		// migration. db.core.read is needed to read/update the host users
		// table for ban/unban enforcement.
		OwnedTables:  []string{"content_moderation_logs"},
		Capabilities: []string{pluginsdk.CapabilityDBCoreRead},

		// Migrations: the content_moderation_logs schema, migrated from the
		// core's backend/migrations/135_content_moderation.sql. The checksum
		// is filled in when the SQL file is copied into migrations/ during the
		// service-layer migration task.
		Migrations: []pluginsdk.MigrationDecl{
			{
				Filename:       "135_content_moderation.sql",
				ChecksumSha256: "4f523dbff3fb838de69d6b557fd65edf3f3b7bdd52b91197fa2c866f8c9aa818",
			},
		},

		Frontend: &pluginsdk.FrontendManifest{
			EntryJS: "dist/entry.js",
			MenuItems: []pluginsdk.MenuItemDecl{
				{
					Path:      "/admin/risk-control",
					IconSVG:   pluginsdk.IconPuzzle,
					Labels:    pluginsdk.Labels("内容审计", "Content Moderation"),
					Section:   pluginsdk.SectionAdmin,
					Placement: &pluginsdk.Placement{Group: pluginsdk.PlacementAdminSystem, Order: 50},
					SortOrder: 500,
				},
			},
			Routes: []pluginsdk.RouteDecl{
				{
					Path:          "/admin/risk-control",
					Name:          "PluginContentModeration",
					ComponentPath: "RiskControlView.vue",
				},
			},
			I18nNamespaces: []string{"contentModerationPlugin"},
		},

		SettingsSchema: &pluginsdk.SettingsSchemaDoc{
			Schema:   contentModerationSettingsSchema,
			Defaults: contentModerationSettingsDefaults,
		},
	}
}
