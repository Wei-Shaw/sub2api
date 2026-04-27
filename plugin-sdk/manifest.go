package pluginsdk

import (
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
	MigrationFiles []string

	// Capabilities lists the privileged SDK features this plugin needs. See
	// the Capability* constants. The core checks each entry against an
	// allow-list and only forwards the approved ones to the plugin runtime
	// via PluginInitRequest.capabilities.
	Capabilities []string
}

// EndpointDecl describes a single HTTP endpoint declaration.
type EndpointDecl struct {
	Path     string
	Methods  []string
	AuthType string // one of AuthType* constants
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
	resp := &pb.ManifestResponse{
		Name:             m.Name,
		DisplayName:      m.DisplayName,
		Version:          m.Version,
		Description:      m.Description,
		Author:           m.Author,
		GatewayEndpoints: endpointsToProto(m.GatewayEndpoints),
		PluginEndpoints:  endpointsToProto(m.PluginEndpoints),
		MigrationFiles:   append([]string(nil), m.MigrationFiles...),
		Capabilities:     append([]string(nil), m.Capabilities...),
	}
	if m.Frontend != nil {
		resp.Frontend = m.Frontend.toProto()
	}
	return resp
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
		out = append(out, &pb.MenuItem{
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
		})
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
