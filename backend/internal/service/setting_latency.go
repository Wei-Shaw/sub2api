package service

import "encoding/json"

const SettingKeyFirstTokenThresholdsMS = "first_token_thresholds_ms"

// FirstTokenThresholds defines the inclusive lower bound of each warning color.
// Values are stored in milliseconds; the settings UI displays seconds.
type FirstTokenThresholds struct {
	Warn     int `json:"warn"`
	Slow     int `json:"slow"`
	Critical int `json:"critical"`
}

func DefaultFirstTokenThresholds() FirstTokenThresholds {
	return FirstTokenThresholds{Warn: 10_000, Slow: 30_000, Critical: 60_000}
}

func (t FirstTokenThresholds) Valid() bool {
	return t.Warn > 0 && t.Warn < t.Slow && t.Slow < t.Critical
}

func parseFirstTokenThresholds(raw string) FirstTokenThresholds {
	var thresholds FirstTokenThresholds
	if err := json.Unmarshal([]byte(raw), &thresholds); err != nil || !thresholds.Valid() {
		return DefaultFirstTokenThresholds()
	}
	return thresholds
}
