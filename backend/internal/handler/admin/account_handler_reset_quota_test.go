package admin

import (
	"context"
	"encoding/json"
	"errors"
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

type accountHandlerRateLimitRepoStub struct {
	service.AccountRepository
	accounts  map[int64]*service.Account
	getErr    map[int64]error
	clearErrs []int64
}

func (s *accountHandlerRateLimitRepoStub) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if err, ok := s.getErr[id]; ok {
		return nil, err
	}
	if account, ok := s.accounts[id]; ok {
		return account, nil
	}
	return nil, service.ErrAccountNotFound
}

func (s *accountHandlerRateLimitRepoStub) ClearError(_ context.Context, id int64) error {
	s.clearErrs = append(s.clearErrs, id)
	if account, ok := s.accounts[id]; ok {
		account.Status = service.StatusActive
		account.ErrorMessage = ""
	}
	return nil
}

func newAccountHandlerForResetQuotaTest(adminSvc service.AdminService, rateLimitSvc *service.RateLimitService) *AccountHandler {
	return NewAccountHandler(adminSvc, nil, nil, nil, nil, rateLimitSvc, nil, nil, nil, nil, nil, nil, nil)
}

type accountHandlerTestHTTPUpstream struct {
	requestCount int
}

func (s *accountHandlerTestHTTPUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return nil, errors.New("unexpected Do call")
}

func (s *accountHandlerTestHTTPUpstream) DoWithTLS(_ *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	s.requestCount++
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{}}\n\n" +
				"data: [DONE]\n\n",
		)),
	}, nil
}

func TestAccountHandler_ResetQuota_RecoversAccountStateAndReturnsUpdatedAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	adminSvc.accounts = []service.Account{{ID: 42, Name: "before", Status: service.StatusError}}
	adminSvc.getAccountByID = map[int64]*service.Account{
		42: {ID: 42, Name: "after", Status: service.StatusActive},
	}

	repo := &accountHandlerRateLimitRepoStub{
		accounts: map[int64]*service.Account{
			42: {
				ID:           42,
				Name:         "before",
				Status:       service.StatusError,
				ErrorMessage: "401",
			},
		},
	}
	rateLimitSvc := service.NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	handler := newAccountHandlerForResetQuotaTest(adminSvc, rateLimitSvc)

	router := gin.New()
	router.POST("/api/v1/admin/accounts/:id/reset-quota", handler.ResetQuota)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/reset-quota", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []int64{42}, adminSvc.resetAccountQuotaIDs)
	require.Equal(t, []int64{42}, repo.clearErrs)

	var payload struct {
		Code int `json:"code"`
		Data struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, 0, payload.Code)
	require.Equal(t, int64(42), payload.Data.ID)
	require.Equal(t, "after", payload.Data.Name)
	require.Equal(t, service.StatusActive, payload.Data.Status)
}

func TestAccountHandler_ResetQuota_OpenAIAccountTriggersAutoTest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	adminSvc.getAccountByID = map[int64]*service.Account{
		42: {ID: 42, Name: "openai", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive},
	}

	repo := &accountHandlerRateLimitRepoStub{
		accounts: map[int64]*service.Account{
			42: {
				ID:           42,
				Name:         "openai",
				Platform:     service.PlatformOpenAI,
				Type:         service.AccountTypeOAuth,
				Status:       service.StatusActive,
				Concurrency:  1,
				Schedulable:  true,
				Credentials:  map[string]any{"access_token": "token"},
				Extra:        map[string]any{},
				ErrorMessage: "",
			},
		},
	}
	rateLimitSvc := service.NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	upstream := &accountHandlerTestHTTPUpstream{}
	accountTestSvc := service.NewAccountTestService(repo, nil, nil, upstream, &config.Config{}, &service.TLSFingerprintProfileService{})

	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, rateLimitSvc, nil, accountTestSvc, nil, nil, nil, nil, nil)

	router := gin.New()
	router.POST("/api/v1/admin/accounts/:id/reset-quota", handler.ResetQuota)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/reset-quota", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, upstream.requestCount)
}

func TestAccountHandler_BatchTest_ReturnsSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	adminSvc.getAccountByID = map[int64]*service.Account{
		42: {ID: 42, Name: "openai-a", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive},
		43: {ID: 43, Name: "openai-b", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive},
	}

	repo := &accountHandlerRateLimitRepoStub{
		accounts: map[int64]*service.Account{
			42: {
				ID:          42,
				Name:        "openai-a",
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeOAuth,
				Status:      service.StatusActive,
				Concurrency: 1,
				Schedulable: true,
				Credentials: map[string]any{"access_token": "token-a"},
				Extra:       map[string]any{},
			},
			43: {
				ID:          43,
				Name:        "openai-b",
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeOAuth,
				Status:      service.StatusActive,
				Concurrency: 1,
				Schedulable: true,
				Credentials: map[string]any{"access_token": "token-b"},
				Extra:       map[string]any{},
			},
		},
	}
	rateLimitSvc := service.NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	upstream := &accountHandlerTestHTTPUpstream{}
	accountTestSvc := service.NewAccountTestService(repo, nil, nil, upstream, &config.Config{}, &service.TLSFingerprintProfileService{})

	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, rateLimitSvc, nil, accountTestSvc, nil, nil, nil, nil, nil)

	router := gin.New()
	router.POST("/api/v1/admin/accounts/batch-test", handler.BatchTest)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/batch-test", strings.NewReader(`{"account_ids":[42,43]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 2, upstream.requestCount)

	var payload struct {
		Code int `json:"code"`
		Data struct {
			Total   int `json:"total"`
			Success int `json:"success"`
			Failed  int `json:"failed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, 0, payload.Code)
	require.Equal(t, 2, payload.Data.Total)
	require.Equal(t, 2, payload.Data.Success)
	require.Equal(t, 0, payload.Data.Failed)
}
