package monitorservice

import (
	"context"
	stderrors "errors"
	"log/slog"

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
	var rt MonitorRuntime
	if err := settings.GetTyped(ctx, "", &rt); err != nil {
		if !stderrors.Is(err, pluginsdk.ErrSettingNotFound) {
			slog.Warn("channel-monitor: load runtime settings failed", "error", err)
		}
		return def
	}
	if rt.DefaultIntervalSec == 0 {
		rt.DefaultIntervalSec = def.DefaultIntervalSec
	}
	if rt.TemplateMaxBodyKB == 0 {
		rt.TemplateMaxBodyKB = def.TemplateMaxBodyKB
	}
	return rt
}
