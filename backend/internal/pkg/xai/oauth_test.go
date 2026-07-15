//go:build unit

package xai

import (
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/stretchr/testify/require"
)

func TestParseAuthorizationInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		raw               string
		wantCode          string
		wantState         string
		wantRequiresState bool
REDACTED{
		{
			name:              "full callback url",
			raw:               "http://127.0.0.1:56121/callback?code=abc123&state=state456",
			wantCode:          "abc123",
			wantState:         "state456",
			wantRequiresState: true,
	REDACTED,
		{
			name:              "query string",
			raw:               "?code=abc123&state=state456",
			wantCode:          "abc123",
			wantState:         "state456",
			wantRequiresState: true,
	REDACTED,
		{
			name:              "full callback url missing state",
			raw:               "http://127.0.0.1:56121/callback?code=abc123",
			wantCode:          "abc123",
			wantRequiresState: true,
	REDACTED,
		{
			name:              "query string missing state",
			raw:               "code=abc123",
			wantCode:          "abc123",
			wantRequiresState: true,
	REDACTED,
		{
			name:     "bare code",
			raw:      "abc123",
			wantCode: "abc123",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ParseAuthorizationInput(tt.raw)
			require.Equal(t, tt.wantCode, got.Code)
			require.Equal(t, tt.wantState, got.State)
			require.Equal(t, tt.wantRequiresState, got.RequiresState)
	REDACTED)
REDACTED
REDACTED

func TestBuildAuthorizationURLIncludesHermesCompatibleParameters(t *testing.T) {
	t.Setenv(EnvAuthorizeURL, "https://auth.example.test/oauth2/authorize")
	t.Setenv(EnvClientID, "client-id")
	t.Setenv(EnvScope, "openid profile offline_access api:access")
	t.Setenv(EnvAllowUnsafeURLOverrides, "true")

	authURL, err := BuildAuthorizationURL("state", "challenge", "http://127.0.0.1:56121/callback", "nonce")
REDACTED
	parsed, err := url.Parse(authURL)
REDACTED

	values := parsed.Query()
	require.Equal(t, "https", parsed.Scheme)
	require.Equal(t, "auth.example.test", parsed.Host)
	require.Equal(t, "/oauth2/authorize", parsed.Path)
	require.Equal(t, "code", values.Get("response_type"))
	require.Equal(t, "client-id", values.Get("client_id"))
	require.Equal(t, "http://127.0.0.1:56121/callback", values.Get("redirect_uri"))
	require.Equal(t, "openid profile offline_access api:access", values.Get("scope"))
	require.Equal(t, "state", values.Get("state"))
	require.Equal(t, "nonce", values.Get("nonce"))
	require.Equal(t, "challenge", values.Get("code_challenge"))
	require.Equal(t, "S256", values.Get("code_challenge_method"))
	require.Equal(t, "generic", values.Get("plan"))
	require.Equal(t, "sub2api", values.Get("referrer"))
REDACTED

func TestValidateXAIURLsAllowOfficialOAuthAndGatewayHosts(t *testing.T) {
	authorizeURL, err := ValidateOAuthEndpointURL(DefaultAuthorizeURL)
REDACTED
	require.Equal(t, DefaultAuthorizeURL, authorizeURL)

	tokenURL, err := ValidateOAuthEndpointURL(DefaultTokenURL)
REDACTED
	require.Equal(t, DefaultTokenURL, tokenURL)

	baseURL, err := ValidateBaseURL(DefaultBaseURL)
REDACTED
	require.Equal(t, DefaultBaseURL, baseURL)

	cliBaseURL, err := ValidateBaseURL(DefaultCLIBaseURL)
REDACTED
	require.Equal(t, DefaultCLIBaseURL, cliBaseURL)

	baseURLNoPath, err := ValidateBaseURL("https://api.x.ai")
REDACTED
	require.Equal(t, DefaultBaseURL, baseURLNoPath)

	chatURL, err := BuildChatCompletionsURL(DefaultCLIBaseURL + "/")
REDACTED
	require.Equal(t, DefaultCLIBaseURL+"/chat/completions", chatURL)
REDACTED

func TestBuildGrokMediaURLs(t *testing.T) {
	imagesURL, err := BuildImagesGenerationsURL(DefaultBaseURL + "/")
REDACTED
	require.Equal(t, DefaultBaseURL+"/images/generations", imagesURL)

	editsURL, err := BuildImagesEditsURL(DefaultBaseURL)
REDACTED
	require.Equal(t, DefaultBaseURL+"/images/edits", editsURL)

	videosURL, err := BuildVideosGenerationsURL(DefaultBaseURL)
REDACTED
	require.Equal(t, DefaultBaseURL+"/videos/generations", videosURL)

	videoEditsURL, err := BuildVideosEditsURL(DefaultBaseURL)
REDACTED
	require.Equal(t, DefaultBaseURL+"/videos/edits", videoEditsURL)

	videoExtensionsURL, err := BuildVideosExtensionsURL(DefaultBaseURL)
REDACTED
	require.Equal(t, DefaultBaseURL+"/videos/extensions", videoExtensionsURL)

	videoURL, err := BuildVideoURL(DefaultBaseURL, "req 123")
REDACTED
	require.Equal(t, DefaultBaseURL+"/videos/req%20123", videoURL)

	_, err = BuildVideoURL(DefaultBaseURL, " ")
REDACTED
REDACTED

func TestValidateXAIURLsRejectUntrustedOAuthAndUnsafeBaseURLsByDefault(t *testing.T) {
	_, err := ValidateOAuthEndpointURL("https://auth.example.test/oauth2/token")
REDACTED

	_, err = ValidateBaseURL("http://127.0.0.1:8080/v1")
REDACTED

	_, err = ValidateBaseURL("https://api.x.ai/custom")
REDACTED
REDACTED

func TestValidateBaseURLAllowsPublicThirdPartyGrokAPI(t *testing.T) {
	baseURL, err := ValidateBaseURL("https://grok.example.test/v1/")
REDACTED
	require.Equal(t, "https://grok.example.test/v1", baseURL)

	_, err = ValidateTrustedBaseURL("https://grok.example.test/v1")
REDACTED
REDACTED

func TestValidateBaseURLPathPrefixPolicy(t *testing.T) {
	// 非官方主机保留管理员配置的任意 path 前缀。
	prefixed, err := ValidateBaseURL("https://relay.example.test/xai/v1/")
REDACTED
	require.Equal(t, "https://relay.example.test/xai/v1", prefixed)

	deepPrefixed, err := ValidateBaseURL("https://relay.example.test/tenant-a/proxy")
REDACTED
	require.Equal(t, "https://relay.example.test/tenant-a/proxy", deepPrefixed)

	// 空 path 仍按惯例补 /v1，保持既有配置兼容。
	rootOnly, err := ValidateBaseURL("https://relay.example.test")
REDACTED
	require.Equal(t, "https://relay.example.test/v1", rootOnly)

	// 官方主机固定 /v1 前缀。
	_, err = ValidateBaseURL("https://api.x.ai/xai/v1")
REDACTED
	_, err = ValidateBaseURL("https://cli-chat-proxy.grok.com/other")
REDACTED
REDACTED

func TestIsOfficialBaseURL(t *testing.T) {
	official := []string{
		"",
		"   ",
		DefaultBaseURL,
		DefaultCLIBaseURL,
		"https://api.x.ai",
		"HTTPS://API.X.AI:443/",
		"https://api.x.ai:0443/v1",
		"https://api.x.ai/%76%31",
		"https://api.x.ai:8443/v1",
		"HTTPS://CLI-CHAT-PROXY.GROK.COM:443/%76%31/",
		"::invalid::url", // 无法解析的值按官方处理，回落默认端点
REDACTED
	for _, raw := range official {
		require.True(t, IsOfficialBaseURL(raw), "expected official: %q", raw)
REDACTED

	custom := []string{
		"https://relay.example.test/v1",
		"https://relay.example.test/xai/v1",
		"http://relay.example.test/v1",
		"https://grok.com.evil.example.test/v1",
REDACTED
	for _, raw := range custom {
		require.False(t, IsOfficialBaseURL(raw), "expected custom: %q", raw)
REDACTED
REDACTED

func TestValidateBaseURLsRejectEmptyQueryDelimiter(t *testing.T) {
	_, err := ValidateBaseURL("https://grok.example.test/v1?")
REDACTED

	_, err = ValidateTrustedBaseURL("https://api.x.ai/v1?")
REDACTED
REDACTED

func TestBuildResponsesURLWithValidatorUsesCallerPolicy(t *testing.T) {
	validator := func(raw string) (string, error) {
		return urlvalidator.ValidateURLFormat(raw, true)
REDACTED

	target, err := BuildResponsesURLWithValidator("http://grok.example.test/v1/", validator)
REDACTED
	require.Equal(t, "http://grok.example.test/v1/responses", target)
REDACTED

func TestBuildResponsesURLPreservesUnsafeOverrideCustomPath(t *testing.T) {
	t.Setenv(EnvAllowUnsafeURLOverrides, "true")

	target, err := BuildResponsesURL("http://localhost:8080/custom")
REDACTED
	require.Equal(t, "http://localhost:8080/custom/responses", target)
REDACTED

func TestBuildResponsesURLWithValidatorRejectsBaseURLComponents(t *testing.T) {
	permissive := func(raw string) (string, error) { return raw, nil REDACTED
	tests := []struct {
		name string
		raw  string
REDACTED{
		{name: "userinfo", raw: "https://user:secret@grok.example.test/v1"REDACTED,
		{name: "query", raw: "https://grok.example.test/v1?token=secret"REDACTED,
		{name: "empty query delimiter", raw: "https://grok.example.test/v1?"REDACTED,
		{name: "fragment", raw: "https://grok.example.test/v1#secret"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildResponsesURLWithValidator(tt.raw, permissive)
		REDACTED
			require.NotContains(t, err.Error(), "secret")
	REDACTED)
REDACTED
REDACTED

func TestValidateXAIURLsAllowUnsafeDevOverride(t *testing.T) {
	t.Setenv(EnvAllowUnsafeURLOverrides, "true")

	tokenURL, err := ValidateOAuthEndpointURL("http://127.0.0.1:8080/oauth2/token")
REDACTED
	require.Equal(t, "http://127.0.0.1:8080/oauth2/token", tokenURL)

	baseURL, err := ValidateBaseURL("http://127.0.0.1:8080/v1/")
REDACTED
	require.Equal(t, "http://127.0.0.1:8080/v1", baseURL)
REDACTED

func TestRuntimeSanityReportsSafeDefaults(t *testing.T) {
	t.Setenv(EnvBaseURL, "")
	t.Setenv(EnvAuthorizeURL, "")
	t.Setenv(EnvTokenURL, "")
	t.Setenv(EnvRedirectURI, "")
	t.Setenv(EnvAllowUnsafeURLOverrides, "")
	t.Setenv(EnvUnsafeAllowHighConcurrency, "")

	report := RuntimeSanity()
	require.True(t, report.BaseURL.Valid)
	require.Equal(t, DefaultBaseURL, report.BaseURL.Value)
	require.True(t, report.BaseURL.IsDefault)
	require.True(t, report.OAuthAuthorizeURL.Valid)
	require.True(t, report.OAuthTokenURL.Valid)
	require.True(t, report.OAuthRedirectURI.Valid)
	require.False(t, report.UnsafeURLOverrides)
	require.False(t, report.UnsafeHighConcurrency)
	require.Equal(t, "responses_only", report.PublicGatewayScope)
	require.Contains(t, report.ProxyPolicy, "account_proxy_optional")
	require.Contains(t, report.ProxyPolicy, "API-key base URLs require public HTTPS")
REDACTED

func TestRuntimeSanityReportsInvalidOverridesWithoutSecrets(t *testing.T) {
	t.Setenv(EnvBaseURL, "http://127.0.0.1:8080/v1?access_token=secret")
	t.Setenv(EnvAuthorizeURL, "https://auth.example.test/oauth2/authorize")
	t.Setenv(EnvTokenURL, "https://auth.example.test/oauth2/token")
	t.Setenv(EnvRedirectURI, "not a url")
	t.Setenv(EnvClientID, "client-secret-like-value")
	t.Setenv(EnvAllowUnsafeURLOverrides, "")

	report := RuntimeSanity()
	require.False(t, report.BaseURL.Valid)
	require.False(t, report.BaseURL.IsDefault)
	require.Contains(t, report.BaseURL.Error, "invalid url")
	require.NotContains(t, report.BaseURL.Value, "secret")
	require.False(t, report.OAuthAuthorizeURL.Valid)
	require.False(t, report.OAuthTokenURL.Valid)
	require.False(t, report.OAuthRedirectURI.Valid)
	require.NotContains(t, report.ProxyPolicy, "client-secret-like-value")
REDACTED

func TestDefaultModelMappingIncludesGrokAliases(t *testing.T) {
	t.Parallel()

	mapping := DefaultModelMapping()
	require.Equal(t, "grok-4.5", mapping["grok"])
	require.Equal(t, "grok-4.5", mapping["grok-latest"])
	require.Equal(t, "grok-4.5", mapping["grok-4.5"])
	require.Equal(t, "grok-4.5", mapping["grok-4.5-latest"])
	require.Equal(t, "grok-build-0.1", mapping["grok-build"])
	require.Equal(t, "grok-4.5", mapping["grok-build-latest"])
	require.Equal(t, "grok-composer-2.5-fast", mapping["grok-composer"])
	require.Equal(t, "grok-composer-2.5-fast", mapping["composer-2.5"])
	require.Equal(t, "grok-4.20-0309-reasoning", mapping["grok-4.20-reasoning"])
	require.Equal(t, "grok-4.20-0309-non-reasoning", mapping["grok-4.20-non-reasoning"])
	require.Equal(t, "grok-4.20-multi-agent-0309", mapping["grok-4.20-multi-agent-0309"])
	require.Equal(t, "grok-imagine", mapping["grok-imagine"])
	require.Equal(t, "grok-imagine-image", mapping["grok-imagine-image"])
	require.Equal(t, "grok-imagine-image-quality", mapping["grok-imagine-image-quality"])
	require.Equal(t, "grok-imagine-edit", mapping["grok-imagine-edit"])
	require.Equal(t, "grok-imagine-video", mapping["grok-imagine-video"])
	require.Equal(t, "grok-imagine-video-1.5", mapping["grok-imagine-video-1.5"])
REDACTED
