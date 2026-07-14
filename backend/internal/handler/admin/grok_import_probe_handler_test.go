//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokImportAdminService struct {
	*stubAdminService
	mu     sync.Mutex
	nextID int64
}

func newGrokImportAdminService() *grokImportAdminService {
	return &grokImportAdminService{
		stubAdminService: newStubAdminService(),
		nextID:           500,
	}
}

func (s *grokImportAdminService) CreateAccount(_ context.Context, input *service.CreateAccountInput) (*service.Account, error) {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()
	return &service.Account{
		ID:          id,
		Name:        input.Name,
		Platform:    input.Platform,
		Type:        input.Type,
		Credentials: input.Credentials,
		Extra:       input.Extra,
		ProxyID:     input.ProxyID,
		Concurrency: input.Concurrency,
		Status:      service.StatusActive,
		Schedulable: true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

type grokImportOAuthClientStub struct{}

func (grokImportOAuthClientStub) ExchangeCode(context.Context, string, string, string, string, string) (*xai.TokenResponse, error) {
	return &xai.TokenResponse{AccessToken: "access-token", RefreshToken: "refresh-token", ExpiresIn: 3600}, nil
}

func (grokImportOAuthClientStub) RefreshToken(context.Context, string, string, string) (*xai.TokenResponse, error) {
	return &xai.TokenResponse{AccessToken: "access-token", RefreshToken: "refresh-token", ExpiresIn: 3600}, nil
}

func (grokImportOAuthClientStub) ConvertSSOToBuild(context.Context, string, string) (*xai.TokenResponse, error) {
	return &xai.TokenResponse{AccessToken: "access-token", RefreshToken: "refresh-token", ExpiresIn: 3600}, nil
}

func TestGrokSSOBatchImportKeepsCreatedAccountsWhenOneAutomaticProbeFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminService := newGrokImportAdminService()
	oauthService := service.NewGrokOAuthService(nil, grokImportOAuthClientStub{})
	defer oauthService.Stop()
	prober := newGrokImportProbeStub(3)
	prober.failures[502] = infraerrors.New(502, "GROK_TEST_PROBE_FAILED", "sensitive-upstream-body")
	handler := NewGrokOAuthHandler(oauthService, adminService, nil)
	handler.importProber = prober

	router := gin.New()
	router.POST("/api/v1/admin/grok/sso-to-oauth", handler.CreateAccountsFromSSO)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/grok/sso-to-oauth",
		strings.NewReader(`{"sso_tokens":["sso-one","sso-two","sso-three"]}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"created"`)
	require.NotContains(t, recorder.Body.String(), `GROK_TEST_PROBE_FAILED`)
	for i := 0; i < 3; i++ {
		awaitGrokProbeSignal(t, prober.done)
	}
	calls, _, _ := prober.snapshot()
	require.Equal(t, map[int64]int{501: 1, 502: 1, 503: 1}, calls)
}

func TestAccountCreateWithoutAutomaticGrokProbeServiceStillSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAccountHandler(
		newGrokImportAdminService(),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)

	router := gin.New()
	router.POST("/api/v1/admin/accounts", handler.Create)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/accounts",
		strings.NewReader(`{"name":"grok-rt","platform":"grok","type":"oauth","credentials":{"refresh_token":"secret"}}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
}
