package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountSparkShadowHelpers(t *testing.T) {
	pid := int64(100)
	normal := &Account{ID: 100}
	require.False(t, normal.IsShadow())
	require.False(t, normal.IsCredentialShadow())
	require.Equal(t, QuotaDimensionGlobal, normal.QuotaDimensionOrDefault())
	shadow := &Account{ID: 200, ParentAccountID: &pid, QuotaDimension: QuotaDimensionSpark}
	require.True(t, shadow.IsShadow())
	require.True(t, shadow.IsCredentialShadow())
	require.Equal(t, QuotaDimensionSpark, shadow.QuotaDimensionOrDefault())
}

func TestCodexUsageSnapshotOwnerID(t *testing.T) {
	parentID := int64(100)

	require.Zero(t, codexUsageSnapshotOwnerID(nil))
	require.Equal(t, int64(101), codexUsageSnapshotOwnerID(&Account{ID: 101}))
	require.Equal(t, parentID, codexUsageSnapshotOwnerID(&Account{
		ID:              102,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionLinked,
	}))
	require.Zero(t, codexUsageSnapshotOwnerID(&Account{
		ID:              103,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
	}))
}
