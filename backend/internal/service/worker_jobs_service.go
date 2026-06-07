package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	WorkerJobCapabilityTextBasic       = "text.basic"
	WorkerJobCapabilityImageGeneration = "image.generation"

	WorkerJobStatusSucceeded = "succeeded"
	WorkerExecutorPyWorker   = "py-worker"

	WorkerJobsTokenHeader      = "X-Sub2API-Worker-Token"
	workerJobsTokenEnv         = "SUB2API_WORKER_SHARED_TOKEN"
	workerPublicBaseURLEnv     = "SUB2API_WORKER_PUBLIC_BASE_URL"
	workerExecutionAPIKeyEnv   = "SUB2API_WORKER_EXECUTION_API_KEY"
	defaultWorkerPublicBaseURL = "http://127.0.0.1:8080"
)

var workerSupportedCapabilities = []string{
	WorkerJobCapabilityTextBasic,
	WorkerJobCapabilityImageGeneration,
}

type WorkerJobExecuteInput struct {
	JobID       string            `json:"job_id" binding:"required"`
	Capability  string            `json:"capability" binding:"required"`
	Input       map[string]any    `json:"input,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	RequestedBy string            `json:"requested_by,omitempty"`
}

type WorkerJobExecuteResult struct {
	JobID      string            `json:"job_id"`
	Status     string            `json:"status"`
	Executor   string            `json:"executor"`
	Capability string            `json:"capability"`
	Result     any               `json:"result,omitempty"`
	Error      string            `json:"error,omitempty"`
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt time.Time         `json:"finished_at"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type WorkerHealthStatus struct {
	Status       string   `json:"status"`
	Executor     string   `json:"executor"`
	Capabilities []string `json:"capabilities"`
	AuthHeader   string   `json:"auth_header"`
}

type WorkerJobsService struct {
	sharedToken     string
	publicBaseURL   string
	executionAPIKey string
	client          *http.Client
}

func NewWorkerJobsService() *WorkerJobsService {
	return &WorkerJobsService{
		sharedToken:     strings.TrimSpace(os.Getenv(workerJobsTokenEnv)),
		publicBaseURL:   workerNormalizedBaseURL(strings.TrimSpace(os.Getenv(workerPublicBaseURLEnv))),
		executionAPIKey: strings.TrimSpace(os.Getenv(workerExecutionAPIKeyEnv)),
		client:          &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *WorkerJobsService) VerifyToken(token string) error {
	if s.sharedToken == "" {
		return infraerrors.ServiceUnavailable("WORKER_AUTH_NOT_CONFIGURED", "worker shared token is not configured")
	}
	if strings.TrimSpace(token) == "" {
		return infraerrors.Unauthorized("WORKER_AUTH_REQUIRED", "worker token is required")
	}
	if strings.TrimSpace(token) != s.sharedToken {
		return infraerrors.Unauthorized("WORKER_AUTH_INVALID", "worker token is invalid")
	}
	return nil
}

func (s *WorkerJobsService) Execute(ctx context.Context, input WorkerJobExecuteInput) (WorkerJobExecuteResult, error) {
	capability := strings.TrimSpace(input.Capability)
	if capability == "" {
		return WorkerJobExecuteResult{}, infraerrors.BadRequest("WORKER_JOB_CAPABILITY_REQUIRED", "capability is required")
	}
	if !slices.Contains(workerSupportedCapabilities, capability) {
		return WorkerJobExecuteResult{}, infraerrors.BadRequest("WORKER_JOB_CAPABILITY_UNSUPPORTED", "capability is not supported by worker")
	}
	if strings.TrimSpace(s.executionAPIKey) == "" {
		return WorkerJobExecuteResult{}, infraerrors.ServiceUnavailable("WORKER_EXECUTION_AUTH_NOT_CONFIGURED", "worker execution API key is not configured")
	}

	startedAt := time.Now().UTC()
	result, err := s.executeLocalGateway(ctx, capability, cloneWorkerAnyMap(input.Input))
	if err != nil {
		return WorkerJobExecuteResult{}, err
	}
	finishedAt := time.Now().UTC()
	return WorkerJobExecuteResult{
		JobID:      strings.TrimSpace(input.JobID),
		Status:     WorkerJobStatusSucceeded,
		Executor:   WorkerExecutorPyWorker,
		Capability: capability,
		Result:     result,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Metadata:   cloneWorkerStringMap(input.Metadata),
	}, nil
}

func (s *WorkerJobsService) Health(_ context.Context) (WorkerHealthStatus, error) {
	capabilities := append([]string(nil), workerSupportedCapabilities...)
	return WorkerHealthStatus{
		Status:       "ok",
		Executor:     WorkerExecutorPyWorker,
		Capabilities: capabilities,
		AuthHeader:   WorkerJobsTokenHeader,
	}, nil
}

func cloneWorkerAnyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneWorkerStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (s *WorkerJobsService) executeLocalGateway(ctx context.Context, capability string, input map[string]any) (any, error) {
	endpoint, err := workerLocalEndpointForCapability(capability)
	if err != nil {
		return nil, err
	}

	if input == nil {
		input = map[string]any{}
	}
	if stream, ok := input["stream"].(bool); ok && stream {
		return nil, infraerrors.BadRequest("WORKER_JOB_STREAM_UNSUPPORTED", "stream=true is not supported for worker job execution")
	}

	body, err := json.Marshal(input)
	if err != nil {
		return nil, infraerrors.BadRequest("WORKER_JOB_INPUT_INVALID", "job input must be valid JSON object").WithCause(err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.publicBaseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, infraerrors.InternalServer("WORKER_JOB_REQUEST_BUILD_FAILED", "failed to build worker job request").WithCause(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.executionAPIKey)
	req.Header.Set("x-api-key", s.executionAPIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("WORKER_JOB_REQUEST_FAILED", "worker local execution request failed").WithCause(err)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if readErr != nil {
		return nil, infraerrors.ServiceUnavailable("WORKER_JOB_RESPONSE_READ_FAILED", "failed to read worker execution response").WithCause(readErr)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, infraerrors.ServiceUnavailable("WORKER_JOB_BAD_STATUS", fmt.Sprintf("worker local executor returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody))))
	}

	var parsed any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, infraerrors.ServiceUnavailable("WORKER_JOB_RESPONSE_INVALID", "worker execution response is not valid JSON").WithCause(err)
	}
	return parsed, nil
}

func workerLocalEndpointForCapability(capability string) (string, error) {
	switch strings.TrimSpace(capability) {
	case WorkerJobCapabilityTextBasic:
		return "/v1/responses", nil
	case WorkerJobCapabilityImageGeneration:
		return "/v1/images/generations", nil
	default:
		return "", infraerrors.BadRequest("WORKER_JOB_CAPABILITY_UNSUPPORTED", "capability is not supported by worker")
	}
}

func workerNormalizedBaseURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return defaultWorkerPublicBaseURL
	}
	return raw
}
