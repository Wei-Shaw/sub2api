package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// adobeRoutesRouter 搭一个最小路由，只为验证 images 端点按分组平台的分派。
func adobeRoutesRouter(t *testing.T, group *service.Group) *gin.Engine {
	return adobeRoutesRouterWithResolver(t, group, nil)
}

func adobeRoutesRouterWithResolver(t *testing.T, group *service.Group, resolver *service.CompositeRouteResolver) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	// 不设上限的话 RequestBodyLimit 会按 0 字节拒收，测试请求进不到 handler。
	cfg.Gateway.MaxBodySize = 1 << 20
	openAI := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := &handler.Handlers{
		Gateway: handler.NewGatewayHandler(nil, openAI, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, service.NewAdobeImageService(nil), cfg, nil),
		OpenAIGateway: handler.NewOpenAIGatewayHandler(openAI, nil, nil, nil, nil, nil, nil, nil, nil, cfg),
		AsyncImage:    handler.NewAsyncImageHandler(nil, nil),
	}
	router := gin.New()
	RegisterGatewayRoutes(router, h, servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{ID: 11, GroupID: &group.ID, Group: group})
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 22, Concurrency: 1})
		c.Next()
	}), nil, nil, nil, nil, resolver, cfg)
	return router
}

func postAdobeImages(t *testing.T, router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// Adobe 分组的 images 请求必须落到 AdobeImages，而不是被 default 分支当成
// 「本平台不支持 Images API」直接 404。
//
// 这里用「分组关掉出图权限」当探针：只有真正进了 Adobe handler 才会走到那道门，
// 分派没接上的话拿到的是 404。
func TestGatewayRoutesDispatchesAdobeImages(t *testing.T) {
	group := &service.Group{ID: 1, Platform: service.PlatformAdobe, AllowImageGeneration: false}
	router := adobeRoutesRouter(t, group)

	// edits 在解析阶段就要求源图，两个端点的请求体不同。
	bodies := map[string]string{
		"/v1/images/generations": `{"model":"gpt-image-2","prompt":"a cat"}`,
		"/v1/images/edits": `{"model":"gpt-image-2","prompt":"a cat","images":[` +
			`{"image_url":"data:image/png;base64,iVBORw0KGgo="}]}`,
	}
	for path, body := range bodies {
		w := postAdobeImages(t, router, path, body)
		require.Equal(t, http.StatusForbidden, w.Code, "%s: %s", path, w.Body.String())
		require.Contains(t, w.Body.String(), service.ImageGenerationPermissionMessage())
	}
}

// 未知模型应在解析阶段就被挡下，且错误体是 OpenAI 形状——images 端点的客户端
// 按 OpenAI 协议解析响应。
func TestGatewayRoutesAdobeImagesRejectsNonImageModel(t *testing.T) {
	group := &service.Group{ID: 1, Platform: service.PlatformAdobe, AllowImageGeneration: true}
	router := adobeRoutesRouter(t, group)

	w := postAdobeImages(t, router, "/v1/images/generations", `{"model":"claude-sonnet-4-6","prompt":"x"}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var payload struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Equal(t, "invalid_request_error", payload.Error.Type)
	require.Contains(t, payload.Error.Message, "image model")
}

// firefly-* 模型必须能通过 images 端点的模型校验——该校验原本只认 gpt-image-*/grok。
func TestGatewayRoutesAdobeImagesAcceptsFireflyModel(t *testing.T) {
	group := &service.Group{ID: 1, Platform: service.PlatformAdobe, AllowImageGeneration: false}
	router := adobeRoutesRouter(t, group)

	// 权限门排在模型校验之后：拿到 403 说明模型已通过校验。
	w := postAdobeImages(t, router, "/v1/images/generations",
		`{"model":"firefly-nano-banana-pro","prompt":"x"}`)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

// 非 Adobe 且非 OpenAI/Grok 的分组仍应拿到 404，确认新增分支没有放宽其它平台。
func TestGatewayRoutesImagesStillRejectsOtherPlatforms(t *testing.T) {
	group := &service.Group{ID: 1, Platform: service.PlatformAnthropic, AllowImageGeneration: true}
	router := adobeRoutesRouter(t, group)

	w := postAdobeImages(t, router, "/v1/images/generations", `{"model":"gpt-image-2","prompt":"x"}`)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "not supported for this platform")
}

func TestGatewayRoutesCompositeDetectableAdobeModelsDispatchToAdobeImages(t *testing.T) {
	group := &service.Group{ID: 1, Platform: service.PlatformComposite, AllowImageGeneration: false}
	router := adobeRoutesRouter(t, group)

	for _, model := range []string{"nano-banana-pro", "firefly-gpt-image-2"} {
		w := postAdobeImages(t, router, "/v1/images/generations", `{"model":"`+model+`","prompt":"x"}`)
		require.Equal(t, http.StatusForbidden, w.Code, "%s: %s", model, w.Body.String())
		require.Contains(t, w.Body.String(), service.ImageGenerationPermissionMessage(), model)
	}
}

func TestGatewayRoutesCompositeDallEDispatchesToOpenAIImages(t *testing.T) {
	group := &service.Group{ID: 1, Platform: service.PlatformComposite, AllowImageGeneration: false}
	router := adobeRoutesRouter(t, group)

	w := postAdobeImages(t, router, "/v1/images/generations", `{"model":"dall-e-3","prompt":"x"}`)
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Service temporarily unavailable")
	require.NotContains(t, w.Body.String(), service.ImageGenerationPermissionMessage())
}

func TestGatewayRoutesCompositeUnresolvedGptImageReturns404(t *testing.T) {
	group := &service.Group{ID: 1, Platform: service.PlatformComposite, AllowImageGeneration: true}
	router := adobeRoutesRouter(t, group)

	w := postAdobeImages(t, router, "/v1/images/generations", `{"model":"gpt-image-2","prompt":"x"}`)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "not supported for this platform")
}

func TestGatewayRoutesCompositeExplicitAdobeRouteDispatchesGptImage(t *testing.T) {
	group := &service.Group{ID: 1, Platform: service.PlatformComposite, AllowImageGeneration: false}
	resolver := service.NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []service.CompositeModelRoute{
			{
				ID:             1,
				GroupID:        1,
				PublicModel:    "gpt-image-2",
				MatchType:      service.CompositeRouteMatchExact,
				TargetPlatform: service.PlatformAdobe,
				UpstreamModel:  "firefly-gpt-image-2",
				Endpoint:       service.CompositeRouteEndpointImages,
				Priority:       100,
				Enabled:        true,
			},
		},
	})
	router := adobeRoutesRouterWithResolver(t, group, resolver)

	w := postAdobeImages(t, router, "/v1/images/generations", `{"model":"gpt-image-2","prompt":"x"}`)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), service.ImageGenerationPermissionMessage())
}
