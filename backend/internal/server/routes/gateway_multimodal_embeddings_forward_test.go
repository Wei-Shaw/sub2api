package routes

import (
	"context"
	"fmt"
	"io"
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
	"github.com/tidwall/gjson"
)

type embeddingsRouteAccountRepo struct {
	service.AccountRepository
	account service.Account
}

func (r embeddingsRouteAccountRepo) GetByID(context.Context, int64) (*service.Account, error) {
	return &r.account, nil
}

func (r embeddingsRouteAccountRepo) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]service.Account, error) {
	return []service.Account{r.account}, nil
}

func (r embeddingsRouteAccountRepo) ListSchedulableByPlatform(context.Context, string) ([]service.Account, error) {
	return []service.Account{r.account}, nil
}

func (r embeddingsRouteAccountRepo) ListSchedulableUngroupedByPlatform(context.Context, string) ([]service.Account, error) {
	return []service.Account{r.account}, nil
}

type embeddingsRouteUsageRepo struct {
	service.UsageLogRepository
	lastLog *service.UsageLog
}

func (r *embeddingsRouteUsageRepo) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	r.lastLog = log
	return true, nil
}

type embeddingsRouteUpstream struct {
	service.HTTPUpstream
	lastURL  string
	lastBody []byte
	calls    int
}

func (u *embeddingsRouteUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls++
	u.lastURL = req.URL.String()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.lastBody = body
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"embedding-route-test"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":{"embedding":[0.1,0.2],"object":"embedding"},"usage":{"prompt_tokens":110,"prompt_tokens_details":{"image_tokens":100,"text_tokens":10}}}`)),
	}, nil
}

func newEmbeddingsForwardTestRouter(t *testing.T, platform string) (*gin.Engine, *embeddingsRouteUpstream, *embeddingsRouteUsageRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{RunMode: config.RunModeSimple, Gateway: config.GatewayConfig{MaxBodySize: 4096, TextMaxBodySize: 256}}
	upstream := &embeddingsRouteUpstream{}
	usage := &embeddingsRouteUsageRepo{}
	repo := embeddingsRouteAccountRepo{account: service.Account{
		ID: 42, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{
			"api_key": "upstream-test-key", "base_url": "https://ark.cn-beijing.volces.com/api/v3",
			"model_mapping": map[string]any{"vision-embedding": "doubao-embedding-vision-251215"},
		},
	}}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	concurrency := service.NewConcurrencyService(nil)
	apiKeys := service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg)
	gateway := service.NewOpenAIGatewayService(
		repo, usage, nil, nil, nil, nil, nil, cfg,
		nil, concurrency, service.NewBillingService(cfg, nil), nil, billing, upstream,
		nil, nil, nil, nil, nil, nil, nil, nil,
	)
	h := handler.NewOpenAIGatewayHandler(gateway, concurrency, billing, apiKeys, nil, nil, nil, nil, cfg)
	resolver := service.NewCompositeRouteResolver(compositeRouteRepoStub{routes: []service.CompositeModelRoute{{
		ID: 1, GroupID: 1, PublicModel: "vision-embedding", MatchType: service.CompositeRouteMatchExact,
		TargetPlatform: service.PlatformOpenAI, UpstreamModel: "vision-embedding", Endpoint: service.CompositeRouteEndpointEmbeddings, Enabled: true,
	}}})
	router := gin.New()
	RegisterGatewayRoutes(router, &handler.Handlers{
		Gateway: &handler.GatewayHandler{}, OpenAIGateway: h, AsyncImage: handler.NewAsyncImageHandler(nil, nil),
	}, servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		groupID := int64(1)
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
			ID: 1, GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: platform}, User: &service.User{ID: 2},
		})
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 2})
		c.Next()
	}), apiKeys, nil, nil, nil, resolver, cfg)
	return router, upstream, usage
}

func TestGatewayRoutesMultimodalEmbeddingsForwardAndRecordUsage(t *testing.T) {
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformComposite} {
		for _, path := range multimodalEmbeddingPaths {
			t.Run(platform+path, func(t *testing.T) {
				router, upstream, usage := newEmbeddingsForwardTestRouter(t, platform)
				image := "data:image/png;base64," + strings.Repeat("a", 512)
				body := `{"model":"vision-embedding","input":[{"type":"image_url","image_url":{"url":"` + image + `"}}],"dimensions":1024}`
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))

				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Equal(t, 1, upstream.calls)
				require.Equal(t, "https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal", upstream.lastURL)
				require.Equal(t, "doubao-embedding-vision-251215", gjson.GetBytes(upstream.lastBody, "model").String())
				require.Equal(t, image, gjson.GetBytes(upstream.lastBody, "input.0.image_url.url").String())
				require.Equal(t, int64(1024), gjson.GetBytes(upstream.lastBody, "dimensions").Int())
				require.Equal(t, "embedding", gjson.GetBytes(rec.Body.Bytes(), "data.object").String())
				require.NotNil(t, usage.lastLog, "fixture must record usage without a background worker")
				require.Equal(t, 110, usage.lastLog.InputTokens)
				require.Equal(t, 100, usage.lastLog.ImageInputTokens)
				require.Equal(t, handler.EndpointMultimodalEmbeddings, *usage.lastLog.InboundEndpoint)
				require.Equal(t, handler.EndpointMultimodalEmbeddings, *usage.lastLog.UpstreamEndpoint)
			})
		}
	}
}

func TestGatewayRoutesEmbeddingsBodyLimits(t *testing.T) {
	for _, path := range append(append([]string{}, multimodalEmbeddingPaths...), "/v1/embeddings", "/embeddings") {
		for _, chunked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/chunked=%t", path, chunked), func(t *testing.T) {
				router, upstream, usage := newEmbeddingsForwardTestRouter(t, service.PlatformOpenAI)
				size := 5000
				if path == "/v1/embeddings" || path == "/embeddings" {
					size = 512 // Above the text limit but below the multimodal limit.
				}
				body := `{"model":"vision-embedding","input":"` + strings.Repeat("a", size) + `"}`
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				if chunked {
					req.ContentLength = -1
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
				require.Zero(t, upstream.calls)
				require.Nil(t, usage.lastLog)
			})
		}
	}
}
