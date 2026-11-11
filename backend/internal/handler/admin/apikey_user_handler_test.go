package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userKeyServiceStub struct {
	creates, updates, deletes int
	userID                    int64
	create                    service.CreateAPIKeyRequest
	update                    service.UpdateAPIKeyRequest
}

func (s *userKeyServiceStub) Create(_ context.Context, id int64, req service.CreateAPIKeyRequest) (*service.APIKey, error) {
	s.creates++
	s.userID = id
	s.create = req
	return &service.APIKey{ID: 9, UserID: id, Key: "sk-test-created", Name: req.Name}, nil
}
func (s *userKeyServiceStub) GetByID(context.Context, int64) (*service.APIKey, error) {
	return &service.APIKey{ID: 9, UserID: 7, Key: "sk-test-existing"}, nil
}
func (s *userKeyServiceStub) Update(_ context.Context, _ int64, id int64, req service.UpdateAPIKeyRequest) (*service.APIKey, error) {
	s.updates++
	s.userID = id
	s.update = req
	return &service.APIKey{ID: 9, UserID: id}, nil
}
func (s *userKeyServiceStub) Delete(_ context.Context, _ int64, id int64) error {
	s.deletes++
	s.userID = id
	return nil
}

func userKeyRouter(s *userKeyServiceStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &AdminAPIKeyHandler{keyService: s}
	r := gin.New()
	r.POST("/users/:id/api-keys", h.CreateForUser)
	r.GET("/users/:id/api-keys/:key_id", h.GetForUser)
	r.PUT("/users/:id/api-keys/:key_id", h.UpdateForUser)
	r.DELETE("/users/:id/api-keys/:key_id", h.DeleteForUser)
	return r
}
func userKeyRequest(r *gin.Engine, method, url, body, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestAdminUserKeyCreate_IdempotencyAndTargetUser(t *testing.T) {
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), service.DefaultIdempotencyConfig()))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(nil) })
	stub := &userKeyServiceStub{}
	r := userKeyRouter(stub)
	body := `{"name":"external","custom_key":"sk-test-custom-key-123456789","quota":5,"rate_limit_5h":2,"expires_in_days":1}`
	first := userKeyRequest(r, http.MethodPost, "/users/7/api-keys", body, "same-request")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	second := userKeyRequest(r, http.MethodPost, "/users/7/api-keys", body, "same-request")
	require.JSONEq(t, first.Body.String(), second.Body.String())
	require.Equal(t, "true", second.Header().Get("X-Idempotency-Replayed"))
	require.Equal(t, 1, stub.creates)
	require.Equal(t, int64(7), stub.userID)
	require.Equal(t, 5.0, stub.create.Quota)
	changedTarget := userKeyRequest(r, http.MethodPost, "/users/8/api-keys", body, "same-request")
	require.Equal(t, http.StatusConflict, changedTarget.Code)
	changedBody := userKeyRequest(r, http.MethodPost, "/users/7/api-keys", `{"name":"different"}`, "same-request")
	require.Equal(t, http.StatusConflict, changedBody.Code)
	require.Equal(t, 1, stub.creates)
}

func TestAdminUserKeyCreate_FailClosed(t *testing.T) {
	stub := &userKeyServiceStub{}
	r := userKeyRouter(stub)
	service.SetDefaultIdempotencyCoordinator(nil)
	missing := userKeyRequest(r, http.MethodPost, "/users/7/api-keys", `{"name":"test"}`, "")
	require.Equal(t, http.StatusBadRequest, missing.Code)
	noStore := userKeyRequest(r, http.MethodPost, "/users/7/api-keys", `{"name":"test"}`, "test")
	require.Equal(t, http.StatusServiceUnavailable, noStore.Code)
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(storeUnavailableRepoStub{}, service.DefaultIdempotencyConfig()))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(nil) })
	failedStore := userKeyRequest(r, http.MethodPost, "/users/7/api-keys", `{"name":"test"}`, "test")
	require.Equal(t, http.StatusServiceUnavailable, failedStore.Code)
	require.Zero(t, stub.creates)
}

func TestAdminUserKeys_OwnershipAndValidation(t *testing.T) {
	stub := &userKeyServiceStub{}
	r := userKeyRouter(stub)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := userKeyRequest(r, method, "/users/8/api-keys/9", `{}`, "")
		require.Equal(t, http.StatusNotFound, rec.Code)
		require.NotContains(t, rec.Body.String(), "sk-test-existing")
	}
	require.Zero(t, stub.updates)
	require.Zero(t, stub.deletes)
	for _, body := range []string{`{"status":"admin"}`, `{"expires_at":"tomorrow"}`} {
		rec := userKeyRequest(r, http.MethodPut, "/users/7/api-keys/9", body, "")
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}
	require.Zero(t, stub.updates)
	for _, url := range []string{"/users/0/api-keys/9", "/users/7/api-keys/-1"} {
		require.Equal(t, http.StatusBadRequest, userKeyRequest(r, http.MethodGet, url, "", "").Code)
	}
}

func TestAdminUserKeys_UpdateFieldsAndRevoke(t *testing.T) {
	stub := &userKeyServiceStub{}
	r := userKeyRouter(stub)
	body := `{"name":"updated","group_id":4,"status":"inactive","quota":0,"reset_quota":true,"rate_limit_5h":3,"rate_limit_1d":4,"rate_limit_7d":5,"reset_rate_limit_usage":true,"ip_whitelist":[],"ip_blacklist":["10.0.0.1"],"expires_at":"2027-01-01T00:00:00Z"}`
	rec := userKeyRequest(r, http.MethodPut, "/users/7/api-keys/9", body, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, int64(7), stub.userID)
	require.Equal(t, "updated", *stub.update.Name)
	require.Equal(t, int64(4), *stub.update.GroupID)
	require.Equal(t, "inactive", *stub.update.Status)
	require.Equal(t, 0.0, *stub.update.Quota)
	require.True(t, *stub.update.ResetQuota)
	require.Equal(t, 3.0, *stub.update.RateLimit5h)
	require.Equal(t, 4.0, *stub.update.RateLimit1d)
	require.Equal(t, 5.0, *stub.update.RateLimit7d)
	require.True(t, *stub.update.ResetRateLimitUsage)
	require.Empty(t, *stub.update.IPWhitelist)
	require.Equal(t, []string{"10.0.0.1"}, *stub.update.IPBlacklist)
	require.Equal(t, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), *stub.update.ExpiresAt)
	rec = userKeyRequest(r, http.MethodPut, "/users/7/api-keys/9", `{"expires_at":""}`, "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, stub.update.ClearExpiration)
	rec = userKeyRequest(r, http.MethodDelete, "/users/7/api-keys/9", "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, stub.deletes)
}
