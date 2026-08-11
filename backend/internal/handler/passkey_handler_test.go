package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type passkeySwitchSettingRepo struct {
	value  string
	values map[string]string
	err    error
REDACTED

type passkeyCaptchaVerifierStub struct {
	calls int
	proof service.TencentCaptchaProof
REDACTED

func (s *passkeyCaptchaVerifierStub) VerifyTicket(_ context.Context, _ service.TencentCaptchaCredentials, proof service.TencentCaptchaProof, _ string) (*service.TencentCaptchaVerifyResponse, error) {
	s.calls++
	s.proof = proof
	return &service.TencentCaptchaVerifyResponse{CaptchaCode: 1REDACTED, nil
REDACTED

type passkeyBeginSessionStoreStub struct {
	service.PasskeySessionStore
	storeCalls int
REDACTED

func (s *passkeyBeginSessionStoreStub) Store(context.Context, *service.PasskeySession, time.Duration) (string, error) {
	s.storeCalls++
	return "passkey-session", nil
REDACTED

func (r *passkeySwitchSettingRepo) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
REDACTED
func (r *passkeySwitchSettingRepo) GetValue(context.Context, string) (string, error) {
	return r.value, r.err
REDACTED
func (r *passkeySwitchSettingRepo) Set(context.Context, string, string) error { return nil REDACTED
func (r *passkeySwitchSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	if r.err != nil {
		return nil, r.err
REDACTED
	return r.values, nil
REDACTED
func (r *passkeySwitchSettingRepo) SetMultiple(context.Context, map[string]string) error {
	return nil
REDACTED
func (r *passkeySwitchSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{REDACTED, nil
REDACTED
func (r *passkeySwitchSettingRepo) Delete(context.Context, string) error { return nil REDACTED

func TestBindPasskeyFinishRequestRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/passkey/login/finish",
		strings.NewReader(`{"credential":"`+strings.Repeat("x", passkeyFinishBodyMaxBytes)+`"REDACTED`),
	)
	context.Request.Header.Set("Content-Type", "application/json")

	_, ok := bindPasskeyFinishRequest(context)
	require.False(t, ok)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
REDACTED

func TestPasskeyBeginLoginRejectsDisabledAdminSwitch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &passkeySwitchSettingRepo{value: "false"REDACTED
	settings := service.NewSettingService(repo, &config.Config{
		WebAuthn: config.WebAuthnConfig{Enabled: trueREDACTED,
REDACTED)
	handler := NewPasskeyHandler(nil, nil, settings)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkey/login/begin", nil)

	handler.BeginLogin(ginContext)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "PASSKEY_DISABLED")
REDACTED

func TestPasskeyBeginLoginReportsSettingStoreFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := service.NewSettingService(
		&passkeySwitchSettingRepo{err: errors.New("database unavailable")REDACTED,
		&config.Config{WebAuthn: config.WebAuthnConfig{Enabled: trueREDACTEDREDACTED,
	)
	handler := NewPasskeyHandler(nil, nil, settings)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkey/login/begin", nil)

	handler.BeginLogin(ginContext)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "PASSKEY_DISABLED")
REDACTED

func newTencentProtectedPasskeyHandler(t *testing.T) (*PasskeyHandler, *passkeyCaptchaVerifierStub, *passkeyBeginSessionStoreStub) {
REDACTED
	cfg := &config.Config{WebAuthn: config.WebAuthnConfig{
		Enabled:       true,
		RPDisplayName: "Sub2API",
		RPID:          "sub2api.example.com",
		RPOrigins:     []string{"https://sub2api.example.com"REDACTED,
REDACTEDREDACTED
	repo := &passkeySwitchSettingRepo{
		value: "true",
		values: map[string]string{
			service.SettingKeyTencentCaptchaEnabled:        "true",
			service.SettingKeyTencentCaptchaAppID:          "123456789",
			service.SettingKeyTencentCaptchaAppSecretKey:   "app-secret",
			service.SettingKeyTencentCaptchaCloudSecretID:  "cloud-secret-id",
			service.SettingKeyTencentCaptchaCloudSecretKey: "cloud-secret-key",
	REDACTED,
REDACTED
	settings := service.NewSettingService(repo, cfg)
	verifier := &passkeyCaptchaVerifierStub{REDACTED
	authService := service.NewAuthService(nil, nil, nil, nil, cfg, settings, nil, nil, nil, nil, nil, nil, nil)
	authService.SetTencentCaptchaService(service.NewTencentCaptchaService(settings, verifier))
	sessions := &passkeyBeginSessionStoreStub{REDACTED
	passkeys, err := service.NewPasskeyService(cfg, nil, sessions, nil)
REDACTED
	return NewPasskeyHandler(passkeys, authService, settings), verifier, sessions
REDACTED

func newPasskeyBeginLoginContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/passkey/login/begin",
		strings.NewReader(body),
	)
	ginContext.Request.Header.Set("Content-Type", "application/json")
	return ginContext, recorder
REDACTED

func TestPasskeyBeginLoginRejectsMissingTencentCaptchaProof(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, verifier, sessions := newTencentProtectedPasskeyHandler(t)
	ginContext, recorder := newPasskeyBeginLoginContext(`{REDACTED`)

	handler.BeginLogin(ginContext)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "TENCENT_CAPTCHA_VERIFICATION_FAILED")
	require.Zero(t, verifier.calls)
	require.Zero(t, sessions.storeCalls)
REDACTED

func TestPasskeyBeginLoginAcceptsTencentCaptchaProofBeforeCeremony(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, verifier, sessions := newTencentProtectedPasskeyHandler(t)
	ginContext, recorder := newPasskeyBeginLoginContext(
		`{"tencent_captcha_ticket":"ticket-value","tencent_captcha_randstr":"@rand-value"REDACTED`,
	)

	handler.BeginLogin(ginContext)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, verifier.calls)
	require.Equal(t, service.TencentCaptchaProof{Ticket: "ticket-value", Randstr: "@rand-value"REDACTED, verifier.proof)
	require.Equal(t, 1, sessions.storeCalls)
REDACTED

func TestPasskeyCredentialListRemainsAvailableWhenSignInDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewPasskeyHandler(nil, nil, nil)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/passkeys", nil)

	handler.List(ginContext)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "PASSKEY_DISABLED")
REDACTED
