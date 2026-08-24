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
)

// 非法 service_tier 必须在两个 OpenAI 端点（/v1/responses、/v1/chat/completions）
// 上以 OpenAI 兼容错误结构返回 HTTP 400；合法值（fast/priority）不被拒绝。

func newServiceTierHandlerTest(t *testing.T) *OpenAIGatewayHandler {
REDACTED
	return &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{REDACTED,
		billingCacheService: service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeSimpleREDACTED, nil),
		apiKeyService:       &service.APIKeyService{REDACTED,
		concurrencyHelper: &ConcurrencyHelper{concurrencyService: service.NewConcurrencyService(
			&helperConcurrencyCacheStub{userSeq: []bool{trueREDACTEDREDACTED,
		)REDACTED,
		cfg:          &config.Config{REDACTED,
		imageLimiter: &imageConcurrencyLimiter{REDACTED,
REDACTED
REDACTED

func runOpenAIHandlerServiceTierTest(t *testing.T, path, body string, handler func(h *OpenAIGatewayHandler, c *gin.Context)) *httptest.ResponseRecorder {
REDACTED
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(6401)
	userID := int64(6402)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      6403,
		GroupID: &groupID,
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
	REDACTED,
		User: &service.User{ID: userID, Status: service.StatusActiveREDACTED,
REDACTED)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: userID, Concurrency: 1REDACTED)

	handler(newServiceTierHandlerTest(t), c)
	return rec
REDACTED

func TestOpenAIGatewayHandlerResponses_InvalidServiceTierRejected400(t *testing.T) {
	for _, body := range []string{
		`{"model":"gpt-5.5","input":"hi","service_tier":"turbo"REDACTED`,
		`{"model":"gpt-5.5","input":"hi","service_tier":"SPEED"REDACTED`,
		`{"model":"gpt-5.5","input":"hi","service_tier":""REDACTED`,
		`{"model":"gpt-5.5","input":"hi","service_tier":123REDACTED`,
		`{"model":"gpt-5.5","input":"hi","service_tier":{REDACTEDREDACTED`,
REDACTED {
		rec := runOpenAIHandlerServiceTierTest(t, "/v1/responses", body, func(h *OpenAIGatewayHandler, c *gin.Context) {
			h.Responses(c)
	REDACTED)
		require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", body)
		require.Contains(t, rec.Body.String(), "invalid_request_error", "body=%s", body)
		require.Contains(t, rec.Body.String(), "invalid service_tier", "body=%s", body)
REDACTED
REDACTED

func TestOpenAIGatewayHandlerResponses_ValidServiceTierNotRejected(t *testing.T) {
	for _, tier := range []string{"fast", "priority", "flex"REDACTED {
		body := `{"model":"gpt-5.5","input":"hi","service_tier":"` + tier + `"REDACTED`
		rec := runOpenAIHandlerServiceTierTest(t, "/v1/responses", body, func(h *OpenAIGatewayHandler, c *gin.Context) {
			h.Responses(c)
	REDACTED)
		require.NotEqual(t, http.StatusBadRequest, rec.Code, "tier=%s must not be rejected as invalid", tier)
		require.NotContains(t, rec.Body.String(), "invalid service_tier", "tier=%s", tier)
REDACTED
REDACTED

func TestOpenAIGatewayHandlerResponses_ServiceTierOmittedKeepsCurrentBehavior(t *testing.T) {
	body := `{"model":"gpt-5.5","input":"hi"REDACTED`
	rec := runOpenAIHandlerServiceTierTest(t, "/v1/responses", body, func(h *OpenAIGatewayHandler, c *gin.Context) {
		h.Responses(c)
REDACTED)
	require.NotContains(t, rec.Body.String(), "invalid service_tier")
REDACTED

func TestOpenAIGatewayHandlerChatCompletions_InvalidServiceTierRejected400(t *testing.T) {
	for _, body := range []string{
		`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"REDACTED],"service_tier":"turbo"REDACTED`,
		`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"REDACTED],"service_tier":"ultra"REDACTED`,
		`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"REDACTED],"service_tier":""REDACTED`,
		`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"REDACTED],"service_tier":["priority"]REDACTED`,
REDACTED {
		rec := runOpenAIHandlerServiceTierTest(t, "/v1/chat/completions", body, func(h *OpenAIGatewayHandler, c *gin.Context) {
			h.ChatCompletions(c)
	REDACTED)
		require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", body)
		require.Contains(t, rec.Body.String(), "invalid_request_error", "body=%s", body)
		require.Contains(t, rec.Body.String(), "invalid service_tier", "body=%s", body)
REDACTED
REDACTED

func TestOpenAIGatewayHandlerChatCompletions_ValidServiceTierNotRejected(t *testing.T) {
	for _, tier := range []string{"fast", "priority", "auto", "default", "scale", "flex"REDACTED {
		body := `{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"REDACTED],"service_tier":"` + tier + `"REDACTED`
		rec := runOpenAIHandlerServiceTierTest(t, "/v1/chat/completions", body, func(h *OpenAIGatewayHandler, c *gin.Context) {
			h.ChatCompletions(c)
	REDACTED)
		require.NotEqual(t, http.StatusBadRequest, rec.Code, "tier=%q must not be rejected as invalid", tier)
		require.NotContains(t, rec.Body.String(), "invalid service_tier", "tier=%q", tier)
REDACTED
REDACTED

func TestOpenAIGatewayHandlerChatCompletions_ServiceTierOmittedKeepsCurrentBehavior(t *testing.T) {
	body := `{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"REDACTED]REDACTED`
	rec := runOpenAIHandlerServiceTierTest(t, "/v1/chat/completions", body, func(h *OpenAIGatewayHandler, c *gin.Context) {
		h.ChatCompletions(c)
REDACTED)
	require.NotContains(t, rec.Body.String(), "invalid service_tier")
REDACTED
