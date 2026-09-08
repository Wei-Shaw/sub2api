//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

// PUT /settings 的响应只回传本次发送的字段及其只读伴生键（整份文档由 GET 负责），
// 避免每次保存都在链路上搬运 ~800KB。以下测试钉住这个契约。

func decodeUpdateSettingsData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok, "response data must be an object")
	return data
}

func TestUpdateSettingsSlimResponseIncludesSentAndCompanionKeys(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeySiteName:                      "Example Gateway",
		service.SettingKeySMTPPassword:                  "stored-smtp-secret",
		service.SettingKeyAuthSourceDefaultEmailBalance: "9.5",
	})

	rec := doUpdateSettings(t, h, map[string]any{
		"registration_enabled": true,
		"smtp_password":        "new-smtp-secret",
	}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "new-smtp-secret", repo.values[service.SettingKeySMTPPassword])

	data := decodeUpdateSettingsData(t, rec)

	// 发送的字段：回传终值。
	require.Equal(t, true, data["registration_enabled"])
	// 密钥的只读伴生键：必须一并回传，且不回传明文。
	require.Equal(t, true, data["smtp_password_configured"])
	require.NotContains(t, data, "smtp_password")
	// always 键：未发送也回传（保存路径 fail-closed 重推导）。
	require.Contains(t, data, "api_key_acl_trust_forwarded_ip")
	require.Contains(t, data, "forwarded_client_ip_headers")
}

func TestUpdateSettingsSlimResponseOmitsUnsentKeys(t *testing.T) {
	h, _ := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeySiteName:     "Example Gateway",
		service.SettingKeySMTPPassword: "stored-smtp-secret",
	})

	rec := doUpdateSettings(t, h, map[string]any{"registration_enabled": true}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	data := decodeUpdateSettingsData(t, rec)

	// 未发送的键不回传——这是瘦身的核心契约，客户端按「键存在才写」合并。
	require.NotContains(t, data, "site_name")
	require.NotContains(t, data, "smtp_password_configured")
	require.NotContains(t, data, "auth_source_default_email_balance")
}

func TestUpdateSettingsSlimResponseIncludesSchedulerEffectiveCompanions(t *testing.T) {
	h, _ := newStepUpSwitchTestHandler(t, map[string]string{})

	rec := doUpdateSettings(t, h, map[string]any{
		"openai_advanced_scheduler_enabled": true,
	}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	data := decodeUpdateSettingsData(t, rec)
	require.Equal(t, true, data["openai_advanced_scheduler_enabled"])
	for _, key := range openAIAdvancedSchedulerEffectiveKeys {
		require.Contains(t, data, key,
			"scheduler effective keys must accompany any scheduler request field")
	}
}

func TestFilterSystemSettingsResponseForPutUnknownSentKeyIsIgnored(t *testing.T) {
	filtered := filterSystemSettingsResponseForPut(
		map[string]any{"site_name": "Example Gateway"},
		map[string]json.RawMessage{"not_a_real_setting": json.RawMessage(`"x"`)},
	)
	require.Empty(t, filtered, "sent keys absent from the response data are dropped, not invented")
}
