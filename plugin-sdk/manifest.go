package pluginsdk

import (
	"encoding/json"
	"fmt"
	"log/slog"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// Authentication kinds accepted by the core gateway. They are mirrored as
// strings on EndpointDecl.AuthType because the proto API uses bare strings.
const (
	AuthTypeAdmin  = "admin"
	AuthTypeUser   = "user"
	AuthTypeAPIKey = "apikey"
	AuthTypeNone   = "none"
)

// Section names used for grouping menu items in the frontend.
const (
	SectionAdmin = "admin"
	SectionUser  = "user"
)

// Capability identifiers a plugin may declare in Manifest.Capabilities.
//
// The SDK server treats these as fine-grained permissions: a plugin lacking
// a capability is rejected when it tries to use the corresponding feature.
// Adding a new capability requires updating both the SDK and the core's
// allow-list (backend/internal/plugin/grpc_server.go).
const (
	// CapabilityRedisRawKeys lets PluginContext.Redis().Raw() bypass the
	// per-plugin Redis key namespace. Required for plugins that intentionally
	// share keys with other components (e.g. the channel-management plugin
	// writes the gateway cache contract documented in GATEWAY_CACHE_SPEC.md).
	CapabilityRedisRawKeys = "redis_raw_keys"

	// CapabilitySecretEncryption authorises a plugin to call
	// PluginContext.Secrets().Encrypt/Decrypt. The host derives a
	// per-plugin key from its master key on first use and uses AES-256-GCM
	// with the plugin name as AAD. Plugins lacking this capability receive
	// a nil SecretEncryptor from PluginContext.Secrets() — see V5-DESIGN
	// §5 for the full design.
	CapabilitySecretEncryption = "secret_encryption"
	// CapabilityJobScheduler grants access to PluginContext.Jobs() for
	// declaring scheduled work (interval / cron / fixed_delay). The host owns
	// the schedule clock and per-(plugin, job) leader lock; the plugin owns
	// the handler. See V5-DESIGN §2 (W2 JobSchedulerCapability).
	CapabilityJobScheduler = "job_scheduler"
	// CapabilitySettingsExtension opts the plugin into the V5
	// SettingsExtension SDK feature. The host validates schema-bearing
	// manifests and wires up Settings.Get / Watch only for plugins that
	// declare this capability (or supply a non-empty SettingsSchema in
	// their Manifest, which the SDK upgrades to this capability
	// automatically). Declaring it explicitly is harmless.
	CapabilitySettingsExtension = "settings_extension"
)

// Manifest is the Go-level representation of pluginsdk.ManifestResponse.
// Plugins build and return this from Plugin.Manifest(); the SDK converts it
// to the proto type when the core calls GetManifest.
type Manifest struct {
	Name        string
	DisplayName string
	Version     string
	Description string
	Author      string

	// GatewayEndpoints declare HTTP routes that the core's gateway should
	// forward to this plugin. They are typically user-facing API paths.
	GatewayEndpoints []EndpointDecl
	// PluginEndpoints declare HTTP routes that the plugin serves on its own
	// HTTP server (admin/management paths).
	PluginEndpoints []EndpointDecl

	Frontend *FrontendManifest

	// MigrationFiles is an optional list of SQL migration filenames the
	// plugin ships with. The core handles applying them.
	//
	// Deprecated: prefer Migrations, which carries the SHA-256 pin the host
	// re-verifies against the body fetched via PluginLifecycle.GetMigration.
	// MigrationFiles entries arrive at the host without a checksum, so the
	// host cannot apply them — they are logged for visibility only. New
	// plugins should populate Migrations and implement MigrationProvider.
	MigrationFiles []string

	// Migrations declares the SQL migration files the plugin ships embedded.
	// Each entry pins a SHA-256 checksum the host re-verifies against the
	// body fetched via PluginLifecycle.GetMigration. The plugin must
	// implement MigrationProvider so the SDK runner can serve those bodies.
	//
	// Files are applied in lexicographical order of Filename. Ordering and
	// existing checksums are immutable once shipped: append-only edits keep
	// historical plugin_migrations rows valid.
	Migrations []MigrationDecl

	// Capabilities lists the privileged SDK features this plugin needs. See
	// the Capability* constants. The core checks each entry against an
	// allow-list and only forwards the approved ones to the plugin runtime
	// via PluginInitRequest.capabilities.
	Capabilities []string

	// SettingsSchema is the optional JSON Schema (Draft-07) describing the
	// plugin's admin-tunable runtime settings. When set, the host renders
	// it as a tab on the admin Settings page. Leaving it empty means the
	// plugin contributes no settings. See SettingsSchemaDoc for the
	// preferred construction helper.
	SettingsSchema *SettingsSchemaDoc

	// IconSVG is the complete SVG markup (including the outer <svg> tag)
	// the admin plugin-management page renders next to the plugin display
	// name. Use one of the Icon* constants in this package or supply your
	// own. Empty falls back to the host's generic plugin icon.
	IconSVG string
}

// Placement describes where on a sidebar a plugin's menu item should land.
// Pointing a MenuItemDecl at a Placement opts that item into the V5/W7
// "Placement DSL" merge algorithm; leaving Placement nil keeps the legacy
// SortOrder path (item is appended at the end of its section).
//
// Group is one of the Placement* constants below. Order is the relative
// position *inside* that bucket — lower renders first.
type Placement struct {
	Group string
	Order int
}

// Placement* are the known sidebar buckets the host honours. Plugins should
// always use these constants instead of bare strings so a typo fails to
// compile rather than silently landing in the fallback bucket.
const (
	PlacementAdminMain   = "admin/main"
	PlacementAdminSystem = "admin/system"
	PlacementAdminEnd    = "admin/end"
	PlacementUserMain    = "user/main"
	PlacementUserEnd     = "user/end"
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

// EndpointDecl describes a single HTTP endpoint declaration.
type EndpointDecl struct {
	Path     string
	Methods  []string
	AuthType string // one of AuthType* constants
}

// MigrationDecl describes a single SQL migration the plugin ships embedded.
// The host fetches the body via PluginLifecycle.GetMigration and re-verifies
// the SHA-256 against ChecksumSha256 before applying it. It mirrors the
// pluginsdk.MigrationDecl proto message; see its docstring for the full
// lifecycle.
type MigrationDecl struct {
	// Filename is applied in lexicographical order, e.g. "001_create_x.sql".
	Filename string

	// ChecksumSha256 is the hex-encoded SHA-256 of the SQL body the plugin
	// intends to ship. The host treats a mismatch with the fetched body as
	// a fatal startup error (drift = potential supply-chain tampering).
	ChecksumSha256 string

	// NonTransactional marks migrations that contain statements PostgreSQL
	// refuses to run inside an explicit transaction (e.g.
	// CREATE INDEX CONCURRENTLY). The host applies these outside BEGIN/COMMIT.
	NonTransactional bool
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

// toProto converts a Manifest into its protobuf wire form. It tolerates a nil
// receiver and a nil Frontend so plugins can leave optional sections empty.
func (m *Manifest) toProto() *pb.ManifestResponse {
	if m == nil {
		return &pb.ManifestResponse{}
	}
	caps := append([]string(nil), m.Capabilities...)
	resp := &pb.ManifestResponse{
		Name:             m.Name,
		DisplayName:      m.DisplayName,
		Version:          m.Version,
		Description:      m.Description,
		Author:           m.Author,
		GatewayEndpoints: endpointsToProto(m.GatewayEndpoints),
		PluginEndpoints:  endpointsToProto(m.PluginEndpoints),
		MigrationFiles:   append([]string(nil), m.MigrationFiles...),
		Migrations:       migrationsToProto(m.Migrations),
		IconSvg:          m.IconSVG,
	}
	if m.Frontend != nil {
		resp.Frontend = m.Frontend.toProto()
	}
	if m.SettingsSchema != nil {
		// Copy bytes defensively so later mutations on the plugin side cannot
		// race with a transmission already in flight.
		if len(m.SettingsSchema.Schema) > 0 {
			resp.SettingsSchemaJson = append([]byte(nil), m.SettingsSchema.Schema...)
		}
		if len(m.SettingsSchema.Defaults) > 0 {
			resp.SettingsDefaultsJson = append([]byte(nil), m.SettingsSchema.Defaults...)
		}
		// Implicitly opt the plugin into SettingsExtension when it ships a
		// schema. The host's allow-list still has the final say but this
		// removes a footgun where authors forget the constant.
		if resp.SettingsSchemaJson != nil && !containsCap(caps, CapabilitySettingsExtension) {
			caps = append(caps, CapabilitySettingsExtension)
		}
		// V5/W6 SETTINGS-V2: ship version + per-property markers so the
		// host does not need to re-walk the schema for every admin GET.
		resp.SettingsSchemaVersion = m.SettingsSchema.Version

		if len(m.SettingsSchema.PropertyMeta) > 0 {
			// Surface invalid Visibility values via the plugin's logger so
			// the author notices during local development. The host's
			// RegisterSchema (W2-B) will reject the manifest as the final
			// defence; emitting nil here keeps the host able to derive
			// markers from schema_json alone.
			if err := validatePropertyMeta(m.SettingsSchema.PropertyMeta); err != nil {
				slog.Default().Warn("plugin-sdk: PropertyMeta validation failed", "error", err)
			}
			metaBytes, err := json.Marshal(m.SettingsSchema.PropertyMeta)
			if err != nil {
				// Falling back to nil keeps the host able to derive markers
				// from schema_json alone; the plugin author saw the error
				// in their logs via slog.Default since toProto runs at
				// manifest send time.
				slog.Default().Warn("plugin-sdk: marshal PropertyMeta failed", "error", err)
				resp.SettingsPropertiesMetaJson = nil
			} else {
				resp.SettingsPropertiesMetaJson = metaBytes
			}
		}
	}
	resp.Capabilities = caps
	return resp
}

func containsCap(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}

func endpointsToProto(decls []EndpointDecl) []*pb.EndpointDeclaration {
	if len(decls) == 0 {
		return nil
	}
	out := make([]*pb.EndpointDeclaration, 0, len(decls))
	for _, d := range decls {
		out = append(out, &pb.EndpointDeclaration{
			Path:     d.Path,
			Methods:  append([]string(nil), d.Methods...),
			AuthType: d.AuthType,
		})
	}
	return out
}

func migrationsToProto(decls []MigrationDecl) []*pb.MigrationDecl {
	if len(decls) == 0 {
		return nil
	}
	out := make([]*pb.MigrationDecl, 0, len(decls))
	for _, d := range decls {
		out = append(out, &pb.MigrationDecl{
			Filename:         d.Filename,
			ChecksumSha256:   d.ChecksumSha256,
			NonTransactional: d.NonTransactional,
		})
	}
	return out
}

func (f *FrontendManifest) toProto() *pb.FrontendManifest {
	if f == nil {
		return nil
	}
	return &pb.FrontendManifest{
		EntryJs:        f.EntryJS,
		EntryCss:       f.EntryCSS,
		MenuItems:      menuItemsToProto(f.MenuItems),
		Routes:         routesToProto(f.Routes),
		I18NNamespaces: append([]string(nil), f.I18nNamespaces...),
	}
}

func menuItemsToProto(items []MenuItemDecl) []*pb.MenuItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]*pb.MenuItem, 0, len(items))
	for _, item := range items {
		var labels map[string]string
		if len(item.Labels) > 0 {
			labels = make(map[string]string, len(item.Labels))
			for k, v := range item.Labels {
				labels[k] = v
			}
		}
		mi := &pb.MenuItem{
			Path:             item.Path,
			LabelKey:         item.LabelKey,
			Icon:             item.Icon,
			Section:          item.Section,
			SortOrder:        int32(item.SortOrder),
			RequiresAdmin:    item.RequiresAdmin,
			HideInSimpleMode: item.HideInSimpleMode,
			FeatureFlag:      item.FeatureFlag,
			Children:         menuItemsToProto(item.Children),
			IconSvg:          item.IconSVG,
			Labels:           labels,
		}
		// V5/W7 Placement DSL — only stamp the wire fields when the plugin
		// opted in; leaving them zero preserves the legacy SortOrder path
		// for callers still on the old API.
		if item.Placement != nil && item.Placement.Group != "" {
			mi.PlacementGroup = item.Placement.Group
			mi.PlacementOrder = int32(item.Placement.Order)
		}
		out = append(out, mi)
	}
	return out
}

func routesToProto(routes []RouteDecl) []*pb.RouteDefinition {
	if len(routes) == 0 {
		return nil
	}
	out := make([]*pb.RouteDefinition, 0, len(routes))
	for _, r := range routes {
		var meta map[string]string
		if len(r.Meta) > 0 {
			meta = make(map[string]string, len(r.Meta))
			for k, v := range r.Meta {
				meta[k] = v
			}
		}
		out = append(out, &pb.RouteDefinition{
			Path:          r.Path,
			Name:          r.Name,
			ComponentPath: r.ComponentPath,
			Meta:          meta,
		})
	}
	return out
}
