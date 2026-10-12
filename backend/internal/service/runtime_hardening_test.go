package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type hardeningBalanceRepo struct {
	AccountRepository
	calls int
	until time.Time
}

func (r *hardeningBalanceRepo) SetTempUnschedulable(_ context.Context, _ int64, until time.Time, _ string) error {
	r.calls++
	r.until = until
	return nil
}

func TestRuntimeHardeningBalanceGuard(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"code":"INSUFFICIENT_BALANCE","message":"Insufficient account balance"}`, true},
		{`{"error":{"message":"insufficient balance"}}`, true},
		{`{"error":{"code":"permission_denied","message":"Access forbidden"}}`, false},
		{`{"error":{"message":"quota exceeded"}}`, false},
	} {
		require.Equal(t, tc.want, IsUpstreamInsufficientBalance(403, []byte(tc.body)))
	}
	require.False(t, IsUpstreamInsufficientBalance(200, []byte(`{"code":"INSUFFICIENT_BALANCE"}`)))
	repo := &hardeningBalanceRepo{}
	svc := &RateLimitService{accountRepo: repo}
	account := &Account{ID: 334, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}}
	got := svc.CheckErrorPolicy(context.Background(), account, 403, []byte(`{"code":"INSUFFICIENT_BALANCE"}`))
	require.Equal(t, ErrorPolicyTempUnscheduled, got)
	require.Equal(t, 1, repo.calls)
	require.WithinDuration(t, time.Now().Add(15*time.Minute), repo.until, time.Second)
	require.Equal(t, ErrorPolicySkipped, svc.CheckErrorPolicy(context.Background(), account, 403, []byte(`{"code":"permission_denied"}`)))
	require.Equal(t, 1, repo.calls)
}

func TestRuntimeHardeningGeminiCanceledDoesNotRetry(t *testing.T) {
	for _, chat := range []bool{false, true} {
		upstream := &httpUpstreamRecorder{err: context.Canceled}
		svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
		account := &Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
		body := []byte(`{"model":"gemini-2.5-flash","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`)
		started := time.Now()
		var err error
		if chat {
			_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body)
		} else {
			_, err = svc.Forward(context.Background(), c, account, body)
		}
		require.ErrorIs(t, err, context.Canceled)
		require.Len(t, upstream.requests, 1)
		require.Less(t, time.Since(started), time.Second)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	require.ErrorIs(t, waitGeminiBackoff(ctx, 5), context.Canceled)
	require.Less(t, time.Since(started), 100*time.Millisecond)
}

const hardeningPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jR5kAAAAASUVORK5CYII="

func TestRuntimeHardeningImagePreviewPipeline(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			requestID := fmt.Sprintf("hardening-%d", time.Now().UnixNano())
			apiKeyID := int64(991823)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			body := []byte(fmt.Sprintf(`{"model":"gpt-image-2","prompt":"fixture","stream":%t}`, stream))
			c.Request = httptest.NewRequest("POST", "/v1/images/generations", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("api_key", &APIKey{ID: apiKeyID})
			payload := fmt.Sprintf(`{"created":1,"data":[{"b64_json":"%s"}]}`, hardeningPNG)
			contentType := "application/json"
			if stream {
				payload = "data: " + payload + "\n\ndata: [DONE]\n\n"
				contentType = "text/event-stream"
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}, "X-Request-Id": {requestID}}, Body: io.NopCloser(strings.NewReader(payload))}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test", "base_url": "https://fixture.example/v1"}}
			result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")
			require.NoError(t, err)
			require.Equal(t, 1, result.ImageCount)
			resolved := ResolveUsageRecordID(context.Background(), result.RequestID)
			directory := filepath.Join(openAIImagePreviewDir, imagePreviewKey(resolved, apiKeyID))
			t.Cleanup(func() { _ = os.RemoveAll(directory) })
			require.Eventually(t, func() bool { _, err := findOpenAIImagePreview(resolved, apiKeyID, 0); return err == nil }, 3*time.Second, 10*time.Millisecond)
			data, mime, err := ReadOpenAIImagePreview(resolved, apiKeyID, 0)
			require.NoError(t, err)
			require.Equal(t, "image/png", mime)
			expected, _ := base64.StdEncoding.DecodeString(hardeningPNG)
			require.Equal(t, expected, data)
			require.Equal(t, []string{"/api/v1/admin/usage/9/images/0"}, ListOpenAIImagePreviewURLs(resolved, apiKeyID, 9, 1))
			_, _, err = ReadOpenAIImagePreview(resolved, apiKeyID+1, 0)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestRuntimeHardeningResponsesImageExtraction(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(fmt.Sprintf(`{"type":"response.output_item.done","item":{"type":"image_generation_call","result":"%s"}}`, hardeningPNG))
	CaptureOpenAIImagePreviewResponse(c, body)
	CaptureOpenAIImagePreviewResponse(c, body)
	require.Len(t, OpenAIImagePreviewAssetsFromContext(c), 1)
	clearOpenAIImagePreviews(c)
	require.Empty(t, OpenAIImagePreviewAssetsFromContext(c))
}

func TestRuntimeHardeningPreviewRejectsPrivateTargets(t *testing.T) {
	for _, u := range []string{"http://localhost/x.png", "http://127.0.0.1/a.png", "http://[::1]/a.png", "http://169.254.169.254/a.png", "http://10.1.2.3/a.png", "http://100.64.0.1/a.png", "https://user:pass@example.com/a.png", "file:///etc/passwd"} {
		require.False(t, isPreviewDownloadURL(u), u)
	}
	require.True(t, isPreviewDownloadURL("https://images.example.com/a.png"))
	for _, ip := range []string{"127.0.0.1", "::ffff:127.0.0.1", "fe80::1", "fc00::1", "192.168.1.1"} {
		require.False(t, isPublicPreviewIP(net.ParseIP(ip)))
	}
	_, err := dialPublicPreview(context.Background(), "tcp", "127.0.0.1:1")
	require.ErrorContains(t, err, "non-public")
}
