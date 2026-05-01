package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const testImageID = "img_abcdefghijklmnopqrstuvwxyzABCDEF"

type fakeOpenCodePublicSettingsProvider struct {
	settings *PublicSettings
	err      error
}

func (f *fakeOpenCodePublicSettingsProvider) GetPublicSettings(ctx context.Context) (*PublicSettings, error) {
	return f.settings, f.err
}

func newTestStoreWithImage(t *testing.T, id string, format string, data []byte) *OpenAIGeneratedImageStore {
	t.Helper()
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	_, err := store.saveDecodedForTest(id, format, data)
	require.NoError(t, err)
	return store
}

func TestRehydrateOpenCodeGeneratedImageMarkers_AddsSyntheticInputImage(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	req := map[string]any{"input": []any{map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Generated image: sub2api-image://" + testImageID}}}}}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.True(t, changed)
	input := req["input"].([]any)
	last := input[len(input)-1].(map[string]any)
	require.Equal(t, "user", last["role"])
	content := last["content"].([]any)
	require.Equal(t, "input_image", content[1].(map[string]any)["type"])
	require.Contains(t, content[1].(map[string]any)["image_url"], "data:image/png;base64,")
}

func TestRehydrateOpenCodeGeneratedImageMarkers_IsIdempotent(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	req := map[string]any{"input": "sub2api-image://" + testImageID}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})
	require.NoError(t, err)
	require.True(t, changed)
	inputLen := len(req["input"].([]any))

	changed, err = rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.False(t, changed)
	require.Len(t, req["input"].([]any), inputLen)
	encoded, _ := json.Marshal(req)
	require.Equal(t, 1, strings.Count(string(encoded), `"type":"input_image"`))
}

func TestRehydrateOpenCodeGeneratedImageMarkers_ExpiredMarkerAddsUnavailableText(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	store.now = func() time.Time { return fixedNow.Add(2 * time.Hour) }
	req := map[string]any{"input": []any{"sub2api-image://" + testImageID}}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.True(t, changed)
	encoded, _ := json.Marshal(req)
	require.Contains(t, string(encoded), "image bytes unavailable")
	require.NotContains(t, string(encoded), "input_image")
}

func TestRehydrateOpenCodeGeneratedImageMarkers_DedupesByMostRecentAndCaps(t *testing.T) {
	ids := []string{
		"img_abcdefghijklmnopqrstuvwxyzABCDEF",
		"img_bcdefghijklmnopqrstuvwxyzABCDEFG",
		"img_cdefghijklmnopqrstuvwxyzABCDEFGH",
		"img_defghijklmnopqrstuvwxyzABCDEFGHI",
	}
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	for _, id := range ids {
		_, err := store.saveDecodedForTest(id, "png", pngBytes)
		require.NoError(t, err)
	}
	req := map[string]any{"input": []any{strings.Join([]string{"sub2api-image://" + ids[0], "sub2api-image://" + ids[1], "sub2api-image://" + ids[2], "sub2api-image://" + ids[0], "sub2api-image://" + ids[3]}, " ")}}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.True(t, changed)
	encoded, _ := json.Marshal(req)
	require.Equal(t, 3, strings.Count(string(encoded), `"type":"input_image"`))
	input := req["input"].([]any)
	rehydratedIDs := make([]string, 0, 3)
	for _, item := range input[1:] {
		message := item.(map[string]any)
		content := message["content"].([]any)
		text := content[0].(map[string]any)["text"].(string)
		rehydratedIDs = append(rehydratedIDs, strings.TrimPrefix(strings.TrimSuffix(text, " from the previous response."), "Attached generated image "))
	}
	require.Equal(t, []string{ids[2], ids[0], ids[3]}, rehydratedIDs)
}

func TestRehydrateOpenCodeGeneratedImageMarkers_CapIsIdempotent(t *testing.T) {
	ids := []string{
		"img_abcdefghijklmnopqrstuvwxyzABCDEF",
		"img_bcdefghijklmnopqrstuvwxyzABCDEFG",
		"img_cdefghijklmnopqrstuvwxyzABCDEFGH",
		"img_defghijklmnopqrstuvwxyzABCDEFGHI",
	}
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	for _, id := range ids {
		_, err := store.saveDecodedForTest(id, "png", pngBytes)
		require.NoError(t, err)
	}
	req := map[string]any{"input": []any{strings.Join([]string{"sub2api-image://" + ids[0], "sub2api-image://" + ids[1], "sub2api-image://" + ids[2], "sub2api-image://" + ids[3]}, " ")}}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})
	require.NoError(t, err)
	require.True(t, changed)
	inputLen := len(req["input"].([]any))

	changed, err = rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.False(t, changed)
	require.Len(t, req["input"].([]any), inputLen)
	encoded, _ := json.Marshal(req)
	require.Equal(t, 3, strings.Count(string(encoded), `"type":"input_image"`))
	require.NotContains(t, string(encoded), "Attached generated image "+ids[0])
	for _, id := range ids[1:] {
		require.Contains(t, string(encoded), "Attached generated image "+id)
	}
}

func TestRehydrateOpenCodeGeneratedImageMarkers_TooLargeSkipsInputImage(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	req := map[string]any{"input": []any{"sub2api-image://" + testImageID}}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3, MaxRehydrateBytes: 4})

	require.NoError(t, err)
	require.True(t, changed)
	encoded, _ := json.Marshal(req)
	require.Contains(t, string(encoded), "image bytes were not attached because the image is too large")
	require.NotContains(t, string(encoded), "input_image")
}

func TestRehydrateOpenCodeGeneratedImageMarkers_MaxRehydrateBytesLimitsLoad(t *testing.T) {
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	rec, _, err := store.Load(context.Background(), testImageID)
	require.NoError(t, err)
	imagePath, err := safeOpenAIGeneratedImagePath(store.root, rec.Filename)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(imagePath, append(append([]byte(nil), pngBytes...), []byte("extra-bytes")...), 0o600))
	req := map[string]any{"input": []any{"sub2api-image://" + testImageID}}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3, MaxRehydrateBytes: 4})

	require.NoError(t, err)
	require.True(t, changed)
	encoded, _ := json.Marshal(req)
	require.Contains(t, string(encoded), "image bytes were not attached because the image is too large")
	require.NotContains(t, string(encoded), "image bytes unavailable")
	require.NotContains(t, string(encoded), "input_image")
}

func TestRehydrateOpenCodeGeneratedImageMarkers_ScansDownloadPathsAndAbsoluteURLs(t *testing.T) {
	ids := []string{"img_abcdefghijklmnopqrstuvwxyzABCDEF", "img_bcdefghijklmnopqrstuvwxyzABCDEFG"}
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	for _, id := range ids {
		_, err := store.saveDecodedForTest(id, "png", pngBytes)
		require.NoError(t, err)
	}
	req := map[string]any{"input": []any{map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Download path: /sub2api/generated-images/" + ids[0] + ".png\nDownload: https://example.com/sub2api/generated-images/" + ids[1] + ".png"}}}}}

	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})

	require.NoError(t, err)
	require.True(t, changed)
	encoded, _ := json.Marshal(req)
	require.Equal(t, 2, strings.Count(string(encoded), `"type":"input_image"`))
}

func TestRedactOpenCodeGeneratedImagesForOps_RemovesDataURLsAndResults(t *testing.T) {
	body := []byte(`{"input":[{"content":[{"type":"input_image","image_url":"data:image/png;base64,` + pngB64 + `"}]}],"output":[{"type":"image_generation_call","result":"` + pngB64 + `"}]}`)

	redacted := redactOpenCodeGeneratedImagesForOps(body)

	require.NotContains(t, string(redacted), "data:image")
	require.NotContains(t, string(redacted), pngB64)
	require.Contains(t, string(redacted), "[redacted-input-image]")
	require.Contains(t, string(redacted), "[redacted-image-result]")
}

func TestRedactOpenCodeGeneratedImagesForOps_MalformedJSONFailClosed(t *testing.T) {
	body := []byte(`{"input":"data:image/png;base64,` + pngB64 + `","marker":"sub2api-image://` + testImageID + `","path":"/sub2api/generated-images/` + testImageID + `.png"`)

	redacted := redactOpenCodeGeneratedImagesForOps(body)

	require.NotContains(t, string(redacted), "data:image")
	require.NotContains(t, string(redacted), pngB64)
	require.NotContains(t, string(redacted), testImageID)
	require.Contains(t, string(redacted), "[redacted-input-image]")
	require.Contains(t, string(redacted), "/sub2api/generated-images/[redacted]")
	require.Contains(t, string(redacted), "sub2api-image://[redacted]")
}

func TestRedactOpenCodeGeneratedImagesForOps_MalformedJSONRedactsBareImageBase64Fields(t *testing.T) {
	body := []byte(`{"type":"image_generation_call","result":"` + pngB64 + `","partial_image_b64":"` + pngB64 + `"`)

	redacted := redactOpenCodeGeneratedImagesForOps(body)

	require.NotContains(t, string(redacted), pngB64)
	require.Contains(t, string(redacted), "[redacted-image-result]")
	require.Contains(t, string(redacted), "[redacted-partial-image]")
}

func TestRewriteOpenCodeImageGenerationOutput_ReplacesImageCallWithMessage(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	body := []byte(`{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}`)

	patched, changed, err := rewriteOpenCodeImageGenerationOutput(context.Background(), body, store, openCodeImageRewriteOptions{BaseURL: "https://example.com"})

	require.NoError(t, err)
	require.True(t, changed)
	require.Regexp(t, `^msg_sub2api_img_`, gjson.GetBytes(patched, "output.0.id").String())
	require.Equal(t, "message", gjson.GetBytes(patched, "output.0.type").String())
	require.Equal(t, "completed", gjson.GetBytes(patched, "output.0.status").String())
	require.Equal(t, "assistant", gjson.GetBytes(patched, "output.0.role").String())
	require.Equal(t, "output_text", gjson.GetBytes(patched, "output.0.content.0.type").String())
	require.Equal(t, int64(0), gjson.GetBytes(patched, "output.0.content.0.annotations.#").Int())
	require.Contains(t, gjson.GetBytes(patched, "output.0.content.0.text").String(), "sub2api-image://img_")
	require.Contains(t, gjson.GetBytes(patched, "output.0.content.0.text").String(), "https://example.com/sub2api/generated-images/")
	require.NotContains(t, string(patched), "image_generation_call")
	require.NotContains(t, string(patched), pngB64)
}

func TestRewriteOpenCodeImageGenerationOutput_DoesNotExposeContinuationToolCall(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	body := []byte(`{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}`)

	patched, changed, err := rewriteOpenCodeImageGenerationOutput(context.Background(), body, store, openCodeImageRewriteOptions{BaseURL: "https://example.com"})

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, int64(1), gjson.GetBytes(patched, "output.#").Int())
	require.Equal(t, "message", gjson.GetBytes(patched, "output.0.type").String())
	require.Contains(t, gjson.GetBytes(patched, "output.0.content.0.text").String(), "I'll download from URL: https://example.com/sub2api/generated-images/")
	require.NotContains(t, string(patched), `"type":"function_call"`)
	require.NotContains(t, string(patched), `"name":"bash"`)
	require.NotContains(t, string(patched), "Server download path")
	require.NotContains(t, string(patched), pngB64)
}

func TestResolveOpenCodeImageDownloadBaseURL_PrefersConfiguredFrontendURL(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Host = "attacker.example"
	cfg := &config.Config{}
	cfg.Server.FrontendURL = "https://sub2api.example/app/"

	require.Equal(t, "https://sub2api.example/app", resolveOpenCodeImageDownloadBaseURL(c, cfg))
}

func TestOpenAIGatewayResolveOpenCodeImageDownloadBaseURL_PrefersPublicAPIBaseURL(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Host = "attacker.example"
	cfg := &config.Config{}
	cfg.Server.FrontendURL = "https://frontend.example/app/"
	svc := &OpenAIGatewayService{
		cfg: cfg,
		publicSettingsProvider: &fakeOpenCodePublicSettingsProvider{
			settings: &PublicSettings{APIBaseURL: "https://api.example.com/v1/"},
		},
	}

	require.Equal(t, "https://api.example.com", svc.resolveOpenCodeImageDownloadBaseURL(context.Background(), c))
}

func TestOpenAIGatewayResolveOpenCodeImageDownloadBaseURL_FallsBackWhenPublicAPIBaseURLUnsafe(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Host = "attacker.example"
	cfg := &config.Config{}
	cfg.Server.FrontendURL = "https://frontend.example/app/"
	svc := &OpenAIGatewayService{
		cfg: cfg,
		publicSettingsProvider: &fakeOpenCodePublicSettingsProvider{
			settings: &PublicSettings{APIBaseURL: "javascript:alert(1)"},
		},
	}

	require.Equal(t, "https://frontend.example/app", svc.resolveOpenCodeImageDownloadBaseURL(context.Background(), c))
}

func TestResolveOpenCodeImageDownloadBaseURL_RejectsUntrustedHostFallback(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Host = "attacker.example"

	require.Equal(t, "", resolveOpenCodeImageDownloadBaseURL(c, &config.Config{}))
}

func TestResolveOpenCodeImageDownloadBaseURL_UsesTrustedForwardedHost(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.RemoteAddr = "10.0.0.10:12345"
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	c.Request.Header.Set("X-Forwarded-Host", " images.example, ignored.example ")
	cfg := &config.Config{}
	cfg.Server.TrustedProxies = []string{"10.0.0.0/24"}

	require.Equal(t, "https://images.example", resolveOpenCodeImageDownloadBaseURL(c, cfg))
}

func TestResolveOpenCodeImageDownloadBaseURL_RejectsUntrustedForwardedHost(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.RemoteAddr = "10.0.1.10:12345"
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	c.Request.Header.Set("X-Forwarded-Host", "images.example")
	cfg := &config.Config{}
	cfg.Server.TrustedProxies = []string{"10.0.0.0/24"}

	require.Equal(t, "", resolveOpenCodeImageDownloadBaseURL(c, cfg))
}

func TestResolveOpenCodeImageDownloadBaseURL_UsesTrustedRequestHostFallback(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.RemoteAddr = "10.0.0.10:12345"
	c.Request.Host = "images.example"
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	cfg := &config.Config{}
	cfg.Server.TrustedProxies = []string{"10.0.0.10"}

	require.Equal(t, "https://images.example", resolveOpenCodeImageDownloadBaseURL(c, cfg))
}

func TestResolveOpenCodeImageDownloadBaseURL_RejectsUnsafeFallbackInputs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		proto string
		host  string
	}{
		{name: "bad proto", proto: "javascript", host: "images.example"},
		{name: "host with path", proto: "https", host: "images.example/path"},
		{name: "host with newline", proto: "https", host: "images.example\nattacker.example"},
		{name: "host with query", proto: "https", host: "images.example?x=1"},
		{name: "host with userinfo", proto: "https", host: "user@images.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.RemoteAddr = "10.0.0.10:12345"
			c.Request.Header.Set("X-Forwarded-Proto", tc.proto)
			c.Request.Header.Set("X-Forwarded-Host", tc.host)
			cfg := &config.Config{}
			cfg.Server.TrustedProxies = []string{"10.0.0.0/24"}

			require.Equal(t, "", resolveOpenCodeImageDownloadBaseURL(c, cfg))
		})
	}
}

func TestBuildOpenCodeGeneratedImageMessage_UsesRelativeOnlyWhenBaseURLEmpty(t *testing.T) {
	rec := OpenAIGeneratedImageRecord{
		ID:        "img_abcdefghijklmnopqrstuvwxyzABCDEF",
		Filename:  "img_abcdefghijklmnopqrstuvwxyzABCDEF.png",
		Format:    "png",
		MIME:      "image/png",
		ExpiresAt: fixedNow.Add(time.Hour),
	}

	msg := buildOpenCodeGeneratedImageMessage(rec, openCodeImageRewriteOptions{})
	content := msg["content"].([]any)[0].(map[string]any)["text"].(string)

	require.Contains(t, content, "sub2api-image://img_abcdefghijklmnopqrstuvwxyzABCDEF")
	require.NotContains(t, content, "Server download path")
	require.NotContains(t, content, "/sub2api/generated-images/")
	require.NotContains(t, content, "Do not treat")
	require.NotContains(t, content, "If no Download URL")
	require.NotContains(t, content, "Download URL:")
}

func TestBuildOpenCodeGeneratedImageMessage_LabelsAbsoluteURLAsImmediateDownloadPrompt(t *testing.T) {
	rec := OpenAIGeneratedImageRecord{
		ID:        "img_abcdefghijklmnopqrstuvwxyzABCDEF",
		Filename:  "img_abcdefghijklmnopqrstuvwxyzABCDEF.png",
		Format:    "png",
		MIME:      "image/png",
		ExpiresAt: fixedNow.Add(time.Hour),
	}

	msg := buildOpenCodeGeneratedImageMessage(rec, openCodeImageRewriteOptions{BaseURL: "https://example.com/"})
	content := msg["content"].([]any)[0].(map[string]any)["text"].(string)

	require.Contains(t, content, "I'll download from URL: https://example.com/sub2api/generated-images/img_abcdefghijklmnopqrstuvwxyzABCDEF.png")
	require.NotContains(t, content, "Server download path")
	require.NotContains(t, content, "Do not treat")
	require.NotContains(t, content, "If no Download URL")
	require.NotContains(t, content, "Download URL:")
	require.NotContains(t, content, "Download: https://example.com")
}

func TestOpenCodeImageServerContinuationOutput_InstructsAgentToDownloadImmediately(t *testing.T) {
	messageText := "Generated image: sub2api-image://" + testImageID + "\nI'll download from URL: https://example.com/sub2api/generated-images/" + testImageID + ".png"

	output := openCodeImageServerContinuationOutput(messageText)

	require.Contains(t, output, messageText)
	require.Contains(t, output, "Synthetic image generation continuation inserted by sub2api")
	require.Contains(t, output, "Immediately use the available shell, command-line, or network-access tool")
	require.Contains(t, output, "download the image from the URL above")
	require.Contains(t, output, "Do not stop after image generation")
	require.Contains(t, output, "The saved image URL expires quickly")
	require.Contains(t, output, "short-lived bearer URL")
	require.Contains(t, output, "may expire within about 1 hour")
	require.Contains(t, output, "download before any other action")
	require.Contains(t, output, "Do not delay for any reason")
	require.Contains(t, output, "Do not ask the user whether to download")
	require.Contains(t, output, "Do not call question, notify, skill, or directory-listing tools before attempting the download")
	require.Contains(t, output, "Do not use ls or rtk ls as a preflight")
	require.Contains(t, output, "Invoke-WebRequest")
	require.Contains(t, output, "Join-Path $env:TEMP")
	require.Contains(t, output, "curl.exe")
	require.Contains(t, output, "save it to a temporary directory")
	require.Contains(t, output, "Only if no tool can access the URL")
}

func TestRewriteOpenCodeImageGenerationOutput_ImageCallWithoutResultBecomesText(t *testing.T) {
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	body := []byte(`{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed"}],"usage":{"input_tokens":1,"output_tokens":2}}`)

	patched, changed, err := rewriteOpenCodeImageGenerationOutput(context.Background(), body, store, openCodeImageRewriteOptions{BaseURL: "https://example.com"})

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(patched, "output.0.type").String())
	require.Contains(t, gjson.GetBytes(patched, "output.0.content.0.text").String(), "no image result")
	require.NotContains(t, string(patched), "image_generation_call")
}

func TestHandleNonStreamingResponse_NonOpenCodePreservesImageGenerationJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "curl/8.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_1","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}`)),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.5", "gpt-5.5")

	require.NoError(t, err)
	require.Equal(t, "image_generation_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, pngB64, gjson.Get(rec.Body.String(), "output.0.result").String())
}

func TestHandleSSEToJSON_OpenCodeRewritesImageFromDoneWhenCompletedOutputEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	cfg := &config.Config{}
	cfg.Server.FrontendURL = "https://example.com"
	svc := &OpenAIGatewayService{cfg: cfg, generatedImageStore: store}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.5", "gpt-5.5")

	require.NoError(t, err)
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Contains(t, gjson.Get(rec.Body.String(), "output.0.content.0.text").String(), "sub2api-image://img_")
	require.Contains(t, rec.Body.String(), "https://example.com/sub2api/generated-images/")
	require.NotContains(t, rec.Body.String(), "image_generation_call")
	require.NotContains(t, rec.Body.String(), pngB64)
}

func TestHandleSSEToJSON_OpenCodeRewritesEventTypedImageDoneWithoutDataType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_item.done",
			`data: {"output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.5", "gpt-5.5")

	require.NoError(t, err)
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Contains(t, gjson.Get(rec.Body.String(), "output.0.content.0.text").String(), "sub2api-image://img_")
	require.NotContains(t, rec.Body.String(), "image_generation_call")
	require.NotContains(t, rec.Body.String(), pngB64)
}

func TestHandleSSEToJSON_OpenCodeRewritesDataOnlyImageMarkerWhenCompletedOutputEmpty(t *testing.T) {
	for _, tc := range []struct {
		name    string
		account *Account
	}{
		{
			name:    "oauth canonical merge",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		},
		{
			name:    "api key reconstruction",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			store := newTestOpenAIGeneratedImageStore(t, fixedNow)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("User-Agent", "opencode/1.0")
			svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
			body := []byte(strings.Join([]string{
				`data: {"output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
				``,
				`event: response.completed`,
				`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
				``,
			}, "\n"))

			_, err := svc.handleSSEToJSONForAccount(resp, c, body, tc.account, "gpt-5.5", "gpt-5.5")

			require.NoError(t, err)
			require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.0.type").String())
			require.Contains(t, gjson.Get(rec.Body.String(), "output.0.content.0.text").String(), "sub2api-image://img_")
			require.NotContains(t, rec.Body.String(), "image_generation_call")
			require.NotContains(t, rec.Body.String(), pngB64)
		})
	}
}

func TestHandleSSEToJSON_OpenCodeResponseFailedUsesEventTypeWhenDataTypeMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	body := []byte(strings.Join([]string{
		`event: response.failed`,
		`data: {"response":{"error":{"message":"upstream rejected request"}}}`,
		``,
	}, "\n"))

	usage, err := svc.handleSSEToJSONForAccount(resp, c, body, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.5", "gpt-5.5")

	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "upstream rejected request")
	require.NotContains(t, rec.Body.String(), "event: response.failed")
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
}

func TestHandleSSEToJSON_OpenCodeRewritesOtherTerminalOutputImage(t *testing.T) {
	for _, eventType := range []string{"response.incomplete", "response.cancelled", "response.canceled"} {
		t.Run(eventType, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			store := newTestOpenAIGeneratedImageStore(t, fixedNow)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("User-Agent", "opencode/1.0")
			svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(strings.Join([]string{
					"event: " + eventType,
					`data: {"type":"` + eventType + `","response":{"id":"resp_1","model":"gpt-5.5","output":[{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
					"",
				}, "\n"))),
			}

			_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, "gpt-5.5", "gpt-5.5")

			require.NoError(t, err)
			require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.0.type").String())
			require.Contains(t, gjson.Get(rec.Body.String(), "output.0.content.0.text").String(), "sub2api-image://img_")
			require.NotContains(t, rec.Body.String(), "image_generation_call")
			require.NotContains(t, rec.Body.String(), pngB64)
		})
	}
}

func TestHandleSSEToJSON_OpenCodeRewritesImageWithoutResultFromDoneWhenCompletedOutputEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed"}}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-5.5", "gpt-5.5")

	require.NoError(t, err)
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Contains(t, gjson.Get(rec.Body.String(), "output.0.content.0.text").String(), "no image result")
	require.NotContains(t, rec.Body.String(), "image_generation_call")
}

func TestHandleSSEToJSON_OpenCodePreservesOutputIndexOrderWhenReconstructingImageAndMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
			"",
			"event: response.output_text.delta",
			`data: {"type":"response.output_text.delta","output_index":1,"content_index":0,"delta":"ordinary text"}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-5.5", "gpt-5.5")

	require.NoError(t, err)
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Contains(t, gjson.Get(rec.Body.String(), "output.0.content.0.text").String(), "sub2api-image://img_")
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.1.type").String())
	require.Equal(t, "ordinary text", gjson.Get(rec.Body.String(), "output.1.content.0.text").String())
	require.NotContains(t, rec.Body.String(), `"name":"bash"`)
	require.NotContains(t, rec.Body.String(), pngB64)
}

func TestHandleSSEToJSON_OpenCodePreservesDoneOnlyNonImageOutputWhenReconstructingImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}, generatedImageStore: store}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"do_work","arguments":"{}","status":"completed"}}`,
			"",
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":1,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-5.5", "gpt-5.5")

	require.NoError(t, err)
	require.Equal(t, "function_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, "call_1", gjson.Get(rec.Body.String(), "output.0.call_id").String())
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.1.type").String())
	require.Contains(t, gjson.Get(rec.Body.String(), "output.1.content.0.text").String(), "sub2api-image://img_")
	require.False(t, gjson.Get(rec.Body.String(), "output.2").Exists())
	require.NotContains(t, rec.Body.String(), `"name":"bash"`)
	require.NotContains(t, rec.Body.String(), pngB64)
}

func TestHandleSSEToJSON_OpenCodeRejectsImageSSEWithoutTerminalResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.image_generation_call.partial_image",
			`data: {"type":"response.image_generation_call.partial_image","output_index":0,"partial_image_index":0,"partial_image_b64":"` + pngB64 + `"}`,
			"",
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"ig_1","type":"image_generation_call","status":"completed","result":"` + pngB64 + `","output_format":"png"}}`,
			"",
		}, "\n"))),
	}

	usage, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-5.5", "gpt-5.5")

	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.NotContains(t, rec.Body.String(), "image_generation_call")
	require.NotContains(t, rec.Body.String(), pngB64)
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
}
