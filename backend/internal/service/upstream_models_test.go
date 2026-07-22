package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func upstreamModelSyncTestConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTED,
	REDACTED,
REDACTED
REDACTED

func grokOAuthModelSyncTestAccount(baseURL string) *Account {
	credentials := map[string]any{
		"access_token":  "oauth-access-token",
		"refresh_token": "oauth-refresh-token",
		"expires_at":    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"sub":           "grok-user-id",
		"email":         "grok-user@example.com",
REDACTED
	if strings.TrimSpace(baseURL) != "" {
		credentials["base_url"] = baseURL
REDACTED
REDACTED
		ID:          10,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Credentials: credentials,
REDACTED
REDACTED

func TestBuildV1ModelsURL(t *testing.T) {
	t.Parallel()

	require.Equal(t, "https://api.anthropic.com/v1/models", buildV1ModelsURL("https://api.anthropic.com"))
	require.Equal(t, "https://api.anthropic.com/v1/models", buildV1ModelsURL("https://api.anthropic.com/v1"))
	require.Equal(t, "https://api.anthropic.com/v1/models", buildV1ModelsURL("https://api.anthropic.com/v1/models"))
	require.Equal(t, "https://gateway.example.com/antigravity/v1/models", buildV1ModelsURL("https://gateway.example.com/antigravity/"))
REDACTED

func TestBuildOpenAIModelsURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		want string
REDACTED{
		{
			name: "zhipu v4 coding base url",
			base: "https://open.bigmodel.cn/api/coding/paas/v4",
			want: "https://open.bigmodel.cn/api/coding/paas/v4/models",
	REDACTED,
		{
			name: "openai v1 base url",
			base: "https://api.openai.com/v1",
			want: "https://api.openai.com/v1/models",
	REDACTED,
		{
			name: "models url unchanged",
			base: "https://api.openai.com/v1/models",
			want: "https://api.openai.com/v1/models",
	REDACTED,
		{
			name: "host fallback uses v1",
			base: "https://api.openai.com",
			want: "https://api.openai.com/v1/models",
	REDACTED,
		{
			name: "trailing slash on v4",
			base: "https://open.bigmodel.cn/api/coding/paas/v4/",
			want: "https://open.bigmodel.cn/api/coding/paas/v4/models",
	REDACTED,
		{
			name: "v2 base url",
			base: "https://gateway.example.com/openai/v2",
			want: "https://gateway.example.com/openai/v2/models",
	REDACTED,
		{
			name: "v3 base url",
			base: "https://gateway.example.com/openai/v3",
			want: "https://gateway.example.com/openai/v3/models",
	REDACTED,
REDACTED

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, buildOpenAIModelsURL(tt.base))
	REDACTED)
REDACTED
REDACTED

func TestBuildGeminiModelsURL(t *testing.T) {
	t.Parallel()

	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models", buildGeminiModelsURL("https://generativelanguage.googleapis.com"))
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models", buildGeminiModelsURL("https://generativelanguage.googleapis.com/v1beta"))
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models", buildGeminiModelsURL("https://generativelanguage.googleapis.com/v1beta/models"))
REDACTED

func TestExtractUpstreamModelIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []string
REDACTED{
		{
			name: "openai and anthropic data array",
			body: `{"data":[{"id":"claude-sonnet-4-5"REDACTED,{"id":"gpt-5"REDACTED,{"id":"gpt-5"REDACTED,{"id":""REDACTED]REDACTED`,
			want: []string{"claude-sonnet-4-5", "gpt-5"REDACTED,
	REDACTED,
		{
			name: "gemini models array strips prefix",
			body: `{"models":[{"name":"models/gemini-2.5-pro"REDACTED,{"name":"gemini-2.5-flash"REDACTED]REDACTED`,
			want: []string{"gemini-2.5-flash", "gemini-2.5-pro"REDACTED,
	REDACTED,
		{
			name: "top level array",
			body: `[{"id":"z-model"REDACTED,{"name":"models/a-model"REDACTED]`,
			want: []string{"a-model", "z-model"REDACTED,
	REDACTED,
		{
			name: "standard id wins over provider-specific model field",
			body: `{"data":[{"id":"canonical-id","model":"display-model"REDACTED]REDACTED`,
			want: []string{"canonical-id"REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := extractUpstreamModelIDs([]byte(tt.body))
		REDACTED
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

func TestExtractGrokUpstreamModelIDs(t *testing.T) {
	t.Parallel()

	models, err := extractGrokUpstreamModelIDs([]byte(`{"data":[{"id":"display-id","model":"grok-4.5"REDACTED,{"modelId":"grok-build-0.1"REDACTED,{"model_id":"grok-composer-2.5-fast"REDACTED,{"name":"Grok Meta Display Name","_meta":{"model":"grok-meta"REDACTEDREDACTED,{"name":"grok-name"REDACTED,{"id":"grok-safe","_meta":"not-an-object"REDACTED]REDACTED`))
REDACTED
	require.Equal(t, []string{"grok-4.5", "grok-build-0.1", "grok-composer-2.5-fast", "grok-meta", "grok-name", "grok-safe"REDACTED, models)
REDACTED

func TestBuildUpstreamModelsRequestsForAPIKeyAccounts(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig()REDACTED
	ctx := context.Background()

	anthropicReq, err := svc.buildAnthropicUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":  "anthropic-key",
			"base_url": "https://anthropic.example.com/v1",
	REDACTED,
REDACTED)
REDACTED
	require.Equal(t, "https://anthropic.example.com/v1/models", anthropicReq.URL.String())
	require.Equal(t, "anthropic-key", anthropicReq.Header.Get("x-api-key"))
	require.Equal(t, "2023-06-01", anthropicReq.Header.Get("anthropic-version"))

	anthropicBearerReq, err := svc.buildAnthropicUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":  "ollama-key",
			"base_url": "https://ollama.com",
	REDACTED,
		Extra: map[string]any{
			"anthropic_apikey_auth_scheme": AnthropicAPIKeyAuthSchemeAuthorizationBearer,
	REDACTED,
REDACTED)
REDACTED
	require.Equal(t, "https://ollama.com/v1/models", anthropicBearerReq.URL.String())
	require.Equal(t, "Bearer ollama-key", anthropicBearerReq.Header.Get("Authorization"))
	require.Empty(t, anthropicBearerReq.Header.Get("x-api-key"))
	require.Equal(t, "2023-06-01", anthropicBearerReq.Header.Get("anthropic-version"))

	openAIReq, err := svc.buildOpenAIUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":  "openai-key",
			"base_url": "https://openai.example.com",
	REDACTED,
REDACTED)
REDACTED
	require.Equal(t, "https://openai.example.com/v1/models", openAIReq.URL.String())
	require.Equal(t, "Bearer openai-key", openAIReq.Header.Get("Authorization"))

	grokReq, err := svc.buildUpstreamModelsRequest(ctx, &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":  "xai-key",
			"base_url": "https://xai.example.com/v1",
	REDACTED,
REDACTED)
REDACTED
	require.Equal(t, "https://xai.example.com/v1/models", grokReq.URL.String())
	require.Equal(t, "Bearer xai-key", grokReq.Header.Get("Authorization"))

	geminiReq, err := svc.buildGeminiUpstreamModelsRequest(ctx, &Account{
REDACTED
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":  "gemini-key",
			"base_url": "https://generativelanguage.googleapis.com/v1beta",
	REDACTED,
REDACTED)
REDACTED
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models", geminiReq.URL.String())
	require.Equal(t, "gemini-key", geminiReq.Header.Get("x-goog-api-key"))

	antigravityReq, err := svc.buildAntigravityAPIKeyModelsRequest(ctx, &Account{
		Platform: PlatformAntigravity,
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":  "antigravity-key",
			"base_url": "https://gateway.example.com/antigravity",
	REDACTED,
REDACTED)
REDACTED
	require.Equal(t, "https://gateway.example.com/antigravity/v1/models", antigravityReq.URL.String())
	require.Equal(t, "antigravity-key", antigravityReq.Header.Get("x-api-key"))
REDACTED

func TestBuildUpstreamModelsRequestSupportsGrokOAuth(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{
		cfg:               upstreamModelSyncTestConfig(),
		grokTokenProvider: NewGrokTokenProvider(nil, nil),
REDACTED
	req, err := svc.buildUpstreamModelsRequest(context.Background(), grokOAuthModelSyncTestAccount(""))
REDACTED
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/models", req.URL.String())
	require.Equal(t, "Bearer oauth-access-token", req.Header.Get("Authorization"))
	require.Equal(t, grokCLIVersion, req.Header.Get("X-Grok-Client-Version"))
	require.Equal(t, "interactive", req.Header.Get("X-Grok-Client-Mode"))
	require.Equal(t, grokUpstreamUserAgent, req.Header.Get("User-Agent"))
	require.Equal(t, "grok-user-id", req.Header.Get("X-UserID"))
	require.Equal(t, "grok-user@example.com", req.Header.Get("X-Email"))
	require.NotContains(t, req.Header.Get("Authorization"), "oauth-refresh-token")
REDACTED

func TestBuildUpstreamModelsRequestGrokOAuthRequiresTokenProvider(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig()REDACTED
	_, err := svc.buildUpstreamModelsRequest(context.Background(), grokOAuthModelSyncTestAccount(""))
REDACTED

	var syncErr *UpstreamModelSyncError
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorConfiguration, syncErr.Kind)
	require.Contains(t, syncErr.SafeMessage(), "token provider")
REDACTED

func TestBuildAntigravityAPIKeyModelsRequestRejectsOfficialCloudCodeBase(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig()REDACTED
	_, err := svc.buildAntigravityAPIKeyModelsRequest(context.Background(), &Account{
		Platform: PlatformAntigravity,
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":  "antigravity-key",
			"base_url": "https://cloudcode-pa.googleapis.com",
	REDACTED,
REDACTED)
REDACTED

	var syncErr *UpstreamModelSyncError
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorUnsupported, syncErr.Kind)
	require.Contains(t, syncErr.SafeMessage(), "compatible gateway")
REDACTED

func TestBuildAnthropicUpstreamModelsRequestRejectsBedrock(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{cfg: upstreamModelSyncTestConfig()REDACTED
	_, err := svc.buildAnthropicUpstreamModelsRequest(context.Background(), &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeBedrock,
REDACTED)
REDACTED

	var syncErr *UpstreamModelSyncError
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorUnsupported, syncErr.Kind)
REDACTED

func TestFetchUpstreamSupportedModelsParsesOpenAIResponse(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"gpt-5"REDACTED,{"id":"gpt-5"REDACTED,{"name":"o3"REDACTED]REDACTED`)),
REDACTEDREDACTED
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
REDACTED

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:       7,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":  "openai-key",
			"base_url": "https://openai.example.com/v1",
	REDACTED,
REDACTED)
REDACTED
	require.Equal(t, []string{"gpt-5", "o3"REDACTED, models)
	require.Equal(t, "https://openai.example.com/v1/models", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer openai-key", upstream.lastReq.Header.Get("Authorization"))
REDACTED

func TestFetchUpstreamSupportedModelsParsesGrokAPIKeyResponse(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"grok-4.5"REDACTED,{"id":"grok-4.5"REDACTED,{"id":"grok-imagine"REDACTED]REDACTED`)),
REDACTEDREDACTED
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
REDACTED

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:       9,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":  "xai-key",
			"base_url": "https://xai.example.com/v1",
	REDACTED,
REDACTED)
REDACTED
	require.Equal(t, []string{"grok-4.5", "grok-imagine"REDACTED, models)
	require.Equal(t, "https://xai.example.com/v1/models", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer xai-key", upstream.lastReq.Header.Get("Authorization"))
REDACTED

func TestFetchUpstreamSupportedModelsParsesGrokOAuthResponse(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"model":"grok-4.5"REDACTED,{"model":"grok-4.5"REDACTED,{"modelId":"grok-build-0.1"REDACTED]REDACTED`)),
REDACTEDREDACTED
	svc := &AccountTestService{
		httpUpstream:      upstream,
		cfg:               upstreamModelSyncTestConfig(),
		grokTokenProvider: NewGrokTokenProvider(nil, nil),
REDACTED

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), grokOAuthModelSyncTestAccount(""))
REDACTED
	require.Equal(t, []string{"grok-4.5", "grok-build-0.1"REDACTED, models)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/models", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer oauth-access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, grokCLIVersion, upstream.lastReq.Header.Get("X-Grok-Client-Version"))
	require.Equal(t, "interactive", upstream.lastReq.Header.Get("X-Grok-Client-Mode"))
	require.Equal(t, "grok-user-id", upstream.lastReq.Header.Get("X-UserID"))
	require.Equal(t, "grok-user@example.com", upstream.lastReq.Header.Get("X-Email"))
REDACTED

func TestBuildUpstreamModelsRequestGrokOAuthDoesNotSendIdentityToCustomBase(t *testing.T) {
	t.Parallel()

	svc := &AccountTestService{
		cfg:               upstreamModelSyncTestConfig(),
		grokTokenProvider: NewGrokTokenProvider(nil, nil),
REDACTED
	req, err := svc.buildUpstreamModelsRequest(context.Background(), grokOAuthModelSyncTestAccount("https://relay.example/v1"))
REDACTED
	require.Equal(t, "https://relay.example/v1/models", req.URL.String())
	require.Empty(t, req.Header.Get("X-UserID"))
	require.Empty(t, req.Header.Get("X-Email"))
REDACTED

func TestFetchUpstreamSupportedModelsDoesNotExposeUpstreamBody(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"error":"SECRET_TOKEN should not be exposed"REDACTED`)),
REDACTEDREDACTED
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          upstreamModelSyncTestConfig(),
REDACTED

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), &Account{
		ID:       8,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":  "openai-key",
			"base_url": "https://openai.example.com/v1",
	REDACTED,
REDACTED)
REDACTED
	require.NotContains(t, err.Error(), "SECRET_TOKEN")

	var syncErr *UpstreamModelSyncError
	require.True(t, errors.As(err, &syncErr))
	require.Equal(t, UpstreamModelSyncErrorUpstream, syncErr.Kind)
	require.NotContains(t, syncErr.SafeMessage(), "SECRET_TOKEN")
	require.Contains(t, syncErr.SafeMessage(), "HTTP 502")
REDACTED
