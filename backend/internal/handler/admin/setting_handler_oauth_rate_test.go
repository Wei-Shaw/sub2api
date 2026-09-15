package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingHandler_OAuthSchedulingRateRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name    string
		initial map[string]string
		body    string
		stored  string
		value   any
		status  int
	}{
		{"absent keeps default", nil, `{}`, "1", float64(1), http.StatusOK},
		{"omitted preserves override", map[string]string{service.SettingKeyOpenAIOAuthSchedulingRateMultiplier: "0.7"}, `{}`, "0.7", 0.7, http.StatusOK},
		{"clear override", map[string]string{service.SettingKeyOpenAIOAuthSchedulingRateMultiplier: "0.7"}, `{"openai_oauth_scheduling_rate_multiplier":null}`, "", nil, http.StatusOK},
		{"omitted preserves cleared", map[string]string{service.SettingKeyOpenAIOAuthSchedulingRateMultiplier: ""}, `{}`, "", nil, http.StatusOK},
		{"set override", nil, `{"openai_oauth_scheduling_rate_multiplier":0.7}`, "0.7", 0.7, http.StatusOK},
		{"zero is explicit", nil, `{"openai_oauth_scheduling_rate_multiplier":0}`, "0", float64(0), http.StatusOK},
		{"negative is rejected", nil, `{"openai_oauth_scheduling_rate_multiplier":-1}`, "", nil, http.StatusBadRequest},
		{"string is rejected", nil, `{"openai_oauth_scheduling_rate_multiplier":"invalid"}`, "", nil, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &settingHandlerRepoStub{values: tt.initial}
			svc := service.NewSettingService(repo, &config.Config{})
			handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewBufferString(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			handler.UpdateSettings(c)
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			if tt.status != http.StatusOK {
				require.Empty(t, repo.lastUpdates)
				return
			}
			require.Equal(t, tt.stored, repo.values[service.SettingKeyOpenAIOAuthSchedulingRateMultiplier])
			var body struct {
				Data map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Contains(t, body.Data, service.SettingKeyOpenAIOAuthSchedulingRateMultiplier)
			require.Equal(t, tt.value, body.Data[service.SettingKeyOpenAIOAuthSchedulingRateMultiplier])

			getRec := httptest.NewRecorder()
			getContext, _ := gin.CreateTestContext(getRec)
			getContext.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
			handler.GetSettings(getContext)
			require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())
			require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &body))
			require.Contains(t, body.Data, service.SettingKeyOpenAIOAuthSchedulingRateMultiplier)
			require.Equal(t, tt.value, body.Data[service.SettingKeyOpenAIOAuthSchedulingRateMultiplier])
		})
	}
}

func TestSettingsAuditTracksOAuthRateValueNotPointer(t *testing.T) {
	beforeRate, afterRate := 0.7, 0.7
	before := &service.SystemSettings{OpenAIOAuthSchedulingRateMultiplier: &beforeRate}
	after := &service.SystemSettings{OpenAIOAuthSchedulingRateMultiplier: &afterRate}
	require.NotContains(t, diffSettings(before, after, nil, nil, UpdateSettingsRequest{}), service.SettingKeyOpenAIOAuthSchedulingRateMultiplier)
	after.OpenAIOAuthSchedulingRateMultiplier = nil
	require.Contains(t, diffSettings(before, after, nil, nil, UpdateSettingsRequest{}), service.SettingKeyOpenAIOAuthSchedulingRateMultiplier)
}
