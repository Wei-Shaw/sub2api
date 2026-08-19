//go:build unit

package leonardo

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
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
			require.Equal(t, "leonardo", request.Provider)
			require.Equal(t, "IMAGE_GENERATION", request.TaskType)
			require.Equal(t, "gpt-image-2", request.Model)
			require.Equal(t, "LOW", request.Input.Quality)
			require.Equal(t, "1:1", request.Input.AspectRatio)
			require.Equal(t, "SMALL", request.Input.Size)
			require.Equal(t, "1024x1024", request.Input.Resolution)
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

func jsonResponse(status int, value any) *http.Response {
	raw, _ := json.Marshal(value)
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(raw)),
	}
}
