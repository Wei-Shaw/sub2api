package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingsCodexTicketProxyWriteReadAndHotReload(t *testing.T) {
	key := service.SettingKeyOpenAICodexTicketHarvestProxyURL
	oldProxy := "http://user:old-secret@old.example.com:8080"
	newProxy := "socks5h://user:new-secret@new.example.com:1080"
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{key: oldProxy})
	require.Equal(t, oldProxy, h.settingService.GetOpenAICodexTicketHarvestProxyURL(context.Background()))
	rec := doUpdateSettings(t, h, map[string]any{key: newProxy}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, newProxy, repo.values[key])
	require.Equal(t, newProxy, h.settingService.GetOpenAICodexTicketHarvestProxyURL(context.Background()))
	require.NotContains(t, rec.Body.String(), "new-secret")
	require.Contains(t, rec.Body.String(), `"openai_codex_ticket_harvest_proxy_configured":true`)
	// Omission, empty input and the masked GET value all preserve the real secret.
	for _, body := range []map[string]any{{"site_name": "updated"}, {key: ""}, {key: service.MaskProxyURL(newProxy)}} {
		rec = doUpdateSettings(t, h, body, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, newProxy, repo.values[key])
	}
	get := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(get)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	h.GetSettings(c)
	require.Equal(t, http.StatusOK, get.Code)
	require.NotContains(t, get.Body.String(), "new-secret")
	require.Contains(t, get.Body.String(), "new.example.com")
}

func TestSettingsCodexTicketRejectInvalidProxyWithoutLeakingPassword(t *testing.T) {
	key := service.SettingKeyOpenAICodexTicketHarvestProxyURL
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{key: "http://previous.example.com:8080"})
	rec := doUpdateSettings(t, h, map[string]any{key: "ftp://user:invalid-secret@proxy.example.com:21"}, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.NotContains(t, rec.Body.String(), "invalid-secret")
	require.Equal(t, "http://previous.example.com:8080", repo.values[key])
}

func TestSettingsCodexTicketTargetLengthWriteReadAndHotReload(t *testing.T) {
	defaultKey := service.SettingKeyOpenAICodexTicketDefaultLength
	rulesKey := service.SettingKeyOpenAICodexTicketPlanLengths
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{defaultKey: "292", rulesKey: "[]"})

	rec := doUpdateSettings(t, h, map[string]any{
		defaultKey: 332,
		rulesKey:   []map[string]any{{"plan": " Team ", "length": 332}},
	}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "332", repo.values[defaultKey])
	require.JSONEq(t, `[{"plan":"team","length":332}]`, repo.values[rulesKey], "规则小写去空白后落库")
	require.Contains(t, rec.Body.String(), `"openai_codex_ticket_default_length":332`)
	require.Contains(t, rec.Body.String(), `"openai_codex_ticket_plan_lengths":[{"length":332,"plan":"team"}]`)

	// 热更新：runtime getter 立即拿到新值。
	tlc := h.settingService.GetOpenAICodexTicketTargetLengthConfig(context.Background(), 292)
	require.Equal(t, 332, tlc.DefaultLength)
	require.Equal(t, []service.OpenAICodexTicketPlanLengthRule{{Plan: "team", Length: 332}}, tlc.Rules)

	// 空数组 = 清空规则；非法长度/重复 plan 拒绝且不落库。
	rec = doUpdateSettings(t, h, map[string]any{rulesKey: []map[string]any{}}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "[]", repo.values[rulesKey])

	for _, body := range []map[string]any{
		{defaultKey: 99},
		{defaultKey: 513},
		{rulesKey: []map[string]any{{"plan": "team", "length": 99}}},
		{rulesKey: []map[string]any{{"plan": "team", "length": 332}, {"plan": "TEAM", "length": 316}}},
		{rulesKey: []map[string]any{{"plan": "  ", "length": 332}}},
	} {
		rec = doUpdateSettings(t, h, body, nil)
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	require.Equal(t, "[]", repo.values[rulesKey], "非法请求不落库")
	require.Equal(t, "332", repo.values[defaultKey])
}
