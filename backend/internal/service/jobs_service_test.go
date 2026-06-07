package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfiguredRemoteJobsExecutorsUsesRegistryFirst(t *testing.T) {
	t.Setenv(jobsWorkerRegistryEnv, `[{"name":"py-worker","base_url":"http://10.0.0.12:8080","capabilities":["text.basic","image.generation"],"timeout_ms":4500}]`)
	t.Setenv(legacyJobsPyWorkerURLEnv, "http://legacy-worker:8080")
	t.Setenv(jobsWorkerSharedTokenEnv, "shared-secret")

	executors := configuredRemoteJobsExecutors(time.Second)
	require.Len(t, executors, 1)
	require.Equal(t, "py-worker", executors[0].Name())
	require.Equal(t, "remote", executors[0].Kind())
	require.True(t, executors[0].Supports(JobCapabilityTextBasic))
	require.True(t, executors[0].Supports(JobCapabilityImageGeneration))
}

func TestConfiguredRemoteJobsExecutorsFallsBackToLegacyEnv(t *testing.T) {
	t.Setenv(jobsWorkerRegistryEnv, `not-json`)
	t.Setenv(legacyJobsPyWorkerURLEnv, "http://legacy-worker:8080")
	t.Setenv(jobsWorkerSharedTokenEnv, "shared-secret")

	executors := configuredRemoteJobsExecutors(time.Second)
	require.Len(t, executors, 1)
	require.Equal(t, JobExecutorPyWorker, executors[0].Name())
	require.True(t, executors[0].Supports(JobCapabilityTextBasic))
}

func TestJobsServiceCreateJobUsesLocalExecutorByDefault(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/responses", r.URL.Path)
		require.Equal(t, "Bearer local-key", r.Header.Get("Authorization"))
		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "hello", req["prompt"])
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp_local",
			"object": "response",
		})
	}))
	defer local.Close()

	t.Setenv(jobsLocalBaseURLEnv, local.URL)
	svc := newJobsService(time.Second, nil)

	job, err := svc.CreateJob(context.Background(), CreateJobInput{
		Capability: JobCapabilityTextBasic,
		Input: map[string]any{
			"prompt": "hello",
		},
		ExecutionToken: "local-key",
	})
	require.NoError(t, err)
	require.Equal(t, JobStatusSucceeded, job.Status)
	require.Equal(t, JobExecutorLocal, job.SelectedExecutor)
	require.Equal(t, "local", job.SelectedExecutorKind)
	require.Equal(t, []string{JobExecutorLocal}, job.DispatchTrace)
	result, ok := job.Result.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "resp_local", result["id"])
}

func TestJobsServiceCreateJobFallsBackToLocalWhenRemoteFails(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "worker unavailable", http.StatusServiceUnavailable)
	}))
	defer remote.Close()
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/images/generations", r.URL.Path)
		require.Equal(t, "Bearer local-key", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"created": 123,
			"data":    []map[string]any{{"b64_json": "abc"}},
		})
	}))
	defer local.Close()

	t.Setenv(jobsLocalBaseURLEnv, local.URL)
	svc := newJobsService(time.Second, []jobsExecutor{
		newRemoteJobsExecutor(JobExecutorPyWorker, remote.URL, time.Second, []string{JobCapabilityImageGeneration}),
	})

	job, err := svc.CreateJob(context.Background(), CreateJobInput{
		Capability: JobCapabilityImageGeneration,
		Input: map[string]any{
			"prompt": "draw a cat",
		},
		ExecutionToken: "local-key",
	})
	require.NoError(t, err)
	require.Equal(t, JobStatusSucceeded, job.Status)
	require.Equal(t, JobExecutorLocal, job.SelectedExecutor)
	require.Equal(t, []string{JobExecutorPyWorker, JobExecutorLocal}, job.DispatchTrace)
}

func TestJobsServiceCreateJobUsesRemoteWorkerWhenAvailable(t *testing.T) {
	t.Setenv(jobsWorkerSharedTokenEnv, "shared-secret")
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/internal/jobs/execute", r.URL.Path)
		require.Equal(t, "shared-secret", r.Header.Get("X-Sub2API-Worker-Token"))
		var req remoteJobExecuteRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, JobCapabilityTextBasic, req.Capability)
		_ = json.NewEncoder(w).Encode(remoteJobExecuteResponse{
			Status: JobStatusSucceeded,
			Result: map[string]any{
				"handled_by": JobExecutorPyWorker,
			},
		})
	}))
	defer remote.Close()

	svc := newJobsService(time.Second, []jobsExecutor{
		newRemoteJobsExecutor(JobExecutorPyWorker, remote.URL, time.Second, []string{JobCapabilityTextBasic}),
	})

	job, err := svc.CreateJob(context.Background(), CreateJobInput{
		Capability: JobCapabilityTextBasic,
		Input: map[string]any{
			"prompt": "hello",
		},
	})
	require.NoError(t, err)
	require.Equal(t, JobStatusSucceeded, job.Status)
	require.Equal(t, JobExecutorPyWorker, job.SelectedExecutor)
	require.Equal(t, "remote", job.SelectedExecutorKind)
	require.Equal(t, []string{JobExecutorPyWorker}, job.DispatchTrace)
}

func TestJobsServiceCreateJobRoutesToPreferredRegistryWorker(t *testing.T) {
	t.Setenv(jobsWorkerSharedTokenEnv, "shared-secret")
	workerA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "wrong worker", http.StatusConflict)
	}))
	defer workerA.Close()
	workerB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "shared-secret", r.Header.Get("X-Sub2API-Worker-Token"))
		_ = json.NewEncoder(w).Encode(remoteJobExecuteResponse{
			Status: JobStatusSucceeded,
			Result: map[string]any{
				"handled_by": "image-worker",
			},
		})
	}))
	defer workerB.Close()

	svc := newJobsService(time.Second, []jobsExecutor{
		newRemoteJobsExecutor("text-worker", workerA.URL, time.Second, []string{JobCapabilityTextBasic}),
		newRemoteJobsExecutor("image-worker", workerB.URL, time.Second, []string{JobCapabilityImageGeneration}),
	})

	job, err := svc.CreateJob(context.Background(), CreateJobInput{
		Capability:     JobCapabilityImageGeneration,
		PreferExecutor: "image-worker",
		Input: map[string]any{
			"prompt": "draw a cat",
		},
	})
	require.NoError(t, err)
	require.Equal(t, JobStatusSucceeded, job.Status)
	require.Equal(t, "image-worker", job.SelectedExecutor)
	require.Equal(t, []string{"image-worker"}, job.DispatchTrace)
}

func TestJobsServiceGetJobReturnsNotFound(t *testing.T) {
	svc := newJobsService(time.Second, nil)
	_, err := svc.GetJob(context.Background(), "missing")
	require.Error(t, err)
}
