package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type displayPricingHandlerRepo struct {
	providers map[string]service.DisplayPricingProvider
	models    []service.DisplayModelPrice
}

func (r *displayPricingHandlerRepo) GetSettings(context.Context) (*service.DisplayPricingSettings, error) {
	return &service.DisplayPricingSettings{GlobalMultiplier: 1}, nil
}
func (r *displayPricingHandlerRepo) UpdateSettings(context.Context, *service.DisplayPricingSettings) error {
	return nil
}
func (r *displayPricingHandlerRepo) ListProviders(context.Context) ([]service.DisplayPricingProvider, error) {
	out := make([]service.DisplayPricingProvider, 0, len(r.providers))
	for _, provider := range r.providers {
		out = append(out, provider)
	}
	return out, nil
}
func (r *displayPricingHandlerRepo) CreateProvider(_ context.Context, provider *service.DisplayPricingProvider) error {
	if _, ok := r.providers[provider.Provider]; ok {
		return service.ErrDisplayProviderExists
	}
	provider.UpdatedAt = time.Now()
	r.providers[provider.Provider] = *provider
	return nil
}
func (r *displayPricingHandlerRepo) UpdateProvider(_ context.Context, provider *service.DisplayPricingProvider) error {
	if _, ok := r.providers[provider.Provider]; !ok {
		return service.ErrDisplayProviderNotFound
	}
	provider.UpdatedAt = time.Now()
	r.providers[provider.Provider] = *provider
	return nil
}
func (r *displayPricingHandlerRepo) DeleteProvider(_ context.Context, provider string) (int64, error) {
	if _, ok := r.providers[provider]; !ok {
		return 0, service.ErrDisplayProviderNotFound
	}
	delete(r.providers, provider)
	kept := r.models[:0]
	var deleted int64
	for _, model := range r.models {
		if model.Provider == provider {
			deleted++
		} else {
			kept = append(kept, model)
		}
	}
	r.models = kept
	return deleted, nil
}
func (r *displayPricingHandlerRepo) ListModels(context.Context) ([]service.DisplayModelPrice, error) {
	return r.models, nil
}
func (r *displayPricingHandlerRepo) GetModel(context.Context, int64) (*service.DisplayModelPrice, error) {
	return nil, service.ErrDisplayPriceNotFound
}
func (r *displayPricingHandlerRepo) UpsertModel(_ context.Context, model *service.DisplayModelPrice) error {
	model.ID = int64(len(r.models) + 1)
	model.CreatedAt = time.Now()
	model.UpdatedAt = model.CreatedAt
	r.models = append(r.models, *model)
	return nil
}
func (r *displayPricingHandlerRepo) UpdateModel(context.Context, *service.DisplayModelPrice) error {
	return nil
}
func (r *displayPricingHandlerRepo) DeleteModel(context.Context, int64) error { return nil }

func TestDisplayPricingProviderCRUDHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &displayPricingHandlerRepo{providers: make(map[string]service.DisplayPricingProvider)}
	h := NewDisplayPricingHandler(service.NewDisplayPricingService(repo), nil)
	router := gin.New()
	router.POST("/providers", h.CreateProvider)
	router.PUT("/providers/:provider", h.UpdateProvider)
	router.DELETE("/providers/:provider", h.DeleteProvider)
	router.POST("/models", h.UpsertModel)

	create := httptest.NewRecorder()
	createBody := `{"provider":"custom","display_name":"Custom","provider_note":"Peak hour note","currency":"USD","multiplier":0.2,"logo_key":"custom","logo_url":"https://cdn.example.com/custom.svg","sort_order":8}`
	createReq := httptest.NewRequest(http.MethodPost, "/providers", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(create, createReq)
	require.Equal(t, http.StatusCreated, create.Code)
	var createResponse response.Response
	require.NoError(t, json.Unmarshal(create.Body.Bytes(), &createResponse))
	createData := createResponse.Data.(map[string]any)
	require.Equal(t, "custom", createData["logo_key"])
	require.Equal(t, "https://cdn.example.com/custom.svg", createData["logo_url"])
	require.Equal(t, "Peak hour note", createData["provider_note"])

	update := httptest.NewRecorder()
	updateBody := `{"display_name":"Custom 2","provider_note":"Updated note","currency":"USD","multiplier":0.25,"logo_key":"custom","logo_url":"/assets/custom.svg","sort_order":9}`
	updateReq := httptest.NewRequest(http.MethodPut, "/providers/custom", strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(update, updateReq)
	require.Equal(t, http.StatusOK, update.Code)
	require.Contains(t, update.Body.String(), `"display_name":"Custom 2"`)
	require.Contains(t, update.Body.String(), `"provider_note":"Updated note"`)

	createModel := httptest.NewRecorder()
	modelBody := `{"platform":"openai","model_name":"custom-model","provider":"custom","billing_mode":"token","currency":"USD","model_note":"  Launch note  "}`
	modelReq := httptest.NewRequest(http.MethodPost, "/models", strings.NewReader(modelBody))
	modelReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createModel, modelReq)
	require.Equal(t, http.StatusOK, createModel.Code)
	require.Contains(t, createModel.Body.String(), `"model_note":"Launch note"`)

	repo.models = []service.DisplayModelPrice{{Provider: "custom"}, {Provider: "custom"}}
	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(http.MethodDelete, "/providers/custom", nil))
	require.Equal(t, http.StatusOK, remove.Code)
	require.Contains(t, remove.Body.String(), `"deleted_models":2`)
}

func TestDisplayPricingCreateProviderRejectsUnsafeLogoURL(t *testing.T) {
	repo := &displayPricingHandlerRepo{providers: make(map[string]service.DisplayPricingProvider)}
	h := NewDisplayPricingHandler(service.NewDisplayPricingService(repo), nil)
	router := gin.New()
	router.POST("/providers", h.CreateProvider)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/providers", strings.NewReader(`{"provider":"custom","display_name":"Custom","currency":"USD","logo_url":"javascript:alert(1)"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "DISPLAY_PROVIDER_INVALID")
}
