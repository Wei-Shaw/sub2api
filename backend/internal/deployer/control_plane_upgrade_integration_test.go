//go:build integration

package deployer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

	cfg := testConfig(t, 18081)
	cfg.DockerBinary = "docker"
	cfg.LoadedFrom = ""
	cfg.ControlPlaneUpgradePath = filepath.Join(t.TempDir(), "control-plane-upgrade.json")
	job := &Job{
		ID:            "real-docker-control-plane-stage",
		Action:        "update",
		TargetVersion: version,
		TargetImage:   image,
		Status:        JobStatusRunning,
		Stage:         StageActivating,
	}
	manager := &Manager{cfg: cfg, runner: ExecRunner{}}
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
}
