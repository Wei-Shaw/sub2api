//go:build unit

package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClientRequestID_GeneratesWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(ClientRequestID())
	r.GET("/t", func(c *gin.Context) {
		v := c.Request.Context().Value(ctxkey.ClientRequestID)
		require.NotNil(t, v)
		id, ok := v.(string)
		require.True(t, ok)
		require.NotEmpty(t, id)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestClientRequestID_PreservesExisting(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(ClientRequestID())
	r.GET("/t", func(c *gin.Context) {
		id, ok := c.Request.Context().Value(ctxkey.ClientRequestID).(string)
		require.True(t, ok)
		require.Equal(t, "keep", id)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.ClientRequestID, "keep"))
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestRequestBodyLimit_LimitsBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(RequestBodyLimit(4))
	r.POST("/t", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		require.Error(t, err)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/t", bytes.NewBufferString("12345"))
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestForcePlatform_SetsContextAndGinValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(ForcePlatform("anthropic"))
	r.GET("/t", func(c *gin.Context) {
		require.True(t, HasForcePlatform(c))
		v, ok := GetForcePlatformFromContext(c)
		require.True(t, ok)
		require.Equal(t, "anthropic", v)

		ctxV := c.Request.Context().Value(ctxkey.ForcePlatform)
		require.Equal(t, "anthropic", ctxV)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestAuthSubjectHelpers_RoundTrip(t *testing.T) {
	c := &gin.Context{}
	c.Set(string(ContextKeyUser), AuthSubject{UserID: 1, Concurrency: 2})
	c.Set(string(ContextKeyUserRole), "admin")

	sub, ok := GetAuthSubjectFromContext(c)
	require.True(t, ok)
	require.Equal(t, int64(1), sub.UserID)
	require.Equal(t, 2, sub.Concurrency)

	role, ok := GetUserRoleFromContext(c)
	require.True(t, ok)
	require.Equal(t, "admin", role)
}

func TestAPIKeyAndSubscriptionFromContext(t *testing.T) {
	c := &gin.Context{}

	key := &service.APIKey{ID: 1}
	c.Set(string(ContextKeyAPIKey), key)
	gotKey, ok := GetAPIKeyFromContext(c)
	require.True(t, ok)
	require.Equal(t, int64(1), gotKey.ID)

	sub := &service.UserSubscription{ID: 2}
	c.Set(string(ContextKeySubscription), sub)
	gotSub, ok := GetSubscriptionFromContext(c)
	require.True(t, ok)
	require.Equal(t, int64(2), gotSub.ID)
}
