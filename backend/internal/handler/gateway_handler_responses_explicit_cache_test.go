package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGatewayHandlerResponsesRejectsGPT56ExplicitCacheBeforeFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"hello","explicit_cache":{"ttl":"30m"}}`)
	c, rec := newResponsesExplicitCacheTestContext(t, body)

	handler := &GatewayHandler{}
	require.NotPanics(t, func() {
		handler.Responses(c)
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	responseBody := rec.Body.Bytes()
	require.Equal(t, "invalid_request_error", gjson.GetBytes(responseBody, "error.type").String())
	require.Equal(t, "unsupported_parameter", gjson.GetBytes(responseBody, "error.code").String())
	require.Equal(t, "explicit_cache", gjson.GetBytes(responseBody, "error.param").String())
	require.Contains(t, gjson.GetBytes(responseBody, "error.message").String(), "explicit_cache")
}

func newResponsesExplicitCacheTestContext(t *testing.T, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	groupID := int64(3131)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      99,
		GroupID: &groupID,
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
		},
		User: &service.User{ID: 100},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 100, Concurrency: 0})
	return c, rec
}
