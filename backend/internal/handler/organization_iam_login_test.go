//go:build unit

package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type iamLoginSettingRepositoryStub struct {
	service.SettingRepository
	values map[string]string
}

func (s *iamLoginSettingRepositoryStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		values[key] = s.values[key]
	}
	return values, nil
}

func (s *iamLoginSettingRepositoryStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

type iamLoginCaptchaVerifierSpy struct {
	called int
	token  string
}

func (s *iamLoginCaptchaVerifierSpy) VerifyToken(_ context.Context, _, token, _ string) (*service.TurnstileVerifyResponse, error) {
	s.called++
	s.token = token
	return &service.TurnstileVerifyResponse{Success: true}, nil
}

type iamLoginOrganizationRepositoryStub struct {
	service.OrganizationRepository
	findCalled int
}

func (s *iamLoginOrganizationRepositoryStub) FindIAMByPrincipal(_ context.Context, _, _ string) (*service.User, *service.OrganizationContext, error) {
	s.findCalled++
	return nil, nil, service.ErrInvalidCredentials
}

func newIAMLoginHandlerForCaptchaTest(t *testing.T) (*OrganizationHandler, *iamLoginCaptchaVerifierSpy, *iamLoginOrganizationRepositoryStub) {
	t.Helper()
	cfg := &config.Config{
		Server:    config.ServerConfig{Mode: "release"},
		Turnstile: config.TurnstileConfig{Required: true},
		Company:   config.CompanyConfig{IAMEnabled: true},
	}
	settings := service.NewSettingService(&iamLoginSettingRepositoryStub{values: map[string]string{
		service.SettingKeyTurnstileEnabled:   "true",
		service.SettingKeyTurnstileSiteKey:   "site-key",
		service.SettingKeyTurnstileSecretKey: "secret-key",
	}}, cfg)
	verifier := &iamLoginCaptchaVerifierSpy{}
	authService := service.NewAuthService(
		nil, nil, nil, nil, cfg, settings, nil,
		service.NewTurnstileService(settings, verifier),
		nil, nil, nil, nil, nil,
	)
	organizationRepo := &iamLoginOrganizationRepositoryStub{}
	organizationService := service.NewOrganizationService(organizationRepo, nil, cfg)
	return NewOrganizationHandler(organizationService, authService, nil, nil), verifier, organizationRepo
}

func performIAMLogin(handler *OrganizationHandler, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/auth/iam/login", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	handler.IAMLogin(context)
	return response
}

func TestIAMLoginRequiresCaptchaBeforeAccountAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, verifier, organizationRepo := newIAMLoginHandlerForCaptchaTest(t)

	response := performIAMLogin(handler, `{"principal":"reader@c123456789012345.opentk.ai","password":"secret-password"}`)

	require.NotEqual(t, http.StatusOK, response.Code)
	require.Zero(t, verifier.called)
	require.Zero(t, organizationRepo.findCalled)
}

func TestIAMLoginPassesCaptchaPayloadBeforeAccountAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, verifier, organizationRepo := newIAMLoginHandlerForCaptchaTest(t)

	response := performIAMLogin(handler, `{"principal":"reader@c123456789012345.opentk.ai","password":"secret-password","turnstile_token":"captcha-token"}`)

	require.NotEqual(t, http.StatusOK, response.Code)
	require.Equal(t, 1, verifier.called)
	require.Equal(t, "captcha-token", verifier.token)
	require.Equal(t, 1, organizationRepo.findCalled)
}
