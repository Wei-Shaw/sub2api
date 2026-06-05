package pluginsdk

import (
	"encoding/json"
	"fmt"
)

// SettingsSchemaDoc bundles a JSON Schema and its default values into the
// shape the SDK ships in the manifest. Both fields are stored as raw JSON so
// plugins can compose them however they like (literal strings, embed.FS
// bytes, generated structs marshalled at startup).
//
// The host is the source of truth once the manifest has been received; the
// plugin should treat the schema as immutable for the lifetime of a given
// plugin version. Bumping the plugin version + restarting is the supported
// upgrade path when the schema changes shape.
//
// V5/W6 SETTINGS-V2 added Version and PropertyMeta to support marker-driven
// admin UI features (visibility / deprecated / requires_reload). See
// docs/plugin-architecture/SETTINGS-V2-DESIGN.md §3.3 for the full spec.
type SettingsSchemaDoc struct {
	// Schema is a JSON Schema Draft-07 document. The top level should be an
	// object with a `properties` map; each property describes one setting.
	// Use the standard `default`, `title`, `description`, `enum` keywords —
	// the admin form renderer (vue-json-schema-form) consumes them directly.
	Schema json.RawMessage

	// Defaults is a JSON object whose keys mirror the top-level properties
	// in Schema and whose values are the default the host should seed when
	// no admin save has happened yet. Marshalling defaults separately from
	// the schema avoids forcing the host to re-walk the schema tree for
	// every plugin install.
	Defaults json.RawMessage

	// Version is the plugin's self-declared schema version, e.g. "1.0.0".
	// The host stamps it into plugin_settings.schema_version_at_write on
	// every write so the plugin SDK's GetTyped can detect stale values
	// (SchemaVersionMismatchError). Empty is normalised to "0" host-side.
	//
	// Semantic versioning is RECOMMENDED but not enforced — the host
	// treats it as an opaque string and only checks for equality. The
	// plugin owns the comparison logic when it wants ordering.
	Version string `json:"version,omitempty"`

	// PropertyMeta carries per-property markers (visibility / deprecated /
	// requires_reload) keyed by the top-level property name. The SDK
	// serialises this map into ManifestResponse.settings_properties_meta_json.
	//
	// Plugins may declare the markers inline as JSON Schema vendor extensions
	// (`x-visibility`, `x-deprecated`, `x-requires-reload`) on the schema
	// node itself instead of populating this map. Both work; when both are
	// present this map wins (per SETTINGS-V2-DESIGN §3.3.2).
	PropertyMeta map[string]PropertyMetadata `json:"property_meta,omitempty"`
}

// PropertyMetadata is the marker triple SETTINGS-V2 attaches to one
// top-level schema property. All three fields default to the zero value;
// `Visibility == ""` is normalised to "frontend" by the host.
type PropertyMetadata struct {
	// Visibility is one of "frontend" | "backend" | "secret". Empty defaults
	// to "frontend" host-side. See SETTINGS-V2-DESIGN §4.2 for read/write
	// semantics.
	Visibility string `json:"visibility,omitempty"`

	// Deprecated, if non-empty, marks the field as deprecated. The string
	// is the human-readable migration message ("use foo instead"). Admin
	// UI renders strikethrough + warning tag. Empty means not deprecated.
	Deprecated string `json:"deprecated,omitempty"`

	// RequiresReload=true means the host should reload the plugin process
	// after admin saves this key. See SETTINGS-V2-DESIGN §4.4 for the
	// reload state machine.
	RequiresReload bool `json:"requires_reload,omitempty"`
}

// validatePropertyMeta checks that every PropertyMetadata.Visibility value
// in meta is one of the allowed strings ("" | "frontend" | "backend" |
// "secret"). It returns the first violation as an error.
//
// Per SETTINGS-V2-DESIGN §3.3.1 this helper exists so the SDK can fail-fast
// on obviously broken plugin manifests; the host's
// PluginSettingsService.RegisterSchema (W2-B) will run an equivalent check
// as a defence-in-depth measure.
func validatePropertyMeta(meta map[string]PropertyMetadata) error {
	for prop, m := range meta {
		switch m.Visibility {
		case "", "frontend", "backend", "secret":
			// ok
		default:
			return fmt.Errorf("pluginsdk: SettingsSchemaDoc.PropertyMeta[%q].Visibility=%q must be one of frontend|backend|secret", prop, m.Visibility)
		}
	}
	return nil
}

// FrontendManifest describes the plugin's frontend integration.
type FrontendManifest struct {
	EntryJS        string
	EntryCSS       string
	MenuItems      []MenuItemDecl
	Routes         []RouteDecl
	I18nNamespaces []string
}

// MenuItemDecl is a sidebar/menu entry contributed by the plugin.
//
// Plugins should prefer the new `IconSVG` and `Labels` fields over the legacy
// `Icon` (icon name) and `LabelKey` (i18n key) so that the core never has to
// maintain a registry of plugin icons or translations. See pluginsdk.Labels
// and the Icon* constants for ergonomic helpers.
type MenuItemDecl struct {
	Path             string
	LabelKey         string // legacy: i18n key resolved by the core. Prefer Labels.
	Icon             string // legacy: icon name. Prefer IconSVG.
	Section          string // SectionAdmin or SectionUser
	SortOrder        int
	RequiresAdmin    bool
	HideInSimpleMode bool
	FeatureFlag      string
	Children         []MenuItemDecl

	// IconSVG is the complete SVG markup (including the outer <svg> tag) the
	// frontend will inject directly. Use one of the Icon* constants in this
	// package or supply your own.
	IconSVG string
	// Labels maps a locale code to the already-translated menu label, e.g.
	// {"zh": "渠道管理", "en": "Channel Management"}. The frontend chooses the
	// entry matching the user's current locale, falling back to "en".
	Labels map[string]string

	// Descriptions maps a locale code to the already-translated menu
	// description text the host AppHeader renders under the page title,
	// e.g. {"zh": "管理渠道和自定义模型定价", "en": "Manage channels..."}.
	// Plugins ship descriptions through the manifest so plugin views no
	// longer need to repeat title/description inside a per-view
	// PluginPageLayout header — the host AppHeader becomes the single
	// source of truth, matching how host pages already work. Empty map
	// = no description (AppHeader leaves the line empty). Use the
	// Descriptions helper to construct the common zh/en pair.
	Descriptions map[string]string

	// Placement opts this menu item into the V5/W7 Placement DSL: the host
	// merges the item into the named sidebar bucket (Group) at the given
	// Order rather than appending at the end of its Section. nil = legacy
	// SortOrder behaviour (item is appended at the end of its section).
	// Both fields must be set together; setting Order without Group has
	// the same effect as nil.
	Placement *Placement
}

// RouteDecl is a Vue Router route definition contributed by the plugin.
type RouteDecl struct {
	Path          string
	Name          string
	ComponentPath string
	Meta          map[string]string
}
