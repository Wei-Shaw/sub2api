package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
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
func (r *displayPricingHandlerRepo) ApplyOfficialPriceUpdates(_ context.Context, updates []service.OfficialPriceUpdate) error {
	for _, update := range updates {
		found := false
		for i := range r.models {
			if r.models[i].ID != update.ModelID || !r.models[i].UpdatedAt.Equal(update.ExpectedUpdatedAt) {
				continue
			}
			found = true
			r.models[i].OfficialInputPerMillion = testDecimalFloat(update.InputPerMillion)
			r.models[i].OfficialOutputPerMillion = testDecimalFloat(update.OutputPerMillion)
			r.models[i].OfficialCacheWritePerMillion = testDecimalFloat(update.CacheWritePerMillion)
			r.models[i].OfficialCacheReadPerMillion = testDecimalFloat(update.CacheReadPerMillion)
			r.models[i].OfficialPriceSource = update.OfficialPriceSource
			r.models[i].OfficialPriceSourceURL = update.OfficialPriceSourceURL
			r.models[i].OfficialPriceSyncedAt = &update.OfficialPriceSyncedAt
			r.models[i].UpdatedAt = update.OfficialPriceSyncedAt
		}
		if !found {
			return service.ErrOfficialPriceApplyConflict
		}
	}
	return nil
}

func testDecimalFloat(value *decimal.Decimal) *float64 {
	if value == nil {
		return nil
	}
	out, _ := value.Float64()
	return &out
}

type displayPricingFetcherStub struct {
	snapshot *service.OfficialPriceSourceSnapshot
}

func (f displayPricingFetcherStub) Fetch(context.Context) (*service.OfficialPriceSourceSnapshot, error) {
	return f.snapshot, nil
}

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
	createBody := `{"provider":"custom","display_name":"Custom","provider_note":"Token note","per_request_note":"Request note","image_note":"Image note","currency":"USD","multiplier":0.2,"logo_key":"custom","logo_url":"https://cdn.example.com/custom.svg","sort_order":8}`
	createReq := httptest.NewRequest(http.MethodPost, "/providers", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(create, createReq)
	require.Equal(t, http.StatusCreated, create.Code)
	var createResponse response.Response
	require.NoError(t, json.Unmarshal(create.Body.Bytes(), &createResponse))
	createData := createResponse.Data.(map[string]any)
	require.Equal(t, "custom", createData["logo_key"])
	require.Equal(t, "https://cdn.example.com/custom.svg", createData["logo_url"])
	require.Equal(t, "Token note", createData["provider_note"])
	require.Equal(t, "Request note", createData["per_request_note"])
	require.Equal(t, "Image note", createData["image_note"])

	update := httptest.NewRecorder()
	updateBody := `{"display_name":"Custom 2","provider_note":"Updated token note","per_request_note":"Updated request note","image_note":"Updated image note","currency":"USD","multiplier":0.25,"logo_key":"custom","logo_url":"/assets/custom.svg","sort_order":9}`
	updateReq := httptest.NewRequest(http.MethodPut, "/providers/custom", strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(update, updateReq)
	require.Equal(t, http.StatusOK, update.Code)
	require.Contains(t, update.Body.String(), `"display_name":"Custom 2"`)
	require.Contains(t, update.Body.String(), `"provider_note":"Updated token note"`)
	require.Contains(t, update.Body.String(), `"per_request_note":"Updated request note"`)
	require.Contains(t, update.Body.String(), `"image_note":"Updated image note"`)

	createModel := httptest.NewRecorder()
	modelBody := `{"platform":"openai","model_name":"custom-model","provider":"custom","billing_mode":"token","currency":"USD","model_note":"  Launch note  ","official_price_source":"herohao_aggregate","official_price_source_url":"https://sub2.herohao.top/pricing/api/pricing","official_price_synced_at":"2026-08-30T10:00:00Z"}`
	modelReq := httptest.NewRequest(http.MethodPost, "/models", strings.NewReader(modelBody))
	modelReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createModel, modelReq)
	require.Equal(t, http.StatusOK, createModel.Code)
	require.Contains(t, createModel.Body.String(), `"model_note":"Launch note"`)
	require.Contains(t, createModel.Body.String(), `"official_price_source":"herohao_aggregate"`)
	require.Contains(t, createModel.Body.String(), `"official_price_synced_at":"2026-08-30T10:00:00Z"`)

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

func TestDisplayPricingOfficialPricePreviewAndApplyHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	input := decimal.RequireFromString("9.8")
	output := decimal.RequireFromString("30.9")
	repo := &displayPricingHandlerRepo{
		providers: make(map[string]service.DisplayPricingProvider),
		models: []service.DisplayModelPrice{{
			ID: 8, Platform: "openai", ModelName: "glm-5.1", Provider: "zhipu",
			BillingMode: service.DisplayBillingModeToken, Currency: service.DisplayCurrencyCNY,
			ModelNote: "unchanged", UpdatedAt: now, OfficialPriceSource: service.DisplayOfficialPriceManual,
		}},
	}
	fetcher := displayPricingFetcherStub{snapshot: &service.OfficialPriceSourceSnapshot{
		FetchedAt: now, Models: map[string]service.OfficialPriceCandidate{
			"glm-5.1": {ModelName: "glm-5.1", ProviderKey: "glm", Currency: service.DisplayCurrencyCNY, Enabled: true, Input: &input, Output: &output},
		},
	}}
	h := NewDisplayPricingHandlerWithOfficialPriceFetcher(service.NewDisplayPricingService(repo), nil, fetcher)
	router := gin.New()
	router.POST("/official-sync/preview", h.PreviewOfficialPrices)
	router.POST("/official-sync/apply", h.ApplyOfficialPrices)

	preview := httptest.NewRecorder()
	router.ServeHTTP(preview, httptest.NewRequest(http.MethodPost, "/official-sync/preview", nil))
	require.Equal(t, http.StatusOK, preview.Code)
	require.Contains(t, preview.Body.String(), `"source":"herohao_aggregate"`)
	require.Contains(t, preview.Body.String(), `"confidence":"unverified"`)
	require.Contains(t, preview.Body.String(), `"input_per_million":9.8`)
	var previewPayload struct {
		Data struct {
			Items []struct {
				ProposalHash string `json:"proposal_hash"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(preview.Body.Bytes(), &previewPayload))
	require.Len(t, previewPayload.Data.Items, 1)
	require.NotEmpty(t, previewPayload.Data.Items[0].ProposalHash)

	applyBody := fmt.Sprintf(`{"models":[{"model_id":8,"expected_updated_at":%q,"proposal_hash":%q}]}`,
		now.Format(time.RFC3339Nano), previewPayload.Data.Items[0].ProposalHash)
	apply := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/official-sync/apply", strings.NewReader(applyBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(apply, request)
	require.Equal(t, http.StatusOK, apply.Code)
	require.Equal(t, 9.8, *repo.models[0].OfficialInputPerMillion)
	require.Equal(t, "unchanged", repo.models[0].ModelNote)
	require.Equal(t, service.OfficialPriceSourceHerohaoAggregate, repo.models[0].OfficialPriceSource)
}
