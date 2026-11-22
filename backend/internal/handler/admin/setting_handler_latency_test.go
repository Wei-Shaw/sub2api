//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateSettingsFirstTokenThresholdsPersistAndReachPublicSettings(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})
	custom := service.FirstTokenThresholds{Warn: 1500, Slow: 5000, Critical: 12000}
	rec := doUpdateSettings(t, h, map[string]any{"first_token_thresholds_ms": custom}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var stored service.FirstTokenThresholds
	require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyFirstTokenThresholdsMS]), &stored))
	require.Equal(t, custom, stored)
	var payload struct {
		Data struct {
			Thresholds service.FirstTokenThresholds `json:"first_token_thresholds_ms"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, custom, payload.Data.Thresholds)
	public, err := h.settingService.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, custom, public.FirstTokenThresholdsMS)
	injected, err := h.settingService.GetPublicSettingsForInjection(context.Background())
	require.NoError(t, err)
	require.Equal(t, custom, injected.(*service.PublicSettingsInjectionPayload).FirstTokenThresholdsMS)
}

func TestUpdateSettingsFirstTokenThresholdsPreservedWhenOmitted(t *testing.T) {
	const stored = `{"warn":1500,"slow":5000,"critical":12000}`
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{service.SettingKeyFirstTokenThresholdsMS: stored})
	rec := doUpdateSettings(t, h, map[string]any{"site_name": "Gateway"}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, stored, repo.values[service.SettingKeyFirstTokenThresholdsMS])
}

func TestUpdateSettingsRejectsInvalidFirstTokenThresholds(t *testing.T) {
	for _, invalid := range []service.FirstTokenThresholds{
		{},
		{Warn: -1, Slow: 5000, Critical: 12000},
		{Warn: 5000, Slow: 1500, Critical: 12000},
		{Warn: 1500, Slow: 5000, Critical: 5000},
	} {
		h, repo := newStepUpSwitchTestHandler(t, map[string]string{})
		rec := doUpdateSettings(t, h, map[string]any{"first_token_thresholds_ms": invalid}, nil)
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		_, written := repo.values[service.SettingKeyFirstTokenThresholdsMS]
		require.False(t, written)
	}
}
