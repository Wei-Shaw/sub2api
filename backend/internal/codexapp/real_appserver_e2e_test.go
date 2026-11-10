//go:build codexapp_real_e2e

package codexapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const realAppServerE2EToken = "fake-codex-e2e-token"

type capturedResponsesRequest struct {
	Method        string
	Path          string
	Authorization string
	ContentType   string
	Body          map[string]any
	Err           error
}

func TestRealAppServerAgainstFakeResponses(t *testing.T) {
	codexBin := strings.TrimSpace(os.Getenv("CODEX_REAL_E2E_BIN"))
	if codexBin == "" {
		t.Skip("CODEX_REAL_E2E_BIN is not set; run backend/scripts/test-codex-appserver-blackbox.sh")
	}

	requests := make(chan capturedResponsesRequest, 4)
	upstream := httptest.NewServer(fakeResponsesHandler(requests))
	defer upstream.Close()

	codexHome := t.TempDir()
	var stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider, err := Start(ctx, Config{
		Command: realAppServerCommand(codexBin, upstream.URL+"/v1"),
		Env: []string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + codexHome,
			"CODEX_HOME=" + codexHome,
			"MOCK_OPENAI_API_KEY=" + realAppServerE2EToken,
			"RUST_LOG=error",
		},
		ClientInfo: ClientInfo{
			Name:    "sub2api_codex_provider_e2e",
			Title:   "Sub2API Codex Provider E2E",
			Version: "test",
		},
		StartTimeout: 15 * time.Second,
		Stderr:       &stderr,
	})
	if err != nil {
		t.Fatalf("Start(real app-server) error = %v\nstderr:\n%s", err, stderr.String())
	}
	t.Cleanup(func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		if err := provider.Close(closeCtx); err != nil {
			t.Errorf("Close(real app-server) error = %v\nstderr:\n%s", err, stderr.String())
		}
	})

	var events []Event
	result, err := provider.RunTurn(ctx, TurnRequest{
		Model:     "gpt-6-sol",
		Input:     []InputItem{{Type: "text", Text: "Reply with hello only."}},
		Ephemeral: true,
	}, func(event Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("RunTurn(real app-server) error = %v\nstderr:\n%s", err, stderr.String())
	}
	if result.ThreadID == "" || result.TurnID == "" || result.Status != "completed" {
		t.Fatalf("RunTurn(real app-server) result = %#v", result)
	}
	if !containsDelta(events, "hello") {
		t.Fatalf("events do not contain hello delta: %#v", events)
	}
	if !containsCompleted(events) {
		t.Fatalf("events do not contain completed: %#v", events)
	}

	select {
	case request := <-requests:
		if request.Err != nil {
			t.Fatalf("fake Responses request error = %v", request.Err)
		}
		if request.Method != http.MethodPost || request.Path != "/v1/responses" {
			t.Fatalf("fake Responses request = %s %s", request.Method, request.Path)
		}
		if request.Authorization != "Bearer "+realAppServerE2EToken {
			t.Fatalf("Authorization = %q", request.Authorization)
		}
		if !strings.HasPrefix(request.ContentType, "application/json") {
			t.Fatalf("Content-Type = %q", request.ContentType)
		}
		if request.Body["model"] != "gpt-6-sol" {
			t.Fatalf("request model = %#v", request.Body["model"])
		}
		if request.Body["stream"] != true {
			t.Fatalf("request stream = %#v", request.Body["stream"])
		}
		if input, ok := request.Body["input"].([]any); !ok || len(input) == 0 {
			t.Fatalf("request input = %#v", request.Body["input"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("fake Responses upstream did not capture a request")
	}

	if _, err := os.Stat(filepath.Join(codexHome, "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("isolated CODEX_HOME unexpectedly contains auth.json: %v", err)
	}
}

func realAppServerCommand(codexBin, baseURL string) []string {
	return []string{
		codexBin,
		"app-server",
		"--listen", "stdio://",
		"-c", `model="gpt-6-sol"`,
		"-c", `model_provider="sub2api-e2e"`,
		"-c", `web_search="disabled"`,
		"-c", `model_providers.sub2api-e2e.name="Sub2API E2E Fake Responses"`,
		"-c", fmt.Sprintf(`model_providers.sub2api-e2e.base_url=%q`, baseURL),
		"-c", `model_providers.sub2api-e2e.env_key="MOCK_OPENAI_API_KEY"`,
		"-c", `model_providers.sub2api-e2e.wire_api="responses"`,
		"-c", `model_providers.sub2api-e2e.requires_openai_auth=false`,
		"-c", `model_providers.sub2api-e2e.supports_websockets=false`,
	}
}

func fakeResponsesHandler(requests chan<- capturedResponsesRequest) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/models") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"gpt-6-sol","object":"model","created":0,"owned_by":"sub2api-e2e"}]}`)
			return
		}

		capture := capturedResponsesRequest{
			Method:        r.Method,
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			ContentType:   r.Header.Get("Content-Type"),
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err != nil {
			capture.Err = fmt.Errorf("read request body: %w", err)
		} else if err := json.Unmarshal(body, &capture.Body); err != nil {
			capture.Err = fmt.Errorf("decode request body: %w", err)
		}
		requests <- capture

		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" || capture.Err != nil {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, event := range []map[string]any{
			{
				"type":     "response.created",
				"response": map[string]any{"id": "resp-sub2api-e2e"},
			},
			{
				"type": "response.output_item.added",
				"item": map[string]any{
					"type": "message", "role": "assistant", "id": "msg-sub2api-e2e",
					"content": []any{map[string]any{"type": "output_text", "text": ""}},
				},
			},
			{"type": "response.output_text.delta", "delta": "hello"},
			{
				"type": "response.output_item.done",
				"item": map[string]any{
					"type": "message", "role": "assistant", "id": "msg-sub2api-e2e",
					"content": []any{map[string]any{"type": "output_text", "text": "hello"}},
				},
			},
			{
				"type": "response.completed",
				"response": map[string]any{
					"id": "resp-sub2api-e2e",
					"usage": map[string]any{
						"input_tokens": 1, "output_tokens": 1, "total_tokens": 2,
						"input_tokens_details": nil, "output_tokens_details": nil,
					},
				},
			},
		} {
			kind, _ := event["type"].(string)
			payload, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, payload)
			if flusher != nil {
				flusher.Flush()
			}
		}
	})
}

func containsDelta(events []Event, delta string) bool {
	for _, event := range events {
		if event.Type == EventAgentMessageDelta && strings.Contains(event.Delta, delta) {
			return true
		}
	}
	return false
}

func containsCompleted(events []Event) bool {
	for _, event := range events {
		if event.Type == EventTurnCompleted && event.Status == "completed" {
			return true
		}
	}
	return false
}
