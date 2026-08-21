//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type estimatePricingAccountRepo struct {
	service.AccountRepository
}

func (r *estimatePricingAccountRepo) ListSchedulableByGroupID(_ context.Context, _ int64) ([]service.Account, error) {
	return []service.Account{{
		Platform: service.PlatformFal,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"fal-ai/flux/dev": "upstream/provider/model"},
		},
	}}, nil
}

func TestExtractEstimateDimensions(t *testing.T) {
	dimensions, err := extractEstimateDimensions(map[string]any{
		"image_size": map[string]any{"width": json.Number("1200"), "height": json.Number("1800")},
	})
	require.NoError(t, err)
	require.Equal(t, 1200, dimensions.Width)
	require.Equal(t, 1800, dimensions.Height)

	dimensions, err = extractEstimateDimensions(map[string]any{"image_size": "landscape_4_3"})
	require.NoError(t, err)
	require.Equal(t, 1024, dimensions.Width)
	require.Equal(t, 768, dimensions.Height)

	_, err = extractEstimateDimensions(map[string]any{})
	require.ErrorContains(t, err, "width is required")
}

func TestExtractEstimateImageCount(t *testing.T) {
	count, err := extractEstimateImageCount(map[string]any{"num_images": json.Number("3")})
	require.NoError(t, err)
	require.Equal(t, 3, count)

	_, err = extractEstimateImageCount(map[string]any{"n": json.Number("1.5")})
	require.ErrorContains(t, err, "positive integer")
}

func TestModelAPIGatewayNativeEstimatePricing(t *testing.T) {
	price := 0.1
	apiKey := &service.APIKey{Group: &service.Group{
		ID:                9,
		RateMultiplier:    1.5,
		ImagePrice1K:      &price,
		ImageResolution1K: "1024x1024",
		ImageResolution2K: "2048x2048",
		ImageResolution4K: "4096x4096",
	}}
	gatewayService := service.NewGatewayService(
		&estimatePricingAccountRepo{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		&service.BillingService{},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	h := NewModelAPIGatewayHandler(gatewayService, nil, nil, nil, nil, nil, nil)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/model/*path", func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
		h.Native(c)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/model/fal-ai/flux/dev/estimate_pricing",
		strings.NewReader(`{"image_size":{"width":800,"height":800},"num_images":2}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var estimate service.ImagePricingEstimate
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &estimate))
	require.Equal(t, "fal-ai/flux/dev", estimate.Endpoint)
	require.Equal(t, "1K", estimate.Tier)
	require.Equal(t, 2, estimate.ImageCount)
	require.InDelta(t, 0.2, estimate.TotalCost, 1e-9)
	require.InDelta(t, 0.3, estimate.EstimatedPrice, 1e-9)

	unsupportedRecorder := httptest.NewRecorder()
	unsupportedRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/model/made-up/provider/model/estimate_pricing",
		strings.NewReader(`{"image_size":"square"}`),
	)
	unsupportedRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(unsupportedRecorder, unsupportedRequest)
	require.Equal(t, http.StatusNotFound, unsupportedRecorder.Code, unsupportedRecorder.Body.String())
}
