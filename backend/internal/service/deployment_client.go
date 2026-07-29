package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
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
	ID                             string     `json:"id"`
	Action                         string     `json:"action"`
	TargetVersion                  string     `json:"target_version"`
	Status                         string     `json:"status"`
	Stage                          string     `json:"stage"`
	Message                        string     `json:"message,omitempty"`
	Error                          string     `json:"error,omitempty"`
	FromVersion                    string     `json:"from_version,omitempty"`
	TargetImage                    string     `json:"target_image,omitempty"`
	TargetDigest                   string     `json:"target_digest,omitempty"`
	RollbackPerformed              bool       `json:"rollback_performed"`
	BackgroundActivated            bool       `json:"background_activated"`
	RollbackError                  string     `json:"rollback_error,omitempty"`
	CleanupWarning                 string     `json:"cleanup_warning,omitempty"`
	ControlPlaneUpgradeStatus      string     `json:"control_plane_upgrade_status,omitempty"`
	ControlPlaneUpgradeError       string     `json:"control_plane_upgrade_error,omitempty"`
	ControlPlaneUpgradeAttempt     int        `json:"control_plane_upgrade_attempt,omitempty"`
	ControlPlaneUpgradeMaxAttempts int        `json:"control_plane_upgrade_max_attempts,omitempty"`
	ControlPlaneUpgradeNextAttempt *time.Time `json:"control_plane_upgrade_next_attempt_at,omitempty"`
	CreatedAt                      time.Time  `json:"created_at"`
	StartedAt                      time.Time  `json:"started_at"`
	UpdatedAt                      time.Time  `json:"updated_at"`
	FinishedAt                     *time.Time `json:"finished_at,omitempty"`
}

type DeploymentRequest struct {
	Action                 string `json:"action"`
	TargetVersion          string `json:"target_version"`
	ExpectedTargetDigest   string `json:"expected_target_digest,omitempty"`
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
	expectedDigest, err := s.verifiedReleaseDigest(ctx, info)
	if err != nil {
		return nil, fmt.Errorf("verify release completion ledger: %w", err)
	}
	return s.deployer.Start(ctx, DeploymentRequest{
		Action:                 "update",
		TargetVersion:          info.LatestVersion,
		ExpectedTargetDigest:   expectedDigest,
		ExpectedCurrentVersion: s.currentVersion,
		RequestID:              requestID,
	})
}

const releaseCompletionAsset = "sub2api-release-complete.json"

var (
	releaseDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	releaseObjectPattern = regexp.MustCompile(`^[0-9a-f]{40,64}$`)
)

type releaseCompletionLedger struct {
	Schema                  int               `json:"schema"`
	Tag                     string            `json:"tag"`
	Commit                  string            `json:"commit"`
	TagObject               string            `json:"tag_object"`
	Image                   string            `json:"image"`
	ImageDigest             string            `json:"image_digest"`
	ImmutableImage          string            `json:"immutable_image"`
	DockerHubImage          *string           `json:"dockerhub_image"`
	DockerHubImageDigest    *string           `json:"dockerhub_image_digest"`
	DockerHubImmutableImage *string           `json:"dockerhub_immutable_image"`
	Architectures           []string          `json:"architectures"`
	ControlPlaneManifestSHA string            `json:"control_plane_manifest_sha256"`
	CandidateManifestSHA    string            `json:"candidate_manifest_sha256"`
	DeployerChecksumsSHA256 string            `json:"deployer_checksums_sha256"`
	DeployerAssets          map[string]string `json:"deployer_assets"`
}

func (s *UpdateService) verifiedReleaseDigest(ctx context.Context, info *UpdateInfo) (string, error) {
	if info == nil || info.ReleaseInfo == nil {
		return "", errors.New("latest release metadata is missing")
	}
	var assetURL string
	for _, asset := range info.ReleaseInfo.Assets {
		if asset.Name != releaseCompletionAsset {
			continue
		}
		if assetURL != "" {
			return "", errors.New("latest release contains duplicate completion ledgers")
		}
		assetURL = asset.DownloadURL
	}
	if assetURL == "" {
		return "", errors.New("latest release has no completion ledger")
	}
	if err := validateDownloadURL(assetURL); err != nil {
		return "", fmt.Errorf("completion ledger URL is invalid: %w", err)
	}
	data, err := s.githubClient.FetchChecksumFile(ctx, assetURL)
	if err != nil {
		return "", fmt.Errorf("download completion ledger: %w", err)
	}
	if len(data) > 1024*1024 {
		return "", errors.New("completion ledger exceeds 1 MiB")
	}
	var ledger releaseCompletionLedger
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ledger); err != nil {
		return "", fmt.Errorf("decode completion ledger: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", errors.New("completion ledger contains trailing JSON data")
	}
	expectedTag := "v" + info.LatestVersion
	expectedImage := "ghcr.io/" + strings.ToLower(githubRepo) + ":" + info.LatestVersion
	if ledger.Schema != 3 || ledger.Tag != expectedTag || ledger.Image != expectedImage ||
		!releaseDigestPattern.MatchString(ledger.ImageDigest) ||
		ledger.ImmutableImage != ledger.Image+"@"+ledger.ImageDigest ||
		!releaseObjectPattern.MatchString(ledger.Commit) ||
		!releaseObjectPattern.MatchString(ledger.TagObject) ||
		!releaseDigestPattern.MatchString(ledger.ControlPlaneManifestSHA) ||
		!releaseDigestPattern.MatchString(ledger.CandidateManifestSHA) ||
		!releaseDigestPattern.MatchString(ledger.DeployerChecksumsSHA256) {
		return "", errors.New("completion ledger identity is invalid")
	}
	architectures := append([]string(nil), ledger.Architectures...)
	sort.Strings(architectures)
	if len(architectures) != 2 || architectures[0] != "amd64" || architectures[1] != "arm64" {
		return "", errors.New("completion ledger architecture set is invalid")
	}
	for _, name := range []string{
		"sub2api-deployer-linux-amd64",
		"sub2api-deployer-linux-arm64",
		"sub2api-deployer-linux-amd64.tar.gz",
		"sub2api-deployer-linux-arm64.tar.gz",
	} {
		if !releaseDigestPattern.MatchString(ledger.DeployerAssets[name]) {
			return "", fmt.Errorf("completion ledger deployer asset %s is invalid", name)
		}
	}
	if len(ledger.DeployerAssets) != 4 {
		return "", errors.New("completion ledger contains an unexpected deployer asset set")
	}
	return ledger.ImageDigest, nil
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
