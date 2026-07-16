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

	ok := enforceStepUp(c, stubStepUpGrantChecker{granted: trueREDACTED, stubStepUpUserReader{user: &service.User{TotpEnabled: trueREDACTEDREDACTED)

	require.False(t, ok)
	require.True(t, c.IsAborted())
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "STEP_UP_ADMIN_API_KEY_FORBIDDEN")
REDACTED

func TestEnforceStepUpRequiresAuthSubject(t *testing.T) {
	c, rec := newStepUpTestContext(t)

	ok := enforceStepUp(c, stubStepUpGrantChecker{granted: trueREDACTED, stubStepUpUserReader{user: &service.User{TotpEnabled: trueREDACTEDREDACTED)

	require.False(t, ok)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
REDACTED

func TestEnforceStepUpRequiresTotpEnabled(t *testing.T) {
	c, rec := newStepUpTestContext(t)
	c.Set(string(ContextKeyUser), AuthSubject{UserID: 1REDACTED)

	ok := enforceStepUp(c, stubStepUpGrantChecker{granted: trueREDACTED, stubStepUpUserReader{user: &service.User{ID: 1, TotpEnabled: falseREDACTEDREDACTED)

	require.False(t, ok)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "STEP_UP_TOTP_NOT_ENABLED")
REDACTED

func TestEnforceStepUpFailsClosedOnGrantError(t *testing.T) {
	c, rec := newStepUpTestContext(t)
	c.Set(string(ContextKeyUser), AuthSubject{UserID: 1REDACTED)

	ok := enforceStepUp(c, stubStepUpGrantChecker{err: errors.New("redis down")REDACTED, stubStepUpUserReader{user: &service.User{ID: 1, TotpEnabled: trueREDACTEDREDACTED)

	require.False(t, ok)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "STEP_UP_UNAVAILABLE")
REDACTED

func TestEnforceStepUpRequiresGrant(t *testing.T) {
	c, rec := newStepUpTestContext(t)
	c.Set(string(ContextKeyUser), AuthSubject{UserID: 1REDACTED)

	ok := enforceStepUp(c, stubStepUpGrantChecker{granted: falseREDACTED, stubStepUpUserReader{user: &service.User{ID: 1, TotpEnabled: trueREDACTEDREDACTED)

	require.False(t, ok)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "STEP_UP_REQUIRED")
REDACTED

func TestEnforceStepUpPassesWithGrant(t *testing.T) {
	c, _ := newStepUpTestContext(t)
	c.Set(string(ContextKeyUser), AuthSubject{UserID: 1REDACTED)

	ok := enforceStepUp(c, stubStepUpGrantChecker{granted: trueREDACTED, stubStepUpUserReader{user: &service.User{ID: 1, TotpEnabled: trueREDACTEDREDACTED)

	require.True(t, ok)
	require.False(t, c.IsAborted())
REDACTED
