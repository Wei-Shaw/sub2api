//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type webImageFixtureUpstream struct {
	HTTPUpstream
	image          []byte
	downloadURL    string
	paths          []string
	assetRequests  []*http.Request
	cleanupDone    chan struct{}
	preparedParent string
	preparedTrace  string
	preparedModel  string
}

func (f *webImageFixtureUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	if !HTTPUpstreamPublicHostsOnly(req.Context()) {
		return nil, errors.New("signed asset request did not enforce public-host validation")
	}
	if req.Header.Get("Authorization") != "" || req.Header.Get("OAI-Device-Id") != "" || req.Header.Get("OAI-Session-Id") != "" || req.Header.Get("oai-did") != "" || req.Header.Get("ChatGPT-Account-ID") != "" {
		return nil, errors.New("signed asset request leaked ChatGPT credentials")
	}
	f.assetRequests = append(f.assetRequests, req.Clone(req.Context()))
	switch {
	case req.URL.Host == "reference.example" && req.URL.Path == "/reference.png" && req.Method == http.MethodGet:
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(f.image))}, nil
	case req.URL.Host == "fixture.example" && req.URL.Path == "/upload" && req.Method == http.MethodPut:
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(nil))}, nil
	case req.URL.Host == "fixture.example" && req.URL.Path == "/image.png" && req.Method == http.MethodGet:
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(f.image))}, nil
	default:
		return nil, errors.New("unexpected signed asset request: " + req.URL.String())
	}
}

func (f *webImageFixtureUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	if profile == nil || profile.Preset != tlsfingerprint.PresetChrome {
		return nil, errors.New("web image request must use Chrome TLS preset")
	}
	f.paths = append(f.paths, req.URL.Path)
	if !HTTPUpstreamRedirectsDisabled(req.Context()) {
		return nil, errors.New("authenticated web image request did not disable redirects")
	}
	if req.URL.Host == "chatgpt.com" && req.URL.Path != "/" && req.Header.Get("Authorization") != "Bearer fixture-access" {
		return nil, errors.New("web image request did not use account access_token")
	}
	if req.URL.Path == "/backend-api/f/conversation/prepare" || req.URL.Path == "/backend-api/f/conversation" {
		var payload map[string]any
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
		if payload["parent_message_id"] != "client-created-root" {
			return nil, errors.New("new image turn has an invalid parent")
		}
		if req.URL.Path == "/backend-api/f/conversation/prepare" {
			if payload["client_prepare_state"] != "sent" {
				return nil, errors.New("prepare state must be sent")
			}
			f.preparedParent, _ = payload["parent_message_id"].(string)
			f.preparedModel, _ = payload["model"].(string)
			f.preparedTrace = req.Header.Get("X-Oai-Turn-Trace-Id")
		} else {
			if payload["client_prepare_state"] != "success" || payload["parent_message_id"] != f.preparedParent || payload["model"] != f.preparedModel || req.Header.Get("X-Oai-Turn-Trace-Id") != f.preparedTrace {
				return nil, errors.New("generation lost prepared turn context")
			}
			if _, present := payload["system_hints"]; present {
				return nil, errors.New("generation injected legacy image hints")
			}
		}
		if disabled, _ := payload["history_and_training_disabled"].(bool); disabled {
			return nil, errors.New("image request used temporary chat mode")
		}

	}
	var body string
	switch req.URL.Path {
	case "/":
		body = `<html><script src="/backend-api/sentinel/sdk.js"></script></html>`
	case "/backend-api/conversation/init":
		body = `{"default_model_slug":"gpt-6-pro"}`
	case "/backend-api/sentinel/chat-requirements/prepare":
		body = `{"prepare_token":"fixture-prepare","proofofwork":{"required":false}}`
	case "/backend-api/sentinel/chat-requirements/finalize":
		body = `{"token":"fixture-sentinel"}`
	case "/backend-api/files":
		body = `{"file_id":"fixture-input","upload_url":"https://fixture.example/upload"}`
	case "/backend-api/files/fixture-input/uploaded":
		body = `{}`
	case "/backend-api/f/conversation/prepare":
		body = `{"conduit_token":"fixture-conduit"}`
	case "/backend-api/f/conversation":
		body = "data: {\"v\":{\"conversation_id\":\"fixture-conversation\",\"message\":{\"author\":{\"role\":\"tool\"},\"metadata\":{\"async_task_type\":\"image_gen\"},\"content\":{\"parts\":[{\"asset_pointer\":\"file-service://fixture-image\"}]}}}}\n\n"
	case "/backend-api/files/download/fixture-image":
		if req.URL.Query().Get("conversation_id") != "fixture-conversation" {
			return nil, errors.New("generated image download is missing its conversation context")
		}
		downloadURL := f.downloadURL
		if downloadURL == "" {
			downloadURL = "https://fixture.example/image.png"
		}
		body = `{"download_url":` + strconv.Quote(downloadURL) + `}`
	case "/backend-api/files/fixture-image/content":
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(f.image))}, nil
	case "/backend-api/conversation/id/fixture-conversation":
		if req.Method != http.MethodDelete {
			return nil, errors.New("expected generated image conversation cleanup")
		}
		if f.cleanupDone != nil {
			select {
			case f.cleanupDone <- struct{}{}:
			default:
			}
		}
		body = `{}`
	default:
		return nil, errors.New("unexpected upstream path: " + req.URL.Path)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(body))}, nil
}

func requireWebImageCleanup(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for generated image conversation cleanup")
	}
}

func TestWebImageAccountCreateScheduleForwardFixture(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 1, 1))
	picture.Set(0, 0, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, picture))

	account, err := buildAccountForCreate(&CreateAccountInput{
		Name: "web-image-fixture", Platform: PlatformOpenAI, Type: AccountTypeWebImage,
		Credentials: map[string]any{"access_token": "fixture-access", "refresh_token": "fixture-refresh"},
		Concurrency: 1,
	}, map[string]any{})
	require.NoError(t, err)
	account.ID = 9201
	cleanupDone := make(chan struct{}, 1)
	transport := &webImageFixtureUpstream{image: encoded.Bytes(), cleanupDone: cleanupDone}
	svc := &OpenAIGatewayService{
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{*account}},
		cache:       &schedulerTestGatewayCache{}, cfg: &config.Config{},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}), httpUpstream: transport,
	}
	selection, _, err := svc.SelectAccountWithSchedulerForImages(context.Background(), nil, "", "gpt-image-2.5-sunburst", nil, OpenAIImagesCapabilityNative)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, account.ID, selection.Account.ID)
	if selection.ReleaseFunc != nil {
		defer selection.ReleaseFunc()
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	parsed := &OpenAIImagesRequest{
		Endpoint: "/v1/images/generations", Model: "gpt-image-2.5-sunburst", Prompt: "a red apple",
		N: 1, Size: "1024x1024", SizeTier: "1024", ResponseFormat: "b64_json",
		InputImageURLs: []string{"https://reference.example/reference.png"},
	}
	forwarded, err := svc.ForwardImages(c.Request.Context(), c, selection.Account, nil, parsed, "")
	require.NoError(t, err)
	requireWebImageCleanup(t, cleanupDone)
	require.Equal(t, 1, forwarded.ImageCount)
	require.Equal(t, http.StatusOK, recorder.Code)
	raw, err := base64.StdEncoding.DecodeString(gjson.GetBytes(recorder.Body.Bytes(), "data.0.b64_json").String())
	require.NoError(t, err)
	require.Equal(t, encoded.Bytes(), raw)
	require.Contains(t, transport.paths, "/backend-api/f/conversation")
	require.NotContains(t, transport.paths, "/backend-api/codex/responses")
	require.Len(t, transport.assetRequests, 3)
	require.Equal(t, http.MethodGet, transport.assetRequests[0].Method)
	require.Equal(t, "reference.example", transport.assetRequests[0].URL.Host)
	require.Equal(t, http.MethodPut, transport.assetRequests[1].Method)
	require.Equal(t, "fixture.example", transport.assetRequests[1].URL.Host)
	require.Equal(t, http.MethodGet, transport.assetRequests[2].Method)
}

func TestWebImageURLResponseReturnsChatGPTDownloadURL(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 1, 1))
	picture.Set(0, 0, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, picture))

	account, err := buildAccountForCreate(&CreateAccountInput{
		Name: "web-image-url-fixture", Platform: PlatformOpenAI, Type: AccountTypeWebImage,
		Credentials: map[string]any{"access_token": "fixture-access", "refresh_token": "fixture-refresh"},
		Concurrency: 1,
	}, map[string]any{})
	require.NoError(t, err)
	account.ID = 9202
	cleanupDone := make(chan struct{}, 1)
	transport := &webImageFixtureUpstream{image: encoded.Bytes(), cleanupDone: cleanupDone}
	svc := &OpenAIGatewayService{httpUpstream: transport, cfg: &config.Config{}}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "https://api.example.test/v1/images/generations", nil)
	parsed := &OpenAIImagesRequest{
		Endpoint: "/v1/images/generations", Model: "gpt-image-2.5-sunburst", Prompt: "a red apple",
		N: 1, Size: "1024x1024", SizeTier: "1024", ResponseFormat: "url",
	}
	_, err = svc.ForwardImages(c.Request.Context(), c, account, nil, parsed, "")
	require.NoError(t, err)
	requireWebImageCleanup(t, cleanupDone)
	require.Equal(t, "https://fixture.example/image.png", gjson.GetBytes(recorder.Body.Bytes(), "data.0.url").String())
	require.Len(t, transport.assetRequests, 0, "URL output must not download the generated image")
}

func TestWebImageURLResponseDownloadsAuthenticatedFirstPartyURL(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 1, 1))
	picture.Set(0, 0, color.RGBA{B: 255, A: 255})
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, picture))

	account, err := buildAccountForCreate(&CreateAccountInput{
		Name: "web-image-first-party-url-fixture", Platform: PlatformOpenAI, Type: AccountTypeWebImage,
		Credentials: map[string]any{"access_token": "fixture-access", "refresh_token": "fixture-refresh"},
		Concurrency: 1,
	}, map[string]any{})
	require.NoError(t, err)
	account.ID = 9203
	cleanupDone := make(chan struct{}, 1)
	transport := &webImageFixtureUpstream{
		image: encoded.Bytes(), downloadURL: "https://chatgpt.com/backend-api/files/fixture-image/content", cleanupDone: cleanupDone,
	}
	svc := &OpenAIGatewayService{httpUpstream: transport, cfg: &config.Config{}}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "https://api.example.test/v1/images/generations", nil)
	parsed := &OpenAIImagesRequest{
		Endpoint: "/v1/images/generations", Model: "gpt-image-2.5-sunburst", Prompt: "a blue apple",
		N: 1, Size: "1024x1024", SizeTier: "1024", ResponseFormat: "url",
	}
	_, err = svc.ForwardImages(c.Request.Context(), c, account, nil, parsed, "")
	require.NoError(t, err)
	requireWebImageCleanup(t, cleanupDone)
	result := gjson.GetBytes(recorder.Body.Bytes(), "data.0.url").String()
	require.True(t, strings.HasPrefix(result, "data:image/png;base64,"))
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(result, "data:image/png;base64,"))
	require.NoError(t, err)
	require.Equal(t, encoded.Bytes(), raw)
	require.Contains(t, transport.paths, "/backend-api/files/fixture-image/content")
	require.Len(t, transport.assetRequests, 0)
}

type cancelOnEOFReadCloser struct {
	io.Reader
	cancel context.CancelFunc
	once   sync.Once
}

func (r *cancelOnEOFReadCloser) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.once.Do(r.cancel)
	}
	return n, err
}

func (r *cancelOnEOFReadCloser) Close() error { return nil }

type webImageFailedGenerationUpstream struct {
	*webImageFixtureUpstream
	cancel context.CancelFunc
}

func (f *webImageFailedGenerationUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	switch req.URL.Path {
	case "/backend-api/f/conversation":
		f.paths = append(f.paths, req.URL.Path)
		body := strings.NewReader("data: {\"conversation_id\":\"fixture-conversation\"}\n\n")
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: &cancelOnEOFReadCloser{Reader: body, cancel: f.cancel}}, nil
	case "/backend-api/conversations/fixture-conversation":
		return nil, errors.New("fixture polling interrupted")
	default:
		return f.webImageFixtureUpstream.DoWithTLS(req, proxyURL, accountID, concurrency, profile)
	}
}

func TestGenerateImageCleansConversationAfterPollingFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cleanupDone := make(chan struct{}, 1)
	base := &webImageFixtureUpstream{cleanupDone: cleanupDone}
	transport := &webImageFailedGenerationUpstream{webImageFixtureUpstream: base, cancel: cancel}
	account := &Account{ID: 9204, Concurrency: 1}
	session := newChatGPTWebSession(transport, account, "fixture-access", chatGPTWebDevice{})

	_, err := session.GenerateImage(ctx, "a failed image", "gpt-image-2", "auto", nil)
	require.ErrorIs(t, err, context.Canceled)
	requireWebImageCleanup(t, cleanupDone)
}

type webImageHandoffUpstream struct {
	*webImageFixtureUpstream
	resumeFails bool
}

func (f *webImageHandoffUpstream) DoWithTLS(req *http.Request, proxy string, account int64, concurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	var body string
	status := http.StatusOK
	switch req.URL.Path {
	case "/backend-api/f/conversation":
		body = "data: {\"type\":\"resume_conversation_token\",\"kind\":\"topic\",\"token\":\"fixture-resume\",\"conversation_id\":\"fixture-conversation\"}\n\n" +
			"data: {\"type\":\"stream_handoff\",\"options\":[{\"type\":\"resume_sse_endpoint\",\"topic_id\":\"fixture-topic\"}]}\n\ndata: [DONE]\n\n"
	case "/backend-api/f/conversation/resume":
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			return nil, err
		}
		if payload["token"] != "fixture-resume" || payload["conversation_id"] != "fixture-conversation" || payload["kind"] != "topic" || req.Header.Get("X-Conduit-Token") != "fixture-conduit" {
			return nil, errors.New("resume lost turn credentials or conversation context")
		}
		if f.resumeFails {
			status = http.StatusServiceUnavailable
			body = `{}`
		} else {
			body = "data: {\"message\":{\"author\":{\"role\":\"tool\"},\"content\":{\"parts\":[{\"asset_pointer\":\"file-service://fixture-image\"}]}}}\n\ndata: [DONE]\n\n"
		}
	case "/backend-api/conversations/fixture-conversation":
		if !f.resumeFails {
			return nil, errors.New("successful SSE resume must not poll")
		}
		body = `{"mapping":{"generated":{"message":{"author":{"role":"tool"},"content":{"parts":[{"asset_pointer":"file-service://fixture-image"}]}}}}}`
	default:
		return f.webImageFixtureUpstream.DoWithTLS(req, proxy, account, concurrency, profile)
	}
	f.paths = append(f.paths, req.URL.Path)
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestChatGPTWebImageHandoffResumesOrPollsWithoutGeneratingAgain(t *testing.T) {
	for _, resumeFails := range []bool{false, true} {
		t.Run(strconv.FormatBool(resumeFails), func(t *testing.T) {
			transport := &webImageHandoffUpstream{webImageFixtureUpstream: &webImageFixtureUpstream{}, resumeFails: resumeFails}
			session := newChatGPTWebSession(transport, nil, "fixture-access", chatGPTWebDevice{})
			outcome, err := session.GenerateImage(context.Background(), "generate an apple image", "gpt-image-2", "", nil)
			require.NoError(t, err)
			require.Equal(t, "fixture-conversation", outcome.ConversationID)
			require.Equal(t, []string{"file-service://fixture-image"}, outcome.Pointers)
			require.Contains(t, transport.paths, "/backend-api/f/conversation/resume")
			count := 0
			for _, path := range transport.paths {
				if path == "/backend-api/f/conversation" {
					count++
				}
			}
			require.Equal(t, 1, count, "handoff recovery must not incur a second image generation")
		})
	}
}
