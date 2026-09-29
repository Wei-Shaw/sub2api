//go:build unit

package handler

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// b05LoginSessionCacheStub 登录会话一律不存在，模拟 temp_token 失效。
type b05LoginSessionCacheStub struct {
	service.TotpCache
}

func (b05LoginSessionCacheStub) GetLoginSession(ctx context.Context, tempToken string) (*service.TotpLoginSession, error) {
	return nil, nil
}

func TestLogin2FASessionInvalidDebugLogDoesNotLeakTempTokenB05(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	totpSvc := service.NewTotpService(nil, nil, b05LoginSessionCacheStub{}, nil, nil, nil)
	h := &AuthHandler{totpService: totpSvc}

	const tempToken = "b05tokenABCDEFGHIJKLMNOPQRSTUVWXYZ"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/2fa",
		strings.NewReader(`{"temp_token":"`+tempToken+`","totp_code":"123456"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Login2FA(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	out := buf.String()
	require.Contains(t, out, "login_2fa_session_invalid")
	require.NotContains(t, out, tempToken[:8])
	require.NotContains(t, out, "temp_token_prefix")
}
