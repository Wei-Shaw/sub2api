package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type geminiVertexModelsTokenCache struct{}

func (geminiVertexModelsTokenCache) GetAccessToken(context.Context, string) (string, error) {
	return "ya29.vertex-token", nil
}

func (geminiVertexModelsTokenCache) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
}

func (geminiVertexModelsTokenCache) DeleteAccessToken(context.Context, string) error {
	return nil
}

func (geminiVertexModelsTokenCache) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return false, nil
}

func (geminiVertexModelsTokenCache) ReleaseRefreshLock(context.Context, string) error {
	return nil
}

// 分组里只有 Agent Platform (Vertex) 服务账号时，模型列表与模型详情接口返回 Vertex 上的 Gemini 模型。
func TestGeminiV1BetaModelsEndpoints_VertexServiceAccountOnlyGroup(t *testing.T) {
	groupID := int64(8)
	repo := &geminiAllowlistAccountRepoStub{gatewayModelsAccountRepoStub: gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{groupID: {{
		ID:       18,
		Platform: service.PlatformGemini,
		Type:     service.AccountTypeServiceAccount,
		Credentials: map[string]any{
			"location": "global",
			"service_account_json": map[string]any{
				"type":         "service_account",
				"project_id":   "proj",
				"private_key":  "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n",
				"client_email": "svc@proj.iam.gserviceaccount.com",
			},
		},
	}}}}}
	upstream := &geminiMixedModelsUpstream{status: http.StatusOK, body: `{"publisherModels":[{"name":"publishers/google/models/gemini-3.8-flash"}]}`}
	tokenProvider := service.NewGeminiTokenProvider(nil, geminiVertexModelsTokenCache{}, nil)
	h := &GatewayHandler{geminiCompatService: service.NewGeminiMessagesCompatService(repo, nil, nil, nil, tokenProvider, nil, upstream, nil, &config.Config{})}
	apiKey := &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformGemini}}

	t.Run("list", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)

		h.GeminiV1BetaListModels(c)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.JSONEq(t, `{"models":[{"name":"models/gemini-3.8-flash","supportedGenerationMethods":["generateContent","streamGenerateContent"]}]}`, rec.Body.String())
	})

	t.Run("get", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-3.8-flash", nil)
		c.Params = gin.Params{{Key: "model", Value: "gemini-3.8-flash"}}
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)

		h.GeminiV1BetaGetModel(c)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.JSONEq(t, `{"name":"models/gemini-3.8-flash","supportedGenerationMethods":["generateContent","streamGenerateContent"]}`, rec.Body.String())
	})
}
