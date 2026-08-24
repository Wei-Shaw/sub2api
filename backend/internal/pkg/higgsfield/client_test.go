package higgsfield

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestClientUsesSDKProtocolAndNormalizesResult(t *testing.T) {
	var methods []string
	var paths []string
	var authHeaders []string
	var requestBodies []string
	const baseURL = "https://higgsfield.test"
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		if r.Body != nil {
			bodyBytes, _ := io.ReadAll(r.Body)
			requestBodies = append(requestBodies, string(bodyBytes))
		}
		var body string
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/bytedance/seedance/v4/text-to-video":
			body = `{"request_id":"req-1","status_url":"/requests/req-1/status","cancel_url":"/requests/req-1/cancel"}`
		case r.Method == http.MethodGet:
			body = `{"status":"completed","output":{"video":{"url":"https://cdn.example/video.mp4"}}}`
		case r.Method == http.MethodPost && r.URL.Path == "/requests/req-1/cancel":
			body = `{}`
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"not found"}`)), Request: r}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})

	client, err := NewClient(Config{APIKey: "hf-key", BaseURL: baseURL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.httpClient.Transport = transport
	submit, err := client.SubmitRaw(context.Background(), "bytedance/seedance/v4/text-to-video", map[string]any{"prompt": "test"})
	if err != nil {
		t.Fatalf("SubmitRaw: %v", err)
	}
	if submit.RequestID != "req-1" || submit.StatusURL != baseURL+"/requests/req-1/status" {
		t.Fatalf("submit = %#v", submit)
	}
	status, err := client.Status(context.Background(), submit.StatusURL)
	if err != nil || status.Status != fal.StatusCompleted {
		t.Fatalf("Status = %#v, err=%v", status, err)
	}
	result, err := client.ResultRaw(context.Background(), submit.ResponseURL)
	if err != nil {
		t.Fatalf("ResultRaw: %v", err)
	}
	if got := fal.ExtractVideoURLs(result); len(got) != 1 || got[0] != "https://cdn.example/video.mp4" {
		t.Fatalf("video urls = %#v", got)
	}
	if err := client.Cancel(context.Background(), submit.CancelURL); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if strings.Join(authHeaders, ",") != "Key hf-key,Key hf-key,Key hf-key,Key hf-key" {
		t.Fatalf("authorization headers = %#v", authHeaders)
	}
	if len(methods) != 4 || paths[0] != "/bytedance/seedance/v4/text-to-video" {
		t.Fatalf("requests = %#v %#v", methods, paths)
	}
	if requestBodies[0] != `{"prompt":"test"}` {
		t.Fatalf("submit body = %q", requestBodies[0])
	}
}

func TestClientRejectsFailedStatus(t *testing.T) {
	client, err := NewClient(Config{APIKey: "hf-key", BaseURL: "https://higgsfield.test"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(`{"status":"failed","error":"blocked"}`)), Request: r}, nil
	})
	_, err = client.Status(context.Background(), "https://higgsfield.test/requests/req/status")
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("Status error = %v", err)
	}
}
