package service

import (
	"encoding/json"
	"math"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	transportErrorCooldownEnabledKey = "transport_error_cooldown_enabled"
	transportErrorCooldownMinutesKey = "transport_error_cooldown_minutes"
	maxTransportErrorCooldownMinutes = 1440
)

// transportErrorCooldown returns zero only for an explicit Anthropic/Bedrock
// opt-out. Missing or malformed stored settings retain the legacy protection.
// Other platforms continue to use their existing cooldown policy.
func (a *Account) transportErrorCooldown() time.Duration {
	if a == nil || a.Platform != PlatformAnthropic {
		return gatewayTransportErrorTempUnschedDuration
	}
	if enabled, ok := a.Extra[transportErrorCooldownEnabledKey].(bool); ok && !enabled {
		return 0
	}
	if minutes, ok := transportErrorCooldownMinutes(a.Extra[transportErrorCooldownMinutesKey]); ok {
		return time.Duration(minutes) * time.Minute
	}
	return gatewayTransportErrorTempUnschedDuration
}

func transportErrorCooldownMinutes(raw any) (int, bool) {
	var value float64
	switch v := raw.(type) {
	case int:
		value = float64(v)
	case int64:
		value = float64(v)
	case float64:
		value = v
	case json.Number:
		var err error
		value, err = v.Float64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	if math.IsNaN(value) || value < 1 || value > maxTransportErrorCooldownMinutes || math.Trunc(value) != value {
		return 0, false
	}
	return int(value), true
}

// ValidateTransportErrorCooldownExtra checks settings before any account mutation.
func ValidateTransportErrorCooldownExtra(extra map[string]any) error {
	if raw, exists := extra[transportErrorCooldownEnabledKey]; exists {
		if _, ok := raw.(bool); !ok {
			return infraerrors.BadRequest("TRANSPORT_ERROR_COOLDOWN_INVALID", "transport_error_cooldown_enabled must be a boolean")
		}
	}
	if raw, exists := extra[transportErrorCooldownMinutesKey]; exists {
		if _, ok := transportErrorCooldownMinutes(raw); !ok {
			return infraerrors.BadRequest("TRANSPORT_ERROR_COOLDOWN_INVALID", "transport_error_cooldown_minutes must be an integer between 1 and 1440")
		}
	}
	return nil
}
