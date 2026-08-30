package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type modelPricePlazaStub struct {
	groups []service.PlazaGroup
	calls  int
}

func (s *modelPricePlazaStub) ListGroups(context.Context) ([]service.PlazaGroup, error) {
	s.calls++
	return append([]service.PlazaGroup(nil), s.groups...), nil
}

type modelPriceCatalogStub struct {
	groups []service.PlazaGroup
}

func (s *modelPriceCatalogStub) BuildCatalog(_ context.Context, groups []service.PlazaGroup) (*service.DisplayPricingCatalog, error) {
	s.groups = append([]service.PlazaGroup(nil), groups...)
	return &service.DisplayPricingCatalog{Providers: []service.DisplayCatalogProvider{}}, nil
}

type modelPriceVisibilityStub struct {
	allowed        map[int64]struct{}
	restrictPublic bool
	userID         int64
	calls          int
}

func (s *modelPriceVisibilityStub) GetUserGroupVisibility(_ context.Context, userID int64) (map[int64]struct{}, bool, error) {
	s.calls++
	s.userID = userID
	return s.allowed, s.restrictPublic, nil
}

type modelPriceRuntimeStub struct {
	runtime service.ModelPlazaRuntime
}

func (s modelPriceRuntimeStub) GetModelPlazaRuntime(context.Context) service.ModelPlazaRuntime {
	return s.runtime
}

func TestModelPriceHandlerAllowsAnonymousAndFiltersExclusiveGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	plaza := &modelPricePlazaStub{groups: []service.PlazaGroup{
		{ID: 1, Name: "public"},
		{ID: 2, Name: "exclusive", IsExclusive: true},
	}}
	catalog := &modelPriceCatalogStub{}
	visibility := &modelPriceVisibilityStub{}
	h := &ModelPriceHandler{
		plazaService:   plaza,
		displayService: catalog,
		apiKeyService:  visibility,
		settingService: modelPriceRuntimeStub{runtime: service.ModelPlazaRuntime{Enabled: true, RequireAuth: false}},
	}
	router := gin.New()
	router.GET("/model-prices", h.Get)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/model-prices", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "no-store, no-cache, must-revalidate", recorder.Header().Get("Cache-Control"))
	require.Equal(t, 1, plaza.calls)
	require.Zero(t, visibility.calls)
	require.Len(t, catalog.groups, 1)
	require.Equal(t, "public", catalog.groups[0].Name)
}

func TestModelPriceHandlerRequiresAuthenticationWhenConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	plaza := &modelPricePlazaStub{}
	h := &ModelPriceHandler{
		plazaService:   plaza,
		displayService: &modelPriceCatalogStub{},
		settingService: modelPriceRuntimeStub{runtime: service.ModelPlazaRuntime{Enabled: true, RequireAuth: true}},
	}
	router := gin.New()
	router.GET("/model-prices", h.Get)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/model-prices", nil))

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Zero(t, plaza.calls)
}

func TestModelPriceHandlerKeepsAuthenticatedGroupVisibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	plaza := &modelPricePlazaStub{groups: []service.PlazaGroup{
		{ID: 1, Name: "public"},
		{ID: 2, Name: "allowed-exclusive", IsExclusive: true},
		{ID: 3, Name: "denied-exclusive", IsExclusive: true},
	}}
	catalog := &modelPriceCatalogStub{}
	visibility := &modelPriceVisibilityStub{allowed: map[int64]struct{}{2: {}}}
	h := &ModelPriceHandler{
		plazaService:   plaza,
		displayService: catalog,
		apiKeyService:  visibility,
		settingService: modelPriceRuntimeStub{runtime: service.ModelPlazaRuntime{Enabled: true, RequireAuth: false}},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		c.Next()
	})
	router.GET("/model-prices", h.Get)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/model-prices", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, visibility.calls)
	require.EqualValues(t, 42, visibility.userID)
	require.Len(t, catalog.groups, 2)
	require.Equal(t, "public", catalog.groups[0].Name)
	require.Equal(t, "allowed-exclusive", catalog.groups[1].Name)
}
