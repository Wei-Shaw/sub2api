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

func TestNormalizeGloballyDisabledImageGeneration(t *testing.T) {
	payload := []byte(`{"model":"gpt-5.5","input":[{"type":"message","role":"user","content":"hello"},{"type":"additional_tools","tools":[{"type":"namespace","name":"image_gen"}]}],"tools":[{"type":"function","name":"shell"},{"type":"image_generation"}],"tool_choice":{"type":"image_generation"}}`)
	h := &OpenAIGatewayHandler{cfg: &config.Config{DisableImageGeneration: true}}

	updated, changed, err := h.normalizeGloballyDisabledImageGeneration(payload)

	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, service.IsImageGenerationIntent("/v1/responses", "gpt-5.5", updated))
	require.True(t, gjson.GetBytes(updated, `tools.#(name=="shell")`).Exists())
	require.Equal(t, "hello", gjson.GetBytes(updated, "input.0.content").String())
}

func TestNormalizeGloballyDisabledImageGenerationNoOp(t *testing.T) {
	payload := []byte(`{"model":"gpt-5.5","tools":[{"type":"image_generation"}]}`)
	h := &OpenAIGatewayHandler{cfg: &config.Config{}}

	updated, changed, err := h.normalizeGloballyDisabledImageGeneration(payload)

	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, payload, updated)
}

func TestImageGenerationAllowedForGroup(t *testing.T) {
	allowedGroup := &service.Group{AllowImageGeneration: true}
	tests := []struct {
		name     string
		disabled bool
		want     bool
	}{
		{name: "existing group permission retained", disabled: false, want: true},
		{name: "global disable overrides allowed group", disabled: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &OpenAIGatewayHandler{cfg: &config.Config{DisableImageGeneration: tt.disabled}}
			require.Equal(t, tt.want, h.imageGenerationAllowedForGroup(allowedGroup))
		})
	}
}

func newGlobalDisableMediaContext(t *testing.T, path string, body string) (*httptest.ResponseRecorder, *gin.Context, *OpenAIGatewayHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	groupID := int64(111)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      222,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, AllowImageGeneration: true},
		User:    &service.User{ID: 333},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 333, Concurrency: 1})
	cfg := &config.Config{RunMode: config.RunModeSimple, DisableImageGeneration: true}
	h := &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil),
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   &ConcurrencyHelper{concurrencyService: &service.ConcurrencyService{}},
		cfg:                 cfg,
		imageLimiter:        &imageConcurrencyLimiter{},
	}
	return rec, c, h
}

func TestOpenAIGatewayHandlerImagesGlobalDisableRejectsAllowedGroup(t *testing.T) {
	rec, c, h := newGlobalDisableMediaContext(t, "/v1/images/generations", `{"model":"gpt-image-2","prompt":"draw"}`)

	h.Images(c)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
}

func TestGrokMediaGlobalDisableRejectsAllowedGroup(t *testing.T) {
	rec, c, h := newGlobalDisableMediaContext(t, "/v1/images/generations", `{"model":"grok-imagine","prompt":"draw"}`)

	h.GrokImages(c)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
}

func newGlobalImageDisableResponsesTestContext(t *testing.T, body string) (*httptest.ResponseRecorder, *gin.Context, *OpenAIGatewayHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	groupID := int64(1)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      10,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, AllowImageGeneration: false},
		User:    &service.User{ID: 20},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 20, Concurrency: 1})
	cfg := &config.Config{RunMode: config.RunModeSimple, DisableImageGeneration: true}
	h := &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil),
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   &ConcurrencyHelper{concurrencyService: service.NewConcurrencyService(&helperConcurrencyCacheStub{userSeq: []bool{true}})},
		cfg:                 cfg,
		imageLimiter:        &imageConcurrencyLimiter{},
	}
	return rec, c, h
}

func TestOpenAIGatewayHandlerResponsesGlobalDisableStripsBeforeGroupGate(t *testing.T) {
	body := `{"model":"gpt-5.5","input":"write code","tools":[{"type":"image_generation"}]}`
	rec, c, h := newGlobalImageDisableResponsesTestContext(t, body)

	h.Responses(c)

	require.NotEqual(t, http.StatusForbidden, rec.Code)
	require.NotContains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
}

func TestOpenAIGatewayHandlerResponsesGlobalDisableRejectsImageOnlyModel(t *testing.T) {
	body := `{"model":"gpt-image-2","input":"draw"}`
	rec, c, h := newGlobalImageDisableResponsesTestContext(t, body)

	h.Responses(c)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
}

func TestOpenAIGatewayHandlerResponsesGlobalDisableRejectsImageOnlyModelForAllowedGroup(t *testing.T) {
	body := `{"model":"gpt-image-2","input":"draw"}`
	rec, c, h := newGlobalImageDisableResponsesTestContext(t, body)
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	require.True(t, ok)
	apiKey.Group.AllowImageGeneration = true

	h.Responses(c)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
}
