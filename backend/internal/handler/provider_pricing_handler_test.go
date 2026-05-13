package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProviderPricingModelFromPricingConvertsToFinalCNYPerMillionTokens(t *testing.T) {
	input := 0.000001
	output := 0.000002
	cacheWrite := 0.000003
	cacheRead := 0.0000005

	row, ok := providerPricingModelFromPricing(
		"gpt-5.4",
		"codex",
		&service.ChannelModelPricing{
			BillingMode:      service.BillingModeToken,
			InputPrice:       &input,
			OutputPrice:      &output,
			CacheWritePrice:  &cacheWrite,
			CacheReadPrice:   &cacheRead,
			PerRequestPrice:  ptrFloat64ProviderPricingTest(9),
			ImageOutputPrice: ptrFloat64ProviderPricingTest(8),
		},
		1.5,
		3,
	)

	require.True(t, ok)
	require.Equal(t, "gpt-5.4", row.ModelName)
	require.Equal(t, "codex", row.GroupName)
	require.InEpsilon(t, 0.5, row.InputPrice, 0.000001)
	require.NotNil(t, row.OutputPrice)
	require.InEpsilon(t, 1.0, *row.OutputPrice, 0.000001)
	require.NotNil(t, row.CacheInputPrice)
	require.InEpsilon(t, 0.25, *row.CacheInputPrice, 0.000001)
	require.NotNil(t, row.CacheCreatePrice)
	require.InEpsilon(t, 1.5, *row.CacheCreatePrice, 0.000001)
	require.Nil(t, row.CacheCreatePrice1h)
	require.True(t, row.Enabled)
	require.Empty(t, row.Note)
}

func TestProviderPricingModelFromPricingSkipsUnsupportedPricing(t *testing.T) {
	input := 0.000001

	_, ok := providerPricingModelFromPricing(
		"dalle",
		"image",
		&service.ChannelModelPricing{BillingMode: service.BillingModeImage, InputPrice: &input},
		1,
		1,
	)
	require.False(t, ok)

	_, ok = providerPricingModelFromPricing(
		"free-form",
		"group",
		&service.ChannelModelPricing{BillingMode: service.BillingModeToken},
		1,
		1,
	)
	require.False(t, ok)
}

func TestProviderPricingHandlerUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/provider/pricing", (&ProviderPricingHandler{}).Get)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/provider/pricing", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	var body providerPricingResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, providerPricingSchemaVersion, body.SchemaVersion)
	require.False(t, body.Success)
	require.NotEmpty(t, body.Message)
	require.Nil(t, body.Data)
}

func ptrFloat64ProviderPricingTest(v float64) *float64 {
	return &v
}
