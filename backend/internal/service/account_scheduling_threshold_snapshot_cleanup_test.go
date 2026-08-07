//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type missingThresholdSnapshotCleanerRepo struct {
	AccountRepository
REDACTED

func TestClearAccountSchedulingThresholdSnapshots_RequiresRepositorySupport(t *testing.T) {
	err := clearAccountSchedulingThresholdSnapshots(context.Background(), missingThresholdSnapshotCleanerRepo{REDACTED, 1)

REDACTED
	require.Contains(t, err.Error(), "does not support account scheduling threshold snapshot cleanup")
REDACTED
