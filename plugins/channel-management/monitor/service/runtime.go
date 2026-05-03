package monitorservice

import (
	"context"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// MonitorRuntime is the typed view of the channel-monitor settings tab.
// Field names match plugins/channel-management/monitor/settings/
// settings_schema.json — the JSON tags drive vue-json-schema-form on the
// admin side and SettingsClient.GetTyped on the plugin side.
type MonitorRuntime struct {
	Enabled            bool `json:"enabled"`
	DefaultIntervalSec int  `json:"defaultIntervalSec"`
	TemplateMaxBodyKB  int  `json:"templateMaxBodyKB"`
	DailyRollupHourUTC int  `json:"dailyRollupHourUTC"`
}

// defaultMonitorRuntime mirrors settings_defaults.json so callers can fall
// back to a sensible value when the SettingsClient is unavailable (older
// host) or returns ErrSettingNotFound (admin has not saved yet).
func defaultMonitorRuntime() MonitorRuntime {
	return MonitorRuntime{
		Enabled:            true,
		DefaultIntervalSec: 60,
		TemplateMaxBodyKB:  16,
		DailyRollupHourUTC: 2,
	}
}

// LoadMonitorRuntime fetches the current runtime config via the SDK
// SettingsClient. Failures (missing key, transport error, nil client)
// surface as warnings + the embedded default — the user-facing list /
// admin endpoints stay functional, the master Enabled switch just falls
// back to "true" so we don't accidentally silence an admin who never
// opened the Settings tab.
func LoadMonitorRuntime(ctx context.Context, settings pluginsdk.SettingsClient) MonitorRuntime {
	def := defaultMonitorRuntime()
	if settings == nil {
		return def
	}
	rt := def
	loadBool(ctx, settings, "enabled", &rt.Enabled)
	loadInt(ctx, settings, "defaultIntervalSec", &rt.DefaultIntervalSec)
	loadInt(ctx, settings, "templateMaxBodyKB", &rt.TemplateMaxBodyKB)
	loadInt(ctx, settings, "dailyRollupHourUTC", &rt.DailyRollupHourUTC)
	return rt
}

func loadBool(ctx context.Context, s pluginsdk.SettingsClient, key string, dst *bool) {
	var v bool
	if err := s.GetTyped(ctx, key, &v); err == nil {
		*dst = v
	}
}

func loadInt(ctx context.Context, s pluginsdk.SettingsClient, key string, dst *int) {
	var v int
	if err := s.GetTyped(ctx, key, &v); err == nil && v != 0 {
		*dst = v
	}
}
