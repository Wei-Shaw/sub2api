// Package service — runtime view of the available-channels feature switch.
//
// AvailableChannelsRuntime is the lightweight view of the
// `availableChannelsEnabled` boolean exposed under the same SettingsClient
// namespace as the channel-monitor settings (monitor/settings/
// settings_schema.json). The plugin only owns one settings namespace so
// monitor + available-channels share it — each runtime helper unmarshals
// only the fields it cares about, keeping the responsibilities decoupled.
//
// The user-facing handler calls LoadAvailableChannelsRuntime per request
// (the SDK SettingsClient cache makes this cheap). Failures fall back to
// "disabled" (fail-closed) — matching the opt-in default. This mirrors
// host commit 9ba42aa55 GetAvailableChannelsRuntime.
package service

import (
	"context"
	stderrors "errors"
	"log/slog"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// AvailableChannelsRuntime is the typed view of the available-channels
// settings slice. Field names match the JSON property in
// monitor/settings/settings_schema.json.
type AvailableChannelsRuntime struct {
	Enabled bool `json:"availableChannelsEnabled"`
}

// defaultAvailableChannelsRuntime mirrors settings_defaults.json so callers
// can fall back to a sensible value when the SettingsClient is unavailable
// or returns ErrSettingNotFound. Default is opt-in (Enabled=false).
func defaultAvailableChannelsRuntime() AvailableChannelsRuntime {
	return AvailableChannelsRuntime{Enabled: false}
}

// LoadAvailableChannelsRuntime fetches the current runtime config via the
// SDK SettingsClient. Failures (missing key, transport error, nil client)
// surface as warnings + the embedded default — fail-closed because the
// feature is opt-in (operator flips it on intentionally). This mirrors
// the host's GetAvailableChannelsRuntime behaviour.
func LoadAvailableChannelsRuntime(
	ctx context.Context,
	settings pluginsdk.SettingsClient,
) AvailableChannelsRuntime {
	def := defaultAvailableChannelsRuntime()
	if settings == nil {
		return def
	}
	var enabled bool
	if err := settings.GetTyped(ctx, "availableChannelsEnabled", &enabled); err != nil {
		if !stderrors.Is(err, pluginsdk.ErrSettingNotFound) {
			slog.Warn("channel-management: load available-channels runtime settings failed", "error", err)
		}
		return def
	}
	return AvailableChannelsRuntime{Enabled: enabled}
}
