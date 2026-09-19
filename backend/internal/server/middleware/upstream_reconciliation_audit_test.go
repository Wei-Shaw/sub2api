package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpstreamBillingCredentialsNeverEnterAuditBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &auditCaptureRepository{}
	svc := service.NewAuditLogService(repo, nil)
	svc.Start()
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAuditLogMiddleware(svc)))
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		path := "/api/v1/upstream-billing/connections"
		if method == http.MethodPut {
			path += "/:id"
		}
		router.Handle(method, path, func(c *gin.Context) { c.Status(http.StatusOK) })
		request := httptest.NewRequest(method, "/api/v1/upstream-billing/connections"+map[bool]string{true: "/1"}[method == http.MethodPut],
			bytes.NewBufferString(`{"secrets":{"client_secret":"billing-canary-secret","secret_id":"billing-canary-id"}}`))
		request.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, request)
		require.Equal(t, http.StatusOK, w.Code)
	}
	svc.Stop()
	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.logs, 2)
	for _, entry := range repo.logs {
		require.Equal(t, "<credential-bearing body omitted>", entry.RequestBody)
		require.NotContains(t, entry.RequestBody, "billing-canary")
	}
}

func TestUpstreamBillingAdminRawReadsAreAuditedWithoutSourcePayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &auditCaptureRepository{}
	svc := service.NewAuditLogService(repo, nil)
	svc.Start()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyUser), AuthSubject{UserID: 7})
		c.Set(string(ContextKeyUserRole), "admin")
		c.Set("auth_method", "admin_api_key")
		c.Next()
	})
	router.Use(gin.HandlerFunc(NewAuditLogMiddleware(svc)))
	for _, path := range []string{"/api/v1/admin/upstream-billing/bills", "/api/v1/admin/upstream-billing/bills/:id"} {
		router.GET(path, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"raw_source": "private-billing-source-canary"}) })
	}
	for _, path := range []string{"/api/v1/admin/upstream-billing/bills?month=2026-09", "/api/v1/admin/upstream-billing/bills/3?month=2026-09"} {
		w := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("x-api-key", "admin-sensitive-api-key-canary")
		router.ServeHTTP(w, request)
		require.Equal(t, http.StatusOK, w.Code)
	}
	svc.Stop()
	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.logs, 2)
	actions := map[string]bool{}
	for _, entry := range repo.logs {
		actions[entry.Action] = true
		require.Equal(t, "admin_api_key", entry.AuthMethod)
		require.Empty(t, entry.RequestBody)
		require.NotContains(t, entry.RequestBody, "private-billing-source-canary")
		require.NotContains(t, entry.CredentialMasked, "admin-sensitive-api-key-canary")
	}
	require.True(t, actions["admin.upstream_billing.sources.list"])
	require.True(t, actions["admin.upstream_billing.source.read"])
}
