package atlascloud

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDoJSONDebugLogIncludesCompleteResponseBody(t *testing.T) {
	var logBuf bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	const responseBody = `{"id":"prediction-1","status":"processing","outputs":["https://cdn.example.com/result.mp4"]}`
	client := &Client{
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(responseBody)),
				Request:    req,
			}, nil
		})},
		apiKey: "atlas-key-for-test",
	}

	if _, err := client.doJSON(context.Background(), http.MethodPost, "https://atlas.example.test/generateVideo", nil); err != nil {
		t.Fatalf("doJSON returned error: %v", err)
	}

	logText := logBuf.String()
	if !strings.Contains(logText, `"msg":"atlascloud_http_response"`) {
		t.Fatalf("debug log should contain response event: %s", logText)
	}
	if !strings.Contains(logText, `"body":`+strconv.Quote(responseBody)) {
		t.Fatalf("debug log should contain complete response body: %s", logText)
	}
}

func TestSubmitRawReadsIDFromData(t *testing.T) {
	client := testClientWithResponse(`{"code":200,"data":{"id":"prediction-1","status":"processing"}}`)

	resp, err := client.SubmitRaw(context.Background(), "model", map[string]any{"prompt": "test"})
	if err != nil {
		t.Fatalf("SubmitRaw returned error: %v", err)
	}
	if resp.RequestID != "prediction-1" {
		t.Fatalf("RequestID = %q, want prediction-1", resp.RequestID)
	}
	wantURL := "https://atlas.example.test/api/v1/model/prediction/prediction-1"
	if resp.StatusURL != wantURL || resp.ResponseURL != wantURL {
		t.Fatalf("prediction URLs = %q / %q, want %q", resp.StatusURL, resp.ResponseURL, wantURL)
	}
}

func TestDoPredictionReadsStatusAndOutputsFromData(t *testing.T) {
	client := testClientWithResponse(`{"code":200,"data":{"id":"prediction-1","status":"completed","outputs":["https://cdn.example.test/result.mp4"]}}`)

	resp, err := client.doPrediction(context.Background(), http.MethodGet, "https://atlas.example.test/prediction/prediction-1", nil)
	if err != nil {
		t.Fatalf("doPrediction returned error: %v", err)
	}
	if resp.ID != "prediction-1" || resp.Status != "completed" {
		t.Fatalf("parsed id/status = %q/%q", resp.ID, resp.Status)
	}
	if len(resp.Outputs) != 1 || resp.Outputs[0] != "https://cdn.example.test/result.mp4" {
		t.Fatalf("parsed outputs = %#v", resp.Outputs)
	}
	if _, ok := resp.Raw["data"]; !ok {
		t.Fatalf("raw response should retain data envelope: %#v", resp.Raw)
	}
}

func testClientWithResponse(responseBody string) *Client {
	return &Client{
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(responseBody)),
				Request:    req,
			}, nil
		})},
		apiKey:  "atlas-key-for-test",
		baseURL: "https://atlas.example.test",
	}
}
