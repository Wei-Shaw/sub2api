//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type openAIQuotaWorkflowStub struct {
	resetResult *service.OpenAIQuotaResetResult
	resetErr    error
	queryResult *service.OpenAIQuotaUsage
	queryErr    error
	cacheErr    error

	resetCalls int
	queryCalls int
	cacheCalls int

	queryCtxErr error
	cacheCtxErr error
}

func (s *openAIQuotaWorkflowStub) ResetCredit(context.Context, int64) (*service.OpenAIQuotaResetResult, error) {
	s.resetCalls++
	return s.resetResult, s.resetErr
}

func (s *openAIQuotaWorkflowStub) QueryUsage(ctx context.Context, _ int64) (*service.OpenAIQuotaUsage, error) {
	s.queryCalls++
	s.queryCtxErr = ctx.Err()
	return s.queryResult, s.queryErr
}

func (s *openAIQuotaWorkflowStub) CacheResetCreditsSnapshot(ctx context.Context, _ int64, _ *service.OpenAIRateLimitResetCredits) error {
	s.cacheCalls++
	s.cacheCtxErr = ctx.Err()
	return s.cacheErr
}

type openAIAccountStateRecovererStub struct {
	err         error
	calls       int
	accountID   int64
	lastOptions service.AccountRecoveryOptions
	lastCtxErr  error
}

func (s *openAIAccountStateRecovererStub) RecoverAccountState(ctx context.Context, accountID int64, options service.AccountRecoveryOptions) (*service.SuccessfulTestRecoveryResult, error) {
	s.calls++
	s.accountID = accountID
	s.lastOptions = options
	s.lastCtxErr = ctx.Err()
	return &service.SuccessfulTestRecoveryResult{}, s.err
}

type openAIResetAdminServiceStub struct {
	service.AdminService
	account *service.Account
	err     error
	calls   int
}

func (s *openAIResetAdminServiceStub) GetAccount(context.Context, int64) (*service.Account, error) {
	s.calls++
	return s.account, s.err
}

type openAIQuotaResetEnvelope struct {
	Code int                      `json:"code"`
	Data openAIQuotaResetResponse `json:"data"`
}

type openAIQuotaRefreshEnvelope struct {
	Code int                        `json:"code"`
	Data openAIQuotaRefreshResponse `json:"data"`
}

func performOpenAIQuotaResetRequest(t *testing.T, handler *OpenAIOAuthHandler) (int, openAIQuotaResetEnvelope) {
	t.Helper()
	return performOpenAIQuotaResetRequestWithContext(t, handler, nil)
}

// performOpenAIQuotaResetRequestWithContext drives the reset endpoint, optionally
// with an already-canceled request context (client disconnect simulation).
func performOpenAIQuotaResetRequestWithContext(t *testing.T, handler *OpenAIOAuthHandler, ctx context.Context) (int, openAIQuotaResetEnvelope) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/api/v1/admin/openai/accounts/:id/reset-quota", handler.ResetQuota)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/accounts/42/reset-quota", nil)
	if ctx != nil {
		request = request.WithContext(ctx)
	}
	router.ServeHTTP(recorder, request)

	var envelope openAIQuotaResetEnvelope
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	return recorder.Code, envelope
}

func performOpenAIQuotaRefreshRequest(t *testing.T, handler *OpenAIOAuthHandler) (int, openAIQuotaRefreshEnvelope) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/api/v1/admin/openai/accounts/:id/quota/refresh", handler.RefreshQuota)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/accounts/42/quota/refresh", nil)
	router.ServeHTTP(recorder, request)

	var envelope openAIQuotaRefreshEnvelope
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	return recorder.Code, envelope
}

func successfulOpenAIQuotaWorkflowStub() *openAIQuotaWorkflowStub {
	return &openAIQuotaWorkflowStub{
		resetResult: &service.OpenAIQuotaResetResult{
			Code:         "success",
			WindowsReset: 1,
		},
		queryResult: &service.OpenAIQuotaUsage{
			FetchedAt: 123,
			RateLimitResetCredits: &service.OpenAIRateLimitResetCredits{
				AvailableCount: 0,
				Credits:        []service.OpenAIRateLimitResetCreditDetail{},
			},
		},
	}
}

func recoveredAccountStub() *openAIResetAdminServiceStub {
	return &openAIResetAdminServiceStub{account: &service.Account{
		ID:          42,
		Name:        "recovered",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: false,
	}}
}

func TestOpenAIResetQuota_ResetFailureStopsWorkflow(t *testing.T) {
	quota := &openAIQuotaWorkflowStub{resetErr: errors.New("upstream reset failed")}
	recoverer := &openAIAccountStateRecovererStub{}
	handler := &OpenAIOAuthHandler{
		adminService:     &openAIResetAdminServiceStub{},
		quotaService:     quota,
		rateLimitService: recoverer,
	}

	status, _ := performOpenAIQuotaResetRequest(t, handler)

	require.Equal(t, http.StatusInternalServerError, status)
	require.Equal(t, 1, quota.resetCalls)
	require.Zero(t, quota.queryCalls)
	require.Zero(t, quota.cacheCalls)
	require.Zero(t, recoverer.calls)
}

// Account-state recovery is the reason the credit was spent (#3672 / #3740), so it
// must run before — and independently of — the reset-credit display cache.
func TestOpenAIResetQuota_RecoversAccountStateBeforeRefreshingCache(t *testing.T) {
	quota := successfulOpenAIQuotaWorkflowStub()
	recoverer := &openAIAccountStateRecovererStub{}
	adminService := recoveredAccountStub()
	handler := &OpenAIOAuthHandler{
		adminService:     adminService,
		quotaService:     quota,
		rateLimitService: recoverer,
	}

	status, envelope := performOpenAIQuotaResetRequest(t, handler)

	require.Equal(t, http.StatusOK, status)
	require.Empty(t, envelope.Data.WarningCode)
	require.True(t, envelope.Data.AccountStateRecovered)
	require.True(t, envelope.Data.CacheRefreshed)
	require.NotNil(t, envelope.Data.Quota)
	require.NotNil(t, envelope.Data.Account)
	require.Equal(t, int64(42), envelope.Data.Account.ID)
	require.False(t, envelope.Data.Account.Schedulable, "manual scheduling switch must not be flipped")
	require.Equal(t, int64(42), recoverer.accountID)
	require.True(t, recoverer.lastOptions.InvalidateToken)
	require.Equal(t, 1, quota.resetCalls)
	require.Equal(t, 1, quota.queryCalls)
	require.Equal(t, 1, quota.cacheCalls)
	require.Equal(t, 1, recoverer.calls)
	require.Equal(t, 1, adminService.calls)
}

func TestOpenAIResetQuota_RecoveryFailureStopsWorkflow(t *testing.T) {
	quota := successfulOpenAIQuotaWorkflowStub()
	recoverer := &openAIAccountStateRecovererStub{err: errors.New("recovery failed")}
	adminService := recoveredAccountStub()
	handler := &OpenAIOAuthHandler{
		adminService:     adminService,
		quotaService:     quota,
		rateLimitService: recoverer,
	}

	status, envelope := performOpenAIQuotaResetRequest(t, handler)

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, openAIQuotaResetWarningAccountRecoveryFailed, envelope.Data.WarningCode)
	require.False(t, envelope.Data.AccountStateRecovered)
	require.False(t, envelope.Data.CacheRefreshed)
	require.Nil(t, envelope.Data.Quota)
	require.Nil(t, envelope.Data.Account)
	require.Equal(t, 1, recoverer.calls)
	require.Zero(t, quota.queryCalls)
	require.Zero(t, quota.cacheCalls)
	require.Zero(t, adminService.calls)
}

func TestOpenAIResetQuota_MissingRecovererReportsRecoveryFailure(t *testing.T) {
	quota := successfulOpenAIQuotaWorkflowStub()
	adminService := recoveredAccountStub()
	handler := &OpenAIOAuthHandler{
		adminService: adminService,
		quotaService: quota,
	}

	status, envelope := performOpenAIQuotaResetRequest(t, handler)

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, openAIQuotaResetWarningAccountRecoveryFailed, envelope.Data.WarningCode)
	require.False(t, envelope.Data.AccountStateRecovered)
	require.Zero(t, quota.queryCalls)
	require.Zero(t, adminService.calls)
}

// A failed cache refresh must not hide the recovered account row: the operator
// still needs the list to drop the stale rate-limit badge.
func TestOpenAIResetQuota_QueryFailureStillRecoversAndReturnsAccount(t *testing.T) {
	quota := successfulOpenAIQuotaWorkflowStub()
	quota.queryResult = nil
	quota.queryErr = errors.New("upstream query failed")
	recoverer := &openAIAccountStateRecovererStub{}
	adminService := recoveredAccountStub()
	handler := &OpenAIOAuthHandler{
		adminService:     adminService,
		quotaService:     quota,
		rateLimitService: recoverer,
	}

	status, envelope := performOpenAIQuotaResetRequest(t, handler)

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, openAIQuotaResetWarningCacheRefreshFailed, envelope.Data.WarningCode)
	require.True(t, envelope.Data.AccountStateRecovered)
	require.False(t, envelope.Data.CacheRefreshed)
	require.Nil(t, envelope.Data.Quota)
	require.NotNil(t, envelope.Data.Account)
	require.Equal(t, 1, quota.queryCalls)
	require.Zero(t, quota.cacheCalls)
	require.Equal(t, 1, recoverer.calls)
	require.Equal(t, 1, adminService.calls)
}

func TestOpenAIResetQuota_CacheFailureStillRecoversAndReturnsAccount(t *testing.T) {
	quota := successfulOpenAIQuotaWorkflowStub()
	quota.cacheErr = errors.New("cache write failed")
	recoverer := &openAIAccountStateRecovererStub{}
	adminService := recoveredAccountStub()
	handler := &OpenAIOAuthHandler{
		adminService:     adminService,
		quotaService:     quota,
		rateLimitService: recoverer,
	}

	status, envelope := performOpenAIQuotaResetRequest(t, handler)

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, openAIQuotaResetWarningCacheRefreshFailed, envelope.Data.WarningCode)
	require.True(t, envelope.Data.AccountStateRecovered)
	require.False(t, envelope.Data.CacheRefreshed)
	require.Nil(t, envelope.Data.Quota)
	require.NotNil(t, envelope.Data.Account)
	require.Equal(t, 1, quota.cacheCalls)
	require.Equal(t, 1, adminService.calls)
}

func TestOpenAIResetQuota_AccountRefreshFailureReportsRecoveredState(t *testing.T) {
	quota := successfulOpenAIQuotaWorkflowStub()
	recoverer := &openAIAccountStateRecovererStub{}
	adminService := &openAIResetAdminServiceStub{err: errors.New("account refresh failed")}
	handler := &OpenAIOAuthHandler{
		adminService:     adminService,
		quotaService:     quota,
		rateLimitService: recoverer,
	}

	status, envelope := performOpenAIQuotaResetRequest(t, handler)

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, openAIQuotaResetWarningAccountRefreshFailed, envelope.Data.WarningCode)
	require.True(t, envelope.Data.CacheRefreshed)
	require.True(t, envelope.Data.AccountStateRecovered)
	require.NotNil(t, envelope.Data.Quota)
	require.Nil(t, envelope.Data.Account)
	require.Equal(t, 1, adminService.calls)
}

// The first (most actionable) failure wins so the UI never downgrades a cache
// problem into a cosmetic "could not reload the row" message.
func TestOpenAIResetQuota_CacheAndAccountFailureKeepsFirstWarning(t *testing.T) {
	quota := successfulOpenAIQuotaWorkflowStub()
	quota.cacheErr = errors.New("cache write failed")
	recoverer := &openAIAccountStateRecovererStub{}
	adminService := &openAIResetAdminServiceStub{err: errors.New("account refresh failed")}
	handler := &OpenAIOAuthHandler{
		adminService:     adminService,
		quotaService:     quota,
		rateLimitService: recoverer,
	}

	status, envelope := performOpenAIQuotaResetRequest(t, handler)

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, openAIQuotaResetWarningCacheRefreshFailed, envelope.Data.WarningCode)
	require.True(t, envelope.Data.AccountStateRecovered)
	require.Nil(t, envelope.Data.Account)
}

// The credit is non-refundable once consumed, so post-reset bookkeeping must
// survive a client disconnect instead of leaving the account rate-limited.
func TestOpenAIResetQuota_PostProcessingSurvivesClientCancellation(t *testing.T) {
	quota := successfulOpenAIQuotaWorkflowStub()
	recoverer := &openAIAccountStateRecovererStub{}
	adminService := recoveredAccountStub()
	handler := &OpenAIOAuthHandler{
		adminService:     adminService,
		quotaService:     quota,
		rateLimitService: recoverer,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	status, _ := performOpenAIQuotaResetRequestWithContext(t, handler, ctx)

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, 1, recoverer.calls)
	require.NoError(t, recoverer.lastCtxErr, "recovery must not inherit the canceled client context")
	require.NoError(t, quota.queryCtxErr)
	require.NoError(t, quota.cacheCtxErr)
	require.Equal(t, 1, adminService.calls)
}

func TestOpenAIRefreshQuota_PersistsSnapshot(t *testing.T) {
	quota := successfulOpenAIQuotaWorkflowStub()
	handler := &OpenAIOAuthHandler{
		adminService: &openAIResetAdminServiceStub{},
		quotaService: quota,
	}

	status, envelope := performOpenAIQuotaRefreshRequest(t, handler)

	require.Equal(t, http.StatusOK, status)
	require.True(t, envelope.Data.CachePersisted)
	require.Equal(t, int64(123), envelope.Data.FetchedAt)
	require.Equal(t, 1, quota.queryCalls)
	require.Equal(t, 1, quota.cacheCalls)
	require.Zero(t, quota.resetCalls)
}

// A rejected snapshot write must never discard the usage payload: otherwise the
// card loses its credit count and the reset button stays disabled forever.
func TestOpenAIRefreshQuota_PersistFailureStillReturnsUsage(t *testing.T) {
	quota := successfulOpenAIQuotaWorkflowStub()
	quota.queryResult = &service.OpenAIQuotaUsage{
		FetchedAt: 456,
		RateLimitResetCredits: &service.OpenAIRateLimitResetCredits{
			AvailableCount: 2,
		},
	}
	quota.cacheErr = errors.New("expiration details unavailable")
	handler := &OpenAIOAuthHandler{
		adminService: &openAIResetAdminServiceStub{},
		quotaService: quota,
	}

	status, envelope := performOpenAIQuotaRefreshRequest(t, handler)

	require.Equal(t, http.StatusOK, status)
	require.False(t, envelope.Data.CachePersisted)
	require.Equal(t, int64(456), envelope.Data.FetchedAt)
	require.NotNil(t, envelope.Data.RateLimitResetCredits)
	require.Equal(t, 2, envelope.Data.RateLimitResetCredits.AvailableCount)
	require.Equal(t, 1, quota.cacheCalls)
}

// An empty-but-successful upstream read must not be dereferenced blindly.
func TestOpenAIQuotaEmptyUsageIsHandledWithoutPanic(t *testing.T) {
	t.Run("refresh reports an internal error", func(t *testing.T) {
		quota := successfulOpenAIQuotaWorkflowStub()
		quota.queryResult = nil
		handler := &OpenAIOAuthHandler{
			adminService: &openAIResetAdminServiceStub{},
			quotaService: quota,
		}

		status, envelope := performOpenAIQuotaRefreshRequest(t, handler)

		require.Equal(t, http.StatusInternalServerError, status)
		require.False(t, envelope.Data.CachePersisted)
		require.Zero(t, quota.cacheCalls)
	})

	t.Run("reset degrades to a cache warning", func(t *testing.T) {
		quota := successfulOpenAIQuotaWorkflowStub()
		quota.queryResult = nil
		recoverer := &openAIAccountStateRecovererStub{}
		adminService := recoveredAccountStub()
		handler := &OpenAIOAuthHandler{
			adminService:     adminService,
			quotaService:     quota,
			rateLimitService: recoverer,
		}

		status, envelope := performOpenAIQuotaResetRequest(t, handler)

		require.Equal(t, http.StatusOK, status)
		require.Equal(t, openAIQuotaResetWarningCacheRefreshFailed, envelope.Data.WarningCode)
		require.True(t, envelope.Data.AccountStateRecovered)
		require.NotNil(t, envelope.Data.Account)
		require.Zero(t, quota.cacheCalls)
	})
}

func TestOpenAIRefreshQuota_QueryFailureIsReported(t *testing.T) {
	quota := &openAIQuotaWorkflowStub{queryErr: errors.New("upstream query failed")}
	handler := &OpenAIOAuthHandler{
		adminService: &openAIResetAdminServiceStub{},
		quotaService: quota,
	}

	status, _ := performOpenAIQuotaRefreshRequest(t, handler)

	require.Equal(t, http.StatusInternalServerError, status)
	require.Equal(t, 1, quota.queryCalls)
	require.Zero(t, quota.cacheCalls)
}

// Storing a nil *Service in an interface field would make the capability guards
// non-nil and panic on the first call; the constructor must keep them nil.
func TestNewOpenAIOAuthHandlerKeepsNilQuotaCapabilitiesGuarded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewOpenAIOAuthHandler(nil, newStubAdminService(), nil, nil)
	require.Nil(t, handler.quotaService)
	require.Nil(t, handler.rateLimitService)

	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/openai/accounts/:id/quota", handler.QueryQuota)
	router.POST("/openai/accounts/:id/quota/refresh", handler.RefreshQuota)
	router.POST("/openai/accounts/:id/reset-quota", handler.ResetQuota)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/openai/accounts/42/quota"},
		{http.MethodPost, "/openai/accounts/42/quota/refresh"},
		{http.MethodPost, "/openai/accounts/42/reset-quota"},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.path, nil))
		require.Equal(t, http.StatusBadRequest, recorder.Code, "%s %s", tc.method, tc.path)
	}
}
