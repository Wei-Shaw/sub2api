package service

import (
	"context"
	"path/filepath"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestDataManagementService_DeprecatedRPCMethods(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(t.TempDir(), "datamanagement.sock")
	svc := NewDataManagementServiceWithOptions(socketPath, 0)

	_, err := svc.GetConfig(context.Background())
	assertDeprecatedDataManagementError(t, err, socketPath)

	_, err = svc.CreateBackupJob(context.Background(), DataManagementCreateBackupJobInput{BackupType: "full"})
	assertDeprecatedDataManagementError(t, err, socketPath)

	err = svc.DeleteS3Profile(context.Background(), "s3-default")
	assertDeprecatedDataManagementError(t, err, socketPath)
}

func assertDeprecatedDataManagementError(t *testing.T, err error, socketPath string) {
	t.Helper()

	require.Error(t, err)
	statusCode, status := infraerrors.ToHTTP(err)
	require.Equal(t, 503, statusCode)
	require.Equal(t, DataManagementDeprecatedReason, status.Reason)
	require.Equal(t, socketPath, status.Metadata["socket_path"])
}
