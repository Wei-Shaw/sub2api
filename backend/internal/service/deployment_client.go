package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type DeploymentHealth struct {
	Status                   string `json:"status"`
	Version                  string `json:"version"`
	ActiveSlot               string `json:"active_slot"`
	ActiveContainer          string `json:"active_container"`
	ActiveContainerID        string `json:"active_container_id"`
	ActivePort               int    `json:"active_port"`
	ActiveVersion            string `json:"active_version"`
	JobRunning               bool   `json:"job_running"`
	ControlPlaneUpgradeReady bool   `json:"control_plane_upgrade_ready"`
}

type DeploymentJob struct {
	ID                        string     `json:"id"`
	Action                    string     `json:"action"`
	TargetVersion             string     `json:"target_version"`
	Status                    string     `json:"status"`
	Stage                     string     `json:"stage"`
	Message                   string     `json:"message,omitempty"`
	Error                     string     `json:"error,omitempty"`
	FromVersion               string     `json:"from_version,omitempty"`
	TargetImage               string     `json:"target_image,omitempty"`
	TargetDigest              string     `json:"target_digest,omitempty"`
	RollbackPerformed         bool       `json:"rollback_performed"`
	BackgroundActivated       bool       `json:"background_activated"`
	RollbackError             string     `json:"rollback_error,omitempty"`
	CleanupWarning            string     `json:"cleanup_warning,omitempty"`
	ControlPlaneUpgradeStatus string     `json:"control_plane_upgrade_status,omitempty"`
	ControlPlaneUpgradeError  string     `json:"control_plane_upgrade_error,omitempty"`
	CreatedAt                 time.Time  `json:"created_at"`
	StartedAt                 time.Time  `json:"started_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
	FinishedAt                *time.Time `json:"finished_at,omitempty"`
}

type DeploymentRequest struct {
	Action                 string `json:"action"`
	TargetVersion          string `json:"target_version"`
	ExpectedCurrentVersion string `json:"expected_current_version,omitempty"`
	RequestID              string `json:"request_id"`
}

type DeploymentClient interface {
	Health(ctx context.Context) (*DeploymentHealth, error)
	Start(ctx context.Context, req DeploymentRequest) (*DeploymentJob, error)
	Job(ctx context.Context, id string) (*DeploymentJob, error)
	CurrentJob(ctx context.Context) (*DeploymentJob, error)
}

type UnixDeploymentClient struct {
	client *http.Client
}

func NewUnixDeploymentClient(socketPath string, timeout time.Duration) *UnixDeploymentClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socketPath)
		},
		DisableCompression: true,
		MaxIdleConns:       4,
		IdleConnTimeout:    30 * time.Second,
	}
	return &UnixDeploymentClient{client: &http.Client{Transport: transport, Timeout: timeout}}
}

func (c *UnixDeploymentClient) Health(ctx context.Context) (*DeploymentHealth, error) {
	var result DeploymentHealth
	if err := c.do(ctx, http.MethodGet, "/v1/health", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *UnixDeploymentClient) Start(ctx context.Context, request DeploymentRequest) (*DeploymentJob, error) {
	var result DeploymentJob
	if err := c.do(ctx, http.MethodPost, "/v1/deployments", request, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *UnixDeploymentClient) Job(ctx context.Context, id string) (*DeploymentJob, error) {
	var result DeploymentJob
	if err := c.do(ctx, http.MethodGet, "/v1/jobs/"+url.PathEscape(id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *UnixDeploymentClient) CurrentJob(ctx context.Context) (*DeploymentJob, error) {
	var result DeploymentJob
	if err := c.do(ctx, http.MethodGet, "/v1/jobs/current", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *UnixDeploymentClient) do(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://unix"+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return ErrManagedDeployerUnavailable.WithCause(fmt.Errorf("deployment agent request failed: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var problem struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &problem)
		if problem.Error == "" {
			problem.Error = strings.TrimSpace(string(data))
		}
		switch resp.StatusCode {
		case http.StatusConflict:
			return infraerrors.Conflict("DEPLOYMENT_CONFLICT", problem.Error)
		case http.StatusNotFound:
			return infraerrors.NotFound("DEPLOYMENT_JOB_NOT_FOUND", problem.Error)
		case http.StatusBadRequest:
			return infraerrors.BadRequest("INVALID_DEPLOYMENT_REQUEST", problem.Error)
		default:
			return ErrManagedDeployerUnavailable.WithCause(fmt.Errorf("deployment agent returned HTTP %d: %s", resp.StatusCode, problem.Error))
		}
	}
	if output != nil && len(data) > 0 {
		if err := json.Unmarshal(data, output); err != nil {
			return fmt.Errorf("decode deployment agent response: %w", err)
		}
	}
	return nil
}

func normalizeDeploymentMode(mode, buildType string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if buildType == "source" {
		return DeploymentModeSource
	}
	switch mode {
	case DeploymentModeSource, DeploymentModeStandaloneBinary, DeploymentModeDockerManual, DeploymentModeDockerManaged:
		return mode
	case "docker":
		return DeploymentModeDockerManaged
	case "binary":
		return DeploymentModeStandaloneBinary
	case "", "auto":
		if _, err := os.Stat("/.dockerenv"); err == nil {
			return DeploymentModeDockerManual
		}
		return DeploymentModeStandaloneBinary
	default:
		return DeploymentModeDockerManual
	}
}

func (s *UpdateService) decorateDeployment(ctx context.Context, info *UpdateInfo) {
	info.DeploymentMode = s.deploymentMode
	switch s.deploymentMode {
	case DeploymentModeStandaloneBinary:
		info.DeploymentReady = true
	case DeploymentModeDockerManaged:
		if s.deployer == nil {
			info.DeploymentMessage = "Docker deployment agent is not configured"
			return
		}
		healthCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		health, err := s.deployer.Health(healthCtx)
		if err != nil || health == nil || health.Status != "ok" {
			info.DeploymentMessage = "Docker deployment agent is unavailable"
			return
		}
		if !health.ControlPlaneUpgradeReady {
			info.DeploymentMessage = "Run the one-time host deployer bootstrap before using one-click updates"
			return
		}
		info.DeploymentReady = true
	case DeploymentModeDockerManual:
		info.DeploymentMessage = "Install and connect sub2api-deployer to enable one-click updates"
	default:
		info.DeploymentMessage = "Online updates are unavailable for source builds"
	}
}

func (s *UpdateService) StartManagedUpdate(ctx context.Context, requestID string) (*DeploymentJob, error) {
	if s.deploymentMode != DeploymentModeDockerManaged || s.deployer == nil {
		if s.deploymentMode == DeploymentModeDockerManual {
			return nil, ErrDockerManualUpdate
		}
		return nil, ErrManagedDeployerUnavailable
	}
	if err := s.requireManagedDeployerUpgradeReady(ctx); err != nil {
		return nil, err
	}
	info, err := s.CheckUpdate(ctx, true)
	if err != nil {
		return nil, err
	}
	if !info.HasUpdate {
		return nil, ErrNoUpdateAvailable
	}
	return s.deployer.Start(ctx, DeploymentRequest{
		Action:                 "update",
		TargetVersion:          info.LatestVersion,
		ExpectedCurrentVersion: s.currentVersion,
		RequestID:              requestID,
	})
}

func (s *UpdateService) requireManagedDeployerUpgradeReady(ctx context.Context) error {
	healthCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	health, err := s.deployer.Health(healthCtx)
	if err != nil || health == nil || health.Status != "ok" {
		return ErrManagedDeployerUnavailable
	}
	if !health.ControlPlaneUpgradeReady {
		return ErrManagedDeployerBootstrapRequired
	}
	return nil
}

func (s *UpdateService) StartManagedRollback(ctx context.Context, version, requestID string) (*DeploymentJob, error) {
	if s.deploymentMode != DeploymentModeDockerManaged || s.deployer == nil {
		if s.deploymentMode == DeploymentModeDockerManual {
			return nil, ErrDockerManualUpdate
		}
		return nil, ErrManagedDeployerUnavailable
	}
	target := strings.TrimPrefix(strings.TrimSpace(version), "v")
	versions, err := s.ListRollbackVersions(ctx)
	if err != nil {
		return nil, err
	}
	allowed := false
	for _, item := range versions {
		if item.Version == target {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, ErrRollbackVersionNotAllowed
	}
	return s.deployer.Start(ctx, DeploymentRequest{
		Action:                 "rollback",
		TargetVersion:          target,
		ExpectedCurrentVersion: s.currentVersion,
		RequestID:              requestID,
	})
}

func (s *UpdateService) DeploymentJob(ctx context.Context, id string) (*DeploymentJob, error) {
	if s.deployer == nil {
		return nil, ErrManagedDeployerUnavailable
	}
	return s.deployer.Job(ctx, id)
}

func (s *UpdateService) CurrentDeploymentJob(ctx context.Context) (*DeploymentJob, error) {
	if s.deployer == nil {
		return nil, ErrManagedDeployerUnavailable
	}
	return s.deployer.CurrentJob(ctx)
}
