package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestShouldStartAutonomousBackgroundServices(t *testing.T) {
	require.True(t, shouldStartAutonomousBackgroundServices(nil))
	require.True(t, shouldStartAutonomousBackgroundServices(&config.Config{
		Deployment: config.DeploymentConfig{Role: config.DeploymentRoleStandalone},
	}))
	require.True(t, shouldStartAutonomousBackgroundServices(&config.Config{
		Deployment: config.DeploymentConfig{Role: config.DeploymentRoleMaster},
	}))
	require.False(t, shouldStartAutonomousBackgroundServices(&config.Config{
		Deployment: config.DeploymentConfig{Role: config.DeploymentRoleSlave},
	}))
}
