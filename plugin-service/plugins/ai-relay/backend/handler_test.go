package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/host/principal"
)

type recordingRelayClientProvider struct {
	client   *http.Client
	proxyIDs []string
}

type recordingPlatformHandler struct {
	request PlatformRequest
}

type streamingPlatformHandler struct{}

func (*streamingPlatformHandler) Platform() string { return "streaming-platform" }

func (*streamingPlatformHandler) Descriptor() PlatformDescriptor {
	return PlatformDescriptor{Key: "streaming-platform", DisplayName: "Streaming Platform"}
}

func (*streamingPlatformHandler) Handle(_ context.Context, _ PlatformRequest) (PlatformResponse, error) {
	return PlatformResponse{
		StatusCode: http.StatusAccepted,
		Headers:    http.Header{"Content-Length": []string{"999"}, "Content-Type": []string{"text/event-stream"}},
		Stream: func(w http.ResponseWriter) error {
			_, err := w.Write([]byte("data: first\n\n"))
			return err
		},
	}, nil
}

func (*recordingPlatformHandler) Platform() string { return "test-platform" }

func (*recordingPlatformHandler) Descriptor() PlatformDescriptor {
	return PlatformDescriptor{Key: "test-platform", DisplayName: "Test Platform"}
}

func (handler *recordingPlatformHandler) Handle(_ context.Context, request PlatformRequest) (PlatformResponse, error) {
	handler.request = request
	return PlatformResponse{
		StatusCode: http.StatusCreated,
		Headers:    http.Header{"X-Platform": []string{"handled"}},
		Body:       []byte(`{"ok":true}`),
	}, nil
}

func TestRelayDispatchesRawRequestToPlatformHandler(t *testing.T) {
	repository := NewMemoryRouteRepository()
	repository.routes["test-platform:primary"] = RouteConfig{Platform: "test-platform", Slug: "primary", BaseURL: "https://example.test/v1"}
	platforms := NewPlatformRegistry()
	platform := &recordingPlatformHandler{}
	platforms.Register(platform)
	handler := &RelayHandler{
		routes:    repository,
		platforms: platforms,
		clients:   &recordingRelayClientProvider{client: http.DefaultClient},
	}
	req := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/test-platform/primary/v1/chat/completions?trace=1", bytes.NewBufferString(`{"model":"test","input":"raw"}`))
	req.Header.Set("Authorization", "Bearer account-key")
	req.Header.Set("X-Relay-Test", "preserved")
	rec := httptest.NewRecorder()

	handler.ProxyOpenAIPath(rec, req)

	if rec.Code != http.StatusCreated || rec.Body.String() != `{"ok":true}` {
		t.Fatalf("response = %d; body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Platform") != "handled" {
		t.Fatalf("X-Platform = %q", rec.Header().Get("X-Platform"))
	}
	if platform.request.Endpoint != "chat/completions" || platform.request.Method != http.MethodPost || platform.request.Query != "trace=1" {
		t.Fatalf("request = %#v", platform.request)
	}
	if platform.request.Headers.Get("X-Relay-Test") != "preserved" || string(platform.request.Body) != `{"model":"test","input":"raw"}` {
		t.Fatalf("platform request = %#v", platform.request)
	}
}

func TestRelayStreamsPlatformResponseWithoutContentLength(t *testing.T) {
	repository := NewMemoryRouteRepository()
	repository.routes["streaming-platform:primary"] = RouteConfig{Platform: "streaming-platform", Slug: "primary", BaseURL: "https://example.test/v1"}
	platforms := NewPlatformRegistry()
	platforms.Register(&streamingPlatformHandler{})
	handler := NewRelayHandler(repository, platforms, http.DefaultClient)
	req := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/streaming-platform/primary/v1/chat/completions", bytes.NewBufferString(`{"model":"test"}`))
	req.Header.Set("Authorization", "Bearer account-key")
	rec := httptest.NewRecorder()

	handler.ProxyPlatformPath(rec, req)

	if rec.Code != http.StatusAccepted || rec.Body.String() != "data: first\n\n" {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length = %q", got)
	}
}

func (p *recordingRelayClientProvider) ClientFor(_ context.Context, proxyID string) (*http.Client, error) {
	p.proxyIDs = append(p.proxyIDs, proxyID)
	return p.client, nil
}

func TestRelayForwardsAccountBearerKeyAndReturnsOpenAIImages(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/images/generations" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer agnes-account-key" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := readRequestBody(t, r); got != `{"model":"gpt-image-1","prompt":"poster","size":"1K","ratio":"1:1","extra_body":{"response_format":"url"}}` {
			t.Fatalf("body = %s", got)
		}
		_, _ = w.Write([]byte(`{"created":1,"data":[{"url":"https://cdn.example/image.png"}]}`))
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["agnes:team-a"] = RouteConfig{
		Platform: "agnes", Slug: "team-a", BaseURL: upstream.URL + "/v1",
	}
	provider := &recordingRelayClientProvider{client: upstream.Client()}
	handler := NewRelayHandlerWithClientProvider(repository, NewDefaultPlatformRegistry(), provider)
	req := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/agnes/team-a", bytes.NewBufferString(`{"model":"gpt-image-1","prompt":"poster","size":"1024x1024","response_format":"url"}`))
	req.Header.Set("Authorization", "Bearer agnes-account-key")
	req.Header.Set(ProxyIDHeader, "42")
	rec := httptest.NewRecorder()

	handler.Relay(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if len(provider.proxyIDs) != 1 || provider.proxyIDs[0] != "42" {
		t.Fatalf("provider proxy IDs = %#v", provider.proxyIDs)
	}
}

func TestRelayConvertsOpenAIImageEditMultipartRequestForAgnes(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		if got := readRequestBody(t, r); got != `{"model":"agnes-image-2.1-flash","prompt":"make it blue","size":"1K","ratio":"1:1","extra_body":{"response_format":"url","image":["data:image/png;base64,cG5nLWJ5dGVz"]}}` {
			t.Fatalf("body = %s", got)
		}
		_, _ = w.Write([]byte(`{"created":1,"data":[{"url":"https://cdn.example/edited.png"}]}`))
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["agnes:team-a"] = RouteConfig{Platform: "agnes", Slug: "team-a", BaseURL: upstream.URL + "/v1"}
	provider := &recordingRelayClientProvider{client: upstream.Client()}
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandlerWithClientProvider(repository, NewDefaultPlatformRegistry(), provider))

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("model", "agnes-image-2.1-flash")
	_ = writer.WriteField("prompt", "make it blue")
	_ = writer.WriteField("size", "1024x1024")
	_ = writer.WriteField("response_format", "url")
	file, err := writer.CreateFormFile("image", "source.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("png-bytes")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/agnes/team-a/v1/images/edits", body)
	req.Header.Set("Authorization", "Bearer account-key")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(ProxyIDHeader, "42")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if len(provider.proxyIDs) != 1 || provider.proxyIDs[0] != "42" {
		t.Fatalf("provider proxy IDs = %#v", provider.proxyIDs)
	}
}

func TestV1ModelsAndChatProxyAccountBearerKey(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer account-key" {
			t.Fatalf("Authorization = %q", got)
		}
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"agnes-image-2.1-flash","object":"model"}]}`))
		case "/v1/chat/completions":
			if got := readRequestBody(t, r); got != `{"model":"agnes-chat","messages":[{"role":"user","content":"hello"}]}` {
				t.Fatalf("chat body = %s", got)
			}
			_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","choices":[]}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["agnes:team-a"] = RouteConfig{Platform: "agnes", Slug: "team-a", BaseURL: upstream.URL + "/v1"}
	provider := &recordingRelayClientProvider{client: upstream.Client()}
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandlerWithClientProvider(repository, NewDefaultPlatformRegistry(), provider))

	for _, test := range []struct{ method, path, body string }{
		{http.MethodGet, "/plugins/ai-relay/agnes/team-a/v1/models", ""},
		{http.MethodPost, "/plugins/ai-relay/agnes/team-a/v1/chat/completions", `{"model":"agnes-chat","messages":[{"role":"user","content":"hello"}]}`},
	} {
		req := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
		req.Header.Set("Authorization", "Bearer account-key")
		req.Header.Set(ProxyIDHeader, "42")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s status = %d; body=%s", test.method, test.path, rec.Code, rec.Body.String())
		}
	}
	if len(provider.proxyIDs) != 2 || provider.proxyIDs[0] != "42" || provider.proxyIDs[1] != "42" {
		t.Fatalf("provider proxy IDs = %#v", provider.proxyIDs)
	}
}

func TestResponsesCompactPathMappingTransparentlyProxiesRequestAndResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/paas/v4/chat/completions" || r.URL.RawQuery != "trace=compact" {
			t.Fatalf("upstream URL = %s", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer account-key" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("Accept = %q", got)
		}
		if got := r.Header.Get("X-Relay-Test"); got != "kept" {
			t.Fatalf("X-Relay-Test = %q", got)
		}
		if got := r.Header.Get(ProxyIDHeader); got != "" {
			t.Fatalf("internal proxy header leaked upstream: %q", got)
		}
		if got := readRequestBody(t, r); got != `{"model":"glm-4.5","input":"compact me"}` {
			t.Fatalf("body = %s", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Upstream-Test", "kept")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("data: first\n\n"))
		_, _ = w.Write([]byte("data: second\n\n"))
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["agnes:zhipu"] = RouteConfig{
		Platform: "agnes", Slug: "zhipu", BaseURL: upstream.URL + "/v1",
		PathMappings: map[string]string{"responses/compact": "api/paas/v4/chat/completions"},
	}
	provider := &recordingRelayClientProvider{client: upstream.Client()}
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandlerWithClientProvider(repository, NewDefaultPlatformRegistry(), provider))

	req := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/agnes/zhipu/v1/responses/compact?trace=compact", bytes.NewBufferString(`{"model":"glm-4.5","input":"compact me"}`))
	req.Header.Set("Authorization", "Bearer account-key")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Relay-Test", "kept")
	req.Header.Set(ProxyIDHeader, "42")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Upstream-Test"); got != "kept" {
		t.Fatalf("X-Upstream-Test = %q", got)
	}
	if got := rec.Body.String(); got != "data: first\n\ndata: second\n\n" {
		t.Fatalf("response body = %q", got)
	}
	if len(provider.proxyIDs) != 1 || provider.proxyIDs[0] != "42" {
		t.Fatalf("provider proxy IDs = %#v", provider.proxyIDs)
	}
}

func TestPathMappingsApplyToExistingRelayEndpoints(t *testing.T) {
	requestedPaths := make(chan string, 3)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths <- r.URL.Path
		if r.URL.Path == "/v1/mapped/images" {
			_, _ = w.Write([]byte(`{"created":1,"data":[{"url":"https://cdn.example/image.png"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["agnes:all"] = RouteConfig{
		Platform: "agnes", Slug: "all", BaseURL: upstream.URL + "/v1",
		PathMappings: map[string]string{
			"models":             "mapped/models",
			"chat/completions":   "mapped/chat",
			"images/generations": "mapped/images",
		},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandlerWithClientProvider(repository, NewDefaultPlatformRegistry(), &recordingRelayClientProvider{client: upstream.Client()}))

	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/plugins/ai-relay/agnes/all/v1/models", nil),
		httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/agnes/all/v1/chat/completions", bytes.NewBufferString(`{}`)),
		httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/agnes/all", bytes.NewBufferString(`{"model":"gpt-image-1","prompt":"poster"}`)),
	}
	for _, req := range requests {
		req.Header.Set("Authorization", "Bearer account-key")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d; body=%s", req.URL.Path, rec.Code, rec.Body.String())
		}
	}
	for _, want := range []string{"/v1/mapped/models", "/v1/mapped/chat", "/v1/mapped/images"} {
		if got := <-requestedPaths; got != want {
			t.Fatalf("upstream path = %q, want %q", got, want)
		}
	}
}

func TestOpenAIWildcardTransparentProxyAppliesMappings(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer openai-key" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path == "/v1/v4/embeddings" {
			if r.URL.RawQuery != "trace=1" {
				t.Fatalf("mapped query = %q", r.URL.RawQuery)
			}
			if got := readRequestBody(t, r); got != `{"model":"text-embedding-3-small","input":"hello"}` {
				t.Fatalf("mapped body = %s", got)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"mapped":true}`))
			return
		}
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Fatalf("unmapped path = %q", r.URL.Path)
		}
		if got := readRequestBody(t, r); got != `{"file":"audio"}` {
			t.Fatalf("unmapped body = %s", got)
		}
		_, _ = w.Write([]byte(`{"unmapped":true}`))
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["openai:demo"] = RouteConfig{
		Platform: "openai", Slug: "demo", BaseURL: upstream.URL + "/v1",
		PathMappings: map[string]string{"v1/embeddings": "v4/embeddings"},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandler(repository, NewDefaultPlatformRegistry(), upstream.Client()))

	mappedRequest := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/openai/demo/v1/embeddings?trace=1", bytes.NewBufferString(`{"model":"text-embedding-3-small","input":"hello"}`))
	mappedRequest.Header.Set("Authorization", "Bearer openai-key")
	mappedResponse := httptest.NewRecorder()
	mux.ServeHTTP(mappedResponse, mappedRequest)
	if mappedResponse.Code != http.StatusAccepted || mappedResponse.Body.String() != `{"mapped":true}` {
		t.Fatalf("mapped response = %d %s", mappedResponse.Code, mappedResponse.Body.String())
	}

	unmappedRequest := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/openai/demo/v1/audio/transcriptions", bytes.NewBufferString(`{"file":"audio"}`))
	unmappedRequest.Header.Set("Authorization", "Bearer openai-key")
	unmappedResponse := httptest.NewRecorder()
	mux.ServeHTTP(unmappedResponse, unmappedRequest)
	if unmappedResponse.Code != http.StatusOK || unmappedResponse.Body.String() != `{"unmapped":true}` {
		t.Fatalf("unmapped response = %d %s", unmappedResponse.Code, unmappedResponse.Body.String())
	}
}

func TestOpenCodeTransparentProxyConvertsNativeChatRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zen/v1/chat/completions" || r.URL.RawQuery != "trace=opencode" {
			t.Fatalf("upstream URL = %s", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer opencode-key" {
			t.Fatalf("Authorization = %q", got)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(readRequestBody(t, r)), &payload); err != nil {
			t.Fatalf("body is invalid JSON: %v", err)
		}
		if payload["model"] != "gpt-5" {
			t.Fatalf("model = %#v", payload["model"])
		}
		messages, ok := payload["messages"].([]any)
		if !ok || len(messages) != 1 || messages[0].(map[string]any)["content"] != "Hello\nworld" {
			t.Fatalf("messages = %#v", payload["messages"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("data: converted\n\n"))
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["opencode:demo"] = RouteConfig{
		Platform: "opencode", Slug: "demo", BaseURL: upstream.URL + "/zen",
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandler(repository, NewDefaultPlatformRegistry(), upstream.Client()))

	body := `{"model":{"providerID":"openai","modelID":"gpt-5"},"parts":[{"text":"Hello"},{"content":"world"}],"stream":true}`
	request := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/opencode/demo/v1/chat/completions?trace=opencode", bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer opencode-key")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted || response.Body.String() != "data: converted\n\n" {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestOpenCodeResponsesBridgeUsesChatCompletions(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zen/v1/chat/completions" {
			t.Fatalf("upstream path = %q", r.URL.Path)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(readRequestBody(t, r)), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "deepseek-v4-flash-free" {
			t.Fatalf("model = %#v", payload["model"])
		}
		messages, ok := payload["messages"].([]any)
		if !ok || len(messages) != 1 || messages[0].(map[string]any)["content"] != "hello" {
			t.Fatalf("messages = %#v", payload["messages"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl_1\",\"model\":\"deepseek-v4-flash-free\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["opencode:demo"] = RouteConfig{Platform: "opencode", Slug: "demo", BaseURL: upstream.URL + "/zen"}
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandler(repository, NewDefaultPlatformRegistry(), upstream.Client()))

	req := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/opencode/demo/v1/responses", bytes.NewBufferString(`{"model":"deepseek-v4-flash-free","stream":true,"input":"hello"}`))
	req.Header.Set("Authorization", "Bearer opencode-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("event: response.output_text.delta")) || !bytes.Contains(rec.Body.Bytes(), []byte("data: [DONE]")) {
		t.Fatalf("response = %s", rec.Body.String())
	}
}

func TestOpenCodeResponsesStreamReachesClientBeforeUpstreamCompletes(t *testing.T) {
	firstUpstreamEvent := make(chan struct{}, 1)
	releaseUpstream := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseUpstream) }) }
	defer release()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zen/v1/chat/completions" {
			t.Fatalf("upstream path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl_1\",\"model\":\"deepseek\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
		w.(http.Flusher).Flush()
		firstUpstreamEvent <- struct{}{}
		<-releaseUpstream
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["opencode:demo"] = RouteConfig{Platform: "opencode", Slug: "demo", BaseURL: upstream.URL + "/zen"}
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandler(repository, NewDefaultPlatformRegistry(), upstream.Client()))
	relay := httptest.NewServer(mux)
	defer relay.Close()

	request, err := http.NewRequest(http.MethodPost, relay.URL+"/plugins/ai-relay/opencode/demo/v1/responses", strings.NewReader(`{"model":"deepseek","stream":true,"input":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer account-key")
	request.Header.Set("Content-Type", "application/json")
	response, err := relay.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	<-firstUpstreamEvent

	firstEvent := make(chan []byte, 1)
	go func() {
		buffer := make([]byte, len("event: response.created"))
		_, _ = io.ReadFull(response.Body, buffer)
		firstEvent <- buffer
	}()
	select {
	case got := <-firstEvent:
		if string(got) != "event: response.created" {
			t.Fatalf("first event = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("client did not receive a converted event before upstream completion")
	}
	release()
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
}

func TestOpenCodeResponsesStreamCancellationClosesUpstreamRequest(t *testing.T) {
	upstreamStarted := make(chan struct{}, 1)
	upstreamCanceled := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl_1\",\"model\":\"deepseek\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
		w.(http.Flusher).Flush()
		upstreamStarted <- struct{}{}
		<-r.Context().Done()
		upstreamCanceled <- struct{}{}
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["opencode:demo"] = RouteConfig{Platform: "opencode", Slug: "demo", BaseURL: upstream.URL + "/zen"}
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandler(repository, NewDefaultPlatformRegistry(), upstream.Client()))
	relay := httptest.NewServer(mux)
	defer relay.Close()

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, relay.URL+"/plugins/ai-relay/opencode/demo/v1/responses", strings.NewReader(`{"model":"deepseek","stream":true,"input":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer account-key")
	response, err := relay.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	<-upstreamStarted
	cancel()

	select {
	case <-upstreamCanceled:
	case <-time.After(time.Second):
		t.Fatal("upstream request remained open after the client canceled")
	}
}

func TestOpenCodeResponsesBridgeRestoresCodexCustomTool(t *testing.T) {
	patchInput := "*** Begin Patch\n*** Update File: README.md\n@@\n-old\n+new\n*** End Patch"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zen/v1/chat/completions" {
			t.Fatalf("upstream path = %q", r.URL.Path)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(readRequestBody(t, r)), &payload); err != nil {
			t.Fatal(err)
		}
		tools := payload["tools"].([]any)
		function := tools[0].(map[string]any)["function"].(map[string]any)
		parameters := function["parameters"].(map[string]any)
		if function["name"] != "apply_patch" || parameters["type"] != "object" {
			t.Fatalf("function = %#v", function)
		}
		required := parameters["required"].([]any)
		if len(required) != 1 || required[0] != "input" {
			t.Fatalf("parameters = %#v", parameters)
		}

		arguments, _ := json.Marshal(map[string]string{"input": patchInput})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl_custom", "created": 1, "model": "deepseek-v4-flash-free",
			"choices": []any{map[string]any{"message": map[string]any{
				"role": "assistant",
				"tool_calls": []any{map[string]any{
					"id": "call_patch", "type": "function",
					"function": map[string]any{"name": "apply_patch", "arguments": string(arguments)},
				}},
			}}},
		})
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["opencode:demo"] = RouteConfig{Platform: "opencode", Slug: "demo", BaseURL: upstream.URL + "/zen"}
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandler(repository, NewDefaultPlatformRegistry(), upstream.Client()))

	req := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/opencode/demo/v1/responses", bytes.NewBufferString(`{
		"model":"deepseek-v4-flash-free","input":"update README",
		"tools":[{"type":"custom","name":"apply_patch","description":"Apply a patch"}]
	}`))
	req.Header.Set("Authorization", "Bearer opencode-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	item := response["output"].([]any)[0].(map[string]any)
	if item["type"] != "custom_tool_call" || item["name"] != "apply_patch" || item["input"] != patchInput {
		t.Fatalf("item = %#v", item)
	}
}

func TestOpenCodeResponsesBridgeStreamsCodexCustomTool(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.Unmarshal([]byte(readRequestBody(t, r)), &payload); err != nil {
			t.Fatal(err)
		}
		tools := payload["tools"].([]any)
		function := tools[0].(map[string]any)["function"].(map[string]any)
		if function["name"] != "apply_patch" {
			t.Fatalf("function = %#v", function)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"id":"chatcmpl_stream_patch","model":"deepseek-v4-flash-free","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_stream_patch","function":{"name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\\n*** End Patch\"}"}}]}}]}` + "\n\ndata: [DONE]\n\n"))
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["opencode:demo"] = RouteConfig{Platform: "opencode", Slug: "demo", BaseURL: upstream.URL + "/zen"}
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandler(repository, NewDefaultPlatformRegistry(), upstream.Client()))
	req := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/opencode/demo/v1/responses", bytes.NewBufferString(`{
		"model":"deepseek-v4-flash-free","stream":true,"input":"update file",
		"tools":[{"type":"custom","name":"apply_patch"}]
	}`))
	req.Header.Set("Authorization", "Bearer opencode-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.Bytes()
	for _, event := range []string{"response.custom_tool_call_input.delta", "response.custom_tool_call_input.done"} {
		if !bytes.Contains(body, []byte("event: "+event)) {
			t.Fatalf("missing %s: %s", event, rec.Body.String())
		}
	}
	if !bytes.Contains(body, []byte(`"type":"custom_tool_call"`)) || !bytes.Contains(body, []byte("data: [DONE]")) {
		t.Fatalf("response = %s", rec.Body.String())
	}
}

func TestOpenCodeResponsesBridgeRestoresCodexWriteAsApplyPatch(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_write","created":1,"model":"deepseek-v4-flash-free",
			"choices":[{"message":{"role":"assistant","tool_calls":[{
				"id":"call_write","type":"function","function":{
					"name":"write","arguments":"{\"filePath\":\"created.txt\",\"content\":\"hello\\nworld\"}"
				}
			}]}}]
		}`))
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["opencode:demo"] = RouteConfig{Platform: "opencode", Slug: "demo", BaseURL: upstream.URL + "/zen"}
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandler(repository, NewDefaultPlatformRegistry(), upstream.Client()))
	req := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/opencode/demo/v1/responses", bytes.NewBufferString(`{
		"model":"deepseek-v4-flash-free","input":"create file",
		"tools":[{"type":"function","name":"write","parameters":{"type":"object"}}]
	}`))
	req.Header.Set("Authorization", "Bearer opencode-key")
	req.Header.Set("originator", "codex_app")
	req.Header.Set("User-Agent", "codex_app/1.0")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	item := response["output"].([]any)[0].(map[string]any)
	if item["type"] != "custom_tool_call" || item["name"] != "apply_patch" || !strings.Contains(stringValue(item["input"]), "*** Add File: created.txt") {
		t.Fatalf("item = %#v", item)
	}
}

func TestOpenCodeResponsesBridgeKeepsWriteForNonCodexClient(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_write","created":1,"model":"deepseek-v4-flash-free",
			"choices":[{"message":{"role":"assistant","tool_calls":[{
				"id":"call_write","type":"function","function":{
					"name":"write","arguments":"{\"filePath\":\"created.txt\",\"content\":\"hello\"}"
				}
			}]}}]
		}`))
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["opencode:demo"] = RouteConfig{Platform: "opencode", Slug: "demo", BaseURL: upstream.URL + "/zen"}
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandler(repository, NewDefaultPlatformRegistry(), upstream.Client()))
	req := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/opencode/demo/v1/responses", bytes.NewBufferString(`{
		"model":"deepseek-v4-flash-free","input":"create file",
		"tools":[{"type":"function","name":"write","parameters":{"type":"object"}}]
	}`))
	req.Header.Set("Authorization", "Bearer opencode-key")
	req.Header.Set("User-Agent", "opencode/1.0")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	item := response["output"].([]any)[0].(map[string]any)
	if item["type"] != "function_call" || item["name"] != "write" {
		t.Fatalf("item = %#v", item)
	}
}

func TestOpenCodeResponsesBridgePassesUpstreamErrors(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"unsupported request"}}`))
	}))
	defer upstream.Close()

	repository := NewMemoryRouteRepository()
	repository.routes["opencode:demo"] = RouteConfig{Platform: "opencode", Slug: "demo", BaseURL: upstream.URL + "/zen"}
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandler(repository, NewDefaultPlatformRegistry(), upstream.Client()))
	req := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/opencode/demo/v1/responses", bytes.NewBufferString(`{"model":"deepseek-v4-flash-free","input":"hello"}`))
	req.Header.Set("Authorization", "Bearer opencode-key")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || rec.Body.String() != `{"error":{"message":"unsupported request"}}` {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRelayRejectsMalformedAccountProxyContext(t *testing.T) {
	repository := NewMemoryRouteRepository()
	repository.routes["agnes:team-a"] = RouteConfig{
		Platform: "agnes", Slug: "team-a", BaseURL: "https://apihub.agnes-ai.com/v1",
	}
	handler := NewRelayHandlerWithClientProvider(
		repository,
		NewDefaultPlatformRegistry(),
		NewProxyClientProvider(http.DefaultClient, &fakeProxyResolver{}),
	)
	req := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/agnes/team-a", bytes.NewBufferString(`{"model":"agnes-image-2.1-flash","prompt":"poster"}`))
	req.Header.Set("Authorization", "Bearer account-key")
	req.Header.Set(ProxyIDHeader, "invalid")
	rec := httptest.NewRecorder()

	handler.Relay(rec, req)

	if rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte(`"type":"invalid_request_error"`)) {
		t.Fatalf("response = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestRelayRejectsMissingBearerAndUnknownSlug(t *testing.T) {
	handler := NewRelayHandler(NewMemoryRouteRepository(), NewDefaultPlatformRegistry(), http.DefaultClient)

	missingKey := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/agnes/team-a", bytes.NewBufferString(`{"prompt":"poster"}`))
	missingKeyRec := httptest.NewRecorder()
	handler.Relay(missingKeyRec, missingKey)
	if missingKeyRec.Code != http.StatusUnauthorized {
		t.Fatalf("missing key status = %d", missingKeyRec.Code)
	}

	unknownSlug := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/agnes/team-a", bytes.NewBufferString(`{"prompt":"poster"}`))
	unknownSlug.Header.Set("Authorization", "Bearer account-key")
	unknownSlugRec := httptest.NewRecorder()
	handler.Relay(unknownSlugRec, unknownSlug)
	if unknownSlugRec.Code != http.StatusNotFound {
		t.Fatalf("unknown route status = %d", unknownSlugRec.Code)
	}
}

func TestRegisterRoutesMountsRelayEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil, NewRelayHandler(NewMemoryRouteRepository(), NewDefaultPlatformRegistry(), http.DefaultClient))

	req := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/agnes/team-a", bytes.NewBufferString(`{"prompt":"poster"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAdminCanManageAgnesRoutes(t *testing.T) {
	repository := NewMemoryRouteRepository()
	mux := http.NewServeMux()
	RegisterRoutes(mux, principal.NewMiddleware(), NewRelayHandler(repository, NewDefaultPlatformRegistry(), http.DefaultClient))

	request := httptest.NewRequest(http.MethodPut, "/plugins/ai-relay/api/routes/agnes/team-a", bytes.NewBufferString(`{"base_url":"https://apihub.agnes-ai.com/v1"}`))
	request.Header.Set("X-Sub2api-User-Id", "7")
	request.Header.Set("X-Sub2api-User-Role", "admin")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("admin upsert status = %d; body=%s", response.Code, response.Body.String())
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/plugins/ai-relay/api/routes", nil)
	listRequest.Header.Set("X-Sub2api-User-Id", "7")
	listRequest.Header.Set("X-Sub2api-User-Role", "admin")
	listResponse := httptest.NewRecorder()
	mux.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK || !bytes.Contains(listResponse.Body.Bytes(), []byte(`"team-a"`)) {
		t.Fatalf("admin list = %d; body=%s", listResponse.Code, listResponse.Body.String())
	}
}

func TestAdminBulkDeleteReportsMalformedItemsSeparately(t *testing.T) {
	repository := NewMemoryRouteRepository()
	repository.routes["agnes:one"] = RouteConfig{Platform: "agnes", Slug: "one", BaseURL: "https://example.test/v1"}
	mux := http.NewServeMux()
	RegisterRoutes(mux, principal.NewMiddleware(), NewRelayHandler(repository, NewDefaultPlatformRegistry(), http.DefaultClient))

	request := httptest.NewRequest(http.MethodDelete, "/plugins/ai-relay/api/routes", bytes.NewBufferString(`{"items":[{"platform":"agnes","slug":"one","name":"extra"}]}`))
	request.Header.Set("X-Sub2api-User-Id", "7")
	request.Header.Set("X-Sub2api-User-Role", "admin")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte(`"error":"invalid route configuration"`)) {
		t.Fatalf("response = %d; body=%s", response.Code, response.Body.String())
	}
}

func TestAdminListsRoutesWithPagination(t *testing.T) {
	repository := NewMemoryRouteRepository()
	for _, slug := range []string{"one", "two", "three"} {
		repository.routes["agnes:"+slug] = RouteConfig{Platform: "agnes", Slug: slug, Name: slug, BaseURL: "https://apihub.agnes-ai.com/v1"}
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, principal.NewMiddleware(), NewRelayHandler(repository, NewDefaultPlatformRegistry(), http.DefaultClient))

	req := httptest.NewRequest(http.MethodGet, "/plugins/ai-relay/api/routes?page=2&page_size=2", nil)
	req.Header.Set("X-Sub2api-User-Id", "7")
	req.Header.Set("X-Sub2api-User-Role", "admin")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Items      []RouteConfig `json:"items"`
		Pagination struct {
			Page       int `json:"page"`
			PageSize   int `json:"page_size"`
			Total      int `json:"total"`
			TotalPages int `json:"total_pages"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || response.Items[0].Slug != "two" {
		t.Fatalf("items = %#v", response.Items)
	}
	if response.Pagination.Page != 2 || response.Pagination.PageSize != 2 || response.Pagination.Total != 3 || response.Pagination.TotalPages != 2 {
		t.Fatalf("pagination = %#v", response.Pagination)
	}
}

func TestAdminBatchDeletesRoutesAtomically(t *testing.T) {
	repository := NewMemoryRouteRepository()
	for _, slug := range []string{"one", "two", "three"} {
		repository.routes["agnes:"+slug] = RouteConfig{Platform: "agnes", Slug: slug, Name: slug, BaseURL: "https://apihub.agnes-ai.com/v1"}
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, principal.NewMiddleware(), NewRelayHandler(repository, NewDefaultPlatformRegistry(), http.DefaultClient))

	req := httptest.NewRequest(http.MethodDelete, "/plugins/ai-relay/api/routes", bytes.NewBufferString(`{"items":[{"platform":"agnes","slug":"one"},{"platform":"agnes","slug":"two"}]}`))
	req.Header.Set("X-Sub2api-User-Id", "7")
	req.Header.Set("X-Sub2api-User-Role", "admin")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if _, found, _ := repository.Get(req.Context(), "agnes", "one"); found {
		t.Fatal("first selected route was not deleted")
	}
	if _, found, _ := repository.Get(req.Context(), "agnes", "two"); found {
		t.Fatal("second selected route was not deleted")
	}
	if _, found, _ := repository.Get(req.Context(), "agnes", "three"); !found {
		t.Fatal("unselected route must remain")
	}
}

func TestAdminListsRegisteredPlatforms(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, principal.NewMiddleware(), NewRelayHandler(NewMemoryRouteRepository(), NewDefaultPlatformRegistry(), http.DefaultClient))
	req := httptest.NewRequest(http.MethodGet, "/plugins/ai-relay/api/platforms", nil)
	req.Header.Set("X-Sub2api-User-Id", "7")
	req.Header.Set("X-Sub2api-User-Role", "admin")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"agnes"`)) {
		t.Fatalf("response = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminReadsRelayRuntime(t *testing.T) {
	t.Setenv("PLUGIN_AI_RELAY_PUBLIC_BASE_URL", "")
	t.Setenv("PLUGIN_SERVER_PORT", "8091")
	mux := http.NewServeMux()
	RegisterRoutes(mux, principal.NewMiddleware(), NewRelayHandler(NewMemoryRouteRepository(), NewDefaultPlatformRegistry(), http.DefaultClient))

	req := httptest.NewRequest(http.MethodGet, "/plugins/ai-relay/api/runtime", nil)
	req.Header.Set("X-Sub2api-User-Id", "7")
	req.Header.Set("X-Sub2api-User-Role", "admin")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		BaseURL string `json:"base_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.BaseURL != "http://127.0.0.1:8091" {
		t.Fatalf("base_url = %q", response.BaseURL)
	}
}

func TestAdminCreatesOnlyRegisteredRelayPlatforms(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, principal.NewMiddleware(), NewRelayHandler(NewMemoryRouteRepository(), NewDefaultPlatformRegistry(), http.DefaultClient))

	create := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/api/routes", bytes.NewBufferString(`{"platform":"agnes","slug":"primary","name":"Primary Agnes","base_url":"https://apihub.agnes-ai.com/v1"}`))
	create.Header.Set("X-Sub2api-User-Id", "7")
	create.Header.Set("X-Sub2api-User-Role", "admin")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, create)
	if createRec.Code != http.StatusCreated || !bytes.Contains(createRec.Body.Bytes(), []byte(`"name":"Primary Agnes"`)) {
		t.Fatalf("create = %d; body=%s", createRec.Code, createRec.Body.String())
	}

	unsupported := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/api/routes", bytes.NewBufferString(`{"platform":"anthropic","slug":"primary","name":"Anthropic","base_url":"https://example.test/v1"}`))
	unsupported.Header.Set("X-Sub2api-User-Id", "7")
	unsupported.Header.Set("X-Sub2api-User-Role", "admin")
	unsupportedRec := httptest.NewRecorder()
	mux.ServeHTTP(unsupportedRec, unsupported)
	if unsupportedRec.Code != http.StatusBadRequest {
		t.Fatalf("unsupported status = %d; body=%s", unsupportedRec.Code, unsupportedRec.Body.String())
	}
}

func TestAdminCreatesRouteWithPathMappings(t *testing.T) {
	repository := NewMemoryRouteRepository()
	mux := http.NewServeMux()
	RegisterRoutes(mux, principal.NewMiddleware(), NewRelayHandler(repository, NewDefaultPlatformRegistry(), http.DefaultClient))

	create := httptest.NewRequest(http.MethodPost, "/plugins/ai-relay/api/routes", bytes.NewBufferString(`{
		"platform":"agnes",
		"slug":"zhipu",
		"name":"Zhipu",
		"base_url":"https://open.bigmodel.cn/v1",
		"path_mappings":{"/v1/responses/compact":"/api/paas/v4/chat/completions"}
	}`))
	create.Header.Set("X-Sub2api-User-Id", "7")
	create.Header.Set("X-Sub2api-User-Role", "admin")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, create)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}

	stored, found, err := repository.Get(create.Context(), "agnes", "zhipu")
	if err != nil || !found {
		t.Fatalf("Get() = %#v, %v, %v", stored, found, err)
	}
	if got := stored.PathMappings["v1/responses/compact"]; got != "api/paas/v4/chat/completions" {
		t.Fatalf("stored mapping = %q", got)
	}
}

func readRequestBody(t *testing.T, r *http.Request) string {
	t.Helper()
	buffer := new(bytes.Buffer)
	if _, err := buffer.ReadFrom(r.Body); err != nil {
		t.Fatal(err)
	}
	return buffer.String()
}
