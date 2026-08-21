package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newGatewayRoutesTestRouter(platform ...string) *gin.Engine {
	return newGatewayRoutesTestRouterWithConfig(&config.Config{
		Gateway: config.GatewayConfig{
			MaxBodySize:     1024 * 1024,
			TextMaxBodySize: 1024 * 1024,
	REDACTED,
REDACTED, platform...)
REDACTED

func newGatewayRoutesTestRouterWithConfig(cfg *config.Config, platform ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	groupPlatform := service.PlatformOpenAI
	if len(platform) > 0 && platform[0] != "" {
		groupPlatform = platform[0]
REDACTED
	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       &handler.GatewayHandler{REDACTED,
			OpenAIGateway: &handler.OpenAIGatewayHandler{REDACTED,
			AsyncImage:    handler.NewAsyncImageHandler(nil, nil),
	REDACTED,
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			groupID := int64(1)
			c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
				GroupID: &groupID,
				Group:   &service.Group{Platform: groupPlatformREDACTED,
		REDACTED)
			c.Next()
	REDACTED),
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
	)

	return router
REDACTED

func TestGatewayRoutesOpenAIResponsesCompactPathIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, path := range []string{
		"/v1/responses/compact",
		"/responses/compact",
		"/backend-api/codex/responses",
		"/backend-api/codex/responses/compact",
REDACTED {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5"REDACTED`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit OpenAI responses handler", path)
REDACTED
REDACTED

func TestGatewayRoutesOpenAIAlphaSearchPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()
	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		if route.Method == http.MethodPost {
			registered[route.Path] = true
	REDACTED
REDACTED

	for _, path := range []string{
		"/v1/alpha/search",
		"/alpha/search",
		"/backend-api/codex/alpha/search",
REDACTED {
		require.True(t, registered[path], "POST %s should be registered", path)
REDACTED
REDACTED

func TestGatewayRoutesAlphaSearchRejectsUnsupportedGroup(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformGrok)
	req := httptest.NewRequest(http.MethodPost, "/v1/alpha/search", strings.NewReader(`{"model":"gpt-5.6-sol"REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "only available for OpenAI and Composite groups")
REDACTED

func TestGatewayRoutesOpenAIImagesPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, path := range []string{
		"/v1/images/generations",
		"/v1/images/edits",
		"/images/generations",
		"/images/edits",
REDACTED {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-image-2","prompt":"draw a cat"REDACTED`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit OpenAI images handler", path)
REDACTED
REDACTED

func TestGatewayRoutesAsyncImagesPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()
	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
REDACTED

	for _, route := range []string{
		"POST /v1/images/generations/async",
		"POST /v1/images/edits/async",
		"GET /v1/images/tasks/:task_id",
		"POST /images/generations/async",
		"POST /images/edits/async",
		"GET /images/tasks/:task_id",
REDACTED {
		require.True(t, registered[route], "%s should be registered", route)
REDACTED
REDACTED

func TestGatewayRoutesGrokImagesAndVideosPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformGrok)

	for _, path := range []string{
		"/v1/images/generations",
		"/v1/images/edits",
		"/images/generations",
		"/images/edits",
		"/v1/videos/generations",
		"/v1/videos",
		"/videos",
		"/videos/generations",
		"/v1/videos/edits",
		"/videos/edits",
		"/v1/videos/extensions",
		"/videos/extensions",
REDACTED {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"grok-imagine","prompt":"draw a cat"REDACTED`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit Grok media handler", path)
		require.NotContains(t, w.Body.String(), "not supported for this platform")
REDACTED

	for _, path := range []string{
		"/v1/videos/request-123",
		"/videos/request-123",
		"/v1/videos/generations/request-123",
		"/videos/generations/request-123",
		"/v1/videos/edits/request-123",
		"/videos/edits/request-123",
		"/v1/videos/extensions/request-123",
		"/videos/extensions/request-123",
		"/v1/videos/request-123/content",
		"/videos/request-123/content",
		"/v1/videos/generations/request-123/content",
		"/videos/generations/request-123/content",
		"/v1/videos/edits/request-123/content",
		"/videos/edits/request-123/content",
		"/v1/videos/extensions/request-123/content",
		"/videos/extensions/request-123/content",
REDACTED {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit Grok video handler", path)
		require.NotContains(t, w.Body.String(), "not supported for this platform")
REDACTED
REDACTED

func TestGatewayRoutesGrokCustomVoiceCRUDPathsAreRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformGrok)
	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
REDACTED
	for _, route := range []string{
		"POST /v1/custom-voices",
		"GET /v1/custom-voices",
		"GET /v1/custom-voices/:voice_id",
		"PATCH /v1/custom-voices/:voice_id",
		"DELETE /v1/custom-voices/:voice_id",
		"GET /v1/custom-voices/:voice_id/audio",
		"POST /custom-voices",
		"GET /custom-voices",
		"GET /custom-voices/:voice_id",
		"PATCH /custom-voices/:voice_id",
		"DELETE /custom-voices/:voice_id",
		"GET /custom-voices/:voice_id/audio",
REDACTED {
		require.True(t, registered[route], "%s should be registered", route)
REDACTED
REDACTED

func TestGrokCustomVoiceEndpointUsesRouteTemplateNotRawPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	var got string
	capture := func(c *gin.Context) {
		got = grokCustomVoiceEndpoint(c)
		c.Status(http.StatusOK)
REDACTED
	router.GET("/v1/custom-voices/:voice_id/audio", capture)
	router.GET("/v1/custom-voices/:voice_id", capture)

	for _, tc := range []struct {
		path string
		want string
REDACTED{
		{path: "/v1/custom-voices/voice-123", want: "custom-voices/voice-123"REDACTED,
		{path: "/v1/custom-voices/voice-123/audio", want: "custom-voices/voice-123/audio"REDACTED,
		// A voice literally named "audio" matches /:voice_id, not /:voice_id/audio.
		// A raw-path suffix check would turn this profile lookup into an audio download.
		{path: "/v1/custom-voices/audio", want: "custom-voices/audio"REDACTED,
		{path: "/v1/custom-voices/audio/audio", want: "custom-voices/audio/audio"REDACTED,
REDACTED {
		got = ""
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "path=%s", tc.path)
		require.Equal(t, tc.want, got, "path=%s", tc.path)
REDACTED
REDACTED

func TestGatewayRoutesCompositeVideoLookupsUseGrokHandler(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformComposite)

	for _, path := range []string{
		"/v1/videos/request-123",
		"/videos/request-123",
		"/v1/videos/request-123/content",
		"/videos/request-123/content",
REDACTED {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit Grok video lookup handler", path)
		require.NotContains(t, w.Body.String(), "not supported for this platform")
REDACTED
REDACTED

func TestGatewayRoutesCompositeMessagesWithGrokModelUsesOpenAIGateway(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformComposite)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"grok-4.3","messages":[{"role":"user","content":"hi"REDACTED]REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.NotEqual(t, http.StatusNotFound, w.Code)
	require.NotContains(t, w.Body.String(), "not supported")
	require.NotContains(t, w.Body.String(), "OpenAI-compatible endpoint")
	require.NotContains(t, w.Body.String(), "composite groups")
REDACTED

func TestGatewayRoutesCompositeChatCompletionsWithGrokModelUsesOpenAIGateway(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformComposite)

	for _, path := range []string{"/v1/chat/completions", "/chat/completions"REDACTED {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"grok-4.3","messages":[{"role":"user","content":"hi"REDACTED]REDACTED`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s", path)
		require.NotContains(t, w.Body.String(), "not supported")
		require.NotContains(t, w.Body.String(), "OpenAI-compatible endpoint")
		require.NotContains(t, w.Body.String(), "composite groups")
REDACTED
REDACTED

func TestGatewayRoutesNonGrokVideosAreRejectedAtPlatformGate(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformOpenAI)

	for _, tc := range []struct {
		method string
		path   string
		body   string
REDACTED{
		{http.MethodPost, "/v1/videos/generations", `{"model":"grok-imagine-video-1.5","prompt":"waves"REDACTED`REDACTED,
		{http.MethodPost, "/v1/videos", `{"model":"grok-imagine-video-1.5","prompt":"waves"REDACTED`REDACTED,
		{http.MethodPost, "/videos", `{"model":"grok-imagine-video-1.5","prompt":"waves"REDACTED`REDACTED,
		{http.MethodPost, "/videos/generations", `{"model":"grok-imagine-video-1.5","prompt":"waves"REDACTED`REDACTED,
		{http.MethodPost, "/v1/videos/edits", `{"model":"grok-imagine-video","prompt":"waves","video":{"url":"https://example.com/in.mp4"REDACTEDREDACTED`REDACTED,
		{http.MethodPost, "/videos/edits", `{"model":"grok-imagine-video","prompt":"waves","video":{"url":"https://example.com/in.mp4"REDACTEDREDACTED`REDACTED,
		{http.MethodPost, "/v1/videos/extensions", `{"model":"grok-imagine-video","prompt":"waves","video":{"url":"https://example.com/in.mp4"REDACTEDREDACTED`REDACTED,
		{http.MethodPost, "/videos/extensions", `{"model":"grok-imagine-video","prompt":"waves","video":{"url":"https://example.com/in.mp4"REDACTEDREDACTED`REDACTED,
		{http.MethodGet, "/v1/videos/request-123", ""REDACTED,
		{http.MethodGet, "/videos/request-123", ""REDACTED,
		{http.MethodGet, "/v1/videos/generations/request-123", ""REDACTED,
		{http.MethodGet, "/videos/generations/request-123", ""REDACTED,
		{http.MethodGet, "/v1/videos/edits/request-123", ""REDACTED,
		{http.MethodGet, "/videos/edits/request-123", ""REDACTED,
		{http.MethodGet, "/v1/videos/extensions/request-123", ""REDACTED,
		{http.MethodGet, "/videos/extensions/request-123", ""REDACTED,
		{http.MethodGet, "/v1/videos/request-123/content", ""REDACTED,
		{http.MethodGet, "/videos/request-123/content", ""REDACTED,
		{http.MethodGet, "/v1/videos/generations/request-123/content", ""REDACTED,
		{http.MethodGet, "/videos/generations/request-123/content", ""REDACTED,
		{http.MethodGet, "/v1/videos/edits/request-123/content", ""REDACTED,
		{http.MethodGet, "/videos/edits/request-123/content", ""REDACTED,
		{http.MethodGet, "/v1/videos/extensions/request-123/content", ""REDACTED,
		{http.MethodGet, "/videos/extensions/request-123/content", ""REDACTED,
REDACTED {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code, "method=%s path=%s", tc.method, tc.path)
		require.Contains(t, w.Body.String(), "Videos API is not supported for this platform")
REDACTED
REDACTED

func TestGatewayRoutesCompositeVideoGenerationAllowed(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformComposite)

	req := httptest.NewRequest(http.MethodPost, "/v1/videos/generations", strings.NewReader(`{"model":"grok-imagine-video-1.5","prompt":"waves"REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.NotEqual(t, http.StatusNotFound, w.Code)
	require.NotContains(t, w.Body.String(), "not supported")
REDACTED

func TestGatewayRoutesCompositeOpenAIOnlyEndpointsRequireOpenAITarget(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformComposite)

	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"gemini-2.5-pro","input":"hello"REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"text-embedding-3-small","input":"hello"REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.NotEqual(t, http.StatusNotFound, w.Code)
REDACTED

func TestGatewayRoutesGrokAllowsCLICompatibilityEntrypoints(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformGrok)

	for _, tc := range []struct {
		method string
		path   string
REDACTED{
		{http.MethodPost, "/v1/messages"REDACTED,
		{http.MethodPost, "/v1/chat/completions"REDACTED,
		{http.MethodPost, "/chat/completions"REDACTED,
		{http.MethodGet, "/v1/responses"REDACTED,
		{http.MethodGet, "/responses"REDACTED,
		{http.MethodGet, "/backend-api/codex/responses"REDACTED,
REDACTED {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"model":"grok"REDACTED`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "method=%s path=%s", tc.method, tc.path)
		require.NotContains(t, w.Body.String(), "not supported for Grok groups")
REDACTED

	countTokensRouter := newGatewayRoutesTestRouterWithConfig(&config.Config{
		Gateway: config.GatewayConfig{MaxBodySize: 1024 * 1024REDACTED,
REDACTED, service.PlatformGrok)
	for _, path := range []string{"/v1/messages/count_tokens", "/messages/count_tokens"REDACTED {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"grok","messages":[{"role":"user","content":"hi"REDACTED]REDACTED`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		countTokensRouter.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "path=%s", path)
		var response struct {
			InputTokens int `json:"input_tokens"`
	REDACTED
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response), "path=%s", path)
		require.Positive(t, response.InputTokens, "path=%s", path)
REDACTED

	for _, path := range []string{
		"/v1/responses",
		"/responses",
		"/backend-api/codex/responses",
REDACTED {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"grok","input":"hi"REDACTED`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should still reach Responses handler", path)
REDACTED
REDACTED

// TestGatewayRoutesResponsesSubpathRejectsNonConformingSubpaths 端到端锁定不变式：
// /responses/*subpath 的子路径会被转发到上游同名端点之后，因此不合规的子路径必须
// 在入口就被拒绝，不得进入调度与转发流程。
func TestGatewayRoutesResponsesSubpathRejectsNonConformingSubpaths(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	for _, path := range []string{
		"/v1/responses/../../x/y",
		"/v1/responses/..%2f..%2fx/y",
		"/v1/responses/%2e%2e/%2e%2e/x",
		"/responses/%2e%2e%2fx",
		"/backend-api/codex/responses/..%2f..%2fx",
		`/v1/responses/..\..\x`,
		"/v1/responses/%3fa=b",
		"/v1/responses/x%23frag",
		"/v1/responses/compact%2f..",
REDACTED {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5"REDACTED`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code, "path=%s must be rejected at the edge", path)
		require.Contains(t, w.Body.String(), "Unsupported responses subpath", "path=%s", path)
REDACTED
REDACTED

func TestGatewayRoutesOpenAICountTokensPathIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformOpenAI)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"REDACTED]REDACTED`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	require.NotEqual(t, http.StatusNotFound, w.Code)
REDACTED
