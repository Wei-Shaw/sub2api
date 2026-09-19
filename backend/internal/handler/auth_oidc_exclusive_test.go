//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOIDCExclusiveModeRejectsPasswordLogin(t *testing.T) {
	handler := &AuthHandler{cfg: &config.Config{OIDC: config.OIDCConnectConfig{
		Enabled:   true,
		Exclusive: true,
	}}}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"user@example.com","password":"secret-123"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.Login(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "OIDC login is required")
}

func TestOIDCExclusiveModeRejectsLocalRegistration(t *testing.T) {
	handler := &AuthHandler{cfg: &config.Config{OIDC: config.OIDCConnectConfig{
		Enabled:   true,
		Exclusive: true,
	}}}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"user@example.com","password":"secret-123"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.Register(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "OIDC login is required")
}

func TestOIDCExclusiveModeAllowsPasswordLoginWhenOIDCIsDisabledAtRuntime(t *testing.T) {
	cfg := &config.Config{OIDC: config.OIDCConnectConfig{
		Enabled:   true,
		Exclusive: true,
	}}
	settings := service.NewSettingService(&settingHandlerPublicRepoStub{values: map[string]string{
		service.SettingKeyOIDCConnectEnabled: "false",
	}}, cfg)
	handler := &AuthHandler{cfg: cfg, settingSvc: settings}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.Login(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "OIDC login is required")
}
