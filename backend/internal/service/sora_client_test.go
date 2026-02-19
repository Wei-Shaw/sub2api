//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSoraDirectClient_DoRequestSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":trueREDACTED`))
REDACTED))
	defer server.Close()

	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{BaseURL: server.URLREDACTED,
	REDACTED,
REDACTED
	client := NewSoraDirectClient(cfg, nil, nil)

	body, _, err := client.doRequest(context.Background(), &Account{ID: 1REDACTED, http.MethodGet, server.URL, http.Header{REDACTED, nil, false)
REDACTED
	require.Contains(t, string(body), "ok")
REDACTED

func TestSoraDirectClient_BuildBaseHeaders(t *testing.T) {
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				Headers: map[string]string{
					"X-Test":                "yes",
					"Authorization":         "should-ignore",
					"openai-sentinel-token": "skip",
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	client := NewSoraDirectClient(cfg, nil, nil)

	headers := client.buildBaseHeaders("token-123", "UA")
	require.Equal(t, "Bearer token-123", headers.Get("Authorization"))
	require.Equal(t, "UA", headers.Get("User-Agent"))
	require.Equal(t, "yes", headers.Get("X-Test"))
	require.Empty(t, headers.Get("openai-sentinel-token"))
REDACTED

func TestSoraDirectClient_GetImageTaskFallbackLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		switch limit {
		case "1":
			_, _ = w.Write([]byte(`{"task_responses":[]REDACTED`))
		case "2":
			_, _ = w.Write([]byte(`{"task_responses":[{"id":"task-1","status":"completed","progress_pct":1,"generations":[{"url":"https://example.com/a.png"REDACTED]REDACTED]REDACTED`))
		default:
			_, _ = w.Write([]byte(`{"task_responses":[]REDACTED`))
	REDACTED
REDACTED))
	defer server.Close()

	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				BaseURL:            server.URL,
				RecentTaskLimit:    1,
				RecentTaskLimitMax: 2,
		REDACTED,
	REDACTED,
REDACTED
	client := NewSoraDirectClient(cfg, nil, nil)
	account := &Account{Credentials: map[string]any{"access_token": "token"REDACTEDREDACTED

	status, err := client.GetImageTask(context.Background(), account, "task-1")
REDACTED
	require.Equal(t, "completed", status.Status)
	require.Equal(t, []string{"https://example.com/a.png"REDACTED, status.URLs)
REDACTED

func TestNormalizeSoraBaseURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want string
REDACTED{
		{
			name: "empty",
			raw:  "",
			want: "",
	REDACTED,
		{
			name: "append_backend_for_sora_host",
			raw:  "https://sora.chatgpt.com",
			want: "https://sora.chatgpt.com/backend",
	REDACTED,
		{
			name: "convert_backend_api_to_backend",
			raw:  "https://sora.chatgpt.com/backend-api",
			want: "https://sora.chatgpt.com/backend",
	REDACTED,
		{
			name: "keep_backend",
			raw:  "https://sora.chatgpt.com/backend",
			want: "https://sora.chatgpt.com/backend",
	REDACTED,
		{
			name: "keep_custom_host",
			raw:  "https://example.com/custom-path",
			want: "https://example.com/custom-path",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeSoraBaseURL(tt.raw)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

func TestSoraDirectClient_BuildURL_UsesNormalizedBaseURL(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				BaseURL: "https://sora.chatgpt.com",
		REDACTED,
	REDACTED,
REDACTED
	client := NewSoraDirectClient(cfg, nil, nil)
	require.Equal(t, "https://sora.chatgpt.com/backend/video_gen", client.buildURL("/video_gen"))
REDACTED

func TestSoraDirectClient_BuildUpstreamError_NotFoundHint(t *testing.T) {
	t.Parallel()
	client := NewSoraDirectClient(&config.Config{REDACTED, nil, nil)

	err := client.buildUpstreamError(http.StatusNotFound, http.Header{REDACTED, []byte(`{"error":{"message":"Not found"REDACTEDREDACTED`), "https://sora.chatgpt.com/video_gen")
	var upstreamErr *SoraUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Contains(t, upstreamErr.Message, "请检查 sora.client.base_url")

	errNoHint := client.buildUpstreamError(http.StatusNotFound, http.Header{REDACTED, []byte(`{"error":{"message":"Not found"REDACTEDREDACTED`), "https://sora.chatgpt.com/backend/video_gen")
	require.ErrorAs(t, errNoHint, &upstreamErr)
	require.NotContains(t, upstreamErr.Message, "请检查 sora.client.base_url")
REDACTED

func TestFormatSoraHeaders_RedactsSensitive(t *testing.T) {
	t.Parallel()
	headers := http.Header{REDACTED
	headers.Set("Authorization", "Bearer secret-token")
	headers.Set("openai-sentinel-token", "sentinel-secret")
	headers.Set("X-Test", "ok")

	out := formatSoraHeaders(headers)
	require.Contains(t, out, `"Authorization":"***"`)
	require.Contains(t, out, `Sentinel-Token":"***"`)
	require.Contains(t, out, `"X-Test":"ok"`)
	require.NotContains(t, out, "secret-token")
	require.NotContains(t, out, "sentinel-secret")
REDACTED

func TestSummarizeSoraResponseBody_RedactsJSON(t *testing.T) {
	t.Parallel()
	body := []byte(`{"error":{"message":"bad"REDACTED,"access_token":"abc123"REDACTED`)
	out := summarizeSoraResponseBody(body, 512)
	require.Contains(t, out, `"access_token":"***"`)
	require.NotContains(t, out, "abc123")
REDACTED

func TestSummarizeSoraResponseBody_Truncates(t *testing.T) {
	t.Parallel()
	body := []byte(strings.Repeat("x", 100))
	out := summarizeSoraResponseBody(body, 10)
	require.Contains(t, out, "(truncated)")
REDACTED

func TestSoraDirectClient_GetAccessToken_SoraDefaultUseCredentials(t *testing.T) {
	t.Parallel()
	cache := newOpenAITokenCacheStub()
	provider := NewOpenAITokenProvider(nil, cache, nil)
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				BaseURL: "https://sora.chatgpt.com/backend",
		REDACTED,
	REDACTED,
REDACTED
	client := NewSoraDirectClient(cfg, nil, provider)
	account := &Account{
		ID:       1,
		Platform: PlatformSora,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "sora-credential-token",
	REDACTED,
REDACTED

	token, err := client.getAccessToken(context.Background(), account)
REDACTED
	require.Equal(t, "sora-credential-token", token)
	require.Equal(t, int32(0), atomic.LoadInt32(&cache.getCalled))
REDACTED

func TestSoraDirectClient_GetAccessToken_SoraCanEnableProvider(t *testing.T) {
	t.Parallel()
	cache := newOpenAITokenCacheStub()
	account := &Account{
		ID:       2,
		Platform: PlatformSora,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "sora-credential-token",
	REDACTED,
REDACTED
	cache.tokens[OpenAITokenCacheKey(account)] = "provider-token"
	provider := NewOpenAITokenProvider(nil, cache, nil)
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				BaseURL:                "https://sora.chatgpt.com/backend",
				UseOpenAITokenProvider: true,
		REDACTED,
	REDACTED,
REDACTED
	client := NewSoraDirectClient(cfg, nil, provider)

	token, err := client.getAccessToken(context.Background(), account)
REDACTED
	require.Equal(t, "provider-token", token)
	require.Greater(t, atomic.LoadInt32(&cache.getCalled), int32(0))
REDACTED

func TestSoraDirectClient_GetAccessToken_FromSessionToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Contains(t, r.Header.Get("Cookie"), "__Secure-next-auth.session-token=session-token")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accessToken": "session-access-token",
			"expires":     "2099-01-01T00:00:00Z",
	REDACTED)
REDACTED))
	defer server.Close()

	origin := soraSessionAuthURL
	soraSessionAuthURL = server.URL
	defer func() { soraSessionAuthURL = origin REDACTED()

	client := NewSoraDirectClient(&config.Config{REDACTED, nil, nil)
	account := &Account{
		ID:       10,
		Platform: PlatformSora,
		Type:     AccountTypeOAuth,
REDACTED
			"session_token": "session-token",
	REDACTED,
REDACTED

	token, err := client.getAccessToken(context.Background(), account)
REDACTED
	require.Equal(t, "session-access-token", token)
	require.Equal(t, "session-access-token", account.GetCredential("access_token"))
REDACTED

func TestSoraDirectClient_GetAccessToken_FromRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/oauth/token", r.URL.Path)
		require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		require.NoError(t, r.ParseForm())
		require.Equal(t, "refresh_token", r.FormValue("grant_type"))
		require.Equal(t, "refresh-token-old", r.FormValue("refresh_token"))
		require.NotEmpty(t, r.FormValue("client_id"))
		require.Equal(t, "com.openai.chat://auth0.openai.com/ios/com.openai.chat/callback", r.FormValue("redirect_uri"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "refresh-access-token",
			"refresh_token": "refresh-token-new",
			"expires_in":    3600,
	REDACTED)
REDACTED))
	defer server.Close()

	origin := soraOAuthTokenURL
	soraOAuthTokenURL = server.URL + "/oauth/token"
	defer func() { soraOAuthTokenURL = origin REDACTED()

	client := NewSoraDirectClient(&config.Config{REDACTED, nil, nil)
	account := &Account{
		ID:       11,
		Platform: PlatformSora,
		Type:     AccountTypeOAuth,
REDACTED
			"refresh_token": "refresh-token-old",
	REDACTED,
REDACTED

	token, err := client.getAccessToken(context.Background(), account)
REDACTED
	require.Equal(t, "refresh-access-token", token)
	require.Equal(t, "refresh-token-new", account.GetCredential("refresh_token"))
	require.NotNil(t, account.GetCredentialAsTime("expires_at"))
REDACTED

func TestSoraDirectClient_PreflightCheck_VideoQuotaExceeded(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/nf/check", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rate_limit_and_credit_balance": map[string]any{
				"estimated_num_videos_remaining": 0,
				"rate_limit_reached":             true,
		REDACTED,
	REDACTED)
REDACTED))
	defer server.Close()

	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				BaseURL: server.URL,
		REDACTED,
	REDACTED,
REDACTED
	client := NewSoraDirectClient(cfg, nil, nil)
	account := &Account{
		ID:       12,
		Platform: PlatformSora,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "ok",
			"expires_at":   time.Now().Add(2 * time.Hour).Format(time.RFC3339),
	REDACTED,
REDACTED
	err := client.PreflightCheck(context.Background(), account, "sora2-landscape-10s", SoraModelConfig{Type: "video"REDACTED)
REDACTED
	var upstreamErr *SoraUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusTooManyRequests, upstreamErr.StatusCode)
REDACTED

func TestShouldAttemptSoraTokenRecover(t *testing.T) {
	t.Parallel()

	require.True(t, shouldAttemptSoraTokenRecover(http.StatusUnauthorized, "https://sora.chatgpt.com/backend/video_gen"))
	require.True(t, shouldAttemptSoraTokenRecover(http.StatusForbidden, "https://chatgpt.com/backend/video_gen"))
	require.False(t, shouldAttemptSoraTokenRecover(http.StatusUnauthorized, "https://sora.chatgpt.com/api/auth/session"))
	require.False(t, shouldAttemptSoraTokenRecover(http.StatusUnauthorized, "https://auth.openai.com/oauth/token"))
	require.False(t, shouldAttemptSoraTokenRecover(http.StatusTooManyRequests, "https://sora.chatgpt.com/backend/video_gen"))
REDACTED

type soraClientRequestCall struct {
	Path      string
	UserAgent string
	ProxyURL  string
REDACTED

type soraClientRecordingUpstream struct {
	calls []soraClientRequestCall
REDACTED

func (u *soraClientRecordingUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return nil, errors.New("unexpected Do call")
REDACTED

func (u *soraClientRecordingUpstream) DoWithTLS(req *http.Request, proxyURL string, _ int64, _ int, _ bool) (*http.Response, error) {
	u.calls = append(u.calls, soraClientRequestCall{
		Path:      req.URL.Path,
		UserAgent: req.Header.Get("User-Agent"),
		ProxyURL:  proxyURL,
REDACTED)
	switch req.URL.Path {
	case "/backend-api/sentinel/req":
		return newSoraClientMockResponse(http.StatusOK, `{"token":"sentinel-token","turnstile":{"dx":"ok"REDACTEDREDACTED`), nil
	case "/backend/nf/create":
		return newSoraClientMockResponse(http.StatusOK, `{"id":"task-123"REDACTED`), nil
	case "/backend/nf/create/storyboard":
		return newSoraClientMockResponse(http.StatusOK, `{"id":"storyboard-123"REDACTED`), nil
	case "/backend/uploads":
		return newSoraClientMockResponse(http.StatusOK, `{"id":"upload-123"REDACTED`), nil
	case "/backend/nf/check":
		return newSoraClientMockResponse(http.StatusOK, `{"rate_limit_and_credit_balance":{"estimated_num_videos_remaining":1,"rate_limit_reached":falseREDACTEDREDACTED`), nil
	case "/backend/characters/upload":
		return newSoraClientMockResponse(http.StatusOK, `{"id":"cameo-123"REDACTED`), nil
	case "/backend/project_y/cameos/in_progress/cameo-123":
		return newSoraClientMockResponse(http.StatusOK, `{"status":"finalized","status_message":"Completed","username_hint":"foo.bar","display_name_hint":"Bar","profile_asset_url":"https://example.com/avatar.webp"REDACTED`), nil
	case "/backend/project_y/file/upload":
		return newSoraClientMockResponse(http.StatusOK, `{"asset_pointer":"asset-123"REDACTED`), nil
	case "/backend/characters/finalize":
		return newSoraClientMockResponse(http.StatusOK, `{"character":{"character_id":"character-123"REDACTEDREDACTED`), nil
	case "/backend/project_y/post":
		return newSoraClientMockResponse(http.StatusOK, `{"post":{"id":"s_post"REDACTEDREDACTED`), nil
	default:
		return newSoraClientMockResponse(http.StatusOK, `{"ok":trueREDACTED`), nil
REDACTED
REDACTED

func newSoraClientMockResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
REDACTED
REDACTED

func TestSoraDirectClient_TaskUserAgent_DefaultMobileFallback(t *testing.T) {
	client := NewSoraDirectClient(&config.Config{REDACTED, nil, nil)
	ua := client.taskUserAgent()
	require.NotEmpty(t, ua)
	allowed := append([]string{REDACTED, soraMobileUserAgents...)
	allowed = append(allowed, soraDesktopUserAgents...)
	require.Contains(t, allowed, ua)
REDACTED

func TestSoraDirectClient_CreateVideoTask_UsesSameUserAgentAndProxyForSentinelAndCreate(t *testing.T) {
	originPowTokenGenerator := soraPowTokenGenerator
	soraPowTokenGenerator = func(_ string) string { return "gAAAAACmock" REDACTED
	defer func() {
		soraPowTokenGenerator = originPowTokenGenerator
REDACTED()

	upstream := &soraClientRecordingUpstream{REDACTED
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				BaseURL: "https://sora.chatgpt.com/backend",
		REDACTED,
	REDACTED,
REDACTED
	client := NewSoraDirectClient(cfg, upstream, nil)
	proxyID := int64(9)
	account := &Account{
		ID:          21,
		Platform:    PlatformSora,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		ProxyID:     &proxyID,
		Proxy: &Proxy{
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
	REDACTED,
REDACTED
			"access_token": "access-token",
			"expires_at":   time.Now().Add(30 * time.Minute).Format(time.RFC3339),
	REDACTED,
REDACTED

	taskID, err := client.CreateVideoTask(context.Background(), account, SoraVideoRequest{Prompt: "test"REDACTED)
REDACTED
	require.Equal(t, "task-123", taskID)
	require.Len(t, upstream.calls, 2)

	sentinelCall := upstream.calls[0]
	createCall := upstream.calls[1]
	require.Equal(t, "/backend-api/sentinel/req", sentinelCall.Path)
	require.Equal(t, "/backend/nf/create", createCall.Path)
	require.Equal(t, "http://127.0.0.1:8080", sentinelCall.ProxyURL)
	require.Equal(t, sentinelCall.ProxyURL, createCall.ProxyURL)
	require.NotEmpty(t, sentinelCall.UserAgent)
	require.Equal(t, sentinelCall.UserAgent, createCall.UserAgent)
REDACTED

func TestSoraDirectClient_UploadImage_UsesTaskUserAgentAndProxy(t *testing.T) {
	upstream := &soraClientRecordingUpstream{REDACTED
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				BaseURL: "https://sora.chatgpt.com/backend",
		REDACTED,
	REDACTED,
REDACTED
	client := NewSoraDirectClient(cfg, upstream, nil)
	proxyID := int64(3)
	account := &Account{
		ID:      31,
		ProxyID: &proxyID,
		Proxy: &Proxy{
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
	REDACTED,
REDACTED
			"access_token": "access-token",
			"expires_at":   time.Now().Add(30 * time.Minute).Format(time.RFC3339),
	REDACTED,
REDACTED

	uploadID, err := client.UploadImage(context.Background(), account, []byte("mock-image"), "a.png")
REDACTED
	require.Equal(t, "upload-123", uploadID)
	require.Len(t, upstream.calls, 1)
	require.Equal(t, "/backend/uploads", upstream.calls[0].Path)
	require.Equal(t, "http://127.0.0.1:8080", upstream.calls[0].ProxyURL)
	require.NotEmpty(t, upstream.calls[0].UserAgent)
REDACTED

func TestSoraDirectClient_PreflightCheck_UsesTaskUserAgentAndProxy(t *testing.T) {
	upstream := &soraClientRecordingUpstream{REDACTED
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				BaseURL: "https://sora.chatgpt.com/backend",
		REDACTED,
	REDACTED,
REDACTED
	client := NewSoraDirectClient(cfg, upstream, nil)
	proxyID := int64(7)
	account := &Account{
		ID:      41,
		ProxyID: &proxyID,
		Proxy: &Proxy{
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
	REDACTED,
REDACTED
			"access_token": "access-token",
			"expires_at":   time.Now().Add(30 * time.Minute).Format(time.RFC3339),
	REDACTED,
REDACTED

	err := client.PreflightCheck(context.Background(), account, "sora2", SoraModelConfig{Type: "video"REDACTED)
REDACTED
	require.Len(t, upstream.calls, 1)
	require.Equal(t, "/backend/nf/check", upstream.calls[0].Path)
	require.Equal(t, "http://127.0.0.1:8080", upstream.calls[0].ProxyURL)
	require.NotEmpty(t, upstream.calls[0].UserAgent)
REDACTED

func TestSoraDirectClient_CreateStoryboardTask(t *testing.T) {
	originPowTokenGenerator := soraPowTokenGenerator
	soraPowTokenGenerator = func(_ string) string { return "gAAAAACmock" REDACTED
	defer func() { soraPowTokenGenerator = originPowTokenGenerator REDACTED()

	upstream := &soraClientRecordingUpstream{REDACTED
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				BaseURL: "https://sora.chatgpt.com/backend",
		REDACTED,
	REDACTED,
REDACTED
	client := NewSoraDirectClient(cfg, upstream, nil)
	account := &Account{
		ID: 51,
REDACTED
			"access_token": "access-token",
			"expires_at":   time.Now().Add(30 * time.Minute).Format(time.RFC3339),
	REDACTED,
REDACTED

	taskID, err := client.CreateStoryboardTask(context.Background(), account, SoraStoryboardRequest{
		Prompt: "Shot 1:\nduration: 5sec\nScene: cat",
REDACTED)
REDACTED
	require.Equal(t, "storyboard-123", taskID)
	require.Len(t, upstream.calls, 2)
	require.Equal(t, "/backend-api/sentinel/req", upstream.calls[0].Path)
	require.Equal(t, "/backend/nf/create/storyboard", upstream.calls[1].Path)
REDACTED

func TestSoraDirectClient_GetVideoTask_ReturnsGenerationID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/nf/pending/v2":
			_, _ = w.Write([]byte(`[]`))
		case "/project_y/profile/drafts":
			_, _ = w.Write([]byte(`{"items":[{"id":"gen_1","task_id":"task-1","kind":"video","downloadable_url":"https://example.com/v.mp4"REDACTED]REDACTED`))
		default:
			http.NotFound(w, r)
	REDACTED
REDACTED))
	defer server.Close()

	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				BaseURL: server.URL,
		REDACTED,
	REDACTED,
REDACTED
	client := NewSoraDirectClient(cfg, nil, nil)
	account := &Account{Credentials: map[string]any{"access_token": "token"REDACTEDREDACTED

	status, err := client.GetVideoTask(context.Background(), account, "task-1")
REDACTED
	require.Equal(t, "completed", status.Status)
	require.Equal(t, "gen_1", status.GenerationID)
	require.Equal(t, []string{"https://example.com/v.mp4"REDACTED, status.URLs)
REDACTED

func TestSoraDirectClient_PostVideoForWatermarkFree(t *testing.T) {
	originPowTokenGenerator := soraPowTokenGenerator
	soraPowTokenGenerator = func(_ string) string { return "gAAAAACmock" REDACTED
	defer func() { soraPowTokenGenerator = originPowTokenGenerator REDACTED()

	upstream := &soraClientRecordingUpstream{REDACTED
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				BaseURL: "https://sora.chatgpt.com/backend",
		REDACTED,
	REDACTED,
REDACTED
	client := NewSoraDirectClient(cfg, upstream, nil)
	account := &Account{
		ID: 52,
REDACTED
			"access_token": "access-token",
			"expires_at":   time.Now().Add(30 * time.Minute).Format(time.RFC3339),
	REDACTED,
REDACTED

	postID, err := client.PostVideoForWatermarkFree(context.Background(), account, "gen_1")
REDACTED
	require.Equal(t, "s_post", postID)
	require.Len(t, upstream.calls, 2)
	require.Equal(t, "/backend-api/sentinel/req", upstream.calls[0].Path)
	require.Equal(t, "/backend/project_y/post", upstream.calls[1].Path)
REDACTED
