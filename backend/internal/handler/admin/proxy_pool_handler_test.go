//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type proxyPoolRebindAdminService struct {
	*stubAdminService
	rebound int64
	err     error
}

func (s *proxyPoolRebindAdminService) RunProxyPoolRebind(context.Context, int64) (int64, error) {
	return s.rebound, s.err
}

type proxyPoolCreateAdminService struct {
	*stubAdminService
	input *service.CreateProxyPoolInput
}

func (s *proxyPoolCreateAdminService) CreateProxyPool(_ context.Context, input *service.CreateProxyPoolInput) (*service.ProxyPool, error) {
	s.input = input
	return &service.ProxyPool{Name: input.Name, AutoRebind: input.AutoRebind != nil && *input.AutoRebind}, nil
}

type proxyPoolAccountsAdminService struct {
	*stubAdminService
	poolID   int64
	page     int
	pageSize int
}

func (s *proxyPoolAccountsAdminService) GetProxyPoolAccounts(_ context.Context, poolID int64, page, pageSize int) ([]service.ProxyPoolAccountSummary, int64, error) {
	s.poolID = poolID
	s.page = page
	s.pageSize = pageSize
	proxyID := int64(11)
	return []service.ProxyPoolAccountSummary{{
		ID: 21, Name: "account-21", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Status: service.StatusActive, ProxyID: &proxyID, ProxyName: "proxy-11",
	}}, 12, nil
}

func TestProxyPoolHandlerListsAssignedAccountsWithPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminService := &proxyPoolAccountsAdminService{stubAdminService: &stubAdminService{}}
	handler := NewProxyPoolHandler(adminService)
	router := gin.New()
	router.GET("/proxy-pools/:id/accounts", handler.GetAccounts)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/proxy-pools/7/accounts?page=2&page_size=5", nil)

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(7), adminService.poolID)
	require.Equal(t, 2, adminService.page)
	require.Equal(t, 5, adminService.pageSize)
	var payload struct {
		Data struct {
			Items []dto.AdminProxyPoolAccountSummary `json:"items"`
			Total int64                              `json:"total"`
			Pages int                                `json:"pages"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, int64(12), payload.Data.Total)
	require.Equal(t, 3, payload.Data.Pages)
	require.Len(t, payload.Data.Items, 1)
	require.Equal(t, "proxy-11", payload.Data.Items[0].ProxyName)
}

func TestProxyPoolHandlerCreatePreservesExplicitAutoRebindFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminService := &proxyPoolCreateAdminService{stubAdminService: &stubAdminService{}}
	handler := NewProxyPoolHandler(adminService)
	router := gin.New()
	router.POST("/proxy-pools", handler.Create)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/proxy-pools", strings.NewReader(`{"name":"manual-pool","auto_rebind":false}`))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusCreated, recorder.Code)
	require.NotNil(t, adminService.input)
	require.NotNil(t, adminService.input.AutoRebind)
	require.False(t, *adminService.input.AutoRebind)
}

func TestProxyPoolHandlerRebindReturnsPartialResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runErr := &service.ProxyPoolRunError{
		FailedProxies: 1,
		Err:           errors.New("database unavailable"),
	}
	adminService := &proxyPoolRebindAdminService{
		stubAdminService: &stubAdminService{},
		rebound:          4,
		err:              runErr,
	}
	handler := NewProxyPoolHandler(adminService)
	router := gin.New()
	router.POST("/proxy-pools/:id/rebind", handler.Rebind)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/proxy-pools/7/rebind", nil)

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{
		"code": 0,
		"message": "success",
		"data": {
			"rebound_accounts": 4,
			"partial_failure": true,
			"failed_proxies": 1
		}
	}`, recorder.Body.String())
}

func TestProxyPoolHandlerRebindReturnsConflictWhenSweepIsRunning(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminService := &proxyPoolRebindAdminService{
		stubAdminService: &stubAdminService{},
		err:              service.ErrProxyPoolRunInProgress,
	}
	handler := NewProxyPoolHandler(adminService)
	router := gin.New()
	router.POST("/proxy-pools/:id/rebind", handler.Rebind)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/proxy-pools/7/rebind", nil)

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), "PROXY_POOL_RUN_IN_PROGRESS")
}
