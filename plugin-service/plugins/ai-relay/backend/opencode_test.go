package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"sync"
	"testing"
	"time"
)

type cancellationReadCloser struct {
	closed chan struct{}
	once   sync.Once
}

type keepaliveSignalWriter struct {
	bytes.Buffer
	firstKeepalive chan struct{}
	keepaliveCount int
	mu             sync.Mutex
}

type errWriter struct {
	err error
}

func (w *keepaliveSignalWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if bytes.Equal(p, []byte(": keepalive\n\n")) {
		w.keepaliveCount++
		select {
		case w.firstKeepalive <- struct{}{}:
		default:
		}
	}
	return w.Buffer.Write(p)
}

func (w errWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func (r *cancellationReadCloser) Read([]byte) (int, error) {
	<-r.closed
	return 0, io.EOF
}

func (r *cancellationReadCloser) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}

func TestOpenCodeStreamClosesUpstreamBodyOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	body := &cancellationReadCloser{closed: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- streamOpenCodeResponses(ctx, body, io.Discard, newResponsesBridgeContext())
	}()
	cancel()

	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("upstream body was not closed after cancellation")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("stream error = %v, want context canceled", err)
	}
}

func TestOpenCodeStreamWritesKeepaliveWhileUpstreamIsSilent(t *testing.T) {
	body := &cancellationReadCloser{closed: make(chan struct{})}
	writer := &keepaliveSignalWriter{firstKeepalive: make(chan struct{}, 1)}
	done := make(chan error, 1)
	go func() {
		done <- streamOpenCodeResponsesWithInterval(context.Background(), body, writer, newResponsesBridgeContext(), time.Millisecond)
	}()

	select {
	case <-writer.firstKeepalive:
	case <-time.After(time.Second):
		t.Fatal("keepalive was not written while upstream was silent")
	}
	deadline := time.Now().Add(time.Second)
	for {
		writer.mu.Lock()
		count := writer.keepaliveCount
		writer.mu.Unlock()
		if count >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("keepalive count = %d, want at least 2", count)
		}
		time.Sleep(time.Millisecond)
	}
	_ = body.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestOpenCodeStreamClosesUpstreamBodyWhenKeepaliveWriteFails(t *testing.T) {
	writeErr := errors.New("downstream disconnected")
	body := &cancellationReadCloser{closed: make(chan struct{})}
	err := streamOpenCodeResponsesWithInterval(context.Background(), body, errWriter{err: writeErr}, newResponsesBridgeContext(), time.Millisecond)
	if !errors.Is(err, writeErr) {
		t.Fatalf("stream error = %v, want %v", err, writeErr)
	}
	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("upstream body was not closed after keepalive write failure")
	}
}

func TestOpenCodeResponsesBridgeLogsOnlyFailures(t *testing.T) {
	originalOutput := log.Writer()
	var output bytes.Buffer
	log.SetOutput(&output)
	defer log.SetOutput(originalOutput)

	payload := []byte(`{"model":"deepseek","stream":true,"messages":[]}`)
	logOpenCodeResponsesBridge("response", payload, 200, []byte(`{"ok":true}`))
	if output.Len() != 0 {
		t.Fatalf("successful response logged: %s", output.String())
	}
	logOpenCodeResponsesBridge("response", payload, 502, []byte(`{"error":"upstream unavailable"}`))
	if !bytes.Contains(output.Bytes(), []byte("status=502")) || !bytes.Contains(output.Bytes(), []byte("upstream unavailable")) {
		t.Fatalf("failure log = %s", output.String())
	}
}

func TestOpenCodeAdapterMetadataAndBaseURL(t *testing.T) {
	adapter := NewOpenCodeAdapter()
	if got := adapter.Platform(); got != "opencode" {
		t.Fatalf("Platform() = %q", got)
	}
	if got := adapter.Descriptor(); got != (PlatformDescriptor{Key: "opencode", DisplayName: "OpenCode", Protocol: "opencode", DefaultBaseURL: "https://opencode.ai/zen"}) {
		t.Fatalf("Descriptor() = %#v", got)
	}
	for _, test := range []struct{ base, want string }{
		{"https://opencode.ai/zen", "https://opencode.ai/zen/v1"},
		{"https://opencode.ai/zen/", "https://opencode.ai/zen/v1"},
		{"https://opencode.ai/zen/v1", "https://opencode.ai/zen/v1"},
	} {
		if got := adapter.NormalizeBaseURL(test.base); got != test.want {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", test.base, got, test.want)
		}
	}
}

func TestOpenCodeAdapterConvertsNativeChatPayload(t *testing.T) {
	body := []byte(`{"model":{"providerID":"openai","modelID":"gpt-5"},"system":"Be concise.","parts":[{"type":"text","text":"Hello"},{"type":"text","text":"world"}],"stream":true,"temperature":0.2}`)
	converted := NewOpenCodeAdapter().TransformRequestBody("chat/completions", body)
	var payload map[string]any
	if err := json.Unmarshal(converted, &payload); err != nil {
		t.Fatalf("converted body is invalid JSON: %v", err)
	}
	if payload["model"] != "gpt-5" || payload["stream"] != true || payload["temperature"] != 0.2 {
		t.Fatalf("payload = %#v", payload)
	}
	messages, ok := payload["messages"].([]any)
	if !ok || len(messages) != 2 || messages[0].(map[string]any)["role"] != "system" || messages[1].(map[string]any)["content"] != "Hello\nworld" {
		t.Fatalf("messages = %#v", payload["messages"])
	}
}

func TestOpenCodeAdapterKeepsOpenAIBodiesAndInvalidBodies(t *testing.T) {
	adapter := NewOpenCodeAdapter()
	openAI := []byte(`{"model":"gpt-5","messages":[{"role":"user","content":"hi"}]}`)
	if got := adapter.TransformRequestBody("chat/completions", openAI); string(got) != string(openAI) {
		t.Fatalf("OpenAI body changed to %s", got)
	}
	invalid := []byte("not-json")
	if got := adapter.TransformRequestBody("chat/completions", invalid); string(got) != string(invalid) {
		t.Fatalf("invalid body changed to %s", got)
	}
	if got := adapter.TransformRequestBody("responses", []byte(`{"parts":[{"text":"hi"}]}`)); string(got) != `{"parts":[{"text":"hi"}]}` {
		t.Fatalf("non-chat body changed to %s", got)
	}
}

func TestOpenCodeAdapterUsesMessageFallbackAndRelativeMappings(t *testing.T) {
	adapter := NewOpenCodeAdapter()
	converted := adapter.TransformRequestBody("chat/completions", []byte(`{"model":"gpt-5","message":" hello "}`))
	var payload struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(converted, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Model != "gpt-5" || len(payload.Messages) != 1 || payload.Messages[0].Content != "hello" {
		t.Fatalf("payload = %#v", payload)
	}

	config := RouteConfig{
		BaseURL:      adapter.NormalizeBaseURL("https://opencode.ai/zen"),
		PathMappings: map[string]string{"v1/chat/completions": "custom/chat"},
	}
	endpoint, err := ResolveRouteEndpointURL(config, "chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "https://opencode.ai/zen/v1/custom/chat" {
		t.Fatalf("mapped endpoint = %q", endpoint)
	}
}
