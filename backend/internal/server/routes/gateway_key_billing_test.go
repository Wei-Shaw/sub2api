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
REDACTED

func (r *keyBillingRouteAPIKeyRepo) GetByKeyForAuth(_ context.Context, key string) (*service.APIKey, error) {
	if r.apiKey == nil || key != r.apiKey.Key {
		return nil, service.ErrAPIKeyNotFound
REDACTED
	clone := *r.apiKey
	return &clone, nil
REDACTED

type keyBillingRouteRateRepo struct {
	service.UserGroupRateRepository
	lookupCalls int
REDACTED

func (r *keyBillingRouteRateRepo) GetByUserAndGroup(context.Context, int64, int64) (*float64, error) {
	r.lookupCalls++
	return nil, nil
REDACTED

func (r *keyBillingRouteRateRepo) GetRPMOverrideByUserAndGroup(context.Context, int64, int64) (*int, error) {
	return nil, nil
REDACTED

func newKeyBillingRouteTestRouter(runMode string) (*gin.Engine, *keyBillingRouteRateRepo, string) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{
		ID:               42,
		Status:           service.StatusActive,
		Hydrated:         true,
		Platform:         service.PlatformOpenAI,
		SubscriptionType: service.SubscriptionTypeStandard,
		RateMultiplier:   0.75,
REDACTED
	user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive, Balance: 10REDACTED
	var groupID *int64
	var apiKeyGroup *service.Group
	if runMode != config.RunModeSimple {
		groupID = &group.ID
		apiKeyGroup = group
REDACTED
	apiKey := &service.APIKey{
		ID:      100,
		UserID:  user.ID,
		Key:     "billing-route-test-key",
		Status:  service.StatusActive,
		User:    user,
		GroupID: groupID,
		Group:   apiKeyGroup,
REDACTED
	cfg := &config.Config{RunMode: runModeREDACTED
	rateRepo := &keyBillingRouteRateRepo{REDACTED
	apiKeyService := service.NewAPIKeyService(
		&keyBillingRouteAPIKeyRepo{apiKey: apiKeyREDACTED, nil, nil, nil, rateRepo, nil, cfg,
	)
	gatewayService := service.NewGatewayService(
		nil, nil, nil, nil, nil, nil, rateRepo, nil, cfg, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
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
REDACTED
	RegisterGatewayRoutes(
		router,
		&handler.Handlers{Gateway: gatewayHandler, OpenAIGateway: &handler.OpenAIGatewayHandler{REDACTEDREDACTED,
		servermiddleware.NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg),
		apiKeyService,
		nil,
		nil,
		nil,
		cfg,
	)
	return router, rateRepo, apiKey.Key
REDACTED

func TestGatewayRoutesKeyBillingInfoPathIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, route := range router.Routes() {
		if route.Method == http.MethodGet && route.Path == "/v1/sub2api/billing" {
			return
	REDACTED
REDACTED

	t.Fatal("GET /v1/sub2api/billing should be registered")
REDACTED

func TestGatewayRoutesKeyBillingInfoEndToEnd(t *testing.T) {
	t.Run("missing credentials", func(t *testing.T) {
		router, rateRepo, _ := newKeyBillingRouteTestRouter(config.RunModeStandard)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/sub2api/billing", nil))

		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Contains(t, w.Header().Get("Content-Type"), "application/json")
		require.NotContains(t, strings.ToLower(w.Body.String()), "<!doctype html>")
		require.Zero(t, rateRepo.lookupCalls)
REDACTED)

	t.Run("standard mode", func(t *testing.T) {
		router, rateRepo, key := newKeyBillingRouteTestRouter(config.RunModeStandard)
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
		require.Equal(t, 0.75, body["effective_rate_multiplier"])
		require.Equal(t, 1, rateRepo.lookupCalls)
REDACTED)

	t.Run("simple mode", func(t *testing.T) {
		router, rateRepo, key := newKeyBillingRouteTestRouter(config.RunModeSimple)
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
		REDACTED
	REDACTED`, w.Body.String())
		require.Zero(t, rateRepo.lookupCalls)
REDACTED)
REDACTED
