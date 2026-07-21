package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupUpstreamBillingRatesRouter(accounts []service.Account) *gin.Engine {
	gin.SetMode(gin.TestMode)
	adminService := &stubAdminService{accounts: accounts}
	handler := NewAccountHandler(adminService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/admin/accounts/upstream-billing-rates", handler.GetUpstreamBillingRates)
	return router
}

func TestAccountHandlerGetUpstreamBillingRatesReturnsCompactPayloadAndETag(t *testing.T) {
	now := time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC)
	router := setupUpstreamBillingRatesRouter([]service.Account{
		{
			ID:       9,
			Name:     "first",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeAPIKey,
			Extra: map[string]any{
				service.UpstreamBillingProbeExtraKey: map[string]any{
					"status":          service.UpstreamBillingProbeStatusOK,
					"last_attempt_at": now.Format(time.RFC3339Nano),
					"next_probe_at":   now.Add(time.Hour).Format(time.RFC3339Nano),
				},
			},
		},
		{ID: 4, Name: "second"},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/accounts/upstream-billing-rates?page=1&page_size=20&sort_by=upstream_billing_rate&sort_order=desc", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	etag := recorder.Header().Get("ETag")
	require.NotEmpty(t, etag)
	var envelope struct {
		Data struct {
			Items []service.UpstreamBillingRateSnapshotItem `json:"items"`
			Total int64                                     `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, int64(2), envelope.Data.Total)
	require.Equal(t, []int64{9, 4}, []int64{envelope.Data.Items[0].AccountID, envelope.Data.Items[1].AccountID})
	require.NotNil(t, envelope.Data.Items[0].Snapshot)
	require.Nil(t, envelope.Data.Items[1].Snapshot)
	require.NotContains(t, recorder.Body.String(), "first")

	notModified := httptest.NewRecorder()
	notModifiedRequest := httptest.NewRequest(http.MethodGet, "/admin/accounts/upstream-billing-rates?page=1&page_size=20&sort_by=upstream_billing_rate&sort_order=desc", nil)
	notModifiedRequest.Header.Set("If-None-Match", etag)
	router.ServeHTTP(notModified, notModifiedRequest)
	require.Equal(t, http.StatusNotModified, notModified.Code)
}

func TestAccountHandlerGetUpstreamBillingRatesRejectsInvalidGroup(t *testing.T) {
	router := setupUpstreamBillingRatesRouter(nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/accounts/upstream-billing-rates?group=not-a-number", nil))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}
