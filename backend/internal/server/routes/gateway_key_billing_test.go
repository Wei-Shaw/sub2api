package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/web"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type keyBillingRouteAPIKeyRepo struct {
	service.APIKeyRepository
	apiKey *service.APIKey
}

func (r *keyBillingRouteAPIKeyRepo) GetByKeyForAuth(_ context.Context, key string) (*service.APIKey, error) {
	if r.apiKey == nil || key != r.apiKey.Key {
		return nil, service.ErrAPIKeyNotFound
	}
	clone := *r.apiKey
	return &clone, nil
}

type keyBillingRouteRateRepo struct {
	service.UserGroupRateRepository
	lookupCalls int
}

type keyBillingRouteGroupRepo struct {
	service.GroupRepository
	group *service.Group
}

func (r *keyBillingRouteGroupRepo) GetByIDLite(_ context.Context, id int64) (*service.Group, error) {
	if r.group == nil || id != r.group.ID {
		return nil, service.ErrGroupNotFound
	}
	return r.group, nil
}

func (r *keyBillingRouteRateRepo) GetByUserAndGroup(context.Context, int64, int64) (*float64, error) {
	r.lookupCalls++
	return nil, nil
}

func (r *keyBillingRouteRateRepo) GetRPMOverrideByUserAndGroup(context.Context, int64, int64) (*int, error) {
	return nil, nil
}

func newKeyBillingRouteTestRouter(runMode string, groups service.GroupRepository) (*gin.Engine, *keyBillingRouteRateRepo, string) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{
		ID:               42,
		Name:             "标准分组",
		Description:      "共享账号",
		I18n:             service.GroupI18n{"en": {Name: "Standard"}},
		Status:           service.StatusActive,
		Hydrated:         true,
		Platform:         service.PlatformOpenAI,
		SubscriptionType: service.SubscriptionTypeStandard,
		RateMultiplier:   0.75,
	}
	user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive, Balance: 10}
	var groupID *int64
	var apiKeyGroup *service.Group
	if runMode != config.RunModeSimple {
		groupID = &group.ID
		apiKeyGroup = group
	}
	apiKey := &service.APIKey{
		ID:      100,
		UserID:  user.ID,
		Key:     "billing-route-test-key",
		Status:  service.StatusActive,
		User:    user,
		GroupID: groupID,
		Group:   apiKeyGroup,
	}
	cfg := &config.Config{RunMode: runMode}
	rateRepo := &keyBillingRouteRateRepo{}
	if groups == nil {
		groups = &keyBillingRouteGroupRepo{group: group}
	}
	apiKeyService := service.NewAPIKeyService(
		&keyBillingRouteAPIKeyRepo{apiKey: apiKey}, nil, groups, nil, rateRepo, nil, cfg,
	)
	gatewayService := service.NewGatewayService(
		nil, nil, nil, nil, nil, nil, rateRepo, nil, cfg, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	openAIGatewayService := service.NewOpenAIGatewayService(
		nil, nil, nil, nil, nil, rateRepo, nil, cfg, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	gatewayHandler := handler.NewGatewayHandler(
		gatewayService, openAIGatewayService, nil, nil, nil, nil, nil, nil,
		apiKeyService, nil, nil, nil, nil, cfg, nil,
	)

	router := gin.New()
	if web.HasEmbeddedFrontend() {
		router.Use(web.ServeEmbeddedFrontend())
	}
	RegisterGatewayRoutes(
		router,
		&handler.Handlers{Gateway: gatewayHandler, OpenAIGateway: &handler.OpenAIGatewayHandler{}},
		servermiddleware.NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg),
		apiKeyService,
		nil,
		nil,
		nil,
		nil,
		cfg,
	)
	return router, rateRepo, apiKey.Key
}

func TestGatewayRoutesKeyBillingInfoPathIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, route := range router.Routes() {
		if route.Method == http.MethodGet && route.Path == "/v1/sub2api/billing" {
			return
		}
	}

	t.Fatal("GET /v1/sub2api/billing should be registered")
}

func TestGatewayRoutesKeyBillingInfoEndToEnd(t *testing.T) {
	t.Run("missing credentials", func(t *testing.T) {
		router, rateRepo, _ := newKeyBillingRouteTestRouter(config.RunModeStandard, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/sub2api/billing", nil))

		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Contains(t, w.Header().Get("Content-Type"), "application/json")
		require.NotContains(t, strings.ToLower(w.Body.String()), "<!doctype html>")
		require.Zero(t, rateRepo.lookupCalls)
	})

	t.Run("standard mode", func(t *testing.T) {
		router, rateRepo, key := newKeyBillingRouteTestRouter(config.RunModeStandard, nil)
		req := httptest.NewRequest(http.MethodGet, "/v1/sub2api/billing", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Header().Get("Content-Type"), "application/json")
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		require.NotContains(t, strings.ToLower(w.Body.String()), "<!doctype html>")
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, "sub2api.key_billing", body["object"])
		require.NotContains(t, body, "group_name")
		require.NotContains(t, body, "group_description")
		require.Equal(t, 0.75, body["effective_rate_multiplier"])
		require.Equal(t, 1, rateRepo.lookupCalls)
	})

	t.Run("requested group belongs to the calling key", func(t *testing.T) {
		router, _, key := newKeyBillingRouteTestRouter(config.RunModeStandard, nil)
		req := httptest.NewRequest(http.MethodGet, "/v1/sub2api/billing?include=group&group_id=99", nil)
		req.Header.Set("x-api-key", key)
		req.Header.Set("Accept-Language", "en")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, "Standard", body["group_name"])
		require.Equal(t, "共享账号", body["group_description"])
		require.Equal(t, 0.75, body["group_rate_multiplier"])
		require.NotContains(t, body, "group_id")
		require.NotContains(t, w.Body.String(), key)
	})

	t.Run("include accepts a list of sections", func(t *testing.T) {
		for query, wantGroup := range map[string]bool{
			"include=group,model_pricing":         true,
			"include=model_pricing,%20group":      true,
			"include=model_pricing&include=group": true,
			"include=group&include=model_pricing": true,
			"include=model_pricing":               false,
			"include=groups":                      false,
			"include=":                            false,
		} {
			router, _, key := newKeyBillingRouteTestRouter(config.RunModeStandard, nil)
			req := httptest.NewRequest(http.MethodGet, "/v1/sub2api/billing?"+query, nil)
			req.Header.Set("x-api-key", key)
			req.Header.Set("Accept-Language", "en")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code, query)
			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), query)
			require.Equal(t, 0.75, body["effective_rate_multiplier"], query)
			if wantGroup {
				require.Equal(t, "Standard", body["group_name"], query)
			} else {
				require.NotContains(t, body, "group_name", query)
			}
		}
	})

	t.Run("optional group lookup failure preserves billing rates", func(t *testing.T) {
		router, _, key := newKeyBillingRouteTestRouter(config.RunModeStandard, &keyBillingRouteGroupRepo{})
		req := httptest.NewRequest(http.MethodGet, "/v1/sub2api/billing?include=group", nil)
		req.Header.Set("x-api-key", key)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, 0.75, body["effective_rate_multiplier"])
		require.NotContains(t, body, "group_name")
	})

	t.Run("simple mode", func(t *testing.T) {
		router, rateRepo, key := newKeyBillingRouteTestRouter(config.RunModeSimple, nil)
		req := httptest.NewRequest(http.MethodGet, "/v1/sub2api/billing", nil)
		req.Header.Set("x-api-key", key)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		require.Contains(t, w.Header().Get("Content-Type"), "application/json")
		require.NotContains(t, strings.ToLower(w.Body.String()), "<!doctype html>")
		require.JSONEq(t, `{
			"type": "error",
			"error": {
				"type": "not_found_error",
				"message": "Billing information is not supported in simple mode"
			}
		}`, w.Body.String())
		require.Zero(t, rateRepo.lookupCalls)
	})
}
