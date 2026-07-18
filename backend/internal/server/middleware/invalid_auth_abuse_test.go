//go:build unit

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func invalidAuthAbuseTestConfig(threshold int) *config.Config {
	return &config.Config{
		RunMode: config.RunModeSimple,
		APIKeyAuth: config.APIKeyAuthCacheConfig{InvalidAbuse: config.InvalidAuthAbuseConfig{
			Enabled: true, Threshold: threshold, WindowSeconds: 60, BlockSeconds: 60, Capacity: 256,
REDACTED
REDACTED
REDACTED

func TestAPIKeyAuthInvalidAbuseReturns429BeforeRepository(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repoCalls := 0
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		repoCalls++
		return nil, service.ErrAPIKeyNotFound
REDACTEDREDACTED
	cfg := invalidAuthAbuseTestConfig(3)
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
	r := gin.New()
	var reason IngressRejectReason
	r.Use(func(c *gin.Context) { c.Next(); reason, _ = GetIngressRejectReason(c) REDACTED)
	r.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg)))
	r.POST("/v1/messages", func(c *gin.Context) { c.Status(http.StatusOK) REDACTED)

	requests := []*http.Request{
		httpRequest(t, "/v1/messages", "", ""),
		httpRequest(t, "/v1/messages", "Basic malformed", ""),
		httpRequest(t, "/v1/messages", "", "random-invalid-key"),
REDACTED
	for _, req := range requests {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusTooManyRequests, w.Code)
REDACTED

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httpRequest(t, "/v1/messages", "", "another-random-key"))
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, "60", w.Header().Get("Retry-After"))
	require.Contains(t, w.Body.String(), "INVALID_AUTH_RATE_LIMITED")
	require.Equal(t, IngressRejectInvalidAuthRateLimited, reason)
	require.Equal(t, 1, repoCalls, "rate-limited request must not reach the repository")
REDACTED

func TestGoogleAPIKeyAuthInvalidAbuseReturnsProtocol429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repoCalls := 0
	repo := fakeAPIKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		repoCalls++
		return nil, service.ErrAPIKeyNotFound
REDACTEDREDACTED
	cfg := invalidAuthAbuseTestConfig(2)
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
	r := gin.New()
	var reason IngressRejectReason
	r.Use(func(c *gin.Context) { c.Next(); reason, _ = GetIngressRejectReason(c) REDACTED)
	r.Use(APIKeyAuthGoogle(svc, cfg))
	r.POST("/v1beta/models/test:generateContent", func(c *gin.Context) { c.Status(http.StatusOK) REDACTED)
	for _, key := range []string{"random-1", "random-2"REDACTED {
		w := httptest.NewRecorder()
		req := httpRequest(t, "/v1beta/models/test:generateContent", "", key)
		req.Header.Del("x-api-key")
		req.Header.Set("x-goog-api-key", key)
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
REDACTED
	w := httptest.NewRecorder()
	req := httpRequest(t, "/v1beta/models/test:generateContent", "", "random-3")
	req.Header.Del("x-api-key")
	req.Header.Set("x-goog-api-key", "random-3")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, "60", w.Header().Get("Retry-After"))
	require.Contains(t, w.Body.String(), "RESOURCE_EXHAUSTED")
	require.Equal(t, IngressRejectInvalidAuthRateLimited, reason)
	require.Equal(t, 2, repoCalls)
REDACTED

func TestInvalidAuthAbuseDoesNotCountValidOrOperationalFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	user := &service.User{ID: 1, Status: service.StatusActive, Role: service.RoleUser, Balance: 1REDACTED
	repo := &stubApiKeyRepo{getByKey: func(_ context.Context, key string) (*service.APIKey, error) {
		switch key {
		case "valid-key":
			return &service.APIKey{ID: 1, UserID: 1, Key: key, Status: service.StatusActive, User: userREDACTED, nil
		case "db-error":
			return nil, errors.New("database unavailable")
		default:
			return nil, service.ErrAPIKeyNotFound
	REDACTED
REDACTEDREDACTED
	cfg := invalidAuthAbuseTestConfig(10)
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
	r := gin.New()
	r.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg)))
	r.POST("/t", func(c *gin.Context) { c.Status(http.StatusOK) REDACTED)

	for _, tc := range []struct {
		key  string
		want int
REDACTED{{"invalid", 401REDACTED, {"valid-key", 200REDACTED, {"db-error", 500REDACTED, {"db-error", 500REDACTEDREDACTED {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httpRequest(t, "/t", "", tc.key))
		require.Equal(t, tc.want, w.Code)
REDACTED
	w := httptest.NewRecorder()
	req := httpRequest(t, "/t", "", "")
	req.Header.Set("x-goog-api-key", "valid-key")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, uint64(1), svc.InvalidAuthAbuseHealth().Recorded)
REDACTED

func TestNormalizeIngressRejectIPGroupsIPv6By64(t *testing.T) {
	require.Equal(t, "2001:db8:abcd:1234::", normalizeIngressRejectIP("2001:db8:abcd:1234:1111::1"))
	require.Equal(t, normalizeIngressRejectIP("2001:db8:abcd:1234:1111::1"), normalizeIngressRejectIP("2001:db8:abcd:1234:ffff::2"))
REDACTED

func httpRequest(t *testing.T, path, authorization, apiKey string) *http.Request {
REDACTED
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.RemoteAddr = "203.0.113.10:12345"
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
REDACTED
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
REDACTED
	return req
REDACTED
