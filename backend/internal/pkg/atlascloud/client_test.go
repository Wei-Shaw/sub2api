package atlascloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
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

func TestDoJSONDebugLogIncludesSanitizedRequest(t *testing.T) {
	var logBuf bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	longPrompt := strings.Repeat("你", 400)
	requestBody := map[string]any{
		"input": map[string]any{
			"prompt": longPrompt,
			"width":  1280,
		},
	}
	var sentBody []byte
	client := &Client{
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var err error
			sentBody, err = io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read outbound request body: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"id":"prediction-1"}`)),
				Request:    req,
			}, nil
		})},
		apiKey: "atlas-secret-key",
	}

	const endpoint = "https://atlas.example.test/api/v1/model/generateVideo"
	if _, err := client.doJSON(context.Background(), http.MethodPost, endpoint, requestBody); err != nil {
		t.Fatalf("doJSON returned error: %v", err)
	}

	var requestLog map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logBuf.String()), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("decode log line: %v", err)
		}
		if entry["msg"] == "atlascloud_http_request" {
			requestLog = entry
			break
		}
	}
	if requestLog == nil {
		t.Fatalf("request log not found: %s", logBuf.String())
	}
	if requestLog["url"] != endpoint {
		t.Fatalf("url = %#v, want %q", requestLog["url"], endpoint)
	}
	headers, ok := requestLog["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers = %#v, want object", requestLog["headers"])
	}
	authorization, ok := headers["Authorization"].([]any)
	if !ok || len(authorization) != 1 || authorization[0] != "[REDACTED]" {
		t.Fatalf("Authorization = %#v, want redacted value", headers["Authorization"])
	}
	if strings.Contains(logBuf.String(), "atlas-secret-key") {
		t.Fatalf("request log leaked API key: %s", logBuf.String())
	}

	loggedBody, ok := requestLog["body"].(string)
	if !ok {
		t.Fatalf("body = %#v, want string", requestLog["body"])
	}
	var decodedBody map[string]any
	if err := json.Unmarshal([]byte(loggedBody), &decodedBody); err != nil {
		t.Fatalf("decode logged body: %v", err)
	}
	input, ok := decodedBody["input"].(map[string]any)
	if !ok {
		t.Fatalf("input = %#v, want object", decodedBody["input"])
	}
	loggedPrompt, ok := input["prompt"].(string)
	if !ok {
		t.Fatalf("prompt = %#v, want string", input["prompt"])
	}
	if len(loggedPrompt) > promptLogLimitBytes {
		t.Fatalf("logged prompt is %d bytes, want at most %d", len(loggedPrompt), promptLogLimitBytes)
	}
	if !utf8.ValidString(loggedPrompt) || !strings.HasSuffix(loggedPrompt, "...(truncated)") {
		t.Fatalf("logged prompt was not safely truncated: %q", loggedPrompt)
	}
	if input["width"] != float64(1280) {
		t.Fatalf("non-prompt field changed: %#v", input)
	}

	var outboundBody map[string]any
	if err := json.Unmarshal(sentBody, &outboundBody); err != nil {
		t.Fatalf("decode outbound body: %v", err)
	}
	outboundInput, ok := outboundBody["input"].(map[string]any)
	if !ok {
		t.Fatalf("outbound input = %#v, want object", outboundBody["input"])
	}
	outboundPrompt := outboundInput["prompt"]
	if outboundPrompt != longPrompt {
		t.Fatalf("outbound prompt was modified")
	}
}

func TestRequestBodyForLogTruncatesPromptFieldsRecursively(t *testing.T) {
	longASCII := strings.Repeat("a", promptLogLimitBytes+1)
	raw := []byte(`{"prompt":"` + longASCII + `","items":[{"PROMPT":"` + longASCII + `","name":"kept"}]}`)

	logged := requestBodyForLog(raw)
	var body map[string]any
	if err := json.Unmarshal([]byte(logged), &body); err != nil {
		t.Fatalf("decode logged body: %v", err)
	}
	got, ok := body["prompt"].(string)
	if !ok {
		t.Fatalf("top-level prompt = %#v, want string", body["prompt"])
	}
	if len(got) > promptLogLimitBytes || !strings.HasSuffix(got, "...(truncated)") {
		t.Fatalf("top-level prompt = %q (%d bytes)", got, len(got))
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v, want one-item array", body["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item = %#v, want object", items[0])
	}
	nestedPrompt, ok := item["PROMPT"].(string)
	if !ok {
		t.Fatalf("nested prompt = %#v, want string", item["PROMPT"])
	}
	if len(nestedPrompt) > promptLogLimitBytes || !strings.HasSuffix(nestedPrompt, "...(truncated)") {
		t.Fatalf("nested prompt = %q (%d bytes)", nestedPrompt, len(nestedPrompt))
	}
	if item["name"] != "kept" {
		t.Fatalf("non-prompt field changed: %#v", item)
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

func TestStatusReturnsCompletedVideoResultFromData(t *testing.T) {
	const videoURL = "https://storage.atlascloud.ai/outputs/result.mp4?token=secret"
	requestCount := 0
	client := &Client{
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{
					"code":200,
					"data":{
						"id":"prediction-1",
						"status":"completed",
						"outputs":["` + videoURL + `"]
					}
				}`)),
				Request: req,
			}, nil
		})},
		apiKey: "atlas-key-for-test",
	}

	status, err := client.Status(context.Background(), "https://atlas.example.test/api/v1/model/prediction/prediction-1")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if !status.IsTerminal() {
		t.Fatalf("status = %q, want completed", status.Status)
	}
	wantVideo := map[string]any{
		"url":       videoURL,
		"file_name": "result.mp4",
	}
	want := map[string]any{
		"video":  wantVideo,
		"videos": []any{wantVideo},
	}
	if !reflect.DeepEqual(status.Result, want) {
		t.Fatalf("status result = %#v, want %#v", status.Result, want)
	}
	if requestCount != 1 {
		t.Fatalf("request count = %d, want 1", requestCount)
	}
}

func TestStatusReturnsOnlyNestedErrorForFailedHTTPResponse(t *testing.T) {
	const providerError = "The requested video duration is not supported by this model."
	client := &Client{
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{
					"code":400,
					"data":{
						"id":"prediction-1",
						"status":"failed",
						"error":"` + providerError + `",
						"urls":{"get":"https://api.atlascloud.ai/api/v1/model/prediction/prediction-1"}
					}
				}`)),
				Request: req,
			}, nil
		})},
		apiKey: "atlas-key-for-test",
	}

	_, err := client.Status(context.Background(), "https://atlas.example.test/api/v1/model/prediction/prediction-1")
	var apiErr *fal.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Status error = %T %v, want *fal.APIError", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusBadRequest)
	}
	if apiErr.Body != providerError {
		t.Fatalf("Body = %q, want only nested error %q", apiErr.Body, providerError)
	}
	if strings.Contains(apiErr.Body, "prediction-1") || strings.Contains(strings.ToLower(apiErr.Body), "atlascloud") {
		t.Fatalf("Body leaked provider response metadata: %q", apiErr.Body)
	}
}

func TestResultRawNormalizesCompletedPrediction(t *testing.T) {
	client := testClientWithResponse(`{
		"id":"pred_abc123",
		"status":"completed",
		"model":"model-name",
		"outputs":["https://storage.atlascloud.ai/outputs/result.mp4?token=secret"],
		"metrics":{"predict_time":45.2},
		"created_at":"2025-01-01T00:00:00Z",
		"completed_at":"2025-01-01T00:00:10Z"
	}`)

	result, err := client.ResultRaw(context.Background(), "https://atlas.example.test/api/v1/model/prediction/pred_abc123")
	if err != nil {
		t.Fatalf("ResultRaw returned error: %v", err)
	}
	wantVideo := map[string]any{
		"url":       "https://storage.atlascloud.ai/outputs/result.mp4?token=secret",
		"file_name": "result.mp4",
	}
	want := map[string]any{
		"video":  wantVideo,
		"videos": []any{wantVideo},
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("ResultRaw = %#v, want %#v", result, want)
	}
}

func TestResultRawReturnsEmptyPayloadWithoutOutputURLs(t *testing.T) {
	client := testClientWithResponse(`{"id":"pred_abc123","status":"completed","outputs":[" "]}`)

	result, err := client.ResultRaw(context.Background(), "https://atlas.example.test/api/v1/model/prediction/pred_abc123")
	if err != nil {
		t.Fatalf("ResultRaw returned error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("ResultRaw = %#v, want empty payload", result)
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
