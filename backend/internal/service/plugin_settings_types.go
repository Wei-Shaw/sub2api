// Package service — plugin_settings_types.go
//
// Shared types, errors, and constants for the plugin_settings subsystem.
// Split out of plugin_settings_service.go (V5 W3) to keep each file under
// the 500-line house limit; the rest of the implementation lives in
// plugin_settings_service.go (struct + ctor), plugin_settings_schema.go
// (schema lifecycle), plugin_settings_io.go (Get/Set), and
// plugin_settings_pubsub.go (Subscribe/notify).
package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// schemaVersionUndeclared is the sentinel stored in
// plugin_settings_schemas.schema_version (and plugin_settings.
// schema_version_at_write) when the plugin did not declare a version.
// See SETTINGS-V2-DESIGN §1.2.
const schemaVersionUndeclared = "0"

// PropertyVisibility constants — values allowed in PropertyMetadata.Visibility.
// See SETTINGS-V2-DESIGN §1.4.
const (
	PropertyVisibilityFrontend = "frontend"
	PropertyVisibilityBackend  = "backend"
	PropertyVisibilitySecret   = "secret"
)

// normalizeSchemaVersion returns v unchanged when non-empty, otherwise the
// "0" sentinel from SETTINGS-V2-DESIGN §1.2. Centralising this default
// keeps the read/write paths from drifting on the empty-string fallback.
func normalizeSchemaVersion(v string) string {
	if v == "" {
		return schemaVersionUndeclared
	}
	return v
}

// ErrPluginSettingsSchemaMissing is returned when an admin tries to
// write to a plugin namespace that has not registered a schema yet.
// Bouncing the request loudly is preferable to silently storing
// arbitrary JSON the plugin will never read.
var ErrPluginSettingsSchemaMissing = errors.New("plugin_settings: no schema registered for plugin")

// ErrInvalidSchemaVisibility is returned by RegisterSchema when a property
// declares an x-visibility outside the allowed set (frontend|backend|secret).
// Maps to errcode PLUGIN_SETTINGS_SCHEMA_INVALID_VISIBILITY in
// SETTINGS-V2-DESIGN §7.2.
var ErrInvalidSchemaVisibility = errors.New("plugin_settings: x-visibility must be one of frontend|backend|secret")

// ErrPluginSettingsValidation is returned when a write fails JSON Schema
// validation. The admin handler maps it to HTTP 422.
type ErrPluginSettingsValidation struct {
	Plugin string
	Key    string
	Reason string
}

func (e *ErrPluginSettingsValidation) Error() string {
	return fmt.Sprintf("plugin_settings: validation failed for %s/%s: %s", e.Plugin, e.Key, e.Reason)
}

// ErrPluginSettingsBackendOnly is returned by SetByKey when an admin write
// targets a key whose schema declares x-visibility=backend. Maps to errcode
// PLUGIN_SETTINGS_BACKEND_ONLY (HTTP 403) in SETTINGS-V2-DESIGN §7.2.
type ErrPluginSettingsBackendOnly struct {
	Plugin string
	Key    string
}

func (e *ErrPluginSettingsBackendOnly) Error() string {
	return fmt.Sprintf("plugin_settings: %s/%s is backend-only and not writable via admin API", e.Plugin, e.Key)
}

// SetSource identifies the caller of SetByKey so the service can apply
// visibility rules. Admin sources are subject to the backend-only guard;
// internal sources skip it. See SETTINGS-V2-DESIGN §4.3.
type SetSource int

const (
	// SetSourceUnknown is the zero value. Treated as admin for safety so a
	// caller that forgets to set a source cannot accidentally bypass the
	// backend-only guard.
	SetSourceUnknown SetSource = iota
	// SetSourceAdmin is the admin REST handler. Backend-only writes are
	// rejected with ErrPluginSettingsBackendOnly.
	SetSourceAdmin
	// SetSourceInternal is host-side code (jobs, hooks, plugin manager
	// reload) that may legitimately mutate backend-only keys.
	SetSourceInternal
)

// PropertyMetadata mirrors the per-property marker triple from
// plugin_settings_schemas.properties_meta. Re-defined in this package to
// avoid pulling plugin-sdk into the host service import graph; field shape
// matches plugin-sdk/manifest.go PropertyMetadata one-to-one (visibility /
// deprecated / requires_reload). See SETTINGS-V2-DESIGN §1.4.
type PropertyMetadata struct {
	Visibility     string `json:"visibility"`
	Deprecated     string `json:"deprecated"`
	RequiresReload bool   `json:"requires_reload"`
}

// RegisterSchemaInput is the V5/W6 SETTINGS-V2 envelope for plugin schema
// registration. PluginManager builds it from ManifestResponse fields; the
// fields mirror SettingsSchemaDoc (Schema/Defaults/Version/PropertyMeta)
// plus a PluginName scoping the request. See SETTINGS-V2-DESIGN §4.1.
type RegisterSchemaInput struct {
	// PluginName scopes every other field. Empty is rejected.
	PluginName string
	// SchemaJSON is Manifest.SettingsSchema.Schema bytes. Empty triggers
	// schema deletion (existing behaviour preserved).
	SchemaJSON []byte
	// DefaultsJSON is Manifest.SettingsSchema.Defaults bytes. Empty is
	// normalised to "{}".
	DefaultsJSON []byte
	// SchemaVersion is Manifest.SettingsSchema.Version. Empty is
	// normalised to schemaVersionUndeclared ("0") host-side.
	SchemaVersion string
	// PropertiesMetaJSON is the SDK-authoritative serialisation of
	// SettingsSchemaDoc.PropertyMeta (a JSON object keyed by top-level
	// property name). When non-empty it wins; when empty the host derives
	// the meta from x-visibility / x-deprecated / x-requires-reload vendor
	// extensions inside SchemaJSON. See SETTINGS-V2-DESIGN §4.1 / §4.2.
	PropertiesMetaJSON []byte
}

// PluginSettingsChange is the host-side event the gRPC server fans out
// on its Watch streams. It mirrors the SDK's SettingsChange but lives in
// the service package so both the gRPC server and the admin handler can
// import it without forming a cycle.
type PluginSettingsChange struct {
	Plugin   string
	Key      string
	Value    json.RawMessage
	Revision int64
	// RequiresReload mirrors the schema's x-requires-reload marker for the
	// changed key. PluginManager subscribes to this signal to coalesce a
	// plugin reload after admin saves; plugin SDK clients ignore the field.
	// See SETTINGS-V2-DESIGN §4.3 / §4.4.
	RequiresReload bool
}

// PluginSettingsSchemaInfo is the admin-facing description of a plugin's
// current schema + values. The handler returns this from GET; the UI
// renders the schema with vue-json-schema-form (or the fallback).
//
// V5/W6 SETTINGS-V2 fields (SchemaVersion / PropertiesMeta / SecretKeys)
// expose the per-property marker triple recorded in
// plugin_settings_schemas.properties_meta plus the schema_version stamp
// so the admin UI widget map can pick the correct widget per property.
// See DESIGN §4.6.
type PluginSettingsSchemaInfo struct {
	Plugin    string                     `json:"plugin"`
	Schema    json.RawMessage            `json:"schema"`
	Defaults  json.RawMessage            `json:"defaults"`
	Values    map[string]json.RawMessage `json:"values"` // secret keys → null
	UpdatedAt time.Time                  `json:"updated_at"`

	// SchemaVersion mirrors plugin_settings_schemas.schema_version.
	// "0" (schemaVersionUndeclared) when the plugin omitted the field.
	SchemaVersion string `json:"schema_version"`

	// PropertiesMeta is the marker triple per top-level property. Keys
	// match Schema's top-level properties; absent keys default to
	// {visibility:"frontend", deprecated:"", requires_reload:false}.
	PropertiesMeta map[string]PropertyMetadata `json:"properties_meta"`

	// SecretKeys is the sorted list of properties with visibility=="secret"
	// that have a stored value (so the UI can render "已配置"). Values for
	// these keys in the Values map are masked to JSON null. Empty slice
	// when no secrets are configured.
	SecretKeys []string `json:"secret_keys"`
}
