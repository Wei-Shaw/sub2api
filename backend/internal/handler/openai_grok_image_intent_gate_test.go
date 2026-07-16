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

func TestOpenAIGatewayHandlerResponses_GrokPassiveImageToolDeclarationBypassesPermissionGate(t *testing.T) {
	body := `{"model":"grok-4.5","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"REDACTED]REDACTED],"tool_choice":"auto","input":"write code"REDACTED`
	rec := runOpenAIResponsesImagePermissionGateTest(t, service.PlatformGrok, body)

	require.NotEqual(t, http.StatusForbidden, rec.Code)
	require.NotContains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
REDACTED

func TestOpenAIGatewayHandlerResponses_GrokResponsesLiteImageToolDeclarationBypassesPermissionGate(t *testing.T) {
	body := `{"model":"grok-4.5","tool_choice":"auto","input":[{"type":"additional_tools","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"REDACTED]REDACTED]REDACTED,{"type":"message","role":"user","content":"write code"REDACTED]REDACTED`
	rec := runOpenAIResponsesImagePermissionGateTest(t, service.PlatformGrok, body)

	require.NotEqual(t, http.StatusForbidden, rec.Code)
	require.NotContains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
REDACTED

func TestOpenAIGatewayHandlerResponses_ImagePermissionHardSignalsStillRejected(t *testing.T) {
	passiveNamespace := `{"model":"gpt-5.5","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"REDACTED]REDACTED],"tool_choice":"auto","input":"write code"REDACTED`
	tests := []struct {
		name     string
		platform string
		body     string
REDACTED{
		{
			name:     "OpenAI keeps declaration semantics",
			platform: service.PlatformOpenAI,
			body:     passiveNamespace,
	REDACTED,
		{
			name:     "Grok native image_generation declaration",
			platform: service.PlatformGrok,
			body:     `{"model":"grok-4.5","tools":[{"type":"image_generation"REDACTED],"input":"draw"REDACTED`,
	REDACTED,
		{
			name:     "Grok explicit image_gen tool choice",
			platform: service.PlatformGrok,
			body:     `{"model":"grok-4.5","tools":[{"type":"namespace","name":"image_gen"REDACTED],"tool_choice":{"type":"namespace","name":"image_gen"REDACTED,"input":"draw"REDACTED`,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := runOpenAIResponsesImagePermissionGateTest(t, tt.platform, tt.body)

			require.Equal(t, http.StatusForbidden, rec.Code)
			require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
	REDACTED)
REDACTED
REDACTED

func runOpenAIResponsesImagePermissionGateTest(t *testing.T, platform string, body string) *httptest.ResponseRecorder {
REDACTED
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(6301)
	userID := int64(6302)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      6303,
		GroupID: &groupID,
		Group: &service.Group{
			ID:                   groupID,
			Platform:             platform,
			AllowImageGeneration: false,
	REDACTED,
		User: &service.User{ID: userID, Status: service.StatusActiveREDACTED,
REDACTED)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: userID, Concurrency: 1REDACTED)

	h := &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{REDACTED,
		billingCacheService: service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeSimpleREDACTED, nil),
		apiKeyService:       &service.APIKeyService{REDACTED,
		concurrencyHelper: &ConcurrencyHelper{concurrencyService: service.NewConcurrencyService(
			&helperConcurrencyCacheStub{userSeq: []bool{trueREDACTEDREDACTED,
		)REDACTED,
		cfg:          &config.Config{REDACTED,
		imageLimiter: &imageConcurrencyLimiter{REDACTED,
REDACTED

	h.Responses(c)
	return rec
REDACTED
