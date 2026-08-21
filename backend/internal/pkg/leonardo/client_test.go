//go:build unit

package leonardo

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestClientSubmitAndGetTask(t *testing.T) {
	var gotKey string
	client, err := NewClient(Config{APIKey: "leo-proxy-api-key", BaseURL: "https://leonardo.example.test"})
	require.NoError(t, err)
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "leo-proxy-api-key", r.Header.Get("X-API-Key"))
		var body []byte
		if r.Body != nil {
			body, err = io.ReadAll(r.Body)
			require.NoError(t, err)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/tasks":
			gotKey = r.Header.Get("Idempotency-Key")
			var request SubmitRequest
			require.NoError(t, json.Unmarshal(body, &request))
			var rawRequest map[string]any
			require.NoError(t, json.Unmarshal(body, &rawRequest))
			rawInput, ok := rawRequest["input"].(map[string]any)
			require.True(t, ok)
			require.NotContains(t, rawInput, "aspect_ratio")
			require.NotContains(t, rawInput, "size")
			require.NotContains(t, rawInput, "resolution")
			require.Equal(t, "leonardo", request.Provider)
			require.Equal(t, "IMAGE_GENERATION", request.TaskType)
			require.Equal(t, "gpt-image-2", request.Model)
			require.Equal(t, "LOW", request.Input.Quality)
			require.Equal(t, 1024, request.Input.Width)
			require.Equal(t, 1024, request.Input.Height)
			return jsonResponse(http.StatusOK, Task{TaskUUID: "task-123", Status: "PENDING"}), nil
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tasks/task-123":
			return jsonResponse(http.StatusOK, Task{TaskUUID: "task-123", Status: StatusCompleted, Output: Output{Media: []Media{{URL: "https://cdn.example/image.png", MediaType: "image/png", Width: 1024, Height: 1024}}}}), nil
		default:
			return jsonResponse(http.StatusNotFound, map[string]string{"error": "not found"}), nil
		}
	})}
	request := BuildSubmitRequest("gpt-image-2", fal.ImageGenInput{Prompt: "studio photo", Quality: "low", Size: "1024x1024"}, 8)
	task, err := client.Submit(context.Background(), request, "readme-gpt-image-2-0001")
	require.NoError(t, err)
	require.Equal(t, "readme-gpt-image-2-0001", gotKey)
	require.Equal(t, "task-123", task.TaskUUID)

	task, err = client.GetTask(context.Background(), task.TaskUUID)
	require.NoError(t, err)
	require.True(t, task.IsCompleted())
	require.Len(t, task.Output.Media, 1)
}

func TestTruncatePromptInJSONOnlyTruncatesPrompt(t *testing.T) {
	longPrompt := strings.Repeat("绘", debugPromptLimit+20)
	got := truncatePromptInJSON([]byte(`{"provider":"leonardo","input":{"prompt":"` + longPrompt + `","quality":"HIGH","resolution":"2048x2048"}}`))

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(got), &payload))
	input, ok := payload["input"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "HIGH", input["quality"])
	require.Equal(t, "2048x2048", input["resolution"])
	prompt, ok := input["prompt"].(string)
	require.True(t, ok)
	require.Contains(t, prompt, "...(truncated)")
	require.Less(t, len([]rune(prompt)), len([]rune(longPrompt)))
}

func TestBuildSubmitRequestAutoQualityUsesMedium(t *testing.T) {
	for _, quality := range []string{"auto", "AUTO"} {
		request := BuildSubmitRequest("gpt-image-2", fal.ImageGenInput{Quality: quality}, 8)
		require.Equal(t, "MEDIUM", request.Input.Quality, quality)
	}
}

func TestBuildSubmitRequestEditUsesReferenceImages(t *testing.T) {
	request := BuildSubmitRequest("gpt-image-2", fal.ImageGenInput{
		Prompt:    "edit this image",
		Size:      "1536x1024",
		IsEdit:    true,
		ImageURLs: []string{" https://cdn.example/input-1.png ", "", "https://cdn.example/input-2.jpg"},
	}, 8)

	require.Equal(t, "image-to-image", request.Mode)
	require.Equal(t, 1536, request.Input.Width)
	require.Equal(t, 1024, request.Input.Height)
	require.Equal(t, []string{"https://cdn.example/input-1.png", "https://cdn.example/input-2.jpg"}, request.Input.ReferenceImageURLs)

	raw, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"provider":"leonardo",
		"task_type":"IMAGE_GENERATION",
		"model":"gpt-image-2",
		"mode":"image-to-image",
		"input":{
			"prompt":"edit this image",
			"quality":"LOW",
			"width":1536,
			"height":1024,
			"reference_image_urls":["https://cdn.example/input-1.png","https://cdn.example/input-2.jpg"]
		},
		"estimated_credit_cost":8
	}`, string(raw))
}

func TestBuildSubmitRequestTextToImageOmitsReferenceImages(t *testing.T) {
	request := BuildSubmitRequest("gpt-image-2", fal.ImageGenInput{
		Prompt:    "draw a new image",
		ImageURLs: []string{"https://cdn.example/ignored.png"},
	}, 8)

	require.Equal(t, "text-to-image", request.Mode)
	require.Empty(t, request.Input.ReferenceImageURLs)
	raw, err := json.Marshal(request)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "reference_image_urls")
}

func TestTaskFailureMessageOmitsPlatformName(t *testing.T) {
	require.Equal(t, "task failed", (&Task{Status: StatusFailed}).FailureMessage())
	require.Equal(t, "task failed: An error occurred.", (&Task{
		Status: StatusFailed,
		Error:  "An error occurred.",
	}).FailureMessage())
}

func jsonResponse(status int, value any) *http.Response {
	raw, _ := json.Marshal(value)
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(raw)),
	}
}
