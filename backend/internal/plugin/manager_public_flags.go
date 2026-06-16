package plugin

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/service"
	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// PluginSettingsReader is the read-only seam PluginManager uses to resolve
// PublicFlagSourceSettings declarations against a plugin's settings store.
//
// Decoupled from PluginSettingsRegistrar because the registrar is the
// write/subscribe surface used by SDK gRPC; flag resolution only needs the
// value-read path. Same concrete type (*service.PluginSettingsService) is
// expected to implement both at wire time.
type PluginSettingsReader interface {
	GetByKey(ctx context.Context, pluginName, key string) (*service.GetByKeyResult, error)
}

// SetPluginSettingsReader installs the read-side seam used by GetPublicFlags
// to resolve PublicFlagSourceSettings declarations. nil disables the lookup;
// PublicFlagSourceSettings flags then fall back to their declared default.
//
// Must be called before Start so the same store the SDK writes through is
// also the one we read for public-flag bootstrap.
func (m *PluginManager) SetPluginSettingsReader(reader PluginSettingsReader) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settingsReader = reader
}

// hostPublicSettingsReservedKeys lists JSON keys host PublicSettings already
// occupies (see backend/internal/handler/dto/settings.go PublicSettings).
// Plugin flags colliding with these are dropped with a warning so a buggy
// or malicious plugin cannot shadow host-controlled toggles.
//
// Keep this list in sync with dto.PublicSettings json tags. The cost of
// drift here is "plugin-flag silently overrides host field" so we err on
// the side of including more keys than strictly needed.
var hostPublicSettingsReservedKeys = map[string]struct{}{
	"registration_enabled":                     {},
	"email_verify_enabled":                     {},
	"force_email_on_third_party_signup":        {},
	"registration_email_suffix_whitelist":      {},
	"promo_code_enabled":                       {},
	"password_reset_enabled":                   {},
	"invitation_code_enabled":                  {},
	"totp_enabled":                             {},
	"turnstile_enabled":                        {},
	"turnstile_site_key":                       {},
	"site_name":                                {},
	"site_logo":                                {},
	"site_subtitle":                            {},
	"api_base_url":                             {},
	"contact_info":                             {},
	"doc_url":                                  {},
	"home_content":                             {},
	"hide_ccs_import_button":                   {},
	"purchase_subscription_enabled":            {},
	"purchase_subscription_url":                {},
	"table_default_page_size":                  {},
	"table_page_size_options":                  {},
	"custom_menu_items":                        {},
	"custom_endpoints":                         {},
	"linuxdo_oauth_enabled":                    {},
	"wechat_oauth_enabled":                     {},
	"wechat_oauth_open_enabled":                {},
	"wechat_oauth_mp_enabled":                  {},
	"wechat_oauth_mobile_enabled":              {},
	"oidc_oauth_enabled":                       {},
	"oidc_oauth_provider_name":                 {},
	"sora_client_enabled":                      {},
	"backend_mode_enabled":                     {},
	"version":                                  {},
	"balance_low_notify_enabled":               {},
	"account_quota_notify_enabled":             {},
	"balance_low_notify_threshold":             {},
	"balance_low_notify_recharge_url":          {},
	"channel_monitor_enabled":                  {},
	"channel_monitor_default_interval_seconds": {},
	"available_channels_enabled":               {},
	"service_quota_enabled":                    {},
	"affiliate_enabled":                        {},
}

// GetPublicFlags returns the union of PublicFlags declared by all currently
// running plugins, with values resolved per-decl:
//
//   - PublicFlagSourceSettings reads via the configured PluginSettingsReader
//     (GetByKey). Missing rows / missing reader / decode errors fall back to
//     the declared Default and emit a warn.
//   - PublicFlagSourceStatic returns the declared Default verbatim.
//
// Conflict resolution:
//
//   - Keys colliding with host PublicSettings (hostPublicSettingsReservedKeys)
//     are dropped with a warn — host fields always win.
//   - When two plugins declare the same key, the first running plugin
//     (iteration order is not stable) wins; subsequent ones are dropped
//     with a warn.
//
// The returned map is always non-nil so callers can range over it without
// nil checks; an empty map means "no plugin contributed any flag".
func (m *PluginManager) GetPublicFlags(ctx context.Context) map[string]any {
	out := map[string]any{}
	if m == nil {
		return out
	}

	m.mu.RLock()
	reader := m.settingsReader
	insts := make([]*PluginInstance, 0, len(m.plugins))
	for _, inst := range m.plugins {
		insts = append(insts, inst)
	}
	m.mu.RUnlock()

	for _, inst := range insts {
		pluginName, decls := snapshotPublicFlagDecls(inst)
		if len(decls) == 0 {
			continue
		}
		for _, decl := range decls {
			key := decl.GetKey()
			if key == "" {
				continue
			}
			if _, reserved := hostPublicSettingsReservedKeys[key]; reserved {
				slog.Warn("plugin: PublicFlag key collides with host PublicSettings, dropping",
					"plugin", pluginName, "key", key)
				continue
			}
			if _, dup := out[key]; dup {
				slog.Warn("plugin: PublicFlag key already contributed by another plugin, dropping",
					"plugin", pluginName, "key", key)
				continue
			}
			value, ok := resolvePublicFlagValue(ctx, reader, pluginName, decl)
			if !ok {
				continue
			}
			out[key] = value
		}
	}
	return out
}

// snapshotPublicFlagDecls returns the plugin name + a copy of its declared
// PublicFlags. Returns ("", nil) when the instance is not running or has no
// manifest. Holds inst.mu only for the snapshot so we never call into
// PluginSettingsReader while a plugin lock is held.
func snapshotPublicFlagDecls(inst *PluginInstance) (string, []*pb.PublicFlagDecl) {
	if inst == nil {
		return "", nil
	}
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.State != StateRunning || inst.Manifest == nil {
		return "", nil
	}
	decls := inst.Manifest.GetPublicFlags()
	if len(decls) == 0 {
		return inst.Manifest.GetName(), nil
	}
	out := make([]*pb.PublicFlagDecl, len(decls))
	copy(out, decls)
	return inst.Manifest.GetName(), out
}

// resolvePublicFlagValue resolves one PublicFlagDecl into a typed value
// suitable for JSON marshalling. The bool ok=false means "drop this flag"
// (e.g. unknown source / unknown type / unrecoverable decode error with no
// usable default).
func resolvePublicFlagValue(
	ctx context.Context,
	reader PluginSettingsReader,
	pluginName string,
	decl *pb.PublicFlagDecl,
) (any, bool) {
	defaultRaw := decl.GetDefaultValue()
	switch decl.GetSource() {
	case pluginsdk.PublicFlagSourceStatic, "":
		return decodePublicFlagJSON(pluginName, decl, defaultRaw)
	case pluginsdk.PublicFlagSourceSettings:
		settingsKey := decl.GetSettingsKey()
		if settingsKey == "" || reader == nil {
			return decodePublicFlagJSON(pluginName, decl, defaultRaw)
		}
		raw, err := reader.GetByKey(ctx, pluginName, settingsKey)
		if err != nil || raw == nil || len(raw.Value) == 0 {
			if err != nil {
				slog.Warn("plugin: PublicFlag settings lookup failed, using default",
					"plugin", pluginName, "key", decl.GetKey(),
					"settings_key", settingsKey, "error", err)
			}
			return decodePublicFlagJSON(pluginName, decl, defaultRaw)
		}
		if v, ok := decodePublicFlagJSON(pluginName, decl, string(raw.Value)); ok {
			return v, true
		}
		// Stored value is unparseable for the declared type → fall back to default.
		return decodePublicFlagJSON(pluginName, decl, defaultRaw)
	default:
		slog.Warn("plugin: PublicFlag has unknown source, dropping",
			"plugin", pluginName, "key", decl.GetKey(), "source", decl.GetSource())
		return nil, false
	}
}

// decodePublicFlagJSON unmarshals a JSON-encoded value into the declared
// type. Empty input is treated as "no value". Returns (nil, false) when the
// flag cannot be reified into the declared type — caller drops it.
func decodePublicFlagJSON(pluginName string, decl *pb.PublicFlagDecl, raw string) (any, bool) {
	if raw == "" {
		return nil, false
	}
	switch decl.GetType() {
	case pluginsdk.PublicFlagTypeBool:
		var v bool
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			slog.Warn("plugin: PublicFlag bool decode failed, dropping",
				"plugin", pluginName, "key", decl.GetKey(), "raw", raw, "error", err)
			return nil, false
		}
		return v, true
	case pluginsdk.PublicFlagTypeString:
		var v string
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			slog.Warn("plugin: PublicFlag string decode failed, dropping",
				"plugin", pluginName, "key", decl.GetKey(), "raw", raw, "error", err)
			return nil, false
		}
		return v, true
	case pluginsdk.PublicFlagTypeNumber:
		var v float64
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			slog.Warn("plugin: PublicFlag number decode failed, dropping",
				"plugin", pluginName, "key", decl.GetKey(), "raw", raw, "error", err)
			return nil, false
		}
		return v, true
	default:
		slog.Warn("plugin: PublicFlag has unknown type, dropping",
			"plugin", pluginName, "key", decl.GetKey(), "type", decl.GetType())
		return nil, false
	}
}
