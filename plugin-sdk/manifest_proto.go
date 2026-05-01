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
		SubscribedEvents: append([]string(nil), m.SubscribedEvents...),
		OwnedTables:      append([]string(nil), m.OwnedTables...),
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
		// schema. We add the canonical CapabilitySettingsOwnRead — the host
		// expands canonical → legacy alias when shipping the approved list
		// back in PluginInitRequest, so plugins still keyed off the legacy
		// "settings_extension" string keep working.
		//
		// Skip when either the canonical or the legacy alias is already
		// present so manifests opting into write-only settings, or pinned to
		// the deprecated alias, are honoured as declared.
		if resp.SettingsSchemaJson != nil &&
			!containsCap(caps, CapabilitySettingsOwnRead) &&
			!containsCap(caps, CapabilitySettingsOwnWrite) &&
			!containsCap(caps, CapabilitySettingsExtension) {
			caps = append(caps, CapabilitySettingsOwnRead)
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
		var labels map[string]string
		if len(item.Labels) > 0 {
			labels = make(map[string]string, len(item.Labels))
			for k, v := range item.Labels {
				labels[k] = v
			}
		}
		var descriptions map[string]string
		if len(item.Descriptions) > 0 {
			descriptions = make(map[string]string, len(item.Descriptions))
			for k, v := range item.Descriptions {
				descriptions[k] = v
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
			Descriptions:     descriptions,
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
