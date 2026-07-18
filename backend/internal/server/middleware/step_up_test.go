package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubStepUpGrantChecker struct {
	granted bool
	err     error
REDACTED

func (s stubStepUpGrantChecker) HasStepUpGrant(ctx context.Context, userID int64, sessionKey string) (bool, error) {
	return s.granted, s.err
REDACTED

type stubStepUpUserReader struct {
	user *service.User
	err  error
REDACTED

func (s stubStepUpUserReader) GetByID(ctx context.Context, id int64) (*service.User, error) {
	return s.user, s.err
REDACTED

type stubStepUpSettingReader struct {
	enabled bool
REDACTED

func (s stubStepUpSettingReader) IsStepUpEnabled(ctx context.Context) bool {
	return s.enabled
REDACTED

// stepUpEnabled 功能开关开启的设置桩，供既有门控分支测试使用。
var stepUpEnabled = stubStepUpSettingReader{enabled: trueREDACTED

func newStepUpTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
REDACTED
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/sensitive", nil)
	return c, rec
REDACTED

func TestEnforceStepUpRejectsAdminAPIKey(t *testing.T) {
	c, rec := newStepUpTestContext(t)
	c.Set("auth_method", service.AuditAuthMethodAdminAPIKey)

	ok := enforceStepUp(c, stubStepUpGrantChecker{granted: trueREDACTED, stubStepUpUserReader{user: &service.User{TotpEnabled: trueREDACTEDREDACTED, stepUpEnabled)

	require.False(t, ok)
	require.True(t, c.IsAborted())
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "STEP_UP_ADMIN_API_KEY_FORBIDDEN")
REDACTED

func TestEnforceStepUpRequiresAuthSubject(t *testing.T) {
	c, rec := newStepUpTestContext(t)

	ok := enforceStepUp(c, stubStepUpGrantChecker{granted: trueREDACTED, stubStepUpUserReader{user: &service.User{TotpEnabled: trueREDACTEDREDACTED, stepUpEnabled)

	require.False(t, ok)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
REDACTED

func TestEnforceStepUpRequiresTotpEnabled(t *testing.T) {
	c, rec := newStepUpTestContext(t)
	c.Set(string(ContextKeyUser), AuthSubject{UserID: 1REDACTED)

	ok := enforceStepUp(c, stubStepUpGrantChecker{granted: trueREDACTED, stubStepUpUserReader{user: &service.User{ID: 1, TotpEnabled: falseREDACTEDREDACTED, stepUpEnabled)

	require.False(t, ok)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "STEP_UP_TOTP_NOT_ENABLED")
REDACTED

func TestEnforceStepUpFailsClosedOnGrantError(t *testing.T) {
	c, rec := newStepUpTestContext(t)
	c.Set(string(ContextKeyUser), AuthSubject{UserID: 1REDACTED)

	ok := enforceStepUp(c, stubStepUpGrantChecker{err: errors.New("redis down")REDACTED, stubStepUpUserReader{user: &service.User{ID: 1, TotpEnabled: trueREDACTEDREDACTED, stepUpEnabled)

	require.False(t, ok)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "STEP_UP_UNAVAILABLE")
REDACTED

func TestEnforceStepUpRequiresGrant(t *testing.T) {
	c, rec := newStepUpTestContext(t)
	c.Set(string(ContextKeyUser), AuthSubject{UserID: 1REDACTED)

	ok := enforceStepUp(c, stubStepUpGrantChecker{granted: falseREDACTED, stubStepUpUserReader{user: &service.User{ID: 1, TotpEnabled: trueREDACTEDREDACTED, stepUpEnabled)

	require.False(t, ok)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "STEP_UP_REQUIRED")
REDACTED

func TestEnforceStepUpPassesWithGrant(t *testing.T) {
	c, _ := newStepUpTestContext(t)
	c.Set(string(ContextKeyUser), AuthSubject{UserID: 1REDACTED)

	ok := enforceStepUp(c, stubStepUpGrantChecker{granted: trueREDACTED, stubStepUpUserReader{user: &service.User{ID: 1, TotpEnabled: trueREDACTEDREDACTED, stepUpEnabled)

	require.True(t, ok)
	require.False(t, c.IsAborted())
REDACTED

// 功能开关关闭时：不论 TOTP/grant/凭证类型，一律放行（恢复门控引入前行为）。
func TestEnforceStepUpDisabledSkipsAllChecks(t *testing.T) {
	disabled := stubStepUpSettingReader{enabled: falseREDACTED

	t.Run("no totp, no grant", func(t *testing.T) {
		c, _ := newStepUpTestContext(t)
		c.Set(string(ContextKeyUser), AuthSubject{UserID: 1REDACTED)

		ok := enforceStepUp(c, stubStepUpGrantChecker{granted: falseREDACTED, stubStepUpUserReader{user: &service.User{ID: 1, TotpEnabled: falseREDACTEDREDACTED, disabled)

		require.True(t, ok)
		require.False(t, c.IsAborted())
REDACTED)

	t.Run("admin api key", func(t *testing.T) {
		c, _ := newStepUpTestContext(t)
		c.Set("auth_method", service.AuditAuthMethodAdminAPIKey)

		ok := enforceStepUp(c, stubStepUpGrantChecker{granted: falseREDACTED, stubStepUpUserReader{user: nil, err: errors.New("should not be called")REDACTED, disabled)

		require.True(t, ok)
		require.False(t, c.IsAborted())
REDACTED)
REDACTED

// settings 为 nil 时保持门控（fail-closed），避免装配缺陷静默关闭安全控制。
func TestEnforceStepUpNilSettingsFailsClosed(t *testing.T) {
	c, rec := newStepUpTestContext(t)
	c.Set(string(ContextKeyUser), AuthSubject{UserID: 1REDACTED)

	ok := enforceStepUp(c, stubStepUpGrantChecker{granted: falseREDACTED, stubStepUpUserReader{user: &service.User{ID: 1, TotpEnabled: trueREDACTEDREDACTED, nil)

	require.False(t, ok)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "STEP_UP_REQUIRED")
REDACTED

// EnforceStepUp 收到 nil *service.SettingService 时不得因 typed-nil 装箱绕过门控：
// 未认证请求仍应被拦截（401），而不是当作"开关关闭"放行。
func TestEnforceStepUpTypedNilSettingServiceFailsClosed(t *testing.T) {
	require.Nil(t, stepUpSettingsOrNil(nil))

	c, rec := newStepUpTestContext(t)

	ok := EnforceStepUp(c, nil, nil, nil)

	require.False(t, ok)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
REDACTED
