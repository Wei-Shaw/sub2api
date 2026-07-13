package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIResponsesAdmissionQueueFullPreservesHTTPResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","input":"hello"}`))
	groupID := int64(1)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      10,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID},
		User:    &service.User{ID: 20},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 20, Concurrency: 1})

	cache := &helperConcurrencyCacheStub{
		userSeq:     []bool{false},
		waitAllowed: false,
	}
	h := &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: &service.BillingCacheService{},
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 0),
		cfg:                 &config.Config{},
	}

	h.Responses(c)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "rate_limit_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
	require.Contains(t, recorder.Body.String(), "Too many pending requests, please retry later")
	require.Equal(t, 1, cache.userAcquireCalls)
	require.Equal(t, 1, cache.waitIncrementCalls)
	require.Equal(t, 0, cache.apiKeyTrackCalls)
}
