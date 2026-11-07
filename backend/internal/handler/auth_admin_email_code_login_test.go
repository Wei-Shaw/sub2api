package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
)

const (
	adminEmailCodeTestPassword   = "secret-123"
	adminEmailCodeTestTotpSecret = "JBSWY3DPEHPK3PXP"
	adminLoginCodeKeyPrefix      = "admin_login:"
)

type adminEmailCodeLoginTestOptions struct {
	featureEnabled bool
	smtpConfigured bool
	role           string
	withTotp       bool
}

type adminEmailCodeLoginTestEnv struct {
	handler    *AuthHandler
	emailCache *oauthPendingFlowEmailCacheStub
	totpCache  *oauthPendingFlowTotpCacheStub
	user       *service.User
}

func newAdminEmailCodeLoginTestEnv(t *testing.T, opts adminEmailCodeLoginTestOptions) *adminEmailCodeLoginTestEnv {
	t.Helper()
	emailCache := &oauthPendingFlowEmailCacheStub{}
	totpCache := &oauthPendingFlowTotpCacheStub{}
	settingValues := map[string]string{
		service.SettingKeyTotpEnabled: "true",
	}
	if opts.featureEnabled {
		settingValues[service.SettingKeyAdminLoginEmailCodeEnabled] = "true"
	}
	emailSettingValues := map[string]string{}
	if opts.smtpConfigured {
		emailSettingValues[service.SettingKeySMTPHost] = "smtp.example.com"
	}
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		emailCache:         emailCache,
		emailSettingValues: emailSettingValues,
		settingValues:      settingValues,
		totpCache:          totpCache,
		totpEncryptor:      oauthPendingFlowTotpEncryptorStub{},
	})

	ctx := context.Background()
	passwordHash, err := handler.authService.HashPassword(adminEmailCodeTestPassword)
	require.NoError(t, err)
	role := opts.role
	if role == "" {
		role = service.RoleAdmin
	}
	create := client.User.Create().
		SetEmail("admin-email-code@example.com").
		SetUsername("admin-email-code").
		SetPasswordHash(passwordHash).
		SetRole(role).
		SetStatus(service.StatusActive)
	if opts.withTotp {
		create = create.
			SetTotpEnabled(true).
			SetTotpSecretEncrypted(adminEmailCodeTestTotpSecret).
			SetTotpEnabledAt(time.Now().UTC().Add(-time.Hour))
	}
	created, err := create.Save(ctx)
	require.NoError(t, err)
	user, err := handler.userService.GetByID(ctx, created.ID)
	require.NoError(t, err)

	return &adminEmailCodeLoginTestEnv{
		handler:    handler,
		emailCache: emailCache,
		totpCache:  totpCache,
		user:       user,
	}
}

// seedLoginCode stores an admin login code as if it had just been emailed.
// Its fresh CreatedAt also keeps the handler from contacting a real SMTP server
// (the send is skipped because of the resend cooldown).
func (e *adminEmailCodeLoginTestEnv) seedLoginCode(t *testing.T, code string) {
	t.Helper()
	require.NoError(t, e.emailCache.SetVerificationCode(context.Background(), adminLoginCodeKeyPrefix+e.user.Email, &service.VerificationCodeData{
		Code:      code,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}, 15*time.Minute))
}

func (e *adminEmailCodeLoginTestEnv) login(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	return postAuthJSON(t, "/api/v1/auth/login", `{"email":"`+e.user.Email+`","password":"`+adminEmailCodeTestPassword+`"}`, e.handler.Login)
}

func postAuthJSON(t *testing.T, path, body string, fn func(*gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	ginCtx.Request = req
	fn(ginCtx)
	return recorder
}

func loginEmailCodeBody(tempToken, code string) string {
	return `{"temp_token":"` + tempToken + `","code":"` + code + `"}`
}

func TestAdminLoginEmailCodeDisabledKeepsDefaultFlow(t *testing.T) {
	env := newAdminEmailCodeLoginTestEnv(t, adminEmailCodeLoginTestOptions{smtpConfigured: true})

	recorder := env.login(t)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := decodeJSONResponseData(t, recorder)
	require.NotEmpty(t, payload["access_token"])
	require.Nil(t, payload["requires_email_code"])
	require.Nil(t, payload["requires_2fa"])
	require.Empty(t, env.emailCache.verificationCodes)
}

func TestAdminLoginEmailCodeDisabledKeepsTotpFlow(t *testing.T) {
	env := newAdminEmailCodeLoginTestEnv(t, adminEmailCodeLoginTestOptions{smtpConfigured: true, withTotp: true})

	recorder := env.login(t)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := decodeJSONResponseData(t, recorder)
	require.Equal(t, true, payload["requires_2fa"])
	require.Nil(t, payload["email_code_available"])
	tempToken := payload["temp_token"].(string)

	session, err := env.totpCache.GetLoginSession(context.Background(), tempToken)
	require.NoError(t, err)
	require.False(t, session.EmailCode)

	// A plain TOTP session cannot be completed with an email code.
	env.seedLoginCode(t, "123456")
	recorder = postAuthJSON(t, "/api/v1/auth/login/email-code", loginEmailCodeBody(tempToken, "123456"), env.handler.LoginEmailCode)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	recorder = postAuthJSON(t, "/api/v1/auth/login/email-code/resend", `{"temp_token":"`+tempToken+`"}`, env.handler.LoginEmailCodeResend)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestAdminLoginEmailCodeRequiredWithoutTotp(t *testing.T) {
	env := newAdminEmailCodeLoginTestEnv(t, adminEmailCodeLoginTestOptions{featureEnabled: true, smtpConfigured: true})
	ctx := context.Background()
	env.seedLoginCode(t, "123456")

	recorder := env.login(t)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := decodeJSONResponseData(t, recorder)
	require.Equal(t, true, payload["requires_email_code"])
	require.Nil(t, payload["access_token"])
	require.Equal(t, service.MaskEmail(env.user.Email), payload["user_email_masked"])
	tempToken := payload["temp_token"].(string)
	require.NotEmpty(t, tempToken)

	// Email-code-only sessions cannot be completed via TOTP.
	recorder = postAuthJSON(t, "/api/v1/auth/login/2fa", `{"temp_token":"`+tempToken+`","totp_code":"123456"}`, env.handler.Login2FA)
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	// Wrong code is rejected and the session is kept.
	recorder = postAuthJSON(t, "/api/v1/auth/login/email-code", loginEmailCodeBody(tempToken, "000000"), env.handler.LoginEmailCode)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	session, err := env.totpCache.GetLoginSession(ctx, tempToken)
	require.NoError(t, err)
	require.NotNil(t, session)

	recorder = postAuthJSON(t, "/api/v1/auth/login/email-code", loginEmailCodeBody(tempToken, "123456"), env.handler.LoginEmailCode)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload = decodeJSONResponseData(t, recorder)
	require.NotEmpty(t, payload["access_token"])
	require.NotEmpty(t, payload["refresh_token"])

	// Session and code are consumed.
	session, err = env.totpCache.GetLoginSession(ctx, tempToken)
	require.NoError(t, err)
	require.Nil(t, session)
	recorder = postAuthJSON(t, "/api/v1/auth/login/email-code", loginEmailCodeBody(tempToken, "123456"), env.handler.LoginEmailCode)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestAdminLoginEmailCodeRejectsExpiredCodeAndSession(t *testing.T) {
	env := newAdminEmailCodeLoginTestEnv(t, adminEmailCodeLoginTestOptions{featureEnabled: true, smtpConfigured: true})
	ctx := context.Background()

	tempToken, err := env.handler.totpService.CreateEmailCodeLoginSession(ctx, env.user.ID, env.user.Email, false)
	require.NoError(t, err)

	// No stored code (expired / never sent).
	recorder := postAuthJSON(t, "/api/v1/auth/login/email-code", loginEmailCodeBody(tempToken, "123456"), env.handler.LoginEmailCode)
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	// A registration code (different namespace) is not accepted.
	require.NoError(t, env.emailCache.SetVerificationCode(ctx, env.user.Email, &service.VerificationCodeData{
		Code:      "123456",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}, 15*time.Minute))
	recorder = postAuthJSON(t, "/api/v1/auth/login/email-code", loginEmailCodeBody(tempToken, "123456"), env.handler.LoginEmailCode)
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	// Expired session.
	env.seedLoginCode(t, "123456")
	require.NoError(t, env.handler.totpService.DeleteLoginSession(ctx, tempToken))
	recorder = postAuthJSON(t, "/api/v1/auth/login/email-code", loginEmailCodeBody(tempToken, "123456"), env.handler.LoginEmailCode)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Invalid or expired login session")
}

func TestAdminLoginEmailCodeResendCooldown(t *testing.T) {
	env := newAdminEmailCodeLoginTestEnv(t, adminEmailCodeLoginTestOptions{featureEnabled: true, smtpConfigured: true})
	env.seedLoginCode(t, "123456")

	recorder := env.login(t)
	require.Equal(t, http.StatusOK, recorder.Code)
	tempToken := decodeJSONResponseData(t, recorder)["temp_token"].(string)

	recorder = postAuthJSON(t, "/api/v1/auth/login/email-code/resend", `{"temp_token":"`+tempToken+`"}`, env.handler.LoginEmailCodeResend)
	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "123456", env.emailCache.verificationCodes[adminLoginCodeKeyPrefix+env.user.Email].Code)

	recorder = postAuthJSON(t, "/api/v1/auth/login/email-code/resend", `{"temp_token":"unknown"}`, env.handler.LoginEmailCodeResend)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestAdminLoginEmailCodeWithTotpAcceptsEitherMethod(t *testing.T) {
	env := newAdminEmailCodeLoginTestEnv(t, adminEmailCodeLoginTestOptions{featureEnabled: true, smtpConfigured: true, withTotp: true})

	// No email is sent until the admin chooses the email method.
	recorder := env.login(t)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := decodeJSONResponseData(t, recorder)
	require.Equal(t, true, payload["requires_2fa"])
	require.Equal(t, true, payload["email_code_available"])
	require.Empty(t, env.emailCache.verificationCodes)

	// Complete with TOTP.
	code, err := totp.GenerateCode(adminEmailCodeTestTotpSecret, time.Now().UTC())
	require.NoError(t, err)
	recorder = postAuthJSON(t, "/api/v1/auth/login/2fa", `{"temp_token":"`+payload["temp_token"].(string)+`","totp_code":"`+code+`"}`, env.handler.Login2FA)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NotEmpty(t, decodeJSONResponseData(t, recorder)["access_token"])

	// Complete with an email code.
	recorder = env.login(t)
	require.Equal(t, http.StatusOK, recorder.Code)
	tempToken := decodeJSONResponseData(t, recorder)["temp_token"].(string)
	env.seedLoginCode(t, "654321")
	recorder = postAuthJSON(t, "/api/v1/auth/login/email-code", loginEmailCodeBody(tempToken, "654321"), env.handler.LoginEmailCode)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotEmpty(t, decodeJSONResponseData(t, recorder)["access_token"])
}

func TestAdminLoginEmailCodeSkippedWithoutSMTP(t *testing.T) {
	env := newAdminEmailCodeLoginTestEnv(t, adminEmailCodeLoginTestOptions{featureEnabled: true})

	recorder := env.login(t)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := decodeJSONResponseData(t, recorder)
	require.NotEmpty(t, payload["access_token"])
	require.Nil(t, payload["requires_email_code"])
}

func TestAdminLoginEmailCodeNotRequiredForRegularUsers(t *testing.T) {
	env := newAdminEmailCodeLoginTestEnv(t, adminEmailCodeLoginTestOptions{featureEnabled: true, smtpConfigured: true, role: service.RoleUser})

	recorder := env.login(t)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := decodeJSONResponseData(t, recorder)
	require.NotEmpty(t, payload["access_token"])
	require.Nil(t, payload["requires_email_code"])
}
