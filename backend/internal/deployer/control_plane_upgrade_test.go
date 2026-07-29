package deployer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type controlPlaneRunner struct {
	base          *fakeRunner
	manifest      []byte
	binary        []byte
	labels        map[string]string
	versionOutput string
	createErr     error
	copyErr       error
	created       int
	removed       int
}

func newControlPlaneRunner(t *testing.T, base *fakeRunner, version, commit string) *controlPlaneRunner {
	t.Helper()
	binary := []byte("verified-deployer-binary")
	manifest := controlPlaneManifest{
		Schema:  1,
		Version: version,
		Commit:  commit,
		RuntimePayload: map[string]controlPlaneRuntimePayload{
			"linux/" + runtime.GOARCH: {
				Path:   controlPlaneBinaryPath,
				SHA256: sha256Digest(binary),
			},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return &controlPlaneRunner{
		base:     base,
		manifest: manifestBytes,
		binary:   binary,
		labels: map[string]string{
			"org.opencontainers.image.source":        "https://github.com/ssharkkky/sub2api",
			"org.opencontainers.image.revision":      commit,
			"io.tokensupply.sub2api.update-protocol": "2",
			controlPlaneProtocolLabel:                controlPlaneProtocolV1,
			controlPlaneManifestDigestLabel:          sha256Digest(manifestBytes),
		},
		versionOutput: fmt.Sprintf("Sub2API Deployer %s (commit: %s, built: 2026-07-29T00:00:00Z, type: release, arch: %s)", version, commit, runtime.GOARCH),
	}
}

func (r *controlPlaneRunner) Run(ctx context.Context, env map[string]string, name string, args ...string) (string, error) {
	if name == "docker" && len(args) >= 4 && args[0] == "image" && args[1] == "inspect" && args[2] == "--format" && args[3] == "{{json .Config.Labels}}" {
		encoded, err := json.Marshal(r.labels)
		return string(encoded), err
	}
	if name == "docker" && len(args) > 0 && args[0] == "create" {
		r.created++
		if r.createErr != nil {
			return "", r.createErr
		}
		return strings.Repeat("e", 64), nil
	}
	if name == "docker" && len(args) == 3 && args[0] == "cp" {
		if r.copyErr != nil {
			return "", r.copyErr
		}
		var content []byte
		switch {
		case strings.HasSuffix(args[1], controlPlaneManifestPath):
			content = r.manifest
		case strings.HasSuffix(args[1], controlPlaneBinaryPath):
			content = r.binary
		default:
			return "", fmt.Errorf("unexpected docker cp source %q", args[1])
		}
		return "", os.WriteFile(args[2], content, 0600)
	}
	if name == "docker" && len(args) == 3 && args[0] == "rm" && args[1] == "--force" {
		r.removed++
		return "", nil
	}
	if strings.HasSuffix(name, "/sub2api-deployer") && len(args) == 1 && args[0] == "--version" {
		return r.versionOutput, nil
	}
	return r.base.Run(ctx, env, name, args...)
}

func TestStageControlPlaneUpgradeVerifiesAndPublishesPayload(t *testing.T) {
	cfg := testConfig(t, 18081)
	cfg.ControlPlaneUpgradePath = filepath.Join(t.TempDir(), "control-plane-upgrade.json")
	job := testControlPlaneJob(cfg)
	runner := newControlPlaneRunner(t, &fakeRunner{}, job.TargetVersion, "0123456789abcdef")
	manager := &Manager{cfg: cfg, runner: runner}

	stage, legacy, err := manager.stageControlPlaneUpgrade(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if legacy || stage == nil {
		t.Fatalf("stage=%+v legacy=%v", stage, legacy)
	}
	if stage.BinarySHA256 != sha256Digest(runner.binary) || stage.ManifestSHA != sha256Digest(runner.manifest) {
		t.Fatalf("unexpected staged identity: %+v", stage)
	}
	if _, err := os.Stat(stage.BinaryPath); err != nil {
		t.Fatalf("staged binary missing: %v", err)
	}
	if runner.created != 1 || runner.removed != 1 {
		t.Fatalf("temporary container lifecycle created=%d removed=%d", runner.created, runner.removed)
	}
}

func TestStageControlPlaneUpgradeSkipsOnlyLegacyImage(t *testing.T) {
	cfg := testConfig(t, 18081)
	cfg.ControlPlaneUpgradePath = filepath.Join(t.TempDir(), "control-plane-upgrade.json")
	job := testControlPlaneJob(cfg)
	runner := newControlPlaneRunner(t, &fakeRunner{}, job.TargetVersion, "0123456789abcdef")
	delete(runner.labels, controlPlaneProtocolLabel)
	delete(runner.labels, controlPlaneManifestDigestLabel)
	manager := &Manager{cfg: cfg, runner: runner}

	stage, legacy, err := manager.stageControlPlaneUpgrade(context.Background(), job)
	if err != nil || !legacy || stage != nil {
		t.Fatalf("stage=%+v legacy=%v err=%v", stage, legacy, err)
	}
	if runner.created != 0 {
		t.Fatalf("legacy image unexpectedly created extraction container")
	}
}

func TestStageControlPlaneUpgradeRejectsInvalidProtocolOneMetadata(t *testing.T) {
	cfg := testConfig(t, 18081)
	cfg.ControlPlaneUpgradePath = filepath.Join(t.TempDir(), "control-plane-upgrade.json")
	job := testControlPlaneJob(cfg)
	runner := newControlPlaneRunner(t, &fakeRunner{}, job.TargetVersion, "0123456789abcdef")
	delete(runner.labels, controlPlaneManifestDigestLabel)
	manager := &Manager{cfg: cfg, runner: runner}

	_, legacy, err := manager.stageControlPlaneUpgrade(context.Background(), job)
	if err == nil || legacy || !strings.Contains(err.Error(), "invalid manifest digest label") {
		t.Fatalf("legacy=%v err=%v", legacy, err)
	}
}

func TestStageControlPlaneUpgradeCleansUpAfterCopyFailure(t *testing.T) {
	cfg := testConfig(t, 18081)
	cfg.ControlPlaneUpgradePath = filepath.Join(t.TempDir(), "control-plane-upgrade.json")
	job := testControlPlaneJob(cfg)
	runner := newControlPlaneRunner(t, &fakeRunner{}, job.TargetVersion, "0123456789abcdef")
	runner.copyErr = errors.New("injected copy failure")
	manager := &Manager{cfg: cfg, runner: runner}

	if _, _, err := manager.stageControlPlaneUpgrade(context.Background(), job); err == nil {
		t.Fatal("copy failure unexpectedly succeeded")
	}
	if runner.created != 1 || runner.removed != 1 {
		t.Fatalf("temporary container leaked: created=%d removed=%d", runner.created, runner.removed)
	}
	entries, err := os.ReadDir(filepath.Join(filepath.Dir(cfg.ControlPlaneUpgradePath), "control-plane-staging"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary staging files leaked: %+v", entries)
	}
}

func TestStageControlPlaneUpgradeReusesIdenticalPublishedStage(t *testing.T) {
	cfg := testConfig(t, 18081)
	cfg.ControlPlaneUpgradePath = filepath.Join(t.TempDir(), "control-plane-upgrade.json")
	job := testControlPlaneJob(cfg)
	runner := newControlPlaneRunner(t, &fakeRunner{}, job.TargetVersion, "0123456789abcdef")
	manager := &Manager{cfg: cfg, runner: runner}

	first, _, err := manager.stageControlPlaneUpgrade(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := manager.stageControlPlaneUpgrade(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if first.Directory != second.Directory || first.BinarySHA256 != second.BinarySHA256 {
		t.Fatalf("recovered stage differs: first=%+v second=%+v", first, second)
	}
	if runner.created != 2 || runner.removed != 2 {
		t.Fatalf("temporary container lifecycle created=%d removed=%d", runner.created, runner.removed)
	}
}

func testControlPlaneJob(cfg Config) *Job {
	return &Job{
		ID:                   "control-plane-upgrade-0001",
		Action:               "update",
		TargetVersion:        "0.1.168-ts.1",
		TargetImage:          cfg.ImageRepository + "@sha256:" + strings.Repeat("a", 64),
		TargetDigest:         "sha256:" + strings.Repeat("a", 64),
		CandidateContainer:   "sub2api-green",
		CandidateContainerID: fakeContainerID("sub2api-green-current"),
		Status:               JobStatusRunning,
		Stage:                StageActivating,
	}
}
