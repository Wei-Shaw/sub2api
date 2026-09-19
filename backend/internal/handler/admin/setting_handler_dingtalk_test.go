package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func doDingTalkTestRequest(t *testing.T, handler *SettingHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/test-channel-monitor-dingtalk", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	handler.TestChannelMonitorDingTalk(c)
	return rec
}

func TestTestChannelMonitorDingTalkUsesStoredCredentialsWithoutPersisting(t *testing.T) {
	handler, repo := newDingTalkSettingsHandler()
	storedWebhook := "https://oapi.dingtalk.com/robot/send?access_token=stored-token"
	storedSecret := "SEC-stored-secret"
	repo.values[service.SettingKeyChannelMonitorDingTalkWebhook] = storedWebhook
	repo.values[service.SettingKeyChannelMonitorDingTalkSecret] = storedSecret
	handler.channelMonitorDingTalkTest = func(_ context.Context, webhook, secret string) error {
		require.Equal(t, storedWebhook, webhook)
		require.Equal(t, storedSecret, secret)
		return nil
	}

	rec := doDingTalkTestRequest(t, handler, `{}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), storedWebhook)
	require.NotContains(t, rec.Body.String(), storedSecret)
	require.Equal(t, storedWebhook, repo.values[service.SettingKeyChannelMonitorDingTalkWebhook])
	require.Equal(t, storedSecret, repo.values[service.SettingKeyChannelMonitorDingTalkSecret])
}

func TestTestChannelMonitorDingTalkUsesRequestCredentialsAndPropagatesFailure(t *testing.T) {
	handler, repo := newDingTalkSettingsHandler()
	storedWebhook := "https://oapi.dingtalk.com/robot/send?access_token=stored-token"
	repo.values[service.SettingKeyChannelMonitorDingTalkWebhook] = storedWebhook
	called := false
	handler.channelMonitorDingTalkTest = func(_ context.Context, webhook, secret string) error {
		called = true
		require.Equal(t, "https://oapi.dingtalk.com/robot/send?access_token=request-token", webhook)
		require.Equal(t, "SEC-request-secret", secret)
		return errors.New("delivery failed")
	}

	rec := doDingTalkTestRequest(t, handler, `{"channel_monitor_dingtalk_webhook":"https://oapi.dingtalk.com/robot/send?access_token=request-token","channel_monitor_dingtalk_secret":"SEC-request-secret"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.True(t, called)
	require.NotContains(t, rec.Body.String(), "request-token")
	require.NotContains(t, rec.Body.String(), "SEC-request-secret")
	require.Equal(t, storedWebhook, repo.values[service.SettingKeyChannelMonitorDingTalkWebhook])
}

func TestTestChannelMonitorDingTalkRejectsClearedWebhook(t *testing.T) {
	handler, _ := newDingTalkSettingsHandler()
	called := false
	handler.channelMonitorDingTalkTest = func(context.Context, string, string) error {
		called = true
		return nil
	}

	rec := doDingTalkTestRequest(t, handler, `{"channel_monitor_dingtalk_webhook_clear":true}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, called)
	require.Contains(t, rec.Body.String(), "webhook is required")
}

// dingtalkSettingsRepoStub 复用 settingHandlerRepoStub（已在 setting_handler_auth_source_defaults_test.go 定义）

func newDingTalkSettingsHandler() (*SettingHandler, *settingHandlerRepoStub) {
	repo := &settingHandlerRepoStub{values: map[string]string{}}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
	return handler, repo
}

// baseValidDingTalkBody 返回一个可以通过所有校验的最小合法 body。
func baseValidDingTalkBody() map[string]any {
	return map[string]any{
		"dingtalk_connect_enabled":                 true,
		"dingtalk_connect_client_id":               "test-client-id",
		"dingtalk_connect_client_secret":           "test-client-secret",
		"dingtalk_connect_redirect_url":            "https://example.com/auth/dingtalk/callback",
		"dingtalk_connect_corp_restriction_policy": "none",
	}
}

// TestSettingsPUT_DingTalk_V3_InternalOnlyAllowsEmptyCorpID 验证方案 A：
// internal_only + internal_corp_id="" 应通过校验（→ 200），不再是 400。
func TestSettingsPUT_DingTalk_V3_InternalOnlyAllowsEmptyCorpID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newDingTalkSettingsHandler()

	body := baseValidDingTalkBody()
	body["dingtalk_connect_corp_restriction_policy"] = "internal_only"
	body["dingtalk_connect_internal_corp_id"] = "" // 空值现在合法

	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
}

// TestSettingsPUT_DingTalk_HappyPath_None 验证 none policy → 200
func TestSettingsPUT_DingTalk_HappyPath_None(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newDingTalkSettingsHandler()

	body := baseValidDingTalkBody()
	body["dingtalk_connect_corp_restriction_policy"] = "none"

	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, data["dingtalk_connect_enabled"])
}

// TestSettingsPUT_DingTalk_HappyPath_InternalOnly_WithCorpID 验证 internal_only + corp_id → 200
func TestSettingsPUT_DingTalk_HappyPath_InternalOnly_WithCorpID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newDingTalkSettingsHandler()

	body := baseValidDingTalkBody()
	body["dingtalk_connect_corp_restriction_policy"] = "internal_only"
	body["dingtalk_connect_internal_corp_id"] = "ding-corp-123"

	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
}

// TestSettingsPUT_DingTalk_BypassRegistration_RoundTrip 验证 bypass_registration 字段 save+load。
// 必须用 policy=internal_only：bypass 仅在该 policy 下生效，其它 policy 写入层会 coerce 为 false。
func TestSettingsPUT_DingTalk_BypassRegistration_RoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newDingTalkSettingsHandler()

	body := baseValidDingTalkBody()
	body["dingtalk_connect_corp_restriction_policy"] = "internal_only"
	body["dingtalk_connect_bypass_registration"] = true

	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, data["dingtalk_connect_bypass_registration"])
}

// TestSettingsPUT_DingTalk_Disabled_SkipsValidation 验证 disabled 时跳过 corp 校验 → 200。
// 用 enabled=true 时必然触发"Client ID is required when enabled"的空 client_id 作为
// 哨兵——只要 enabled=false 仍能 200 就证明跳过了。
func TestSettingsPUT_DingTalk_Disabled_SkipsValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newDingTalkSettingsHandler()

	body := map[string]any{
		"dingtalk_connect_enabled":                 false,
		"dingtalk_connect_client_id":               "", // 这种空值在 enabled=true 时会被 400 拒绝
		"dingtalk_connect_corp_restriction_policy": "internal_only",
	}

	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
}

// TestSettingsPUT_DingTalk_SyncFlags_InternalOnly_RoundTrip 验证三个 sync 开关在 internal_only 下可正常 save+load。
func TestSettingsPUT_DingTalk_SyncFlags_InternalOnly_RoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newDingTalkSettingsHandler()

	body := baseValidDingTalkBody()
	body["dingtalk_connect_corp_restriction_policy"] = "internal_only"
	body["dingtalk_connect_sync_corp_email"] = true
	body["dingtalk_connect_sync_display_name"] = true
	body["dingtalk_connect_sync_dept"] = true

	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, data["dingtalk_connect_sync_corp_email"], "sync_corp_email should be true for internal_only")
	require.Equal(t, true, data["dingtalk_connect_sync_display_name"], "sync_display_name should be true for internal_only")
	require.Equal(t, true, data["dingtalk_connect_sync_dept"], "sync_dept should be true for internal_only")
}

// TestSettingsPUT_DingTalk_SyncFlags_PolicyNone_CoercedToFalse 验证 policy=none 时三个 sync 开关被 coerce 为 false。
func TestSettingsPUT_DingTalk_SyncFlags_PolicyNone_CoercedToFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newDingTalkSettingsHandler()

	body := baseValidDingTalkBody()
	body["dingtalk_connect_corp_restriction_policy"] = "none"
	body["dingtalk_connect_sync_corp_email"] = true
	body["dingtalk_connect_sync_display_name"] = true
	body["dingtalk_connect_sync_dept"] = true

	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, data["dingtalk_connect_sync_corp_email"], "sync_corp_email must be coerced to false when policy=none")
	require.Equal(t, false, data["dingtalk_connect_sync_display_name"], "sync_display_name must be coerced to false when policy=none")
	require.Equal(t, false, data["dingtalk_connect_sync_dept"], "sync_dept must be coerced to false when policy=none")
}

// TestSettingsPUT_DingTalk_StaleWhitelist_CoercedToNone 验证升级兼容：
// admin 直接把 corp_restriction_policy=whitelist 提交（前端 UI 已无此选项，但 API 仍可命中）
// 不应导致 400 失败，应该被静默 coerce 为 none 后通过校验。
func TestSettingsPUT_DingTalk_StaleWhitelist_CoercedToNone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, repo := newDingTalkSettingsHandler()

	body := baseValidDingTalkBody()
	body["dingtalk_connect_corp_restriction_policy"] = "whitelist"

	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "none", repo.values[service.SettingKeyDingTalkConnectCorpRestrictionPolicy],
		"stale whitelist 应在写入路径被 coerce 为 none")
}

// TestSettingsPUT_DingTalk_SyncAttrKey_RoundTrip 验证 3 个 attr key 字段 save+load + 空值 fallback 到默认值。
func TestSettingsPUT_DingTalk_SyncAttrKey_RoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("custom_attr_keys_saved", func(t *testing.T) {
		handler, repo := newDingTalkSettingsHandler()

		body := baseValidDingTalkBody()
		body["dingtalk_connect_corp_restriction_policy"] = "internal_only"
		body["dingtalk_connect_sync_corp_email"] = true
		body["dingtalk_connect_sync_display_name"] = true
		body["dingtalk_connect_sync_dept"] = true
		body["dingtalk_connect_sync_corp_email_attr_key"] = "my_email_attr"
		body["dingtalk_connect_sync_display_name_attr_key"] = "my_name_attr"
		body["dingtalk_connect_sync_dept_attr_key"] = "my_dept_attr"

		rawBody, err := json.Marshal(body)
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
		c.Request.Header.Set("Content-Type", "application/json")

		handler.UpdateSettings(c)

		require.Equal(t, http.StatusOK, rec.Code)

		// 验证写入 DB 的 key
		require.Equal(t, "my_email_attr", repo.values[service.SettingKeyDingTalkConnectSyncCorpEmailAttrKey])
		require.Equal(t, "my_name_attr", repo.values[service.SettingKeyDingTalkConnectSyncDisplayNameAttrKey])
		require.Equal(t, "my_dept_attr", repo.values[service.SettingKeyDingTalkConnectSyncDeptAttrKey])

		// 验证响应中的 attr key
		var resp response.Response
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "my_email_attr", data["dingtalk_connect_sync_corp_email_attr_key"])
		require.Equal(t, "my_name_attr", data["dingtalk_connect_sync_display_name_attr_key"])
		require.Equal(t, "my_dept_attr", data["dingtalk_connect_sync_dept_attr_key"])
	})

	t.Run("empty_attr_keys_fallback_to_defaults", func(t *testing.T) {
		handler, repo := newDingTalkSettingsHandler()

		body := baseValidDingTalkBody()
		body["dingtalk_connect_corp_restriction_policy"] = "internal_only"
		// 不传 attr key → 写入层 fallback 到默认值
		body["dingtalk_connect_sync_corp_email_attr_key"] = ""
		body["dingtalk_connect_sync_display_name_attr_key"] = ""
		body["dingtalk_connect_sync_dept_attr_key"] = ""

		rawBody, err := json.Marshal(body)
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
		c.Request.Header.Set("Content-Type", "application/json")

		handler.UpdateSettings(c)

		require.Equal(t, http.StatusOK, rec.Code)

		// 空值应 fallback 到默认值并持久化
		require.Equal(t, "dingtalk_email", repo.values[service.SettingKeyDingTalkConnectSyncCorpEmailAttrKey])
		require.Equal(t, "dingtalk_name", repo.values[service.SettingKeyDingTalkConnectSyncDisplayNameAttrKey])
		require.Equal(t, "dingtalk_department", repo.values[service.SettingKeyDingTalkConnectSyncDeptAttrKey])
	})
}
