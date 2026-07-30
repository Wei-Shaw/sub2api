package deployer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type activationRunner struct {
	mu             sync.Mutex
	restarts       int
	restartErrors  []error
	currentVersion string
	systemctlPath  string
}

func (r *activationRunner) Run(_ context.Context, _ map[string]string, name string, args ...string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	systemctlPath := r.systemctlPath
	if systemctlPath == "" {
		systemctlPath = "/bin/systemctl"
	}
	if name == systemctlPath && strings.Join(args, " ") == "restart sub2api-deployer.service" {
		r.restarts++
		if r.restarts <= len(r.restartErrors) && r.restartErrors[r.restarts-1] != nil {
			return "", r.restartErrors[r.restarts-1]
		}
		return "", nil
	}
	if len(args) == 1 && args[0] == "--version" && strings.HasSuffix(name, "sub2api-deployer") {
		version := r.currentVersion
		if version == "" {
			version = "0.1.168-ts.2"
		}
		return "Sub2API Deployer " + version + " (commit: " + strings.Repeat("a", 40) + ", built: 2026-07-30T00:00:00Z, type: release, arch: " + runtime.GOARCH + ")", nil
	}
	return "", nil
}

func (r *activationRunner) restartCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.restarts
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestControlPlaneActivatorNoRequestIsSilentNoOp(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return Health{} })

	if err := activator.activate(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{paths.Lock, paths.Status, paths.Quarantine} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("no-request activation created %s", path)
		}
	}
}

func TestControlPlaneActivatorUsesConfiguredUsrBinSystemctl(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	runner := &activationRunner{systemctlPath: "/usr/bin/systemctl"}
	req := writeActivationFixture(t, paths, []byte("new-binary"))
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, runner, func() Health { return targetActivationHealth(req) })
	activator.systemctl = "/usr/bin/systemctl"

	if err := activator.activate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runner.restartCount() != 1 {
		t.Fatalf("expected one restart through configured /usr/bin/systemctl, got %d", runner.restartCount())
	}
}

func TestControlPlaneActivatorIdenticalPayloadDoesNotRestart(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	runner := &activationRunner{}
	req := writeActivationFixture(t, paths, []byte("same-binary"))
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("same-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, runner, func() Health { return targetActivationHealth(req) })

	if err := activator.activate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runner.restartCount() != 0 {
		t.Fatalf("identical payload restarted deployer %d times", runner.restartCount())
	}
	if _, err := os.Lstat(paths.Request); !os.IsNotExist(err) {
		t.Fatal("successful activation did not remove request")
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "succeeded" || status.Attempt != 1 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestControlPlaneActivatorSkipsControlPlaneDowngrade(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	runner := &activationRunner{currentVersion: "0.1.168-ts.4"}
	req := writeActivationFixture(t, paths, []byte("older-binary"))
	installed := []byte("newer-binary")
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], installed, 0755); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, runner, func() Health { return targetActivationHealth(req) })

	if err := activator.activate(context.Background()); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(paths.InstalledAssets[controlPlaneAssetDeployer])
	if err != nil || string(actual) != string(installed) {
		t.Fatalf("downgrade changed installed binary: %q err=%v", actual, err)
	}
	if runner.restartCount() != 0 {
		t.Fatalf("downgrade restarted deployer %d times", runner.restartCount())
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "skipped" || !strings.Contains(status.Error, "not newer") {
		t.Fatalf("skip status=%+v err=%v", status, err)
	}
}

func TestControlPlaneActivatorRejectsMissingStatusTransactionEvidence(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	runner := &activationRunner{}
	req := writeActivationFixture(t, paths, []byte("new-binary"))
	installed := []byte("old-binary")
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], installed, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(paths.Status); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, runner, func() Health { return targetActivationHealth(req) })

	if err := activator.activate(context.Background()); err == nil || !strings.Contains(err.Error(), "status is missing") {
		t.Fatalf("missing status error=%v", err)
	}
	actual, err := os.ReadFile(paths.InstalledAssets[controlPlaneAssetDeployer])
	if err != nil || string(actual) != string(installed) {
		t.Fatalf("missing status changed installed binary: %q err=%v", actual, err)
	}
	if runner.restartCount() != 0 {
		t.Fatalf("missing status restarted deployer %d times", runner.restartCount())
	}
	if _, err := os.Lstat(paths.Request); !os.IsNotExist(err) {
		t.Fatalf("missing-status request was not quarantined: %v", err)
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "failed" || status.ErrorClass != "permanent" {
		t.Fatalf("missing-status terminal record=%+v err=%v", status, err)
	}
}

func TestControlPlaneActivatorRejectsMismatchedStatusRetryContract(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("new-binary"))
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil {
		t.Fatal(err)
	}
	status.MaxAttempts = req.MaxAttempts - 1
	data, _ := json.Marshal(status)
	if err := os.WriteFile(paths.Status, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return targetActivationHealth(req) })

	if err := activator.activate(context.Background()); err == nil || !strings.Contains(err.Error(), "transaction evidence") {
		t.Fatalf("mismatched status error=%v", err)
	}
	if _, err := os.Lstat(paths.Request); !os.IsNotExist(err) {
		t.Fatalf("mismatched-status request was not quarantined: %v", err)
	}
}

func TestControlPlaneActivatorRestoresOldBinaryAfterHealthFailure(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	runner := &activationRunner{}
	req := writeActivationFixture(t, paths, []byte("new-binary"))
	oldBinary := []byte("old-binary")
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], oldBinary, 0755); err != nil {
		t.Fatal(err)
	}
	checks := 0
	activator := newTestActivator(paths, runner, func() Health {
		checks++
		health := targetActivationHealth(req)
		if checks <= 40 {
			health.Build.Version = "wrong"
		}
		return health
	})

	if err := activator.activate(context.Background()); err == nil {
		t.Fatal("health mismatch unexpectedly succeeded")
	}
	installed, err := os.ReadFile(paths.InstalledAssets[controlPlaneAssetDeployer])
	if err != nil || string(installed) != string(oldBinary) {
		t.Fatalf("installed binary=%q err=%v", installed, err)
	}
	if runner.restartCount() != 2 {
		t.Fatalf("restart count=%d, want replacement and rollback", runner.restartCount())
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "retrying" || status.Attempt != 1 || status.MaxAttempts != defaultActivationAttempts {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if _, err := os.Stat(paths.Request); err != nil {
		t.Fatalf("retryable request was removed: %v", err)
	}
}

func TestControlPlaneActivatorResumesAfterCrashBetweenRenameAndRestart(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	runner := &activationRunner{}
	target := []byte("new-binary")
	req := writeActivationFixture(t, paths, target)
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], target, 0755); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(paths.StagingRoot, req.JobID, ".previous-"+controlPlaneAssetDeployer)
	if err := os.WriteFile(backup, []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	checks := 0
	activator := newTestActivator(paths, runner, func() Health {
		checks++
		health := targetActivationHealth(req)
		if checks <= 40 {
			health.Build.Version = "old"
		}
		return health
	})

	if err := activator.activate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runner.restartCount() != 1 {
		t.Fatalf("restart count=%d, want one resumed restart", runner.restartCount())
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "succeeded" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestControlPlaneActivatorMalformedRequestUsesPersistedIdentityAndQuarantines(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	if err := os.MkdirAll(filepath.Dir(paths.Request), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Request, []byte("{not-json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	persisted := controlPlaneUpgradeStatus{Schema: 1, JobID: "job-malformed-0001", ContainerID: strings.Repeat("a", 64), TargetVersion: "0.1.168-ts.3", Status: "pending", MaxAttempts: 5, UpdatedAt: &now}
	data, _ := json.Marshal(persisted)
	if err := os.WriteFile(paths.Status, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return Health{} })

	if err := activator.activate(context.Background()); err == nil {
		t.Fatal("malformed request unexpectedly succeeded")
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "failed" || status.JobID != persisted.JobID || status.ErrorClass != "permanent" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if _, err := os.Lstat(paths.Request); !os.IsNotExist(err) {
		t.Fatal("malformed request was not quarantined")
	}
	entries, err := os.ReadDir(paths.Quarantine)
	if err != nil || len(entries) != 2 {
		t.Fatalf("quarantine entries=%d err=%v", len(entries), err)
	}
}

func TestControlPlaneActivatorRejectsUnsafeJobIDBeforeTerminalCleanup(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("target"))
	marker := filepath.Join(dir, "must-not-be-removed")
	if err := os.Mkdir(marker, 0700); err != nil {
		t.Fatal(err)
	}
	req.JobID = "../../must-not-be-removed"
	writeActivationRequest(t, paths.Request, req)
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return Health{} })
	if err := activator.writeStatus(req, "succeeded", 1, "", "", nil); err != nil {
		t.Fatal(err)
	}

	if err := activator.activate(context.Background()); err == nil || !strings.Contains(err.Error(), "job id is invalid") {
		t.Fatalf("unsafe job id error=%v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("unsafe terminal cleanup removed marker: %v", err)
	}
}

func TestControlPlaneActivatorSkipsWhenActivationLockIsHeld(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	writeActivationFixture(t, paths, []byte("target"))
	lock, acquired, err := acquireActivationLock(paths.Lock, os.Geteuid(), os.Getegid())
	if err != nil || !acquired {
		t.Fatalf("acquire first lock: acquired=%v err=%v", acquired, err)
	}
	defer func() { _ = lock.Close() }()
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return Health{} })
	if err := activator.activate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.Request); err != nil {
		t.Fatalf("contended activation changed request: %v", err)
	}
}

func TestControlPlaneActivatorRejectsSymlinkedActivationLock(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	writeActivationFixture(t, paths, []byte("target"))
	outside := filepath.Join(dir, "outside-lock")
	if err := os.WriteFile(outside, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, paths.Lock); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return Health{} })

	if err := activator.activate(context.Background()); err == nil {
		t.Fatal("symlinked activation lock unexpectedly succeeded")
	}
	if _, err := os.Stat(paths.Request); err != nil {
		t.Fatalf("unsafe lock changed activation request: %v", err)
	}
}

func TestReadRegularFileRejectsWrongGroup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "root-asset")
	if err := os.WriteFile(path, []byte("asset"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := readRegularFile(path, os.Geteuid(), os.Getegid()+1)
	if err == nil || !strings.Contains(err.Error(), "group") {
		t.Fatalf("wrong group error=%v", err)
	}
}

func TestControlPlaneActivatorRejectsOverexposedRequestMode(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	writeActivationFixture(t, paths, []byte("target"))
	if err := os.Chmod(paths.Request, 0644); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return Health{} })

	if err := activator.activate(context.Background()); err == nil || !strings.Contains(err.Error(), "unexpected mode 0644") {
		t.Fatalf("overexposed request mode error=%v", err)
	}
	if _, err := os.Lstat(paths.Request); !os.IsNotExist(err) {
		t.Fatal("unsafe request was not quarantined")
	}
}

func TestReadControlPlaneStatusRejectsOverexposedMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control-plane.status")
	if err := os.WriteFile(path, []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := readControlPlaneStatus(path)
	if err == nil || !strings.Contains(err.Error(), "unexpected mode 0644") {
		t.Fatalf("overexposed status mode error=%v", err)
	}
}

func TestControlPlaneActivatorRejectsSymlinkedStagedBinary(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("target"))
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside-target")
	if err := os.WriteFile(outside, []byte("target"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(req.Assets[0].StagedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, req.Assets[0].StagedPath); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return Health{} })

	if err := activator.activate(context.Background()); err == nil {
		t.Fatal("symlinked staged binary unexpectedly succeeded")
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "failed" || status.ErrorClass != "permanent" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestControlPlaneActivatorRejectsSymlinkedStagingRoot(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	writeActivationFixture(t, paths, []byte("target"))
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	relocated := filepath.Join(dir, "relocated-staging")
	if err := os.Rename(paths.StagingRoot, relocated); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(relocated, paths.StagingRoot); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return Health{} })

	if err := activator.activate(context.Background()); err == nil || !strings.Contains(err.Error(), "staging root is unsafe") {
		t.Fatalf("symlinked staging root error=%v", err)
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "failed" || status.ErrorClass != "permanent" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestControlPlaneActivatorRestartFailureRollsBackAndRetries(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	runner := &activationRunner{restartErrors: []error{errors.New("restart failed"), nil}}
	req := writeActivationFixture(t, paths, []byte("new-binary"))
	oldBinary := []byte("old-binary")
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], oldBinary, 0755); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, runner, func() Health { return targetActivationHealth(req) })

	if err := activator.activate(context.Background()); err == nil {
		t.Fatal("restart failure unexpectedly succeeded")
	}
	installed, err := os.ReadFile(paths.InstalledAssets[controlPlaneAssetDeployer])
	if err != nil || string(installed) != string(oldBinary) {
		t.Fatalf("installed binary=%q err=%v", installed, err)
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "retrying" || status.Attempt != 1 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if runner.restartCount() != 2 {
		t.Fatalf("restart count=%d, want failed replacement plus rollback", runner.restartCount())
	}
}

func TestControlPlaneActivatorRollbackRestartFailureIsTerminal(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	runner := &activationRunner{restartErrors: []error{errors.New("new restart failed"), errors.New("rollback restart failed")}}
	writeActivationFixture(t, paths, []byte("new-binary"))
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, runner, func() Health { return Health{} })

	if err := activator.activate(context.Background()); err == nil {
		t.Fatal("rollback restart failure unexpectedly succeeded")
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "rollback_failed" || status.ErrorClass != "permanent" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestControlPlaneActivatorRollbackHealthFailureIsTerminal(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	runner := &activationRunner{}
	writeActivationFixture(t, paths, []byte("new-binary"))
	oldBinary := []byte("old-binary")
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], oldBinary, 0755); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, runner, func() Health { return Health{} })

	if err := activator.activate(context.Background()); err == nil {
		t.Fatal("rollback health failure unexpectedly succeeded")
	}
	installed, err := os.ReadFile(paths.InstalledAssets[controlPlaneAssetDeployer])
	if err != nil || string(installed) != string(oldBinary) {
		t.Fatalf("installed binary=%q err=%v", installed, err)
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "rollback_failed" || status.ErrorClass != "permanent" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if runner.restartCount() != 2 {
		t.Fatalf("restart count=%d, want replacement and rollback", runner.restartCount())
	}
}

func TestControlPlaneActivatorBackupWriteFailureIsPermanentWithoutReplacement(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("new-binary"))
	oldBinary := []byte("old-binary")
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], oldBinary, 0755); err != nil {
		t.Fatal(err)
	}
	runner := &activationRunner{}
	activator := newTestActivator(paths, runner, func() Health { return targetActivationHealth(req) })
	write := activator.files.write
	backupPath := filepath.Join(paths.StagingRoot, req.JobID, ".previous-"+controlPlaneAssetDeployer)
	activator.files.write = func(path string, data []byte, mode os.FileMode) error {
		if path == backupPath {
			return errors.New("injected backup write failure")
		}
		return write(path, data, mode)
	}

	if err := activator.activate(context.Background()); err == nil || !strings.Contains(err.Error(), "backup write failure") {
		t.Fatalf("backup write error=%v", err)
	}
	installed, err := os.ReadFile(paths.InstalledAssets[controlPlaneAssetDeployer])
	if err != nil || string(installed) != string(oldBinary) {
		t.Fatalf("installed binary=%q err=%v", installed, err)
	}
	if runner.restartCount() != 0 {
		t.Fatalf("backup failure restarted deployer %d times", runner.restartCount())
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "failed" || status.ErrorClass != "permanent" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestControlPlaneActivatorReplacementWriteFailureRetriesAndConverges(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("new-binary"))
	installedPath := paths.InstalledAssets[controlPlaneAssetDeployer]
	if err := os.WriteFile(installedPath, []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	runner := &activationRunner{}
	activator := newTestActivator(paths, runner, func() Health { return targetActivationHealth(req) })
	write := activator.files.write
	failed := false
	activator.files.write = func(path string, data []byte, mode os.FileMode) error {
		if path == installedPath && !failed {
			failed = true
			return errors.New("injected replacement write failure")
		}
		return write(path, data, mode)
	}

	if err := activator.activate(context.Background()); err == nil || !strings.Contains(err.Error(), "replacement write failure") {
		t.Fatalf("replacement write error=%v", err)
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "retrying" || status.Attempt != 1 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	activator.files.write = write
	if err := activator.activate(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err = readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "succeeded" || status.Attempt != 2 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if runner.restartCount() != 1 {
		t.Fatalf("replacement retry restart count=%d", runner.restartCount())
	}
}

func TestControlPlaneActivatorStatusWriteFailureConvergesWithoutSecondRestart(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("new-binary"))
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	runner := &activationRunner{}
	activator := newTestActivator(paths, runner, func() Health { return targetActivationHealth(req) })
	write := activator.files.write
	failed := false
	activator.files.write = func(path string, data []byte, mode os.FileMode) error {
		if path == paths.Status && !failed {
			failed = true
			return errors.New("injected status write failure")
		}
		return write(path, data, mode)
	}

	if err := activator.activate(context.Background()); err == nil || !strings.Contains(err.Error(), "status write failure") {
		t.Fatalf("status write error=%v", err)
	}
	if _, err := os.Stat(paths.Request); err != nil {
		t.Fatalf("status failure removed request: %v", err)
	}
	activator.files.write = write
	if err := activator.activate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runner.restartCount() != 1 {
		t.Fatalf("status recovery restarted deployer %d times", runner.restartCount())
	}
}

func TestControlPlaneActivatorQuarantineCommitFailureConverges(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("new-binary"))
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	req.MaxAttempts = 1
	writeActivationRequest(t, paths.Request, req)
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return Health{} })
	if err := activator.writeStatus(req, "failed", 1, "previous terminal failure", "permanent", nil); err != nil {
		t.Fatal(err)
	}
	remove := activator.files.remove
	activator.files.remove = func(path string) error {
		if path == paths.Request {
			return errors.New("injected quarantine commit failure")
		}
		return remove(path)
	}

	if err := activator.activate(context.Background()); err == nil || !strings.Contains(err.Error(), "quarantine commit failure") {
		t.Fatalf("quarantine commit error=%v", err)
	}
	if _, err := os.Stat(paths.Request); err != nil {
		t.Fatalf("commit failure removed request: %v", err)
	}
	activator.files.remove = remove
	if err := activator.activate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(paths.Request); !os.IsNotExist(err) {
		t.Fatal("terminal request did not converge into quarantine")
	}
}

func TestControlPlaneActivatorCleanupFsyncFailureLeavesSuccessfulTerminalState(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("same-binary"))
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("same-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return targetActivationHealth(req) })
	syncDir := activator.files.syncDir
	activator.files.syncDir = func(string) error { return errors.New("injected directory fsync failure") }

	if err := activator.activate(context.Background()); err == nil || !strings.Contains(err.Error(), "fsync failure") {
		t.Fatalf("cleanup fsync error=%v", err)
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "succeeded" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	activator.files.syncDir = syncDir
	if err := activator.activate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestControlPlaneActivatorExhaustsBoundedRetryBudget(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("new-binary"))
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	healthChecks := 0
	activator := newTestActivator(paths, &activationRunner{}, func() Health {
		healthChecks++
		if healthChecks <= 40 {
			return Health{}
		}
		return targetActivationHealth(req)
	})
	next := time.Now().UTC().Add(time.Minute)
	if err := activator.writeStatus(req, "retrying", req.MaxAttempts-1, "previous failure", "transient", &next); err != nil {
		t.Fatal(err)
	}

	if err := activator.activate(context.Background()); err == nil || !strings.Contains(err.Error(), "health verification") {
		t.Fatalf("retry exhaustion error=%v", err)
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "failed" || status.Attempt != req.MaxAttempts {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if _, err := os.Lstat(paths.Request); !os.IsNotExist(err) {
		t.Fatal("exhausted request was not quarantined")
	}
}

func TestControlPlaneActivatorRejectsStatusFromDifferentJob(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("same-binary"))
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("same-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return targetActivationHealth(req) })
	stale := req
	stale.JobID = "stale-activation-job"
	next := time.Now().UTC().Add(time.Minute)
	if err := activator.writeStatus(stale, "retrying", 4, "stale failure", "transient", &next); err != nil {
		t.Fatal(err)
	}

	if err := activator.activate(context.Background()); err == nil || !strings.Contains(err.Error(), "transaction evidence") {
		t.Fatalf("stale status error=%v", err)
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "failed" || status.Attempt != 1 || status.JobID != req.JobID {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestControlPlaneActivatorRejectsUnsupportedPayloadSchema(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("new-binary"))
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	req.PayloadSchema++
	writeActivationRequest(t, paths.Request, req)
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return Health{} })

	if err := activator.activate(context.Background()); err == nil || !strings.Contains(err.Error(), "unsupported activation request schema") {
		t.Fatalf("unsupported schema error=%v", err)
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "failed" || status.ErrorClass != "permanent" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestControlPlaneActivatorAllowsUnknownRequestFields(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("same-binary"))
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("same-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(paths.Request)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	object["future_optional_field"] = "ignored"
	data, _ = json.Marshal(object)
	if err := os.WriteFile(paths.Request, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return targetActivationHealth(req) })

	if err := activator.activate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestControlPlaneOperatorQuarantinesFailedStatusWithoutRequest(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("target"))
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return targetActivationHealth(req) })
	if err := activator.writeStatus(req, "failed", 1, "staging failed", "permanent", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(paths.Request); err != nil {
		t.Fatal(err)
	}

	if err := quarantineControlPlaneUpgrade(activator, req.JobID, "operator verified application health"); err != nil {
		t.Fatal(err)
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "quarantined" || status.ErrorClass != "operator" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	entries, err := os.ReadDir(paths.Quarantine)
	if err != nil || len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), ".operator.json") {
		t.Fatalf("operator audit entries=%v err=%v", entries, err)
	}
}

func TestControlPlaneOperatorQuarantinesCorruptStatusUsingLiveStateIdentity(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("target"))
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return targetActivationHealth(req) })
	if err := os.Remove(paths.Request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Status, []byte("{corrupt\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := quarantineControlPlaneUpgrade(activator, req.JobID, "operator verified corrupt status against live state"); err != nil {
		t.Fatal(err)
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "quarantined" || status.JobID != req.JobID || status.ContainerID != req.ContainerID {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	entries, err := os.ReadDir(paths.Quarantine)
	if err != nil || len(entries) != 1 {
		t.Fatalf("operator audit entries=%v err=%v", entries, err)
	}
	data, err := os.ReadFile(filepath.Join(paths.Quarantine, entries[0].Name()))
	if err != nil || !strings.Contains(string(data), `"status_reconstructed":true`) {
		t.Fatalf("operator audit=%s err=%v", data, err)
	}
}

func TestControlPlaneOperatorRetryRejectsPermanentFailure(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("target"))
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return targetActivationHealth(req) })
	if err := activator.writeStatus(req, "failed", 1, "identity failed", "permanent", nil); err != nil {
		t.Fatal(err)
	}

	err := retryControlPlaneUpgrade(context.Background(), activator, req.JobID)
	if err == nil || !strings.Contains(err.Error(), "not a retryable error") {
		t.Fatalf("permanent retry error=%v", err)
	}
}

func TestControlPlaneOperatorRetryRejectsUnsafeSystemctlWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	runner := &activationRunner{}
	req := writeActivationFixture(t, paths, []byte("target-binary"))
	installed := []byte("installed-binary")
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], installed, 0755); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, runner, func() Health { return targetActivationHealth(req) })
	activator.systemctl = ""
	if err := activator.writeStatus(req, "failed", 1, "temporary failure", "transient", nil); err != nil {
		t.Fatal(err)
	}
	requestBefore, err := os.ReadFile(paths.Request)
	if err != nil {
		t.Fatal(err)
	}
	statusBefore, err := os.ReadFile(paths.Status)
	if err != nil {
		t.Fatal(err)
	}

	err = retryControlPlaneUpgrade(context.Background(), activator, req.JobID)
	if err == nil || !strings.Contains(err.Error(), "systemctl command is not safely configured") {
		t.Fatalf("unsafe systemctl retry error=%v", err)
	}
	for path, expected := range map[string][]byte{
		paths.Request: requestBefore,
		paths.Status:  statusBefore,
		paths.InstalledAssets[controlPlaneAssetDeployer]: installed,
	} {
		actual, readErr := os.ReadFile(path)
		if readErr != nil || string(actual) != string(expected) {
			t.Fatalf("retry mutated %s: data=%q err=%v", path, actual, readErr)
		}
	}
	if runner.restartCount() != 0 {
		t.Fatalf("unsafe systemctl retry restarted the deployer %d times", runner.restartCount())
	}
	if _, err := os.Lstat(paths.Lock); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsafe systemctl retry created the activation lock: %v", err)
	}
}

func TestControlPlaneOperatorRetryRequiresHealthyMatchingApplication(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("same-binary"))
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("same-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, &activationRunner{}, func() Health {
		health := targetActivationHealth(req)
		health.ActiveContainerID = strings.Repeat("e", 64)
		return health
	})
	if err := activator.writeStatus(req, "failed", 1, "temporary failure", "transient", nil); err != nil {
		t.Fatal(err)
	}

	err := retryControlPlaneUpgrade(context.Background(), activator, req.JobID)
	if err == nil || !strings.Contains(err.Error(), "live deployer health") {
		t.Fatalf("mismatched live health retry error=%v", err)
	}
	status, statusErr := readControlPlaneStatus(paths.Status)
	if statusErr != nil || status.Status != "failed" || status.Attempt != 1 {
		t.Fatalf("retry changed status despite failed preflight: status=%+v err=%v", status, statusErr)
	}
}

func TestControlPlaneOperatorRetriesTransientFailure(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("same-binary"))
	if err := os.WriteFile(paths.InstalledAssets[controlPlaneAssetDeployer], []byte("same-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return targetActivationHealth(req) })
	if err := activator.writeStatus(req, "failed", req.MaxAttempts, "temporary failure", "transient", nil); err != nil {
		t.Fatal(err)
	}

	if err := retryControlPlaneUpgrade(context.Background(), activator, req.JobID); err != nil {
		t.Fatal(err)
	}
	status, err := readControlPlaneStatus(paths.Status)
	if err != nil || status.Status != "succeeded" || status.Attempt != 1 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestControlPlaneOperatorRecoveryRespectsActivationLock(t *testing.T) {
	dir := t.TempDir()
	paths := testActivationPaths(dir)
	req := writeActivationFixture(t, paths, []byte("same-binary"))
	activator := newTestActivator(paths, &activationRunner{}, func() Health { return targetActivationHealth(req) })
	if err := activator.writeStatus(req, "failed", 1, "temporary failure", "transient", nil); err != nil {
		t.Fatal(err)
	}
	lock, acquired, err := acquireActivationLock(paths.Lock, os.Geteuid(), os.Getegid())
	if err != nil || !acquired {
		t.Fatalf("hold activation lock: acquired=%v err=%v", acquired, err)
	}
	defer func() { _ = lock.Close() }()

	err = retryControlPlaneUpgrade(context.Background(), activator, req.JobID)
	if err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("contended operator retry error=%v", err)
	}
	status, statusErr := readControlPlaneStatus(paths.Status)
	if statusErr != nil || status.Status != "failed" || status.Attempt != 1 {
		t.Fatalf("contended retry mutated status: status=%+v err=%v", status, statusErr)
	}
}

func testActivationPaths(dir string) controlPlaneActivationPaths {
	stateDir := filepath.Join(dir, "state")
	installed := filepath.Join(dir, "usr", "local", "sbin", "sub2api-deployer")
	if err := os.MkdirAll(filepath.Dir(installed), 0700); err != nil {
		panic(err)
	}
	if err := os.Chmod(filepath.Dir(installed), 0755); err != nil {
		panic(err)
	}
	return controlPlaneActivationPaths{
		Request:     filepath.Join(stateDir, "control-plane-upgrade.json"),
		Status:      filepath.Join(stateDir, "control-plane-upgrade.json.status"),
		State:       filepath.Join(stateDir, "state.json"),
		StagingRoot: filepath.Join(stateDir, "control-plane-staging"),
		Quarantine:  filepath.Join(stateDir, "quarantine"),
		Lock:        filepath.Join(stateDir, "control-plane-activation.lock"),
		InstalledAssets: map[string]string{
			controlPlaneAssetDeployer: installed,
		},
	}
}

func writeActivationFixture(t *testing.T, paths controlPlaneActivationPaths, target []byte) controlPlaneActivationRequest {
	t.Helper()
	jobID := "activation-job-0001"
	stage := filepath.Join(paths.StagingRoot, jobID)
	if err := os.MkdirAll(stage, 0700); err != nil {
		t.Fatal(err)
	}
	stagedBinary := filepath.Join(stage, controlPlaneAssetDeployer)
	manifest := filepath.Join(stage, "CONTROL-PLANE-MANIFEST.json")
	if err := os.WriteFile(stagedBinary, target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("manifest\n"), 0644); err != nil {
		t.Fatal(err)
	}
	containerID := strings.Repeat("b", 64)
	digest := "sha256:" + strings.Repeat("c", 64)
	req := controlPlaneActivationRequest{
		Schema:               controlPlaneRequestSchema,
		PayloadSchema:        controlPlanePayloadSchema,
		JobID:                jobID,
		ContainerID:          containerID,
		ContainerName:        "sub2api-green",
		TargetVersion:        "0.1.168-ts.3",
		ExpectedImage:        "ghcr.io/ssharkkky/sub2api@" + digest,
		ExpectedImageDigest:  digest,
		StagedManifest:       manifest,
		StagedManifestSHA256: sha256Digest([]byte("manifest\n")),
		ExpectedCommit:       strings.Repeat("d", 40),
		ExpectedArch:         runtime.GOARCH,
		MaxAttempts:          defaultActivationAttempts,
		CreatedAt:            time.Now().UTC(),
		Assets: []controlPlaneActivationAsset{{
			Type:       controlPlaneAssetDeployer,
			StagedPath: stagedBinary,
			SHA256:     sha256Digest(target),
			Owner:      0,
			Group:      0,
			Mode:       0755,
		}},
	}
	writeActivationRequest(t, paths.Request, req)
	writePendingActivationStatus(t, paths.Status, req)
	state := State{
		ActiveContainer:   req.ContainerName,
		ActiveContainerID: req.ContainerID,
		ActiveVersion:     req.TargetVersion,
		ActiveImage:       req.ExpectedImage,
		Job:               &Job{ID: req.JobID, Status: JobStatusSucceeded},
	}
	stateData, _ := json.Marshal(state)
	if err := os.WriteFile(paths.State, append(stateData, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	return req
}

func writePendingActivationStatus(t *testing.T, path string, req controlPlaneActivationRequest) {
	t.Helper()
	now := time.Now().UTC()
	status := controlPlaneUpgradeStatus{
		Schema:        controlPlaneStatusSchema,
		JobID:         req.JobID,
		ContainerID:   req.ContainerID,
		TargetVersion: req.TargetVersion,
		Status:        "pending",
		MaxAttempts:   req.MaxAttempts,
		UpdatedAt:     &now,
	}
	data, _ := json.Marshal(status)
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func writeActivationRequest(t *testing.T, path string, req controlPlaneActivationRequest) {
	t.Helper()
	data, _ := json.Marshal(req)
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func newTestActivator(paths controlPlaneActivationPaths, runner CommandRunner, health func() Health) *controlPlaneActivator {
	return &controlPlaneActivator{
		runner:       runner,
		systemctl:    "/bin/systemctl",
		paths:        paths,
		now:          time.Now,
		sleep:        func(time.Duration) {},
		expectedUID:  os.Geteuid(),
		expectedGID:  os.Getegid(),
		healthClient: healthClientForTest(health),
		files:        defaultControlPlaneFileOps(),
	}
}

func healthClientForTest(health func() Health) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		data, _ := json.Marshal(health())
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
	})}
}

func targetActivationHealth(req controlPlaneActivationRequest) Health {
	return Health{
		Status:                   "ok",
		ActiveContainerID:        req.ContainerID,
		ActiveVersion:            req.TargetVersion,
		ControlPlaneUpgradeReady: true,
		ControlPlane: ControlPlaneCapabilities{
			Activator:        "go-v1",
			PayloadSchemaMin: controlPlanePayloadSchema,
			PayloadSchemaMax: controlPlanePayloadSchema,
		},
		Build: BuildInfo{
			Version: req.TargetVersion,
			Commit:  req.ExpectedCommit,
			Type:    "release",
			Arch:    req.ExpectedArch,
			SHA256:  req.Assets[0].SHA256,
		},
	}
}
