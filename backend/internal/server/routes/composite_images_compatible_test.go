//go:build unit

package routes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
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

type compatibleImagesAccounts struct {
	service.AccountRepository
	accounts []service.Account
}

func (r compatibleImagesAccounts) GetByID(_ context.Context, id int64) (*service.Account, error) {
	for _, account := range r.accounts {
		if account.ID == id {
			return &account, nil
		}
	}
	return nil, service.ErrNoAvailableAccounts
}

func (r compatibleImagesAccounts) ListSchedulableByPlatform(context.Context, string) ([]service.Account, error) {
	return r.accounts, nil
}

func (r compatibleImagesAccounts) ListSchedulableUngroupedByPlatform(context.Context, string) ([]service.Account, error) {
	return r.accounts, nil
}

func (r compatibleImagesAccounts) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]service.Account, error) {
	return r.accounts, nil
}

type compatibleImagesUpstream struct {
	service.HTTPUpstream
	accountIDs []int64
	body       []byte
}

func (u *compatibleImagesUpstream) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	u.accountIDs = append(u.accountIDs, id)
	var err error
	u.body, err = io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}, "X-Request-Id": {"compatible-image-test"}}, Body: io.NopCloser(strings.NewReader(`{"data":[{"b64_json":"aW1hZ2U="}]}`))}, nil
}

type compatibleImagesUsage struct {
	service.UsageLogRepository
	logs []*service.UsageLog
}

func (r *compatibleImagesUsage) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	r.logs = append(r.logs, log)
	return true, nil
}

func TestCompositeCompatibleImagesEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, scenario := range []string{"generation", "json_edit", "multipart_alias", "disabled", "native_only", "restricted"} {
		t.Run(scenario, func(t *testing.T) {
			const model = "gemini-3.1-flash-image"
			groupID := int64(101)
			price := 0.17
			group := &service.Group{ID: groupID, Platform: service.PlatformComposite, AllowImageGeneration: scenario != "disabled", RateMultiplier: 1, ImagePrice1K: &price, ImagePrice2K: &price, ImagePrice4K: &price}
			accounts := []service.Account{}
			// Native accounts deliberately advertise the same model and have higher
			// priority, proving the image capability fence controls selection.
			for i, typ := range []string{service.AccountTypeOAuth, service.AccountTypeSetupToken, service.AccountTypeAPIKey} {
				if scenario == "native_only" && typ == service.AccountTypeAPIKey {
					continue
				}
				mapping := map[string]any{model: model}
				if scenario == "restricted" && typ == service.AccountTypeAPIKey {
					mapping = map[string]any{"gpt-image-2": "gpt-image-2"}
				}
				accounts = append(accounts, service.Account{ID: int64(i + 1), Platform: service.PlatformOpenAI, Type: typ, Status: service.StatusActive, Schedulable: true, Priority: i, Credentials: map[string]any{"api_key": "fixture-key", "access_token": "unused-native-token", "base_url": "https://compatible.example/v1", "model_mapping": mapping}})
			}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			repo := compatibleImagesAccounts{accounts: accounts}
			upstream, usage := &compatibleImagesUpstream{}, &compatibleImagesUsage{}
			billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billingCache.Stop)
			gateway := service.NewOpenAIGatewayService(repo, usage, nil, nil, nil, nil, nil, cfg, nil, nil, service.NewBillingService(cfg, nil), nil, billingCache, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
			imagesHandler := handler.NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billingCache, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
			publicModel := model
			if scenario == "multipart_alias" {
				publicModel = "public-image"
			}
			resolver := service.NewCompositeRouteResolver(compositeRouteRepoStub{routes: []service.CompositeModelRoute{{ID: 1, GroupID: groupID, PublicModel: publicModel, MatchType: service.CompositeRouteMatchExact, TargetPlatform: service.PlatformOpenAI, UpstreamModel: model, Endpoint: service.CompositeRouteEndpointImages, Enabled: true}}})
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{ID: 202, GroupID: &groupID, Group: group, User: &service.User{ID: 303}})
				c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 303})
				c.Next()
			})
			router.Use(compositeTargetPlatformMiddleware(resolver))
			router.POST("/v1/images/generations", imagesHandler.Images)
			router.POST("/v1/images/edits", imagesHandler.Images)
			endpoint, contentType := "/v1/images/generations", "application/json"
			body := []byte(fmt.Sprintf(`{"model":%q,"prompt":"draw","size":"1024x1024"}`, publicModel))
			if scenario == "json_edit" {
				endpoint = "/v1/images/edits"
				body = []byte(fmt.Sprintf(`{"model":%q,"prompt":"draw","images":[{"image_url":"https://source.example/input.png"}]}`, publicModel))
			}
			if scenario == "multipart_alias" {
				endpoint = "/v1/images/edits"
				var buf bytes.Buffer
				writer := multipart.NewWriter(&buf)
				require.NoError(t, writer.WriteField("model", publicModel))
				require.NoError(t, writer.WriteField("prompt", "draw"))
				part, err := writer.CreateFormFile("image", "input.png")
				require.NoError(t, err)
				_, err = part.Write([]byte("fixture-image"))
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				body, contentType = buf.Bytes(), writer.FormDataContentType()
			}
			req := httptest.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
			req.Header.Set("Content-Type", contentType)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if scenario == "disabled" || scenario == "native_only" || scenario == "restricted" {
				require.GreaterOrEqual(t, rec.Code, 400, rec.Body.String())
				if scenario == "disabled" {
					require.Equal(t, http.StatusForbidden, rec.Code)
				}
				require.Empty(t, upstream.accountIDs)
				require.Empty(t, usage.logs)
				return
			}
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Equal(t, []int64{3}, upstream.accountIDs)
			require.Contains(t, string(upstream.body), model)
			require.Contains(t, rec.Body.String(), "aW1hZ2U=")
			require.Len(t, usage.logs, 1, "the image must reach usage recording, not just return HTTP 200")
			require.Equal(t, 1, usage.logs[0].ImageCount)
			require.InDelta(t, price, usage.logs[0].ActualCost, 1e-9)
			require.Equal(t, publicModel, usage.logs[0].OriginalModel)
		})
	}
}
