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
		{"/v1/images/generations", EndpointImagesGenerationsREDACTED,
		{"/v1/images/edits", EndpointImagesEditsREDACTED,
		{"/v1/videos/generations", EndpointVideosGenerationsREDACTED,
		{"/v1/videos/req_123", EndpointVideosREDACTED,
		{"/v1beta/models", EndpointGeminiModelsREDACTED,

		// Prefixed paths (antigravity, openai).
		{"/antigravity/v1/messages", EndpointMessagesREDACTED,
		{"/openai/v1/responses", EndpointResponsesREDACTED,
		{"/openai/v1/responses/compact", EndpointResponsesREDACTED,
		{"/openai/v1/images/generations", EndpointImagesGenerationsREDACTED,
		{"/openai/v1/images/edits", EndpointImagesEditsREDACTED,
		{"/antigravity/v1beta/models/gemini:generateContent", EndpointGeminiModelsREDACTED,

		// Gin route patterns with wildcards.
		{"/v1beta/models/*modelAction", EndpointGeminiModelsREDACTED,
		{"/v1/responses/*subpath", EndpointResponsesREDACTED,

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

		// OpenAI — always /v1/responses.
		{"openai responses root", EndpointResponses, "/v1/responses", service.PlatformOpenAI, EndpointResponsesREDACTED,
		{"openai responses compact", EndpointResponses, "/openai/v1/responses/compact", service.PlatformOpenAI, "/v1/responses/compact"REDACTED,
		{"openai responses nested", EndpointResponses, "/openai/v1/responses/compact/detail", service.PlatformOpenAI, "/v1/responses/compact/detail"REDACTED,
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

func TestGetUpstreamEndpoint_FullFlow(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses/compact", nil)

	// Simulate middleware.
	c.Set(ctxKeyInboundEndpoint, NormalizeInboundEndpoint(c.Request.URL.Path))

	got := GetUpstreamEndpoint(c, service.PlatformOpenAI)
	require.Equal(t, "/v1/responses/compact", got)
REDACTED
