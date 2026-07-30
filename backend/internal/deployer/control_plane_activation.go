package deployer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	controlPlaneRequestSchema = 2
	controlPlanePayloadSchema = 1
	controlPlaneStatusSchema  = 1
	controlPlaneAssetDeployer = "sub2api-deployer"
	installedDeployerPath     = "/usr/local/sbin/sub2api-deployer"
	defaultActivationAttempts = 5
)

type controlPlaneActivationRequest struct {
	Schema                int                           `json:"schema"`
	PayloadSchema         int                           `json:"payload_schema"`
	JobID                 string                        `json:"job_id"`
	ContainerID           string                        `json:"container_id"`
	ContainerName         string                        `json:"container_name"`
	TargetVersion         string                        `json:"target_version"`
	ExpectedImage         string                        `json:"expected_image"`
	ExpectedImageDigest   string                        `json:"expected_image_digest"`
	StagedManifest        string                        `json:"staged_manifest"`
	StagedManifestSHA256  string                        `json:"staged_manifest_sha256"`
	ExpectedCommit        string                        `json:"expected_commit"`
	ExpectedArch          string                        `json:"expected_arch"`
	MaxAttempts           int                           `json:"max_attempts"`
	CreatedAt             time.Time                     `json:"created_at"`
	Assets                []controlPlaneActivationAsset `json:"assets"`
	LegacyStagedBinary    string                        `json:"staged_binary,omitempty"`
	LegacyStagedBinarySHA string                        `json:"staged_binary_sha256,omitempty"`
}

type controlPlaneActivationAsset struct {
	Type       string `json:"type"`
	StagedPath string `json:"staged_path"`
	SHA256     string `json:"sha256"`
	Owner      int    `json:"owner"`
	Group      int    `json:"group"`
	Mode       uint32 `json:"mode"`
}

type controlPlaneActivationPaths struct {
	Request         string
	Status          string
	State           string
	StagingRoot     string
	Quarantine      string
	Lock            string
	InstalledAssets map[string]string
}

type controlPlaneActivator struct {
	cfg          Config
	runner       CommandRunner
	systemctl    string
	paths        controlPlaneActivationPaths
	now          func() time.Time
	sleep        func(time.Duration)
	healthClient *http.Client
	expectedUID  int
	expectedGID  int
	files        controlPlaneFileOps
}

type controlPlaneFileOps struct {
	write     func(string, []byte, os.FileMode) error
	rename    func(string, string) error
	remove    func(string) error
	removeAll func(string) error
	syncDir   func(string) error
}

func defaultControlPlaneFileOps() controlPlaneFileOps {
	return controlPlaneFileOps{
		write:     atomicWrite,
		rename:    os.Rename,
		remove:    os.Remove,
		removeAll: os.RemoveAll,
		syncDir:   syncDirectory,
	}
}

func controlPlaneStateDirectory(cfg Config) string {
	return filepath.Clean(filepath.Dir(cfg.ControlPlaneUpgradePath))
}

func defaultControlPlaneActivationPaths(cfg Config) controlPlaneActivationPaths {
	stateDir := controlPlaneStateDirectory(cfg)
	request := cfg.ControlPlaneUpgradePath
	return controlPlaneActivationPaths{
		Request:     request,
		Status:      request + ".status",
		State:       cfg.StatePath,
		StagingRoot: filepath.Join(stateDir, "control-plane-staging"),
		Quarantine:  filepath.Join(stateDir, "quarantine"),
		Lock:        filepath.Join(stateDir, "control-plane-activation.lock"),
		InstalledAssets: map[string]string{
			controlPlaneAssetDeployer: installedDeployerPath,
		},
	}
}

// ActivateStagedControlPlane is the stable systemd activator entry point. It
// deliberately runs while the deployer daemon is alive and uses its own lock.
func ActivateStagedControlPlane(ctx context.Context, cfg Config, runner CommandRunner) error {
	if err := requireRoot(); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.ControlPlaneUpgradePath) == "" {
		return errors.New("control-plane upgrade path is not configured")
	}
	activator := newRuntimeControlPlaneActivator(cfg, runner)
	return activator.activate(ctx)
}

func (a *controlPlaneActivator) activate(ctx context.Context) error {
	// This is the timer's normal path. Do not create the lock, status, or logs.
	if _, err := os.Lstat(a.paths.Request); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect activation request: %w", err)
	}
	if err := a.requireSafeSystemctl(); err != nil {
		return err
	}
	if err := validateOwnedDirectory(filepath.Dir(a.paths.Request), a.expectedUID, a.expectedGID, 0700); err != nil {
		return fmt.Errorf("activation state directory is unsafe: %w", err)
	}
	lock, acquired, err := acquireActivationLock(a.paths.Lock, a.expectedUID, a.expectedGID)
	if err != nil {
		return err
	}
	if !acquired {
		return nil
	}
	defer func() { _ = lock.Close() }()
	return a.activateLocked(ctx)
}

func (a *controlPlaneActivator) activateLocked(ctx context.Context) error {
	if err := a.requireSafeSystemctl(); err != nil {
		return err
	}
	req, err := a.readRequest()
	if err != nil {
		return a.failMalformed(err)
	}
	if !requestIDPattern.MatchString(req.JobID) {
		return a.failMalformed(errors.New("activation request job id is invalid"))
	}
	status, err := readControlPlaneStatus(a.paths.Status)
	if err != nil {
		return a.failRequest(req, 1, "activation status is missing or unreadable: "+err.Error(), "permanent", false)
	}
	if err := validatePublishedActivationStatus(req, status); err != nil {
		return a.failRequest(req, 1, "activation status is not valid transaction evidence: "+err.Error(), "permanent", false)
	}
	switch status.Status {
	case "succeeded":
		return a.cleanupSuccessful(req)
	case "failed", "rollback_failed", "quarantined":
		return a.quarantineRequest(req, status.Error)
	}

	attempt := status.Attempt + 1
	if attempt > req.MaxAttempts {
		return a.failRequest(req, req.MaxAttempts, "activation retry budget is exhausted", "transient", false)
	}

	if err := a.validateRequest(req); err != nil {
		return a.failRequest(req, attempt, err.Error(), "permanent", false)
	}
	currentDigest, err := digestRegularFile(a.paths.InstalledAssets[controlPlaneAssetDeployer], a.expectedUID, a.expectedGID, 0755)
	if err != nil {
		return a.failRequest(req, attempt, "inspect installed deployer: "+err.Error(), "permanent", false)
	}
	target := req.Assets[0]
	backupPath := filepath.Join(a.paths.StagingRoot, req.JobID, ".previous-"+target.Type)
	if currentDigest == target.SHA256 {
		if err := a.waitForHealth(ctx, req, true); err == nil {
			return a.succeed(req, attempt)
		}
		// A crash may have happened after the atomic rename but before restart.
		// If a durable previous copy exists, finish the restart instead of
		// overwriting that only known-good rollback point.
		if _, err := digestRegularFile(backupPath, a.expectedUID, a.expectedGID, 0755); err != nil {
			return a.failRequest(req, attempt, "installed deployer matches the target but no safe rollback copy exists", "permanent", false)
		}
		if _, err := a.runner.Run(ctx, nil, a.systemctl, "restart", "sub2api-deployer.service"); err != nil {
			return a.rollback(req, attempt, backupPath, "resume interrupted deployer restart: "+err.Error())
		}
		if err := a.waitForHealth(ctx, req, true); err != nil {
			return a.rollback(req, attempt, backupPath, err.Error())
		}
		return a.succeed(req, attempt)
	}
	currentVersionOutput, err := a.runner.Run(ctx, nil, a.paths.InstalledAssets[controlPlaneAssetDeployer], "--version")
	if err != nil {
		return a.failRequest(req, attempt, "read installed deployer build identity: "+err.Error(), "permanent", false)
	}
	currentIdentity, err := parseControlPlaneBuildIdentity(currentVersionOutput)
	if err != nil {
		return a.failRequest(req, attempt, "read installed deployer build identity: "+err.Error(), "permanent", false)
	}
	if controlPlaneVersionNotNewer(currentIdentity.Version, req.TargetVersion) {
		return a.skip(req, attempt, fmt.Sprintf("target control plane %s is not newer than installed deployer %s", req.TargetVersion, currentIdentity.Version))
	}

	installedPath := a.paths.InstalledAssets[target.Type]
	if _, err := os.Lstat(backupPath); errors.Is(err, os.ErrNotExist) {
		if err := a.copyRegularFile(installedPath, backupPath, os.FileMode(target.Mode)); err != nil {
			return a.failRequest(req, attempt, "create deployer rollback copy: "+err.Error(), "permanent", false)
		}
	} else if err != nil {
		return a.failRequest(req, attempt, "inspect deployer rollback copy: "+err.Error(), "permanent", false)
	} else if _, err := digestRegularFile(backupPath, a.expectedUID, a.expectedGID, 0755); err != nil {
		return a.failRequest(req, attempt, "existing deployer rollback copy is unsafe: "+err.Error(), "permanent", false)
	}
	if err := a.installRegularFile(target.StagedPath, installedPath, os.FileMode(target.Mode)); err != nil {
		return a.failRequest(req, attempt, "install staged deployer: "+err.Error(), "transient", true)
	}

	if _, err := a.runner.Run(ctx, nil, a.systemctl, "restart", "sub2api-deployer.service"); err != nil {
		return a.rollback(req, attempt, backupPath, "restart new deployer: "+err.Error())
	}
	if err := a.waitForHealth(ctx, req, true); err != nil {
		return a.rollback(req, attempt, backupPath, err.Error())
	}
	return a.succeed(req, attempt)
}

func (a *controlPlaneActivator) readRequest() (controlPlaneActivationRequest, error) {
	data, err := readRegularFileMode(a.paths.Request, a.expectedUID, a.expectedGID, 0600)
	if err != nil {
		return controlPlaneActivationRequest{}, fmt.Errorf("read activation request: %w", err)
	}
	var req controlPlaneActivationRequest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(&req); err != nil {
		return controlPlaneActivationRequest{}, fmt.Errorf("decode activation request: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return controlPlaneActivationRequest{}, errors.New("activation request contains trailing JSON data")
	}
	return req, nil
}

func (a *controlPlaneActivator) validateRequest(req controlPlaneActivationRequest) error {
	if req.Schema != controlPlaneRequestSchema || req.PayloadSchema != controlPlanePayloadSchema {
		return fmt.Errorf("unsupported activation request schema %d payload schema %d", req.Schema, req.PayloadSchema)
	}
	if !requestIDPattern.MatchString(req.JobID) || !containerIDRE.MatchString(req.ContainerID) || !containerNamePattern.MatchString(req.ContainerName) || !versionPattern.MatchString(req.TargetVersion) {
		return errors.New("activation request identity is invalid")
	}
	if !digestPattern.MatchString(req.ExpectedImageDigest) || !strings.HasSuffix(req.ExpectedImage, "@"+req.ExpectedImageDigest) {
		return errors.New("activation request image identity is invalid")
	}
	if !digestPattern.MatchString(req.StagedManifestSHA256) || !regexpCommit.MatchString(req.ExpectedCommit) {
		return errors.New("activation request manifest identity is invalid")
	}
	if req.ExpectedArch != "amd64" && req.ExpectedArch != "arm64" {
		return errors.New("activation request architecture is invalid")
	}
	if req.ExpectedArch != runtime.GOARCH {
		return fmt.Errorf("activation request architecture %q does not match host %q", req.ExpectedArch, runtime.GOARCH)
	}
	if req.MaxAttempts < 1 || req.MaxAttempts > 20 || req.CreatedAt.IsZero() {
		return errors.New("activation request retry metadata is invalid")
	}
	if err := validateOwnedDirectory(a.paths.StagingRoot, a.expectedUID, a.expectedGID, 0700); err != nil {
		return fmt.Errorf("activation staging root is unsafe: %w", err)
	}
	stage := filepath.Join(a.paths.StagingRoot, req.JobID)
	if err := validateOwnedDirectory(stage, a.expectedUID, a.expectedGID, 0700); err != nil {
		return fmt.Errorf("verified staging directory is unsafe: %w", err)
	}
	if filepath.Clean(req.StagedManifest) != filepath.Join(stage, "CONTROL-PLANE-MANIFEST.json") {
		return errors.New("staged manifest escaped the verified staging directory")
	}
	manifestDigest, err := digestRegularFile(req.StagedManifest, a.expectedUID, a.expectedGID, 0644)
	if err != nil || manifestDigest != req.StagedManifestSHA256 {
		return errors.New("staged manifest no longer matches the verified request")
	}
	if len(req.Assets) != 1 || req.Assets[0].Type != controlPlaneAssetDeployer {
		return errors.New("activation request contains an unsupported asset set")
	}
	asset := req.Assets[0]
	if _, ok := a.paths.InstalledAssets[asset.Type]; !ok || filepath.Clean(asset.StagedPath) != filepath.Join(stage, asset.Type) {
		return errors.New("staged asset escaped the fixed asset mapping")
	}
	if asset.Owner != 0 || asset.Group != 0 || asset.Mode != 0755 || !digestPattern.MatchString(asset.SHA256) {
		return errors.New("staged asset ownership or mode is invalid")
	}
	installedPath := a.paths.InstalledAssets[asset.Type]
	if err := validateOwnedDirectory(filepath.Dir(installedPath), a.expectedUID, a.expectedGID, 0755); err != nil {
		return fmt.Errorf("installed asset directory is unsafe: %w", err)
	}
	assetDigest, err := digestRegularFile(asset.StagedPath, a.expectedUID, a.expectedGID, 0755)
	if err != nil || assetDigest != asset.SHA256 {
		return errors.New("staged deployer no longer matches the verified request")
	}

	state, err := readActivationState(a.paths.State, a.expectedUID, a.expectedGID)
	if err != nil {
		return fmt.Errorf("read deployer state: %w", err)
	}
	if state.Degraded || state.ActiveContainerID != req.ContainerID || state.ActiveContainer != req.ContainerName || state.ActiveVersion != req.TargetVersion || state.ActiveImage != req.ExpectedImage || state.Job == nil || state.Job.ID != req.JobID || state.Job.Status != JobStatusSucceeded {
		return errors.New("activation request no longer matches the successful active deployment")
	}
	return nil
}

var regexpCommit = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

func (a *controlPlaneActivator) waitForHealth(ctx context.Context, req controlPlaneActivationRequest, targetBuild bool) error {
	var lastErr error
	for probe := 0; probe < 40; probe++ {
		if probe > 0 {
			a.sleep(250 * time.Millisecond)
		}
		health, err := a.readHealth(ctx)
		if err == nil && health.Status == "ok" && !health.Degraded && !health.JobRunning && health.ControlPlaneUpgradeReady && health.ActiveContainerID == req.ContainerID && health.ActiveVersion == req.TargetVersion {
			targetSHA := req.Assets[0].SHA256
			if !targetBuild || (health.Build.Version == req.TargetVersion && health.Build.Commit == req.ExpectedCommit && health.Build.Type == "release" && health.Build.Arch == req.ExpectedArch && health.Build.SHA256 == targetSHA && health.ControlPlane.Activator == "go-v1" && health.ControlPlane.PayloadSchemaMin <= req.PayloadSchema && health.ControlPlane.PayloadSchemaMax >= req.PayloadSchema) {
				return nil
			}
			lastErr = errors.New("deployer build identity does not match the activation target")
		} else if err != nil {
			lastErr = err
		} else {
			lastErr = errors.New("deployer health does not match the active application")
		}
	}
	return fmt.Errorf("new deployer failed health verification: %w", lastErr)
}

func (a *controlPlaneActivator) readHealth(ctx context.Context) (Health, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/v1/health", nil)
	if err != nil {
		return Health{}, err
	}
	response, err := a.healthClient.Do(req)
	if err != nil {
		return Health{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return Health{}, fmt.Errorf("health returned HTTP %d", response.StatusCode)
	}
	var health Health
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&health); err != nil {
		return Health{}, err
	}
	return health, nil
}

func (a *controlPlaneActivator) rollback(req controlPlaneActivationRequest, attempt int, backupPath, cause string) error {
	installedPath := a.paths.InstalledAssets[controlPlaneAssetDeployer]
	if err := a.installRegularFile(backupPath, installedPath, 0755); err != nil {
		message := cause + "; previous deployer could not be restored: " + err.Error()
		return errors.Join(errors.New(message), a.writeStatus(req, "rollback_failed", attempt, message, "permanent", nil))
	}
	if _, err := a.runner.Run(context.Background(), nil, a.systemctl, "restart", "sub2api-deployer.service"); err != nil {
		message := cause + "; restored deployer could not be restarted: " + err.Error()
		return errors.Join(errors.New(message), a.writeStatus(req, "rollback_failed", attempt, message, "permanent", nil))
	}
	if err := a.waitForHealth(context.Background(), req, false); err != nil {
		message := cause + "; restored deployer failed health verification: " + err.Error()
		return errors.Join(errors.New(message), a.writeStatus(req, "rollback_failed", attempt, message, "permanent", nil))
	}
	return a.failRequest(req, attempt, cause, "transient", true)
}

func (a *controlPlaneActivator) succeed(req controlPlaneActivationRequest, attempt int) error {
	if err := a.writeStatus(req, "succeeded", attempt, "", "", nil); err != nil {
		return err
	}
	return a.cleanupSuccessful(req)
}

func (a *controlPlaneActivator) skip(req controlPlaneActivationRequest, attempt int, message string) error {
	if err := a.writeStatus(req, "skipped", attempt, message, "", nil); err != nil {
		return err
	}
	return a.cleanupSuccessful(req)
}

func (a *controlPlaneActivator) cleanupSuccessful(req controlPlaneActivationRequest) error {
	if err := a.files.removeAll(filepath.Join(a.paths.StagingRoot, req.JobID)); err != nil {
		return err
	}
	if err := a.files.remove(a.paths.Request); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return a.files.syncDir(filepath.Dir(a.paths.Request))
}

func (a *controlPlaneActivator) failRequest(req controlPlaneActivationRequest, attempt int, message, class string, retryable bool) error {
	status := "failed"
	var next *time.Time
	if retryable && attempt < req.MaxAttempts {
		status = "retrying"
		value := a.now().UTC().Add(time.Minute)
		next = &value
	}
	if status == "failed" {
		staged, err := a.stageQuarantineRequest(req, message)
		if err != nil {
			return errors.Join(errors.New(message), err)
		}
		if err := a.writeStatus(req, status, attempt, message, class, next); err != nil {
			return errors.Join(errors.New(message), err)
		}
		if staged {
			if err := a.finalizeQuarantineRequest(); err != nil {
				return errors.Join(errors.New(message), err)
			}
		}
		return errors.New(message)
	}
	if err := a.writeStatus(req, status, attempt, message, class, next); err != nil {
		return errors.Join(errors.New(message), err)
	}
	return errors.New(message)
}

func (a *controlPlaneActivator) failMalformed(cause error) error {
	identity := controlPlaneActivationRequest{JobID: "unknown", ContainerID: "unknown", TargetVersion: "unknown", MaxAttempts: 1}
	if existing, err := readControlPlaneStatus(a.paths.Status); err == nil {
		identity.JobID = existing.JobID
		identity.ContainerID = existing.ContainerID
		identity.TargetVersion = existing.TargetVersion
	}
	staged, quarantineErr := a.stageQuarantineRequest(identity, cause.Error())
	if quarantineErr != nil {
		return errors.Join(cause, quarantineErr)
	}
	statusErr := a.writeStatus(identity, "failed", 1, cause.Error(), "permanent", nil)
	if statusErr != nil {
		return errors.Join(cause, statusErr)
	}
	if staged {
		quarantineErr = a.finalizeQuarantineRequest()
	}
	return errors.Join(cause, quarantineErr)
}

func (a *controlPlaneActivator) writeStatus(req controlPlaneActivationRequest, status string, attempt int, message, class string, next *time.Time) error {
	now := a.now().UTC()
	record := controlPlaneUpgradeStatus{
		Schema:        controlPlaneStatusSchema,
		JobID:         req.JobID,
		ContainerID:   req.ContainerID,
		TargetVersion: req.TargetVersion,
		Status:        status,
		Attempt:       attempt,
		MaxAttempts:   req.MaxAttempts,
		UpdatedAt:     &now,
		NextAttemptAt: next,
		Error:         strings.TrimSpace(message),
		LastError:     strings.TrimSpace(message),
		ErrorClass:    strings.TrimSpace(class),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return a.files.write(a.paths.Status, append(data, '\n'), 0600)
}

func (a *controlPlaneActivator) quarantineRequest(req controlPlaneActivationRequest, reason string) error {
	staged, err := a.stageQuarantineRequest(req, reason)
	if err != nil || !staged {
		return err
	}
	return a.finalizeQuarantineRequest()
}

func (a *controlPlaneActivator) stageQuarantineRequest(req controlPlaneActivationRequest, reason string) (bool, error) {
	if _, err := os.Lstat(a.paths.Request); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if err := ensureOwnedDirectory(a.paths.Quarantine, a.expectedUID, a.expectedGID, 0700); err != nil {
		return false, err
	}
	name := fmt.Sprintf("control-plane-upgrade.%s.%d.json", safeFilename(req.JobID), a.now().UTC().UnixNano())
	target := filepath.Join(a.paths.Quarantine, name)
	if err := a.copyRegularFile(a.paths.Request, target, 0600); err != nil {
		return false, err
	}
	audit := map[string]any{"job_id": req.JobID, "reason": strings.TrimSpace(reason), "quarantined_at": a.now().UTC()}
	data, _ := json.Marshal(audit)
	if err := a.files.write(target+".audit.json", append(data, '\n'), 0600); err != nil {
		return false, err
	}
	if err := a.files.syncDir(a.paths.Quarantine); err != nil {
		return false, err
	}
	return true, nil
}

func (a *controlPlaneActivator) finalizeQuarantineRequest() error {
	if err := a.files.remove(a.paths.Request); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return a.files.syncDir(filepath.Dir(a.paths.Request))
}

func readControlPlaneStatus(path string) (controlPlaneUpgradeStatus, error) {
	data, err := readRegularFileMode(path, os.Geteuid(), os.Getegid(), 0600)
	if err != nil {
		return controlPlaneUpgradeStatus{}, err
	}
	var status controlPlaneUpgradeStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return controlPlaneUpgradeStatus{}, err
	}
	return status, nil
}

func readActivationState(path string, uid, gid int) (State, error) {
	data, err := readRegularFileMode(path, uid, gid, 0600)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode state: %w", err)
	}
	return state, nil
}

func (s controlPlaneUpgradeStatus) matches(req controlPlaneActivationRequest) bool {
	return s.Schema == controlPlaneStatusSchema && s.JobID == req.JobID && s.ContainerID == req.ContainerID && s.TargetVersion == req.TargetVersion
}

func validatePublishedActivationStatus(req controlPlaneActivationRequest, status controlPlaneUpgradeStatus) error {
	if !status.matches(req) || status.MaxAttempts != req.MaxAttempts || status.UpdatedAt == nil || status.UpdatedAt.IsZero() {
		return errors.New("status identity or retry contract does not match the request")
	}
	if status.Attempt < 0 || status.Attempt > status.MaxAttempts {
		return errors.New("status attempt is outside the retry contract")
	}
	switch status.Status {
	case "pending":
		if status.Attempt != 0 || status.Error != "" || status.LastError != "" || status.ErrorClass != "" || status.NextAttemptAt != nil {
			return errors.New("pending status contains invalid retry or error state")
		}
	case "retrying":
		if status.Attempt < 1 || status.Attempt >= status.MaxAttempts || status.Error == "" || status.ErrorClass != "transient" || status.NextAttemptAt == nil {
			return errors.New("retrying status contains invalid retry state")
		}
	case "succeeded", "skipped":
		if status.Attempt < 1 || status.NextAttemptAt != nil {
			return errors.New("successful terminal status contains invalid retry state")
		}
	case "failed", "rollback_failed", "quarantined":
		if status.Error == "" || status.NextAttemptAt != nil {
			return errors.New("failed terminal status has no error evidence")
		}
	default:
		return fmt.Errorf("unsupported activation status %q", status.Status)
	}
	return nil
}

func acquireActivationLock(path string, uid, gid int) (*os.File, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, false, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, false, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, false, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || !ok || (uid >= 0 && int(stat.Uid) != uid) || (gid >= 0 && int(stat.Gid) != gid) {
		_ = file.Close()
		return nil, false, errors.New("activation lock file is unsafe")
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		if errors.Is(err, ErrProcessLocked) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return file, true, nil
}

func readRegularFile(path string, uid, gid int) ([]byte, error) {
	file, _, err := openRegularFile(path, uid, gid)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(file)
}

func readRegularFileMode(path string, uid, gid int, expectedMode os.FileMode) ([]byte, error) {
	file, info, err := openRegularFile(path, uid, gid)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if info.Mode().Perm() != expectedMode.Perm() {
		return nil, fmt.Errorf("unexpected mode %04o", info.Mode().Perm())
	}
	return io.ReadAll(file)
}

func openRegularFile(path string, uid, gid int) (*os.File, os.FileInfo, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, errors.New("not a regular file")
	}
	if uid >= 0 || gid >= 0 {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			_ = file.Close()
			return nil, nil, errors.New("file ownership metadata is unavailable")
		}
		if uid >= 0 && int(stat.Uid) != uid {
			_ = file.Close()
			return nil, nil, fmt.Errorf("file owner is not uid %d", uid)
		}
		if gid >= 0 && int(stat.Gid) != gid {
			_ = file.Close()
			return nil, nil, fmt.Errorf("file group is not gid %d", gid)
		}
	}
	return file, info, nil
}

func validateOwnedDirectory(path string, uid, gid int, mode os.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != mode {
		return fmt.Errorf("not a directory with mode %04o", mode)
	}
	if uid >= 0 || gid >= 0 {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return errors.New("directory ownership metadata is unavailable")
		}
		if uid >= 0 && int(stat.Uid) != uid {
			return fmt.Errorf("directory owner is not uid %d", uid)
		}
		if gid >= 0 && int(stat.Gid) != gid {
			return fmt.Errorf("directory group is not gid %d", gid)
		}
	}
	return nil
}

func ensureOwnedDirectory(path string, uid, gid int, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	return validateOwnedDirectory(path, uid, gid, mode)
}

func digestRegularFile(path string, uid, gid int, expectedMode os.FileMode) (string, error) {
	file, info, err := openRegularFile(path, uid, gid)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	if info.Mode().Perm() != expectedMode.Perm() {
		return "", fmt.Errorf("unexpected mode %04o", info.Mode().Perm())
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func (a *controlPlaneActivator) copyRegularFile(source, destination string, mode os.FileMode) error {
	data, err := readRegularFile(source, a.expectedUID, a.expectedGID)
	if err != nil {
		return err
	}
	return a.files.write(destination, data, mode)
}

func (a *controlPlaneActivator) installRegularFile(source, destination string, mode os.FileMode) error {
	data, err := readRegularFile(source, a.expectedUID, a.expectedGID)
	if err != nil {
		return err
	}
	return a.files.write(destination, data, mode)
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}

func safeFilename(value string) string {
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("._-", char) {
			_, _ = builder.WriteRune(char)
		} else {
			_ = builder.WriteByte('_')
		}
	}
	return builder.String()
}

func controlPlaneStatusJSON(cfg Config) ([]byte, error) {
	paths := defaultControlPlaneActivationPaths(cfg)
	result := map[string]any{"request_present": false, "status_present": false, "recommended_action": "none"}
	if data, err := readRegularFileMode(paths.Request, os.Geteuid(), os.Getegid(), 0600); err == nil {
		var request any
		if err := json.Unmarshal(data, &request); err != nil {
			result["request_error"] = err.Error()
		} else {
			result["request_present"] = true
			result["request"] = request
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if data, err := readRegularFileMode(paths.Status, os.Geteuid(), os.Getegid(), 0600); err == nil {
		var status any
		if err := json.Unmarshal(data, &status); err != nil {
			result["status_error"] = err.Error()
		} else {
			result["status_present"] = true
			result["status"] = status
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if state, err := readActivationState(paths.State, os.Geteuid(), os.Getegid()); err == nil {
		result["active_container"] = state.ActiveContainer
		result["active_container_id"] = state.ActiveContainerID
		result["active_version"] = state.ActiveVersion
		result["degraded"] = state.Degraded
		result["job_running"] = state.Job != nil && state.Job.Status == JobStatusRunning
	} else {
		result["state_error"] = err.Error()
	}
	if present, _ := result["request_present"].(bool); present {
		result["recommended_action"] = "wait for the timer, or retry/quarantine the exact job after reviewing status and application health"
	} else if present, _ := result["status_present"].(bool); present {
		result["recommended_action"] = "no pending request; retain the terminal status as audit evidence"
	}
	return json.MarshalIndent(result, "", "  ")
}

func ControlPlaneStatus(cfg Config) ([]byte, error) {
	if err := requireRoot(); err != nil {
		return nil, err
	}
	return controlPlaneStatusJSON(cfg)
}

func RetryControlPlaneUpgrade(ctx context.Context, cfg Config, runner CommandRunner, jobID string) error {
	if err := requireRoot(); err != nil {
		return err
	}
	return retryControlPlaneUpgrade(ctx, newRuntimeControlPlaneActivator(cfg, runner), jobID)
}

func retryControlPlaneUpgrade(ctx context.Context, activator *controlPlaneActivator, jobID string) error {
	jobID = strings.TrimSpace(jobID)
	if !requestIDPattern.MatchString(jobID) {
		return errors.New("retry requires a valid job id")
	}
	if err := activator.requireSafeSystemctl(); err != nil {
		return err
	}
	paths := activator.paths
	if err := validateOwnedDirectory(filepath.Dir(paths.Request), activator.expectedUID, activator.expectedGID, 0700); err != nil {
		return fmt.Errorf("activation state directory is unsafe: %w", err)
	}
	lock, acquired, err := acquireActivationLock(paths.Lock, activator.expectedUID, activator.expectedGID)
	if err != nil {
		return err
	}
	if !acquired {
		return errors.New("control-plane activation is already in progress")
	}
	defer func() { _ = lock.Close() }()
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.JobID != jobID || status.ErrorClass != "transient" || (status.Status != "failed" && status.Status != "retrying") {
		return errors.New("control-plane status is not a retryable error for the requested job")
	}
	restoredFrom := ""
	if _, err := os.Lstat(paths.Request); errors.Is(err, os.ErrNotExist) {
		if info, quarantineErr := os.Lstat(paths.Quarantine); quarantineErr == nil {
			if !info.IsDir() || validateOwnedDirectory(paths.Quarantine, activator.expectedUID, activator.expectedGID, 0700) != nil {
				return errors.New("control-plane quarantine directory is unsafe")
			}
		} else if !errors.Is(quarantineErr, os.ErrNotExist) {
			return fmt.Errorf("inspect control-plane quarantine directory: %w", quarantineErr)
		}
		entries, readErr := os.ReadDir(paths.Quarantine)
		if readErr != nil {
			return fmt.Errorf("find quarantined activation request: %w", readErr)
		}
		var matches []string
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".audit.json") {
				continue
			}
			path := filepath.Join(paths.Quarantine, entry.Name())
			data, fileErr := readRegularFileMode(path, activator.expectedUID, activator.expectedGID, 0600)
			if fileErr != nil {
				continue
			}
			var candidate controlPlaneActivationRequest
			if json.Unmarshal(data, &candidate) == nil && candidate.JobID == jobID {
				matches = append(matches, path)
			}
		}
		if len(matches) != 1 {
			return fmt.Errorf("expected exactly one quarantined request for job %q, found %d", jobID, len(matches))
		}
		if err := activator.files.rename(matches[0], paths.Request); err != nil {
			return fmt.Errorf("restore quarantined activation request: %w", err)
		}
		restoredFrom = matches[0]
	} else if err != nil {
		return err
	}
	req, err := activator.readRequest()
	if err != nil || req.JobID != jobID {
		if restoredFrom != "" {
			_ = activator.files.rename(paths.Request, restoredFrom)
		}
		return errors.New("pending activation request does not match the requested job")
	}
	state, err := readActivationState(paths.State, activator.expectedUID, activator.expectedGID)
	if err != nil || state.Degraded || state.Job == nil || state.Job.ID != jobID || state.Job.Status != JobStatusSucceeded {
		if restoredFrom != "" {
			_ = activator.files.rename(paths.Request, restoredFrom)
		}
		return errors.New("deployment state does not permit activation retry")
	}
	health, err := activator.readHealth(ctx)
	if err != nil || health.Status != "ok" || health.Degraded || health.JobRunning || health.ActiveContainerID != req.ContainerID || health.ActiveVersion != req.TargetVersion {
		if restoredFrom != "" {
			_ = activator.files.rename(paths.Request, restoredFrom)
		}
		return errors.New("live deployer health does not permit activation retry")
	}
	if err := activator.writeStatus(req, "pending", 0, "", "", nil); err != nil {
		return err
	}
	return activator.activateLocked(ctx)
}

func (a *controlPlaneActivator) requireSafeSystemctl() error {
	if a.systemctl == "" {
		return errors.New("control-plane activation systemctl command is not safely configured")
	}
	return nil
}

func QuarantineControlPlaneUpgrade(cfg Config, jobID, reason string) error {
	if err := requireRoot(); err != nil {
		return err
	}
	return quarantineControlPlaneUpgrade(newRuntimeControlPlaneActivator(cfg, ExecRunner{}), jobID, reason)
}

func quarantineControlPlaneUpgrade(activator *controlPlaneActivator, jobID, reason string) error {
	jobID = strings.TrimSpace(jobID)
	reason = strings.TrimSpace(reason)
	if !requestIDPattern.MatchString(jobID) || reason == "" {
		return errors.New("quarantine requires a valid job id and non-empty reason")
	}
	if err := validateOwnedDirectory(filepath.Dir(activator.paths.Request), activator.expectedUID, activator.expectedGID, 0700); err != nil {
		return fmt.Errorf("activation state directory is unsafe: %w", err)
	}
	lock, acquired, err := acquireActivationLock(activator.paths.Lock, activator.expectedUID, activator.expectedGID)
	if err != nil {
		return err
	}
	if !acquired {
		return errors.New("control-plane activation is already in progress")
	}
	defer func() { _ = lock.Close() }()
	req, err := activator.readRequest()
	requestAlreadyQuarantined := false
	requestMissing := false
	statusReconstructed := false
	quarantinedPath := ""
	if errors.Is(err, os.ErrNotExist) {
		if info, quarantineErr := os.Lstat(activator.paths.Quarantine); quarantineErr == nil {
			if !info.IsDir() || validateOwnedDirectory(activator.paths.Quarantine, activator.expectedUID, activator.expectedGID, 0700) != nil {
				return errors.New("control-plane quarantine directory is unsafe")
			}
		} else if !errors.Is(quarantineErr, os.ErrNotExist) {
			return fmt.Errorf("inspect control-plane quarantine directory: %w", quarantineErr)
		}
		entries, readErr := os.ReadDir(activator.paths.Quarantine)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return errors.New("no pending or quarantined activation request matches the requested job")
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".audit.json") {
				continue
			}
			path := filepath.Join(activator.paths.Quarantine, entry.Name())
			data, fileErr := readRegularFileMode(path, activator.expectedUID, activator.expectedGID, 0600)
			var candidate controlPlaneActivationRequest
			if fileErr == nil && json.Unmarshal(data, &candidate) == nil && candidate.JobID == jobID {
				if quarantinedPath != "" {
					return errors.New("multiple quarantined activation requests match the requested job")
				}
				req = candidate
				quarantinedPath = path
			}
		}
		requestAlreadyQuarantined = quarantinedPath != ""
		if !requestAlreadyQuarantined {
			status, statusErr := readControlPlaneStatus(activator.paths.Status)
			state, stateErr := readActivationState(activator.paths.State, activator.expectedUID, activator.expectedGID)
			if stateErr != nil || state.Job == nil || state.Job.ID != jobID {
				return errors.New("no pending or quarantined activation request matches the requested job")
			}
			if statusErr != nil {
				if _, secureReadErr := readRegularFileMode(activator.paths.Status, activator.expectedUID, activator.expectedGID, 0600); secureReadErr != nil {
					return errors.New("control-plane status is missing or unsafe; use the Bundle Installer break-glass procedure")
				}
				status = controlPlaneUpgradeStatus{
					Schema:        controlPlaneStatusSchema,
					JobID:         jobID,
					ContainerID:   state.ActiveContainerID,
					TargetVersion: state.ActiveVersion,
					MaxAttempts:   defaultActivationAttempts,
				}
				statusReconstructed = true
			} else if status.JobID != jobID {
				return errors.New("no pending or quarantined activation request matches the requested job")
			}
			req = controlPlaneActivationRequest{
				JobID:         status.JobID,
				ContainerID:   status.ContainerID,
				ContainerName: state.ActiveContainer,
				TargetVersion: status.TargetVersion,
				ExpectedImage: state.ActiveImage,
				MaxAttempts:   status.MaxAttempts,
				CreatedAt:     state.UpdatedAt,
			}
			requestMissing = true
		}
	} else if err != nil {
		return err
	}
	if req.JobID != jobID {
		return errors.New("pending or quarantined activation request does not match the requested job")
	}
	state, err := readActivationState(activator.paths.State, activator.expectedUID, activator.expectedGID)
	if err != nil || state.Degraded || state.Job == nil || state.Job.ID != jobID || state.Job.Status == JobStatusRunning || state.ActiveContainerID != req.ContainerID || state.ActiveVersion != req.TargetVersion {
		return errors.New("deployment state does not permit request quarantine")
	}
	health, err := activator.readHealth(context.Background())
	if err != nil || health.Status != "ok" || health.Degraded || health.JobRunning || health.ActiveContainerID != req.ContainerID || health.ActiveVersion != req.TargetVersion {
		return errors.New("live deployer health does not permit request quarantine")
	}
	if requestAlreadyQuarantined {
		audit := map[string]any{"job_id": req.JobID, "reason": reason, "accepted_at": activator.now().UTC()}
		data, _ := json.Marshal(audit)
		if err := activator.files.write(quarantinedPath+".operator.json", append(data, '\n'), 0600); err != nil {
			return err
		}
		return activator.writeStatus(req, "quarantined", 0, reason, "operator", nil)
	}
	if requestMissing {
		if err := ensureOwnedDirectory(activator.paths.Quarantine, activator.expectedUID, activator.expectedGID, 0700); err != nil {
			return err
		}
		audit := map[string]any{"job_id": req.JobID, "reason": reason, "accepted_at": activator.now().UTC(), "request_missing": true, "status_reconstructed": statusReconstructed}
		data, _ := json.Marshal(audit)
		if err := activator.files.write(filepath.Join(activator.paths.Quarantine, "control-plane-upgrade."+safeFilename(req.JobID)+".operator.json"), append(data, '\n'), 0600); err != nil {
			return err
		}
		return activator.writeStatus(req, "quarantined", 0, reason, "operator", nil)
	}
	staged, err := activator.stageQuarantineRequest(req, reason)
	if err != nil {
		return err
	}
	if err := activator.writeStatus(req, "quarantined", 0, reason, "operator", nil); err != nil {
		return err
	}
	if staged {
		return activator.finalizeQuarantineRequest()
	}
	return nil
}

func newRuntimeControlPlaneActivator(cfg Config, runner CommandRunner) *controlPlaneActivator {
	systemctl := ""
	if validControlPlaneUpgradeCommand(cfg.ControlPlaneUpgradeCommand) {
		systemctl = filepath.Clean(cfg.ControlPlaneUpgradeCommand[0])
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", cfg.SocketPath)
		},
	}
	return &controlPlaneActivator{
		cfg:          cfg,
		runner:       runner,
		systemctl:    systemctl,
		paths:        defaultControlPlaneActivationPaths(cfg),
		now:          time.Now,
		sleep:        time.Sleep,
		healthClient: &http.Client{Transport: transport, Timeout: 2 * time.Second},
		expectedUID:  0,
		expectedGID:  0,
		files:        defaultControlPlaneFileOps(),
	}
}

func requireRoot() error {
	if os.Geteuid() != 0 {
		return errors.New("control-plane recovery commands require root")
	}
	return nil
}

func CurrentExecutableSHA256() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
