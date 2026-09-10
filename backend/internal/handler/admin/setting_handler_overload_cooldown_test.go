package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type overloadCooldownSettingRepo struct {
	settingHandlerRepoStub
}

func (r *overloadCooldownSettingRepo) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}
func TestUpdateOverloadCooldownSettingsPreservesOAuthFieldsForOlderClients(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := service.OverloadCooldownSettings{
		Enabled:                             true,
		CooldownMinutes:                     10,
		OpenAIOAuthCapacityEnabled:          true,
		OpenAIOAuthCapacityWindowMinutes:    7,
		OpenAIOAuthCapacityFailureThreshold: 4,
		OpenAIOAuthCapacityCooldownMinutes:  13,
	}
	encoded, err := json.Marshal(original)
	require.NoError(t, err)

	repo := &overloadCooldownSettingRepo{settingHandlerRepoStub: settingHandlerRepoStub{values: map[string]string{
		service.SettingKeyOverloadCooldownSettings: string(encoded),
	}}}
	svc := service.NewSettingService(repo, &config.Config{})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/overload-cooldown", bytes.NewBufferString(`{"enabled":false,"cooldown_minutes":12}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateOverloadCooldownSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var stored service.OverloadCooldownSettings
	require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyOverloadCooldownSettings]), &stored))
	require.False(t, stored.Enabled)
	require.Equal(t, 12, stored.CooldownMinutes)
	require.True(t, stored.OpenAIOAuthCapacityEnabled)
	require.Equal(t, 7, stored.OpenAIOAuthCapacityWindowMinutes)
	require.Equal(t, 4, stored.OpenAIOAuthCapacityFailureThreshold)
	require.Equal(t, 13, stored.OpenAIOAuthCapacityCooldownMinutes)
}
