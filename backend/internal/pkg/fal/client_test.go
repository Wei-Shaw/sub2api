package fal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDoJSONDebugLogSanitizesFileContent(t *testing.T) {
	var logBuf bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	t.Cleanup(func() {
		slog.SetDefault(originalLogger)
	})

	client := &Client{
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			_, _ = io.Copy(io.Discard, req.Body)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"images":[]}`)),
				Request:    req,
			}, nil
		})},
		apiKey: "fal-key-for-test",
	}
	filePayload := "data:image/png;base64," + strings.Repeat("A", 512) + "secret-file-content"
	reqBody := &Request{
		Prompt:    "ordinary prompt should stay visible",
		ImageURLs: []string{filePayload, "https://cdn.example.com/input.png"},
	}
	rawBody, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	if err := client.doJSON(context.Background(), http.MethodPost, "https://fal.example.test/model", reqBody, nil); err != nil {
		t.Fatalf("doJSON returned error: %v", err)
	}

	logText := logBuf.String()
	if !strings.Contains(logText, `"body":`) {
		t.Fatalf("debug log should contain sanitized body field: %s", logText)
	}
	if !strings.Contains(logText, "ordinary prompt should stay visible") {
		t.Fatalf("debug log should keep ordinary body fields, got: %s", logText)
	}
	if !strings.Contains(logText, "https://cdn.example.com/input.png") {
		t.Fatalf("debug log should keep ordinary URLs, got: %s", logText)
	}
	if strings.Contains(logText, "secret-file-content") || strings.Contains(logText, strings.Repeat("A", 128)) {
		t.Fatalf("debug log leaked file content: %s", logText)
	}
	if !strings.Contains(logText, "redacted file content: kind=data:image/png;base64") {
		t.Fatalf("debug log should redact file content with summary, got: %s", logText)
	}
	if !strings.Contains(logText, fmt.Sprintf(`"body_bytes":%d`, len(rawBody))) {
		t.Fatalf("debug log should contain request body length, got: %s", logText)
	}
	if !strings.Contains(logText, `"body_sanitized":true`) {
		t.Fatalf("debug log should mark sanitized body, got: %s", logText)
	}
}

func TestSanitizeRequestBodyForLogTruncatesLongSanitizedBody(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"prompt": strings.Repeat("x", debugRequestBodyLogLimitBytes+128),
	})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	body, sanitized, truncated := sanitizeRequestBodyForLog(raw)
	if sanitized {
		t.Fatalf("plain long body should not be marked sanitized")
	}
	if !truncated {
		t.Fatalf("plain long body should be marked truncated")
	}
	if len(body) <= debugRequestBodyLogLimitBytes {
		t.Fatalf("truncated body should include suffix, got len=%d", len(body))
	}
	if !strings.Contains(body, "...(truncated, bytes=") {
		t.Fatalf("truncated body should include byte summary, got: %s", body)
	}
}
