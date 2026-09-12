package service

import "encoding/json"

// UnmarshalJSON retains whether all fields required for a reset decision were
// actually present. A missing/null percentage is unknown, never an observed 0%.
func (w *OpenAIRateLimitWindow) UnmarshalJSON(data []byte) error {
	type wireWindow OpenAIRateLimitWindow
	var value wireWindow
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var presence struct {
		UsedPercent   *float64 `json:"used_percent"`
		ResetAt       *int64   `json:"reset_at"`
		WindowSeconds *int64   `json:"limit_window_seconds"`
	}
	if err := json.Unmarshal(data, &presence); err != nil {
		return err
	}
	*w = OpenAIRateLimitWindow(value)
	w.observationFieldsPresent = presence.UsedPercent != nil && presence.ResetAt != nil && presence.WindowSeconds != nil
	return nil
}
