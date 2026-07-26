package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type passkeySwitchSettingRepo struct {
	value string
	err   error
REDACTED

func (r *passkeySwitchSettingRepo) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
REDACTED
func (r *passkeySwitchSettingRepo) GetValue(context.Context, string) (string, error) {
	return r.value, r.err
REDACTED
func (r *passkeySwitchSettingRepo) Set(context.Context, string, string) error { return nil REDACTED
func (r *passkeySwitchSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{REDACTED, nil
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
