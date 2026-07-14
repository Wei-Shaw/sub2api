package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildVertexGeminiURL(t *testing.T) {
	got, err := buildVertexGeminiURL("my-project", "us-central1", "gemini-3-pro", "streamGenerateContent", true)
REDACTED
	require.Equal(t, "https://us-central1-aiplatform.googleapis.com/v1/projects/my-project/locations/us-central1/publishers/google/models/gemini-3-pro:streamGenerateContent?alt=sse", got)
REDACTED

func TestBuildVertexGeminiURLUsesGlobalEndpointHost(t *testing.T) {
	got, err := buildVertexGeminiURL("my-project", "global", "gemini-3-flash-preview", "streamGenerateContent", true)
REDACTED
	require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/my-project/locations/global/publishers/google/models/gemini-3-flash-preview:streamGenerateContent?alt=sse", got)
REDACTED

func TestBuildVertexAnthropicURL(t *testing.T) {
	got, err := buildVertexAnthropicURL("my-project", "us-east5", "claude-sonnet-4-5@20250929", false)
REDACTED
	require.Equal(t, "https://us-east5-aiplatform.googleapis.com/v1/projects/my-project/locations/us-east5/publishers/anthropic/models/claude-sonnet-4-5@20250929:rawPredict", got)
REDACTED

func TestBuildVertexAnthropicURLUsesGlobalEndpointHost(t *testing.T) {
	got, err := buildVertexAnthropicURL("my-project", "global", "claude-haiku-4-5@20251001", true)
REDACTED
	require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/my-project/locations/global/publishers/anthropic/models/claude-haiku-4-5@20251001:streamRawPredict", got)
REDACTED

func TestNormalizeVertexAnthropicModelID(t *testing.T) {
	require.Equal(t, "claude-sonnet-4-5@20250929", normalizeVertexAnthropicModelID("claude-sonnet-4-5-20250929"))
	require.Equal(t, "claude-sonnet-4-5@20250929", normalizeVertexAnthropicModelID("claude-sonnet-4-5@20250929"))
	require.Equal(t, "claude-sonnet-4-6", normalizeVertexAnthropicModelID("claude-sonnet-4-6"))
REDACTED

func TestBuildVertexAnthropicRequestBody(t *testing.T) {
	got, err := buildVertexAnthropicRequestBody([]byte(`{"model":"claude-sonnet-4-5","anthropic_version":"2023-06-01","max_tokens":64,"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`))
REDACTED
	require.Equal(t, "", gjson.GetBytes(got, "model").String())
	require.Equal(t, vertexAnthropicVersion, gjson.GetBytes(got, "anthropic_version").String())
	require.Equal(t, int64(64), gjson.GetBytes(got, "max_tokens").Int())
	require.Equal(t, "hi", gjson.GetBytes(got, "messages.0.content").String())
REDACTED

func TestBuildVertexGeminiURLRejectsInvalidLocation(t *testing.T) {
	_, err := buildVertexGeminiURL("my-project", "us-central1/path", "gemini-3-pro", "generateContent", false)
REDACTED
	require.Contains(t, err.Error(), "invalid vertex location")
REDACTED

func TestParseVertexServiceAccountKey(t *testing.T) {
	raw := `{
		"type": "service_account",
		"project_id": "vertex-proj",
		"private_key_id": "kid",
		"private_key": "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n",
		"client_email": "svc@vertex-proj.iam.gserviceaccount.com"
REDACTED`
	account := &Account{
REDACTED
REDACTED
REDACTED
			"service_account_json": raw,
	REDACTED,
REDACTED
	key, err := parseVertexServiceAccountKey(account)
REDACTED
	require.Equal(t, "vertex-proj", key.ProjectID)
	require.Equal(t, "svc@vertex-proj.iam.gserviceaccount.com", key.ClientEmail)
	require.Equal(t, vertexDefaultTokenURL, key.TokenURI)
	require.True(t, strings.Contains(key.PrivateKey, "BEGIN PRIVATE KEY"))
REDACTED

func TestVertexServiceAccountProxyURL(t *testing.T) {
	proxyID := int64(7)
	account := &Account{
		ProxyID: &proxyID,
		Proxy: &Proxy{
			Protocol: "http",
			Host:     "proxy.example.com",
			Port:     8080,
	REDACTED,
REDACTED

	require.Equal(t, "http://proxy.example.com:8080", vertexServiceAccountProxyURL(account))
	require.Empty(t, vertexServiceAccountProxyURL(&Account{Proxy: account.ProxyREDACTED))
	require.Empty(t, vertexServiceAccountProxyURL(&Account{ProxyID: &proxyIDREDACTED))
REDACTED

func TestVertexServiceAccountHTTPClientRecordsDependency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
REDACTED))
	defer server.Close()

	client, err := newVertexServiceAccountHTTPClient("")
REDACTED
	collector := servertiming.New(time.Now())
	ctx := servertiming.WithCollector(context.Background(), collector)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
REDACTED
	response, err := client.Do(request)
REDACTED
	require.NoError(t, response.Body.Close())
	require.Contains(t, collector.HeaderValue(time.Now(), "bypass"), "dep_http;dur=")
REDACTED

func TestExchangeVertexServiceAccountTokenUsesProxy(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
REDACTED
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
REDACTED)

	seenProxyRequest := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenProxyRequest <- r.URL.String()
		require.Equal(t, "oauth2.googleapis.com", r.URL.Host)
		require.Equal(t, "/token", r.URL.Path)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "urn:ietf:params:oauth:grant-type:jwt-bearer", r.PostForm.Get("grant_type"))

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(vertexTokenResponse{
			AccessToken: "proxied-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
	REDACTED))
REDACTED))
	defer proxy.Close()

	key := &vertexServiceAccountKey{
		ClientEmail: "svc@example.iam.gserviceaccount.com",
		PrivateKey:  string(pemBytes),
		TokenURI:    "http://oauth2.googleapis.com/token",
REDACTED

	token, ttl, err := exchangeVertexServiceAccountToken(contextWithTestTimeout(t), key, proxy.URL)
REDACTED
	require.Equal(t, "proxied-token", token)
	require.Equal(t, 55*time.Minute, ttl)

	select {
	case got := <-seenProxyRequest:
		require.Equal(t, "http://oauth2.googleapis.com/token", got)
	default:
		t.Fatal("token request did not reach proxy")
REDACTED
REDACTED

func contextWithTestTimeout(t *testing.T) context.Context {
REDACTED
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
REDACTED
