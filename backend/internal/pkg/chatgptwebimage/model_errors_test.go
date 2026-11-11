package chatgptwebimage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestConversation404ClassifiesOnlyStructuredModelNotFound(t *testing.T) {
	for _, tc := range []struct {
		name, body, kind string
	}{
		{"detail", `{"detail":"Model not found","private":"account-secret"}`, "model_not_found"},
		{"message", `{"message":"Model not found"}`, "model_not_found"},
		{"code", `{"code":"model_not_found","message":"private upstream message"}`, "model_not_found"},
		{"detail object", `{"detail":{"message":"Model not found"}}`, "model_not_found"},
		{"error object", `{"error":{"message":"Model not found"}}`, "model_not_found"},
		{"nested code", `{"error":{"code":"model_not_found","message":"private upstream message"}}`, "model_not_found"},
		{"non JSON", `<html>Model not found: account-secret</html>`, "upstream_http"},
		{"other JSON error", `{"detail":"Conversation not found"}`, "upstream_http"},
		{"message prefix", `{"message":"Model not found: account-secret"}`, "upstream_http"},
		{"different case", `{"message":"model not found"}`, "upstream_http"},
		{"unrelated field", `{"prompt":"Model not found"}`, "upstream_http"},
		{"oversized body", `{"detail":"Model not found","private":"` + strings.Repeat("x", 64<<10) + `"}`, "upstream_http"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var generations atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if fixtureHandshake(t, w, r) {
					return
				}
				if r.URL.Path != "/backend-api/f/conversation" {
					t.Errorf("unexpected retry or fallback path: %s", r.URL.Path)
				}
				generations.Add(1)
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			_, err := testClient(t, server, nil).Generate(context.Background(), GenerateRequest{
				AccessToken: "account-secret", Prompt: "image", AllowRegularChat: true,
			})
			var upstream *Error
			if !errors.As(err, &upstream) {
				t.Fatalf("expected provider error, got %v", err)
			}
			if upstream.Kind != tc.kind || upstream.Stage != "conversation" || upstream.StatusCode != http.StatusNotFound || upstream.Accepted {
				t.Fatalf("unexpected classification: %v", upstream)
			}
			if upstream.Diagnostic().Kind != tc.kind {
				t.Fatalf("diagnostic lost safe classification: %#v", upstream.Diagnostic())
			}
			if generations.Load() != 1 {
				t.Fatalf("generation was automatically retried: %d", generations.Load())
			}
			if strings.Contains(upstream.Error(), "account-secret") || strings.Contains(upstream.Error(), "private upstream message") {
				t.Fatal("upstream response content escaped")
			}
		})
	}
}

type unexpectedModelErrorBody struct{ t *testing.T }

func (r unexpectedModelErrorBody) Read([]byte) (int, error) {
	r.t.Error("classification must not read bodies outside conversation 404")
	return 0, io.EOF
}

func (unexpectedModelErrorBody) Close() error { return nil }

func TestModelNotFoundClassificationDoesNotChangeOtherStatusHandling(t *testing.T) {
	for _, tc := range []struct {
		stage, kind string
		status      int
	}{
		{"prepare", "upstream_http", http.StatusNotFound},
		{"conversation", "upstream_http", http.StatusBadRequest},
		{"conversation", "authentication_or_challenge", http.StatusForbidden},
	} {
		s := &session{accepted: true}
		err := s.status(tc.stage, &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: unexpectedModelErrorBody{t}})
		if err.Kind != tc.kind || !err.Accepted {
			t.Fatalf("existing status or acceptance semantics changed: %v", err)
		}
	}
	// Parsing a model error itself must never rewrite the acceptance flag.
	s := &session{accepted: true}
	err := s.status("conversation", &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"detail":"Model not found"}`))})
	if err.Kind != "model_not_found" || !err.Accepted {
		t.Fatalf("model classification changed acceptance semantics: %v", err)
	}
}
