package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type complianceGuardRepoStub struct {
	values map[string]string
REDACTED

func (r *complianceGuardRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	if value, ok := r.values[key]; ok {
		return &service.Setting{Key: key, Value: valueREDACTED, nil
REDACTED
	return nil, service.ErrSettingNotFound
REDACTED

func (r *complianceGuardRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
REDACTED
	return setting.Value, nil
REDACTED

func (r *complianceGuardRepoStub) Set(ctx context.Context, key, value string) error { return nil REDACTED
func (r *complianceGuardRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	return map[string]string{REDACTED, nil
REDACTED
func (r *complianceGuardRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	return nil
REDACTED
func (r *complianceGuardRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	return map[string]string{REDACTED, nil
REDACTED
func (r *complianceGuardRepoStub) Delete(ctx context.Context, key string) error { return nil REDACTED

func TestAdminComplianceGuardBlocksAdminRouteWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewSettingService(&complianceGuardRepoStub{REDACTED, &config.Config{REDACTED)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyUser), AuthSubject{UserID: 1REDACTED)
		c.Next()
REDACTED)
	router.Use(AdminComplianceGuard(svc))
	router.GET("/api/v1/admin/users", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
REDACTED)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusLocked, w.Code)
	require.Contains(t, w.Body.String(), "ADMIN_COMPLIANCE_ACK_REQUIRED")
REDACTED

func TestAdminComplianceGuardBypassesComplianceEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewSettingService(&complianceGuardRepoStub{REDACTED, &config.Config{REDACTED)
	router := gin.New()
	router.Use(AdminComplianceGuard(svc))
	router.GET("/api/v1/admin/compliance", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
REDACTED)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/compliance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "ok", w.Body.String())
REDACTED
