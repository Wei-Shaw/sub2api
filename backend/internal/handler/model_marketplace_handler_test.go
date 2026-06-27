package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelMarketplaceHandlerListPricing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	billingSvc := service.NewBillingService(&config.Config{}, nil)
	marketplaceSvc := service.NewModelMarketplaceService(nil, billingSvc, nil)
	h := NewModelMarketplaceHandler(marketplaceSvc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		c.Next()
	})
	r.GET("/api/v1/models/pricing", h.ListPricing)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/models/pricing?q=deepseek&page=1&page_size=5", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Items      []service.ModelMarketplaceItem     `json:"items"`
			Pagination service.ModelMarketplacePagination `json:"pagination"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)
	require.NotEmpty(t, envelope.Data.Items)
	require.Equal(t, 1, envelope.Data.Pagination.Page)
	require.LessOrEqual(t, len(envelope.Data.Items), 5)
	require.Equal(t, "DeepSeek", envelope.Data.Items[0].ProviderLabel)
}
