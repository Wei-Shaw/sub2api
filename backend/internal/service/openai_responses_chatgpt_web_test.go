package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptwebimage"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWebImageResponsesPreservesClientToolsAndStorage(t *testing.T) {
	body := []byte(`{"model":"gpt-6-luna","stream":true,"store":false,"input":"draw a circle","tools":[{"type":"function","name":"exec_command","parameters":{"type":"object"}},{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}],"tool_choice":{"type":"namespace","name":"image_gen"}}`)
	prepared, err := prepareWebImageResponses(body)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(prepared, &got))
	require.Equal(t, "gpt-6-luna", got["model"])
	require.Equal(t, false, got["store"])
	tools := requireWebImageTestValue[[]any](t, got["tools"])
	require.Len(t, tools, 2)
	require.Equal(t, "exec_command", requireWebImageTestValue[map[string]any](t, tools[0])["name"])
	require.Equal(t, webImageFunctionName, requireWebImageTestValue[map[string]any](t, tools[1])["name"])
	require.Equal(t, map[string]any{"type": "function", "name": webImageFunctionName}, got["tool_choice"])
}

func TestWebImagePreviewBoundedAndRemovedOnlyFromAssistantHistory(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{B: 255, A: 255})
		}
	}
	var pngBytes bytes.Buffer
	require.NoError(t, png.Encode(&pngBytes, img))
	preview, err := webImageMarkdownPreview(chatgptwebimage.Image{Bytes: pngBytes.Bytes()})
	require.NoError(t, err)
	require.Less(t, len(preview), 90_000)
	require.Contains(t, preview, "data:image/jpeg;base64,")
	body, _ := json.Marshal(map[string]any{"model": "gpt-6-luna", "input": []any{
		map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": preview}}},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": preview}}},
	}})
	prepared, err := prepareWebImageResponses(body)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(prepared, &got))
	input := requireWebImageTestValue[[]any](t, got["input"])
	assistant := requireWebImageTestValue[map[string]any](t, requireWebImageTestValue[[]any](t, requireWebImageTestValue[map[string]any](t, input[0])["content"])[0])["text"]
	user := requireWebImageTestValue[map[string]any](t, requireWebImageTestValue[[]any](t, requireWebImageTestValue[map[string]any](t, input[1])["content"])[0])["text"]
	require.NotContains(t, assistant, "base64")
	require.Equal(t, preview, user)
}

func TestWebImageResponsesReplaysOnlyBridgeOwnedHistory(t *testing.T) {
	body := []byte(`{"model":"gpt-6-luna","input":[{"type":"image_generation_call","id":"ig_web_example","result":"AAAA"},{"type":"image_generation_call","id":"ig_other","result":"BBBB"},{"type":"message","role":"user","content":"continue"}]}`)
	prepared, err := prepareWebImageResponses(body)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(prepared, &got))
	input := requireWebImageTestValue[[]any](t, got["input"])
	require.Equal(t, "message", requireWebImageTestValue[map[string]any](t, input[0])["type"])
	require.NotContains(t, requireWebImageTestValue[map[string]any](t, input[0]), "result")
	require.Equal(t, "image_generation_call", requireWebImageTestValue[map[string]any](t, input[1])["type"])
	require.Equal(t, "BBBB", requireWebImageTestValue[map[string]any](t, input[1])["result"])
}

func TestWebImageResponsesRejectsUnimplementedContinuationBeforeGeneration(t *testing.T) {
	_, err := prepareWebImageResponses([]byte(`{"model":"gpt-6-luna","previous_response_id":"resp_example"}`))
	require.ErrorContains(t, err, "previous_response_id")
}

func TestWebImageResponseSSEHasConsistentResultAndLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	encoded := base64.StdEncoding.EncodeToString([]byte("verified-image-fixture"))
	response := map[string]any{"id": "resp_web_test", "object": "response", "created_at": 1, "status": "completed", "model": "gpt-6-luna", "output": []any{map[string]any{"type": "image_generation_call", "id": "ig_web_test", "status": "completed", "result": encoded, "output_format": "png"}}}
	writeWebImageResponseSSE(c, response)
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	var events []map[string]any
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var e map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &e))
		events = append(events, e)
	}
	require.Len(t, events, 8)
	for i, e := range events {
		require.Equal(t, float64(i), e["sequence_number"])
	}
	require.Equal(t, "response.output_item.added", events[2]["type"])
	require.Equal(t, "", requireWebImageTestValue[map[string]any](t, events[2]["item"])["result"])
	for _, e := range events[3:6] {
		require.Equal(t, "ig_web_test", e["item_id"])
	}
	require.Equal(t, encoded, requireWebImageTestValue[map[string]any](t, events[6]["item"])["result"])
	parsed, err := webImageCompletedResponse(recorder.Body.Bytes())
	require.NoError(t, err)
	require.Equal(t, encoded, requireWebImageTestValue[map[string]any](t, requireWebImageTestValue[[]any](t, parsed["output"])[0])["result"])
}

func webImageTestEvent(t *testing.T, event map[string]any) []byte {
	t.Helper()
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	return append(append([]byte("data: "), encoded...), []byte("\n\n")...)
}

func webImageTestEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		events = append(events, event)
	}
	return events
}

func TestWebImageResponseStreamsTextBeforePlannerCompletes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	stream := newWebImageResponseStream(context.Background(), c, time.Hour)
	defer stream.Close()
	capture := newWebImageCaptureWriter(context.Background())
	defer capture.Close()
	capture.stream = stream
	created := webImageTestEvent(t, map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_stream", "status": "in_progress", "output": []any{}, "tools": []any{map[string]any{"name": webImageFunctionName}}}})
	_, err := capture.Write(created[:19])
	require.NoError(t, err)
	require.Empty(t, recorder.Body.String())
	_, err = capture.Write(created[19:])
	require.NoError(t, err)
	_, err = capture.Write(webImageTestEvent(t, map[string]any{"type": "response.output_text.delta", "item_id": "msg_text", "output_index": 0, "content_index": 0, "delta": "visible before completion"}))
	require.NoError(t, err)
	// This assertion runs before the upstream completion has been delivered.
	require.Contains(t, recorder.Body.String(), "visible before completion")
	require.NotContains(t, recorder.Body.String(), webImageFunctionName)
	require.NotContains(t, recorder.Body.String(), "response.completed")
	response := map[string]any{"id": "resp_stream", "status": "completed", "output": []any{}}
	_, err = capture.Write(webImageTestEvent(t, map[string]any{"type": "response.completed", "response": response}))
	require.NoError(t, err)
	require.NotContains(t, recorder.Body.String(), "response.completed")
	stream.Complete(response)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.completed"))
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "visible before completion"))
}

func TestWebImageResponseSuppressesPrivateCallAndFinishesSameStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	stream := newWebImageResponseStream(context.Background(), c, time.Hour)
	defer stream.Close()
	feed := func(event map[string]any) {
		t.Helper()
		packet := webImageTestEvent(t, event)
		for offset := 0; offset < len(packet); offset += 7 {
			require.NoError(t, stream.Feed(packet[offset:min(offset+7, len(packet))], http.Header{}))
		}
	}
	feed(map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_image", "output": []any{}}})
	feed(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"type": "function_call", "id": "fc_private", "name": webImageFunctionName, "arguments": ""}})
	feed(map[string]any{"type": "response.function_call_arguments.delta", "output_index": 0, "item_id": "fc_private", "delta": "private prompt"})
	feed(map[string]any{"type": "response.output_item.done", "output_index": 0, "item": map[string]any{"type": "function_call", "id": "fc_private", "name": webImageFunctionName, "arguments": "private prompt"}})
	response := map[string]any{"id": "resp_image", "status": "completed", "output": []any{map[string]any{"type": "image_generation_call", "id": "ig_web_stream", "status": "completed", "result": "cG5n", "output_format": "png"}, map[string]any{"type": "message", "id": "msg_web_preview", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": "preview", "annotations": []any{}}}}}}
	stream.RememberResponse(response)
	stream.BeginImage(0, "ig_web_stream")
	require.Contains(t, recorder.Body.String(), "response.image_generation_call.generating")
	require.NotContains(t, recorder.Body.String(), "response.completed")
	stream.Complete(response)
	body := recorder.Body.String()
	require.NotContains(t, body, "fc_private")
	require.NotContains(t, body, "private prompt")
	require.NotContains(t, body, webImageFunctionName)
	events := webImageTestEvents(t, body)
	imageAdded, completed := 0, 0
	for index, event := range events {
		require.Equal(t, float64(index), event["sequence_number"])
		if event["type"] == "response.output_item.added" && requireWebImageTestValue[map[string]any](t, event["item"])["type"] == "image_generation_call" {
			imageAdded++
			require.Equal(t, "", requireWebImageTestValue[map[string]any](t, event["item"])["result"])
		}
		if event["type"] == "response.completed" {
			completed++
			require.Equal(t, "resp_image", requireWebImageTestValue[map[string]any](t, event["response"])["id"])
		}
	}
	require.Equal(t, 1, imageAdded)
	require.Equal(t, 1, completed)
	parsed, err := webImageCompletedResponse(recorder.Body.Bytes())
	require.NoError(t, err)
	require.Equal(t, "cG5n", requireWebImageTestValue[map[string]any](t, requireWebImageTestValue[[]any](t, parsed["output"])[0])["result"])
}

type webImageHeartbeatRecorder struct {
	*httptest.ResponseRecorder
	heartbeat chan struct{}
}

func (w *webImageHeartbeatRecorder) Write(p []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(p)
	if bytes.Contains(p, []byte(": keepalive")) {
		select {
		case w.heartbeat <- struct{}{}:
		default:
		}
	}
	return n, err
}

func (w *webImageHeartbeatRecorder) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func TestWebImageResponseHeartbeatStopsAndFailureIsSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &webImageHeartbeatRecorder{ResponseRecorder: httptest.NewRecorder(), heartbeat: make(chan struct{}, 1)}
	c, _ := gin.CreateTestContext(recorder)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := newWebImageResponseStream(ctx, c, time.Millisecond)
	defer stream.Close()
	stream.RememberResponse(map[string]any{"id": "resp_failed", "output": []any{}})
	stream.BeginImage(0, "ig_web_failed")
	select {
	case <-recorder.heartbeat:
	case <-time.After(time.Second):
		t.Fatal("image generation did not keep the SSE connection alive")
	}
	stream.Fail(&OpenAIImagesUpstreamError{StatusCode: 400, ErrorType: "image_generation_error", Code: "image_generation_outcome_unknown", Message: "The existing image task is retained; no new generation was submitted"})
	cancel()
	stream.Close()
	body := recorder.Body.String()
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Contains(t, body, ": keepalive\n\n")
	require.NotContains(t, body, "event: response.completed")
	require.Equal(t, 1, strings.Count(body, "event: response.failed"))
	events := webImageTestEvents(t, body)
	last := events[len(events)-1]
	require.Equal(t, "response.failed", last["type"])
	require.Equal(t, "image_generation_outcome_unknown", requireWebImageTestValue[map[string]any](t, requireWebImageTestValue[map[string]any](t, last["response"])["error"])["code"])
	before := recorder.Body.Len()
	stream.Complete(map[string]any{"output": []any{}})
	require.Equal(t, before, recorder.Body.Len())
}

func TestWebImageResponseFailureKeepsRequiredCreatedAt(t *testing.T) {
	for _, first := range []string{
		`{"type":"response.output_text.delta","delta":"Planning image"}`,
		`{"type":"response.created","response":{"id":"resp_web","created_at":1800000000}}`,
		"",
	} {
		t.Run(first, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			stream := newWebImageResponseStream(context.Background(), c, time.Hour)
			defer stream.Close()
			if first != "" {
				require.NoError(t, stream.Feed([]byte("data: "+first+"\n\n"), http.Header{}))
			} else {
				stream.BeginImage(0, "ig_web_failure")
			}
			stream.Fail(&OpenAIImagesUpstreamError{ErrorType: "upstream_error", Code: "upstream_response_failed", Message: "The upstream Responses turn did not complete"})
			events := webImageTestEvents(t, recorder.Body.String())
			require.Equal(t, "response.failed", events[len(events)-1]["type"])
			var createdAt float64
			for index, event := range events {
				require.Equal(t, float64(index), event["sequence_number"])
				if response, ok := event["response"].(map[string]any); ok {
					value := requireWebImageTestValue[float64](t, response["created_at"])
					require.Positive(t, value)
					if createdAt == 0 {
						createdAt = value
					}
					require.Equal(t, createdAt, value)
				}
			}
			if strings.Contains(first, "1800000000") {
				require.Equal(t, float64(1800000000), createdAt)
			}
		})
	}
}

func TestWebImageCapturePropagatesClientCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	capture := newWebImageCaptureWriter(ctx)
	defer capture.Close()
	cancel()
	select {
	case <-capture.CloseNotify():
	case <-time.After(time.Second):
		t.Fatal("capture did not propagate the request cancellation")
	}
}

// requireWebImageTestValue validates decoded fixtures before inspecting fields.
func requireWebImageTestValue[T any](t *testing.T, value any) T {
	t.Helper()
	result, ok := value.(T)
	require.True(t, ok, "unexpected fixture type: %T", value)
	return result
}
