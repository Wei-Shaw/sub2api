package service

import (
	"context"
	"path/filepath"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestDataManagementService_GetAgentHealth_Deprecated(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(t.TempDir(), "unused.sock")
	svc := NewDataManagementServiceWithOptions(socketPath, 0)
	health := svc.GetAgentHealth(context.Background())

	require.False(t, health.Enabled)
	require.Equal(t, DataManagementDeprecatedReason, health.Reason)
	require.Equal(t, socketPath, health.SocketPath)
	require.Nil(t, health.Agent)
}

func TestDataManagementService_EnsureAgentEnabled_Deprecated(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(t.TempDir(), "unused.sock")
	svc := NewDataManagementServiceWithOptions(socketPath, 100)
	err := svc.EnsureAgentEnabled(context.Background())
	require.Error(t, err)

	statusCode, status := infraerrors.ToHTTP(err)
	require.Equal(t, 503, statusCode)
	require.Equal(t, DataManagementDeprecatedReason, status.Reason)
	require.Equal(t, socketPath, status.Metadata["socket_path"])
}
