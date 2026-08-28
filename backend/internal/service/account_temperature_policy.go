package service

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type accountTemperatureMode string
type temperaturePath string

const (
	accountTemperatureModeInherit  accountTemperatureMode = "inherit"
	accountTemperatureModeOverride accountTemperatureMode = "override"
	accountTemperatureModeOmit     accountTemperatureMode = "omit"

	temperaturePathTopLevel temperaturePath = "temperature"
	temperaturePathGemini   temperaturePath = "generationConfig.temperature"
)

type accountTemperaturePolicy struct {
	mode        accountTemperatureMode
	temperature float64
}

func validateAccountTemperatureCredentials(credentials map[string]any) error {
	_, err := parseAccountTemperaturePolicy(credentials)
	return err
}

func hasAccountTemperatureCredentialUpdate(credentials map[string]any) bool {
	if credentials == nil {
		return false
	}
	_, hasMode := credentials["temperature_mode"]
	_, hasValue := credentials["temperature"]
	return hasMode || hasValue
}

func parseAccountTemperaturePolicy(credentials map[string]any) (accountTemperaturePolicy, error) {
	policy := accountTemperaturePolicy{mode: accountTemperatureModeInherit}
	if credentials == nil {
		return policy, nil
	}

	if rawMode, ok := credentials["temperature_mode"]; ok {
		mode, valid := rawMode.(string)
		if !valid {
			return policy, invalidAccountTemperature("temperature_mode must be inherit, override, or omit")
		}
		policy.mode = accountTemperatureMode(strings.TrimSpace(mode))
	}

	switch policy.mode {
	case accountTemperatureModeInherit, accountTemperatureModeOverride, accountTemperatureModeOmit:
	default:
		return policy, invalidAccountTemperature("temperature_mode must be inherit, override, or omit")
	}

	rawTemperature, hasTemperature := credentials["temperature"]
	if !hasTemperature || rawTemperature == nil {
		if policy.mode == accountTemperatureModeOverride {
			return policy, invalidAccountTemperature("temperature is required when temperature_mode is override")
		}
		return policy, nil
	}

	temperature, valid := accountTemperatureNumber(rawTemperature)
	if !valid {
		return policy, invalidAccountTemperature("temperature must be a finite number")
	}
	policy.temperature = temperature
	return policy, nil
}

func applyAccountTemperaturePolicy(body []byte, account *Account, path temperaturePath) ([]byte, error) {
	if !gjson.ValidBytes(body) {
		return nil, infraerrors.BadRequest("INVALID_REQUEST_BODY", "request body must be valid JSON")
	}

	var credentials map[string]any
	if account != nil {
		credentials = account.Credentials
	}
	policy, err := parseAccountTemperaturePolicy(credentials)
	if err != nil {
		return nil, err
	}

	field := gjson.GetBytes(body, string(path))
	if field.Exists() && field.Type != gjson.Number && field.Type != gjson.Null {
		return nil, infraerrors.BadRequest("INVALID_TEMPERATURE", "temperature must be a number or null")
	}
	if field.Type == gjson.Number {
		value := field.Float()
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, infraerrors.BadRequest("INVALID_TEMPERATURE", "temperature must be a finite number")
		}
	}

	normalized := body
	if field.Exists() && field.Type == gjson.Null {
		normalized, err = sjson.DeleteBytes(normalized, string(path))
		if err != nil {
			return nil, infraerrors.BadRequest("INVALID_TEMPERATURE", "temperature could not be removed from request body")
		}
	}

	switch policy.mode {
	case accountTemperatureModeInherit:
		return normalized, nil
	case accountTemperatureModeOmit:
		updated, deleteErr := sjson.DeleteBytes(normalized, string(path))
		if deleteErr != nil {
			return nil, infraerrors.BadRequest("INVALID_TEMPERATURE", "temperature could not be removed from request body")
		}
		return updated, nil
	case accountTemperatureModeOverride:
		updated, setErr := sjson.SetBytes(normalized, string(path), policy.temperature)
		if setErr != nil {
			return nil, infraerrors.BadRequest("INVALID_TEMPERATURE", "temperature could not be written to request body")
		}
		return updated, nil
	default:
		return nil, invalidAccountTemperature("temperature_mode must be inherit, override, or omit")
	}
}

func applyResolvedAccountTemperaturePolicy(
	ctx context.Context,
	repo AccountRepository,
	account *Account,
	body []byte,
	path temperaturePath,
) ([]byte, error) {
	credentialAccount, err := resolveCredentialAccount(ctx, repo, account)
	if err != nil {
		return nil, err
	}
	return applyAccountTemperaturePolicy(body, credentialAccount, path)
}

func accountTemperatureNumber(value any) (float64, bool) {
	var number float64
	switch v := value.(type) {
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, false
		}
		number = parsed
	case float64:
		number = v
	case float32:
		number = float64(v)
	case int:
		number = float64(v)
	case int8:
		number = float64(v)
	case int16:
		number = float64(v)
	case int32:
		number = float64(v)
	case int64:
		number = float64(v)
	case uint:
		number = float64(v)
	case uint8:
		number = float64(v)
	case uint16:
		number = float64(v)
	case uint32:
		number = float64(v)
	case uint64:
		number = float64(v)
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}

func invalidAccountTemperature(message string) error {
	return infraerrors.BadRequest("INVALID_ACCOUNT_TEMPERATURE", message)
}
