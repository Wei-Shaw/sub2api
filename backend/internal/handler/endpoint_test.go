package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) REDACTED

// ──────────────────────────────────────────────────────────
// NormalizeInboundEndpoint
// ──────────────────────────────────────────────────────────

func TestNormalizeInboundEndpoint(t *testing.T) {
	tests := []struct {
		path string
		want string
REDACTED{
		// Direct canonical paths.
		{"/v1/messages", EndpointMessagesREDACTED,
		{"/v1/chat/completions", EndpointChatCompletionsREDACTED,
		{"/v1/embeddings", EndpointEmbeddingsREDACTED,
		{"/v1/responses", EndpointResponsesREDACTED,
		{"/v1/responses/compact", EndpointResponsesCompactREDACTED,
		{"/v1/responses/compact/detail", EndpointResponsesCompactREDACTED,
		{"/v1/images/generations", EndpointImagesGenerationsREDACTED,
		{"/v1/images/edits", EndpointImagesEditsREDACTED,
		{"/v1/videos/generations", EndpointVideosGenerationsREDACTED,
		{"/v1/videos/req_123", EndpointVideosREDACTED,
		{"/v1beta/models", EndpointGeminiModelsREDACTED,

		// Prefixed paths (antigravity, openai) — root Responses.
		{"/antigravity/v1/messages", EndpointMessagesREDACTED,
		{"/openai/v1/responses", EndpointResponsesREDACTED,
		{"/openai/v1/images/generations", EndpointImagesGenerationsREDACTED,
		{"/openai/v1/images/edits", EndpointImagesEditsREDACTED,
		{"/antigravity/v1beta/models/gemini:generateContent", EndpointGeminiModelsREDACTED,

		// Prefixed paths — "/responses/compact" is its OWN distinct
		// inbound endpoint, not folded into the root Responses endpoint.
		{"/openai/v1/responses/compact", EndpointResponsesCompactREDACTED,
		{"/openai/v1/responses/compact/detail", EndpointResponsesCompactREDACTED,

		// Bare top-level alias route "/responses" — root vs. compact.
		{"/responses", EndpointResponsesREDACTED,
		{"/responses/compact", EndpointResponsesCompactREDACTED,
		{"/responses/compact/detail", EndpointResponsesCompactREDACTED,

		// Bare Codex direct alias route — root vs. compact.
		{"/backend-api/codex/responses", EndpointResponsesREDACTED,
		{"/backend-api/codex/responses/compact", EndpointResponsesCompactREDACTED,
		{"/backend-api/codex/responses/compact/detail", EndpointResponsesCompactREDACTED,

		// Must NOT generalize to arbitrary paths merely ending in
		// "/responses" (or "/responses/compact") that are unrelated to
		// the two known bare alias roots, unless they already carry a
		// supported "/v1/responses..." prefix form.
		{"/foo/responses", "/foo/responses"REDACTED,
		{"/foo/responses/compact", "/foo/responses/compact"REDACTED,

		// Unknown path is returned as-is.
		{"/v1/embeddings", "/v1/embeddings"REDACTED,
		{"", ""REDACTED,
		{"  /v1/messages  ", EndpointMessagesREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeInboundEndpoint(tt.path))
	REDACTED)
REDACTED
REDACTED

// ──────────────────────────────────────────────────────────
// DeriveUpstreamEndpoint
// ──────────────────────────────────────────────────────────

func TestDeriveUpstreamEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		inbound  string
		rawPath  string
		platform string
		want     string
REDACTED{
		// Anthropic.
		{"anthropic messages", EndpointMessages, "/v1/messages", service.PlatformAnthropic, EndpointMessagesREDACTED,

		// Gemini.
		{"gemini models", EndpointGeminiModels, "/v1beta/models/gemini:gen", service.PlatformGemini, EndpointGeminiModelsREDACTED,

		// OpenAI — root Responses.
		{"openai responses root", EndpointResponses, "/v1/responses", service.PlatformOpenAI, EndpointResponsesREDACTED,

		// OpenAI — compact, raw path carries the derivable "/compact"
		// (or nested) suffix, which must be preserved on the upstream
		// endpoint.
		{"openai responses compact", EndpointResponsesCompact, "/openai/v1/responses/compact", service.PlatformOpenAI, "/v1/responses/compact"REDACTED,
		{"openai responses nested", EndpointResponsesCompact, "/openai/v1/responses/compact/detail", service.PlatformOpenAI, "/v1/responses/compact/detail"REDACTED,
		{"openai bare responses compact", EndpointResponsesCompact, "/responses/compact", service.PlatformOpenAI, "/v1/responses/compact"REDACTED,
		{"openai bare responses compact detail", EndpointResponsesCompact, "/responses/compact/detail", service.PlatformOpenAI, "/v1/responses/compact/detail"REDACTED,
		{"openai codex direct responses compact", EndpointResponsesCompact, "/backend-api/codex/responses/compact", service.PlatformOpenAI, "/v1/responses/compact"REDACTED,
		{"openai codex direct responses compact detail", EndpointResponsesCompact, "/backend-api/codex/responses/compact/detail", service.PlatformOpenAI, "/v1/responses/compact/detail"REDACTED,

		// OpenAI — bare root alias routes normalize to root Responses.
		{"openai bare responses", EndpointResponses, "/responses", service.PlatformOpenAI, EndpointResponsesREDACTED,
		{"openai codex direct responses", EndpointResponses, "/backend-api/codex/responses", service.PlatformOpenAI, EndpointResponsesREDACTED,

		// OpenAI — inbound is already the canonical compact endpoint but
		// the raw path carries no derivable "/responses..." suffix (e.g.
		// it was already normalized upstream). Must not silently fall
		// back to the root Responses endpoint.
		{"openai responses compact inbound only, unrelated raw path", EndpointResponsesCompact, "/v1/messages", service.PlatformOpenAI, EndpointResponsesCompactREDACTED,

		{"openai from messages", EndpointMessages, "/v1/messages", service.PlatformOpenAI, EndpointResponsesREDACTED,
		{"openai from completions", EndpointChatCompletions, "/v1/chat/completions", service.PlatformOpenAI, EndpointResponsesREDACTED,
		{"openai embeddings", EndpointEmbeddings, "/v1/embeddings", service.PlatformOpenAI, EndpointEmbeddingsREDACTED,
		{"openai image generations", EndpointImagesGenerations, "/v1/images/generations", service.PlatformOpenAI, EndpointImagesGenerationsREDACTED,
		{"openai image edits", EndpointImagesEdits, "/openai/v1/images/edits", service.PlatformOpenAI, EndpointImagesEditsREDACTED,
		{"grok video generations", EndpointVideosGenerations, "/v1/videos/generations", service.PlatformGrok, EndpointVideosGenerationsREDACTED,
		{"grok video status", EndpointVideos, "/videos/req_123", service.PlatformGrok, EndpointVideosREDACTED,

		// Antigravity — uses inbound to pick Claude vs Gemini upstream.
		{"antigravity claude", EndpointMessages, "/antigravity/v1/messages", service.PlatformAntigravity, EndpointMessagesREDACTED,
		{"antigravity gemini", EndpointGeminiModels, "/antigravity/v1beta/models", service.PlatformAntigravity, EndpointGeminiModelsREDACTED,

		// Unknown platform — passthrough.
		{"unknown platform", "/v1/embeddings", "/v1/embeddings", "unknown", "/v1/embeddings"REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, DeriveUpstreamEndpoint(tt.inbound, tt.rawPath, tt.platform))
	REDACTED)
REDACTED
REDACTED

// ──────────────────────────────────────────────────────────
// responsesSubpathSuffix
// ──────────────────────────────────────────────────────────

func TestResponsesSubpathSuffix(t *testing.T) {
	tests := []struct {
		raw  string
		want string
REDACTED{
		{"/v1/responses", ""REDACTED,
		{"/v1/responses/", ""REDACTED,
		{"/v1/responses/compact", "/compact"REDACTED,
		{"/openai/v1/responses/compact/detail", "/compact/detail"REDACTED,
		{"/responses", ""REDACTED,
		{"/responses/compact", "/compact"REDACTED,
		{"/responses/compact/detail", "/compact/detail"REDACTED,
		{"/backend-api/codex/responses", ""REDACTED,
		{"/backend-api/codex/responses/compact", "/compact"REDACTED,
		{"/backend-api/codex/responses/compact/detail", "/compact/detail"REDACTED,
		{"/v1/messages", ""REDACTED,
		{"", ""REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			require.Equal(t, tt.want, responsesSubpathSuffix(tt.raw))
	REDACTED)
REDACTED
REDACTED

// ──────────────────────────────────────────────────────────
// InboundEndpointMiddleware + context helpers
// ──────────────────────────────────────────────────────────

func TestInboundEndpointMiddleware(t *testing.T) {
	router := gin.New()
	router.Use(InboundEndpointMiddleware())

	var captured string
	router.POST("/v1/messages", func(c *gin.Context) {
		captured = GetInboundEndpoint(c)
		c.Status(http.StatusOK)
REDACTED)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, EndpointMessages, captured)
REDACTED

func TestGetInboundEndpoint_FallbackWithoutMiddleware(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/antigravity/v1/messages", nil)

	// Middleware did not run — fallback to normalizing c.Request.URL.Path.
	got := GetInboundEndpoint(c)
	require.Equal(t, EndpointMessages, got)
REDACTED

// TestInboundEndpointMiddleware_WildcardRoutes verifies that, when a
// gateway route is registered with a Gin wildcard pattern (e.g.
// "/v1/responses/*subpath"), InboundEndpointMiddleware normalizes based
// on the concrete request path (c.Request.URL.Path) rather than the
// route pattern (c.FullPath()). Using c.FullPath() here would collapse
// every request under the wildcard — including "/v1/responses/compact"
// — down to the literal pattern string, which never matches the
// "compact" alias detection and would incorrectly normalize to the root
// Responses endpoint.
func TestInboundEndpointMiddleware_WildcardRoutes(t *testing.T) {
	tests := []struct {
		name        string
		routePath   string
		requestPath string
		want        string
REDACTED{
		{
			name:        "v1 responses wildcard route, compact request",
			routePath:   "/v1/responses/*subpath",
			requestPath: "/v1/responses/compact",
			want:        EndpointResponsesCompact,
	REDACTED,
		{
			name:        "bare responses wildcard route, compact request",
			routePath:   "/responses/*subpath",
			requestPath: "/responses/compact",
			want:        EndpointResponsesCompact,
	REDACTED,
		{
			name:        "codex direct wildcard route, compact request",
			routePath:   "/backend-api/codex/responses/*subpath",
			requestPath: "/backend-api/codex/responses/compact",
			want:        EndpointResponsesCompact,
	REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(InboundEndpointMiddleware())

			var captured string
			router.POST(tt.routePath, func(c *gin.Context) {
				captured = GetInboundEndpoint(c)
				c.Status(http.StatusOK)
		REDACTED)

			req := httptest.NewRequest(http.MethodPost, tt.requestPath, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, tt.want, captured)
	REDACTED)
REDACTED
REDACTED

// TestGetInboundEndpoint_FallbackWildcardRouteWithoutMiddleware verifies
// that when InboundEndpointMiddleware did NOT run (so no value is stored
// in gin.Context), the GetInboundEndpoint fallback path still prefers
// c.Request.URL.Path over c.FullPath(). This guards against the fallback
// regressing to prefer c.FullPath() again, which would misnormalize
// concrete requests matched by a wildcard route pattern (e.g.
// "/v1/responses/*subpath" matching "/v1/responses/compact") down to
// the root Responses endpoint.
func TestGetInboundEndpoint_FallbackWildcardRouteWithoutMiddleware(t *testing.T) {
	router := gin.New()
	// Deliberately do NOT register InboundEndpointMiddleware.

	var captured string
	router.POST("/v1/responses/*subpath", func(c *gin.Context) {
		// Sanity check: FullPath returns the route pattern, not the
		// concrete request path, when a wildcard route matches.
		require.Equal(t, "/v1/responses/*subpath", c.FullPath())
		captured = GetInboundEndpoint(c)
		c.Status(http.StatusOK)
REDACTED)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, EndpointResponsesCompact, captured)
REDACTED

func TestGetUpstreamEndpoint_FullFlow(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses/compact", nil)

	// Simulate middleware.
	c.Set(ctxKeyInboundEndpoint, NormalizeInboundEndpoint(c.Request.URL.Path))

	got := GetUpstreamEndpoint(c, service.PlatformOpenAI)
	require.Equal(t, "/v1/responses/compact", got)
REDACTED
