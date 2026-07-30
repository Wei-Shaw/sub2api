//go:build integration

package deployer

import (
	"archive/tar"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRealDockerControlPlaneStagingRejectsSymlinkWithoutChangingHostTarget(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	probe := exec.Command("docker", "info")
	if output, err := probe.CombinedOutput(); err != nil {
		t.Skipf("docker daemon is unavailable: %v (%s)", err, strings.TrimSpace(string(output)))
	}

	stateDir := t.TempDir()
	if err := os.Chmod(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	hostTarget := filepath.Join(t.TempDir(), "must-not-be-chmodded")
	if err := os.WriteFile(hostTarget, []byte("host data"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := []byte("{}\n")
	rootfsTar := filepath.Join(t.TempDir(), "rootfs.tar")
	writeControlPlaneSymlinkRootFS(t, rootfsTar, hostTarget, manifest)

	commit := strings.Repeat("a", 40)
	importCtx, cancelImport := context.WithTimeout(context.Background(), time.Minute)
	defer cancelImport()
	command := exec.CommandContext(importCtx, "docker", "import",
		"--change", "LABEL org.opencontainers.image.revision="+commit,
		"--change", "LABEL "+controlPlaneProtocolLabel+"="+controlPlaneImageProtocolV1,
		"--change", "LABEL "+controlPlaneManifestDigestLabel+"="+sha256Digest(manifest),
		rootfsTar,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("import symlink test image: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	imageID := strings.TrimSpace(string(output))
	defer exec.Command("docker", "image", "rm", "--force", imageID).Run()

	cfg := testConfig(t, 18082)
	cfg.DockerBinary = "docker"
	cfg.StatePath = filepath.Join(stateDir, "state.json")
	cfg.ControlPlaneUpgradePath = filepath.Join(stateDir, "control-plane-upgrade.json")
	job := testControlPlaneJob(cfg)
	job.TargetImage = imageID
	job.TargetDigest = imageID
	manager := &Manager{cfg: cfg, runner: ExecRunner{}, now: time.Now}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if _, _, err := manager.stageControlPlaneUpgrade(ctx, job); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("symlinked deployer payload was not rejected: %v", err)
	}
	info, err := os.Stat(hostTarget)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("host symlink target mode changed to %04o", info.Mode().Perm())
	}
}

func writeControlPlaneSymlinkRootFS(t *testing.T, path, hostTarget string, manifest []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(file)
	for _, name := range []string{"opt/", "opt/sub2api-control-plane/"} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: 0755}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.WriteHeader(&tar.Header{Name: "opt/sub2api-control-plane/CONTROL-PLANE-MANIFEST.json", Typeflag: tar.TypeReg, Mode: 0644, Size: int64(len(manifest))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(manifest); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{
		Name:     "opt/sub2api-control-plane/sub2api-deployer",
		Typeflag: tar.TypeSymlink,
		Mode:     0777,
		Linkname: hostTarget,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRealDockerControlPlaneStaging(t *testing.T) {
	image := os.Getenv("SUB2API_CONTROL_PLANE_TEST_IMAGE")
	version := os.Getenv("SUB2API_CONTROL_PLANE_TEST_VERSION")
	commit := os.Getenv("SUB2API_CONTROL_PLANE_TEST_COMMIT")
	if image == "" || version == "" || commit == "" {
		t.Skip("real Docker candidate identity is not configured")
	}
	if !strings.Contains(image, "@sha256:") {
		t.Fatal("integration image must be pinned by digest")
	}

	runner := ExecRunner{}
	pullCtx, cancelPull := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelPull()
	if _, err := runner.Run(pullCtx, nil, "docker", "pull", "--platform", "linux/"+runtime.GOARCH, image); err != nil {
		t.Fatalf("pull immutable integration image: %v", err)
	}

	cfg := testConfig(t, 18081)
	cfg.DockerBinary = "docker"
	cfg.LoadedFrom = ""
	stateDir := t.TempDir()
	if err := os.Chmod(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	cfg.StatePath = filepath.Join(stateDir, "state.json")
	cfg.ControlPlaneUpgradePath = filepath.Join(stateDir, "control-plane-upgrade.json")
	job := &Job{
		ID:                   "real-docker-control-plane-stage",
		Action:               "update",
		TargetVersion:        version,
		TargetImage:          image,
		TargetDigest:         image[strings.LastIndex(image, "@")+1:],
		CandidateContainer:   "sub2api-green",
		CandidateContainerID: strings.Repeat("f", 64),
		Status:               JobStatusRunning,
		Stage:                StageActivating,
	}
	manager := &Manager{cfg: cfg, runner: runner, now: time.Now}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	first, legacy, err := manager.stageControlPlaneUpgrade(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if legacy || first == nil || first.Commit != commit {
		t.Fatalf("unexpected real Docker stage: stage=%+v legacy=%v", first, legacy)
	}
	second, legacy, err := manager.stageControlPlaneUpgrade(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if legacy || second == nil || first.BinarySHA256 != second.BinarySHA256 || first.ManifestSHA != second.ManifestSHA {
		t.Fatalf("real Docker idempotent stage changed: first=%+v second=%+v", first, second)
	}

	manager.state = State{
		ActiveContainer:   job.CandidateContainer,
		ActiveContainerID: job.CandidateContainerID,
		ActiveVersion:     job.TargetVersion,
		ActiveImage:       job.TargetImage,
		Job:               job,
	}
	if err := saveState(cfg.StatePath, manager.state); err != nil {
		t.Fatal(err)
	}
	prepared, activationLock, err := manager.prepareControlPlaneUpgrade(job)
	if err != nil || !prepared || activationLock == nil {
		t.Fatalf("prepare real Docker activation: prepared=%v lock=%v err=%v", prepared, activationLock, err)
	}
	if err := manager.complete(job.ID, "Deployment completed", ""); err != nil {
		t.Fatal(err)
	}
	if err := activationLock.Close(); err != nil {
		t.Fatal(err)
	}

	paths := defaultControlPlaneActivationPaths(cfg)
	installedDir := filepath.Join(stateDir, "host", "usr", "local", "sbin")
	if err := os.MkdirAll(installedDir, 0755); err != nil {
		t.Fatal(err)
	}
	installedPath := filepath.Join(installedDir, controlPlaneAssetDeployer)
	if err := os.WriteFile(installedPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	paths.InstalledAssets = map[string]string{controlPlaneAssetDeployer: installedPath}
	activationRunner := &activationRunner{}
	activator := newTestActivator(paths, activationRunner, func() Health { return Health{} })
	request, err := activator.readRequest()
	if err != nil {
		t.Fatal(err)
	}
	activator.healthClient = healthClientForTest(func() Health { return targetActivationHealth(request) })
	expectedBinary, err := os.ReadFile(second.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := activator.activate(ctx); err != nil {
		t.Fatal(err)
	}
	installedBinary, err := os.ReadFile(installedPath)
	if err != nil || string(installedBinary) != string(expectedBinary) {
		t.Fatalf("activated real Docker binary mismatch: err=%v", err)
	}
	if activationRunner.restartCount() != 1 {
		t.Fatalf("real Docker activation restart count=%d", activationRunner.restartCount())
	}
	if err := activator.activate(ctx); err != nil {
		t.Fatal(err)
	}
	if activationRunner.restartCount() != 1 {
		t.Fatalf("no-request rerun restarted deployer: %d", activationRunner.restartCount())
	}
}
