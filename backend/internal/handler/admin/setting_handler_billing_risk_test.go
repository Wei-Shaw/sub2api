//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingHandlerBillingRiskRoundTripHotRefreshAndPreservesOmittedValues(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})

	getRecorder := httptest.NewRecorder()
	getContext, _ := gin.CreateTestContext(getRecorder)
	getContext.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	h.GetSettings(getContext)
	require.Equal(t, http.StatusOK, getRecorder.Code)
	var getResponse response.Response
	require.NoError(t, json.Unmarshal(getRecorder.Body.Bytes(), &getResponse))
	getData, ok := getResponse.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, getData["billing_risk_enabled"])
	require.NotContains(t, getData, "billing_risk_mode")
	require.Equal(t, float64(10), getData["billing_risk_low_balance_threshold"])
	require.Equal(t, float64(1.25), getData["billing_risk_safety_factor"])

	payload := map[string]any{
		"billing_risk_enabled":                    true,
		"billing_risk_low_balance_threshold":      7.5,
		"billing_risk_safety_factor":              1.5,
		"billing_risk_minimum_request_risk":       0.002,
		"billing_risk_overdraft_allowance":        0.35,
		"billing_risk_high_cost_trigger":          2.0,
		"billing_risk_lease_ttl_seconds":          90,
		"billing_risk_refresh_interval_seconds":   20,
		"billing_risk_uncertain_cooldown_seconds": 420,
		"billing_risk_video_lease_ttl_seconds":    172800,
		"billing_risk_idle_balance_ttl_seconds":   180,
	}
	recorder := doUpdateSettings(t, h, payload, nil)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "true", repo.values[service.SettingKeyBillingRiskEnabled])
	require.Equal(t, "7.5", repo.values[service.SettingKeyBillingRiskLowBalanceThreshold])
	require.Equal(t, "172800", repo.values[service.SettingKeyBillingRiskVideoLeaseTTLSeconds])
	require.True(t, h.settingService.GetBillingRiskSettings().Enabled)
	require.Equal(t, 7.5, h.settingService.GetBillingRiskSettings().LowBalanceThreshold)

	disableRecorder := doUpdateSettings(t, h, map[string]any{"billing_risk_enabled": false}, nil)
	require.Equal(t, http.StatusOK, disableRecorder.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyBillingRiskEnabled])
	require.False(t, h.settingService.GetBillingRiskSettings().Enabled)

	omittedRecorder := doUpdateSettings(t, h, map[string]any{"promo_code_enabled": true}, nil)
	require.Equal(t, http.StatusOK, omittedRecorder.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyBillingRiskEnabled])
	require.Equal(t, "7.5", repo.values[service.SettingKeyBillingRiskLowBalanceThreshold])
	require.False(t, h.settingService.GetBillingRiskSettings().Enabled)
}

func TestSettingHandlerBillingRiskRejectsInvalidCombination(t *testing.T) {
	h, _ := newStepUpSwitchTestHandler(t, map[string]string{})

	recorder := doUpdateSettings(t, h, map[string]any{
		"billing_risk_enabled":                    true,
		"billing_risk_low_balance_threshold":      10,
		"billing_risk_safety_factor":              1.25,
		"billing_risk_minimum_request_risk":       0.001,
		"billing_risk_overdraft_allowance":        0.2,
		"billing_risk_high_cost_trigger":          1,
		"billing_risk_lease_ttl_seconds":          30,
		"billing_risk_refresh_interval_seconds":   15,
		"billing_risk_uncertain_cooldown_seconds": 300,
		"billing_risk_video_lease_ttl_seconds":    86400,
		"billing_risk_idle_balance_ttl_seconds":   120,
	}, nil)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "INVALID_BILLING_RISK_SETTINGS")
}
