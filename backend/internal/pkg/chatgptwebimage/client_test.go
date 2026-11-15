package chatgptwebimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fixturePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{B: 255, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func fixtureHandshake(t *testing.T, w http.ResponseWriter, r *http.Request) bool {
	t.Helper()
	switch r.URL.Path {
	case "/":
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "private-cookie", Path: "/"})
		_, _ = io.WriteString(w, `<html data-build="test-build"><script src="/assets/test.js"></script></html>`)
	case "/backend-api/sentinel/chat-requirements":
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if !strings.HasPrefix(body["p"], "gAAAAAC") {
			t.Error("missing actual requirements proof")
		}
		if cookie, err := r.Cookie("session"); err != nil || cookie.Value != "private-cookie" {
			t.Error("requirements lost session cookie")
		}
		writeJSON(w, map[string]any{"token": "requirements-secret", "turnstile": map[string]any{"required": true}, "proofofwork": map[string]any{"required": true, "seed": "test-seed", "difficulty": "ff"}})
	case "/backend-api/f/conversation/prepare":
		if r.Header.Get("Openai-Sentinel-Chat-Requirements-Token") != "requirements-secret" {
			t.Error("missing requirements token")
		}
		if !strings.HasPrefix(r.Header.Get("Openai-Sentinel-Proof-Token"), "gAAAAAB") {
			t.Error("missing proof token")
		}
		if r.Header.Get("Openai-Sentinel-Turnstile-Token") != "" {
			t.Error("must not fabricate challenge token")
		}
		writeJSON(w, map[string]string{"conduit_token": "conduit-secret"})
	default:
		return false
	}
	return true
}

func testClient(t *testing.T, server *httptest.Server, mutate func(*Options)) *Client {
	t.Helper()
	opts := Options{BaseURL: server.URL, HTTPClient: server.Client(), Timeout: 2 * time.Second, PollInterval: time.Millisecond, MaxPolls: 2}
	if mutate != nil {
		mutate(&opts)
	}
	c, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestGenerateUsesRegularSessionAndDeduplicatesImages(t *testing.T) {
	imageBytes := fixturePNG(t)
	var server *httptest.Server
	var generations, polls, downloads atomic.Int32
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer account-secret" {
			t.Errorf("missing same-origin Authorization: %s", r.URL.Path)
		}
		if fixtureHandshake(t, w, r) {
			return
		}
		switch r.URL.Path {
		case "/backend-api/f/conversation":
			generations.Add(1)
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["history_and_training_disabled"] != false {
				t.Error("provider changed requested regular-chat semantics")
			}
			if body["model"] != "auto" {
				t.Error("unexpected chat driver")
			}
			messages, ok := body["messages"].([]any)
			if !ok || len(messages) != 1 {
				t.Error("expected one conversation message")
				return
			}
			message, ok := messages[0].(map[string]any)
			if !ok {
				t.Error("expected a conversation message object")
				return
			}
			content, ok := message["content"].(map[string]any)
			if !ok {
				t.Error("expected message content")
				return
			}
			parts, ok := content["parts"].([]any)
			if !ok || len(parts) != 1 || parts[0] != "Draw a blue circle" {
				t.Error("prompt was not forwarded")
			}
			_, _ = io.WriteString(w, "data: {\"conversation_id\":\"existing-job\",\"model_slug\":\"gpt-6\"}\n\ndata: [DONE]\n\n")
		case "/backend-api/conversation/existing-job":
			polls.Add(1)
			if cookie, err := r.Cookie("session"); err != nil || cookie.Value != "private-cookie" {
				t.Error("poll did not reuse session")
			}
			writeJSON(w, map[string]any{"conversation_id": "existing-job", "parts": []string{"file-service://file_00000000abcdefabcdefabcdefabcdef", "sediment://image-alias"}})
		case "/backend-api/files/file_00000000abcdefabcdefabcdefabcdef/download", "/backend-api/conversation/existing-job/attachment/image-alias/download":
			writeJSON(w, map[string]string{"download_url": server.URL + "/estuary/image?signature=private"})
		case "/estuary/image":
			downloads.Add(1)
			if cookie, err := r.Cookie("session"); err != nil || cookie.Value != "private-cookie" {
				t.Error("same-origin image lost session cookie")
			}
			_, _ = w.Write(imageBytes)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := testClient(t, server, nil)
	result, err := c.Generate(context.Background(), GenerateRequest{AccessToken: "account-secret", Prompt: "Draw a blue circle", AllowRegularChat: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Images) != 1 || !bytes.Equal(result.Images[0].Bytes, imageBytes) || result.Images[0].Width != 2 || result.Images[0].Height != 3 || result.Images[0].Format != "png" {
		t.Fatalf("unexpected image result: %+v", result.Images)
	}
	if result.ObservedImageModel != "" || result.WebDriver != "auto" || result.ConversationID != "existing-job" {
		t.Fatalf("model identity or conversation changed: %+v", result)
	}
	if generations.Load() != 1 || polls.Load() != 1 || downloads.Load() != 2 {
		t.Fatalf("unexpected request counts generation=%d polls=%d downloads=%d", generations.Load(), polls.Load(), downloads.Load())
	}
}

func TestHTTP200WithoutImageIsNotSuccess(t *testing.T) {
	var generationCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fixtureHandshake(t, w, r) {
			return
		}
		if r.URL.Path == "/backend-api/f/conversation" {
			generationCalls.Add(1)
			_, _ = io.WriteString(w, "data: {\"conversation_id\":\"text-job\",\"message\":{\"content\":{\"parts\":[\"I cannot generate images.\"]}}}\n\ndata: [DONE]\n\n")
			return
		}
		if r.URL.Path == "/backend-api/conversation/text-job" {
			writeJSON(w, map[string]string{"conversation_id": "text-job"})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	result, err := testClient(t, server, nil).Generate(context.Background(), GenerateRequest{AccessToken: "secret", Prompt: "image", AllowRegularChat: true})
	var upstream *Error
	if !errors.As(err, &upstream) || upstream.Kind != "no_image" || !upstream.Accepted || upstream.ConversationID != "text-job" || len(result.Images) != 0 {
		t.Fatalf("HTTP 200 text counted as image: result=%+v err=%v", result, err)
	}
	if generationCalls.Load() != 1 {
		t.Fatal("no-image outcome resubmitted generation")
	}
}

func TestTimeoutNeverResubmitsAcceptedGeneration(t *testing.T) {
	var generations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fixtureHandshake(t, w, r) {
			return
		}
		if r.URL.Path == "/backend-api/f/conversation" {
			generations.Add(1)
			_, _ = io.WriteString(w, "data: {\"conversation_id\":\"pending-job\"}\n\ndata: [DONE]\n\n")
			return
		}
		if r.URL.Path == "/backend-api/conversation/pending-job" {
			<-r.Context().Done()
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := testClient(t, server, func(o *Options) { o.Timeout = 100 * time.Millisecond })
	_, err := client.Generate(context.Background(), GenerateRequest{AccessToken: "secret", Prompt: "image", AllowRegularChat: true})
	var upstream *Error
	if !errors.As(err, &upstream) || !upstream.Accepted || upstream.ConversationID != "pending-job" || upstream.Kind != "timeout_or_canceled" {
		t.Fatalf("missing accepted outcome: %v", err)
	}
	if generations.Load() != 1 {
		t.Fatalf("generation submitted %d times", generations.Load())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestExternalDownloadAndRedirectNeverCarryCredentials(t *testing.T) {
	imageBytes := fixturePNG(t)
	var externalRequests atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fixtureHandshake(t, w, r) {
			return
		}
		switch r.URL.Path {
		case "/backend-api/f/conversation":
			_, _ = io.WriteString(w, "data: {\"conversation_id\":\"job\",\"asset_pointer\":\"file-service://asset\"}\n\ndata: [DONE]\n\n")
		case "/backend-api/files/asset/download":
			writeJSON(w, map[string]string{"download_url": server.URL + "/estuary/start?sig=secret"})
		case "/estuary/start":
			if r.Header.Get("Authorization") != "Bearer account-secret" {
				t.Error("same origin needs account auth")
			}
			http.Redirect(w, r, "https://image.oaiusercontent.com/image?sig=private", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := testClient(t, server, func(o *Options) {
		baseTransport := server.Client().Transport
		if baseTransport == nil {
			baseTransport = http.DefaultTransport
		}
		o.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "image.oaiusercontent.com" {
				externalRequests.Add(1)
				for key := range r.Header {
					if strings.EqualFold(key, "Authorization") || strings.EqualFold(key, "Cookie") || strings.Contains(strings.ToLower(key), "sentinel") || strings.EqualFold(key, "Oai-Device-Id") {
						t.Errorf("credential header leaked to CDN: %s", key)
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(imageBytes)), Request: r}, nil
			}
			return baseTransport.RoundTrip(r)
		})}
	})
	result, err := client.Generate(context.Background(), GenerateRequest{AccessToken: "account-secret", Prompt: "image", AllowRegularChat: true})
	if err != nil || len(result.Images) != 1 || externalRequests.Load() != 1 {
		t.Fatalf("download failed: %v requests=%d", err, externalRequests.Load())
	}
}

func TestUntrustedDownloadRedirectIsRejected(t *testing.T) {
	var badOriginCalls atomic.Int32
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "chatgpt.com" {
			badOriginCalls.Add(1)
		}
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://attacker.example/image"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}
	c, err := New(Options{HTTPClient: hc})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{client: c, http: c.http, public: c.http, headers: make(http.Header), accepted: true}
	s.headers.Set("Authorization", "Bearer secret")
	_, err = s.download(context.Background(), "https://chatgpt.com/estuary/image")
	var upstream *Error
	if !errors.As(err, &upstream) || upstream.Kind != "untrusted_download_origin" || badOriginCalls.Load() != 0 {
		t.Fatalf("unsafe redirect followed: %v", err)
	}
}

func TestRegularChatMustBeExplicit(t *testing.T) {
	c, err := New(Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid request reached network")
		return nil, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Generate(context.Background(), GenerateRequest{AccessToken: "secret", Prompt: "image"})
	var upstream *Error
	if !errors.As(err, &upstream) || upstream.Kind != "invalid_request" || upstream.Accepted {
		t.Fatalf("regular conversation was implicit: %v", err)
	}
}

func TestProofKeepsNonceJSONAndRespectsCancellation(t *testing.T) {
	config := proofConfig([]byte(`<html data-build="deployment"><script src="/assets/build.js"></script></html>`))
	answer, err := solveProof(context.Background(), "seed", "ff", config)
	if err != nil {
		t.Fatal(err)
	}
	b, err := base64.StdEncoding.DecodeString(answer)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []any
	if err := json.Unmarshal(b, &decoded); err != nil || len(decoded) != 18 {
		t.Fatalf("invalid proof configuration: %s %v", b, err)
	}
	nonce, ok := decoded[3].(float64)
	if !ok {
		t.Fatalf("invalid proof nonce: %v", decoded[3])
	}
	if decoded[5] != "/assets/build.js" || decoded[6] != "deployment" || decoded[9] != float64(int(nonce)>>1) {
		t.Fatalf("unexpected configuration: %+v", decoded)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = solveProof(ctx, "seed", "0000000000000000", config)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("proof ignored cancellation: %v", err)
	}
}

func TestAuthenticationFailureDoesNotExposeResponseBody(t *testing.T) {
	var generations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = io.WriteString(w, "<html></html>")
			return
		}
		if r.URL.Path == "/backend-api/f/conversation" {
			generations.Add(1)
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"error":"account-secret signed-download-url"}`)
	}))
	defer server.Close()
	_, err := testClient(t, server, nil).Generate(context.Background(), GenerateRequest{AccessToken: "account-secret", Prompt: "image", AllowRegularChat: true})
	var upstream *Error
	if !errors.As(err, &upstream) || upstream.Kind != "authentication_or_challenge" || upstream.Accepted || generations.Load() != 0 {
		t.Fatalf("unexpected authentication result: %v", err)
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "signed-download") {
		t.Fatal("upstream error body exposed")
	}
}

func TestAllowedDownloadHosts(t *testing.T) {
	c, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := &session{client: c}
	for _, input := range []string{"https://evil.example/image", "https://chatgpt.com.evil.example/image", "https://user:password@chatgpt.com/image", "http://blob.oaiusercontent.com/image", "https://image.oaiusercontent.com:8443/image", "https://oaiusercontent.com.evil.example/image"} {
		u, _ := url.Parse(input)
		if s.allowedAssetURL(u) {
			t.Errorf("unsafe host allowed: %s", input)
		}
	}
}

func TestUserMessageFileReferencesAreNotGeneratedImages(t *testing.T) {
	s := &session{}
	assets := make(map[string]asset)
	s.observe(map[string]any{"message": map[string]any{
		"author":  map[string]any{"role": "user"},
		"content": map[string]any{"parts": []any{"Draw file-service://prior-upload"}},
	}}, assets)
	if len(assets) != 0 {
		t.Fatal("user-supplied file reference counted as generated output")
	}
}
