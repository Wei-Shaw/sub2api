package pluginsdk

import (
	"encoding/json"
	"log/slog"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// toProto converts a Manifest into its protobuf wire form. It tolerates a nil
// receiver and a nil Frontend so plugins can leave optional sections empty.
func (m *Manifest) toProto() *pb.ManifestResponse {
	if m == nil {
		return &pb.ManifestResponse{}
	}
	caps := append([]string(nil), m.Capabilities...)
	// MigrationFiles is the deprecated checksum-less list. When the plugin
	// populates the new Migrations slice, skip MigrationFiles entirely so
	// the host doesn't double-track migrations under the legacy field.
	var migrationFiles []string
	if len(m.Migrations) == 0 {
		migrationFiles = append([]string(nil), m.MigrationFiles...)
	}
	resp := &pb.ManifestResponse{
		Name:                  m.Name,
		DisplayName:           m.DisplayName,
		Version:               m.Version,
		Description:           m.Description,
		Author:                m.Author,
		GatewayEndpoints:      endpointsToProto(m.GatewayEndpoints),
		PluginEndpoints:       endpointsToProto(m.PluginEndpoints),
		MigrationFiles:        migrationFiles,
		Migrations:            migrationsToProto(m.Migrations),
		IconSvg:               m.IconSVG,
		SubscribedEvents:      append([]string(nil), m.SubscribedEvents...),
		OwnedTables:           append([]string(nil), m.OwnedTables...),
		PublicFlags:           publicFlagsToProto(m.PublicFlags),
		SettingsComponentPath: m.SettingsComponentPath,
		Platforms:             platformsToProto(m.Platforms),
	}
	if m.Frontend != nil {
		resp.Frontend = m.Frontend.toProto()
	}
	if m.SettingsSchema != nil {
		caps = applySettingsSchema(resp, m.SettingsSchema, caps)
	}
	resp.Capabilities = caps
	return resp
}

// applySettingsSchema copies the SettingsSchemaDoc onto the proto response and
// returns a possibly-extended capability slice. Two concerns lived inside
// toProto before T53 split them out: defensive byte copies of the schema /
// defaults blobs (so concurrent plugin-side mutations cannot race a
// transmission in flight), and the canonical-capability auto-grant that
// implicitly opts a schema-bearing plugin into CapabilitySettingsOwnRead.
//
// Returning the (possibly-extended) caps slice makes the side-effect
// explicit: callers chain "caps = applySettingsSchema(...)" rather than
// having to know which fields the helper mutated.
func applySettingsSchema(resp *pb.ManifestResponse, doc *SettingsSchemaDoc, caps []string) []string {
	// Copy bytes defensively so later mutations on the plugin side cannot
	// race with a transmission already in flight.
	if len(doc.Schema) > 0 {
		resp.SettingsSchemaJson = append([]byte(nil), doc.Schema...)
	}
	if len(doc.Defaults) > 0 {
		resp.SettingsDefaultsJson = append([]byte(nil), doc.Defaults...)
	}
	caps = grantSettingsReadIfImplied(caps, resp.SettingsSchemaJson != nil)
	resp.SettingsSchemaVersion = doc.Version
	if len(doc.PropertyMeta) > 0 {
		resp.SettingsPropertiesMetaJson = encodePropertyMeta(doc.PropertyMeta)
	}
	return caps
}

// grantSettingsReadIfImplied implements the V5/W6 SETTINGS-V2 implicit
// opt-in: shipping a schema implies the plugin wants to read its own
// settings, so we add CapabilitySettingsOwnRead to the canonical list.
//
// Skip when either the canonical or the legacy alias is already present so
// manifests opting into write-only settings, or pinned to the deprecated
// alias, are honoured as declared.
func grantSettingsReadIfImplied(caps []string, hasSchema bool) []string {
	if !hasSchema {
		return caps
	}
	if containsCap(caps, CapabilitySettingsOwnRead) ||
		containsCap(caps, CapabilitySettingsOwnWrite) ||
		containsCap(caps, CapabilitySettingsExtension) {
		return caps
	}
	return append(caps, CapabilitySettingsOwnRead)
}

// encodePropertyMeta validates and JSON-encodes the per-property metadata
// the plugin author declared. Validation failures are surfaced via the
// plugin's slog so the author sees them during local development; encoding
// failures fall back to nil so the host can still derive markers from
// schema_json alone (the host RegisterSchema gate is the authoritative
// defence).
func encodePropertyMeta(meta map[string]PropertyMetadata) []byte {
	if err := validatePropertyMeta(meta); err != nil {
		slog.Default().Warn("plugin-sdk: PropertyMeta validation failed", "error", err)
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		slog.Default().Warn("plugin-sdk: marshal PropertyMeta failed", "error", err)
		return nil
	}
	return metaBytes
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
			Filename:           d.Filename,
			ChecksumSha256:     d.ChecksumSha256,
			NonTransactional:   d.NonTransactional,
			DownFilename:       d.DownFilename,
			DownChecksumSha256: d.DownChecksumSHA256,
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
		out = append(out, menuItemToProto(item))
	}
	return out
}

// menuItemToProto translates one MenuItemDecl onto the proto type. Pulled
// out of menuItemsToProto so the loop body stays small and adding a new
// menu field touches only this function.
func menuItemToProto(item MenuItemDecl) *pb.MenuItem {
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
		Labels:           copyStringMap(item.Labels),
		Descriptions:     copyStringMap(item.Descriptions),
	}
	// V5/W7 Placement DSL — only stamp the wire fields when the plugin
	// opted in; leaving them zero preserves the legacy SortOrder path
	// for callers still on the old API.
	if item.Placement != nil && item.Placement.Group != "" {
		mi.PlacementGroup = item.Placement.Group
		mi.PlacementOrder = int32(item.Placement.Order)
	}
	return mi
}

// copyStringMap returns a defensive copy of m or nil when m is empty.
// Used by menuItemToProto so concurrent plugin-side mutations of the
// labels / descriptions maps cannot race with a transmission in flight.
func copyStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
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

// publicFlagsToProto translates the Manifest.PublicFlags slice onto the proto
// wire form. default_value is JSON-encoded so the host can preserve the
// declared type (bool vs string vs number) without a schema lookup.
func publicFlagsToProto(flags []PublicFlagDecl) []*pb.PublicFlagDecl {
	if len(flags) == 0 {
		return nil
	}
	out := make([]*pb.PublicFlagDecl, 0, len(flags))
	for _, fl := range flags {
		defaultJSON := encodePublicFlagDefault(fl.Default)
		out = append(out, &pb.PublicFlagDecl{
			Key:          fl.Key,
			Source:       fl.Source,
			SettingsKey:  fl.SettingsKey,
			Type:         fl.Type,
			DefaultValue: defaultJSON,
		})
	}
	return out
}

// encodePublicFlagDefault marshals the flag's default Go value into JSON. A
// failure (e.g. unsupported value type) collapses to "" so the host treats
// the flag as having no default; we log the error so plugin authors notice.
func encodePublicFlagDefault(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		slog.Default().Warn("plugin-sdk: marshal PublicFlag default failed", "error", err)
		return ""
	}
	return string(b)
}
