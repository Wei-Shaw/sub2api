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

// TestOpenAIChatCompletions_ImageModelRejectedBeforeScheduling 验证图像生成模型
// （如 gpt-image-2）在 /v1/chat/completions 上于选账号之前即被拒绝，归类为
// invalid_request，避免被原样发往上游 Codex 触发 400 + plan-gated 冷却级联限流。
func TestOpenAIChatCompletions_ImageModelRejectedBeforeScheduling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-image-2","messages":[{"role":"user","content":"draw a cat"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	groupID := int64(111)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      222,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID},
		User:    &service.User{ID: 333},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 333, Concurrency: 1})

	h := &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: &service.BillingCacheService{},
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   &ConcurrencyHelper{concurrencyService: &service.ConcurrencyService{}},
	}

	h.ChatCompletions(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Contains(t, rec.Body.String(), "/v1/images/generations")
}

// TestOpenAIChatCompletions_ImageModelClassifiesAsInvalidRequest 验证被拒绝的
// 图像模型请求经 ops 分类链路映射为用户侧 "invalid_request" 分类。
func TestOpenAIChatCompletions_ImageModelClassifiesAsInvalidRequest(t *testing.T) {
	phase := service.MapUserErrorCategory("request", "invalid_request_error")
	require.Equal(t, "invalid_request", phase)
}
