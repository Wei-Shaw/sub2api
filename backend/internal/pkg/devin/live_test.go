//go:build live

package devin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

// Explicit opt-in: never read CLI/browser stores or reuse unrelated API keys.
// Successful subscription checks make at most four short inference requests.
func newLiveDevinClient(t *testing.T) *Client {
	t.Helper()
	if os.Getenv("DEVIN_LIVE_TEST") != "1" {
		t.Skip("live network checks require DEVIN_LIVE_TEST=1")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
		// Tokens are inside the protobuf body; never follow HTTP redirects.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	validate := func(raw string) (string, error) {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Port() != "" && u.Port() != "443") {
			return "", errors.New("invalid live Devin origin")
		}
		host := strings.ToLower(u.Hostname())
		for _, domain := range []string{"codeium.com", "windsurf.com", "devin.ai"} {
			if host == domain || strings.HasSuffix(host, "."+domain) {
				return strings.TrimRight(u.String(), "/"), nil
			}
		}
		return "", errors.New("untrusted live Devin origin")
	}
	return &Client{ValidateBaseURL: validate, Do: func(req *http.Request) (*http.Response, error) {
		if _, err := validate(req.URL.Scheme + "://" + req.URL.Host); err != nil {
			return nil, err
		}
		return client.Do(req)
	}}
}

func liveDevinCheckError(t *testing.T, err error, phase string) {
	t.Helper()
	if err == nil {
		return
	}
	var provider *Error
	if errors.As(err, &provider) {
		t.Fatalf("%s failed: status=%d code=%s (upstream content and credentials withheld)", phase, provider.StatusCode, provider.Code)
	}
	t.Fatalf("%s failed: transport/context error (details withheld to protect proxy credentials)", phase)
}

func TestDevinLiveUnauthenticated(t *testing.T) {
	client := newLiveDevinClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	jwt, _, err := client.auth(ctx, "sub2api-deliberately-invalid-live-validation-token")
	var provider *Error
	if jwt != "" || !errors.As(err, &provider) || (provider.StatusCode != 400 && provider.StatusCode != 401 && provider.StatusCode != 403) {
		if err != nil {
			liveDevinCheckError(t, err, "expected credential rejection")
		}
		t.Fatal("invalid credential was not rejected with an authentication/validation error")
	}
	t.Logf("real Devin auth endpoint rejected deliberately invalid credential: status=%d; subscription access NOT validated", provider.StatusCode)
}

func liveDevinToken() (string, error) {
	if token := strings.TrimSpace(os.Getenv("DEVIN_SESSION_TOKEN")); token != "" {
		return token, nil
	}
	path := strings.TrimSpace(os.Getenv("DEVIN_SESSION_TOKEN_FILE"))
	if path == "" {
		return "", nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("cannot open the explicitly configured Devin token file")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("Devin token file must be a regular file readable only by its owner (chmod 600)")
	}
	data, err := io.ReadAll(io.LimitReader(file, 64*1024+1))
	if err != nil || len(data) > 64*1024 {
		return "", errors.New("Devin token file is unreadable or exceeds 64 KiB")
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", errors.New("Devin token file is empty")
	}
	return token, nil
}

func TestDevinTokenFilePolicy(t *testing.T) {
	t.Setenv("DEVIN_SESSION_TOKEN", "")
	t.Setenv("DEVIN_SESSION_TOKEN_FILE", "")
	if token, err := liveDevinToken(); token != "" || err != nil {
		t.Fatal("unconfigured token source should remain unavailable")
	}
	path := filepath.Join(t.TempDir(), "session-token")
	if err := os.WriteFile(path, []byte("synthetic-session\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEVIN_SESSION_TOKEN_FILE", path)
	if token, err := liveDevinToken(); token != "synthetic-session" || err != nil {
		t.Fatal("explicit owner-only token file was not read")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDevinToken(); err == nil {
		t.Fatal("world-readable token file was accepted")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 64*1024+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := liveDevinToken(); err == nil {
		t.Fatal("oversized token file was accepted")
	}
}

func TestDevinLiveCatalog(t *testing.T) {
	client := newLiveDevinClient(t)
	token, err := liveDevinToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Skip("no explicit Devin credential; live catalog NOT queried")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	models, err := client.Models(ctx, token)
	liveDevinCheckError(t, err, "model discovery")
	t.Logf("authenticated native catalog: %d models", len(models))
	for _, model := range models {
		tools := "unspecified"
		if model.supportsTools != nil {
			if *model.supportsTools {
				tools = "yes"
			} else {
				tools = "no"
			}
		}
		t.Logf("model=%s max_tokens=%d tools=%s router=%t images=%t", model.ID, model.MaxTokens, tools, model.Router, model.SupportsImages)
	}
}

func TestDevinLiveSubscription(t *testing.T) {
	client := newLiveDevinClient(t)
	token, err := liveDevinToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Skip("DEVIN_SESSION_TOKEN/DEVIN_SESSION_TOKEN_FILE is not configured: authenticated live validation NOT run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	models, err := client.Models(ctx, token)
	liveDevinCheckError(t, err, "model discovery")
	if len(models) == 0 {
		models = DefaultModels()
		t.Log("empty live catalog; using documented seeds")
	}
	wanted := strings.TrimSpace(os.Getenv("DEVIN_LIVE_MODEL"))
	if wanted == "" {
		wanted = "swe-1-6"
	}
	var selected *Model
	for i := range models {
		if models[i].ID == wanted {
			selected = &models[i]
			break
		}
	}
	if selected == nil {
		t.Fatal("requested DEVIN_LIVE_MODEL is not in this credential's catalog; no inference performed")
	}
	t.Logf("authenticated model discovery returned %d models; using explicitly selected/default model", len(models))

	call := func(t *testing.T, cascade string, payload any) *http.Response {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal("failed to encode fixed live test request")
		}
		response, err := client.Chat(ctx, token, cascade, *selected, body)
		liveDevinCheckError(t, err, "inference")
		t.Cleanup(func() { _ = response.Body.Close() })
		if response.StatusCode != http.StatusOK {
			t.Fatalf("unexpected adapted response status=%d", response.StatusCode)
		}
		return response
	}
	decode := func(t *testing.T, response *http.Response) apicompat.ChatCompletionsResponse {
		t.Helper()
		var result apicompat.ChatCompletionsResponse
		liveDevinCheckError(t, json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result), "JSON response")
		if len(result.Choices) != 1 || result.ID == "" || result.Usage == nil || result.Usage.TotalTokens <= 0 {
			t.Fatal("missing choice, response identity, or token usage")
		}
		return result
	}
	messages := []map[string]string{{"role": "user", "content": "Reply briefly with DEVIN_OK."}}
	t.Run("nonstream", func(t *testing.T) {
		result := decode(t, call(t, randomID(), map[string]any{"model": wanted, "max_tokens": 128, "messages": messages}))
		var text string
		if json.Unmarshal(result.Choices[0].Message.Content, &text) != nil || strings.TrimSpace(text) == "" {
			t.Fatal("empty non-streaming text")
		}
		t.Logf("live nonstream validated: input_tokens=%d output_tokens=%d", result.Usage.PromptTokens, result.Usage.CompletionTokens)
	})
	if t.Failed() {
		return
	}
	t.Run("stream", func(t *testing.T) {
		response := call(t, randomID(), map[string]any{"model": wanted, "stream": true, "max_tokens": 128, "messages": messages})
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 4096), 2<<20)
		id, text, done, usage := "", false, false, false
		for scanner.Scan() {
			payload, ok := strings.CutPrefix(scanner.Text(), "data: ")
			if !ok {
				continue
			}
			if payload == "[DONE]" {
				done = true
				continue
			}
			var chunk apicompat.ChatCompletionsChunk
			if json.Unmarshal([]byte(payload), &chunk) != nil {
				t.Fatal("invalid SSE JSON")
			}
			if id == "" {
				id = chunk.ID
			}
			if chunk.ID != id {
				t.Fatal("stream completion identity changed")
			}
			if chunk.Usage != nil && chunk.Usage.TotalTokens > 0 {
				usage = true
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.Content != nil && *choice.Delta.Content != "" {
					text = true
				}
			}
		}
		liveDevinCheckError(t, scanner.Err(), "SSE response")
		if !done || !text || !usage {
			t.Fatal("stream missing terminal event, text, or usage")
		}
		t.Log("live streaming text, stable identity, usage and successful trailer validated")
	})
	if t.Failed() {
		return
	}
	t.Run("tool_roundtrip", func(t *testing.T) {
		if selected.supportsTools != nil && !*selected.supportsTools {
			t.Skip("selected live model explicitly does not support tools")
		}
		cascade := randomID()
		tools := []map[string]any{{"type": "function", "function": map[string]any{"name": "echo", "description": "Echo the supplied marker", "parameters": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string", "enum": []string{"DEVIN_OK"}}}, "required": []string{"value"}, "additionalProperties": false}}}}
		initial := map[string]string{"role": "user", "content": "Call echo with the value DEVIN_OK, then briefly report the returned value."}
		result := decode(t, call(t, cascade, map[string]any{"model": wanted, "max_tokens": 128, "messages": []any{initial}, "tools": tools, "tool_choice": map[string]any{"type": "function", "function": map[string]string{"name": "echo"}}}))
		assistant := result.Choices[0].Message
		if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].Function.Name != "echo" || assistant.ToolCalls[0].ID == "" || !json.Valid([]byte(assistant.ToolCalls[0].Function.Arguments)) {
			t.Fatal("live response did not contain the requested valid tool call")
		}
		continued := decode(t, call(t, cascade, map[string]any{"model": wanted, "max_tokens": 128, "tools": tools, "tool_choice": "none", "messages": []any{initial, assistant, map[string]string{"role": "tool", "tool_call_id": assistant.ToolCalls[0].ID, "content": "DEVIN_OK"}}}))
		var text string
		if json.Unmarshal(continued.Choices[0].Message.Content, &text) != nil || strings.TrimSpace(text) == "" {
			t.Fatal("tool-result continuation returned no text")
		}
		t.Log("live function call and linked tool-result continuation validated")
	})
}
