//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type deploymentClientStub struct {
	health    *DeploymentHealth
	healthErr error
	started   DeploymentRequest
	job       *DeploymentJob
}

func (s *deploymentClientStub) Health(context.Context) (*DeploymentHealth, error) {
	return s.health, s.healthErr
}

func (s *deploymentClientStub) Start(_ context.Context, req DeploymentRequest) (*DeploymentJob, error) {
	s.started = req
	return s.job, nil
}

func (s *deploymentClientStub) Job(context.Context, string) (*DeploymentJob, error) {
	return s.job, nil
}

func (s *deploymentClientStub) CurrentJob(context.Context) (*DeploymentJob, error) {
	return s.job, nil
}

func TestManagedUpdateStartsDigestDeploymentJob(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	github := &updateServiceGitHubClientStub{
		release: &GitHubRelease{
			TagName: "v0.1.165-ts.1",
			Assets: []GitHubAsset{{
				Name: releaseCompletionAsset, BrowserDownloadURL: "https://github.com/ssharkkky/sub2api/releases/download/v0.1.165-ts.1/" + releaseCompletionAsset,
			}},
		},
		checksumData: testCompletionLedger(t, "0.1.165-ts.1", digest),
	}
	deployer := &deploymentClientStub{
		health: &DeploymentHealth{Status: "ok", ControlPlaneUpgradeReady: true},
		job:    &DeploymentJob{ID: "job-1", Status: "running"},
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, github, "0.1.164-ts.1", "release")
	svc.ConfigureDeployment(DeploymentModeDockerManaged, deployer)

	job, err := svc.StartManagedUpdate(context.Background(), "sysop-request-1")

	require.NoError(t, err)
	require.Equal(t, "job-1", job.ID)
	require.Equal(t, "update", deployer.started.Action)
	require.Equal(t, "0.1.165-ts.1", deployer.started.TargetVersion)
	require.Equal(t, digest, deployer.started.ExpectedTargetDigest)
	require.Equal(t, "0.1.164-ts.1", deployer.started.ExpectedCurrentVersion)
	require.Equal(t, "sysop-request-1", deployer.started.RequestID)
}

func TestManagedUpdateRejectsMissingCompletionLedger(t *testing.T) {
	github := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v0.1.165-ts.1"}}
	deployer := &deploymentClientStub{
		health: &DeploymentHealth{Status: "ok", ControlPlaneUpgradeReady: true},
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, github, "0.1.164-ts.1", "release")
	svc.ConfigureDeployment(DeploymentModeDockerManaged, deployer)

	_, err := svc.StartManagedUpdate(context.Background(), "sysop-request-ledger")

	require.ErrorContains(t, err, "no completion ledger")
	require.Empty(t, deployer.started.Action)
}

func testCompletionLedger(t *testing.T, version, digest string) []byte {
	t.Helper()
	assetSHA := "sha256:" + strings.Repeat("b", 64)
	data, err := json.Marshal(map[string]any{
		"schema":                        3,
		"tag":                           "v" + version,
		"commit":                        strings.Repeat("c", 40),
		"tag_object":                    strings.Repeat("d", 40),
		"image":                         "ghcr.io/ssharkkky/sub2api:" + version,
		"image_digest":                  digest,
		"immutable_image":               "ghcr.io/ssharkkky/sub2api:" + version + "@" + digest,
		"dockerhub_image":               nil,
		"dockerhub_image_digest":        nil,
		"dockerhub_immutable_image":     nil,
		"architectures":                 []string{"amd64", "arm64"},
		"control_plane_manifest_sha256": assetSHA,
		"candidate_manifest_sha256":     assetSHA,
		"deployer_checksums_sha256":     assetSHA,
		"deployer_assets": map[string]string{
			"sub2api-deployer-linux-amd64":        assetSHA,
			"sub2api-deployer-linux-arm64":        assetSHA,
			"sub2api-deployer-linux-amd64.tar.gz": assetSHA,
			"sub2api-deployer-linux-arm64.tar.gz": assetSHA,
		},
	})
	require.NoError(t, err)
	return data
}

func TestManagedRollbackRejectsVersionOutsideReleaseAllowlist(t *testing.T) {
	github := &updateServiceGitHubClientStub{recentReleases: []*GitHubRelease{{TagName: "v0.1.163-ts.1"}}}
	deployer := &deploymentClientStub{health: &DeploymentHealth{Status: "ok", ControlPlaneUpgradeReady: true}}
	svc := NewUpdateService(&updateServiceCacheStub{}, github, "0.1.164-ts.1", "release")
	svc.ConfigureDeployment(DeploymentModeDockerManaged, deployer)

	_, err := svc.StartManagedRollback(context.Background(), "0.1.100-ts.1", "sysop-request-2")

	require.ErrorIs(t, err, ErrRollbackVersionNotAllowed)
	require.Empty(t, deployer.started.Action)
}

func TestCheckUpdateReportsManagedDeployerReadiness(t *testing.T) {
	github := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v0.1.165-ts.1"}}
	deployer := &deploymentClientStub{health: &DeploymentHealth{Status: "ok", ControlPlaneUpgradeReady: true}}
	svc := NewUpdateService(&updateServiceCacheStub{}, github, "0.1.164-ts.1", "release")
	svc.ConfigureDeployment(DeploymentModeDockerManaged, deployer)

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.Equal(t, DeploymentModeDockerManaged, info.DeploymentMode)
	require.True(t, info.DeploymentReady)
}

func TestManagedUpdateRequiresHostBootstrapCapability(t *testing.T) {
	github := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v0.1.165-ts.1"}}
	deployer := &deploymentClientStub{health: &DeploymentHealth{Status: "ok"}}
	svc := NewUpdateService(&updateServiceCacheStub{}, github, "0.1.164-ts.1", "release")
	svc.ConfigureDeployment(DeploymentModeDockerManaged, deployer)

	info, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.False(t, info.DeploymentReady)
	require.Contains(t, info.DeploymentMessage, "one-time host deployer bootstrap")

	_, err = svc.StartManagedUpdate(context.Background(), "sysop-request-bootstrap")
	require.ErrorIs(t, err, ErrManagedDeployerBootstrapRequired)
	require.Empty(t, deployer.started.Action)
}

func TestDockerManualModeNeverFallsBackToBinaryReplacement(t *testing.T) {
	github := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v0.1.165-ts.1"}}
	svc := NewUpdateService(&updateServiceCacheStub{}, github, "0.1.164-ts.1", "release")
	svc.ConfigureDeployment(DeploymentModeDockerManual, nil)

	err := svc.PerformUpdate(context.Background())

	require.ErrorIs(t, err, ErrDockerManualUpdate)
}
