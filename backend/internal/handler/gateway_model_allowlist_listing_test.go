package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 通配条目在 /v1/models 中展开为候选来源中所有匹配项，保持来源顺序。
func TestGatewayModels_ModelAllowlistWildcardExpandsAgainstSource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(31)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformOpenAI,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gpt-5.4":       "gpt-5.4",
								"gpt-5.5-codex": "gpt-5.5-codex",
								"gpt-5.5-mini":  "gpt-5.5-mini",
								"other-foo":     "other-foo",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
			ModelAllowlist: service.GroupModelAllowlist{
				Enabled: true,
				Models:  []string{"gpt-5.5-*", "gpt-5.4"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gpt-5.5-codex", "gpt-5.5-mini", "gpt-5.4"}, modelIDsForTest(got.Data))
}

// geminiAllowlistAccountRepoStub 在 gatewayModelsAccountRepoStub 之上补充
// Gemini 兼容层用到的按平台过滤查询（分组内无任何账号，触发 fallback 列表）。
type geminiAllowlistAccountRepoStub struct {
	gatewayModelsAccountRepoStub
}

func (s *geminiAllowlistAccountRepoStub) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]service.Account, error) {
	allowed := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		allowed[platform] = struct{}{}
	}
	accounts := s.byGroup[groupID]
	filtered := make([]service.Account, 0, len(accounts))
	for _, account := range accounts {
		if _, ok := allowed[account.Platform]; ok {
			filtered = append(filtered, account)
		}
	}
	return filtered, nil
}

// Gemini 原生 /v1beta/models：白名单开启时过滤 fallback 列表的 models[].name。
func TestGeminiV1BetaListModels_FiltersFallbackByAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &geminiAllowlistAccountRepoStub{gatewayModelsAccountRepoStub: gatewayModelsAccountRepoStub{
		byGroup: map[int64][]service.Account{
			// 只有 antigravity 账号：Gemini 选号失败后回落静态模型列表。
			41: {{ID: 2, Platform: service.PlatformAntigravity, Status: service.StatusActive, Schedulable: true}},
		},
	}}
	h := &GatewayHandler{
		geminiCompatService: service.NewGeminiMessagesCompatService(repo, nil, nil, nil, nil, nil, nil, nil, nil),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	geminiGroupID := int64(41)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		GroupID: &geminiGroupID,
		Group: &service.Group{
			ID:       geminiGroupID,
			Platform: service.PlatformGemini,
			ModelAllowlist: service.GroupModelAllowlist{
				Enabled: true,
				Models:  []string{"gemini-2.5-pro", "gemini-3-*"},
			},
		},
	})

	h.GeminiV1BetaListModels(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	names := make([]string, 0, len(got.Models))
	for _, model := range got.Models {
		names = append(names, model.Name)
	}
	// models/ 前缀的候选形式也应命中条目。
	require.Contains(t, names, "models/gemini-2.5-pro")
	require.Contains(t, names, "models/gemini-3-pro-preview")
	require.NotContains(t, names, "models/gemini-2.5-flash")
	require.NotContains(t, names, "models/gemini-2.0-flash")
}

// Antigravity /antigravity/models：白名单开启时按条目过滤静态列表。
func TestAntigravityModels_FiltersByAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &GatewayHandler{}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/antigravity/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       51,
			Platform: service.PlatformAntigravity,
			ModelAllowlist: service.GroupModelAllowlist{
				Enabled: true,
				Models:  []string{"gemini-2.5-flash"},
			},
		},
	})

	h.AntigravityModels(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got struct {
		Object string `json:"object"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "list", got.Object)
	// -thinking 后缀的宽容规则：gemini-2.5-flash-thinking 也命中 gemini-2.5-flash 条目。
	require.Len(t, got.Data, 2)
	require.Equal(t, "gemini-2.5-flash", got.Data[0].ID)
	require.Equal(t, "gemini-2.5-flash-thinking", got.Data[1].ID)
}

func TestFilterUpstreamGeminiModelsBody(t *testing.T) {
	allowlist := service.GroupModelAllowlist{Enabled: true, Models: []string{"gemini-2.5-pro"}}

	t.Run("full hit returns original body with dropped=false", func(t *testing.T) {
		body := []byte(`{"models":[{"name":"models/gemini-2.5-pro"},{"name":"models/gemini-2.5-pro"}]}`)
		filtered, dropped, ok := filterUpstreamGeminiModelsBody(body, allowlist)
		require.True(t, ok)
		require.False(t, dropped, "full hit must keep the original response so headers pass through")
		require.Equal(t, string(body), string(filtered))
	})

	t.Run("partial hit filters models and preserves envelope fields", func(t *testing.T) {
		body := []byte(`{"nextPageToken":"tok","models":[{"name":"models/gemini-2.5-pro"},{"name":"models/gemini-2.5-flash"}],"other":"kept"}`)
		filtered, dropped, ok := filterUpstreamGeminiModelsBody(body, allowlist)
		require.True(t, ok)
		require.True(t, dropped)
		require.JSONEq(t, `{"nextPageToken":"tok","other":"kept","models":[{"name":"models/gemini-2.5-pro"}]}`, string(filtered))
	})

	t.Run("models prefix stripped candidate form matches entry", func(t *testing.T) {
		body := []byte(`{"models":[{"name":"gemini-2.5-pro"}]}`)
		filtered, dropped, ok := filterUpstreamGeminiModelsBody(body, allowlist)
		require.True(t, ok)
		require.False(t, dropped)
		require.Equal(t, string(body), string(filtered))
	})

	t.Run("missing models field passes through", func(t *testing.T) {
		body := []byte(`{"error":"odd upstream"}`)
		_, dropped, ok := filterUpstreamGeminiModelsBody(body, allowlist)
		require.True(t, ok)
		require.False(t, dropped)
	})

	t.Run("invalid JSON signals passthrough", func(t *testing.T) {
		_, dropped, ok := filterUpstreamGeminiModelsBody([]byte(`not-json`), allowlist)
		require.False(t, ok)
		require.False(t, dropped)
	})
}

func TestFilterBatchImageModelsByAllowlist(t *testing.T) {
	models := []service.BatchImagePublicModel{
		{ID: "gemini-2.5-flash-image", Object: "image.batch.model", Provider: "gemini_api"},
		{ID: "gemini-3-pro-image", Object: "image.batch.model", Provider: "vertex"},
		{ID: "gpt-image-1", Object: "image.batch.model", Provider: "openai"},
	}

	t.Run("disabled allowlist keeps everything", func(t *testing.T) {
		got := filterBatchImageModelsByAllowlist(models, service.GroupModelAllowlist{Enabled: false})
		require.Equal(t, models, got)
	})

	t.Run("exact and wildcard entries filter by model id", func(t *testing.T) {
		got := filterBatchImageModelsByAllowlist(models, service.GroupModelAllowlist{
			Enabled: true,
			Models:  []string{"gemini-3-*", "gpt-image-1"},
		})
		require.Equal(t, []string{"gemini-3-pro-image", "gpt-image-1"}, []string{got[0].ID, got[1].ID})
		require.Len(t, got, 2)
	})

	t.Run("no match yields empty list", func(t *testing.T) {
		got := filterBatchImageModelsByAllowlist(models, service.GroupModelAllowlist{
			Enabled: true,
			Models:  []string{"claude-*"},
		})
		require.Empty(t, got)
	})
}
