//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// fakePlazaService 是 plazaServiceAPI 的可观测桩。
type fakePlazaService struct {
	listModelCalls int
	lastFilter     service.PlazaModelFilter
	rows           []service.PlazaModelRow
	cards          []service.PlazaPlanCard
	meta           service.PlazaCurrencyMeta
	listModelErr   error
	listPlansErr   error
}

func (f *fakePlazaService) ListModelRows(_ context.Context, filter service.PlazaModelFilter) ([]service.PlazaModelRow, service.PlazaCurrencyMeta, error) {
	f.listModelCalls++
	f.lastFilter = filter
	if f.listModelErr != nil {
		return nil, service.PlazaCurrencyMeta{}, f.listModelErr
	}
	return f.rows, f.meta, nil
}

func (f *fakePlazaService) ListPlanCards(_ context.Context) ([]service.PlazaPlanCard, service.PlazaCurrencyMeta, error) {
	if f.listPlansErr != nil {
		return nil, service.PlazaCurrencyMeta{}, f.listPlansErr
	}
	return f.cards, f.meta, nil
}

func newPlazaHandlerForTest(svc plazaServiceAPI) *PlazaHandler {
	return &PlazaHandler{plazaService: svc}
}

func performGET(t *testing.T, h gin.HandlerFunc, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/route", h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	router.ServeHTTP(rec, req)
	return rec
}

// =====================================================================
// /api/v1/plaza/models
// =====================================================================

func TestPlazaHandler_ListModels_Anonymous(t *testing.T) {
	fake := &fakePlazaService{
		rows: []service.PlazaModelRow{{GroupID: 1, Model: "claude-3-5-sonnet", Type: "token", InputPricePerMTok: 3, OutputPricePerMTok: 15, SiteInputPricePerMTok: 3, SiteOutputPricePerMTok: 15, Multiplier: 1}},
		meta: service.PlazaCurrencyMeta{BalanceRechargeMultiplier: 0.14, ModelNative: "USD", PlanNative: "CNY"},
	}
	h := newPlazaHandlerForTest(fake)

	rec := performGET(t, h.ListModels, "/route", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Rows         []map[string]any `json:"rows"`
			CurrencyMeta map[string]any   `json:"currency_meta"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Rows, 1)
	require.Equal(t, 0.14, resp.Data.CurrencyMeta["balance_recharge_multiplier"])
}

func TestPlazaHandler_ListModels_IgnoresInvalidAuthHeader(t *testing.T) {
	fake := &fakePlazaService{}
	h := newPlazaHandlerForTest(fake)

	rec := performGET(t, h.ListModels, "/route", map[string]string{
		"Authorization": "Bearer this-is-junk-token",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, fake.listModelCalls)
}

func TestPlazaHandler_ListModels_ParsesQueryParams(t *testing.T) {
	fake := &fakePlazaService{}
	h := newPlazaHandlerForTest(fake)

	rec := performGET(t, h.ListModels, "/route?group_id=42&platform=anthropic&q=Claude", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(42), fake.lastFilter.GroupID)
	require.Equal(t, "anthropic", fake.lastFilter.Platform)
	require.Equal(t, "Claude", fake.lastFilter.Q)
}

func TestPlazaHandler_ListModels_RejectsNegativeGroupID(t *testing.T) {
	fake := &fakePlazaService{}
	h := newPlazaHandlerForTest(fake)

	rec := performGET(t, h.ListModels, "/route?group_id=-5", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, fake.listModelCalls)
}

func TestPlazaHandler_ListModels_RejectsNonNumericGroupID(t *testing.T) {
	fake := &fakePlazaService{}
	h := newPlazaHandlerForTest(fake)

	rec := performGET(t, h.ListModels, "/route?group_id=abc", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, fake.listModelCalls)
}

func TestPlazaHandler_ListModels_RejectsLongQ(t *testing.T) {
	fake := &fakePlazaService{}
	h := newPlazaHandlerForTest(fake)

	long := strings.Repeat("x", 65)
	rec := performGET(t, h.ListModels, "/route?q="+long, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, fake.listModelCalls)
}

// =====================================================================
// /api/v1/plaza/plans
// =====================================================================

func TestPlazaHandler_ListPlans_Anonymous(t *testing.T) {
	op := 199.0
	fake := &fakePlazaService{
		cards: []service.PlazaPlanCard{{
			ID: 1, Name: "Plan A", Price: 99, OriginalPrice: &op, ValidityDays: 30, ValidityUnit: "day",
			GroupID: 10, GroupName: "Alpha", Platform: "anthropic", Models: []string{"claude-3-5-sonnet"},
		}},
		meta: service.PlazaCurrencyMeta{BalanceRechargeMultiplier: 0.14, ModelNative: "USD", PlanNative: "CNY"},
	}
	h := newPlazaHandlerForTest(fake)

	rec := performGET(t, h.ListPlans, "/route", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Cards        []map[string]any `json:"cards"`
			CurrencyMeta map[string]any   `json:"currency_meta"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Cards, 1)
	require.Equal(t, "CNY", resp.Data.CurrencyMeta["plan_native"])
}
