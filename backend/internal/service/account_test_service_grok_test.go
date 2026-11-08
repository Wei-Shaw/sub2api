//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type grokAccountTestRateLimitRepo struct {
	*mockAccountRepoForGemini
	rateLimitedCalls int
	resetAt          time.Time
}

func TestObserveGrokTestResponseClassifiesBodyOnlyQuotaErrors(t *testing.T) {
	account := &Account{ID: 1901, Platform: PlatformGrok, Type: AccountTypeOAuth}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	svc := &AccountTestService{accountRepo: repo}

	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"subscription:free-usage-exhausted","message":"included free usage exhausted"}}`)),
	}
	svc.observeGrokTestResponse(context.Background(), account, resp)
	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Equal(t, "grok free usage exhausted", repo.lastTempUnschedReason)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "free-usage-exhausted")
}

func TestObserveGrokTestResponseDoesNotQuarantineContentPolicy(t *testing.T) {
	account := &Account{ID: 1902, Platform: PlatformGrok, Type: AccountTypeOAuth}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	svc := &AccountTestService{accountRepo: repo}
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"new_sensitive","message":"text is sensitive"}}`)),
	}
	svc.observeGrokTestResponse(context.Background(), account, resp)
	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.rateLimitedCalls)
}

func TestObserveGrokTestResponseKeepsEntitlement403Cooldown(t *testing.T) {
	account := &Account{ID: 1903, Platform: PlatformGrok, Type: AccountTypeOAuth}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	svc := &AccountTestService{accountRepo: repo}
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"subscription required"}}`)),
	}
	before := time.Now()
	svc.observeGrokTestResponse(context.Background(), account, resp)
	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Equal(t, "grok entitlement or subscription tier denied", repo.lastTempUnschedReason)
	require.Greater(t, repo.lastTempUnschedUntil, before.Add(29*time.Minute))
}

func (r *grokAccountTestRateLimitRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitedCalls++
	r.resetAt = resetAt
	return nil
}

func TestAccountTestService_TestAccountConnection_GrokUsesXAIResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	account := &Account{
		ID:          13,
		Name:        "grok-oauth",
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "grok-access-token",
			"refresh_token": "grok-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
			"model_mapping": map[string]any{
				"grok": "grok-4.3",
			},
		},
	}
	repo := &mockAccountRepoForGemini{
		accountsByID: map[int64]*Account{account.ID: account},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n" +
				"data: {\"type\":\"response.completed\"}\n\n",
		)),
	}}
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		httpUpstream:      upstream,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/13/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "grok", "", AccountTestModeDefault)
	require.NoError(t, err)

	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer grok-access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, grokCLIVersion, upstream.lastReq.Header.Get("X-Grok-Client-Version"))
	require.Equal(t, "application/json, text/event-stream", upstream.lastReq.Header.Get("Accept"))
	require.Equal(t, "grok-4.3", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, grokQuotaProbeInput, gjson.GetBytes(upstream.lastBody, "input").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.False(t, gjson.GetBytes(upstream.lastBody, "max_output_tokens").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "store").Exists())
	require.NotContains(t, rec.Body.String(), "claude")
	require.Contains(t, rec.Body.String(), `"model":"grok-4.3"`)
	require.Contains(t, rec.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_TestAccountConnection_GrokDefaultsEmptyModelTo45(t *testing.T) {
	gin.SetMode(gin.TestMode)

	account := &Account{
		ID:          16,
		Name:        "grok-oauth-default-model",
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "grok-access-token",
			"refresh_token": "grok-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n" +
				"data: {\"type\":\"response.completed\"}\n\n",
		)),
	}}
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		httpUpstream:      upstream,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/16/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "", "", AccountTestModeDefault)

	require.NoError(t, err)
	require.Equal(t, grokDefaultResponsesModel, gjson.GetBytes(upstream.lastBody, "model").String())
	require.Contains(t, recorder.Body.String(), `"model":"grok-4.5"`)
}

func TestAccountTestService_Grok429PersistsRateLimitReset(t *testing.T) {
	gin.SetMode(gin.TestMode)

	account := &Account{
		ID:          14,
		Name:        "grok-oauth-limited",
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "grok-access-token",
			"refresh_token": "grok-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	baseRepo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	repo := &grokAccountTestRateLimitRepo{mockAccountRepoForGemini: baseRepo}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{"45"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`)),
	}}
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		httpUpstream:      upstream,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/14/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "grok", "", AccountTestModeDefault)

	require.Error(t, err)
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.WithinDuration(t, time.Now().Add(45*time.Second), repo.resetAt, time.Second)
}

func TestAccountTestService_Grok429WithoutQuotaHeadersUsesFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 15, Name: "grok-oauth-limited-no-headers", Platform: PlatformGrok,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "grok-access-token",
			"refresh_token": "grok-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	baseRepo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	repo := &grokAccountTestRateLimitRepo{mockAccountRepoForGemini: baseRepo}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"quota exhausted"}}`)),
	}}
	svc := &AccountTestService{
		accountRepo: repo, grokTokenProvider: NewGrokTokenProvider(repo, nil), httpUpstream: upstream,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/15/test", nil)
	before := time.Now()

	err := svc.TestAccountConnection(c, account.ID, "grok", "", AccountTestModeDefault)

	require.Error(t, err)
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.WithinDuration(t, before.Add(grokRateLimitFallbackCooldown), repo.resetAt, time.Second)
}

func TestAccountTestService_GrokImageModelUsesImagesGenerations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 17, Name: "grok-oauth-image", Platform: PlatformGrok,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "grok-access-token",
			"refresh_token": "grok-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
			"base_url":      "https://cli-chat-proxy.grok.com/v1",
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"data":[{"b64_json":"QUJD","mime_type":"image/jpeg"}]}`,
		)),
	}}
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		httpUpstream:      upstream,
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/17/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "grok-imagine-image", "a red apple", AccountTestModeDefault)

	require.NoError(t, err)
	require.Equal(t, "https://api.x.ai/v1/images/generations", upstream.lastReq.URL.String())
	require.Equal(t, "grok-imagine-image", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "a red apple", gjson.GetBytes(upstream.lastBody, "prompt").String())
	require.Equal(t, "b64_json", gjson.GetBytes(upstream.lastBody, "response_format").String())
	require.Contains(t, rec.Body.String(), `"type":"image"`)
	require.Contains(t, rec.Body.String(), "data:image/jpeg;base64,QUJD")
	require.Contains(t, rec.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_GrokWebSearchModeUsesResponsesWebSearchTool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 18, Name: "grok-oauth-search", Platform: PlatformGrok,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "grok-access-token",
			"refresh_token": "grok-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`{"id":"r1","output":[{"type":"web_search_call","id":"ws1","status":"completed"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Grok is built by xAI."}]}]}`,
		)),
	}}
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		httpUpstream:      upstream,
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/18/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "grok-4.5", "xAI Grok", AccountTestModeGrokSearch)

	require.NoError(t, err)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/responses", upstream.lastReq.URL.String())
	// Standalone web_search wraps the query in the gateway-style prompt.
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "input").String(), "xAI Grok")
	require.Equal(t, "web_search", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
	require.Equal(t, "web_search_call.action.sources", gjson.GetBytes(upstream.lastBody, "include.0").String())
	require.Contains(t, rec.Body.String(), "web_search ok")
	require.Contains(t, rec.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_GrokTTSIncludesLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 19, Name: "grok-oauth-tts", Platform: PlatformGrok,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "grok-access-token",
			"refresh_token": "grok-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"audio/mpeg"}},
		Body:       io.NopCloser(strings.NewReader("ID3fakeaudio")),
	}}
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		httpUpstream:      upstream,
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/19/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "", "hello voice", AccountTestModeGrokTTS)

	require.NoError(t, err)
	require.Equal(t, "https://api.x.ai/v1/tts", upstream.lastReq.URL.String())
	require.Equal(t, "hello voice", gjson.GetBytes(upstream.lastBody, "text").String())
	require.Equal(t, "en", gjson.GetBytes(upstream.lastBody, "language").String())
	require.Contains(t, rec.Body.String(), "tts ok")
	require.Contains(t, rec.Body.String(), `"type":"audio"`)
	require.Contains(t, rec.Body.String(), "data:audio/mpeg;base64,")
	require.Contains(t, rec.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_GrokImageEditUsesUploadedImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 24, Name: "grok-oauth-image-edit", Platform: PlatformGrok,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "grok-access-token",
			"refresh_token": "grok-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"data":[{"b64_json":"QUJD","mime_type":"image/png"}]}`,
		)),
	}}
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		httpUpstream:      upstream,
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/24/test", nil)

	// 8x8 solid PNG (xAI min dimension is 8px).
	src := minimalAccountTestPNGDataURL(8, 8)
	err := svc.TestAccountConnection(c, account.ID, "grok-imagine-image", "edit me", AccountTestModeGrokImage, AccountTestOptions{
		ImageDataURL: src,
	})

	require.NoError(t, err)
	require.Equal(t, "https://api.x.ai/v1/images/edits", upstream.lastReq.URL.String())
	require.True(t, strings.HasPrefix(gjson.GetBytes(upstream.lastBody, "image.url").String(), "data:image/png;base64,"))
	require.Equal(t, "image_url", gjson.GetBytes(upstream.lastBody, "image.type").String())
	require.Equal(t, "b64_json", gjson.GetBytes(upstream.lastBody, "response_format").String())
	// concrete image model ids pass through; only bare "grok-imagine" is aliased.
	require.Equal(t, "grok-imagine-image", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Contains(t, rec.Body.String(), `"type":"image"`)
}

func TestAccountTestService_GrokImageEditRejectsTinySource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 25, Name: "grok-oauth-image-tiny", Platform: PlatformGrok,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "grok-access-token",
			"refresh_token": "grok-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		httpUpstream:      &httpUpstreamRecorder{},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/25/test", nil)

	// 1x1 PNG data URL
	tiny := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	err := svc.TestAccountConnection(c, account.ID, "grok-imagine-image-quality", "edit", AccountTestModeGrokImage, AccountTestOptions{
		ImageDataURL: tiny,
	})
	require.Error(t, err)
	require.Contains(t, rec.Body.String(), "too small")
}

// minimalAccountTestPNGDataURL builds a solid RGBA PNG as a data URL for tests.
func minimalAccountTestPNGDataURL(w, h int) string {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestAccountTestService_GrokExplicitImageModeDefaultsModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 20, Name: "grok-oauth-image-mode", Platform: PlatformGrok,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "grok-access-token",
			"refresh_token": "grok-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"data":[{"b64_json":"QUJD","mime_type":"image/jpeg"}]}`,
		)),
	}}
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		httpUpstream:      upstream,
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/20/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "", "", AccountTestModeGrokImage)

	require.NoError(t, err)
	require.Equal(t, "https://api.x.ai/v1/images/generations", upstream.lastReq.URL.String())
	require.Equal(t, "grok-imagine-image", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "b64_json", gjson.GetBytes(upstream.lastBody, "response_format").String())
	require.Contains(t, rec.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_GrokVideoUpstreamErrorIsNotMaskedAsSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 21, Name: "grok-oauth-video-err", Platform: PlatformGrok,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "grok-access-token",
			"refresh_token": "grok-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"code":"invalid-argument","error":"bad video request"}`,
		)),
	}}
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		httpUpstream:      upstream,
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/21/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "grok-imagine-video", "bounce ball", AccountTestModeGrokVideo)

	require.Error(t, err)
	require.Equal(t, "https://api.x.ai/v1/videos/generations", upstream.lastReq.URL.String())
	require.Contains(t, rec.Body.String(), `"type":"error"`)
	require.Contains(t, rec.Body.String(), "Grok videos API returned 400")
	require.NotContains(t, rec.Body.String(), `"success":true`)
}

type grokRealtimeTestConn struct {
	msg []byte
}

func (c *grokRealtimeTestConn) WriteJSON(context.Context, any) error { return nil }
func (c *grokRealtimeTestConn) ReadMessage(context.Context) ([]byte, error) {
	if c == nil || len(c.msg) == 0 {
		return nil, context.DeadlineExceeded
	}
	return c.msg, nil
}
func (c *grokRealtimeTestConn) Ping(context.Context) error { return nil }
func (c *grokRealtimeTestConn) Close() error               { return nil }

type grokRealtimeTestDialer struct {
	lastURL   string
	lastAuth  string
	lastProxy string
	conn      openAIWSClientConn
	err       error
	status    int
}

func (d *grokRealtimeTestDialer) Dial(_ context.Context, wsURL string, headers http.Header, proxyURL string) (openAIWSClientConn, int, http.Header, error) {
	d.lastURL = wsURL
	d.lastAuth = headers.Get("Authorization")
	d.lastProxy = proxyURL
	if d.err != nil {
		return nil, d.status, nil, d.err
	}
	if d.conn == nil {
		d.conn = &grokRealtimeTestConn{}
	}
	return d.conn, 0, nil, nil
}

func TestAccountTestService_GrokRealtimeModeDialsWS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 22, Name: "grok-oauth-realtime", Platform: PlatformGrok,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "grok-access-token",
			"refresh_token": "grok-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	dialer := &grokRealtimeTestDialer{
		conn: &grokRealtimeTestConn{msg: []byte(`{"type":"session.created","session":{"id":"sess_1"}}`)},
	}
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		grokWSDialer:      dialer,
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/22/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "", "", AccountTestModeGrokRealtime)

	require.NoError(t, err)
	require.Contains(t, dialer.lastURL, "wss://api.x.ai/v1/realtime")
	require.Contains(t, dialer.lastURL, "model=grok-voice-latest")
	require.Equal(t, "Bearer grok-access-token", dialer.lastAuth)
	require.Contains(t, rec.Body.String(), "realtime ws handshake ok")
	require.Contains(t, rec.Body.String(), "session.created")
	require.Contains(t, rec.Body.String(), `"type":"test_complete"`)
	require.Contains(t, rec.Body.String(), `"success":true`)
}

func TestAccountTestService_GrokRealtimeModeDialFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{
		ID: 23, Name: "grok-oauth-realtime-fail", Platform: PlatformGrok,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "grok-access-token",
			"refresh_token": "grok-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	dialer := &grokRealtimeTestDialer{
		status: 401,
		err:    &openAIWSHandshakeError{Body: []byte(`{"error":"unauthorized"}`), Err: errors.New("websocket handshake failed")},
	}
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		grokWSDialer:      dialer,
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/23/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "", "", AccountTestModeGrokRealtime)

	require.Error(t, err)
	require.Contains(t, rec.Body.String(), `"type":"error"`)
	require.Contains(t, rec.Body.String(), "Realtime")
}
