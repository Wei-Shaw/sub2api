//go:build unit

package service

import (
	"context"
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
	github := &updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v0.1.165-ts.1"}}
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
	require.Equal(t, "0.1.164-ts.1", deployer.started.ExpectedCurrentVersion)
	require.Equal(t, "sysop-request-1", deployer.started.RequestID)
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
