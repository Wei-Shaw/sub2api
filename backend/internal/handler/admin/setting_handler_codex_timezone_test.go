//go:build unit

package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestUpdateSettings_CodexTimezone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		body   string
		status int
		want   string
	}{
		{"save and trim", `{"openai_codex_timezone":" Asia/Taipei "}`, http.StatusOK, "Asia/Taipei"},
		{"change", `{"openai_codex_timezone":"UTC"}`, http.StatusOK, "UTC"},
		{"clear", `{"openai_codex_timezone":""}`, http.StatusOK, ""},
		{"omit preserves value", `{"promo_code_enabled":true}`, http.StatusOK, "Asia/Taipei"},
		{"invalid timezone", `{"openai_codex_timezone":"invalid-zone"}`, http.StatusBadRequest, "Asia/Taipei"},
		{"reject server-dependent Local", `{"openai_codex_timezone":"Local"}`, http.StatusBadRequest, "Asia/Taipei"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &settingHandlerRepoStub{values: map[string]string{
				service.SettingKeyPromoCodeEnabled:    "true",
				service.SettingKeyOpenAICodexTimezone: "Asia/Taipei",
			}}
			svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
			handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewBufferString(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			handler.UpdateSettings(c)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			require.Equal(t, tc.want, repo.values[service.SettingKeyOpenAICodexTimezone])
			if tc.status == http.StatusOK {
				require.Equal(t, tc.want, gjson.GetBytes(rec.Body.Bytes(), "data.openai_codex_timezone").String())
			}
		})
	}
}
