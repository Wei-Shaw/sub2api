package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupUpstreamBillingProbeRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	handler := NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.SetUpstreamBillingProbeService(service.NewUpstreamBillingProbeService(nil, nil, nil))

	router := gin.New()
	router.GET("/admin/accounts/upstream-billing-probe/settings", handler.GetUpstreamBillingProbeSettings)
	router.POST("/admin/accounts/upstream-billing-probe/batch", handler.ProbeUpstreamBillingBatch)
	router.PUT("/admin/accounts/:id/upstream-billing-probe", handler.SetUpstreamBillingProbeEnabled)
	router.POST("/admin/accounts/:id/upstream-quota/query", handler.QueryUpstreamQuota)
	return router
}

type upstreamQuotaHandlerRepo struct {
	service.AccountRepository
	account *service.Account
	logo    *service.UpstreamSiteLogo
	logoKey string
}

func (r *upstreamQuotaHandlerRepo) GetByID(context.Context, int64) (*service.Account, error) {
	if r.account == nil {
		return nil, service.ErrAccountNotFound
	}
	return r.account, nil
}

func (r *upstreamQuotaHandlerRepo) GetUpstreamSiteLogoCache(_ context.Context, key string) (*service.UpstreamSiteLogo, bool, error) {
	if key != r.logoKey || r.logo == nil {
		return nil, false, nil
	}
	return r.logo, true, nil
}

func (r *upstreamQuotaHandlerRepo) PutUpstreamSiteLogoCache(context.Context, string, *service.UpstreamSiteLogo) error {
	return nil
}

type upstreamQuotaHandlerHTTP struct {
	body string
}

func (u *upstreamQuotaHandlerHTTP) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, concurrency, nil)
}

func (u *upstreamQuotaHandlerHTTP) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(u.body))}, nil
}

func setupUpstreamQuotaHandlerRouter(body string) *gin.Engine {
	account := &service.Account{
		ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-handler-secret", "base_url": "https://upstream.example"},
	}
	repo := &upstreamQuotaHandlerRepo{account: account}
	cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}}
	accountTest := service.NewAccountTestService(repo, nil, nil, nil, nil, &upstreamQuotaHandlerHTTP{body: body}, cfg, nil)
	handler := NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.SetUpstreamBillingProbeService(service.NewUpstreamBillingProbeService(repo, accountTest, nil))
	router := gin.New()
	router.POST("/admin/accounts/:id/upstream-quota/query", handler.QueryUpstreamQuota)
	return router
}

func TestAccountHandlerQueryUpstreamQuotaSuccess(t *testing.T) {
	router := setupUpstreamQuotaHandlerRouter(`{"mode":"quota_limited","isValid":true,"status":"active","quota":{"limit":100,"used":25,"remaining":75,"unit":"USD"},"remaining":75,"unit":"USD"}`)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/admin/accounts/7/upstream-quota/query", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope struct {
		Data service.UpstreamQuotaQueryResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, int64(7), envelope.Data.AccountID)
	require.NotNil(t, envelope.Data.Quota)
	require.Equal(t, "quota", envelope.Data.Quota.Mode)
}

func TestAccountHandlerQueryUpstreamQuotaReturnsSafeTypedError(t *testing.T) {
	router := setupUpstreamQuotaHandlerRouter(`{"api_key":"sk-body-secret","unexpected":true}`)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/admin/accounts/7/upstream-quota/query", nil))

	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Contains(t, recorder.Body.String(), "UPSTREAM_QUOTA_INVALID_RESPONSE")
	require.NotContains(t, recorder.Body.String(), "sk-handler-secret")
	require.NotContains(t, recorder.Body.String(), "sk-body-secret")
}

func TestAccountHandlerQueryUpstreamQuotaRejectsInvalidID(t *testing.T) {
	router := setupUpstreamBillingProbeRouter()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/admin/accounts/not-an-id/upstream-quota/query", nil))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestAccountHandlerServesImmutableUpstreamSiteLogo(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	repo := &upstreamQuotaHandlerRepo{
		logoKey: key,
		logo:    &service.UpstreamSiteLogo{ContentType: "image/png", Data: []byte("\x89PNG\r\n\x1a\ncustom")},
	}
	handler := NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.SetUpstreamBillingProbeService(service.NewUpstreamBillingProbeService(repo, nil, nil))
	router := gin.New()
	router.GET("/admin/accounts/upstream-site-logos/:key", handler.GetUpstreamSiteLogo)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/accounts/upstream-site-logos/"+key, nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
	require.Equal(t, "private, max-age=31536000, immutable", recorder.Header().Get("Cache-Control"))
	require.Equal(t, `"`+key+`"`, recorder.Header().Get("ETag"))

	notModified := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/accounts/upstream-site-logos/"+key, nil)
	request.Header.Set("If-None-Match", `"`+key+`"`)
	router.ServeHTTP(notModified, request)
	require.Equal(t, http.StatusNotModified, notModified.Code)
}

func TestAccountHandlerGetUpstreamBillingProbeSettingsReturnsDefaults(t *testing.T) {
	router := setupUpstreamBillingProbeRouter()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/accounts/upstream-billing-probe/settings", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data service.UpstreamBillingProbeSettings `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Data.Enabled)
	require.Equal(t, 30, response.Data.IntervalMinutes)
}

func TestAccountHandlerProbeUpstreamBillingBatchValidatesIDs(t *testing.T) {
	router := setupUpstreamBillingProbeRouter()

	for _, body := range []string{`{"account_ids":[]}`, `{"account_ids":[0]}`} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/admin/accounts/upstream-billing-probe/batch", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
	}
}

func TestAccountHandlerSetUpstreamBillingProbeEnabledRejectsInvalidID(t *testing.T) {
	router := setupUpstreamBillingProbeRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/admin/accounts/not-an-id/upstream-billing-probe", bytes.NewBufferString(`{"enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestAccountHandlerSetUpstreamBillingProbeEnabledRequiresValue(t *testing.T) {
	router := setupUpstreamBillingProbeRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/admin/accounts/1/upstream-billing-probe", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}
