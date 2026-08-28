package service

import (
	"context"
	"encoding/json"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestValidateAccountTemperatureCredentials(t *testing.T) {
	tests := []struct {
		name        string
		credentials map[string]any
		wantErr     bool
	}{
		{name: "legacy account inherits", credentials: map[string]any{}},
		{name: "inherit without value", credentials: map[string]any{"temperature_mode": "inherit"}},
		{name: "omit with stale value", credentials: map[string]any{"temperature_mode": "omit", "temperature": 0.7}},
		{name: "override accepts zero", credentials: map[string]any{"temperature_mode": "override", "temperature": 0.0}},
		{name: "override accepts json number", credentials: map[string]any{"temperature_mode": "override", "temperature": json.Number("0.75")}},
		{name: "override requires value", credentials: map[string]any{"temperature_mode": "override"}, wantErr: true},
		{name: "unknown mode", credentials: map[string]any{"temperature_mode": "automatic"}, wantErr: true},
		{name: "mode must be string", credentials: map[string]any{"temperature_mode": true}, wantErr: true},
		{name: "temperature must be numeric", credentials: map[string]any{"temperature_mode": "override", "temperature": "0.7"}, wantErr: true},
		{name: "temperature must be finite", credentials: map[string]any{"temperature_mode": "override", "temperature": json.Number("1e9999")}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAccountTemperatureCredentials(tt.credentials)
			if tt.wantErr {
				require.Error(t, err)
				require.True(t, infraerrors.IsBadRequest(err))
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestApplyAccountTemperaturePolicyTopLevel(t *testing.T) {
	tests := []struct {
		name        string
		credentials map[string]any
		body        string
		wantExists  bool
		wantValue   float64
		wantErr     bool
	}{
		{name: "inherit preserves caller value", credentials: map[string]any{}, body: `{"temperature":0}`, wantExists: true, wantValue: 0},
		{name: "inherit leaves missing value absent", credentials: map[string]any{"temperature_mode": "inherit"}, body: `{}`},
		{name: "inherit treats null as omitted", credentials: map[string]any{"temperature_mode": "inherit"}, body: `{"temperature":null}`},
		{name: "override replaces caller value", credentials: map[string]any{"temperature_mode": "override", "temperature": 0.35}, body: `{"temperature":1}`, wantExists: true, wantValue: 0.35},
		{name: "override adds missing value", credentials: map[string]any{"temperature_mode": "override", "temperature": 0.6}, body: `{}`, wantExists: true, wantValue: 0.6},
		{name: "omit removes caller value", credentials: map[string]any{"temperature_mode": "omit"}, body: `{"temperature":0.8}`},
		{name: "invalid caller type", credentials: map[string]any{}, body: `{"temperature":"0.8"}`, wantErr: true},
		{name: "invalid caller non finite value", credentials: map[string]any{}, body: `{"temperature":1e9999}`, wantErr: true},
		{name: "invalid account policy", credentials: map[string]any{"temperature_mode": "override"}, body: `{}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyAccountTemperaturePolicy([]byte(tt.body), &Account{Credentials: tt.credentials}, temperaturePathTopLevel)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			value := gjson.GetBytes(got, "temperature")
			require.Equal(t, tt.wantExists, value.Exists())
			if tt.wantExists {
				require.InDelta(t, tt.wantValue, value.Float(), 1e-9)
			}
		})
	}
}

func TestApplyAccountTemperaturePolicyGemini(t *testing.T) {
	override := &Account{Credentials: map[string]any{"temperature_mode": "override", "temperature": 0.25}}
	got, err := applyAccountTemperaturePolicy([]byte(`{"generationConfig":{"topP":0.9}}`), override, temperaturePathGemini)
	require.NoError(t, err)
	require.InDelta(t, 0.25, gjson.GetBytes(got, "generationConfig.temperature").Float(), 1e-9)
	require.InDelta(t, 0.9, gjson.GetBytes(got, "generationConfig.topP").Float(), 1e-9)

	omit := &Account{Credentials: map[string]any{"temperature_mode": "omit"}}
	got, err = applyAccountTemperaturePolicy(got, omit, temperaturePathGemini)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(got, "generationConfig.temperature").Exists())
	require.InDelta(t, 0.9, gjson.GetBytes(got, "generationConfig.topP").Float(), 1e-9)
}

func TestApplyResolvedAccountTemperaturePolicyUsesSparkParent(t *testing.T) {
	parentID := int64(41)
	parent := Account{
		ID:       parentID,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"temperature_mode": "override",
			"temperature":      0.15,
		},
	}
	shadow := &Account{
		ID:              42,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		Credentials:     map[string]any{"temperature_mode": "omit"},
	}

	got, err := applyResolvedAccountTemperaturePolicy(
		context.Background(),
		stubOpenAIAccountRepo{accounts: []Account{parent}},
		shadow,
		[]byte(`{"temperature":0.9}`),
		temperaturePathTopLevel,
	)

	require.NoError(t, err)
	require.InDelta(t, 0.15, gjson.GetBytes(got, "temperature").Float(), 1e-9)
}
