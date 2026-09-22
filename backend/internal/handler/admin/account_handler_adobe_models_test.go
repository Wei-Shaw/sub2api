//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func adobeAvailableModelsIDs(t *testing.T, account service.Account) []string {
	t.Helper()
	ids := make([]string, 0)
	for _, m := range adobeAvailableModels(t, account) {
		ids = append(ids, m.ID)
	}
	return ids
}

type adobeTestModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

func adobeAvailableModels(t *testing.T, account service.Account) []adobeTestModel {
	t.Helper()
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account:          account,
	}
	rec := httptest.NewRecorder()
	setupAvailableModelsRouter(svc).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/91/models", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data []adobeTestModel `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Data
}

// Step 11：对外清单里的模型给人类可读展示名，别名/历史名保留裸 id。
//
// 之前一律用 id 当 display_name，管理端的 Adobe 下拉显示 "gpt-image-2"，
// 而隔壁 OpenAI 显示 "GPT Image 2"，观感割裂。
func TestAccountHandlerGetAvailableModels_AdobeDisplayNames(t *testing.T) {
	models := adobeAvailableModels(t, service.Account{
		ID:       91,
		Name:     "adobe-oauth",
		Platform: domain.PlatformAdobe,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"cookie": "aux_sid=abc",
		},
	})

	byID := make(map[string]string, len(models))
	for _, m := range models {
		byID[m.ID] = m.DisplayName
	}

	// 对外清单：可读展示名，且不泄露内部族前缀。
	require.Equal(t, "GPT Image 2", byID["gpt-image-2"])
	require.Equal(t, "Imagen 4", byID["imagen-4"])
	require.Equal(t, "FLUX Pro", byID["flux-pro"])
	require.Equal(t, "GPT Image 2.5 Sunburst", byID["gpt-image-2.5-sunburst"])
	require.Equal(t, "Gemini 3 Pro Image", byID["gemini-3-pro-image"])
	require.Equal(t, "Gemini 2.5 Flash Image", byID["gemini-2.5-flash-image"])
	require.Equal(t, "Gemini 3.1 Flash Image", byID["gemini-3.1-flash-image"])
	for _, id := range adobe.ImageModelIDs() {
		require.NotContains(t, byID[id], "firefly-", "展示名不应泄露内部族 id：%s", id)
	}

	// 历史别名保留裸 id：三个都落 firefly-gpt-image-2，套 label 会出现三行「GPT Image 2」。
	for _, legacy := range []string{"gpt-image", "gpt-image-1", "gpt-image-1-mini"} {
		require.Equal(t, legacy, byID[legacy], "历史别名 %s 应显示自身", legacy)
	}
	for _, legacy := range []string{"nano-banana", "nano-banana-pro", "nano-banana2"} {
		require.Equal(t, legacy, byID[legacy], "历史别名 %s 应显示自身", legacy)
	}
}

// 无显式 mapping 的 Adobe 账号（GetModelMapping 回落到 DefaultAdobeModelMapping）
// 应返回全部对外模型名，且**首项是 ImageModelIDs 的首项**——前端默认选中依赖这个顺序。
func TestAccountHandlerGetAvailableModels_AdobeUsesDefaultMapping(t *testing.T) {
	ids := adobeAvailableModelsIDs(t, service.Account{
		ID:       91,
		Name:     "adobe-oauth",
		Platform: domain.PlatformAdobe,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"cookie":       "aux_sid=abc",
			"access_token": "adobe-token",
		},
	})

	require.Len(t, ids, len(domain.DefaultAdobeModelMapping))
	require.Equal(t, adobe.ImageModelIDs()[0], ids[0],
		"首项必须稳定——前端默认选中它")

	// 改前 Adobe 会落到本函数结尾的 Claude 兜底返回 claude.DefaultModels。
	for _, id := range ids {
		require.False(t, strings.HasPrefix(id, "claude-"),
			"Adobe 测试弹窗不该出现 Claude 模型：%s", id)
	}
	// Step 8 之后用户面只有干净外部名，内部族 id 不该外露。
	for _, id := range ids {
		require.False(t, strings.HasPrefix(id, adobe.ImageModelIDPrefix),
			"不该出现内部族 id：%s", id)
	}
}

// 运维自定义 mapping 时只返回它配的那些——测试弹窗要反映账号真实会服务的模型集合。
func TestAccountHandlerGetAvailableModels_AdobeRespectsExplicitMapping(t *testing.T) {
	ids := adobeAvailableModelsIDs(t, service.Account{
		ID:       91,
		Name:     "adobe-custom",
		Platform: domain.PlatformAdobe,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"cookie":       "aux_sid=abc",
			"access_token": "adobe-token",
			"model_mapping": map[string]any{
				"imagen-4":     "firefly-imagen-4",
				"my-own-alias": "firefly-flux-pro",
			},
		},
	})

	// imagen-4 在 ImageModelIDs 里 → 排前面；自定义别名不在 → 字典序追加在后。
	require.Equal(t, []string{"imagen-4", "my-own-alias"}, ids)
}

// 顺序必须确定：map 迭代顺序随机，若直接遍历 mapping 会让默认选中每次刷新都变。
func TestAccountHandlerGetAvailableModels_AdobeOrderIsStable(t *testing.T) {
	account := service.Account{
		ID:       91,
		Platform: domain.PlatformAdobe,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"cookie":       "aux_sid=abc",
			"access_token": "adobe-token",
		},
	}
	want := adobeAvailableModelsIDs(t, account)
	for i := 0; i < 20; i++ {
		require.Equal(t, want, adobeAvailableModelsIDs(t, account))
	}
}
