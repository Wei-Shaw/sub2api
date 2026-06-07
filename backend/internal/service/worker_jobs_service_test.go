package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestWorkerJobsServiceVerifyTokenRequiresConfig(t *testing.T) {
	t.Setenv(workerJobsTokenEnv, "")
	svc := NewWorkerJobsService()

	err := svc.VerifyToken("secret")
	require.Error(t, err)
	require.Equal(t, "WORKER_AUTH_NOT_CONFIGURED", infraerrors.Reason(err))
}

func TestWorkerJobsServiceExecuteSupportsPilotCapabilities(t *testing.T) {
	t.Setenv(workerJobsTokenEnv, "secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/images/generations", r.URL.Path)
		require.Equal(t, "Bearer worker-local-key", r.Header.Get("Authorization"))
		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "draw a cat", req["prompt"])
		_ = json.NewEncoder(w).Encode(map[string]any{
			"created": 123,
			"data":    []map[string]any{{"b64_json": "png"}},
		})
	}))
	defer server.Close()
	t.Setenv(workerPublicBaseURLEnv, server.URL)
	t.Setenv(workerExecutionAPIKeyEnv, "worker-local-key")
	svc := NewWorkerJobsService()

	result, err := svc.Execute(context.Background(), WorkerJobExecuteInput{
		JobID:      "job-123",
		Capability: WorkerJobCapabilityImageGeneration,
		Input: map[string]any{
			"prompt": "draw a cat",
		},
		Metadata: map[string]string{
			"origin": "center",
		},
		RequestedBy: "center",
	})
	require.NoError(t, err)
	require.Equal(t, WorkerExecutorPyWorker, result.Executor)
	require.Equal(t, WorkerJobStatusSucceeded, result.Status)
	require.Equal(t, WorkerJobCapabilityImageGeneration, result.Capability)
	payload, ok := result.Result.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(123), payload["created"])
}

func TestWorkerJobsServiceExecuteRejectsUnsupportedCapability(t *testing.T) {
	t.Setenv(workerJobsTokenEnv, "secret")
	svc := NewWorkerJobsService()

	_, err := svc.Execute(context.Background(), WorkerJobExecuteInput{
		JobID:      "job-unsupported",
		Capability: "audio.transcription",
	})
	require.Error(t, err)
	require.Equal(t, "WORKER_JOB_CAPABILITY_UNSUPPORTED", infraerrors.Reason(err))
}
