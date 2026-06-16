// Package service — plugin_settings_schema.go
//
// Schema lifecycle for the plugin_settings subsystem: compile, persist,
// seed defaults, and resolve property metadata. See plugin_settings_types.go
// for the shared types and constants this file references.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// RegisterSchema is the legacy entry point retained for callers that
// have not yet migrated to RegisterSchemaWithInput. It forwards to the
// V2 implementation with empty version + properties_meta — the V2
// implementation derives meta from schema vendor extensions in that
// case (see SETTINGS-V2-DESIGN §4.1).
//
// New callers should build a RegisterSchemaInput so they can carry
// SchemaVersion + PropertiesMetaJSON from the plugin manifest.
func (s *PluginSettingsService) RegisterSchema(
	ctx context.Context, pluginName string, schemaJSON, defaultsJSON []byte,
) error {
	return s.RegisterSchemaWithInput(ctx, RegisterSchemaInput{
		PluginName:   pluginName,
		SchemaJSON:   schemaJSON,
		DefaultsJSON: defaultsJSON,
	})
}

// RegisterSchemaWithInput is the V5/W6 SETTINGS-V2 entry point used by
// PluginManager during plugin start. It compiles the schema, persists
// the raw bytes + schema_version + properties_meta for GET responses,
// and seeds defaults for keys that have not been written yet.
//
// Calling RegisterSchemaWithInput again with the same plugin name
// replaces the cached schema. When schema_version changes the service
// drops existing watch subscribers so SDK clients reconnect and pick
// up the new schema_version (see SETTINGS-V2-DESIGN §4.5; the drop
// is wired in commit 2).
func (s *PluginSettingsService) RegisterSchemaWithInput(
	ctx context.Context, in RegisterSchemaInput,
) error {
	if in.PluginName == "" {
		return errors.New("plugin_settings: empty plugin name")
	}
	if len(in.SchemaJSON) == 0 {
		// Nothing to register — clear any cached entry so a previously
		// schema-bearing plugin can drop the requirement on next restart.
		s.mu.Lock()
		delete(s.compiledSchemas, in.PluginName)
		delete(s.rawSchemas, in.PluginName)
		delete(s.defaults, in.PluginName)
		s.mu.Unlock()
		_, err := s.db.ExecContext(ctx,
			`DELETE FROM plugin_settings_schemas WHERE plugin_name = $1`, in.PluginName)
		return err
	}
	compiled, err := compileSchema(in.SchemaJSON)
	if err != nil {
		return fmt.Errorf("plugin_settings: compile schema for %s: %w", in.PluginName, err)
	}

	// Resolve properties_meta: SDK-authoritative bytes win when present,
	// otherwise derive from x-visibility / x-deprecated / x-requires-reload
	// vendor extensions in the schema. See SETTINGS-V2-DESIGN §4.2.
	meta, err := s.resolvePropertiesMeta(in.SchemaJSON, in.PropertiesMetaJSON)
	if err != nil {
		return err
	}
	if err := validateVisibilities(in.PluginName, meta); err != nil {
		return err
	}

	schemaVersion := normalizeSchemaVersion(in.SchemaVersion)

	// V5/W6 SETTINGS-V2 §4.5: capture the previous schema_version BEFORE we
	// overwrite the in-memory cache so a bump can be detected and existing
	// subscribers can be force-disconnected. prevVersion=="" means the
	// service has not seen this plugin before in this process lifetime
	// (cold start) — never drop on cold start because there are no
	// subscribers yet to drop, and we do not want to disturb cache
	// rehydration in SchemaInfo (which also leaves the entry empty until
	// first read).
	s.mu.RLock()
	prevVersion := s.schemaVersions[in.PluginName]
	s.mu.RUnlock()

	// Marshal meta back to JSON for persistence. The host always writes
	// `{}` (never SQL NULL) so handlers do not need to nil-check.
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("plugin_settings: marshal properties_meta: %w", err)
	}

	rawSchema := append(json.RawMessage(nil), in.SchemaJSON...)
	rawDefaults := append(json.RawMessage(nil), in.DefaultsJSON...)
	if len(rawDefaults) == 0 {
		rawDefaults = json.RawMessage("{}")
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO plugin_settings_schemas
			(plugin_name, schema_json, defaults_json, schema_version, properties_meta, updated_at)
		VALUES ($1, $2::jsonb, $3::jsonb, $4, $5::jsonb, NOW())
		ON CONFLICT (plugin_name) DO UPDATE
		   SET schema_json     = EXCLUDED.schema_json,
		       defaults_json   = EXCLUDED.defaults_json,
		       schema_version  = EXCLUDED.schema_version,
		       properties_meta = EXCLUDED.properties_meta,
		       updated_at      = NOW()
	`, in.PluginName, string(rawSchema), string(rawDefaults), schemaVersion, string(metaJSON)); err != nil {
		return fmt.Errorf("plugin_settings: persist schema: %w", err)
	}

	s.mu.Lock()
	s.compiledSchemas[in.PluginName] = compiled
	s.rawSchemas[in.PluginName] = rawSchema
	s.defaults[in.PluginName] = rawDefaults
	s.schemaVersions[in.PluginName] = schemaVersion
	s.propertiesMeta[in.PluginName] = meta
	s.mu.Unlock()

	// Force every existing subscriber to reconnect when schema_version
	// changed. SDK Watch loops detect the closed channel and re-snapshot
	// under the new schema, satisfying DESIGN §4.5 ("schema-version bump
	// requires fresh snapshot for correctness").
	if prevVersion != "" && prevVersion != schemaVersion {
		s.dropAllSubscribersForPlugin(in.PluginName)
		s.logger.Info("plugin_settings: schema_version changed; dropped subscribers to force resync",
			"plugin", in.PluginName, "prev", prevVersion, "current", schemaVersion)
	}

	if err := s.seedDefaults(ctx, in.PluginName, rawDefaults); err != nil {
		s.logger.Warn("plugin_settings: seed defaults failed",
			"plugin", in.PluginName, "error", err)
	}
	return nil
}

// seedDefaults writes default values for keys that do not exist yet.
// Existing values (whether plugin- or admin-supplied) are left alone.
//
// V5/W6 SETTINGS-V2 invariants:
//   - Secret-visibility keys are NEVER seeded. A secret with no row means
//     "not configured / cleared by admin"; pre-populating from a manifest
//     constant would silently undo admin clears on every plugin restart
//     (the row is gone after clear, so ON CONFLICT DO NOTHING does not
//     protect us). Operators must enter secrets explicitly.
//   - schema_version_at_write is populated from the current cached
//     schema_version so seeded rows do not appear as "stored=0,current=1"
//     drift the moment we start surfacing the version fields on Get.
func (s *PluginSettingsService) seedDefaults(
	ctx context.Context, pluginName string, defaultsJSON json.RawMessage,
) error {
	var defaults map[string]json.RawMessage
	if err := json.Unmarshal(defaultsJSON, &defaults); err != nil {
		return fmt.Errorf("unmarshal defaults: %w", err)
	}
	s.mu.RLock()
	meta := s.propertiesMeta[pluginName]
	currentVersion := normalizeSchemaVersion(s.schemaVersions[pluginName])
	s.mu.RUnlock()
	for key, val := range defaults {
		if m, ok := meta[key]; ok && m.Visibility == PropertyVisibilitySecret {
			continue
		}
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO plugin_settings (plugin_name, key, value_json, revision, schema_version_at_write, updated_at)
			VALUES ($1, $2, $3::jsonb, 1, $4, NOW())
			ON CONFLICT (plugin_name, key) DO NOTHING
		`, pluginName, key, string(val), currentVersion)
		if err != nil {
			return fmt.Errorf("seed default %s/%s: %w", pluginName, key, err)
		}
	}
	return nil
}

// UnregisterSchema clears in-memory caches for a plugin that has been
// disabled. Stored values are left intact so a re-enable does not lose
// the admin's edits.
func (s *PluginSettingsService) UnregisterSchema(pluginName string) {
	s.mu.Lock()
	delete(s.compiledSchemas, pluginName)
	delete(s.rawSchemas, pluginName)
	delete(s.defaults, pluginName)
	delete(s.schemaVersions, pluginName)
	delete(s.propertiesMeta, pluginName)
	s.mu.Unlock()
}

// compileSchema turns raw JSON Schema bytes into a compiled validator.
// jsonschema v5 supports Draft-07 + Draft 2020-12; we let the schema
// declare $schema if it wants to opt into a specific draft.
func compileSchema(raw []byte) (*jsonschema.Schema, error) {
	return jsonschema.CompileString("plugin-settings.json", string(raw))
}

// extractMetaFromSchema parses x-visibility / x-deprecated / x-requires-reload
// vendor extensions out of the top-level schema "properties" map. Nested
// properties are intentionally ignored — SETTINGS-V2-DESIGN §1.4 pins the
// marker scope to top-level keys only.
//
// Empty schemas (no top-level "properties") return an empty map without an
// error so callers do not need to special-case schema-less plugins.
func (s *PluginSettingsService) extractMetaFromSchema(schemaJSON []byte) (map[string]PropertyMetadata, error) {
	if len(schemaJSON) == 0 {
		return map[string]PropertyMetadata{}, nil
	}
	var doc struct {
		Properties map[string]map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schemaJSON, &doc); err != nil {
		return nil, fmt.Errorf("plugin_settings: extract meta: unmarshal schema: %w", err)
	}
	out := make(map[string]PropertyMetadata, len(doc.Properties))
	for prop, node := range doc.Properties {
		var m PropertyMetadata
		if raw, ok := node["x-visibility"]; ok {
			_ = json.Unmarshal(raw, &m.Visibility)
		}
		if raw, ok := node["x-deprecated"]; ok {
			_ = json.Unmarshal(raw, &m.Deprecated)
		}
		if raw, ok := node["x-requires-reload"]; ok {
			_ = json.Unmarshal(raw, &m.RequiresReload)
		}
		// Normalise empty visibility to the "frontend" default (§1.4).
		if m.Visibility == "" {
			m.Visibility = PropertyVisibilityFrontend
		}
		out[prop] = m
	}
	return out, nil
}

// resolvePropertiesMeta picks between two sources for the marker triple:
//  1. SDK-authoritative bytes from RegisterSchemaInput.PropertiesMetaJSON
//     (the plugin SDK has serialised SettingsSchemaDoc.PropertyMeta into
//     ManifestResponse.settings_properties_meta_json).
//  2. Fallback: derive from x-visibility / x-deprecated / x-requires-reload
//     vendor extensions inside SchemaJSON.
//
// When source (1) is non-empty it wins (per SETTINGS-V2-DESIGN §3.3.2 / §4.1)
// — the SDK serialisation is canonical because the plugin author can populate
// SettingsSchemaDoc.PropertyMeta even when their schema bytes come from an
// embed.FS and they cannot edit them at runtime.
func (s *PluginSettingsService) resolvePropertiesMeta(schemaJSON, sdkMeta []byte) (map[string]PropertyMetadata, error) {
	if len(sdkMeta) > 0 {
		var meta map[string]PropertyMetadata
		if err := json.Unmarshal(sdkMeta, &meta); err != nil {
			return nil, fmt.Errorf("plugin_settings: parse properties_meta: %w", err)
		}
		// Apply the same "" → "frontend" normalisation the schema-derivation
		// path uses so callers see a single shape regardless of source.
		for k, v := range meta {
			if v.Visibility == "" {
				v.Visibility = PropertyVisibilityFrontend
				meta[k] = v
			}
		}
		if meta == nil {
			meta = map[string]PropertyMetadata{}
		}
		return meta, nil
	}
	return s.extractMetaFromSchema(schemaJSON)
}

// validateVisibilities rejects any property declaring a visibility outside
// the allowed set (frontend|backend|secret). Empty visibility is permitted
// because callers normalise it to "frontend" before reaching this guard.
// The Backstage-style fail-fast (Curator decision 2.1, point 6) means a
// plugin with an invalid x-visibility refuses to register at all.
func validateVisibilities(pluginName string, meta map[string]PropertyMetadata) error {
	for prop, m := range meta {
		switch m.Visibility {
		case "", PropertyVisibilityFrontend, PropertyVisibilityBackend, PropertyVisibilitySecret:
			// ok
		default:
			return fmt.Errorf("%w: plugin=%s property=%s value=%q",
				ErrInvalidSchemaVisibility, pluginName, prop, m.Visibility)
		}
	}
	return nil
}

// validateAgainst runs jsonschema validation against the sub-schema for
// the supplied key. Writes are key-scoped (admin saves a single property
// at a time), so we deliberately validate the value against the property's
// own sub-schema instead of wrapping it into `{key: value}` and running
// the whole-object validator — wrapping incorrectly triggers top-level
// `required` arrays for properties that are not part of the partial save.
//
// See SETTINGS-V2-INSPECT §4 (table row for `validateAgainst`, line 114):
// the wrapper approach lets `required` enforce on partial saves only when
// the partial object happens to satisfy them. Symptom seen on test deploy:
// `PUT /api/v1/admin/plugin-settings/channel-management/defaultIntervalSec`
// with body `{"value": 120}` returned HTTP 422 because the schema's
// top-level `required: ["enabled"]` could not be satisfied by the wrapper
// `{"defaultIntervalSec": 120}`.
//
// The sub-schema validates the value's own constraints (type / minimum /
// maximum / pattern / enum / format / etc.) without dragging in object-level
// constraints that do not apply to a single-property write.
func validateAgainst(schema *jsonschema.Schema, key string, value json.RawMessage) error {
	sub, ok := schema.Properties[key]
	if !ok {
		return &ErrPluginSettingsValidation{Key: key, Reason: "unknown property: " + key}
	}
	var doc any
	if err := json.Unmarshal(value, &doc); err != nil {
		return &ErrPluginSettingsValidation{Key: key, Reason: "decode value: " + err.Error()}
	}
	if err := sub.Validate(doc); err != nil {
		return &ErrPluginSettingsValidation{Key: key, Reason: err.Error()}
	}
	return nil
}
